import { cn } from "@/lib/utils";

// Colour means status (design language, 03). Green, amber and red appear only
// here, always beside a word, so a colour-blind reader has the word and
// everyone else can find it. Pending is an outline, never a colour. The
// status colours are the same in both themes.
export type StatusTone = "ready" | "caution" | "error" | "pending";

const markTone: Record<StatusTone, string> = {
  ready: "bg-status-ready",
  caution: "bg-status-caution",
  error: "bg-status-error",
  pending: "border-[1.5px] border-current",
};

// An 8px square in front of a line of ink text: on cards, list rows, the
// ticker. The word next to it carries the meaning.
export function StatusMark({ tone, className }: { tone: StatusTone; className?: string }) {
  return (
    <span
      aria-hidden="true"
      data-tone={tone}
      className={cn("inline-block size-2 shrink-0", markTone[tone], className)}
    />
  );
}

const chipTone: Record<StatusTone, string> = {
  ready: "bg-status-ready text-paper",
  caution: "bg-status-caution text-ink",
  error: "bg-status-error text-paper",
  pending: "border border-current text-foreground",
};

// Solid fill, mono capitals, 24px tall. Caution text is always ink, never
// white; pending is an outline in the text colour.
export function StatusChip({
  tone,
  children,
  className,
}: {
  tone: StatusTone;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <span
      data-tone={tone}
      className={cn(
        "inline-flex h-6 shrink-0 items-center px-2 font-mono text-[11.2px] leading-4 font-semibold tracking-[0.05em] uppercase whitespace-nowrap",
        chipTone[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}
