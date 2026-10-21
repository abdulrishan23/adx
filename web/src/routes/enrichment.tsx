import { Pause, Play } from "lucide-react";
import { type FormEvent, useId, useState } from "react";
import {
  useCandidateCount,
  useEnrichmentQueue,
  usePauseEnrichment,
  useProjectsList,
  useStartEnrichment,
} from "@/api/adx";
import type { CandidateFilter, EnrichmentJob } from "@/api/models";
import { Meter } from "@/components/charts";
import { Form } from "@/components/form";
import { ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { StatTile } from "@/components/stat-tile";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { StatusChip } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { formatNumber, formatRelative } from "@/lib/format";

const JOB_STATE: Record<
  EnrichmentJob["state"],
  { chip: "ready" | "pending" | "caution"; label: string }
> = {
  running: { chip: "pending", label: "Running" },
  queued: { chip: "pending", label: "Queued" },
  paused: { chip: "caution", label: "Paused" },
  done: { chip: "ready", label: "Done" },
};

export function EnrichmentPage() {
  const queue = useEnrichmentQueue();
  const pause = usePauseEnrichment();
  const q = queue.data;
  return (
    <>
      <PageHeader
        title="Enrichment"
        actions={
          q && (
            <Button
              variant="outline"
              disabled={pause.isPending}
              onClick={() => pause.mutate(!q.paused)}
            >
              {q.paused ? <Play /> : <Pause />}
              {q.paused ? "Resume" : "Pause all"}
            </Button>
          )
        }
      />
      {queue.isPending && <LoadingRows rows={8} />}
      {queue.error && <ErrorNote error={queue.error} />}
      {q && (
        <div className="space-y-6">
          {q.paused && (
            <p className="rounded-lg border border-status-caution/50 bg-status-caution/10 px-4 py-3 text-sm">
              Paused.
            </p>
          )}
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <div className="rounded-lg border bg-card p-5">
              <p className="font-mono text-[0.68rem] font-medium tracking-widest text-muted-foreground uppercase">
                Lookups this month
              </p>
              <p className="mt-2 font-heading text-3xl font-semibold tabular-nums">
                {formatNumber(q.monthUsed)}
              </p>
              <Meter value={q.monthUsed} max={q.monthLimit} className="mt-3" />
              <p className="mt-2 text-xs text-muted-foreground">of {formatNumber(q.monthLimit)}</p>
            </div>
            <StatTile label="Cold leads" value={formatNumber(q.candidates.cold)} />
            <StatTile label="No status" value={formatNumber(q.candidates.noStatus)} />
            <StatTile label="Never enriched" value={formatNumber(q.candidates.neverEnriched)} />
          </div>

          <PickLeads left={q.monthLimit - q.monthUsed} />

          <Panel title="Jobs" bodyClassName="p-0">
            <Table>
              <THead>
                <TR>
                  <TH className="min-w-48">Batch</TH>
                  <TH className="w-56">Progress</TH>
                  <TH numeric>No match</TH>
                  <TH>Started</TH>
                  <TH />
                </TR>
              </THead>
              <TBody>
                {q.jobs.map((j) => (
                  <TR key={j.id}>
                    <TD>
                      <div className="font-medium">{j.label}</div>
                      <div className="text-xs text-muted-foreground">{j.requestedBy}</div>
                    </TD>
                    <TD>
                      <div className="flex items-center gap-3">
                        <Meter value={j.done + j.failed} max={j.total} />
                        <span className="shrink-0 font-mono text-xs tabular-nums">
                          {formatNumber(j.done + j.failed)}/{formatNumber(j.total)}
                        </span>
                      </div>
                    </TD>
                    <TD numeric>{formatNumber(j.failed)}</TD>
                    <TD className="text-muted-foreground">{formatRelative(j.createdAt)}</TD>
                    <TD className="text-right">
                      <StatusChip status={JOB_STATE[j.state].chip}>
                        {JOB_STATE[j.state].label}
                      </StatusChip>
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          </Panel>
        </div>
      )}
    </>
  );
}

const DEFAULT_FILTER: CandidateFilter = {
  temperature: "cold",
  noStatus: true,
  projectId: "all",
  olderThanDays: 30,
  skipEnriched: true,
};

function labelFor(f: CandidateFilter, project: string | null) {
  const temp = f.temperature === "any" ? "All" : f.temperature === "cold" ? "Cold" : "Warm";
  return `${temp} ${project ? `${project} ` : ""}leads${f.noStatus ? " with no status" : ""}${
    f.olderThanDays ? `, older than ${f.olderThanDays} days` : ""
  }`;
}

function PickLeads({ left }: { left: number }) {
  const [filter, setFilter] = useState<CandidateFilter>(DEFAULT_FILTER);
  const [limit, setLimit] = useState("500");
  const projects = useProjectsList();
  const count = useCandidateCount(filter);
  const start = useStartEnrichment();
  const limitId = useId();
  const set = (patch: Partial<CandidateFilter>) => {
    start.reset();
    setFilter((f) => ({ ...f, ...patch }));
  };

  const available = count.data ?? 0;
  const wanted = Math.max(0, Math.floor(Number(limit) || 0));
  const n = Math.min(wanted, available);
  const project =
    filter.projectId === "all"
      ? null
      : (projects.data?.find((p) => p.id === filter.projectId)?.name ?? null);

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    start.mutate({ filter, limit: n, label: labelFor(filter, project) });
  }

  return (
    <Panel title="New batch">
      <Form onSubmit={onSubmit} className="space-y-4">
        <div className="flex flex-wrap items-end gap-3">
          <Field label="Temperature">
            <Select
              aria-label="Temperature"
              value={filter.temperature}
              onValueChange={(temperature) => set({ temperature })}
              options={[
                { value: "cold", label: "Cold" },
                { value: "warm", label: "Warm" },
                { value: "any", label: "Any" },
              ]}
            />
          </Field>
          <Field label="Project">
            <Select
              aria-label="Project"
              value={String(filter.projectId)}
              onValueChange={(v) => set({ projectId: v === "all" ? "all" : Number(v) })}
              options={[
                { value: "all", label: "All projects" },
                ...(projects.data ?? []).map((p) => ({ value: String(p.id), label: p.name })),
              ]}
            />
          </Field>
          <Field label="Arrived">
            <Select
              aria-label="Arrived"
              value={String(filter.olderThanDays)}
              onValueChange={(v) =>
                set({ olderThanDays: Number(v) as CandidateFilter["olderThanDays"] })
              }
              options={[
                { value: "0", label: "Any time" },
                { value: "30", label: "Over 30 days ago" },
                { value: "90", label: "Over 90 days ago" },
              ]}
            />
          </Field>
          <label className="flex h-8 items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={filter.noStatus}
              onChange={(e) => set({ noStatus: e.target.checked })}
              className="size-4 accent-foreground"
            />
            No status only
          </label>
          <label className="flex h-8 items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={filter.skipEnriched}
              onChange={(e) => set({ skipEnriched: e.target.checked })}
              className="size-4 accent-foreground"
            />
            Skip already enriched
          </label>
        </div>

        <div className="flex flex-wrap items-center gap-3 border-t pt-4">
          <p className="text-sm">
            <span className="font-mono font-semibold tabular-nums">{formatNumber(available)}</span>{" "}
            leads match.
          </p>
          <span className="text-muted-foreground">·</span>
          <label htmlFor={limitId} className="flex items-center gap-2 text-sm">
            Enrich up to
            <Input
              inputMode="numeric"
              value={limit}
              onChange={(e) => {
                start.reset();
                setLimit(e.target.value.replace(/\D/g, ""));
              }}
              id={limitId}
              className="h-8 w-24 font-mono"
              aria-label="How many leads to enrich"
            />
          </label>
          <Button type="submit" className="ml-auto" disabled={n === 0 || start.isPending}>
            Enrich {formatNumber(n)} leads
          </Button>
        </div>
        {n > left && (
          <p className="text-sm text-destructive">
            That's more than the {formatNumber(left)} lookups left this month.
          </p>
        )}
        {start.error && <ErrorNote error={start.error} />}
        {start.isSuccess && (
          <p className="text-sm text-muted-foreground" role="status">
            Queued "{start.data.label}". It appears under Jobs below.
          </p>
        )}
      </Form>
    </Panel>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    // biome-ignore lint/a11y/noLabelWithoutControl: the control is the child
    <label className="flex flex-col gap-1 text-xs text-muted-foreground">
      {label}
      {children}
    </label>
  );
}
