import { getRouteApi, Link } from "@tanstack/react-router";
import { useProjectsOverview } from "@/api/adx";
import type { Pacing } from "@/api/models";
import { AnalyticsDatePicker } from "@/components/analytics-date-picker";
import { Meter, Sparkline } from "@/components/charts";
import { ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { type AnalyticsSearch, dubaiToday, rangeError } from "@/lib/analytics";
import {
  change,
  formatChange,
  formatDate,
  formatMoneyShort,
  formatNumber,
  formatPercent,
  NOT_AVAILABLE,
} from "@/lib/format";
import type { ProjectPerformance } from "@/lib/project-performance";
import { MoveMark } from "./brief";

const route = getRouteApi("/app/perform/projects");
const cost = (spend: number | null, count: number | null) =>
  spend !== null && count !== null && count > 0 ? Math.round(spend / count) : null;

export function ProjectsOverviewPage() {
  const dates = route.useSearch();
  const navigate = route.useNavigate();
  const search: AnalyticsSearch = {
    ...dates,
    project: "all",
    platform: "all",
    metric: "spendMinor",
  };
  const projects = useProjectsOverview(search);
  const error = rangeError(search);
  const compare = search.compare !== "none";
  return (
    <>
      <PageHeader
        title="Projects"
        actions={
          <AnalyticsDatePicker
            search={search}
            onChange={(next) => navigate({ search: { ...dates, ...next } })}
          />
        }
      />
      {error ? (
        <p role="alert" className="border border-status-caution/40 p-4 text-sm">
          {error}
        </p>
      ) : (
        <>
          {projects.isPending && <LoadingRows rows={8} />}
          {projects.error && <ErrorNote error={projects.error} />}
          {projects.data && (
            <div className="space-y-6">
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {projects.data.map((p) => (
                  <ProjectTile key={p.projectId} p={p} compare={compare} />
                ))}
              </div>
              <FunnelByProject rows={projects.data} />
            </div>
          )}
        </>
      )}
    </>
  );
}

function FunnelByProject({ rows }: { rows: ProjectPerformance[] }) {
  return (
    <Panel title="Funnel by project" bodyClassName="p-0">
      <Table>
        <THead>
          <TR>
            <TH>Project</TH>
            <TH numeric>Spend</TH>
            <TH numeric>Leads</TH>
            <TH numeric>Qualified</TH>
            <TH numeric>Meetings</TH>
            <TH numeric>Offers</TH>
            <TH numeric>Done-Deals</TH>
            <TH numeric>Per qualified</TH>
            <TH numeric>Per meeting</TH>
          </TR>
        </THead>
        <TBody>
          {rows.map((p) => (
            <TR key={p.projectId}>
              <TD className="font-medium whitespace-nowrap">
                <Link
                  to="/projects/$projectId"
                  params={{ projectId: String(p.projectId) }}
                  className="hover:underline"
                >
                  {p.name}
                </Link>
              </TD>
              <TD numeric>{formatMoneyShort(p.current.spendMinor)}</TD>
              <TD numeric>{formatNumber(p.current.leads)}</TD>
              <TD numeric>{formatNumber(p.current.qualified)}</TD>
              <TD numeric>{formatNumber(p.meetings)}</TD>
              <TD numeric>{formatNumber(p.offers)}</TD>
              <TD numeric>{formatNumber(p.current.deals)}</TD>
              <TD numeric className="font-semibold">
                {formatMoneyShort(p.current.cpq === null ? null : Math.round(p.current.cpq))}
              </TD>
              <TD numeric className="text-muted-foreground">
                {formatMoneyShort(cost(p.current.spendMinor, p.meetings))}
              </TD>
            </TR>
          ))}
        </TBody>
      </Table>
    </Panel>
  );
}

function ProjectTile({ p, compare }: { p: ProjectPerformance; compare: boolean }) {
  const cpq = change(p.current.cpq, p.previous.cpq);
  const enough = (p.previous.qualified ?? 0) >= 10;
  const cpMeeting = cost(p.current.spendMinor, p.meetings);
  return (
    <article className="flex flex-col border bg-card">
      <div className="flex items-baseline justify-between gap-3 px-5 pt-4">
        <h3 className="truncate font-heading text-base font-semibold">
          <Link
            to="/projects/$projectId"
            params={{ projectId: String(p.projectId) }}
            className="hover:underline"
          >
            {p.name}
          </Link>
        </h3>
        <Eyebrow className="shrink-0">Per qualified</Eyebrow>
      </div>
      <div className="flex items-baseline justify-between gap-3 px-5 pt-2">
        {p.qualifiedTrend ? (
          <Sparkline values={p.qualifiedTrend} area={false} className="h-7 w-28" />
        ) : (
          <span />
        )}
        <p className="text-right">
          {p.current.cpq === null ? (
            <span className="block text-sm text-muted-foreground">
              {p.current.qualified === 0 ? "No qualified leads" : NOT_AVAILABLE}
            </span>
          ) : (
            <>
              <span className="block font-mono text-2xl font-semibold tabular-nums">
                {formatMoneyShort(Math.round(p.current.cpq))}
              </span>
              {compare && (
                <span className="block font-mono text-xs whitespace-nowrap tabular-nums">
                  {enough && <MoveMark c={cpq} better="down" />}
                  {formatChange(cpq)}
                </span>
              )}
            </>
          )}
        </p>
      </div>
      <dl className="mt-4 grid grid-cols-3 border-y">
        <Figure
          label="Leads"
          value={p.current.leads}
          hint={compare ? `vs ${formatNumber(p.previous.leads)}` : undefined}
        />
        <Figure
          label="Qualified"
          value={p.current.qualified}
          hint={compare ? `vs ${formatNumber(p.previous.qualified)}` : undefined}
        />
        <Figure
          label="Meetings"
          value={p.meetings}
          hint={cpMeeting === null ? undefined : `${formatMoneyShort(cpMeeting)} each`}
        />
      </dl>
      <div className="mt-auto px-5 py-4">
        <PacingCell pacing={p.pacing} />
      </div>
    </article>
  );
}

function Figure({ label, value, hint }: { label: string; value: number | null; hint?: string }) {
  return (
    <div className="min-w-0 border-r px-4 py-3 first:pl-5 last:border-r-0">
      <dt className="font-mono text-[0.6rem] tracking-wider whitespace-nowrap text-muted-foreground uppercase">
        {label}
      </dt>
      <dd
        className={
          value === null
            ? "mt-1 text-xs text-muted-foreground"
            : "mt-1 font-mono text-lg font-semibold tabular-nums"
        }
      >
        {formatNumber(value)}
      </dd>
      {hint && <dd className="truncate text-xs text-muted-foreground">{hint}</dd>}
    </div>
  );
}

function PacingCell({ pacing }: { pacing: Pacing | null }) {
  if (!pacing) return <p className="text-xs text-muted-foreground">No monthly budget set</p>;
  const expected = (pacing.budgetMinor * pacing.dayOfMonth) / pacing.daysInMonth;
  const off = pacing.projectedMinor / pacing.budgetMinor - 1;
  return (
    <div className="space-y-1.5">
      <div className="flex items-baseline justify-between gap-3 text-xs">
        <span className="text-muted-foreground">
          {formatDate(`${dubaiToday()}T00:00:00Z`, { month: "long" })} budget
        </span>
        <span className="font-mono tabular-nums">
          {formatMoneyShort(pacing.spentMinor)} of {formatMoneyShort(pacing.budgetMinor)}
        </span>
      </div>
      <Meter value={pacing.spentMinor} max={pacing.budgetMinor} marker={expected} />
      <p className="text-xs whitespace-nowrap">
        {Math.abs(off) < 0.05 ? (
          <span className="text-muted-foreground">On track</span>
        ) : (
          <>
            <StatusMark status={off > 0 ? "caution" : "pending"} className="mr-1.5" />
            {formatPercent(Math.abs(off))} {off > 0 ? "over" : "under"} by month end
          </>
        )}
      </p>
    </div>
  );
}
