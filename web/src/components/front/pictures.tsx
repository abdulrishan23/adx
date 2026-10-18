import { useId } from "react";
import type { BarRow } from "@/api/fronts";
import { formatNumber } from "@/lib/format";
import { cn } from "@/lib/utils";

// The pictures a front composes from. Each takes a `size` so the same picture
// draws small in a panel and large on a Focus slide. Everything is ink on
// paper or paper on ink; colour only appears on status marks.

export type Size = "small" | "card" | "panel" | "wall";

// Bars draws one bar per value with the last one in full ink.
export function Bars({
  values,
  labels,
  size = "panel",
}: {
  values: number[];
  labels: string[];
  size?: Size;
}) {
  const max = Math.max(...values, 1);
  return (
    <div
      className={cn(
        "flex items-end gap-[6%] sm:gap-2",
        size === "wall" ? "h-[min(44vh,420px)]" : "h-28",
      )}
    >
      {values.map((v, i) => (
        <div key={labels[i] ?? i} className="flex h-full flex-1 flex-col justify-end gap-2">
          <div
            className={cn(
              "w-full animate-grow",
              i === values.length - 1 ? "bg-foreground" : "bg-chart-4",
            )}
            style={{ height: `${(v / max) * 100}%`, animationDelay: `${i * 30}ms` }}
            title={`${labels[i]}: ${formatNumber(v)}`}
          />
          <span
            className={cn(
              "hidden text-center font-mono text-muted-foreground sm:block",
              size === "wall" ? "text-xs" : "text-[0.6rem]",
            )}
          >
            {labels[i]}
          </span>
        </div>
      ))}
    </div>
  );
}

// HBars draws labelled horizontal bars: shares, pacing, a funnel.
export function HBars({
  rows,
  size = "panel",
  labelWidth,
}: {
  rows: BarRow[];
  size?: Size;
  labelWidth?: string;
}) {
  return (
    <div className={cn("grid", size === "wall" ? "gap-3" : size === "card" ? "gap-2.5" : "gap-2")}>
      {rows.map((r, i) => (
        <div
          key={r.label}
          className={cn("grid items-center", size === "wall" ? "gap-4 text-lg" : "gap-3 text-sm")}
          style={{
            gridTemplateColumns: `${labelWidth ?? (size === "wall" ? "12rem" : "7rem")} 1fr auto`,
          }}
        >
          <span className="truncate">{r.label}</span>
          <div
            className={cn("bg-muted", size === "wall" ? "h-4" : size === "card" ? "h-5" : "h-3")}
          >
            <div
              className="h-full animate-widen bg-foreground"
              style={{ width: `${Math.max(r.share, 1)}%`, animationDelay: `${i * 60}ms` }}
            />
          </div>
          <span className="min-w-[3.5rem] text-right font-mono text-muted-foreground tabular-nums">
            {r.value}
          </span>
        </div>
      ))}
    </div>
  );
}

// Trend is the sparkline a glance or a stat carries: a line and a dot on today.
export function Trend({
  values,
  className,
  tall,
  draw,
}: {
  values: number[];
  className?: string;
  tall?: boolean;
  // Draw the line in from the left when it appears.
  draw?: boolean;
}) {
  const clip = useId();
  if (values.length < 2) {
    return null;
  }
  const max = Math.max(...values, 1);
  const w = 100;
  const h = 28;
  const pts = values.map((v, i) => `${(i / (values.length - 1)) * w},${h - (v / max) * h}`);
  return (
    <svg
      viewBox={`0 0 ${w} ${h}`}
      preserveAspectRatio="none"
      aria-hidden="true"
      className={cn(
        "block w-full overflow-visible text-foreground",
        tall ? "h-24" : "h-7",
        className,
      )}
    >
      {draw && (
        <defs>
          <clipPath id={clip}>
            <rect x="0" y="-4" height={h + 8} width={w} className="animate-reveal" />
          </clipPath>
        </defs>
      )}
      <polyline
        points={pts.join(" ")}
        clipPath={draw ? `url(#${clip})` : undefined}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        vectorEffect="non-scaling-stroke"
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  );
}

// Pie is a donut, for share questions only. Slices go from ink to light. It's
// the one round thing in Adx; a square pie reads as a block, not a share.
export function Pie({
  slices,
  size = "panel",
}: {
  slices: { label: string; value: number }[];
  size?: Size;
}) {
  const total = slices.reduce((a, s) => a + s.value, 0) || 1;
  // Ink, then two clearly separated lighter steps, with a paper gap between
  // slices so the ring reads as parts rather than one smudge.
  const shades = ["var(--chart-1)", "var(--chart-3)", "var(--chart-5)", "var(--chart-4)"];
  const gap = 1.2;
  let acc = 0;
  const stops = slices
    .map((s, i) => {
      const from = (acc / total) * 100;
      acc += s.value;
      const to = (acc / total) * 100;
      return `${shades[i % shades.length]} ${from + gap / 2}% ${Math.max(from + gap / 2, to - gap / 2)}%, var(--card) ${to - gap / 2}% ${to + gap / 2}%`;
    })
    .join(", ");
  const wall = size === "wall";
  return (
    <div className={cn("flex items-center", wall ? "gap-10" : "gap-5")}>
      <div
        role="img"
        aria-label={slices
          .map((s) => `${s.label} ${Math.round((s.value / total) * 100)}%`)
          .join(", ")}
        className={cn(
          "shrink-0 animate-rise rounded-full",
          wall ? "size-[min(34vh,300px)]" : size === "small" ? "size-28" : "size-24",
        )}
        style={{
          background: `conic-gradient(${stops})`,
          // A donut: the hole is cut with a mask, so the ring keeps its shades.
          maskImage: "radial-gradient(circle, transparent 60%, #000 61%)",
          WebkitMaskImage: "radial-gradient(circle, transparent 60%, #000 61%)",
        }}
      />
      <ul
        className={cn(
          "grid",
          wall ? "gap-2.5 text-lg" : size === "small" ? "gap-1.5 text-sm" : "gap-1 text-sm",
        )}
      >
        {slices.map((s, i) => (
          <li key={s.label} className="flex items-center gap-2">
            <i
              className={cn("inline-block shrink-0", wall ? "size-3.5" : "size-2.5")}
              style={{ background: shades[i % shades.length] }}
            />
            <span>{s.label}</span>
            <span className="ml-auto pl-4 font-mono text-muted-foreground tabular-nums">
              {Math.round((s.value / total) * 100)}%
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
