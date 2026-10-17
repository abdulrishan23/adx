import { useKillSwitch, usePolicies, usePutPolicy } from "@/api/accounts";
import { useSession } from "@/api/session";
import { Button } from "@/components/ui/button";

// The permission checklist (D-13). Until an admin saves a document every
// external write is denied, and the screen says exactly what is still
// unconfigured. "Apply suggested defaults" saves D-13's table as version N+1
// with caps and write accounts still empty, which keeps writes denied.
export function PoliciesPage() {
  const policies = usePolicies();
  const session = useSession();
  const put = usePutPolicy();
  const kill = useKillSwitch();
  const isAdmin = session.data?.role === "admin";

  if (policies.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (policies.error) return <p className="text-destructive">{policies.error.message}</p>;
  const state = policies.data;
  const current = state.current;
  const workspaceSwitch = state.killSwitches.find((k) => k.accountId === 0);

  return (
    <section className="space-y-8">
      <div>
        <h1 className="text-2xl font-semibold">Policies</h1>
        <p className="text-muted-foreground">
          {state.configured
            ? `Version ${current?.version} · saved ${new Date(current?.createdAt ?? "").toLocaleString()}`
            : "Not configured. Every external write is denied until an admin saves a policy."}
        </p>
      </div>

      {state.unconfigured.length > 0 && (
        <div className="rounded-lg border p-4">
          <h2 className="font-medium">Still unconfigured</h2>
          <p className="text-sm text-muted-foreground">
            Writes that depend on these stay denied. These values come from the owner, not from ADX.
          </p>
          <ul className="mt-2 list-disc pl-5 text-sm">
            {state.unconfigured.map((u) => (
              <li key={u}>{u}</li>
            ))}
          </ul>
        </div>
      )}

      <div className="rounded-lg border p-4">
        <div className="flex items-center justify-between">
          <h2 className="font-medium">Kill switch (whole workspace)</h2>
          <span className={workspaceSwitch?.engaged ? "text-destructive" : "text-muted-foreground"}>
            {workspaceSwitch?.engaged
              ? `Engaged: ${workspaceSwitch.reason || "no reason given"}`
              : "Off"}
          </span>
        </div>
        <p className="text-sm text-muted-foreground">
          Blocks every external write that hasn’t been sent yet. It can’t undo a change already
          made.
        </p>
        {isAdmin && (
          <Button
            variant="outline"
            size="sm"
            className="mt-2"
            disabled={kill.isPending}
            onClick={() =>
              kill.mutate({
                accountId: 0,
                engaged: !workspaceSwitch?.engaged,
                reason: workspaceSwitch?.engaged ? "" : "engaged from the policy screen",
              })
            }
          >
            {workspaceSwitch?.engaged ? "Clear kill switch" : "Engage kill switch"}
          </Button>
        )}
        {kill.error && <p className="text-sm text-destructive">{kill.error.message}</p>}
      </div>

      <div>
        <h2 className="font-medium">Checklist</h2>
        <table className="mt-2 w-full text-sm">
          <thead className="text-left text-muted-foreground">
            <tr>
              <th className="py-1 font-normal">Action</th>
              <th className="py-1 font-normal">Marketer</th>
              <th className="py-1 font-normal">Admin</th>
            </tr>
          </thead>
          <tbody className="divide-y border-y">
            {state.catalog.map((a) => {
              const rule = current?.document.actions?.[a.action];
              return (
                <tr key={a.action}>
                  <td className="py-2">
                    <div>{a.description}</div>
                    <div className="text-muted-foreground">{a.action}</div>
                  </td>
                  <td className="py-2">{mode(rule?.marketer)}</td>
                  <td className="py-2">{mode(rule?.admin)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
        <dl className="mt-4 grid grid-cols-2 gap-2 text-sm">
          <dt className="text-muted-foreground">Daily spend increase cap</dt>
          <dd>{current?.document.caps.dailySpendIncreasePct ?? "Unconfigured"}</dd>
          <dt className="text-muted-foreground">Changes per day cap</dt>
          <dd>{current?.document.caps.maxChangesPerDay ?? "Unconfigured"}</dd>
          <dt className="text-muted-foreground">Accounts enabled for writes</dt>
          <dd>
            {current && current.document.writeAccounts.length > 0
              ? current.document.writeAccounts.join(", ")
              : "None"}
          </dd>
        </dl>
        {isAdmin && (
          <div className="mt-4 space-y-2">
            <Button
              variant="outline"
              size="sm"
              disabled={put.isPending}
              onClick={() => put.mutate(state.suggested)}
            >
              Apply suggested defaults (D-13 table)
            </Button>
            <p className="text-xs text-muted-foreground">
              Saves a new version with D-13’s checklist. Caps and write accounts stay empty, so
              writes stay denied until they are set.
            </p>
            {put.error && <p className="text-sm text-destructive">{put.error.message}</p>}
          </div>
        )}
      </div>
    </section>
  );
}

function mode(m: string | undefined): string {
  switch (m) {
    case "allowed":
      return "Allowed";
    case "needs_approval":
      return "Needs approval";
    case "off":
      return "Off";
    default:
      return "Unconfigured";
  }
}
