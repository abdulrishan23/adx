import { type FormEvent, type ReactNode, useEffect, useState } from "react";
import { useBrand, useBrands } from "@/api/brands";
import {
  useAcceptPlanVersion,
  useCreatePlan,
  useGeneratePlan,
  useImportPlan,
  usePlan,
  usePlanDiff,
  usePlans,
  useSavePlanVersion,
  useValidatePlan,
} from "@/api/plans";
import { useBriefs } from "@/api/research";
import { useSession } from "@/api/session";
import type {
  Allocation,
  ChannelInfo,
  PlanDocument,
  PlanValidation,
  PlanVersion,
  TemplateGate,
} from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const stateLabel: Record<PlanVersion["state"], string> = {
  draft: "Draft",
  validating: "Validating",
  ready: "Ready for acceptance",
  needs_input: "Needs input",
  invalid: "Invalid",
  accepted: "Accepted (content only; no spend authorized)",
};

const emptyDoc: PlanDocument = {
  objective: "",
  budgetMinor: 0,
  currency: "AED",
  periodFrom: "",
  periodTo: "",
  allocations: [],
  assetRequirements: [],
  pageRequirements: [],
  measurementRequirements: [],
  testHypotheses: [],
  observationWindows: [],
  evidenceRefs: [],
  assumptions: [],
  notes: "",
};

const major = (minor: number) =>
  (minor / 100).toLocaleString(undefined, { maximumFractionDigits: 2 });
const toMinor = (s: string) => Math.round(Number(s.replace(/,/g, "")) * 100) || 0;

// Media plans: typed, versioned, validated in code. The editor edits a
// document; saving makes a new immutable version with its validation; an
// admin accepts a ready version, which says the content is right and
// nothing about spending. Export is generic (JSON, CSV) until Reportage's
// Excel template arrives; that gate is shown, not hidden.
export function PlansPage() {
  const plans = usePlans();
  const session = useSession();
  const [current, setCurrent] = useState<number | null>(null);
  const canEdit = session.data?.role === "marketer" || session.data?.role === "admin";

  return (
    <section className="grid gap-6 md:grid-cols-[16rem_1fr]">
      <aside className="space-y-3">
        <h1 className="text-3xl font-semibold tracking-tight">Plans</h1>
        {plans.isPending && <p className="text-sm text-muted-foreground">Loading…</p>}
        {plans.error && <p className="text-sm text-destructive">{plans.error.message}</p>}
        {plans.data?.plans.length === 0 && (
          <p className="text-sm text-muted-foreground">No plans yet.</p>
        )}
        <ul className="space-y-1 text-sm">
          {plans.data?.plans.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                className={`w-full rounded-md px-2 py-1 text-left ${p.id === current ? "bg-muted" : "hover:bg-muted"}`}
                onClick={() => setCurrent(p.id)}
              >
                {p.title}{" "}
                <span className="text-muted-foreground">
                  · v{p.latestVersion} · {p.latestState}
                  {p.accepted ? " · has accepted version" : ""}
                </span>
              </button>
            </li>
          ))}
        </ul>
        {canEdit && <NewPlan onCreated={setCurrent} />}
        {plans.data && <TemplateNote gate={plans.data.templates} />}
      </aside>
      <div>
        {current === null ? (
          <p className="text-muted-foreground">Pick or create a plan.</p>
        ) : (
          plans.data && (
            <PlanView
              id={current}
              channels={plans.data.channels}
              canEdit={canEdit}
              isAdmin={session.data?.role === "admin"}
            />
          )
        )}
      </div>
    </section>
  );
}

function TemplateNote({ gate }: { gate: TemplateGate }) {
  return (
    <div className="rounded-lg border p-3 text-sm">
      <h2 className="font-semibold">Export formats</h2>
      <p className="text-muted-foreground">Generic: {gate.generic.join(", ")}.</p>
      {gate.exact.available ? (
        <p className="text-muted-foreground">Exact template: available ({gate.exact.reason}).</p>
      ) : (
        <p className="text-destructive">
          Exact Excel template: blocked. {gate.exact.reason}. Unblocked by: {gate.exact.unblocks}.
        </p>
      )}
    </div>
  );
}

