import { Link, useNavigate } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import { useBrands } from "@/api/brands";
import { useCreativeOverview } from "@/api/creative";
import { usePages } from "@/api/pages";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { buttonVariants } from "@/components/ui/button";
import { ReviewTally } from "@/creative/ui";
import { useProjects } from "@/engine/api/projects";
import { CampaignsView } from "./studio-campaigns";

// Projects is where Studio starts: each development Reportage markets, with
// its ads and pages at a glance. Competitors live on their own screen.
export function ProjectsPage({ view }: { view: "projects" | "campaigns" }) {
  const navigate = useNavigate();
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
        subtitle={
          view === "campaigns"
            ? "Every push across projects, with its window, budget and where it stands."
            : "Each development Adx markets, with its campaigns, pictures, brand and research."
        }
        actions={
          <>
            <Segmented
              label="View"
              value={view}
              onChange={(v) =>
                navigate({ to: "/projects", search: v === "campaigns" ? { view: v } : {} })
              }
              options={[
                { value: "projects", label: "Projects" },
                { value: "campaigns", label: "Campaigns" },
              ]}
            />
            {session.role === "admin" && (
              <Link to="/projects/new" className={buttonVariants()}>
                <Plus /> New project
              </Link>
            )}
          </>
        }
      />
      {view === "campaigns" && <CampaignsView />}
      {view === "projects" && (projects.isPending || brands.isPending) && <LoadingRows rows={4} />}
      {view === "projects" && projects.error && <ErrorNote error={projects.error} />}
      {view === "projects" && projects.data && brands.data && (
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
                return (
                  <Link
                    key={p.id}
                    to="/projects/$projectId"
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
