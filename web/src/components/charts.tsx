import { useId } from "react";
import { cn } from "@/lib/utils";

// Hand-drawn SVG charts in the monochrome palette. Small on purpose: sparklines,
// a bar-and-line daily chart and proportion bars cover the screens so far.

export function Sparkline({
  values,
  className,
  area = true,
}: {
  values: number[];
  className?: string;
  area?: boolean;
}) {
  const gradient = useId();
  if (values.length < 2) {
    return null;
  }
  const max = Math.max(...values);
  const min = Math.min(...values);
  const span = max - min || 1;
  const w = 100;
  const h = 32;
  const points = values.map((v, i) => [
    (i / (values.length - 1)) * w,
    h - 2 - ((v - min) / span) * (h - 4),
  ]);
  const line = points.map(([x, y]) => `${x},${y}`).join(" ");
  return (
    <svg
      viewBox={`0 0 ${w} ${h}`}
      preserveAspectRatio="none"
      className={cn("h-8 w-full text-foreground", className)}
      aria-hidden="true"
    >
      {area && (
        <>
          <defs>
            <linearGradient id={gradient} x1="0" x2="0" y1="0" y2="1">
              <stop offset="0" stopColor="currentColor" stopOpacity="0.14" />
              <stop offset="1" stopColor="currentColor" stopOpacity="0" />
            </linearGradient>
          </defs>
          <polygon points={`0,${h} ${line} ${w},${h}`} fill={`url(#${gradient})`} />
        </>
      )}
      <polyline
        points={line}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        vectorEffect="non-scaling-stroke"
        strokeLinejoin="round"
      />
    </svg>
  );
}

// DailyChart draws spend as bars and qualified leads as a line on its own
// scale, so a marketer sees whether money and results move together.
export function DailyChart({
  points,
  className,
  label = "Daily spend and qualified leads",
  describe = (p) =>
    `${p.date}: spend ${Math.round(p.bar / 100).toLocaleString("en")}, qualified ${p.line}`,
}: {
  points: { date: string; bar: number; line: number }[];
  className?: string;
  label?: string;
  describe?: (p: { date: string; bar: number; line: number }) => string;
}) {
  if (points.length === 0) {
    return null;
  }
  const w = 720;
  const h = 200;
  const pad = { top: 8, bottom: 22 };
  const plotH = h - pad.top - pad.bottom;
  const barMax = Math.max(...points.map((p) => p.bar)) || 1;
  const lineMax = Math.max(...points.map((p) => p.line)) || 1;
  const step = w / points.length;
  const barW = Math.max(1, step * 0.62);
  const line = points
    .map((p, i) => `${i * step + step / 2},${pad.top + plotH - (p.line / lineMax) * plotH}`)
    .join(" ");
  const labelEvery = Math.ceil(points.length / 6);
  return (
    <svg
      viewBox={`0 0 ${w} ${h}`}
      className={cn("w-full", className)}
      role="img"
      aria-label={label}
    >
      {[0.25, 0.5, 0.75, 1].map((f) => (
        <line
          key={f}
          x1="0"
          x2={w}
          y1={pad.top + plotH - f * plotH}
          y2={pad.top + plotH - f * plotH}
          className="stroke-border"
          strokeDasharray="2 4"
        />
      ))}
      {points.map((p, i) => (
        <rect
          key={p.date}
          x={i * step + (step - barW) / 2}
          y={pad.top + plotH - (p.bar / barMax) * plotH}
          width={barW}
          height={(p.bar / barMax) * plotH}
          rx="1"
          className="fill-chart-4"
        >
          <title>{describe(p)}</title>
        </rect>
      ))}
      <polyline
        points={line}
        fill="none"
        className="stroke-foreground"
        strokeWidth="2"
        strokeLinejoin="round"
      />
      {points.map((p, i) =>
        i % labelEvery === 0 ? (
          <text
            key={p.date}
            x={i === 0 ? 0 : i * step + step / 2}
            y={h - 6}
            textAnchor={i === 0 ? "start" : "middle"}
            className="fill-muted-foreground font-mono text-[10px]"
          >
            {new Date(p.date).toLocaleDateString("en-GB", { day: "numeric", month: "short" })}
          </text>
        ) : null,
      )}
    </svg>
  );
}

export function ChartLegend({ items }: { items: { label: string; kind: "bar" | "line" }[] }) {
  return (
    <div className="flex items-center gap-4 text-xs text-muted-foreground">
      {items.map((item) => (
        <span key={item.label} className="flex items-center gap-1.5">
          {item.kind === "bar" ? (
            <span className="size-2.5 rounded-none bg-chart-4" />
          ) : (
            <span className="h-0.5 w-3.5 bg-foreground" />
          )}
          {item.label}
        </span>
      ))}
    </div>
  );
}

// ProportionBar splits one total into parts, darkest first.
export function ProportionBar({
  parts,
  className,
}: {
  parts: { label: string; value: number }[];
  className?: string;
}) {
  const total = parts.reduce((a, p) => a + p.value, 0);
  const shades = ["bg-chart-1", "bg-chart-3", "bg-chart-5"];
  if (total === 0) {
    return <div className={cn("h-1.5 w-full rounded-none bg-muted", className)} />;
  }
  return (
    <div
      className={cn("flex h-1.5 w-full overflow-hidden rounded-none bg-muted", className)}
      title={parts.map((p) => `${p.label} ${p.value}`).join(" · ")}
    >
      {parts.map((p, i) => (
        <span
          key={p.label}
          className={shades[i % shades.length]}
          style={{ width: `${(p.value / total) * 100}%` }}
        />
      ))}
    </div>
  );
}

export function Meter({
  value,
  max,
  marker,
  className,
}: {
  value: number;
  max: number;
  marker?: number;
  className?: string;
}) {
  const pct = Math.min(100, (value / (max || 1)) * 100);
  const over = value > max;
  return (
    <div className={cn("relative h-2 w-full rounded-none bg-muted", className)}>
      <div
        className={cn("h-full rounded-none", over ? "bg-status-caution" : "bg-foreground")}
        style={{ width: `${pct}%` }}
      />
      {marker !== undefined && (
        <span
          className="absolute -top-1 h-4 w-0.5 bg-foreground/50"
          style={{ left: `${Math.min(100, (marker / (max || 1)) * 100)}%` }}
          aria-hidden="true"
        />
      )}
    </div>
  );
}
