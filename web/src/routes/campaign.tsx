import { Link } from "@tanstack/react-router";
import { ArrowLeft, ExternalLink, Link2, Sparkles } from "lucide-react";
import type { ReactNode } from "react";
import { Eyebrow, PageHeader, Panel } from "@/components/page";
import { Button } from "@/components/ui/button";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { useProjects } from "@/engine/api/projects";
import { cn } from "@/lib/utils";
import { SampleChip } from "./studio/shared";

// A campaign is one push for a project: an offer, a window, a budget and
// channels. It owns the ads, the page, the plan and the launch for that
// push, so a marketer has one place that says what's done and what's next.
// Sample data until campaigns exist on the server (see BD-64).

type Stage = "brief" | "ads" | "page" | "plan" | "approvals" | "live";

const STAGES: { id: Stage; label: string; status: Status; note: string }[] = [
  { id: "brief", label: "Brief", status: "ready", note: "Set" },
  { id: "ads", label: "Ads", status: "caution", note: "1 needs changes" },
  { id: "page", label: "Page", status: "caution", note: "Arabic in review" },
  { id: "plan", label: "Plan and budget", status: "ready", note: "Accepted" },
  { id: "approvals", label: "Approvals", status: "pending", note: "Not authorized" },
  { id: "live", label: "Live", status: "pending", note: "Not yet" },
];

const FACTS = [
  ["Starting price", "AED 1,200,000"],
  ["Max discount", "15%"],
  ["Payment plan", "10% on booking, 40% during construction, 50% on handover"],
  ["Handover", "December 2027"],
  ["Units", "3 and 4 bedroom villas"],
  ["Location", "Dubailand, off Al Ain Road"],
];

type SampleAd = {
  name: string;
  size: "story" | "square";
  lang: "EN" | "AR";
  status: Status;
  word: string;
  tint: string;
};

const IDEAS: { title: string; angle: string; ads: SampleAd[] }[] = [
  {
    title: "Launch weekend",
    angle: "The discount, for families upgrading from an apartment.",
    ads: [
      {
        name: "Story",
        size: "story",
        lang: "EN",
        status: "ready",
        word: "Approved",
        tint: "#c49854",
      },
      {
        name: "Story",
        size: "story",
        lang: "AR",
        status: "ready",
        word: "Approved",
        tint: "#c49854",
      },
      {
        name: "Feed",
        size: "square",
        lang: "EN",
        status: "ready",
        word: "Approved",
        tint: "#486e8c",
      },
      {
        name: "Feed",
        size: "square",
        lang: "AR",
        status: "caution",
        word: "Needs changes",
        tint: "#486e8c",
      },
    ],
  },
  {
    title: "First villa",
    angle: "The payment plan, for first-time villa buyers.",
    ads: [
      {
        name: "Story",
        size: "story",
        lang: "EN",
        status: "pending",
        word: "Awaiting review",
        tint: "#7a6a58",
      },
      {
        name: "Story",
        size: "story",
        lang: "AR",
        status: "pending",
        word: "Awaiting review",
        tint: "#7a6a58",
      },
    ],
  },
];

const LINES = [
  {
    channel: "Meta",
    what: "Lead ads and a conversion set to the page",
    share: 60,
    amount: "AED 120,000",
  },
  {
    channel: "Google",
    what: "Search on villa and Dubailand terms, Performance Max",
    share: 40,
    amount: "AED 80,000",
  },
];

const CHECKS: { label: string; status: Status; note: string }[] = [
  { label: "Meta and Google accounts connected", status: "ready", note: "Reportage · both" },
  { label: "Every ad approved on the review board", status: "caution", note: "5 of 6" },
  {
    label: "Page published in both languages",
    status: "caution",
    note: "English live, Arabic in review",
  },
  { label: "Plan accepted", status: "ready", note: "Accepted 18 Oct" },
  { label: "Spend authorized", status: "pending", note: "AED 200,000 waiting" },
];

