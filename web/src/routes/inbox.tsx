import { Link, useNavigate } from "@tanstack/react-router";
import { ChevronDown, LayoutGrid, List, MessageSquareText } from "lucide-react";
import { useState } from "react";
import { type Card, type Door, needsYou, useHomeDesk, type WorkItem } from "@/api/fronts";
import { CardView, DOOR_LABEL } from "@/components/front/card";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { buttonVariants } from "@/components/ui/button";
import { StatusMark } from "@/components/ui/status-chip";
import { formatRelative } from "@/lib/format";
import { AlertActions } from "./brief";
import { SampleChip } from "./studio/shared";

type View = "cards" | "list";
type Show = "all" | "needs" | "updates";

const VIEW_KEY = "adx.inbox.view";
const readView = (): View => {
  try {
    return localStorage.getItem(VIEW_KEY) === "list" ? "list" : "cards";
  } catch {
    return "cards";
  }
};

const RANK = { error: 0, caution: 1, pending: 2, ready: 3 } as const;

// Things that need a person come first, worst first; then updates, newest
// first. Home's carousel shows the top of the same order.
export function inboxOrder(cards: Card[]): Card[] {
  return [...cards].sort((a, b) => {
    const n = Number(needsYou(b)) - Number(needsYou(a));
    if (n !== 0) return n;
    const r = RANK[a.status] - RANK[b.status];
    if (r !== 0) return r;
    return b.at.localeCompare(a.at);
  });
}

// Inbox holds every card Adx has put in front of you: what needs you
// (approvals, blocks, anomalies) and what Adx finished. Home shows the top of
// it. Work still running is a count you can open, not a feed to watch.
export function InboxPage() {
  const desk = useHomeDesk();
  const navigate = useNavigate();
  const ask = (q: string) => navigate({ to: "/ask", search: { q } });
  const [view, setView] = useState<View>(readView);
  const [show, setShow] = useState<Show>("all");
  const [door, setDoor] = useState<Door | "any">("any");

  const pickView = (v: View) => {
    setView(v);
    try {
      localStorage.setItem(VIEW_KEY, v);
    } catch {
      // Private windows refuse storage; the choice lasts for this visit.
    }
  };

  const all = desk.data ? inboxOrder(desk.data.cards) : [];
  const needs = all.filter(needsYou);
  const shown = all
    .filter((c) => (show === "needs" ? needsYou(c) : show === "updates" ? !needsYou(c) : true))
    .filter((c) => door === "any" || c.door === door);

  return (
    <>
      <PageHeader
        title="Inbox"
        subtitle={
          desk.data && (
            <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
              {desk.data.summary}
              {desk.data.sample && <SampleChip />}
            </span>
          )
        }
        actions={
          <Segmented
            label="View"
            value={view}
            onChange={pickView}
            options={[
              { value: "cards", label: "Cards", icon: LayoutGrid },
              { value: "list", label: "List", icon: List },
            ]}
          />
        }
      />
      {desk.error && <ErrorNote error={desk.error} />}
      {desk.isPending && <LoadingRows rows={6} />}
      {desk.data && (
        <div className="space-y-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex flex-wrap items-center gap-3">
              <Segmented
                label="Show"
                value={show}
                onChange={setShow}
                options={[
                  { value: "all", label: "All", count: all.length },
                  { value: "needs", label: "Needs you", count: needs.length, tone: "caution" },
                  { value: "updates", label: "Updates", count: all.length - needs.length },
                ]}
              />
              <Segmented
                label="Door"
                value={door}
                onChange={setDoor}
                options={[
                  { value: "any", label: "Every door" },
                  { value: "studio", label: "Projects" },
                  { value: "perform", label: "Perform" },
                  { value: "convert", label: "Convert" },
                ]}
              />
            </div>
            {desk.data.working.length > 0 && <Working items={desk.data.working} />}
          </div>

          {shown.length === 0 ? (
            <Panel>
              <EmptyState title="Nothing here">
                {show === "needs"
                  ? "Nothing needs you right now."
                  : "No cards match these filters."}
              </EmptyState>
            </Panel>
          ) : view === "cards" ? (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
              {shown.map((c, i) => (
                <CardView key={c.id} card={c} delay={i * 40} onAsk={ask} />
              ))}
            </div>
          ) : (
            <ul className="divide-y border bg-card">
              {shown.map((c) => (
                <Row key={c.id} card={c} onAsk={ask} />
              ))}
            </ul>
          )}
        </div>
      )}
    </>
  );
}

function Row({ card: c, onAsk }: { card: Card; onAsk: (q: string) => void }) {
  return (
    <li className="grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-4 gap-y-2 px-5 py-4 transition-colors hover:bg-muted/40 md:grid-cols-[auto_minmax(0,1fr)_9rem_6rem_auto] md:items-center">
      <StatusMark status={c.status} className="mt-1.5 md:mt-0" />
      <div className="min-w-0">
        <p className="font-mono text-[0.6rem] tracking-widest text-muted-foreground uppercase">
          {DOOR_LABEL[c.door]}
        </p>
        <p className="truncate font-medium">{c.title}</p>
        <p className="line-clamp-1 text-sm text-muted-foreground">{c.why}</p>
        {c.alertId && <AlertActions id={c.alertId} />}
      </div>
      <p className="col-start-2 font-heading text-xl font-semibold tabular-nums md:col-start-auto md:text-right">
        {c.figure ? (
          <>
            {c.figure.value}
            {c.figure.unit && (
              <span className="ml-1.5 font-sans text-xs font-medium text-muted-foreground">
                {c.figure.unit}
              </span>
            )}
          </>
        ) : (
          <span className="text-sm font-normal text-muted-foreground">—</span>
        )}
      </p>
      <p className="col-start-2 font-mono text-xs text-muted-foreground md:col-start-auto md:text-right">
        {formatRelative(c.at)}
      </p>
      <div className="col-start-2 flex items-center gap-2 md:col-start-auto md:justify-end">
        <button
          type="button"
          onClick={() => onAsk(c.question)}
          className={buttonVariants({ variant: "ghost", size: "sm" })}
        >
          <MessageSquareText /> Talk about this
        </button>
        {c.action && (
          <Link
            to={c.action.to}
            className={buttonVariants({
              variant: c.action.primary ? "default" : "outline",
              size: "sm",
            })}
          >
            {c.action.label}
          </Link>
        )}
      </div>
    </li>
  );
}

// Working is Adx's running jobs, folded away. Nobody needs to watch them; the
// result arrives as a card.
function Working({ items }: { items: WorkItem[] }) {
  return (
    <details className="group relative">
      <summary className="flex cursor-pointer list-none items-center gap-2 text-sm text-muted-foreground hover:text-foreground">
        <span className="relative flex size-1.5">
          <span className="absolute inline-flex size-full animate-ping bg-foreground/50 motion-reduce:hidden" />
          <span className="relative inline-flex size-1.5 bg-foreground" />
        </span>
        Adx is working on {items.length} {items.length === 1 ? "thing" : "things"}
        <ChevronDown className="size-4 transition-transform group-open:rotate-180" />
      </summary>
      <ul className="absolute right-0 z-20 mt-2 w-80 divide-y border bg-popover shadow-lg">
        {items.map((w) => (
          <li key={w.id}>
            <Link to={w.to} className="block px-4 py-3 hover:bg-muted">
              <span className="block text-sm font-medium">{w.title}</span>
              <span className="block text-xs text-muted-foreground">{w.detail}</span>
            </Link>
          </li>
        ))}
      </ul>
    </details>
  );
}
