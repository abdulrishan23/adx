import { useState } from "react";
import type { LaunchItem } from "@/api/types.gen";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAccounts } from "@/engine/api/accounts";
import {
  useCompensate,
  useExecutionAction,
  useExecutionCapabilities,
  useLaunchStatus,
  useMilestones,
  useRepair,
  useReversibility,
} from "@/engine/api/execution";
import {
  useLaunchReadiness,
  useMeasurementCatalog,
  useMeasurementChecks,
  useRunCheck,
} from "@/engine/api/measurement";
import { usePages } from "@/engine/api/pages";
import { usePlan, usePlans } from "@/engine/api/plans";
import { useResearchEvidence } from "@/engine/api/research";
import { useSession } from "@/engine/api/session";

const areaLabel: Record<string, string> = {
  plan: "Plan",
  assets: "Creatives",
  pages: "Landing pages",
  accounts: "Ad accounts",
  measurement: "Measurement",
  external_review: "Platform review",
};

const tone: Record<string, string> = {
  ok: "",
  blocked: "text-destructive",
  warning: "text-foreground",
  unsupported: "text-muted-foreground",
};

// Launch readiness joins the accepted plan, its approved creatives, its live
// tested pages, the connected accounts and the measurement checks into one
// verdict. Ready is not live: nothing is live until an authorized execution
// (B3.5) confirms delivery, and that is said on the page.
export function LaunchPage() {
  const plans = usePlans();
  const session = useSession();
  const [planId, setPlanId] = useState<number | null>(null);
  const [version, setVersion] = useState<number | null>(null);
  const plan = usePlan(planId);
  const canEdit = session.data?.role === "marketer" || session.data?.role === "admin";
  const versions = plan.data?.versions ?? [];
  const shownVersion = version ?? versions[versions.length - 1]?.version ?? null;

  return (
    <section className="space-y-6 text-sm">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-3xl font-semibold tracking-tight">Launch readiness</h1>
        <div className="flex flex-wrap gap-2">
          <select
            className="rounded-md border bg-background px-2 py-1"
            value={planId ?? ""}
            onChange={(e) => {
              setPlanId(e.target.value ? Number(e.target.value) : null);
              setVersion(null);
            }}
          >
            <option value="">Plan…</option>
            {plans.data?.plans.map((p) => (
              <option key={p.id} value={p.id}>
                {p.title}
              </option>
            ))}
          </select>
          {planId !== null && (
            <select
              className="rounded-md border bg-background px-2 py-1"
              value={shownVersion ?? ""}
              onChange={(e) => setVersion(Number(e.target.value))}
            >
              {versions.map((v) => (
                <option key={v.id} value={v.version}>
                  v{v.version} · {v.state}
                </option>
              ))}
            </select>
          )}
        </div>
      </div>
      {planId === null ? (
        <p className="text-muted-foreground">Pick a plan.</p>
      ) : (
        shownVersion !== null && (
          <Readiness planId={planId} version={shownVersion} canEdit={canEdit} />
        )
      )}
      {planId !== null && shownVersion !== null && (
        <Deployment
          planId={planId}
          version={shownVersion}
          isAdmin={session.data?.role === "admin"}
          canEdit={canEdit}
        />
      )}
      <CatalogView />
      <WriteCapabilities />
    </section>
  );
}

