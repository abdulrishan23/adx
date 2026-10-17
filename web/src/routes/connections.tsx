import { useAccounts } from "@/api/accounts";
import type { Account } from "@/api/types.gen";

const healthLabel: Record<Account["health"], string> = {
  connected: "Connected",
  stale: "Stale: not checked in the last day",
  needs_permission: "Needs permission",
  failed: "Failed",
};

const healthClass: Record<Account["health"], string> = {
  connected: "text-foreground",
  stale: "text-muted-foreground",
  needs_permission: "text-destructive",
  failed: "text-destructive",
};

// Every connected account, whatever the provider, with the health derived
// from its last check. An empty list is empty: there are no sample accounts.
export function ConnectionsPage() {
  const accounts = useAccounts();
  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-semibold">Connections</h1>
      {accounts.isPending && <p className="text-muted-foreground">Loading…</p>}
      {accounts.error && <p className="text-destructive">{accounts.error.message}</p>}
      {accounts.data && accounts.data.accounts.length === 0 && (
        <p className="text-muted-foreground">
          No accounts are connected. GoHighLevel connects at{" "}
          <a className="underline underline-offset-4" href="/api/connections/crm/connect">
            /api/connections/crm/connect
          </a>
          . Google and Meta arrive in phase 1.
        </p>
      )}
      {accounts.data && accounts.data.accounts.length > 0 && (
        <ul className="divide-y border-y">
          {accounts.data.accounts.map((a) => (
            <li key={a.id} className="space-y-1 py-3">
              <div className="flex items-baseline justify-between gap-4">
                <span>
                  <span className="font-medium">{a.name ?? a.externalId}</span>{" "}
                  <span className="text-muted-foreground">
                    {a.provider} · {a.externalId} · {a.authMethod}
                  </span>
                </span>
                <span className={healthClass[a.health]}>{healthLabel[a.health]}</span>
              </div>
              <div className="text-sm text-muted-foreground">
                Verified {new Date(a.verifiedAt).toLocaleString()}
                {a.timeZone ? ` · ${a.timeZone}` : ""}
                {a.currency ? ` · ${a.currency}` : ""}
              </div>
              {a.capabilities.length > 0 && (
                <ul className="text-sm">
                  {a.capabilities.map((c) => (
                    <li key={c.name}>
                      {c.name}: {c.state}
                      {c.lastError ? ` (${c.lastError})` : ""}
                    </li>
                  ))}
                </ul>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
