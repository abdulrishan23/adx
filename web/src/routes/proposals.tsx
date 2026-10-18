import { getRouteApi, useNavigate } from "@tanstack/react-router";
import { type ReactNode, useState } from "react";
import { useDecideProposal, useProposals, useRetryProposal, useUndoProposal } from "@/api/adx";
import type { Proposal, ProposalStatus } from "@/api/models";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { PlatformTag } from "@/components/platform";
import { Segmented } from "@/components/segmented";
import { Button } from "@/components/ui/button";
import { Sheet } from "@/components/ui/sheet";
import { StatusChip } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatRelative } from "@/lib/format";

const route = getRouteApi("/app/proposals");

type Tab = "pending" | "applied" | "rejected";

const TAB_OF: Record<ProposalStatus, Tab> = {
  pending: "pending",
  applying: "applied",
  applied: "applied",
  failed: "applied",
  undone: "applied",
  rejected: "rejected",
  expired: "rejected",
};

const STATUS_CHIP: Record<
  ProposalStatus,
  { status: "ready" | "caution" | "error" | "pending"; label: string }
> = {
  pending: { status: "pending", label: "Waiting" },
  applying: { status: "pending", label: "Applying" },
  applied: { status: "ready", label: "Applied" },
  failed: { status: "error", label: "Failed" },
  undone: { status: "pending", label: "Undone" },
  rejected: { status: "error", label: "Rejected" },
  expired: { status: "caution", label: "Expired" },
};

const SOURCE_LABEL = {
  alert: "Found by a check",
  marketer: "From a marketer",
  copilot: "From the copilot",
} as const;

export function ProposalsPage() {
  const proposals = useProposals();
  const { open } = route.useSearch();
  const navigate = useNavigate();
  const [tab, setTab] = useState<Tab>("pending");
  const list = proposals.data ?? [];
  const count = (t: Tab) => list.filter((p) => TAB_OF[p.status] === t).length;
  const rows = list.filter((p) => TAB_OF[p.status] === tab);
  const opened = list.find((p) => p.id === open) ?? null;

  const setOpen = (id: string | undefined) =>
    navigate({ to: "/proposals", search: id ? { open: id } : {} });

  return (
    <>
      <PageHeader
        title="Proposals"
        actions={
          <Segmented
            label="Proposal status"
            value={tab}
            onChange={setTab}
            options={[
              { value: "pending", label: "Waiting", count: count("pending"), tone: "pending" },
              { value: "applied", label: "Approved", count: count("applied"), tone: "ready" },
              { value: "rejected", label: "Rejected", count: count("rejected"), tone: "error" },
            ]}
          />
        }
      />
      {proposals.isPending && <LoadingRows rows={5} />}
      {proposals.error && <ErrorNote error={proposals.error} />}
      {proposals.data && rows.length === 0 && (
        <Panel>
          <EmptyState title={tab === "pending" ? "Nothing is waiting on you" : "Nothing here yet"}>
            {tab === "pending"
              ? "Checks run every morning. When one finds something worth changing, its proposal lands here."
              : "Decided proposals stay here with who decided and why."}
          </EmptyState>
        </Panel>
      )}
      <ul className="space-y-3">
        {rows.map((p) => (
          <li key={p.id}>
            <button
              type="button"
              onClick={() => setOpen(p.id)}
              className="flex w-full flex-wrap items-start gap-x-4 gap-y-2 rounded-lg border bg-card px-5 py-4 text-left transition-colors hover:border-foreground/40"
            >
              <PlatformTag
                platform={p.platform === "ghl" ? "ghl" : p.platform}
                className="mt-1 w-24"
              />
              <div className="min-w-0 flex-1 space-y-0.5">
                <p className="font-medium">{p.title}</p>
                <p className="text-sm text-muted-foreground">
                  {p.action} · {p.target}
                </p>
                <p className="text-xs text-muted-foreground">
                  {formatRelative(p.createdAt)} · {SOURCE_LABEL[p.source]}
                  {p.status === "pending" && ` · expires ${formatRelative(p.expiresAt)}`}
                  {p.decidedBy &&
                    ` · ${p.status === "rejected" ? "rejected" : "approved"} by ${p.decidedBy}`}
                </p>
              </div>
              <div className="flex w-full items-center justify-between gap-2 sm:w-auto sm:justify-start">
                <span className="text-sm text-muted-foreground">{p.impact.label}</span>
                <StatusChip status={STATUS_CHIP[p.status].status}>
                  {STATUS_CHIP[p.status].label}
                </StatusChip>
              </div>
            </button>
          </li>
        ))}
      </ul>
      {opened && <ProposalSheet proposal={opened} onClose={() => setOpen(undefined)} />}
    </>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-2 border-b py-5 first:pt-0 last:border-b-0">
      <Eyebrow>{title}</Eyebrow>
      {children}
    </section>
  );
}

