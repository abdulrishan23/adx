import type { Account, ConnectorCapability, Freshness } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { useAccounts, useCapabilityMatrix, useSetAccountEnabled } from "@/engine/api/accounts";
import { useFreshness, useRefreshAnalytics } from "@/engine/api/analytics";
import { useSession } from "@/engine/api/session";

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

const providerLabel: Record<string, string> = {
  gohighlevel: "GoHighLevel",
  google_ads: "Google Ads",
  meta: "Meta",
  whatsapp: "WhatsApp Cloud",
  email: "Email (SMTP)",
  telephony: "Telephony",
  speech: "Speech engine",
  model: "Model",
  mail: "Staff mail",
};

function dayAgo(days: number): string {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() - days);
  return d.toISOString().slice(0, 10);
}

// Every connected account, whatever the provider, with the health derived
// from its last check and, for ad accounts, how fresh its figures are. An
// empty list is empty: there are no sample accounts.
export function ConnectionsPage() {
  const session = useSession();
  const isAdmin = session.data?.role === "admin";
  // Admins see the accounts that are off too, so they can switch them on.
  const accounts = useAccounts(isAdmin);
  const freshness = useFreshness();
  const refresh = useRefreshAnalytics();
  const setEnabled = useSetAccountEnabled();
  const canRefresh = isAdmin || session.data?.role === "marketer";
  const freshByAccount = new Map<number, Freshness>();
  for (const f of freshness.data?.accounts ?? []) freshByAccount.set(f.accountId, f);

  return (
    <section className="space-y-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-3xl font-semibold tracking-tight">Connections</h1>
        {isAdmin && (
          <div className="flex gap-2">
            <a href="/api/connections/google_ads/start">
              <Button variant="outline" size="sm">
                Connect Google Ads
              </Button>
            </a>
            <a href="/api/connections/meta/start">
              <Button variant="outline" size="sm">
                Connect Meta
              </Button>
            </a>
            <a href="/api/connections/crm/connect">
              <Button variant="outline" size="sm">
                Connect GoHighLevel
              </Button>
            </a>
          </div>
        )}
      </div>
      <p className="text-sm text-muted-foreground">
        Connecting an ad platform lists every account the sign-in reaches. Each one starts off; an
        admin switches on the ones that belong to this workspace, and only those are read. Nothing
        here publishes or writes on its own: writes stay off until live writes are on and the policy
        allows them, and ADX never publishes creative or posts. The matrix below says exactly what
        each connector does and does not do on this server.
      </p>
      {setEnabled.error && <p className="text-sm text-destructive">{setEnabled.error.message}</p>}
      {refresh.data && (
        <p className="text-sm text-muted-foreground">
          {refresh.data.created ? "Refresh started" : "Refresh already running"}: run{" "}
          {refresh.data.run.id}. See Runs for progress.
        </p>
      )}
      {refresh.error && <p className="text-sm text-destructive">{refresh.error.message}</p>}
      {accounts.isPending && <p className="text-muted-foreground">Loading…</p>}
      {accounts.error && <p className="text-destructive">{accounts.error.message}</p>}
      {accounts.data && accounts.data.accounts.length === 0 && (
        <p className="text-muted-foreground">
          No accounts are connected yet. An admin connects them with the buttons above; each
          platform needs its own app credentials on the server first.
        </p>
      )}
      {accounts.data && accounts.data.accounts.length > 0 && (
        <ul className="divide-y border-y">
          {accounts.data.accounts.map((a) => {
            const fresh = freshByAccount.get(a.id);
            const isAds = a.provider === "google_ads" || a.provider === "meta";
            return (
              <li key={a.id} className="space-y-1 py-3">
                <div className="flex items-baseline justify-between gap-4">
                  <span>
                    <span className="font-medium">{a.name ?? a.externalId}</span>{" "}
                    <span className="text-muted-foreground">
                      {providerLabel[a.provider] ?? a.provider} · {a.externalId} · {a.authMethod}
                    </span>
                  </span>
                  <span className="flex items-center gap-3">
                    {a.enabled ? (
                      <span className={healthClass[a.health]}>{healthLabel[a.health]}</span>
                    ) : (
                      <span className="text-muted-foreground">Off: not read</span>
                    )}
                    {isAdmin && isAds && (
                      <Button
                        variant="outline"
                        size="xs"
                        disabled={setEnabled.isPending}
                        onClick={() => setEnabled.mutate({ accountId: a.id, enabled: !a.enabled })}
                      >
                        {a.enabled ? "Switch off" : "Switch on"}
                      </Button>
                    )}
                  </span>
                </div>
                <div className="text-sm text-muted-foreground">
                  Verified {new Date(a.verifiedAt).toLocaleString()}
                  {a.timeZone ? ` · ${a.timeZone}` : ""}
                  {a.currency ? ` · ${a.currency}` : ""}
                </div>
                {isAds && a.enabled && (
                  <div className="flex flex-wrap items-center gap-3 text-sm">
                    <FreshnessLine fresh={fresh} />
                    {canRefresh && (
                      <Button
                        variant="outline"
                        size="xs"
                        disabled={refresh.isPending}
                        onClick={() =>
                          refresh.mutate({ accountId: a.id, from: dayAgo(30), to: dayAgo(0) })
                        }
                      >
                        Refresh last 30 days
                      </Button>
                    )}
                  </div>
                )}
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
            );
          })}
        </ul>
      )}
      <CapabilityMatrix />
    </section>
  );
}

