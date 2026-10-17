import { type FormEvent, useState } from "react";
import { useAccounts } from "@/api/accounts";
import { useBrand, useBrands } from "@/api/brands";
import {
  useBrief,
  useBriefs,
  useCreateBrief,
  useResearchEvidence,
  useResolveCompetitor,
  useStartResearchRun,
} from "@/api/research";
import { useRun } from "@/api/runs";
import { useSession } from "@/api/session";
import type { BriefDetail, Finding, Item } from "@/api/types.gen";
import { RunStatus } from "@/components/run-status";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

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
        <h1 className="text-2xl font-semibold">Research</h1>
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
    <form onSubmit={onSubmit} className="space-y-2 rounded-lg border p-3 text-sm">
      <h2 className="font-medium">New brief</h2>
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
    </form>
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
          <h2 className="text-xl font-semibold">{d.brief.title}</h2>
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
          <h3 className="font-medium">Competitors ADX can't place</h3>
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
        <h3 className="font-medium">Sources</h3>
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
          <h3 className="font-medium">
            Your account{" "}
            <span className="font-normal text-muted-foreground">(observed, private)</span>
          </h3>
          {account.length === 0 && <p className="text-muted-foreground">No audit yet.</p>}
          {account.map((f) => (
            <AuditFinding key={f.id} f={f} onOpen={setEvidenceId} />
          ))}
        </section>
        <section className="space-y-2">
          <h3 className="font-medium">
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
        <h3 className="font-medium">
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

      {evidenceId !== null && <EvidencePanel id={evidenceId} onClose={() => setEvidenceId(null)} />}
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

function EvidencePanel({ id, onClose }: { id: number; onClose: () => void }) {
  const ev = useResearchEvidence(id);
  return (
    <section className="space-y-2 rounded-lg border p-3">
      <div className="flex items-baseline justify-between">
        <h3 className="font-medium">Evidence {id}</h3>
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
