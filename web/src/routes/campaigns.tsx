import { getRouteApi, Link } from "@tanstack/react-router";
import { ArrowDown, ArrowUp, Rocket } from "lucide-react";
import { useId, useMemo, useState } from "react";
import { useAds, useCampaigns, useDailyBudget, useProposeChange } from "@/api/adx";
import { projectOf } from "@/api/mock/data";
import type { AdRow, Campaign, Proposal } from "@/api/models";
import { PERIOD_LABEL, ScopeBar, useScope } from "@/app/scope";
import { ChartLegend, DailyChart, ProportionBar, Sparkline } from "@/components/charts";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { PlatformMark, platformLabel } from "@/components/platform";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Sheet } from "@/components/ui/sheet";
import { StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatMoneyShort, formatNumber, formatPercent, NOT_AVAILABLE } from "@/lib/format";
import { released } from "@/lib/release";
import { cn } from "@/lib/utils";

type SortKey = "spend" | "leads" | "cpl" | "qualified" | "cpq" | "deals";

const per = (minor: number, n: number) => (n > 0 ? Math.round(minor / n) : null);
const roas = (c: Campaign) =>
  c.revenueMinor !== null && c.spendMinor > 0 ? c.revenueMinor / c.spendMinor : null;

const SORTERS: Record<SortKey, (c: Campaign) => number | null> = {
  spend: (c) => c.spendMinor,
  leads: (c) => c.leads,
  cpl: (c) => per(c.spendMinor, c.leads),
  qualified: (c) => c.qualified,
  cpq: (c) => per(c.spendMinor, c.qualified),
  deals: (c) => c.deals,
};

const STATUS_MARK = { paused: "pending", limited: "caution" } as const;
const STATUS_LABEL = { paused: "Paused", limited: "Limited by budget" } as const;

function formatRoas(c: Campaign) {
  const r = roas(c);
  if (r === null) return NOT_AVAILABLE;
  return `${r.toFixed(1)}×`;
}

const route = getRouteApi("/app/campaigns");

