import { useState } from "react";
import { useAccounts, usePolicies } from "@/api/accounts";
import { usePlans } from "@/api/plans";
import {
  useCancelExecution,
  useCreateProposal,
  useDecideProposal,
  useEnvelopes,
  useExecuteProposal,
  useExecutions,
  useOpenEnvelopes,
  useProposal,
  useProposals,
} from "@/api/proposals";
import { useSession } from "@/api/session";
import type { Envelope } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const states = [
  "",
  "proposed",
  "approved",
  "denied",
  "stale",
  "executing",
  "rejected",
  "cancelled",
];
const money = (minor: number | null | undefined, cur?: string | null) =>
  typeof minor === "number" ? `${cur ?? ""} ${(minor / 100).toLocaleString()}` : "none";

// Proposals: every change to an ad account is written down with its target
// version, policy decision and approval before anything happens. Executing
// reserves budget from an accepted plan and writes an intent into the
// outbox; nothing is sent from this page. The assistant can do none of it.
export function ProposalsPage() {
  const session = useSession();
  const [state, setState] = useState("");
  const [current, setCurrent] = useState<number | null>(null);
  const list = useProposals(state);
  const isAdmin = session.data?.role === "admin";
  const canEdit = isAdmin || session.data?.role === "marketer";
  return (
    <section className="space-y-6 text-sm">
      <h1 className="text-3xl font-semibold tracking-tight">Proposals</h1>
      <Envelopes isAdmin={isAdmin} />
      <div className="grid gap-6 md:grid-cols-[22rem_1fr]">
        <aside className="space-y-3">
          <div className="flex flex-wrap gap-1">
            {states.map((s) => (
              <Button
                key={s || "all"}
                size="xs"
                variant={state === s ? "default" : "outline"}
                onClick={() => setState(s)}
              >
                {s || "all"}
              </Button>
            ))}
          </div>
          {list.error && <p className="text-destructive">{list.error.message}</p>}
          {list.data?.proposals.length === 0 && (
            <p className="text-muted-foreground">No proposals.</p>
          )}
          <ul className="space-y-1">
            {list.data?.proposals.map((p) => (
              <li key={p.id}>
                <button
                  type="button"
                  className={`w-full rounded-md px-2 py-1 text-left ${p.id === current ? "bg-muted" : "hover:bg-muted"}`}
                  onClick={() => setCurrent(p.id)}
                >
                  <span className="font-medium">{p.action}</span> on {p.targetKind} {p.targetRef}
                  <span className="block text-muted-foreground">
                    {p.state} · {p.policyReason}
                    {p.amountMinor !== null ? ` · ${money(p.amountMinor, p.currency)}` : ""}
                  </span>
                </button>
              </li>
            ))}
          </ul>
          {canEdit && <NewProposal onCreated={setCurrent} />}
        </aside>
        <div className="space-y-6">
          {current === null ? (
            <p className="text-muted-foreground">Pick a proposal.</p>
          ) : (
            <ProposalView id={current} isAdmin={isAdmin} canEdit={canEdit} />
          )}
          <Executions isAdmin={isAdmin} />
        </div>
      </div>
    </section>
  );
}

