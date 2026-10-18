import { Link } from "@tanstack/react-router";
import { useCreativeProject } from "@/api/creative";
import type { Evidence } from "@/api/fronts";
import { StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { fileUrl } from "@/creative/files";
import { cn } from "@/lib/utils";
import { ItemList } from "./blocks";
import { CountUp } from "./count-up";
import { Bars, HBars, Trend } from "./pictures";

// EvidenceView draws what a beat brings with it. The same component serves
// Home (panel) and Focus (wall); only the size changes.
export function EvidenceView({
  evidence: e,
  size = "panel",
}: {
  evidence: Evidence;
  size?: "panel" | "wall";
}) {
  const wall = size === "wall";
  switch (e.kind) {
    case "pace":
      return (
        <div className="space-y-4">
          <HBars rows={e.rows} size={size} labelWidth={wall ? "10rem" : "6rem"} />
          <Trend values={e.trend} tall draw className={wall ? "h-40" : "h-14"} />
          <p className="font-mono text-[0.65rem] tracking-widest text-muted-foreground uppercase">
            Qualified leads, 14 days
          </p>
        </div>
      );
    case "rows":
      return <HBars rows={e.rows} size={size} labelWidth={wall ? "22rem" : "12rem"} />;
    case "bars":
      return <Bars values={e.values} labels={e.labels} size={size} />;
    case "figure":
      return (
        <div>
          <p
            className={cn(
              "flex items-baseline gap-3 font-heading font-semibold leading-none tracking-[-0.035em] tabular-nums",
              wall ? "text-[9rem]" : "text-6xl md:text-7xl",
            )}
          >
            <CountUp value={e.figure} to={Number(e.figure.replace(/,/g, "")) || undefined} />
            {e.unit && (
              <span
                className={cn(
                  "font-medium tracking-normal text-muted-foreground",
                  wall ? "text-3xl" : "text-lg",
                )}
              >
                {e.unit}
              </span>
            )}
          </p>
          <p className={cn("mt-3 text-muted-foreground", wall ? "text-xl" : "text-sm")}>
            {e.detail}
          </p>
        </div>
      );
    case "ads":
      return <AdStrip projectId={e.projectId} items={e.items} size={size} />;
    case "leads":
      return (
        <ul className="divide-y">
          {e.items.map((l) => (
            <li key={l.id} className={cn("flex items-baseline gap-4", wall ? "py-4" : "py-2.5")}>
              <Link
                to="/leads/$leadId"
                params={{ leadId: String(l.id) }}
                className={cn(
                  "shrink-0 font-medium hover:underline",
                  wall ? "font-heading text-2xl" : "text-sm",
                )}
              >
                {l.name}
              </Link>
              <span
                className={cn(
                  "min-w-0 flex-1 truncate text-muted-foreground",
                  wall ? "text-lg" : "text-sm",
                )}
              >
                {l.detail}
              </span>
              <span
                className={cn(
                  "shrink-0 font-mono text-muted-foreground",
                  wall ? "text-base" : "text-xs",
                )}
              >
                {l.waited}
              </span>
            </li>
          ))}
        </ul>
      );
    case "list":
      return <ItemList items={e.items} size={size} />;
    case "table":
      return (
        <div className="overflow-x-auto border">
          <Table>
            <THead>
              <TR>
                {e.columns.map((c) => (
                  <TH key={c.label} numeric={c.numeric}>
                    {c.label}
                  </TH>
                ))}
              </TR>
            </THead>
            <TBody>
              {e.rows.map((r) => (
                <TR key={r.cells.join("|")}>
                  {r.cells.map((cell, i) => (
                    <TD key={e.columns[i]?.label} numeric={e.columns[i]?.numeric}>
                      {i === 0 && r.leadId !== undefined ? (
                        <Link
                          to="/leads/$leadId"
                          params={{ leadId: String(r.leadId) }}
                          className="font-medium hover:underline"
                        >
                          {cell}
                        </Link>
                      ) : (
                        cell
                      )}
                    </TD>
                  ))}
                </TR>
              ))}
            </TBody>
          </Table>
        </div>
      );
  }
}

// AdStrip shows the ads a beat is about, with their real previews when the
// creative service has them, and the code alone when it doesn't.
function AdStrip({
  projectId,
  items,
  size,
}: {
  projectId: number;
  items: { code: string; status: "ready" | "caution" | "error" | "pending"; note: string }[];
  size: "panel" | "wall";
}) {
  const project = useCreativeProject(projectId);
  const wall = size === "wall";
  const ads = project.data?.concepts.flatMap((c) => c.ads) ?? [];
  return (
    <div className="flex flex-wrap gap-5">
      {items.map((it) => {
        const ad = ads.find((a) => a.code === it.code);
        const preview = ad?.current ? fileUrl(ad.current.preview) : null;
        return (
          <figure key={it.code} className={cn("min-w-0", wall ? "w-56" : "w-36")}>
            <div
              className={cn(
                "aspect-[9/16] w-full border bg-muted bg-cover bg-center",
                wall ? "border-chart-4" : "",
              )}
              style={preview ? { backgroundImage: `url(${preview})` } : undefined}
            >
              {!preview && (
                <span className="flex h-full items-end p-2 font-mono text-[0.6rem] text-muted-foreground">
                  {it.code}
                </span>
              )}
            </div>
            <figcaption
              className={cn("mt-2 flex items-start gap-2", wall ? "text-base" : "text-xs")}
            >
              <StatusMark status={it.status} className="mt-1.5" />
              <span className="text-muted-foreground">{it.note}</span>
            </figcaption>
          </figure>
        );
      })}
    </div>
  );
}
