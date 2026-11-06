import { Link } from "@tanstack/react-router";
import { ArrowLeft, Check, RefreshCw } from "lucide-react";
import { type FormEvent, useId, useState } from "react";
import { ApiError } from "@/api/client";
import { REQUIRED_SCOPES, useCheckCrm, useConnectCrm, useCrmConnections } from "@/api/gohighlevel";
import { useImportLeads, useLeadCoverage } from "@/api/leads";
import type { Account, Connection, Run } from "@/api/types.gen";
import { useSession } from "@/app/session";
import { Form } from "@/components/form";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusChip, StatusMark } from "@/components/ui/status-chip";
import { useAccounts } from "@/engine/api/accounts";
import { useRuns } from "@/engine/api/runs";
import { formatDate, formatNumber, formatRelative, NOT_AVAILABLE } from "@/lib/format";

const AUTH_LABEL: Record<string, string> = {
  private_token: "Private integration token",
  oauth: "Marketplace app",
};

// The server answers 409 while it has no connection key, so there is
// nowhere to keep a token encrypted.
const notConfigured = (e: Error | null) => e instanceof ApiError && e.status === 409;

// The GoHighLevel connection (core/connections): the workspaces connected
// with a private integration token, each with its scopes and a check, the
// form that connects another, and for each workspace the state of its lead
// copy (core/leads): what the last import did and how much of the copy
// carries a status and a project.
export function ConnectGhlPage() {
  const list = useCrmConnections();
  if (notConfigured(list.error)) return <SetupNeeded />;
  return (
    <>
      <BackLink />
      <PageHeader title="GoHighLevel" />
      <div className="grid gap-6 lg:grid-cols-[3fr_2fr]">
        <div className="space-y-4">
          <Eyebrow>Connected workspaces</Eyebrow>
          {list.isPending && <LoadingRows rows={3} />}
          {list.error && <ErrorNote error={list.error} />}
          {list.data?.length === 0 && (
            <Panel>
              <EmptyState title="No workspace connected yet">
                Connect one with the form. Leads arrive once a workspace is connected and imported.
              </EmptyState>
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

function BackLink() {
  return (
    <Link
      to="/settings/connections"
      className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
    >
      <ArrowLeft className="size-4" /> Connections
    </Link>
  );
}

function WorkspaceCard({ connection: c }: { connection: Connection }) {
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

const IMPORT_KIND = "ghl.import";

// Adx's copy of the workspace's contacts. GoHighLevel stays the source of
// truth; an import reads every contact again, keeps each raw status and
// source verbatim, and marks contacts it no longer sees as removed.
function LeadCopy({ locationId }: { locationId: string }) {
  const accounts = useAccounts();
  const account = accounts.data?.accounts.find(
    (a: Account) => a.provider === "gohighlevel" && a.externalId === locationId,
  );
  return (
    <Panel title="Leads" bodyClassName="p-0">
      {accounts.isPending && (
        <div className="p-5">
          <LoadingRows rows={3} />
        </div>
      )}
      {accounts.error && (
        <div className="p-5">
          <ErrorNote error={accounts.error} />
        </div>
      )}
      {accounts.data && !account && (
        <p className="p-5 text-sm text-muted-foreground">
          This workspace has no account on the server yet, so nothing can be imported. Reconnect it
          with the form.
        </p>
      )}
      {account && <LeadCopyBody account={account} />}
    </Panel>
  );
}

function LeadCopyBody({ account }: { account: Account }) {
  const coverage = useLeadCoverage(account.id);
  const runs = useRuns();
  const importLeads = useImportLeads();
  const session = useSession();
  const canImport = session.role === "admin" || session.role === "marketer";
  const last = lastImport(runs.data?.runs ?? [], account.id);
  const running = importLeads.isPending || last?.state === "queued" || last?.state === "running";
  const c = coverage.data;
  return (
    <>
      <div className="flex flex-wrap items-start justify-between gap-3 border-b px-5 py-3 text-sm">
        <div className="space-y-1">
          <ImportLine run={last} />
          {importLeads.data && (
            <p className="text-muted-foreground" role="status">
              {importLeads.data.created ? "Import started" : "An import is already running"}: run{" "}
              <span className="font-mono text-xs">{importLeads.data.run.id}</span>.
            </p>
          )}
          {importLeads.error && <ErrorNote error={importLeads.error} />}
        </div>
        {canImport && (
          <Button
            size="sm"
            variant="outline"
            disabled={running}
            onClick={() => importLeads.mutate({ accountId: account.id, mode: "full", since: null })}
          >
            <RefreshCw className={running ? "animate-spin" : undefined} />
            {running ? "Importing" : "Import now"}
          </Button>
        )}
      </div>
      <div className="px-5 py-4 text-sm">
        {coverage.isPending && <LoadingRows rows={2} />}
        {coverage.error && <ErrorNote error={coverage.error} />}
        {c && c.total === 0 && (
          <p className="text-muted-foreground">No leads copied yet. Import the workspace once.</p>
        )}
        {c && c.total > 0 && (
          <dl className="grid gap-x-6 gap-y-2.5 sm:grid-cols-3">
            <Row label="Leads">{formatNumber(c.total)}</Row>
            <Row label="With a status">{share(c.withStatus, c.total)}</Row>
            <Row label="With a project">{share(c.withProject, c.total)}</Row>
            <Row label="With a stage">
              {c.funnelConfigured ? share(c.withStage, c.total) : "No table saved"}
            </Row>
            <Row label="With a phone">{share(c.withPhone, c.total)}</Row>
            <Row label="Phones shared by several leads">{formatNumber(c.sharedPhones)}</Row>
          </dl>
        )}
        {c && c.statusMissingOnLastScan > 0 && (
          <p className="mt-3 flex items-baseline gap-2">
            <StatusMark status="caution" className="translate-y-[-1px]" />
            <span>
              {c.statusMissingOnLastScan === 1
                ? "1 lead came without the status field on the last scan"
                : `${formatNumber(c.statusMissingOnLastScan)} leads came without the status field on the last scan`}
              , so the last known status is kept. Check which field holds the status in{" "}
              <Link to="/settings/lists" className="underline underline-offset-4">
                Stages and lists
              </Link>
              .
            </span>
          </p>
        )}
        {c && c.total > 0 && (
          <p className="mt-3">
            <Link to="/leads" className="text-sm underline underline-offset-4">
              Open leads
            </Link>
          </p>
        )}
      </div>
    </>
  );
}

// The newest import run for this account, whatever its state.
export function lastImport(runs: Run[], accountId: number): Run | null {
  return (
    runs
      .filter((r) => r.kind === IMPORT_KIND && r.accountId === accountId)
      .sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))[0] ?? null
  );
}

function ImportLine({ run }: { run: Run | null }) {
  if (!run) return <p className="text-muted-foreground">Not imported yet.</p>;
  const progress = run.progress as { contacts?: number; pages?: number } | null;
  const contacts = progress?.contacts;
  if (run.state === "queued" || run.state === "running") {
    return (
      <p className="text-muted-foreground">
        Reading every contact from GoHighLevel
        {contacts !== undefined ? `: ${formatNumber(contacts)} so far` : ""}.
      </p>
    );
  }
  if (run.state === "failed") {
    return (
      <p className="flex items-baseline gap-2">
        <StatusMark status="error" className="translate-y-[-1px]" />
        <span>
          Last import failed {formatRelative(run.finishedAt ?? run.createdAt)}
          {run.error ? `: ${run.error}` : ""}
          {run.completeness === "partial" ? " (the pages read before it failed are kept)" : ""}
        </span>
      </p>
    );
  }
  return (
    <p>
      Imported {formatRelative(run.finishedAt ?? run.createdAt)}
      <span className="text-muted-foreground">
        {" · "}
        {contacts !== undefined ? `${formatNumber(contacts)} contacts read` : NOT_AVAILABLE}
        {run.completeness === "partial" ? " · partial" : ""}
      </span>
    </p>
  );
}

const share = (part: number, total: number) =>
  total === 0 ? NOT_AVAILABLE : `${Math.round((part / total) * 100)}%`;

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
  const session = useSession();
  const [locationId, setLocationId] = useState("");
  const [token, setToken] = useState("");
  const idField = useId();
  const tokenField = useId();

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    connect.mutate({ locationId: locationId.trim(), token }, { onSuccess: () => setToken("") });
  }

  if (session.role !== "admin") {
    return (
      <Panel title="Connect a workspace">
        <p className="text-sm text-muted-foreground">
          An admin connects a workspace with a private integration token.
        </p>
      </Panel>
    );
  }
  return (
    <Panel title="Connect a workspace">
      <ol className="mb-5 list-decimal space-y-1.5 pl-5 text-sm text-muted-foreground">
        <li>Open the workspace in GoHighLevel and copy its ID from the URL, after /location/.</li>
        <li>Go to Settings, then Private Integrations, then Create new integration.</li>
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
      <BackLink />
      <PageHeader title="GoHighLevel" />
      <Panel title="The server has no connection key">
        <p className="text-sm">
          Nothing is connected and nothing can be: tokens are kept encrypted with a key the server
          has not got. Create one, then restart make dev.
        </p>
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
