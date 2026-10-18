// The brand kit: what every ad of a project is built from. The server stores
// it as JSON per project; normalizeKit makes any stored shape safe to use.
import { fileUrl } from "./files";

export type BrandAsset = { sha256: string; width?: number; height?: number; name?: string };
export type BrandFont = { family: string; sha256: string };
export type FontRole = "title" | "text" | "label" | "arTitle" | "arText";
export type LogoRole = "onDark" | "onLight" | "developer";

export type BrandKit = {
  v: 1;
  logos: Partial<Record<LogoRole, BrandAsset>>;
  fonts: Partial<Record<FontRole, BrandFont>>;
  colors: { inkLight: string; inkDark: string; accent: string };
  notes: string;
};

export const EMPTY_KIT: BrandKit = {
  v: 1,
  logos: {},
  fonts: {},
  colors: { inkLight: "#ffffff", inkDark: "#1f2328", accent: "#c9a96a" },
  notes: "",
};

export const LOGO_ROLES: { role: LogoRole; label: string; hint: string }[] = [
  {
    role: "onDark",
    label: "Logo on dark",
    hint: "Light wordmark, transparent PNG or SVG. Used on photos and dark backgrounds.",
  },
  {
    role: "onLight",
    label: "Logo on light",
    hint: "Dark wordmark, transparent PNG or SVG. Used on sky and light backgrounds.",
  },
  {
    role: "developer",
    label: "Developer logo",
    hint: "Optional, such as the Cosmo logo on Marlin Icon. Goes bottom centre.",
  },
];

export const FONT_ROLES: { role: FontRole; label: string; hint: string }[] = [
  { role: "title", label: "Title font", hint: "Headlines. TTF, OTF, WOFF or WOFF2." },
  { role: "text", label: "Text font", hint: "Body lines and sub-lines." },
  { role: "label", label: "Label font", hint: "Eyebrows, buttons and small caps." },
  { role: "arTitle", label: "Arabic title font", hint: "Headlines in Arabic." },
  { role: "arText", label: "Arabic text font", hint: "Body lines in Arabic." },
];

const HEX = /^#[0-9a-fA-F]{6}$/;
const SHA = /^[0-9a-f]{64}$/;

function asset(v: unknown): BrandAsset | undefined {
  if (!v || typeof v !== "object") return undefined;
  const a = v as Record<string, unknown>;
  if (typeof a.sha256 !== "string" || !SHA.test(a.sha256)) return undefined;
  return {
    sha256: a.sha256,
    width: typeof a.width === "number" ? a.width : undefined,
    height: typeof a.height === "number" ? a.height : undefined,
    name: typeof a.name === "string" ? a.name : undefined,
  };
}

function font(v: unknown): BrandFont | undefined {
  if (!v || typeof v !== "object") return undefined;
  const f = v as Record<string, unknown>;
  if (typeof f.family !== "string" || typeof f.sha256 !== "string" || !SHA.test(f.sha256))
    return undefined;
  return { family: f.family.slice(0, 60), sha256: f.sha256 };
}

export function normalizeKit(raw: unknown): BrandKit {
  const d = (raw && typeof raw === "object" ? raw : {}) as Record<string, unknown>;
  const logosIn = (d.logos && typeof d.logos === "object" ? d.logos : {}) as Record<
    string,
    unknown
  >;
  const fontsIn = (d.fonts && typeof d.fonts === "object" ? d.fonts : {}) as Record<
    string,
    unknown
  >;
  const colorsIn = (d.colors && typeof d.colors === "object" ? d.colors : {}) as Record<
    string,
    unknown
  >;
  const logos: BrandKit["logos"] = {};
  for (const { role } of LOGO_ROLES) {
    const a = asset(logosIn[role]);
    if (a) logos[role] = a;
  }
  const fonts: BrandKit["fonts"] = {};
  for (const { role } of FONT_ROLES) {
    const f = font(fontsIn[role]);
    if (f) fonts[role] = f;
  }
  const color = (k: keyof BrandKit["colors"]) =>
    typeof colorsIn[k] === "string" && HEX.test(colorsIn[k] as string)
      ? (colorsIn[k] as string)
      : EMPTY_KIT.colors[k];
  return {
    v: 1,
    logos,
    fonts,
    colors: { inkLight: color("inkLight"), inkDark: color("inkDark"), accent: color("accent") },
    notes: typeof d.notes === "string" ? d.notes : "",
  };
}

// A font family from a file name: "Bellefair-Regular.ttf" -> "Bellefair Regular".
export function familyFromFile(name: string) {
  return (
    name
      .replace(/\.[A-Za-z0-9]+$/, "")
      .replace(/[_-]+/g, " ")
      .replace(/\s+/g, " ")
      .trim()
      .slice(0, 60) || "Brand font"
  );
}

// The kit's fonts, once each, with the address the browser loads them from.
export function kitFonts(kit: BrandKit): { family: string; url: string }[] {
  const seen = new Set<string>();
  const out: { family: string; url: string }[] = [];
  for (const f of Object.values(kit.fonts)) {
    if (!f || seen.has(f.family)) continue;
    seen.add(f.family);
    out.push({ family: f.family, url: fileUrl(f.sha256) });
  }
  return out;
}