export function CampaignPage({ projectId }: { projectId: number; campaignId: string }) {
  const projects = useProjects();
  const project = projects.data?.projects.find((p) => p.id === projectId);
  const name = project?.name ?? "The Wadi";
  return (
    <>
      <Link
        to="/projects/$projectId"
        params={{ projectId: String(projectId) }}
        className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> {name}
      </Link>
      <PageHeader
        title={
          <span className="flex flex-wrap items-center gap-3">
            Launch weekend <SampleChip />
          </span>
        }
        subtitle={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>{name}</span>
            <span aria-hidden="true">·</span>
            <span>24 to 26 October</span>
            <span aria-hidden="true">·</span>
            <span>AED 200,000</span>
            <span aria-hidden="true">·</span>
            <span>Meta and Google</span>
            <span aria-hidden="true">·</span>
            <span>UAE and India</span>
          </span>
        }
        actions={
          <>
            <StatusChip status="caution">In production</StatusChip>
            <Button variant="outline">Edit brief</Button>
          </>
        }
      />

      <div className="flex flex-col gap-8 xl:flex-row">
        <nav className="shrink-0 xl:sticky xl:top-6 xl:w-64 xl:self-start">
          <ol className="flex gap-1 overflow-x-auto xl:flex-col">
            {STAGES.map((s, i) => (
              <li key={s.id}>
                <a
                  href={`#${s.id}`}
                  className="flex items-center gap-3 px-3 py-2 text-sm whitespace-nowrap hover:bg-muted"
                >
                  <span className="w-4 font-mono text-[0.7rem] text-muted-foreground tabular-nums">
                    {i + 1}
                  </span>
                  <StatusMark status={s.status} />
                  <span className="font-medium">{s.label}</span>
                  <span className="hidden text-muted-foreground xl:ml-auto xl:inline">
                    {s.note}
                  </span>
                </a>
              </li>
            ))}
          </ol>
          <p className="mt-4 hidden px-3 text-xs text-muted-foreground xl:block">
            Adx moves a stage on when it can. A person decides at Approvals and Live.
          </p>
        </nav>

        <div className="min-w-0 flex-1 space-y-6">
          <StageSection
            id="brief"
            n={1}
            title="Brief"
            status="ready"
            summary="Set on 15 October. Adx writes and plans from this."
            action={<Button variant="outline">Edit brief</Button>}
          >
            <dl className="grid gap-x-8 gap-y-4 sm:grid-cols-2">
              <Field label="Goal">150 qualified leads at AED 1,300 or less each</Field>
              <Field label="Offer">
                <span className="flex items-center gap-2">
                  <StatusMark status="ready" /> Up to 15% off at the launch weekend
                </span>
                <span className="text-muted-foreground">
                  Approved · خصم يصل إلى 15% في عطلة الإطلاق
                </span>
              </Field>
              <Field label="Who it's for">
                Families in Dubai and Abu Dhabi moving up from an apartment. Investors in India
                buying off-plan.
              </Field>
              <Field label="Stay away from">Guaranteed returns. Any figure not listed below.</Field>
            </dl>
            <div className="mt-6">
              <Eyebrow>What Adx may claim</Eyebrow>
              <ul className="mt-2 divide-y border-y">
                {FACTS.map(([k, v]) => (
                  <li key={k} className="flex gap-4 py-2.5 text-sm">
                    <span className="w-36 shrink-0 text-muted-foreground">{k}</span>
                    <span className="flex items-center gap-2">
                      <StatusMark status="ready" /> {v}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          </StageSection>

          <StageSection
            id="ads"
            n={2}
            title="Ads"
            status="caution"
            summary="6 ads in 2 ideas · 3 approved · 2 awaiting review · 1 needs changes"
            action={
              <>
                <Button variant="outline">
                  <Link2 /> Copy review link
                </Button>
                <Button>
                  <Sparkles /> Ask Adx for an idea
                </Button>
              </>
            }
          >
            <div className="space-y-6">
              {IDEAS.map((idea) => (
                <div key={idea.title}>
                  <div className="mb-3 flex flex-wrap items-baseline gap-x-3">
                    <h3 className="font-heading text-base font-semibold">{idea.title}</h3>
                    <p className="text-sm text-muted-foreground">{idea.angle}</p>
                  </div>
                  <ul className="flex flex-wrap gap-4">
                    {idea.ads.map((ad) => (
                      <li key={`${ad.name}-${ad.lang}`} className="w-36">
                        <AdTile ad={ad} />
                        <p className="mt-2 flex items-center gap-2 text-sm">
                          <StatusMark status={ad.status} />
                          <span>
                            {ad.name} {ad.lang}
                          </span>
                        </p>
                        <p className="text-xs text-muted-foreground">{ad.word}</p>
                      </li>
                    ))}
                    <li className="flex w-36">
                      <button
                        type="button"
                        className="flex min-h-36 w-full items-center justify-center self-stretch border border-dashed text-sm text-muted-foreground hover:border-foreground/40 hover:text-foreground"
                      >
                        Add a size
                      </button>
                    </li>
                  </ul>
                </div>
              ))}
            </div>
          </StageSection>

          <StageSection
            id="page"
            n={3}
            title="Page"
            status="caution"
            summary="wadi-launch · English live · Arabic twin awaiting native review"
            action={
              <>
                <Button variant="outline">
                  <ExternalLink /> Preview
                </Button>
                <Button variant="outline">
                  <Sparkles /> Draft again
                </Button>
              </>
            }
          >
            <ul className="divide-y border-y text-sm">
              <PageRow
                status="ready"
                name="English"
                note="Published 19 October · 2 forms received"
              />
              <PageRow
                status="caution"
                name="Arabic"
                note="Draft 3 · 1 claim to check: الحجز بنسبة 10%"
              />
            </ul>
            <p className="mt-3 text-sm text-muted-foreground">
              Every figure on the page is checked against the claims above before it can go live.
            </p>
          </StageSection>

          <StageSection
            id="plan"
            n={4}
            title="Plan and budget"
            status="ready"
            summary="AED 200,000 over 3 days · expected 150 to 190 leads at AED 1,050 to 1,330"
            action={<Button variant="outline">Open plan</Button>}
          >
            <ul className="divide-y border-y text-sm">
              {LINES.map((l) => (
                <li key={l.channel} className="flex items-center gap-4 py-3">
                  <span className="w-20 shrink-0 font-medium">{l.channel}</span>
                  <span className="min-w-0 flex-1 text-muted-foreground">{l.what}</span>
                  <span className="w-10 text-right font-mono text-xs tabular-nums">{l.share}%</span>
                  <span className="w-28 text-right tabular-nums">{l.amount}</span>
                </li>
              ))}
            </ul>
            <p className="mt-3 text-sm text-muted-foreground">
              Drafted by Adx from the Wadi research on 17 October. Accepted on 18 October.
            </p>
          </StageSection>

          <StageSection
            id="approvals"
            n={5}
            title="Approvals"
            status="pending"
            summary="Creative: 5 of 6 approved · Spend: AED 200,000 not yet authorized"
            action={<Button>Ask for spend authorization</Button>}
          >
            <ul className="divide-y border-y text-sm">
              <li className="flex items-center gap-3 py-3">
                <StatusMark status="caution" />
                <span className="flex-1">Creative, on the review board</span>
                <span className="text-muted-foreground">1 ad needs changes · Feed AR</span>
              </li>
              <li className="flex items-center gap-3 py-3">
                <StatusMark status="pending" />
                <span className="flex-1">Spend, AED 200,000</span>
                <span className="text-muted-foreground">Nobody asked yet</span>
              </li>
            </ul>
          </StageSection>

          <StageSection
            id="live"
            n={6}
            title="Live"
            status="pending"
            summary="Not live. 3 checks to pass."
            action={<Button disabled>Launch</Button>}
          >
            <ul className="divide-y border-y text-sm">
              {CHECKS.map((c) => (
                <li key={c.label} className="flex items-center gap-3 py-3">
                  <StatusMark status={c.status} />
                  <span className="flex-1">{c.label}</span>
                  <span className="text-muted-foreground">{c.note}</span>
                </li>
              ))}
            </ul>
            <p className="mt-3 text-sm text-muted-foreground">
              When the checks pass, Adx submits to both platforms and reports here where each one
              stands. Results then show under Perform.
            </p>
          </StageSection>
        </div>
      </div>
    </>
  );
}

function StageSection({
  id,
  n,
  title,
  status,
  summary,
  action,
  children,
}: {
  id: string;
  n: number;
  title: string;
  status: Status;
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
          <StatusMark status={status} />
          {title}
        </span>
      }
      description={summary}
      action={action && <div className="flex shrink-0 gap-2">{action}</div>}
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

function PageRow({ status, name, note }: { status: Status; name: string; note: string }) {
  return (
    <li className="flex items-center gap-3 py-3">
      <StatusMark status={status} />
      <span className="w-20 font-medium">{name}</span>
      <span className="text-muted-foreground">{note}</span>
    </li>
  );
}

// AdTile stands in for a rendered ad: the picture's tint with the headline
// over it, in the ad's shape.
function AdTile({ ad }: { ad: SampleAd }) {
  const ar = ad.lang === "AR";
  return (
    <div
      className={cn("flex w-36 flex-col justify-end p-3", ad.size === "story" ? "h-64" : "h-36")}
      style={{ background: `linear-gradient(180deg, ${ad.tint}66, ${ad.tint})` }}
      dir={ar ? "rtl" : "ltr"}
    >
      <p className="font-heading text-sm leading-tight font-semibold text-white">
        {ar ? "عطلة الإطلاق" : "Launch weekend"}
      </p>
      <p className="mt-1 text-[0.65rem] text-white/80">
        {ar ? "خصم يصل إلى 15%" : "Up to 15% off"}
      </p>
    </div>
  );
}

export const campaignLink = (projectId: number) => ({
  to: "/projects/$projectId/campaigns/$campaignId" as const,
  params: { projectId: String(projectId), campaignId: "launch-weekend" },
});
