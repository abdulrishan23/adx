import { type FormEvent, useState } from "react";
import type { Conflict, Fact, FactInput, Offer, OfferInput } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  useAddFact,
  useAssignProject,
  useBrand,
  useBrands,
  useClaims,
  useCreateBrand,
  useCreateOffer,
  useFactAction,
  useOffer,
  useResolveConflict,
  useReviseOffer,
  useSetOfferState,
} from "@/engine/api/brands";
import { useSession } from "@/engine/api/session";

const classLabel: Record<string, string> = {
  approved: "Approved",
  needs_review: "Needs review",
  inferred: "Inferred by ADX",
  retired: "Retired",
};

const suggestedKeys = [
  "price.from",
  "price.per_sqft",
  "payment_plan",
  "handover",
  "discount.max",
  "location",
  "unit_types",
  "service_charge",
  "developer",
  "rera_number",
];

// The brand workspace: which projects belong to a brand, the offers it runs
// and the facts behind its claims. Facts arrive as "needs review" or
// "inferred"; only an admin's approval makes a claim, and two sources that
// disagree block approval until a person picks one.
export function BrandsPage() {
  const brands = useBrands();
  const session = useSession();
  const [current, setCurrent] = useState<number | null>(null);
  const isAdmin = session.data?.role === "admin";
  const canEdit = isAdmin || session.data?.role === "marketer";

  return (
    <section className="grid gap-6 md:grid-cols-[16rem_1fr]">
      <aside className="space-y-3">
        <h1 className="text-3xl font-semibold tracking-tight">Brands</h1>
        {brands.isPending && <p className="text-sm text-muted-foreground">Loading…</p>}
        {brands.error && <p className="text-sm text-destructive">{brands.error.message}</p>}
        {brands.data?.brands.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No brands yet. An admin adds Reportage first.
          </p>
        )}
        <ul className="space-y-1 text-sm">
          {brands.data?.brands.map((b) => (
            <li key={b.id}>
              <button
                type="button"
                className={`w-full rounded-md px-2 py-1 text-left ${b.id === current ? "bg-muted" : "hover:bg-muted"}`}
                onClick={() => setCurrent(b.id)}
              >
                {b.name}{" "}
                <span className="text-muted-foreground">
                  · {b.kind === "competitor" ? "competitor" : `${b.projectCount} projects`}
                  {b.openConflicts > 0 && ` · ${b.openConflicts} to review`}
                </span>
              </button>
            </li>
          ))}
        </ul>
        {isAdmin && <NewBrand onCreated={setCurrent} />}
        {isAdmin && brands.data && brands.data.unassigned.length > 0 && (
          <div className="rounded-lg border p-3 text-sm">
            <h2 className="font-semibold">Projects under no brand</h2>
            <ul className="mt-1 space-y-1">
              {brands.data.unassigned.map((p) => (
                <li key={p.id} className="flex items-center justify-between gap-2">
                  <span>{p.name}</span>
                  {current !== null && <AssignButton projectId={p.id} brandId={current} />}
                </li>
              ))}
            </ul>
          </div>
        )}
      </aside>
      <div>
        {current === null ? (
          <p className="text-muted-foreground">Pick a brand.</p>
        ) : (
          <BrandDetailView id={current} canEdit={canEdit} isAdmin={isAdmin} />
        )}
      </div>
    </section>
  );
}

function AssignButton({ projectId, brandId }: { projectId: number; brandId: number }) {
  const assign = useAssignProject();
  return (
    <Button
      size="xs"
      variant="outline"
      disabled={assign.isPending}
      onClick={() => assign.mutate({ projectId, brandId })}
    >
      Add to selected brand
    </Button>
  );
}

