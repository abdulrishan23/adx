import { cn } from "@/lib/utils";

export type PlatformKey =
  | "google"
  | "meta"
  | "ghl"
  | "web"
  | "bayut"
  | "propertyfinder"
  | "dubizzle"
  | "propertyguru";

const LABEL: Record<PlatformKey, string> = {
  google: "Google",
  meta: "Meta",
  ghl: "GoHighLevel",
  web: "Website",
  bayut: "Bayut",
  propertyfinder: "Property Finder",
  dubizzle: "dubizzle",
  propertyguru: "PropertyGuru",
};

// Each portal's main brand colour, close to its logo. Swap in the exact values
// from each brand's press kit when we have them.
const PORTAL_COLOUR: Partial<Record<PlatformKey, string>> = {
  bayut: "bg-[#28B16D]",
  propertyfinder: "bg-[#EF5E4E]",
  dubizzle: "bg-[#C8102E]",
  propertyguru: "bg-[#0FA3B1]",
};

// Meta and Google are their own glyphs, the infinity and the G, white in an
// ink box. Colour read as the wrong company (Google's four squares looked
// like Microsoft), so the shape carries it instead. The glyphs are the ones
// Hisham picked from SVG Repo. The portals keep a small square in their
// brand colour.
const GLYPH: Partial<Record<PlatformKey, { viewBox: string; d: string }>> = {
  meta: {
    viewBox: "2 2 28 28",
    d: "M5,19.5c0-4.6,2.3-9.4,5-9.4c1.5,0,2.7,0.9,4.6,3.6c-1.8,2.8-2.9,4.5-2.9,4.5c-2.4,3.8-3.2,4.6-4.5,4.6  C5.9,22.9,5,21.7,5,19.5 M20.7,17.8L19,15c-0.4-0.7-0.9-1.4-1.3-2c1.5-2.3,2.7-3.5,4.2-3.5c3,0,5.4,4.5,5.4,10.1  c0,2.1-0.7,3.3-2.1,3.3S23.3,22,20.7,17.8 M16.4,11c-2.2-2.9-4.1-4-6.3-4C5.5,7,2,13.1,2,19.5c0,4,1.9,6.5,5.1,6.5  c2.3,0,3.9-1.1,6.9-6.3c0,0,1.2-2.2,2.1-3.7c0.3,0.5,0.6,1,0.9,1.6l1.4,2.4c2.7,4.6,4.2,6.1,6.9,6.1c3.1,0,4.8-2.6,4.8-6.7  C30,12.6,26.4,7,22.1,7C19.8,7,18,8.8,16.4,11",
  },
  google: {
    viewBox: "0 0 512 512",
    d: "M473.16,221.48l-2.26-9.59H262.46v88.22H387c-12.93,61.4-72.93,93.72-121.94,93.72-35.66,0-73.25-15-98.13-39.11a140.08,140.08,0,0,1-41.8-98.88c0-37.16,16.7-74.33,41-98.78s61-38.13,97.49-38.13c41.79,0,71.74,22.19,82.94,32.31l62.69-62.36C390.86,72.72,340.34,32,261.6,32h0c-60.75,0-119,23.27-161.58,65.71C58,139.5,36.25,199.93,36.25,256S56.83,369.48,97.55,411.6C141.06,456.52,202.68,480,266.13,480c57.73,0,112.45-22.62,151.45-63.66,38.34-40.4,58.17-96.3,58.17-154.9C475.75,236.77,473.27,222.12,473.16,221.48Z",
  },
};

export function PlatformMark({
  platform,
  className,
}: {
  platform: PlatformKey;
  className?: string;
}) {
  const glyph = GLYPH[platform];
  if (glyph) {
    return (
      <span
        aria-hidden="true"
        className={cn(
          "inline-grid size-4 shrink-0 place-items-center bg-foreground text-background",
          className,
        )}
      >
        <svg viewBox={glyph.viewBox} className="size-[70%]" fill="currentColor" aria-hidden="true">
          <path d={glyph.d} />
        </svg>
      </span>
    );
  }
  const box = cn("inline-block size-2.5 shrink-0", className);
  const portal = PORTAL_COLOUR[platform];
  if (portal) return <span aria-hidden="true" className={cn(box, portal)} />;
  if (platform === "ghl") return <span aria-hidden="true" className={cn(box, "bg-foreground")} />;
  return <span aria-hidden="true" className={cn(box, "border-[1.5px] border-current")} />;
}

// PlatformTag is the mark with the platform's name, for lists that mix them.
export function PlatformTag({
  platform,
  className,
}: {
  platform: PlatformKey;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1.5 text-xs whitespace-nowrap text-muted-foreground",
        className,
      )}
    >
      <PlatformMark platform={platform} />
      {LABEL[platform]}
    </span>
  );
}

export const platformLabel = (p: PlatformKey) => LABEL[p];

// Sources arrive as free text from GoHighLevel and the portals ("Meta form",
// "Property Finder", "dubizzle"). This finds the platform behind one, if any.
export function sourceKey(name: string): PlatformKey | null {
  const n = name.toLowerCase().replace(/[^a-z]/g, "");
  if (n.startsWith("meta") || n.startsWith("facebook") || n.startsWith("instagram")) return "meta";
  if (n.startsWith("google")) return "google";
  if (n.startsWith("bayut")) return "bayut";
  if (n.startsWith("propertyfinder")) return "propertyfinder";
  if (n.startsWith("dubizzle")) return "dubizzle";
  if (n.startsWith("propertyguru")) return "propertyguru";
  if (n.startsWith("website")) return "web";
  return null;
}

// SourceName is a source as written, with its platform's mark when there is one.
// Unknown sources keep a blank space the mark's width, so names still line up.
export function SourceName({ name, className }: { name: string; className?: string }) {
  const key = sourceKey(name);
  return (
    <span className={cn("inline-flex min-w-0 items-center gap-2", className)}>
      {key ? (
        <PlatformMark platform={key} />
      ) : (
        <span aria-hidden="true" className="inline-block size-4 shrink-0" />
      )}
      <span className="truncate">{name}</span>
    </span>
  );
}
