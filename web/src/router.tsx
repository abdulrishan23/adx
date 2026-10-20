import {
  createRootRoute,
  createRoute,
  createRouter,
  HeadContent,
  Link,
  Outlet,
  redirect,
} from "@tanstack/react-router";
import { AppShell } from "@/app/app-shell";
import { findNavItem } from "@/app/nav";
import { AdxWordmark } from "@/components/brand";
import { ENGINE_PAGES as E } from "@/engine/routes";
import { type AnalyticsSearch, analyticsSearch } from "@/lib/analytics";
import { released } from "@/lib/release";
import { AnalyticsPage } from "@/routes/analytics";
import { ApprovePage } from "@/routes/approve";
import { AskPage } from "@/routes/ask";
import { AudiencesPage } from "@/routes/audiences";
import { BrandPage } from "@/routes/brand";
import { CampaignPage, type CampaignStageId, STAGE_IDS } from "@/routes/campaign";
import { CampaignsPage } from "@/routes/campaigns";
import { ComingSoonPage } from "@/routes/coming-soon";
import { ConnectGhlPage } from "@/routes/connect-ghl";
import { ConnectionsPage } from "@/routes/connections";
import { CreativeConceptPage } from "@/routes/creative-concept";
import { CreativeEditorPage } from "@/routes/creative-editor";
import { CreativeUploadPage } from "@/routes/creative-upload";
import { DataQualityPage } from "@/routes/data-quality";
import { EnginePage } from "@/routes/engine";
import { EnrichmentPage } from "@/routes/enrichment";
import { GuardrailsPage } from "@/routes/guardrails";
import { HomePage } from "@/routes/home";
import { InboxPage } from "@/routes/inbox";
import { LeadDetailPage } from "@/routes/lead-detail";
import { LeadsPage } from "@/routes/leads";
import { ListsPage } from "@/routes/lists";
import { MarketersPage } from "@/routes/marketers";
import { OrganicPage } from "@/routes/organic";
import { OutcomesPage } from "@/routes/outcomes";
import { PeoplePage } from "@/routes/people";
import { PortalsPage } from "@/routes/portals";
import { PresentPage } from "@/routes/present";
import { PrivacyPage } from "@/routes/privacy";
import { ProjectPage, projectTabSearch } from "@/routes/project";
import { ProjectsPage } from "@/routes/projects";
import { ProjectsOverviewPage } from "@/routes/projects-overview";
import { ProposalsPage } from "@/routes/proposals";
import { ReportsPage } from "@/routes/reports";
import { RevenuePage } from "@/routes/revenue";
import { ReviewPage } from "@/routes/review";
import { ScoringPage } from "@/routes/scoring";
import { SignInPage } from "@/routes/sign-in";
import { SourcePage } from "@/routes/source";
import { LaunchPage } from "@/routes/studio/launch";
import { PagePage } from "@/routes/studio/pages";
import { PlanPage } from "@/routes/studio/plans";
import { StartBrandPage } from "@/routes/studio/start";
import { NewCampaignPage } from "@/routes/studio-campaigns";
import { WebsitePage } from "@/routes/website";

// Routes are declared in code instead of generated from files, so the whole
// route tree is readable in one place. The sidebar's entries live in
// app/nav.ts; their `to` paths are type-checked against the routes below.

const rootRoute = createRootRoute({
  component: Root,
  head: () => ({ meta: [{ title: "Adx" }] }),
});

function Root() {
  return (
    <>
      <HeadContent />
      <Outlet />
    </>
  );
}

// Public pages sit outside the app shell: no sidebar, no sign-in.
function PublicLayout() {
  return (
    <div className="min-h-screen">
      <header className="border-b">
        <nav className="mx-auto flex max-w-5xl items-center px-6 py-4" aria-label="Main navigation">
          <Link to="/" aria-label="Adx home">
            <AdxWordmark />
          </Link>
        </nav>
      </header>
      <main className="mx-auto max-w-5xl px-6 py-8">
        <Outlet />
      </main>
    </div>
  );
}

const publicRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "public",
  component: PublicLayout,
});

const privacyRoute = createRoute({
  getParentRoute: () => publicRoute,
  path: "/privacy",
  component: PrivacyPage,
  head: () => ({ meta: [{ title: "Privacy policy | ADX" }] }),
});

