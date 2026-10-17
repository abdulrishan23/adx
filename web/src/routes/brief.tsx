import { useState } from "react";
import {
  useApproveDraft,
  useBrief,
  useCreateDraft,
  useLiftSuppression,
  useRecordObjection,
  useResolveObjection,
  useSendDraft,
  useSuppress,
} from "@/api/outreach";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// The sales brief for one enquiry: what we know, what the customer pushed
// back on, how they want to be contacted, and one recommended step with its
// reason. Drafts quote approved facts only and go out only through a
// configured channel to someone who has not opted out.
export function BriefPanel({
  enquiryId,
  canEdit,
  isAdmin,
}: {
  enquiryId: number;
  canEdit: boolean;
  isAdmin: boolean;
}) {
  const q = useBrief(enquiryId);
  const objection = useRecordObjection();
  const resolve = useResolveObjection();
  const suppress = useSuppress();
  const lift = useLiftSuppression();
  const draft = useCreateDraft();
  const approve = useApproveDraft();
  const send = useSendDraft();
  const [obj, setObj] = useState({ kind: "price", text: "" });
  const [body, setBody] = useState("");
  if (q.isPending) return <p className="text-muted-foreground">Loading brief…</p>;
  if (q.error) return <p className="text-destructive">{q.error.message}</p>;
  const b = q.data;
  const err = [objection, resolve, suppress, lift, draft, approve, send].find(
    (m) => m.error,
  )?.error;
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <p>
        <span className="font-medium">Next: {b.next.action}</span> · {b.next.reason}
        <span className="text-muted-foreground">
          {" "}
          · {b.daysSinceActivity} day{b.daysSinceActivity === 1 ? "" : "s"} since activity
          {b.score?.temperature ? ` · ${b.score.temperature}` : " · temperature not available"}
        </span>
      </p>
      <div>
        <h4 className="font-medium">Approved facts to quote</h4>
        {b.claims.length === 0 && (
          <p className="text-muted-foreground">
            {b.claimsNote || "None approved for this project."}
          </p>
        )}
        {b.claims.map((c) => (
          <p key={c.id} className="text-muted-foreground">
            {c.key}: {c.value} {c.currency ?? ""}
          </p>
        ))}
        {b.claims.length > 0 && b.claimsNote && (
          <p className="text-muted-foreground">{b.claimsNote}</p>
        )}
      </div>
      <div>
        <h4 className="font-medium">Objections</h4>
        {b.objections.length === 0 && <p className="text-muted-foreground">None recorded.</p>}
        {b.objections.map((o) => (
          <p key={o.id} className={o.resolvedAt ? "text-muted-foreground line-through" : ""}>
            {o.kind}: {o.text} <span className="text-muted-foreground">({o.source})</span>
            {o.resolvedAt && ` · resolved: ${o.resolution}`}
            {canEdit && !o.resolvedAt && (
              <Button
                size="xs"
                variant="outline"
                className="ml-2"
                onClick={() => resolve.mutate({ id: o.id, reason: "answered" })}
              >
                Resolved
              </Button>
            )}
          </p>
        ))}
        {canEdit && (
          <div className="flex flex-wrap items-center gap-1 pt-1">
            <select
              className="h-7 rounded-md border bg-background px-1"
              value={obj.kind}
              onChange={(e) => setObj({ ...obj, kind: e.target.value })}
            >
              {["price", "location", "timing", "payment", "trust", "unit", "other"].map((k) => (
                <option key={k}>{k}</option>
              ))}
            </select>
            <Input
              className="h-7 max-w-64"
              placeholder="what they said"
              value={obj.text}
              onChange={(e) => setObj({ ...obj, text: e.target.value })}
            />
            <Button
              size="xs"
              variant="outline"
              disabled={!obj.text}
              onClick={() =>
                objection.mutate(
                  {
                    personId: b.person.id,
                    enquiryId: enquiryId,
                    kind: obj.kind,
                    text: obj.text,
                    source: "note",
                  },
                  { onSuccess: () => setObj({ ...obj, text: "" }) },
                )
              }
            >
              Record objection
            </Button>
          </div>
        )}
      </div>
      <div>
        <h4 className="font-medium">Contact preferences</h4>
        <p className="text-muted-foreground">
          language {b.preferences.language}
          {b.preferences.preferredChannel ? ` · prefers ${b.preferences.preferredChannel}` : ""}
          {b.preferences.doNotContact ? " · asked not to be contacted" : ""}
        </p>
        {b.suppressions.map((s) => (
          <p
            key={s.id}
            className={s.liftedAt ? "text-muted-foreground line-through" : "text-destructive"}
          >
            opted out of {s.channel}: {s.reason} ({s.source},{" "}
            {new Date(s.since).toLocaleDateString()}){s.liftedAt && ` · lifted: ${s.liftReason}`}
            {isAdmin && !s.liftedAt && (
              <Button
                size="xs"
                variant="outline"
                className="ml-2"
                onClick={() => lift.mutate({ id: s.id, reason: "consent renewed in writing" })}
              >
                Lift (consent renewed)
              </Button>
            )}
          </p>
        ))}
        {canEdit && (
          <div className="flex flex-wrap gap-1 pt-1">
            {["all", "whatsapp", "sms", "email", "call"].map((ch) => (
              <Button
                key={ch}
                size="xs"
                variant="outline"
                onClick={() =>
                  suppress.mutate({
                    personId: b.person.id,
                    channel: ch,
                    reason: "customer asked",
                    source: "manual",
                  })
                }
              >
                Opt out: {ch}
              </Button>
            ))}
          </div>
        )}
      </div>
      <div>
        <h4 className="font-medium">Messages</h4>
        {b.drafts.length === 0 && <p className="text-muted-foreground">No drafts.</p>}
        {b.drafts.map((d) => (
          <div key={d.id} className="border-t pt-1">
            <p>
              {d.channel} · {d.state}
              {d.reason && <span className="text-muted-foreground"> · {d.reason}</span>}
              {d.providerRef && (
                <span className="text-muted-foreground"> · ref {d.providerRef}</span>
              )}
            </p>
            <p className="whitespace-pre-wrap text-muted-foreground">{d.body}</p>
            {canEdit && d.state === "draft" && (
              <Button size="xs" variant="outline" onClick={() => approve.mutate(d.id)}>
                Approve
              </Button>
            )}
            {canEdit &&
              (d.state === "approved" ||
                d.state === "held" ||
                d.state === "failed" ||
                d.state === "suppressed") && (
                <Button
                  size="xs"
                  variant="outline"
                  className="ml-1"
                  onClick={() => send.mutate(d.id)}
                >
                  Send (checks run again)
                </Button>
              )}
          </div>
        ))}
        {canEdit && (
          <div className="flex flex-wrap items-center gap-1 pt-1">
            <Input
              className="h-7 max-w-96"
              placeholder={b.next.draft || "leave empty for the suggested draft"}
              value={body}
              onChange={(e) => setBody(e.target.value)}
            />
            <Button
              size="xs"
              variant="outline"
              onClick={() =>
                draft.mutate(
                  { personId: b.person.id, enquiryId, channel: "whatsapp", body },
                  { onSuccess: () => setBody("") },
                )
              }
            >
              Draft WhatsApp
            </Button>
          </div>
        )}
      </div>
      {b.fieldWrites.length > 0 && (
        <div>
          <h4 className="font-medium">CRM fields</h4>
          {b.fieldWrites.map((f) => (
            <p
              key={f.id}
              className={f.state === "conflict" ? "text-destructive" : "text-muted-foreground"}
            >
              {f.field} = {f.value} · {f.state}
              {f.reason && ` · ${f.reason}`}
            </p>
          ))}
        </div>
      )}
      {err && <p className="text-destructive">{err.message}</p>}
    </div>
  );
}
