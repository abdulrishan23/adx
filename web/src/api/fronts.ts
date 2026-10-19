// Home is a composed front: the server decides what earns a place and in what
// order, and the web app only draws the blocks it is given. The same block
// model tells Home as a Focus story on a wall. Per-door fronts come back in
// phases 2 and 3, when Create and Convert have something to compose. Until
// the Go side has a metrics engine, api/mock/fronts.ts plays the server.

import { useQuery } from "@tanstack/react-query";
import type { RoutePath } from "@/app/nav";
import type { Status } from "@/components/ui/status-chip";
import { mockResponse } from "./mock";
import * as mock from "./mock/fronts";

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
  title: string;
  // One line on why it's here today.
  why: string;
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

// A card needs a person when something is wrong or waiting on them; the rest
// are updates: work Adx finished, figures worth a look.
export const needsYou = (c: Card) => c.status === "error" || c.status === "caution";

// Ready-to-fire prompts, the way a marketer would ask them.
// Icons a template can carry. The web app maps each name to its glyph.
export type TemplateIcon =
  | "changes"
  | "cost"
  | "waste"
  | "call"
  | "pace"
  | "compare"
  | "budget"
  | "exclude"
  | "revive"
  | "reasons"
  | "sources"
  | "forms"
  | "launch"
  | "fatigue"
  | "review"
  | "plan"
  | "report"
  | "summary";

export type Template = {
  id: string;
  door: Door | "any";
  icon: TemplateIcon;
  title: string;
  description: string;
  prompt: string;
};

// Work Adx has under way. Nobody needs to watch it; the Inbox shows a count
// and what each job is, and the result arrives as a card when it's done.
export type WorkItem = { id: string; title: string; detail: string; to: RoutePath };

export type HomeDesk = {
  checkedAt: string;
  // One line under the greeting: what Adx checked and what needs you.
  summary: string;
  // True while the desk is drawn from sample data rather than connected
  // accounts. Home says so, so sample figures never pass for real ones.
  sample: boolean;
  cards: Card[];
  working: WorkItem[];
  templates: Template[];
};

export function useHomeDesk() {
  return useQuery({ queryKey: ["home-desk"], queryFn: () => mockResponse(mock.desk) });
}
