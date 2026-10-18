import { Link } from "@tanstack/react-router";
import { useOutcomes } from "@/api/adx";
import type { AuditItem, Outcomes } from "@/api/models";
import { ChartLegend, DailyChart } from "@/components/charts";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { PlatformTag } from "@/components/platform";
import { StatTile } from "@/components/stat-tile";
import { StatusChip } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatNumber, formatPercent, formatRelative } from "@/lib/format";

const SEVERITY: Record<
  AuditItem["severity"],
  { chip: "error" | "caution" | "pending"; label: string }
> = {
  high: { chip: "error", label: "High" },
  medium: { chip: "caution", label: "Medium" },
  low: { chip: "pending", label: "Low" },
};

export function OutcomesPage() {
  const outcomes = useOutcomes();
  return (
    <>
      <PageHeader title="Outcome sync" />
      {outcomes.isPending && <LoadingRows rows={8} />}
      {outcomes.error && <ErrorNote error={outcomes.error} />}
      {outcomes.data && <Body data={outcomes.data} />}
    </>
  );
}

function Body({ data }: { data: Outcomes }) {
  const broken = data.destinations.filter((d) => d.state === "error");
  const sent = data.destinations.reduce((a, d) => a + d.sent30d, 0);
  const failed = data.destinations.reduce((a, d) => a + d.failed30d, 0);
  const meta = data.destinations.filter((d) => d.platform === "meta" && d.matched30d !== null);
  const metaSent = meta.reduce((a, d) => a + d.sent30d, 0);
  const metaMatched = meta.reduce((a, d) => a + (d.matched30d ?? 0), 0);
  const open = data.audit.filter((a) => a.status === "open");

  return (
    <div className="space-y-6">
      {broken.map((d) => (
        <div
          key={d.key}
          className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-status-error/40 bg-status-error/10 px-4 py-3 text-sm"
        >
          <p>
            <span className="font-medium">
              {d.name} stopped {d.lastSentAt ? formatRelative(d.lastSentAt) : ""}.
            </span>{" "}
          </p>
          <Link to="/settings/connections" className="font-medium underline underline-offset-4">
            Reconnect Google Ads
          </Link>
        </div>
      ))}

      <div className="grid gap-4 sm:grid-cols-3">
        <StatTile label="Outcomes sent" value={formatNumber(sent)} hint="Last 30 days" />
        <StatTile
          label="Matched by Meta"
          value={formatPercent(metaSent > 0 ? metaMatched / metaSent : null)}
        />
        <StatTile label="Failed" value={formatNumber(failed)} />
      </div>

      <Panel bodyClassName="p-0">
        <Table>
          <THead>
            <TR>
              <TH>Destination</TH>
              <TH numeric>Sent</TH>
              <TH numeric>Matched</TH>
              <TH numeric>Failed</TH>
              <TH>Last sent</TH>
              <TH />
            </TR>
          </THead>
          <TBody>
            {data.destinations.map((d) => (
              <TR key={d.key}>
                <TD>
                  <div className="flex items-center gap-2">
                    <PlatformTag platform={d.platform} />
                    <span className="font-medium">{d.name}</span>
                  </div>
                  <div className="mt-0.5 text-xs text-muted-foreground">{d.event}</div>
                </TD>
                <TD numeric>{formatNumber(d.sent30d)}</TD>
                <TD numeric>
                  {d.matched30d === null
                    ? formatPercent(null)
                    : formatPercent(d.sent30d > 0 ? d.matched30d / d.sent30d : null)}
                </TD>
                <TD numeric>{formatNumber(d.failed30d)}</TD>
                <TD className="text-muted-foreground">
                  {d.lastSentAt ? formatRelative(d.lastSentAt) : "Never"}
                </TD>
                <TD className="text-right">
                  <StatusChip status={d.state}>{d.stateLabel}</StatusChip>
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </Panel>

      <Panel
        title="Sent daily"
        action={
          <ChartLegend
            items={[
              { label: "Qualified leads", kind: "bar" },
              { label: "Done-Deals", kind: "line" },
            ]}
          />
        }
      >
        <DailyChart
          label="Qualified leads and Done-Deals sent per day"
          describe={(p) => `${p.date}: ${p.bar} qualified, ${p.line} deals`}
          points={data.daily.map((d) => ({ date: d.date, bar: d.qualified, line: d.deals }))}
        />
      </Panel>

      <div className="grid gap-4 lg:grid-cols-2">
        <Panel title="Setup audit" description={`${open.length} open`} bodyClassName="p-0">
          <ul className="divide-y">
            {data.audit.map((a) => (
              <li key={a.id} className="space-y-1.5 px-5 py-4">
                <div className="flex items-start justify-between gap-3">
                  <p className="font-medium">{a.title}</p>
                  {a.status === "fixed" ? (
                    <StatusChip status="ready">Fixed</StatusChip>
                  ) : (
                    <StatusChip status={SEVERITY[a.severity].chip}>
                      {SEVERITY[a.severity].label}
                    </StatusChip>
                  )}
                </div>
                <p className="text-sm text-muted-foreground">{a.finding}</p>
                <p className="text-sm">
                  <span className="font-medium">Fix: </span>
                  {a.fix}
                </p>
              </li>
            ))}
          </ul>
        </Panel>

        <Panel title="Recent failures" bodyClassName="p-0">
          <ul className="divide-y">
            {data.failures.map((f) => (
              <li key={f.id} className="space-y-1 px-5 py-3 text-sm">
                <div className="flex items-baseline justify-between gap-3">
                  <Link
                    to="/leads/$leadId"
                    params={{ leadId: String(f.leadId) }}
                    className="font-medium underline decoration-foreground/30 underline-offset-4 hover:decoration-foreground"
                  >
                    {f.leadName}
                  </Link>
                  <span className="shrink-0 font-mono text-xs text-muted-foreground">
                    {formatRelative(f.at)}
                  </span>
                </div>
                <p className="text-xs text-muted-foreground">{f.destination}</p>
                <p>{f.reason}</p>
              </li>
            ))}
          </ul>
        </Panel>
      </div>
    </div>
  );
}
