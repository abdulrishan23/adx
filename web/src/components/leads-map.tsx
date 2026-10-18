import { geoEqualEarth, geoPath } from "d3-geo";
import type { Feature, FeatureCollection, Geometry } from "geojson";
import { useEffect, useMemo, useState } from "react";
import { feature } from "topojson-client";
import type { GeometryCollection, Topology } from "topojson-specification";
import worldUrl from "world-atlas/countries-110m.json?url";
import type { MarketRow } from "@/api/models";
import { formatNumber, formatPercent } from "@/lib/format";

// Phone code to the country's ISO 3166 numeric code, which is the map data's ID.
export const COUNTRY_ID: Record<string, string> = {
  "+971": "784",
  "+966": "682",
  "+91": "356",
  "+92": "586",
  "+44": "826",
  "+7": "643",
};

const W = 640;
const H = 380;

// The part of the world Reportage's buyers come from: the UK to India.
const FRAME = {
  type: "MultiPoint" as const,
  coordinates: [
    [-12, 4],
    [98, 4],
    [-12, 62],
    [98, 62],
  ],
};

type Countries = FeatureCollection<Geometry, { name: string }>;

// Darker means more leads. The scale is logarithmic and runs from the smallest
// market to the largest: the UAE has several times any other market, and a
// straight scale from zero would paint every other country the same grey.
// In dark mode the same mix runs dark to bright.
function shade(leads: number, min: number, max: number) {
  const lo = Math.log(Math.max(1, min));
  const hi = Math.log(Math.max(2, max));
  const t = hi > lo ? (Math.log(Math.max(1, leads)) - lo) / (hi - lo) : 1;
  const pct = Math.round(22 + Math.min(1, Math.max(0, t)) * 78);
  return `color-mix(in oklab, var(--chart-1) ${pct}%, var(--chart-5))`;
}

// LeadsMap shades each market's country by its leads. `active` is the market
// highlighted from either the map or the table beside it.
export function LeadsMap({
  rows,
  active,
  onActive,
}: {
  rows: MarketRow[];
  active: string | null;
  onActive: (code: string | null) => void;
}) {
  const [world, setWorld] = useState<Countries | null>(null);

  useEffect(() => {
    let live = true;
    fetch(worldUrl)
      .then((r) => r.json())
      .then((topo: Topology<{ countries: GeometryCollection<{ name: string }> }>) => {
        if (live) setWorld(feature(topo, topo.objects.countries) as Countries);
      })
      .catch(() => {
        // The table beside the map still has every figure.
      });
    return () => {
      live = false;
    };
  }, []);

  const projection = useMemo(
    () =>
      geoEqualEarth().fitExtent(
        [
          [0, 0],
          [W, H],
        ],
        FRAME,
      ),
    [],
  );
  const path = geoPath(projection);
  const byId = new Map(
    rows.filter((r) => COUNTRY_ID[r.code]).map((r) => [COUNTRY_ID[r.code] as string, r]),
  );
  const counts = [...byId.values()].map((r) => r.leads);
  const max = Math.max(1, ...counts);
  const min = Math.min(max, ...counts);
  const activeId = active ? COUNTRY_ID[active] : undefined;
  const activeRow = active ? rows.find((r) => r.code === active) : undefined;
  const activeShape = activeId ? world?.features.find((f) => String(f.id) === activeId) : undefined;
  const [[x0, y0], [x1, y1]] = activeShape
    ? path.bounds(activeShape)
    : [
        [0, 0],
        [0, 0],
      ];

  return (
    <figure className="relative">
      <svg
        viewBox={`0 0 ${W} ${H}`}
        className="h-auto w-full"
        role="img"
        aria-label={`Leads by country: ${rows
          .filter((r) => COUNTRY_ID[r.code])
          .map((r) => `${r.market} ${r.leads}`)
          .join(", ")}`}
      >
        {world?.features.map((f: Feature<Geometry, { name: string }>, i) => {
          const id = String(f.id);
          const row = byId.get(id);
          const isActive = id === activeId;
          return (
            // biome-ignore lint/a11y/noStaticElementInteractions: hover only links the map to the table, which has the same figures
            <path
              // A few disputed territories have no ID in the map data.
              key={f.id === undefined ? `n${i}` : id}
              d={path(f) ?? undefined}
              fill={row ? shade(row.leads, min, max) : "var(--muted)"}
              fillOpacity={activeId && row && !isActive ? 0.45 : 1}
              stroke="var(--card)"
              strokeWidth={0.6}
              onMouseEnter={row ? () => onActive(row.code) : undefined}
              onMouseLeave={row ? () => onActive(null) : undefined}
              className={row ? "cursor-pointer" : undefined}
            />
          );
        })}
        {/* The highlighted outline goes last: neighbours drawn after the country
            would otherwise paint their pale borders over its shared edges. */}
        {activeShape && (
          <path
            d={path(activeShape) ?? undefined}
            fill="none"
            stroke="var(--foreground)"
            strokeWidth={1.5}
            strokeLinejoin="round"
            className="pointer-events-none"
          />
        )}
        {/* A country this small is easy to miss, so a ring marks it. */}
        {activeShape && x1 - x0 < 24 && (
          <circle
            cx={(x0 + x1) / 2}
            cy={(y0 + y1) / 2}
            r={14}
            fill="none"
            stroke="var(--foreground)"
            strokeWidth={1.5}
            className="pointer-events-none"
          />
        )}
      </svg>

      <div className="pointer-events-none absolute bottom-2 left-2 border bg-card/90 px-2.5 py-2 text-[0.65rem]">
        <div
          className="h-2 w-32"
          style={{
            background: `linear-gradient(to right, ${shade(min, min, max)}, ${shade(max, min, max)})`,
          }}
        />
        <div className="mt-1 flex justify-between font-mono text-muted-foreground tabular-nums">
          <span>{formatNumber(min)}</span>
          <span>leads</span>
          <span>{formatNumber(max)}</span>
        </div>
      </div>

      {activeRow && (
        <figcaption className="pointer-events-none absolute right-2 bottom-2 border bg-popover px-3 py-2 text-xs shadow-sm">
          <span className="font-medium">{activeRow.market}</span>{" "}
          <span className="font-mono text-muted-foreground">{activeRow.code}</span>
          <br />
          {formatNumber(activeRow.leads)} leads · {formatNumber(activeRow.qualified)} qualified (
          {formatPercent(activeRow.leads ? activeRow.qualified / activeRow.leads : null)})
        </figcaption>
      )}
    </figure>
  );
}
