import { getRouteApi, Link } from "@tanstack/react-router";
import { ArrowLeft, Plus } from "lucide-react";
import { type FormEvent, useId, useState } from "react";
import {
  useAddFact,
  useAssignProject,
  useBrand,
  useBrands,
  useFactAction,
  useResolveConflict,
} from "@/api/brands";
import { useBriefs } from "@/api/research";
import type {
  Brief,
  Conflict,
  Fact,
  FactClass,
  FactKind,
  FactSource,
  Offer,
} from "@/api/types.gen";
import { useSession } from "@/app/session";
import { Form } from "@/components/form";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { formatRelative } from "@/lib/format";
import { cn } from "@/lib/utils";

const route = getRouteApi("/app/brands/$brandId");

export const CLASS: Record<FactClass, { status: Status; label: string }> = {
  approved: { status: "ready", label: "Approved" },
  needs_review: { status: "caution", label: "Needs review" },
  inferred: { status: "pending", label: "Inferred" },
  retired: { status: "pending", label: "Retired" },
};

const SOURCE: Record<FactSource, string> = {
  document: "Document",
  manual: "Told to us",
  evidence: "Observed",
};

// Keys read as sentences in the brand page. Anything else shows as typed.
const KEY_LABEL: Record<string, string> = {
  "price.from": "Prices from",
  "discount.max": "Max discount",
  handover: "Handover",
  location: "Location",
  units: "Units",
  unit_types: "Units",
  payment_plan: "Payment plan",
  usp: "USPs",
  audience: "Audience",
};
export const keyLabel = (k: string) => KEY_LABEL[k] ?? k.replace(/[._]/g, " ");

// A brand is where Create starts: the projects it markets, the facts Adx is
// allowed to state about them, the offers running, and the research that
// grounds the plan. Two sources that disagree block the fact until someone
// decides; only approved facts reach research, plans, creatives and pages.
export function BrandPage() {
  const { brandId } = route.useParams();
  const id = Number(brandId);
  const brand = useBrand(id);
  return (
    <>
      <Link
        to="/projects"
        className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" />{" "}
        {brand.data?.brand.kind === "competitor" ? "Competitors" : "Projects"}
      </Link>
      {brand.isPending && <LoadingRows rows={8} />}
      {brand.error && <ErrorNote error={brand.error} />}
      {brand.data && <Body id={id} d={brand.data} />}
    </>
  );
}

function Body({ id, d }: { id: number; d: NonNullable<ReturnType<typeof useBrand>["data"]> }) {
  const session = useSession();
  const admin = session.role === "admin";
  const open = d.conflicts.filter((c) => c.state === "open");
  const live = d.facts.filter((f) => f.class !== "retired");
  const keys = [...new Set(live.map((f) => f.key))];
  const briefs = useBriefs();
  const mine = briefs.data?.briefs.filter((b) => b.brandId === id) ?? [];
  return (
    <>
      <PageHeader
        title={d.brand.name}
        subtitle={
          d.brand.kind === "own"
            ? `${d.projects.length} ${d.projects.length === 1 ? "project" : "projects"} · ${live.filter((f) => f.class === "approved").length} approved facts · ${d.offers.length} ${d.offers.length === 1 ? "offer" : "offers"}`
            : "A competitor Adx watches. Its facts come from research and stay on this side."
        }
      />
      <div className="space-y-6">
        {open.length > 0 && (
          <Panel
            title={open.length === 1 ? "A fact to decide" : `${open.length} facts to decide`}
            description="Two sources disagree. Research and plans can't use this fact until one is approved."
            className="border-status-caution/50"
          >
            <div className="space-y-4">
              {open.map((c) => (
                <ConflictRow key={c.id} conflict={c} canDecide={admin} />
              ))}
            </div>
          </Panel>
        )}

        <div className="grid gap-6 lg:grid-cols-[2fr_1fr]">
          <div className="space-y-6">
            <Panel
              title="Facts"
              description="What Adx may say, with where each came from. Only approved facts become claims."
              bodyClassName="p-0"
            >
              {keys.length === 0 ? (
                <EmptyState title="Nothing recorded yet">
                  Record a fact below, or let research bring some in.
                </EmptyState>
              ) : (
                keys.map((k) => (
                  <section key={k} className="border-b last:border-b-0">
                    <h3 className="px-5 pt-4 pb-1 font-mono text-[0.65rem] font-medium tracking-widest text-muted-foreground uppercase">
                      {keyLabel(k)}
                    </h3>
                    <ul>
                      {live
                        .filter((f) => f.key === k)
                        .map((f) => (
                          <FactRow key={f.id} fact={f} projects={d.projects} canDecide={admin} />
                        ))}
                    </ul>
                  </section>
                ))
              )}
            </Panel>
            {admin && d.projects.length > 0 && <RecordFact brandId={id} projects={d.projects} />}
          </div>

          <div className="space-y-6">
            <Projects brandId={id} projects={d.projects} canEdit={admin} />
            <Offers offers={d.offers} projects={d.projects} />
            <ResearchPanel briefs={mine} loading={briefs.isPending} />
          </div>
        </div>
      </div>
    </>
  );
}

