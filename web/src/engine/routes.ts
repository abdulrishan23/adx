import { AgentsPage } from "@/engine/routes/agents";
import { AlertsPage } from "@/engine/routes/alerts";
import { AnalyticsPage } from "@/engine/routes/analytics";
import { AskPage } from "@/engine/routes/ask";
import { BrandsPage } from "@/engine/routes/brands";
import { CallsPage } from "@/engine/routes/calls";
import { ConnectionsPage } from "@/engine/routes/connections";
import { CreativesPage } from "@/engine/routes/creatives";
import { EvaluationPage } from "@/engine/routes/evaluation";
import { FunnelPage } from "@/engine/routes/funnel";
import { HomePage } from "@/engine/routes/home";
import { LaunchPage } from "@/engine/routes/launch";
import { LeadsPage } from "@/engine/routes/leads";
import { LearningPage } from "@/engine/routes/learning";
import { OpsPage } from "@/engine/routes/ops";
import { OutreachPage } from "@/engine/routes/outreach";
import { PagesPage } from "@/engine/routes/pages";
import { PeoplePage } from "@/engine/routes/people";
import { PipelinePage } from "@/engine/routes/pipeline";
import { PlansPage } from "@/engine/routes/plans";
import { PoliciesPage } from "@/engine/routes/policies";
import { ProjectsPage } from "@/engine/routes/projects";
import { ProposalsPage } from "@/engine/routes/proposals";
import { ReportsPage } from "@/engine/routes/reports";
import { ResearchPage } from "@/engine/routes/research";
import { RunsPage } from "@/engine/routes/runs";
import { ScoringPage } from "@/engine/routes/scoring";
import { SupervisorPage } from "@/engine/routes/supervisor";
import { WorkflowsPage } from "@/engine/routes/workflows";
import { WorkspacesPage } from "@/engine/routes/workspaces";

// The engine's own screens, mounted under /engine inside our shell. Each one
// talks to the real server. They stay here, reachable from the sidebar's
// Engine group, until the screen in our design is rewired to the same API;
// then its entry here goes.
export const ENGINE_SCREENS: { path: string; label: string; group: string }[] = [
  { path: "/engine/home", label: "Home desk", group: "Work" },
  { path: "/engine/projects", label: "Projects", group: "Work" },
  { path: "/engine/analytics", label: "Analytics", group: "Work" },
  { path: "/engine/ask", label: "Ask", group: "Work" },
  { path: "/engine/brands", label: "Brands", group: "Studio" },
  { path: "/engine/research", label: "Research", group: "Studio" },
  { path: "/engine/plans", label: "Plans", group: "Studio" },
  { path: "/engine/pages", label: "Pages", group: "Studio" },
  { path: "/engine/creatives", label: "Creatives", group: "Studio" },
  { path: "/engine/launch", label: "Launch", group: "Perform" },
  { path: "/engine/proposals", label: "Proposals", group: "Perform" },
  { path: "/engine/reports", label: "Reports", group: "Perform" },
  { path: "/engine/alerts", label: "Alerts", group: "Perform" },
  { path: "/engine/people", label: "People", group: "Convert" },
  { path: "/engine/pipeline", label: "Pipeline", group: "Convert" },
  { path: "/engine/calls", label: "Calls", group: "Convert" },
  { path: "/engine/leads", label: "Leads", group: "Convert" },
  { path: "/engine/supervisor", label: "Supervisor", group: "Agents" },
  { path: "/engine/workflows", label: "Workflows", group: "Agents" },
  { path: "/engine/settings/agents", label: "Agents", group: "Agents" },
  { path: "/engine/learning", label: "Learning", group: "Agents" },
  { path: "/engine/evaluation", label: "Evaluation", group: "Agents" },
  { path: "/engine/connections", label: "Connections", group: "Workspace" },
  { path: "/engine/settings/policies", label: "Policies", group: "Workspace" },
  { path: "/engine/settings/funnel", label: "Funnel", group: "Workspace" },
  { path: "/engine/settings/outreach", label: "Outreach", group: "Workspace" },
  { path: "/engine/settings/scoring", label: "Scoring", group: "Workspace" },
  { path: "/engine/settings/workspaces", label: "Workspaces", group: "Workspace" },
  { path: "/engine/ops", label: "Ops", group: "Workspace" },
  { path: "/engine/runs", label: "Runs", group: "Workspace" },
];

// The page components, keyed by screen, for router.tsx to mount.
export const ENGINE_PAGES = {
  agents: AgentsPage,
  alerts: AlertsPage,
  analytics: AnalyticsPage,
  ask: AskPage,
  brands: BrandsPage,
  calls: CallsPage,
  connections: ConnectionsPage,
  creatives: CreativesPage,
  evaluation: EvaluationPage,
  funnel: FunnelPage,
  home: HomePage,
  launch: LaunchPage,
  leads: LeadsPage,
  learning: LearningPage,
  ops: OpsPage,
  outreach: OutreachPage,
  pages: PagesPage,
  people: PeoplePage,
  pipeline: PipelinePage,
  plans: PlansPage,
  policies: PoliciesPage,
  projects: ProjectsPage,
  proposals: ProposalsPage,
  reports: ReportsPage,
  research: ResearchPage,
  runs: RunsPage,
  scoring: ScoringPage,
  supervisor: SupervisorPage,
  workflows: WorkflowsPage,
  workspaces: WorkspacesPage,
} as const;
