import { Link } from "@tanstack/react-router";
import { useState } from "react";
import {
  useExecutionAction,
  useExecutionCapabilities,
  useLaunchReadiness,
  useLaunchResolution,
  useLaunchStatus,
  useMilestones,
  usePickCampaign,
  useSubmitLaunch,
} from "@/api/launch";
import { usePlans } from "@/api/plans";
import type {
  ChannelInfo,
  IntentWithEvents,
  LaunchItem,
  LaunchResolution,
  LineResolution,
  Milestones,
  PlanLaunchReadiness,
  PlanSummary,
  PlatformLaunch,
  SubmitResult,
} from "@/api/types.gen";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { type PlatformKey, PlatformTag, platformLabel } from "@/components/platform";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { useAccounts } from "@/engine/api/accounts";
import { useEnvelopes } from "@/engine/api/proposals";
import { useSession as useServerSession } from "@/engine/api/session";
import { formatDate, formatDateTime, formatMoneyShort, formatRelative } from "@/lib/format";
import {
  accountsByPlatform,
  campaignSpend,
  lineFigures,
  pickCaption,
  plainNote,
  platformsWithoutAccount,
  SUBMIT_STATE,
  submitBlocker,
} from "@/lib/launch";

// Readiness areas in the order a launch is checked.
const AREAS: { key: string; label: string }[] = [
  { key: "plan", label: "Plan" },
  { key: "assets", label: "Creatives" },
  { key: "pages", label: "Landing pages" },
  { key: "accounts", label: "Ad accounts" },
  { key: "measurement", label: "Measurement" },
  { key: "external_review", label: "Platform review" },
];

const ITEM: Record<string, Status> = {
  ok: "ready",
  blocked: "error",
  warning: "caution",
  unsupported: "pending",
};

// An intent is one request to one platform. Submitted means accepted,
// confirmed means the object exists there; neither is delivery.
const INTENT: Record<string, { status: Status; label: string }> = {
  pending_authorization: { status: "pending", label: "Waiting for authorization" },
  held: { status: "caution", label: "Held" },
  reserved: { status: "pending", label: "Budget reserved" },
  dispatching: { status: "pending", label: "Sending" },
  submitted: { status: "caution", label: "Submitted" },
  uncertain: { status: "caution", label: "No answer yet" },
  reconciling: { status: "pending", label: "Looking it up" },
  confirmed: { status: "ready", label: "Confirmed" },
  externally_confirmed: { status: "ready", label: "Confirmed on the platform" },
  failed: { status: "error", label: "Failed" },
  cancelled: { status: "pending", label: "Cancelled" },
  compensated: { status: "pending", label: "Reversed" },
};

const ROLLUP: Record<string, { status: Status; label: string }> = {
  none: { status: "pending", label: "Nothing sent" },
  in_progress: { status: "pending", label: "In progress" },
  submitted: { status: "caution", label: "Submitted" },
  confirmed: { status: "ready", label: "Confirmed" },
  partial: { status: "caution", label: "Partly failed" },
  failed: { status: "error", label: "Failed" },
};

function platformKey(p: string): PlatformKey {
  return p === "google_ads" ? "google" : p === "meta" ? "meta" : "web";
}

// Launch is the accepted plan becoming live campaigns: readiness across the
// plan, its creatives, pages, accounts and measurement, then each platform's
// intents and what came back. Ready is not live, and submitted is not
// delivery; delivery is read from the platform's own figures.
export function LaunchPage() {
  const plans = usePlans();
  const [pickedPlan, setPickedPlan] = useState<number | null>(null);
  const list = plans.data?.plans ?? [];
  const plan =
    list.find((p) => p.id === pickedPlan) ?? list.find((p) => p.accepted) ?? list[0] ?? null;
  return (
    <>
      <PageHeader
        title="Launch"
        subtitle="From accepted plan to delivering campaigns. Adx checks everything the plan needs, submits when a person authorizes spend, and says where each platform stands."
        actions={
          list.length > 0 && (
            <Select
              aria-label="Plan"
              value={plan ? String(plan.id) : null}
              onValueChange={(v) => setPickedPlan(Number(v))}
              options={list.map((p: PlanSummary) => ({
                value: String(p.id),
                label: `${p.title} · v${p.latestVersion}${p.accepted ? "" : " · not accepted"}`,
              }))}
            />
          )
        }
      />
      {plans.isPending && <LoadingRows rows={8} />}
      {plans.error && <ErrorNote error={plans.error} />}
      {plans.data && !plan && (
        <Panel>
          <EmptyState title="Nothing to launch yet">
            A launch starts from a{" "}
            <Link to="/projects" className="underline underline-offset-4">
              campaign
            </Link>{" "}
            whose plan is accepted.
          </EmptyState>
        </Panel>
      )}
      {plan && <LaunchBody plan={plan} channels={plans.data?.channels ?? []} />}
    </>
  );
}

