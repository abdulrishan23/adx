import { type FormEvent, useState } from "react";
import type { BriefDetail, ComparativeReport, Finding, Hypothesis, Item } from "@/api/types.gen";
import { Form } from "@/components/form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAccounts } from "@/engine/api/accounts";
import { useBrand, useBrands } from "@/engine/api/brands";
import {
  useBrief,
  useBriefs,
  useCreateBrief,
  useCreateResearchSchedule,
  useGenerateResearchReport,
  useMarketEvents,
  useRecordMarketEvent,
  useResearchEvidence,
  useResearchReports,
  useResearchSchedules,
  useResolveCompetitor,
  useReviewHypothesis,
  useStartResearchRun,
  useUpdateResearchSchedule,
} from "@/engine/api/research";
import { useRun } from "@/engine/api/runs";
import { useSession } from "@/engine/api/session";
import { RunStatus } from "@/engine/components/run-status";

const itemState: Record<Item["state"], string> = {
  pending: "Waiting to run",
  needs_identity: "Who is this competitor?",
  fetched: "Read",
  failed: "Unavailable",
  refused: "Refused",
};

const itemKind: Record<Item["kind"], string> = {
  web_page: "Page",
  competitor_page: "Competitor page",
  keywords: "Keyword ideas",
  account_audit: "Own account audit",
};

type Rec = Record<string, unknown>;
// Payloads are JSON documents; the generated type calls them strings
// because Go sends them raw. Either way, read them as objects.
function rec(p: unknown): Rec {
  if (typeof p === "string") {
    try {
      const parsed: unknown = JSON.parse(p);
      return parsed && typeof parsed === "object" ? (parsed as Rec) : {};
    } catch {
      return {};
    }
  }
  return p && typeof p === "object" ? (p as Rec) : {};
}

// Research: a brief names pages, competitors, keyword seeds and whether to
// audit our own history; a run reads each once. Figures from our account
// and anything from the public web are shown apart; Keyword Planner numbers
// are estimates and are labelled so; a source that didn't answer shows as a
// gap, never as a number.
export function ResearchPage() {
  const briefs = useBriefs();
  const session = useSession();
  const [current, setCurrent] = useState<number | null>(null);
  const canEdit = session.data?.role === "marketer" || session.data?.role === "admin";

  return (
    <section className="grid gap-6 md:grid-cols-[18rem_1fr]">
      <aside className="space-y-3">
        <h1 className="text-3xl font-semibold tracking-tight">Research</h1>
        {briefs.isPending && <p className="text-sm text-muted-foreground">Loading…</p>}
        {briefs.error && <p className="text-sm text-destructive">{briefs.error.message}</p>}
        {briefs.data?.briefs.length === 0 && (
          <p className="text-sm text-muted-foreground">No briefs yet.</p>
        )}
        <ul className="space-y-1 text-sm">
          {briefs.data?.briefs.map((b) => (
            <li key={b.id}>
              <button
                type="button"
                className={`w-full rounded-md px-2 py-1 text-left ${b.id === current ? "bg-muted" : "hover:bg-muted"}`}
                onClick={() => setCurrent(b.id)}
              >
                {b.title}{" "}
                <span className="text-muted-foreground">
                  · {b.geography} · {b.language}
                </span>
              </button>
            </li>
          ))}
        </ul>
        {canEdit && <NewBrief onCreated={setCurrent} />}
        <MarketEvents canEdit={canEdit} />
      </aside>
      <div>
        {current === null ? (
          <p className="text-muted-foreground">Pick or create a brief.</p>
        ) : (
          <BriefView id={current} canEdit={canEdit} />
        )}
      </div>
    </section>
  );
}

