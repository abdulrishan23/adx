import { useEffect, useState } from "react";
import {
  useApplyImport,
  useCompleteTask,
  useCreateTask,
  useImports,
  useOpportunities,
  useOpportunity,
  usePipelineConfig,
  usePreviewImport,
  usePutPipelineConfig,
  useSetStage,
  useTasks,
} from "@/api/pipeline";
import { useSession } from "@/api/session";
import type { PipelineDocument } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// The pipeline: opportunities by stage (one per enquiry), follow-up tasks,
// and the import that previews before it changes anything. A stage a
// person set survives an import when ADX owns the stage.
export function PipelinePage() {
  const config = usePipelineConfig();
  const put = usePutPipelineConfig();
  const [stage, setStage] = useState("");
  const opps = useOpportunities(stage, "");
  const tasks = useTasks("open");
  const imports = useImports();
  const preview = usePreviewImport();
  const apply = useApplyImport();
  const complete = useCompleteTask();
  const session = useSession();
  const isAdmin = session.data?.role === "admin";
  const canEdit = isAdmin || session.data?.role === "marketer";
  const [text, setText] = useState("");
  const [open, setOpen] = useState<number | null>(null);
  useEffect(() => {
    if (config.data)
      setText(JSON.stringify(config.data.current?.document ?? config.data.suggested, null, 2));
  }, [config.data]);
  function save() {
    try {
      put.mutate(JSON.parse(text) as PipelineDocument);
    } catch {
      /* invalid JSON stays in the editor */
    }
  }
  const stages = config.data?.current?.document.stages ?? [];
  return (
    <section className="space-y-6 text-sm">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Pipeline</h1>
        <p className="text-muted-foreground">
          {config.data?.configured
            ? `Stages v${config.data.current?.version} · stage owner: ${config.data.current?.document.stageOwner}`
            : "Not configured: an admin saves the stages before opportunities exist."}
        </p>
      </div>
      <div className="grid gap-6 md:grid-cols-[1fr_20rem]">
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-1">
            <Button
              size="xs"
              variant={stage === "" ? "default" : "outline"}
              onClick={() => setStage("")}
            >
              All
            </Button>
            {stages.map((s) => (
              <Button
                key={s.key}
                size="xs"
                variant={stage === s.key ? "default" : "outline"}
                onClick={() => setStage(s.key)}
              >
                {s.label}
              </Button>
            ))}
          </div>
          {opps.data?.opportunities.length === 0 && (
            <p className="text-muted-foreground">No opportunities.</p>
          )}
          {opps.data?.opportunities.map((o) => (
            <div key={o.id} className="border-t pt-1">
              <p>
                #{o.id} · person {o.personId} · enquiry {o.enquiryId} · {o.stage} ({o.stageSetBy}) ·{" "}
                {o.state}
                {o.valueMinor !== null &&
                  ` · ${o.currency} ${(o.valueMinor / 100).toLocaleString()}`}
                {o.ownerUserId !== null && ` · owner ${o.ownerUserId}`}
                <span className="text-muted-foreground"> · v{o.version}</span>
                <Button
                  size="xs"
                  variant="outline"
                  className="ml-2"
                  onClick={() => setOpen(open === o.id ? null : o.id)}
                >
                  {open === o.id ? "Hide" : "Open"}
                </Button>
              </p>
              {open === o.id && (
                <OpportunityPanel id={o.id} stages={stages.map((s) => s.key)} canEdit={canEdit} />
              )}
            </div>
          ))}
          <h2 className="pt-4 font-semibold">Open follow-ups</h2>
          {tasks.data?.tasks.length === 0 && <p className="text-muted-foreground">None.</p>}
          {tasks.data?.tasks.map((t) => (
            <p key={t.id}>
              {t.title} · person {t.personId}
              {t.dueAt && ` · due ${new Date(t.dueAt).toLocaleDateString()}`}
              {t.assigneeUserId !== null && ` · assignee ${t.assigneeUserId}`}
              {canEdit && (
                <>
                  <Button
                    size="xs"
                    variant="outline"
                    className="ml-2"
                    onClick={() => complete.mutate({ id: t.id, canceled: false })}
                  >
                    Done
                  </Button>
                  <Button
                    size="xs"
                    variant="outline"
                    className="ml-1"
                    onClick={() => complete.mutate({ id: t.id, canceled: true })}
                  >
                    Cancel
                  </Button>
                </>
              )}
            </p>
          ))}
        </div>
        <aside className="space-y-3">
          <h2 className="font-semibold">Import from GoHighLevel leads</h2>
          <p className="text-muted-foreground">
            A preview plans every change and writes nothing. Apply walks the plan and resumes if it
            stops.
          </p>
          {canEdit && (
            <Button
              size="sm"
              variant="outline"
              disabled={preview.isPending}
              onClick={() => preview.mutate("gohighlevel")}
            >
              Preview import
            </Button>
          )}
          {imports.data?.imports.map((b) => (
            <div key={b.id} className="border-t pt-1">
              <p>
                #{b.id} · {b.state} · {b.total} rows: {b.created} create, {b.updated} update,{" "}
                {b.skipped} skip, {b.conflicts} conflict
                {b.error && <span className="text-destructive"> · {b.error}</span>}
                {b.state !== "applied" && (
                  <span className="text-muted-foreground"> · cursor {b.cursor}</span>
                )}
              </p>
              {canEdit && b.state !== "applied" && (
                <Button
                  size="xs"
                  variant="outline"
                  disabled={apply.isPending}
                  onClick={() => apply.mutate(b.id)}
                >
                  {b.state === "failed" ? "Resume" : "Apply"}
                </Button>
              )}
            </div>
          ))}
          {(preview.error || apply.error) && (
            <p className="text-destructive">{(preview.error ?? apply.error)?.message}</p>
          )}
          <h2 className="pt-4 font-semibold">Stages (JSON)</h2>
          <textarea
            className="h-64 w-full rounded-md border bg-background p-2 font-mono text-xs"
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
        </aside>
      </div>
    </section>
  );
}

