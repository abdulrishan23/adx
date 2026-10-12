import {
  createRootRoute,
  createRoute,
  createRouter,
  Link,
  Outlet,
  redirect,
} from "@tanstack/react-router";
import { ProjectsPage } from "@/routes/projects";

// Routes are declared in code instead of generated from files, so the whole
// route tree is readable in one place.

const rootRoute = createRootRoute({ component: Layout });

function Layout() {
  return (
    <div className="min-h-screen">
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
        </nav>
      </header>
      <main className="mx-auto max-w-5xl px-6 py-8">
        <Outlet />
      </main>
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

const routeTree = rootRoute.addChildren([indexRoute, projectsRoute]);

export const router = createRouter({ routeTree });

// Registering the router type makes every Link and navigate call type-checked
// against the routes above.
declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
