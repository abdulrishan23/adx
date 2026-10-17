import { useState } from "react";
import { type AnalyticsParams, useAnalytics, useAssignProjects } from "@/api/analytics";
import { useSession } from "@/api/session";
import type { Change, Ratio, ReportRow } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { formatMoney, NOT_AVAILABLE } from "@/lib/format";

function dayAgo(days: number): string {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() - days);
  return d.toISOString().slice(0, 10);
}

// Ratios are minor units per unit; shown as money. Undefined (no leads) is
// said so; null (no spend figures) is "Not available".
function ratio(r: Ratio, currency: string): string {
  if (r.undefined) return "No leads";
  if (r.value === null) return NOT_AVAILABLE;
  return formatMoney(Math.round(r.value), currency || "AED");
}

function pct(c: Change | undefined): string {
  if (!c) return "";
  if (c.undefined || c.relative === null) {
    return c.absolute === null
      ? "no comparison"
      : `${c.absolute >= 0 ? "+" : ""}${Math.round(c.absolute)} (no base)`;
  }
  const v = Math.round(c.relative * 100);
  return `${v >= 0 ? "+" : ""}${v}%`;
}

function count(v: number | null | undefined): string {
  return v === null || v === undefined ? NOT_AVAILABLE : String(v);
}

// Deterministic figures only. Every number comes from the API's formula
// version; nothing is computed in the browser except formatting.
export function AnalyticsPage() {
  const [params, setParams] = useState<AnalyticsParams>({
    from: dayAgo(30),
    to: dayAgo(1),
    group: "campaign",
    compare: "previous",
    platform: "",
  });
  const analytics = useAnalytics(params);
  const session = useSession();
  const assign = useAssignProjects();
  const canAssign = session.data?.role === "marketer" || session.data?.role === "admin";

  return (
    <section className="space-y-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-2xl font-semibold">Analytics</h1>
        {analytics.data && (
          <span className="text-sm text-muted-foreground">
            Formula {analytics.data.formulaVersion} · days in {analytics.data.period.timeZone}
          </span>
        )}
      </div>

      <div className="flex flex-wrap items-end gap-3 text-sm">
        <label className="flex flex-col gap-1">
          <span className="text-muted-foreground">From</span>
          <input
            type="date"
            className="rounded-md border bg-background px-2 py-1"
            value={params.from}
            onChange={(e) => setParams({ ...params, from: e.target.value })}
          />
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-muted-foreground">To</span>
          <input
            type="date"
            className="rounded-md border bg-background px-2 py-1"
            value={params.to}
            onChange={(e) => setParams({ ...params, to: e.target.value })}
          />
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-muted-foreground">Group by</span>
          <select
            className="rounded-md border bg-background px-2 py-1"
            value={params.group}
            onChange={(e) =>
              setParams({ ...params, group: e.target.value as AnalyticsParams["group"] })
            }
          >
            <option value="campaign">Campaign</option>
            <option value="project">Project</option>
            <option value="platform">Platform</option>
            <option value="day">Day</option>
          </select>
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-muted-foreground">Platform</span>
          <select
            className="rounded-md border bg-background px-2 py-1"
            value={params.platform}
            onChange={(e) =>
              setParams({ ...params, platform: e.target.value as AnalyticsParams["platform"] })
            }
          >
            <option value="">Both (claims kept apart)</option>
            <option value="google_ads">Google Ads</option>
            <option value="meta">Meta</option>
          </select>
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-muted-foreground">Compare with</span>
          <select
            className="rounded-md border bg-background px-2 py-1"
            value={params.compare}
            onChange={(e) =>
              setParams({ ...params, compare: e.target.value as AnalyticsParams["compare"] })
            }
          >
            <option value="">Nothing</option>
            <option value="previous">Previous period</option>
            <option value="year">Same dates last year</option>
          </select>
        </label>
        {canAssign && (
          <Button
            variant="outline"
            size="sm"
            disabled={assign.isPending}
            onClick={() => assign.mutate()}
          >
            Read projects from campaign names
          </Button>
        )}
      </div>
      {assign.data && (
        <p className="text-sm text-muted-foreground">
          Assigned {assign.data.assigned}.{" "}
          {assign.data.unresolved.length > 0
            ? `Unresolved: ${assign.data.unresolved.join(", ")}`
            : "Every campaign names one project."}
        </p>
      )}

      {analytics.isPending && <p className="text-muted-foreground">Loading…</p>}
      {analytics.error && <p className="text-destructive">{analytics.error.message}</p>}
      {analytics.data && (
        <>
          <div className="space-y-1 rounded-lg border p-4 text-sm">
            <div>
              <span className="text-muted-foreground">Period </span>
              {analytics.data.period.start} to {analytics.data.period.end}
              {analytics.data.period.comparison &&
                ` · compared with ${analytics.data.period.comparison.start} to ${analytics.data.period.comparison.end}`}
            </div>
            <div>
              <span className="text-muted-foreground">Figures </span>
              {analytics.data.freshness.observedThrough
                ? `observed through ${analytics.data.freshness.observedThrough}`
                : "never read"}
              {analytics.data.freshness.partial && " · partial"}
              {analytics.data.freshness.missingPeriods.length > 0 &&
                ` · missing: ${analytics.data.freshness.missingPeriods.join("; ")}`}
            </div>
            <div>
              <span className="text-muted-foreground">Lead coverage </span>
              {analytics.data.coverage.matchedCount} of {analytics.data.coverage.eligibleCount}{" "}
              leads matched to a campaign · {analytics.data.coverage.withStatus} with a status ·{" "}
              {analytics.data.coverage.unclassifiedCount} unclassified
            </div>
            {analytics.data.coverage.dependenciesMissing.map((d: string) => (
              <div key={d} className="text-destructive">
                {d}
              </div>
            ))}
            {analytics.data.warnings.map((w) => (
              <div key={w.code + w.message} className="text-destructive">
                {w.message}
              </div>
            ))}
          </div>

          {analytics.data.rows.length === 0 ? (
            <p className="text-muted-foreground">
              No figures for this period. Connect an ad account and refresh it, or widen the period.
            </p>
          ) : (
            <table className="w-full text-sm">
              <thead className="text-left text-muted-foreground">
                <tr>
                  <th className="py-1 font-normal">{analytics.data.groupBy}</th>
                  <th className="py-1 font-normal">Spend</th>
                  <th className="py-1 font-normal">Leads (GHL)</th>
                  <th className="py-1 font-normal">Qualified</th>
                  <th className="py-1 font-normal">Deals</th>
                  <th className="py-1 font-normal">Cost / lead</th>
                  <th className="py-1 font-normal">Cost / qualified</th>
                  <th className="py-1 font-normal">Google conv.</th>
                  <th className="py-1 font-normal">Meta leads</th>
                  <th className="py-1 font-normal">Revenue</th>
                  <th className="py-1 font-normal">ROAS</th>
                </tr>
              </thead>
              <tbody className="divide-y border-y">
                {[...analytics.data.rows, analytics.data.summary].map((r) => (
                  <MetricRow key={r.key} row={r} summary={r.key === "summary"} />
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </section>
  );
}

function MetricRow({ row, summary }: { row: ReportRow; summary: boolean }) {
  const m = row.metrics;
  const ch = row.changes ?? {};
  const cur = m.spend.currency || "AED";
  return (
    <tr className={summary ? "font-medium" : ""}>
      <td className="py-2">
        {row.label}
        {row.platform && !summary && (
          <span className="text-muted-foreground"> · {row.platform}</span>
        )}
      </td>
      <td className="py-2">
        {formatMoney(m.spend.minor, cur)}
        <Delta c={ch.spend} />
      </td>
      <td className="py-2">
        {m.leads}
        <Delta c={ch.leads} />
      </td>
      <td className="py-2">{count(m.qualifiedLeads)}</td>
      <td className="py-2">{count(m.deals)}</td>
      <td className="py-2">
        {ratio(m.costPerLead, cur)}
        <Delta c={ch.costPerLead} />
      </td>
      <td className="py-2">{ratio(m.costPerQualifiedLead, cur)}</td>
      <td className="py-2">{m.googleConversions === null ? NOT_AVAILABLE : m.googleConversions}</td>
      <td className="py-2">{m.metaLeads === null ? NOT_AVAILABLE : m.metaLeads}</td>
      <td className="py-2">{formatMoney(m.revenue.minor, cur)}</td>
      <td className="py-2">{m.roas.value === null ? NOT_AVAILABLE : m.roas.value.toFixed(2)}</td>
    </tr>
  );
}

function Delta({ c }: { c: Change | undefined }) {
  if (!c) return null;
  return <span className="ml-1 text-xs text-muted-foreground">{pct(c)}</span>;
}
