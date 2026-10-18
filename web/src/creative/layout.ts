// Automatic layout: copy, brand kit and render become a design file for one
// size. A port of the old generator's scale factors, block order, margins
// and safe zones. Copy sits at the top on light ink.
import type { BrandKit } from "./brand";
import type { CopyBlocks } from "./copy";
import { type DesignDoc, emptyDoc, type ImageLayer, newId, type TextLayer } from "./doc";
import { fileUrl } from "./files";
import { type Fmt, FORMATS, type Lang } from "./naming";

const K: Record<Fmt, number> = { "4:5": 1.0, "1:1": 0.86, "9:16": 1.08, "1.91:1": 0.55 };
const SAFE_TOP = 269;
const SAFE_BOTTOM = 1536; // 9:16 only
const TITLE_SIZE = { xl: 154, lg: 112, md: 90 } as const;

export type LayoutInput = {
  fmt: Fmt;
  lang: Lang;
  cta: boolean;
  copy: CopyBlocks;
  kit: BrandKit;
  render: { url: string; renderId?: string } | null;
};

// A rough text height, so blocks stack without measuring fonts.
function textHeight(
  text: string,
  fontSize: number,
  width: number,
  lineHeight: number,
  uppercase: boolean,
) {
  const avg = fontSize * (uppercase ? 0.62 : 0.52);
  const lines = text
    .split(/\r?\n/)
    .reduce((n, line) => n + Math.max(1, Math.ceil((line.length * avg) / width)), 0);
  return Math.round(lines * fontSize * lineHeight);
}

export function layoutDoc(input: LayoutInput): DesignDoc {
  const { fmt, lang, cta, copy, kit } = input;
  const f = FORMATS[fmt];
  const k = K[fmt];
  const W = f.width;
  const H = f.height;
  const rtl = lang === "ar";
  const doc = emptyDoc(fmt);
  doc.background = kit.colors.inkDark;
  if (input.render) {
    doc.image = { src: input.render.url, renderId: input.render.renderId, fx: 0.5, fy: 0, zoom: 1 };
  }

  const ink = kit.colors.inkLight;
  const titleFont =
    (rtl ? kit.fonts.arTitle?.family : kit.fonts.title?.family) ??
    (rtl ? "Almarai" : "Playfair Display");
  const textFont =
    (rtl ? kit.fonts.arText?.family : kit.fonts.text?.family) ?? (rtl ? "Almarai" : "Montserrat");
  const labelFont = (rtl ? kit.fonts.arText?.family : kit.fonts.label?.family) ?? textFont;
  const margin = 44;
  const width = W - margin * 2;
  const layers: (TextLayer | ImageLayer)[] = [];

  // Logo top centre. A tall logo, icon above wordmark, is sized by height.
  let y = fmt === "9:16" ? SAFE_TOP + 14 : 44;
  const logo = kit.logos.onDark ?? kit.logos.onLight;
  if (logo) {
    const ratio = logo.width && logo.height ? logo.width / logo.height : 3;
    let lw = Math.round(W * 0.2 * (fmt === "1.91:1" ? 0.6 : 1));
    let lh = Math.round(lw / ratio);
    if (ratio < 1) {
      lh = Math.round(H * (fmt === "1.91:1" ? 0.22 : 0.11));
      lw = Math.round(lh * ratio);
    }
    layers.push({
      id: newId(),
      type: "image",
      kind: "logo",
      src: fileUrl(logo.sha256),
      x: Math.round((W - lw) / 2),
      y,
      width: lw,
      height: lh,
      opacity: 1,
    });
    y += lh + Math.round(30 * k);
  }

  const text = (
    t: string,
    opts: Partial<TextLayer> & { fontSize: number; fontFamily: string },
    gapAfter: number,
  ) => {
    if (!t.trim()) return;
    const uppercase = opts.uppercase ?? false;
    const lineHeight = opts.lineHeight ?? 1.1;
    const h = textHeight(t, opts.fontSize, width, lineHeight, uppercase);
    layers.push({
      id: newId(),
      type: "text",
      text: t,
      x: margin,
      y,
      width,
      fontFamily: opts.fontFamily,
      fontSize: Math.round(opts.fontSize),
      fontStyle: opts.fontStyle ?? "normal",
      fill: ink,
      align: "center",
      lineHeight,
      letterSpacing: rtl ? 0 : (opts.letterSpacing ?? 0),
      uppercase: rtl ? false : uppercase,
      rtl,
    });
    y += h + Math.round(gapAfter * k);
  };

  const eyebrowSize = (rtl ? 30 : 24) * k;
  text(
    copy.eyebrow,
    {
      fontSize: eyebrowSize,
      fontFamily: labelFont,
      letterSpacing: eyebrowSize * 0.24,
      uppercase: true,
      lineHeight: 1.2,
    },
    18,
  );
  const titleSize = (fmt === "1.91:1" ? TITLE_SIZE.md : TITLE_SIZE[copy.titleSize]) * k;
  text(
    copy.title,
    {
      fontSize: titleSize,
      fontFamily: titleFont,
      lineHeight: 0.98,
      uppercase: !rtl && copy.titleSize === "xl",
    },
    22,
  );
  text(copy.text, { fontSize: 43 * k, fontFamily: textFont, lineHeight: rtl ? 1.5 : 1.16 }, 16);
  if (copy.stats.length) {
    text(
      copy.stats.map((s) => s.figure).join("    "),
      { fontSize: 100 * k, fontFamily: titleFont, lineHeight: 1 },
      4,
    );
    text(
      copy.stats.map((s) => s.label).join("    "),
      {
        fontSize: 22 * k,
        fontFamily: labelFont,
        letterSpacing: 22 * k * 0.16,
        uppercase: true,
        lineHeight: 1.2,
      },
      18,
    );
  }
  text(copy.event, { fontSize: 25 * k, fontFamily: textFont, lineHeight: 1.4 }, 12);
  text(
    copy.note,
    { fontSize: 30 * k, fontFamily: textFont, fontStyle: "italic", lineHeight: 1.3 },
    12,
  );
  text(
    copy.info,
    { fontSize: 22 * k, fontFamily: labelFont, letterSpacing: 22 * k * 0.13, lineHeight: 1.3 },
    12,
  );
  if (cta) {
    text(
      copy.cta,
      {
        fontSize: (rtl ? 25 : 20) * k,
        fontFamily: labelFont,
        letterSpacing: 20 * k * 0.2,
        uppercase: true,
        lineHeight: 1.2,
      },
      0,
    );
  }

  // Developer logo bottom centre, above the 9:16 bottom zone.
  const dev = kit.logos.developer;
  if (dev) {
    const ratio = dev.width && dev.height ? dev.width / dev.height : 3;
    const dw = Math.round(250 * k);
    const dh = Math.round(dw / ratio);
    const bottom = fmt === "9:16" ? SAFE_BOTTOM - 6 : H - 30;
    layers.push({
      id: newId(),
      type: "image",
      kind: "logo",
      src: fileUrl(dev.sha256),
      x: Math.round((W - dw) / 2),
      y: bottom - dh,
      width: dw,
      height: dh,
      opacity: 1,
    });
  }

  doc.layers = layers;
  return doc;
}