// The matrix is what a connector's name may claim: every unsupported row is
// written down with its reason, so a logo never implies publishing or any
// write the server cannot do.
function CapabilityMatrix() {
  const matrix = useCapabilityMatrix();
  if (!matrix.data) return null;
  const byProvider = new Map<string, ConnectorCapability[]>();
  for (const c of matrix.data.capabilities) {
    const list = byProvider.get(c.provider) ?? [];
    list.push(c);
    byProvider.set(c.provider, list);
  }
  return (
    <div className="space-y-3">
      <h2 className="text-2xl font-semibold tracking-tight">What each connector can do</h2>
      <p className="text-sm text-muted-foreground">
        Live writes are {matrix.data.liveWrites ? "on" : "off"} on this server. Supported means the
        code exists; wired means this server is configured for it.
      </p>
      <div className="grid gap-4 md:grid-cols-2">
        {[...byProvider.entries()].map(([provider, caps]) => (
          <div key={provider} className="rounded border p-3">
            <h3 className="font-semibold">{providerLabel[provider] ?? provider}</h3>
            <ul className="mt-2 space-y-1 text-sm">
              {caps.map((c) => (
                <li key={c.capability} className="flex flex-wrap gap-x-2">
                  <span className="font-mono text-xs">{c.capability}</span>
                  <span
                    className={
                      c.supported
                        ? c.wired
                          ? "text-foreground"
                          : "text-muted-foreground"
                        : "text-destructive"
                    }
                  >
                    {c.supported ? (c.wired ? "wired" : "supported, not wired") : "not supported"}
                  </span>
                  <span className="w-full text-muted-foreground">{c.reason}</span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </div>
  );
}

// Freshness says through which day a complete read exists. "Never read" is
// a real state, not a zero.
function FreshnessLine({ fresh }: { fresh: Freshness | undefined }) {
  if (!fresh?.lastSuccessAt) {
    return (
      <span className="text-muted-foreground">
        Figures: never read{fresh?.lastError ? ` (last attempt failed: ${fresh.lastError})` : ""}
      </span>
    );
  }
  return (
    <span className="text-muted-foreground">
      Figures observed through {fresh.observedThrough ?? "unknown"}
      {fresh.lastPartial ? " (last read partial)" : ""} · read{" "}
      {new Date(fresh.lastSuccessAt).toLocaleString()}
      {fresh.lastError ? ` · last attempt failed: ${fresh.lastError}` : ""}
    </span>
  );
}