function NewPlan({ onCreated }: { onCreated: (id: number) => void }) {
  const brands = useBrands();
  const create = useCreatePlan();
  const [brandId, setBrandId] = useState(0);
  const brand = useBrand(brandId || null);
  const [form, setForm] = useState({
    projectId: 0,
    title: "",
    objective: "",
    budget: "",
    currency: "AED",
    from: "",
    to: "",
  });
  const own = brands.data?.brands.filter((b) => b.kind === "own") ?? [];
  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    create.mutate(
      {
        brandId,
        projectId: form.projectId,
        title: form.title,
        note: "",
        document: {
          ...emptyDoc,
          objective: form.objective,
          budgetMinor: toMinor(form.budget),
          currency: form.currency,
          periodFrom: form.from,
          periodTo: form.to,
        },
      },
      { onSuccess: (d) => onCreated(d.plan.id) },
    );
  }
  return (
    <form onSubmit={onSubmit} className="space-y-2 rounded-lg border p-3 text-sm">
      <h2 className="font-semibold">New plan</h2>
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
        placeholder="Objective"
        value={form.objective}
        onChange={(e) => setForm({ ...form, objective: e.target.value })}
      />
      <div className="flex gap-2">
        <Input
          placeholder="Budget"
          value={form.budget}
          onChange={(e) => setForm({ ...form, budget: e.target.value })}
        />
        <Input
          value={form.currency}
          onChange={(e) => setForm({ ...form, currency: e.target.value })}
          className="max-w-20"
          aria-label="Currency"
        />
      </div>
      <div className="flex gap-2">
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
      <Button type="submit" size="sm" disabled={create.isPending || !brandId}>
        Create (version 1)
      </Button>
      {create.error && <p className="text-destructive">{create.error.message}</p>}
    </form>
  );
}

function PlanView({
  id,
  channels,
  canEdit,
  isAdmin,
}: {
  id: number;
  channels: ChannelInfo[];
  canEdit: boolean;
  isAdmin: boolean;
}) {
  const plan = usePlan(id);
  const [version, setVersion] = useState<number | null>(null);
  const [diffFrom, setDiffFrom] = useState<number | null>(null);
  if (plan.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (plan.error) return <p className="text-destructive">{plan.error.message}</p>;
  const d = plan.data;
  const latest = d.versions[d.versions.length - 1];
  const shown = d.versions.find((v) => v.version === version) ?? latest;
  if (!shown) return null;
  const supersededAccepted = d.versions.some(
    (v) => v.state === "accepted" && v.version < (latest?.version ?? 0),
  );
  return (
    <div className="space-y-4 text-sm">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="text-2xl font-semibold tracking-tight">{d.plan.title}</h2>
        <div className="flex flex-wrap gap-1">
          {d.versions.map((v) => (
            <Button
              key={v.id}
              size="xs"
              variant={v.version === shown.version ? "default" : "outline"}
              onClick={() => setVersion(v.version)}
            >
              v{v.version}
              {v.state === "accepted" ? " ✓" : ""}
            </Button>
          ))}
        </div>
      </div>
      {supersededAccepted && (
        <p className="rounded-lg border border-destructive/40 p-2 text-destructive">
          An accepted version is older than the latest. Its acceptance stands for its own content;
          anything built from it has to be re-checked against the newer version.
        </p>
      )}
      {canEdit && (
        <GeneratePanel planId={id} brandId={d.plan.brandId} onGenerated={(v) => setVersion(v)} />
      )}
      {d.versions.length > 1 && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-muted-foreground">Compare v{shown.version} with</span>
          <select
            className="rounded-md border bg-background px-2 py-1"
            value={diffFrom ?? ""}
            onChange={(e) => setDiffFrom(e.target.value === "" ? null : Number(e.target.value))}
          >
            <option value="">none</option>
            {d.versions
              .filter((v) => v.version !== shown.version)
              .map((v) => (
                <option key={v.id} value={v.version}>
                  v{v.version}
                </option>
              ))}
          </select>
        </div>
      )}
      {diffFrom !== null && diffFrom !== shown.version && (
        <DiffView planId={id} from={diffFrom} to={shown.version} />
      )}
      <VersionEditor
        key={shown.id}
        planId={id}
        brandId={d.plan.brandId}
        version={shown}
        channels={channels}
        canEdit={canEdit}
        isAdmin={isAdmin}
      />
    </div>
  );
}

