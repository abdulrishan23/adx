import { Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, Download, ExternalLink, Plus } from "lucide-react";
import { type ReactNode, useState } from "react";
import { useBrand } from "@/api/brands";
import { useCampaign, useUpdateCampaign } from "@/api/campaigns";
import { useCreativeProject } from "@/api/creative";
import { useCreatePlan, usePlans } from "@/api/plans";
import type {
  CampaignDetail,
  CampaignInput,
  CampaignStage,
  ChannelInfo,
  ChannelNeed,
} from "@/api/types.gen";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Textarea } from "@/components/ui/textarea";
import { useProjects } from "@/engine/api/projects";
import { formatDate, formatMoneyWhole } from "@/lib/format";
import { cn } from "@/lib/utils";
import { CLASS, factValue, keyLabel } from "./brand";
import {
  AskForConcepts,
  ConceptSection,
  NewConceptForm,
  ReviewLinkButtons,
} from "./creative-project";
import { AskForPage } from "./studio/pages";

// A campaign is one push for a project: an offer, a window, a budget and
// channels. It owns the ads, the page, the plan and the launch for that
// push, so a marketer has one place that says what's done and what's next.
// The stages come from the server (core/campaigns); this screen lays them
// out top to bottom with one action each.

const STAGE_LABEL: Record<string, string> = {
  brief: "Brief",
  ads: "Ads",
  page: "Page",
  plan: "Plan and budget",
  approvals: "Approvals",
  live: "Live",
};

export const STATE: Record<string, { label: string; status: Status }> = {
  planning: { label: "Planning", status: "pending" },
  in_production: { label: "In production", status: "caution" },
  awaiting_approval: { label: "Awaiting approval", status: "caution" },
  launching: { label: "Launching", status: "caution" },
  live: { label: "Live", status: "ready" },
};

// PLATFORM names the ad platform a channel runs on; channel labels come
// from the plan's catalogue, which the plans response carries.
const PLATFORM: Record<string, string> = {
  meta: "Meta",
  google_ads: "Google",
  tiktok: "TikTok",
  snapchat: "Snapchat",
  linkedin: "LinkedIn",
  other: "Other",
};

// platformOf reads the platform off a channel key, the way the catalogue
// names them: meta_lead_form, google_search.
const platformOf = (key: string) =>
  key.startsWith("meta") ? "meta" : key.startsWith("google") ? "google_ads" : key;

const asStatus = (s: string): Status =>
  s === "ready" || s === "caution" || s === "error" ? s : "pending";

export type CampaignStageId = "brief" | "ads" | "page" | "plan" | "approvals" | "live";
export const STAGE_IDS: CampaignStageId[] = ["brief", "ads", "page", "plan", "approvals", "live"];

