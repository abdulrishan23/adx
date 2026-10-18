import { getRouteApi, Link } from "@tanstack/react-router";
import { ArrowDown, ArrowUp, ArrowUpRight, ChevronRight } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useAlerts, useProposals } from "@/api/adx";
import { useAnalytics } from "@/api/analytics";
import type { Headline } from "@/api/fronts";
import { USE_MOCK } from "@/api/mock";
import { useSession } from "@/app/session";
import { AnalyticsDatePicker } from "@/components/analytics-date-picker";
import { HeadlineBand } from "@/components/front/blocks";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { PlatformMark } from "@/components/platform";
import { ProjectFilter } from "@/components/project-filter";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import {
  type Analytics,
  type AnalyticsMetric,
  type AnalyticsRow,
  type AnalyticsSearch,
  type AnalyticsTotals,
  dateShift,
  rangeError,
} from "@/lib/analytics";
import {
  change,
  formatChange,
  formatDate,
  formatMoneyShort,
  formatNumber,
  formatPercent,
  NOT_AVAILABLE,
} from "@/lib/format";
import { released } from "@/lib/release";
import { cn } from "@/lib/utils";

const route = getRouteApi("/app/analytics");
const METRICS: { key: AnalyticsMetric; label: string; money?: boolean }[] = [
  { key: "spendMinor", label: "Spend", money: true },
  { key: "revenueMinor", label: "Revenue", money: true },
  { key: "roas", label: "ROAS" },
  { key: "leads", label: "Leads" },
  { key: "qualified", label: "Qualified leads" },
  { key: "cpq", label: "Cost per qualified", money: true },
  { key: "deals", label: "Done-Deals" },
];
const metricValue = (key: AnalyticsMetric, n: number | null) =>
  n === null
    ? NOT_AVAILABLE
    : key === "roas"
      ? `${n.toFixed(1)}×`
      : key === "spendMinor" || key === "cpq" || key === "revenueMinor"
        ? formatMoneyShort(Math.round(n))
        : formatNumber(n);
const shortDate = (date: string) => formatDate(`${date}T00:00:00Z`);
const datesLabel = (from: string, to: string) =>
  `${shortDate(from)} – ${formatDate(`${to}T00:00:00Z`, { day: "numeric", month: "short", year: "numeric" })}`;

export function AnalyticsPage() {
  const search = route.useSearch();
  const navigate = route.useNavigate();
  const update = (next: Partial<AnalyticsSearch>) => navigate({ search: { ...search, ...next } });
  const analytics = useAnalytics(search);
  const error = rangeError(search);
  return (
    <>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            Analytics{" "}
            {USE_MOCK && (
              <span className="border px-2 py-1 font-sans text-xs font-normal tracking-normal text-muted-foreground">
                Sample data
              </span>
            )}
          </span>
        }
        actions={
          <div className="flex flex-wrap items-center gap-2">
            {(search.platform !== "all" || search.project !== "all") && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => update({ platform: "all", project: "all" })}
              >
                Reset filters
              </Button>
            )}
            <Select
              aria-label="Advertising source"
              className="h-10"
              value={search.platform}
              onValueChange={(platform) => update({ platform })}
              options={[
                { value: "all", label: "All ad sources" },
                { value: "meta", label: "Meta", icon: <PlatformMark platform="meta" /> },
                { value: "google", label: "Google", icon: <PlatformMark platform="google" /> },
              ]}
            />
            <ProjectFilter
              value={search.project}
              projects={analytics.data?.projectOptions ?? []}
              onChange={(project) => update({ project })}
            />
            <AnalyticsDatePicker search={search} onChange={update} />
          </div>
        }
        className="mb-5"
      />
      {error ? (
        <p role="alert" className="border border-status-caution/40 p-4 text-sm">
          {error}
        </p>
      ) : analytics.isPending ? (
        <LoadingRows rows={12} />
      ) : analytics.error ? (
        <ErrorNote error={analytics.error} />
      ) : (
        analytics.data && <AnalyticsBody data={analytics.data} search={search} onChange={update} />
      )}
    </>
  );
}