// The model proposes the next version from research the team names; code
// pins the budget, drops uncited evidence and uncited costs, and validates.
// The notes say what was corrected. Nothing generated is accepted.
function GeneratePanel({
  planId,
  brandId,
  onGenerated,
}: {
  planId: number;
  brandId: number;
  onGenerated: (version: number) => void;
}) {
  const briefs = useBriefs();
  const generate = useGeneratePlan(planId);
  const [selected, setSelected] = useState<number[]>([]);
  const [instructions, setInstructions] = useState("");
  const mine = briefs.data?.briefs.filter((b) => b.brandId === brandId) ?? [];
  return (
    <details className="rounded-lg border p-3">
      <summary className="cursor-pointer font-medium">Draft the next version from research</summary>
      <div className="mt-2 space-y-2">
        <p className="text-muted-foreground">
          The model distributes the plan's budget across channels ADX can deploy and cites only the
          research, claims and offers listed. Figures it can't source are recorded as unavailable.
          The result is a new version with its validation; a person still accepts it.
        </p>
        {mine.length === 0 && (
          <p className="text-muted-foreground">
            No research briefs for this brand yet; the model will see approved claims and offers
            only.
          </p>
        )}
        {mine.map((b) => (
          <label key={b.id} className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={selected.includes(b.id)}
              onChange={(e) =>
                setSelected(
                  e.target.checked ? [...selected, b.id] : selected.filter((x) => x !== b.id),
                )
              }
            />
            {b.title}{" "}
            <span className="text-muted-foreground">
              · {b.geography} · {b.language}
            </span>
          </label>
        ))}
        <Input
          placeholder="Instructions for the model (optional)"
          value={instructions}
          onChange={(e) => setInstructions(e.target.value)}
        />
        <Button
          size="sm"
          disabled={generate.isPending}
          onClick={() =>
            generate.mutate(
              { briefIds: selected, instructions },
              { onSuccess: (g) => onGenerated(g.version.version) },
            )
          }
        >
          {generate.isPending ? "Asking the model…" : "Propose next version"}
        </Button>
        {generate.error && <p className="text-destructive">{generate.error.message}</p>}
        {generate.data && (
          <div className="rounded-md border p-2">
            <p>
              Saved as version {generate.data.version.version} ({generate.data.version.state}) ·{" "}
              {generate.data.model} ·{" "}
              {generate.data.usage.inputTokens + generate.data.usage.outputTokens} tokens
            </p>
            {generate.data.notes.length > 0 && (
              <ul className="mt-1 text-muted-foreground">
                {generate.data.notes.map((n) => (
                  <li key={`${n.code}:${n.message}`}>Corrected: {n.message}</li>
                ))}
              </ul>
            )}
          </div>
        )}
      </div>
    </details>
  );
}

function DiffView({ planId, from, to }: { planId: number; from: number; to: number }) {
  const diff = usePlanDiff(planId, from, to);
  if (diff.isPending) return <p className="text-muted-foreground">Comparing…</p>;
  if (diff.error) return <p className="text-destructive">{diff.error.message}</p>;
  const d = diff.data;
  const empty = d.fields.length + d.allocations.length + d.sections.length === 0;
  return (
    <section className="space-y-1 rounded-lg border p-3">
      <h3 className="font-semibold">
        Changes from v{d.from} to v{d.to}
      </h3>
      {empty && <p className="text-muted-foreground">No differences.</p>}
      {d.invalidates && <p className="text-destructive">{d.invalidates.reason}</p>}
      {d.fields.map((f) => (
        <p key={f.path}>
          <span className="font-medium">{f.path}</span>: {f.before || "—"} → {f.after || "—"}
        </p>
      ))}
      {d.allocations.map((a) => (
        <p key={`${a.channel}|${a.objective}`}>
          <span className="font-medium">{a.channel}</span> ({a.objective}): {a.change}
          {a.beforeMinor !== null && ` ${major(a.beforeMinor)}`}
          {a.beforeMinor !== null && a.afterMinor !== null && " →"}
          {a.afterMinor !== null && ` ${major(a.afterMinor)}`}
        </p>
      ))}
      {d.sections.map((s) => (
        <p key={s.path} className="text-muted-foreground">
          {s.path}: {s.before} → {s.after} rows
        </p>
      ))}
    </section>
  );
}