function Envelopes({ isAdmin }: { isAdmin: boolean }) {
  const envelopes = useEnvelopes();
  const plans = usePlans();
  const accounts = useAccounts();
  const open = useOpenEnvelopes();
  const [planId, setPlanId] = useState(0);
  const [version, setVersion] = useState(1);
  const [google, setGoogle] = useState(0);
  const [meta, setMeta] = useState(0);
  const byProvider = (p: string) => accounts.data?.accounts.filter((a) => a.provider === p) ?? [];
  return (
    <section className="space-y-2 rounded-lg border p-3">
      <h2 className="font-semibold">
        Budget envelopes{" "}
        <span className="font-normal text-muted-foreground">
          (what accepted plans allow; reservations are checked against them)
        </span>
      </h2>
      {envelopes.data?.envelopes.length === 0 && (
        <p className="text-muted-foreground">
          None open. An admin opens them from an accepted plan version.
        </p>
      )}
      <ul className="space-y-1">
        {envelopes.data?.envelopes.map((e: Envelope) => (
          <li key={e.id}>
            plan {e.planId} v{e.planVersion} · {e.channel} · {money(e.amountMinor, e.currency)} ·
            reserved {money(e.reservedMinor, e.currency)} · spent{" "}
            {money(e.consumedMinor, e.currency)} ·{" "}
            <span className={e.remainingMinor <= 0 ? "text-destructive" : ""}>
              remaining {money(e.remainingMinor, e.currency)}
            </span>
          </li>
        ))}
      </ul>
      {isAdmin && (
        <div className="flex flex-wrap items-center gap-2">
          <select
            className="rounded-md border bg-background px-2 py-1"
            value={planId}
            onChange={(e) => setPlanId(Number(e.target.value))}
          >
            <option value={0}>Accepted plan…</option>
            {plans.data?.plans
              .filter((p) => p.accepted)
              .map((p) => (
                <option key={p.id} value={p.id}>
                  {p.title}
                </option>
              ))}
          </select>
          <Input
            className="h-7 max-w-20"
            type="number"
            value={version}
            onChange={(e) => setVersion(Number(e.target.value))}
            aria-label="Version"
          />
          <select
            className="rounded-md border bg-background px-2 py-1"
            value={google}
            onChange={(e) => setGoogle(Number(e.target.value))}
          >
            <option value={0}>Google Ads account…</option>
            {byProvider("google_ads").map((a) => (
              <option key={a.id} value={a.id}>
                {a.name ?? a.externalId}
              </option>
            ))}
          </select>
          <select
            className="rounded-md border bg-background px-2 py-1"
            value={meta}
            onChange={(e) => setMeta(Number(e.target.value))}
          >
            <option value={0}>Meta account…</option>
            {byProvider("meta").map((a) => (
              <option key={a.id} value={a.id}>
                {a.name ?? a.externalId}
              </option>
            ))}
          </select>
          <Button
            size="xs"
            disabled={!planId || open.isPending}
            onClick={() =>
              open.mutate({
                planId,
                planVersion: version,
                accountByPlatform: {
                  ...(google ? { google_ads: google } : {}),
                  ...(meta ? { meta } : {}),
                },
              })
            }
          >
            Open envelopes
          </Button>
          {open.error && <span className="text-destructive">{open.error.message}</span>}
        </div>
      )}
    </section>
  );
}

