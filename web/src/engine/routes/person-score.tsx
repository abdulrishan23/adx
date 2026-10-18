import { Button } from "@/components/ui/button";
import { useCorrectEvent, usePersonScore } from "@/engine/api/signals";

// A person's score with its explanation, the events behind it and the
// corrections made. A missing figure says why; it is never shown as 0.
export function ScoreSection({ id, canEdit }: { id: number; canEdit: boolean }) {
  const q = usePersonScore(id);
  const correct = useCorrectEvent();
  if (q.isPending) return <p className="text-muted-foreground">Loading score…</p>;
  if (q.error) return <p className="text-destructive">{q.error.message}</p>;
  const v = q.data;
  const latest = v.latest;
  return (
    <section className="space-y-2">
      <h3 className="font-semibold">Fit, intent and temperature</h3>
      {!latest && <p className="text-muted-foreground">Not assessed yet: no signals recorded.</p>}
      {latest && (
        <div className="space-y-1">
          <p>
            Fit:{" "}
            {latest.fit === null ? (
              <span className="text-muted-foreground">Not available</span>
            ) : (
              latest.fit
            )}{" "}
            · Intent:{" "}
            {latest.intent === null ? (
              <span className="text-muted-foreground">Not available</span>
            ) : (
              latest.intent
            )}{" "}
            · Temperature:{" "}
            {latest.temperature ?? <span className="text-muted-foreground">Not available</span>}
            <span className="text-muted-foreground">
              {" "}
              · rules v{latest.rulesVersion ?? "none"} · {latest.trigger} ·{" "}
              {new Date(latest.assessedAt).toLocaleString()}
            </span>
          </p>
          {latest.explanation.unconfigured.length > 0 && (
            <p className="text-muted-foreground">
              Unconfigured: {latest.explanation.unconfigured.join(", ")}.
            </p>
          )}
          {!latest.explanation.fitAvailable && latest.rulesVersion && (
            <p className="text-muted-foreground">
              Fit is not available: none of the fit attributes are known for this person.
            </p>
          )}
          {latest.explanation.fit.map((f) => (
            <p key={f.attribute} className="text-muted-foreground">
              fit · {f.attribute} = {f.value} ({f.class}) → {f.points}
            </p>
          ))}
          {latest.explanation.intent.map((i) => (
            <p key={i.kind} className="text-muted-foreground">
              intent · {i.kind} × {i.events} → {i.points} (raw {i.raw}, cap {i.cap})
            </p>
          ))}
          {latest.explanation.excluded.map((x) => (
            <p key={x.eventId} className="text-muted-foreground">
              excluded · event {x.eventId}: {x.reason}
            </p>
          ))}
        </div>
      )}
      <h4 className="pt-2 font-medium">Timeline</h4>
      {v.events.length === 0 && <p className="text-muted-foreground">No events.</p>}
      {v.events.map((e) => (
        <p key={e.id} className={e.discardedAt ? "text-muted-foreground line-through" : ""}>
          {new Date(e.occurredAt).toLocaleString()} · {e.kind} · {e.source} · {e.confidence}
          {e.discardedAt && ` · discarded: ${e.discardReason}`}
          {canEdit && !e.discardedAt && (
            <Button
              size="xs"
              variant="outline"
              className="ml-2"
              onClick={() =>
                correct.mutate({
                  id: e.id,
                  action: "discard",
                  personId: 0,
                  reason: "not a real signal",
                })
              }
            >
              Discard
            </Button>
          )}
        </p>
      ))}
      {correct.error && <p className="text-destructive">{correct.error.message}</p>}
      {v.history.length > 1 && (
        <>
          <h4 className="pt-2 font-medium">History</h4>
          {v.history.map((h) => (
            <p key={h.id} className="text-muted-foreground">
              {new Date(h.assessedAt).toLocaleString()} · fit {h.fit ?? "n/a"} · intent{" "}
              {h.intent ?? "n/a"} · {h.temperature ?? "n/a"} · v{h.rulesVersion ?? "none"} ·{" "}
              {h.trigger}
            </p>
          ))}
        </>
      )}
      {v.corrections.length > 0 && (
        <>
          <h4 className="pt-2 font-medium">SignalCorrections</h4>
          {v.corrections.map((c) => (
            <p key={c.id} className="text-muted-foreground">
              {new Date(c.createdAt).toLocaleString()} · event {c.eventId} · {c.action}
              {c.toPersonId ? ` → person ${c.toPersonId}` : ""} · {c.reason}
            </p>
          ))}
        </>
      )}
    </section>
  );
}