function LaunchBody({ plan, channels }: { plan: PlanSummary; channels: ChannelInfo[] }) {
  const version = plan.latestVersion;
  const readiness = useLaunchReadiness(plan.id, version);
  const status = useLaunchStatus(plan.id, version);
  const milestones = useMilestones(plan.id, version);
  const capabilities = useExecutionCapabilities();
  const r = readiness.data;
  const blocked = r?.items.filter((i) => i.state === "blocked") ?? [];
  const liveWrites = capabilities.data?.platforms.some((p) => p.live) ?? false;
  return (
    <div className="space-y-5">
      {readiness.error && <ErrorNote error={readiness.error} />}
      {r && blocked.length > 0 && (
        <Panel
          title={`${blocked.length} ${blocked.length === 1 ? "thing" : "things"} blocking ${plan.title}`}
          description="Each one names what to fix and where."
          className="border-status-error/50"
          bodyClassName="p-0"
        >
          <ul className="divide-y">
            {blocked.map((b) => (
              <li key={b.area + b.code + b.refId} className="flex items-start gap-4 px-5 py-3">
                <StatusMark status="error" className="mt-2" />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium">
                    {AREAS.find((a) => a.key === b.area)?.label ?? b.area}
                  </p>
                  <p className="text-sm text-muted-foreground">{b.message}</p>
                </div>
                <FixLink item={b} />
              </li>
            ))}
          </ul>
        </Panel>
      )}

      <MoneyPanel plan={plan} version={version} channels={channels} liveWrites={liveWrites} />

      <div className="grid gap-5 lg:grid-cols-2">
        <Readiness readiness={r} pending={readiness.isPending} />
        <Where milestones={milestones.data} status={status.data} pending={milestones.isPending} />
      </div>

      {status.error && <ErrorNote error={status.error} />}
      {status.data && status.data.platforms.length > 0 && (
        <div className="grid gap-5 lg:grid-cols-2">
          {status.data.platforms.map((p) => (
            <Platform key={p.platform} launch={p} />
          ))}
        </div>
      )}
      {status.data && (
        <p className="text-sm text-muted-foreground">
          {status.data.platforms.length === 0
            ? "Nothing has been sent to a platform for this version. "
            : `${status.data.note} `}
          {!liveWrites &&
            "Live writes to ad accounts are off, so submissions are held until they're turned on. "}
          Submitting starts from "Where the money goes" above; approvals are in{" "}
          <Link to="/proposals" className="underline underline-offset-4">
            Perform › Change log
          </Link>
          .
        </p>
      )}
    </div>
  );
}

// Where a blocker points: the plan, a creative, a page, the connected
// accounts or the measurement checks.
function FixLink({ item }: { item: LaunchItem }) {
  const cls = "shrink-0 text-sm underline underline-offset-4";
  if (item.area === "accounts") {
    return (
      <Link to="/settings/connections" className={cls}>
        Connect
      </Link>
    );
  }
  if (item.area === "measurement") {
    return (
      <Link to="/engine/launch" className={cls}>
        Run the check
      </Link>
    );
  }
  if (item.area === "assets") {
    return (
      <Link to="/projects" className={cls}>
        Open creatives
      </Link>
    );
  }
  if (item.refKind === "page") {
    return (
      <Link to="/studio/pages/$pageId" params={{ pageId: String(item.refId) }} className={cls}>
        Open page
      </Link>
    );
  }
  if (item.area === "pages") {
    return (
      <Link to="/studio/pages" className={cls}>
        Open pages
      </Link>
    );
  }
  if (item.refKind === "plan") {
    return (
      <Link to="/studio/plans/$planId" params={{ planId: String(item.refId) }} className={cls}>
        Open plan
      </Link>
    );
  }
  return null;
}

