import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, Plus } from "lucide-react";
import { useState } from "react";
import { useBrand } from "@/api/brands";
import { useProjectCampaigns } from "@/api/campaigns";
import { useSetUpCreative } from "@/api/creative";
import { usePages } from "@/api/pages";
import { useBriefs } from "@/api/research";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { Button, buttonVariants } from "@/components/ui/button";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { useProjects } from "@/engine/api/projects";
import { EvidencePanel, Reports, Schedules } from "@/engine/routes/research";
import { formatDate, formatRelative } from "@/lib/format";
import { CLASS, factValue, keyLabel } from "./brand";
import { STATE } from "./campaign";
import { CompetitorsPanel } from "./projects";
import { AskForPage } from "./studio/pages";
import { BriefView } from "./studio/research";
import { CampaignsTab } from "./studio-campaigns";

export type ProjectTab =
  | "overview"
  | "campaigns"
  | "research"
  | "pages"
  // Creative's now (BD-66); kept so old links redirect.
  | "renders"
  | "brand"
  | "settings"
  | "concepts";

const TABS: ProjectTab[] = [
  "overview",
  "campaigns",
  "research",
  "pages",
  "renders",
  "brand",
  "settings",
  "concepts",
];

export function projectTabSearch(raw: Record<string, unknown>): { tab?: ProjectTab } {
  const t = raw.tab;
  return typeof t === "string" && (TABS as string[]).includes(t) && t !== "overview"
    ? { tab: t as ProjectTab }
    : {};
}

// A project is one development, such as The Wadi: what Adx may say about it,
// its offers, its campaigns and the research behind them. Its pictures,
// brand and ads are in Creative; Reportage, the developer behind it, sits
// under every project rather than in front of it.
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
  const campaigns = useProjectCampaigns(projectId);
  const pages = usePages();
  const briefs = useBriefs();

  if (projects.isPending) return <LoadingRows rows={8} />;
  if (projects.error) return <ErrorNote error={projects.error} />;
  if (!project) {
    return (
      <Panel>
        <EmptyState title="Project not found" />
      </Panel>
    );
  }
  const loosePages = (pages.data?.pages ?? []).filter(
    (x) => x.projectId === projectId && x.campaignId === null,
  ).length;

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
          brand.data ? (
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
          )
        }
      />
      <div className="mb-8 flex flex-wrap items-center justify-between gap-4">
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
            { value: "campaigns", label: "Campaigns", count: campaigns.data?.campaigns.length },
            {
              value: "research",
              label: "Research",
              count: briefs.data?.briefs.filter((b) => b.projectId === projectId).length,
            },
            ...(loosePages > 0 || tab === "pages"
              ? [{ value: "pages" as const, label: "Unfiled pages", count: loosePages }]
              : []),
          ]}
        />
        {tab === "campaigns" && canEdit && (
          <Link to="/campaigns/new" search={{ project: projectId }} className={buttonVariants()}>
            <Plus /> New campaign
          </Link>
        )}
      </div>
      {tab === "overview" && <Overview projectId={projectId} brandId={brandId} />}
      {tab === "campaigns" && <CampaignsTab projectId={projectId} />}
      {tab === "research" && <ResearchTab projectId={projectId} />}
      {tab === "pages" && <PagesTab projectId={projectId} brandId={brandId} />}
    </section>
  );
}

