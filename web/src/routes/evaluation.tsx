import { useEvaluation } from "@/api/evaluation";

// How the lead loop is doing, measured against what people said happened.
// A rate with nothing to divide by is "Not available", never 0, and the
// predictive gate stays shut until the evidence is there.
export function EvaluationPage() {
  const q = useEvaluation();
  if (q.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (q.error) return <p className="text-destructive">{q.error.message}</p>;
  const r = q.data;
  const pct = (v: number | null) => (v === null ? "Not available" : `${Math.round(v * 100)}%`);
  return (
    <section className="space-y-6 text-sm">
      <div>
        <h1 className="text-2xl font-semibold">Evaluation</h1>
        <p className="text-muted-foreground">
          Generated {new Date(r.generatedAt).toLocaleString()}.
        </p>
      </div>
      <div
        className={`rounded-lg border p-3 ${r.predictive.enabled ? "" : "border-destructive/40"}`}
      >
        <h2 className="font-medium">
          Predictive claims: {r.predictive.enabled ? "enabled" : "disabled"}
        </h2>
        <p className="text-muted-foreground">
          {r.predictive.reason}. Requirement: {r.predictive.requirements.minLabels} labelled calls
          with agreement of at least {Math.round(r.predictive.requirements.minAgreement * 100)}%.
        </p>
      </div>
      <div className="grid gap-6 md:grid-cols-3">
        <div className="space-y-1">
          <h2 className="font-medium">Qualification</h2>
          <p>Conversations: {r.qualification.sessions}</p>
          <p>Read by the model: {r.qualification.read}</p>
          <p>Pending review: {r.qualification.pending}</p>
          <p>Labelled by the team: {r.qualification.labeled}</p>
          <p>
            Agreement with labels: {pct(r.qualification.agreementRate)}
            {r.qualification.labeled > 0 &&
              ` (${r.qualification.agreements} of ${r.qualification.labeled})`}
          </p>
          <p>
            Corrections: {r.qualification.corrections} · rate {pct(r.qualification.correctionRate)}
          </p>
          {r.qualification.note && <p className="text-muted-foreground">{r.qualification.note}</p>}
        </div>
        <div className="space-y-1">
          <h2 className="font-medium">Alerts</h2>
          <p>Total: {r.alerts.total}</p>
          <p>Open: {r.alerts.open}</p>
          <p>Acknowledged: {r.alerts.acknowledged}</p>
          <p>
            Resolved: {r.alerts.resolved} · rate {pct(r.alerts.resolvedRate)}
          </p>
          <p>Raised in the last 7 days: {r.alerts.raisedLast7Days}</p>
        </div>
        <div className="space-y-1">
          <h2 className="font-medium">Observed feedback</h2>
          <p>Objections recorded: {r.feedback.objections}</p>
          <p>Active opt-outs: {r.feedback.activeSuppressions}</p>
          <p>Drafts held or suppressed: {r.feedback.draftsHeld}</p>
          <p>Drafts sent: {r.feedback.draftsSent}</p>
          <p>CRM field conflicts: {r.feedback.fieldConflicts}</p>
          <p>Signal corrections: {r.feedback.signalCorrections}</p>
        </div>
      </div>
      <div>
        <h2 className="font-medium">Labels</h2>
        {r.labels.length === 0 && (
          <p className="text-muted-foreground">
            No calls labelled yet. Label a call from its evidence panel.
          </p>
        )}
        {r.labels.map((l) => (
          <p key={l.id} className="text-muted-foreground">
            call {l.callId} · {l.outcome} · {new Date(l.labeledAt).toLocaleDateString()}
            {l.note ? ` · ${l.note}` : ""}
          </p>
        ))}
      </div>
    </section>
  );
}
