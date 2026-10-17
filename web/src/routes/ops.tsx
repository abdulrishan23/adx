import { useState } from "react";
import {
  useCost,
  useDecideRelease,
  useEvaluateRelease,
  useLoad,
  useReadiness,
  useReleases,
} from "@/api/ops";
import { useSession } from "@/api/session";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// Operations: is the server ready, what has the model cost today and how
// close the limits are, which model release is approved, and what load was
// measured under which conditions. Nothing here is estimated: a figure with
// no measurement behind it says so.
export function OpsPage() {
  const session = useSession();
  const isAdmin = session.data?.role === "admin";
  const canEvaluate = isAdmin || session.data?.role === "marketer";
  return (
    <section className="space-y-8 text-sm">
      <h1 className="text-2xl font-semibold">Operations</h1>
      <Readiness />
      <Cost />
      <Releases canEvaluate={canEvaluate} isAdmin={isAdmin} />
      <Load />
    </section>
  );
}

function Readiness() {
  const ready = useReadiness();
  if (ready.isPending) return <p className="text-muted-foreground">Checking server…</p>;
  if (ready.error) return <p className="text-destructive">Server unreachable</p>;
  return (
    <div className="space-y-1">
      <h2 className="font-medium">Readiness: {ready.data.ready ? "ready" : "not ready"}</h2>
      <ul className="flex flex-wrap gap-3">
        {ready.data.checks.map((c) => (
          <li key={c.name} className={c.ok ? "text-muted-foreground" : "text-destructive"}>
            {c.name}: {c.ok ? "ok" : "failed"}
          </li>
        ))}
      </ul>
    </div>
  );
}

function tokens(n: number) {
  return n.toLocaleString();
}

function Cost() {
  const cost = useCost();
  if (cost.isPending) return <p className="text-muted-foreground">Loading cost…</p>;
  if (cost.error) return <p className="text-destructive">{cost.error.message}</p>;
  const c = cost.data;
  const money = (micros: number | null) =>
    micros === null
      ? "Not available (no price configured)"
      : `${(micros / 1_000_000).toFixed(2)} USD`;
  return (
    <div className="grid gap-6 md:grid-cols-3">
      <div className="space-y-1">
        <h2 className="font-medium">This workspace, last 24 hours</h2>
        <p>
          Assistant tokens: {tokens(c.workspace.inputTokens)} in, {tokens(c.workspace.outputTokens)}{" "}
          out
        </p>
        <p>Cost: {money(c.workspace.costMicros)}</p>
        <p>
          Budget: {c.workspace.budget > 0 ? tokens(c.workspace.budget) : "none"}
          {c.workspace.remaining !== null && ` · remaining ${tokens(c.workspace.remaining)}`}
          {c.workspace.exhausted && <span className="text-destructive"> · exhausted</span>}
        </p>
        <p>
          Workflow tokens spent: {tokens(c.workflows.spentTokens)} · runs blocked on budget:{" "}
          {c.workflows.blockedOnBudget} · default run budget {tokens(c.workflows.defaultBudget)}
        </p>
      </div>
      <div className="space-y-1">
        <h2 className="font-medium">Server, UTC day {c.server.day}</h2>
        <p>Model: {c.running || "none configured"}</p>
        <p>
          Tokens: {tokens(c.server.inputTokens)} in, {tokens(c.server.outputTokens)} out ·{" "}
          {c.server.calls} calls · {c.server.refused} refused
        </p>
        <p>
          Daily cap: {c.server.cap > 0 ? tokens(c.server.cap) : "none"}
          {c.server.remaining !== null && ` · remaining ${tokens(c.server.remaining)}`}
          {c.server.exhausted && (
            <span className="text-destructive"> · reached, calls refused</span>
          )}
        </p>
      </div>
      <div className="space-y-1">
        <h2 className="font-medium">Limits in force</h2>
        <p>Assistant per workspace per day: {tokens(c.budgets.assistantDailyTokens)} tokens</p>
        <p>
          Server per day:{" "}
          {c.budgets.modelDailyTokenCap > 0 ? tokens(c.budgets.modelDailyTokenCap) : "no cap"}
        </p>
        <p>Workflow run: {tokens(c.budgets.workflowDefaultBudget)} tokens</p>
        <p className="text-muted-foreground">
          {c.pricing
            ? `Prices: ${c.budgets.inputPriceMicros / 1_000_000} / ${c.budgets.outputPriceMicros / 1_000_000} USD per million tokens`
            : "No token prices configured, so no money figures are shown."}
        </p>
      </div>
    </div>
  );
}

