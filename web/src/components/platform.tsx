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

// Platform colours are the one place colour means "which company" rather than
// "what state". They're marks, never logos: Meta's blue square, and Google's
// four colours in a 2×2 square.
export function PlatformMark({
  platform,
  className,
}: {
  platform: PlatformKey;
  className?: string;
}) {
  const box = cn("inline-block size-2.5 shrink-0", className);
  if (platform === "google") {
    return (
      <span aria-hidden="true" className={cn(box, "grid grid-cols-2 grid-rows-2")}>
        <span className="bg-[#4285F4]" />
        <span className="bg-[#EA4335]" />
        <span className="bg-[#34A853]" />
        <span className="bg-[#FBBC05]" />
      </span>
    );
  }
  if (platform === "meta") return <span aria-hidden="true" className={cn(box, "bg-[#0866FF]")} />;
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
        <span aria-hidden="true" className="inline-block size-2.5 shrink-0" />
      )}
      <span className="truncate">{name}</span>
    </span>
  );
}