// headlineFor leads the page with spend against the period before, and the
// one figure that moved most: qualified leads or their cost.
function headlineFor(data: Analytics, search: AnalyticsSearch): Headline {
  const cur = data.current;
  const prev = data.previous;
  const spend = cur.spendMinor;
  const short = spend === null ? NOT_AVAILABLE : formatMoneyShort(spend);
  const spendChange = change(spend, prev.spendMinor);
  const qualChange = change(cur.qualified, prev.qualified);
  const cpqChange = change(cur.cpq, prev.cpq);
  const compared = search.compare !== "none" && prev.spendMinor !== null;
  const title = compared
    ? `Spent ${datesLabel(search.from, search.to)}, ${formatChange(spendChange)} on the period before.`
    : `Spent ${datesLabel(search.from, search.to)}.`;
  const parts = [
    cur.qualified === null
      ? "Qualified leads are not available for this range."
      : `${formatNumber(cur.qualified)} qualified leads${compared && qualChange ? `, ${formatChange(qualChange)}` : ""}.`,
    cur.cpq === null
      ? ""
      : `Cost per qualified ${formatMoneyShort(cur.cpq)}${compared && cpqChange ? `, ${formatChange(cpqChange)}` : ""}.`,
  ];
  const status =
    cpqChange && cpqChange.direction === "up" && cpqChange.ratio > 0.1
      ? "caution"
      : cur.qualified === null
        ? "pending"
        : "ready";
  const max = Math.max(spend ?? 0, prev.spendMinor ?? 0, 1);
  const rows = [
    { label: "This period", value: short, share: ((spend ?? 0) / max) * 100 },
    ...(compared
      ? [
          {
            label: "Before",
            value: formatMoneyShort(prev.spendMinor),
            share: ((prev.spendMinor ?? 0) / max) * 100,
          },
        ]
      : []),
  ];
  return {
    status,
    figure: short.replace("AED ", ""),
    count: spend === null ? undefined : Number(short.replace(/[^\d.]/g, "")) || undefined,
    unit: spend === null ? undefined : "AED",
    title,
    text: parts.filter(Boolean).join(" "),
    chart: {
      kind: "pace",
      rowsLabel: "Spend",
      rows,
      trend: data.daily.map((d) => d.current?.qualified ?? 0),
      trendLabel: "Qualified leads per day",
    },
    actions: [],
  };
}

function AnalyticsBody({
  data,
  search,
  onChange,
}: {
  data: Analytics;
  search: AnalyticsSearch;
  onChange: (next: Partial<AnalyticsSearch>) => void;
}) {
  if (!data.projects.length)
    return (
      <EmptyState title="No campaigns in this selection">
        Try another project or advertising source.
      </EmptyState>
    );
  return (
    <div className="space-y-6">
      <HeadlineBand headline={headlineFor(data, search)} />
      {data.availableDays < data.days && (
        <p
          role="status"
          className="border border-status-caution/40 bg-status-caution/5 px-4 py-3 text-sm"
        >
          Only {data.availableDays} of {data.days} days have data. Period totals are not available;
          choose a shorter range to compare complete periods.
        </p>
      )}
      <section
        aria-label="Performance summary"
        className="grid grid-cols-2 border-t border-l bg-card xl:grid-cols-7"
      >
        {METRICS.map((m) => (
          <button
            key={m.key}
            type="button"
            aria-pressed={search.metric === m.key}
            onClick={() => onChange({ metric: m.key })}
            className={cn(
              "relative flex flex-col border-r border-b px-3 py-3 text-left outline-none hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
              search.metric === m.key &&
                "bg-muted/35 after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:bg-foreground",
            )}
          >
            <span className="flex min-h-8 items-baseline justify-between gap-1 text-xs text-muted-foreground">
              <span>{m.label}</span>
              {m.money && <span className="text-[10px]">AED</span>}
            </span>
            <strong
              className={cn(
                "mt-2 font-mono font-semibold tracking-tight whitespace-nowrap tabular-nums",
                data.current[m.key] !== null &&
                  "text-[2rem] sm:text-4xl xl:text-[2rem] 2xl:text-[2.5rem]",
                data.current[m.key] === null && "text-sm font-normal text-muted-foreground",
              )}
            >
              {m.money
                ? metricValue(m.key, data.current[m.key]).replace(/^AED\s/, "")
                : metricValue(m.key, data.current[m.key])}
            </strong>
            {search.compare !== "none" && (
              <span className="mt-2 text-sm">
                <Delta
                  prominent
                  current={data.current[m.key]}
                  previous={data.previous[m.key]}
                  cost={m.key === "cpq"}
                  neutral={m.key === "spendMinor"}
                />
              </span>
            )}
          </button>
        ))}
      </section>
      <TrendChart
        key={`${search.from}/${search.to}/${search.platform}/${search.project}`}
        data={data}
        metric={search.metric}
        compare={search.compare !== "none"}
        from={search.from}
        to={search.to}
      />
      <div className="grid gap-6 xl:grid-cols-[3fr_2fr]">
        <ProjectReturns
          rows={data.projects}
          onSelect={(row) => onChange({ project: Number(row.id) })}
        />
        <LeadQuality totals={data.current} />
      </div>
      <Breakdown
        title="By project"
        rows={data.projects}
        onSelect={(row) => onChange({ project: Number(row.id) })}
        showSales
        compare={search.compare !== "none"}
      />
      <Breakdown
        title="By ad source"
        rows={data.sources}
        onSelect={(row) => onChange({ platform: row.platform ?? "all" })}
        showSales
        compare={search.compare !== "none"}
      />
      <Attention />
    </div>
  );
}

