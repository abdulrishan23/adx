import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowRight, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { useLeads, useLeadsSummary } from "@/api/adx";
import { projectOf } from "@/api/mock/data";
import { matchesFilter } from "@/api/mock/server";
import type { Lead, LeadFilter, MatchLevel, Temperature } from "@/api/models";
import { ScopeBar } from "@/app/scope";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { TemperatureMeter } from "@/components/temperature";
import { Input } from "@/components/ui/input";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";

export const MATCH_LABEL: Record<MatchLevel, string> = {
  click_id: "Click ID",
  lead_id: "Platform lead ID",
  contact: "Phone or email",
  utm: "UTM only",
  none: "Unknown",
};

type Tab = Temperature | "all";
const PAGE = 50;

export function LeadsPage() {
  const leads = useLeads();
  const summary = useLeadsSummary();
  const navigate = useNavigate();
  const [tab, setTab] = useState<Tab>("hot");
  const [filter, setFilter] = useState<LeadFilter | null>(null);
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);

  const all = leads.data ?? [];
  const counts = {
    hot: all.filter((l) => l.temperature === "hot").length,
    warm: all.filter((l) => l.temperature === "warm").length,
    cold: all.filter((l) => l.temperature === "cold").length,
    all: all.length,
  };

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase();
    return all.filter(
      (l) =>
        // A search spans every temperature, so a name is never hidden by the tab.
        (q ? l.name.toLowerCase().includes(q) : tab === "all" || l.temperature === tab) &&
        (!filter || matchesFilter(l, filter)),
    );
  }, [all, tab, filter, query]);

  const shown = rows.slice(page * PAGE, page * PAGE + PAGE);
  const pages = Math.ceil(rows.length / PAGE);

  function toggleFilter(f: LeadFilter) {
    setFilter((cur) => (cur === f ? null : f));
    setTab("all");
    setPage(0);
  }

  return (
    <>
      <PageHeader title="Leads" actions={<ScopeBar showPlatform={false} />} />

      {summary.data && (
        <div className="mb-6 space-y-4">
          <button
            type="button"
            disabled={!summary.data.headline.filter}
            onClick={() =>
              summary.data.headline.filter && toggleFilter(summary.data.headline.filter)
            }
            className="group flex w-full items-center justify-between gap-4 rounded-lg bg-primary px-5 py-4 text-left text-primary-foreground disabled:cursor-default"
          >
            <span className="font-heading text-lg font-medium">{summary.data.headline.text}</span>
            {summary.data.headline.filter && (
              <ArrowRight className="size-5 shrink-0 transition-transform group-hover:translate-x-1" />
            )}
          </button>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            {summary.data.stats.map((s) => (
              <button
                key={s.key}
                type="button"
                aria-pressed={filter === s.key}
                onClick={() => toggleFilter(s.key)}
                className={cn(
                  "rounded-lg border bg-card px-4 py-3 text-left transition-colors hover:border-foreground/40",
                  filter === s.key && "border-foreground ring-1 ring-foreground",
                )}
              >
                <span
                  className={cn(
                    "block font-mono text-2xl font-semibold tabular-nums",
                    s.urgent && "text-destructive",
                  )}
                >
                  {s.count}
                </span>
                <span className="text-sm text-muted-foreground">{s.label}</span>
              </button>
            ))}
          </div>
        </div>
      )}

      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <Segmented
          label="Temperature"
          value={tab}
          onChange={(t) => {
            setTab(t);
            setPage(0);
          }}
          options={[
            { value: "hot", label: "Hot", count: counts.hot },
            { value: "warm", label: "Warm", count: counts.warm },
            { value: "cold", label: "Cold", count: counts.cold },
            { value: "all", label: "All", count: counts.all },
          ]}
        />
        <div className="relative w-full sm:w-64">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label="Search leads"
            placeholder="Search name…"
            className="pl-8"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setPage(0);
            }}
          />
        </div>
      </div>

      {leads.isPending && <LoadingRows rows={12} />}
      {leads.error && <ErrorNote error={leads.error} />}
      {leads.data && rows.length === 0 && (
        <Panel>
          <EmptyState title="No leads match">
            {filter
              ? "Clear the filter above to see every lead."
              : "Nothing in this temperature for this scope."}
          </EmptyState>
        </Panel>
      )}
      {shown.length > 0 && (
        <section className="rounded-lg border bg-card">
          <Table>
            <THead>
              <TR>
                <TH>Lead</TH>
                <TH>Temperature</TH>
                <TH>Why</TH>
                <TH>Project</TH>
                <TH>Status</TH>
                <TH>Source</TH>
                <TH>Owner</TH>
                <TH numeric>Last active</TH>
              </TR>
            </THead>
            <TBody>
              {shown.map((l) => (
                <TR
                  key={l.id}
                  tabIndex={0}
                  className="cursor-pointer hover:bg-muted/50"
                  onClick={() =>
                    navigate({ to: "/leads/$leadId", params: { leadId: String(l.id) } })
                  }
                  onKeyDown={(e) =>
                    e.key === "Enter" &&
                    navigate({ to: "/leads/$leadId", params: { leadId: String(l.id) } })
                  }
                >
                  <TD>
                    <Link
                      to="/leads/$leadId"
                      params={{ leadId: String(l.id) }}
                      className="font-medium"
                      onClick={(e) => e.stopPropagation()}
                    >
                      {l.name}
                    </Link>
                    <p className="font-mono text-xs text-muted-foreground">{l.phoneCountry}</p>
                  </TD>
                  <TD>
                    <TemperatureMeter temperature={l.temperature} />
                  </TD>
                  <TD className="max-w-64 truncate text-muted-foreground">{l.reason}</TD>
                  <TD className="whitespace-nowrap">
                    {l.projectId ? projectOf(l.projectId) : "—"}
                  </TD>
                  <TD className="whitespace-nowrap">
                    {l.status ?? <span className="text-muted-foreground">No status</span>}
                  </TD>
                  <TD>
                    <SourceCell lead={l} />
                  </TD>
                  <TD>{l.owner}</TD>
                  <TD numeric className="text-muted-foreground">
                    {formatRelative(l.lastActivityAt)}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
          {pages > 1 && (
            <div className="flex items-center justify-between border-t px-5 py-3 text-sm">
              <span className="text-muted-foreground">
                {page * PAGE + 1}–{Math.min(rows.length, (page + 1) * PAGE)} of {rows.length}
              </span>
              <div className="flex gap-1">
                <button
                  type="button"
                  className="rounded-md px-2 py-1 hover:bg-muted disabled:opacity-40"
                  disabled={page === 0}
                  onClick={() => setPage(page - 1)}
                >
                  Previous
                </button>
                <button
                  type="button"
                  className="rounded-md px-2 py-1 hover:bg-muted disabled:opacity-40"
                  disabled={page >= pages - 1}
                  onClick={() => setPage(page + 1)}
                >
                  Next
                </button>
              </div>
            </div>
          )}
        </section>
      )}
    </>
  );
}

function SourceCell({ lead }: { lead: Lead }) {
  const recovered = lead.source !== lead.originalSource;
  return (
    <div className="whitespace-nowrap">
      <p>{lead.originalSource}</p>
      <p className="text-xs text-muted-foreground">
        {recovered ? `Recovered from ${lead.source}` : MATCH_LABEL[lead.matchLevel]}
      </p>
    </div>
  );
}
