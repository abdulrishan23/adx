import { Link } from "@tanstack/react-router";
import { Plus, Sparkles, X } from "lucide-react";
import { type ReactNode, useEffect, useMemo, useState } from "react";
import {
  publishAd,
  saveGenerated,
  upload,
  useCreativeProject,
  useGenerateImagery,
  useRefreshCreative,
  useSaveConcept,
  useStartVideo,
} from "@/api/creative";
import type { Concept, CreativeProject } from "@/api/types.gen";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { kitFonts, normalizeKit } from "@/creative/brand";
import { checkDoc } from "@/creative/checks";
import { type ConceptCopy, type CopyBlocks, hasCopy, normalizeCopy } from "@/creative/copy";
import type { DesignDoc } from "@/creative/doc";
import { DocPreview } from "@/creative/doc-preview";
import { fileUrl } from "@/creative/files";
import { previewOf } from "@/creative/image";
import { layoutDoc } from "@/creative/layout";
import { FMT_ORDER, type Fmt, type Lang, slugFromCode, slugFromName } from "@/creative/naming";
import { loadGoogleFonts, registerFonts, renderDoc } from "@/creative/render";
import { Field, Progress, textareaClass } from "@/creative/ui";
import { useRun } from "@/engine/api/runs";
import { cn } from "@/lib/utils";

export function CreativeConceptPage({
  projectId,
  conceptId,
}: {
  projectId: number;
  conceptId: number;
}) {
  const project = useCreativeProject(projectId);
  if (project.isPending) return <LoadingRows rows={8} />;
  if (project.error) return <ErrorNote error={project.error} />;
  const concept = project.data.concepts.find((c) => c.id === conceptId);
  if (!concept) return <ErrorNote error={new Error("That concept isn't in this project.")} />;
  return <ConceptEditor key={concept.id} project={project.data} concept={concept} />;
}

type Fields = {
  name: string;
  why: string;
  primaryText: string;
  headline: string;
  description: string;
  cta: string;
  copy: ConceptCopy;
};