// Signed-in pages share the app shell. The layout route has an id instead of
// a path, so it adds nothing to the URL.
const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "app",
  component: AppShell,
  // A screen from a phase this release doesn't ship is unreachable, not just
  // missing from the sidebar.
  beforeLoad: ({ location }) => {
    const item = findNavItem(location.pathname) ?? findNavItem(parentPath(location.pathname));
    if (item && !released(item.phase)) {
      throw redirect({ to: "/" });
    }
  },
});

const signInRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sign-in",
  component: SignInPage,
});

const indexRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/",
  component: HomePage,
  head: () => ({ meta: [{ title: "Adx" }] }),
});

// Present stands outside the shell, edge to edge, for a wall screen.
const presentRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/present",
  component: PresentPage,
  head: () => ({ meta: [{ title: "Present | Adx" }] }),
});

// Focus became Present.
const oldFocusRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/focus",
  beforeLoad: () => {
    throw redirect({ to: "/present" });
  },
});

// The Funnel page became Revenue; its daily funnel is on the Brief.
const oldFunnelRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/funnel",
  beforeLoad: () => {
    throw redirect({ to: "/revenue" });
  },
});

// Projects lived under Settings once; they lead Studio now.
const oldProjectsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings/projects",
  beforeLoad: () => {
    throw redirect({ to: "/projects" });
  },
});
// Alerts are cards in the Inbox now.
const oldAlertsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/alerts",
  beforeLoad: () => {
    throw redirect({ to: "/inbox" });
  },
});
// Brand knowledge is the brand page now.
const oldBrandKnowledgeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/create/brand",
  beforeLoad: () => {
    throw redirect({ to: "/projects" });
  },
});
// Create became Studio. Old links land on the same screen.
const oldCreateRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/create/$",
  beforeLoad: ({ params }) => {
    throw redirect({ href: `/studio/${params._splat ?? ""}` });
  },
});
// Our side has one developer, so Studio opens on its projects; Brands and
// the Creatives and Landing pages lists became tabs of a project (BD-63).
const oldBrandsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/brands",
  beforeLoad: () => {
    throw redirect({ to: "/projects" });
  },
});
const oldStartRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/brands/new",
  beforeLoad: () => {
    throw redirect({ to: "/projects/new" });
  },
});
const oldBrandProjectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/brands/$brandId/projects/$projectId",
  beforeLoad: ({ params }) => {
    throw redirect({ to: "/projects/$projectId", params: { projectId: params.projectId } });
  },
});
const oldCreativeListRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/creative",
  beforeLoad: () => {
    throw redirect({ to: "/projects" });
  },
});
const oldCreativeProjectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/creative/$projectId",
  validateSearch: projectTabSearch,
  beforeLoad: ({ params, search }) => {
    throw redirect({
      to: "/projects/$projectId",
      params: { projectId: params.projectId },
      search: { tab: search.tab ?? "concepts" },
    });
  },
});
const oldPagesListRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/studio/pages",
  beforeLoad: () => {
    throw redirect({ to: "/projects" });
  },
});

// Research moved into the project (its Research tab); plans and launch are
// stages of a campaign. The old lists redirect to where the work now starts.
// Competitors live in a project's Research tab; campaigns live in their
// project.
const oldCompetitorsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/competitors",
  beforeLoad: () => {
    throw redirect({ to: "/projects" });
  },
});

const oldCampaignsListRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/studio/campaigns",
  beforeLoad: () => {
    throw redirect({ to: "/projects" });
  },
});

const oldPlansRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/studio/plans",
  beforeLoad: () => {
    throw redirect({ to: "/projects" });
  },
});

const oldResearchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/studio/research",
  beforeLoad: () => {
    throw redirect({ to: "/projects" });
  },
});

const projectRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/projects/$projectId",
  validateSearch: projectTabSearch,
  component: function Project() {
    const { projectId } = projectRoute.useParams();
    const { tab } = projectRoute.useSearch();
    return <ProjectPage projectId={Number(projectId)} tab={tab} />;
  },
});

const campaignRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/projects/$projectId/campaigns/$campaignId",
  // ?stage= picks the stage that fills the view; the brief is the default.
  validateSearch: (search: Record<string, unknown>): { stage?: CampaignStageId } =>
    typeof search.stage === "string" &&
    (STAGE_IDS as string[]).includes(search.stage) &&
    search.stage !== "brief"
      ? { stage: search.stage as CampaignStageId }
      : {},
  component: function Campaign() {
    const { projectId, campaignId } = campaignRoute.useParams();
    const { stage } = campaignRoute.useSearch();
    return (
      <CampaignPage projectId={Number(projectId)} campaignId={Number(campaignId)} stage={stage} />
    );
  },
});

// parentPath drops the last segment, so a detail page is gated by its list.
const parentPath = (p: string) => p.replace(/\/[^/]+$/, "");

function page<P extends string>(path: P, component: () => React.JSX.Element) {
  return createRoute({ getParentRoute: () => appRoute, path, component });
}

const proposalsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/proposals",
  // ?open=<id> opens a proposal's sheet, so alerts and the brief can link
  // straight to one.
  validateSearch: (search: Record<string, unknown>): { open?: string } =>
    typeof search.open === "string" ? { open: search.open } : {},
  component: ProposalsPage,
});

const newCampaignRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/campaigns/new",
  // ?project= fixes the project when the link came from one.
  validateSearch: (search: Record<string, unknown>): { project?: number } =>
    typeof search.project === "number" && search.project > 0 ? { project: search.project } : {},
  component: function NewCampaign() {
    const { project } = newCampaignRoute.useSearch();
    return <NewCampaignPage projectId={project} />;
  },
});

const campaignsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/campaigns",
  validateSearch: (search: Record<string, unknown>): { open?: string } =>
    typeof search.open === "string" ? { open: search.open } : {},
  component: CampaignsPage,
});

const askRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/ask",
  // ?c opens a past conversation; ?q asks a question straight away, which is
  // how the command palette hands a question over.
  validateSearch: (search: Record<string, unknown>): { c?: string; q?: string } => ({
    ...(typeof search.c === "string" ? { c: search.c } : {}),
    ...(typeof search.q === "string" ? { q: search.q } : {}),
  }),
  component: AskPage,
});

// Where approval emails link. It sits outside the shell so it reads well on a
// phone; sign-in for it comes with auth.
const approveRoute = createRoute({
  getParentRoute: () => publicRoute,
  path: "/approve/$proposalId",
  beforeLoad: () => {
    if (!released(3)) {
      throw redirect({ to: "/" });
    }
  },
  component: ApprovePage,
  head: () => ({ meta: [{ title: "Approve a change | Adx" }] }),
});

const projectsOverviewRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/perform/projects",
  validateSearch: (
    raw: Record<string, unknown>,
  ): Pick<AnalyticsSearch, "from" | "to" | "compare" | "comparisonFrom" | "comparisonTo"> => {
    const { from, to, compare, comparisonFrom, comparisonTo } = analyticsSearch(raw);
    return { from, to, compare, comparisonFrom, comparisonTo };
  },
  component: ProjectsOverviewPage,
});

const analyticsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/analytics",
  validateSearch: analyticsSearch,
  component: AnalyticsPage,
  head: () => ({ meta: [{ title: "Analytics | Adx" }] }),
});

// The public review board stands alone: no app shell and no sign-in.
const reviewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/review/$key",
  component: function Review() {
    const { key } = reviewRoute.useParams();
    return <ReviewPage reviewKey={key} />;
  },
  head: () => ({
    meta: [{ title: "Creative review | Adx" }, { name: "robots", content: "noindex" }],
  }),
});

const creativeUploadRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/creative/$projectId/upload",
  component: function CreativeUpload() {
    const { projectId } = creativeUploadRoute.useParams();
    return <CreativeUploadPage projectId={Number(projectId)} />;
  },
});

const creativeConceptRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/creative/$projectId/concepts/$conceptId",
  component: function CreativeConcept() {
    const { projectId, conceptId } = creativeConceptRoute.useParams();
    return <CreativeConceptPage projectId={Number(projectId)} conceptId={Number(conceptId)} />;
  },
});

const creativeEditorRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/creative/$projectId/ads/$adId/edit",
  component: function CreativeEditor() {
    const { adId } = creativeEditorRoute.useParams();
    return <CreativeEditorPage adId={Number(adId)} />;
  },
});

const briefRedirect = createRoute({
  getParentRoute: () => rootRoute,
  path: "/brief",
  beforeLoad: () => {
    throw redirect({ to: "/analytics", search: analyticsSearch({}) });
  },
});