function Readiness({
  readiness: r,
  pending,
}: {
  readiness: PlanLaunchReadiness | undefined;
  pending: boolean;
}) {
  const ok = r?.items.filter((i) => i.state === "ok").length ?? 0;
  const counted = r?.items.filter((i) => i.state !== "unsupported").length ?? 0;
  return (
    <Panel
      title={
        <span className="flex items-center gap-2">
          {r && <StatusMark status={r.ready ? "ready" : "caution"} />}
          {r ? (r.ready ? "Ready to launch" : "Not ready yet") : "Readiness"}
        </span>
      }
      description={
        r
          ? `${ok} of ${counted} checks pass. All must, before Adx submits anything.${r.live ? " Delivery observed." : ""}`
          : undefined
      }
      bodyClassName="p-0"
    >
      {pending && (
        <div className="p-5">
          <LoadingRows rows={5} />
        </div>
      )}
      {r && (
        <ul className="divide-y">
          {AREAS.map((a) => {
            const items = r.items.filter((i) => i.area === a.key);
            if (items.length === 0) return null;
            return (
              <li key={a.key} className="px-5 py-3">
                <Eyebrow className="mb-1.5">{a.label}</Eyebrow>
                <ul className="space-y-1.5">
                  {items.map((i) => (
                    <li key={i.code + i.refId} className="flex items-start gap-2 text-sm">
                      <StatusMark status={ITEM[i.state] ?? "pending"} className="mt-1.5" />
                      <span className={i.state === "unsupported" ? "text-muted-foreground" : ""}>
                        {i.message}
                      </span>
                    </li>
                  ))}
                </ul>
              </li>
            );
          })}
          {r.links.length > 0 && (
            <li className="px-5 py-3">
              <Eyebrow className="mb-1.5">Tracked links</Eyebrow>
              <ul className="space-y-1">
                {r.links.map((l) => (
                  <li
                    key={`${l.creativeId}-${l.pageId}-${l.channel}`}
                    className="truncate font-mono text-xs text-muted-foreground"
                  >
                    {l.channel} · {l.url}
                  </li>
                ))}
              </ul>
            </li>
          )}
        </ul>
      )}
    </Panel>
  );
}

// Each step is its own milestone, so "launched" never hides "rejected".
function Where({
  milestones: m,
  status,
  pending,
}: {
  milestones: Milestones | undefined;
  status: { state: string } | undefined;
  pending: boolean;
}) {
  const steps = m
    ? [
        { label: "Plan accepted", at: m.planAcceptedAt },
        { label: "First request sent", at: m.firstDispatchedAt },
        { label: "Accepted by a platform", at: m.firstSubmittedAt },
        { label: "Confirmed on the platform", at: m.firstConfirmedAt },
        {
          label: "Delivery observed",
          at: m.deliveryObservedAt,
          note: m.firstDeliveryDay ? `First day with delivery: ${m.firstDeliveryDay}` : undefined,
        },
      ]
    : [];
  const reached = steps.filter((s) => s.at).length;
  return (
    <Panel
      title="Where it is"
      description={status ? (ROLLUP[status.state]?.label ?? status.state) : undefined}
      action={
        m?.readyToDeliveryHours !== null &&
        m?.readyToDeliveryHours !== undefined && (
          <StatusChip status={m.withinOneHour ? "ready" : "caution"}>
            {m.readyToDeliveryHours.toFixed(1)} h to delivery
          </StatusChip>
        )
      }
      bodyClassName="p-0"
    >
      {pending && (
        <div className="p-5">
          <LoadingRows rows={5} />
        </div>
      )}
      {m && (
        <>
          <ol className="divide-y">
            {steps.map((s, i) => (
              <li key={s.label} className="flex items-start gap-3 px-5 py-3">
                <span className="w-5 shrink-0 font-mono text-xs text-muted-foreground">
                  {i + 1}
                </span>
                <StatusMark
                  status={s.at ? "ready" : i === reached ? "caution" : "pending"}
                  className="mt-2"
                />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium">{s.label}</p>
                  {s.note && <p className="text-xs text-muted-foreground">{s.note}</p>}
                </div>
                {s.at && (
                  <span className="shrink-0 font-mono text-xs text-muted-foreground">
                    {formatRelative(s.at)}
                  </span>
                )}
              </li>
            ))}
          </ol>
          <p className="border-t px-5 py-3 text-xs text-muted-foreground">
            Target from the CEO's flow: plan accepted to delivering in under an hour. {m.note}
          </p>
        </>
      )}
    </Panel>
  );
}