function Delta({
  current,
  previous,
  cost = false,
  neutral = false,
  prominent = false,
}: {
  current: number | null;
  previous: number | null;
  cost?: boolean;
  neutral?: boolean;
  prominent?: boolean;
}) {
  const c = change(current, previous);
  if (current === null)
    return <span className="text-muted-foreground">Comparison unavailable</span>;
  if (previous === null)
    return <span className="text-muted-foreground">Earlier data unavailable</span>;
  if (previous === 0)
    return (
      <span className="text-muted-foreground">{current === 0 ? "No change" : "From zero"}</span>
    );
  if (!c) return <span className="text-muted-foreground">Comparison unavailable</span>;
  const colour =
    neutral || c.direction === "flat"
      ? "text-muted-foreground"
      : (c.direction === "down") === cost
        ? "text-status-ready"
        : "text-status-error";
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 font-mono tabular-nums",
        prominent &&
          c.direction !== "flat" &&
          "gap-1.5 text-base font-semibold [&_svg]:size-4 [&_svg]:stroke-[2.5]",
        colour,
      )}
    >
      {c.direction === "up" ? (
        <ArrowUp className="size-3" />
      ) : c.direction === "down" ? (
        <ArrowDown className="size-3" />
      ) : null}
      {formatChange(c)}
    </span>
  );
}

