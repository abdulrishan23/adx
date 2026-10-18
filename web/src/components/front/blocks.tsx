import { Link } from "@tanstack/react-router";
import type { Action, Block, Headline, ListItem } from "@/api/fronts";
import { Eyebrow } from "@/components/page";
import { buttonVariants } from "@/components/ui/button";
import { StatusMark } from "@/components/ui/status-chip";
import { cn } from "@/lib/utils";
import { CountUp } from "./count-up";
import { Bars, HBars, Trend } from "./pictures";

// Blocks are the panels a front composes itself from. Each carries its "why":
// the rule that put it on the front, shown above the title.

export function ActionLink({ action, className }: { action: Action; className?: string }) {
  return (
    <Link
      to={action.to}
      className={cn(
        buttonVariants({ variant: action.primary ? "default" : "outline", size: "sm" }),
        className,
      )}
    >
      {action.label}
    </Link>
  );
}

export function ItemList({
  items,
  size = "panel",
}: {
  items: ListItem[];
  size?: "panel" | "wall";
}) {
  const wall = size === "wall";
  return (
    <ul className="divide-y">
      {items.map((it) => (
        <li key={it.title} className={cn("flex items-start gap-4", wall ? "py-4" : "py-3")}>
          <StatusMark status={it.status} className={cn(wall ? "mt-3 size-3" : "mt-2")} />
          <div className="min-w-0 flex-1">
            <p
              className={cn(
                "font-medium",
                wall ? "font-heading text-2xl leading-tight" : "text-sm",
              )}
            >
              {it.title}
            </p>
            {it.detail && (
              <p className={cn("text-muted-foreground", wall ? "mt-1 text-lg" : "mt-0.5 text-sm")}>
                {it.detail}
              </p>
            )}
          </div>
          {it.tag && (
            <span
              className={cn(
                "shrink-0 font-mono text-muted-foreground",
                wall ? "mt-2 text-sm" : "mt-1 text-[0.65rem]",
              )}
            >
              {it.tag}
            </span>
          )}
          {it.action && !wall && <ActionLink action={it.action} />}
        </li>
      ))}
    </ul>
  );
}

// BlockBody draws a block's content without panel chrome, so a Focus slide can
// draw the same block a panel does.
export function BlockBody({ block, size = "panel" }: { block: Block; size?: "panel" | "wall" }) {
  switch (block.kind) {
    case "list":
      return <ItemList items={block.items} size={size} />;
    case "bars":
      return <Bars values={block.values} labels={block.labels} size={size} />;
    case "hbars":
      return <HBars rows={block.rows} size={size} />;
  }
}

// HeadlineBand is the one thing that matters today on a screen: a big figure,
// a sentence of why, actions, and a picture. Always ink, in both themes.
export function HeadlineBand({ headline }: { headline: Headline }) {
  const h = headline;
  return (
    <section className="ink grid animate-rise gap-8 px-7 py-8 md:grid-cols-[1.2fr_1fr] md:gap-10 md:px-9 md:py-9">
      <div className="min-w-0">
        <StatusMark status={h.status} className="size-3" />
        <p className="mt-4 flex items-baseline gap-3 font-heading text-6xl font-semibold tracking-[-0.035em] tabular-nums md:text-7xl lg:text-[5.5rem] lg:leading-none">
          <CountUp value={h.figure} to={h.count} />
          {h.unit && (
            <span className="text-xl font-medium tracking-normal text-muted-foreground">
              {h.unit}
            </span>
          )}
        </p>
        <h2 className="mt-5 font-heading text-2xl font-semibold leading-tight">{h.title}</h2>
        <p className="mt-2 max-w-[52ch] text-[0.95rem] leading-relaxed text-chart-2">{h.text}</p>
        {h.actions.length > 0 && (
          <div className="mt-5 flex flex-wrap gap-2">
            {h.actions.map((a) => (
              <ActionLink key={a.label} action={a} />
            ))}
          </div>
        )}
      </div>
      <div className="flex min-w-0 flex-col justify-center gap-3">
        <Eyebrow>{h.chart.rowsLabel}</Eyebrow>
        <HBars rows={h.chart.rows} labelWidth="5rem" />
        <Trend values={h.chart.trend} tall className="mt-3" />
        <Eyebrow>{h.chart.trendLabel}</Eyebrow>
      </div>
    </section>
  );
}
