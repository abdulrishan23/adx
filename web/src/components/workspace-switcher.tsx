import { useSession } from "@/api/session";
import { useSwitchWorkspace, useWorkspaces } from "@/api/workspaces";

// Header switcher: one workspace at a time, and switching reloads the page so
// nothing from the previous client lingers in memory.
export function WorkspaceSwitcher() {
  const session = useSession();
  const list = useWorkspaces();
  const switchTo = useSwitchWorkspace();
  if (!session.data || !list.data || list.data.workspaces.length < 2) return null;
  return (
    <select
      className="rounded border bg-background px-2 py-1 text-sm"
      value={session.data.workspaceId}
      disabled={switchTo.isPending}
      onChange={(e) =>
        switchTo.mutate(Number(e.target.value), {
          onSuccess: () => window.location.assign("/"),
        })
      }
      aria-label="Workspace"
    >
      {list.data.workspaces.map((w) => (
        <option key={w.id} value={w.id}>
          {w.name}
        </option>
      ))}
    </select>
  );
}