function TrendChart({
  data,
  metric,
  compare,
  from,
  to,
}: {
  data: Analytics;
  metric: AnalyticsMetric;
  compare: boolean;
  from: string;
  to: string;
}) {
  const [hovered, setHovered] = useState<number | null>(null);
  const chartContainer = useRef<HTMLButtonElement>(null);
  const [chartWidth, setChartWidth] = useState(880);
  useEffect(() => {
    if (!chartContainer.current) return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry) setChartWidth(Math.max(280, entry.contentRect.width));
    });
    observer.observe(chartContainer.current);
    return () => observer.disconnect();
  }, []);
  const index = hovered ?? data.daily.length - 1;
  const active = data.daily[index];
  const comparisonDate = dateShift(data.previousFrom, index);
  const comparisonDay =
    comparisonDate <= data.previousTo ? shortDate(comparisonDate) : "No matching day";
  const value = (p: Analytics["daily"][number]["current"]) =>
    p === null
      ? null
      : metric === "cpq"
        ? p.qualified > 0
          ? p.spendMinor / p.qualified
          : null
        : metric === "roas"
          ? p.revenueMinor !== null && p.spendMinor > 0
            ? p.revenueMinor / p.spendMinor
            : null
          : p[metric];
  const current = data.daily.map((p) => value(p.current));
  const previous = data.daily.map((p) => value(p.previous));
  const rawMax = Math.max(
    1,
    ...current.filter((v): v is number => v !== null),
    ...(compare ? previous.filter((v): v is number => v !== null) : []),
  );
  const magnitude = 10 ** Math.floor(Math.log10(rawMax));
  const max = Math.ceil(rawMax / magnitude) * magnitude;
  const w = chartWidth,
    h = 236,
    left = 66,
    right = 18,
    top = 14,
    bottom = 32;
  const width = w - left - right,
    height = h - top - bottom;
  const x = (i: number) =>
    left + (data.daily.length === 1 ? width / 2 : (i / (data.daily.length - 1)) * width);
  const y = (n: number) => top + height * (1 - n / max);
  function path(values: (number | null)[]) {
    let drawing = false;
    return values
      .map((v, i) => {
        if (v === null) {
          drawing = false;
          return "";
        }
        const command = drawing ? "L" : "M";
        drawing = true;
        return `${command}${x(i)},${y(v)}`;
      })
      .join(" ");
  }
  const label = METRICS.find((m) => m.key === metric)?.label;
  const money = metric === "spendMinor" || metric === "cpq" || metric === "revenueMinor";
  const currentValue = current[index] ?? null;
  const previousValue = previous[index] ?? null;
  const direction =
    !compare || currentValue === null || previousValue === null || currentValue === previousValue
      ? null
      : currentValue > previousValue
        ? "up"
        : "down";
  const lowerIsBetter = metric === "cpq" || metric === "spendMinor";
  const arrowColour =
    (direction === "down") === lowerIsBetter ? "fill-status-ready" : "fill-status-error";
  const calloutWidth = Math.min(260, width);
  const calloutHeight = compare ? 102 : 58;
  const pointY = y(current[index] ?? 0);
  const calloutX = Math.max(left, Math.min(w - right - calloutWidth, x(index) + 14));
  const calloutY = Math.max(
    top,
    Math.min(
      h - bottom - calloutHeight,
      pointY > top + calloutHeight + 12 ? pointY - calloutHeight - 12 : pointY + 12,
    ),
  );
  return (
    <Panel
      title={`${label} over time`}
      description={
        compare
          ? "Compared day by day with the dates shown"
          : "Daily results within your date filter"
      }
      action={
        <div className="flex flex-col items-start gap-1 text-xs text-muted-foreground">
          <span className="flex items-center gap-2">
            <span className="h-0.5 w-5 bg-foreground" />
            {datesLabel(from, to)}
          </span>
          {compare && (
            <span className="flex items-center gap-2">
              <span className="w-5 border-t-2 border-dashed border-muted-foreground" />
              {datesLabel(data.previousFrom, data.previousTo)}
            </span>
          )}
        </div>
      }
      bodyClassName="px-3 pt-4 pb-3 sm:px-5"
    >
      <button
        ref={chartContainer}
        type="button"
        className="block w-full cursor-default outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-label={`${label} chart. ${active ? shortDate(active.date) : ""}: ${metricValue(metric, current[index] ?? null)}.${compare ? ` ${active ? comparisonDay : ""}: ${metricValue(metric, previous[index] ?? null)}.` : ""} Use left and right arrow keys to move between days.`}
        onKeyDown={(e) => {
          const next =
            e.key === "ArrowLeft"
              ? index - 1
              : e.key === "ArrowRight"
                ? index + 1
                : e.key === "Home"
                  ? 0
                  : e.key === "End"
                    ? data.days - 1
                    : null;
          if (next !== null) {
            e.preventDefault();
            setHovered(Math.max(0, Math.min(data.days - 1, next)));
          }
        }}
        onPointerMove={(e) => {
          const rect = e.currentTarget.getBoundingClientRect();
          const position = ((e.clientX - rect.left) / rect.width) * w;
          setHovered(
            Math.max(
              0,
              Math.min(data.days - 1, Math.round(((position - left) / width) * (data.days - 1))),
            ),
          );
        }}
        onPointerDown={(e) => {
          const rect = e.currentTarget.getBoundingClientRect();
          const position = ((e.clientX - rect.left) / rect.width) * w;
          setHovered(
            Math.max(
              0,
              Math.min(data.days - 1, Math.round(((position - left) / width) * (data.days - 1))),
            ),
          );
        }}
      >
        <svg viewBox={`0 0 ${w} ${h}`} className="w-full" aria-hidden="true">
          {[0, 0.25, 0.5, 0.75, 1].map((f) => (
            <g key={f}>
              <line
                x1={left}
                x2={w - right}
                y1={y(max * f)}
                y2={y(max * f)}
                className="stroke-border"
                strokeDasharray={f === 0 ? undefined : "2 4"}
              />
              <text
                x={left - 10}
                y={y(max * f) + 4}
                textAnchor="end"
                className="fill-muted-foreground font-mono text-[11px]"
              >
                {new Intl.NumberFormat("en", {
                  notation: "compact",
                  maximumFractionDigits: 1,
                }).format((max * f) / (money ? 100 : 1))}
                {metric === "roas" ? "×" : ""}
              </text>
            </g>
          ))}
          <text
            x={left - 10}
            y={h - 4}
            textAnchor="end"
            className="fill-muted-foreground text-[10px]"
          >
            {money ? "AED" : metric === "roas" ? "ROAS" : metric === "deals" ? "Deals" : "Leads"}
          </text>
          {compare && (
            <path
              d={path(previous)}
              fill="none"
              className="stroke-muted-foreground/65"
              strokeWidth="1.6"
              strokeDasharray="5 5"
            />
          )}
          <path
            d={path(current)}
            fill="none"
            className="stroke-foreground"
            strokeWidth="2.3"
            strokeLinejoin="round"
          />
          <line
            x1={x(index)}
            x2={x(index)}
            y1={top}
            y2={h - bottom}
            className="stroke-foreground/20"
          />
          {current[index] !== null && current[index] !== undefined && (
            <circle
              cx={x(index)}
              cy={y(current[index] ?? 0)}
              r="4"
              className="fill-foreground stroke-card"
              strokeWidth="2"
            />
          )}
          {compare && previous[index] !== null && previous[index] !== undefined && (
            <circle
              cx={x(index)}
              cy={y(previous[index] ?? 0)}
              r="4"
              className="fill-muted-foreground stroke-card"
              strokeWidth="2"
            />
          )}
          <g transform={`translate(${calloutX}, ${calloutY})`}>
            <rect width={calloutWidth} height={calloutHeight} className="fill-card stroke-border" />
            <text x="12" y="19" className="fill-muted-foreground text-[11px]">
              {active && shortDate(active.date)}
            </text>
            <text x="12" y="43" className="fill-foreground font-mono text-[18px] font-semibold">
              {metricValue(metric, current[index] ?? null)}
              {direction && (
                <tspan dx="8" className={arrowColour}>
                  {direction === "up" ? "↑" : "↓"}
                  <tspan dx="4" className="text-[12px] font-normal">
                    {previousValue === 0
                      ? "From zero"
                      : formatPercent(
                          Math.abs(
                            ((currentValue ?? 0) - (previousValue ?? 0)) / (previousValue ?? 1),
                          ),
                          1,
                        )}
                  </tspan>
                </tspan>
              )}
            </text>
            {compare && (
              <>
                <text x="12" y="66" className="fill-muted-foreground text-[11px]">
                  {comparisonDay}
                </text>
                <text x="12" y="88" className="fill-muted-foreground font-mono text-[16px]">
                  {metricValue(metric, previous[index] ?? null)}
                </text>
              </>
            )}
          </g>
          {[
            ...new Set([
              0,
              Math.floor((data.days - 1) / 3),
              Math.floor(((data.days - 1) * 2) / 3),
              data.days - 1,
            ]),
          ].map((i) => (
            <text
              key={i}
              x={x(i)}
              y={h - 8}
              textAnchor={i === 0 ? "start" : i === data.days - 1 ? "end" : "middle"}
              className="fill-muted-foreground font-mono text-[11px]"
            >
              {shortDate(data.daily[i]?.date ?? data.previousTo)}
            </text>
          ))}
        </svg>
      </button>
    </Panel>
  );
}