function VersionEditor({
  planId,
  brandId,
  version,
  channels,
  canEdit,
  isAdmin,
}: {
  planId: number;
  brandId: number;
  version: PlanVersion;
  channels: ChannelInfo[];
  canEdit: boolean;
  isAdmin: boolean;
}) {
  const [doc, setDoc] = useState<PlanDocument>(version.document);
  const [dirty, setDirty] = useState(false);
  const save = useSavePlanVersion(planId);
  const accept = useAcceptPlanVersion(planId);
  const validate = useValidatePlan();
  const importPlan = useImportPlan(planId);
  const validation: PlanValidation = dirty && validate.data ? validate.data : version.validation;
  const allocated = doc.allocations.reduce((s, a) => s + a.budgetMinor, 0);
  const runValidation = validate.mutate;

  // Live validation while editing, so blockers show before saving.
  useEffect(() => {
    if (!dirty) return;
    const t = setTimeout(() => runValidation({ brandId, document: doc }), 400);
    return () => clearTimeout(t);
  }, [doc, dirty, brandId, runValidation]);

  function update(next: PlanDocument) {
    setDoc(next);
    setDirty(true);
  }
  const editable = canEdit;

  return (
    <div className="space-y-4">
      <p className="text-muted-foreground">
        Version {version.version} · {stateLabel[version.state]} · {version.source}
        {version.note ? ` · ${version.note}` : ""}
        {version.acceptance &&
          ` · accepted by user ${version.acceptance.acceptedBy} on ${new Date(version.acceptance.acceptedAt).toLocaleDateString()}`}
        {version.executionAuthorization === null && " · no spend authorized"}
      </p>

      <section className="rounded-lg border p-3">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h3 className="font-semibold">Validation</h3>
          <span
            className={allocated === doc.budgetMinor ? "text-muted-foreground" : "text-destructive"}
          >
            Allocated {doc.currency} {major(allocated)} of {doc.currency} {major(doc.budgetMinor)}
          </span>
        </div>
        {validation.blocking.length === 0 && validation.warnings.length === 0 && (
          <p className="text-muted-foreground">No issues.</p>
        )}
        <ul className="space-y-1">
          {validation.blocking.map((i) => (
            <li key={`${i.code}:${i.path}`} className="text-destructive">
              Blocks: {i.message} <span className="text-muted-foreground">({i.path})</span>
            </li>
          ))}
          {validation.warnings.map((i) => (
            <li key={`${i.code}:${i.path}`} className="text-muted-foreground">
              Note: {i.message}
            </li>
          ))}
        </ul>
      </section>

      <section className="space-y-2">
        <h3 className="font-semibold">Plan</h3>
        <div className="grid gap-2 md:grid-cols-2">
          <Input
            placeholder="Objective"
            value={doc.objective}
            onChange={(e) => update({ ...doc, objective: e.target.value })}
            disabled={!editable}
          />
          <div className="flex gap-2">
            <Input
              placeholder="Budget"
              value={major(doc.budgetMinor)}
              onChange={(e) => update({ ...doc, budgetMinor: toMinor(e.target.value) })}
              disabled={!editable}
              aria-label="Budget"
            />
            <Input
              value={doc.currency}
              onChange={(e) => update({ ...doc, currency: e.target.value })}
              className="max-w-20"
              disabled={!editable}
              aria-label="Currency"
            />
          </div>
          <Input
            type="date"
            value={doc.periodFrom}
            onChange={(e) => update({ ...doc, periodFrom: e.target.value })}
            disabled={!editable}
            aria-label="From"
          />
          <Input
            type="date"
            value={doc.periodTo}
            onChange={(e) => update({ ...doc, periodTo: e.target.value })}
            disabled={!editable}
            aria-label="To"
          />
        </div>
      </section>

      <section className="space-y-2">
        <h3 className="font-semibold">Allocations</h3>
        <table className="w-full text-left">
          <thead className="text-muted-foreground">
            <tr>
              <th className="py-1 pr-2 font-normal">Channel</th>
              <th className="py-1 pr-2 font-normal">Objective</th>
              <th className="py-1 pr-2 font-normal">Budget</th>
              <th className="py-1 pr-2 font-normal">Geo</th>
              <th className="py-1 pr-2 font-normal">Expected CPL</th>
              <th className="py-1 font-normal" />
            </tr>
          </thead>
          <tbody className="divide-y">
            {doc.allocations.map((a, i) => (
              <AllocationRow
                // biome-ignore lint/suspicious/noArrayIndexKey: allocation rows have no identity besides their position
                key={`${i}-${a.channel}`}
                a={a}
                channels={channels}
                editable={editable}
                onChange={(next) =>
                  update({
                    ...doc,
                    allocations: doc.allocations.map((x, j) => (j === i ? next : x)),
                  })
                }
                onRemove={() =>
                  update({ ...doc, allocations: doc.allocations.filter((_, j) => j !== i) })
                }
              />
            ))}
          </tbody>
        </table>
        {editable && (
          <Button
            size="xs"
            variant="outline"
            onClick={() =>
              update({
                ...doc,
                allocations: [
                  ...doc.allocations,
                  {
                    channel: "",
                    objective: "leads",
                    budgetMinor: 0,
                    geography: "AE",
                    audience: "",
                    projectId: null,
                    expectedCplMinor: null,
                    note: "",
                  },
                ],
              })
            }
          >
            Add line
          </Button>
        )}
      </section>

      <div className="grid gap-4 md:grid-cols-3">
        <ListEditor
          title="Assets"
          items={doc.assetRequirements}
          editable={editable}
          render={(r) => `${r.count} × ${r.format} ${r.language}`}
          blank={{ format: "9:16", language: "en", count: 1, offerId: null, note: "" }}
          fields={(r, set) => (
            <>
              <select
                className="rounded-md border bg-background px-1"
                value={r.format}
                onChange={(e) => set({ ...r, format: e.target.value as typeof r.format })}
              >
                {["9:16", "4:5", "1:1", "1.91:1"].map((f) => (
                  <option key={f}>{f}</option>
                ))}
              </select>
              <select
                className="rounded-md border bg-background px-1"
                value={r.language}
                onChange={(e) => set({ ...r, language: e.target.value as typeof r.language })}
              >
                <option value="en">en</option>
                <option value="ar">ar</option>
              </select>
              <Input
                type="number"
                className="h-7 max-w-16"
                value={r.count}
                onChange={(e) => set({ ...r, count: Number(e.target.value) })}
                aria-label="Count"
              />
            </>
          )}
          onChange={(items) => update({ ...doc, assetRequirements: items })}
        />
        <ListEditor
          title="Pages"
          items={doc.pageRequirements}
          editable={editable}
          render={(r) => `${r.kind} (${r.language}) ${r.url}`}
          blank={{ kind: "landing", language: "en", url: "", formPath: "", note: "" }}
          fields={(r, set) => (
            <>
              <select
                className="rounded-md border bg-background px-1"
                value={r.kind}
                onChange={(e) => set({ ...r, kind: e.target.value })}
              >
                <option value="landing">landing</option>
                <option value="thank_you">thank_you</option>
              </select>
              <select
                className="rounded-md border bg-background px-1"
                value={r.language}
                onChange={(e) => set({ ...r, language: e.target.value })}
              >
                <option value="en">en</option>
                <option value="ar">ar</option>
              </select>
              <Input
                className="h-7"
                placeholder="URL"
                value={r.url}
                onChange={(e) => set({ ...r, url: e.target.value })}
              />
            </>
          )}
          onChange={(items) => update({ ...doc, pageRequirements: items })}
        />
        <ListEditor
          title="Measurement"
          items={doc.measurementRequirements}
          editable={editable}
          render={(r) => `${r.kind}: ${r.name}${r.required ? " (required)" : ""}`}
          blank={{
            kind: "conversion_action",
            platform: "google_ads",
            name: "",
            required: true,
            note: "",
          }}
          fields={(r, set) => (
            <>
              <select
                className="rounded-md border bg-background px-1"
                value={r.kind}
                onChange={(e) => set({ ...r, kind: e.target.value })}
              >
                {[
                  "conversion_action",
                  "pixel_event",
                  "utm_scheme",
                  "crm_stage",
                  "call_tracking",
                ].map((k) => (
                  <option key={k}>{k}</option>
                ))}
              </select>
              <Input
                className="h-7"
                placeholder="Name"
                value={r.name}
                onChange={(e) => set({ ...r, name: e.target.value })}
              />
            </>
          )}
          onChange={(items) => update({ ...doc, measurementRequirements: items })}
        />
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        <ListEditor
          title="Test hypotheses"
          items={doc.testHypotheses}
          editable={editable}
          render={(r) => `${r.hypothesis} → ${r.metric} over ${r.windowDays}d`}
          blank={{ hypothesis: "", metric: "cost_per_lead", windowDays: 14 }}
          fields={(r, set) => (
            <>
              <Input
                className="h-7"
                placeholder="Hypothesis"
                value={r.hypothesis}
                onChange={(e) => set({ ...r, hypothesis: e.target.value })}
              />
              <Input
                className="h-7 max-w-32"
                placeholder="Metric"
                value={r.metric}
                onChange={(e) => set({ ...r, metric: e.target.value })}
              />
              <Input
                className="h-7 max-w-16"
                type="number"
                value={r.windowDays}
                onChange={(e) => set({ ...r, windowDays: Number(e.target.value) })}
                aria-label="Days"
              />
            </>
          )}
          onChange={(items) => update({ ...doc, testHypotheses: items })}
        />
        <ListEditor
          title="Evidence"
          items={doc.evidenceRefs}
          editable={editable}
          render={(r) => `${r.kind} ${r.id}${r.note ? ` · ${r.note}` : ""}`}
          blank={{ kind: "fact", id: 0, note: "" }}
          fields={(r, set) => (
            <>
              <select
                className="rounded-md border bg-background px-1"
                value={r.kind}
                onChange={(e) => set({ ...r, kind: e.target.value })}
              >
                {["fact", "offer", "research_finding", "evidence"].map((k) => (
                  <option key={k}>{k}</option>
                ))}
              </select>
              <Input
                className="h-7 max-w-20"
                type="number"
                value={r.id}
                onChange={(e) => set({ ...r, id: Number(e.target.value) })}
                aria-label="Id"
              />
              <Input
                className="h-7"
                placeholder="Why it matters"
                value={r.note}
                onChange={(e) => set({ ...r, note: e.target.value })}
              />
            </>
          )}
          onChange={(items) => update({ ...doc, evidenceRefs: items })}
        />
        <ListEditor
          title="Assumptions"
          items={doc.assumptions}
          editable={editable}
          render={(r) => `${r.key}: ${r.text} (${r.class})`}
          blank={{ key: "", text: "", value: null, class: "assumption", evidenceRef: null }}
          fields={(r, set) => (
            <>
              <Input
                className="h-7 max-w-24"
                placeholder="Key"
                value={r.key}
                onChange={(e) => set({ ...r, key: e.target.value })}
              />
              <Input
                className="h-7"
                placeholder="What is assumed"
                value={r.text}
                onChange={(e) => set({ ...r, text: e.target.value })}
              />
              <select
                className="rounded-md border bg-background px-1"
                value={r.class}
                onChange={(e) => set({ ...r, class: e.target.value })}
              >
                <option value="assumption">assumption</option>
                <option value="unavailable">unavailable</option>
                <option value="cited">cited</option>
              </select>
            </>
          )}
          onChange={(items) => update({ ...doc, assumptions: items })}
        />
      </div>

      <div className="flex flex-wrap items-center gap-2">
        {editable && (
          <Button
            size="sm"
            disabled={!dirty || save.isPending}
            onClick={() =>
              save.mutate({ document: doc, note: "" }, { onSuccess: () => setDirty(false) })
            }
          >
            Save as version {version.version + 1}
          </Button>
        )}
        {isAdmin && version.state === "ready" && !dirty && (
          <Button
            size="sm"
            variant="outline"
            disabled={accept.isPending}
            onClick={() => accept.mutate(version.version)}
          >
            Accept content (v{version.version})
          </Button>
        )}
        <a href={`/api/plans/${planId}/versions/${version.version}/export?format=json`}>
          <Button size="sm" variant="outline">
            Export JSON
          </Button>
        </a>
        <a href={`/api/plans/${planId}/versions/${version.version}/export?format=csv`}>
          <Button size="sm" variant="outline">
            Export CSV
          </Button>
        </a>
        {editable && (
          <label className="text-muted-foreground">
            Import JSON as new version
            <input
              type="file"
              accept="application/json,.json"
              className="ml-2 text-xs"
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) importPlan.mutate({ format: "json", file: f });
              }}
            />
          </label>
        )}
        {save.error && <span className="text-destructive">{save.error.message}</span>}
        {accept.error && <span className="text-destructive">{accept.error.message}</span>}
        {importPlan.error && <span className="text-destructive">{importPlan.error.message}</span>}
      </div>
    </div>
  );
}