export function CampaignsPage() {
  const campaigns = useCampaigns();
  const scope = useScope();
  const [sort, setSort] = useState<{ key: SortKey; dir: "asc" | "desc" }>({
    key: "spend",
    dir: "desc",
  });
  // ?open=<id> opens a campaign's sheet, so Ask Adx and search can link to one.
  const openId = route.useSearch().open ?? null;
  const navigate = route.useNavigate();
  const setOpenId = (id: string | null) =>
    navigate({ search: id ? { open: id } : {}, replace: id === null });

  // Campaigns sit under their project, the way marketers plan budgets. The
  // sort orders projects by their own totals, and campaigns within each.
  const groups = useMemo(() => {
    const get = SORTERS[sort.key];
    const order = (av: number | null, bv: number | null) => {
      // Rows with no figure always sink, whichever way the column is sorted.
      if (av === null) return 1;
      if (bv === null) return -1;
      return sort.dir === "asc" ? av - bv : bv - av;
    };
    const byProject = new Map<number, Campaign[]>();
    for (const c of campaigns.data ?? []) {
      byProject.set(c.projectId, [...(byProject.get(c.projectId) ?? []), c]);
    }
    return [...byProject.entries()]
      .map(([projectId, list]) => ({
        projectId,
        total: sumOf(list),
        campaigns: [...list].sort((a, b) => order(get(a), get(b))),
      }))
      .sort((a, b) => order(get(a.total), get(b.total)));
  }, [campaigns.data, sort]);

  const rows = groups.flatMap((g) => g.campaigns);
  const open = rows.find((c) => c.id === openId) ?? null;
  const totals = sumOf(rows);

  function SortHead({ k, children }: { k: SortKey; children: string }) {
    const active = sort.key === k;
    return (
      <TH numeric aria-sort={active ? (sort.dir === "asc" ? "ascending" : "descending") : "none"}>
        <button
          type="button"
          className={cn("inline-flex items-center gap-1 uppercase", active && "text-foreground")}
          onClick={() => setSort({ key: k, dir: active && sort.dir === "desc" ? "asc" : "desc" })}
        >
          {children}
          {active &&
            (sort.dir === "desc" ? (
              <ArrowDown className="size-3" />
            ) : (
              <ArrowUp className="size-3" />
            ))}
        </button>
      </TH>
    );
  }

  return (
    <>
      <PageHeader
        title="Campaigns"
        subtitle={`${rows.length} campaigns · ${PERIOD_LABEL[scope.days]}`}
        actions={
          <span className="flex flex-wrap items-center gap-2">
            <ScopeBar />
            {/* A new campaign starts from a plan, so this is a door into Create's
                Launch (BD-58), not a second launch flow. Phase 3 with Launch. */}
            {released(3) && (
              <Link
                to="/create/launch"
                search={scope.projectId === "all" ? {} : { project: scope.projectId }}
                className={buttonVariants({ variant: "outline", size: "sm" })}
              >
                <Rocket /> Launch a campaign
              </Link>
            )}
          </span>
        }
      />
      {campaigns.isPending && <LoadingRows rows={10} />}
      {campaigns.error && <ErrorNote error={campaigns.error} />}
      {campaigns.data && rows.length === 0 && (
        <Panel>
          <EmptyState title="No campaigns in this scope">
            Try all platforms or all projects.
          </EmptyState>
        </Panel>
      )}
      {rows.length > 0 && (
        <section className="border bg-card">
          <Table>
            <THead>
              <TR>
                <TH>Campaign</TH>
                <SortHead k="spend">Spend</SortHead>
                <SortHead k="leads">Leads</SortHead>
                <SortHead k="qualified">Qualified</SortHead>
                <SortHead k="cpl">Per lead</SortHead>
                <SortHead k="cpq">Per qualified</SortHead>
                <SortHead k="deals">Deals</SortHead>
              </TR>
            </THead>
            {groups.map((g) => (
              <TBody key={g.projectId}>
                <TR className="bg-muted/60">
                  <TD
                    className="font-heading font-semibold"
                    colSpan={g.campaigns.length > 1 ? 1 : 7}
                  >
                    {projectOf(g.projectId)}
                  </TD>
                  {/* A lone campaign's figures are the project's, so show them once. */}
                  {g.campaigns.length > 1 && <Figures c={g.total} />}
                </TR>
                {g.campaigns.map((c) => (
                  <TR
                    key={c.id}
                    className="cursor-pointer hover:bg-muted/40"
                    onClick={() => setOpenId(c.id)}
                    tabIndex={0}
                    onKeyDown={(e) => e.key === "Enter" && setOpenId(c.id)}
                  >
                    <TD className="pl-8">
                      <div className="flex items-center gap-2">
                        <PlatformMark platform={c.platform} />
                        <span className="truncate font-medium">{c.name}</span>
                      </div>
                      <p className="mt-0.5 flex items-center gap-1.5 pl-[1.125rem] text-xs text-muted-foreground">
                        {platformLabel(c.platform)} · {c.type}
                        {c.status !== "active" && (
                          <>
                            <span aria-hidden="true">·</span>
                            <StatusMark status={STATUS_MARK[c.status]} />
                            <span className="text-foreground">{STATUS_LABEL[c.status]}</span>
                          </>
                        )}
                      </p>
                    </TD>
                    <Figures c={c} muted />
                  </TR>
                ))}
              </TBody>
            ))}
            <TBody>
              <TR className="border-t-2 border-t-foreground/20 font-medium">
                <TD>All campaigns</TD>
                <Figures c={totals} />
              </TR>
            </TBody>
          </Table>
        </section>
      )}
      {open && <CampaignSheet campaign={open} onClose={() => setOpenId(null)} />}
    </>
  );
}

type Totals = Pick<Campaign, "spendMinor" | "leads" | "qualified" | "deals" | "revenueMinor">;

function sumOf(list: Totals[]): Campaign {
  const known = list.filter((c) => c.revenueMinor !== null);
  return {
    spendMinor: list.reduce((a, c) => a + c.spendMinor, 0),
    leads: list.reduce((a, c) => a + c.leads, 0),
    qualified: list.reduce((a, c) => a + c.qualified, 0),
    deals: list.reduce((a, c) => a + c.deals, 0),
    revenueMinor: known.length ? known.reduce((a, c) => a + (c.revenueMinor ?? 0), 0) : null,
  } as Campaign;
}

