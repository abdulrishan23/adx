import { type FormEvent, useState } from "react";
import {
  useAssets,
  useCreateCreative,
  useCreative,
  useCreativeAction,
  useCreatives,
  useSaveDocument,
  useUploadAsset,
} from "@/api/creative";
import { useProjects } from "@/api/projects";
import { useSession } from "@/api/session";
import type { Document, Version } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const stateLabel: Record<string, string> = {
  draft: "Draft",
  rendered: "Rendered",
  in_review: "On the review board",
  approved: "Approved (content only; not delivered)",
  revise: "Sent back",
};

// The pilot creative journey: upload a render, lay out copy in a format and
// language, render, put on the board, approve or send back, export. Review
// binds to a version; editing an approved version makes a new, unapproved one.
export function CreativesPage() {
  const creatives = useCreatives();
  const [current, setCurrent] = useState<number | null>(null);
  const session = useSession();
  const canEdit = session.data?.role === "marketer" || session.data?.role === "admin";

  return (
    <section className="grid gap-6 md:grid-cols-[16rem_1fr]">
      <aside className="space-y-3">
        <h1 className="text-2xl font-semibold">Creatives</h1>
        {creatives.data?.creatives.length === 0 && (
          <p className="text-sm text-muted-foreground">Nothing yet.</p>
        )}
        <ul className="space-y-1 text-sm">
          {creatives.data?.creatives.map((c) => (
            <li key={c.id}>
              <button
                type="button"
                className={`w-full rounded-md px-2 py-1 text-left ${c.id === current ? "bg-muted" : "hover:bg-muted"}`}
                onClick={() => setCurrent(c.id)}
              >
                {c.name}{" "}
                <span className="text-muted-foreground">
                  · {c.format} · {c.language}
                </span>
              </button>
            </li>
          ))}
        </ul>
        {canEdit && <NewCreative onCreated={setCurrent} />}
      </aside>
      <div>
        {current === null ? (
          <p className="text-muted-foreground">Pick or create a creative.</p>
        ) : (
          <CreativeDetail id={current} canEdit={canEdit} isAdmin={session.data?.role === "admin"} />
        )}
      </div>
    </section>
  );
}