function NewBrand({ onCreated }: { onCreated: (id: number) => void }) {
  const create = useCreateBrand();
  const [kind, setKind] = useState<"own" | "competitor">("own");
  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const name = String(new FormData(form).get("name") ?? "");
    create.mutate(
      { name, kind, notes: "" },
      {
        onSuccess: (b) => {
          form.reset();
          onCreated(b.id);
        },
      },
    );
  }
  return (
    <form onSubmit={onSubmit} className="space-y-2 rounded-lg border p-3 text-sm">
      <h2 className="font-semibold">New brand</h2>
      <Input name="name" placeholder="Name" aria-label="Brand name" />
      <select
        className="w-full rounded-md border bg-background px-2 py-1"
        value={kind}
        onChange={(e) => setKind(e.target.value as "own" | "competitor")}
      >
        <option value="own">Ours (we market it)</option>
        <option value="competitor">Competitor (we only observe it)</option>
      </select>
      <Button type="submit" size="sm" disabled={create.isPending}>
        Add
      </Button>
      {create.error && <p className="text-destructive">{create.error.message}</p>}
    </form>
  );
}

function BrandDetailView({
  id,
  canEdit,
  isAdmin,
}: {
  id: number;
  canEdit: boolean;
  isAdmin: boolean;
}) {
  const brand = useBrand(id);
  const [project, setProject] = useState(0);
  const [offer, setOffer] = useState<number | null>(null);
  if (brand.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (brand.error) return <p className="text-destructive">{brand.error.message}</p>;
  const d = brand.data;

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-semibold tracking-tight">{d.brand.name}</h2>
        <p className="text-sm text-muted-foreground">
          {d.brand.kind === "competitor" ? "Competitor: observed, never marketed." : "Ours."}{" "}
          {d.projects.length === 0
            ? "No projects assigned."
            : `Projects: ${d.projects.map((p) => p.name).join(", ")}.`}
        </p>
      </div>

      {d.conflicts.length > 0 && (
        <section className="space-y-2 rounded-lg border border-destructive/40 p-3 text-sm">
          <h3 className="font-semibold">Sources disagree ({d.conflicts.length})</h3>
          <p className="text-muted-foreground">
            Neither value can be approved until an admin picks one. Source precedence is not
            configured; every conflict is decided by a person.
          </p>
          {d.conflicts.map((c) => (
            <ConflictRow key={c.id} conflict={c} isAdmin={isAdmin} />
          ))}
        </section>
      )}

      <section className="space-y-3 text-sm">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="font-semibold">Approved claims</h3>
          <select
            className="rounded-md border bg-background px-2 py-1"
            value={project}
            onChange={(e) => setProject(Number(e.target.value))}
          >
            <option value={0}>Brand-wide</option>
            {d.projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </div>
        <Claims brandId={id} projectId={project} />
      </section>

      <section className="space-y-3 text-sm">
        <h3 className="font-semibold">Facts</h3>
        {d.facts.length === 0 && <p className="text-muted-foreground">No facts recorded.</p>}
        {d.facts.length > 0 && (
          <table className="w-full text-left">
            <thead className="text-muted-foreground">
              <tr>
                <th className="py-1 pr-2 font-normal">Key</th>
                <th className="py-1 pr-2 font-normal">Value</th>
                <th className="py-1 pr-2 font-normal">Scope</th>
                <th className="py-1 pr-2 font-normal">Effective</th>
                <th className="py-1 pr-2 font-normal">Source</th>
                <th className="py-1 pr-2 font-normal">Class</th>
                <th className="py-1 font-normal" />
              </tr>
            </thead>
            <tbody className="divide-y">
              {d.facts.map((f) => (
                <FactRow key={f.id} fact={f} projects={d.projects} isAdmin={isAdmin} />
              ))}
            </tbody>
          </table>
        )}
        {canEdit && <NewFact brandId={id} projects={d.projects} />}
      </section>

      <section className="space-y-3 text-sm">
        <h3 className="font-semibold">Offers</h3>
        {d.offers.length === 0 && <p className="text-muted-foreground">No offers.</p>}
        <ul className="divide-y border-y">
          {d.offers.map((o) => (
            <li key={o.id} className="py-2">
              <button
                type="button"
                className="text-left"
                onClick={() => setOffer(offer === o.id ? null : o.id)}
              >
                <span className="font-medium">{o.name}</span>{" "}
                <span className="text-muted-foreground">
                  · v{o.current.version} · {o.current.state} · {o.current.effectiveFrom}
                  {o.current.effectiveTo ? ` to ${o.current.effectiveTo}` : " onwards"} ·{" "}
                  {o.current.document.headline || o.current.document.headlineAr}
                </span>
              </button>
              {offer === o.id && <OfferDetail offer={o} canEdit={canEdit} isAdmin={isAdmin} />}
            </li>
          ))}
        </ul>
        {canEdit && <NewOffer brandId={id} projects={d.projects} />}
      </section>
    </div>
  );
}

function Claims({ brandId, projectId }: { brandId: number; projectId: number }) {
  const claims = useClaims(brandId, projectId);
  if (claims.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (claims.error) return <p className="text-destructive">{claims.error.message}</p>;
  const c = claims.data;
  return (
    <div className="space-y-1">
      {c.claims.length === 0 && (
        <p className="text-muted-foreground">
          Nothing approved for {c.at}. A draft may not state any fact here yet.
        </p>
      )}
      {c.claims.map((f) => (
        <div key={f.id}>
          <span className="font-medium">{f.key}</span>: {formatValue(f)}
          {f.projectId === null && projectId !== 0 && (
            <span className="text-muted-foreground"> (brand-wide)</span>
          )}
        </div>
      ))}
      {c.pending.length > 0 && (
        <p className="text-muted-foreground">
          Awaiting review, not claimable: {c.pending.join(", ")}
        </p>
      )}
    </div>
  );
}

function FactRow({
  fact,
  projects,
  isAdmin,
}: {
  fact: Fact;
  projects: { id: number; name: string }[];
  isAdmin: boolean;
}) {
  const act = useFactAction();
  const [reason, setReason] = useState("");
  const scope =
    fact.projectId === null
      ? "Brand-wide"
      : (projects.find((p) => p.id === fact.projectId)?.name ?? `Project ${fact.projectId}`);
  return (
    <tr className={fact.class === "retired" ? "text-muted-foreground" : ""}>
      <td className="py-1 pr-2">{fact.key}</td>
      <td className="py-1 pr-2">{formatValue(fact)}</td>
      <td className="py-1 pr-2">{scope}</td>
      <td className="py-1 pr-2">
        {fact.effectiveFrom}
        {fact.effectiveTo ? ` to ${fact.effectiveTo}` : ""}
      </td>
      <td className="py-1 pr-2">
        {fact.sourceKind}
        {fact.sourceRef ? `: ${fact.sourceRef}` : ""}
        {fact.evidenceId ? ` (evidence ${fact.evidenceId})` : ""}
      </td>
      <td className="py-1 pr-2">
        <span
          className={
            fact.class === "approved" ? "font-medium" : fact.inConflict ? "text-destructive" : ""
          }
        >
          {fact.inConflict ? "In conflict" : classLabel[fact.class]}
        </span>
      </td>
      <td className="py-1">
        {isAdmin && fact.class !== "approved" && fact.class !== "retired" && (
          <span className="flex flex-wrap items-center gap-1">
            <Button
              size="xs"
              disabled={fact.inConflict || act.isPending}
              onClick={() => act.mutate({ id: fact.id, action: "approve" })}
            >
              Approve
            </Button>
            <Input
              placeholder="Reason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              className="h-7 max-w-32"
            />
            <Button
              size="xs"
              variant="outline"
              disabled={!reason || act.isPending}
              onClick={() => act.mutate({ id: fact.id, action: "retire", reason })}
            >
              Retire
            </Button>
          </span>
        )}
        {act.error && <span className="text-destructive">{act.error.message}</span>}
      </td>
    </tr>
  );
}

function ConflictRow({ conflict, isAdmin }: { conflict: Conflict; isAdmin: boolean }) {
  const resolve = useResolveConflict();
  const [resolution, setResolution] = useState("");
  const side = (f: Fact) =>
    `${formatValue(f)} (${f.sourceKind}${f.sourceRef ? `: ${f.sourceRef}` : ""}, from ${f.effectiveFrom})`;
  return (
    <div className="space-y-1 border-t pt-2">
      <div>
        <span className="font-medium">{conflict.key}</span>: {side(conflict.factA)} vs{" "}
        {side(conflict.factB)}
      </div>
      {isAdmin && (
        <div className="flex flex-wrap items-center gap-1">
          <Input
            placeholder="Why this source wins"
            value={resolution}
            onChange={(e) => setResolution(e.target.value)}
            className="h-7 max-w-xs"
          />
          <Button
            size="xs"
            variant="outline"
            disabled={!resolution || resolve.isPending}
            onClick={() =>
              resolve.mutate({ id: conflict.id, winnerId: conflict.factA.id, resolution })
            }
          >
            Keep first
          </Button>
          <Button
            size="xs"
            variant="outline"
            disabled={!resolution || resolve.isPending}
            onClick={() =>
              resolve.mutate({ id: conflict.id, winnerId: conflict.factB.id, resolution })
            }
          >
            Keep second
          </Button>
          {resolve.error && <span className="text-destructive">{resolve.error.message}</span>}
        </div>
      )}
    </div>
  );
}

function NewFact({
  brandId,
  projects,
}: {
  brandId: number;
  projects: { id: number; name: string }[];
}) {
  const add = useAddFact(brandId);
  const [form, setForm] = useState<FactInput>({
    projectId: 0,
    key: "",
    kind: "text",
    value: "",
    currency: "",
    effectiveFrom: today(),
    effectiveTo: null,
    sourceKind: "manual",
    sourceRef: "",
    evidenceId: 0,
    note: "",
    supersedesId: 0,
  });
  const [opened, setOpened] = useState(0);
  return (
    <form
      className="space-y-2 rounded-lg border p-3"
      onSubmit={(e) => {
        e.preventDefault();
        add.mutate(form, {
          onSuccess: (r) => {
            setOpened(r.conflicts.length);
            setForm({ ...form, value: "", sourceRef: "", note: "" });
          },
        });
      }}
    >
      <h4 className="font-medium">Record a fact</h4>
      <p className="text-muted-foreground">
        It arrives as "needs review". Only an admin's approval makes it a claim.
      </p>
      <div className="flex flex-wrap gap-2">
        <select
          className="rounded-md border bg-background px-2 py-1"
          value={form.projectId}
          onChange={(e) => setForm({ ...form, projectId: Number(e.target.value) })}
        >
          <option value={0}>Brand-wide</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
        <Input
          list="fact-keys"
          placeholder="Key, e.g. price.from"
          value={form.key}
          onChange={(e) => setForm({ ...form, key: e.target.value })}
          className="max-w-48"
        />
        <datalist id="fact-keys">
          {suggestedKeys.map((k) => (
            <option key={k} value={k} />
          ))}
        </datalist>
        <select
          className="rounded-md border bg-background px-2 py-1"
          value={form.kind}
          onChange={(e) => setForm({ ...form, kind: e.target.value as FactInput["kind"] })}
        >
          {["text", "money", "number", "percent", "date"].map((k) => (
            <option key={k} value={k}>
              {k}
            </option>
          ))}
        </select>
        <Input
          placeholder={form.kind === "money" ? "Minor units, e.g. 120000000" : "Value"}
          value={form.value}
          onChange={(e) => setForm({ ...form, value: e.target.value })}
          className="max-w-56"
        />
        {form.kind === "money" && (
          <Input
            placeholder="AED"
            value={form.currency}
            onChange={(e) => setForm({ ...form, currency: e.target.value })}
            className="max-w-20"
          />
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        <Input
          type="date"
          value={form.effectiveFrom}
          onChange={(e) => setForm({ ...form, effectiveFrom: e.target.value })}
          className="max-w-44"
          aria-label="Effective from"
        />
        <Input
          type="date"
          value={form.effectiveTo ?? ""}
          onChange={(e) => setForm({ ...form, effectiveTo: e.target.value || null })}
          className="max-w-44"
          aria-label="Effective to"
        />
        <select
          className="rounded-md border bg-background px-2 py-1"
          value={form.sourceKind}
          onChange={(e) =>
            setForm({ ...form, sourceKind: e.target.value as FactInput["sourceKind"] })
          }
        >
          <option value="manual">Told to us</option>
          <option value="document">From a document</option>
        </select>
        <Input
          placeholder="Source (who or which document)"
          value={form.sourceRef}
          onChange={(e) => setForm({ ...form, sourceRef: e.target.value })}
          className="max-w-64"
        />
      </div>
      <Button type="submit" size="sm" disabled={add.isPending}>
        Record
      </Button>
      {add.error && <p className="text-destructive">{add.error.message}</p>}
      {opened > 0 && (
        <p className="text-destructive">
          Recorded. It disagrees with {opened} existing source{opened > 1 ? "s" : ""}; see "Sources
          disagree" above.
        </p>
      )}
    </form>
  );
}

const emptyOffer: OfferInput = {
  projectId: 0,
  name: "",
  effectiveFrom: today(),
  effectiveTo: null,
  note: "",
  document: {
    headline: "",
    headlineAr: "",
    terms: "",
    termsAr: "",
    discountPct: null,
    priceFrom: null,
    paymentPlan: "",
    conditions: [],
    extra: {},
  },
};

function OfferFields({
  form,
  setForm,
  projects,
}: {
  form: OfferInput;
  setForm: (f: OfferInput) => void;
  projects?: { id: number; name: string }[];
}) {
  const doc = form.document;
  return (
    <>
      {projects && (
        <select
          className="rounded-md border bg-background px-2 py-1"
          value={form.projectId}
          onChange={(e) => setForm({ ...form, projectId: Number(e.target.value) })}
        >
          <option value={0}>Brand-wide</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
      )}
      {projects && (
        <Input
          placeholder="Offer name"
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
        />
      )}
      <Input
        placeholder="Headline (English)"
        value={doc.headline}
        onChange={(e) => setForm({ ...form, document: { ...doc, headline: e.target.value } })}
      />
      <Input
        placeholder="العنوان (عربي)"
        dir="rtl"
        value={doc.headlineAr}
        onChange={(e) => setForm({ ...form, document: { ...doc, headlineAr: e.target.value } })}
      />
      <Input
        placeholder="Conditions, separated by ;"
        value={doc.conditions.join("; ")}
        onChange={(e) =>
          setForm({
            ...form,
            document: {
              ...doc,
              conditions: e.target.value
                .split(";")
                .map((s) => s.trim())
                .filter(Boolean),
            },
          })
        }
      />
      <div className="flex flex-wrap gap-2">
        <Input
          type="number"
          placeholder="Discount %"
          value={doc.discountPct ?? ""}
          onChange={(e) =>
            setForm({
              ...form,
              document: {
                ...doc,
                discountPct: e.target.value === "" ? null : Number(e.target.value),
              },
            })
          }
          className="max-w-32"
        />
        <Input
          placeholder="Payment plan"
          value={doc.paymentPlan}
          onChange={(e) => setForm({ ...form, document: { ...doc, paymentPlan: e.target.value } })}
          className="max-w-48"
        />
        <Input
          type="date"
          value={form.effectiveFrom}
          onChange={(e) => setForm({ ...form, effectiveFrom: e.target.value })}
          className="max-w-44"
          aria-label="Effective from"
        />
        <Input
          type="date"
          value={form.effectiveTo ?? ""}
          onChange={(e) => setForm({ ...form, effectiveTo: e.target.value || null })}
          className="max-w-44"
          aria-label="Effective to"
        />
      </div>
      <Input
        placeholder="Note on this version"
        value={form.note}
        onChange={(e) => setForm({ ...form, note: e.target.value })}
      />
    </>
  );
}

function NewOffer({
  brandId,
  projects,
}: {
  brandId: number;
  projects: { id: number; name: string }[];
}) {
  const create = useCreateOffer(brandId);
  const [form, setForm] = useState<OfferInput>(emptyOffer);
  return (
    <form
      className="space-y-2 rounded-lg border p-3"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(form, { onSuccess: () => setForm(emptyOffer) });
      }}
    >
      <h4 className="font-medium">New offer</h4>
      <OfferFields form={form} setForm={setForm} projects={projects} />
      <Button type="submit" size="sm" disabled={create.isPending}>
        Create draft
      </Button>
      {create.error && <p className="text-destructive">{create.error.message}</p>}
    </form>
  );
}

function OfferDetail({
  offer,
  canEdit,
  isAdmin,
}: {
  offer: Offer;
  canEdit: boolean;
  isAdmin: boolean;
}) {
  const history = useOffer(offer.id);
  const revise = useReviseOffer();
  const setState = useSetOfferState();
  const [form, setForm] = useState<OfferInput>({
    projectId: offer.projectId ?? 0,
    name: offer.name,
    document: offer.current.document,
    effectiveFrom: offer.current.effectiveFrom,
    effectiveTo: offer.current.effectiveTo,
    note: "",
  });
  const [editing, setEditing] = useState(false);
  const affected = revise.data?.affected ?? history.data?.affected ?? [];
  return (
    <div className="mt-2 space-y-2 rounded-lg border p-3">
      <div className="flex flex-wrap items-center gap-2">
        {isAdmin && offer.current.state !== "approved" && (
          <Button size="xs" onClick={() => setState.mutate({ id: offer.id, state: "approved" })}>
            Approve as true
          </Button>
        )}
        {isAdmin && offer.current.state !== "withdrawn" && (
          <Button
            size="xs"
            variant="outline"
            onClick={() => setState.mutate({ id: offer.id, state: "withdrawn" })}
          >
            Withdraw
          </Button>
        )}
        {canEdit && (
          <Button size="xs" variant="outline" onClick={() => setEditing(!editing)}>
            {editing ? "Cancel" : "Change (new version)"}
          </Button>
        )}
        {setState.error && <span className="text-destructive">{setState.error.message}</span>}
      </div>
      <p className="text-muted-foreground">
        Approval says the offer is true as written. It authorizes no spend and publishes nothing.
      </p>
      {editing && (
        <form
          className="space-y-2"
          onSubmit={(e) => {
            e.preventDefault();
            revise.mutate({ id: offer.id, input: form }, { onSuccess: () => setEditing(false) });
          }}
        >
          <OfferFields form={form} setForm={setForm} />
          <Button type="submit" size="sm" disabled={revise.isPending}>
            Save as version {offer.current.version + 1}
          </Button>
          {revise.error && <p className="text-destructive">{revise.error.message}</p>}
        </form>
      )}
      {affected.length > 0 && (
        <div className="rounded-md border border-destructive/40 p-2">
          <p className="font-medium">Built on an older version ({affected.length})</p>
          <ul>
            {affected.map((a) => (
              <li key={a.id}>
                {a.name} · {a.format} · {a.language} · on v{a.offerVersion} · {a.reviewState}
              </li>
            ))}
          </ul>
          <p className="text-muted-foreground">
            Each needs a person to re-lay it out, which makes a new creative version without
            approval.
          </p>
        </div>
      )}
      {history.data && history.data.affectedPlans.length > 0 && (
        <div className="rounded-md border border-destructive/40 p-2">
          <p className="font-medium">
            Plans citing this offer ({history.data.affectedPlans.length})
          </p>
          <ul>
            {history.data.affectedPlans.map((p) => (
              <li key={`${p.planId}-${p.version}`}>
                {p.planTitle} · v{p.version} · {p.state}
              </li>
            ))}
          </ul>
        </div>
      )}
      {history.data && (
        <details>
          <summary className="cursor-pointer text-muted-foreground">
            Versions ({history.data.versions.length})
          </summary>
          <ul className="mt-1 space-y-1">
            {history.data.versions.map((v) => (
              <li key={v.id}>
                v{v.version} · {v.state} · {v.effectiveFrom}
                {v.effectiveTo ? ` to ${v.effectiveTo}` : ""} ·{" "}
                {v.document.headline || v.document.headlineAr}
                {v.note ? ` · ${v.note}` : ""}
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}

function formatValue(f: Fact): string {
  if (f.kind === "money") {
    const n = Number(f.value);
    return Number.isFinite(n) ? `${f.currency ?? ""} ${(n / 100).toLocaleString()}` : f.value;
  }
  if (f.kind === "percent") return `${f.value}%`;
  return f.value;
}

function today(): string {
  return new Date().toISOString().slice(0, 10);
}
