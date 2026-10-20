import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, Download, Upload } from "lucide-react";
import { useBrand } from "@/api/brands";
import { useCreativeOverview, useCreativeProject, useSetUpCreative } from "@/api/creative";
import { usePages } from "@/api/pages";
import { usePlans } from "@/api/plans";
import { useBriefs } from "@/api/research";
import type { Ad, CreativeProject } from "@/api/types.gen";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { Button, buttonVariants } from "@/components/ui/button";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { BrandKitForm } from "@/creative/brand-kit-form";
import { RendersTab } from "@/creative/renders-tab";
import { SettingsTab } from "@/creative/settings-tab";
import { ReviewTally } from "@/creative/ui";
import { useProjects } from "@/engine/api/projects";
import { formatRelative } from "@/lib/format";
import { CLASS, factValue } from "./brand";
import { ConceptsTab, ReviewLinkButtons } from "./creative-project";
import { AskForPage } from "./studio/pages";

export type ProjectTab = "overview" | "concepts" | "renders" | "pages" | "brand" | "settings";

const TABS: ProjectTab[] = ["overview", "concepts", "renders", "pages", "brand", "settings"];

export function projectTabSearch(raw: Record<string, unknown>): { tab?: ProjectTab } {
  const t = raw.tab;
  // "concepts" was the old Creatives default, kept for links from before.
  return typeof t === "string" && (TABS as string[]).includes(t) && t !== "overview"
    ? { tab: t as ProjectTab }
    : {};
}

// A project is one development, such as The Wadi: its own name, kit, facts,
// offers, ads and pages. Reportage, the developer behind it, sits under
// every project rather than in front of it.
export function ProjectPage({
  projectId,
  tab = "overview",
}: {
  projectId: number;
  tab?: ProjectTab;
}) {
  const projects = useProjects();
  const navigate = useNavigate();
  const session = useSession();
  const canEdit = session.role !== "viewer";
  const project = projects.data?.projects.find((p) => p.id === projectId);
  const brandId = project?.brandId ?? null;
  const brand = useBrand(brandId);
  const creative = useCreativeOverview();
  const studio = creative.data?.projects.find((p) => p.projectId === projectId) ?? null;
  const studioProject = useCreativeProject(studio ? projectId : 0);

  if (projects.isPending) return <LoadingRows rows={8} />;
  if (projects.error) return <ErrorNote error={projects.error} />;
  if (!project) {
    return (
      <Panel>
        <EmptyState title="Project not found" />
      </Panel>
    );
  }
  const p = studio && studioProject.data ? studioProject.data : null;
  const ads = p?.concepts.flatMap((c) => c.ads) ?? [];
  const count = (s: Ad["status"]) => ads.filter((a) => a.status === s).length;

  return (
    <section>
      <Link
        to="/projects"
        className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> Projects
      </Link>
      <PageHeader
        title={project.name}
        subtitle={
          <span className="flex flex-wrap items-center gap-x-4 gap-y-1">
            <span>
              {brand.data ? (
                <>
                  By{" "}
                  <Link
                    to="/brands/$brandId"
                    params={{ brandId: String(brand.data.brand.id) }}
                    className="underline-offset-4 hover:underline"
                  >
                    {brand.data.brand.name}
                  </Link>
                </>
              ) : (
                "Not under a developer yet"
              )}
              {p && (
                <>
                  {" "}
                  · {p.concepts.length} {p.concepts.length === 1 ? "concept" : "concepts"} ·{" "}
                  {ads.length} {ads.length === 1 ? "ad" : "ads"}
                </>
              )}
            </span>
            {p && (
              <ReviewTally
                approved={count("approved")}
                revise={count("revise")}
                waiting={count("waiting")}
              />
            )}
          </span>
        }
        actions={
          p && (
            <>
              <ReviewLinkButtons settings={p.settings} />
              <a
                href={`/api/creative/projects/${projectId}/download?set=approved`}
                className={buttonVariants({ variant: "outline" })}
              >
                <Download />
                Approved
              </a>
              {canEdit && (
                <Link
                  to="/creative/$projectId/upload"
                  params={{ projectId: String(projectId) }}
                  className={buttonVariants()}
                >
                  <Upload />
                  Upload ads
                </Link>
              )}
            </>
          )
        }
      />
      <Segmented
        label="Project sections"
        value={tab}
        onChange={(t) =>
          navigate({
            to: "/projects/$projectId",
            params: { projectId: String(projectId) },
            search: t === "overview" ? {} : { tab: t },
            replace: true,
          })
        }
        options={[
          { value: "overview", label: "Overview" },
          { value: "concepts", label: "Concepts", count: p?.concepts.length },
          { value: "renders", label: "Renders", count: p?.renders.filter((r) => !r.hidden).length },
          { value: "pages", label: "Landing pages" },
          { value: "brand", label: "Brand kit" },
          { value: "settings", label: "Settings" },
        ]}
        className="mb-8"
      />
      {tab === "overview" && <Overview projectId={projectId} brandId={brandId} studio={p} />}
      {tab === "pages" && <PagesTab projectId={projectId} brandId={brandId} />}
      {(tab === "concepts" || tab === "renders" || tab === "brand" || tab === "settings") &&
        (studio === null && creative.data ? (
          <SetUpStudio projectId={projectId} canEdit={canEdit} />
        ) : !p ? (
          <LoadingRows rows={6} />
        ) : tab === "concepts" ? (
          <ConceptsTab project={p} canEdit={canEdit} />
        ) : tab === "renders" ? (
          <RendersTab project={p} canEdit={canEdit} />
        ) : tab === "brand" ? (
          <BrandKitForm settings={p.settings} canEdit={canEdit} />
        ) : (
          <SettingsTab settings={p.settings} canEdit={canEdit} />
        ))}
    </section>
  );
}

