import type { LucideIcon } from "lucide-react";
import { type Status, StatusMark } from "@/components/ui/status-chip";
import { cn } from "@/lib/utils";

// Tabs that filter by severity or status carry the matching chip's mark.
export type SegmentTone = Status;

// Segmented is a row of mutually exclusive tabs, with optional counts that
// only show when they're above zero. An option with an icon shows only the
// icon and keeps its label for screen readers and the tooltip.
export function Segmented<T extends string>({
  value,
  onChange,
  options,
  label,
  className,
}: {
  value: T;
  onChange: (value: T) => void;
  options: { value: T; label: string; count?: number; tone?: SegmentTone; icon?: LucideIcon }[];
  label: string;
  className?: string;
}) {
  return (
    <div
      role="tablist"
      aria-label={label}
      className={cn("inline-flex gap-1 rounded-lg border bg-card p-1", className)}
    >
      {options.map((o) => {
        const active = o.value === value;
        return (
          <button
            key={o.value}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange(o.value)}
            aria-label={o.icon ? o.label : undefined}
            title={o.icon ? o.label : undefined}
            className={cn(
              "inline-flex h-7 shrink-0 items-center gap-1.5 rounded-md text-sm whitespace-nowrap transition-colors",
              o.icon ? "w-8 justify-center" : "px-3",
              active
                ? "bg-primary text-primary-foreground"
                : "text-muted-foreground hover:bg-muted hover:text-foreground",
            )}
          >
            {o.tone && <StatusMark status={o.tone} />}
            {o.icon ? <o.icon className="size-4" /> : o.label}
            {o.count !== undefined && o.count > 0 && (
              <span
                className={cn("font-mono text-[0.7rem] tabular-nums", active ? "opacity-80" : "")}
              >
                {o.count}
              </span>
            )}
          </button>
        );
      })}
    </div>
  );
}
