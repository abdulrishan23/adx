// House-rule checks on a design file, from Creative Studio's rules. Each
// result is one plain sentence.
import type { DesignDoc, TextLayer } from "./doc";

export type Check = { level: "error" | "warn" | "info"; text: string };

export function checkDoc(
  doc: DesignDoc,
  opts: { lang?: "en" | "ar"; arReviewed?: boolean } = {},
): Check[] {
  const out: Check[] = [];
  const texts = doc.layers.filter((l): l is TextLayer => l.type === "text");
  const allText = texts.map((t) => t.text).join("\n");

  // Discounts: never above 15% on screen, and always with their condition.
  for (const m of allText.matchAll(/(\d{1,3})\s*%/g)) {
    if (Number(m[1]) > 15) {
      out.push({
        level: "error",
        text: `"${m[0]}" is over the 15% limit. Write "Up to 15% off and more" instead.`,
      });
    }
  }
  if (
    /\d{1,3}\s*%/.test(allText) &&
    !/(t&c|terms|condition|within|on |for |when|pay|plan|cash|select|limited|\*)/i.test(allText)
  ) {
    out.push({
      level: "warn",
      text: "A discount is shown without its condition in the same frame.",
    });
  }

  // 9:16 safe zones: nothing in the top 14% or the bottom 20%.
  if (doc.fmt === "9:16") {
    const top = doc.height * 0.14;
    const bottom = doc.height * 0.8;
    for (const l of doc.layers) {
      const h =
        l.type === "text"
          ? l.fontSize * l.lineHeight * Math.max(1, l.text.split("\n").length)
          : l.height;
      const name =
        l.type === "text"
          ? `Text "${l.text.slice(0, 24)}"`
          : l.kind === "logo"
            ? "A logo"
            : "A picture";
      if (l.y < top - 1)
        out.push({ level: "error", text: `${name} sits in the top 14% keep-clear zone.` });
      if (l.y + h > bottom + 1) {
        out.push({ level: "error", text: `${name} sits in the bottom 20% keep-clear zone.` });
      }
    }
  }

  // Logo top centre.
  const logo = doc.layers.find((l) => l.type === "image" && l.kind === "logo");
  if (!logo) {
    out.push({ level: "warn", text: "There's no logo on this ad." });
  } else if (logo.type === "image") {
    const centre = logo.x + logo.width / 2;
    if (Math.abs(centre - doc.width / 2) > doc.width * 0.04) {
      out.push({ level: "warn", text: "The logo isn't centred. House rule: logo top centre." });
    }
    if (logo.y > doc.height * 0.35) {
      out.push({ level: "info", text: "The logo sits low. House rule: logo top centre." });
    }
  }

  // 1.91:1 titles stay on one line.
  if (doc.fmt === "1.91:1" && texts.some((t) => t.fontSize > 40 && t.text.includes("\n"))) {
    out.push({ level: "warn", text: "1.91:1 titles should be one line." });
  }

  if (opts.lang === "ar" && !opts.arReviewed) {
    out.push({
      level: "info",
      text: "Arabic copy needs a native speaker's review before it goes out.",
    });
  }
  if (texts.some((t) => /[؀-ۿ]/.test(t.text)) && opts.lang !== "ar") {
    out.push({
      level: "info",
      text: "This has Arabic text. Mark the ad as Arabic so it's flagged for native review.",
    });
  }
  return out;
}
