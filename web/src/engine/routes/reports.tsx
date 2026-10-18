import { useState } from "react";
import type { Report, ReportSchedule } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  useCreateSchedule,
  useGenerateScheduled,
  useReport,
  useReports,
  useSchedules,
} from "@/engine/api/reports";
import { useSession } from "@/engine/api/session";
import { formatMoney, NOT_AVAILABLE } from "@/engine/lib/format";

const delivery: Record<string, string> = {
  unconfigured: "Not delivered: delivery not configured",
  pending: "Delivery pending",
  sent: "Sent",
  failed: "Delivery failed",
};

// Scheduled and ad hoc reports. A report shows when it was generated, what
// it covers, how fresh its figures were, and whether it went anywhere.
// Unconfigured recipients are stated, never silently skipped.
export function ReportsPage() {
  const schedules = useSchedules();
  const reports = useReports();
  const session = useSession();
  const create = useCreateSchedule();
  const generate = useGenerateScheduled();
  const [open, setOpen] = useState<number | null>(null);
  const detail = useReport(open);
  const isAdmin = session.data?.role === "admin";
  const canGenerate = isAdmin || session.data?.role === "marketer";
  const [form, setForm] = useState({ name: "", cadence: "daily", recipients: "" });

  return (
    <section className="space-y-8">
      <h1 className="text-3xl font-semibold tracking-tight">Reports</h1>

      <div className="space-y-3">
        <h2 className="font-semibold">Schedules</h2>
        {schedules.isPending && <p className="text-muted-foreground">Loading…</p>}
        {schedules.error && <p className="text-destructive">{schedules.error.message}</p>}
        {schedules.data && schedules.data.schedules.length === 0 && (
          <p className="text-muted-foreground">
            No schedules yet. Reports are generated only from a schedule or on request.
          </p>
        )}
        {schedules.data && schedules.data.schedules.length > 0 && (
          <ul className="divide-y border-y">
            {schedules.data.schedules.map((s: ReportSchedule) => (
              <li
                key={s.id}
                className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm"
              >
                <span>
                  <span className="font-medium">{s.name}</span>{" "}
                  <span className="text-muted-foreground">
                    {s.cadence} · {s.enabled ? "enabled" : "disabled"} ·{" "}
                    {s.deliveryConfigured
                      ? `to ${s.recipients.join(", ")}`
                      : s.recipients.length === 0
                        ? "no recipients: generated, not delivered"
                        : "no mail transport: generated, not delivered"}
                  </span>
                </span>
                {canGenerate && (
                  <Button
                    variant="outline"
                    size="xs"
                    disabled={generate.isPending}
                    onClick={() => generate.mutate(s.id)}
                  >
                    Generate now
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
        {generate.data && (
          <p className="text-sm text-muted-foreground">
            {generate.data.created ? "Generated" : "Already generated for this period"}: report{" "}
            {generate.data.report.id} · {delivery[generate.data.report.deliveryStatus]}
          </p>
        )}
        {generate.error && <p className="text-sm text-destructive">{generate.error.message}</p>}
        {isAdmin && (
          <form
            className="flex flex-wrap items-end gap-2 text-sm"
            onSubmit={(e) => {
              e.preventDefault();
              create.mutate(
                {
                  name: form.name,
                  cadence: form.cadence as ReportSchedule["cadence"],
                  enabled: true,
                  params: {
                    accountId: 0,
                    platform: "",
                    projectId: 0,
                    groupBy: "campaign",
                    compare: "previous",
                  },
                  recipients: form.recipients
                    .split(",")
                    .map((r) => r.trim())
                    .filter(Boolean),
                },
                { onSuccess: () => setForm({ name: "", cadence: "daily", recipients: "" }) },
              );
            }}
          >
            <Input
              placeholder="Schedule name"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              className="max-w-xs"
            />
            <select
              className="rounded-md border bg-background px-2 py-1"
              value={form.cadence}
              onChange={(e) => setForm({ ...form, cadence: e.target.value })}
            >
              <option value="daily">Daily</option>
              <option value="weekly">Weekly</option>
              <option value="monthly">Monthly</option>
              <option value="quarterly">Quarterly</option>
            </select>
            <Input
              placeholder="Recipients, comma separated (optional)"
              value={form.recipients}
              onChange={(e) => setForm({ ...form, recipients: e.target.value })}
              className="max-w-sm"
            />
            <Button type="submit" size="sm" disabled={create.isPending}>
              Add schedule
            </Button>
            {create.error && <span className="text-destructive">{create.error.message}</span>}
          </form>
        )}
      </div>

      <div className="space-y-3">
        <h2 className="font-semibold">Generated</h2>
        {reports.data && reports.data.reports.length === 0 && (
          <p className="text-muted-foreground">Nothing generated yet.</p>
        )}
        {reports.data && reports.data.reports.length > 0 && (
          <ul className="divide-y border-y">
            {reports.data.reports.map((r: Report) => (
              <li key={r.id} className="py-2 text-sm">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span>
                    <span className="font-medium">{r.cadence}</span> {r.periodStart} to{" "}
                    {r.periodEnd}{" "}
                    <span className="text-muted-foreground">
                      · generated {new Date(r.generatedAt).toLocaleString()} · formula{" "}
                      {r.formulaVersion}
                    </span>
                  </span>
                  <span
                    className={
                      r.deliveryStatus === "failed" || r.status === "failed"
                        ? "text-destructive"
                        : "text-muted-foreground"
                    }
                  >
                    {r.status === "failed" ? `Failed: ${r.error}` : delivery[r.deliveryStatus]}
                    {r.deliveryDetail ? ` (${r.deliveryDetail})` : ""}
                  </span>
                  <Button
                    variant="ghost"
                    size="xs"
                    onClick={() => setOpen(open === r.id ? null : r.id)}
                  >
                    {open === r.id ? "Hide" : "Show"}
                  </Button>
                </div>
                {open === r.id && detail.data?.result && <ReportBody report={detail.data} />}
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

function ReportBody({ report }: { report: Report }) {
  const res = report.result;
  if (!res) return null;
  return (
    <div className="mt-2 space-y-1 rounded-lg border p-3 text-sm">
      <div className="text-muted-foreground">
        Figures{" "}
        {res.freshness.observedThrough
          ? `observed through ${res.freshness.observedThrough}`
          : "never read"}
        {res.freshness.partial ? " (partial)" : ""} · {res.coverage.matchedCount} of{" "}
        {res.coverage.eligibleCount} leads matched
      </div>
      {res.coverage.dependenciesMissing.map((d: string) => (
        <div key={d} className="text-destructive">
          {d}
        </div>
      ))}
      <table className="w-full">
        <tbody className="divide-y">
          {[...res.rows, res.summary].map((row) => (
            <tr key={row.key}>
              <td className="py-1">{row.label}</td>
              <td className="py-1">
                {formatMoney(row.metrics.spend.minor, row.metrics.spend.currency || "AED")}
              </td>
              <td className="py-1">{row.metrics.leads} leads</td>
              <td className="py-1">{row.metrics.qualifiedLeads ?? NOT_AVAILABLE} qualified</td>
              <td className="py-1">{row.metrics.deals ?? NOT_AVAILABLE} deals</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
