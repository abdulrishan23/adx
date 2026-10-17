import { useEffect, useState } from "react";
import {
  useDispatchFields,
  useFieldWrites,
  useOutreachConfig,
  usePutOutreachConfig,
  useResolveFieldConflict,
  useSyncScores,
} from "@/api/outreach";
import { useSession } from "@/api/session";
import type { OutreachDocument } from "@/api/types.gen";
import { Button } from "@/components/ui/button";

// Outreach settings: who owns which CRM field, which channels may send, and
// the CRM field outbox with its conflicts. Every channel is off until an
// admin says otherwise, and no adapter can send until one is wired.
export function OutreachPage() {
  const config = useOutreachConfig();
  const put = usePutOutreachConfig();
  const fields = useFieldWrites();
  const dispatch = useDispatchFields();
  const sync = useSyncScores();
  const resolve = useResolveFieldConflict();
  const session = useSession();
  const isAdmin = session.data?.role === "admin";
  const canEdit = isAdmin || session.data?.role === "marketer";
  const [text, setText] = useState("");
  useEffect(() => {
    if (config.data)
      setText(JSON.stringify(config.data.current?.document ?? config.data.suggested, null, 2));
  }, [config.data]);
  function save() {
    try {
      put.mutate(JSON.parse(text) as OutreachDocument);
    } catch {
      /* invalid JSON stays in the editor */
    }
  }
  const st = config.data;
  return (
    <section className="space-y-6 text-sm">
      <div>
        <h1 className="text-2xl font-semibold">Outreach</h1>
        {st && (
          <p className="text-muted-foreground">
            {st.configured
              ? `Config version ${st.current?.version}.`
              : "Not configured: every channel is off and every CRM field edit is a conflict."}{" "}
            Messaging:{" "}
            {st.messaging.adapter
              ? `${st.messaging.adapter} (${st.messaging.channels.join(", ")})`
              : "no adapter"}
            {st.messaging.reason ? ` — ${st.messaging.reason}` : ""}. CRM field writes:{" "}
            {st.fieldWrites.adapter || "no writer"}
            {st.fieldWrites.reason ? ` — ${st.fieldWrites.reason}` : ""}.
          </p>
        )}
      </div>
      <div className="grid gap-6 md:grid-cols-2">
        <div className="space-y-2">
          <h2 className="font-medium">Configuration (JSON)</h2>
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
          <div className="flex items-baseline justify-between">
            <h2 className="font-medium">CRM field outbox</h2>
            {canEdit && (
              <span className="flex gap-1">
                <Button size="xs" variant="outline" onClick={() => sync.mutate()}>
                  Queue scores
                </Button>
                <Button size="xs" variant="outline" onClick={() => dispatch.mutate()}>
                  Dispatch now
                </Button>
              </span>
            )}
          </div>
          {dispatch.data && <p className="text-muted-foreground">Sent {dispatch.data.sent}.</p>}
          {sync.data && <p className="text-muted-foreground">Queued {sync.data.queued}.</p>}
          {fields.data?.writes.length === 0 && (
            <p className="text-muted-foreground">Nothing queued.</p>
          )}
          {fields.data?.writes.map((f) => (
            <div
              key={f.id}
              className={`border-t pt-1 ${f.state === "conflict" ? "text-destructive" : ""}`}
            >
              <p>
                person {f.personId} · {f.externalId} · {f.field} = {f.value} · {f.state}
                {f.reason && <span className="text-muted-foreground"> · {f.reason}</span>}
              </p>
              {f.state === "conflict" && (
                <p className="text-muted-foreground">
                  {JSON.stringify(f.conflict)}
                  {canEdit && (
                    <>
                      <Button
                        size="xs"
                        variant="outline"
                        className="ml-2"
                        onClick={() =>
                          resolve.mutate({ id: f.id, keepOurs: true, reason: "ADX value stands" })
                        }
                      >
                        Keep ADX's
                      </Button>
                      <Button
                        size="xs"
                        variant="outline"
                        className="ml-1"
                        onClick={() =>
                          resolve.mutate({ id: f.id, keepOurs: false, reason: "CRM value stands" })
                        }
                      >
                        Take the CRM's
                      </Button>
                    </>
                  )}
                </p>
              )}
            </div>
          ))}
          {resolve.error && <p className="text-destructive">{resolve.error.message}</p>}
        </div>
      </div>
    </section>
  );
}
