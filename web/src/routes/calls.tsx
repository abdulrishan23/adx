import { useEffect, useState } from "react";
import {
  useCall,
  useCallPolicy,
  useCalls,
  useHangup,
  usePutCallPolicy,
  useReconcile,
} from "@/api/calls";
import { useSession } from "@/api/session";
import type { CallPolicyDocument } from "@/api/types.gen";
import { Button } from "@/components/ui/button";

// Calls: the policy that allows calling at all, whether a live call can be
// placed on this server and why not, and every call with its state, its
// reason and its events. An unknown result must be reconciled before another
// attempt.
export function CallsPage() {
  const policy = useCallPolicy();
  const put = usePutCallPolicy();
  const list = useCalls();
  const hangup = useHangup();
  const reconcile = useReconcile();
  const session = useSession();
  const isAdmin = session.data?.role === "admin";
  const canEdit = isAdmin || session.data?.role === "marketer";
  const [text, setText] = useState("");
  const [open, setOpen] = useState<number | null>(null);
  useEffect(() => {
    if (policy.data)
      setText(JSON.stringify(policy.data.current?.document ?? policy.data.suggested, null, 2));
  }, [policy.data]);
  function save() {
    try {
      put.mutate(JSON.parse(text) as CallPolicyDocument);
    } catch {
      /* invalid JSON stays in the editor */
    }
  }
  const st = policy.data;
  return (
    <section className="space-y-6 text-sm">
      <div>
        <h1 className="text-2xl font-semibold">Calls</h1>
        {st && (
          <p className={st.liveCalling ? "text-muted-foreground" : "text-destructive"}>
            {st.liveCalling
              ? `Live calling is possible through ${st.adapter}.`
              : "Live calling is off."}{" "}
            {st.reasons.length > 0 && st.reasons.join("; ")}
          </p>
        )}
      </div>
      <div className="grid gap-6 md:grid-cols-2">
        <div className="space-y-2">
          <h2 className="font-medium">Calling policy (JSON)</h2>
          <textarea
            className="h-72 w-full rounded-md border bg-background p-2 font-mono text-xs"
            value={text}
            onChange={(e) => setText(e.target.value)}
            readOnly={!isAdmin}
          />
          {isAdmin && (
            <Button size="sm" onClick={save} disabled={put.isPending}>
              Save as new version
            </Button>
          )}
          {put.error && <p className="text-destructive">{put.error.message}</p>}
        </div>
        <div className="space-y-2">
          <h2 className="font-medium">Calls</h2>
          {list.data?.calls.length === 0 && <p className="text-muted-foreground">No calls.</p>}
          {list.data?.calls.map((c) => (
            <div
              key={c.id}
              className={`border-t pt-1 ${c.state === "unknown" ? "text-destructive" : ""}`}
            >
              <p>
                #{c.id} · {c.mode} · person {c.personId} · attempt {c.attempt} · {c.state}
                {c.reason && <span className="text-muted-foreground"> · {c.reason}</span>}
                {c.durationSeconds !== null && (
                  <span className="text-muted-foreground"> · {c.durationSeconds}s</span>
                )}
                {c.recordingRef && (
                  <span className="text-muted-foreground">
                    {" "}
                    · recording {c.recordingDeletedAt ? "deleted" : "kept"}
                  </span>
                )}
                <Button
                  size="xs"
                  variant="outline"
                  className="ml-2"
                  onClick={() => setOpen(open === c.id ? null : c.id)}
                >
                  {open === c.id ? "Hide" : "Events"}
                </Button>
                {canEdit && c.state === "unknown" && (
                  <Button
                    size="xs"
                    variant="outline"
                    className="ml-1"
                    onClick={() => reconcile.mutate(c.id)}
                  >
                    Reconcile
                  </Button>
                )}
                {canEdit &&
                  (c.state === "queued" || c.state === "ringing" || c.state === "in_progress") && (
                    <Button
                      size="xs"
                      variant="outline"
                      className="ml-1"
                      onClick={() => hangup.mutate({ id: c.id, reason: "ended from ADX" })}
                    >
                      Hang up
                    </Button>
                  )}
              </p>
              {open === c.id && <CallEvents id={c.id} />}
            </div>
          ))}
          {(hangup.error || reconcile.error) && (
            <p className="text-destructive">{(hangup.error ?? reconcile.error)?.message}</p>
          )}
        </div>
      </div>
    </section>
  );
}

function CallEvents({ id }: { id: number }) {
  const q = useCall(id);
  if (!q.data) return null;
  return (
    <div className="pl-4 text-muted-foreground">
      {q.data.events.map((e) => (
        <p key={e.id}>
          {new Date(e.receivedAt).toLocaleString()} · {e.kind} · {e.fromState || "—"} → {e.toState}{" "}
          · {e.detail}
        </p>
      ))}
    </div>
  );
}