function Breakdown({
  title,
  rows,
  onSelect,
  showSales = false,
  compare = false,
}: {
  title: string;
  rows: AnalyticsRow[];
  showSales?: boolean;
  compare?: boolean;
  onSelect: (row: AnalyticsRow) => void;
}) {
  const [sort, setSort] = useState<"spendMinor" | "qualified" | "cpq">("spendMinor");
  const [ascending, setAscending] = useState(false);
  const sorted = [...rows].sort((a, b) => {
    const av = a.current[sort],
      bv = b.current[sort];
    if (av === null) return bv === null ? 0 : 1;
    if (bv === null) return -1;
    return ascending ? av - bv : bv - av;
  });
  function heading(key: typeof sort, text: string) {
    return (
      <TH numeric aria-sort={sort === key ? (ascending ? "ascending" : "descending") : "none"}>
        <button
          type="button"
          className="inline-flex items-center gap-1"
          onClick={() => {
            setSort(key);
            setAscending(sort === key ? !ascending : key === "cpq");
          }}
        >
          {text}
          {sort === key &&
            (ascending ? <ArrowUp className="size-3" /> : <ArrowDown className="size-3" />)}
        </button>
      </TH>
    );
  }
  return (
    <Panel title={title} description="Select a row to filter this view" bodyClassName="p-0">
      <Table>
        <THead>
          <TR>
            <TH>{title === "By project" ? "Project" : "Source"}</TH>
            {heading("spendMinor", "Spend")}
            <TH numeric>Leads</TH>
            {heading("qualified", "Qualified")}
            {showSales && (
              <>
                <TH numeric>Done-Deals</TH>
                <TH numeric>Revenue</TH>
                <TH numeric>ROAS</TH>
              </>
            )}
            {heading("cpq", "Per qualified")}
          </TR>
        </THead>
        <TBody>
          {sorted.map((row) => (
            <TR key={row.id} className="hover:bg-muted/35">
              <TD>
                <button
                  type="button"
                  onClick={() => onSelect(row)}
                  className="flex items-center gap-2 whitespace-nowrap font-medium hover:underline"
                >
                  {row.platform && <PlatformMark platform={row.platform} />}
                  {row.name}
                  <ChevronRight className="size-3 text-muted-foreground" />
                </button>
              </TD>
              <TD numeric>{formatMoneyShort(row.current.spendMinor)}</TD>
              <TD numeric>{formatNumber(row.current.leads)}</TD>
              <TD numeric>{formatNumber(row.current.qualified)}</TD>
              {showSales && (
                <>
                  <TD numeric>{formatNumber(row.current.deals)}</TD>
                  <TD numeric>{formatMoneyShort(row.current.revenueMinor)}</TD>
                  <TD numeric className="font-semibold">
                    {metricValue("roas", row.current.roas)}
                  </TD>
                </>
              )}
              <TD numeric>
                <div>
                  {formatMoneyShort(row.current.cpq === null ? null : Math.round(row.current.cpq))}
                </div>
                {compare && (
                  <div className="py-1 text-[10px]">
                    <Delta current={row.current.cpq} previous={row.previous.cpq} cost />
                  </div>
                )}
              </TD>
            </TR>
          ))}
        </TBody>
      </Table>
    </Panel>
  );
}