export function SetUpStudio({ projectId, canEdit }: { projectId: number; canEdit: boolean }) {
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

export function Overview({ projectId, brandId }: { projectId: number; brandId: number | null }) {
  const brand = useBrand(brandId);
  const briefs = useBriefs();
  if (brandId !== null && brand.isPending) return <LoadingRows rows={6} />;
  const d = brand.data;
  const facts = d?.facts.filter((f) => f.projectId === projectId || f.projectId === null) ?? [];
  const offers = d?.offers.filter((o) => o.projectId === projectId || o.projectId === null) ?? [];
  const projectBriefs = (briefs.data?.briefs ?? []).filter((x) => x.projectId === projectId);
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
                    <p className="text-sm text-muted-foreground" title={f.key}>
                      {keyLabel(f.key)}
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
        <CampaignsSide projectId={projectId} />
        <SideList
          title="Research"
          open={{ tab: "research", projectId }}
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
type Open = { tab: ProjectTab; projectId: number };

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
  const action = (
    <Link
      to="/projects/$projectId"
      params={{ projectId: String(open.projectId) }}
      search={{ tab: open.tab }}
      className={cls}
    >
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

export function PagesTab({ projectId, brandId }: { projectId: number; brandId: number | null }) {
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
                      to="/projects/$projectId/pages/$pageId"
                      params={{ projectId: String(projectId), pageId: String(x.id) }}
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

// CampaignsSide is the project's campaigns at a glance: the one to open
// first is the one furthest along.
function CampaignsSide({ projectId }: { projectId: number }) {
  const campaigns = useProjectCampaigns(projectId);
  const list = campaigns.data?.campaigns ?? [];
  return (
    <Panel
      title="Campaigns"
      action={
        <Link
          to="/campaigns/new"
          search={{ project: projectId }}
          className={buttonVariants({ variant: "outline", size: "sm" })}
        >
          <Plus /> New campaign
        </Link>
      }
      bodyClassName="p-0"
    >
      {campaigns.isPending && <LoadingRows rows={2} />}
      {campaigns.data && list.length === 0 && (
        <p className="px-6 py-5 text-sm text-muted-foreground">
          No campaigns yet. Ads and pages are made inside a campaign;{" "}
          <Link
            to="/campaigns/new"
            search={{ project: projectId }}
            className="underline underline-offset-4"
          >
            start one
          </Link>
          .
        </p>
      )}
      {list.length > 0 && (
        <ul className="divide-y">
          {list.map((c) => {
            const st = STATE[c.state] ?? { label: c.state, status: "pending" as Status };
            return (
              <li key={c.id}>
                <Link
                  to="/projects/$projectId/campaigns/$campaignId"
                  params={{ projectId: String(projectId), campaignId: String(c.id) }}
                  className="flex items-center gap-3 px-6 py-3.5 text-sm hover:bg-muted"
                >
                  <StatusMark status={st.status} />
                  <span className="min-w-0 flex-1 truncate font-medium">{c.name}</span>
                  <span className="text-xs text-muted-foreground">
                    {c.startsOn ? `${formatDate(c.startsOn)} · ` : ""}
                    {st.label}
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </Panel>
  );
}

// ResearchTab is what Adx read for this project: its briefs, one at a time.
export function ResearchTab({ projectId }: { projectId: number }) {
  const briefs = useBriefs();
  const session = useSession();
  const canEdit = session.role !== "viewer";
  const [chosen, setChosen] = useState<number | null>(null);
  const [evidence, setEvidence] = useState<number | null>(null);
  const list = (briefs.data?.briefs ?? []).filter((b) => b.projectId === projectId);
  const id = chosen ?? list[0]?.id ?? null;
  if (briefs.isPending) return <LoadingRows rows={6} />;
  if (briefs.error) return <ErrorNote error={briefs.error} />;
  if (list.length === 0) {
    return (
      <div className="space-y-6">
        <CompetitorsPanel />
        <Panel>
          <EmptyState title="No research for this project yet">
            Adx opens a brief when a project starts with its website and competitors. Anything else
            can be set up on the{" "}
            <Link to="/engine/research" className="underline underline-offset-4">
              engine's research screen
            </Link>
            .
          </EmptyState>
        </Panel>
      </div>
    );
  }
  return (
    <div className="space-y-6">
      <CompetitorsPanel />
      {list.length > 1 && (
        <Segmented
          label="Briefs"
          value={String(id)}
          onChange={(v) => setChosen(Number(v))}
          options={list.map((b) => ({ value: String(b.id), label: b.title }))}
        />
      )}
      {id !== null && <BriefView id={id} />}
      {id !== null && (
        <Panel
          title="Keep it current"
          description="A schedule re-reads every source each period and writes a comparative report of what changed."
        >
          <div className="grid gap-6 text-sm md:grid-cols-2">
            <Schedules briefId={id} canEdit={canEdit} />
            <Reports briefId={id} canEdit={canEdit} onOpen={setEvidence} />
          </div>
          {evidence !== null && <EvidencePanel id={evidence} onClose={() => setEvidence(null)} />}
        </Panel>
      )}
    </div>
  );
}
