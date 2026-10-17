import { useReadiness } from "@/api/ops";

// Shown in the footer. Loading, ready, not ready (naming the failed checks)
// and unreachable are all distinct, so a stale page never looks healthy.
export function ServerStatus() {
  const ready = useReadiness();
  if (ready.isPending) {
    return <span className="text-muted-foreground">Checking server…</span>;
  }
  if (ready.error) {
    return <span className="text-destructive">Server unreachable</span>;
  }
  const failed = ready.data.checks.filter((c) => !c.ok).map((c) => c.name);
  if (!ready.data.ready) {
    return (
      <span className="text-destructive">
        Server not ready{failed.length > 0 ? `: ${failed.join(", ")}` : ""}
      </span>
    );
  }
  return <span className="text-muted-foreground">Server ready</span>;
}
