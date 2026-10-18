import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";
import { type FormEvent, type KeyboardEvent, type ReactNode, useState } from "react";
import { useAnalytics } from "@/api/analytics";
import { useCost, useReadiness } from "@/api/ops";
import { useAlerts } from "@/api/reports";
import { useRuns } from "@/api/runs";
import { useSession } from "@/api/session";
import { useInbox } from "@/api/supervisor";
import { StatusMark, type StatusTone } from "@/components/status";
import { dayAgo, greeting, plural, summaryLine } from "@/lib/desk";
import { formatMoney, NOT_AVAILABLE } from "@/lib/format";

// The desk (design language, 09): the greeting and one summary line, the
// cards that earned their place today, and the question in the middle.
// Every figure comes from an API that already exists; a missing one says
// "Not available".
export function HomePage() {
  const session = useSession();
  const inbox = useInbox();
  const alerts = useAlerts("open");
  const ready = useReadiness();
  const runs = useRuns();
  const cost = useCost();
  const analytics = useAnalytics({
    from: dayAgo(30),
    to: dayAgo(1),
    group: "campaign",
    compare: "previous",
    platform: "",
  });

  const name = session.data?.user?.name ?? null;
  const waiting = inbox.data ? inbox.data.items.length : null;
  const openAlerts = alerts.data ? alerts.data.alerts.length : null;
  const jobs = runs.data
    ? runs.data.runs.filter((r) => r.state === "queued" || r.state === "running").length
    : null;
  const server = ready.error
    ? "unreachable"
    : ready.data
      ? ready.data.ready
        ? "ready"
        : "not ready"
      : "checking";
  const failedChecks = ready.data?.checks.filter((c) => !c.ok).map((c) => c.name);

  const summary = analytics.data?.summary.metrics;
  const freshness = analytics.data?.freshness;
  const figuresTone: StatusTone = analytics.error
    ? "error"
    : !freshness || freshness.lastSuccessAt === null
      ? "pending"
      : freshness.partial
        ? "caution"
        : "ready";
  const figuresWhy = analytics.error
    ? analytics.error.message
    : !freshness
      ? "Loading."
      : freshness.lastSuccessAt === null
        ? "Figures never read: no ad account is connected."
        : freshness.partial
          ? `Partial read, missing ${freshness.missingPeriods.join(", ")}.`
          : `Observed through ${freshness.observedThrough}.`;
  const currency = summary?.spend.currency || "AED";
  const leadsChange = analytics.data?.summary.changes?.leads;
  const leadsWhy = summary
    ? leadsChange && !leadsChange.undefined && leadsChange.relative !== null
      ? `${leadsChange.relative >= 0 ? "Up" : "Down"} ${Math.abs(Math.round(leadsChange.relative * 100))}% on the 30 days before. ${figuresWhy}`
      : `${plural(analytics.data?.coverage.matchedCount ?? 0, "lead", "leads")} matched to a campaign. ${figuresWhy}`
    : figuresWhy;
  const cpl = summary?.costPerLead;
  const cplFigure = !summary
    ? null
    : cpl?.undefined
      ? "No leads"
      : cpl?.value === null || cpl?.value === undefined
        ? null
        : formatMoney(Math.round(cpl.value), currency);
  const cplWhy = summary
    ? summary.spend.minor === null
      ? `Spend not available. ${figuresWhy}`
      : `${formatMoney(summary.spend.minor, currency)} spent in 30 days. ${figuresWhy}`
    : figuresWhy;

  const waitingTone: StatusTone = inbox.error
    ? "error"
    : waiting === null
      ? "pending"
      : waiting > 0
        ? "caution"
        : "ready";
  const waitingWhy = inbox.error
    ? inbox.error.message
    : waiting === null
      ? "Loading."
      : waiting === 0
        ? openAlerts
          ? `Nothing in the inbox. ${plural(openAlerts, "open alert", "open alerts")}.`
          : "Nothing in the inbox. No open alerts."
        : `${inbox.data?.items[0]?.title ?? ""}${openAlerts ? ` ${plural(openAlerts, "open alert", "open alerts")}.` : ""}`;

  const usage = cost.data?.workspace;
  const tokensUsed = usage ? usage.inputTokens + usage.outputTokens : null;
  const modelTone: StatusTone = cost.error
    ? "error"
    : !usage
      ? "pending"
      : usage.exhausted
        ? "error"
        : "ready";
  const modelWhy = cost.error
    ? cost.error.message
    : !usage
      ? "Loading."
      : `${usage.remaining === null ? NOT_AVAILABLE : usage.remaining.toLocaleString("en")} of ${usage.budget.toLocaleString("en")} tokens left today. ${
          cost.data?.pricing
            ? `Cost ${usage.costMicros === null ? NOT_AVAILABLE : formatMoney(Math.round(usage.costMicros / 10_000), "USD")}.`
            : "Cost not available: no pricing set."
        }`;

  return (
    <div className="space-y-8">
      <header className="space-y-1.5">
        <h1>{greeting(new Date().getHours(), name)}</h1>
        <p className="text-base leading-6 text-muted-foreground">
          {summaryLine({ waiting, openAlerts, jobs, server, failedChecks })}
        </p>
      </header>

      <section
        aria-label="Today"
        className="grid grid-cols-1 gap-3 min-[640px]:grid-cols-2 min-[1024px]:grid-cols-3 min-[1400px]:grid-cols-4"
      >
        <MetricCard
          door="Perform"
          tone={figuresTone}
          title="Leads, 30 days"
          why={leadsWhy}
          figure={summary ? summary.leads.toLocaleString("en") : null}
          ask="What happened to leads in the last 30 days?"
          action={{ to: "/analytics", label: "Analytics" }}
        />
        <MetricCard
          door="Perform"
          tone={figuresTone}
          title="Cost per lead, 30 days"
          why={cplWhy}
          figure={cplFigure}
          ask="Why did cost per lead change?"
          action={{ to: "/analytics", label: "Analytics" }}
        />
        <MetricCard
          door="Agents"
          tone={waitingTone}
          title="Waiting on you"
          why={waitingWhy}
          figure={waiting === null ? null : waiting.toLocaleString("en")}
          ask="What is waiting on me?"
          action={{ to: "/supervisor", label: "Supervisor" }}
        />
        <MetricCard
          door="Workspace"
          tone={modelTone}
          title="Model use today"
          why={modelWhy}
          figure={tokensUsed === null ? null : tokensUsed.toLocaleString("en")}
          unit="tokens"
          ask="How much has the model been used today?"
          action={{ to: "/ops", label: "Ops" }}
        />
      </section>

      <Composer />
    </div>
  );
}

