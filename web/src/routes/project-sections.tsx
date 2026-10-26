import { Link, type LinkOptions, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, ArrowUpRight, Plus } from "lucide-react";
import { useEffect } from "react";
import { useBrand } from "@/api/brands";
import { useProjectCampaigns, useProjectProgress } from "@/api/campaigns";
import { useCreativeOverview, useCreativeProject } from "@/api/creative";
import { usePlans } from "@/api/plans";
import type { ProgressItem, ProjectNeed } from "@/api/types.gen";
import { PROJECT_SECTIONS, type ProjectSection, projectSectionPath } from "@/app/nav";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { buttonVariants } from "@/components/ui/button";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { useProjects } from "@/engine/api/projects";
import { NewPlan } from "@/engine/routes/plans";
import { MarketEvents } from "@/engine/routes/research";
import { formatRelative } from "@/lib/format";
import { released } from "@/lib/release";
import { ConceptsTab, ReviewLinkButtons } from "./creative-project";
import { Overview, PagesTab, ResearchTab, SetUpStudio } from "./project";
import { PLAN_STATE } from "./studio/plans";
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
        {/* The same five as the sidebar, inside the page at every width, so a
            section is one click away wherever the eye is. It scrolls
            sideways on a phone rather than clipping. */}
        <div className="no-scrollbar max-w-full overflow-x-auto">
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
        {section === "campaigns" && canEdit && (
          <Link to="/campaigns/new" search={{ project: projectId }} className={buttonVariants()}>
            <Plus /> New campaign
          </Link>
        )}
      </div>
      <NeedsStrip projectId={projectId} />
      {!released(current.phase) ? (
        <Panel>
          <EmptyState title="Not started">
            {current.summary} This step arrives in a later phase.
          </EmptyState>
        </Panel>
      ) : (
        <>
          {section === "brand" && <BrandSection projectId={projectId} brandId={brandId} />}
          {section === "media" && <MediaSection projectId={projectId} brandId={brandId} />}
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

// needLink sends a need to the section that clears it. Authorizing a launch
// happens on the engine's launch screen until the designed one lands.
function needLink(need: ProjectNeed, projectId: number): LinkOptions {
  if (need.kind === "launch_authorize") return { to: "/engine/launch" };
  const section = PROJECT_SECTIONS.find((s) => s.segment === need.section);
  return {
    to: projectSectionPath(section?.segment ?? "brand"),
    params: { projectId: String(projectId) },
  };
}

// NeedsStrip sits under the section row on every section: what waits on a
// person right now, the signed-in role's items first. Nothing shows while
// progress loads or when nothing waits.
function NeedsStrip({ projectId }: { projectId: number }) {
  const progress = useProjectProgress(projectId);
  const session = useSession();
  const needs = progress.data?.needs ?? [];
  if (needs.length === 0) return null;
  const mine = needs.filter((n) => n.role === session.role);
  const rest = needs.filter((n) => n.role !== session.role);
  return (
    <Panel title="Needs you" className="mb-6" bodyClassName="p-0">
      <ul className="divide-y">
        {[...mine, ...rest].map((n) => (
          <li key={`${n.kind}-${n.section}-${n.role}`}>
            <Link
              {...needLink(n, projectId)}
              className="flex items-center gap-3 px-6 py-3 text-sm transition-colors hover:bg-accent"
            >
              <StatusMark status={n.kind === "measurement_blocked" ? "error" : "caution"} />
              <span className="min-w-0 flex-1">{n.label}</span>
              <span className="shrink-0 text-xs text-muted-foreground">· {n.role}</span>
            </Link>
          </li>
        ))}
      </ul>
    </Panel>
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
  const session = useSession();
  return (
    <div className="space-y-8">
      <Overview projectId={projectId} brandId={brandId} />
      <div>
        <h3 className="mb-4 font-heading text-lg font-semibold tracking-tight">Research</h3>
        <ResearchTab projectId={projectId} />
      </div>
      {/* Brand-wide, not the project's: a festival or a rate change moves
          every project. Recorded with a source so reports can cite it. */}
      <MarketEvents canEdit={session.role !== "viewer"} />
    </div>
  );
}

// Media: the project's plans, each at its latest version and state. A plan
// opens on the designed plan page (versions, the check, allocations,
// Accept, Draft). Creating a plan's frame by hand stays on the engine.
function MediaSection({ projectId, brandId }: { projectId: number; brandId: number | null }) {
  const navigate = useNavigate();
  const session = useSession();
  const canEdit = session.role !== "viewer";
  const plans = usePlans();
  const campaigns = useProjectCampaigns(projectId);
  const mine = new Set((campaigns.data?.campaigns ?? []).map((c) => c.id));
  const nameOf = new Map((campaigns.data?.campaigns ?? []).map((c) => [c.id, c.name]));
  const list = (plans.data?.plans ?? []).filter(
    (p) => p.projectId === projectId || (p.campaignId !== null && mine.has(p.campaignId)),
  );
  return (
    <div className="space-y-6">
      {plans.isPending && <LoadingRows rows={4} />}
      {plans.error && <ErrorNote error={plans.error} />}
      {plans.data && list.length === 0 && (
        <Panel>
          <EmptyState title="No plan for this project yet">
            A plan is made for one push: start a campaign and Adx drafts its plan from the research.{" "}
            <Link
              to="/campaigns/new"
              search={{ project: projectId }}
              className="underline underline-offset-4"
            >
              Start a campaign
            </Link>
            , or set up a plan's frame on the{" "}
            <Link to="/engine/plans" className="underline underline-offset-4">
              engine's plans screen
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
                <TH>Plan</TH>
                <TH>Campaign</TH>
                <TH>Latest</TH>
                <TH>Standing</TH>
                <TH>Updated</TH>
              </TR>
            </THead>
            <TBody>
              {list.map((p) => {
                const st = PLAN_STATE[p.latestState];
                return (
                  <TR key={p.id}>
                    <TD>
                      <Link
                        to="/projects/$projectId/plans/$planId"
                        params={{ projectId: String(projectId), planId: String(p.id) }}
                        className="font-medium hover:underline"
                      >
                        {p.title}
                      </Link>
                    </TD>
                    <TD className="text-muted-foreground">
                      {p.campaignId !== null ? (nameOf.get(p.campaignId) ?? "") : "Project-wide"}
                    </TD>
                    <TD>v{p.latestVersion}</TD>
                    <TD>
                      <StatusChip status={p.accepted ? "ready" : st.status}>
                        {p.accepted ? "Accepted" : st.label}
                      </StatusChip>
                    </TD>
                    <TD className="text-muted-foreground">{formatRelative(p.updatedAt)}</TD>
                  </TR>
                );
              })}
            </TBody>
          </Table>
        </Panel>
      )}
      {canEdit && brandId !== null && (
        <Panel
          title="New plan"
          description="The frame: title, objective, budget and period. Versions are drafted from research or edited by hand on the plan page."
        >
          <NewPlan
            fixed={{ brandId, projectId }}
            onCreated={(planId) =>
              navigate({
                to: "/projects/$projectId/plans/$planId",
                params: { projectId: String(projectId), planId: String(planId) },
              })
            }
          />
        </Panel>
      )}
      <p className="text-xs text-muted-foreground">
        Accepting a plan is content only; it authorizes no spend. The same plans are on the{" "}
        <Link to="/engine/plans" className="underline underline-offset-4">
          engine's plans screen
        </Link>
        .
      </p>
    </div>
  );
}

// Studio: the project's concepts and ads with their review state, and its
// landing pages with what's live against the latest draft. Both open on
// the designed screens; the Creative door keeps the cross-project view.
function StudioSection({ projectId, brandId }: { projectId: number; brandId: number | null }) {
  const session = useSession();
  const canEdit = session.role !== "viewer";
  const overview = useCreativeOverview();
  const setUp = overview.data?.projects.some((p) => p.projectId === projectId) ?? false;
  const creative = useCreativeProject(setUp ? projectId : 0);
  return (
    <div className="space-y-10">
      <div>
        <div className="mb-4 flex flex-wrap items-baseline justify-between gap-3">
          <h3 className="font-heading text-lg font-semibold tracking-tight">Creatives</h3>
          <span className="flex items-center gap-3 text-xs text-muted-foreground">
            {creative.data && <ReviewLinkButtons settings={creative.data.settings} />}
            <Link
              to="/creative/library"
              search={{ project: projectId }}
              className="hover:text-foreground"
            >
              Pictures and brand kit
            </Link>
          </span>
        </div>
        {overview.isPending && <LoadingRows rows={3} />}
        {overview.error && <ErrorNote error={overview.error} />}
        {overview.data && !setUp && <SetUpStudio projectId={projectId} canEdit={canEdit} />}
        {setUp && creative.isPending && <LoadingRows rows={4} />}
        {setUp && creative.error && <ErrorNote error={creative.error} />}
        {creative.data && <ConceptsTab project={creative.data} canEdit={canEdit} />}
      </div>
      <div>
        <h3 className="mb-4 font-heading text-lg font-semibold tracking-tight">Landing pages</h3>
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

// checkId names one measurement line, so a link can land on its row.
function checkId(item: ProgressItem): string {
  return `${item.planId}-${item.code}-${item.refId}`;
}

// fixFor links a measurement line to the thing that fixes it: the page or
// plan it names, or the connections list for an account (that route takes no
// search param, so the account can't be picked out).
function fixFor(item: ProgressItem, projectId: number): LinkOptions {
  switch (item.refKind) {
    case "account":
      return { to: "/settings/connections" };
    case "page":
      return {
        to: "/projects/$projectId/pages/$pageId",
        params: { projectId: String(projectId), pageId: String(item.refId) },
      };
    case "plan":
      return {
        to: "/projects/$projectId/plans/$planId",
        params: { projectId: String(projectId), planId: String(item.planId) },
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
          <li
            key={checkId(it)}
            id={checkId(it)}
            className="scroll-mt-4 px-6 py-3.5 text-sm target:bg-accent"
          >
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
// blocker: a launch can't run until it's fixed. Each row lands on its line
// in Measurement by hash, since the section routes take no search param.
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
          <li key={checkId(it)}>
            <Link
              to="/projects/$projectId/measurement"
              params={{ projectId: String(projectId) }}
              hash={checkId(it)}
              className="flex items-center gap-3 px-6 py-3 text-sm transition-colors hover:bg-accent"
            >
              <StatusMark status="error" />
              <span className="min-w-0 flex-1">{it.message}</span>
              {it.campaignName && (
                <span className="text-xs text-muted-foreground">{it.campaignName}</span>
              )}
              <ArrowUpRight className="size-4 shrink-0 text-muted-foreground" />
            </Link>
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
