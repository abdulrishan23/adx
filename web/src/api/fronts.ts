// Home, the Inbox and Present are composed fronts: the server decides what
// earns a place and in what order, and the web app only draws the cards it
// is given. The cards come from GET /api/fronts and GET /api/inbox, built in
// core/fronts from the alerts, the supervisor's inbox, the figures, plans and
// their launch, runs and the review board. Per-door fronts come back in
// phases 2 and 3, when Create and Convert have something to compose.

import { useQuery } from "@tanstack/react-query";
import type { RoutePath } from "@/app/nav";
import type { Status } from "@/components/ui/status-chip";
import { NOT_AVAILABLE } from "@/lib/format";
import { getJSON } from "./client";
import type {
  Card as CardWire,
  Desk as DeskWire,
  Inbox as InboxWire,
  Picture,
  Template as TemplateWire,
  WorkItem,
} from "./types.gen";

export type { Counts, WorkItem } from "./types.gen";

export type Door = "studio" | "perform" | "convert";

// Where a block's action goes. Only plain paths, so a front never has to know
// a screen's search params.
export type Action = { label: string; to: RoutePath; primary?: boolean };

// Why a block is on the front: a status and one sentence. Shown above the title
// so the reader knows what put it there before reading it.
export type Why = { status: Status; text: string };

export type ListItem = {
  status: Status;
  title: string;
  detail?: string;
  tag?: string;
  action?: Action;
};

export type BarRow = { label: string; value: string; share: number };

export type Block =
  | { kind: "list"; title: string; why: Why; items: ListItem[]; action?: Action }
  | { kind: "bars"; title: string; why: Why; values: number[]; labels: string[]; note?: string }
  | { kind: "hbars"; title: string; why: Why; rows: BarRow[]; note?: string };

// The picture beside a headline.
export type HeadlineChart = {
  kind: "pace";
  rowsLabel: string;
  rows: BarRow[];
  trend: number[];
  trendLabel: string;
};

// The one thing that matters today, in big type. Analytics builds one from
// its own figures; a server-composed front will carry its own.
export type Headline = {
  status: Status;
  figure: string;
  count?: number;
  unit?: string;
  title: string;
  text: string;
  chart: HeadlineChart;
  actions: Action[];
};

// Evidence is the picture a card carries: a figure, bars, the ads in
// question, the leads waiting.
export type Evidence =
  | { kind: "pace"; rows: BarRow[]; trend: number[] }
  | { kind: "bars"; values: number[]; labels: string[] }
  | { kind: "figure"; figure: string; count?: number; unit?: string; detail: string }
  | {
      kind: "ads";
      projectId: number;
      items: { code: string; status: Status; note: string }[];
    }
  | { kind: "leads"; items: { id: number; name: string; detail: string; waited: string }[] }
  | { kind: "list"; items: ListItem[] }
  // Labelled bars without a trend: the terms, ads or forms behind a figure.
  | { kind: "rows"; rows: BarRow[] }
  | {
      kind: "table";
      columns: { label: string; numeric?: boolean }[];
      rows: { cells: string[]; leadId?: number }[];
    };

// Home's cards: whatever deserves attention today, from any door, each a
// figure or a picture with a question to put to Adx about it. The server
// picks and orders them; a card that stops mattering stops appearing.
export type Card = {
  id: string;
  door: Door;
  status: Status;
  // A card needs a person when something is wrong or waiting on them; the
  // rest are updates: work Adx finished, figures worth a look. The server
  // decides which is which.
  needsYou: boolean;
  title: string;
  // One line on why it's here today.
  why: string;
  // A missing figure says "Not available" and never counts up.
  figure?: { value: string; count?: number; unit?: string; change?: string };
  evidence?: Evidence | { kind: "pie"; slices: { label: string; value: number }[] };
  // What "Talk about this" asks.
  question: string;
  action?: Action;
  // When it arrived, so the Inbox can list newest first.
  at: string;
  // Set when the card comes from an alert check, so the Inbox can snooze or
  // dismiss it the way the check expects.
  alertId?: string;
};