function NewBrief({ onCreated }: { onCreated: (id: number) => void }) {
  const brands = useBrands();
  const accounts = useAccounts();
  const create = useCreateBrief();
  const [brandId, setBrandId] = useState(0);
  const brand = useBrand(brandId || null);
  const [form, setForm] = useState({
    projectId: 0,
    title: "",
    question: "",
    geography: "AE",
    language: "en",
    urls: "",
    competitors: "",
    seeds: "",
    googleAdsAccountId: 0,
    audit: true,
  });
  const own = brands.data?.brands.filter((b) => b.kind === "own") ?? [];
  const googleAccounts = accounts.data?.accounts.filter((a) => a.provider === "google_ads") ?? [];

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const competitors = form.competitors
      .split("\n")
      .map((l) => l.trim())
      .filter(Boolean)
      .map((l) => {
        const [name = "", rest = ""] = l.split("|");
        return {
          name: name.trim(),
          urls: rest
            .split(/\s+/)
            .map((u) => u.trim())
            .filter(Boolean),
        };
      });
    create.mutate(
      {
        brandId,
        projectId: form.projectId,
        title: form.title,
        question: form.question,
        geography: form.geography,
        language: form.language,
        urls: form.urls
          .split("\n")
          .map((u) => u.trim())
          .filter(Boolean),
        competitors,
        keywordSeeds: form.seeds
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        googleAdsAccountId: form.googleAdsAccountId,
        auditAccount: form.audit,
      },
      { onSuccess: (d) => onCreated(d.brief.id) },
    );
  }

  return (
    <Form onSubmit={onSubmit} className="space-y-2 rounded-lg border p-3 text-sm">
      <h2 className="font-semibold">New brief</h2>
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={brandId}
        onChange={(e) => {
          setBrandId(Number(e.target.value));
          setForm({ ...form, projectId: 0 });
        }}
      >
        <option value={0}>Brand…</option>
        {own.map((b) => (
          <option key={b.id} value={b.id}>
            {b.name}
          </option>
        ))}
      </select>
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={form.projectId}
        onChange={(e) => setForm({ ...form, projectId: Number(e.target.value) })}
        disabled={!brandId}
      >
        <option value={0}>Whole brand</option>
        {brand.data?.projects.map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
          </option>
        ))}
      </select>
      <Input
        placeholder="Title"
        value={form.title}
        onChange={(e) => setForm({ ...form, title: e.target.value })}
      />
      <Input
        placeholder="The question this answers"
        value={form.question}
        onChange={(e) => setForm({ ...form, question: e.target.value })}
      />
      <div className="flex gap-2">
        <Input
          placeholder="Country (AE)"
          value={form.geography}
          onChange={(e) => setForm({ ...form, geography: e.target.value })}
          className="max-w-28"
        />
        <select
          className="rounded-md border bg-background px-2 py-1"
          value={form.language}
          onChange={(e) => setForm({ ...form, language: e.target.value })}
        >
          <option value="en">English</option>
          <option value="ar">Arabic</option>
        </select>
      </div>
      <textarea
        className="w-full rounded-md border bg-background px-2 py-1"
        rows={2}
        placeholder="Pages to read, one URL per line"
        value={form.urls}
        onChange={(e) => setForm({ ...form, urls: e.target.value })}
      />
      <textarea
        className="w-full rounded-md border bg-background px-2 py-1"
        rows={2}
        placeholder={"Competitors, one per line: Name | https://page1 https://page2"}
        value={form.competitors}
        onChange={(e) => setForm({ ...form, competitors: e.target.value })}
      />
      <Input
        placeholder="Keyword seeds, comma separated"
        value={form.seeds}
        onChange={(e) => setForm({ ...form, seeds: e.target.value })}
      />
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={form.googleAdsAccountId}
        onChange={(e) => setForm({ ...form, googleAdsAccountId: Number(e.target.value) })}
      >
        <option value={0}>No Google Ads account (keyword ideas unavailable)</option>
        {googleAccounts.map((a) => (
          <option key={a.id} value={a.id}>
            {a.name ?? a.externalId}
          </option>
        ))}
      </select>
      <label className="flex items-center gap-2">
        <input
          type="checkbox"
          checked={form.audit}
          onChange={(e) => setForm({ ...form, audit: e.target.checked })}
        />
        Audit our own last 90 days
      </label>
      <Button type="submit" size="sm" disabled={create.isPending || !brandId}>
        Create brief
      </Button>
      {create.error && <p className="text-destructive">{create.error.message}</p>}
    </Form>
  );
}

