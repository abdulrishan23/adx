import { useState } from "react";
import { upload, useAddRenders, useUpdateRender } from "@/api/creative";
import type { CreativeProject, Render } from "@/api/types.legacy";
import { EmptyState, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { fileUrl } from "./files";
import { readImage, toJpeg } from "./image";
import { Dropzone, Progress } from "./ui";

// A project's CGI images. Each upload is stored up to 2400 px wide with a
// 720 px preview, and numbered R01, R02 and on.
export function RendersTab({ project, canEdit }: { project: CreativeProject; canEdit: boolean }) {
  const add = useAddRenders(project.settings.projectId);
  const [progress, setProgress] = useState<number | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const visible = project.renders.filter((r) => !r.hidden);
  const hidden = project.renders.filter((r) => r.hidden);

  async function onFiles(files: File[]) {
    const pictures = files.filter((f) => /^image\/(jpeg|png|webp)$/.test(f.type));
    if (pictures.length === 0) {
      setMessage("Those aren't JPG, PNG or WebP pictures.");
      return;
    }
    setMessage(null);
    setProgress(0);
    try {
      const rows = [];
      for (const [i, f] of pictures.entries()) {
        const { bitmap } = await readImage(f);
        const w = Math.min(bitmap.width, 2400);
        const full = await toJpeg(bitmap, w, Math.round(w * (bitmap.height / bitmap.width)), 0.9);
        const small = await toJpeg(
          bitmap,
          720,
          Math.round(720 * (bitmap.height / bitmap.width)),
          0.8,
        );
        const [a, b] = await Promise.all([upload(full, f.name), upload(small, f.name)]);
        rows.push({
          file: a.sha256,
          preview: b.sha256,
          label: f.name.replace(/\.[A-Za-z0-9]+$/, "").slice(0, 80),
        });
        setProgress(((i + 1) / pictures.length) * 100);
      }
      const res = await add.mutateAsync(rows);
      setMessage(
        `${rows.length} ${rows.length === 1 ? "render" : "renders"} added. There are ${res.renders.length} now.`,
      );
    } catch (e) {
      setMessage(`Upload failed: ${(e as Error).message}`);
    }
    setProgress(null);
  }

  return (
    <div className="space-y-6">
      {canEdit && (
        <Dropzone
          title="Drop renders here"
          hint="JPG, PNG or WebP at any size. Each gets the next number, R01, R02 and on, and its file name becomes the label."
          accept="image/jpeg,image/png,image/webp"
          disabled={progress !== null}
          onFiles={onFiles}
        >
          {progress !== null && <Progress value={progress} label="Uploading renders" />}
          {message && <p className="text-sm">{message}</p>}
        </Dropzone>
      )}
      {visible.length === 0 ? (
        <Panel>
          <EmptyState title="No renders yet">
            Renders are the pictures ads are built on. The editor and the review board both use
            them.
          </EmptyState>
        </Panel>
      ) : (
        <ul className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
          {visible.map((r) => (
            <RenderCard key={r.id} render={r} canEdit={canEdit} />
          ))}
        </ul>
      )}
      {canEdit && hidden.length > 0 && (
        <details className="text-sm">
          <summary className="cursor-pointer text-muted-foreground">
            {hidden.length} hidden {hidden.length === 1 ? "render" : "renders"}
          </summary>
          <ul className="mt-3 flex flex-wrap gap-2">
            {hidden.map((r) => (
              <li key={r.id}>
                <ShowButton render={r} />
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}

function RenderCard({ render, canEdit }: { render: Render; canEdit: boolean }) {
  const update = useUpdateRender();
  const [label, setLabel] = useState(render.label);
  return (
    <li className="space-y-2">
      <a
        href={fileUrl(render.file)}
        target="_blank"
        rel="noreferrer"
        className="block aspect-[3/2] overflow-hidden border bg-muted"
      >
        <img
          src={fileUrl(render.preview)}
          alt={render.code}
          loading="lazy"
          className="size-full object-cover"
        />
      </a>
      <div className="flex items-center gap-2">
        <span className="font-mono text-sm font-medium">{render.code}</span>
        {canEdit ? (
          <Input
            aria-label={`Label for ${render.code}`}
            value={label}
            placeholder="Label"
            className="h-7 text-xs"
            onChange={(e) => setLabel(e.target.value)}
            onBlur={() => {
              if (label !== render.label) update.mutate({ id: render.id, label, hidden: false });
            }}
          />
        ) : (
          <span className="truncate text-xs text-muted-foreground">{render.label}</span>
        )}
        {canEdit && (
          <Button
            size="sm"
            variant="ghost"
            disabled={update.isPending}
            onClick={() => update.mutate({ id: render.id, label: render.label, hidden: true })}
          >
            Hide
          </Button>
        )}
      </div>
    </li>
  );
}

function ShowButton({ render }: { render: Render }) {
  const update = useUpdateRender();
  return (
    <Button
      size="sm"
      variant="outline"
      disabled={update.isPending}
      onClick={() => update.mutate({ id: render.id, label: render.label, hidden: false })}
    >
      Show {render.code}
    </Button>
  );
}