function ConceptEditor({ project, concept }: { project: CreativeProject; concept: Concept }) {
  const projectId = project.settings.projectId;
  const save = useSaveConcept(concept.id);
  const [f, setF] = useState<Fields>(() => ({
    name: concept.name,
    why: concept.why,
    primaryText: concept.primaryText,
    headline: concept.headline,
    description: concept.description,
    cta: concept.cta,
    copy: normalizeCopy(concept.copy),
  }));
  const [dirty, setDirty] = useState(false);
  const set = (patch: Partial<Fields>) => {
    setF((x) => ({ ...x, ...patch }));
    setDirty(true);
  };
  const setCopy = (patch: Partial<ConceptCopy>) => set({ copy: { ...f.copy, ...patch } });
  const setBlocks = (lang: Lang, patch: Partial<CopyBlocks>) =>
    setCopy({ [lang]: { ...f.copy[lang], ...patch } });
  const renders = project.renders.filter((r) => !r.hidden);
  // Generated ads share the slug of the concept's existing ads, so an upload
  // and a generated set name the same ad the same way.
  const slug =
    concept.ads.map((a) => slugFromCode(a.code, concept.code)).find(Boolean) ??
    slugFromName(f.name);

  return (
    <section>
      <Link
        to="/creative/$projectId"
        params={{ projectId: String(projectId) }}
        className="text-sm text-muted-foreground hover:text-foreground"
      >
        ← {project.settings.projectName}
      </Link>
      <PageHeader
        className="mt-2"
        title={
          <>
            <span className="font-mono text-muted-foreground">{concept.code}</span> {f.name}
          </>
        }
        subtitle={
          <>
            Write the copy once. Generated ads are named like{" "}
            <span className="font-mono">
              {project.settings.prefix}_{concept.code}_{slug}_EN_9x16.jpg
            </span>
            .
          </>
        }
        actions={
          <>
            {dirty && <span className="text-sm text-muted-foreground">Unsaved changes</span>}
            <Button
              disabled={save.isPending || !dirty}
              onClick={() =>
                save.mutate({ ...f, copy: f.copy }, { onSuccess: () => setDirty(false) })
              }
            >
              {save.isPending ? "Saving…" : "Save copy"}
            </Button>
          </>
        }
      />
      {save.error && <ErrorNote error={save.error} />}

      <div className="space-y-6">
        <Panel title="Concept">
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Name" htmlFor="name">
              <Input id="name" value={f.name} onChange={(e) => set({ name: e.target.value })} />
            </Field>
            <Field
              label="Why this concept"
              htmlFor="why"
              hint="Reviewers see it under the concept's title."
            >
              <Input id="why" value={f.why} onChange={(e) => set({ why: e.target.value })} />
            </Field>
            <PictureIt concept={concept} />
            <MakeVideo concept={concept} project={project} />
            <div className="space-y-2 md:col-span-2">
              <p className="text-sm font-medium">Render behind generated ads</p>
              <div className="flex gap-2 overflow-x-auto pb-1">
                <RenderChoice
                  selected={f.copy.render === ""}
                  onClick={() => setCopy({ render: "" })}
                >
                  <span className="text-xs">None</span>
                </RenderChoice>
                {renders.map((r) => (
                  <RenderChoice
                    key={r.code}
                    selected={f.copy.render === r.code}
                    onClick={() => setCopy({ render: r.code })}
                    title={`${r.code} ${r.label}`}
                  >
                    <img src={fileUrl(r.preview)} alt={r.code} className="size-full object-cover" />
                  </RenderChoice>
                ))}
                {renders.length === 0 && (
                  <span className="self-center text-xs text-muted-foreground">
                    No renders yet. Add them on the project's Renders tab.
                  </span>
                )}
              </div>
            </div>
          </div>
        </Panel>

        <div className="grid gap-6 lg:grid-cols-2">
          <CopyEditor
            title="English copy"
            lang="en"
            b={f.copy.en}
            onChange={(p) => setBlocks("en", p)}
          />
          <CopyEditor
            title="Arabic copy"
            lang="ar"
            b={f.copy.ar}
            onChange={(p) => setBlocks("ar", p)}
            extra={
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={f.copy.arReviewed}
                  onChange={(e) => setCopy({ arReviewed: e.target.checked })}
                />
                A native speaker has reviewed this Arabic
              </label>
            }
          />
        </div>

        <Panel
          title="Meta ad text"
          description="Shown around the picture in the review board's phone frames."
        >
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Primary text" htmlFor="primary">
              <textarea
                id="primary"
                rows={4}
                value={f.primaryText}
                onChange={(e) => set({ primaryText: e.target.value })}
                className={textareaClass}
              />
            </Field>
            <div className="space-y-4">
              <Field label="Headline" htmlFor="headline">
                <Input
                  id="headline"
                  value={f.headline}
                  onChange={(e) => set({ headline: e.target.value })}
                />
              </Field>
              <Field label="Description" htmlFor="desc">
                <Input
                  id="desc"
                  value={f.description}
                  onChange={(e) => set({ description: e.target.value })}
                />
              </Field>
              <Field label="Button" htmlFor="button">
                <Input
                  id="button"
                  value={f.cta}
                  placeholder="Learn more"
                  onChange={(e) => set({ cta: e.target.value })}
                />
              </Field>
            </div>
          </div>
        </Panel>

        <Generate project={project} concept={concept} slug={slug} copy={f.copy} dirty={dirty} />
      </div>
    </section>
  );
}