export const needsYou = (c: Card) => c.needsYou;

// Ready-to-fire prompts, the way a marketer would ask them.
// Icons a template can carry. The web app maps each name to its glyph.
export const TEMPLATE_ICONS = [
  "changes",
  "cost",
  "waste",
  "call",
  "pace",
  "compare",
  "budget",
  "exclude",
  "revive",
  "reasons",
  "sources",
  "forms",
  "launch",
  "fatigue",
  "review",
  "plan",
  "report",
  "summary",
] as const;
export type TemplateIcon = (typeof TEMPLATE_ICONS)[number];

export type Template = {
  id: string;
  door: Door | "any";
  icon: TemplateIcon;
  title: string;
  description: string;
  prompt: string;
};

export type HomeDesk = {
  checkedAt: string;
  // One line under the greeting: what Adx checked and what needs you.
  summary: string;
  cards: Card[];
  working: WorkItem[];
  templates: Template[];
};

export type InboxShow = "all" | "needs" | "updates";

export type InboxPage = {
  summary: string;
  counts: { all: number; needsYou: number; updates: number };
  cards: Card[];
  working: WorkItem[];
};

// pictureOf narrows the server's one picture shape to the union the
// components draw. An unknown kind draws nothing rather than crashing.
function pictureOf(p: Picture | null): Card["evidence"] {
  if (!p) return undefined;
  switch (p.kind) {
    case "bars":
      return { kind: "bars", values: p.values ?? [], labels: p.labels ?? [] };
    case "pie":
      return { kind: "pie", slices: p.slices ?? [] };
    case "rows":
      return { kind: "rows", rows: p.rows ?? [] };
    case "pace":
      return { kind: "pace", rows: p.rows ?? [], trend: p.trend ?? [] };
    case "leads":
      return { kind: "leads", items: p.leads ?? [] };
    case "ads":
      return { kind: "ads", projectId: p.projectId ?? 0, items: p.ads ?? [] };
    default:
      return undefined;
  }
}

export function toCard(c: CardWire): Card {
  const card: Card = {
    id: c.id,
    door: c.door === "any" ? "perform" : c.door,
    status: c.status,
    needsYou: c.needsYou,
    title: c.title,
    why: c.why,
    question: c.question,
    at: c.at,
  };
  if (c.figure) {
    card.figure =
      c.figure.value === null
        ? { value: NOT_AVAILABLE }
        : {
            value: c.figure.value,
            count: c.figure.count ?? undefined,
            unit: c.figure.unit || undefined,
            change: c.figure.change || undefined,
          };
  }
  const evidence = pictureOf(c.picture);
  if (evidence) card.evidence = evidence;
  if (c.action) {
    card.action = {
      label: c.action.label,
      to: c.action.to as RoutePath,
      primary: c.action.primary,
    };
  }
  if (c.alertId !== null) card.alertId = String(c.alertId);
  return card;
}

const isIcon = (s: string): s is TemplateIcon => (TEMPLATE_ICONS as readonly string[]).includes(s);

function toTemplate(t: TemplateWire): Template {
  return { ...t, icon: isIcon(t.icon) ? t.icon : "changes" };
}

export function useHomeDesk() {
  return useQuery({
    queryKey: ["home-desk"],
    queryFn: async (): Promise<HomeDesk> => {
      const d = await getJSON<DeskWire>("/api/fronts");
      return { ...d, cards: d.cards.map(toCard), templates: d.templates.map(toTemplate) };
    },
    refetchInterval: 60_000,
  });
}

export function useInbox(show: InboxShow, door: Door | "any") {
  return useQuery({
    queryKey: ["home-desk", "inbox", show, door],
    queryFn: async (): Promise<InboxPage> => {
      const q = new URLSearchParams({ show, door });
      const i = await getJSON<InboxWire>(`/api/inbox?${q}`);
      return { ...i, cards: i.cards.map(toCard) };
    },
    refetchInterval: 60_000,
  });
}
