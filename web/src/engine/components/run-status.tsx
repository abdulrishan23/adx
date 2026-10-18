import type { Run } from "@/api/types.gen";

const label: Record<Run["state"], string> = {
  queued: "Queued",
  running: "Running",
  succeeded: "Done",
  partial: "Partial",
  failed: "Failed",
  interrupted: "Interrupted",
};

const tone: Record<Run["state"], string> = {
  queued: "text-muted-foreground",
  running: "text-foreground",
  succeeded: "text-foreground",
  partial: "text-destructive",
  failed: "text-destructive",
  interrupted: "text-destructive",
};

// One line about a run: its state, what it read through, and why it failed.
// A partial run is never shown as done, and "observed through" is the only
// freshness claim made.
export function RunStatus({ run }: { run: Run }) {
  return (
    <span className="inline-flex flex-wrap items-baseline gap-x-2 text-sm">
      <span className={tone[run.state]}>{label[run.state]}</span>
      {run.completeness === "partial" && (
        <span className="text-destructive">(incomplete read)</span>
      )}
      <span className="text-muted-foreground">
        <Freshness run={run} />
      </span>
      {run.error && <span className="text-destructive">{run.error}</span>}
      {run.attempt > 0 && <span className="text-muted-foreground">attempt {run.attempt + 1}</span>}
    </span>
  );
}

export function Freshness({ run }: { run: Run }) {
  if (run.observedThrough) {
    return <>observed through {new Date(run.observedThrough).toLocaleString()}</>;
  }
  if (run.finishedAt) {
    return <>finished {new Date(run.finishedAt).toLocaleString()}</>;
  }
  return <>not yet observed</>;
}
