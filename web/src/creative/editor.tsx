import { Link } from "@tanstack/react-router";
import type Konva from "konva";
import { ArrowDown, ArrowUp, Trash2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Group,
  Image as KImage,
  Layer as KLayer,
  Text as KText,
  Rect,
  Stage,
  Transformer,
} from "react-konva";
import { createVariation, publishAd, saveDraft, upload, useRefreshCreative } from "@/api/creative";
import type { AdDetail, Render } from "@/api/types.gen";
import { Eyebrow } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { kitFonts, type LogoRole, normalizeKit } from "./brand";
import { checkDoc } from "./checks";
import {
  coverRect,
  type DesignDoc,
  defaultLogo,
  defaultText,
  docFromFlatAd,
  emptyDoc,
  FONT_FAMILIES,
  type ImageLayer,
  type Layer,
  logoTop,
  mapToFormat,
  normalizeDoc,
  type TextLayer,
} from "./doc";
import { DocPreview } from "./doc-preview";
import { fileUrl } from "./files";
import { previewOf } from "./image";
import { FORMATS } from "./naming";
import { fontsUsed, loadGoogleFonts, registerFonts, renderDoc } from "./render";
import { ReviewState, textareaClass } from "./ui";

function useImg(src: string | null) {
  const [state, setState] = useState<{ src: string; img: HTMLImageElement } | null>(null);
  useEffect(() => {
    if (!src) return;
    const i = new window.Image();
    i.crossOrigin = "anonymous";
    i.onload = () => setState({ src, img: i });
    i.src = src;
  }, [src]);
  return state && state.src === src ? state.img : null;
}

function initialDoc(d: AdDetail): DesignDoc {
  const flat = d.ad.current ? fileUrl(d.ad.current.file) : null;
  if (d.draft) return normalizeDoc(d.draft, d.ad.fmt, flat);
  if (d.design) return normalizeDoc(d.design, d.ad.fmt, flat);
  return flat ? docFromFlatAd(d.ad.fmt, flat) : emptyDoc(d.ad.fmt);
}

type Message = { kind: "ok" | "error"; text: string };

