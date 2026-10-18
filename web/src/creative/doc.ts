// The design file: everything needed to redraw one ad. The server stores it
// as JSON per ad. It's deliberately small: one background picture with a
// crop, then text and picture layers on top.
import { type Fmt, FORMATS } from "./naming";

export type TextLayer = {
  id: string;
  type: "text";
  text: string;
  x: number;
  y: number;
  width: number; // wrap width in canvas pixels
  fontFamily: string;
  fontSize: number;
  fontStyle: "normal" | "bold" | "italic" | "bold italic";
  fill: string;
  align: "left" | "center" | "right";
  lineHeight: number;
  letterSpacing: number;
  uppercase: boolean;
  rtl: boolean;
};

export type ImageLayer = {
  id: string;
  type: "image";
  kind: "logo" | "image";
  src: string; // /api/files/<sha256>
  x: number;
  y: number;
  width: number;
  height: number;
  opacity: number;
};

export type Layer = TextLayer | ImageLayer;

export type DesignDoc = {
  v: 1;
  fmt: Fmt;
  width: number;
  height: number;
  background: string;
  // Cover-fit crop: focal point from 0 to 1, zoom of 1 or more.
  image: { src: string; renderId?: string; fx: number; fy: number; zoom: number } | null;
  layers: Layer[];
};

export const FONTS = [
  {
    family: "Playfair Display",
    kind: "display",
    google: "Playfair+Display:ital,wght@0,400;0,600;0,700;1,400",
  },
  {
    family: "Cormorant Garamond",
    kind: "display",
    google: "Cormorant+Garamond:ital,wght@0,400;0,600;0,700;1,400",
  },
  { family: "Bellefair", kind: "display", google: "Bellefair" },
  {
    family: "Montserrat",
    kind: "text",
    google: "Montserrat:ital,wght@0,300;0,400;0,500;0,600;0,700;1,400",
  },
  {
    family: "Poppins",
    kind: "text",
    google: "Poppins:ital,wght@0,300;0,400;0,500;0,600;0,700;1,400",
  },
  { family: "Inter", kind: "text", google: "Inter:wght@300;400;500;600;700" },
  { family: "Almarai", kind: "arabic", google: "Almarai:wght@300;400;700;800" },
  { family: "Cairo", kind: "arabic", google: "Cairo:wght@300;400;600;700" },
] as const;

export const FONT_FAMILIES: string[] = FONTS.map((f) => f.family);

export const GOOGLE_FONTS_HREF = `https://fonts.googleapis.com/css2?${FONTS.map((f) => `family=${f.google}`).join("&")}&display=swap`;

export function newId() {
  return Math.random().toString(36).slice(2, 10);
}

export function emptyDoc(fmt: Fmt): DesignDoc {
  const f = FORMATS[fmt];
  return {
    v: 1,
    fmt,
    width: f.width,
    height: f.height,
    background: "#1f2328",
    image: null,
    layers: [],
  };
}

// The first layout for an ad uploaded as a flat picture: the picture becomes
// the background.
export function docFromFlatAd(fmt: Fmt, imageUrl: string): DesignDoc {
  const d = emptyDoc(fmt);
  d.image = { src: imageUrl, fx: 0.5, fy: 0.5, zoom: 1 };
  return d;
}

export function defaultText(doc: DesignDoc, text = "Your headline", rtl = false): TextLayer {
  return {
    id: newId(),
    type: "text",
    text,
    x: Math.round(doc.width * 0.08),
    y: Math.round(doc.height * 0.2),
    width: Math.round(doc.width * 0.84),
    fontFamily: rtl ? "Almarai" : "Playfair Display",
    fontSize: Math.round(doc.width * 0.09),
    fontStyle: "normal",
    fill: "#ffffff",
    align: "center",
    lineHeight: 1.05,
    letterSpacing: 0,
    uppercase: !rtl,
    rtl,
  };
}

// Where "Add logo" puts a logo: top centre, the house rule, clear of the
// 9:16 top zone.
export function logoTop(doc: DesignDoc) {
  return doc.fmt === "9:16" ? Math.round(doc.height * 0.14) + 30 : 44;
}

export function defaultLogo(doc: DesignDoc, src: string, ratio: number): ImageLayer {
  const width = Math.round(doc.width * 0.2);
  const height = Math.round(width / ratio);
  return {
    id: newId(),
    type: "image",
    kind: "logo",
    src,
    x: Math.round((doc.width - width) / 2),
    y: logoTop(doc),
    width,
    height,
    opacity: 1,
  };
}

// Moves a layout to another size. Positions keep their relative place and
// type scales with the smaller side's change.
export function mapToFormat(doc: DesignDoc, fmt: Fmt): DesignDoc {
  const f = FORMATS[fmt];
  const sx = f.width / doc.width;
  const sy = f.height / doc.height;
  const k = Math.min(sx, sy);
  const out: DesignDoc = { ...doc, fmt, width: f.width, height: f.height };
  out.layers = doc.layers.map((l): Layer => {
    if (l.type === "text") {
      const moved = {
        ...l,
        x: l.x * sx,
        y: l.y * sy,
        width: l.width * sx,
        fontSize: l.fontSize * k,
      };
      if (fmt !== "9:16") return moved;
      const safeTop = f.height * 0.14 + 20;
      const safeBottom = f.height * 0.8;
      return {
        ...moved,
        y: Math.min(Math.max(moved.y, safeTop), safeBottom - moved.fontSize * 2),
      };
    }
    const moved = { ...l, x: l.x * sx, y: l.y * sy, width: l.width * k, height: l.height * k };
    if (l.kind !== "logo") return moved;
    return {
      ...moved,
      x: Math.round((f.width - moved.width) / 2),
      y: Math.min(Math.max(moved.y, logoTop(out)), f.height - moved.height),
    };
  });
  return out;
}

// Where to draw the background so it fills the canvas.
export function coverRect(
  cw: number,
  ch: number,
  iw: number,
  ih: number,
  fx: number,
  fy: number,
  zoom: number,
) {
  const scale = Math.max(cw / iw, ch / ih) * Math.max(1, zoom);
  const w = iw * scale;
  const h = ih * scale;
  const clamp = (v: number) => Math.min(1, Math.max(0, v));
  return { x: (cw - w) * clamp(fx), y: (ch - h) * clamp(fy), width: w, height: h };
}

const num = (v: unknown, fallback: number) =>
  typeof v === "number" && Number.isFinite(v) ? v : fallback;

// A sanity pass so an old or hand-edited layout can't crash the editor.
export function normalizeDoc(raw: unknown, fmt: Fmt, fallbackImage: string | null): DesignDoc {
  const d = (raw && typeof raw === "object" ? raw : {}) as Partial<DesignDoc>;
  if (d.v !== 1 || !d.fmt || !FORMATS[d.fmt] || !Array.isArray(d.layers)) {
    return fallbackImage ? docFromFlatAd(fmt, fallbackImage) : emptyDoc(fmt);
  }
  const base = emptyDoc(d.fmt);
  const img = d.image;
  return {
    ...base,
    background: typeof d.background === "string" ? d.background : base.background,
    image:
      img && typeof img.src === "string"
        ? {
            src: img.src,
            renderId: typeof img.renderId === "string" ? img.renderId : undefined,
            fx: num(img.fx, 0.5),
            fy: num(img.fy, 0.5),
            zoom: num(img.zoom, 1),
          }
        : null,
    layers: d.layers.filter(
      (l): l is Layer => !!l && typeof l === "object" && (l.type === "text" || l.type === "image"),
    ),
  };
}