function LeadQuality({ totals }: { totals: AnalyticsTotals }) {
  const rate =
    totals.leads !== null && totals.leads > 0 && totals.qualified !== null
      ? totals.qualified / totals.leads
      : null;
  const other =
    totals.leads !== null && totals.qualified !== null ? totals.leads - totals.qualified : null;
  return (
    <Panel
      title="Lead quality"
      description={`${formatNumber(totals.leads)} leads in this period`}
      className="flex flex-col"
      bodyClassName="flex flex-1 flex-col items-center justify-center gap-5 py-6"
    >
      <div className="relative size-52 shrink-0">
        <svg
          viewBox="0 0 200 200"
          className="size-full -rotate-90"
          role="img"
          aria-label={`${formatNumber(totals.qualified)} qualified leads and ${formatNumber(other)} other leads. Qualified share: ${formatPercent(rate, 1)}.`}
        >
          <circle cx="100" cy="100" r="82" fill="none" strokeWidth="20" className="stroke-border" />
          {rate !== null && (
            <circle
              cx="100"
              cy="100"
              r="82"
              fill="none"
              strokeWidth="20"
              pathLength="100"
              strokeDasharray={`${Math.max(0, Math.min(100, rate * 100))} 100`}
              className="stroke-foreground"
            />
          )}
        </svg>
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center gap-1">
          <strong
            className={cn(
              "font-mono font-semibold tracking-tight tabular-nums",
              rate === null ? "text-sm" : "text-4xl",
            )}
          >
            {formatPercent(rate, 1)}
          </strong>
          <span className="text-xs text-muted-foreground">qualified</span>
        </div>
      </div>
      <dl className="w-full max-w-64 space-y-3 text-sm">
        <div className="flex items-center justify-between gap-4">
          <dt className="flex items-center gap-2">
            <span className="size-2.5 bg-foreground" />
            Qualified
          </dt>
          <dd className="font-mono font-semibold tabular-nums">{formatNumber(totals.qualified)}</dd>
        </div>
        <div className="flex items-center justify-between gap-4">
          <dt className="flex items-center gap-2">
            <span className="size-2.5 bg-border" />
            Other leads
          </dt>
          <dd className="font-mono font-semibold tabular-nums">{formatNumber(other)}</dd>
        </div>
      </dl>
      <p className="text-xs text-muted-foreground">Other leads include missing statuses.</p>
    </Panel>
  );
}

