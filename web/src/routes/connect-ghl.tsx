import { Link } from "@tanstack/react-router";
import { ArrowLeft, Check, RefreshCw } from "lucide-react";
import { type FormEvent, useId, useState } from "react";
import {
  useCheckCrm,
  useConnectCrm,
  useCrmConnections,
  useCrmLeads,
  useSyncCrmLeads,
} from "@/api/adx";
import { ApiError } from "@/api/client";
import { REQUIRED_SCOPES } from "@/api/mock/crm";
import type { Connection as CrmConnection, LeadSync } from "@/api/types.legacy";
import { Form } from "@/components/form";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { SourceName } from "@/components/platform";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatDate, formatNumber, formatRelative } from "@/lib/format";

const AUTH_LABEL: Record<string, string> = {
  private_token: "Private integration token",
  oauth: "Marketplace app",
};

// Replaces the server's stopgap HTML form at /api/connections/crm/connect.
// The server answers 409 with this when it has no connection key.
const notConfigured = (e: Error | null) => e instanceof ApiError && e.status === 409;

export function ConnectGhlPage() {
  const list = useCrmConnections();
  if (notConfigured(list.error)) return <SetupNeeded />;
  return (
    <>
      <Link
        to="/settings/connections"
        className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> Connections
      </Link>
      <PageHeader title="GoHighLevel" />
      <div className="grid gap-6 lg:grid-cols-[3fr_2fr]">
        <div className="space-y-4">
          <Eyebrow>Connected workspaces</Eyebrow>
          {list.isPending && <LoadingRows rows={3} />}
          {list.error && <ErrorNote error={list.error} />}
          {list.data?.length === 0 && (
            <Panel>
              <EmptyState title="No workspace connected yet" />
            </Panel>
          )}
          {list.data?.map((c) => (
            <div key={c.locationId} className="space-y-4">
              <WorkspaceCard connection={c} />
              <LeadCopy locationId={c.locationId} />
            </div>
          ))}
        </div>
        <ConnectForm />
      </div>
    </>
  );
}

function WorkspaceCard({ connection: c }: { connection: CrmConnection }) {
  const check = useCheckCrm();
  const missing = REQUIRED_SCOPES.filter((s) => !c.scopes.includes(s));
  return (
    <Panel
      title={c.name}
      description={<span className="font-mono text-xs">{c.locationId}</span>}
      action={
        missing.length ? (
          <StatusChip status="caution">Missing access</StatusChip>
        ) : (
          <StatusChip status="ready">Connected</StatusChip>
        )
      }
    >
      <dl className="grid gap-x-6 gap-y-2.5 text-sm sm:grid-cols-2">
        <Row label="Connected with">{AUTH_LABEL[c.authMethod] ?? c.authMethod}</Row>
        <Row label="Connected">
          {formatDate(c.connectedAt, { day: "numeric", month: "short", year: "numeric" })}
        </Row>
        <Row label="Last checked">{formatRelative(c.verifiedAt)}</Row>
        <Row label="Token expires">
          {c.tokenExpiresAt ? formatRelative(c.tokenExpiresAt) : "Doesn't expire"}
        </Row>
      </dl>
      <div className="mt-4 flex flex-wrap gap-1.5">
        {REQUIRED_SCOPES.map((s) => (
          <span
            key={s}
            className="inline-flex items-center gap-1 rounded-sm border px-1.5 py-0.5 font-mono text-[0.65rem]"
          >
            {c.scopes.includes(s) && (
              <Check className="size-3 text-status-ready" aria-hidden="true" />
            )}
            {s}
          </span>
        ))}
      </div>
      <div className="mt-4 flex items-center gap-3 border-t pt-4">
        <Button
          size="sm"
          variant="outline"
          disabled={check.isPending}
          onClick={() => check.mutate(c.locationId)}
        >
          <RefreshCw className={check.isPending ? "animate-spin" : undefined} />
          Test connection
        </Button>
        {check.isSuccess && (
          <span className="text-sm text-muted-foreground" role="status">
            Works. Adx can still read this workspace.
          </span>
        )}
      </div>
      {check.error && (
        <div className="mt-3">
          <ErrorNote error={check.error} />
        </div>
      )}
    </Panel>
  );
}

