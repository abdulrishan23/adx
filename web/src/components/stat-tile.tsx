import type { ReactNode } from "react";
import { type Change, formatChange, NOT_AVAILABLE } from "@/lib/format";
import { cn } from "@/lib/utils";
import { Sparkline } from "./charts";
import { Eyebrow } from "./page";
import { StatusMark } from "./ui/status-chip";

// StatTile shows one figure, how it moved, and a small trend. `better` says
// which direction is good, since cost going up is bad and leads going up good.
export function StatTile({
  label,
  value,
  change,
  better = "up",
  hint,
  trend,
  className,
}: {
  label: string;
  value: string;
  change?: Change | null;
  better?: "up" | "down" | "neither";
  hint?: ReactNode;
  trend?: number[];
  className?: string;
}) {
  const unavailable = value === NOT_AVAILABLE;
  const moved = change && change.direction !== "flat" && better !== "neither";
  const good = moved && change.direction === better;
  const bad = moved && change.direction !== better;
  return (
    <section className={cn("flex min-h-36 flex-col rounded-lg border bg-card p-5", className)}>
      <Eyebrow>{label}</Eyebrow>
      <div className="mt-3 flex items-baseline justify-between gap-3">
        <strong
          className={cn(
            "font-mono text-2xl font-semibold tracking-tight tabular-nums",
            unavailable && "text-base font-normal text-muted-foreground",
          )}
        >
          {value}
        </strong>
        {change !== undefined && (
          <span className="font-mono text-xs whitespace-nowrap tabular-nums">
            {(good || bad) && <StatusMark status={good ? "ready" : "error"} className="mr-1.5" />}
            {formatChange(change)}
          </span>
        )}
      </div>
      {hint && <p className="mt-1 text-xs text-muted-foreground">{hint}</p>}
      {trend && trend.length > 1 && <Sparkline values={trend} className="mt-auto pt-3" />}
    </section>
  );
}
