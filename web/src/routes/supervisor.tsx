import { useEffect, useState } from "react";
import { useSession } from "@/api/session";
import {
  useArbitrate,
  useConflicts,
  useHalt,
  useInbox,
  usePutSupervisorConfig,
  useRecommendations,
  useSupervisorAct,
  useSupervisorConfig,
} from "@/api/supervisor";
import type { InboxItem, SupervisorDocument } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// The supervisor inbox: everything waiting on a person, most urgent first,
// with the action each item takes through its own domain. The kill switch
// is policy's; the autonomy configuration says what agents may do alone.
export function SupervisorPage() {
  const inbox = useInbox();
  const act = useSupervisorAct();
  const config = useSupervisorConfig();
  const put = usePutSupervisorConfig();
  const conflicts = useConflicts();
  const recs = useRecommendations();
  const arbitrate = useArbitrate();
  const halt = useHalt();
  const session = useSession();
  const isAdmin = session.data?.role === "admin";
  const canEdit = isAdmin || session.data?.role === "marketer";
  const [text, setText] = useState("");
  const [note, setNote] = useState("");
  const [target, setTarget] = useState("");
  useEffect(() => {
    if (config.data)
      setText(JSON.stringify(config.data.current?.document ?? config.data.suggested, null, 2));
  }, [config.data]);
  function save() {
    try {
      put.mutate(JSON.parse(text) as SupervisorDocument);
    } catch {
      /* invalid JSON stays in the editor */
    }
  }
  const engaged = config.data?.killSwitches.some((k) => k.engaged) ?? false;
  return (
    <section className="space-y-6 text-sm">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <div>
          <h1 className="text-2xl font-semibold">Supervisor</h1>
          <p className="text-muted-foreground">
            {inbox.data
              ? `${inbox.data.items.length} item${inbox.data.items.length === 1 ? "" : "s"} waiting.`
              : "Loading…"}
          </p>
        </div>
        {isAdmin && (
          <Button
            size="sm"
            variant={engaged ? "default" : "outline"}
            onClick={() =>
              halt.mutate({
                accountId: 0,
                engaged: !engaged,
                reason: engaged ? "released from the supervisor" : "stopped from the supervisor",
              })
            }
          >
            {engaged ? "Release kill switch" : "Stop everything (kill switch)"}
          </Button>
        )}
      </div>
      <div className="grid gap-6 md:grid-cols-[1fr_22rem]">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-1">
            <Input
              className="h-7 max-w-64"
              placeholder="note for the next action"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
            <Input
              className="h-7 max-w-40"
              placeholder="target id (keep)"
              value={target}
              onChange={(e) => setTarget(e.target.value)}
            />
          </div>
          {inbox.data?.items.length === 0 && (
            <p className="text-muted-foreground">Nothing is waiting.</p>
          )}
          {inbox.data?.items.map((it) => (
            <InboxRow
              key={`${it.kind}-${it.ref}`}
              item={it}
              canEdit={canEdit}
              onAct={(action) =>
                act.mutate({
                  kind: it.kind,
                  ref: it.ref,
                  action,
                  note,
                  target: Number(target) || 0,
                })
              }
            />
          ))}
          {act.error && <p className="text-destructive">{act.error.message}</p>}
        </div>
        <aside className="space-y-3">
          <h2 className="font-medium">Autonomy and precedence (JSON)</h2>
          <p className="text-muted-foreground">
            {config.data?.configured
              ? `Version ${config.data.current?.version}.`
              : "Not configured: every capability recommends only and every conflict goes to a person."}
          </p>
          <textarea
            className="h-56 w-full rounded-md border bg-background p-2 font-mono text-xs"
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
          <div className="flex items-baseline justify-between pt-2">
            <h2 className="font-medium">Conflicts</h2>
            {canEdit && (
              <Button size="xs" variant="outline" onClick={() => arbitrate.mutate()}>
                Arbitrate now
              </Button>
            )}
          </div>
          {conflicts.data?.conflicts.length === 0 && <p className="text-muted-foreground">None.</p>}
          {conflicts.data?.conflicts.map((c) => (
            <p
              key={c.id}
              className={c.state === "escalated" ? "text-destructive" : "text-muted-foreground"}
            >
              #{c.id} · envelope {c.envelopeId} · proposals {c.proposalIds.join(", ")} · {c.state}
              {c.resolution && ` · ${c.resolution}`}
            </p>
          ))}
          <h2 className="pt-2 font-medium">Recommendations</h2>
          {recs.data?.recommendations.length === 0 && (
            <p className="text-muted-foreground">None recorded.</p>
          )}
          {recs.data?.recommendations.map((r) => (
            <p key={r.id} className="text-muted-foreground">
              proposal {r.proposalId} · {r.source} · {r.capability}
              {r.objective && ` · ${r.objective}`}
            </p>
          ))}
        </aside>
      </div>
    </section>
  );
}

function InboxRow({
  item,
  canEdit,
  onAct,
}: {
  item: InboxItem;
  canEdit: boolean;
  onAct: (action: string) => void;
}) {
  const tone =
    item.severity === "urgent"
      ? "border-destructive/60"
      : item.severity === "warning"
        ? "border-amber-500/40"
        : "";
  return (
    <div className={`rounded-md border p-2 ${tone}`}>
      <p>
        <span className="font-medium">{item.title}</span>
        <span className="text-muted-foreground">
          {" "}
          · {item.kind} {item.ref} · {item.severity} · since {new Date(item.since).toLocaleString()}
        </span>
      </p>
      {item.detail && <p className="text-muted-foreground">{item.detail}</p>}
      {canEdit && (
        <p className="flex flex-wrap gap-1 pt-1">
          {item.actions
            .filter((a) => a !== "inspect")
            .map((a) => (
              <Button key={a} size="xs" variant="outline" onClick={() => onAct(a)}>
                {a.replace("_", " ")}
              </Button>
            ))}
        </p>
      )}
    </div>
  );
}
