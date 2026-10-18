import { getRouteApi, Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { useState } from "react";
import {
  type Plan,
  type PlanStatus,
  useConfirmAssumption,
  useDecidePlan,
  usePlan,
  usePlans,
} from "@/api/create";
import { CountUp } from "@/components/front/count-up";
import { HBars } from "@/components/front/pictures";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatDate, formatMoneyShort, formatNumber, formatRelative } from "@/lib/format";
import { SampleChip } from "./shared";

const STATUS: Record<
  PlanStatus,
  { chip: "ready" | "caution" | "pending" | "error"; label: string }
> = {
  draft: { chip: "pending", label: "Draft" },
  waiting: { chip: "caution", label: "Waiting for approval" },
  approved: { chip: "ready", label: "Approved" },
  superseded: { chip: "pending", label: "Superseded" },
};
const PLATFORM = {
  meta: "Meta",
  google: "Google",
  snapchat: "Snapchat",
  tiktok: "TikTok",
} as const;
const shortDate = (d: string) => formatDate(`${d}T00:00:00Z`, { day: "numeric", month: "short" });

// A media plan is the bible for a campaign: budget by channel and week, the
// assumptions behind it, and the targets. Adx drafts one from the brand and
// the research; a person approves it, and launch starts from there.
export function PlansPage() {
  const plans = usePlans();
  return (
    <>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            Media plans <SampleChip phase={2} />
          </span>
        }
        subtitle="Adx drafts a plan from the brand and the research. Approving one starts the launch."
      />
      {plans.isPending && <LoadingRows rows={5} />}
      {plans.error && <ErrorNote error={plans.error} />}
      {plans.data && (
        <Panel bodyClassName="p-0">
          <Table>
            <THead>
              <TR>
                <TH>Plan</TH>
                <TH>Status</TH>
                <TH>Runs</TH>
                <TH numeric>Budget</TH>
                <TH numeric>Expected qualified</TH>
                <TH numeric>Per qualified</TH>
                <TH>Changed</TH>
              </TR>
            </THead>
            <TBody>
              {plans.data.map((p) => (
                <TR
                  key={p.id}
                  className={p.status === "superseded" ? "text-muted-foreground" : undefined}
                >
                  <TD>
                    <Link
                      to="/create/plans/$planId"
                      params={{ planId: p.id }}
                      className="font-medium hover:underline"
                    >
                      {p.projectName} · v{p.version}
                    </Link>
                  </TD>
                  <TD>
                    <StatusChip status={STATUS[p.status].chip}>{STATUS[p.status].label}</StatusChip>
                  </TD>
                  <TD className="whitespace-nowrap">
                    {shortDate(p.from)} – {shortDate(p.to)}
                  </TD>
                  <TD numeric>{formatMoneyShort(p.budgetMinor)}</TD>
                  <TD numeric>{formatNumber(p.expectedQualified)}</TD>
                  <TD numeric>{formatMoneyShort(p.expectedCpqMinor)}</TD>
                  <TD className="text-muted-foreground">{formatRelative(p.createdAt)}</TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </Panel>
      )}
    </>
  );
}

const route = getRouteApi("/app/create/plans/$planId");

export function PlanPage() {
  const { planId } = route.useParams();
  const plan = usePlan(planId);
  return (
    <>
      <Link
        to="/create/plans"
        className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> Media plans
      </Link>
      {plan.isPending && <LoadingRows rows={8} />}
      {plan.error && <ErrorNote error={plan.error} />}
      {plan.data === null && (
        <Panel>
          <EmptyState title="This plan doesn't exist" />
        </Panel>
      )}
      {plan.data && <PlanBody plan={plan.data} />}
    </>
  );
}

function PlanBody({ plan: p }: { plan: Plan }) {
  const confirm = useConfirmAssumption();
  const decide = useDecidePlan();
  const [note, setNote] = useState("");
  const unconfirmed = p.assumptions.filter((a) => !a.confirmed).length;
  const canDecide = p.status === "waiting" || p.status === "draft";
  return (
    <>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            {p.projectName} · plan v{p.version}
            <StatusChip status={STATUS[p.status].chip}>{STATUS[p.status].label}</StatusChip>
          </span>
        }
        subtitle={`${shortDate(p.from)} – ${shortDate(p.to)} · drafted by ${p.author} ${formatRelative(p.createdAt)}${p.decidedBy ? ` · ${STATUS[p.status].label.toLowerCase()} by ${p.decidedBy} ${formatRelative(p.decidedAt ?? p.createdAt)}` : ""}`}
      />
      <div className="space-y-5">
        <section className="grid animate-rise gap-8 border bg-card px-7 py-8 md:grid-cols-[1.2fr_1fr] md:px-9">
          <div>
            <StatusMark status={STATUS[p.status].chip} className="size-3" />
            <p className="mt-4 flex items-baseline gap-3 font-heading text-6xl font-semibold tracking-[-0.035em] tabular-nums md:text-7xl">
              <CountUp
                value={formatMoneyShort(p.budgetMinor).replace("AED ", "")}
                to={p.budgetMinor / 100 / 1000}
              />
              <span className="text-xl font-medium tracking-normal text-muted-foreground">AED</span>
            </p>
            <h2 className="mt-5 font-heading text-2xl font-semibold leading-tight">
              {formatNumber(p.expectedQualified)} qualified leads at{" "}
              {formatMoneyShort(p.expectedCpqMinor)} each, if the assumptions hold.
            </h2>
            <p className="mt-2 max-w-[52ch] text-[0.95rem] leading-relaxed text-muted-foreground">
              {p.changes}
            </p>
            {canDecide && (
              <div className="mt-5 flex flex-wrap items-center gap-2">
                <Button
                  disabled={decide.isPending}
                  onClick={() => decide.mutate({ planId: p.id, decision: "approve" })}
                >
                  Approve v{p.version}
                </Button>
                <Button
                  variant="outline"
                  disabled={decide.isPending}
                  onClick={() => decide.mutate({ planId: p.id, decision: "changes", note })}
                >
                  Ask for changes
                </Button>
                {unconfirmed > 0 && (
                  <span className="text-sm text-muted-foreground">
                    {unconfirmed} {unconfirmed === 1 ? "assumption" : "assumptions"} not confirmed
                  </span>
                )}
              </div>
            )}
          </div>
          <div className="flex flex-col justify-center gap-3">
            <Eyebrow>By channel</Eyebrow>
            <HBars
              rows={p.channels.map((c) => ({
                label: PLATFORM[c.platform],
                value: `${c.share}%`,
                share: c.share,
              }))}
              labelWidth="6rem"
            />
          </div>
        </section>

        <div className="grid gap-5 lg:grid-cols-2">
          <Panel title="Channels" bodyClassName="p-0">
            <Table>
              <THead>
                <TR>
                  <TH>Channel</TH>
                  <TH numeric>Budget</TH>
                  <TH numeric>Expected qualified</TH>
                  <TH numeric>Per qualified</TH>
                </TR>
              </THead>
              <TBody>
                {p.channels.map((c) => (
                  <TR key={c.platform}>
                    <TD className="font-medium">{PLATFORM[c.platform]}</TD>
                    <TD numeric>{formatMoneyShort(c.budgetMinor)}</TD>
                    <TD numeric>{formatNumber(c.expectedQualified)}</TD>
                    <TD numeric>{formatMoneyShort(c.expectedCpqMinor)}</TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </Panel>
          <Panel title="By week" bodyClassName="p-0">
            <Table>
              <THead>
                <TR>
                  <TH>Week of</TH>
                  <TH numeric>Budget</TH>
                  <TH>Note</TH>
                </TR>
              </THead>
              <TBody>
                {p.weekly.map((w) => (
                  <TR key={w.week}>
                    <TD className="font-medium">{w.week}</TD>
                    <TD numeric>{formatMoneyShort(w.budgetMinor)}</TD>
                    <TD className="text-muted-foreground">{w.note ?? ""}</TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </Panel>
          <Panel
            title="Assumptions"
            description="Tick each one you've checked. Adx says where it got it."
            bodyClassName="p-0"
          >
            <ul className="divide-y">
              {p.assumptions.map((a) => (
                <li key={a.id} className="flex items-start gap-3 px-5 py-3">
                  <input
                    type="checkbox"
                    aria-label={`Confirm: ${a.text}`}
                    className="mt-1"
                    checked={a.confirmed}
                    disabled={!canDecide}
                    onChange={(e) =>
                      confirm.mutate({
                        planId: p.id,
                        assumptionId: a.id,
                        confirmed: e.target.checked,
                      })
                    }
                  />
                  <div>
                    <p className="text-sm">{a.text}</p>
                    <p className="text-xs text-muted-foreground">{a.source}</p>
                  </div>
                </li>
              ))}
            </ul>
          </Panel>
          <Panel
            title="Targets"
            description="What the weekly review measures the campaign against."
          >
            <dl className="grid grid-cols-2 gap-4">
              {p.kpis.map((k) => (
                <div key={k.label}>
                  <dt className="text-xs text-muted-foreground">{k.label}</dt>
                  <dd className="font-medium">{k.target}</dd>
                </div>
              ))}
            </dl>
            {canDecide && (
              <div className="mt-5">
                <label htmlFor="plan-note" className="text-xs text-muted-foreground">
                  Note for Adx when asking for changes
                </label>
                <textarea
                  id="plan-note"
                  rows={2}
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  placeholder="Move more to Google; keep Snapchat under 5%"
                  className="mt-1 w-full border bg-card px-3 py-2 text-sm outline-none focus:border-foreground"
                />
              </div>
            )}
          </Panel>
        </div>
        {p.status === "approved" && (
          <p className="text-sm text-muted-foreground">
            Approved. Launch picks it up:{" "}
            <Link to="/create/launch" className="underline underline-offset-4">
              see where {p.projectName} is
            </Link>
            .
          </p>
        )}
      </div>
    </>
  );
}