// The engine's Ask keeps its ?q= search param.
const engineAskRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/engine/ask",
  component: E.ask,
  validateSearch: (search: Record<string, unknown>): { q?: string } =>
    typeof search.q === "string" && search.q !== "" ? { q: search.q } : {},
});

const routeTree = rootRoute.addChildren([
  signInRoute,
  oldFunnelRoute,
  oldProjectsRoute,
  oldBrandsRoute,
  oldStartRoute,
  oldBrandProjectRoute,
  oldCreativeListRoute,
  oldCreativeProjectRoute,
  oldPagesListRoute,
  oldBrandKnowledgeRoute,
  oldCreateRoute,
  oldAlertsRoute,
  presentRoute,
  oldFocusRoute,
  briefRedirect,
  reviewRoute,
  publicRoute.addChildren([privacyRoute, approveRoute]),
  appRoute.addChildren([
    indexRoute,
    oldCampaignsListRoute,
    oldResearchRoute,
    oldPlansRoute,
    page("/studio/plans/$planId", PlanPage),
    page("/studio/pages/$pageId", PagePage),
    page("/studio/launch", LaunchPage),
    page("/convert/calls", ComingSoonPage),
    page("/convert/inbox", ComingSoonPage),
    page("/convert/crm", ComingSoonPage),
    page("/workflows", ComingSoonPage),
    analyticsRoute,
    askRoute,
    proposalsRoute,
    newCampaignRoute,
    campaignsRoute,
    page("/revenue", RevenuePage),
    projectsOverviewRoute,
    page("/leads", LeadsPage),
    page("/leads/$leadId", LeadDetailPage),
    creativeUploadRoute,
    creativeConceptRoute,
    creativeEditorRoute,
    page("/enrichment", EnrichmentPage),
    page("/marketers", MarketersPage),
    page("/portals", PortalsPage),
    page("/data-quality", DataQualityPage),
    page("/reports", ReportsPage),
    page("/audiences", AudiencesPage),
    page("/organic", OrganicPage),
    page("/website", WebsitePage),
    page("/outcomes", OutcomesPage),
    page("/settings/connections", ConnectionsPage),
    page("/settings/connections/gohighlevel", ConnectGhlPage),
    page("/settings/connections/$key", SourcePage),
    page("/settings/people", PeoplePage),
    page("/settings/lists", ListsPage),
    page("/settings/guardrails", GuardrailsPage),
    page("/settings/scoring", ScoringPage),
    page("/inbox", InboxPage),
    page("/projects", ProjectsPage),
    page("/projects/new", StartBrandPage),
    projectRoute,
    campaignRoute,
    oldCompetitorsRoute,
    page("/brands/$brandId", BrandPage),
    page("/engine", EnginePage),
    page("/engine/home", E.home),
    page("/engine/projects", E.projects),
    page("/engine/analytics", E.analytics),
    engineAskRoute,
    page("/engine/brands", E.brands),
    page("/engine/research", E.research),
    page("/engine/plans", E.plans),
    page("/engine/pages", E.pages),
    page("/engine/creatives", E.creatives),
    page("/engine/launch", E.launch),
    page("/engine/proposals", E.proposals),
    page("/engine/reports", E.reports),
    page("/engine/alerts", E.alerts),
    page("/engine/people", E.people),
    page("/engine/pipeline", E.pipeline),
    page("/engine/calls", E.calls),
    page("/engine/leads", E.leads),
    page("/engine/supervisor", E.supervisor),
    page("/engine/workflows", E.workflows),
    page("/engine/settings/agents", E.agents),
    page("/engine/learning", E.learning),
    page("/engine/evaluation", E.evaluation),
    page("/engine/connections", E.connections),
    page("/engine/settings/policies", E.policies),
    page("/engine/settings/funnel", E.funnel),
    page("/engine/settings/outreach", E.outreach),
    page("/engine/settings/scoring", E.scoring),
    page("/engine/settings/workspaces", E.workspaces),
    page("/engine/ops", E.ops),
    page("/engine/runs", E.runs),
  ]),
]);

// basepath follows Vite's base, so route paths below stay rooted whether the
// app is served from / in dev or from /app in the deployment.
export const router = createRouter({ routeTree, basepath: import.meta.env.BASE_URL });

// Registering the router type makes every Link and navigate call type-checked
// against the routes above.
declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