function NewProposal({ onCreated }: { onCreated: (id: number) => void }) {
  const policies = usePolicies();
  const accounts = useAccounts();
  const envelopes = useEnvelopes();
  const create = useCreateProposal();
  const [form, setForm] = useState({
    accountId: 0,
    action: "budget.raise",
    targetKind: "campaign",
    targetRef: "",
    targetVersion: "",
    expected: "{}",
    proposed: "{}",
    reason: "",
    amount: "",
    currency: "AED",
    envelopeId: 0,
    key: "",
  });
  const actions = policies.data?.catalog ?? [];
  return (
    <form
      className="space-y-2 rounded-lg border p-3"
      onSubmit={(e) => {
        e.preventDefault();
        let expectedCurrent: unknown = {};
        let proposed: unknown = {};
        try {
          expectedCurrent = JSON.parse(form.expected || "{}");
          proposed = JSON.parse(form.proposed || "{}");
        } catch {
          return;
        }
        create.mutate(
          {
            accountId: form.accountId,
            action: form.action as never,
            targetKind: form.targetKind,
            targetRef: form.targetRef,
            targetVersion: form.targetVersion,
            expectedCurrent: expectedCurrent as never,
            proposed: proposed as never,
            reason: form.reason,
            evidenceRefs: [],
            amountMinor: Math.round(Number(form.amount || 0) * 100),
            currency: form.currency,
            envelopeId: form.envelopeId,
            idempotencyKey: form.key || `${form.action}:${form.targetRef}:${Date.now()}`,
            requireApproval: false,
            supersedes: 0,
          },
          { onSuccess: (p) => onCreated(p.id) },
        );
      }}
    >
      <h2 className="font-semibold">New proposal</h2>
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={form.accountId}
        onChange={(e) => setForm({ ...form, accountId: Number(e.target.value) })}
      >
        <option value={0}>Account…</option>
        {accounts.data?.accounts.map((a) => (
          <option key={a.id} value={a.id}>
            {a.provider} {a.name ?? a.externalId}
          </option>
        ))}
      </select>
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={form.action}
        onChange={(e) => setForm({ ...form, action: e.target.value })}
      >
        {actions.map((a) => (
          <option key={a.action} value={a.action}>
            {a.action} — {a.description}
          </option>
        ))}
      </select>
      <div className="flex gap-1">
        <Input
          className="h-7 max-w-28"
          placeholder="target kind"
          value={form.targetKind}
          onChange={(e) => setForm({ ...form, targetKind: e.target.value })}
        />
        <Input
          className="h-7"
          placeholder="target ref (external id)"
          value={form.targetRef}
          onChange={(e) => setForm({ ...form, targetRef: e.target.value })}
        />
        <Input
          className="h-7"
          placeholder="target version (as observed)"
          value={form.targetVersion}
          onChange={(e) => setForm({ ...form, targetVersion: e.target.value })}
        />
      </div>
      <Input
        className="h-7"
        placeholder='Current, as JSON e.g. {"dailyBudgetMinor":500000}'
        value={form.expected}
        onChange={(e) => setForm({ ...form, expected: e.target.value })}
      />
      <Input
        className="h-7"
        placeholder='Proposed, as JSON e.g. {"dailyBudgetMinor":600000}'
        value={form.proposed}
        onChange={(e) => setForm({ ...form, proposed: e.target.value })}
      />
      <Input
        className="h-7"
        placeholder="Why"
        value={form.reason}
        onChange={(e) => setForm({ ...form, reason: e.target.value })}
      />
      <div className="flex gap-1">
        <Input
          className="h-7 max-w-28"
          placeholder="New spend"
          value={form.amount}
          onChange={(e) => setForm({ ...form, amount: e.target.value })}
        />
        <Input
          className="h-7 max-w-16"
          value={form.currency}
          onChange={(e) => setForm({ ...form, currency: e.target.value })}
          aria-label="Currency"
        />
        <select
          className="rounded-md border bg-background px-1"
          value={form.envelopeId}
          onChange={(e) => setForm({ ...form, envelopeId: Number(e.target.value) })}
        >
          <option value={0}>Envelope…</option>
          {envelopes.data?.envelopes.map((en) => (
            <option key={en.id} value={en.id}>
              plan {en.planId} v{en.planVersion} {en.channel} (
              {money(en.remainingMinor, en.currency)} left)
            </option>
          ))}
        </select>
      </div>
      <Input
        className="h-7"
        placeholder="Idempotency key (optional)"
        value={form.key}
        onChange={(e) => setForm({ ...form, key: e.target.value })}
      />
      <Button type="submit" size="sm" disabled={create.isPending || !form.accountId}>
        Propose
      </Button>
      {create.error && <p className="text-destructive">{create.error.message}</p>}
    </form>
  );
}

