import { Link } from "@tanstack/react-router";
import { RefreshCw, TriangleAlert } from "lucide-react";
import { useLeadEnrichment, useRequestEnrichment } from "@/api/adx";
import type { Confidence, EnrichmentState, LeadEnrichment } from "@/api/models";
import { ErrorNote, Eyebrow, LoadingRows, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { StatusChip, StatusMark } from "@/components/ui/status-chip";
import { formatRelative, NOT_AVAILABLE } from "@/lib/format";
import { cn } from "@/lib/utils";

const STATE: Record<
  EnrichmentState,
  { chip: "ready" | "caution" | "error" | "pending"; label: string }
> = {
  done: { chip: "ready", label: "Enriched" },
  running: { chip: "pending", label: "Looking up" },
  queued: { chip: "pending", label: "Queued" },
  failed: { chip: "error", label: "No match" },
  never: { chip: "pending", label: "Not enriched" },
};

const CONFIDENCE_LABEL: Record<Confidence, string> = {
  high: "Sure",
  medium: "Likely",
  low: "Guess",
};

// The brief and the enrichment it was written from. The brief says what to do
// next; the facts beside it say why, and where each one came from.
export function LeadBriefPanels({ leadId }: { leadId: number }) {
  const enrichment = useLeadEnrichment(leadId);
  if (enrichment.isPending) return <LoadingRows rows={4} />;
  if (enrichment.error) return <ErrorNote error={enrichment.error} />;
  if (!enrichment.data) return null;
  return (
    <div className="grid gap-4 lg:grid-cols-[3fr_2fr]">
      <BriefPanel enrichment={enrichment.data} />
      <FactsPanel enrichment={enrichment.data} />
    </div>
  );
}

function BriefPanel({ enrichment: e }: { enrichment: LeadEnrichment }) {
  const { brief } = e;
  return (
    <section className="rounded-lg border bg-foreground p-5 text-background sm:p-6">
      <Eyebrow className="text-background/60">Do this</Eyebrow>
      <p className="mt-2 font-heading text-xl leading-snug font-semibold tracking-tight sm:text-2xl">
        {brief.doThis}
      </p>

      {brief.angles.length > 0 && (
        <div className="mt-5">
          <Eyebrow className="mb-2 text-background/60">Angles</Eyebrow>
          <ul className="grid gap-2 sm:grid-cols-2">
            {brief.angles.map((a) => (
              <li
                key={a}
                className="rounded-md bg-background/10 px-3 py-2.5 text-sm leading-relaxed"
              >
                {a}
              </li>
            ))}
          </ul>
        </div>
      )}

      {brief.watchOut && (
        <div className="mt-4 flex gap-2.5 rounded-md bg-status-caution/15 px-3 py-2.5 text-sm leading-relaxed">
          <TriangleAlert
            className="mt-0.5 size-4 shrink-0 text-status-caution"
            aria-hidden="true"
          />
          <p>
            <span className="font-medium">Watch out. </span>
            {brief.watchOut}
          </p>
        </div>
      )}

      {e.state !== "done" && <p className="mt-4 text-xs text-background/60">Enrichment pending.</p>}
    </section>
  );
}

function FactsPanel({ enrichment: e }: { enrichment: LeadEnrichment }) {
  const request = useRequestEnrichment();
  const state = STATE[e.state];
  const busy = e.state === "queued" || e.state === "running";
  return (
    <Panel
      title="Enrichment"
      description={
        e.enrichedAt
          ? `Looked up ${formatRelative(e.enrichedAt)}`
          : (e.failure ?? "Only what's in GoHighLevel so far")
      }
      action={<StatusChip status={state.chip}>{state.label}</StatusChip>}
      bodyClassName="p-0"
    >
      <dl className="divide-y">
        {e.facts.map((f) => (
          <div
            key={f.label}
            className="grid grid-cols-[7.5rem_1fr_auto] items-baseline gap-3 px-5 py-2.5 text-sm"
          >
            <dt className="text-muted-foreground">{f.label}</dt>
            <dd className={cn(f.value === null && "text-muted-foreground")}>
              {f.value ?? NOT_AVAILABLE}
              <span className="block text-xs text-muted-foreground">{f.source}</span>
            </dd>
            <dd
              className={cn(
                "font-mono text-[0.65rem] tracking-wider whitespace-nowrap uppercase",
                f.confidence === "low" ? "text-foreground" : "text-muted-foreground",
              )}
              title={`Confidence: ${f.confidence}`}
            >
              {f.value !== null && f.confidence === "low" && (
                <StatusMark status="caution" className="mr-1.5" />
              )}
              {f.value === null ? "" : CONFIDENCE_LABEL[f.confidence]}
            </dd>
          </div>
        ))}
      </dl>
      <div className="flex items-center justify-between gap-3 border-t px-5 py-3">
        <Link to="/enrichment" className="text-xs text-muted-foreground hover:text-foreground">
          Enrichment queue
        </Link>
        <Button
          size="sm"
          variant="outline"
          disabled={busy || request.isPending}
          onClick={() => request.mutate(e.leadId)}
        >
          <RefreshCw className={cn(request.isPending && "animate-spin")} />
          {busy ? "In the queue" : e.state === "done" ? "Look up again" : "Enrich now"}
        </Button>
      </div>
      {request.error && (
        <div className="px-5 pb-3">
          <ErrorNote error={request.error} />
        </div>
      )}
    </Panel>
  );
}