// Adx's copy of the workspace's leads. GoHighLevel stays the source of truth;
// the copy keeps the source each lead first came in with.
function LeadCopy({ locationId }: { locationId: string }) {
  const leads = useCrmLeads(locationId);
  const sync = useSyncCrmLeads();
  const last = leads.data?.lastSync ?? null;
  const running = sync.isPending || last?.state === "running";
  return (
    <Panel
      title="Leads"
      action={
        <Button
          size="sm"
          variant="outline"
          disabled={running}
          onClick={() => sync.mutate(locationId)}
        >
          <RefreshCw className={running ? "animate-spin" : undefined} />
          {running ? "Syncing" : "Sync now"}
        </Button>
      }
      bodyClassName="p-0"
    >
      <div className="space-y-2 border-b px-5 py-3 text-sm empty:hidden">
        {leads.data && <SyncLine sync={last} total={leads.data.total} />}
        {last && !last.statusFieldFound && last.state !== "running" && (
          <p className="flex items-baseline gap-2">
            <StatusMark status="caution" className="translate-y-[-1px]" />
            This workspace has no "Lead Status" field, so statuses weren't updated.
          </p>
        )}
        {sync.error && <ErrorNote error={sync.error} />}
      </div>
      {leads.isPending && (
        <div className="p-5">
          <LoadingRows rows={4} />
        </div>
      )}
      {leads.error && (
        <div className="p-5">
          <ErrorNote error={leads.error} />
        </div>
      )}
      {leads.data && leads.data.leads.length > 0 && (
        <Table>
          <THead>
            <TR>
              <TH>Name</TH>
              <TH>Status</TH>
              <TH>Source</TH>
              <TH numeric>Added</TH>
            </TR>
          </THead>
          <TBody>
            {leads.data.leads.map((l) => (
              <TR key={l.id}>
                <TD className="font-medium">{l.name || l.email || l.phone || "No name"}</TD>
                <TD>{l.status ?? <span className="text-muted-foreground">Blank</span>}</TD>
                <TD>
                  {l.source ? (
                    <SourceName name={l.source} />
                  ) : (
                    <span className="text-muted-foreground">Blank</span>
                  )}
                  {l.originalSource !== l.source && (
                    <span className="block text-xs text-muted-foreground">
                      First came in as {l.originalSource || "blank"}
                    </span>
                  )}
                </TD>
                <TD numeric className="text-muted-foreground">
                  {formatRelative(l.createdAt)}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}
    </Panel>
  );
}

function SyncLine({ sync, total }: { sync: LeadSync | null; total: number | undefined }) {
  if (!sync) return <p className="text-muted-foreground">Not synced yet.</p>;
  if (sync.state === "running") {
    return <p className="text-muted-foreground">Reading every contact from GoHighLevel.</p>;
  }
  if (sync.state === "failed") {
    return (
      <p className="flex items-baseline gap-2">
        <StatusMark status="error" className="translate-y-[-1px]" />
        <span>
          Last sync failed {formatRelative(sync.finishedAt ?? sync.startedAt)}: {sync.error}
        </span>
      </p>
    );
  }
  const changes = [
    sync.created && `${formatNumber(sync.created)} new`,
    sync.changed && `${formatNumber(sync.changed)} changed`,
    sync.removed && `${formatNumber(sync.removed)} removed`,
  ].filter(Boolean);
  return (
    <p>
      {formatNumber(total ?? sync.seen)} leads · synced{" "}
      {formatRelative(sync.finishedAt ?? sync.startedAt)}
      {changes.length > 0 && <span className="text-muted-foreground"> · {changes.join(", ")}</span>}
    </p>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}

function ConnectForm() {
  const connect = useConnectCrm();
  const [locationId, setLocationId] = useState("");
  const [token, setToken] = useState("");
  const idField = useId();
  const tokenField = useId();

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    connect.mutate({ locationId: locationId.trim(), token }, { onSuccess: () => setToken("") });
  }

  return (
    <Panel title="Connect a workspace">
      <ol className="mb-5 list-decimal space-y-1.5 pl-5 text-sm text-muted-foreground">
        <li>Open the workspace in GoHighLevel and copy its ID from the URL, after /location/.</li>
        <li>Go to Settings → Private Integrations → Create new integration.</li>
        <li>
          Name it "ADX" and give it read access to contacts, opportunities, locations and custom
          fields.
        </li>
        <li>Copy the token and paste it below.</li>
      </ol>
      <Form onSubmit={onSubmit} className="space-y-3">
        <div className="space-y-1">
          <label htmlFor={idField} className="text-sm">
            Workspace ID
          </label>
          <Input
            id={idField}
            value={locationId}
            onChange={(e) => {
              connect.reset();
              setLocationId(e.target.value);
            }}
            autoComplete="off"
            spellCheck={false}
            className="font-mono"
          />
        </div>
        <div className="space-y-1">
          <label htmlFor={tokenField} className="text-sm">
            Private integration token
          </label>
          <Input
            id={tokenField}
            type="password"
            value={token}
            onChange={(e) => {
              connect.reset();
              setToken(e.target.value);
            }}
            autoComplete="off"
            className="font-mono"
          />
        </div>
        <Button
          type="submit"
          className="w-full"
          disabled={connect.isPending || !locationId || !token}
        >
          {connect.isPending ? "Checking with GoHighLevel" : "Connect and test"}
        </Button>
        {connect.error && <ErrorNote error={connect.error} />}
        {connect.isSuccess && (
          <p className="text-sm" role="status">
            Connected <span className="font-medium">{connect.data.name}</span>. It's in the list.
          </p>
        )}
      </Form>
    </Panel>
  );
}

// Shown when the server runs without ADX_CONNECTION_KEY_FILE, so there's
// nowhere to keep tokens encrypted. make dev picks the key up once it exists.
function SetupNeeded() {
  return (
    <>
      <Link
        to="/settings/connections"
        className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> Connections
      </Link>
      <PageHeader title="GoHighLevel" />
      <Panel title="The server has no connection key">
        <p className="text-sm">Create one, then restart make dev.</p>
        <pre className="mt-3 overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs leading-relaxed">
          {`mkdir -p ~/.config/adx && chmod 700 ~/.config/adx
python3 -c "import os; open(os.path.expanduser('~/.config/adx/connections.key'),'xb').write(os.urandom(32))"
chmod 600 ~/.config/adx/connections.key`}
        </pre>
        <p className="mt-3 text-xs text-muted-foreground">docs/gohighlevel-development.md</p>
      </Panel>
    </>
  );
}
