import { RefreshCw } from "lucide-react";
import { useState } from "react";
import { useRefreshResearch, useResearch, useToggleKeyword } from "@/api/create";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { StatusChip } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatMoneyShort, formatNumber, formatRelative } from "@/lib/format";
import { BrandPicker, SampleChip } from "./shared";

const BRANDS_WITH_RESEARCH = [1, 4];
const INTENT = { buy: "Buying", rent: "Renting", explore: "Exploring" } as const;

// Research is what Adx found about the market around a brand: competitors
// and the ads they run, the keywords worth buying, and the audiences. It is
// refreshed monthly, and the media plan is built from it.
export function ResearchPage() {
  const [brandId, setBrandId] = useState<number>(1);
  const research = useResearch(brandId);
  const toggle = useToggleKeyword();
  const refresh = useRefreshResearch();
  const r = research.data;
  const due = r ? Date.parse(r.nextRefreshAt) < Date.now() : false;

  return (
    <>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            Research <SampleChip phase={2} />
          </span>
        }
        subtitle="Competitors, keywords and audiences for a brand. Refreshed every month; the media plan is built from it."
        actions={
          <>
            <BrandPicker value={brandId} onChange={setBrandId} only={BRANDS_WITH_RESEARCH} />
            {r && (
              <Button
                variant="outline"
                disabled={refresh.isPending}
                onClick={() => refresh.mutate(brandId)}
              >
                <RefreshCw className={refresh.isPending ? "animate-spin" : undefined} />
                {refresh.isPending ? "Refreshing" : "Refresh now"}
              </Button>
            )}
          </>
        }
      />
      {research.isPending && <LoadingRows rows={8} />}
      {research.error && <ErrorNote error={research.error} />}
      {research.data === null && (
        <Panel>
          <EmptyState title="No research for this brand yet">
            Research starts once the brand's facts are approved. That arrives with phase 2.
          </EmptyState>
        </Panel>
      )}
      {r && (
        <div className="space-y-5">
          <p className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
            <StatusChip status={due ? "caution" : "ready"}>
              {due ? "Refresh due" : "Current"}
            </StatusChip>
            Refreshed {formatRelative(r.refreshedAt)} · next {formatRelative(r.nextRefreshAt)}
          </p>

          <Panel
            title="Competitors"
            description="From ad libraries, their sites and their social handles. Spend is estimated, not reported."
            bodyClassName="p-0"
          >
            <Table>
              <THead>
                <TR>
                  <TH>Competitor</TH>
                  <TH>Positioning</TH>
                  <TH numeric>From</TH>
                  <TH numeric>Ads seen</TH>
                  <TH>Channels</TH>
                  <TH>What Adx noticed</TH>
                </TR>
              </THead>
              <TBody>
                {r.competitors.map((c) => (
                  <TR key={c.name}>
                    <TD className="font-medium">{c.name}</TD>
                    <TD className="text-muted-foreground">{c.positioning}</TD>
                    <TD numeric>{c.priceFrom}</TD>
                    <TD numeric>{c.adsSeen}</TD>
                    <TD className="text-muted-foreground">{c.channels.join(", ")}</TD>
                    <TD className="max-w-md text-muted-foreground">{c.note}</TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </Panel>

          <div className="grid gap-5 lg:grid-cols-[3fr_2fr]">
            <Panel
              title="Keywords"
              description="Tick what the plan may buy. Renting and exploring intent is left out by default."
              bodyClassName="p-0"
            >
              <Table>
                <THead>
                  <TR>
                    <TH>In plan</TH>
                    <TH>Term</TH>
                    <TH numeric>Searches</TH>
                    <TH numeric>CPC</TH>
                    <TH>Intent</TH>
                  </TR>
                </THead>
                <TBody>
                  {r.keywords.map((k) => (
                    <TR key={k.term} className={k.included ? undefined : "text-muted-foreground"}>
                      <TD>
                        <input
                          type="checkbox"
                          aria-label={`Include ${k.term}`}
                          checked={k.included}
                          onChange={() => toggle.mutate({ projectId: brandId, term: k.term })}
                        />
                      </TD>
                      <TD className="font-medium">{k.term}</TD>
                      <TD numeric>{formatNumber(k.volume)}</TD>
                      <TD numeric>{formatMoneyShort(k.cpcMinor)}</TD>
                      <TD>{INTENT[k.intent]}</TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
            </Panel>
            <div className="space-y-5">
              <Panel title="Audiences" bodyClassName="p-0">
                <ul className="divide-y">
                  {r.audiences.map((a) => (
                    <li key={a.name} className="px-5 py-3">
                      <p className="flex items-baseline justify-between gap-3 font-medium">
                        {a.name}
                        <span className="font-mono text-xs text-muted-foreground">{a.size}</span>
                      </p>
                      <p className="mt-0.5 text-sm text-muted-foreground">{a.why}</p>
                      <p className="mt-1 font-mono text-[0.65rem] tracking-wider text-muted-foreground uppercase">
                        {a.platforms.join(" · ")}
                      </p>
                    </li>
                  ))}
                </ul>
              </Panel>
              <Panel title="What this research can't tell you">
                <ul className="list-disc space-y-1 pl-4 text-sm text-muted-foreground">
                  {r.gaps.map((g) => (
                    <li key={g}>{g}</li>
                  ))}
                </ul>
              </Panel>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
