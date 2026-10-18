import { Link } from "@tanstack/react-router";
import { useConnections } from "@/api/adx";
import type { Connection } from "@/api/models";
import { ErrorNote, Eyebrow, LoadingRows, PageHeader } from "@/components/page";
import { buttonVariants } from "@/components/ui/button";
import { StatusChip } from "@/components/ui/status-chip";
import { formatRelative } from "@/lib/format";

const GROUPS: Connection["group"][] = ["Advertising", "CRM", "Analytics", "Organic"];

const ACTION: Record<Connection["state"], string> = {
  ready: "Manage",
  caution: "Review",
  error: "Reconnect",
  pending: "Set up",
};

export function ConnectionsPage() {
  const connections = useConnections();
  return (
    <>
      <PageHeader title="Connections" />
      {connections.isPending && <LoadingRows rows={6} />}
      {connections.error && <ErrorNote error={connections.error} />}
      {connections.data && (
        <div className="space-y-8">
          {GROUPS.map((group) => {
            const items = connections.data.filter((c) => c.group === group);
            if (items.length === 0) return null;
            return (
              <section key={group} className="space-y-3">
                <Eyebrow>{group}</Eyebrow>
                <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                  {items.map((c) => (
                    <ConnectionCard key={c.key} connection={c} />
                  ))}
                </div>
              </section>
            );
          })}
        </div>
      )}
    </>
  );
}

function ConnectionCard({ connection: c }: { connection: Connection }) {
  return (
    <article className="flex flex-col rounded-lg border bg-card p-5">
      <div className="flex items-start justify-between gap-3">
        <h2 className="text-lg font-semibold">{c.name}</h2>
        <StatusChip status={c.state}>{c.stateLabel}</StatusChip>
      </div>
      <p className="mt-2 text-sm text-muted-foreground">{c.detail}</p>
      {c.accounts.length > 0 && (
        <ul className="mt-3 space-y-0.5 text-sm">
          {c.accounts.map((a) => (
            <li key={a}>{a}</li>
          ))}
        </ul>
      )}
      <div className="mt-auto flex items-center justify-between pt-5">
        <span className="font-mono text-xs text-muted-foreground">
          {c.lastSyncAt ? `Synced ${formatRelative(c.lastSyncAt)}` : "Never synced"}
        </span>
        {c.key === "ghl" ? (
          <Link
            to="/settings/connections/gohighlevel"
            className={buttonVariants({
              size: "sm",
              variant: c.state === "error" ? "default" : "outline",
            })}
          >
            {ACTION[c.state]}
          </Link>
        ) : (
          <Link
            to="/settings/connections/$key"
            params={{ key: c.key }}
            className={buttonVariants({
              size: "sm",
              variant: c.state === "error" ? "default" : "outline",
            })}
          >
            {ACTION[c.state]}
          </Link>
        )}
      </div>
    </article>
  );
}
