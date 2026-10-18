import { useEffect, useState } from "react";
import type { FunnelDocument, Stage } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useFunnel, usePutFunnel } from "@/engine/api/leads";
import { useSession } from "@/engine/api/session";

const empty: FunnelDocument = {
  statusField: { kind: "", ref: "" },
  projectField: { kind: "", ref: "" },
  statuses: {},
  blankStatusStage: null,
  sources: {},
  note: "",
};

// The status-to-stage table (reference.md: Stage). Raw statuses seen in the
// data are listed with their counts; each gets a stage or stays "Unmapped",
// and unmapped statuses are flagged. Which CRM field holds the status is an
// open question (#34), so it is a setting here, not a guess in code.
export function FunnelPage() {
  const funnel = useFunnel();
  const session = useSession();
  const put = usePutFunnel();
  const [doc, setDoc] = useState<FunnelDocument>(empty);
  const isAdmin = session.data?.role === "admin";

  useEffect(() => {
    if (funnel.data?.current) setDoc(funnel.data.current.document);
  }, [funnel.data]);

  if (funnel.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (funnel.error) return <p className="text-destructive">{funnel.error.message}</p>;
  const state = funnel.data;

  const observed = new Map<string, number>();
  for (const o of state.observedStatuses) {
    if (o.raw) observed.set(o.raw, o.count);
  }
  const rows = new Set<string>([...observed.keys(), ...Object.keys(doc.statuses ?? {})]);

  function setStage(raw: string, stage: string) {
    const statuses = { ...(doc.statuses ?? {}) };
    const key = raw.trim().toLowerCase().replace(/\s+/g, " ");
    if (stage === "") delete statuses[key];
    else statuses[key] = stage as Stage;
    setDoc({ ...doc, statuses });
  }

  function stageOf(raw: string): string {
    const key = raw.trim().toLowerCase().replace(/\s+/g, " ");
    return doc.statuses?.[key] ?? "";
  }

  return (
    <section className="space-y-8">
      <div>
        <h1 className="text-3xl font-semibold tracking-tight">Funnel</h1>
        <p className="text-muted-foreground">
          {state.configured
            ? `Version ${state.current?.version}. Changing it re-stages every lead.`
            : "Not configured. Leads keep their raw status and have no stage until this is saved."}
        </p>
      </div>

      {state.unmapped.length > 0 && (
        <p className="rounded-lg border p-3 text-sm">
          Statuses waiting for a stage: <strong>{state.unmapped.join(", ")}</strong>
        </p>
      )}

      <fieldset className="space-y-3" disabled={!isAdmin}>
        <legend className="font-medium">Where the values live in GoHighLevel</legend>
        <FieldLocationEditor
          label="Status"
          value={doc.statusField}
          onChange={(statusField) => setDoc({ ...doc, statusField })}
        />
        <FieldLocationEditor
          label="Project"
          value={doc.projectField}
          onChange={(projectField) => setDoc({ ...doc, projectField })}
        />
      </fieldset>

      <fieldset disabled={!isAdmin}>
        <legend className="font-medium">Status to stage</legend>
        <table className="mt-2 w-full text-sm">
          <thead className="text-left text-muted-foreground">
            <tr>
              <th className="py-1 font-normal">Raw status</th>
              <th className="py-1 font-normal">Leads</th>
              <th className="py-1 font-normal">Stage</th>
            </tr>
          </thead>
          <tbody className="divide-y border-y">
            <tr>
              <td className="py-2 text-muted-foreground">(blank)</td>
              <td className="py-2">
                {state.observedStatuses.find((o) => o.raw === null)?.count ?? 0}
              </td>
              <td className="py-2">
                <StageSelect
                  stages={state.stages}
                  value={doc.blankStatusStage ?? ""}
                  onChange={(v) =>
                    setDoc({ ...doc, blankStatusStage: v === "" ? null : (v as Stage) })
                  }
                />
              </td>
            </tr>
            {[...rows].sort().map((raw) => (
              <tr key={raw}>
                <td className="py-2">{raw}</td>
                <td className="py-2">{observed.get(raw) ?? 0}</td>
                <td className="py-2">
                  <StageSelect
                    stages={state.stages}
                    value={stageOf(raw)}
                    onChange={(v) => setStage(raw, v)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </fieldset>

      {isAdmin && (
        <div className="space-y-2">
          <Button disabled={put.isPending} onClick={() => put.mutate(doc)}>
            Save as new version
          </Button>
          {put.error && <p className="text-sm text-destructive">{put.error.message}</p>}
          {put.data && (
            <p className="text-sm text-muted-foreground">Saved version {put.data.version}.</p>
          )}
        </div>
      )}
    </section>
  );
}

function StageSelect({
  stages,
  value,
  onChange,
}: {
  stages: Stage[];
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <select
      className="rounded-md border bg-background px-2 py-1"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    >
      <option value="">Unmapped</option>
      {stages.map((s) => (
        <option key={s} value={s}>
          {s}
        </option>
      ))}
    </select>
  );
}

function FieldLocationEditor({
  label,
  value,
  onChange,
}: {
  label: string;
  value: FunnelDocument["statusField"];
  onChange: (v: FunnelDocument["statusField"]) => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 text-sm">
      <span className="w-16 text-muted-foreground">{label}</span>
      <select
        className="rounded-md border bg-background px-2 py-1"
        value={value.kind}
        onChange={(e) => onChange({ ...value, kind: e.target.value })}
      >
        <option value="">Not configured</option>
        <option value="custom_field">Custom field (id)</option>
        <option value="contact_field">Contact field (source, country)</option>
        <option value="tag_prefix">Tag prefix</option>
      </select>
      <Input
        className="max-w-xs"
        placeholder={value.kind === "custom_field" ? "custom field id" : "reference"}
        value={value.ref}
        onChange={(e) => onChange({ ...value, ref: e.target.value })}
      />
    </div>
  );
}
