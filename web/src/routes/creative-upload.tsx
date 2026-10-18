import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { upload, useAddUploads, useCreativeProject } from "@/api/creative";
import type { UploadedAd } from "@/api/types.legacy";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { StatusChip } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { previewOf, readImage, toJpeg } from "@/creative/image";
import {
  adSortKey,
  buildFileName,
  compareConcepts,
  detectFormat,
  type Fmt,
  FORMATS,
  type Lang,
  parseAdFilename,
} from "@/creative/naming";
import { Dropzone, Progress } from "@/creative/ui";
import { cn } from "@/lib/utils";

type Item = {
  file: File;
  status: "checking" | "ready" | "skip";
  problem?: string;
  note?: string;
  concept?: string;
  slug?: string;
  lang?: Lang;
  fmt?: Fmt;
  cta?: boolean;
  bitmap?: ImageBitmap;
};

// Upload finished ads. The pixels decide each one's size and the file name
// its concept, language and CTA. Nothing is saved until the check table has
// been looked at.
export function CreativeUploadPage({ projectId }: { projectId: number }) {
  const project = useCreativeProject(projectId);
  const save = useAddUploads(projectId);
  const navigate = useNavigate();
  const [items, setItems] = useState<Item[]>([]);
  const [phase, setPhase] = useState<"pick" | "review" | "uploading" | "done">("pick");
  const [progress, setProgress] = useState(0);
  const [message, setMessage] = useState<string | null>(null);

  if (project.isPending) return <LoadingRows />;
  if (project.error) return <ErrorNote error={project.error} />;
  const prefix = project.data.settings.prefix;

  async function analyse(files: File[]) {
    const list: Item[] = files.map((file) => ({ file, status: "checking" }));
    setItems([...list]);
    setPhase("review");
    for (const [i, it] of list.entries()) {
      const parsed = parseAdFilename(it.file.name, prefix);
      if (!parsed) {
        list[i] = {
          ...it,
          status: "skip",
          problem: "The name has no concept id, like C1_CASH_9x16.jpg",
        };
      } else {
        try {
          const { bitmap, width, height } = await readImage(it.file);
          const det = detectFormat(width, height);
          if (!det) {
            list[i] = {
              ...it,
              status: "skip",
              problem: `${width}×${height} isn't one of the four ad sizes`,
            };
          } else {
            const notes: string[] = [];
            if (!det.exact) {
              notes.push(
                `${width}×${height} will be resized to ${FORMATS[det.fmt].width}×${FORMATS[det.fmt].height}`,
              );
            }
            if (parsed.fmt && parsed.fmt !== det.fmt) {
              notes.push(
                `the name says ${parsed.fmt} but the picture is ${det.fmt}, so ${det.fmt} it is`,
              );
            }
            list[i] = {
              ...it,
              status: "ready",
              bitmap,
              concept: parsed.concept,
              slug: parsed.slug,
              lang: parsed.lang,
              fmt: det.fmt,
              cta: parsed.cta,
              note: notes.join(". ") || undefined,
            };
          }
        } catch {
          list[i] = { ...it, status: "skip", problem: "This picture couldn't be read" };
        }
      }
      setItems([...list]);
    }
    // Two files that would become the same ad: keep the first.
    const seen = new Set<string>();
    for (const [i, it] of list.entries()) {
      if (it.status !== "ready") continue;
      const key = `${it.concept}|${it.slug}|${it.lang}|${it.fmt}|${it.cta}`;
      if (seen.has(key))
        list[i] = { ...it, status: "skip", problem: "Another file here is the same ad" };
      seen.add(key);
    }
    setItems([...list]);
  }

  async function send() {
    const ready = items
      .filter((i) => i.status === "ready")
      .sort(
        (a, b) =>
          compareConcepts(a.concept ?? "", b.concept ?? "") ||
          adSortKey({ lang: a.lang ?? "en", fmt: a.fmt ?? "9:16", cta: !!a.cta }) -
            adSortKey({ lang: b.lang ?? "en", fmt: b.fmt ?? "9:16", cta: !!b.cta }),
      );
    if (ready.length === 0) return;
    setPhase("uploading");
    setMessage(null);
    setProgress(0);
    const rows: UploadedAd[] = [];
    try {
      for (const [i, it] of ready.entries()) {
        if (!it.bitmap || !it.fmt || !it.concept || !it.slug || !it.lang) continue;
        const { width, height } = FORMATS[it.fmt];
        const full = await toJpeg(it.bitmap, width, height, 0.86);
        const [a, b] = await Promise.all([
          upload(full, it.file.name),
          previewOf(full).then((p) => upload(p)),
        ]);
        rows.push({
          concept: it.concept,
          slug: it.slug,
          lang: it.lang,
          fmt: it.fmt,
          cta: !!it.cta,
          file: a.sha256,
          preview: b.sha256,
        });
        setProgress(((i + 1) / ready.length) * 95);
      }
      const res = await save.mutateAsync(rows);
      setProgress(100);
      setPhase("done");
      setMessage(
        [
          `Saved ${res.versions} ${res.versions === 1 ? "picture" : "pictures"}`,
          res.ads > 0 ? `${res.ads} new ${res.ads === 1 ? "ad" : "ads"}` : null,
          res.concepts > 0
            ? `${res.concepts} new ${res.concepts === 1 ? "concept" : "concepts"}`
            : null,
        ]
          .filter(Boolean)
          .join(", ") +
          ". Ads that already existed got a new version, so their review starts again.",
      );
    } catch (e) {
      setPhase("review");
      setMessage(`Upload failed: ${(e as Error).message}`);
    }
  }

  const ready = items.filter((i) => i.status === "ready").length;
  const skipped = items.filter((i) => i.status === "skip").length;
  const checking = items.some((i) => i.status === "checking");
  const sorted = [...items].sort(
    (a, b) => Number(a.status === "skip") - Number(b.status === "skip"),
  );

  return (
    <section>
      <Link
        to="/creative/$projectId"
        params={{ projectId: String(projectId) }}
        className="text-sm text-muted-foreground hover:text-foreground"
      >
        ← {project.data.settings.projectName}
      </Link>
      <PageHeader
        className="mt-2"
        title="Upload ads"
        subtitle="Each picture's pixels decide its size, and its file name the concept, language and CTA. Uploading an ad that exists adds a new version."
      />
      {phase === "pick" ? (
        <Dropzone
          title="Drop finished ads here"
          hint={
            <>
              JPG, PNG or WebP, as many as you like, named like{" "}
              <span className="font-mono">{prefix}_C1_CASH_EN_9x16_CTA.jpg</span>.
            </>
          }
          accept="image/jpeg,image/png,image/webp"
          onFiles={(files) =>
            void analyse(files.filter((f) => /^image\/(jpeg|png|webp)$/.test(f.type)))
          }
        />
      ) : (
        <Panel
          title="Check before uploading"
          description="Skipped files are left out. Nothing else stops the upload."
          action={
            <div className="flex flex-wrap items-center gap-2">
              <StatusChip status="ready">{ready} ready</StatusChip>
              {skipped > 0 && <StatusChip status="caution">{skipped} skipped</StatusChip>}
              {phase === "review" && (
                <>
                  <Button
                    variant="ghost"
                    onClick={() => {
                      setItems([]);
                      setPhase("pick");
                      setMessage(null);
                    }}
                  >
                    Start over
                  </Button>
                  <Button disabled={ready === 0 || checking} onClick={() => void send()}>
                    Upload {ready} {ready === 1 ? "ad" : "ads"}
                  </Button>
                </>
              )}
              {phase === "done" && (
                <Button
                  onClick={() =>
                    navigate({
                      to: "/creative/$projectId",
                      params: { projectId: String(projectId) },
                    })
                  }
                >
                  Back to the project
                </Button>
              )}
            </div>
          }
          bodyClassName="p-0"
        >
          {(phase === "uploading" || message) && (
            <div className="space-y-2 border-b px-5 py-3">
              {phase === "uploading" && <Progress value={progress} label="Uploading ads" />}
              {message && (
                <p className={cn("text-sm", phase !== "done" && "text-destructive")}>{message}</p>
              )}
            </div>
          )}
          <Table>
            <THead>
              <TR>
                <TH>File</TH>
                <TH>Concept</TH>
                <TH>Size</TH>
                <TH>Variant</TH>
                <TH>Saved as</TH>
                <TH>Note</TH>
              </TR>
            </THead>
            <TBody>
              {sorted.map((it) => (
                <TR key={it.file.name} className={cn(it.status === "skip" && "opacity-60")}>
                  <TD className="font-mono text-xs">{it.file.name}</TD>
                  <TD>
                    {it.concept ? (
                      <>
                        <span className="font-mono">{it.concept}</span>{" "}
                        {it.slug?.replace(/_/g, " ")}
                      </>
                    ) : (
                      "–"
                    )}
                  </TD>
                  <TD>{it.fmt ? FORMATS[it.fmt].label : it.status === "checking" ? "…" : "–"}</TD>
                  <TD>
                    {it.status === "ready"
                      ? `${it.cta ? "With CTA" : "No CTA"}${it.lang === "ar" ? " · Arabic" : ""}`
                      : "–"}
                  </TD>
                  <TD className="font-mono text-xs">
                    {it.status === "ready" && it.concept && it.slug && it.lang && it.fmt
                      ? buildFileName(prefix, {
                          concept: it.concept,
                          slug: it.slug,
                          lang: it.lang,
                          fmt: it.fmt,
                          cta: !!it.cta,
                        })
                      : "–"}
                  </TD>
                  <TD className="text-xs text-muted-foreground">{it.problem ?? it.note ?? ""}</TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </Panel>
      )}
    </section>
  );
}