function Releases({ canEvaluate, isAdmin }: { canEvaluate: boolean; isAdmin: boolean }) {
  const releases = useReleases();
  const evaluate = useEvaluateRelease();
  const decide = useDecideRelease();
  const [model, setModel] = useState("");
  if (releases.isPending) return <p className="text-muted-foreground">Loading releases…</p>;
  if (releases.error) return <p className="text-destructive">{releases.error.message}</p>;
  const r = releases.data;
  const pct = (v: number | null) => (v === null ? "Not available" : `${Math.round(v * 100)}%`);
  return (
    <div className="space-y-3">
      <h2 className="font-medium">Model releases</h2>
      <p className="text-muted-foreground">
        A candidate is replayed over the team's labelled calls with the live reading prompt. It is
        promoted only by an admin, only with at least {r.requirements.minLabels} labelled calls and{" "}
        {Math.round(r.requirements.minAgreement * 100)}% agreement, and never below the approved
        release.
      </p>
      {r.warning && <p className="text-destructive">{r.warning}</p>}
      <p>
        Running: <strong>{r.running || "none"}</strong> · Approved:{" "}
        <strong>{r.approved?.model ?? "none"}</strong>
      </p>
      {r.releases.length > 0 && (
        <ul className="divide-y rounded border">
          {r.releases.map((rel) => (
            <li
              key={rel.model}
              className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
            >
              <span>
                <span className="font-medium">{rel.model}</span>{" "}
                <span className="text-muted-foreground">
                  · {rel.state} · agreement {pct(rel.agreement)} on {rel.labels} labelled calls ·{" "}
                  {rel.note}
                </span>
              </span>
              {isAdmin && rel.state === "candidate" && (
                <span className="flex gap-2">
                  <Button
                    size="sm"
                    disabled={decide.isPending}
                    onClick={() => decide.mutate({ model: rel.model, decision: "promote" })}
                  >
                    Promote
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={decide.isPending}
                    onClick={() => decide.mutate({ model: rel.model, decision: "reject" })}
                  >
                    Reject
                  </Button>
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
      {canEvaluate && (
        <form
          className="flex items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            evaluate.mutate(model, { onSuccess: () => setModel("") });
          }}
        >
          <label className="text-sm" htmlFor="release-model">
            Candidate model
            <Input
              id="release-model"
              value={model}
              onChange={(e) => setModel(e.target.value)}
              placeholder="claude-sonnet-5-5"
            />
          </label>
          <Button type="submit" disabled={evaluate.isPending || !model}>
            Evaluate
          </Button>
        </form>
      )}
      {(evaluate.error || decide.error) && (
        <p className="text-destructive">{(evaluate.error ?? decide.error)?.message}</p>
      )}
    </div>
  );
}

function Load() {
  const load = useLoad();
  if (load.isPending) return <p className="text-muted-foreground">Loading measurements…</p>;
  if (load.error) return <p className="text-destructive">{load.error.message}</p>;
  const m = load.data.measurement;
  return (
    <div className="space-y-1">
      <h2 className="font-medium">Load measurement</h2>
      {!m && <p className="text-muted-foreground">{load.data.note}</p>}
      {m && (
        <>
          <p>
            {Math.round(m.opsPerSecond)} operations/second · p50 {m.p50Ms.toFixed(1)} ms · p95{" "}
            {m.p95Ms.toFixed(1)} ms · max {m.maxMs.toFixed(1)} ms · {m.errors} errors
          </p>
          <p className="text-muted-foreground">
            Measured {new Date(m.ranAt).toLocaleString()} on {m.host} ({m.cpus} CPUs, {m.goVersion})
            with {m.concurrency} workers for {(m.durationMs / 1000).toFixed(1)} s. Mix:{" "}
            {Object.entries(m.mix ?? {})
              .map(([k, v]) => `${k} ${v}`)
              .join(", ")}
            . {m.notes}
          </p>
        </>
      )}
    </div>
  );
}