function ProposalSheet({ proposal: p, onClose }: { proposal: Proposal; onClose: () => void }) {
  const decide = useDecideProposal();
  const session = useSession();
  const [note, setNote] = useState("");
  // The CMO approves anything that needs approval; a marketer can't approve
  // their own (reference.md, roles). The dev session is an admin.
  const canDecide = p.status === "pending" && (!p.needsApproval || session.role === "admin");

  const footer =
    p.status === "pending" ? (
      <div className="space-y-3">
        <label className="block space-y-1.5">
          <span className="text-sm">Note for the record (optional)</span>
          <textarea
            value={note}
            onChange={(e) => setNote(e.target.value)}
            rows={2}
            placeholder="For example: only the brand campaign, keep the weekend budget"
            className="w-full rounded-lg border border-input bg-transparent px-2.5 py-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
          />
        </label>
        {decide.error && <p className="text-sm text-destructive">{decide.error.message}</p>}
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground">
            {p.needsApproval
              ? "Needs the CMO's approval"
              : "Allowed for marketers by the checklist"}
          </p>
          <div className="flex gap-2">
            <Button
              variant="outline"
              disabled={!canDecide || decide.isPending}
              onClick={() =>
                decide.mutate(
                  { id: p.id, decision: "reject", note: note || undefined },
                  { onSuccess: onClose },
                )
              }
            >
              Reject
            </Button>
            <Button
              disabled={!canDecide || decide.isPending}
              onClick={() =>
                decide.mutate({ id: p.id, decision: "approve", note: note || undefined })
              }
            >
              Approve and apply
            </Button>
          </div>
        </div>
      </div>
    ) : (
      <AfterFooter proposal={p} />
    );

  return (
    <Sheet
      open
      onOpenChange={(o) => !o && onClose()}
      title={p.title}
      description={`${p.action} · ${p.target}`}
      footer={footer}
    >
      <Section title="The problem">
        <p className="text-sm leading-relaxed">{p.problem}</p>
      </Section>
      <Section title="The change">
        <div className="rounded-lg border">
          <Table>
            <THead>
              <TR>
                <TH>What</TH>
                <TH>Now</TH>
                <TH>After</TH>
              </TR>
            </THead>
            <TBody>
              {p.changes.map((c) => (
                <TR key={c.field}>
                  <TD>{c.field}</TD>
                  <TD className="text-muted-foreground">{c.before}</TD>
                  <TD className="font-medium">{c.after}</TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </div>
      </Section>
      <Section title="Evidence">
        <dl className="space-y-1.5 text-sm">
          {p.evidence.map((e) => (
            <div key={e.label} className="flex justify-between gap-4">
              <dt className="text-muted-foreground">{e.label}</dt>
              <dd className="font-mono tabular-nums">{e.value}</dd>
            </div>
          ))}
        </dl>
      </Section>
      <Section title="Expected impact">
        <p className="text-sm">
          {p.impact.label}{" "}
          <span className="font-mono text-xs text-muted-foreground uppercase">
            {p.impact.kind === "estimate" ? "est." : "measured"}
          </span>
        </p>
      </Section>
      <Section title="Can it be undone">
        <p className="text-sm">
          {p.reversible
            ? "Yes. Adx logs the old values and can put them back in one click."
            : "Only partly. Check before approving."}
        </p>
      </Section>
      <Section title="Where it came from">
        <p className="text-sm">
          {SOURCE_LABEL[p.source]}: {p.requestedBy}, {formatRelative(p.createdAt)}.
        </p>
        {p.decidedBy && (
          <p className="text-sm">
            {p.status === "rejected" ? "Rejected" : "Approved"} by {p.decidedBy}.
          </p>
        )}
        {p.note && <p className="text-sm text-muted-foreground">Note: {p.note}</p>}
      </Section>
      {p.log && p.log.length > 0 && (
        <Section title="History">
          <ol className="space-y-1.5 text-sm">
            {p.log.map((e) => (
              <li key={`${e.at}${e.text}`} className="flex justify-between gap-4">
                <span>{e.text}</span>
                <span className="shrink-0 font-mono text-xs text-muted-foreground">
                  {formatRelative(e.at)}
                </span>
              </li>
            ))}
          </ol>
        </Section>
      )}
    </Sheet>
  );
}

// What can still happen to a decided change: watch it apply, retry a failure,
// or undo it.
function AfterFooter({ proposal: p }: { proposal: Proposal }) {
  const retry = useRetryProposal();
  const undo = useUndoProposal();
  const [confirming, setConfirming] = useState(false);
  const error = retry.error ?? undo.error;
  if (p.status === "applying") {
    return (
      <p className="flex items-center gap-2 text-sm" role="status">
        <span className="size-2 animate-pulse rounded-none bg-foreground" aria-hidden="true" />
        Applying
      </p>
    );
  }
  if (p.status === "failed") {
    return (
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-destructive">{p.failure}</p>
        <Button disabled={retry.isPending} onClick={() => retry.mutate(p.id)}>
          Try again
        </Button>
        {error && <p className="w-full text-sm text-destructive">{error.message}</p>}
      </div>
    );
  }
  if (p.status === "applied" && p.reversible) {
    return (
      <div className="flex flex-wrap items-center justify-end gap-2">
        {confirming ? (
          <>
            <span className="mr-auto text-sm">Put the old values back?</span>
            <Button variant="ghost" onClick={() => setConfirming(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={undo.isPending}
              onClick={() => undo.mutate(p.id)}
            >
              Undo
            </Button>
          </>
        ) : (
          <Button variant="outline" onClick={() => setConfirming(true)}>
            Undo this change
          </Button>
        )}
        {error && <p className="w-full text-sm text-destructive">{error.message}</p>}
      </div>
    );
  }
  return null;
}