function Platform({ launch: p }: { launch: PlatformLaunch }) {
  const roll = ROLLUP[p.state] ?? { status: "pending" as Status, label: p.state };
  return (
    <Panel
      title={
        <span className="flex items-center gap-2">
          <PlatformTag platform={platformKey(p.platform)} />
          <StatusChip status={roll.status}>{roll.label}</StatusChip>
        </span>
      }
      bodyClassName="p-0"
    >
      <ul className="divide-y">
        {p.intents.map((i) => (
          <IntentRow key={i.intent.id} item={i} />
        ))}
      </ul>
    </Panel>
  );
}

function IntentRow({ item }: { item: IntentWithEvents }) {
  const session = useSession();
  const act = useExecutionAction();
  const i = item.intent;
  const st = INTENT[i.state] ?? { status: "pending" as Status, label: i.state };
  const last = item.events[item.events.length - 1];
  const canAct = session.role === "admin";
  return (
    <li className="px-5 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <p className="flex items-center gap-2 text-sm">
            <StatusMark status={st.status} />
            <span className="font-medium">{st.label}</span>
            <span className="font-mono text-xs text-muted-foreground">#{i.id}</span>
            {i.attempts > 1 && (
              <span className="text-xs text-muted-foreground">{i.attempts} attempts</span>
            )}
          </p>
          {last && <p className="mt-1 text-sm text-muted-foreground">{last.detail}</p>}
          {i.lastError && <p className="mt-1 text-sm text-status-error">{i.lastError}</p>}
          {i.providerRefs.length > 0 && (
            <p className="mt-1 font-mono text-xs text-muted-foreground">
              {i.providerRefs.map((ref) => `${ref.kind} ${ref.externalId}`).join(" · ")}
            </p>
          )}
          <p className="mt-1 text-xs text-muted-foreground">
            {formatDateTime(i.updatedAt)}
            {i.confirmedAt && ` · confirmed ${formatRelative(i.confirmedAt)}`}
          </p>
        </div>
        {canAct && (i.state === "failed" || i.state === "uncertain") && (
          <Button
            size="sm"
            variant="outline"
            disabled={act.isPending}
            onClick={() =>
              act.mutate({ id: i.id, action: i.state === "failed" ? "retry" : "reconcile" })
            }
          >
            {i.state === "failed" ? "Retry" : "Look it up"}
          </Button>
        )}
      </div>
      {act.error && <p className="mt-2 text-sm text-status-error">{act.error.message}</p>}
    </li>
  );
}

// What the launch would do for a line, as the server resolved it (BD-68).
const KIND: Record<string, { status: Status; label: string }> = {
  create: { status: "pending", label: "Creates" },
  update: { status: "ready", label: "Updates" },
  resume: { status: "ready", label: "Resumes" },
  pick: { status: "caution", label: "Needs a pick" },
  unsupported: { status: "pending", label: "Not launched" },
};

// Where the money goes: each plan line against its project's campaigns,
// resolved by the server as of today in Dubai. One running match is
// updated; more than one and a person picks; none running but a paused one
// and it is resumed rather than duplicated; none and a campaign is created.
// The plan is the month's ceiling, so spend this month nets off the line
// and the rest is spread over the days left. Submit writes it out as one
// proposal per line; nothing here reaches a platform.
function MoneyPanel({
  plan,
  version,
  channels,
  liveWrites,
}: {
  plan: PlanSummary;
  version: number;
  channels: ChannelInfo[];
  liveWrites: boolean;
}) {
  const resolution = useLaunchResolution(plan.id, version);
  const label = (key: string) => channels.find((c) => c.key === key)?.label ?? key;
  const res = resolution.data;
  return (
    <Panel
      title="Where the money goes"
      description={
        res
          ? `Each plan line against its project's campaigns, as of ${formatDate(res.today)}. One running match is updated; more than one and you pick; a paused one is resumed; none and Adx creates one. Spend inside the plan's period comes off the line and the rest is spread over the ${res.daysLeft} days to ${formatDate(res.periodEnd)}.`
          : undefined
      }
      bodyClassName="p-0"
    >
      {resolution.isPending && (
        <div className="p-5">
          <LoadingRows rows={3} />
        </div>
      )}
      {resolution.error && (
        <div className="p-5">
          <ErrorNote error={resolution.error} />
        </div>
      )}
      {res && (
        <>
          <ul className="divide-y">
            {res.lines.map((l) => (
              <MoneyLine key={l.line.index} line={l} resolution={res} label={label} />
            ))}
          </ul>
          {res.unmatched.length > 0 && (
            <p className="border-t px-5 py-3 text-xs text-muted-foreground">
              Not under this plan: {res.unmatched.map((c) => c.name).join(", ")}. Running campaigns
              no line takes; this launch leaves them as they are.
            </p>
          )}
          <SubmitBlock resolution={res} label={label} liveWrites={liveWrites} />
        </>
      )}
    </Panel>
  );
}