function Readiness({
  planId,
  version,
  canEdit,
}: {
  planId: number;
  version: number;
  canEdit: boolean;
}) {
  const readiness = useLaunchReadiness(planId, version);
  const checks = useMeasurementChecks(planId, version);
  const [evidenceId, setEvidenceId] = useState<number | null>(null);
  if (readiness.isPending) return <p className="text-muted-foreground">Loading…</p>;
  if (readiness.error) return <p className="text-destructive">{readiness.error.message}</p>;
  const r = readiness.data;
  const byArea = new Map<string, LaunchItem[]>();
  for (const it of r.items) byArea.set(it.area, [...(byArea.get(it.area) ?? []), it]);
  return (
    <div className="space-y-4">
      <div className="rounded-lg border p-3">
        <p className="font-medium">
          {r.ready ? "Ready to put forward for launch" : "Not ready"} ·{" "}
          {r.live ? "live" : "not live"}
        </p>
        <p className="text-muted-foreground">
          Ready means every gate ADX can check is clear. Nothing is live until an authorized
          execution confirms delivery; acceptance and readiness authorize no spend
          {r.executionAuthorization === null && " (no execution authorization exists)"}.
        </p>
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        {["plan", "assets", "pages", "accounts", "measurement", "external_review"].map((area) => (
          <section key={area} className="rounded-lg border p-3">
            <h3 className="font-semibold">{areaLabel[area]}</h3>
            <ul className="mt-1 space-y-1">
              {(byArea.get(area) ?? []).map((it) => (
                <li
                  key={`${it.code}:${it.refKind}:${it.refId}:${it.message}`}
                  className={tone[it.state]}
                >
                  {it.state === "blocked"
                    ? "Blocks: "
                    : it.state === "unsupported"
                      ? "Unsupported: "
                      : it.state === "warning"
                        ? "Note: "
                        : ""}
                  {it.message}
                </li>
              ))}
              {(byArea.get(area) ?? []).length === 0 && (
                <li className="text-muted-foreground">Nothing to check.</li>
              )}
            </ul>
          </section>
        ))}
      </div>
      {canEdit && <RunChecks planId={planId} version={version} />}
      <section className="space-y-1">
        <h3 className="font-semibold">Checks on this version</h3>
        {checks.data?.checks.length === 0 && <p className="text-muted-foreground">None run yet.</p>}
        <ul className="space-y-1">
          {checks.data?.checks.map((c) => (
            <li
              key={c.id}
              className={
                c.state === "passed"
                  ? ""
                  : c.state === "failed"
                    ? "text-destructive"
                    : "text-muted-foreground"
              }
            >
              {new Date(c.checkedAt).toLocaleString()} · {c.kind} · {c.state} · {c.detail}
              {c.evidenceId && (
                <>
                  {" "}
                  ·{" "}
                  <button
                    type="button"
                    className="underline"
                    onClick={() => setEvidenceId(c.evidenceId as number)}
                  >
                    evidence
                  </button>
                </>
              )}
            </li>
          ))}
        </ul>
      </section>
      {r.links.length > 0 && (
        <section className="space-y-1">
          <h3 className="font-semibold">
            Tracked links{" "}
            <span className="font-normal text-muted-foreground">(built with the fixed scheme)</span>
          </h3>
          <ul className="space-y-1">
            {r.links.map((l) => (
              <li key={`${l.creativeId}-${l.pageId}-${l.channel}`} className="break-all">
                creative {l.creativeId} · {l.channel} · <code className="text-xs">{l.url}</code>
              </li>
            ))}
          </ul>
        </section>
      )}
      {evidenceId !== null && <Evidence id={evidenceId} onClose={() => setEvidenceId(null)} />}
    </div>
  );
}