// Adx pictures the concept from its imagery prompt; the pictures join the
// project's renders, so the choice below fills up as they arrive.
function PictureIt({ concept }: { concept: Concept }) {
  const [prompt, setPrompt] = useState(concept.imagery);
  const [count, setCount] = useState(2);
  const [useRenders, setUseRenders] = useState(true);
  const generate = useGenerateImagery(concept.id);
  return (
    <div className="space-y-2 md:col-span-2">
      <Field
        label="Picture"
        htmlFor="imagery"
        hint="What the picture shows: scene, light, mood. Copy is laid over it, so leave the words out."
      >
        <textarea
          id="imagery"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          rows={2}
          className={textareaClass}
          placeholder="Golden hour over the lagoon villas, calm water in the foreground"
        />
      </Field>
      <div className="flex flex-wrap items-center gap-3">
        <Button
          size="sm"
          disabled={generate.isPending || prompt.trim() === ""}
          onClick={() => generate.mutate({ count, prompt, aspectRatio: "9:16", useRenders })}
        >
          <Sparkles />
          {generate.isPending ? "Picturing" : "Picture it"}
        </Button>
        <Input
          aria-label="How many pictures"
          type="number"
          min={1}
          max={4}
          value={count}
          onChange={(e) => setCount(Number(e.target.value))}
          className="w-16"
        />
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          <input
            type="checkbox"
            checked={useRenders}
            onChange={(e) => setUseRenders(e.target.checked)}
          />
          Take the cue from the project's renders
        </label>
        {generate.data && (
          <span className="text-sm text-muted-foreground">
            {generate.data.renders.length} added as{" "}
            {generate.data.renders.map((r) => r.code).join(", ")}.
          </span>
        )}
      </div>
      {generate.error && <p className="text-sm text-destructive">{generate.error.message}</p>}
      {generate.data?.notes.map((n) => (
        <p key={n} className="text-sm text-muted-foreground">
          {n}
        </p>
      ))}
    </div>
  );
}

// A video ad per language: motion moves the chosen renders and ends on the
// finished story ad; the video model grows a clip from the first render and
// the concept's picture prompt. Either is made in the background and lands
// on the project page as an ad to review.
function MakeVideo({ concept, project }: { concept: Concept; project: CreativeProject }) {
  const [mode, setMode] = useState<"motion" | "model">("motion");
  const [lang, setLang] = useState<Lang>("en");
  const [picked, setPicked] = useState<string[]>([]);
  const start = useStartVideo(concept.id);
  const refresh = useRefreshCreative();
  const run = useRun(start.data?.run.id ?? "");
  const state = run.data?.state;
  useEffect(() => {
    if (state === "succeeded" || state === "failed") refresh();
  }, [state, refresh]);
  const renders = project.renders.filter((r) => !r.hidden);
  const toggle = (code: string) =>
    setPicked((xs) => (xs.includes(code) ? xs.filter((x) => x !== code) : [...xs, code]));
  return (
    <div className="space-y-2 md:col-span-2">
      <p className="text-sm font-medium">Video</p>
      <div className="flex flex-wrap items-center gap-3">
        <Segmented
          label="How"
          value={mode}
          onChange={setMode}
          options={[
            { value: "motion", label: "Motion from renders" },
            { value: "model", label: "Veo clip" },
          ]}
        />
        <Segmented
          label="Language"
          value={lang}
          onChange={setLang}
          options={[
            { value: "en", label: "English" },
            { value: "ar", label: "Arabic" },
          ]}
        />
        <Button
          size="sm"
          disabled={start.isPending || state === "queued" || state === "running"}
          onClick={() => start.mutate({ mode, lang, renderCodes: picked, prompt: "", slug: "" })}
        >
          <Sparkles />
          {state === "queued" || state === "running" ? "Making" : "Make video"}
        </Button>
      </div>
      {mode === "motion" && renders.length > 0 && (
        <div className="flex gap-2 overflow-x-auto pb-1">
          {renders.map((r) => (
            <RenderChoice
              key={r.code}
              selected={picked.includes(r.code)}
              onClick={() => toggle(r.code)}
              title={`${r.code} ${r.label}`}
            >
              <img src={fileUrl(r.preview)} alt={r.code} className="size-full object-cover" />
            </RenderChoice>
          ))}
        </div>
      )}
      <p className="text-xs text-muted-foreground">
        {mode === "motion"
          ? "Pick the renders to move, in order, or leave them for Adx to choose. The finished story ad in that language closes the video."
          : "Veo grows an eight-second clip from the first render and the picture prompt above."}
      </p>
      {start.error && <p className="text-sm text-destructive">{start.error.message}</p>}
      {run.data && (
        <p className="text-sm text-muted-foreground">
          {state === "succeeded"
            ? "Done. The video is on the project page, waiting for review."
            : state === "failed"
              ? `Failed: ${run.data.error ?? "unknown"}`
              : `${state}…`}
        </p>
      )}
    </div>
  );
}