function ProposalView({
  id,
  isAdmin,
  canEdit,
}: {
  id: number;
  isAdmin: boolean;
  canEdit: boolean;
}) {
  const q = useProposal(id);
  const decide = useDecideProposal();
  const execute = useExecuteProposal();
  const [note, setNote] = useState("");
  if (q.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (q.error) return <p className="text-destructive">{q.error.message}</p>;
  const { proposal: p, authority } = q.data;
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <div>
        <h2 className="text-2xl font-semibold tracking-tight">
          {p.action} on {p.targetKind} {p.targetRef}
        </h2>
        <p className="text-muted-foreground">
          {p.state} · target version {p.targetVersion} · policy v{p.policyVersion ?? "—"}:{" "}
          {p.policyOutcome} ({p.policyReason}) · now: {authority.outcome} ({authority.reason})
          {p.amountMinor !== null &&
            ` · ${money(p.amountMinor, p.currency)} from envelope ${p.envelopeId}`}
        </p>
      </div>
      <div className="grid gap-2 md:grid-cols-2">
        <div>
          <p className="font-medium">Now</p>
          <pre className="rounded-md bg-muted p-2 text-xs">
            {JSON.stringify(p.expectedCurrent, null, 2)}
          </pre>
        </div>
        <div>
          <p className="font-medium">Proposed</p>
          <pre className="rounded-md bg-muted p-2 text-xs">
            {JSON.stringify(p.proposed, null, 2)}
          </pre>
        </div>
      </div>
      <p>{p.reason}</p>
      <div>
        <p className="font-medium">Approvals</p>
        {p.approvals.length === 0 && <p className="text-muted-foreground">None.</p>}
        {p.approvals.map((a) => (
          <p
            key={a.id}
            className={a.targetVersion === p.targetVersion ? "" : "text-muted-foreground"}
          >
            {a.decision} by user {a.decidedBy} on {new Date(a.decidedAt).toLocaleString()} for
            target version {a.targetVersion}
            {a.targetVersion !== p.targetVersion && " (no longer the target's version)"}
            {a.note ? ` · ${a.note}` : ""}
          </p>
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {isAdmin && p.state === "proposed" && (
          <>
            <Input
              className="h-7 max-w-xs"
              placeholder="Note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
            <Button
              size="xs"
              disabled={decide.isPending}
              onClick={() => decide.mutate({ id, decision: "approve", note })}
            >
              Approve
            </Button>
            <Button
              size="xs"
              variant="outline"
              disabled={decide.isPending}
              onClick={() => decide.mutate({ id, decision: "reject", note })}
            >
              Reject
            </Button>
          </>
        )}
        {canEdit && p.state === "approved" && (
          <Button
            size="xs"
            disabled={execute.isPending}
            onClick={() => execute.mutate({ id, idempotencyKey: `exec:${id}` })}
          >
            Execute (reserve budget, write intent)
          </Button>
        )}
        {decide.error && <span className="text-destructive">{decide.error.message}</span>}
        {execute.error && <span className="text-destructive">{execute.error.message}</span>}
      </div>
      {p.intent && (
        <p className="text-muted-foreground">
          Execution {p.intent.id}: {p.intent.state} · attempts {p.intent.attempts}
          {p.intent.lastError ? ` · ${p.intent.lastError}` : ""} · nothing is sent to the platform
          until the worker dispatches it (B3.5)
        </p>
      )}
      <p className="text-muted-foreground">
        Approval and execution are people's acts; the assistant can only read. A changed target
        makes this proposal stale.
      </p>
    </div>
  );
}

function Executions({ isAdmin }: { isAdmin: boolean }) {
  const q = useExecutions();
  const cancel = useCancelExecution();
  return (
    <section className="space-y-1">
      <h3 className="font-semibold">Action history (outbox)</h3>
      {q.data?.executions.length === 0 && <p className="text-muted-foreground">No executions.</p>}
      <ul className="space-y-1">
        {q.data?.executions.map((x) => (
          <li key={x.id} className="flex flex-wrap items-center gap-2">
            <span>
              #{x.id} · proposal {x.proposalId} · {x.state} ·{" "}
              {new Date(x.updatedAt).toLocaleString()}
              {x.lastError ? ` · ${x.lastError}` : ""}
              {x.providerRefs.length > 0 &&
                ` · ${x.providerRefs.map((r) => `${r.platform} ${r.kind} ${r.externalId}`).join(", ")}`}
            </span>
            {isAdmin &&
              (x.state === "reserved" || x.state === "requested" || x.state === "authorized") && (
                <Button
                  size="xs"
                  variant="outline"
                  disabled={cancel.isPending}
                  onClick={() => cancel.mutate({ id: x.id, reason: "cancelled from the outbox" })}
                >
                  Cancel before dispatch
                </Button>
              )}
          </li>
        ))}
      </ul>
      {cancel.error && <p className="text-destructive">{cancel.error.message}</p>}
    </section>
  );
}
