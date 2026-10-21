import { lazy, Suspense, useState } from "react";
import { useFunnel, useMarkets, useRevenue } from "@/api/adx";
import type { Funnel, FunnelStage, MarketRow, Revenue } from "@/api/models";
import { PERIOD_LABEL, ScopeBar, useScope } from "@/app/scope";
import { Meter } from "@/components/charts";
import { ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import {
  formatDate,
  formatMoneyShort,
  formatNumber,
  formatPercent,
  NOT_AVAILABLE,
} from "@/lib/format";
import { cn } from "@/lib/utils";

const per = (minor: number, n: number) => (n > 0 ? Math.round(minor / n) : null);

// Money in against money out, and who it comes from: the monthly questions the
// CMO asks. The daily funnel is on the Brief, and each project's on Projects.
export function RevenuePage() {
  const funnel = useFunnel();
  const scope = useScope();
  return (
    <>
      <PageHeader title="Revenue" subtitle={PERIOD_LABEL[scope.days]} actions={<ScopeBar />} />
      {funnel.isPending && <LoadingRows rows={8} />}
      {funnel.error && <ErrorNote error={funnel.error} />}
      {funnel.data && <FunnelBody funnel={funnel.data} />}
    </>
  );
}

function FunnelBody({ funnel }: { funnel: Funnel }) {
  const roas =
    funnel.revenueMinor && funnel.spendMinor ? funnel.revenueMinor / funnel.spendMinor : null;
  return (
    <div className="space-y-6">
      <Panel
        title="Spend to deal"
        description={`${formatMoneyShort(funnel.spendMinor)} spent · ${formatMoneyShort(funnel.revenueMinor)} revenue · ${roas ? `${roas.toFixed(1)}× return` : "return not available"}`}
      >
        <FunnelBars stages={funnel.stages} spendMinor={funnel.spendMinor} />
      </Panel>
      <ChannelsPanel />
      <MonthsPanel months={funnel.months} />
      <MarketsSection />
    </div>
  );
}

function FunnelBars({ stages, spendMinor }: { stages: FunnelStage[]; spendMinor: number }) {
  const top = stages[0]?.count || 1;
  return (
    <ol className="space-y-4">
      {stages.map((s, i) => {
        const prev = i > 0 ? stages[i - 1] : undefined;
        const step = prev && prev.count > 0 ? s.count / prev.count : null;
        return (
          <li
            key={s.key}
            className="grid grid-cols-[7rem_1fr] items-center gap-4 sm:grid-cols-[8rem_1fr_9rem]"
          >
            <div>
              <p className="text-sm font-medium">{s.label}</p>
              <p className="font-mono text-xs text-muted-foreground tabular-nums">
                {i === 0
                  ? "all leads"
                  : `${formatPercent(step, 1)} of ${prev?.label.toLowerCase()}`}
              </p>
            </div>
            <div className="h-9 rounded-md bg-muted">
              <div
                className="flex h-full min-w-10 items-center rounded-md bg-foreground px-2.5 font-mono text-sm font-semibold text-background tabular-nums"
                style={{ width: `${Math.max(3, (s.count / top) * 100)}%` }}
              >
                {formatNumber(s.count)}
              </div>
            </div>
            <p className="hidden text-right font-mono text-sm tabular-nums sm:block">
              {formatMoneyShort(per(spendMinor, s.count))}
              <span className="ml-1 text-xs text-muted-foreground">each</span>
            </p>
          </li>
        );
      })}
    </ol>
  );
}

function MonthsPanel({ months }: { months: Funnel["months"] }) {
  const max = Math.max(...months.flatMap((m) => [m.spendMinor, m.revenueMinor ?? 0])) || 1;
  return (
    <Panel title="Revenue against spend, by month">
      <div className="flex h-56 items-end gap-3 sm:gap-6">
        {months.map((m) => (
          <div key={m.month} className="flex flex-1 flex-col items-center gap-2">
            <div className="flex h-44 w-full items-end justify-center gap-1">
              <div
                className="w-1/3 max-w-8 rounded-t-sm bg-chart-4"
                style={{ height: `${(m.spendMinor / max) * 100}%` }}
                title={`Spend ${formatMoneyShort(m.spendMinor)}`}
              />
              {m.revenueMinor === null ? (
                <div
                  className="w-1/3 max-w-8 rounded-t-sm border border-dashed border-foreground/40"
                  style={{ height: "20%" }}
                  title={`Revenue ${NOT_AVAILABLE}: deals still closing`}
                />
              ) : (
                <div
                  className="w-1/3 max-w-8 rounded-t-sm bg-foreground"
                  style={{ height: `${(m.revenueMinor / max) * 100}%` }}
                  title={`Revenue ${formatMoneyShort(m.revenueMinor)}`}
                />
              )}
            </div>
            <Eyebrow>{formatDate(`${m.month}-01T00:00:00Z`, { month: "short" })}</Eyebrow>
          </div>
        ))}
      </div>
      <div className="mt-3 flex gap-4 text-xs text-muted-foreground">
        <span className="flex items-center gap-1.5">
          <span className="size-2.5 rounded-none bg-chart-4" /> Spend
        </span>
        <span className="flex items-center gap-1.5">
          <span className="size-2.5 rounded-none bg-foreground" /> Revenue
        </span>
        <span className="flex items-center gap-1.5">
          <span className="size-2.5 rounded-none border border-dashed border-foreground/40" /> Still
          closing
        </span>
      </div>
    </Panel>
  );
}

// Revenue by channel, including the ones that cost nothing to run. Telesales and
// walk-ins close deals too, and leaving them out would flatter the ad channels.
function ChannelsPanel() {
  const revenue = useRevenue();
  if (revenue.isPending) return <LoadingRows rows={4} />;
  if (revenue.error) return <ErrorNote error={revenue.error} />;
  return <ChannelsTable data={revenue.data} />;
}

function ChannelsTable({ data }: { data: Revenue }) {
  const totalDeals = data.byChannel.reduce((a, r) => a + r.deals, 0);
  return (
    <Panel
      title="By channel"
      description={`Last 30 days · ${data.dealsMissingValue} deals without a value yet`}
      bodyClassName="p-0"
    >
      <Table>
        <THead>
          <TR>
            <TH>Channel</TH>
            <TH numeric>Spend</TH>
            <TH numeric>Leads</TH>
            <TH numeric>Qualified</TH>
            <TH numeric>Deals</TH>
            <TH numeric>Share of deals</TH>
            <TH numeric>Revenue</TH>
            <TH numeric>Return</TH>
          </TR>
        </THead>
        <TBody>
          {data.byChannel.map((r) => (
            <TR key={r.key}>
              <TD className="font-medium">{r.label}</TD>
              <TD numeric>{r.spendMinor > 0 ? formatMoneyShort(r.spendMinor) : "No ad spend"}</TD>
              <TD numeric>{formatNumber(r.leads)}</TD>
              <TD numeric>{formatNumber(r.qualified)}</TD>
              <TD numeric>{formatNumber(r.deals)}</TD>
              <TD numeric>{formatPercent(totalDeals > 0 ? r.deals / totalDeals : null)}</TD>
              <TD numeric>{formatMoneyShort(r.revenueMinor)}</TD>
              <TD numeric className="font-semibold">
                {r.revenueMinor !== null && r.spendMinor > 0
                  ? `${(r.revenueMinor / r.spendMinor).toFixed(1)}×`
                  : NOT_AVAILABLE}
              </TD>
            </TR>
          ))}
        </TBody>
      </Table>
    </Panel>
  );
}

// The map's country shapes are a few hundred kilobytes, so they load only here.
const LeadsMap = lazy(() =>
  import("@/components/leads-map").then((m) => ({ default: m.LeadsMap })),
);

function MarketsSection() {
  const markets = useMarkets();
  if (!markets.data) return null;
  return <MarketsPanel rows={markets.data} />;
}

function MarketsPanel({ rows }: { rows: MarketRow[] }) {
  // The market under the pointer, on the map or in the table, so each lights
  // up the other.
  const [active, setActive] = useState<string | null>(null);
  const best = Math.max(...rows.map((r) => (r.leads ? r.qualified / r.leads : 0)), 0.01);
  return (
    <Panel
      title="Where leads live"
      description="From the phone country code, not nationality"
      bodyClassName="grid p-0 lg:grid-cols-[3fr_2fr]"
    >
      <div className="border-b p-4 lg:border-r lg:border-b-0">
        <Suspense fallback={<div className="aspect-[640/380] w-full bg-muted/40" />}>
          <LeadsMap rows={rows} active={active} onActive={setActive} />
        </Suspense>
      </div>
      <Table>
        <THead>
          <TR>
            <TH>Market</TH>
            <TH numeric>Leads</TH>
            <TH>Qualified share</TH>
          </TR>
        </THead>
        <TBody>
          {rows.map((r) => {
            const rate = r.leads ? r.qualified / r.leads : null;
            return (
              <TR
                key={r.market}
                onMouseEnter={() => setActive(r.code || null)}
                onMouseLeave={() => setActive(null)}
                className={cn(r.code && active === r.code && "bg-muted")}
              >
                <TD className="whitespace-nowrap">
                  {r.market}
                  {r.code && (
                    <span className="ml-1.5 font-mono text-xs text-muted-foreground">{r.code}</span>
                  )}
                </TD>
                <TD numeric>{formatNumber(r.leads)}</TD>
                <TD className="min-w-36">
                  <div className="grid grid-cols-[1fr_2.5rem] items-center gap-2">
                    <Meter value={rate ?? 0} max={best} className="h-1.5" />
                    <span className="text-right font-mono text-xs tabular-nums">
                      {formatPercent(rate)}
                    </span>
                  </div>
                </TD>
              </TR>
            );
          })}
        </TBody>
      </Table>
    </Panel>
  );
}
