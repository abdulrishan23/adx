import { useState } from "react";
import {
  useDecideRecord,
  useInsights,
  useLearningRecords,
  useLearningReport,
  useObserveOutcome,
  usePromoteInsight,
  useProposeInsight,
  useReviseInsight,
} from "@/api/learning";
import { useSession } from "@/api/session";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// Action-to-outcome history and the insights the team keeps. A record says
// what was advised, what a person decided, and what was observed later on
// the intervention it attaches to. Insights climb only on observed outcomes
// and a controlled comparison; a model agreeing is never evidence.
export function LearningPage() {
  const report = useLearningReport();
  const records = useLearningRecords();
  const insights = useInsights();
  const decide = useDecideRecord();
  const observe = useObserveOutcome();
  const propose = useProposeInsight();
  const promote = usePromoteInsight();
  const revise = useReviseInsight();
  const session = useSession();
  const canEdit = session.data?.role === "admin" || session.data?.role === "marketer";
  const [statement, setStatement] = useState("");
  const [supportIds, setSupportIds] = useState("");
  const [control, setControl] = useState("");
  const [outcome, setOutcome] = useState({ subjectRef: "", metric: "cpl_minor", value: "" });
  const r = report.data;
  const err = [decide, observe, propose, promote, revise].find((m) => m.error)?.error;
  return (
    <section className="space-y-6 text-sm">
      <div>
        <h1 className="text-2xl font-semibold">Learning</h1>
        {r && (
          <p className="text-muted-foreground">
            {r.records} records · {r.interventions} interventions (adopted or modified) ·{" "}
            {r.ignored} ignored · {r.outcomes} outcomes ({r.unattachedOutcomes} unattached) ·
            insights:{" "}
            {Object.entries(r.insights ?? {})
              .map(([k, v]) => `${v} ${k}`)
              .join(", ") || "none"}
          </p>
        )}
        {r && <p className="text-muted-foreground">{r.note}</p>}
      </div>
      <div className="grid gap-6 md:grid-cols-2">
        <div className="space-y-2">
          <h2 className="font-medium">Records</h2>
          {records.data?.records.length === 0 && (
            <p className="text-muted-foreground">
              None yet. Records are opened when a recommendation is made.
            </p>
          )}
          {records.data?.records.map((rec) => (
            <div key={rec.id} className="border-t pt-1">
              <p>
                #{rec.id} · {rec.recommendationRef} · {rec.subjectKind} {rec.subjectRef} ·{" "}
                <span className={rec.decision === "pending" ? "text-destructive" : ""}>
                  {rec.decision}
                </span>
                {rec.actionRef && (
                  <span className="text-muted-foreground"> · action {rec.actionRef}</span>
                )}
                {rec.decisionNote && (
                  <span className="text-muted-foreground"> · {rec.decisionNote}</span>
                )}
              </p>
              {rec.outcomes.map((o) => (
                <p key={o.id} className="text-muted-foreground">
                  outcome · {o.metric} {o.value} {o.unit} · {o.class} ·{" "}
                  {new Date(o.observedAt).toLocaleDateString()}
                </p>
              ))}
              {canEdit && rec.decision === "pending" && (
                <p className="flex flex-wrap gap-1">
                  <Button
                    size="xs"
                    variant="outline"
                    onClick={() =>
                      decide.mutate({
                        id: rec.id,
                        decision: "adopted",
                        note: "",
                        actionRef: `manual:${rec.recommendationRef}`,
                        actionAt: new Date().toISOString(),
                      })
                    }
                  >
                    Adopted
                  </Button>
                  <Button
                    size="xs"
                    variant="outline"
                    onClick={() =>
                      decide.mutate({
                        id: rec.id,
                        decision: "modified",
                        note: "",
                        actionRef: `manual:${rec.recommendationRef}`,
                        actionAt: new Date().toISOString(),
                      })
                    }
                  >
                    Modified
                  </Button>
                  <Button
                    size="xs"
                    variant="outline"
                    onClick={() =>
                      decide.mutate({
                        id: rec.id,
                        decision: "ignored",
                        note: "",
                        actionRef: "",
                        actionAt: new Date(0).toISOString(),
                      })
                    }
                  >
                    Ignored
                  </Button>
                </p>
              )}
            </div>
          ))}
          {canEdit && (
            <div className="flex flex-wrap items-center gap-1 pt-2">
              <Input
                className="h-7 max-w-32"
                placeholder="campaign ref"
                value={outcome.subjectRef}
                onChange={(e) => setOutcome({ ...outcome, subjectRef: e.target.value })}
              />
              <Input
                className="h-7 max-w-28"
                placeholder="metric"
                value={outcome.metric}
                onChange={(e) => setOutcome({ ...outcome, metric: e.target.value })}
              />
              <Input
                className="h-7 max-w-24"
                placeholder="value"
                value={outcome.value}
                onChange={(e) => setOutcome({ ...outcome, value: e.target.value })}
              />
              <Button
                size="xs"
                variant="outline"
                disabled={!outcome.subjectRef || !outcome.value}
                onClick={() =>
                  observe.mutate({
                    subjectKind: "campaign",
                    subjectRef: outcome.subjectRef,
                    metric: outcome.metric,
                    value: Number(outcome.value),
                    unit: "",
                    class: "observed",
                    observedAt: new Date().toISOString(),
                    evidenceId: 0,
                    note: "",
                  })
                }
              >
                Record outcome
              </Button>
            </div>
          )}
        </div>
        <div className="space-y-2">
          <h2 className="font-medium">Insights</h2>
          {insights.data?.insights.length === 0 && <p className="text-muted-foreground">None.</p>}
          {insights.data?.insights.map((i) => (
            <div
              key={i.id}
              className={`border-t pt-1 ${i.retiredAt ? "text-muted-foreground line-through" : ""}`}
            >
              <p>
                L{i.lineage} v{i.version} · {i.class}
                {i.causal
                  ? " · causal (controlled comparison)"
                  : i.attributed
                    ? " · attributed, not causal"
                    : ""}{" "}
                · {i.source} · {i.supportingRecords} supporting record
                {i.supportingRecords === 1 ? "" : "s"}
              </p>
              <p>{i.statement}</p>
              {canEdit && !i.retiredAt && (
                <p className="flex flex-wrap gap-1">
                  {i.class === "hypothesis" && (
                    <Button
                      size="xs"
                      variant="outline"
                      onClick={() =>
                        promote.mutate({ id: i.id, to: "supported", controlEvidenceId: 0 })
                      }
                    >
                      Promote to supported
                    </Button>
                  )}
                  {i.class === "supported" && (
                    <>
                      <Input
                        className="h-7 max-w-32"
                        placeholder="control evidence id"
                        value={control}
                        onChange={(e) => setControl(e.target.value)}
                      />
                      <Button
                        size="xs"
                        variant="outline"
                        disabled={!control}
                        onClick={() =>
                          promote.mutate({
                            id: i.id,
                            to: "validated",
                            controlEvidenceId: Number(control),
                          })
                        }
                      >
                        Validate
                      </Button>
                    </>
                  )}
                  <Button
                    size="xs"
                    variant="outline"
                    onClick={() =>
                      revise.mutate({
                        id: i.id,
                        statement: `${i.statement} (revised)`,
                        scope: i.scope,
                      })
                    }
                  >
                    Revise
                  </Button>
                </p>
              )}
            </div>
          ))}
          {canEdit && (
            <div className="space-y-1 pt-2">
              <Input
                placeholder="new hypothesis"
                value={statement}
                onChange={(e) => setStatement(e.target.value)}
              />
              <div className="flex gap-1">
                <Input
                  className="h-7 max-w-48"
                  placeholder="supporting record ids, comma-separated"
                  value={supportIds}
                  onChange={(e) => setSupportIds(e.target.value)}
                />
                <Button
                  size="xs"
                  variant="outline"
                  disabled={!statement}
                  onClick={() =>
                    propose.mutate(
                      {
                        statement,
                        scope: "",
                        support: supportIds
                          .split(",")
                          .map((s) => Number(s.trim()))
                          .filter((n) => n > 0)
                          .map((n) => ({ kind: "record", recordId: n, note: "" })),
                      },
                      { onSuccess: () => setStatement("") },
                    )
                  }
                >
                  Propose hypothesis
                </Button>
              </div>
            </div>
          )}
        </div>
      </div>
      {err && <p className="text-destructive">{err.message}</p>}
    </section>
  );
}