export function CampaignPage({
  projectId,
  campaignId,
  stage: active = "brief",
}: {
  projectId: number;
  campaignId: number;
  stage?: CampaignStageId;
}) {
  const navigate = useNavigate();
  const campaign = useCampaign(campaignId);
  const projects = useProjects();
  const session = useSession();
  const canEdit = session.role !== "viewer";
  const project = projects.data?.projects.find((p) => p.id === projectId);
  const brand = useBrand(project?.brandId ?? null);
  const studio = useCreativeProject(projectId);
  const plansList = usePlans();
  const _channelLabel = (key: string) =>
    plansList.data?.channels.find((ch) => ch.key === key)?.label ?? key;
  const [editing, setEditing] = useState(false);

  if (campaign.isPending) return <LoadingRows rows={8} />;
  if (campaign.error) return <ErrorNote error={campaign.error} />;
  const c = campaign.data;
  const state = STATE[c.state] ?? { label: "Planning", status: "pending" as Status };
  const stage = (id: string): CampaignStage =>
    c.stages.find((s) => s.id === id) ?? { id, status: "pending", note: "" };
  const facts = (brand.data?.facts ?? []).filter(
    (f) => f.projectId === projectId || f.projectId === null,
  );
  const window = [c.startsOn && formatDate(c.startsOn), c.endsOn && formatDate(c.endsOn)]
    .filter(Boolean)
    .join(" to ");
  const meta = [
    c.projectName,
    window,
    c.budgetMinor !== null ? formatMoneyWhole(c.budgetMinor, c.currency) : null,
    c.channels.length > 0
      ? [...new Set(c.channels.map((ch) => PLATFORM[platformOf(ch)] ?? ch))].join(" and ")
      : null,
    c.markets.length > 0 ? c.markets.join(" and ") : null,
  ].filter(Boolean) as string[];

  return (
    <>
      <Link
        to="/projects/$projectId"
        params={{ projectId: String(projectId) }}
        className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> {c.projectName}
      </Link>
      <PageHeader
        title={c.name}
        subtitle={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            {meta.map((m, i) => (
              <span key={m} className="flex items-center gap-2">
                {i > 0 && <span aria-hidden="true">·</span>}
                {m}
              </span>
            ))}
          </span>
        }
        actions={
          <>
            <StatusChip status={state.status}>{state.label}</StatusChip>
            {canEdit && (
              <Button variant="outline" onClick={() => setEditing((v) => !v)}>
                {editing ? "Close" : "Edit brief"}
              </Button>
            )}
          </>
        }
      />

      {editing && (
        <EditBrief
          campaign={c}
          offers={(brand.data?.offers ?? []).filter(
            (o) => o.projectId === null || o.projectId === projectId,
          )}
          channels={plansList.data?.channels ?? []}
          onDone={() => setEditing(false)}
        />
      )}

      <Stepper
        stages={c.stages}
        active={active}
        onPick={(id) =>
          navigate({
            to: "/projects/$projectId/campaigns/$campaignId",
            params: { projectId: String(projectId), campaignId: String(c.id) },
            search: id === "brief" ? {} : { stage: id },
          })
        }
      />

      <div className="space-y-6">
        {active === "brief" && (
          <StageSection
            id="brief"
            n={1}
            title="Brief"
            stage={stage("brief")}
            summary={
              c.goal
                ? "Adx writes and plans from this."
                : "Set a goal so Adx knows what to aim for."
            }
            action={
              canEdit && (
                <Button variant="outline" onClick={() => setEditing(true)}>
                  Edit brief
                </Button>
              )
            }
          >
            <dl className="grid gap-x-8 gap-y-4 sm:grid-cols-2">
              <Field label="Goal">{c.goal || <Muted>Not set</Muted>}</Field>
              <Field label="Offer">
                {c.offer ? (
                  <>
                    <span className="flex items-center gap-2">
                      <StatusMark
                        status={c.offer.current.state === "approved" ? "ready" : "caution"}
                      />
                      {c.offer.current.document.headline || c.offer.name}
                    </span>
                    <span className="text-muted-foreground">
                      {c.offer.current.state === "approved" ? "Approved" : c.offer.current.state}
                      {c.offer.current.document.headlineAr &&
                        ` · ${c.offer.current.document.headlineAr}`}
                    </span>
                  </>
                ) : (
                  <Muted>No offer</Muted>
                )}
              </Field>
              <Field label="Who it's for">{c.audience || <Muted>Not set</Muted>}</Field>
              <Field label="Stay away from">{c.avoid || <Muted>Nothing noted</Muted>}</Field>
            </dl>
            <div className="mt-6">
              <Eyebrow>What Adx may claim</Eyebrow>
              {facts.length === 0 ? (
                <p className="mt-2 text-sm text-muted-foreground">
                  No facts yet. Figures reach ads and pages only once they're approved.
                </p>
              ) : (
                <ul className="mt-2 divide-y border-y">
                  {facts.map((f) => (
                    <li key={f.id} className="flex gap-4 py-2.5 text-sm">
                      <span className="w-36 shrink-0 text-muted-foreground">{keyLabel(f.key)}</span>
                      <span className="flex items-center gap-2">
                        <StatusMark status={CLASS[f.class].status} /> {factValue(f)}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </StageSection>
        )}

        {active === "ads" && (
          <StageSection
            id="ads"
            n={2}
            title="Ads"
            stage={stage("ads")}
            summary={adsSummary(c)}
            action={
              studio.data && (
                <>
                  <ReviewLinkButtons settings={studio.data.settings} />
                  {c.ads.approved > 0 && (
                    <a
                      href={`/api/creative/projects/${projectId}/download?set=approved`}
                      className={buttonVariants({ variant: "outline" })}
                    >
                      <Download /> Approved
                    </a>
                  )}
                </>
              )
            }
          >
            {c.needs.length > 0 && <Needs needs={c.needs} page={asStatus(stage("page").status)} />}
            {canEdit && (
              <div className="mb-6 flex flex-wrap items-start gap-x-8 gap-y-4">
                <AskForConcepts projectId={projectId} campaignId={c.id} />
                <NewConceptForm projectId={projectId} campaignId={c.id} />
              </div>
            )}
            {c.concepts.length === 0 ? (
              <EmptyState title="No ideas yet">
                Ask Adx for ideas from the brief and the approved facts, or add one and build its
                ads from copy.
              </EmptyState>
            ) : (
              <div className="space-y-8">
                {c.concepts.map((k) => (
                  <ConceptSection key={k.id} concept={k} projectId={projectId} canEdit={canEdit} />
                ))}
              </div>
            )}
          </StageSection>
        )}

        {active === "page" && (
          <StageSection
            id="page"
            n={3}
            title="Page"
            stage={stage("page")}
            summary={
              c.pages.length === 0
                ? c.needs.length > 0 && c.needs.every((n) => !n.page)
                  ? "None of this campaign's channels sends people to a page. Add one anyway if you want a place for the offer."
                  : "The page the ads send people to."
                : `${c.pages.length} ${c.pages.length === 1 ? "page" : "pages"} · ${c.pages.filter((p) => p.publishedVersion !== null).length} live`
            }
          >
            {c.pages.length > 0 && (
              <ul className="mb-5 divide-y border-y text-sm">
                {c.pages.map((p) => (
                  <li key={p.id} className="flex items-center gap-3 py-3">
                    <StatusMark status={p.publishedVersion !== null ? "ready" : "caution"} />
                    <Link
                      to="/studio/pages/$pageId"
                      params={{ pageId: String(p.id) }}
                      className="w-24 font-medium hover:underline"
                    >
                      {p.language === "ar" ? "Arabic" : "English"}
                    </Link>
                    <span className="min-w-0 flex-1 truncate text-muted-foreground">
                      {p.name} · /{p.slug} ·{" "}
                      {p.publishedVersion !== null
                        ? `version ${p.publishedVersion} live`
                        : `draft ${p.latestVersion}, not published`}
                    </span>
                    {p.publicUrl && (
                      <a
                        href={p.publicUrl}
                        target="_blank"
                        rel="noreferrer"
                        className="text-muted-foreground hover:text-foreground"
                        aria-label="Open the live page"
                      >
                        <ExternalLink className="size-4" />
                      </a>
                    )}
                  </li>
                ))}
              </ul>
            )}
            {canEdit && project?.brandId !== null && project?.brandId !== undefined && (
              <AskForPage
                fixed={{ brandId: project.brandId, projectId }}
                campaignId={c.id}
                offerId={c.offerId ?? 0}
              />
            )}
            <p className="text-sm text-muted-foreground">
              Every figure on the page is checked against the claims above before it can go live.
            </p>
          </StageSection>
        )}

        {active === "plan" && (
          <StageSection
            id="plan"
            n={4}
            title="Plan and budget"
            stage={stage("plan")}
            summary={
              c.plan
                ? `${c.plan.title} · version ${c.plan.latestVersion} · ${c.plan.accepted ? "accepted" : c.plan.latestState}`
                : "How the budget splits across Meta and Google, and what to expect from it."
            }
            action={
              c.plan ? (
                <Link
                  to="/studio/plans/$planId"
                  params={{ planId: String(c.plan.id) }}
                  className={buttonVariants({ variant: "outline" })}
                >
                  Open plan
                </Link>
              ) : (
                canEdit &&
                project?.brandId !== null &&
                project?.brandId !== undefined && (
                  <StartPlan campaign={c} brandId={project.brandId} />
                )
              )
            }
          >
            <p className="text-sm text-muted-foreground">
              {c.plan
                ? "Adx drafts versions from the project's research; a person accepts one. Nothing spends until a launch is authorized."
                : "Start a plan with the campaign's budget and dates, then ask Adx to draft it from the research."}
            </p>
          </StageSection>
        )}

        {active === "approvals" && (
          <StageSection
            id="approvals"
            n={5}
            title="Approvals"
            stage={stage("approvals")}
            summary="Creative is approved on the review board. Spend is authorized at launch."
          >
            <ul className="divide-y border-y text-sm">
              <li className="flex items-center gap-3 py-3">
                <StatusMark
                  status={
                    c.ads.total === 0
                      ? "pending"
                      : c.ads.approved === c.ads.total
                        ? "ready"
                        : "caution"
                  }
                />
                <span className="flex-1">Creative, on the review board</span>
                <span className="text-muted-foreground">
                  {c.ads.total === 0
                    ? "No ads yet"
                    : `${c.ads.approved} of ${c.ads.total} approved`}
                </span>
              </li>
              <li className="flex items-center gap-3 py-3">
                <StatusMark status={c.launch && c.launch.state !== "none" ? "ready" : "pending"} />
                <span className="flex-1">
                  Spend
                  {c.budgetMinor !== null && `, ${formatMoneyWhole(c.budgetMinor, c.currency)}`}
                </span>
                <span className="text-muted-foreground">
                  {c.launch && c.launch.state !== "none" ? "Authorized" : "Not yet authorized"}
                </span>
              </li>
            </ul>
          </StageSection>
        )}

        {active === "live" && (
          <StageSection
            id="live"
            n={6}
            title="Live"
            stage={stage("live")}
            summary={
              c.launch && c.launch.state !== "none"
                ? c.launch.note
                : "When the checks pass, Adx submits to both platforms and reports here where each one stands."
            }
            action={
              c.plan?.accepted && (
                <Link to="/studio/launch" className={buttonVariants()}>
                  Launch
                </Link>
              )
            }
          >
            <ul className="divide-y border-y text-sm">
              <Check
                status={c.ads.total > 0 && c.ads.approved === c.ads.total ? "ready" : "caution"}
                label="Every ad approved on the review board"
                note={c.ads.total === 0 ? "No ads yet" : `${c.ads.approved} of ${c.ads.total}`}
              />
              <Check
                status={
                  c.pages.length > 0 && c.pages.every((p) => p.publishedVersion !== null)
                    ? "ready"
                    : "caution"
                }
                label="Page published in every language"
                note={
                  c.pages.length === 0
                    ? "No page yet"
                    : `${c.pages.filter((p) => p.publishedVersion !== null).length} of ${c.pages.length}`
                }
              />
              <Check
                status={c.plan?.accepted ? "ready" : "caution"}
                label="Plan accepted"
                note={c.plan ? (c.plan.accepted ? "Accepted" : c.plan.latestState) : "No plan yet"}
              />
              {(c.launch?.platforms ?? []).map((p) => (
                <Check
                  key={p.platform}
                  status={
                    p.state === "confirmed" ? "ready" : p.state === "failed" ? "error" : "caution"
                  }
                  label={PLATFORM[p.platform] ?? p.platform}
                  note={p.state}
                />
              ))}
            </ul>
            <p className="mt-3 text-sm text-muted-foreground">Results then show under Perform.</p>
          </StageSection>
        )}
      </div>
    </>
  );
}

// Stepper is the six stages across the top. One stage fills the view at a
// time, so each can grow into an editor.
function Stepper({
  stages,
  active,
  onPick,
}: {
  stages: CampaignStage[];
  active: CampaignStageId;
  onPick: (id: CampaignStageId) => void;
}) {
  return (
    <ol className="mb-8 grid grid-cols-3 gap-px border bg-border sm:grid-cols-6">
      {stages.map((s, i) => {
        const id = s.id as CampaignStageId;
        const on = id === active;
        return (
          <li key={s.id} className="min-w-0">
            <button
              type="button"
              onClick={() => onPick(id)}
              aria-current={on ? "step" : undefined}
              className={cn(
                "flex h-full w-full flex-col gap-1 px-4 py-3 text-left transition-colors",
                on ? "bg-primary text-primary-foreground" : "bg-card hover:bg-muted",
              )}
            >
              <span
                className={cn(
                  "font-mono text-[0.65rem] tracking-widest uppercase",
                  on ? "opacity-70" : "text-muted-foreground",
                )}
              >
                {i + 1}
              </span>
              <span className="flex items-center gap-2 text-sm font-medium">
                <StatusMark status={asStatus(s.status)} />
                <span className="truncate">{STAGE_LABEL[s.id] ?? s.id}</span>
              </span>
              <span className={cn("truncate text-xs", on ? "opacity-70" : "text-muted-foreground")}>
                {s.note}
              </span>
            </button>
          </li>
        );
      })}
    </ol>
  );
}

// Needs is what each channel asks of the campaign, against what it has.
function Needs({ needs, page }: { needs: ChannelNeed[]; page: Status }) {
  return (
    <div className="mb-6">
      <Eyebrow>What the channels need</Eyebrow>
      <ul className="mt-2 divide-y border-y">
        {needs.map((n) => (
          <li key={n.channel} className="flex flex-wrap items-start gap-x-6 gap-y-2 py-3 text-sm">
            <span className="w-44 shrink-0">
              <span className="font-medium">{n.label}</span>
              {!n.supported && <span className="block text-xs text-muted-foreground">By hand</span>}
            </span>
            <span className="flex min-w-0 flex-1 flex-wrap gap-x-5 gap-y-1">
              {n.assets.map((a) => {
                const en = a.have?.en ?? 0;
                const ar = a.have?.ar ?? 0;
                const have = en + ar;
                const ok = a.kind !== "text" && en >= a.want && ar >= a.want;
                return (
                  <span key={a.label} className="flex items-center gap-2">
                    <StatusMark status={ok ? "ready" : have > 0 ? "caution" : "pending"} />
                    {a.label}
                    {a.kind !== "text" && (
                      <span className="text-muted-foreground">
                        {en} EN · {ar} AR
                      </span>
                    )}
                  </span>
                );
              })}
              {n.page ? (
                <span className="flex items-center gap-2">
                  <StatusMark status={page} />
                  Landing page
                </span>
              ) : (
                <span className="text-muted-foreground">No page</span>
              )}
            </span>
            {n.note && <span className="w-full text-xs text-muted-foreground">{n.note}</span>}
          </li>
        ))}
      </ul>
    </div>
  );
}

function adsSummary(c: CampaignDetail) {
  if (c.concepts.length === 0) return "Ideas, and the ads built from them, in both languages.";
  const parts = [
    `${c.ads.total} ${c.ads.total === 1 ? "ad" : "ads"} in ${c.concepts.length} ${c.concepts.length === 1 ? "idea" : "ideas"}`,
  ];
  if (c.ads.approved) parts.push(`${c.ads.approved} approved`);
  if (c.ads.waiting) parts.push(`${c.ads.waiting} awaiting review`);
  if (c.ads.revise) parts.push(`${c.ads.revise} ${c.ads.revise === 1 ? "needs" : "need"} changes`);
  return parts.join(" · ");
}

function StageSection({
  id,
  n,
  title,
  stage,
  summary,
  action,
  children,
}: {
  id: string;
  n: number;
  title: string;
  stage: CampaignStage;
  summary: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Panel
      className="scroll-mt-6"
      title={
        <span id={id} className="flex items-center gap-3">
          <span className="font-mono text-xs text-muted-foreground tabular-nums">{n}</span>
          <StatusMark status={asStatus(stage.status)} />
          {title}
          <span className="text-sm font-normal text-muted-foreground">{stage.note}</span>
        </span>
      }
      description={summary}
      action={action && <div className="flex shrink-0 flex-wrap gap-2">{action}</div>}
    >
      {children}
    </Panel>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 flex flex-col gap-0.5 text-sm">{children}</dd>
    </div>
  );
}

function Muted({ children }: { children: ReactNode }) {
  return <span className="text-muted-foreground">{children}</span>;
}

function Check({ status, label, note }: { status: Status; label: string; note: string }) {
  return (
    <li className="flex items-center gap-3 py-3">
      <StatusMark status={status} />
      <span className="flex-1">{label}</span>
      <span className="text-muted-foreground">{note}</span>
    </li>
  );
}

// EditBrief saves the campaign's brief in place.
function EditBrief({
  campaign: c,
  offers,
  channels,
  onDone,
}: {
  campaign: CampaignDetail;
  offers: { id: number; name: string }[];
  channels: ChannelInfo[];
  onDone: () => void;
}) {
  const update = useUpdateCampaign(c.id);
  return (
    <BriefForm
      title="Brief"
      initial={{
        projectId: c.projectId,
        name: c.name,
        goal: c.goal,
        audience: c.audience,
        avoid: c.avoid,
        offerId: c.offerId ?? 0,
        startsOn: c.startsOn,
        endsOn: c.endsOn,
        budgetMinor: c.budgetMinor,
        currency: c.currency,
        channels: c.channels,
        markets: c.markets,
      }}
      offers={offers}
      channels={channels}
      pending={update.isPending}
      error={update.error}
      onCancel={onDone}
      onSubmit={(input) => update.mutate(input, { onSuccess: onDone })}
    />
  );
}

// BriefForm is the campaign's brief as a form: what it's called, the offer,
// the goal, who it's for, what to stay away from, the window, the budget,
// the markets and the channels.
export function BriefForm({
  title,
  initial,
  offers,
  channels,
  pending,
  error,
  onCancel,
  onSubmit,
}: {
  title: string;
  initial: CampaignInput;
  offers: { id: number; name: string }[];
  channels: ChannelInfo[];
  pending: boolean;
  error: Error | null;
  onCancel: () => void;
  onSubmit: (input: CampaignInput) => void;
}) {
  const [form, setForm] = useState<CampaignInput>(initial);
  const set = <K extends keyof CampaignInput>(k: K, v: CampaignInput[K]) =>
    setForm((f) => ({ ...f, [k]: v }));
  const toggle = (ch: string) =>
    set(
      "channels",
      form.channels.includes(ch) ? form.channels.filter((x) => x !== ch) : [...form.channels, ch],
    );
  return (
    <Panel title={title} className="mb-6">
      <form
        className="grid gap-4 sm:grid-cols-2"
        onSubmit={(e) => {
          e.preventDefault();
          onSubmit(form);
        }}
      >
        <Labelled id="brief-name" label="Name">
          <Input
            id="brief-name"
            value={form.name}
            onChange={(e) => set("name", e.target.value)}
            required
          />
        </Labelled>
        <Labelled id="brief-offer" label="Offer">
          <select
            id="brief-offer"
            className="h-8 w-full rounded-lg border border-input bg-transparent px-2 text-sm"
            value={form.offerId}
            onChange={(e) => set("offerId", Number(e.target.value))}
          >
            <option value={0}>No offer</option>
            {offers.map((o) => (
              <option key={o.id} value={o.id}>
                {o.name}
              </option>
            ))}
          </select>
        </Labelled>
        <Labelled id="brief-goal" label="Goal" wide>
          <Input
            id="brief-goal"
            value={form.goal}
            onChange={(e) => set("goal", e.target.value)}
            placeholder="150 qualified leads at AED 1,300 or less each"
          />
        </Labelled>
        <Labelled id="brief-audience" label="Who it's for" wide>
          <Textarea
            id="brief-audience"
            value={form.audience}
            onChange={(e) => set("audience", e.target.value)}
            rows={2}
          />
        </Labelled>
        <Labelled id="brief-avoid" label="Stay away from" wide>
          <Textarea
            id="brief-avoid"
            value={form.avoid}
            onChange={(e) => set("avoid", e.target.value)}
            rows={2}
          />
        </Labelled>
        <Labelled id="brief-starts" label="Starts">
          <Input
            id="brief-starts"
            type="date"
            value={form.startsOn}
            onChange={(e) => set("startsOn", e.target.value)}
          />
        </Labelled>
        <Labelled id="brief-ends" label="Ends">
          <Input
            id="brief-ends"
            type="date"
            value={form.endsOn}
            onChange={(e) => set("endsOn", e.target.value)}
          />
        </Labelled>
        <Labelled id="brief-budget" label={`Budget, ${form.currency}`}>
          <Input
            id="brief-budget"
            type="number"
            min={0}
            step={1000}
            value={form.budgetMinor === null ? "" : Math.round(form.budgetMinor / 100)}
            onChange={(e) =>
              set(
                "budgetMinor",
                e.target.value === "" ? null : Math.round(Number(e.target.value) * 100),
              )
            }
          />
        </Labelled>
        <Labelled id="brief-markets" label="Markets, comma separated">
          <Input
            id="brief-markets"
            value={form.markets.join(", ")}
            onChange={(e) =>
              set(
                "markets",
                e.target.value
                  .split(",")
                  .map((m) => m.trim())
                  .filter(Boolean),
              )
            }
          />
        </Labelled>
        <fieldset className="flex flex-col gap-1.5 sm:col-span-2">
          <legend className="text-sm font-medium">Channels</legend>
          <div className="grid gap-x-8 gap-y-2 text-sm sm:grid-cols-3">
            {Object.entries(PLATFORM).map(([platform, label]) => {
              const own = channels.filter((ch) => ch.platform === platform);
              if (own.length === 0) return null;
              return (
                <div key={platform} className="space-y-1">
                  <p className="text-xs text-muted-foreground">{label}</p>
                  {own.map((ch) => (
                    <label key={ch.key} className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        checked={form.channels.includes(ch.key)}
                        onChange={() => toggle(ch.key)}
                      />
                      <span className={ch.supported ? "" : "text-muted-foreground"}>
                        {ch.label}
                      </span>
                    </label>
                  ))}
                </div>
              );
            })}
          </div>
          <p className="text-xs text-muted-foreground">
            Greyed channels can be planned but Adx can't launch or measure them; they run by hand.
          </p>
        </fieldset>
        <div className="flex items-end justify-end gap-2">
          <Button type="button" variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button type="submit" disabled={pending}>
            Save
          </Button>
        </div>
        {error && <p className="text-sm text-destructive sm:col-span-2">{error.message}</p>}
      </form>
    </Panel>
  );
}

function Labelled({
  id,
  label,
  wide,
  children,
}: {
  id: string;
  label: string;
  wide?: boolean;
  children: ReactNode;
}) {
  return (
    <div className={wide ? "flex flex-col gap-1.5 sm:col-span-2" : "flex flex-col gap-1.5"}>
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      {children}
    </div>
  );
}

// StartPlan opens a plan for the campaign with its budget and window, so the
// marketer can ask Adx to draft it from the research.
function StartPlan({ campaign: c, brandId }: { campaign: CampaignDetail; brandId: number }) {
  const create = useCreatePlan();
  return (
    <Button
      disabled={create.isPending}
      onClick={() =>
        create.mutate({
          brandId,
          projectId: c.projectId,
          campaignId: c.id,
          title: c.name,
          note: "",
          document: {
            objective: c.goal,
            budgetMinor: c.budgetMinor ?? 0,
            currency: c.currency,
            periodFrom: c.startsOn,
            periodTo: c.endsOn,
            allocations: [],
            assetRequirements: [],
            pageRequirements: [],
            measurementRequirements: [],
            testHypotheses: [],
            observationWindows: [],
            evidenceRefs: [],
            assumptions: [],
            notes: "",
          },
        })
      }
    >
      <Plus /> Start a plan
    </Button>
  );
}