function ConflictRow({ conflict: c, canDecide }: { conflict: Conflict; canDecide: boolean }) {
  const resolve = useResolveConflict();
  const [why, setWhy] = useState("");
  const pick = (winnerId: number) => resolve.mutate({ id: c.id, winnerId, resolution: why.trim() });
  return (
    <div>
      <Eyebrow>{keyLabel(c.key)}</Eyebrow>
      <div className="mt-2 grid gap-3 md:grid-cols-2">
        {[c.factA, c.factB].map((f) => (
          <div key={f.id} className="flex flex-col gap-3 border bg-card p-4">
            <p className="font-medium">{factValue(f)}</p>
            <Source fact={f} />
            {canDecide && (
              <Button
                size="sm"
                variant="outline"
                className="self-start"
                disabled={resolve.isPending || why.trim() === ""}
                onClick={() => pick(f.id)}
              >
                Use this one
              </Button>
            )}
          </div>
        ))}
      </div>
      {canDecide && (
        <div className="mt-3 flex items-center gap-2">
          <Input
            value={why}
            onChange={(e) => setWhy(e.target.value)}
            placeholder="Why this source wins (kept with the decision)"
            className="max-w-md"
          />
          {resolve.error && (
            <span className="text-xs text-destructive">{resolve.error.message}</span>
          )}
        </div>
      )}
    </div>
  );
}

function FactRow({
  fact: f,
  projects,
  canDecide,
}: {
  fact: Fact;
  projects: { id: number; name: string }[];
  canDecide: boolean;
}) {
  const act = useFactAction();
  const cls = CLASS[f.class];
  const scope =
    f.projectId === null
      ? "Brand-wide"
      : (projects.find((p) => p.id === f.projectId)?.name ?? "A project");
  return (
    <li className="flex items-start gap-3 px-5 py-2.5">
      <StatusMark status={f.inConflict ? "caution" : cls.status} className="mt-1.5" />
      <div className="min-w-0 flex-1">
        <p className={cn(f.class === "retired" && "text-muted-foreground line-through")}>
          {factValue(f)}
        </p>
        <p className="mt-0.5 text-xs text-muted-foreground">
          {scope} · <Source fact={f} inline />
          {f.note && ` · ${f.note}`}
        </p>
      </div>
      {f.class !== "approved" && <StatusChip status={cls.status}>{cls.label}</StatusChip>}
      {canDecide && f.class === "needs_review" && !f.inConflict && (
        <Button
          size="sm"
          variant="outline"
          disabled={act.isPending}
          onClick={() => act.mutate({ id: f.id, action: "approve" })}
        >
          Approve
        </Button>
      )}
    </li>
  );
}

