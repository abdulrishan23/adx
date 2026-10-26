import { Link } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { useBrands } from "@/api/brands";
import { healthOf, useCampaigns, useProjectsProgress } from "@/api/campaigns";
import { useCreativeOverview } from "@/api/creative";
import { PROJECT_SECTIONS } from "@/app/nav";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { buttonVariants } from "@/components/ui/button";
import { StatusMark } from "@/components/ui/status-chip";
import { useProjects } from "@/engine/api/projects";
import { released } from "@/lib/release";
import { cn } from "@/lib/utils";
import { STATE } from "./campaign";

// Only released sections count towards a project's health.
const RELEASED_SECTIONS = PROJECT_SECTIONS.filter((s) => released(s.phase)).map((s) => s.segment);

// Projects is where Studio starts: each development Reportage markets, with
// its campaigns at a glance. Competitors sit in each project's Research.
export function ProjectsPage() {
  const projects = useProjects();
  const brands = useBrands();
  const creative = useCreativeOverview();
  const campaigns = useCampaigns();
  // Health is a second read; the page stands without it while it loads or fails.
  const progress = useProjectsProgress();
  const session = useSession();
  const own = new Set(brands.data?.brands.filter((b) => b.kind === "own").map((b) => b.id) ?? []);
  const all = projects.data?.projects ?? [];
  const ours = all.filter((p) => p.brandId !== null && own.has(p.brandId));
  const loose = all.filter((p) => p.brandId === null);
  return (
    <>
      <PageHeader
        title="Projects"
        subtitle="Each development Adx markets, with its campaigns and the research behind them."
        actions={
          session.role === "admin" && (
            <Link to="/projects/new" className={buttonVariants()}>
              <Plus /> New project
            </Link>
          )
        }
      />
      {(projects.isPending || brands.isPending) && <LoadingRows rows={4} />}
      {projects.error && <ErrorNote error={projects.error} />}
      {projects.data && brands.data && (
        <div className="space-y-8">
          {ours.length === 0 ? (
            <Panel>
              <EmptyState title="No projects yet">
                Start one with its website and competitors, and Adx begins the research.
              </EmptyState>
            </Panel>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
              {ours.map((p, i) => {
                const s = creative.data?.projects.find((x) => x.projectId === p.id);
                const cs = (campaigns.data?.campaigns ?? []).filter((x) => x.projectId === p.id);
                const lead = cs[0];
                const leadState = lead
                  ? (STATE[lead.state] ?? { label: lead.state, status: "pending" as const })
                  : null;
                const pr = progress.data?.projects.find((x) => x.projectId === p.id);
                const health = pr ? healthOf(pr, RELEASED_SECTIONS) : null;
                return (
                  <Link
                    key={p.id}
                    to="/projects/$projectId/brand"
                    params={{ projectId: String(p.id) }}
                    className="flex animate-rise flex-col border bg-card p-6 transition-colors hover:border-foreground/40"
                    style={{ animationDelay: `${i * 40}ms` }}
                  >
                    <div className="flex h-16 items-center">
                      {s?.logo ? (
                        <img
                          src={`/api/files/${s.logo}`}
                          alt=""
                          className="h-full w-auto max-w-[70%] object-contain object-left"
                        />
                      ) : (
                        <span className="flex size-16 items-center justify-center border font-heading text-2xl font-semibold text-muted-foreground">
                          {p.name.replace(/^the\s+/i, "").charAt(0)}
                        </span>
                      )}
                    </div>
                    <h2 className="mt-4 font-heading text-xl font-semibold tracking-tight">
                      {p.name}
                    </h2>
                    {pr && health && (
                      <>
                        {/* Health: released sections ready, or what blocks the project. */}
                        <p className="mt-3 flex items-center gap-2 text-sm">
                          <StatusMark status={health.status} />
                          {pr.blocked ? (
                            <span className="truncate text-destructive">{pr.blocker}</span>
                          ) : (
                            <span>
                              {health.ready} of {health.total}
                            </span>
                          )}
                        </p>
                        {/* The next step, and whose it is. */}
                        <p className="mt-1 truncate text-sm">
                          <span
                            className={
                              pr.next.label === "Nothing waiting" ? "text-muted-foreground" : ""
                            }
                          >
                            {pr.next.label}
                          </span>
                          {pr.next.role && (
                            <span className="text-muted-foreground"> · {pr.next.role}</span>
                          )}
                        </p>
                      </>
                    )}
                    <p className={cn("text-sm text-muted-foreground", health ? "mt-1" : "mt-3")}>
                      {cs.length === 0
                        ? "No campaigns yet"
                        : `${cs.length} ${cs.length === 1 ? "campaign" : "campaigns"}`}
                    </p>
                    {lead && (
                      <p className="mt-1 flex items-center gap-2 text-sm">
                        <StatusMark status={leadState?.status ?? "pending"} />
                        <span className="truncate">{lead.name}</span>
                        <span className="text-muted-foreground">· {leadState?.label}</span>
                      </p>
                    )}
                  </Link>
                );
              })}
            </div>
          )}
          {loose.length > 0 && (
            <Panel
              title="Not under a developer yet"
              description="Attach them on the developer's page so their facts and offers have a home."
              bodyClassName="p-0"
            >
              <ul className="divide-y">
                {loose.map((p) => (
                  <li key={p.id} className="px-6 py-3.5 text-sm">
                    <Link
                      to="/projects/$projectId"
                      params={{ projectId: String(p.id) }}
                      className="hover:underline"
                    >
                      {p.name}
                    </Link>
                  </li>
                ))}
              </ul>
            </Panel>
          )}
        </div>
      )}
    </>
  );
}

// CompetitorsPanel is the developers Adx watches, in a project's Research
// tab: research reads their pages and compares; nothing here is ever stated
// as ours.
export function CompetitorsPanel() {
  const brands = useBrands();
  const list = brands.data?.brands.filter((b) => b.kind === "competitor") ?? [];
  return (
    <Panel
      title="Competitors"
      description="The developers Adx watches. Research reads their pages and compares; nothing here is ever stated as ours."
      action={
        <Link
          to="/engine/brands"
          className="text-xs whitespace-nowrap text-muted-foreground hover:text-foreground"
        >
          Add a competitor
        </Link>
      }
      bodyClassName="p-0"
    >
      {brands.isPending && <LoadingRows rows={2} />}
      {brands.error && <ErrorNote error={brands.error} />}
      {brands.data && list.length === 0 && (
        <p className="px-6 py-5 text-sm text-muted-foreground">
          No competitors yet. Name them when starting a project, or add one on the engine's brands
          screen.
        </p>
      )}
      {list.length > 0 && (
        <ul className="divide-y">
          {list.map((b) => (
            <li key={b.id}>
              <Link
                to="/brands/$brandId"
                params={{ brandId: String(b.id) }}
                className="flex items-center gap-3 px-6 py-3.5 text-sm hover:bg-muted"
              >
                <span className="min-w-0 flex-1 truncate font-medium">{b.name}</span>
                <span className="text-xs text-muted-foreground">
                  {b.projectCount === 0
                    ? "No projects tracked"
                    : `${b.projectCount} ${b.projectCount === 1 ? "project" : "projects"} tracked`}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}
