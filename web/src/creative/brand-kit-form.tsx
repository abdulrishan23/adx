import { useEffect, useState } from "react";
import { upload, useSaveBrandKit } from "@/api/creative";
import type { Settings } from "@/api/types.gen";
import { Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  type BrandKit,
  FONT_ROLES,
  type FontRole,
  familyFromFile,
  LOGO_ROLES,
  type LogoRole,
  normalizeKit,
} from "./brand";
import { fileUrl } from "./files";
import { registerFonts } from "./render";
import { textareaClass } from "./ui";

const FONT_FILE = /\.(ttf|otf|woff2?)$/i;

const COLOURS = [
  ["inkLight", "Light ink", "Text on photos and dark areas"],
  ["inkDark", "Dark ink", "Text on sky and light areas, and the plain background"],
  ["accent", "Accent", "Buttons and highlights"],
] as const;

// The project's logos, fonts and colours. Generated ads are built from it.
export function BrandKitForm({ settings, canEdit }: { settings: Settings; canEdit: boolean }) {
  const save = useSaveBrandKit(settings.projectId);
  const [kit, setKit] = useState<BrandKit>(() => normalizeKit(settings.brandKit));
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    registerFonts(
      Object.values(kit.fonts).flatMap((f) =>
        f ? [{ family: f.family, url: fileUrl(f.sha256) }] : [],
      ),
    );
  }, [kit.fonts]);

  const change = (fn: (k: BrandKit) => BrandKit) => {
    setKit(fn);
    setDirty(true);
  };

  async function onLogo(role: LogoRole, file: File) {
    setBusy(role);
    setMessage(null);
    try {
      const ref = await upload(file, file.name);
      let width = ref.width ?? undefined;
      let height = ref.height ?? undefined;
      if (!width) {
        try {
          const b = await createImageBitmap(file);
          width = b.width;
          height = b.height;
        } catch {
          // An SVG without a size: layouts assume a wide logo.
        }
      }
      change((k) => ({
        ...k,
        logos: { ...k.logos, [role]: { sha256: ref.sha256, width, height, name: file.name } },
      }));
    } catch (e) {
      setMessage(`Upload failed: ${(e as Error).message}`);
    }
    setBusy(null);
  }

  async function onFont(role: FontRole, file: File) {
    setMessage(null);
    if (!FONT_FILE.test(file.name)) {
      setMessage("Use a TTF, OTF, WOFF or WOFF2 file.");
      return;
    }
    setBusy(role);
    try {
      const ref = await upload(file, file.name);
      change((k) => ({
        ...k,
        fonts: { ...k.fonts, [role]: { family: familyFromFile(file.name), sha256: ref.sha256 } },
      }));
    } catch (e) {
      setMessage(`Upload failed: ${(e as Error).message}`);
    }
    setBusy(null);
  }

  return (
    <div className="space-y-6">
      <Panel title="Logos">
        <div className="grid gap-6 md:grid-cols-3">
          {LOGO_ROLES.map(({ role, label, hint }) => {
            const a = kit.logos[role];
            return (
              <div key={role} className="space-y-2">
                <p className="text-sm font-medium">{label}</p>
                <div
                  className={`flex h-28 items-center justify-center border p-3 ${role === "onLight" ? "bg-white" : "bg-neutral-900"}`}
                >
                  {a ? (
                    <img
                      src={fileUrl(a.sha256)}
                      alt={label}
                      className="max-h-full max-w-full object-contain"
                    />
                  ) : (
                    <span className="text-xs text-neutral-500">None yet</span>
                  )}
                </div>
                <p className="text-xs text-muted-foreground">{hint}</p>
                {canEdit && (
                  <FilePick
                    label={a ? "Replace" : "Upload"}
                    accept="image/png,image/svg+xml,image/webp"
                    disabled={!!busy}
                    busy={busy === role}
                    onFile={(f) => void onLogo(role, f)}
                  />
                )}
              </div>
            );
          })}
        </div>
      </Panel>

      <Panel title="Fonts">
        <div className="grid gap-6 md:grid-cols-2">
          {FONT_ROLES.map(({ role, label, hint }) => {
            const f = kit.fonts[role];
            return (
              <div key={role} className="space-y-2">
                <p className="text-sm font-medium">{label}</p>
                {f ? (
                  <div className="flex items-center gap-3">
                    <Input
                      aria-label={`${label} family`}
                      value={f.family}
                      disabled={!canEdit}
                      onChange={(e) =>
                        change((k) => ({
                          ...k,
                          fonts: { ...k.fonts, [role]: { ...f, family: e.target.value } },
                        }))
                      }
                    />
                    <span
                      className="text-xl whitespace-nowrap"
                      style={{ fontFamily: `"${f.family}"` }}
                    >
                      Aa Bb 123
                    </span>
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">None yet</p>
                )}
                <p className="text-xs text-muted-foreground">{hint}</p>
                {canEdit && (
                  <FilePick
                    label={f ? "Replace" : "Upload"}
                    accept=".ttf,.otf,.woff,.woff2"
                    disabled={!!busy}
                    busy={busy === role}
                    onFile={(file) => void onFont(role, file)}
                  />
                )}
              </div>
            );
          })}
        </div>
      </Panel>

      <Panel title="Colours">
        <div className="grid gap-6 sm:grid-cols-3">
          {COLOURS.map(([key, label, hint]) => (
            <div key={key} className="space-y-1.5">
              <label htmlFor={`colour-${key}`} className="text-sm font-medium">
                {label}
              </label>
              <div className="flex items-center gap-2">
                <input
                  id={`colour-${key}`}
                  type="color"
                  value={kit.colors[key]}
                  disabled={!canEdit}
                  onChange={(e) =>
                    change((k) => ({ ...k, colors: { ...k.colors, [key]: e.target.value } }))
                  }
                  className="h-8 w-10 cursor-pointer border bg-transparent"
                />
                <span className="font-mono text-sm">{kit.colors[key]}</span>
              </div>
              <p className="text-xs text-muted-foreground">{hint}</p>
            </div>
          ))}
        </div>
      </Panel>

      <Panel title="Notes for designers">
        <textarea
          aria-label="Notes for designers"
          value={kit.notes}
          disabled={!canEdit}
          rows={3}
          onChange={(e) => change((k) => ({ ...k, notes: e.target.value }))}
          placeholder="Clear space around the logo, tone of voice, anything the team should know."
          className={textareaClass}
        />
      </Panel>

      {canEdit && (
        <div className="flex flex-wrap items-center gap-3">
          <Button
            disabled={!dirty || save.isPending || !!busy}
            onClick={() => save.mutate(kit, { onSuccess: () => setDirty(false) })}
          >
            {save.isPending ? "Saving…" : "Save brand kit"}
          </Button>
          {dirty && <span className="text-sm text-muted-foreground">Unsaved changes</span>}
          {save.error && <span className="text-sm text-destructive">{save.error.message}</span>}
          {message && <span className="text-sm text-destructive">{message}</span>}
        </div>
      )}
    </div>
  );
}

function FilePick({
  label,
  accept,
  disabled,
  busy,
  onFile,
}: {
  label: string;
  accept: string;
  disabled: boolean;
  busy: boolean;
  onFile: (f: File) => void;
}) {
  return (
    <label className={`inline-flex ${disabled ? "pointer-events-none opacity-50" : ""}`}>
      <span className="inline-flex h-7 cursor-pointer items-center border px-2.5 text-xs font-medium hover:bg-muted">
        {busy ? "Uploading…" : label}
      </span>
      <input
        type="file"
        accept={accept}
        className="sr-only"
        disabled={disabled}
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) onFile(f);
          e.target.value = "";
        }}
      />
    </label>
  );
}
