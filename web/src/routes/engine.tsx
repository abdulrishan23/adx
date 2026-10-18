import { Link } from "@tanstack/react-router";
import { ArrowUpRight } from "lucide-react";
import { PageHeader, Panel } from "@/components/page";
import { ENGINE_SCREENS } from "@/engine/routes";

// Engine lists the server's own screens: every capability the build shipped,
// on real data, in its plain form. Each one stays here until the screen in
// our design is wired to the same API, then it leaves this list.
export function EnginePage() {
  const groups = [...new Set(ENGINE_SCREENS.map((s) => s.group))];
  return (
    <>
      <PageHeader
        title="Engine"
        subtitle="The server's own screens, on real data. Plain, but every one of them works. Our screens take these over one at a time."
      />
      <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
        {groups.map((g) => (
          <Panel key={g} title={g} bodyClassName="p-0">
            <ul className="divide-y">
              {ENGINE_SCREENS.filter((s) => s.group === g).map((s) => (
                <li key={s.path}>
                  <Link
                    to={s.path}
                    className="group flex items-center justify-between gap-3 px-5 py-2.5 text-sm transition-colors hover:bg-muted"
                  >
                    {s.label}
                    <ArrowUpRight className="size-3.5 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
                  </Link>
                </li>
              ))}
            </ul>
          </Panel>
        ))}
      </div>
    </>
  );
}