function Attention() {
  const alerts = useAlerts();
  const proposals = useProposals();
  const session = useSession();
  const pending = proposals.data?.filter((p) => p.status === "pending").length;
  const items = alerts.data?.filter((a) => a.severity !== "low").slice(0, 3);
  return (
    <Panel
      title="Needs attention"
      description="Current items across all accounts, independent of the reporting dates"
      action={
        <Link to="/inbox" className="text-xs text-muted-foreground hover:text-foreground">
          Inbox <ArrowUpRight className="inline size-3" />
        </Link>
      }
      bodyClassName="p-0"
    >
      {alerts.error && (
        <div className="p-4">
          <ErrorNote error={alerts.error} />
        </div>
      )}
      {alerts.isPending && (
        <div className="p-4">
          <LoadingRows rows={2} />
        </div>
      )}
      {items?.map((a) => (
        <Link
          to="/inbox"
          key={a.id}
          className="flex items-center justify-between gap-4 border-b px-5 py-3 text-sm hover:bg-muted/30"
        >
          <span className="flex items-center gap-3">
            <StatusMark status={a.severity === "high" ? "error" : "caution"} />
            {a.title}
          </span>
          <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
        </Link>
      ))}
      {alerts.isSuccess && items?.length === 0 && (
        <p className="px-5 py-3 text-sm text-muted-foreground">
          No high or medium priority alerts.
        </p>
      )}
      {released(3) && (
        <div className="flex flex-wrap items-center justify-between gap-3 px-5 py-3 text-sm">
          <span className="text-muted-foreground">
            {proposals.error
              ? "Approval queue unavailable"
              : pending === undefined
                ? "Checking approval queue…"
                : `${pending} ${pending === 1 ? "change" : "changes"} waiting for approval`}
          </span>
          <Link
            to="/proposals"
            className="inline-flex items-center gap-1 font-medium hover:underline"
          >
            {session.role === "admin" ? "Review changes" : "View proposals"}
            <ArrowUpRight className="size-3.5" />
          </Link>
        </div>
      )}
    </Panel>
  );
}

function ProjectReturns({
  rows,
  onSelect,
}: {
  rows: AnalyticsRow[];
  onSelect: (row: AnalyticsRow) => void;
}) {
  const sorted = [...rows].sort((a, b) => (b.current.roas ?? -1) - (a.current.roas ?? -1));
  const max = Math.max(1, ...rows.map((r) => r.current.roas ?? 0));
  return (
    <Panel
      title="ROAS by project"
      description="Revenue for every AED 1 spent on ads"
      bodyClassName="space-y-4"
    >
      {sorted.map((row) => (
        <button
          type="button"
          key={row.id}
          onClick={() => onSelect(row)}
          className="group block w-full text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="mb-1.5 flex items-baseline justify-between gap-3 text-sm">
            <span className="group-hover:underline">{row.name}</span>
            <strong className="font-mono tabular-nums">
              {metricValue("roas", row.current.roas)}
            </strong>
          </span>
          <span className="block h-2 bg-muted">
            <span
              className="block h-full bg-foreground"
              style={{ width: `${((row.current.roas ?? 0) / max) * 100}%` }}
            />
          </span>
        </button>
      ))}
      <p className="border-t pt-3 text-xs leading-relaxed text-muted-foreground">
        Done-Deal property value ÷ ad spend for this period. Revenue return, not profit.
      </p>
    </Panel>
  );
}