function RenderChoice({
  selected,
  onClick,
  title,
  children,
}: {
  selected: boolean;
  onClick: () => void;
  title?: string;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      title={title}
      aria-pressed={selected}
      onClick={onClick}
      className={cn(
        "grid h-14 w-21 shrink-0 place-items-center overflow-hidden border",
        selected ? "outline-2 outline-offset-1 outline-foreground" : "hover:border-foreground",
      )}
    >
      {children}
    </button>
  );
}

const BLOCKS: { k: keyof CopyBlocks; label: string; hint?: string; rows?: number }[] = [
  {
    k: "eyebrow",
    label: "Eyebrow",
    hint: "The small line above the title, such as FOR CASH BUYERS.",
  },
  {
    k: "title",
    label: "Title",
    hint: "A new line is a line break. About 16 characters a line at XL, 22 at M.",
    rows: 2,
  },
  {
    k: "text",
    label: "Text",
    hint: "The sub-line. Every discount needs its condition here or in the small print.",
    rows: 2,
  },
  { k: "event", label: "Event line", hint: "Day, date, time and venue for launch events." },
  { k: "note", label: "Note" },
  { k: "info", label: "Small print", hint: "Such as T&C apply, or prices from AED 1.2M." },
  { k: "cta", label: "Button, on the With CTA ads", hint: "Such as Register your interest." },
];

function CopyEditor({
  title,
  lang,
  b,
  onChange,
  extra,
}: {
  title: string;
  lang: Lang;
  b: CopyBlocks;
  onChange: (p: Partial<CopyBlocks>) => void;
  extra?: ReactNode;
}) {
  const dir = lang === "ar" ? "rtl" : "ltr";
  return (
    <Panel
      title={title}
      action={
        lang === "ar" ? (
          <span className="text-xs text-muted-foreground">Needs native review</span>
        ) : undefined
      }
    >
      <div className="space-y-4">
        {BLOCKS.map(({ k, label, hint, rows }) => {
          if (k === "stats" || k === "titleSize") return null;
          const id = `${lang}-${k}`;
          return (
            <div
              key={k}
              className={cn(k === "title" && "grid grid-cols-[1fr_auto] items-end gap-3")}
            >
              <Field label={label} htmlFor={id} hint={hint}>
                {rows ? (
                  <textarea
                    id={id}
                    dir={dir}
                    lang={lang}
                    rows={rows}
                    value={b[k]}
                    onChange={(e) => onChange({ [k]: e.target.value })}
                    className={textareaClass}
                  />
                ) : (
                  <Input
                    id={id}
                    dir={dir}
                    lang={lang}
                    value={b[k]}
                    onChange={(e) => onChange({ [k]: e.target.value })}
                  />
                )}
              </Field>
              {k === "title" && (
                <Segmented
                  label="Title size"
                  value={b.titleSize}
                  onChange={(titleSize) => onChange({ titleSize })}
                  options={[
                    { value: "xl", label: "XL" },
                    { value: "lg", label: "L" },
                    { value: "md", label: "M" },
                  ]}
                  className="mb-5"
                />
              )}
            </div>
          );
        })}
        <StatsEditor stats={b.stats} dir={dir} onChange={(stats) => onChange({ stats })} />
        {extra}
      </div>
    </Panel>
  );
}