function BriefView({ id, canEdit }: { id: number; canEdit: boolean }) {
  const brief = useBrief(id);
  const start = useStartResearchRun(id);
  const run = useRun(start.data?.run.id ?? "");
  const [evidenceId, setEvidenceId] = useState<number | null>(null);
  if (brief.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (brief.error) return <p className="text-destructive">{brief.error.message}</p>;
  const d = brief.data;
  const account = d.findings.filter((f) => f.scope === "account");
  const pages = d.findings.filter((f) => f.scope === "public" && f.kind === "page_text");
  const keywords = d.findings.filter((f) => f.kind === "keyword");
  const unresolved = d.competitors.filter((c) => c.state !== "resolved");

  return (
    <div className="space-y-6 text-sm">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight">{d.brief.title}</h2>
          <p className="text-muted-foreground">
            {d.brief.question || "No question written."} · {d.brief.geography} · {d.brief.language}{" "}
            · {d.runs} run{d.runs === 1 ? "" : "s"}
          </p>
        </div>
        {canEdit && (
          <div className="flex flex-col items-end gap-1">
            <Button size="sm" disabled={start.isPending} onClick={() => start.mutate()}>
              Run research
            </Button>
            {run.data && <RunStatus run={run.data} />}
            {start.error && <span className="text-destructive">{start.error.message}</span>}
          </div>
        )}
      </div>

      {unresolved.length > 0 && (
        <section className="space-y-2 rounded-lg border border-destructive/40 p-3">
          <h3 className="font-semibold">Competitors ADX can't place</h3>
          <p className="text-muted-foreground">
            A name is matched only when it is exactly a competitor brand. These wait for a person;
            their pages are not read until then.
          </p>
          {unresolved.map((c) => (
            <ResolveRow
              key={c.name}
              briefId={id}
              name={c.name}
              candidates={c.candidates}
              canEdit={canEdit}
            />
          ))}
        </section>
      )}

      <section className="space-y-2">
        <h3 className="font-semibold">Sources</h3>
        <table className="w-full text-left">
          <thead className="text-muted-foreground">
            <tr>
              <th className="py-1 pr-2 font-normal">Kind</th>
              <th className="py-1 pr-2 font-normal">Target</th>
              <th className="py-1 pr-2 font-normal">State</th>
              <th className="py-1 font-normal">Note</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {d.items.map((it) => (
              <tr key={it.id}>
                <td className="py-1 pr-2">
                  {itemKind[it.kind]}
                  {it.competitorName ? ` · ${it.competitorName}` : ""}
                </td>
                <td className="max-w-md truncate py-1 pr-2">
                  {it.target || <span className="text-muted-foreground">no page yet</span>}
                </td>
                <td
                  className={`py-1 pr-2 ${it.state === "failed" || it.state === "refused" ? "text-destructive" : it.state === "fetched" ? "" : "text-muted-foreground"}`}
                >
                  {itemState[it.state]}
                </td>
                <td className="py-1 text-muted-foreground">
                  {it.error}
                  {it.fetchedAt ? `read ${new Date(it.fetchedAt).toLocaleString()}` : ""}
                  {it.attempts > 1 ? ` · ${it.attempts} attempts` : ""}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <div className="grid gap-6 md:grid-cols-2">
        <section className="space-y-2">
          <h3 className="font-semibold">
            Your account{" "}
            <span className="font-normal text-muted-foreground">(observed, private)</span>
          </h3>
          {account.length === 0 && <p className="text-muted-foreground">No audit yet.</p>}
          {account.map((f) => (
            <AuditFinding key={f.id} f={f} onOpen={setEvidenceId} />
          ))}
        </section>
        <section className="space-y-2">
          <h3 className="font-semibold">
            Public web{" "}
            <span className="font-normal text-muted-foreground">
              (pages as read; nothing inferred)
            </span>
          </h3>
          {pages.length === 0 && <p className="text-muted-foreground">No pages read yet.</p>}
          {pages.map((f) => (
            <PageFinding key={f.id} f={f} detail={d} onOpen={setEvidenceId} />
          ))}
        </section>
      </div>

      <section className="space-y-2">
        <h3 className="font-semibold">
          Keyword ideas{" "}
          <span className="font-normal text-muted-foreground">
            (estimated by Google Keyword Planner, not observed)
          </span>
        </h3>
        {keywords.length === 0 && (
          <p className="text-muted-foreground">
            None. Keyword ideas need a Google Ads account on the brief and a run.
          </p>
        )}
        {keywords.length > 0 && (
          <table className="w-full text-left">
            <thead className="text-muted-foreground">
              <tr>
                <th className="py-1 pr-2 font-normal">Keyword</th>
                <th className="py-1 pr-2 font-normal">Est. monthly searches</th>
                <th className="py-1 pr-2 font-normal">Competition</th>
                <th className="py-1 font-normal">Top-of-page bid (est.)</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {keywords.map((f) => {
                const idea = rec(rec(f.payload).idea);
                const cur = String(rec(f.payload).currency ?? "");
                const bid = (v: unknown) =>
                  typeof v === "number" ? `${cur} ${(v / 100).toLocaleString()}` : "Not available";
                return (
                  <tr key={f.id}>
                    <td className="py-1 pr-2">{f.label}</td>
                    <td className="py-1 pr-2">
                      {typeof idea.avgMonthlySearches === "number"
                        ? idea.avgMonthlySearches.toLocaleString()
                        : "Not available"}
                    </td>
                    <td className="py-1 pr-2">{String(idea.competition || "Not available")}</td>
                    <td className="py-1">
                      {bid(idea.lowTopOfPageBidMinor)} to {bid(idea.highTopOfPageBidMinor)}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </section>

      <Schedules briefId={id} canEdit={canEdit} />
      <Reports briefId={id} canEdit={canEdit} onOpen={setEvidenceId} />

      {evidenceId !== null && <EvidencePanel id={evidenceId} onClose={() => setEvidenceId(null)} />}
    </div>
  );
}

// Market events are the only way a festival or economic explanation enters
// a report: a person records what happened, with a source. Nothing is
// derived from a calendar ADX doesn't have.
function MarketEvents({ canEdit }: { canEdit: boolean }) {
  const events = useMarketEvents();
  const record = useRecordMarketEvent();
  const [form, setForm] = useState({
    name: "",
    kind: "festival",
    from: "",
    to: "",
    sourceUrl: "",
    note: "",
  });
  return (
    <div className="space-y-2 rounded-lg border p-3 text-sm">
      <h2 className="font-semibold">Market events</h2>
      <p className="text-muted-foreground">
        Recorded by the team, with a source. Reports can only mention events recorded here, and only
        as hypotheses.
      </p>
      <ul className="space-y-1">
        {events.data?.events.map((e) => (
          <li key={e.evidenceId}>
            {e.name}{" "}
            <span className="text-muted-foreground">
              · {e.kind} · {e.from} to {e.to}
            </span>
          </li>
        ))}
        {events.data?.events.length === 0 && (
          <li className="text-muted-foreground">None recorded.</li>
        )}
      </ul>
      {canEdit && (
        <Form
          className="space-y-1"
          onSubmit={(e) => {
            e.preventDefault();
            record.mutate(form, {
              onSuccess: () =>
                setForm({ name: "", kind: "festival", from: "", to: "", sourceUrl: "", note: "" }),
            });
          }}
        >
          <Input
            placeholder="Event (e.g. Eid al-Fitr, Cityscape)"
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
          />
          <div className="flex gap-1">
            <select
              className="rounded-md border bg-background px-1"
              value={form.kind}
              onChange={(e) => setForm({ ...form, kind: e.target.value })}
            >
              {["festival", "economic", "regulatory", "competitor", "other"].map((k) => (
                <option key={k}>{k}</option>
              ))}
            </select>
            <Input
              type="date"
              value={form.from}
              onChange={(e) => setForm({ ...form, from: e.target.value })}
              aria-label="From"
            />
            <Input
              type="date"
              value={form.to}
              onChange={(e) => setForm({ ...form, to: e.target.value })}
              aria-label="To"
            />
          </div>
          <Input
            placeholder="Source URL"
            value={form.sourceUrl}
            onChange={(e) => setForm({ ...form, sourceUrl: e.target.value })}
          />
          <Button type="submit" size="xs" variant="outline" disabled={record.isPending}>
            Record
          </Button>
          {record.error && <p className="text-destructive">{record.error.message}</p>}
        </Form>
      )}
    </div>
  );
}

export function Schedules({ briefId, canEdit }: { briefId: number; canEdit: boolean }) {
  const schedules = useResearchSchedules();
  const create = useCreateResearchSchedule();
  const update = useUpdateResearchSchedule();
  const [cadence, setCadence] = useState<"weekly" | "monthly" | "quarterly">("monthly");
  const mine = schedules.data?.schedules.filter((s) => s.briefId === briefId) ?? [];
  return (
    <section className="space-y-2">
      <h3 className="font-semibold">Recurring</h3>
      {mine.length === 0 && (
        <p className="text-muted-foreground">
          Not scheduled. A schedule re-reads every source each period and writes a comparative
          report.
        </p>
      )}
      {mine.map((s) => (
        <div key={s.id} className="flex flex-wrap items-center gap-2">
          <span>
            {s.name} · {s.cadence} · {s.enabled ? "on" : "paused"}
          </span>
          {canEdit && (
            <Button
              size="xs"
              variant="outline"
              disabled={update.isPending}
              onClick={() =>
                update.mutate({
                  id: s.id,
                  input: { briefId, name: s.name, cadence: s.cadence, enabled: !s.enabled },
                })
              }
            >
              {s.enabled ? "Pause" : "Resume"}
            </Button>
          )}
        </div>
      ))}
      {canEdit && mine.length === 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <select
            className="rounded-md border bg-background px-2 py-1"
            value={cadence}
            onChange={(e) => setCadence(e.target.value as typeof cadence)}
          >
            <option value="weekly">Weekly</option>
            <option value="monthly">Monthly</option>
            <option value="quarterly">Quarterly</option>
          </select>
          <Button
            size="xs"
            variant="outline"
            disabled={create.isPending}
            onClick={() =>
              create.mutate({ briefId, name: `${cadence} comparison`, cadence, enabled: true })
            }
          >
            Schedule
          </Button>
          {create.error && <span className="text-destructive">{create.error.message}</span>}
        </div>
      )}
    </section>
  );
}

export function Reports({
  briefId,
  canEdit,
  onOpen,
}: {
  briefId: number;
  canEdit: boolean;
  onOpen: (id: number) => void;
}) {
  const reports = useResearchReports(briefId);
  const generate = useGenerateResearchReport(briefId);
  const [open, setOpen] = useState<number | null>(null);
  const list = reports.data?.reports ?? [];
  const shown = list.find((r) => r.id === open) ?? list[0];
  return (
    <section className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="font-semibold">Comparative reports</h3>
        {canEdit && (
          <Button
            size="xs"
            variant="outline"
            disabled={generate.isPending}
            onClick={() => generate.mutate({ from: "", to: "" })}
          >
            Write report for last month
          </Button>
        )}
      </div>
      {generate.error && <p className="text-destructive">{generate.error.message}</p>}
      {list.length === 0 && <p className="text-muted-foreground">No reports yet.</p>}
      {list.length > 1 && (
        <div className="flex flex-wrap gap-1">
          {list.map((r) => (
            <Button
              key={r.id}
              size="xs"
              variant={shown?.id === r.id ? "default" : "outline"}
              onClick={() => setOpen(r.id)}
            >
              {r.periodStart} to {r.periodEnd}
            </Button>
          ))}
        </div>
      )}
      {shown && <ReportView report={shown} canEdit={canEdit} onOpen={onOpen} />}
    </section>
  );
}

// Observed on the left, hypothetical cause on the right. Every hypothesis
// cites evidence and stays a hypothesis; the team's review is recorded
// beside it, not folded into it.
function ReportView({
  report,
  canEdit,
  onOpen,
}: {
  report: ComparativeReport;
  canEdit: boolean;
  onOpen: (id: number) => void;
}) {
  const d = report.document;
  const reviews = new Map(report.reviews.map((r) => [r.key, r]));
  const pct = (v: number | null | undefined) =>
    typeof v === "number" ? `${v >= 0 ? "+" : ""}${Math.round(v * 100)}%` : "Not available";
  const money = (minor: number | null | undefined, cur: string) =>
    typeof minor === "number" ? `${cur} ${(minor / 100).toLocaleString()}` : "Not available";
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <p className="text-muted-foreground">
        {d.period.from} to {d.period.to} against {d.previous.from} to {d.previous.to} · written{" "}
        {new Date(report.generatedAt).toLocaleString()}
        {report.previousReportId && ` · follows report ${report.previousReportId}`}
      </p>
      {report.limited ? (
        <p className="rounded-md border border-destructive/40 p-2 text-destructive">
          Limited audit: {report.limitedReason}.
        </p>
      ) : (
        d.account && (
          <div>
            <h4 className="font-medium">
              Your account{" "}
              <span className="font-normal text-muted-foreground">
                (observed · formula {d.account.formulaVersion})
              </span>
            </h4>
            <p>
              Spend{" "}
              {money(
                d.account.summary.metrics.spend.minor,
                d.account.summary.metrics.spend.currency,
              )}{" "}
              ({pct(d.account.summary.changes?.spend?.relative)}) · Leads{" "}
              {d.account.summary.metrics.leads} ({pct(d.account.summary.changes?.leads?.relative)})
              {" · "}
              <button
                type="button"
                className="underline text-muted-foreground"
                onClick={() => onOpen(d.account?.evidenceId ?? 0)}
              >
                source
              </button>
            </p>
            {d.account.freshness.partial && (
              <p className="text-destructive">Figures are partial for this period.</p>
            )}
          </div>
        )
      )}
      {d.competitors.length > 0 && (
        <div>
          <h4 className="font-medium">
            Competitor pages <span className="font-normal text-muted-foreground">(observed)</span>
          </h4>
          {d.competitors.map((c) =>
            c.pages.map((p) => (
              <p key={`${c.brandId}-${p.itemId}`}>
                {c.name}:{" "}
                <a href={p.url} target="_blank" rel="noreferrer noopener" className="underline">
                  {p.title || p.url}
                </a>{" "}
                {p.changed ? (
                  <span className="text-destructive">changed since last read</span>
                ) : (
                  <span className="text-muted-foreground">
                    {p.previousEvidenceId ? "unchanged" : "first read"}
                  </span>
                )}
                {" · "}
                <button
                  type="button"
                  className="underline text-muted-foreground"
                  onClick={() => onOpen(p.evidenceId)}
                >
                  now
                </button>
                {p.previousEvidenceId && (
                  <>
                    {" "}
                    ·{" "}
                    <button
                      type="button"
                      className="underline text-muted-foreground"
                      onClick={() => onOpen(p.previousEvidenceId as number)}
                    >
                      before
                    </button>
                  </>
                )}
              </p>
            )),
          )}
        </div>
      )}
      {d.keywords.length > 0 && (
        <div>
          <h4 className="font-medium">
            Keyword estimates{" "}
            <span className="font-normal text-muted-foreground">
              (Google's estimates, not observations)
            </span>
          </h4>
          {d.keywords.map((k) => (
            <p key={k.text}>
              {k.text}: {k.before ?? "—"} → {k.after ?? "—"}
              {typeof k.changePct === "number"
                ? ` (${k.changePct >= 0 ? "+" : ""}${k.changePct}%)`
                : ""}
            </p>
          ))}
        </div>
      )}
      {d.events.length > 0 && (
        <div>
          <h4 className="font-medium">Recorded events in the period</h4>
          {d.events.map((e) => (
            <p key={e.evidenceId}>
              {e.name} · {e.kind} · {e.from} to {e.to}
            </p>
          ))}
        </div>
      )}
      <div>
        <h4 className="font-medium">
          Possible explanations{" "}
          <span className="font-normal text-muted-foreground">
            (hypotheses; none is a demonstrated cause)
          </span>
        </h4>
        {d.hypotheses.length === 0 && (
          <p className="text-muted-foreground">
            Nothing to explain or nothing recorded that could explain it.
          </p>
        )}
        {d.hypotheses.map((h) => (
          <HypothesisRow
            key={h.key}
            reportId={report.id}
            h={h}
            review={reviews.get(h.key)}
            canEdit={canEdit}
            onOpen={onOpen}
          />
        ))}
      </div>
      {d.prior && d.prior.hypotheses.length > 0 && (
        <div>
          <h4 className="font-medium">
            From the previous report{" "}
            <span className="font-normal text-muted-foreground">
              ({d.prior.periodStart} to {d.prior.periodEnd})
            </span>
          </h4>
          {d.prior.hypotheses.map((p) => (
            <p key={p.key} className="text-muted-foreground">
              {p.text} — {p.status ? `${p.status}${p.note ? `: ${p.note}` : ""}` : "not reviewed"}
            </p>
          ))}
        </div>
      )}
    </div>
  );
}

function HypothesisRow({
  reportId,
  h,
  review,
  canEdit,
  onOpen,
}: {
  reportId: number;
  h: Hypothesis;
  review?: { status: string; note: string };
  canEdit: boolean;
  onOpen: (id: number) => void;
}) {
  const act = useReviewHypothesis();
  const [note, setNote] = useState("");
  return (
    <div className="grid gap-2 border-t py-2 md:grid-cols-2">
      <div>
        <span className="text-muted-foreground">Observed:</span> {h.observed}
      </div>
      <div>
        <span className="text-muted-foreground">Hypothetical cause:</span> {h.hypothetical}{" "}
        <span className="text-muted-foreground">
          · evidence{" "}
          {h.evidenceRefs.map((id, i) => (
            <button key={id} type="button" className="underline" onClick={() => onOpen(id)}>
              {i > 0 ? `, ${id}` : id}
            </button>
          ))}
        </span>
        <div className="mt-1 flex flex-wrap items-center gap-1">
          {review ? (
            <span
              className={review.status === "rejected" ? "text-muted-foreground" : "font-medium"}
            >
              Team: {review.status}
              {review.note ? ` — ${review.note}` : ""}
            </span>
          ) : (
            <span className="text-muted-foreground">Not reviewed.</span>
          )}
          {canEdit && (
            <>
              <Input
                className="h-7 max-w-xs"
                placeholder="Note or corrected explanation"
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
              <Button
                size="xs"
                variant="outline"
                disabled={act.isPending}
                onClick={() =>
                  act.mutate({ reportId, input: { key: h.key, status: "accepted", note } })
                }
              >
                Plausible
              </Button>
              <Button
                size="xs"
                variant="outline"
                disabled={act.isPending}
                onClick={() =>
                  act.mutate({ reportId, input: { key: h.key, status: "rejected", note } })
                }
              >
                Reject
              </Button>
              <Button
                size="xs"
                variant="outline"
                disabled={act.isPending || !note}
                onClick={() =>
                  act.mutate({ reportId, input: { key: h.key, status: "corrected", note } })
                }
              >
                Correct
              </Button>
            </>
          )}
          {act.error && <span className="text-destructive">{act.error.message}</span>}
        </div>
      </div>
    </div>
  );
}

function ResolveRow({
  briefId,
  name,
  candidates,
  canEdit,
}: {
  briefId: number;
  name: string;
  candidates: { id: number; name: string }[];
  canEdit: boolean;
}) {
  const brands = useBrands();
  const resolve = useResolveCompetitor(briefId);
  const [brandId, setBrandId] = useState(0);
  const competitors = brands.data?.brands.filter((b) => b.kind === "competitor") ?? [];
  return (
    <div className="flex flex-wrap items-center gap-2 border-t pt-2">
      <span className="font-medium">{name}</span>
      <span className="text-muted-foreground">
        {candidates.length > 0
          ? `could be: ${candidates.map((c) => c.name).join(", ")}`
          : "no competitor brand matches; add one under Brands first"}
      </span>
      {canEdit && competitors.length > 0 && (
        <>
          <select
            className="rounded-md border bg-background px-2 py-1"
            value={brandId}
            onChange={(e) => setBrandId(Number(e.target.value))}
          >
            <option value={0}>Pick…</option>
            {competitors.map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </select>
          <Button
            size="xs"
            variant="outline"
            disabled={!brandId || resolve.isPending}
            onClick={() => resolve.mutate({ name, brandId })}
          >
            This one
          </Button>
        </>
      )}
      {resolve.error && <span className="text-destructive">{resolve.error.message}</span>}
    </div>
  );
}

function AuditFinding({ f, onOpen }: { f: Finding; onOpen: (id: number) => void }) {
  const p = rec(f.payload);
  const m = rec(p.metrics);
  const spend = rec(m.spend);
  return (
    <div className="rounded-lg border p-2">
      <div className="flex items-baseline justify-between gap-2">
        <span className="font-medium">{f.label}</span>
        {f.evidenceId && (
          <button
            type="button"
            className="text-muted-foreground underline"
            onClick={() => onOpen(f.evidenceId as number)}
          >
            source
          </button>
        )}
      </div>
      {f.kind === "audit_metric" ? (
        <p className="text-muted-foreground">
          Spend{" "}
          {typeof spend.minor === "number"
            ? `${String(spend.currency ?? "")} ${(spend.minor / 100).toLocaleString()}`
            : "Not available"}{" "}
          · Leads {typeof m.leads === "number" ? m.leads : "Not available"} · formula{" "}
          {String(p.formulaVersion ?? "")}
        </p>
      ) : (
        <p className="text-muted-foreground">{String(p.note ?? p.message ?? "")}</p>
      )}
    </div>
  );
}

function PageFinding({
  f,
  detail,
  onOpen,
}: {
  f: Finding;
  detail: BriefDetail;
  onOpen: (id: number) => void;
}) {
  const p = rec(f.payload);
  const item = detail.items.find((i) => i.id === f.itemId);
  return (
    <div className="rounded-lg border p-2">
      <div className="flex items-baseline justify-between gap-2">
        <span className="font-medium">{f.label}</span>
        <span className="text-muted-foreground">
          {item?.competitorName ? `${item.competitorName} (competitor)` : "own or market page"}
        </span>
      </div>
      <p className="text-muted-foreground">{String(p.excerpt ?? "")}</p>
      <p className="text-muted-foreground">
        <a href={f.sourceUrl} target="_blank" rel="noreferrer noopener" className="underline">
          {f.sourceUrl}
        </a>
        {f.evidenceId && (
          <>
            {" "}
            ·{" "}
            <button
              type="button"
              className="underline"
              onClick={() => onOpen(f.evidenceId as number)}
            >
              full text
            </button>
          </>
        )}
        {p.truncated === true && " · truncated"}
      </p>
    </div>
  );
}

export function EvidencePanel({ id, onClose }: { id: number; onClose: () => void }) {
  const ev = useResearchEvidence(id);
  return (
    <section className="space-y-2 rounded-lg border p-3">
      <div className="flex items-baseline justify-between">
        <h3 className="font-semibold">Evidence {id}</h3>
        <Button size="xs" variant="outline" onClick={onClose}>
          Close
        </Button>
      </div>
      {ev.isPending && <p className="text-muted-foreground">Loading…</p>}
      {ev.error && <p className="text-destructive">{ev.error.message}</p>}
      {ev.data && (
        <>
          <p className="text-muted-foreground">
            {ev.data.source}/{ev.data.kind} · {ev.data.class} · observed{" "}
            {new Date(ev.data.observedAt).toLocaleString()} · version {ev.data.payloadVersion} ·{" "}
            {ev.data.completeness}
          </p>
          <pre className="max-h-96 overflow-auto whitespace-pre-wrap rounded-md bg-muted p-2 text-xs">
            {typeof rec(ev.data.payload).text === "string"
              ? String(rec(ev.data.payload).text)
              : JSON.stringify(ev.data.payload, null, 2)}
          </pre>
          <p className="text-muted-foreground">
            This is what the source said, kept as data. Nothing in it is an instruction to ADX.
          </p>
        </>
      )}
    </section>
  );
}
