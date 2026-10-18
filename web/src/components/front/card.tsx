import { Link } from "@tanstack/react-router";
import { MessageSquareText } from "lucide-react";
import { useCreativeProject } from "@/api/creative";
import type { Card } from "@/api/fronts";
import { StatusMark } from "@/components/ui/status-chip";
import { fileUrl } from "@/creative/files";
import { cn } from "@/lib/utils";
import { CountUp } from "./count-up";
import { HBars, Pie } from "./pictures";

export const DOOR_LABEL = {
  create: "Create",
  perform: "Perform",
  convert: "Convert",
  any: "Adx",
} as const;

// CardView is one card from the desk: why it's there, its figure or picture,
// a question to put to Adx and at most one action. Home's carousel and the
// Inbox draw the same card, so a card reads the same wherever it shows.
export function CardView({
  card: c,
  delay,
  onAsk,
}: {
  card: Card;
  delay: number;
  onAsk: (q: string) => void;
}) {
  return (
    <article
      className="flex h-[16.5rem] min-w-0 animate-rise flex-col border bg-card transition-colors hover:border-foreground/40"
      style={{ animationDelay: `${delay}ms` }}
    >
      <div className="px-4 pt-3">
        <p className="flex items-center gap-1.5 font-mono text-[0.6rem] tracking-widest text-muted-foreground uppercase">
          <StatusMark status={c.status} />
          {DOOR_LABEL[c.door]}
        </p>
        <h2 className="mt-1 truncate font-heading text-[0.95rem] font-semibold leading-tight">
          {c.title}
        </h2>
      </div>
      <p className="line-clamp-2 px-4 pt-1 text-xs text-muted-foreground">{c.why}</p>
      <div className="flex min-h-0 flex-1 flex-col justify-end gap-2.5 overflow-hidden px-4 pt-2">
        {c.figure && (
          <p className="flex items-baseline gap-1.5 font-heading text-[2.75rem] font-semibold leading-none tracking-[-0.03em] tabular-nums">
            <CountUp value={c.figure.value} to={c.figure.count} />
            {c.figure.unit && (
              <span className="text-sm font-medium tracking-normal text-muted-foreground">
                {c.figure.unit}
              </span>
            )}
          </p>
        )}
        {c.evidence && <SmallEvidence evidence={c.evidence} />}
      </div>
      <div className="mt-3 flex items-stretch border-t">
        <button
          type="button"
          onClick={() => onAsk(c.question)}
          className="flex flex-1 items-center gap-2 px-4 py-2 text-left text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <MessageSquareText className="size-3.5 shrink-0" />
          Talk about this
        </button>
        {c.action && (
          <Link
            to={c.action.to}
            className={cn(
              "flex items-center border-l px-4 text-xs font-medium transition-colors",
              c.action.primary
                ? "bg-foreground text-background hover:bg-foreground/85"
                : "hover:bg-muted",
            )}
          >
            {c.action.label}
          </Link>
        )}
      </div>
    </article>
  );
}

// SmallEvidence is a card's picture at carousel size: the essentials only,
// so a card stays a glance. Present shows the full picture.
export function SmallEvidence({ evidence: e }: { evidence: NonNullable<Card["evidence"]> }) {
  switch (e.kind) {
    case "pace":
      return <HBars rows={e.rows} labelWidth="3.5rem" size="card" />;
    case "bars":
      return <MiniBars values={e.values} />;
    case "pie":
      return <Pie slices={e.slices} size="small" />;
    case "ads":
      return <AdThumbs projectId={e.projectId} items={e.items} />;
    case "rows":
      return <StackedRows rows={e.rows} />;
    case "leads":
      return (
        <ul className="divide-y text-sm">
          {e.items.slice(0, 2).map((l) => (
            <li key={l.id} className="flex items-baseline gap-2 py-1.5">
              <span className="truncate font-medium">{l.name}</span>
              <span className="ml-auto shrink-0 font-mono text-muted-foreground">{l.waited}</span>
            </li>
          ))}
        </ul>
      );
    default:
      return null;
  }
}

// StackedRows puts each label over its bar, so long labels such as search
// terms keep their words at card width.
function StackedRows({ rows }: { rows: { label: string; value: string; share: number }[] }) {
  return (
    <ul className="grid gap-1">
      {rows.map((r, i) => (
        <li key={r.label}>
          <div className="flex items-baseline gap-3 text-xs">
            <span className="min-w-0 flex-1 truncate">{r.label}</span>
            <span className="shrink-0 font-mono text-muted-foreground tabular-nums">{r.value}</span>
          </div>
          <div className="mt-0.5 h-1 bg-muted">
            <div
              className="h-full animate-widen bg-foreground"
              style={{ width: `${Math.max(r.share, 1)}%`, animationDelay: `${i * 60}ms` }}
            />
          </div>
        </li>
      ))}
    </ul>
  );
}

function MiniBars({ values }: { values: number[] }) {
  const max = Math.max(...values, 1);
  return (
    <div className="flex h-24 items-end gap-[3px]">
      {values.map((v, i) => (
        <span
          // biome-ignore lint/suspicious/noArrayIndexKey: bars are positional
          key={i}
          className={cn(
            "flex-1 animate-grow",
            i === values.length - 1 ? "bg-foreground" : "bg-chart-4",
          )}
          style={{ height: `${(v / max) * 100}%` }}
        />
      ))}
    </div>
  );
}

function AdThumbs({
  projectId,
  items,
}: {
  projectId: number;
  items: { code: string; status: "ready" | "caution" | "error" | "pending"; note: string }[];
}) {
  const project = useCreativeProject(projectId);
  const ads = project.data?.concepts.flatMap((c) => c.ads) ?? [];
  return (
    <div className="flex gap-4">
      {items.map((it) => {
        const ad = ads.find((a) => a.code === it.code);
        const preview = ad?.current ? fileUrl(ad.current.preview) : null;
        return (
          <figure key={it.code} className="w-16 shrink-0">
            <div
              className="aspect-[9/16] w-full border bg-muted bg-cover bg-center"
              style={preview ? { backgroundImage: `url(${preview})` } : undefined}
            />
            <figcaption className="mt-1.5 flex items-start gap-1 text-[0.65rem] leading-tight text-muted-foreground">
              <StatusMark status={it.status} className="mt-0.5" />
              {it.note}
            </figcaption>
          </figure>
        );
      })}
    </div>
  );
}
