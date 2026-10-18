import { useRuns } from "@/api/runs";
import { RunStatus } from "@/components/run-status";

// Recent background work in this workspace: imports, refreshes, reports.
export function RunsPage() {
  const runs = useRuns();
  return (
    <section className="space-y-6">
      <h1 className="text-3xl font-semibold tracking-tight">Runs</h1>
      {runs.isPending && <p className="text-muted-foreground">Loading…</p>}
      {runs.error && <p className="text-destructive">{runs.error.message}</p>}
      {runs.data && runs.data.runs.length === 0 && (
        <p className="text-muted-foreground">
          Nothing has run yet. Imports and refreshes appear here once connections exist.
        </p>
      )}
      {runs.data && runs.data.runs.length > 0 && (
        <ul className="divide-y border-y">
          {runs.data.runs.map((r) => (
            <li key={r.id} className="space-y-1 py-3">
              <div className="flex items-baseline justify-between gap-4">
                <span>
                  <span className="font-medium">{r.kind}</span>{" "}
                  <span className="text-muted-foreground">{r.occurrenceKey}</span>
                </span>
                <span className="text-sm text-muted-foreground">
                  {new Date(r.createdAt).toLocaleString()}
                </span>
              </div>
              <RunStatus run={r} />
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
