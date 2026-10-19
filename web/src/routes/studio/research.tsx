import { Link } from "@tanstack/react-router";
import { ExternalLink, Play, RefreshCw } from "lucide-react";
import { useState } from "react";
import { useBrands } from "@/api/brands";
import { useBrief, useBriefs, useResolveCompetitor, useStartResearchRun } from "@/api/research";
import type {
  Brief,
  BriefDetail,
  CompetitorStatus,
  Finding,
  Item,
  ItemKind,
  ItemState,
} from "@/api/types.gen";
import { EmptyState, ErrorNote, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { formatNumber, formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";

const ITEM_KIND: Record<ItemKind, string> = {
  web_page: "Page",
  competitor_page: "Competitor page",
  keywords: "Keyword ideas",
  account_audit: "Account audit",
};

const ITEM_STATE: Record<ItemState, { status: Status; label: string }> = {
  pending: { status: "pending", label: "Waiting to run" },
  fetched: { status: "ready", label: "Read" },
  failed: { status: "error", label: "Failed" },
  refused: { status: "error", label: "Refused" },
  needs_identity: { status: "caution", label: "Who is this?" },
};

// Research is what grounds a plan: pages Adx read, competitor pages kept on
// their side, keyword ideas from Google (estimates, labelled so) and an
// audit of our own account history. A source that didn't answer leaves a
// gap; nothing is estimated by Adx itself.
export function ResearchPage() {
  const briefs = useBriefs();
  const brands = useBrands();
  const [chosen, setChosen] = useState<number | null>(null);
  const list = briefs.data?.briefs ?? [];
  const id = chosen ?? list[0]?.id ?? null;
  const brandName = (b: Brief) =>
    brands.data?.brands.find((x) => x.id === b.brandId)?.name ?? `Brand ${b.brandId}`;

  return (
    <>
      <PageHeader
        title="Research"
        subtitle="What Adx read and measured for a brand. Plans cite these findings; gaps stay gaps."
        actions={
          list.length > 0 && (
            <Select
              aria-label="Brief"
              value={id === null ? null : String(id)}
              onValueChange={(v) => setChosen(Number(v))}
              options={list.map((b) => ({
                value: String(b.id),
                label: `${brandName(b)} · ${b.title}`,
              }))}
            />
          )
        }
      />
      {briefs.isPending && <LoadingRows rows={6} />}
      {briefs.error && <ErrorNote error={briefs.error} />}
      {briefs.data && list.length === 0 && (
        <Panel>
          <EmptyState title="No research yet">
            <Link to="/brands/new" className="underline underline-offset-4">
              Start a brand
            </Link>{" "}
            and Adx opens its first brief. Anything else can be set up on the{" "}
            <Link to="/engine/research" className="underline underline-offset-4">
              engine's research screen
            </Link>
            .
          </EmptyState>
        </Panel>
      )}
      {id !== null && <BriefView id={id} />}
    </>
  );
}

function BriefView({ id }: { id: number }) {
  const brief = useBrief(id);
  const start = useStartResearchRun(id);
  const d = brief.data;
  if (brief.isPending) return <LoadingRows rows={6} />;
  if (brief.error) return <ErrorNote error={brief.error} />;
  if (!d) return null;
  const pending = d.items.filter((i) => i.state === "pending").length;
  const running = pending > 0 && d.runs > 0;
  const pages = d.findings.filter((f) => f.kind === "page_text");
  const keywords = d.findings.filter((f) => f.kind === "keyword");
  const audit = d.findings.filter((f) => f.kind === "audit_metric" || f.kind === "audit_note");
  const unresolved = d.competitors.filter((c) => c.state !== "resolved" && c.state !== "n/a");
  return (
    <div className="space-y-6">
      <Panel
        title={d.brief.title}
        description={d.brief.question || "No question set"}
        action={
          <div className="flex items-center gap-2">
            <span className="font-mono text-xs text-muted-foreground">
              {d.brief.geography} · {d.brief.language} · {d.runs} {d.runs === 1 ? "run" : "runs"}
            </span>
            <Button
              size="sm"
              variant="outline"
              disabled={start.isPending || running}
              onClick={() => start.mutate()}
            >
              {running ? (
                <>
                  <RefreshCw className="animate-spin" /> Running
                </>
              ) : (
                <>
                  <Play /> {d.runs === 0 ? "Run" : "Run again"}
                </>
              )}
            </Button>
          </div>
        }
        bodyClassName="p-0"
      >
        <ul className="divide-y">
          {d.items.map((it) => (
            <ItemRow key={it.id} item={it} />
          ))}
        </ul>
        {start.error && <p className="px-5 py-3 text-xs text-destructive">{start.error.message}</p>}
      </Panel>

      {unresolved.length > 0 && (
        <Panel
          title="Competitors Adx can't place"
          description="A name is matched only when it is exactly a competitor brand. Pick the brand, or add one under Brands."
          className="border-status-caution/50"
        >
          <ul className="space-y-3">
            {unresolved.map((c) => (
              <CompetitorRow key={c.name} briefId={id} c={c} />
            ))}
          </ul>
        </Panel>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <Panel
          title="Pages read"
          description="What the sites say, as observed. Ours on one side, competitors on theirs."
          bodyClassName="p-0"
        >
          {pages.length === 0 ? (
            <EmptyState title="Nothing read yet" />
          ) : (
            <ul className="divide-y">
              {pages.map((f) => (
                <PageFinding key={f.id} f={f} d={d} />
              ))}
            </ul>
          )}
        </Panel>

        <div className="space-y-6">
          <Panel
            title="Keyword ideas"
            description="Google's estimates, not observations. Volumes are rounded ranges."
            bodyClassName="p-0"
          >
            {keywords.length === 0 ? (
              <EmptyState title="None yet">
                Keyword ideas need a Google Ads account on the brief and a run.
              </EmptyState>
            ) : (
              <table className="w-full text-sm">
                <thead>
                  <tr className="font-mono text-[0.65rem] tracking-widest text-muted-foreground uppercase">
                    <th className="px-5 py-2 text-left font-medium">Term</th>
                    <th className="px-3 py-2 text-right font-medium">Searches</th>
                    <th className="px-5 py-2 text-right font-medium">Competition</th>
                  </tr>
                </thead>
                <tbody>
                  {keywords.map((f) => {
                    const idea = parse<{
                      idea?: { avgMonthlySearches?: number | null; competition?: string };
                    }>(f.payload).idea;
                    return (
                      <tr key={f.id} className="border-t">
                        <td className="px-5 py-2">{f.label}</td>
                        <td className="px-3 py-2 text-right font-mono tabular-nums">
                          {idea?.avgMonthlySearches == null ? (
                            <span className="text-muted-foreground">Not available</span>
                          ) : (
                            formatNumber(idea.avgMonthlySearches)
                          )}
                        </td>
                        <td className="px-5 py-2 text-right text-muted-foreground">
                          {idea?.competition || "Not available"}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            )}
          </Panel>

          <Panel
            title="Our account"
            description="Observed from connected ad accounts. Private; never mixed with competitors."
            bodyClassName="p-0"
          >
            {audit.length === 0 ? (
              <EmptyState title="No audit yet">
                {d.brief.googleAdsAccountId === null &&
                !d.items.some((i) => i.kind === "account_audit")
                  ? "The brief didn't ask for one."
                  : "Runs once an ad account is connected."}
              </EmptyState>
            ) : (
              <ul className="divide-y">
                {audit.map((f) => (
                  <li key={f.id} className="px-5 py-3">
                    <p className="text-sm font-medium">{f.label}</p>
                    <AuditBody f={f} />
                  </li>
                ))}
              </ul>
            )}
          </Panel>
        </div>
      </div>

      <p className="text-xs text-muted-foreground">
        Schedules, market events and the monthly comparative report live on the{" "}
        <Link to="/engine/research" className="underline underline-offset-4">
          engine's research screen
        </Link>{" "}
        until they're brought here.
      </p>
    </div>
  );
}

function ItemRow({ item: it }: { item: Item }) {
  const s = ITEM_STATE[it.state];
  return (
    <li className="flex items-center gap-3 px-5 py-2.5 text-sm">
      <StatusMark status={s.status} className={cn(it.state === "pending" && "animate-pulse")} />
      <span className="w-32 shrink-0 text-muted-foreground">{ITEM_KIND[it.kind]}</span>
      <span className="min-w-0 flex-1 truncate">
        {it.kind === "competitor_page" && it.competitorName && (
          <span className="font-medium">{it.competitorName} · </span>
        )}
        {it.target || "Connected account"}
      </span>
      <span className="shrink-0 font-mono text-xs text-muted-foreground">
        {it.state === "failed" || it.state === "refused" ? it.error || s.label : s.label}
        {it.fetchedAt && ` · ${formatRelative(it.fetchedAt)}`}
      </span>
    </li>
  );
}

function CompetitorRow({ briefId, c }: { briefId: number; c: CompetitorStatus }) {
  const resolve = useResolveCompetitor(briefId);
  const brands = useBrands();
  const options = (brands.data?.brands ?? [])
    .filter((b) => b.kind === "competitor")
    .map((b) => ({ value: String(b.id), label: b.name }));
  return (
    <li className="flex flex-wrap items-center gap-3 text-sm">
      <span className="font-medium">{c.name}</span>
      <StatusChip status="caution">
        {c.state === "ambiguous" ? "Several match" : "Unknown"}
      </StatusChip>
      {c.candidates.length > 0 && (
        <span className="text-xs text-muted-foreground">
          Could be {c.candidates.map((x) => x.name).join(", ")}
        </span>
      )}
      <Select
        aria-label={`Brand for ${c.name}`}
        className="ml-auto"
        value={null}
        onValueChange={(v) => resolve.mutate({ name: c.name, brandId: Number(v) })}
        options={options}
        placeholder="Pick the brand"
      />
    </li>
  );
}

function PageFinding({ f, d }: { f: Finding; d: BriefDetail }) {
  const p = parse<{
    title?: string;
    status?: number;
    text?: string;
    truncated?: boolean;
    chars?: number;
  }>(f.payload);
  const competitor = d.items.find((i) => i.id === f.itemId)?.competitorName;
  return (
    <li className="px-5 py-3">
      <p className="flex items-center gap-2 text-sm font-medium">
        {competitor && <StatusChip status="pending">{competitor}</StatusChip>}
        <span className="min-w-0 truncate">{p.title || f.label}</span>
      </p>
      <p className="mt-0.5 flex items-center gap-2 text-xs text-muted-foreground">
        <a
          href={f.sourceUrl}
          target="_blank"
          rel="noreferrer"
          className="inline-flex min-w-0 items-center gap-1 truncate hover:text-foreground"
        >
          <span className="truncate">{f.sourceUrl}</span>
          <ExternalLink className="size-3 shrink-0" />
        </a>
        <span className="shrink-0">· {formatRelative(f.sourceObservedAt)}</span>
        {p.chars !== undefined && (
          <span className="shrink-0">
            · {formatNumber(p.chars)} chars{p.truncated ? ", cut" : ""}
          </span>
        )}
      </p>
      {p.text && <p className="mt-2 line-clamp-3 text-sm text-muted-foreground">{p.text}</p>}
    </li>
  );
}

function AuditBody({ f }: { f: Finding }) {
  const p = parse<{ note?: string; message?: string; metrics?: Record<string, unknown> }>(
    f.payload,
  );
  if (f.kind === "audit_note") {
    return <p className="mt-0.5 text-xs text-muted-foreground">{p.note || p.message || ""}</p>;
  }
  const m = p.metrics ?? {};
  const cells = Object.entries(m).filter(([, v]) => typeof v === "number" || v === null);
  return (
    <dl className="mt-1 flex flex-wrap gap-x-5 gap-y-1 text-xs">
      {cells.map(([k, v]) => (
        <div key={k} className="flex gap-1.5">
          <dt className="text-muted-foreground">{k}</dt>
          <dd className="font-mono tabular-nums">
            {v === null ? "Not available" : formatNumber(v as number)}
          </dd>
        </div>
      ))}
    </dl>
  );
}

// A finding's payload arrives as the JSON object the engine stored. A string
// is tolerated for fixtures that quote it.
function parse<T>(raw: unknown): T {
  if (typeof raw !== "string") {
    return (raw ?? {}) as T;
  }
  try {
    return JSON.parse(raw) as T;
  } catch {
    return {} as T;
  }
}