function SetUpStudio({ projectId, canEdit }: { projectId: number; canEdit: boolean }) {
  const setUp = useSetUpCreative();
  return (
    <Panel>
      <EmptyState title="Creatives aren't set up for this project">
        {canEdit ? (
          <span className="mt-4 flex flex-col items-center gap-2">
            Setting up gives the project a file prefix and a review link.
            <Button disabled={setUp.isPending} onClick={() => setUp.mutate(projectId)}>
              Set up
            </Button>
            {setUp.error && <span className="text-destructive">{setUp.error.message}</span>}
          </span>
        ) : (
          "Someone on the marketing team sets it up."
        )}
      </EmptyState>
    </Panel>
  );
}

function Overview({
  projectId,
  brandId,
  studio,
}: {
  projectId: number;
  brandId: number | null;
  studio: CreativeProject | null;
}) {
  const brand = useBrand(brandId);
  const pages = usePages();
  const plans = usePlans();
  const briefs = useBriefs();
  if (brandId !== null && brand.isPending) return <LoadingRows rows={6} />;
  const d = brand.data;
  const facts = d?.facts.filter((f) => f.projectId === projectId || f.projectId === null) ?? [];
  const offers = d?.offers.filter((o) => o.projectId === projectId || o.projectId === null) ?? [];
  const projectPages = (pages.data?.pages ?? []).filter((x) => x.projectId === projectId);
  const projectPlans = (plans.data?.plans ?? []).filter((x) => x.projectId === projectId);
  const projectBriefs = (briefs.data?.briefs ?? []).filter((x) => x.projectId === projectId);
  const ads = studio?.concepts.flatMap((c) => c.ads) ?? [];

  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_380px]">
      <div className="space-y-6">
        <Panel
          title="What Adx may say"
          description="Approved facts are the only figures that reach ads and pages. Facts recorded for the developer apply to every project."
          action={
            brandId !== null && (
              <Link
                to="/brands/$brandId"
                params={{ brandId: String(brandId) }}
                className="text-xs whitespace-nowrap text-muted-foreground hover:text-foreground"
              >
                Record a fact
              </Link>
            )
          }
          bodyClassName="p-0"
        >
          {facts.length === 0 ? (
            <EmptyState title="No facts yet" />
          ) : (
            <ul className="divide-y">
              {facts.map((f) => (
                <li key={f.id} className="flex items-start gap-4 px-6 py-4">
                  <StatusMark status={CLASS[f.class].status} className="mt-1.5" />
                  <div className="min-w-0 flex-1">
                    <p className="font-mono text-xs text-muted-foreground">
                      {f.key}
                      {f.projectId === null && " · all projects"}
                    </p>
                    <p className="mt-0.5">{factValue(f)}</p>
                  </div>
                  {f.class !== "approved" && (
                    <span className="text-xs text-muted-foreground">{CLASS[f.class].label}</span>
                  )}
                </li>
              ))}
            </ul>
          )}
        </Panel>

        <Panel title="Offers" bodyClassName="p-0">
          {offers.length === 0 ? (
            <EmptyState title="No offers" />
          ) : (
            <ul className="divide-y">
              {offers.map((o) => (
                <li key={o.id} className="flex items-start gap-4 px-6 py-4">
                  <StatusMark
                    status={o.current.state === "approved" ? "ready" : "pending"}
                    className="mt-1.5"
                  />
                  <div className="min-w-0 flex-1">
                    <p className="font-medium">{o.name}</p>
                    <p className="mt-0.5 text-sm text-muted-foreground">
                      {o.current.document.headline}
                    </p>
                    {o.current.document.headlineAr && (
                      <p
                        className="mt-0.5 text-left text-sm text-muted-foreground"
                        dir="rtl"
                        lang="ar"
                      >
                        {o.current.document.headlineAr}
                      </p>
                    )}
                  </div>
                  <span className="text-xs text-muted-foreground capitalize">
                    {o.current.state}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>

      <div className="space-y-6">
        <SideList
          title="Creatives"
          open={{ tab: "concepts", projectId }}
          empty={studio ? undefined : "Not set up yet."}
          rows={
            studio
              ? studio.concepts.map((c) => ({
                  label: `${c.code} ${c.name}`,
                  status: (c.ads.some((a) => a.status === "revise")
                    ? "caution"
                    : c.ads.length > 0 && c.ads.every((a) => a.status === "approved")
                      ? "ready"
                      : "pending") as Status,
                  note: `${c.ads.length} ${c.ads.length === 1 ? "ad" : "ads"}`,
                }))
              : []
          }
          footer={studio && ads.length > 0 ? `${ads.length} ads in all` : undefined}
        />
        <SideList
          title="Landing pages"
          open={{ tab: "pages", projectId }}
          empty={projectPages.length === 0 ? "No pages yet." : undefined}
          rows={projectPages.map((x) => ({
            label: x.name,
            status: x.publishedVersion === null ? "pending" : "ready",
            note: x.publishedVersion === null ? "Draft" : `Live v${x.publishedVersion}`,
          }))}
        />
        <SideList
          title="Media plans"
          open={{ to: "plans" }}
          empty={projectPlans.length === 0 ? "No plans yet." : undefined}
          rows={projectPlans.map((x) => ({
            label: x.title,
            status: x.accepted ? "ready" : "pending",
            note: `v${x.latestVersion}${x.accepted ? " · accepted" : ""}`,
          }))}
        />
        <SideList
          title="Research"
          open={{ to: "research" }}
          empty={projectBriefs.length === 0 ? "No research yet." : undefined}
          rows={projectBriefs.map((x) => ({
            label: x.title,
            status: "pending",
            note: formatRelative(x.createdAt),
          }))}
        />
      </div>
    </div>
  );
}

type SideRow = { label: string; status: Status; note?: string };
type Open = { tab: ProjectTab; projectId: number } | { to: "plans" | "research" };

function SideList({
  title,
  open,
  rows,
  empty,
  footer,
}: {
  title: string;
  open: Open;
  rows: SideRow[];
  empty?: string;
  footer?: string;
}) {
  const cls = "text-xs text-muted-foreground hover:text-foreground";
  const action =
    "tab" in open ? (
      <Link
        to="/projects/$projectId"
        params={{ projectId: String(open.projectId) }}
        search={{ tab: open.tab }}
        className={cls}
      >
        Open
      </Link>
    ) : (
      <Link to={open.to === "plans" ? "/studio/plans" : "/studio/research"} className={cls}>
        Open
      </Link>
    );
  return (
    <Panel title={title} action={action} bodyClassName="p-0">
      {empty ? (
        <p className="px-6 py-5 text-sm text-muted-foreground">{empty}</p>
      ) : (
        <>
          <ul className="divide-y">
            {rows.map((r) => (
              <li key={r.label} className="flex items-center gap-3 px-6 py-3.5 text-sm">
                <StatusMark status={r.status} />
                <span className="min-w-0 flex-1 truncate">{r.label}</span>
                {r.note && <span className="text-xs text-muted-foreground">{r.note}</span>}
              </li>
            ))}
          </ul>
          {footer && <p className="border-t px-6 py-3 text-xs text-muted-foreground">{footer}</p>}
        </>
      )}
    </Panel>
  );
}

function PagesTab({ projectId, brandId }: { projectId: number; brandId: number | null }) {
  const pages = usePages();
  const list = (pages.data?.pages ?? []).filter((p) => p.projectId === projectId);
  return (
    <div className="space-y-6">
      {brandId !== null && <AskForPage fixed={{ brandId, projectId }} />}
      {pages.isPending && <LoadingRows rows={4} />}
      {pages.error && <ErrorNote error={pages.error} />}
      {pages.data && list.length === 0 && (
        <Panel>
          <EmptyState title="No pages for this project yet">
            Ask Adx above, or start one on the{" "}
            <Link to="/engine/pages" className="underline underline-offset-4">
              engine's pages screen
            </Link>
            .
          </EmptyState>
        </Panel>
      )}
      {list.length > 0 && (
        <Panel bodyClassName="p-0">
          <Table>
            <THead>
              <TR>
                <TH>Page</TH>
                <TH>Standing</TH>
                <TH>Language</TH>
                <TH>Updated</TH>
              </TR>
            </THead>
            <TBody>
              {list.map((x) => (
                <TR key={x.id}>
                  <TD>
                    <Link
                      to="/studio/pages/$pageId"
                      params={{ pageId: String(x.id) }}
                      className="font-medium hover:underline"
                    >
                      {x.name}
                    </Link>
                    <p className="font-mono text-xs text-muted-foreground">{x.publicUrl}</p>
                  </TD>
                  <TD>
                    {x.publishedVersion === null ? (
                      <StatusChip status="pending">Draft</StatusChip>
                    ) : x.publishedVersion < x.latestVersion ? (
                      <StatusChip status="caution">
                        Live v{x.publishedVersion}, draft v{x.latestVersion}
                      </StatusChip>
                    ) : (
                      <StatusChip status="ready">Live v{x.publishedVersion}</StatusChip>
                    )}
                  </TD>
                  <TD>{x.language === "ar" ? "Arabic" : "English"}</TD>
                  <TD className="text-muted-foreground">{formatRelative(x.updatedAt)}</TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </Panel>
      )}
    </div>
  );
}