function StatsEditor({
  stats,
  dir,
  onChange,
}: {
  stats: CopyBlocks["stats"];
  dir: "rtl" | "ltr";
  onChange: (s: CopyBlocks["stats"]) => void;
}) {
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <p className="text-sm font-medium">Figures</p>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => onChange([...stats, { figure: "", label: "" }])}
        >
          <Plus />
          Figure
        </Button>
      </div>
      {stats.length === 0 && (
        <p className="text-xs text-muted-foreground">Such as 10% with Down payment.</p>
      )}
      {stats.map((s, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: figures have no id and are edited in place
        <div key={i} className="grid grid-cols-[1fr_1fr_auto] gap-2">
          <Input
            aria-label="Figure"
            dir={dir}
            value={s.figure}
            placeholder="10%"
            onChange={(e) =>
              onChange(stats.map((x, j) => (j === i ? { ...x, figure: e.target.value } : x)))
            }
          />
          <Input
            aria-label="Label"
            dir={dir}
            value={s.label}
            placeholder="Down payment"
            onChange={(e) =>
              onChange(stats.map((x, j) => (j === i ? { ...x, label: e.target.value } : x)))
            }
          />
          <Button
            size="icon"
            variant="ghost"
            aria-label="Remove figure"
            onClick={() => onChange(stats.filter((_, j) => j !== i))}
          >
            <X />
          </Button>
        </div>
      ))}
    </div>
  );
}

type Plan = { fmt: Fmt; lang: Lang; cta: boolean; doc: DesignDoc };
type Result = { status: "published" | "kept" | "failed"; adId?: number; note?: string };

