import { useState } from "react";
import { useRegistry } from "@/api/registry";
import { useSession } from "@/api/session";
import {
  useCorrectTask,
  useRetryTask,
  useStartWorkflow,
  useTickWorkflows,
  useWorkflowGraph,
  useWorkflowRuns,
  useWorkflowTransition,
} from "@/api/workflows";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// Workflow runs and their task graphs. A blocked task says what it waits
// for (an approval, a budget, a person's decision after an upstream
// change); an external step's action is recorded on the task and never
// repeated by a retry.
export function WorkflowsPage() {
  const runs = useWorkflowRuns();
  const defs = useRegistry("workflow");
  const start = useStartWorkflow();
  const tick = useTickWorkflows();
  const session = useSession();
  const canEdit = session.data?.role === "admin" || session.data?.role === "marketer";
  const [open, setOpen] = useState<number | null>(null);
  const [definition, setDefinition] = useState("");
  const active = defs.data?.entries.filter((e) => e.state === "active") ?? [];
  return (
    <section className="space-y-6 text-sm">
      <div className="flex items-baseline justify-between">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Workflows</h1>
          <p className="text-muted-foreground">
            Durable task graphs over active definitions. Changes to the outside are proposals;
            nothing here executes one.
          </p>
        </div>
        {canEdit && (
          <Button size="xs" variant="outline" onClick={() => tick.mutate()}>
            Tick now
          </Button>
        )}
      </div>
      {canEdit && (
        <div className="flex flex-wrap items-center gap-1">
          <select
            className="h-8 rounded-md border bg-background px-1"
            value={definition}
            onChange={(e) => setDefinition(e.target.value)}
          >
            <option value="">pick an active definition</option>
            {active.map((e) => (
              <option key={e.id} value={e.key}>
                {e.key} v{e.version}
              </option>
            ))}
          </select>
          <Button
            size="sm"
            disabled={!definition || start.isPending}
            onClick={() => start.mutate({ definition, params: "{}", budgetTokens: 0 })}
          >
            Start run
          </Button>
          {start.error && <span className="text-destructive">{start.error.message}</span>}
        </div>
      )}
      {runs.data?.runs.length === 0 && <p className="text-muted-foreground">No runs.</p>}
      {runs.data?.runs.map((r) => (
        <div key={r.id} className="border-t pt-1">
          <p>
            #{r.id} · {r.definitionKey} v{r.definitionVersion} ·{" "}
            <span
              className={r.state === "failed" || r.state === "blocked" ? "text-destructive" : ""}
            >
              {r.state}
            </span>
            {r.reason && <span className="text-muted-foreground"> · {r.reason}</span>}
            <span className="text-muted-foreground">
              {" "}
              · {r.spentTokens}
              {r.budgetTokens > 0 ? ` / ${r.budgetTokens}` : ""} tokens
            </span>
            <Button
              size="xs"
              variant="outline"
              className="ml-2"
              onClick={() => setOpen(open === r.id ? null : r.id)}
            >
              {open === r.id ? "Hide" : "Graph"}
            </Button>
          </p>
          {open === r.id && <GraphPanel id={r.id} canEdit={canEdit} />}
        </div>
      ))}
    </section>
  );
}

function GraphPanel({ id, canEdit }: { id: number; canEdit: boolean }) {
  const q = useWorkflowGraph(id);
  const transition = useWorkflowTransition();
  const retry = useRetryTask();
  const correct = useCorrectTask();
  const [fix, setFix] = useState<{ id: number; output: string; reason: string } | null>(null);
  if (!q.data) return null;
  const g = q.data;
  const err = [transition, retry, correct].find((m) => m.error)?.error;
  return (
    <div className="space-y-2 pl-4">
      {canEdit && (
        <p className="flex gap-1">
          {(g.run.state === "running" || g.run.state === "blocked") && (
            <Button
              size="xs"
              variant="outline"
              onClick={() =>
                transition.mutate({ id, kind: "pause", reason: "paused from the page" })
              }
            >
              Pause
            </Button>
          )}
          {(g.run.state === "paused" || g.run.state === "blocked") && (
            <Button
              size="xs"
              variant="outline"
              onClick={() => transition.mutate({ id, kind: "resume", reason: "" })}
            >
              Resume
            </Button>
          )}
          {g.run.state !== "done" && g.run.state !== "failed" && g.run.state !== "canceled" && (
            <Button
              size="xs"
              variant="outline"
              onClick={() =>
                transition.mutate({ id, kind: "cancel", reason: "canceled from the page" })
              }
            >
              Cancel
            </Button>
          )}
        </p>
      )}
      {g.tasks.map((t) => (
        <div
          key={t.id}
          className={`border-t pt-1 ${t.state === "failed" || t.state === "blocked" ? "text-destructive" : t.state === "done" ? "" : "text-muted-foreground"}`}
        >
          <p>
            {t.key}
            {t.iteration > 0 ? `#${t.iteration}` : ""} · {t.skill || t.agent} · {t.state} · attempt{" "}
            {t.attempt}/{t.maxAttempts}
            {t.external && ` · external${t.externalRef ? ` (${t.externalRef})` : ""}`}
            {t.dependsOn.length > 0 && (
              <span className="text-muted-foreground"> · after {t.dependsOn.join(", ")}</span>
            )}
            {t.reason && <span className="text-muted-foreground"> · {t.reason}</span>}
            {canEdit &&
              (t.state === "failed" || t.state === "blocked") &&
              !(t.external && t.externalRef) && (
                <Button
                  size="xs"
                  variant="outline"
                  className="ml-2"
                  onClick={() => retry.mutate(t.id)}
                >
                  Retry
                </Button>
              )}
            {canEdit && t.state === "done" && (
              <Button
                size="xs"
                variant="outline"
                className="ml-2"
                onClick={() => setFix({ id: t.id, output: t.output, reason: "" })}
              >
                Correct output
              </Button>
            )}
          </p>
          {t.state === "done" && (
            <pre className="max-h-32 overflow-auto whitespace-pre-wrap text-xs text-muted-foreground">
              {t.output}
            </pre>
          )}
          {fix?.id === t.id && (
            <div className="space-y-1">
              <textarea
                className="h-24 w-full rounded-md border bg-background p-1 font-mono text-xs"
                value={fix.output}
                onChange={(e) => setFix({ ...fix, output: e.target.value })}
              />
              <div className="flex gap-1">
                <Input
                  className="h-7 max-w-64"
                  placeholder="why"
                  value={fix.reason}
                  onChange={(e) => setFix({ ...fix, reason: e.target.value })}
                />
                <Button
                  size="xs"
                  disabled={!fix.reason}
                  onClick={() => {
                    try {
                      correct.mutate(
                        { id: t.id, output: JSON.parse(fix.output), reason: fix.reason },
                        { onSuccess: () => setFix(null) },
                      );
                    } catch {
                      /* invalid JSON stays in the editor */
                    }
                  }}
                >
                  Save correction
                </Button>
                <Button size="xs" variant="outline" onClick={() => setFix(null)}>
                  Cancel
                </Button>
              </div>
            </div>
          )}
        </div>
      ))}
      <details>
        <summary className="text-muted-foreground">{g.events.length} events</summary>
        {g.events.map((e) => (
          <p key={e.id} className="text-muted-foreground">
            {new Date(e.at).toLocaleTimeString()} · {e.kind}
            {e.taskId !== null ? ` · task ${e.taskId}` : ""} · {e.detail}
          </p>
        ))}
      </details>
      {err && <p className="text-destructive">{err.message}</p>}
    </div>
  );
}
