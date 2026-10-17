import { useState } from "react";
import { useConfirmReading, useCorrectReading, useEvidence } from "@/api/calls";
import { useLabelCall } from "@/api/evaluation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// A call's evidence: the transcript, what the model read from it and how
// sure it was, what was applied and what was held, corrections, and the
// recording when the policy allows it.
export function CallEvidencePanel({ id, canEdit }: { id: number; canEdit: boolean }) {
  const q = useEvidence(id);
  const confirm = useConfirmReading();
  const correct = useCorrectReading();
  const [outcome, setOutcome] = useState("callback");
  const [note, setNote] = useState("");
  const label = useLabelCall();
  const [labelOutcome, setLabelOutcome] = useState("interested");
  if (!q.data) return null;
  const ev = q.data;
  const s = ev.session;
  return (
    <div className="space-y-2 pl-4 text-muted-foreground">
      {!s && <p>No conversation ran on this call.</p>}
      {s && (
        <>
          <p className="text-foreground">
            Conversation {s.state}
            {s.endedBy && ` · ended by ${s.endedBy}`} · {s.interruptions} interruption
            {s.interruptions === 1 ? "" : "s"} · reading {s.reviewStatus}
            {s.confidence && ` (${s.confidence} confidence)`}
            {s.outcome && ` · ${s.outcome}`}
          </p>
          {s.reviewReason && <p>{s.reviewReason}</p>}
          {s.summary && <p className="text-foreground">{s.summary}</p>}
          {Object.entries(s.answers ?? {}).map(([k, a]) => (
            <p key={k} className={a.uncertain ? "text-destructive" : ""}>
              {k}: {a.value}
              {a.uncertain ? " · uncertain, not recorded" : ""}
              {a.quote ? ` · "${a.quote}"` : ""}
            </p>
          ))}
          {s.appliedVersion > 0 && (
            <p>
              Applied v{s.appliedVersion}
              {s.applied.stage ? ` · stage ${s.applied.stage}` : ""}
              {s.applied.attributes.length > 0
                ? ` · recorded ${s.applied.attributes.join(", ")}`
                : ""}
              {s.applied.held.length > 0 ? ` · held: ${s.applied.held.join("; ")}` : ""}
              {s.applied.note ? ` · ${s.applied.note}` : ""}
            </p>
          )}
          <details>
            <summary>Transcript ({s.transcript.length} lines)</summary>
            {s.transcript.map((seg, i) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: transcript lines have no id
              <p key={i}>
                <span className="font-medium">{seg.speaker}:</span> {seg.text}
              </p>
            ))}
          </details>
          {canEdit && (
            <div className="flex flex-wrap items-center gap-1">
              {s.reviewStatus === "pending" && s.outcome && (
                <Button size="xs" variant="outline" onClick={() => confirm.mutate(id)}>
                  Confirm reading
                </Button>
              )}
              <select
                className="h-7 rounded-md border bg-background px-1"
                value={outcome}
                onChange={(e) => setOutcome(e.target.value)}
              >
                {[
                  "interested",
                  "not_interested",
                  "callback",
                  "transfer",
                  "no_answer",
                  "voicemail",
                  "wrong_number",
                  "unclear",
                ].map((o) => (
                  <option key={o}>{o}</option>
                ))}
              </select>
              <Input
                className="h-7 max-w-64"
                placeholder="why the reading was wrong"
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
              <Button
                size="xs"
                variant="outline"
                disabled={!note}
                onClick={() =>
                  correct.mutate(
                    { id, outcome, answers: {}, note },
                    { onSuccess: () => setNote("") },
                  )
                }
              >
                Correct
              </Button>
            </div>
          )}
          {canEdit && (
            <div className="flex flex-wrap items-center gap-1">
              <span>Agreed label (for evaluation only, changes nothing):</span>
              <select
                className="h-7 rounded-md border bg-background px-1"
                value={labelOutcome}
                onChange={(e) => setLabelOutcome(e.target.value)}
              >
                {[
                  "interested",
                  "not_interested",
                  "callback",
                  "transfer",
                  "no_answer",
                  "voicemail",
                  "wrong_number",
                  "unclear",
                ].map((o) => (
                  <option key={o}>{o}</option>
                ))}
              </select>
              <Button
                size="xs"
                variant="outline"
                onClick={() =>
                  label.mutate({
                    callId: id,
                    outcome: labelOutcome,
                    answers: {},
                    note: "labelled from the call page",
                  })
                }
              >
                Label
              </Button>
              {label.data && <span>Labelled {label.data.outcome}.</span>}
              {label.error && <span className="text-destructive">{label.error.message}</span>}
            </div>
          )}
          {ev.corrections.map((c) => (
            <p key={c.id}>
              correction {new Date(c.createdAt).toLocaleString()} · {c.outcome} · {c.note}
            </p>
          ))}
        </>
      )}
      <p>Recording: {ev.recordingAccess ? ev.recordingRef : ev.recordingReason}</p>
      {(confirm.error || correct.error) && (
        <p className="text-destructive">{(confirm.error ?? correct.error)?.message}</p>
      )}
    </div>
  );
}
