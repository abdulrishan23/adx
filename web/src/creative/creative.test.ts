import { describe, expect, it } from "vitest";
import { EMPTY_KIT, normalizeKit } from "./brand";
import { checkDoc } from "./checks";
import { EMPTY_BLOCKS, normalizeCopy } from "./copy";
import { defaultText, emptyDoc, mapToFormat, normalizeDoc } from "./doc";
import { layoutDoc } from "./layout";
import {
  buildAdCode,
  buildFileName,
  compareConcepts,
  detectFormat,
  parseAdFilename,
  slugFromCode,
} from "./naming";

describe("ad names", () => {
  // The upload screen reads files exported from the old system; misreading
  // one files it under the wrong concept or size.
  it("reads concept, slug, language, size and CTA from a file name", () => {
    expect(parseAdFilename("MARLIN_O1_CASH_EN_1.91x1_CTA.jpg", "MARLIN")).toEqual({
      concept: "O1",
      slug: "CASH",
      lang: "en",
      fmt: "1.91:1",
      cta: true,
      prefix: "MARLIN",
    });
    expect(parseAdFilename("T1AR_RETAIL_2080_AR_9x16.png")?.slug).toBe("RETAIL_2080");
    expect(parseAdFilename("holiday-photo.jpg")).toBeNull();
  });

  it("builds the same code and file name as the server", () => {
    const spec = { concept: "C1", slug: "CASH", lang: "en", fmt: "1.91:1", cta: true } as const;
    expect(buildAdCode(spec)).toBe("C1_CASH_EN_191x1_CTA");
    expect(buildFileName("MARLIN", spec)).toBe("MARLIN_C1_CASH_EN_1.91x1_CTA.jpg");
    expect(slugFromCode("C1_CASH_EN_191x1_CTA", "C1")).toBe("CASH");
  });

  it("decides the size from the pixels, allowing 2% off", () => {
    expect(detectFormat(1080, 1920)).toEqual({ fmt: "9:16", exact: true });
    expect(detectFormat(2160, 3840)).toEqual({ fmt: "9:16", exact: false });
    expect(detectFormat(1000, 1000)).toEqual({ fmt: "1:1", exact: false });
    expect(detectFormat(1000, 400)).toBeNull();
  });

  it("orders concepts naturally", () => {
    expect(["C10", "O1", "C2"].sort(compareConcepts)).toEqual(["C2", "C10", "O1"]);
  });
});

describe("house rules", () => {
  // These are Reportage's rules for anything that goes out.
  it("flags a discount over 15% and one without its condition", () => {
    const doc = emptyDoc("4:5");
    doc.layers = [{ ...defaultText(doc, "20% off"), y: 300 }];
    const texts = checkDoc(doc).map((c) => c.text);
    expect(texts.some((t) => t.includes("over the 15% limit"))).toBe(true);
    expect(texts.some((t) => t.includes("without its condition"))).toBe(true);
  });

  it("flags text in the 9:16 keep-clear zones", () => {
    const doc = emptyDoc("9:16");
    doc.layers = [{ ...defaultText(doc, "Headline"), y: 50 }];
    expect(checkDoc(doc).some((c) => c.level === "error" && c.text.includes("top 14%"))).toBe(true);
  });

  it("always asks for a native review of Arabic until it's signed off", () => {
    const doc = emptyDoc("1:1");
    expect(checkDoc(doc, { lang: "ar" }).some((c) => c.text.includes("native"))).toBe(true);
    expect(
      checkDoc(doc, { lang: "ar", arReviewed: true }).some((c) => c.text.includes("native")),
    ).toBe(false);
  });
});

describe("layouts", () => {
  it("keeps a 9:16 layout out of the keep-clear zones", () => {
    const copy = {
      ...EMPTY_BLOCKS,
      eyebrow: "For cash buyers",
      title: "Marlin Icon",
      text: "Pay 10% now",
    };
    const doc = layoutDoc({
      fmt: "9:16",
      lang: "en",
      cta: false,
      copy,
      kit: EMPTY_KIT,
      render: null,
    });
    expect(doc.layers.length).toBe(3);
    expect(checkDoc(doc).filter((c) => c.level === "error")).toEqual([]);
  });

  it("moves a layout to another size with the logo still centred", () => {
    const doc = emptyDoc("4:5");
    doc.layers = [
      {
        id: "l",
        type: "image",
        kind: "logo",
        src: "/x",
        x: 432,
        y: 44,
        width: 216,
        height: 72,
        opacity: 1,
      },
    ];
    const wide = mapToFormat(doc, "1.91:1");
    const logo = wide.layers[0];
    expect(wide.width).toBe(1200);
    expect(logo && logo.x + logo.width / 2).toBeCloseTo(600, 0);
  });

  // Stored documents may be old or hand-edited; the editor must still open.
  it("turns a broken stored layout into a safe one", () => {
    expect(normalizeDoc({ v: 9 }, "1:1", "/api/files/abc").image?.src).toBe("/api/files/abc");
    expect(normalizeCopy("nonsense").en.title).toBe("");
    expect(normalizeKit({ colors: { accent: "red" } }).colors.accent).toBe(EMPTY_KIT.colors.accent);
  });
});
