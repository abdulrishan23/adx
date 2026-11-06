import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowRight, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { useLeadCoverage, useLeads } from "@/api/leads";
import { useProjects } from "@/api/projects";
import type { Lead } from "@/api/types.gen";
import { ScopeBar, useScope } from "@/app/scope";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Segmented } from "@/components/segmented";
import { TemperatureMeter } from "@/components/temperature";
import { Input } from "@/components/ui/input";
import { StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatRelative, NOT_AVAILABLE } from "@/lib/format";
import {
  coverageTiles,
  filterLeads,
  headline,
  type LeadFilter,
  lastActivity,
  leadName,
  matchLabel,
  STAGE_LABEL,
  type TemperatureTab,
  temperatureCounts,
  temperatureOf,
} from "@/lib/leads";
import { cn } from "@/lib/utils";

const PAGE = 50;

// Leads as GoHighLevel last gave them (core/leads), with the coverage every
// figure depends on across the top. The raw status sits beside the ADX
// stage, so an unmapped status is visible rather than hidden, and a lead
// nobody has scored reads as not assessed, never as cold.
export function LeadsPage() {
  const scope = useScope();
  const leads = useLeads();
  const coverage = useLeadCoverage();
  const projects = useProjects();
  const navigate = useNavigate();
  const [tab, setTab] = useState<TemperatureTab>("all");
  const [filter, setFilter] = useState<LeadFilter | null>(null);
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(0);

  const all = useMemo(
    () =>
      (leads.data ?? []).filter(
        (l) => scope.projectId === "all" || l.projectId === scope.projectId,
      ),
    [leads.data, scope.projectId],
  );
  const counts = temperatureCounts(all);
  const rows = useMemo(() => filterLeads(all, { tab, filter, query }), [all, tab, filter, query]);
  const projectName = (l: Lead) =>
    projects.data?.projects.find((p) => p.id === l.projectId)?.name ?? l.projectRaw ?? null;

  const shown = rows.slice(page * PAGE, page * PAGE + PAGE);
  const pages = Math.ceil(rows.length / PAGE);
  const head = coverage.data ? headline(coverage.data) : null;
  const tiles = coverage.data ? coverageTiles(coverage.data) : [];

  function toggleFilter(f: LeadFilter) {
    setFilter((cur) => (cur === f ? null : f));
    setTab("all");
    setPage(0);
  }

  return (
    <>
      <PageHeader title="Leads" actions={<ScopeBar showPlatform={false} />} />

      {coverage.error && <ErrorNote error={coverage.error} />}
      {coverage.data && head && (
        <div className="mb-6 space-y-4">
          <button
            type="button"
            disabled={!head.filter}
            onClick={() => head.filter && toggleFilter(head.filter)}
            className="group flex w-full items-center justify-between gap-4 rounded-lg bg-primary px-5 py-4 text-left text-primary-foreground disabled:cursor-default"
          >
            <span className="font-heading text-lg font-medium">{head.text}</span>
            {head.filter && (
              <ArrowRight className="size-5 shrink-0 transition-transform group-hover:translate-x-1" />
            )}
          </button>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            {tiles.map((t) => (
              <button
                key={t.key}
                type="button"
                aria-pressed={filter === t.key}
                disabled={t.count === null}
                onClick={() => toggleFilter(t.key)}
                className={cn(
                  "rounded-lg border bg-card px-4 py-3 text-left transition-colors hover:border-foreground/40 disabled:cursor-default disabled:hover:border-border",
                  filter === t.key && "border-foreground ring-1 ring-foreground",
                )}
              >
                {t.count === null ? (
                  <span className="block py-1 text-sm text-muted-foreground">{NOT_AVAILABLE}</span>
                ) : (
                  <span
                    className={cn(
                      "block font-mono text-2xl font-semibold tabular-nums",
                      t.urgent && "text-destructive",
                    )}
                  >
                    {t.count}
                  </span>
                )}
                <span className="block text-sm text-muted-foreground">{t.label}</span>
                <span className="block text-xs text-muted-foreground">{t.detail}</span>
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
            placeholder="Search name, phone or email"
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
      {leads.data && all.length === 0 && (
        <Panel>
          <EmptyState title="No leads yet">
            Connect a GoHighLevel workspace and import its contacts from{" "}
            <Link to="/settings/connections/gohighlevel" className="underline underline-offset-4">
              Connections
            </Link>
            .
          </EmptyState>
        </Panel>
      )}
      {leads.data && all.length > 0 && rows.length === 0 && (
        <Panel>
          <EmptyState title="No leads match">
            {filter
              ? "Clear the filter above to see every lead."
              : tab === "all"
                ? "Nothing matches the search."
                : "Nothing scored in this temperature for this scope."}
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
                <TH>Stage</TH>
                <TH>Project</TH>
                <TH>Status in GoHighLevel</TH>
                <TH>Source</TH>
                <TH numeric>Last activity</TH>
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
                      {leadName(l)}
                    </Link>
                    <p className="font-mono text-xs text-muted-foreground">
                      {l.dialCode ? `+${l.dialCode}` : (l.phone ?? "No phone")}
                    </p>
                  </TD>
                  <TD>
                    <TemperatureCell lead={l} />
                  </TD>
                  <TD className="whitespace-nowrap">
                    <StageCell lead={l} />
                  </TD>
                  <TD className="whitespace-nowrap">
                    {projectName(l) ?? <span className="text-muted-foreground">No project</span>}
                  </TD>
                  <TD className="whitespace-nowrap">
                    <StatusCell lead={l} />
                  </TD>
                  <TD>
                    <SourceCell lead={l} />
                  </TD>
                  <TD numeric className="text-muted-foreground">
                    {formatRelative(lastActivity(l))}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
          {pages > 1 && (
            <div className="flex items-center justify-between border-t px-5 py-3 text-sm">
              <span className="text-muted-foreground">
                {page * PAGE + 1} to {Math.min(rows.length, (page + 1) * PAGE)} of {rows.length}
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

export function TemperatureCell({ lead }: { lead: Lead }) {
  const t = temperatureOf(lead);
  if (!t) return <span className="text-sm text-muted-foreground">Not assessed</span>;
  return <TemperatureMeter temperature={t} />;
}

// The stage is the engine's, from the status-to-stage table. No stage is a
// status the table does not know yet (or no table at all), flagged so it
// gets a row rather than hiding in the funnel figures.
export function StageCell({ lead }: { lead: Lead }) {
  if (lead.stage) return <>{STAGE_LABEL[lead.stage]}</>;
  return (
    <span className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
      <StatusMark status="caution" />
      No stage
    </span>
  );
}

export function StatusCell({ lead }: { lead: Lead }) {
  return (
    <>
      {lead.statusRaw ?? <span className="text-muted-foreground">No status</span>}
      {lead.statusMissingSince && (
        <span
          className="block text-xs text-muted-foreground"
          title={`The status field was missing when GoHighLevel was last read, since ${lead.statusMissingSince}`}
        >
          Field missing on last scan
        </span>
      )}
    </>
  );
}

function SourceCell({ lead }: { lead: Lead }) {
  return (
    <div className="whitespace-nowrap">
      <p>{lead.sourceRaw ?? <span className="text-muted-foreground">No source</span>}</p>
      <p className="text-xs text-muted-foreground">
        {lead.source && lead.source !== lead.sourceRaw ? `Counted as ${lead.source} · ` : ""}
        {matchLabel(lead.matchLevel)}
      </p>
    </div>
  );
}
