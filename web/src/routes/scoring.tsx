import { useEffect, useState } from "react";
import { useSession } from "@/api/session";
import {
  useCorrectEvent,
  usePutScoringRules,
  useScoringRules,
  useUnknownEvents,
} from "@/api/signals";
import type { ScoringDocument } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// Scoring rules: what counts, how much, how fast it fades. Nobody is hot or
// warm until the stage thresholds are set, and every assessment keeps the
// version it used.
export function ScoringPage() {
  const rules = useScoringRules();
  const put = usePutScoringRules();
  const unknown = useUnknownEvents();
  const correct = useCorrectEvent();
  const session = useSession();
  const isAdmin = session.data?.role === "admin";
  const canEdit = isAdmin || session.data?.role === "marketer";
  const [text, setText] = useState("");
  const [attribute, setAttribute] = useState<Record<number, string>>({});
  useEffect(() => {
    if (rules.data) {
      setText(JSON.stringify(rules.data.current?.document ?? rules.data.suggested, null, 2));
    }
  }, [rules.data]);
  function save() {
    let doc: ScoringDocument;
    try {
      doc = JSON.parse(text) as ScoringDocument;
    } catch {
      return;
    }
    put.mutate(doc);
  }
  return (
    <section className="space-y-6 text-sm">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Scoring</h1>
        <p className="text-muted-foreground">
          {rules.data?.configured
            ? `Rules version ${rules.data.current?.version}, saved ${new Date(rules.data.current?.createdAt ?? 0).toLocaleString()}.`
            : "No rules saved. Nobody is scored until an admin saves a version."}
          {rules.data?.current &&
            !rules.data.current.document.stages &&
            " Stage thresholds are unset, so nobody is hot, warm or cold."}
        </p>
      </div>
      <div className="grid gap-6 md:grid-cols-2">
        <div className="space-y-2">
          <h2 className="font-semibold">Rules (JSON)</h2>
          <textarea
            className="h-96 w-full rounded-md border bg-background p-2 font-mono text-xs"
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
          <h2 className="font-semibold">What ADX can observe</h2>
          <ul className="space-y-1">
            {rules.data?.catalog.map((k) => (
              <li key={k.kind} className={k.supported ? "" : "text-muted-foreground"}>
                <span className="font-mono">{k.kind}</span> · {k.source} · {k.description}
                {!k.supported && ` · not available: ${k.reason}`}
              </li>
            ))}
          </ul>
          <h2 className="pt-4 font-semibold">Events nobody is tied to</h2>
          <p className="text-muted-foreground">
            A browser or device id alone is not a person. These stay unscored until someone
            attributes them.
          </p>
          {unknown.data?.events.length === 0 && <p className="text-muted-foreground">None.</p>}
          {unknown.data?.events.map((e) => (
            <div key={e.id} className="flex flex-wrap items-center gap-2 border-t pt-2">
              <span>
                {e.kind} · {e.source} · {new Date(e.occurredAt).toLocaleString()} ·{" "}
                <span className="text-muted-foreground">{JSON.stringify(e.identity)}</span>
              </span>
              {canEdit && (
                <>
                  <Input
                    className="h-7 max-w-28"
                    placeholder="person id"
                    value={attribute[e.id] ?? ""}
                    onChange={(ev) => setAttribute({ ...attribute, [e.id]: ev.target.value })}
                  />
                  <Button
                    size="xs"
                    variant="outline"
                    disabled={!attribute[e.id]}
                    onClick={() =>
                      correct.mutate({
                        id: e.id,
                        action: "reattribute",
                        personId: Number(attribute[e.id]),
                        reason: "attributed by hand",
                      })
                    }
                  >
                    Attribute
                  </Button>
                  <Button
                    size="xs"
                    variant="outline"
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
                </>
              )}
            </div>
          ))}
          {correct.error && <p className="text-destructive">{correct.error.message}</p>}
        </div>
      </div>
    </section>
  );
}