function NewCreative({ onCreated }: { onCreated: (id: number) => void }) {
  const projects = useProjects();
  const renders = useAssets("render");
  const upload = useUploadAsset();
  const create = useCreateCreative();
  const [form, setForm] = useState({
    projectId: 0,
    name: "",
    format: "9:16",
    language: "en",
    renderAssetId: 0,
    eyebrow: "",
    title: "",
    text: "",
    note: "",
    cta: "",
  });

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    create.mutate(
      {
        projectId: form.projectId,
        name: form.name,
        format: form.format as "9:16" | "4:5" | "1:1" | "1.91:1",
        language: form.language as "en" | "ar",
        renderAssetId: form.renderAssetId,
        copy: {
          eyebrow: form.eyebrow,
          title: form.title,
          text: form.text,
          note: form.note,
          cta: form.cta,
        },
      },
      { onSuccess: (r) => onCreated(r.creative.id) },
    );
  }

  return (
    <form onSubmit={onSubmit} className="space-y-2 rounded-lg border p-3 text-sm">
      <h2 className="font-medium">New creative</h2>
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={form.projectId}
        onChange={(e) => setForm({ ...form, projectId: Number(e.target.value) })}
      >
        <option value={0}>No project</option>
        {projects.data?.projects.map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
          </option>
        ))}
      </select>
      <Input
        placeholder="Name"
        value={form.name}
        onChange={(e) => setForm({ ...form, name: e.target.value })}
      />
      <div className="flex gap-2">
        <select
          className="rounded-md border bg-background px-2 py-1"
          value={form.format}
          onChange={(e) => setForm({ ...form, format: e.target.value })}
        >
          {["9:16", "4:5", "1:1", "1.91:1"].map((f) => (
            <option key={f} value={f}>
              {f}
            </option>
          ))}
        </select>
        <select
          className="rounded-md border bg-background px-2 py-1"
          value={form.language}
          onChange={(e) => setForm({ ...form, language: e.target.value })}
        >
          <option value="en">English</option>
          <option value="ar">Arabic</option>
        </select>
      </div>
      <Input
        placeholder="Eyebrow (project or offer line)"
        value={form.eyebrow}
        onChange={(e) => setForm({ ...form, eyebrow: e.target.value })}
        dir="auto"
      />
      <Input
        placeholder="Title"
        value={form.title}
        onChange={(e) => setForm({ ...form, title: e.target.value })}
        dir="auto"
      />
      <Input
        placeholder="Text"
        value={form.text}
        onChange={(e) => setForm({ ...form, text: e.target.value })}
        dir="auto"
      />
      <Input
        placeholder="Offer condition (note)"
        value={form.note}
        onChange={(e) => setForm({ ...form, note: e.target.value })}
        dir="auto"
      />
      <Input
        placeholder="Call to action"
        value={form.cta}
        onChange={(e) => setForm({ ...form, cta: e.target.value })}
        dir="auto"
      />
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={form.renderAssetId}
        onChange={(e) => setForm({ ...form, renderAssetId: Number(e.target.value) })}
      >
        <option value={0}>No render (black background)</option>
        {renders.data?.assets.map((a) => (
          <option key={a.id} value={a.id}>
            {a.name}
          </option>
        ))}
      </select>
      <label className="block text-muted-foreground">
        Upload a render
        <input
          type="file"
          accept="image/jpeg,image/png,image/webp"
          className="mt-1 block w-full text-xs"
          onChange={(e) => {
            const file = e.target.files?.[0];
            if (file)
              upload.mutate(
                { file, kind: "render", projectId: form.projectId },
                { onSuccess: (a) => setForm((f) => ({ ...f, renderAssetId: a.id })) },
              );
          }}
        />
      </label>
      {upload.error && <p className="text-destructive">{upload.error.message}</p>}
      <Button type="submit" size="sm" disabled={create.isPending}>
        Create draft
      </Button>
      {create.error && <p className="text-destructive">{create.error.message}</p>}
    </form>
  );
}

