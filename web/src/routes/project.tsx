import { getRouteApi, Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { useProjectSummary } from "@/api/adx";
import {
  type Analyst,
  type LoopState,
  type SetupStep,
  useBrandSetup,
  useLoopStates,
} from "@/api/create";
import { useCreativeOverview } from "@/api/creative";
import type { ProjectSummary } from "@/api/models";
import { useProjects } from "@/api/projects";
import type { ProjectSummary as CreativeSummary } from "@/api/types.legacy";
import { PERIOD_LABEL, ScopeBar, useScope } from "@/app/scope";
import { ChartLegend, DailyChart, ProportionBar } from "@/components/charts";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { PlatformTag } from "@/components/platform";
import { StatTile } from "@/components/stat-tile";
import { buttonVariants } from "@/components/ui/button";
import { type Status, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatMoneyShort, formatNumber, formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";
import { SampleChip } from "./create/shared";

const route = getRouteApi("/app/brands/$projectId");

const per = (minor: number, n: number) => (n > 0 ? Math.round(minor / n) : null);

export function ProjectPage() {
  const { projectId } = route.useParams();
  const summary = useProjectSummary(Number(projectId));
  return (
    <>
      <Link
        to="/brands"
        className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> Brands
      </Link>
      {summary.isPending && <LoadingRows rows={8} />}
      {summary.error && <ErrorNote error={summary.error} />}
      {summary.data === null && <NoFigures id={Number(projectId)} />}
      {summary.data && <Body p={summary.data} />}
    </>
  );
}

// A brand is the hub: one development Reportage markets, followed from brand knowledge to live
// campaign to leads. The page runs in that order. Brand, research, plan and
// pages arrive with phases 2 and 3; creatives, campaigns and leads are live.
function Body({ p }: { p: ProjectSummary }) {
  const scope = useScope();
  const creative = useCreativeOverview();
  const loops = useLoopStates();
  const loop = loops.data?.find((l) => l.projectId === p.id);
  const cr = creative.data?.projects.find((c) => c.projectId === p.id);
  const mixTotal = p.temperatureMix.hot + p.temperatureMix.warm + p.temperatureMix.cold;
  const live = p.campaigns.some((c) => c.status === "active");
  return (
    <>
      <PageHeader
        title={p.name}
        subtitle={`${p.city} · ${PERIOD_LABEL[scope.days]}`}
        actions={<ScopeBar showPlatform={false} showProject={false} />}
      />
      <div className="space-y-6">
        <SetupPanel projectId={p.id} />
        <LoopStrip creative={cr} live={live} loop={loop} />

        <SectionLabel>Create</SectionLabel>
        <CreativeCard projectId={p.id} summary={cr} loading={creative.isPending} />

        <SectionLabel>Perform</SectionLabel>
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
          <StatTile label="Spend" value={formatMoneyShort(p.spendMinor)} />
          <StatTile label="Leads" value={formatNumber(p.leads)} />
          <StatTile label="Qualified" value={formatNumber(p.qualified)} />
          <StatTile
            label="Per qualified"
            value={formatMoneyShort(per(p.spendMinor, p.qualified))}
            better="down"
          />
          <StatTile
            label="Done-Deals"
            value={formatNumber(p.deals)}
            hint={p.revenueMinor !== null ? formatMoneyShort(p.revenueMinor) : undefined}
          />
        </div>

        <Panel
          title="Daily"
          action={
            <ChartLegend
              items={[
                { label: "Spend", kind: "bar" },
                { label: "Qualified", kind: "line" },
              ]}
            />
          }
        >
          <DailyChart
            points={p.daily.map((d) => ({ date: d.date, bar: d.spendMinor, line: d.qualified }))}
          />
        </Panel>

        <Panel title="Campaigns" bodyClassName="p-0">
          <Table>
            <THead>
              <TR>
                <TH>Campaign</TH>
                <TH numeric>Spend</TH>
                <TH numeric>Qualified</TH>
                <TH numeric>Per qualified</TH>
                <TH numeric>Deals</TH>
              </TR>
            </THead>
            <TBody>
              {p.campaigns.map((c) => (
                <TR key={c.id}>
                  <TD>
                    <Link
                      to="/campaigns"
                      search={{ open: c.id }}
                      className="flex items-center gap-2 font-medium hover:underline"
                    >
                      <PlatformTag platform={c.platform} />
                      {c.name}
                    </Link>
                  </TD>
                  <TD numeric>{formatMoneyShort(c.spendMinor)}</TD>
                  <TD numeric>{formatNumber(c.qualified)}</TD>
                  <TD numeric>{formatMoneyShort(per(c.spendMinor, c.qualified))}</TD>
                  <TD numeric>{c.deals}</TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </Panel>

        <SectionLabel>Convert</SectionLabel>
        <div className="grid gap-4 lg:grid-cols-2">
          <Panel title="Leads by temperature">
            <ProportionBar
              parts={[
                { label: "Hot", value: p.temperatureMix.hot },
                { label: "Warm", value: p.temperatureMix.warm },
                { label: "Cold", value: p.temperatureMix.cold },
              ]}
            />
            <p className="mt-2 font-mono text-xs text-muted-foreground tabular-nums">
              {formatNumber(p.temperatureMix.hot)} hot · {formatNumber(p.temperatureMix.warm)} warm
              · {formatNumber(p.temperatureMix.cold)} cold
              {mixTotal === 0 && " · no leads in this period"}
            </p>
          </Panel>

          <Panel title="Hot leads" bodyClassName="p-0">
            {p.hotLeads.length === 0 ? (
              <p className="px-5 py-4 text-sm text-muted-foreground">None right now.</p>
            ) : (
              <ul className="divide-y">
                {p.hotLeads.map((l) => (
                  <li
                    key={l.id}
                    className="flex items-baseline justify-between gap-3 px-5 py-2.5 text-sm"
                  >
                    <Link
                      to="/leads/$leadId"
                      params={{ leadId: String(l.id) }}
                      className="font-medium hover:underline"
                    >
                      {l.name}
                    </Link>
                    <span className="truncate text-xs text-muted-foreground">{l.reason}</span>
                    <span className="shrink-0 font-mono text-xs text-muted-foreground">
                      {formatRelative(l.lastActivityAt)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </div>
        <p className="text-xs text-muted-foreground">
          Also spelled{" "}
          {p.aliases.map((a) => (
            <span key={a} className="mr-1 border px-1.5 py-0.5 font-mono">
              {a}
            </span>
          ))}
        </p>
      </div>
    </>
  );
}

function SectionLabel({ children }: { children: string }) {
  return <Eyebrow className="pt-2">{children}</Eyebrow>;
}

// LoopStrip is where the project is on the way from brand to live campaign.
// Steps whose system hasn't shipped are shown as not started, not hidden,
// so the whole loop is visible from day one.
function LoopStrip({
  creative,
  live,
  loop,
}: {
  creative?: CreativeSummary;
  live: boolean;
  loop?: LoopState;
}) {
  const creativeStatus: Status = !creative
    ? "pending"
    : creative.revise > 0
      ? "caution"
      : creative.approved > 0
        ? "ready"
        : "pending";
  const later = (key: "brand" | "research" | "plan" | "page"): Status =>
    loop ? loop.steps[key] : "pending";
  const noteFor = (key: "brand" | "research" | "plan" | "page", phase: string) =>
    !loop || loop.steps[key] === "pending"
      ? phase
      : loop.steps[key] === "ready"
        ? "Done"
        : "Needs a look";
  const steps: { label: string; status: Status; note: string }[] = [
    { label: "Brand", status: later("brand"), note: noteFor("brand", "Phase 2") },
    {
      label: "Research",
      status: later("research"),
      note: noteFor("research", "Phase 2"),
    },
    { label: "Plan", status: later("plan"), note: noteFor("plan", "Phase 2") },
    {
      label: "Creative",
      status: creativeStatus,
      note: creative
        ? `${creative.approved} approved · ${creative.revise} to change · ${creative.waiting} waiting`
        : "Not set up",
    },
    { label: "Page", status: later("page"), note: noteFor("page", "Phase 3") },
    {
      label: "Launch",
      status: live ? "ready" : "pending",
      note: live ? "Delivering" : "Nothing live",
    },
  ];
  return (
    <ol className="grid grid-cols-3 gap-px border bg-border md:grid-cols-6">
      {steps.map((s) => (
        <li key={s.label} className="bg-card px-4 py-3">
          <p className="flex items-center gap-2 text-sm font-medium">
            <StatusMark status={s.status} />
            {s.label}
          </p>
          <p className="mt-1 truncate text-xs text-muted-foreground">{s.note}</p>
        </li>
      ))}
    </ol>
  );
}

function CreativeCard({
  projectId,
  summary,
  loading,
}: {
  projectId: number;
  summary?: CreativeSummary;
  loading: boolean;
}) {
  if (loading) return <LoadingRows rows={2} />;
  if (!summary) {
    return (
      <Panel>
        <EmptyState title="No creatives yet">
          Set this brand up on Creatives to upload ads or build them from copy.
        </EmptyState>
      </Panel>
    );
  }
  return (
    <Panel
      title="Creatives"
      description={`${summary.concepts} concepts · ${summary.ads} ads · ${summary.renders} renders`}
      action={
        <Link
          to="/creative/$projectId"
          params={{ projectId: String(projectId) }}
          className={buttonVariants({ variant: "outline", size: "sm" })}
        >
          Open
        </Link>
      }
    >
      <div className="flex flex-wrap gap-5 text-sm">
        <span className="flex items-center gap-2">
          <StatusMark status="ready" /> {summary.approved} approved
        </span>
        <span className="flex items-center gap-2">
          <StatusMark status="caution" /> {summary.revise} need changes
        </span>
        <span className="flex items-center gap-2">
          <StatusMark status="pending" /> {summary.waiting} waiting
        </span>
        {summary.unpublished > 0 && (
          <span className="text-muted-foreground">{summary.unpublished} unpublished</span>
        )}
      </div>
    </Panel>
  );
}

// A project the server knows but nothing reports on yet, such as one just
// added in Settings. Its figures arrive once campaigns are linked to it.
function NoFigures({ id }: { id: number }) {
  const projects = useProjects();
  const project = projects.data?.projects.find((p) => p.id === id);
  if (projects.isPending) return <LoadingRows rows={3} />;
  if (!project) {
    return (
      <Panel>
        <EmptyState title="This brand doesn't exist" />
      </Panel>
    );
  }
  return (
    <>
      <PageHeader title={project.name} />
      <div className="space-y-6">
        <SetupPanel projectId={id} />
        <Panel>
          <EmptyState title="No campaigns linked yet">
            Figures arrive once campaigns are linked to this brand.
          </EmptyState>
        </Panel>
      </div>
    </>
  );
}

const ANALYST_NAME: Record<Analyst, string> = {
  performance: "Performance marketer",
  media: "Media planner",
  brand: "Brand strategist",
  seo: "SEO specialist",
  creative: "Creative director",
  crm: "CRM and sales analyst",
};

const STEP_MARK: Record<SetupStep["state"], "ready" | "pending"> = {
  done: "ready",
  working: "pending",
  queued: "pending",
};

// SetupPanel follows a brand started from Start a brand: Adx works through
// analysis, audit and research to a first media plan. Nothing shows for a
// brand that wasn't started that way.
function SetupPanel({ projectId }: { projectId: number }) {
  const setup = useBrandSetup(projectId);
  const s = setup.data;
  if (!s) return null;
  const doneCount = s.steps.filter((x) => x.state === "done").length;
  return (
    <Panel
      title={
        <span className="flex items-center gap-2">
          {s.done ? "Set up" : "Setting up"} <SampleChip phase={2} />
        </span>
      }
      description={`${doneCount} of ${s.steps.length} done · working as ${s.analysts.map((a) => ANALYST_NAME[a]).join(", ")}`}
      action={
        s.done && (
          <Link to="/create/plans" className={buttonVariants({ size: "sm" })}>
            Review the plan
          </Link>
        )
      }
    >
      <div className="mb-4 h-[3px] bg-muted">
        <div
          className="h-full bg-foreground transition-[width] duration-700"
          style={{ width: `${(doneCount / s.steps.length) * 100}%` }}
        />
      </div>
      <ol className="grid gap-x-8 gap-y-3 md:grid-cols-2">
        {s.steps.map((step) => (
          <li key={step.id} className="flex items-start gap-3">
            <StatusMark
              status={STEP_MARK[step.state]}
              className={cn(
                "mt-1.5",
                step.state === "working" && "animate-pulse",
                step.state === "queued" && "opacity-40",
              )}
            />
            <span className="min-w-0 flex-1">
              <span
                className={cn(
                  "block text-sm font-medium",
                  step.state === "queued" && "text-muted-foreground",
                )}
              >
                {step.label}
              </span>
              <span className="block text-xs text-muted-foreground">{step.detail}</span>
            </span>
            <span className="shrink-0 font-mono text-xs text-muted-foreground">
              {step.state === "done" ? "Done" : step.state === "working" ? "Working" : "Next"}
            </span>
          </li>
        ))}
      </ol>
    </Panel>
  );
}