function MoneyLine({
  line: l,
  resolution: res,
  label,
}: {
  line: LineResolution;
  resolution: LaunchResolution;
  label: (channel: string) => string;
}) {
  const pick = usePickCampaign();
  const session = useServerSession();
  const kind = KIND[l.kind] ?? { status: "pending" as Status, label: l.kind };
  const figures = lineFigures(l);
  const cur = l.line.currency;
  const sentence =
    l.kind === "create"
      ? `Creates a ${label(l.line.channel)} campaign.`
      : l.kind === "update"
        ? `Updates ${l.campaign?.name ?? "the matched campaign"}.`
        : l.kind === "resume"
          ? `Resumes ${l.campaign?.name ?? "the paused campaign"}.`
          : l.kind === "pick"
            ? l.note ||
              `${l.candidates.length} campaigns match. Pick the one this line is for; Adx never updates both unasked.`
            : l.note || "Not launched.";
  return (
    <li className="flex items-start gap-3 px-5 py-3">
      <StatusMark status={kind.status} className="mt-2" />
      <div className="min-w-0 flex-1 space-y-1.5">
        <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm font-medium">
          <PlatformTag platform={platformKey(l.line.platform)} />
          <span>
            {label(l.line.channel)} · {formatMoneyShort(l.line.budgetMinor, cur)} this month
          </span>
          <StatusChip status={kind.status}>{kind.label}</StatusChip>
        </p>
        <p className={l.kind === "unsupported" ? "text-sm text-muted-foreground" : "text-sm"}>
          {sentence}
        </p>
        {figures && (
          <dl className="flex flex-wrap gap-x-6 gap-y-1">
            <Figure label="Spent this month" value={figures.spent} />
            <Figure label="Left" value={figures.left} />
            <Figure label={`A day to ${formatDate(res.periodEnd)}`} value={figures.daily} />
          </dl>
        )}
        {l.candidates.length > 0 && (
          <div className="flex flex-wrap items-center gap-2 pt-0.5">
            <Select
              aria-label={`Campaign for line ${l.line.index + 1}, ${label(l.line.channel)}`}
              value={l.pick ? String(l.pick.campaignId) : null}
              onValueChange={(id) =>
                pick.mutate({
                  planId: res.planId,
                  planVersion: res.planVersion,
                  line: l.line.index,
                  campaignId: Number(id),
                })
              }
              options={l.candidates.map((c) => ({
                value: String(c.id),
                label: `${c.name} · ${campaignSpend(c, cur)} spent this month${c.running ? "" : " · paused"}`,
              }))}
              placeholder="Pick a campaign"
              disabled={pick.isPending}
              className="max-w-md"
            />
            {l.pick && (
              <span className="text-xs text-muted-foreground">
                {pickCaption(l.pick, session.data?.user?.id, formatRelative)}
              </span>
            )}
          </div>
        )}
        {pick.error && <p className="text-sm text-status-error">{pick.error.message}</p>}
        {l.note && l.kind !== "pick" && l.kind !== "unsupported" && (
          <p className="text-xs text-muted-foreground">{l.note}</p>
        )}
        {l.paused.length > 0 && (
          <p className="text-xs text-muted-foreground">
            Left paused: {l.paused.map((c) => c.name).join(", ")}.
          </p>
        )}
      </div>
    </li>
  );
}

function Figure({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm font-medium tabular-nums">{value}</dd>
    </div>
  );
}

