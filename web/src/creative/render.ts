// Draws a design file to a picture without React: for previews, variations
// and the pictures that get published.
import Konva from "konva";
import { coverRect, type DesignDoc, GOOGLE_FONTS_HREF, type TextLayer } from "./doc";

const images = new Map<string, Promise<HTMLImageElement>>();

export function loadImage(src: string): Promise<HTMLImageElement> {
  let p = images.get(src);
  if (!p) {
    p = new Promise((resolve, reject) => {
      const i = new window.Image();
      // Cross-origin so the canvas stays exportable.
      i.crossOrigin = "anonymous";
      i.onload = () => resolve(i);
      i.onerror = () => {
        images.delete(src);
        reject(new Error(`Couldn't load ${src}`));
      };
      i.src = src;
    });
    images.set(src, p);
  }
  return p;
}

const FONT_LINK_ID = "adx-creative-fonts";

// The Google fonts the editor offers, loaded once.
export function loadGoogleFonts() {
  if (document.getElementById(FONT_LINK_ID)) return;
  const l = document.createElement("link");
  l.id = FONT_LINK_ID;
  l.rel = "stylesheet";
  l.href = GOOGLE_FONTS_HREF;
  document.head.appendChild(l);
}

// Brand fonts from the kit are registered once, then work like installed fonts.
const registered = new Set<string>();
export function registerFonts(fonts: { family: string; url: string }[]) {
  for (const f of fonts) {
    if (!f.family || !f.url || registered.has(f.family)) continue;
    registered.add(f.family);
    try {
      const face = new FontFace(f.family, `url("${f.url}")`);
      document.fonts.add(face);
      void face.load().catch(() => null);
    } catch {
      // An unsupported file: the fallback font is used.
    }
  }
}

export function fontsUsed(doc: DesignDoc) {
  return [
    ...new Set(
      doc.layers
        .filter((l): l is TextLayer => l.type === "text")
        .map((l) => `${l.fontStyle} 16px "${l.fontFamily}"`),
    ),
  ];
}

export async function loadFonts(doc: DesignDoc) {
  await Promise.all(fontsUsed(doc).map((f) => document.fonts.load(f).catch(() => null)));
}

// Builds the Konva scene for a doc at a scale. The caller destroys the stage.
export async function buildStage(
  doc: DesignDoc,
  scale: number,
  container?: HTMLDivElement,
): Promise<Konva.Stage> {
  const stage = new Konva.Stage({
    container: container ?? document.createElement("div"),
    width: doc.width * scale,
    height: doc.height * scale,
    scaleX: scale,
    scaleY: scale,
  });
  const layer = new Konva.Layer();
  stage.add(layer);
  layer.add(
    new Konva.Rect({ x: 0, y: 0, width: doc.width, height: doc.height, fill: doc.background }),
  );
  await loadFonts(doc);
  if (doc.image) {
    try {
      const img = await loadImage(doc.image.src);
      const r = coverRect(
        doc.width,
        doc.height,
        img.width,
        img.height,
        doc.image.fx,
        doc.image.fy,
        doc.image.zoom,
      );
      layer.add(new Konva.Image({ image: img, ...r }));
    } catch {
      // A missing picture: the background colour shows.
    }
  }
  for (const l of doc.layers) {
    if (l.type === "text") {
      layer.add(
        new Konva.Text({
          x: l.x,
          y: l.y,
          width: l.width,
          text: l.uppercase ? l.text.toUpperCase() : l.text,
          fontFamily: l.fontFamily,
          fontSize: l.fontSize,
          fontStyle: l.fontStyle,
          fill: l.fill,
          align: l.align,
          lineHeight: l.lineHeight,
          letterSpacing: l.letterSpacing,
          ...({ direction: l.rtl ? "rtl" : "ltr" } as object),
        }),
      );
    } else {
      try {
        const img = await loadImage(l.src);
        layer.add(
          new Konva.Image({
            image: img,
            x: l.x,
            y: l.y,
            width: l.width,
            height: l.height,
            opacity: l.opacity,
          }),
        );
      } catch {
        // Skip a missing picture.
      }
    }
  }
  layer.draw();
  return stage;
}

export async function renderDoc(
  doc: DesignDoc,
  mime: "image/jpeg" | "image/png" = "image/jpeg",
  quality = 0.92,
): Promise<Blob> {
  const stage = await buildStage(doc, 1);
  try {
    const dataUrl = stage.toDataURL({ mimeType: mime, quality, pixelRatio: 1 });
    return await (await fetch(dataUrl)).blob();
  } finally {
    stage.destroy();
  }
}
