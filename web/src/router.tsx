import {
  createRootRoute,
  createRoute,
  createRouter,
  HeadContent,
  Link,
  Outlet,
  redirect,
} from "@tanstack/react-router";
import { Sidebar } from "@/components/sidebar";
import { AgentsPage } from "@/routes/agents";
import { AlertsPage } from "@/routes/alerts";
import { AnalyticsPage } from "@/routes/analytics";
import { AskPage } from "@/routes/ask";
import { BrandsPage } from "@/routes/brands";
import { CallsPage } from "@/routes/calls";
import { ConnectionsPage } from "@/routes/connections";
import { CreativesPage } from "@/routes/creatives";
import { EvaluationPage } from "@/routes/evaluation";
import { FunnelPage } from "@/routes/funnel";
import { LaunchPage } from "@/routes/launch";
import { LeadsPage } from "@/routes/leads";
import { LearningPage } from "@/routes/learning";
import { OpsPage } from "@/routes/ops";
import { OutreachPage } from "@/routes/outreach";
import { PagesPage } from "@/routes/pages";
import { PeoplePage } from "@/routes/people";
import { PipelinePage } from "@/routes/pipeline";
import { PlansPage } from "@/routes/plans";
import { PoliciesPage } from "@/routes/policies";
import { PrivacyPage } from "@/routes/privacy";
import { ProjectsPage } from "@/routes/projects";
import { ProposalsPage } from "@/routes/proposals";
import { ReportsPage } from "@/routes/reports";
import { ResearchPage } from "@/routes/research";
import { RunsPage } from "@/routes/runs";
import { ScoringPage } from "@/routes/scoring";
import { SupervisorPage } from "@/routes/supervisor";
import { WorkflowsPage } from "@/routes/workflows";
import { WorkspacesPage } from "@/routes/workspaces";

// Routes are declared in code instead of generated from files, so the whole
// route tree is readable in one place.

const rootRoute = createRootRoute({
  component: Layout,
  head: () => ({ meta: [{ title: "ADX" }] }),
});

function Layout() {
  return (
    <div className="flex min-h-screen">
      <HeadContent />
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <main className="mx-auto w-full max-w-[1280px] flex-1 px-8 pt-10 pb-10">
          <Outlet />
        </main>
        <footer className="border-t px-8 py-3 text-xs text-muted-foreground">
          <Link to="/privacy" className="underline underline-offset-4">
            Privacy and data deletion
          </Link>
        </footer>
      </div>
    </div>
  );
}

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ to: "/projects" });
  },
});

const projectsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/projects",
  component: ProjectsPage,
});

const connectionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/connections",
  component: ConnectionsPage,
});

const outreachRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings/outreach",
  component: OutreachPage,
});

const opsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ops",
  component: OpsPage,
});

const workspacesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings/workspaces",
  component: WorkspacesPage,
});

const learningRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/learning",
  component: LearningPage,
});

const supervisorRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/supervisor",
  component: SupervisorPage,
});

const workflowsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workflows",
  component: WorkflowsPage,
});

const agentsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings/agents",
  component: AgentsPage,
});

const evaluationRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/evaluation",
  component: EvaluationPage,
});

const scoringRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings/scoring",
  component: ScoringPage,
});

const policiesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings/policies",
  component: PoliciesPage,
});

const analyticsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/analytics",
  component: AnalyticsPage,
});

const askRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ask",
  component: AskPage,
});

const brandsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/brands",
  component: BrandsPage,
});

const researchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/research",
  component: ResearchPage,
});

const plansRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/plans",
  component: PlansPage,
});

const pagesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/pages",
  component: PagesPage,
});

const launchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/launch",
  component: LaunchPage,
});

const proposalsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/proposals",
  component: ProposalsPage,
});

const creativesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/creatives",
  component: CreativesPage,
});

const reportsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/reports",
  component: ReportsPage,
});

const alertsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/alerts",
  component: AlertsPage,
});

const peopleRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/people",
  component: PeoplePage,
});

const pipelineRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/pipeline",
  component: PipelinePage,
});

const callsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/calls",
  component: CallsPage,
});

const leadsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/leads",
  component: LeadsPage,
});

const funnelRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings/funnel",
  component: FunnelPage,
});

const runsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/runs",
  component: RunsPage,
});

const privacyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/privacy",
  component: PrivacyPage,
  head: () => ({ meta: [{ title: "Privacy policy | ADX" }] }),
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  projectsRoute,
  connectionsRoute,
  outreachRoute,
  learningRoute,
  workspacesRoute,
  opsRoute,
  supervisorRoute,
  workflowsRoute,
  agentsRoute,
  evaluationRoute,
  scoringRoute,
  policiesRoute,
  analyticsRoute,
  askRoute,
  brandsRoute,
  researchRoute,
  plansRoute,
  pagesRoute,
  launchRoute,
  proposalsRoute,
  creativesRoute,
  reportsRoute,
  alertsRoute,
  peopleRoute,
  pipelineRoute,
  callsRoute,
  leadsRoute,
  funnelRoute,
  runsRoute,
  privacyRoute,
]);

export const router = createRouter({ routeTree });

// Registering the router type makes every Link and navigate call type-checked
// against the routes above.
declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