function AllocationRow({
  a,
  channels,
  editable,
  onChange,
  onRemove,
}: {
  a: Allocation;
  channels: ChannelInfo[];
  editable: boolean;
  onChange: (a: Allocation) => void;
  onRemove: () => void;
}) {
  const info = channels.find((c) => c.key === a.channel);
  return (
    <tr>
      <td className="py-1 pr-2">
        <select
          className="rounded-md border bg-background px-1"
          value={a.channel}
          onChange={(e) => onChange({ ...a, channel: e.target.value })}
          disabled={!editable}
        >
          <option value="">Channel…</option>
          {channels.map((c) => (
            <option key={c.key} value={c.key}>
              {c.label}
              {c.supported ? "" : " (manual)"}
            </option>
          ))}
        </select>
        {info && !info.supported && <span className="ml-1 text-muted-foreground">manual</span>}
      </td>
      <td className="py-1 pr-2">
        <Input
          className="h-7"
          value={a.objective}
          onChange={(e) => onChange({ ...a, objective: e.target.value })}
          disabled={!editable}
          aria-label="Objective"
        />
      </td>
      <td className="py-1 pr-2">
        <Input
          className="h-7 w-32 min-w-32"
          value={major(a.budgetMinor)}
          onChange={(e) => onChange({ ...a, budgetMinor: toMinor(e.target.value) })}
          disabled={!editable}
          aria-label="Budget"
        />
      </td>
      <td className="py-1 pr-2">
        <Input
          className="h-7 w-16 min-w-16"
          value={a.geography}
          onChange={(e) => onChange({ ...a, geography: e.target.value })}
          disabled={!editable}
          aria-label="Geography"
        />
      </td>
      <td className="py-1 pr-2">
        <Input
          className="h-7 w-28 min-w-28"
          placeholder="Not available"
          value={a.expectedCplMinor === null ? "" : major(a.expectedCplMinor)}
          onChange={(e) =>
            onChange({
              ...a,
              expectedCplMinor: e.target.value === "" ? null : toMinor(e.target.value),
            })
          }
          disabled={!editable}
          aria-label="Expected CPL"
        />
      </td>
      <td className="py-1">
        {editable && (
          <Button size="xs" variant="outline" onClick={onRemove}>
            Remove
          </Button>
        )}
      </td>
    </tr>
  );
}

