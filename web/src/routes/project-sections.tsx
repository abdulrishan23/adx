import { Link, type LinkOptions, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, ArrowUpRight, Plus } from "lucide-react";
import { useEffect } from "react";
import { useBrand } from "@/api/brands";
import { useProjectCampaigns, useProjectProgress } from "@/api/campaigns";
import { usePages } from "@/api/pages";
import type { ProgressItem } from "@/api/types.gen";
import { PROJECT_SECTIONS, type ProjectSection, projectSectionPath } from "@/app/nav";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { buttonVariants } from "@/components/ui/button";
import { type Status, StatusMark } from "@/components/ui/status-chip";
import { useProjects } from "@/engine/api/projects";
import { released } from "@/lib/release";
import { STATE } from "./campaign";
import { Overview, PagesTab, ResearchTab } from "./project";
import { CampaignsTab } from "./studio-campaigns";

const LAST_PROJECT_KEY = "adx-last-project";

// rememberProject keeps the last project opened, so the sidebar can offer it
// again from the Projects list.
export function rememberProject(projectId: number) {
  try {
    localStorage.setItem(LAST_PROJECT_KEY, String(projectId));
  } catch {
    // Storage can be blocked; nothing depends on remembering.
  }
}

export function lastProject(): number | null {
  try {
    const raw = localStorage.getItem(LAST_PROJECT_KEY);
    return raw ? Number(raw) : null;
  } catch {
    return null;
  }
}

// A project section is one of the five screens every project has, in the
// order the work happens: brand, media, studio, measurement, campaigns. Each
// is a shell for now: it shows what the project already has for that step
// and links to the screen that makes more. The sidebar carries the same five.
export function ProjectSectionPage({
  projectId,
  section,
}: {
  projectId: number;
  section: ProjectSection["segment"];
}) {
  const projects = useProjects();
  const navigate = useNavigate();
  const session = useSession();
  const canEdit = session.role !== "viewer";
  const project = projects.data?.projects.find((p) => p.id === projectId);
  const brandId = project?.brandId ?? null;
  const brand = useBrand(brandId);
  const campaigns = useProjectCampaigns(projectId);
  const current = PROJECT_SECTIONS.find((s) => s.segment === section);

  useEffect(() => {
    if (project) rememberProject(project.id);
  }, [project]);

  if (projects.isPending) return <LoadingRows rows={8} />;
  if (projects.error) return <ErrorNote error={projects.error} />;
  if (!project || !current) {
    return (
      <Panel>
        <EmptyState title="Project not found" />
      </Panel>
    );
  }

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
        {/* The sidebar carries the same five; this row is for the phone, where
            the sidebar is a drawer. It scrolls sideways rather than clipping. */}
        <div className="no-scrollbar max-w-full overflow-x-auto md:hidden">
          <Segmented
            label="Project sections"
            value={section}
            onChange={(s) =>
              navigate({ to: projectSectionPath(s), params: { projectId: String(projectId) } })
            }
            options={PROJECT_SECTIONS.map((s) => ({
              value: s.segment,
              label: s.label,
              ...(s.segment === "campaigns" ? { count: campaigns.data?.campaigns.length } : {}),
            }))}
          />
        </div>
        <h2 className="hidden font-heading text-xl font-semibold tracking-tight md:block">
          {current.label}
        </h2>
        {section === "campaigns" && canEdit && (
          <Link to="/campaigns/new" search={{ project: projectId }} className={buttonVariants()}>
            <Plus /> New campaign
          </Link>
        )}
      </div>
      {!released(current.phase) ? (
        <Panel>
          <EmptyState title="Not started">
            {current.summary} This step arrives in a later phase.
          </EmptyState>
        </Panel>
      ) : (
        <>
          {section === "brand" && <BrandSection projectId={projectId} brandId={brandId} />}
          {section === "media" && <MediaSection projectId={projectId} />}
          {section === "studio" && <StudioSection projectId={projectId} brandId={brandId} />}
          {section === "measurement" && <MeasurementSection projectId={projectId} />}
          {section === "campaigns" && (
            <>
              <MeasurementGate projectId={projectId} />
              <CampaignsTab projectId={projectId} />
            </>
          )}
        </>
      )}
    </section>
  );
}

// Where links a subsection to the screen that holds the full thing.
function Where({ rows }: { rows: { label: string; note: string; link: LinkOptions }[] }) {
  return (
    <ul className="divide-y">
      {rows.map((r) => (
        <li key={r.label}>
          <Link
            {...r.link}
            className="flex items-center gap-3 px-6 py-3.5 text-sm transition-colors hover:bg-accent"
          >
            <span className="min-w-0 flex-1">
              <span className="font-medium">{r.label}</span>
              <span className="block text-xs text-muted-foreground">{r.note}</span>
            </span>
            <ArrowUpRight className="size-4 text-muted-foreground" />
          </Link>
        </li>
      ))}
    </ul>
  );
}

