import { Link } from "@tanstack/react-router";
import { useState } from "react";
import {
  useExecutionAction,
  useExecutionCapabilities,
  useLaunchReadiness,
  useLaunchStatus,
  useMilestones,
} from "@/api/launch";
import { usePlans } from "@/api/plans";
import type {
  IntentWithEvents,
  LaunchItem,
  Milestones,
  PlanLaunchReadiness,
  PlanSummary,
  PlatformLaunch,
} from "@/api/types.gen";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { type PlatformKey, PlatformTag } from "@/components/platform";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { formatDateTime, formatRelative } from "@/lib/format";

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
            A launch starts from an accepted{" "}
            <Link to="/create/plans" className="underline underline-offset-4">
              media plan
            </Link>
            .
          </EmptyState>
        </Panel>
      )}
      {plan && <LaunchBody plan={plan} />}
    </>
  );
}

function LaunchBody({ plan }: { plan: PlanSummary }) {
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
          Authorizing a launch happens on the{" "}
          <Link to="/engine/launch" className="underline underline-offset-4">
            engine's launch screen
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
      <Link to="/creative" className={cls}>
        Open creatives
      </Link>
    );
  }
  if (item.refKind === "page") {
    return (
      <Link to="/create/pages/$pageId" params={{ pageId: String(item.refId) }} className={cls}>
        Open page
      </Link>
    );
  }
  if (item.area === "pages") {
    return (
      <Link to="/create/pages" className={cls}>
        Open pages
      </Link>
    );
  }
  if (item.refKind === "plan") {
    return (
      <Link to="/create/plans/$planId" params={{ planId: String(item.refId) }} className={cls}>
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
