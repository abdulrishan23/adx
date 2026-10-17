import { useState } from "react";
import {
  useActivateEntry,
  useCapabilities,
  useRegistry,
  useRetireEntry,
  useSaveAgent,
  useSaveSkill,
  useSaveWorkflow,
  useValidateWorkflow,
} from "@/api/registry";
import { useSession } from "@/api/session";
import type { AgentDocument, SkillDocument, WorkflowDocument } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// Skills, agents and workflow definitions. Every version is a draft until
// activated; a version that widens access, or one a model wrote, waits for
// an admin. The capability catalogue is fixed: read tools and proposals,
// nothing that writes directly.
export function AgentsPage() {
  const [kind, setKind] = useState<"skill" | "agent" | "workflow">("skill");
  const list = useRegistry(kind);
  const caps = useCapabilities();
  const activate = useActivateEntry();
  const retire = useRetireEntry();
  const saveSkill = useSaveSkill();
  const saveAgent = useSaveAgent();
  const saveWorkflow = useSaveWorkflow();
  const validate = useValidateWorkflow();
  const session = useSession();
  const isAdmin = session.data?.role === "admin";
  const canEdit = isAdmin || session.data?.role === "marketer";
  const [key, setKey] = useState("");
  const [text, setText] = useState("");
  const [parseError, setParseError] = useState("");
  function parse<T>(): T | null {
    try {
      setParseError("");
      return JSON.parse(text) as T;
    } catch (e) {
      setParseError(`Not valid JSON: ${(e as Error).message}`);
      return null;
    }
  }
  function save() {
    if (kind === "skill") {
      const d = parse<SkillDocument>();
      if (d) saveSkill.mutate({ key, document: d, fromModel: false });
    } else if (kind === "agent") {
      const d = parse<AgentDocument>();
      if (d) saveAgent.mutate({ key, document: d, fromModel: false });
    } else {
      const d = parse<WorkflowDocument>();
      if (d) saveWorkflow.mutate({ key, document: d, fromModel: false });
    }
  }
  const err = [activate, retire, saveSkill, saveAgent, saveWorkflow, validate].find(
    (m) => m.error,
  )?.error;
  return (
    <section className="space-y-6 text-sm">
      <div>
        <h1 className="text-2xl font-semibold">Agents and skills</h1>
        <p className="text-muted-foreground">
          A skill is instructions with a typed input and output, an evidence scope and permitted
          tools. An agent bundles skills under an account scope and never acts above marketer. A
          workflow is a bounded graph of steps. New versions are drafts; widening access or a
          model's draft needs an admin.
        </p>
      </div>
      <div className="flex gap-1">
        {(["skill", "agent", "workflow"] as const).map((k) => (
          <Button
            key={k}
            size="xs"
            variant={kind === k ? "default" : "outline"}
            onClick={() => setKind(k)}
          >
            {k}s
          </Button>
        ))}
      </div>
      <div className="grid gap-6 md:grid-cols-2">
        <div className="space-y-2">
          {list.data?.entries.length === 0 && <p className="text-muted-foreground">None yet.</p>}
          {list.data?.entries.map((e) => (
            <div
              key={e.id}
              className={`border-t pt-1 ${e.state === "retired" ? "text-muted-foreground" : ""}`}
            >
              <p>
                {e.key} v{e.version} · {e.state} · {e.source}
                {e.needsAdmin && e.state === "draft" && (
                  <span className="text-destructive"> · needs an admin: {e.reason}</span>
                )}
                {!e.needsAdmin && e.reason && (
                  <span className="text-muted-foreground"> · {e.reason}</span>
                )}
                {canEdit && e.state === "draft" && e.validation.length === 0 && (
                  <Button
                    size="xs"
                    variant="outline"
                    className="ml-2"
                    onClick={() => activate.mutate(e.id)}
                  >
                    Activate
                  </Button>
                )}
                {isAdmin && e.state !== "retired" && (
                  <Button
                    size="xs"
                    variant="outline"
                    className="ml-1"
                    onClick={() => retire.mutate(e.id)}
                  >
                    Retire
                  </Button>
                )}
              </p>
              {e.validation.length > 0 && (
                <ul className="text-destructive">
                  {e.validation.map((v) => (
                    <li key={v}>{v}</li>
                  ))}
                </ul>
              )}
              <details>
                <summary className="text-muted-foreground">document</summary>
                <pre className="max-h-64 overflow-auto whitespace-pre-wrap text-xs">
                  {JSON.stringify(JSON.parse(e.document), null, 2)}
                </pre>
              </details>
            </div>
          ))}
        </div>
        <div className="space-y-2">
          <h2 className="font-medium">New {kind} version (JSON)</h2>
          <Input
            placeholder="key, e.g. research.pass"
            value={key}
            onChange={(e) => setKey(e.target.value)}
          />
          <textarea
            className="h-64 w-full rounded-md border bg-background p-2 font-mono text-xs"
            value={text}
            onChange={(e) => setText(e.target.value)}
            readOnly={!canEdit}
          />
          {canEdit && (
            <div className="flex gap-1">
              <Button size="sm" onClick={save} disabled={!key}>
                Save draft
              </Button>
              {kind === "workflow" && (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    const d = parse<WorkflowDocument>();
                    if (d) validate.mutate(d);
                  }}
                >
                  Validate only
                </Button>
              )}
            </div>
          )}
          {parseError && <p className="text-destructive">{parseError}</p>}
          {validate.data && (
            <p
              className={
                validate.data.problems.length ? "text-destructive" : "text-muted-foreground"
              }
            >
              {validate.data.problems.length ? validate.data.problems.join("; ") : "No problems."}
            </p>
          )}
          {err && <p className="text-destructive">{err.message}</p>}
          <h2 className="pt-4 font-medium">Capability catalogue</h2>
          <ul className="text-muted-foreground">
            {caps.data?.capabilities.map((c) => (
              <li key={c.name}>
                <span className="font-mono">{c.name}</span> · {c.kind} · {c.domain} ·{" "}
                {c.description}
              </li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  );
}
