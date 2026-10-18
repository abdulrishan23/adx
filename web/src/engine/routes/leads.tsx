import { useState } from "react";
import { Button } from "@/components/ui/button";
import { useAccounts } from "@/engine/api/accounts";
import { useImportLeads, useLeadCoverage, useLeads } from "@/engine/api/leads";
import { useSession } from "@/engine/api/session";
import { NOT_AVAILABLE } from "@/engine/lib/format";

function pct(part: number, total: number): string {
  if (total === 0) return NOT_AVAILABLE;
  return `${Math.round((part / total) * 100)}%`;
}

// Leads as GoHighLevel last gave them, with the coverage numbers every
// figure depends on. Raw status is shown next to the ADX stage so an
// unmapped status is visible, not hidden.
export function LeadsPage() {
  const [stage, setStage] = useState("");
  const leads = useLeads({ stage });
  const coverage = useLeadCoverage();
  const accounts = useAccounts();
  const session = useSession();
  const importLeads = useImportLeads();
  const canImport = session.data?.role === "marketer" || session.data?.role === "admin";
  const ghlAccounts = accounts.data?.accounts.filter((a) => a.provider === "gohighlevel") ?? [];

  return (
    <section className="space-y-6">
      <div className="flex items-baseline justify-between">
        <h1 className="text-3xl font-semibold tracking-tight">Leads</h1>
        {canImport && ghlAccounts.length > 0 && (
          <div className="flex items-center gap-2">
            {ghlAccounts.map((a) => (
              <Button
                key={a.id}
                variant="outline"
                size="sm"
                disabled={importLeads.isPending}
                onClick={() => importLeads.mutate({ accountId: a.id, mode: "full", since: null })}
              >
                Import from {a.name ?? a.externalId}
              </Button>
            ))}
          </div>
        )}
      </div>
      {importLeads.data && (
        <p className="text-sm text-muted-foreground">
          {importLeads.data.created ? "Import started" : "Import already running"}: run{" "}
          {importLeads.data.run.id} ({importLeads.data.run.state}). See Runs for progress.
        </p>
      )}
      {importLeads.error && <p className="text-sm text-destructive">{importLeads.error.message}</p>}

      {coverage.data && (
        <dl className="grid grid-cols-2 gap-2 rounded-lg border p-4 text-sm sm:grid-cols-4">
          <dt className="text-muted-foreground">Leads</dt>
          <dd>{coverage.data.total}</dd>
          <dt className="text-muted-foreground">With a status</dt>
          <dd>{pct(coverage.data.withStatus, coverage.data.total)}</dd>
          <dt className="text-muted-foreground">With a stage</dt>
          <dd>
            {coverage.data.funnelConfigured
              ? pct(coverage.data.withStage, coverage.data.total)
              : "Funnel not configured"}
          </dd>
          <dt className="text-muted-foreground">With a project</dt>
          <dd>{pct(coverage.data.withProject, coverage.data.total)}</dd>
          <dt className="text-muted-foreground">Status missing on last scan</dt>
          <dd>{coverage.data.statusMissingOnLastScan}</dd>
          <dt className="text-muted-foreground">Phones shared by several leads</dt>
          <dd>{coverage.data.sharedPhones}</dd>
          <dt className="text-muted-foreground">Click-id matched</dt>
          <dd>{coverage.data.matchLevels?.click_id ?? 0}</dd>
          <dt className="text-muted-foreground">UTM matched</dt>
          <dd>{coverage.data.matchLevels?.utm ?? 0}</dd>
        </dl>
      )}

      <div className="flex items-center gap-2 text-sm">
        <label htmlFor="stage" className="text-muted-foreground">
          Stage
        </label>
        <select
          id="stage"
          className="rounded-md border bg-background px-2 py-1"
          value={stage}
          onChange={(e) => setStage(e.target.value)}
        >
          <option value="">All</option>
          {[
            "open",
            "attempted",
            "engaged",
            "meeting",
            "offer",
            "won",
            "disqualified",
            "excluded",
            "dead",
          ].map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
      </div>

      {leads.isPending && <p className="text-muted-foreground">Loading…</p>}
      {leads.error && <p className="text-destructive">{leads.error.message}</p>}
      {leads.data && leads.data.leads.length === 0 && (
        <p className="text-muted-foreground">
          No leads yet. Connect a GoHighLevel location and run an import.
        </p>
      )}
      {leads.data && leads.data.leads.length > 0 && (
        <table className="w-full text-sm">
          <thead className="text-left text-muted-foreground">
            <tr>
              <th className="py-1 font-normal">Name</th>
              <th className="py-1 font-normal">Phone</th>
              <th className="py-1 font-normal">Source</th>
              <th className="py-1 font-normal">Project</th>
              <th className="py-1 font-normal">Status (raw)</th>
              <th className="py-1 font-normal">Stage</th>
            </tr>
          </thead>
          <tbody className="divide-y border-y">
            {leads.data.leads.map((l) => (
              <tr key={l.id}>
                <td className="py-2">
                  {[l.firstName, l.lastName].filter(Boolean).join(" ") || l.externalId}
                </td>
                <td className="py-2">{l.phone ?? NOT_AVAILABLE}</td>
                <td className="py-2">
                  {l.source ?? l.sourceRaw ?? NOT_AVAILABLE}
                  {l.source === "handling_channel" && (
                    <span className="text-muted-foreground"> (bucket, not a source)</span>
                  )}
                </td>
                <td className="py-2">{l.projectRaw ?? NOT_AVAILABLE}</td>
                <td className="py-2">
                  {l.statusRaw ?? "No status"}
                  {l.statusMissingSince && (
                    <span className="text-muted-foreground"> · field missing on last scan</span>
                  )}
                </td>
                <td className="py-2">
                  {l.stage ?? <span className="text-muted-foreground">Unmapped</span>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