function RunChecks({ planId, version }: { planId: number; version: number }) {
  const pages = usePages();
  const accounts = useAccounts();
  const run = useRunCheck();
  const [pageId, setPageId] = useState(0);
  const [accountId, setAccountId] = useState(0);
  const [name, setName] = useState("");
  return (
    <section className="space-y-2 rounded-lg border p-3">
      <h3 className="font-semibold">Run a check</h3>
      <p className="text-muted-foreground">
        A form check posts a real test submission through the live page; it is marked as a test and
        never counts as an enquiry. A conversion-action check reads what the last Google Ads refresh
        saw. Pixel events cannot be verified by ADX and are recorded as unsupported.
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <select
          className="rounded-md border bg-background px-2 py-1"
          value={pageId}
          onChange={(e) => setPageId(Number(e.target.value))}
        >
          <option value={0}>Page…</option>
          {pages.data?.pages.map((p) => (
            <option key={p.id} value={p.id}>
              /p/{p.slug}
            </option>
          ))}
        </select>
        <Button
          size="xs"
          disabled={!pageId || run.isPending}
          onClick={() =>
            run.mutate({
              planId,
              planVersion: version,
              kind: "form_submit",
              pageId,
              accountId: 0,
              name: "",
            })
          }
        >
          Check form
        </Button>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <select
          className="rounded-md border bg-background px-2 py-1"
          value={accountId}
          onChange={(e) => setAccountId(Number(e.target.value))}
        >
          <option value={0}>Account…</option>
          {accounts.data?.accounts.map((a) => (
            <option key={a.id} value={a.id}>
              {a.provider} {a.name ?? a.externalId}
            </option>
          ))}
        </select>
        <Input
          className="h-7 max-w-56"
          placeholder="Conversion action name"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <Button
          size="xs"
          disabled={!accountId || !name || run.isPending}
          onClick={() =>
            run.mutate({
              planId,
              planVersion: version,
              kind: "conversion_action",
              pageId: 0,
              accountId,
              name,
            })
          }
        >
          Check conversion action
        </Button>
        <Button
          size="xs"
          variant="outline"
          disabled={!accountId || run.isPending}
          onClick={() =>
            run.mutate({
              planId,
              planVersion: version,
              kind: "account_permission",
              pageId: 0,
              accountId,
              name: "",
            })
          }
        >
          Check account
        </Button>
        <Button
          size="xs"
          variant="outline"
          disabled={run.isPending}
          onClick={() =>
            run.mutate({
              planId,
              planVersion: version,
              kind: "pixel_event",
              pageId: 0,
              accountId: 0,
              name: name || "Lead",
            })
          }
        >
          Record pixel as unsupported
        </Button>
      </div>
      {run.error && <p className="text-destructive">{run.error.message}</p>}
      {run.data && (
        <p className={run.data.state === "passed" ? "" : "text-destructive"}>
          {run.data.kind}: {run.data.state} · {run.data.detail}
        </p>
      )}
    </section>
  );
}

function CatalogView() {
  const cat = useMeasurementCatalog();
  if (!cat.data) return null;
  return (
    <details className="rounded-lg border p-3">
      <summary className="cursor-pointer font-medium">What ADX can and cannot measure</summary>
      <ul className="mt-2 space-y-1">
        {cat.data.events.map((e) => (
          <li key={e.key} className={e.supported ? "" : "text-muted-foreground"}>
            <span className="font-medium">{e.label}</span>:{" "}
            {e.supported ? "supported" : "unsupported"} · {e.how}
          </li>
        ))}
      </ul>
      <p className="mt-2 text-muted-foreground">
        Attribution fields kept: {cat.data.attributionFields.join(", ")}.
      </p>
      <p className="text-muted-foreground">Links: {cat.data.utmScheme}.</p>
      <p className="text-muted-foreground">Deduplication: {cat.data.dedupeRule}</p>
    </details>
  );
}

function Evidence({ id, onClose }: { id: number; onClose: () => void }) {
  const ev = useResearchEvidence(id);
  return (
    <section className="space-y-1 rounded-lg border p-3">
      <div className="flex items-baseline justify-between">
        <h3 className="font-semibold">Evidence {id}</h3>
        <Button size="xs" variant="outline" onClick={onClose}>
          Close
        </Button>
      </div>
      {ev.error && <p className="text-destructive">{ev.error.message}</p>}
      {ev.data && (
        <pre className="max-h-72 overflow-auto whitespace-pre-wrap rounded-md bg-muted p-2 text-xs">
          {JSON.stringify(ev.data.payload, null, 2)}
        </pre>
      )}
    </section>
  );
}

