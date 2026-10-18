import { useOrganic } from "@/api/adx";
import type { Organic, OrganicAccount } from "@/api/models";
import { PERIOD_LABEL, ScopeBar, useScope } from "@/app/scope";
import { ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { StatusChip } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatDate, formatNumber, formatPercent } from "@/lib/format";
import { cn } from "@/lib/utils";

export function OrganicPage() {
  const organic = useOrganic();
  const scope = useScope();
  return (
    <>
      <PageHeader
        title="Organic"
        subtitle={PERIOD_LABEL[scope.days]}
        actions={<ScopeBar showPlatform={false} />}
      />
      {organic.isPending && <LoadingRows rows={8} />}
      {organic.error && <ErrorNote error={organic.error} />}
      {organic.data && <Body data={organic.data} />}
    </>
  );
}

function Body({ data }: { data: Organic }) {
  const questions = data.posts.reduce((a, p) => a + p.buyingQuestions, 0);
  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {data.accounts.map((a) => (
          <AccountTile key={a.key} account={a} />
        ))}
      </div>

      <Panel
        title="Top posts"
        description={`${formatNumber(questions)} buying questions in comments`}
        bodyClassName="p-0"
      >
        <Table>
          <THead>
            <TR>
              <TH>Post</TH>
              <TH numeric>Reach</TH>
              <TH numeric>Engagement</TH>
              <TH numeric>Comments</TH>
              <TH numeric>Buying questions</TH>
            </TR>
          </THead>
          <TBody>
            {data.posts.map((p) => (
              <TR key={p.id}>
                <TD>
                  <div className="font-medium">{p.caption}</div>
                  <div className="text-xs text-muted-foreground">
                    {p.account} · {formatDate(p.postedAt)}
                  </div>
                </TD>
                <TD numeric>{formatNumber(p.reach)}</TD>
                <TD numeric>{formatPercent(p.reach > 0 ? p.engagements / p.reach : null, 1)}</TD>
                <TD numeric>{formatNumber(p.comments)}</TD>
                <TD numeric className="font-semibold">
                  {formatNumber(p.buyingQuestions)}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </Panel>

      <Panel title="Search Console" bodyClassName="p-0">
        <Table>
          <THead>
            <TR>
              <TH>Query</TH>
              <TH numeric>Impressions</TH>
              <TH numeric>Clicks</TH>
              <TH numeric>Click rate</TH>
              <TH numeric>Position</TH>
            </TR>
          </THead>
          <TBody>
            {data.queries.map((q) => (
              <TR key={q.query}>
                <TD className="font-medium">{q.query}</TD>
                <TD numeric>{formatNumber(q.impressions)}</TD>
                <TD numeric>{formatNumber(q.clicks)}</TD>
                <TD numeric>
                  {formatPercent(q.impressions > 0 ? q.clicks / q.impressions : null, 1)}
                </TD>
                <TD numeric className={cn(q.position > 5 && "text-destructive")}>
                  {q.position.toFixed(1)}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </Panel>
    </div>
  );
}

function AccountTile({ account: a }: { account: OrganicAccount }) {
  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="flex items-center justify-between">
        <Eyebrow>{a.name}</Eyebrow>
        {a.state === "pending" && <StatusChip status="pending">Not connected</StatusChip>}
      </div>
      <div className="mt-2 font-heading text-2xl font-semibold tabular-nums">
        {formatNumber(a.followers)}
      </div>
      <div className="text-xs text-muted-foreground">
        followers
        {a.followersChange !== null &&
          ` · ${a.followersChange >= 0 ? "+" : ""}${formatPercent(a.followersChange, 1)}`}
      </div>
      <dl className="mt-3 grid grid-cols-2 gap-2 border-t pt-3 text-xs">
        <div>
          <dt className="text-muted-foreground">Reach</dt>
          <dd className="font-mono tabular-nums">{formatNumber(a.reach)}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">Engagement</dt>
          <dd className="font-mono tabular-nums">{formatPercent(a.engagementRate, 1)}</dd>
        </div>
      </dl>
    </div>
  );
}