export function factValue(f: Fact) {
  if (f.kind === "money" && f.currency) {
    // Stored in minor units; shown in whole dirhams, cents would be noise.
    const n = Number(f.value) / 100;
    return Number.isFinite(n) ? `${f.currency} ${Math.round(n).toLocaleString("en")}` : f.value;
  }
  if (f.kind === "percent") return `${f.value}%`;
  if (f.kind === "date" && /^\d{4}-\d{2}-\d{2}$/.test(f.value)) {
    return new Date(`${f.value}T00:00:00Z`).toLocaleDateString("en-GB", {
      month: "long",
      year: "numeric",
      timeZone: "UTC",
    });
  }
  return f.value;
}

function Source({ fact: f, inline }: { fact: Fact; inline?: boolean }) {
  const text = `${SOURCE[f.sourceKind]}${f.sourceRef ? ` · ${f.sourceRef}` : ""} · from ${f.effectiveFrom}${f.effectiveTo ? ` to ${f.effectiveTo}` : ""}`;
  return inline ? <span>{text}</span> : <p className="text-xs text-muted-foreground">{text}</p>;
}

const KINDS: { value: FactKind; label: string }[] = [
  { value: "text", label: "Text" },
  { value: "money", label: "Money" },
  { value: "number", label: "Number" },
  { value: "percent", label: "Percent" },
  { value: "date", label: "Date" },
];

// RecordFact is how a person adds what they know. It arrives as "needs
// review"; an admin's approval makes it a claim.
function RecordFact({
  brandId,
  projects,
}: {
  brandId: number;
  projects: { id: number; name: string }[];
}) {
  const add = useAddFact(brandId);
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<FactKind>("text");
  const [source, setSource] = useState<FactSource>("manual");
  const [projectId, setProjectId] = useState<string>("0");
  const ids = { key: useId(), value: useId(), ref: useId(), from: useId() };
  if (!open) {
    return (
      <Button variant="outline" onClick={() => setOpen(true)}>
        <Plus /> Record a fact
      </Button>
    );
  }
  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const text = (k: string) => String(f.get(k) ?? "").trim();
    add.mutate(
      {
        projectId: Number(projectId),
        key: text("key"),
        kind,
        value: text("value"),
        currency: kind === "money" ? "AED" : "",
        effectiveFrom: text("from") || new Date().toISOString().slice(0, 10),
        effectiveTo: null,
        sourceKind: source,
        sourceRef: text("ref"),
        evidenceId: 0,
        note: "",
        supersedesId: 0,
      },
      { onSuccess: () => setOpen(false) },
    );
  }
  return (
    <Panel
      title="Record a fact"
      description="Arrives as needs review. An admin's approval makes it a claim."
    >
      <Form onSubmit={submit} className="grid gap-3 sm:grid-cols-2">
        <Select
          aria-label="Scope"
          value={projectId}
          onValueChange={(v) => setProjectId(v ?? "0")}
          options={[
            { value: "0", label: "Brand-wide" },
            ...projects.map((p) => ({ value: String(p.id), label: p.name })),
          ]}
        />
        <Select
          aria-label="Kind"
          value={kind}
          onValueChange={(v) => setKind((v as FactKind) ?? "text")}
          options={KINDS}
        />
        <Input id={ids.key} name="key" required placeholder="Key, e.g. price.from" />
        <Input
          id={ids.value}
          name="value"
          required
          placeholder={kind === "money" ? "Amount in AED" : "Value"}
        />
        <Select
          aria-label="Source"
          value={source}
          onValueChange={(v) => setSource((v as FactSource) ?? "manual")}
          options={[
            { value: "manual", label: "Told to us" },
            { value: "document", label: "A document" },
          ]}
        />
        <Input id={ids.ref} name="ref" placeholder="Who said it, or which document" />
        <Input id={ids.from} name="from" type="date" />
        <div className="flex items-center gap-2">
          <Button type="submit" disabled={add.isPending}>
            Record
          </Button>
          <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
            Cancel
          </Button>
        </div>
        {add.error && <p className="text-xs text-destructive sm:col-span-2">{add.error.message}</p>}
      </Form>
    </Panel>
  );
}