// Deployment: what the outbox did for this plan version, per platform, with
// each intent's timeline. Submitted and confirmed are the platform's answers
// about the request and the object; neither is delivery.
function Deployment({
  planId,
  version,
  isAdmin,
  canEdit,
}: {
  planId: number;
  version: number;
  isAdmin: boolean;
  canEdit: boolean;
}) {
  const launch = useLaunchStatus(planId, version);
  const act = useExecutionAction();
  if (launch.isPending) return <p className="text-muted-foreground">Loading deployment…</p>;
  if (launch.error) return <p className="text-destructive">{launch.error.message}</p>;
  const l = launch.data;
  return (
    <section className="space-y-3 rounded-lg border p-3">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h3 className="font-semibold">
          Deployment: {l.state}
          {l.delivered ? " · delivered" : " · not delivered"}
        </h3>
      </div>
      <p className="text-muted-foreground">{l.note}</p>
      <MilestonesView planId={planId} version={version} />
      {l.platforms.length === 0 && (
        <p className="text-muted-foreground">
          No executions for this plan version. Proposals that spend from its envelopes appear here
          once executed.
        </p>
      )}
      <div className="grid gap-3 md:grid-cols-2">
        {l.platforms.map((p) => (
          <div key={p.platform} className="rounded-lg border p-2">
            <p className="font-medium">
              {p.platform} · {p.state}
            </p>
            {p.intents.map(({ intent, events }) => (
              <div key={intent.id} className="mt-2 border-t pt-2">
                <div className="flex flex-wrap items-center gap-2">
                  <span>
                    #{intent.id} · {intent.state} · attempts {intent.attempts}
                  </span>
                  {intent.providerRefs.map((r) => (
                    <span key={`${r.kind}-${r.externalId}`} className="text-muted-foreground">
                      {r.platform} {r.kind} {r.externalId}
                    </span>
                  ))}
                  {isAdmin && intent.state === "failed" && (
                    <Button
                      size="xs"
                      variant="outline"
                      disabled={act.isPending}
                      onClick={() => act.mutate({ id: intent.id, action: "retry" })}
                    >
                      Retry
                    </Button>
                  )}
                  {canEdit &&
                    ["reserved", "uncertain", "reconciling", "submitted"].includes(
                      intent.state,
                    ) && (
                      <Button
                        size="xs"
                        variant="outline"
                        disabled={act.isPending}
                        onClick={() => act.mutate({ id: intent.id, action: "reconcile" })}
                      >
                        Reconcile now
                      </Button>
                    )}
                </div>
                {intent.lastError && (
                  <p
                    className={
                      intent.state === "failed" ? "text-destructive" : "text-muted-foreground"
                    }
                  >
                    {intent.lastError}
                  </p>
                )}
                <Recovery intent={intent} isAdmin={isAdmin} canEdit={canEdit} />
                <ol className="mt-1 space-y-0.5 text-muted-foreground">
                  {events.map((e) => (
                    <li key={e.id}>
                      {new Date(e.at).toLocaleTimeString()} · {e.fromState} → {e.toState} ·{" "}
                      {e.detail}
                    </li>
                  ))}
                </ol>
              </div>
            ))}
          </div>
        ))}
      </div>
      {act.error && <p className="text-destructive">{act.error.message}</p>}
    </section>
  );
}

function WriteCapabilities() {
  const caps = useExecutionCapabilities();
  if (!caps.data) return null;
  return (
    <details className="rounded-lg border p-3">
      <summary className="cursor-pointer font-medium">What ADX can write to each platform</summary>
      {caps.data.platforms.map((p) => (
        <div key={p.platform} className="mt-2">
          <p className="font-medium">
            {p.platform} · {p.live ? "live writes enabled" : "live writes off"}
          </p>
          {p.note && <p className="text-destructive">{p.note}</p>}
          <p className="text-muted-foreground">
            Implemented: {p.implemented.length > 0 ? p.implemented.join(", ") : "nothing"}.
          </p>
          <p className="text-muted-foreground">
            Not implemented (fails honestly if proposed): {p.notImplemented.join(", ")}.
          </p>
        </div>
      ))}
    </details>
  );
}

