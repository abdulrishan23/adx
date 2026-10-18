import { Link } from "@tanstack/react-router";
import { ArrowRight, ChevronDown } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { useAlerts, useBrief, useConnections, useDismissAlert, useSnoozeAlert } from "@/api/adx";
import type {
  Alert,
  Brief,
  BriefProject,
  Connection,
  LeadHandling,
  Pacing,
  SourceRow,
} from "@/api/models";
import { Meter } from "@/components/charts";
import { ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { SourceName } from "@/components/platform";
import { Button } from "@/components/ui/button";
import { StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { analyticsSearch } from "@/lib/analytics";
import {
  type Change,
  change,
  formatChange,
  formatDate,
  formatMoneyShort,
  formatNumber,
  formatPercent,
  formatRelative,
  NOT_AVAILABLE,
} from "@/lib/format";
import { cn } from "@/lib/utils";

export function BriefPage() {
  const brief = useBrief();
  return (
    <>
      <PageHeader
        title="Brief"
        subtitle={
          brief.data
            ? `${formatDate(brief.data.date, { weekday: "long", day: "numeric", month: "long" })}, against the 7 days before`
            : undefined
        }
      />
      {brief.isPending && <LoadingRows rows={8} />}
      {brief.error && <ErrorNote error={brief.error} />}
      {brief.data && <BriefBody brief={brief.data} />}
    </>
  );
}

// The Brief answers the five things a marketer decides each morning, in the
// same order every day. Each answer is a status, one sentence and one action;
// the detail underneath opens on its own when the answer is "act".
//
// Thresholds live here until they move to Guardrails (#34).
const THRESHOLD = {
  budgetWatch: 0.05, // heading this far over or under the monthly budget
  budgetAct: 0.1,
  costWatch: 0.08, // cost per qualified lead up this much on the week before
  costAct: 0.15,
  leadsDrop: 0.25, // leads yesterday this far below a usual day
  waitedWatch: 1, // leads untouched for over an hour
  waitedAct: 10,
};

const BROKEN_CHECKS = new Set(["Sync gap", "Delivery", "Landing page", "Status coverage"]);

type Verdict = "fine" | "watch" | "act";
const VERDICT_MARK: Record<Verdict, "ready" | "caution" | "error"> = {
  fine: "ready",
  watch: "caution",
  act: "error",
};
const VERDICT_LABEL: Record<Verdict, string> = { fine: "Fine", watch: "Watch", act: "Act" };

type Answer = {
  key: string;
  question: string;
  verdict: Verdict;
  sentence: string;
  note?: string; // a quieter second line of context
  action?: {
    label: string;
    to: "/proposals" | "/leads" | "/campaigns" | "/settings/connections" | "/inbox";
  };
  detail: ReactNode;
};

// The five answers in three panels: what to fix, what the money is doing, and
// whether sales is following up.
const PANELS = [
  { key: "fix", title: "Fix today", answers: ["broken", "approvals"] },
  { key: "money", title: "Money", answers: ["budget", "leads"] },
  { key: "sales", title: "Sales follow-up", answers: ["sales"] },
] as const;

const RANK: Record<Verdict, number> = { fine: 0, watch: 1, act: 2 };

function BriefBody({ brief }: { brief: Brief }) {
  const alerts = useAlerts();
  const connections = useConnections();
  const answers = useAnswers(brief, alerts.data ?? [], connections.data ?? []);
  const toAct = answers.filter((a) => a.verdict === "act").length;
  const [openKey, setOpenKey] = useState<string | null>(null);
  const opened = answers.find((a) => a.key === openKey);
  // On a phone the detail sits below every panel, so bring it into view.
  useEffect(() => {
    if (openKey) {
      document
        .getElementById(`detail-${openKey}`)
        ?.scrollIntoView({ behavior: "smooth", block: "nearest" });
    }
  }, [openKey]);
  return (
    <div className="space-y-6">
      <p className="font-heading text-xl font-medium">
        {toAct === 0
          ? "Nothing needs you this morning."
          : `${toAct} ${toAct === 1 ? "thing needs" : "things need"} you this morning.`}
      </p>
      <div className="grid gap-4 lg:grid-cols-3">
        {PANELS.map((panel) => {
          const items = panel.answers
            .map((k) => answers.find((a) => a.key === k))
            .filter((a): a is Answer => a !== undefined);
          const worst = items.reduce<Verdict>(
            (w, a) => (RANK[a.verdict] > RANK[w] ? a.verdict : w),
            "fine",
          );
          return (
            <section key={panel.key} className="flex flex-col border bg-card">
              <header className="flex items-center justify-between gap-3 border-b px-5 py-3.5">
                <h2 className="font-heading text-base font-semibold">{panel.title}</h2>
                <span className="flex items-center gap-2 font-mono text-[0.65rem] tracking-wider uppercase">
                  <StatusMark status={VERDICT_MARK[worst]} />
                  {VERDICT_LABEL[worst]}
                </span>
              </header>
              <ul className="flex flex-1 flex-col divide-y">
                {items.map((a) => (
                  <AnswerItem
                    key={a.key}
                    answer={a}
                    open={openKey === a.key}
                    onToggle={() => setOpenKey((k) => (k === a.key ? null : a.key))}
                  />
                ))}
              </ul>
            </section>
          );
        })}
      </div>
      {opened && (
        <section id={`detail-${opened.key}`} className="space-y-4 border bg-muted/30 p-4 sm:p-5">
          <div className="flex items-center justify-between gap-3">
            <h2 className="font-heading text-base font-semibold">{opened.question}</h2>
            <button
              type="button"
              onClick={() => setOpenKey(null)}
              className="text-sm font-medium underline-offset-4 hover:underline"
            >
              Close
            </button>
          </div>
          {opened.detail}
        </section>
      )}
    </div>
  );
}

function AnswerItem({
  answer: a,
  open,
  onToggle,
}: {
  answer: Answer;
  open: boolean;
  onToggle: () => void;
}) {
  return (
    <li className="flex flex-1 flex-col gap-3 px-5 py-4">
      <div className="flex items-start gap-2.5">
        <StatusMark status={VERDICT_MARK[a.verdict]} className="mt-1.5" />
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">{a.question}</p>
          <p className="mt-0.5 font-medium">{a.sentence}</p>
          {a.note && <p className="mt-1.5 text-sm text-muted-foreground">{a.note}</p>}
        </div>
      </div>
      <div className="mt-auto flex flex-wrap items-center gap-3 pl-[1.125rem]">
        {a.action && (
          <Link
            to={a.action.to}
            className="inline-flex h-8 items-center gap-1.5 border bg-card px-3 text-sm font-medium hover:bg-muted"
          >
            {a.action.label}
            <ArrowRight className="size-3.5" />
          </Link>
        )}
        <button
          type="button"
          aria-expanded={open}
          aria-controls={`detail-${a.key}`}
          onClick={onToggle}
          className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          {open ? "Hide details" : "Details"}
          <ChevronDown className={cn("size-3.5 transition-transform", open && "rotate-180")} />
        </button>
      </div>
    </li>
  );
}

function useAnswers(brief: Brief, alerts: Alert[], connections: Connection[]): Answer[] {
  const broken = connections.filter((c) => c.state === "error");
  const brokenAlerts = alerts.filter((a) => BROKEN_CHECKS.has(a.check) && a.severity !== "low");
  const lowStatus = brief.statusCoverage < 0.6;
  const problems = broken.length + brokenAlerts.length;
  const brokenAnswer: Answer = {
    key: "broken",
    question: "Is anything broken?",
    verdict: problems > 0 ? "act" : lowStatus ? "watch" : "fine",
    sentence:
      problems > 0
        ? [
            ...broken.map((c) => `${c.name} needs reconnecting`),
            ...brokenAlerts.map((a) => a.title),
          ][0] + (problems > 1 ? `, and ${problems - 1} more` : "")
        : lowStatus
          ? `Only ${formatPercent(brief.statusCoverage)} of leads have a status, so qualified counts run low`
          : "Every source is connected and leads are arriving",
    action: broken.length ? { label: "Fix", to: "/settings/connections" } : undefined,
    detail: (
      <>
        <DataHealth brief={brief} />
        {brokenAlerts.length > 0 && (
          <ul className="divide-y border bg-card">
            {brokenAlerts.map((al) => (
              <AlertRow key={al.id} alert={al} />
            ))}
          </ul>
        )}
      </>
    ),
  };

  const budgeted = brief.projects.filter((p) => p.pacing);
  const off = budgeted
    .map((p) => ({
      p,
      off: (p.pacing as Pacing).projectedMinor / (p.pacing as Pacing).budgetMinor - 1,
    }))
    .sort((x, y) => Math.abs(y.off) - Math.abs(x.off));
  const worst = off[0];
  const budgetVerdict: Verdict = !worst
    ? "fine"
    : Math.abs(worst.off) >= THRESHOLD.budgetAct
      ? "act"
      : Math.abs(worst.off) >= THRESHOLD.budgetWatch
        ? "watch"
        : "fine";
  const offCount = off.filter((o) => Math.abs(o.off) >= THRESHOLD.budgetAct).length;
  const budgetAnswer: Answer = {
    key: "budget",
    question: "Am I on budget?",
    verdict: budgetVerdict,
    sentence:
      budgetVerdict === "fine" || !worst
        ? "Every project is on track for its monthly budget"
        : `${worst.p.name} will end the month ${formatPercent(Math.abs(worst.off))} ${worst.off > 0 ? "over" : "under"} budget${offCount > 1 ? `, and ${offCount - 1} more ${offCount - 1 === 1 ? "is" : "are"} off` : ""}`,
    action: budgetVerdict === "fine" ? undefined : { label: "Change budgets", to: "/campaigns" },
    detail: <BudgetList projects={brief.projects} />,
  };

  const s = brief.summary;
  const cost = change(s.costPerQualified.current, s.costPerQualified.previous);
  const leadsDown =
    s.average.leads > 0 && s.yesterday.leads < s.average.leads * (1 - THRESHOLD.leadsDrop);
  const costUp = cost && cost.direction === "up" ? cost.ratio : 0;
  const leadsVerdict: Verdict =
    leadsDown || costUp >= THRESHOLD.costAct
      ? "act"
      : costUp >= THRESHOLD.costWatch
        ? "watch"
        : "fine";
  const leadsAnswer: Answer = {
    key: "leads",
    question: "Are good leads coming in?",
    verdict: leadsVerdict,
    sentence: `${formatNumber(s.yesterday.leads)} leads yesterday (usually ${formatNumber(s.average.leads)}), ${formatNumber(s.yesterday.qualified)} qualified. A qualified lead cost ${formatMoneyShort(round(s.costPerQualified.current))} this week, ${weekOnWeek(cost)}`,
    action: leadsVerdict === "fine" ? undefined : { label: "See campaigns", to: "/campaigns" },
    detail: (
      <>
        <FunnelPanel funnel={brief.funnel} />
        <SourcesPanel rows={brief.sources} />
      </>
    ),
  };

  const h = brief.handling;
  const topSource = h.bySource[0];
  const topOwner = h.byOwner[0];
  const salesVerdict: Verdict =
    h.untouchedOverAnHour >= THRESHOLD.waitedAct
      ? "act"
      : h.untouchedOverAnHour >= THRESHOLD.waitedWatch
        ? "watch"
        : "fine";
  const salesAnswer: Answer = {
    key: "sales",
    question: "Is sales calling them?",
    verdict: salesVerdict,
    sentence:
      h.untouched === 0
        ? "Every lead from yesterday has been touched"
        : `${formatNumber(h.untouchedOverAnHour)} of yesterday's ${formatNumber(h.arrived)} leads waited over an hour${topSource ? `, most from ${topSource.source}` : ""}${topOwner ? `, most with ${topOwner.owner}` : ""}`,
    note:
      h.untouched > 0
        ? `${formatNumber(h.untouched)} not touched at all yet. Half of yesterday's leads were first touched within ${h.medianFirstTouchMinutes ?? "?"} minutes.`
        : undefined,
    action: h.untouched > 0 ? { label: "Open leads", to: "/leads" } : undefined,
    detail: <HandlingPanel handling={h} />,
  };

  const decisions = alerts.filter((a) => !BROKEN_CHECKS.has(a.check) && a.severity !== "low");
  const approvalsAnswer: Answer = {
    key: "approvals",
    question: "What do I approve today?",
    verdict: brief.pendingProposals > 0 ? "act" : decisions.length > 0 ? "watch" : "fine",
    sentence:
      brief.pendingProposals > 0
        ? `${brief.pendingProposals} ${brief.pendingProposals === 1 ? "change is" : "changes are"} waiting on you${decisions.length ? `, and ${decisions.length} ${decisions.length === 1 ? "alert" : "alerts"} to look at` : ""}`
        : decisions.length > 0
          ? `${decisions.length} ${decisions.length === 1 ? "alert" : "alerts"} to look at, nothing to approve`
          : "Nothing to approve",
    action: brief.pendingProposals > 0 ? { label: "Review changes", to: "/proposals" } : undefined,
    detail:
      decisions.length > 0 ? (
        <ul className="divide-y border bg-card">
          {decisions.map((al) => (
            <AlertRow key={al.id} alert={al} />
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">No open alerts.</p>
      ),
  };

  return [brokenAnswer, budgetAnswer, leadsAnswer, salesAnswer, approvalsAnswer];
}

// "down 8% on last week", in words, for a sentence.
function weekOnWeek(c: Change | null): string {
  if (!c) return "with no earlier week to compare";
  if (c.direction === "flat") return "the same as last week";
  return `${c.direction === "up" ? "up" : "down"} ${formatPercent(Math.abs(c.ratio))} on last week`;
}

// Each project's month so far against its budget, worst first.
function BudgetList({ projects }: { projects: BriefProject[] }) {
  const rows = [...projects].sort((a, b) => {
    const off = (p: BriefProject) =>
      p.pacing ? Math.abs(p.pacing.projectedMinor / p.pacing.budgetMinor - 1) : -1;
    return off(b) - off(a);
  });
  return (
    <ul className="divide-y border bg-card">
      {rows.map((p) => (
        <li
          key={p.projectId}
          className="grid grid-cols-[8rem_1fr] items-center gap-4 px-5 py-3 text-sm sm:grid-cols-[10rem_1fr_14rem]"
        >
          <span className="font-medium">{p.name}</span>
          <PacingLine pacing={p.pacing} />
        </li>
      ))}
    </ul>
  );
}

function PacingLine({ pacing }: { pacing: Pacing | null }) {
  if (!pacing)
    return (
      <span className="text-xs text-muted-foreground sm:col-span-2">No monthly budget set</span>
    );
  const expected = (pacing.budgetMinor * pacing.dayOfMonth) / pacing.daysInMonth;
  const off = pacing.projectedMinor / pacing.budgetMinor - 1;
  return (
    <>
      <Meter value={pacing.spentMinor} max={pacing.budgetMinor} marker={expected} />
      <span className="col-span-2 text-xs sm:col-span-1 sm:text-right">
        {Math.abs(off) < THRESHOLD.budgetWatch ? (
          <span className="text-muted-foreground">On track · </span>
        ) : (
          <>
            <StatusMark
              status={Math.abs(off) >= THRESHOLD.budgetAct ? "error" : "caution"}
              className="mr-1.5"
            />
            {formatPercent(Math.abs(off))} {off > 0 ? "over" : "under"} ·{" "}
          </>
        )}
        <span className="font-mono text-muted-foreground tabular-nums">
          {formatMoneyShort(pacing.spentMinor)} of {formatMoneyShort(pacing.budgetMinor)}
        </span>
      </span>
    </>
  );
}

// The name column stays put while a wide table scrolls on a phone.
const STICKY = "sticky left-0 z-10 bg-card";

export const round = (n: number | null) => (n === null ? null : Math.round(n));

// A move worth a second look, as a mark beside the figure: green when it's the
// good direction, red when it isn't. Small moves get no mark.
export function MoveMark({ c, better }: { c: Change | null; better: "up" | "down" }) {
  if (!c || Math.abs(c.ratio) < 0.15) return null;
  return <StatusMark status={c.direction === better ? "ready" : "error"} className="mr-1.5" />;
}

const STAGES = [
  { key: "leads", label: "Leads" },
  { key: "qualified", label: "Qualified" },
  { key: "meetings", label: "Meetings" },
  { key: "offers", label: "Offers" },
  { key: "deals", label: "Done-Deals" },
] as const;

// Last 7 days from lead to deal. A stage counts when a lead reaches it, so the
// share between stages mixes this week's leads with older ones.
function FunnelPanel({ funnel }: { funnel: Brief["funnel"] }) {
  const { week, previous } = funnel;
  return (
    <Panel
      title="Funnel · 7 days"
      description="Each stage counts leads that reached it this week, against last week"
      action={
        <Link
          to="/perform/projects"
          search={analyticsSearch({})}
          className="text-sm font-medium underline-offset-4 hover:underline"
        >
          By project
        </Link>
      }
      bodyClassName="p-0"
    >
      <ol className="grid sm:grid-cols-5">
        {STAGES.map((stage, i) => {
          const now = week[stage.key];
          const before = previous[stage.key];
          const c = change(now, before);
          const prev = i > 0 ? week[STAGES[i - 1]?.key ?? "leads"] : null;
          return (
            <li
              key={stage.key}
              className="relative border-b px-5 py-4 last:border-b-0 sm:border-r sm:border-b-0 sm:last:border-r-0"
            >
              <Eyebrow>{stage.label}</Eyebrow>
              <p className="mt-2 font-mono text-2xl font-semibold tabular-nums">
                {formatNumber(now)}
              </p>
              <p className="mt-0.5 font-mono text-xs whitespace-nowrap tabular-nums">
                <MoveMark c={c} better="up" />
                {formatChange(c)}
              </p>
              <p className="mt-3 text-xs text-muted-foreground">
                {now > 0
                  ? `${compact(week.spendMinor / now)} each`
                  : `No ${stage.label.toLowerCase()}`}
              </p>
              {prev !== null && (
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {prev > 0
                    ? `${formatPercent(now / prev)} of ${STAGES[i - 1]?.label.toLowerCase()}`
                    : "·"}
                </p>
              )}
            </li>
          );
        })}
      </ol>
    </Panel>
  );
}

// AED 1.4K rather than AED 1,405, where a cell has no room for more.
export const compact = (minor: number) =>
  `AED ${new Intl.NumberFormat("en", {
    notation: "compact",
    maximumFractionDigits: minor >= 100_000 ? 1 : 0,
  }).format(minor / 100)}`;

function HandlingPanel({ handling: h }: { handling: LeadHandling }) {
  const maxSource = Math.max(1, ...h.bySource.map((s) => s.untouched));
  const maxOwner = Math.max(1, ...h.byOwner.map((s) => s.untouched));
  return (
    <Panel
      title="Leads nobody has touched"
      description="Yesterday's leads whose status hasn't changed since they arrived"
      action={
        <Link to="/leads" className="text-sm font-medium underline-offset-4 hover:underline">
          Open leads
        </Link>
      }
    >
      <div className="grid gap-6 lg:grid-cols-[1fr_1fr_1fr_1fr]">
        <div>
          <p className="font-mono text-3xl font-semibold tabular-nums">
            {formatNumber(h.untouched)}
            <span className="text-base font-normal text-muted-foreground">
              {" "}
              of {formatNumber(h.arrived)}
            </span>
          </p>
          <p className="mt-1 text-sm">
            {h.untouchedOverAnHour > 0 && <StatusMark status="caution" className="mr-1.5" />}
            {formatNumber(h.untouchedOverAnHour)} waited over an hour
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            First touch took{" "}
            {h.medianFirstTouchMinutes === null
              ? NOT_AVAILABLE.toLowerCase()
              : `${h.medianFirstTouchMinutes} min`}{" "}
            in the middle case
          </p>
        </div>
        <BarList
          title="By source"
          rows={h.bySource.map((s) => ({
            label: s.source,
            value: s.untouched,
            max: maxSource,
            source: true,
          }))}
        />
        <BarList
          title="By owner"
          rows={h.byOwner.map((s) => ({ label: s.owner, value: s.untouched, max: maxOwner }))}
        />
        <BarList
          title="Junk · 7 days"
          percent
          rows={h.junk.map((j) => ({
            label: j.source,
            value: j.share ?? 0,
            max: 0.25,
            source: true,
          }))}
        />
      </div>
    </Panel>
  );
}

function BarList({
  title,
  rows,
  percent = false,
}: {
  title: string;
  rows: { label: string; value: number; max: number; source?: boolean }[];
  percent?: boolean;
}) {
  return (
    <div>
      <Eyebrow className="mb-2.5">{title}</Eyebrow>
      <ul className="space-y-2">
        {rows.map((r) => (
          <li
            key={r.label}
            className="grid grid-cols-[8.5rem_1fr_2.5rem] items-center gap-2 text-sm"
          >
            {r.source ? <SourceName name={r.label} /> : <span className="truncate">{r.label}</span>}
            <Meter value={r.value} max={r.max} className="h-1.5" />
            <span className="text-right font-mono text-xs tabular-nums">
              {percent ? formatPercent(r.value) : formatNumber(r.value)}
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function SourcesPanel({ rows }: { rows: SourceRow[] }) {
  return (
    <Panel title="By source · 7 days" bodyClassName="p-0">
      <Table>
        <THead>
          <TR>
            <TH className={STICKY}>Source</TH>
            <TH numeric>Spend</TH>
            <TH numeric>Leads</TH>
            <TH numeric>Qualified</TH>
            <TH numeric>Per qualified</TH>
          </TR>
        </THead>
        <TBody>
          {rows.map((r) => {
            const c = change(r.costPerQualified.current, r.costPerQualified.previous);
            return (
              <TR key={r.source}>
                <TD className={cn(STICKY, "font-medium whitespace-nowrap")}>
                  <SourceName name={r.source} />
                  {r.kind === "portal" && (
                    <span className="ml-1.5 text-xs font-normal text-muted-foreground">portal</span>
                  )}
                </TD>
                <TD numeric>
                  {r.kind === "organic" ? (
                    <span className="text-muted-foreground">No spend</span>
                  ) : (
                    formatMoneyShort(r.spendMinor)
                  )}
                </TD>
                <TD numeric>{formatNumber(r.leads)}</TD>
                <TD numeric>{formatNumber(r.qualified)}</TD>
                <TD numeric className="whitespace-nowrap">
                  {r.kind === "organic" ? (
                    <span className="text-muted-foreground">·</span>
                  ) : (
                    <>
                      <MoveMark c={c} better="down" />
                      {formatMoneyShort(round(r.costPerQualified.current))}
                      <span className="ml-2 text-xs text-muted-foreground">{formatChange(c)}</span>
                    </>
                  )}
                </TD>
              </TR>
            );
          })}
        </TBody>
      </Table>
    </Panel>
  );
}

// Only what's broken: a source that stopped, or too few statuses to trust the
// qualified counts. Setup that's merely unfinished lives in Settings.
function DataHealth({ brief }: { brief: Brief }) {
  const connections = useConnections();
  const broken = (connections.data ?? []).filter((c) => c.state === "error");
  const lowStatus = brief.statusCoverage < 0.6;
  if (broken.length === 0 && !lowStatus) return null;
  return (
    <div className="space-y-1.5 border px-4 py-3 text-sm">
      {broken.map((c) => (
        <p key={c.key}>
          <StatusMark status="error" className="mr-2" />
          {c.name} needs reconnecting, so its figures stopped updating.{" "}
          <Link
            to={
              c.key === "ghl" ? "/settings/connections/gohighlevel" : "/settings/connections/$key"
            }
            params={{ key: c.key }}
            className="font-medium underline-offset-4 hover:underline"
          >
            Fix
          </Link>
        </p>
      ))}
      {lowStatus && (
        <p>
          <StatusMark status="caution" className="mr-2" />
          Only {formatPercent(brief.statusCoverage)} of leads have a status in GoHighLevel, so
          qualified counts run low.{" "}
          <Link to="/data-quality" className="font-medium underline-offset-4 hover:underline">
            See the gaps
          </Link>
        </p>
      )}
    </div>
  );
}

const SEVERITY_CHIP = { high: "error", medium: "caution", low: "pending" } as const;
const SEVERITY_LABEL = { high: "High", medium: "Medium", low: "Low" } as const;

const DISMISS_REASONS = ["Expected", "Handled outside Adx", "Not useful"];

export function AlertRow({ alert, actions = false }: { alert: Alert; actions?: boolean }) {
  return (
    <li className="grid grid-cols-[5rem_1fr] items-start gap-x-4 gap-y-2 px-5 py-4 sm:grid-cols-[5rem_1fr_auto]">
      <StatusChip status={SEVERITY_CHIP[alert.severity]} className="mt-0.5 justify-center">
        {SEVERITY_LABEL[alert.severity]}
      </StatusChip>
      <div className="min-w-0 flex-1 space-y-0.5">
        <p className="font-medium">{alert.title}</p>
        <p className="text-sm text-muted-foreground">{alert.detail}</p>
        <p className="font-mono text-[0.7rem] tracking-wide text-muted-foreground uppercase">
          {alert.check} · {formatRelative(alert.detectedAt)}
        </p>
        {actions && <AlertActions id={alert.id} />}
      </div>
      {alert.proposalId ? (
        <Link
          to="/proposals"
          search={{ open: alert.proposalId }}
          className="col-start-2 text-sm font-medium underline-offset-4 hover:underline sm:col-start-3"
        >
          Review the change
        </Link>
      ) : (
        <span className="col-start-2 text-sm text-muted-foreground sm:col-start-3">
          No change suggested
        </span>
      )}
    </li>
  );
}

export function AlertActions({ id }: { id: string }) {
  const snooze = useSnoozeAlert();
  const dismiss = useDismissAlert();
  const [dismissing, setDismissing] = useState(false);
  const busy = snooze.isPending || dismiss.isPending;
  if (dismissing) {
    return (
      <div className="flex flex-wrap items-center gap-1.5 pt-2">
        <span className="text-xs text-muted-foreground">Dismiss as</span>
        {DISMISS_REASONS.map((r) => (
          <Button
            key={r}
            size="xs"
            variant="outline"
            disabled={busy}
            onClick={() => dismiss.mutate({ id, reason: r })}
          >
            {r}
          </Button>
        ))}
        <Button size="xs" variant="ghost" onClick={() => setDismissing(false)}>
          Cancel
        </Button>
      </div>
    );
  }
  return (
    <div className="-ml-2 flex gap-1.5 pt-2">
      <Button
        size="xs"
        variant="ghost"
        disabled={busy}
        onClick={() => snooze.mutate({ id, days: 7 })}
      >
        Snooze 7 days
      </Button>
      <Button size="xs" variant="ghost" disabled={busy} onClick={() => setDismissing(true)}>
        Dismiss
      </Button>
      {(snooze.error || dismiss.error) && (
        <span className="text-xs text-destructive">{(snooze.error ?? dismiss.error)?.message}</span>
      )}
    </div>
  );
}