function OpportunityPanel({
  id,
  stages,
  canEdit,
}: {
  id: number;
  stages: string[];
  canEdit: boolean;
}) {
  const q = useOpportunity(id);
  const setStage = useSetStage();
  const createTask = useCreateTask();
  const [title, setTitle] = useState("");
  if (!q.data) return null;
  const d = q.data;
  return (
    <div className="space-y-1 pl-4 text-muted-foreground">
      {canEdit && (
        <p className="flex flex-wrap gap-1">
          {stages.map((s) => (
            <Button
              key={s}
              size="xs"
              variant={s === d.opportunity.stage ? "default" : "outline"}
              onClick={() =>
                setStage.mutate({
                  id,
                  stage: s,
                  version: d.opportunity.version,
                  reason: "moved from the pipeline",
                })
              }
            >
              {s}
            </Button>
          ))}
        </p>
      )}
      {setStage.error && <p className="text-destructive">{setStage.error.message}</p>}
      {d.assignments.map((a) => (
        <p key={a.id}>
          assigned to {a.userId}{" "}
          {a.endedAt ? `until ${new Date(a.endedAt).toLocaleDateString()}` : "(current)"} ·{" "}
          {a.reason}
        </p>
      ))}
      {d.interactions.map((i) => (
        <p key={i.id}>
          {new Date(i.occurredAt).toLocaleDateString()} · {i.kind} · {i.summary}
        </p>
      ))}
      {d.tasks.map((t) => (
        <p key={t.id}>
          task · {t.title} · {t.state}
        </p>
      ))}
      {canEdit && (
        <p className="flex items-center gap-1">
          <Input
            className="h-7 max-w-64"
            placeholder="follow-up title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
          <Button
            size="xs"
            variant="outline"
            disabled={!title}
            onClick={() =>
              createTask.mutate(
                {
                  opportunityId: id,
                  personId: d.opportunity.personId,
                  title,
                  dueAt: null,
                  assigneeUserId: 0,
                  occurrenceKey: "",
                },
                { onSuccess: () => setTitle("") },
              )
            }
          >
            Add follow-up
          </Button>
        </p>
      )}
    </div>
  );
}
