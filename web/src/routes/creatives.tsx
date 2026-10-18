import { type FormEvent, type PointerEvent as ReactPointerEvent, useRef, useState } from "react";
import {
  useAssets,
  useCreateCreative,
  useCreative,
  useCreativeAction,
  useCreativeQueue,
  useProjectOffers,
  useReadiness,
  useSaveDocument,
  useStartRender,
  useUploadAsset,
} from "@/api/creative";
import { usePlans } from "@/api/plans";
import { useProjects } from "@/api/projects";
import { useSession } from "@/api/session";
import type { Document, Layer, QueueItem, Version } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const stateLabel: Record<string, string> = {
  draft: "Draft",
  rendered: "Rendered",
  in_review: "On the review board",
  approved: "Approved (content only; not delivered)",
  revise: "Sent back",
};

const queueColumns: { state: QueueItem["reviewState"]; title: string }[] = [
  { state: "draft", title: "Drafts" },
  { state: "rendered", title: "Rendered" },
  { state: "in_review", title: "Review board" },
  { state: "revise", title: "Sent back" },
  { state: "approved", title: "Approved content" },
];

// Production: the queue by state, a canvas to lay out copy, a durable render
// job, a review board, and a readiness check that reads the copy against the
// brand's approved claims. Every output is a template layout and says so;
// approval binds one output's checksum and never delivers anything.
export function CreativesPage() {
  const queue = useCreativeQueue();
  const [current, setCurrent] = useState<number | null>(null);
  const session = useSession();
  const canEdit = session.data?.role === "marketer" || session.data?.role === "admin";

  return (
    <section className="space-y-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-3xl font-semibold tracking-tight">Creatives</h1>
        {queue.data && (
          <p className="text-sm text-muted-foreground">
            Renderer: {queue.data.capabilities.renderer} layout.{" "}
            {queue.data.capabilities.aiGeneration.available
              ? `AI generation: ${queue.data.capabilities.aiGeneration.provider}.`
              : "AI image generation: not configured; nothing here is AI-generated."}
          </p>
        )}
      </div>
      {queue.error && <p className="text-sm text-destructive">{queue.error.message}</p>}
      <div className="grid gap-2 md:grid-cols-5">
        {queueColumns.map((col) => {
          const items = queue.data?.items.filter((i) => i.reviewState === col.state) ?? [];
          return (
            <div key={col.state} className="rounded-lg border p-2 text-sm">
              <h2 className="font-semibold">
                {col.title} <span className="text-muted-foreground">{items.length}</span>
              </h2>
              <ul className="mt-1 space-y-1">
                {items.map((i) => (
                  <li key={i.creative.id}>
                    <button
                      type="button"
                      className={`w-full rounded-md px-2 py-1 text-left ${i.creative.id === current ? "bg-muted" : "hover:bg-muted"}`}
                      onClick={() => setCurrent(i.creative.id)}
                    >
                      {i.creative.name}
                      <span className="text-muted-foreground">
                        {" "}
                        · {i.creative.format} · {i.creative.language} · v{i.version}
                      </span>
                      {i.renderError && (
                        <span className="block text-destructive">
                          render failed: {i.renderError}
                        </span>
                      )}
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          );
        })}
      </div>
      <div className="grid gap-6 lg:grid-cols-[18rem_minmax(0,1fr)]">
        <aside>{canEdit && <NewCreative onCreated={setCurrent} />}</aside>
        <div>
          {current === null ? (
            <p className="text-muted-foreground">Pick a creative from the queue or create one.</p>
          ) : (
            <CreativeDetail
              id={current}
              canEdit={canEdit}
              isAdmin={session.data?.role === "admin"}
            />
          )}
        </div>
      </div>
    </section>
  );
}

function NewCreative({ onCreated }: { onCreated: (id: number) => void }) {
  const projects = useProjects();
  const renders = useAssets("render");
  const plans = usePlans();
  const upload = useUploadAsset();
  const create = useCreateCreative();
  const [projectId, setProjectId] = useState(0);
  const offers = useProjectOffers(projectId);
  const [form, setForm] = useState({
    projectId: 0,
    name: "",
    format: "9:16",
    language: "en",
    renderAssetId: 0,
    offerId: 0,
    planId: 0,
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
        offerId: form.offerId,
        planId: form.planId,
        planVersion: 0,
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
      <h2 className="font-semibold">New creative</h2>
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={form.projectId}
        onChange={(e) => {
          setProjectId(Number(e.target.value));
          setForm({ ...form, projectId: Number(e.target.value), offerId: 0 });
        }}
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
            {a.name} ({a.provenance})
          </option>
        ))}
      </select>
      {projectId > 0 && (
        <select
          className="w-full rounded-md border bg-background px-2 py-1"
          value={form.offerId}
          onChange={(e) => setForm({ ...form, offerId: Number(e.target.value) })}
        >
          <option value={0}>No offer</option>
          {offers.data?.offers.map((o) => (
            <option key={o.id} value={o.id}>
              {o.name} (v{o.current.version}, {o.current.state})
            </option>
          ))}
        </select>
      )}
      {projectId > 0 && offers.data?.brandId === 0 && (
        <p className="text-muted-foreground">
          This project is under no brand, so it has no offers or claims.
        </p>
      )}
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={form.planId}
        onChange={(e) => setForm({ ...form, planId: Number(e.target.value) })}
      >
        <option value={0}>Not for a plan</option>
        {plans.data?.plans.map((p) => (
          <option key={p.id} value={p.id}>
            {p.title} (latest v{p.latestVersion})
          </option>
        ))}
      </select>
      <label className="block text-muted-foreground">
        Upload a render (photo)
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
  const render = useStartRender(id);
  const save = useSaveDocument(id);
  const readiness = useReadiness(id);
  const [note, setNote] = useState("");
  const [compareWith, setCompareWith] = useState<number | null>(null);
  if (creative.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (creative.error) return <p className="text-destructive">{creative.error.message}</p>;
  const { creative: c, versions } = creative.data;
  const latest = versions[versions.length - 1];
  if (!latest) return null;
  const other = versions.find((v) => v.version === compareWith);
  const renderQueued = render.data && "run" in render.data ? render.data.run : null;

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-semibold tracking-tight">{c.name}</h2>
        <p className="text-sm text-muted-foreground">
          {c.format} · {c.language} · version {latest.version} · {stateLabel[latest.reviewState]}
          {latest.renderer && ` · ${latest.renderer} layout`}
          {c.offerId && ` · offer ${c.offerId} v${c.offerVersion}`}
          {c.planId && ` · plan ${c.planId} v${c.planVersion}`}
        </p>
      </div>
      {act.error && <p className="text-sm text-destructive">{act.error.message}</p>}
      {render.error && <p className="text-sm text-destructive">{render.error.message}</p>}
      {save.error && <p className="text-sm text-destructive">{save.error.message}</p>}
      {latest.renderError && (
        <p className="text-sm text-destructive">
          Last render failed: {latest.renderError}. Render again to retry.
        </p>
      )}
      {renderQueued && latest.reviewState === "draft" && !latest.output && (
        <p className="text-sm text-muted-foreground">
          Render queued as run {renderQueued.id}; this page refreshes when it finishes.
        </p>
      )}

      <div className="space-y-4">
        <div className="space-y-2">
          {canEdit &&
          (latest.reviewState === "draft" ||
            latest.reviewState === "rendered" ||
            latest.reviewState === "revise" ||
            latest.reviewState === "approved") ? (
            <Canvas
              key={latest.id}
              version={latest}
              onSave={(doc) => save.mutate(doc)}
              saving={save.isPending}
            />
          ) : (
            <Output version={latest} name={c.name} />
          )}
          {other && (
            <div className="grid gap-2 md:grid-cols-2">
              <div>
                <p className="text-sm font-medium">
                  v{latest.version} · {stateLabel[latest.reviewState]}
                </p>
                <Output version={latest} name={c.name} />
                <Checks version={latest} />
              </div>
              <div>
                <p className="text-sm font-medium">
                  v{other.version} · {stateLabel[other.reviewState]}
                </p>
                <Output version={other} name={c.name} />
                <Checks version={other} />
                {other.reviews.map((r) => (
                  <p key={`${r.decidedAt}`} className="text-sm text-muted-foreground">
                    {r.decision} by user {r.decidedBy}
                    {r.note ? `: ${r.note}` : ""}
                  </p>
                ))}
              </div>
            </div>
          )}
        </div>
        <div className="space-y-3 text-sm">
          <Checks version={latest} />
          <div className="flex flex-wrap gap-2">
            {canEdit && (latest.reviewState === "draft" || latest.reviewState === "rendered") && (
              <Button size="sm" onClick={() => render.mutate()} disabled={render.isPending}>
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
          <Readiness readiness={readiness.data} error={readiness.error?.message} />
          <div>
            <p className="font-medium">Versions ({versions.length})</p>
            <ul className="mt-1 space-y-1">
              {versions.map((v) => (
                <li key={v.id} className="flex flex-wrap items-center gap-2">
                  <span>
                    v{v.version} · {stateLabel[v.reviewState]}
                    {v.output ? ` · ${v.output.sha256.slice(0, 12)}…` : ""}
                  </span>
                  {v.version !== latest.version && (
                    <Button
                      size="xs"
                      variant={compareWith === v.version ? "default" : "outline"}
                      onClick={() => setCompareWith(compareWith === v.version ? null : v.version)}
                    >
                      {compareWith === v.version ? "Hide" : "Compare"}
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}

function Output({ version, name }: { version: Version; name: string }) {
  return (
    <div className="rounded-lg border bg-black p-2">
      {version.output ? (
        <img
          src={`/api/assets/files/${version.output.key}`}
          alt={`${name} version ${version.version}`}
          className="mx-auto max-h-[28rem]"
        />
      ) : (
        <p className="p-8 text-center text-sm text-muted-foreground">Not rendered yet.</p>
      )}
    </div>
  );
}

function Checks({ version }: { version: Version }) {
  return (
    <ul className="space-y-1 text-sm">
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

function Readiness({
  readiness,
  error,
}: {
  readiness?: {
    ready: boolean;
    blockers: { code: string; message: string }[];
    warnings: { code: string; message: string }[];
    claims: { text: string; status: string; detail: string }[];
  };
  error?: string;
}) {
  if (error) return <p className="text-destructive">{error}</p>;
  if (!readiness) return null;
  return (
    <div className="rounded-lg border p-2">
      <p className="font-medium">
        {readiness.ready ? "Ready for launch review" : "Not ready for launch"}
      </p>
      <p className="text-muted-foreground">
        Ready means approved content with approved claims. It is not delivery and authorizes no
        spend.
      </p>
      <ul className="mt-1 space-y-1">
        {readiness.blockers.map((b) => (
          <li key={`${b.code}:${b.message}`} className="text-destructive">
            Blocks: {b.message}
          </li>
        ))}
        {readiness.warnings.map((w) => (
          <li key={`${w.code}:${w.message}`} className="text-muted-foreground">
            Note: {w.message}
          </li>
        ))}
      </ul>
      {readiness.claims.length > 0 && (
        <ul className="mt-1 space-y-1">
          {readiness.claims.map((c) => (
            <li key={c.text} className={c.status === "approved" ? "" : "text-destructive"}>
              Claim "{c.text}": {c.status} ({c.detail})
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// Canvas: the document drawn at scale as positioned boxes over the render.
// Drag a text layer to move it; edit its text and size beside the canvas.
// Saving a rendered, reviewed or approved version makes a new draft version
// with no approval of its own.
function Canvas({
  version,
  onSave,
  saving,
}: {
  version: Version;
  onSave: (d: Document) => void;
  saving: boolean;
}) {
  const [doc, setDoc] = useState<Document>(version.document);
  const [selected, setSelected] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);
  const frame = useRef<HTMLDivElement>(null);
  const drag = useRef<{ id: string; startX: number; startY: number; x: number; y: number } | null>(
    null,
  );
  const width = 360;
  const scale = width / doc.width;
  const layers = doc.layers ?? [];

  function update(next: Document) {
    setDoc(next);
    setDirty(true);
  }
  function setLayer(id: string, patch: Partial<Layer>) {
    update({ ...doc, layers: layers.map((l) => (l.id === id ? { ...l, ...patch } : l)) });
  }
  function onPointerDown(e: ReactPointerEvent, l: Layer) {
    if (l.kind !== "text" && l.kind !== "logo") return;
    drag.current = { id: l.id, startX: e.clientX, startY: e.clientY, x: l.x, y: l.y };
    setSelected(l.id);
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  }
  function onPointerMove(e: ReactPointerEvent) {
    const d = drag.current;
    if (!d) return;
    const nx = Math.max(0, Math.min(doc.width, d.x + (e.clientX - d.startX) / scale));
    const ny = Math.max(0, Math.min(doc.height, d.y + (e.clientY - d.startY) / scale));
    setLayer(d.id, { x: Math.round(nx), y: Math.round(ny) });
  }
  function onPointerUp() {
    drag.current = null;
  }
  const sel = layers.find((l) => l.id === selected);

  return (
    <div className="grid gap-3 md:grid-cols-[auto_1fr]">
      <div
        ref={frame}
        className="relative overflow-hidden rounded-lg border"
        style={{ width, height: doc.height * scale, background: doc.background || "#000" }}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
      >
        {doc.image && (
          <img
            src={`/api/assets/files/${doc.image.ref.key}`}
            alt=""
            className="absolute inset-0 h-full w-full object-cover"
            draggable={false}
          />
        )}
        {layers.map((l) => (
          <button
            key={l.id}
            type="button"
            onPointerDown={(e) => onPointerDown(e, l)}
            className={`absolute cursor-move select-none ${selected === l.id ? "outline outline-1 outline-white" : ""}`}
            style={{
              left: l.x * scale,
              top: l.y * scale,
              width: l.width * scale,
              minHeight: (l.height || l.fontSize || 20) * scale,
              color: l.fill || "#fff",
              fontFamily: l.fontFamily,
              fontSize: (l.fontSize || 20) * scale,
              lineHeight: l.lineHeight || 1.2,
              textAlign: (l.align as "left" | "center" | "right") || "left",
              direction: l.rtl ? "rtl" : "ltr",
              textTransform: l.uppercase ? "uppercase" : "none",
              opacity: l.opacity || 1,
            }}
          >
            {l.kind === "text" ? (
              l.text
            ) : l.src ? (
              <img
                src={`/api/assets/files/${l.src.key}`}
                alt=""
                className="h-full w-full object-contain"
                draggable={false}
              />
            ) : null}
          </button>
        ))}
      </div>
      <div className="space-y-2 text-sm">
        <p className="text-muted-foreground">
          Drag a layer to move it. Zones: top 14% and bottom 20% stay clear in 9:16.
        </p>
        {sel && sel.kind === "text" && (
          <div className="space-y-1 rounded-lg border p-2">
            <p className="font-medium">Text layer {sel.id}</p>
            <Input
              value={sel.text ?? ""}
              dir="auto"
              onChange={(e) => setLayer(sel.id, { text: e.target.value })}
            />
            <label className="flex items-center gap-2 text-muted-foreground">
              size
              <input
                type="range"
                min={16}
                max={200}
                value={sel.fontSize || 40}
                onChange={(e) => setLayer(sel.id, { fontSize: Number(e.target.value) })}
              />
              {Math.round(sel.fontSize || 40)}
            </label>
            <label className="flex items-center gap-2 text-muted-foreground">
              y
              <input
                type="range"
                min={0}
                max={doc.height}
                value={Math.round(sel.y)}
                onChange={(e) => setLayer(sel.id, { y: Number(e.target.value) })}
              />
              {Math.round(sel.y)}
            </label>
          </div>
        )}
        {!sel && <p className="text-muted-foreground">Select a layer to edit its text and size.</p>}
        <Button size="sm" variant="outline" disabled={saving || !dirty} onClick={() => onSave(doc)}>
          {version.reviewState === "draft"
            ? "Save draft"
            : `Save as version ${version.version + 1}`}
        </Button>
        {version.reviewState === "approved" && dirty && (
          <p className="text-destructive">
            Saving makes a new version without the approval. The approved version stays as it is.
          </p>
        )}
      </div>
    </div>
  );
}
