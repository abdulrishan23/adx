import { useLandingPages, useProjectsList } from "@/api/adx";
import type { LandingPage } from "@/api/models";
import { PERIOD_LABEL, ScopeBar, useScope } from "@/app/scope";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { StatTile } from "@/components/stat-tile";
import { StatusChip } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatNumber, formatPercent } from "@/lib/format";

const FLAG: Record<
  LandingPage["flag"],
  { status: "ready" | "caution" | "error"; label: string; why?: string }
> = {
  ok: { status: "ready", label: "OK" },
  slow: { status: "caution", label: "Slow", why: "Takes over 4 seconds to load on mobile" },
  password: {
    status: "error",
    label: "Password",
    why: "Ads send people to a password-protected page",
  },
  not_found: { status: "error", label: "Not found", why: "The page returns 404" },
};

export function WebsitePage() {
  const pages = useLandingPages();
  const projects = useProjectsList();
  const scope = useScope();
  const name = (id: number | null) =>
    id === null ? null : (projects.data?.find((p) => p.id === id)?.name ?? null);

  const rows = pages.data ?? [];
  const sessions = rows.reduce((a, p) => a + p.sessions, 0);
  const leads = rows.reduce((a, p) => a + p.leads, 0);
  const broken = rows.filter((p) => p.flag === "password" || p.flag === "not_found");
  const wasted = broken.reduce((a, p) => a + p.sessions, 0);

  return (
    <>
      <PageHeader
        title="Website"
        subtitle={PERIOD_LABEL[scope.days]}
        actions={<ScopeBar showPlatform={false} />}
      />
      {pages.isPending && <LoadingRows rows={8} />}
      {pages.error && <ErrorNote error={pages.error} />}
      {pages.data && (
        <div className="space-y-6">
          <div className="grid gap-4 sm:grid-cols-3">
            <StatTile label="Sessions" value={formatNumber(sessions)} />
            <StatTile
              label="Leads from the site"
              value={formatNumber(leads)}
              hint={`${formatPercent(leads / Math.max(1, sessions), 1)} of sessions`}
            />
            <StatTile
              label="Sessions on broken pages"
              value={formatNumber(wasted)}
              hint={
                broken.length
                  ? `${broken.length} ${broken.length === 1 ? "page" : "pages"} with ads pointing at them`
                  : "None"
              }
            />
          </div>

          <Panel bodyClassName="p-0">
            <Table>
              <THead>
                <TR>
                  <TH>Page</TH>
                  <TH>Project</TH>
                  <TH numeric>Sessions</TH>
                  <TH numeric>Leads</TH>
                  <TH numeric>Lead rate</TH>
                  <TH numeric>Bounce</TH>
                  <TH />
                </TR>
              </THead>
              <TBody>
                {[...pages.data]
                  .sort((a, b) => b.sessions - a.sessions)
                  .map((p) => {
                    const flag = FLAG[p.flag];
                    return (
                      <TR key={`${p.site}${p.url}`}>
                        <TD>
                          <div className="max-w-96 truncate font-mono text-xs">{p.url}</div>
                          <div className="text-xs text-muted-foreground">
                            {p.site}
                            {flag.why && ` · ${flag.why}`}
                          </div>
                        </TD>
                        <TD>
                          {name(p.projectId) ?? <span className="text-muted-foreground">None</span>}
                        </TD>
                        <TD numeric>{formatNumber(p.sessions)}</TD>
                        <TD numeric>{formatNumber(p.leads)}</TD>
                        <TD numeric>
                          {formatPercent(p.sessions > 0 ? p.leads / p.sessions : null, 1)}
                        </TD>
                        <TD numeric>{formatPercent(p.bounceRate)}</TD>
                        <TD className="text-right">
                          <StatusChip status={flag.status}>{flag.label}</StatusChip>
                        </TD>
                      </TR>
                    );
                  })}
              </TBody>
            </Table>
          </Panel>
        </div>
      )}
    </>
  );
}