// The canvas editor: move and resize text and logos, edit copy, crop the
// render, then save the layers or publish a new version for review.
export function Editor({ detail }: { detail: AdDetail }) {
  const { ad, concept, settings } = detail;
  const projectId = settings.projectId;
  const kit = useMemo(() => normalizeKit(settings.brandKit), [settings.brandKit]);
  const brandFonts = useMemo(() => kitFonts(kit), [kit]);
  const renders = detail.renders.filter((r) => !r.hidden);
  const refresh = useRefreshCreative();

  const [doc, setDoc] = useState<DesignDoc>(() => initialDoc(detail));
  const [selected, setSelected] = useState<string | null>(null);
  const [zones, setZones] = useState(ad.fmt === "9:16");
  const [fontsReady, setFontsReady] = useState(0);
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<Message | null>(null);
  const [dirty, setDirty] = useState(false);
  const [version, setVersion] = useState(ad.current?.version ?? 0);
  const [box, setBox] = useState({ w: 600, h: 600 });
  const [showVariations, setShowVariations] = useState(false);
  const [made, setMade] = useState<Record<string, number>>({});
  const stageRef = useRef<Konva.Stage>(null);
  const trRef = useRef<Konva.Transformer>(null);
  const holder = useRef<HTMLDivElement>(null);
  const nodes = useRef<Record<string, Konva.Node>>({});
  const bg = useImg(doc.image?.src ?? null);

  useEffect(() => {
    const el = holder.current;
    if (!el) return;
    const ro = new ResizeObserver(() =>
      setBox({ w: el.clientWidth - 24, h: Math.max(320, window.innerHeight - 220) }),
    );
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const scale = Math.min(box.w / doc.width, box.h / doc.height, 1);

  useEffect(() => {
    loadGoogleFonts();
    registerFonts(brandFonts);
  }, [brandFonts]);
  useEffect(() => {
    void Promise.all(fontsUsed(doc).map((f) => document.fonts.load(f).catch(() => null))).then(() =>
      setFontsReady((n) => n + 1),
    );
  }, [doc]);

  // The transformer follows the selection.
  // biome-ignore lint/correctness/useExhaustiveDependencies: redraw when layers or fonts change too
  useEffect(() => {
    const tr = trRef.current;
    if (!tr) return;
    const node = selected ? nodes.current[selected] : null;
    tr.nodes(node ? [node] : []);
    tr.getLayer()?.batchDraw();
  }, [selected, doc.layers, fontsReady]);

  // Leaving with unsaved layers asks first.
  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  const update = useCallback((fn: (d: DesignDoc) => DesignDoc) => {
    setDoc(fn);
    setDirty(true);
  }, []);
  const patchLayer = (id: string, patch: Partial<Layer>) =>
    update((d) => ({
      ...d,
      layers: d.layers.map((l) => (l.id === id ? ({ ...l, ...patch } as Layer) : l)),
    }));
  const sel = doc.layers.find((l) => l.id === selected) ?? null;

  function addLayer(l: Layer) {
    update((d) => ({ ...d, layers: [...d.layers, l] }));
    setSelected(l.id);
  }
  function removeLayer(id: string) {
    update((d) => ({ ...d, layers: d.layers.filter((l) => l.id !== id) }));
    setSelected(null);
  }
  function moveLayer(id: string, dir: -1 | 1) {
    update((d) => {
      const i = d.layers.findIndex((l) => l.id === id);
      const j = i + dir;
      if (i < 0 || j < 0 || j >= d.layers.length) return d;
      const layers = [...d.layers];
      [layers[i], layers[j]] = [layers[j] as Layer, layers[i] as Layer];
      return { ...d, layers };
    });
  }
  function addBrandLogo(role: LogoRole) {
    const a = kit.logos[role];
    if (!a) return;
    const ratio = a.width && a.height ? a.width / a.height : 3;
    const l = defaultLogo(doc, fileUrl(a.sha256), ratio);
    if (role === "developer") {
      l.width = Math.round(doc.width * 0.16);
      l.height = Math.round(l.width / ratio);
      l.x = Math.round((doc.width - l.width) / 2);
      l.y = (doc.fmt === "9:16" ? Math.round(doc.height * 0.8) : doc.height) - l.height - 40;
    }
    addLayer(l);
  }
  async function addOtherPicture(file: File) {
    setBusy("logo");
    try {
      const ref = await upload(file, file.name);
      const bmp = await createImageBitmap(file).catch(() => null);
      addLayer(defaultLogo(doc, fileUrl(ref), bmp ? bmp.width / bmp.height : 3));
    } catch (e) {
      setMessage({ kind: "error", text: `Upload failed: ${(e as Error).message}` });
    }
    setBusy(null);
  }
  function pickRender(r: Render) {
    update((d) => ({
      ...d,
      image: {
        src: fileUrl(r.file),
        renderId: r.code,
        fx: d.image?.fx ?? 0.5,
        fy: d.image?.fy ?? 0.5,
        zoom: d.image?.zoom ?? 1,
      },
    }));
  }

  // The full-size picture, with guides and handles hidden.
  async function drawCanvas(): Promise<Blob> {
    const stage = stageRef.current;
    if (!stage) throw new Error("The canvas isn't ready");
    await Promise.all(fontsUsed(doc).map((f) => document.fonts.load(f).catch(() => null)));
    const guides = stage.findOne<Konva.Layer>("#guides");
    guides?.hide();
    trRef.current?.hide();
    const dataUrl = stage.toDataURL({
      pixelRatio: 1 / scale,
      mimeType: "image/jpeg",
      quality: 0.92,
    });
    guides?.show();
    trRef.current?.show();
    return (await fetch(dataUrl)).blob();
  }

  async function run(what: string, fn: () => Promise<Message>) {
    setBusy(what);
    setMessage(null);
    try {
      setMessage(await fn());
    } catch (e) {
      setMessage({ kind: "error", text: (e as Error).message });
    }
    setBusy(null);
  }

  const save = () =>
    run("save", async () => {
      await saveDraft(ad.id, { design: doc, renderCode: doc.image?.renderId ?? "" });
      setDirty(false);
      return { kind: "ok", text: "Layout saved. Reviewers still see the published picture." };
    });

  const publish = () =>
    run("publish", async () => {
      const full = await drawCanvas();
      const [a, b] = await Promise.all([upload(full), previewOf(full).then((p) => upload(p))]);
      const res = await publishAd(ad.id, {
        file: a.sha256,
        preview: b.sha256,
        design: doc,
        renderCode: doc.image?.renderId ?? "",
        source: "editor",
      });
      setVersion(res.current?.version ?? version + 1);
      setDirty(false);
      void refresh();
      return {
        kind: "ok",
        text: `Published as version ${res.current?.version}. Its review starts again.`,
      };
    });

  const download = () =>
    run("download", async () => {
      const blob = await drawCanvas();
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = ad.fileName;
      a.click();
      setTimeout(() => URL.revokeObjectURL(a.href), 10_000);
      return { kind: "ok", text: "Downloaded." };
    });

  const copyToSizes = () =>
    run("copy", async () => {
      for (const s of detail.siblings) {
        await saveDraft(s.id, {
          design: mapToFormat(doc, s.fmt),
          renderCode: doc.image?.renderId ?? "",
        });
      }
      void refresh();
      const n = detail.siblings.length;
      return {
        kind: "ok",
        text: `Layout copied to ${n} other ${n === 1 ? "size" : "sizes"}. Open each to check it and publish.`,
      };
    });

  const makeVariation = (r: Render) =>
    run(`var-${r.code}`, async () => {
      const vdoc: DesignDoc = {
        ...doc,
        image: {
          src: fileUrl(r.file),
          renderId: r.code,
          fx: doc.image?.fx ?? 0.5,
          fy: doc.image?.fy ?? 0.5,
          zoom: doc.image?.zoom ?? 1,
        },
      };
      const created = await createVariation(ad.id, { renderCode: r.code, design: vdoc });
      const full = await renderDoc(vdoc);
      const [a, b] = await Promise.all([upload(full), previewOf(full).then((p) => upload(p))]);
      await publishAd(created.id, {
        file: a.sha256,
        preview: b.sha256,
        design: vdoc,
        renderCode: r.code,
        source: "editor",
      });
      setMade((m) => ({ ...m, [r.code]: created.id }));
      void refresh();
      return { kind: "ok", text: `${created.code} made and on the review board.` };
    });

  const checks = useMemo(() => checkDoc(doc, { lang: ad.lang }), [doc, ad.lang]);
  const bgRect = useMemo(
    () =>
      bg && doc.image
        ? coverRect(
            doc.width,
            doc.height,
            bg.width,
            bg.height,
            doc.image.fx,
            doc.image.fy,
            doc.image.zoom,
          )
        : null,
    [bg, doc.image, doc.width, doc.height],
  );
  const f = FORMATS[doc.fmt];

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
      <div className="min-w-0 space-y-3">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
          <Link
            to="/projects/$projectId"
            params={{ projectId: String(projectId) }}
            search={{ tab: "concepts" }}
            className="text-muted-foreground hover:text-foreground"
          >
            ← {settings.projectName}
          </Link>
          <span className="font-mono text-xs">{ad.code}</span>
          <span className="text-xs text-muted-foreground">
            {f.label} · {doc.width}×{doc.height}
            {version > 0 ? ` · v${version}` : " · not published"}
          </span>
          <ReviewState status={ad.status} />
          {ad.lang === "ar" && <span className="text-xs">Arabic, needs native review</span>}
          {dirty && <span className="text-xs text-muted-foreground">Unsaved changes</span>}
        </div>
        <div ref={holder} className="border bg-muted/50 p-3" style={{ minHeight: 320 }}>
          <div
            style={{ width: doc.width * scale, height: doc.height * scale }}
            className="mx-auto shadow-[0_0_0_1px_rgb(0_0_0/0.1)]"
          >
            <Stage
              ref={stageRef}
              width={doc.width * scale}
              height={doc.height * scale}
              scaleX={scale}
              scaleY={scale}
              onMouseDown={(e) => {
                if (e.target === e.target.getStage() || e.target.name() === "bg") setSelected(null);
              }}
            >
              <KLayer>
                <Rect
                  name="bg"
                  x={0}
                  y={0}
                  width={doc.width}
                  height={doc.height}
                  fill={doc.background}
                />
                {bg && bgRect && <KImage name="bg" image={bg} {...bgRect} />}
              </KLayer>
              <KLayer>
                {doc.layers.map((l) => (
                  <LayerNode
                    key={l.id}
                    layer={l}
                    register={(n) => {
                      if (n) nodes.current[l.id] = n;
                      else delete nodes.current[l.id];
                    }}
                    onSelect={() => setSelected(l.id)}
                    onChange={(patch) => patchLayer(l.id, patch)}
                  />
                ))}
                <Transformer
                  ref={trRef}
                  rotateEnabled={false}
                  keepRatio={sel?.type === "image"}
                  enabledAnchors={
                    sel?.type === "text"
                      ? [
                          "middle-left",
                          "middle-right",
                          "top-left",
                          "top-right",
                          "bottom-left",
                          "bottom-right",
                        ]
                      : undefined
                  }
                  boundBoxFunc={(o, n) => (n.width < 20 || n.height < 10 ? o : n)}
                />
              </KLayer>
              <KLayer id="guides" listening={false}>
                {zones && doc.fmt === "9:16" && (
                  <Group>
                    <Rect
                      x={0}
                      y={0}
                      width={doc.width}
                      height={doc.height * 0.14}
                      fill="rgba(200,60,40,.3)"
                    />
                    <Rect
                      x={0}
                      y={doc.height * 0.8}
                      width={doc.width}
                      height={doc.height * 0.2}
                      fill="rgba(200,60,40,.3)"
                    />
                    <KText
                      x={0}
                      y={doc.height * 0.14 - 40}
                      width={doc.width}
                      align="center"
                      text="TOP 14%: KEEP CLEAR"
                      fontSize={24}
                      fill="#fff"
                      fontFamily="Inter"
                    />
                    <KText
                      x={0}
                      y={doc.height * 0.8 + 16}
                      width={doc.width}
                      align="center"
                      text="BOTTOM 20%: KEEP CLEAR"
                      fontSize={24}
                      fill="#fff"
                      fontFamily="Inter"
                    />
                  </Group>
                )}
              </KLayer>
            </Stage>
          </div>
        </div>
        {message && (
          <p
            className={cn("text-sm", message.kind === "error" && "text-destructive")}
            role="status"
          >
            {message.text}
          </p>
        )}
      </div>

      <aside className="space-y-4 text-sm">
        <Section>
          <div className="flex flex-wrap gap-2">
            <Button size="sm" onClick={() => void publish()} disabled={!!busy}>
              {busy === "publish" ? "Publishing…" : "Publish for review"}
            </Button>
            <Button size="sm" variant="outline" onClick={() => void save()} disabled={!!busy}>
              {busy === "save" ? "Saving…" : "Save layout"}
            </Button>
            <Button size="sm" variant="ghost" onClick={() => void download()} disabled={!!busy}>
              Download JPG
            </Button>
          </div>
          <p className="text-xs text-muted-foreground">
            Save keeps the layers. Publish makes a new version that reviewers see, and its review
            starts again.
          </p>
        </Section>

        <Section title="House rules">
          {checks.length === 0 ? (
            <p className="text-xs text-muted-foreground">Every check passes.</p>
          ) : (
            <ul className="space-y-1.5 text-xs">
              {checks.map((c) => (
                <li key={c.text} className="flex gap-2">
                  <span
                    aria-hidden="true"
                    className={cn(
                      "mt-1 inline-block size-1.5 shrink-0",
                      c.level === "error"
                        ? "bg-status-error"
                        : c.level === "warn"
                          ? "bg-status-caution"
                          : "border border-current",
                    )}
                  />
                  {c.text}
                </li>
              ))}
            </ul>
          )}
        </Section>

        <Section title="Other sizes">
          <div className="flex flex-wrap gap-2">
            {detail.siblings.map((s) => (
              <Link
                key={s.id}
                to="/creative/$projectId/ads/$adId/edit"
                params={{ projectId: String(projectId), adId: String(s.id) }}
                className="border px-2 py-1 text-xs hover:bg-muted"
              >
                {s.fmt}
                {!s.hasDraft && " · no layout yet"}
              </Link>
            ))}
            {detail.siblings.length === 0 && (
              <span className="text-xs text-muted-foreground">
                No other sizes of this variant yet.
              </span>
            )}
          </div>
          {detail.siblings.length > 0 && (
            <Button size="sm" variant="ghost" disabled={!!busy} onClick={() => void copyToSizes()}>
              Copy this layout to the other sizes
            </Button>
          )}
        </Section>

        <Section title="Variations">
          <p className="text-xs text-muted-foreground">
            {renders.length === 0
              ? "Add renders to the project first."
              : "See this layout on every render, and make any of them its own ad."}
          </p>
          {renders.length > 0 && (
            <Button size="sm" variant="outline" onClick={() => setShowVariations((v) => !v)}>
              {showVariations ? "Hide" : "Preview on every render"}
            </Button>
          )}
          {doc.layers.length === 0 && renders.length > 0 && (
            <p className="bg-muted p-2 text-xs">
              This ad is a flat picture with its text baked in, so another render shows up bare. Add
              the headline and logo as layers first.
            </p>
          )}
          {showVariations && (
            <ul className="grid grid-cols-2 gap-3">
              {renders.map((r) => (
                <li key={r.code} className="space-y-1">
                  <DocPreview
                    doc={{
                      ...doc,
                      image: {
                        src: fileUrl(r.preview),
                        renderId: r.code,
                        fx: doc.image?.fx ?? 0.5,
                        fy: doc.image?.fy ?? 0.5,
                        zoom: doc.image?.zoom ?? 1,
                      },
                    }}
                    width={150}
                    className="overflow-hidden border bg-muted"
                  />
                  <div className="flex items-center justify-between gap-1">
                    <span className="font-mono text-xs">{r.code}</span>
                    {made[r.code] ? (
                      <Link
                        to="/creative/$projectId/ads/$adId/edit"
                        params={{ projectId: String(projectId), adId: String(made[r.code]) }}
                        className="text-xs underline"
                      >
                        Open
                      </Link>
                    ) : doc.image?.renderId === r.code ? (
                      <span className="text-xs text-muted-foreground">current</span>
                    ) : (
                      <Button
                        size="xs"
                        variant="ghost"
                        disabled={!!busy}
                        onClick={() => void makeVariation(r)}
                      >
                        {busy === `var-${r.code}` ? "…" : "Make"}
                      </Button>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Section>

        <Section title="Background">
          {doc.image ? (
            <>
              <Slider
                label="Focus left to right"
                value={doc.image.fx}
                min={0}
                max={1}
                step={0.01}
                onChange={(v) => update((d) => ({ ...d, image: d.image && { ...d.image, fx: v } }))}
              />
              <Slider
                label="Focus top to bottom"
                value={doc.image.fy}
                min={0}
                max={1}
                step={0.01}
                onChange={(v) => update((d) => ({ ...d, image: d.image && { ...d.image, fy: v } }))}
              />
              <Slider
                label="Zoom"
                value={doc.image.zoom}
                min={1}
                max={3}
                step={0.01}
                onChange={(v) =>
                  update((d) => ({ ...d, image: d.image && { ...d.image, zoom: v } }))
                }
              />
            </>
          ) : (
            <p className="text-xs text-muted-foreground">No picture. Choose a render below.</p>
          )}
          {renders.length > 0 && (
            <div className="flex gap-2 overflow-x-auto pb-1">
              {renders.map((r) => (
                <button
                  key={r.code}
                  type="button"
                  title={`${r.code} ${r.label}`}
                  aria-pressed={doc.image?.renderId === r.code}
                  onClick={() => pickRender(r)}
                  className={cn(
                    "h-12 w-18 shrink-0 overflow-hidden border",
                    doc.image?.renderId === r.code &&
                      "outline-2 outline-offset-1 outline-foreground",
                  )}
                >
                  <img src={fileUrl(r.preview)} alt={r.code} className="size-full object-cover" />
                </button>
              ))}
            </div>
          )}
          <Link
            to="/projects/$projectId"
            params={{ projectId: String(projectId) }}
            search={{ tab: "renders" }}
            className="text-xs underline-offset-4 hover:underline"
          >
            Manage renders
          </Link>
          <label className="flex items-center gap-2 text-xs">
            Background colour
            <input
              type="color"
              value={doc.background}
              onChange={(e) => update((d) => ({ ...d, background: e.target.value }))}
              className="h-6 w-8 cursor-pointer border bg-transparent"
            />
          </label>
          {doc.fmt === "9:16" && (
            <label className="flex items-center gap-2 text-xs">
              <input type="checkbox" checked={zones} onChange={(e) => setZones(e.target.checked)} />
              Show the 9:16 keep-clear zones
            </label>
          )}
        </Section>

        <Section title="Layers">
          <div className="flex flex-wrap gap-1.5">
            <Button
              size="xs"
              variant="outline"
              onClick={() =>
                addLayer(defaultText(doc, concept.headline || "Your headline", ad.lang === "ar"))
              }
            >
              Text
            </Button>
            {kit.logos.onDark && (
              <Button size="xs" variant="outline" onClick={() => addBrandLogo("onDark")}>
                Logo for dark
              </Button>
            )}
            {kit.logos.onLight && (
              <Button size="xs" variant="outline" onClick={() => addBrandLogo("onLight")}>
                Logo for light
              </Button>
            )}
            {kit.logos.developer && (
              <Button size="xs" variant="outline" onClick={() => addBrandLogo("developer")}>
                Developer logo
              </Button>
            )}
            <label className={cn("inline-flex", busy && "pointer-events-none opacity-50")}>
              <span className="inline-flex h-6 cursor-pointer items-center border px-2 text-xs font-medium hover:bg-muted">
                Other picture
              </span>
              <input
                type="file"
                accept="image/png,image/svg+xml,image/webp,image/jpeg"
                className="sr-only"
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  if (file) void addOtherPicture(file);
                  e.target.value = "";
                }}
              />
            </label>
          </div>
          {doc.layers.length === 0 && (
            <p className="text-xs text-muted-foreground">No layers yet.</p>
          )}
          <ul className="space-y-0.5">
            {[...doc.layers].reverse().map((l) => (
              <li key={l.id}>
                <button
                  type="button"
                  onClick={() => setSelected(l.id)}
                  className={cn(
                    "w-full truncate px-2 py-1 text-left text-xs",
                    selected === l.id ? "bg-muted font-medium" : "hover:bg-muted/60",
                  )}
                >
                  {l.type === "text"
                    ? `Text: ${l.text.slice(0, 30) || "(empty)"}`
                    : l.kind === "logo"
                      ? "Logo"
                      : "Picture"}
                </button>
              </li>
            ))}
          </ul>
        </Section>

        {sel && (
          <Section
            title={sel.type === "text" ? "Text" : sel.kind === "logo" ? "Logo" : "Picture"}
            action={
              <div className="flex">
                <Button
                  size="icon-xs"
                  variant="ghost"
                  aria-label="Bring forward"
                  onClick={() => moveLayer(sel.id, 1)}
                >
                  <ArrowUp />
                </Button>
                <Button
                  size="icon-xs"
                  variant="ghost"
                  aria-label="Send back"
                  onClick={() => moveLayer(sel.id, -1)}
                >
                  <ArrowDown />
                </Button>
                <Button
                  size="icon-xs"
                  variant="ghost"
                  aria-label="Delete layer"
                  onClick={() => removeLayer(sel.id)}
                >
                  <Trash2 />
                </Button>
              </div>
            }
          >
            {sel.type === "text" ? (
              <TextPanel
                l={sel}
                onChange={(p) => patchLayer(sel.id, p)}
                brandFonts={brandFonts.map((x) => x.family)}
                swatches={[kit.colors.inkLight, kit.colors.inkDark, kit.colors.accent]}
              />
            ) : (
              <ImagePanel l={sel} doc={doc} onChange={(p) => patchLayer(sel.id, p)} />
            )}
          </Section>
        )}
      </aside>
    </div>
  );
}

function Section({
  title,
  action,
  children,
}: {
  title?: string;
  action?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section className="space-y-2 border p-3">
      {(title || action) && (
        <div className="flex items-center justify-between gap-2">
          {title && <Eyebrow>{title}</Eyebrow>}
          {action}
        </div>
      )}
      {children}
    </section>
  );
}

function LayerNode({
  layer,
  register,
  onSelect,
  onChange,
}: {
  layer: Layer;
  register: (n: Konva.Node | null) => void;
  onSelect: () => void;
  onChange: (p: Partial<Layer>) => void;
}) {
  const img = useImg(layer.type === "image" ? layer.src : null);
  const common = {
    id: layer.id,
    x: layer.x,
    y: layer.y,
    draggable: true,
    onClick: onSelect,
    onTap: onSelect,
    onDragEnd: (e: Konva.KonvaEventObject<DragEvent>) =>
      onChange({ x: Math.round(e.target.x()), y: Math.round(e.target.y()) }),
  };
  if (layer.type === "text") {
    return (
      <KText
        ref={register}
        {...common}
        text={layer.uppercase ? layer.text.toUpperCase() : layer.text}
        width={layer.width}
        fontFamily={layer.fontFamily}
        fontSize={layer.fontSize}
        fontStyle={layer.fontStyle}
        fill={layer.fill}
        align={layer.align}
        lineHeight={layer.lineHeight}
        letterSpacing={layer.letterSpacing}
        {...({ direction: layer.rtl ? "rtl" : "ltr" } as object)}
        onTransformEnd={(e) => {
          const n = e.target as Konva.Text;
          const sx = n.scaleX();
          n.scaleX(1);
          n.scaleY(1);
          onChange({
            x: Math.round(n.x()),
            y: Math.round(n.y()),
            width: Math.max(20, Math.round(n.width() * sx)),
            fontSize: Math.max(8, Math.round(layer.fontSize * sx)),
          });
        }}
      />
    );
  }
  if (!img) return null;
  return (
    <KImage
      ref={register}
      {...common}
      image={img}
      width={layer.width}
      height={layer.height}
      opacity={layer.opacity}
      onTransformEnd={(e) => {
        const n = e.target;
        const w = Math.round(n.width() * n.scaleX());
        const h = Math.round(n.height() * n.scaleY());
        n.scaleX(1);
        n.scaleY(1);
        onChange({ x: Math.round(n.x()), y: Math.round(n.y()), width: w, height: h });
      }}
    />
  );
}

function TextPanel({
  l,
  onChange,
  brandFonts,
  swatches,
}: {
  l: TextLayer;
  onChange: (p: Partial<TextLayer>) => void;
  brandFonts: string[];
  swatches: string[];
}) {
  const bold = l.fontStyle.includes("bold");
  const italic = l.fontStyle.includes("italic");
  const style = (b: boolean, i: boolean): TextLayer["fontStyle"] =>
    b && i ? "bold italic" : b ? "bold" : i ? "italic" : "normal";
  const families = [...new Set([...brandFonts, ...FONT_FAMILIES, l.fontFamily])];
  return (
    <div className="space-y-3">
      <textarea
        aria-label="Text"
        value={l.text}
        rows={3}
        dir={l.rtl ? "rtl" : "ltr"}
        onChange={(e) => onChange({ text: e.target.value })}
        className={textareaClass}
      />
      <div className="grid grid-cols-[1fr_5rem] gap-2">
        <Select
          aria-label="Font"
          value={l.fontFamily}
          onValueChange={(fontFamily) => onChange({ fontFamily })}
          options={families.map((f) => ({
            value: f,
            label: brandFonts.includes(f) ? `${f} · brand` : f,
          }))}
        />
        <Input
          aria-label="Size"
          type="number"
          min={8}
          max={600}
          value={l.fontSize}
          onChange={(e) => onChange({ fontSize: Number(e.target.value) || 8 })}
        />
      </div>
      <div className="flex flex-wrap items-center gap-1.5">
        <Button
          size="xs"
          variant={bold ? "default" : "outline"}
          onClick={() => onChange({ fontStyle: style(!bold, italic) })}
        >
          Bold
        </Button>
        <Button
          size="xs"
          variant={italic ? "default" : "outline"}
          onClick={() => onChange({ fontStyle: style(bold, !italic) })}
        >
          Italic
        </Button>
        {(["left", "center", "right"] as const).map((a) => (
          <Button
            key={a}
            size="xs"
            variant={l.align === a ? "default" : "outline"}
            onClick={() => onChange({ align: a })}
          >
            {a === "center" ? "Centre" : a === "left" ? "Left" : "Right"}
          </Button>
        ))}
      </div>
      <div className="flex items-center gap-1.5">
        {swatches.map((c) => (
          <button
            key={c}
            type="button"
            title={c}
            aria-label={`Colour ${c}`}
            onClick={() => onChange({ fill: c })}
            className="size-6 border"
            style={{ background: c }}
          />
        ))}
        <input
          type="color"
          aria-label="Text colour"
          value={l.fill}
          onChange={(e) => onChange({ fill: e.target.value })}
          className="h-6 w-8 cursor-pointer border bg-transparent"
        />
      </div>
      <Slider
        label="Line height"
        value={l.lineHeight}
        min={0.8}
        max={2}
        step={0.02}
        onChange={(v) => onChange({ lineHeight: v })}
      />
      <Slider
        label="Letter spacing"
        value={l.letterSpacing}
        min={-5}
        max={40}
        step={0.5}
        onChange={(v) => onChange({ letterSpacing: v })}
      />
      <div className="flex gap-4 text-xs">
        <label className="flex items-center gap-1.5">
          <input
            type="checkbox"
            checked={l.uppercase}
            onChange={(e) => onChange({ uppercase: e.target.checked })}
          />
          Capitals
        </label>
        <label className="flex items-center gap-1.5">
          <input
            type="checkbox"
            checked={l.rtl}
            onChange={(e) => onChange({ rtl: e.target.checked })}
          />
          Right to left
        </label>
      </div>
      <p className="text-xs text-muted-foreground">
        House rule: no text shadows or panels. Put copy on calm sky.
      </p>
    </div>
  );
}

function ImagePanel({
  l,
  doc,
  onChange,
}: {
  l: ImageLayer;
  doc: DesignDoc;
  onChange: (p: Partial<ImageLayer>) => void;
}) {
  return (
    <div className="space-y-3">
      <Slider
        label="Opacity"
        value={l.opacity}
        min={0.1}
        max={1}
        step={0.05}
        onChange={(v) => onChange({ opacity: v })}
      />
      <Slider
        label="Width"
        value={l.width}
        min={40}
        max={doc.width}
        step={1}
        onChange={(v) => onChange({ width: v, height: Math.round(v * (l.height / l.width)) })}
      />
      {l.kind === "logo" && (
        <Button
          size="xs"
          variant="outline"
          onClick={() => onChange({ x: Math.round((doc.width - l.width) / 2), y: logoTop(doc) })}
        >
          Move to top centre
        </Button>
      )}
    </div>
  );
}

function Slider({
  label,
  value,
  min,
  max,
  step,
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  onChange: (v: number) => void;
}) {
  return (
    <label className="block">
      <span className="flex justify-between text-xs text-muted-foreground">
        <span>{label}</span>
        <span className="font-mono">{Number.isInteger(step) ? value : value.toFixed(2)}</span>
      </span>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full accent-foreground"
      />
    </label>
  );
}
