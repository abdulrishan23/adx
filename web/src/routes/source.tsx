import { getRouteApi, Link } from "@tanstack/react-router";
import { ArrowLeft, RefreshCw } from "lucide-react";
import { useId, useState } from "react";
import {
  useDisconnectSource,
  useSaveSourceKey,
  useSetSourceAccounts,
  useSignInSource,
  useSource,
  useSyncSource,
} from "@/api/adx";
import type { SourceDetail } from "@/api/models";
import { Form } from "@/components/form";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusChip } from "@/components/ui/status-chip";
import { formatRelative } from "@/lib/format";

const route = getRouteApi("/app/settings/connections/$key");

export function SourcePage() {
  const { key } = route.useParams();
  const source = useSource(key);
  return (
    <>
      <Link
        to="/settings/connections"
        className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> Connections
      </Link>
      {source.isPending && <LoadingRows rows={6} />}
      {source.error && <ErrorNote error={source.error} />}
      {source.data === null && (
        <Panel>
          <EmptyState title="Adx doesn't connect to that" />
        </Panel>
      )}
      {source.data && <Body s={source.data} />}
    </>
  );
}

function Body({ s }: { s: SourceDetail }) {
  return (
    <>
      <PageHeader
        title={s.name}
        subtitle={s.signedInAs ? `Signed in as ${s.signedInAs}` : undefined}
        actions={<StatusChip status={s.state}>{s.stateLabel}</StatusChip>}
      />
      {s.issue && (
        <p className="mb-6 rounded-lg border border-status-caution/50 bg-status-caution/10 px-4 py-3 text-sm">
          {s.issue}
        </p>
      )}
      <div className="grid gap-6 lg:grid-cols-[3fr_2fr]">
        {s.method === "key" ? <KeyPanel s={s} /> : <AccountsPanel s={s} />}
        <Panel title="Access">
          <dl className="space-y-4 text-sm">
            <List label="Reads" items={s.reads} />
            {s.writes.length > 0 && <List label="Writes, through proposals" items={s.writes} />}
            <div>
              <dt>
                <Eyebrow>Needs</Eyebrow>
              </dt>
              <dd className="mt-1.5 flex flex-wrap gap-1.5">
                {s.access.map((a) => (
                  <span
                    key={a}
                    className="rounded-sm border px-1.5 py-0.5 font-mono text-[0.65rem]"
                  >
                    {a}
                  </span>
                ))}
              </dd>
            </div>
          </dl>
        </Panel>
      </div>
    </>
  );
}

function List({ label, items }: { label: string; items: string[] }) {
  return (
    <div>
      <dt>
        <Eyebrow>{label}</Eyebrow>
      </dt>
      <dd>
        <ul className="mt-1.5 list-disc space-y-0.5 pl-5">
          {items.map((i) => (
            <li key={i}>{i}</li>
          ))}
        </ul>
      </dd>
    </div>
  );
}

function AccountsPanel({ s }: { s: SourceDetail }) {
  const signIn = useSignInSource();
  const save = useSetSourceAccounts();
  const sync = useSyncSource();
  const disconnect = useDisconnectSource();
  const [picked, setPicked] = useState<string[]>(
    s.accounts.filter((a) => a.connected).map((a) => a.id),
  );
  const [confirming, setConfirming] = useState(false);
  const saved = s.accounts.filter((a) => a.connected).map((a) => a.id);
  const changed = picked.length !== saved.length || picked.some((id) => !saved.includes(id));
  const error = signIn.error ?? save.error ?? sync.error ?? disconnect.error;

  if (s.accounts.length === 0) {
    return (
      <Panel title="Connect">
        <Button
          disabled={signIn.isPending}
          onClick={() => signIn.mutate({ key: s.key, input: null })}
        >
          {signIn.isPending ? "Waiting for " : "Sign in with "}
          {s.signInWith}
        </Button>
        {error && (
          <div className="mt-3">
            <ErrorNote error={error} />
          </div>
        )}
      </Panel>
    );
  }

  return (
    <Panel
      title="Accounts"
      bodyClassName="p-0"
      action={
        <Button
          size="sm"
          variant="outline"
          disabled={sync.isPending || saved.length === 0}
          onClick={() => sync.mutate({ key: s.key, input: null })}
        >
          <RefreshCw className={sync.isPending ? "animate-spin" : undefined} /> Sync now
        </Button>
      }
    >
      <ul className="divide-y">
        {s.accounts.map((a) => (
          <li key={a.id}>
            <label className="flex cursor-pointer items-center gap-3 px-5 py-3 text-sm">
              <input
                type="checkbox"
                className="size-4 accent-foreground"
                checked={picked.includes(a.id)}
                onChange={(e) =>
                  setPicked((p) => (e.target.checked ? [...p, a.id] : p.filter((x) => x !== a.id)))
                }
              />
              <span className="min-w-0 flex-1">
                <span className="block font-medium">{a.name}</span>
                <span className="block font-mono text-xs text-muted-foreground">{a.id}</span>
              </span>
              <span className="shrink-0 font-mono text-xs text-muted-foreground">
                {a.connected
                  ? a.lastSyncAt
                    ? `Synced ${formatRelative(a.lastSyncAt)}`
                    : "Syncing"
                  : ""}
              </span>
            </label>
          </li>
        ))}
      </ul>
      <div className="flex flex-wrap items-center gap-2 border-t px-5 py-3">
        {changed && (
          <Button
            size="sm"
            disabled={save.isPending}
            onClick={() => save.mutate({ key: s.key, input: picked })}
          >
            Save accounts
          </Button>
        )}
        <span className="ml-auto" />
        {confirming ? (
          <>
            <span className="text-sm">Stop reading {s.name}?</span>
            <Button size="sm" variant="ghost" onClick={() => setConfirming(false)}>
              Cancel
            </Button>
            <Button
              size="sm"
              variant="destructive"
              disabled={disconnect.isPending}
              onClick={() =>
                disconnect.mutate(
                  { key: s.key, input: null },
                  {
                    onSuccess: () => {
                      setConfirming(false);
                      setPicked([]);
                    },
                  },
                )
              }
            >
              Disconnect
            </Button>
          </>
        ) : (
          <Button size="sm" variant="ghost" onClick={() => setConfirming(true)}>
            Disconnect
          </Button>
        )}
      </div>
      {error && (
        <div className="px-5 pb-3">
          <ErrorNote error={error} />
        </div>
      )}
    </Panel>
  );
}

function KeyPanel({ s }: { s: SourceDetail }) {
  const save = useSaveSourceKey();
  const [value, setValue] = useState("");
  const id = useId();
  return (
    <Panel title={s.accounts.length ? "Project" : "Connect"}>
      {s.accounts.map((a) => (
        <p key={a.id} className="mb-4 text-sm">
          <span className="font-medium">{a.name}</span>
          <span className="ml-2 font-mono text-xs text-muted-foreground">{a.id}</span>
        </p>
      ))}
      <Form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate({ key: s.key, input: value });
        }}
      >
        <div className="space-y-1">
          <label htmlFor={id} className="text-sm">
            Project API key
          </label>
          <Input
            id={id}
            value={value}
            onChange={(e) => {
              save.reset();
              setValue(e.target.value);
            }}
            autoComplete="off"
            spellCheck={false}
            className="font-mono"
          />
        </div>
        <Button type="submit" disabled={!value || save.isPending}>
          Save key
        </Button>
        {save.error && <ErrorNote error={save.error} />}
      </Form>
    </Panel>
  );
}