// Milestones from the plan's acceptance to delivery as the platform's own
// figures show it. The one-hour mark is met only when impressions exist.
function MilestonesView({ planId, version }: { planId: number; version: number }) {
  const ms = useMilestones(planId, version);
  if (!ms.data) return null;
  const m = ms.data;
  const t = (v: string | null | undefined) => (v ? new Date(v).toLocaleString() : "—");
  return (
    <div className="rounded-md border p-2">
      <p className="font-medium">
        Milestones ·{" "}
        {m.withinOneHour
          ? "delivery observed within an hour of acceptance"
          : "one-hour mark not met"}
      </p>
      <p className="text-muted-foreground">
        plan accepted {t(m.planAcceptedAt)} · first sent {t(m.firstDispatchedAt)} · first accepted
        by platform {t(m.firstSubmittedAt)} · first confirmed {t(m.firstConfirmedAt)} · first
        delivery day {m.firstDeliveryDay ?? "none observed"}
        {typeof m.readyToDeliveryHours === "number"
          ? ` · ${m.readyToDeliveryHours.toFixed(1)} h from acceptance to observed delivery`
          : ""}
      </p>
      <p className="text-muted-foreground">{m.note}</p>
    </div>
  );
}

// Recovery actions for one intent: what reversing can and cannot do,
// compensation for what reached the platform, repair for what it refused.
function Recovery({
  intent,
  isAdmin,
  canEdit,
}: {
  intent: { id: number; state: string };
  isAdmin: boolean;
  canEdit: boolean;
}) {
  const rev = useReversibility(intent.id);
  const compensate = useCompensate();
  const repair = useRepair();
  const [reason, setReason] = useState("");
  const [proposed, setProposed] = useState("{}");
  if (!rev.data) return null;
  const r = rev.data;
  const reached = ["submitted", "externally_confirmed", "confirmed"].includes(intent.state);
  return (
    <div className="mt-1 space-y-1 rounded-md border border-dashed p-2">
      <p className="text-muted-foreground">
        {r.reversible ? "Reversible: " : "Not reversible: "}
        {r.how}
        {typeof r.spentMinor === "number" &&
          ` Spent so far: ${r.currency} ${(r.spentMinor / 100).toLocaleString()} (not recovered by any action here).`}
      </p>
      {isAdmin && reached && r.reversible && (
        <div className="flex flex-wrap items-center gap-1">
          <Input
            className="h-7 max-w-xs"
            placeholder="Why compensate"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
          <Button
            size="xs"
            variant="outline"
            disabled={!reason || compensate.isPending}
            onClick={() => compensate.mutate({ id: intent.id, reason })}
          >
            Compensate (stop further spend)
          </Button>
          {compensate.error && <span className="text-destructive">{compensate.error.message}</span>}
        </div>
      )}
      {canEdit && intent.state === "failed" && (
        <div className="flex flex-wrap items-center gap-1">
          <Input
            className="h-7 flex-1"
            placeholder="Corrected proposed document, as JSON"
            value={proposed}
            onChange={(e) => setProposed(e.target.value)}
          />
          <Button
            size="xs"
            variant="outline"
            disabled={repair.isPending}
            onClick={() => {
              try {
                repair.mutate({
                  id: intent.id,
                  proposed: JSON.parse(proposed),
                  note: "repaired from the launch page",
                });
              } catch {
                /* invalid JSON: leave the field for the person to fix */
              }
            }}
          >
            Repair as a new proposal
          </Button>
          {repair.error && <span className="text-destructive">{repair.error.message}</span>}
          {repair.data && (
            <span className="text-muted-foreground">
              Proposal {repair.data.id} created ({repair.data.state}).
            </span>
          )}
        </div>
      )}
    </div>
  );
}
