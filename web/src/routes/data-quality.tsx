import { useDataQuality } from "@/api/adx";
import { projectOf } from "@/api/mock/data";
import type { CoverageRow, DataQuality } from "@/api/models";
import { Meter } from "@/components/charts";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { PlatformTag } from "@/components/platform";
import { StatTile } from "@/components/stat-tile";
import { StatusChip } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatNumber, formatPercent } from "@/lib/format";
import { MATCH_LABEL } from "./leads";

export function DataQualityPage() {
  const dq = useDataQuality();
  return (
    <>
      <PageHeader title="Data quality" />
      {dq.isPending && <LoadingRows rows={8} />}
      {dq.error && <ErrorNote error={dq.error} />}
      {dq.data && <Body dq={dq.data} />}
    </>
  );
}

// A gap under 1% is normal sync timing, not a broken sync.
function gapState(gap: number, platformLeads: number) {
  const share = platformLeads > 0 ? gap / platformLeads : 0;
  if (share >= 0.05) return { status: "error" as const, label: "Broken" };
  if (share >= 0.01) return { status: "caution" as const, label: "Gap" };
  return { status: "ready" as const, label: "OK" };
}

function Body({ dq }: { dq: DataQuality }) {
  const missing = dq.syncGaps.reduce((a, g) => a + Math.max(0, g.platformLeads - g.crmLeads), 0);
  const delivered = dq.syncGaps.reduce((a, g) => a + g.platformLeads, 0);
  const totalLeads = dq.matchLevels.reduce((a, m) => a + m.count, 0);
  const known = dq.matchLevels.filter((m) => m.level !== "none").reduce((a, m) => a + m.count, 0);
  const statusTotal = dq.statusBySource.reduce((a, r) => a + r.total, 0);
  const statusCovered = dq.statusBySource.reduce((a, r) => a + r.covered, 0);
  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-3">
        <StatTile
          label="Leads missing from GoHighLevel"
          value={formatNumber(missing)}
          hint={`${formatPercent(missing / Math.max(1, delivered), 1)} of what the platforms delivered`}
        />
        <StatTile
          label="Leads with a status"
          value={formatPercent(statusCovered / Math.max(1, statusTotal))}
          hint={`${formatNumber(dq.staleLeads)} still blank after 7 days`}
        />
        <StatTile
          label="Leads with a known source"
          value={formatPercent(known / Math.max(1, totalLeads))}
        />
      </div>

      <Panel title="Sync gaps" bodyClassName="p-0">
        <Table>
          <THead>
            <TR>
              <TH>Form</TH>
              <TH>Project</TH>
              <TH numeric>Platform</TH>
              <TH numeric>GoHighLevel</TH>
              <TH numeric>Missing</TH>
              <TH />
            </TR>
          </THead>
          <TBody>
            {dq.syncGaps.map((g) => {
              const gap = Math.max(0, g.platformLeads - g.crmLeads);
              const state = gapState(gap, g.platformLeads);
              return (
                <TR key={g.form}>
                  <TD>
                    <div className="flex items-center gap-2">
                      <PlatformTag platform={g.platform} />
                      <span className="font-medium">{g.form}</span>
                    </div>
                  </TD>
                  <TD>{projectOf(g.projectId)}</TD>
                  <TD numeric>{formatNumber(g.platformLeads)}</TD>
                  <TD numeric>{formatNumber(g.crmLeads)}</TD>
                  <TD numeric className="font-semibold">
                    {formatNumber(gap)}
                  </TD>
                  <TD className="text-right">
                    <StatusChip status={state.status}>{state.label}</StatusChip>
                  </TD>
                </TR>
              );
            })}
          </TBody>
        </Table>
      </Panel>

      <div className="grid gap-4 lg:grid-cols-2">
        <CoveragePanel title="Statuses by source" rows={dq.statusBySource} />
        <CoveragePanel title="Statuses by owner" rows={dq.statusByOwner} />
      </div>

      <Panel title="Match level">
        <ul className="space-y-3">
          {dq.matchLevels.map((m) => (
            <li key={m.level} className="grid grid-cols-[9rem_1fr_4rem] items-center gap-3 text-sm">
              <span>{MATCH_LABEL[m.level]}</span>
              <Meter value={m.count} max={totalLeads} />
              <span className="text-right font-mono tabular-nums">
                {formatPercent(m.count / Math.max(1, totalLeads))}
              </span>
            </li>
          ))}
        </ul>
      </Panel>
    </div>
  );
}

function CoveragePanel({
  title,
  description,
  rows,
}: {
  title: string;
  description?: string;
  rows: CoverageRow[];
}) {
  return (
    <Panel title={title} description={description}>
      <ul className="space-y-3">
        {rows.map((r) => (
          <li key={r.label}>
            <div className="mb-1 flex justify-between text-sm">
              <span>{r.label}</span>
              <span className="font-mono tabular-nums">
                {formatPercent(r.covered / Math.max(1, r.total))}
                <span className="ml-2 text-xs text-muted-foreground">
                  of {formatNumber(r.total)}
                </span>
              </span>
            </div>
            <Meter value={r.covered} max={r.total} />
          </li>
        ))}
      </ul>
    </Panel>
  );
}
