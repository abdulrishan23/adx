import { createContext, type ReactNode, useContext, useState } from "react";
import { useProjectsList } from "@/api/adx";
import type { Scope } from "@/api/models";
import { PlatformMark } from "@/components/platform";
import { Select } from "@/components/ui/select";
import { cn } from "@/lib/utils";

// Scope is what every data screen is filtered by: the period, the platform and
// the project. It's chosen once in the bar at the top and kept for the visit.

const DEFAULT_SCOPE: Scope = { days: 30, platform: "all", projectId: "all" };
const STORAGE_KEY = "adx-scope";

type ScopeContextValue = { scope: Scope; setScope: (next: Scope) => void };

const ScopeContext = createContext<ScopeContextValue | null>(null);

function readStored(): Scope {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    return raw ? { ...DEFAULT_SCOPE, ...(JSON.parse(raw) as Partial<Scope>) } : DEFAULT_SCOPE;
  } catch {
    return DEFAULT_SCOPE;
  }
}

export function ScopeProvider({ children }: { children: ReactNode }) {
  const [scope, setScopeState] = useState<Scope>(readStored);
  function setScope(next: Scope) {
    setScopeState(next);
    try {
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify(next));
    } catch {
      // Storage can be blocked; the scope still holds for this page.
    }
  }
  return <ScopeContext.Provider value={{ scope, setScope }}>{children}</ScopeContext.Provider>;
}

export function useScope(): Scope {
  const value = useContext(ScopeContext);
  return value?.scope ?? DEFAULT_SCOPE;
}

export function useSetScope() {
  const value = useContext(ScopeContext);
  if (!value) throw new Error("useSetScope must be used inside ScopeProvider");
  return value;
}

export const PERIOD_LABEL: Record<Scope["days"], string> = {
  7: "Last 7 days",
  30: "Last 30 days",
  90: "Last 90 days",
};

export const PLATFORM_LABEL = { all: "All platforms", google: "Google", meta: "Meta" } as const;

export function ScopeBar({
  className,
  showPlatform = true,
  showProject = true,
}: {
  className?: string;
  showPlatform?: boolean;
  showProject?: boolean;
}) {
  const { scope, setScope } = useSetScope();
  const projects = useProjectsList();
  return (
    <div className={cn("flex flex-wrap items-center gap-2", className)}>
      {showPlatform && (
        <Select
          aria-label="Platform"
          value={scope.platform}
          onValueChange={(platform) => setScope({ ...scope, platform })}
          options={(["all", "google", "meta"] as const).map((p) => ({
            value: p,
            label: PLATFORM_LABEL[p],
            icon: p === "all" ? undefined : <PlatformMark platform={p} />,
          }))}
        />
      )}
      {showProject && (
        <Select
          aria-label="Project"
          value={String(scope.projectId)}
          onValueChange={(v) => setScope({ ...scope, projectId: v === "all" ? "all" : Number(v) })}
          options={[
            { value: "all", label: "All projects" },
            ...(projects.data ?? []).map((p) => ({ value: String(p.id), label: p.name })),
          ]}
        />
      )}
      <Select
        aria-label="Period"
        value={String(scope.days)}
        onValueChange={(v) => setScope({ ...scope, days: Number(v) as Scope["days"] })}
        options={([7, 30, 90] as const).map((d) => ({ value: String(d), label: PERIOD_LABEL[d] }))}
      />
    </div>
  );
}