// Submit posts the resolution to /api/launch/submit. It waits while a line
// still needs a pick, and says so. The version's envelopes are opened on
// the way when none are, which needs an account per platform: the one the
// line's campaign sits in, the only connected one, or a person's choice.
function SubmitBlock({
  resolution: res,
  label,
  liveWrites,
}: {
  resolution: LaunchResolution;
  label: (channel: string) => string;
  liveWrites: boolean;
}) {
  const session = useSession();
  const envelopes = useEnvelopes();
  const accounts = useAccounts();
  const submit = useSubmitLaunch();
  const [chosen, setChosen] = useState<Record<string, number>>({});
  const connected = accounts.data?.accounts ?? [];
  const open = (envelopes.data?.envelopes ?? []).some(
    (e) => e.planId === res.planId && e.planVersion === res.planVersion,
  );
  const byPlatform = { ...accountsByPlatform(res.lines, connected), ...chosen };
  const missing = open ? [] : platformsWithoutAccount(res.lines, byPlatform);
  const toName = missing.filter((p) => connected.some((a) => a.provider === p));
  const toConnect = missing.filter((p) => !connected.some((a) => a.provider === p));
  const pl = (p: string) => platformLabel(platformKey(p));
  const blocker =
    submitBlocker(res.lines, label) ??
    (toConnect.length > 0
      ? `No ${toConnect.map(pl).join(" or ")} account is connected, so the version's envelopes cannot be opened.`
      : toName.length > 0
        ? `Name the ${toName.map(pl).join(" and ")} account to launch into.`
        : null);
  const result = submit.data;
  return (
    <div className="space-y-3 border-t px-5 py-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="max-w-2xl space-y-1 text-sm text-muted-foreground">
          <p>
            Submit writes one proposal per line under the version's budget envelopes
            {open ? "" : ", opening them first"}. What policy allows you is sent to the outbox; the
            rest waits for an admin in Perform.
            {!liveWrites &&
              " Live writes are off, so the outbox holds everything until they're on."}
          </p>
          {blocker && <p className="text-foreground">{blocker}</p>}
        </div>
        {session.role !== "viewer" && (
          <Button
            disabled={blocker !== null || submit.isPending}
            onClick={() =>
              submit.mutate({
                planId: res.planId,
                planVersion: res.planVersion,
                accountByPlatform: open ? null : byPlatform,
              })
            }
          >
            {submit.isPending ? "Submitting…" : result ? "Submit again" : "Submit the launch"}
          </Button>
        )}
      </div>
      {toName.map((p) => (
        <Select
          key={p}
          aria-label={`${pl(p)} account`}
          value={chosen[p] ? String(chosen[p]) : null}
          onValueChange={(id) => setChosen((c) => ({ ...c, [p]: Number(id) }))}
          options={connected
            .filter((a) => a.provider === p)
            .map((a) => ({ value: String(a.id), label: a.name ?? a.externalId }))}
          placeholder={`${pl(p)} account`}
          className="max-w-sm"
        />
      ))}
      {toConnect.length > 0 && (
        <Link to="/settings/connections" className="text-sm underline underline-offset-4">
          Connect an account
        </Link>
      )}
      {submit.error && <ErrorNote error={submit.error} />}
      {result && <SubmitOutcome result={result} label={label} />}
    </div>
  );
}

// What Submit did, line by line. Executed is an intent in the outbox, not
// delivery; a line waiting for approval links to the proposal in Perform.
function SubmitOutcome({
  result,
  label,
}: {
  result: SubmitResult;
  label: (channel: string) => string;
}) {
  return (
    <ul className="divide-y rounded-md border">
      {result.lines.map((s) => {
        const st = SUBMIT_STATE[s.state] ?? { status: "pending" as Status, label: s.state };
        return (
          <li key={s.line.index} className="flex items-start gap-3 px-4 py-2.5 text-sm">
            <StatusMark status={st.status} className="mt-2" />
            <div className="min-w-0 flex-1 space-y-0.5">
              <p className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{label(s.line.channel)}</span>
                <StatusChip status={st.status}>{st.label}</StatusChip>
                {s.proposal && (
                  <span className="font-mono text-xs text-muted-foreground">
                    proposal #{s.proposal.id}
                  </span>
                )}
                {s.intent && (
                  <span className="font-mono text-xs text-muted-foreground">
                    intent #{s.intent.id}
                  </span>
                )}
              </p>
              {s.note && <p className="text-muted-foreground">{plainNote(s.note)}</p>}
              {s.state === "awaiting_approval" && s.proposal && (
                // The engine's proposals screen is the working one (BD-57). This
                // moves to /proposals?open=<id> when Perform › Proposals is rewired.
                <Link to="/engine/proposals" className="underline underline-offset-4">
                  Open in the proposals screen
                </Link>
              )}
            </div>
          </li>
        );
      })}
      <li className="px-4 py-2.5 text-xs text-muted-foreground">{result.note}</li>
    </ul>
  );
}