// 264px tall so a row lines up: door label with a status mark, a one-line
// title, a two-line why, the figure pinned to the bottom, and a footer with
// "Talk about this" and at most one action.
function MetricCard({
  door,
  tone,
  title,
  why,
  figure,
  unit,
  ask,
  action,
}: {
  door: string;
  tone: StatusTone;
  title: string;
  why: string;
  figure: string | null;
  unit?: string;
  ask: string;
  action: { to: string; label: string };
}) {
  return (
    <article className="flex h-[264px] flex-col border bg-card p-4">
      <p className="eyebrow flex items-center gap-1.5">
        <StatusMark tone={tone} />
        {door}
      </p>
      <h2 className="mt-2 truncate">{title}</h2>
      <p className="mt-1.5 line-clamp-2 text-sm leading-5 text-muted-foreground">{why}</p>
      <p className="mt-auto flex items-baseline gap-2">
        {figure === null ? (
          <span className="font-heading text-2xl font-semibold tracking-tight text-muted-foreground">
            {NOT_AVAILABLE}
          </span>
        ) : (
          <>
            <span className="font-heading text-[44px] leading-none font-semibold tracking-[-0.03em] tabular-nums">
              {figure}
            </span>
            {unit && <span className="text-sm text-muted-foreground">{unit}</span>}
          </>
        )}
      </p>
      <footer className="mt-4 flex items-center justify-between border-t pt-3 text-sm">
        <Link to="/ask" search={{ q: ask }} className="hover:underline underline-offset-4">
          Talk about this
        </Link>
        <Link to={action.to} className="font-medium hover:underline underline-offset-4">
          {action.label}
        </Link>
      </footer>
    </article>
  );
}

const PROMPTS = [
  "What changed since yesterday?",
  "Why did cost per lead change?",
  "Who should sales call first today?",
  "Are we on pace this month?",
];

// The centre of the home screen, 768px wide. One line of text is exactly the
// 40px send button's height; the button sits at 40% until there is text.
function Composer() {
  const navigate = useNavigate();
  const [text, setText] = useState("");

  function ask(q: string) {
    const trimmed = q.trim();
    if (!trimmed) return;
    navigate({ to: "/ask", search: { q: trimmed } });
  }

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    ask(text);
  }

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      ask(text);
    }
  }

  return (
    <section aria-labelledby="ask-title" className="mx-auto w-full max-w-[768px] space-y-3">
      <h2 id="ask-title" className="text-2xl leading-8">
        Ask ADX
      </h2>
      <form
        onSubmit={onSubmit}
        className="flex items-end gap-2 border border-input bg-card p-2 transition-colors hover:border-ring focus-within:border-foreground focus-within:hover:border-foreground"
      >
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
          rows={1}
          placeholder="Ask about any brand, figure or lead, or tell ADX what to do"
          aria-label="Ask ADX"
          className="max-h-40 min-h-10 flex-1 resize-none bg-transparent px-2 py-2 text-base leading-6 outline-none placeholder:text-muted-foreground"
        />
        <button
          type="submit"
          disabled={!text.trim()}
          aria-label="Ask"
          className="flex size-10 shrink-0 items-center justify-center bg-primary text-primary-foreground hover:bg-primary/80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:cursor-default disabled:opacity-40 active:not-disabled:translate-y-px"
        >
          <ArrowRight size={16} strokeWidth={2} aria-hidden="true" />
        </button>
      </form>
      <ul className="flex flex-wrap gap-2">
        {PROMPTS.map((p) => (
          <li key={p}>
            <Pill onClick={() => ask(p)}>{p}</Pill>
          </li>
        ))}
      </ul>
    </section>
  );
}

function Pill({ children, onClick }: { children: ReactNode; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="h-8 border px-3 text-sm transition-colors hover:border-foreground/40 hover:bg-muted focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
    >
      {children}
    </button>
  );
}