// The figure cells, the same for a campaign, a project and the total.
function Figures({ c, muted = false }: { c: Totals; muted?: boolean }) {
  return (
    <>
      <TD numeric>{formatMoneyShort(c.spendMinor)}</TD>
      <TD numeric>{formatNumber(c.leads)}</TD>
      <TD numeric>{formatNumber(c.qualified)}</TD>
      <TD numeric className="text-muted-foreground">
        {formatMoneyShort(per(c.spendMinor, c.leads))}
      </TD>
      <TD numeric className={cn("font-semibold", muted && "font-medium")}>
        {per(c.spendMinor, c.qualified) === null ? (
          <span className="font-normal text-muted-foreground">No qualified</span>
        ) : (
          formatMoneyShort(per(c.spendMinor, c.qualified))
        )}
      </TD>
      <TD numeric>{c.deals}</TD>
    </>
  );
}

function CampaignSheet({ campaign: c, onClose }: { campaign: Campaign; onClose: () => void }) {
  const figures = [
    { label: "Spend", value: formatMoneyShort(c.spendMinor) },
    { label: "Leads", value: formatNumber(c.leads) },
    { label: "Qualified", value: formatNumber(c.qualified) },
    { label: "Per qualified", value: formatMoneyShort(per(c.spendMinor, c.qualified)) },
    { label: "Deals", value: String(c.deals) },
    { label: "Revenue", value: formatMoneyShort(c.revenueMinor) },
    { label: "Per lead", value: formatMoneyShort(per(c.spendMinor, c.leads)) },
    { label: "Return on spend", value: formatRoas(c) },
    { label: "Status", value: c.status === "active" ? "Active" : STATUS_LABEL[c.status] },
  ];
  return (
    <Sheet
      open
      onOpenChange={(o) => !o && onClose()}
      title={c.name}
      description={`${platformLabel(c.platform)} · ${c.type} · ${projectOf(c.projectId)}`}
    >
      <div className="space-y-6">
        <dl className="grid grid-cols-3 gap-px overflow-hidden rounded-lg border bg-border">
          {figures.map((f) => (
            <div key={f.label} className="bg-card p-3">
              <dt>
                <Eyebrow>{f.label}</Eyebrow>
              </dt>
              <dd className="mt-1 font-mono text-lg font-semibold tabular-nums">{f.value}</dd>
            </div>
          ))}
        </dl>
        <div className="space-y-2">
          <Eyebrow>Lead temperature</Eyebrow>
          <ProportionBar
            parts={[
              { label: "Hot", value: c.temperatureMix.hot },
              { label: "Warm", value: c.temperatureMix.warm },
              { label: "Cold", value: c.temperatureMix.cold },
            ]}
          />
          <p className="font-mono text-xs text-muted-foreground tabular-nums">
            {formatNumber(c.temperatureMix.hot)} hot · {formatNumber(c.temperatureMix.warm)} warm ·{" "}
            {formatNumber(c.temperatureMix.cold)} cold
          </p>
        </div>
        {/* Budget changes are writes, which arrive with proposals in phase 3. */}
        {released(3) && <BudgetForm campaignId={c.id} />}
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <Eyebrow>Daily</Eyebrow>
            <ChartLegend
              items={[
                { label: "Spend", kind: "bar" },
                { label: "Qualified", kind: "line" },
              ]}
            />
          </div>
          <DailyChart
            points={c.daily.map((d) => ({ date: d.date, bar: d.spendMinor, line: d.qualified }))}
          />
        </div>
        {c.adSets.length > 0 && (
          <div className="space-y-2">
            <Eyebrow>Ad sets</Eyebrow>
            <div className="rounded-lg border">
              <Table>
                <THead>
                  <TR>
                    <TH>Ad set</TH>
                    <TH numeric>Spend</TH>
                    <TH numeric>Leads</TH>
                    <TH numeric>Per qualified</TH>
                  </TR>
                </THead>
                <TBody>
                  {c.adSets.map((a) => (
                    <TR key={a.name}>
                      <TD>{a.name}</TD>
                      <TD numeric>{formatMoneyShort(a.spendMinor)}</TD>
                      <TD numeric>{formatNumber(a.leads)}</TD>
                      <TD numeric>{formatMoneyShort(per(a.spendMinor, a.qualified))}</TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </div>
          </div>
        )}
        <AdsSection campaignId={c.id} platform={c.platform} />
      </div>
    </Sheet>
  );
}

const AD_FLAG: Record<
  NonNullable<AdRow["flag"]>,
  { chip: "error" | "caution"; label: string; why: string }
> = {
  disapproved: {
    chip: "error",
    label: "Disapproved",
    why: "Not delivering until the policy issue is fixed",
  },
  fatigued: {
    chip: "caution",
    label: "Fatigued",
    why: "Frequency over 6 and click rate down for 4 weeks",
  },
  sold_out: {
    chip: "caution",
    label: "Sold out",
    why: "Promotes a unit type Akarati shows as sold",
  },
};

function AdsSection({
  campaignId,
  platform,
}: {
  campaignId: string;
  platform: Campaign["platform"];
}) {
  const ads = useAds(campaignId);
  if (ads.isPending) return <LoadingRows rows={3} />;
  if (ads.error) return <ErrorNote error={ads.error} />;
  if (ads.data.length === 0) return null;
  return (
    <div className="space-y-2">
      <Eyebrow>{platform === "google" ? "Ads" : "Ads, worst first"}</Eyebrow>
      <ul className="divide-y rounded-lg border">
        {[...ads.data]
          .sort((a, b) => Number(b.flag !== null) - Number(a.flag !== null))
          .map((a) => (
            <li
              key={a.id}
              className="grid grid-cols-[1fr_auto] items-center gap-x-4 gap-y-1 px-4 py-3 text-sm"
            >
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="truncate font-medium">{a.name}</span>
                  {a.flag && (
                    <StatusChip status={AD_FLAG[a.flag].chip} className="h-5 px-1.5 text-[0.6rem]">
                      {AD_FLAG[a.flag].label}
                    </StatusChip>
                  )}
                </div>
                <p className="text-xs text-muted-foreground">
                  {a.flag ? AD_FLAG[a.flag].why : a.adSet}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <Sparkline values={a.ctrWeeks} className="h-6 w-20" />
                {a.status === "active" ? (
                  <PauseAd campaignId={campaignId} adId={a.id} />
                ) : (
                  <span className="w-14 text-right text-xs text-muted-foreground capitalize">
                    {a.status}
                  </span>
                )}
              </div>
              <p className="col-span-2 flex flex-wrap gap-x-4 font-mono text-xs text-muted-foreground tabular-nums">
                <span>{formatMoneyShort(a.spendMinor)}</span>
                <span>{formatNumber(a.leads)} leads</span>
                <span>CTR {formatPercent(a.ctr, 1)}</span>
                {a.frequency !== null && <span>Freq {a.frequency.toFixed(1)}</span>}
              </p>
            </li>
          ))}
      </ul>
    </div>
  );
}

function ProposalResult({ proposal }: { proposal: Proposal }) {
  return (
    <p className="text-sm" role="status">
      {proposal.status === "applied" ? "Applied and logged. " : "Sent for approval. "}
      <Link to="/proposals" search={{ open: proposal.id }} className="underline underline-offset-4">
        View
      </Link>
    </p>
  );
}

function BudgetForm({ campaignId }: { campaignId: string }) {
  const budget = useDailyBudget(campaignId);
  const propose = useProposeChange();
  const [value, setValue] = useState("");
  const id = useId();
  if (!budget.data) return null;
  const next = Math.round(Number(value) * 100);
  return (
    <form
      className="space-y-2 rounded-lg border p-4"
      onSubmit={(e) => {
        e.preventDefault();
        propose.mutate({ kind: "daily_budget", campaignId, dailyBudgetMinor: next });
      }}
    >
      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1">
          <label htmlFor={id} className="text-xs text-muted-foreground">
            Daily budget, now {formatMoneyShort(budget.data)}
          </label>
          <Input
            id={id}
            inputMode="decimal"
            placeholder={String(budget.data / 100)}
            value={value}
            onChange={(e) => {
              propose.reset();
              setValue(e.target.value.replace(/[^\d.]/g, ""));
            }}
            className="h-9 w-36 font-mono"
          />
        </div>
        <Button type="submit" size="sm" disabled={!value || propose.isPending}>
          {next > budget.data ? "Propose raise" : "Lower budget"}
        </Button>
      </div>
      {propose.error && <ErrorNote error={propose.error} />}
      {propose.data && <ProposalResult proposal={propose.data} />}
    </form>
  );
}

function PauseAd({ campaignId, adId }: { campaignId: string; adId: string }) {
  const propose = useProposeChange();
  return (
    <Button
      size="xs"
      variant="outline"
      className="w-14"
      disabled={propose.isPending}
      title={propose.error?.message}
      onClick={() => propose.mutate({ kind: "pause_ad", campaignId, adId })}
    >
      Pause
    </Button>
  );
}
