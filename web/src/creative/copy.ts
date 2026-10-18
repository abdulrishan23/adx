// Copy blocks per concept, in the order the old generator stacked them.
export type CopyBlocks = {
  eyebrow: string;
  title: string;
  titleSize: "xl" | "lg" | "md";
  text: string;
  stats: { figure: string; label: string }[];
  event: string; // "Saturday 25 Oct · 4 pm · Conrad Abu Dhabi"
  note: string;
  info: string; // small print, such as a discount's condition
  cta: string; // the button on the With-CTA variant
};

export type ConceptCopy = {
  v: 1;
  render: string; // default render code for generated ads, "" for none
  en: CopyBlocks;
  ar: CopyBlocks;
  arReviewed: boolean; // a native speaker signed off the Arabic
};

export const EMPTY_BLOCKS: CopyBlocks = {
  eyebrow: "",
  title: "",
  titleSize: "md",
  text: "",
  stats: [],
  event: "",
  note: "",
  info: "",
  cta: "",
};

const TEXT_KEYS = ["eyebrow", "title", "text", "event", "note", "info", "cta"] as const;

function blocks(raw: unknown): CopyBlocks {
  const x = (raw && typeof raw === "object" ? raw : {}) as Record<string, unknown>;
  const out: CopyBlocks = { ...EMPTY_BLOCKS, stats: [] };
  for (const k of TEXT_KEYS) {
    if (typeof x[k] === "string") out[k] = x[k] as string;
  }
  out.titleSize = x.titleSize === "xl" || x.titleSize === "lg" ? x.titleSize : "md";
  if (Array.isArray(x.stats)) {
    out.stats = x.stats
      .filter((s): s is Record<string, unknown> => !!s && typeof s === "object")
      .filter((s) => typeof s.figure === "string")
      .map((s) => ({ figure: String(s.figure), label: String(s.label ?? "") }));
  }
  return out;
}

export function normalizeCopy(raw: unknown): ConceptCopy {
  const d = (raw && typeof raw === "object" ? raw : {}) as Record<string, unknown>;
  return {
    v: 1,
    render: typeof d.render === "string" ? d.render : "",
    en: blocks(d.en),
    ar: blocks(d.ar),
    arReviewed: d.arReviewed === true,
  };
}

export function hasCopy(b: CopyBlocks) {
  return !!(b.title.trim() || b.text.trim() || b.eyebrow.trim());
}