function ListEditor<T>({
  title,
  items,
  editable,
  render,
  blank,
  fields,
  onChange,
}: {
  title: string;
  items: T[];
  editable: boolean;
  render: (r: T) => string;
  blank: T;
  fields: (r: T, set: (next: T) => void) => ReactNode;
  onChange: (items: T[]) => void;
}) {
  return (
    <section className="space-y-1 rounded-lg border p-2">
      <h3 className="font-semibold">{title}</h3>
      {items.length === 0 && <p className="text-muted-foreground">None.</p>}
      {items.map((r, i) => (
        <div
          // biome-ignore lint/suspicious/noArrayIndexKey: requirement rows have no identity besides their position
          key={`${title}-${i}`}
          className="flex flex-wrap items-center gap-1"
        >
          {editable ? (
            fields(r, (next) => onChange(items.map((x, j) => (j === i ? next : x))))
          ) : (
            <span>{render(r)}</span>
          )}
          {editable && (
            <Button
              size="xs"
              variant="outline"
              onClick={() => onChange(items.filter((_, j) => j !== i))}
            >
              ×
            </Button>
          )}
        </div>
      ))}
      {editable && (
        <Button size="xs" variant="outline" onClick={() => onChange([...items, blank])}>
          Add
        </Button>
      )}
    </section>
  );
}
