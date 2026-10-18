import { getRouteApi, Link } from "@tanstack/react-router";
import { Check, X } from "lucide-react";
import { useState } from "react";
import { useDecideProposal, useProposal } from "@/api/adx";
import type { Proposal } from "@/api/models";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows } from "@/components/page";
import { PlatformTag } from "@/components/platform";
import { Button } from "@/components/ui/button";
import { StatusChip } from "@/components/ui/status-chip";
import { formatRelative } from "@/lib/format";

const route = getRouteApi("/public/approve/$proposalId");

// The page an approval email links to (D-14). The CMO won't open a dashboard,
// so this shows one change, plainly, with two buttons that work on a phone.
export function ApprovePage() {
  const { proposalId } = route.useParams();
  const proposal = useProposal(proposalId);
  return (
    <div className="mx-auto max-w-xl">
      {proposal.isPending && <LoadingRows rows={6} />}
      {proposal.error && <ErrorNote error={proposal.error} />}
      {proposal.data === null && (
        <EmptyState title="This link doesn't match a change">
          It may be mistyped, or the proposal was withdrawn.
        </EmptyState>
      )}
      {proposal.data && <Decision proposal={proposal.data} />}
    </div>
  );
}

function Decision({ proposal: p }: { proposal: Proposal }) {
  const decide = useDecideProposal();
  const [rejecting, setRejecting] = useState(false);
  const [note, setNote] = useState("");
  const expired = p.status === "expired" || Date.parse(p.expiresAt) < Date.now();
  const decided = p.status !== "pending" && p.status !== "expired";

  return (
    <article className="space-y-6">
      <header className="space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <PlatformTag platform={p.platform} />
          <span className="text-sm text-muted-foreground">
            From {p.requestedBy} · {formatRelative(p.createdAt)}
          </span>
        </div>
        <h1 className="text-2xl leading-tight font-semibold tracking-tight">{p.title}</h1>
        <p className="text-muted-foreground">{p.target}</p>
      </header>

      <section className="space-y-2">
        <Eyebrow>Why</Eyebrow>
        <p className="leading-relaxed">{p.problem}</p>
      </section>

      <section className="space-y-2">
        <Eyebrow>What changes</Eyebrow>
        <ul className="divide-y rounded-lg border bg-card">
          {p.changes.map((c) => (
            <li key={c.field} className="space-y-1 px-4 py-3">
              <p className="text-sm text-muted-foreground">{c.field}</p>
              <p className="flex flex-wrap items-baseline gap-2">
                <span className="text-muted-foreground line-through decoration-foreground/30">
                  {c.before}
                </span>
                <span aria-hidden="true">→</span>
                <span className="font-medium">{c.after}</span>
              </p>
            </li>
          ))}
        </ul>
        <p className="text-sm text-muted-foreground">
          {p.impact.kind === "estimate" ? "Estimate: " : "Measured: "}
          {p.impact.label}.{" "}
          {p.reversible ? "Can be undone from Adx." : "Can't be undone automatically."}
        </p>
      </section>

      {p.evidence.length > 0 && (
        <section className="space-y-2">
          <Eyebrow>Evidence</Eyebrow>
          <dl className="grid grid-cols-2 gap-3">
            {p.evidence.map((e) => (
              <div key={e.label} className="rounded-lg border bg-card p-3">
                <dt className="text-xs text-muted-foreground">{e.label}</dt>
                <dd className="mt-1 font-mono font-semibold tabular-nums">{e.value}</dd>
              </div>
            ))}
          </dl>
        </section>
      )}

      <footer className="sticky bottom-0 -mx-6 border-t bg-background px-6 py-4">
        {decided ? (
          <div className="flex items-center gap-3">
            <StatusChip status={p.status === "rejected" ? "pending" : "ready"}>
              {p.status === "rejected" ? "Rejected" : "Approved"}
            </StatusChip>
            <p className="text-sm text-muted-foreground">
              {p.decidedBy ? `By ${p.decidedBy}. ` : ""}
              {p.status === "rejected"
                ? "Nothing will change."
                : "Adx applies it and logs the result."}
            </p>
          </div>
        ) : expired ? (
          <p className="text-sm text-muted-foreground">Expired.</p>
        ) : rejecting ? (
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              decide.mutate({ id: p.id, decision: "reject", note: note || undefined });
            }}
          >
            <label className="block text-sm" htmlFor="reject-note">
              Why not?{" "}
              <span className="text-muted-foreground">(optional, goes to {p.requestedBy})</span>
            </label>
            <textarea
              id="reject-note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              rows={2}
              className="w-full rounded-md border bg-card px-3 py-2 text-sm outline-none focus:border-ring"
            />
            <div className="flex gap-2">
              <Button
                type="submit"
                variant="destructive"
                className="flex-1"
                disabled={decide.isPending}
              >
                Reject
              </Button>
              <Button type="button" variant="ghost" onClick={() => setRejecting(false)}>
                Back
              </Button>
            </div>
          </form>
        ) : (
          <div className="space-y-2">
            <div className="flex gap-2">
              <Button
                className="h-11 flex-1 text-base"
                disabled={decide.isPending}
                onClick={() => decide.mutate({ id: p.id, decision: "approve" })}
              >
                <Check /> Approve
              </Button>
              <Button
                variant="outline"
                className="h-11 flex-1 text-base"
                disabled={decide.isPending}
                onClick={() => setRejecting(true)}
              >
                <X /> Reject
              </Button>
            </div>
            <p className="text-center text-xs text-muted-foreground">
              Expires {formatRelative(p.expiresAt)}.{" "}
              <Link
                to="/proposals"
                search={{ open: p.id }}
                className="underline underline-offset-4"
              >
                Open in Adx
              </Link>
            </p>
          </div>
        )}
        {decide.error && (
          <div className="mt-3">
            <ErrorNote error={decide.error} />
          </div>
        )}
      </footer>
    </article>
  );
}