// Brand, facts and research: the facts and offers that apply to this project
// (Overview), then the briefs. Competitors and brand-wide facts stay on the
// brand page, one link away.
function BrandSection({ projectId, brandId }: { projectId: number; brandId: number | null }) {
  return (
    <div className="space-y-8">
      <Overview projectId={projectId} brandId={brandId} />
      <div>
        <h3 className="mb-4 font-heading text-lg font-semibold tracking-tight">Research</h3>
        <ResearchTab projectId={projectId} />
      </div>
    </div>
  );
}

// Media: each campaign's plan stage, and the engine's plans screen for a
// plan's frame and versions.
function MediaSection({ projectId }: { projectId: number }) {
  const campaigns = useProjectCampaigns(projectId);
  const list = campaigns.data?.campaigns ?? [];
  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_380px]">
      <Panel
        title="Plans by campaign"
        description="A plan is made for one push. Open a campaign at its plan stage to draft, check and accept it."
        bodyClassName="p-0"
      >
        {campaigns.isPending && <LoadingRows rows={3} />}
        {campaigns.data && list.length === 0 && (
          <EmptyState title="No campaigns yet">
            A plan needs a push to belong to.{" "}
            <Link
              to="/campaigns/new"
              search={{ project: projectId }}
              className="underline underline-offset-4"
            >
              Start a campaign
            </Link>{" "}
            and Adx drafts its plan from the research.
          </EmptyState>
        )}
        {list.length > 0 && (
          <Where
            rows={list.map((c) => ({
              label: c.name,
              note: `${STATE[c.state]?.label ?? c.state} · plan stage`,
              link: {
                link: { to: "/projects/$projectId/campaigns/$campaignId" },
                params: { projectId: String(projectId), campaignId: String(c.id) },
                search: { stage: "plan" },
              },
            }))}
          />
        )}
      </Panel>
      <Panel title="Elsewhere" bodyClassName="p-0">
        <Where
          rows={[
            {
              label: "Plans on the engine",
              note: "A plan's frame (brand, budget, dates), its versions and the check.",
              link: { to: "/engine/plans" },
            },
          ]}
        />
      </Panel>
    </div>
  );
}

// Studio: the project's creatives live in Creative, filtered to it; its
// pages are the campaign pages plus any unfiled ones.
function StudioSection({ projectId, brandId }: { projectId: number; brandId: number | null }) {
  const pages = usePages();
  const pageCount = (pages.data?.pages ?? []).filter((p) => p.projectId === projectId).length;
  return (
    <div className="space-y-6">
      <div className="grid gap-6 md:grid-cols-2">
        <Panel
          title="Creatives"
          description="Concepts, pictures and ads for this project."
          bodyClassName="p-0"
        >
          <Where
            rows={[
              {
                label: "Concepts",
                note: "Every idea for this project and the ads built from it.",
                link: { to: "/creative/concepts" },
              },
              {
                label: "Library",
                note: "Pictures, logos, fonts and colours.",
                link: { to: "/creative/library", search: { project: projectId } },
              },
              {
                label: "Producer",
                note: "What needs changes, what's awaiting review, what's approved.",
                link: { to: "/creative/producer" },
              },
            ]}
          />
        </Panel>
        <Panel
          title="Landing pages"
          description={`${pageCount} page${pageCount === 1 ? "" : "s"} for this project.`}
          bodyClassName="p-0"
        >
          <Where
            rows={[
              {
                label: "Pages on the engine",
                note: "Each page's live version, drafts, readiness and submissions.",
                link: { to: "/engine/pages" },
              },
            ]}
          />
        </Panel>
      </div>
      <div>
        <h3 className="mb-4 font-heading text-lg font-semibold tracking-tight">Unfiled pages</h3>
        <PagesTab projectId={projectId} brandId={brandId} />
      </div>
    </div>
  );
}

const ITEM_STATUS: Record<string, { label: string; status: Status }> = {
  ok: { label: "Passed", status: "ready" },
  warning: { label: "Warning", status: "caution" },
  blocked: { label: "Blocked", status: "error" },
  unsupported: { label: "Not available", status: "pending" },
};

// fixFor links a measurement line to the screen that fixes it, by what the
// line points at.
function fixFor(item: ProgressItem, projectId: number): LinkOptions {
  switch (item.refKind) {
    case "account":
      return { to: "/settings/connections" };
    case "page":
      return { to: "/engine/pages" };
    case "plan":
      return {
        to: "/projects/$projectId/media",
        params: { projectId: String(projectId) },
      };
    default:
      return { to: "/engine/launch" };
  }
}

