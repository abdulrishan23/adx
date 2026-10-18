import { useMarketers } from "@/api/adx";
import { PERIOD_LABEL, ScopeBar, useScope } from "@/app/scope";
import { Sparkline } from "@/components/charts";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatMoneyShort, formatNumber, formatPercent } from "@/lib/format";
import { cn } from "@/lib/utils";

const per = (minor: number | null, n: number) =>
  minor !== null && n > 0 ? Math.round(minor / n) : null;

export function MarketersPage() {
  const marketers = useMarketers();
  const scope = useScope();
  return (
    <>
      <PageHeader
        title="Marketers"
        subtitle={PERIOD_LABEL[scope.days]}
        actions={<ScopeBar showPlatform={false} />}
      />
      {marketers.isPending && <LoadingRows rows={5} />}
      {marketers.error && <ErrorNote error={marketers.error} />}
      {marketers.data && (
        <Panel bodyClassName="p-0">
          <Table>
            <THead>
              <TR>
                <TH>Owner</TH>
                <TH>Sources</TH>
                <TH numeric>Spend</TH>
                <TH numeric>Leads</TH>
                <TH numeric>Qualified</TH>
                <TH numeric>Per qualified</TH>
                <TH numeric>Deals</TH>
                <TH numeric>Statuses set</TH>
                <TH className="w-32">Leads, 8 weeks</TH>
              </TR>
            </THead>
            <TBody>
              {marketers.data.map((m) => (
                <TR key={m.name}>
                  <TD className="font-medium">{m.name}</TD>
                  <TD className="text-muted-foreground">{m.sources.join(", ")}</TD>
                  <TD numeric>{formatMoneyShort(m.spendMinor)}</TD>
                  <TD numeric>{formatNumber(m.leads)}</TD>
                  <TD numeric>{formatNumber(m.qualified)}</TD>
                  <TD numeric className="font-semibold">
                    {formatMoneyShort(per(m.spendMinor, m.qualified))}
                  </TD>
                  <TD numeric>{m.deals}</TD>
                  <TD numeric className={cn(m.statusCoverage < 0.5 && "text-destructive")}>
                    {formatPercent(m.statusCoverage)}
                  </TD>
                  <TD>
                    <Sparkline values={m.weekly} className="h-6" />
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </Panel>
      )}
    </>
  );
}
