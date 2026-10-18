import { Link } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { type LoopState, useLoopStates } from "@/api/create";
import { useCreativeOverview } from "@/api/creative";
import { useProjects } from "@/api/projects";
import { useSession } from "@/app/session";
import { ErrorNote, LoadingRows, PageHeader } from "@/components/page";
import { buttonVariants } from "@/components/ui/button";
import { type Status, StatusMark } from "@/components/ui/status-chip";
import { released } from "@/lib/release";
import { cn } from "@/lib/utils";

const STEPS: { key: keyof LoopState["steps"]; label: string }[] = [
  { key: "brand", label: "Brand" },
  { key: "research", label: "Research" },
  { key: "plan", label: "Plan" },
  { key: "creative", label: "Creative" },
  { key: "page", label: "Page" },
  { key: "launch", label: "Launch" },
];

// Brands is where Create starts: each development Reportage markets, and
// where it is on the way from brand knowledge to a live campaign.
export function ProjectsPage() {
  const projects = useProjects();
  const loops = useLoopStates();
  const creative = useCreativeOverview();
  const session = useSession();

  const creativeState = (projectId: number): Status => {
    const c = creative.data?.projects.find((p) => p.projectId === projectId);
    if (!c) return "pending";
    if (c.revise > 0) return "caution";
    return c.approved > 0 ? "ready" : "pending";
  };

  return (
    <>
      <PageHeader
        title="Brands"
        subtitle="Each development Reportage markets, and where it is on the way from brand knowledge to a live campaign."
        actions={
          session.role === "admin" && (
            <Link to="/brands/new" className={buttonVariants()}>
              <Plus /> Start a brand
            </Link>
          )
        }
      />
      {projects.isPending && <LoadingRows rows={6} />}
      {projects.error && <ErrorNote error={projects.error} />}
      {projects.data && (
        <div className="space-y-5">
          <ol className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {projects.data.projects.map((p, i) => {
              const loop = loops.data?.find((l) => l.projectId === p.id);
              const steps = { ...(loop?.steps ?? emptySteps()), creative: creativeState(p.id) };
              const live = Object.values(steps).some((s) => s !== "pending");
              return (
                <li key={p.id} className="animate-rise" style={{ animationDelay: `${i * 40}ms` }}>
                  <Link
                    to="/brands/$projectId"
                    params={{ projectId: String(p.id) }}
                    className="block border bg-card p-5 transition-colors hover:border-foreground/40"
                  >
                    <p className="flex items-baseline justify-between gap-3">
                      <span className="font-heading text-xl font-semibold">{p.name}</span>
                      <span className="font-mono text-[0.65rem] tracking-wider text-muted-foreground uppercase">
                        Open →
                      </span>
                    </p>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {live ? (loop?.note ?? "") : "Not started"}
                    </p>
                    <div className="mt-4 flex gap-1">
                      {STEPS.map((s) => (
                        <span
                          key={s.key}
                          title={`${s.label}: ${steps[s.key]}`}
                          className={cn("h-3 flex-1", {
                            "bg-status-ready": steps[s.key] === "ready",
                            "bg-status-caution": steps[s.key] === "caution",
                            "bg-status-error": steps[s.key] === "error",
                            "bg-muted": steps[s.key] === "pending",
                          })}
                        />
                      ))}
                    </div>
                    <div className="mt-1.5 flex gap-1">
                      {STEPS.map((s) => (
                        <span
                          key={s.key}
                          className="flex-1 text-center font-mono text-[0.55rem] tracking-wider text-muted-foreground uppercase"
                        >
                          {s.label}
                        </span>
                      ))}
                    </div>
                  </Link>
                </li>
              );
            })}
          </ol>
          {!released(2) && (
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <StatusMark status="pending" /> Brand, Research, Plan, Page and Launch arrive with
              phases 2 and 3. Creatives and campaigns are live.
            </p>
          )}
        </div>
      )}
    </>
  );
}

const emptySteps = (): LoopState["steps"] => ({
  brand: "pending",
  research: "pending",
  plan: "pending",
  creative: "pending",
  page: "pending",
  launch: "pending",
});
