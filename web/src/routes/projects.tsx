import { Link } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { useBrands } from "@/api/brands";
import { useCreativeOverview } from "@/api/creative";
import { usePages } from "@/api/pages";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { buttonVariants } from "@/components/ui/button";
import { type Status, StatusMark } from "@/components/ui/status-chip";
import { ReviewTally } from "@/creative/ui";
import { useProjects } from "@/engine/api/projects";

// Projects is where Studio starts: each development Reportage markets, with
// its ads and pages at a glance. Competitors live on their own screen.
export function ProjectsPage() {
  const projects = useProjects();
  const brands = useBrands();
  const creative = useCreativeOverview();
  const pages = usePages();
  const session = useSession();
  const own = new Set(brands.data?.brands.filter((b) => b.kind === "own").map((b) => b.id) ?? []);
  const all = projects.data?.projects ?? [];
  const ours = all.filter((p) => p.brandId !== null && own.has(p.brandId));
  const loose = all.filter((p) => p.brandId === null);
  return (
    <>
      <PageHeader
        title="Projects"
        subtitle="Each development Adx markets, with its own facts, offers, ads and pages."
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
                const pp = (pages.data?.pages ?? []).filter((x) => x.projectId === p.id);
                const live = pp.filter((x) => x.publishedVersion !== null).length;
                const status: Status = s?.revise ? "caution" : s && s.ads > 0 ? "ready" : "pending";
                return (
                  <Link
                    key={p.id}
                    to="/projects/$projectId"
                    params={{ projectId: String(p.id) }}
                    className="flex animate-rise flex-col border bg-card p-6 transition-colors hover:border-foreground/40"
                    style={{ animationDelay: `${i * 40}ms` }}
                  >
                    <h2 className="flex items-center gap-2.5 font-heading text-xl font-semibold tracking-tight">
                      <StatusMark status={status} />
                      {p.name}
                    </h2>
                    <p className="mt-3 text-sm text-muted-foreground">
                      {s
                        ? `${s.concepts} ${s.concepts === 1 ? "concept" : "concepts"} · ${s.ads} ${s.ads === 1 ? "ad" : "ads"}`
                        : "No creatives yet"}
                      {" · "}
                      {pp.length === 0
                        ? "no pages"
                        : `${pp.length} ${pp.length === 1 ? "page" : "pages"}, ${live} live`}
                    </p>
                    {s && s.ads > 0 && (
                      <div className="mt-3">
                        <ReviewTally approved={s.approved} revise={s.revise} waiting={s.waiting} />
                      </div>
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

// Competitors are the other developers Adx watches; research compares
// against what it reads of them.
export function CompetitorsPage() {
  const brands = useBrands();
  const list = brands.data?.brands.filter((b) => b.kind === "competitor") ?? [];
  return (
    <>
      <PageHeader
        title="Competitors"
        subtitle="The developers Adx watches. Research reads their pages and compares; nothing here is ever stated as ours."
        actions={
          <Link to="/engine/brands" className={buttonVariants({ variant: "outline", size: "sm" })}>
            Add a competitor
          </Link>
        }
      />
      {brands.isPending && <LoadingRows rows={4} />}
      {brands.error && <ErrorNote error={brands.error} />}
      {brands.data && list.length === 0 && (
        <Panel>
          <EmptyState title="No competitors yet">
            Name them when starting a project, or add one on the engine's brands screen.
          </EmptyState>
        </Panel>
      )}
      {list.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {list.map((b) => (
            <Link
              key={b.id}
              to="/brands/$brandId"
              params={{ brandId: String(b.id) }}
              className="flex flex-col border bg-card p-6 transition-colors hover:border-foreground/40"
            >
              <h2 className="font-heading text-xl font-semibold tracking-tight">{b.name}</h2>
              <p className="mt-2 text-sm text-muted-foreground">
                {b.projectCount === 0
                  ? "No projects tracked"
                  : `${b.projectCount} ${b.projectCount === 1 ? "project" : "projects"} tracked`}
              </p>
              {b.notes && (
                <p className="mt-3 line-clamp-2 text-sm text-muted-foreground">{b.notes}</p>
              )}
            </Link>
          ))}
        </div>
      )}
    </>
  );
}
