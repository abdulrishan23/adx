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
import { WorkspaceSwitcher } from "@/components/workspace-switcher";
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

// The desk (design language, 09): a 200px sidebar on its own tone with the
// pages grouped by what they are for, and a content column up to 1280px.
const NAV = [
  {
    heading: "Work",
    items: [
      { to: "/projects", label: "Projects" },
      { to: "/analytics", label: "Analytics" },
      { to: "/ask", label: "Ask ADX" },
    ],
  },
  {
    heading: "Create",
    items: [
      { to: "/brands", label: "Brands" },
      { to: "/research", label: "Research" },
      { to: "/plans", label: "Plans" },
      { to: "/pages", label: "Pages" },
      { to: "/creatives", label: "Creatives" },
    ],
  },
  {
    heading: "Perform",
    items: [
      { to: "/launch", label: "Launch" },
      { to: "/proposals", label: "Proposals" },
      { to: "/reports", label: "Reports" },
      { to: "/alerts", label: "Alerts" },
    ],
  },
  {
    heading: "Convert",
    items: [
      { to: "/people", label: "People" },
      { to: "/pipeline", label: "Pipeline" },
      { to: "/calls", label: "Calls" },
      { to: "/leads", label: "Leads" },
    ],
  },
  {
    heading: "Agents",
    items: [
      { to: "/supervisor", label: "Supervisor" },
      { to: "/workflows", label: "Workflows" },
      { to: "/settings/agents", label: "Agents" },
      { to: "/learning", label: "Learning" },
      { to: "/evaluation", label: "Evaluation" },
    ],
  },
  {
    heading: "Workspace",
    items: [
      { to: "/connections", label: "Connections" },
      { to: "/settings/policies", label: "Policies" },
      { to: "/settings/funnel", label: "Funnel" },
      { to: "/settings/outreach", label: "Outreach" },
      { to: "/settings/scoring", label: "Scoring" },
      { to: "/settings/workspaces", label: "Workspaces" },
      { to: "/ops", label: "Ops" },
      { to: "/runs", label: "Runs" },
    ],
  },
] as const;

function Layout() {
  return (
    <div className="flex min-h-screen">
      <HeadContent />
      <aside className="sticky top-0 flex h-screen w-[200px] shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground">
        <div className="px-4 pt-5 pb-4">
          <Link to="/projects" className="font-heading text-xl font-bold tracking-tight">
            adx
          </Link>
        </div>
        <nav className="flex-1 overflow-y-auto px-2 pb-4 [scrollbar-width:none]">
          {NAV.map((group) => (
            <div key={group.heading} className="mb-4">
              <p className="eyebrow px-2 pb-1.5">{group.heading}</p>
              {group.items.map((item) => (
                <Link
                  key={item.to}
                  to={item.to}
                  className="flex h-8 items-center px-2 hover:bg-sidebar-accent"
                  activeProps={{ className: "bg-sidebar-accent font-medium" }}
                >
                  {item.label}
                </Link>
              ))}
            </div>
          ))}
        </nav>
        <div className="space-y-2 border-t px-4 py-3 text-xs">
          <WorkspaceSwitcher />
          <AccountMenu />
          <ServerStatus />
        </div>
      </aside>
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
