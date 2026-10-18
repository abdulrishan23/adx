import { useRouterState } from "@tanstack/react-router";
import { findNavItem } from "@/app/nav";
import { StatusChip } from "@/components/ui/status-chip";

// ComingSoon stands in for a screen that isn't built yet, so the whole map of
// Adx is visible from day one. It reads the nav entry for the current path.
export function ComingSoonPage() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const item = findNavItem(pathname);

  return (
    <section className="space-y-6">
      <header className="flex flex-wrap items-center gap-3">
        <h1 className="text-3xl font-semibold">{item?.label ?? "Coming soon"}</h1>
        <StatusChip status="pending">Coming soon</StatusChip>
      </header>
      {item && <p className="max-w-2xl text-muted-foreground">{item.summary}</p>}
      <div className="flex max-w-2xl items-start gap-4 rounded-lg border border-dashed p-6">
        {item && <item.icon className="mt-0.5 size-5 shrink-0 text-muted-foreground" />}
        <p className="text-sm text-muted-foreground">
          This screen isn't built yet. It will appear here when it is.
        </p>
      </div>
    </section>
  );
}
