import {
  createRootRoute,
  createRoute,
  createRouter,
  HeadContent,
  Link,
  Outlet,
  redirect,
} from "@tanstack/react-router";
import { AccountMenu } from "@/components/account-menu";
import { ServerStatus } from "@/components/server-status";
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

// Routes are declared in code instead of generated from files, so the whole
// route tree is readable in one place.

const rootRoute = createRootRoute({
  component: Layout,
  head: () => ({ meta: [{ title: "ADX" }] }),
});

function Layout() {
  return (
    <div className="min-h-screen">
      <HeadContent />
      <header className="border-b">
        <nav className="mx-auto flex max-w-5xl items-center gap-6 px-6 py-4">
          <span className="font-semibold">ADX</span>
          <Link
            to="/projects"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Projects
          </Link>
          <Link
            to="/analytics"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Analytics
          </Link>
          <Link
            to="/ask"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Ask ADX
          </Link>
          <Link
            to="/brands"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Brands
          </Link>
          <Link
            to="/research"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Research
          </Link>
          <Link
            to="/plans"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Plans
          </Link>
          <Link
            to="/pages"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Pages
          </Link>
          <Link
            to="/launch"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Launch
          </Link>
          <Link
            to="/proposals"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Proposals
          </Link>
          <Link
            to="/creatives"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Creatives
          </Link>
          <Link
            to="/reports"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Reports
          </Link>
          <Link
            to="/alerts"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Alerts
          </Link>
          <Link
            to="/people"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            People
          </Link>
          <Link
            to="/pipeline"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Pipeline
          </Link>
          <Link
            to="/calls"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Calls
          </Link>
          <Link
            to="/supervisor"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Supervisor
          </Link>
          <Link
            to="/workflows"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Workflows
          </Link>
          <Link
            to="/settings/agents"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Agents
          </Link>
          <Link
            to="/evaluation"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Evaluation
          </Link>
          <Link
            to="/settings/outreach"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Outreach
          </Link>
          <Link
            to="/settings/scoring"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Scoring
          </Link>
          <Link
            to="/leads"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Leads
          </Link>
          <Link
            to="/connections"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Connections
          </Link>
          <Link
            to="/settings/policies"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Policies
          </Link>
          <Link
            to="/settings/funnel"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Funnel
          </Link>
          <Link
            to="/runs"
            className="text-muted-foreground"
            activeProps={{ className: "text-foreground" }}
          >
            Runs
          </Link>
          <div className="ml-auto">
            <AccountMenu />
          </div>
        </nav>
      </header>
      <main className="mx-auto max-w-5xl px-6 py-8">
        <Outlet />
      </main>
      <footer className="border-t">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-4 text-sm text-muted-foreground">
          <Link to="/privacy" className="underline underline-offset-4">
            Privacy and data deletion
          </Link>
          <ServerStatus />
        </div>
      </footer>
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