function Projects({
  brandId,
  projects,
  canEdit,
}: {
  brandId: number;
  projects: { id: number; name: string }[];
  canEdit: boolean;
}) {
  const brands = useBrands();
  const assign = useAssignProject();
  const unassigned = brands.data?.unassigned ?? [];
  return (
    <Panel title="Projects" description="The developments this brand markets." bodyClassName="p-0">
      {projects.length === 0 && unassigned.length === 0 && <EmptyState title="No projects yet" />}
      <ul className="divide-y">
        {projects.map((p) => (
          <li key={p.id} className="flex items-center justify-between gap-3 px-5 py-2.5 text-sm">
            <Link
              to="/projects/$projectId"
              params={{ projectId: String(p.id) }}
              className="font-medium hover:underline"
            >
              {p.name}
            </Link>
            <Link
              to="/projects/$projectId"
              params={{ projectId: String(p.id) }}
              search={{ tab: "concepts" }}
              className="text-xs text-muted-foreground hover:text-foreground"
            >
              Creatives
            </Link>
          </li>
        ))}
      </ul>
      {canEdit && unassigned.length > 0 && (
        <div className="border-t px-5 py-3">
          <p className="mb-2 text-xs text-muted-foreground">Not under a brand yet</p>
          <ul className="space-y-1.5">
            {unassigned.map((p) => (
              <li key={p.id} className="flex items-center justify-between gap-3 text-sm">
                {p.name}
                <Button
                  size="xs"
                  variant="outline"
                  disabled={assign.isPending}
                  onClick={() => assign.mutate({ projectId: p.id, brandId })}
                >
                  Attach
                </Button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Panel>
  );
}

function Offers({
  offers,
  projects,
}: {
  offers: Offer[];
  projects: { id: number; name: string }[];
}) {
  const state: Record<Offer["current"]["state"], Status> = {
    approved: "ready",
    draft: "pending",
    withdrawn: "pending",
  };
  return (
    <Panel
      title="Offers"
      description="What's on the table, versioned. Copy can only claim what an approved offer says."
      bodyClassName="p-0"
      action={
        <Link to="/engine/brands" className="text-xs text-muted-foreground hover:text-foreground">
          Manage
        </Link>
      }
    >
      {offers.length === 0 ? (
        <EmptyState title="No offers yet" />
      ) : (
        <ul className="divide-y">
          {offers.map((o) => (
            <li key={o.id} className="px-5 py-3">
              <p className="flex items-center gap-2 text-sm font-medium">
                <StatusMark status={state[o.current.state]} />
                {o.name}
                <span className="font-mono text-xs font-normal text-muted-foreground">
                  v{o.current.version}
                </span>
              </p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {o.projectId === null
                  ? "Brand-wide"
                  : (projects.find((p) => p.id === o.projectId)?.name ?? "A project")}
                {o.current.document.headline && ` · ${o.current.document.headline}`}
                {` · from ${o.current.effectiveFrom}`}
              </p>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}

function ResearchPanel({ briefs, loading }: { briefs: Brief[]; loading: boolean }) {
  return (
    <Panel
      title="Research"
      description="Briefs Adx has run for this brand. Plans are grounded on their findings."
      bodyClassName="p-0"
      action={
        <Link to="/engine/research" className={buttonVariants({ variant: "outline", size: "sm" })}>
          Open
        </Link>
      }
    >
      {loading && (
        <div className="p-4">
          <LoadingRows rows={2} />
        </div>
      )}
      {!loading && briefs.length === 0 && (
        <EmptyState title="No research yet">
          Start a project with a website and Adx opens the first brief.
        </EmptyState>
      )}
      <ul className="divide-y">
        {briefs.map((b) => (
          <li key={b.id} className="px-5 py-3">
            <p className="text-sm font-medium">{b.title}</p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {b.question || "No question set"} · {b.geography} · {b.language} ·{" "}
              {formatRelative(b.createdAt)}
            </p>
          </li>
        ))}
      </ul>
    </Panel>
  );
}