function CreativeDetail({
  id,
  canEdit,
  isAdmin,
}: {
  id: number;
  canEdit: boolean;
  isAdmin: boolean;
}) {
  const creative = useCreative(id);
  const act = useCreativeAction(id);
  const save = useSaveDocument(id);
  const [note, setNote] = useState("");
  if (creative.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (creative.error) return <p className="text-destructive">{creative.error.message}</p>;
  const { creative: c, versions } = creative.data;
  const latest = versions[versions.length - 1];
  if (!latest) return null;

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-xl font-semibold">{c.name}</h2>
        <p className="text-sm text-muted-foreground">
          {c.format} · {c.language} · version {latest.version} · {stateLabel[latest.reviewState]}
        </p>
      </div>
      {act.error && <p className="text-sm text-destructive">{act.error.message}</p>}
      {save.error && <p className="text-sm text-destructive">{save.error.message}</p>}

      <div className="grid gap-4 md:grid-cols-[1fr_20rem]">
        <div className="rounded-lg border bg-black p-2">
          {latest.output ? (
            <img
              src={`/api/assets/files/${latest.output.key}`}
              alt={`${c.name} version ${latest.version}`}
              className="mx-auto max-h-[32rem]"
            />
          ) : (
            <p className="p-8 text-center text-sm text-muted-foreground">Not rendered yet.</p>
          )}
        </div>
        <div className="space-y-3 text-sm">
          <Checks version={latest} />
          {canEdit && (
            <TextEditor
              version={latest}
              onSave={(doc) => save.mutate(doc)}
              saving={save.isPending}
            />
          )}
          <div className="flex flex-wrap gap-2">
            {canEdit && (latest.reviewState === "draft" || latest.reviewState === "rendered") && (
              <Button
                size="sm"
                onClick={() => act.mutate({ action: "render" })}
                disabled={act.isPending}
              >
                Render
              </Button>
            )}
            {canEdit && latest.reviewState === "rendered" && (
              <Button
                size="sm"
                variant="outline"
                onClick={() => act.mutate({ action: "submit" })}
                disabled={act.isPending}
              >
                Put on review board
              </Button>
            )}
            {isAdmin && latest.reviewState === "in_review" && (
              <>
                <Input
                  placeholder="Review note"
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  className="max-w-xs"
                />
                <Button
                  size="sm"
                  onClick={() =>
                    act.mutate({ action: "review", body: { decision: "approved", note } })
                  }
                >
                  Approve content
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() =>
                    act.mutate({ action: "review", body: { decision: "revise", note } })
                  }
                >
                  Send back
                </Button>
              </>
            )}
            {latest.output && (
              <a href={`/api/creatives/${c.id}/export/${latest.version}`}>
                <Button size="sm" variant="outline">
                  Export SVG
                </Button>
              </a>
            )}
          </div>
          {latest.acceptance && (
            <p className="text-muted-foreground">
              Approved by user {latest.acceptance.acceptedBy} for output{" "}
              {latest.acceptance.ref.sha256.slice(0, 12)}…. Approval is content acceptance; delivery
              is a separate, authorized step.
            </p>
          )}
          <details>
            <summary className="cursor-pointer text-muted-foreground">
              Versions ({versions.length})
            </summary>
            <ul className="mt-1 space-y-1">
              {versions.map((v) => (
                <li key={v.id}>
                  v{v.version} · {stateLabel[v.reviewState]}
                  {v.output ? ` · ${v.output.sha256.slice(0, 12)}…` : ""}
                  {v.reviews.map(
                    (r) => ` · ${r.decision} by ${r.decidedBy}${r.note ? ` (${r.note})` : ""}`,
                  )}
                </li>
              ))}
            </ul>
          </details>
        </div>
      </div>
    </div>
  );
}

function Checks({ version }: { version: Version }) {
  return (
    <ul className="space-y-1">
      {version.checks.map((c) => (
        <li
          key={c.text}
          className={
            c.level === "error"
              ? "text-destructive"
              : c.level === "warn"
                ? "text-foreground"
                : "text-muted-foreground"
          }
        >
          {c.level}: {c.text}
        </li>
      ))}
    </ul>
  );
}

// A text-layer editor: change copy and vertical position. Saving a rendered
// or reviewed version creates a new draft version. The canvas drag editor is
// later work (B3.1).
function TextEditor({
  version,
  onSave,
  saving,
}: {
  version: Version;
  onSave: (d: Document) => void;
  saving: boolean;
}) {
  const [doc, setDoc] = useState<Document>(version.document);
  const layers = doc.layers ?? [];
  return (
    <div className="space-y-2 rounded-lg border p-2">
      <h3 className="font-medium">Text layers</h3>
      {layers.map((l, i) =>
        l.kind === "text" ? (
          <div key={l.id} className="space-y-1">
            <Input
              value={l.text ?? ""}
              dir="auto"
              onChange={(e) => {
                const next = layers.slice();
                next[i] = { ...l, text: e.target.value };
                setDoc({ ...doc, layers: next });
              }}
            />
            <label className="flex items-center gap-2 text-xs text-muted-foreground">
              y
              <input
                type="range"
                min={0}
                max={doc.height}
                value={Math.round(l.y)}
                onChange={(e) => {
                  const next = layers.slice();
                  next[i] = { ...l, y: Number(e.target.value) };
                  setDoc({ ...doc, layers: next });
                }}
              />
              {Math.round(l.y)}
            </label>
          </div>
        ) : null,
      )}
      <Button size="sm" variant="outline" disabled={saving} onClick={() => onSave(doc)}>
        {version.reviewState === "draft" ? "Save draft" : "Save as new version"}
      </Button>
    </div>
  );
}
