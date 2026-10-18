import { useReadiness } from "@/engine/api/ops";
import { StatusMark, type StatusTone } from "@/engine/components/status";

// Shown in the sidebar footer. Loading, ready, not ready (naming the failed
// checks) and unreachable are all distinct, so a stale page never looks
// healthy. Compact shows the mark alone with the words as its title, for the
// collapsed sidebar.
export function ServerStatus({ compact = false }: { compact?: boolean }) {
  const ready = useReadiness();
  let tone: StatusTone = "pending";
  let text = "Checking server…";
  if (ready.error) {
    tone = "error";
    text = "Server unreachable";
  } else if (ready.data) {
    const failed = ready.data.checks.filter((c) => !c.ok).map((c) => c.name);
    if (ready.data.ready) {
      tone = "ready";
      text = "Server ready";
    } else {
      tone = "error";
      text = `Server not ready${failed.length > 0 ? `: ${failed.join(", ")}` : ""}`;
    }
  }
  if (compact) {
    return (
      <span title={text} className="inline-flex">
        <StatusMark tone={tone} />
        <span className="sr-only">{text}</span>
      </span>
    );
  }
  return (
    <span
      className={`inline-flex items-center gap-1.5 ${tone === "error" ? "text-destructive" : "text-muted-foreground"}`}
    >
      <StatusMark tone={tone} />
      {text}
    </span>
  );
}