// MeasurementChecks lists the launch-readiness lines about measurement for
// each of the project's plans: passed, warning, blocked or not available,
// each with the screen that fixes it. No plan means nothing to check yet.
function MeasurementChecks({ projectId }: { projectId: number }) {
  const progress = useProjectProgress(projectId);
  if (progress.isPending) return <LoadingRows rows={3} />;
  if (progress.error) return <ErrorNote error={progress.error} />;
  const items = progress.data?.measurement ?? [];
  const state = progress.data?.sections.find((s) => s.id === "measurement");
  if (items.length === 0) {
    return (
      <EmptyState title={state?.note ?? "Nothing to check yet"}>
        The checks run against a campaign's plan: that each ad account has a verified conversion
        action, each page has its pixel and a passing test submission, and outcomes sync.{" "}
        <Link
          to="/projects/$projectId/campaigns"
          params={{ projectId: String(projectId) }}
          className="underline underline-offset-4"
        >
          Start a campaign
        </Link>{" "}
        and the checks appear here.
      </EmptyState>
    );
  }
  return (
    <ul className="divide-y">
      {items.map((it) => {
        const s = ITEM_STATUS[it.state] ?? { label: it.state, status: "pending" as const };
        return (
          <li key={`${it.planId}-${it.code}-${it.refId}`} className="px-6 py-3.5 text-sm">
            <div className="flex items-start gap-3">
              <StatusMark status={s.status} className="mt-1.5" />
              <span className="min-w-0 flex-1">
                <span className="font-medium">{it.message}</span>
                <span className="block text-xs text-muted-foreground">
                  {s.label}
                  {it.campaignName ? ` · ${it.campaignName}` : ""} · {it.area}
                </span>
              </span>
              {it.state !== "ok" && (
                <Link
                  {...fixFor(it, projectId)}
                  className="shrink-0 text-xs text-muted-foreground hover:text-foreground"
                >
                  Fix
                </Link>
              )}
            </div>
          </li>
        );
      })}
    </ul>
  );
}

// MeasurementGate sits at the top of Campaigns while measurement has a
// blocker: a launch can't run until it's fixed.
function MeasurementGate({ projectId }: { projectId: number }) {
  const progress = useProjectProgress(projectId);
  if (!progress.data?.blocked) return null;
  const blockers = progress.data.measurement.filter((it) => it.state === "blocked");
  return (
    <Panel
      className="mb-6 border-status-error"
      title="Blocked by measurement"
      description={`${blockers.length} ${blockers.length === 1 ? "check" : "checks"} must pass before a launch can run.`}
      action={
        <Link
          to="/projects/$projectId/measurement"
          params={{ projectId: String(projectId) }}
          className="text-xs whitespace-nowrap text-muted-foreground hover:text-foreground"
        >
          Open Measurement
        </Link>
      }
      bodyClassName="p-0"
    >
      <ul className="divide-y">
        {blockers.map((it) => (
          <li
            key={`${it.planId}-${it.code}-${it.refId}`}
            className="flex items-center gap-3 px-6 py-3 text-sm"
          >
            <StatusMark status="error" />
            <span className="min-w-0 flex-1">{it.message}</span>
            {it.campaignName && (
              <span className="text-xs text-muted-foreground">{it.campaignName}</span>
            )}
          </li>
        ))}
      </ul>
    </Panel>
  );
}

// Measurement is setup only: what's measured and whether it works. Figures
// stay under Perform. It sits before Campaigns because a launch is blocked
// while measurement has a blocker.
function MeasurementSection({ projectId }: { projectId: number }) {
  return (
    <div className="space-y-6">
      <Panel
        title="Checks for this project"
        description="What each campaign's launch still needs from measurement. Blockers gate Campaigns."
        bodyClassName="p-0"
      >
        <MeasurementChecks projectId={projectId} />
      </Panel>
      <div className="grid gap-6 md:grid-cols-2">
        <Panel
          title="What's measured"
          description="Conversion actions, pixels and the outcome events sent back to the platforms."
          bodyClassName="p-0"
        >
          <Where
            rows={[
              {
                label: "Connections",
                note: "Google Ads, Meta and GoHighLevel, each with its health and last sync.",
                link: { to: "/settings/connections" },
              },
              {
                label: "Outcome sync",
                note: "Qualified leads and Done-Deals sent back to Google and Meta.",
                link: { to: "/outcomes" },
              },
            ]}
          />
        </Panel>
        <Panel
          title="Whether it works"
          description="The checks a launch needs to pass, and the gaps in the data behind the figures."
          bodyClassName="p-0"
        >
          <Where
            rows={[
              {
                label: "Launch readiness",
                note: "Measurement is one of the areas a launch checks before it runs.",
                link: { to: "/engine/launch" },
              },
              {
                label: "Data quality",
                note: "Platform leads against GoHighLevel leads, status and attribution coverage.",
                link: { to: "/data-quality" },
              },
            ]}
          />
        </Panel>
      </div>
    </div>
  );
}