// Builds every size and variant from the saved copy, shows them, then makes
// the ads and publishes their pictures.
function Generate({
  project,
  concept,
  slug,
  copy,
  dirty,
}: {
  project: CreativeProject;
  concept: Concept;
  slug: string;
  copy: ConceptCopy;
  dirty: boolean;
}) {
  const kit = useMemo(() => normalizeKit(project.settings.brandKit), [project.settings.brandKit]);
  const refresh = useRefreshCreative();
  const [plans, setPlans] = useState<Plan[] | null>(null);
  const [results, setResults] = useState<Result[]>([]);
  const [progress, setProgress] = useState<number | null>(null);
  const [replace, setReplace] = useState(false);
  const render = project.renders.find((r) => r.code === copy.render && !r.hidden) ?? null;
  const langs = (["en", "ar"] as Lang[]).filter((l) => hasCopy(copy[l]));
  const hasLogo = !!(kit.logos.onDark || kit.logos.onLight);

  useEffect(() => {
    loadGoogleFonts();
    registerFonts(kitFonts(kit));
  }, [kit]);

  function preview() {
    const out: Plan[] = [];
    for (const lang of langs) {
      for (const fmt of FMT_ORDER) {
        for (const cta of [false, true]) {
          if (cta && !copy[lang].cta.trim()) continue;
          out.push({
            fmt,
            lang,
            cta,
            doc: layoutDoc({
              fmt,
              lang,
              cta,
              copy: copy[lang],
              kit,
              render: render ? { url: fileUrl(render.file), renderId: render.code } : null,
            }),
          });
        }
      }
    }
    setPlans(out);
    setResults([]);
  }

  async function generate() {
    if (!plans) return;
    setProgress(0);
    const out: Result[] = [];
    for (const [i, p] of plans.entries()) {
      try {
        const saved = await saveGenerated(concept.id, {
          fmt: p.fmt,
          lang: p.lang,
          cta: p.cta,
          slug,
          renderCode: render?.code ?? "",
          design: p.doc,
        });
        if (saved.hadPicture && !replace) {
          out.push({ status: "kept", adId: saved.ad.id, note: "layout saved, picture kept" });
        } else {
          const full = await renderDoc(p.doc, "image/jpeg", 0.92);
          const [a, b] = await Promise.all([upload(full), previewOf(full).then((x) => upload(x))]);
          await publishAd(saved.ad.id, {
            file: a.sha256,
            preview: b.sha256,
            design: p.doc,
            renderCode: render?.code ?? "",
            source: "generated",
          });
          out.push({ status: "published", adId: saved.ad.id });
        }
      } catch (e) {
        out.push({ status: "failed", note: (e as Error).message });
      }
      setResults([...out]);
      setProgress(((i + 1) / plans.length) * 100);
    }
    setProgress(null);
    void refresh();
  }

  const running = progress !== null;
  return (
    <Panel
      title="Generate ads from this copy"
      description={
        hasLogo
          ? "All four sizes, without and with the CTA, in English and Arabic where there's copy."
          : "Add a logo and fonts in the brand kit first, or the ads come out plain."
      }
      action={
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Button variant="outline" disabled={langs.length === 0 || running} onClick={preview}>
            {plans ? "Rebuild the preview" : "Preview all sizes"}
          </Button>
          {plans && (
            <Button disabled={running || dirty} onClick={() => void generate()}>
              {running ? "Making ads…" : `Make ${plans.length} ads`}
            </Button>
          )}
        </div>
      }
    >
      <div className="space-y-4">
        {langs.length === 0 && (
          <p className="text-sm text-muted-foreground">
            Write at least a title in English or Arabic first.
          </p>
        )}
        {dirty && plans && (
          <p className="text-sm text-muted-foreground">Save the copy before making the ads.</p>
        )}
        {plans && (
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={replace}
              onChange={(e) => setReplace(e.target.checked)}
            />
            Replace the pictures of ads that already have one. Their review starts again.
          </label>
        )}
        {!render && langs.length > 0 && (
          <p className="text-xs text-muted-foreground">
            No render chosen, so the ads get a plain background.
          </p>
        )}
        {running && <Progress value={progress ?? 0} label="Making ads" />}
        {plans && (
          <ul className="grid grid-cols-2 gap-5 sm:grid-cols-3 lg:grid-cols-4">
            {plans.map((p, i) => {
              const checks = checkDoc(p.doc, { lang: p.lang, arReviewed: copy.arReviewed }).filter(
                (c) => c.level !== "info",
              );
              const res = results[i];
              return (
                <li key={`${p.lang}-${p.fmt}-${p.cta}`} className="space-y-1.5">
                  <DocPreview doc={p.doc} width={200} className="overflow-hidden border bg-muted" />
                  <p className="text-xs">
                    <span className="font-mono">{p.fmt}</span> · {p.lang.toUpperCase()} ·{" "}
                    {p.cta ? "With CTA" : "No CTA"}
                  </p>
                  {checks.map((c) => (
                    <p key={c.text} className="text-[0.7rem] text-muted-foreground">
                      <span
                        className={cn(
                          "mr-1 inline-block size-1.5 align-middle",
                          c.level === "error" ? "bg-status-error" : "bg-status-caution",
                        )}
                      />
                      {c.text}
                    </p>
                  ))}
                  {res && (
                    <p className="text-[0.7rem] text-muted-foreground">
                      {res.status === "published"
                        ? "Published"
                        : res.status === "kept"
                          ? "Layout saved, picture kept"
                          : `Failed: ${res.note}`}
                      {res.adId && (
                        <>
                          {" · "}
                          <Link
                            to="/creative/$projectId/ads/$adId/edit"
                            params={{
                              projectId: String(project.settings.projectId),
                              adId: String(res.adId),
                            }}
                            className="underline"
                          >
                            Edit
                          </Link>
                        </>
                      )}
                    </p>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </Panel>
  );
}
