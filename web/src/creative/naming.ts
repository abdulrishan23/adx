// Ad sizes and file names, kept pure so they can be tested on their own. The
// server builds the same codes (server/core/creative/naming.go); this side
// reads names off uploaded files and shows what an ad will be called.
//   ad code  <Concept>_<SLUG>_<LANG>_<SIZE>[_CTA]          e.g. O1_CASH_EN_191x1_CTA
//   file     <PREFIX>_<Concept>_<SLUG>_<LANG>_<SIZE>[_CTA].jpg, with 1.91x1 in the name
import type { Fmt, Lang } from "@/api/types.legacy";

export type { Fmt, Lang };

export const FORMATS: Record<
  Fmt,
  { width: number; height: number; idToken: string; fileToken: string; label: string }
> = {
  "9:16": { width: 1080, height: 1920, idToken: "9x16", fileToken: "9x16", label: "9:16 story" },
  "4:5": { width: 1080, height: 1350, idToken: "4x5", fileToken: "4x5", label: "4:5 feed" },
  "1:1": { width: 1080, height: 1080, idToken: "1x1", fileToken: "1x1", label: "1:1 square" },
  "1.91:1": {
    width: 1200,
    height: 628,
    idToken: "191x1",
    fileToken: "1.91x1",
    label: "1.91:1 landscape",
  },
};

export const FMT_ORDER: Fmt[] = ["9:16", "4:5", "1:1", "1.91:1"];

const SIZE_TOKENS: Record<string, Fmt> = {
  "9X16": "9:16",
  "4X5": "4:5",
  "1X1": "1:1",
  "1.91X1": "1.91:1",
  "191X1": "1.91:1",
};

// Pixels decide the size. An exact match first, otherwise the same shape
// within 2%, which the upload then resizes.
export function detectFormat(width: number, height: number): { fmt: Fmt; exact: boolean } | null {
  for (const fmt of FMT_ORDER) {
    const f = FORMATS[fmt];
    if (f.width === width && f.height === height) return { fmt, exact: true };
  }
  const ratio = width / height;
  for (const fmt of FMT_ORDER) {
    const f = FORMATS[fmt];
    const target = f.width / f.height;
    if (Math.abs(ratio - target) / target <= 0.02) return { fmt, exact: false };
  }
  return null;
}

export type ParsedName = {
  concept: string; // C1, O1, T1AR
  slug: string; // CASH, PLAN10, RETAIL_2080
  lang: Lang;
  fmt: Fmt | null; // from the name, which may leave it out
  cta: boolean;
  prefix: string | null;
};

const CONCEPT = /^[A-Z]{1,3}\d{1,3}[A-Z]{0,3}$/;

// Reads "MARLIN_O1_CASH_EN_1.91x1_CTA.jpg" into its parts. Returns null when
// there's no concept id.
export function parseAdFilename(filename: string, knownPrefix?: string): ParsedName | null {
  const base = filename.replace(/\.[A-Za-z0-9]+$/, "").trim();
  let tokens = base
    .split(/[_\s-]+/)
    .filter(Boolean)
    .map((t) => t.toUpperCase());
  if (tokens.length === 0) return null;

  let prefix: string | null = null;
  const [first = "", second = ""] = tokens;
  if (knownPrefix && first === knownPrefix.toUpperCase() && tokens.length > 1) {
    prefix = first;
    tokens = tokens.slice(1);
  } else if (!CONCEPT.test(first) && tokens.length > 1 && CONCEPT.test(second)) {
    prefix = first;
    tokens = tokens.slice(1);
  }
  const concept = tokens[0] ?? "";
  if (!CONCEPT.test(concept)) return null;
  tokens = tokens.slice(1);

  let cta = false;
  if (tokens.at(-1) === "CTA") {
    cta = true;
    tokens = tokens.slice(0, -1);
  }
  let fmt: Fmt | null = null;
  const last = tokens.at(-1);
  const sized = last ? SIZE_TOKENS[last] : undefined;
  if (sized) {
    fmt = sized;
    tokens = tokens.slice(0, -1);
  }
  let lang: Lang = "en";
  const maybeLang = tokens.at(-1);
  if (maybeLang === "EN" || maybeLang === "AR") {
    lang = maybeLang === "AR" ? "ar" : "en";
    tokens = tokens.slice(0, -1);
  }
  const slug =
    tokens
      .filter((t) => t !== "CTA")
      .join("_")
      .replace(/[^A-Z0-9_]/g, "")
      .replace(/_+/g, "_")
      .replace(/^_|_$/g, "") || "AD";
  return { concept, slug, lang, fmt, cta, prefix };
}

type Spec = { concept: string; slug: string; lang: Lang; fmt: Fmt; cta: boolean };

export function buildAdCode(p: Spec) {
  const parts = [p.concept, p.slug, p.lang.toUpperCase(), FORMATS[p.fmt].idToken];
  if (p.cta) parts.push("CTA");
  return parts.join("_");
}

export function buildFileName(prefix: string, p: Spec) {
  const parts = [prefix, p.concept, p.slug, p.lang.toUpperCase(), FORMATS[p.fmt].fileToken];
  if (p.cta) parts.push("CTA");
  return `${parts.join("_")}.jpg`;
}

// Natural order for concept ids: C1, C2, C10, O1.
export function compareConcepts(a: string, b: string) {
  const ma = a.match(/^([A-Z]+)(\d+)([A-Z]*)$/);
  const mb = b.match(/^([A-Z]+)(\d+)([A-Z]*)$/);
  if (!ma || !mb) return a.localeCompare(b);
  const [, la = "", na = "0", sa = ""] = ma;
  const [, lb = "", nb = "0", sb = ""] = mb;
  return la.localeCompare(lb) || Number(na) - Number(nb) || sa.localeCompare(sb);
}

export function adSortKey(p: { lang: Lang; fmt: Fmt; cta: boolean }) {
  return (p.lang === "ar" ? 100 : 0) + FMT_ORDER.indexOf(p.fmt) * 2 + (p.cta ? 1 : 0);
}

// The slug an ad was named with, for generating its siblings: C1_CASH_EN_9x16 -> CASH.
export function slugFromCode(code: string, concept: string): string | null {
  const m = code.match(
    new RegExp(`^${concept}_(.+?)(?:_(?:EN|AR))?_(?:9x16|4x5|1x1|191x1)(?:_CTA)?$`),
  );
  return m?.[1] ?? null;
}

// A slug from a concept's name, for a concept with no ads yet: "Cash offer" -> CASH_OFFER.
export function slugFromName(name: string) {
  return (
    name
      .toUpperCase()
      .replace(/[^A-Z0-9]+/g, "_")
      .replace(/^_|_$/g, "")
      .slice(0, 24)
      .replace(/_$/, "") || "AD"
  );
}
