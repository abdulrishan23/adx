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
import { ConnectionsPage } from "@/routes/connections";
import { PoliciesPage } from "@/routes/policies";
import { PrivacyPage } from "@/routes/privacy";
import { ProjectsPage } from "@/routes/projects";
import { RunsPage } from "@/routes/runs";

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

const policiesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings/policies",
  component: PoliciesPage,
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
  policiesRoute,
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
