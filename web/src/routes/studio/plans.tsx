import { Link } from "@tanstack/react-router";
import { ArrowLeft, Sparkles } from "lucide-react";
import { useState } from "react";
import { useAcceptPlanVersion, useGeneratePlan, usePlan, usePlans } from "@/api/plans";
import { useBriefs } from "@/api/research";
import type {
  ChannelInfo,
  PlanDocument,
  PlanState,
  PlanVersion,
  ValidationIssue,
} from "@/api/types.gen";
import { useSession } from "@/app/session";
import { CountUp } from "@/components/front/count-up";
import { HBars } from "@/components/front/pictures";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, PageHeader, Panel } from "@/components/page";
import { Button, buttonVariants } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { type Status, StatusChip, StatusMark } from "@/components/ui/status-chip";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { VersionEditor } from "@/engine/routes/plans";
import { formatDate, formatMoneyShort, formatRelative } from "@/lib/format";

// Plans are whole dirhams; cents would be noise.
const money = (minor: number, currency: string) =>
  `${currency} ${Math.round(minor / 100).toLocaleString("en")}`;

// "Ready" is ready for acceptance; accepted is content only and authorizes
// no spend. That stays separate on purpose (handover B2.4).
export const PLAN_STATE: Record<PlanState, { status: Status; label: string }> = {
  draft: { status: "pending", label: "Draft" },
  validating: { status: "pending", label: "Validating" },
  needs_input: { status: "caution", label: "Needs input" },
  invalid: { status: "error", label: "Invalid" },
  ready: { status: "caution", label: "Ready to accept" },
  accepted: { status: "ready", label: "Accepted" },
};

const shortDate = (d: string) => formatDate(`${d}T00:00:00Z`, { day: "numeric", month: "short" });

// PlanPage renders at /projects/$projectId/plans/$planId, which keeps the
// project's sections in the sidebar, and at the older /studio/plans/$planId.
export function PlanPage({ planId }: { planId: number }) {
  const id = planId;
  const plan = usePlan(id);
  const plans = usePlans();
  const session = useSession();
  const [picked, setPicked] = useState<number | null>(null);
  const [editing, setEditing] = useState(false);
  if (plan.isPending || plans.isPending) return <LoadingRows rows={8} />;
  if (plan.error) return <ErrorNote error={plan.error} />;
  if (!plan.data || !plans.data) return <LoadingRows rows={8} />;
  const d = plan.data;
  const versions = [...d.versions].sort((a, b) => b.version - a.version);
  const latest = versions[0];
  const v = versions.find((x) => x.version === picked) ?? latest;
  return (
    <>
      {d.plan.campaignId !== null && d.plan.projectId !== null ? (
        <Link
          to="/projects/$projectId/campaigns/$campaignId"
          params={{ projectId: String(d.plan.projectId), campaignId: String(d.plan.campaignId) }}
          search={{ stage: "plan" }}
          className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" /> Campaign
        </Link>
      ) : d.plan.projectId !== null ? (
        <Link
          to="/projects/$projectId/media"
          params={{ projectId: String(d.plan.projectId) }}
          className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" /> Media
        </Link>
      ) : (
        <Link
          to="/projects"
          className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" /> Projects
        </Link>
      )}
      <PageHeader
        title={d.plan.title}
        subtitle={
          v
            ? `v${v.version} of ${versions.length} · ${v.source} · ${formatRelative(v.createdAt)}${v.acceptance ? ` · accepted ${formatRelative(v.acceptance.acceptedAt)}` : ""}`
            : "No version yet"
        }
        actions={
          versions.length > 1 && (
            <Select
              aria-label="Version"
              value={String(v?.version ?? "")}
              onValueChange={(x) => setPicked(Number(x))}
              options={versions.map((x) => ({
                value: String(x.version),
                label: `v${x.version} · ${PLAN_STATE[x.state].label}`,
              }))}
            />
          )
        }
      />
      <div className="space-y-5">
        {v ? (
          <VersionView
            planId={id}
            v={v}
            channels={plans.data.channels}
            latest={v.id === latest?.id}
          />
        ) : (
          <Panel>
            <EmptyState title="Nothing drafted yet">
              Draft the first version from research below.
            </EmptyState>
          </Panel>
        )}
        <Draft planId={id} brandId={d.plan.brandId} onDone={(n) => setPicked(n)} />
        {v && session.role !== "viewer" && (
          <Panel
            title="Edit by hand"
            description="Change allocations, assumptions or the period yourself and save it as the next version. The check runs as you type."
            action={
              <Button variant="outline" size="sm" onClick={() => setEditing((e) => !e)}>
                {editing ? "Close" : `Edit v${v.version}`}
              </Button>
            }
          >
            {editing ? (
              <VersionEditor
                key={v.id}
                planId={id}
                brandId={d.plan.brandId}
                version={v}
                channels={plans.data.channels}
                canEdit
                isAdmin={session.role === "admin"}
              />
            ) : (
              <p className="text-sm text-muted-foreground">
                Saving makes a new version; nothing accepted changes.
              </p>
            )}
          </Panel>
        )}
      </div>
    </>
  );
}

function VersionView({
  planId,
  v,
  channels,
  latest,
}: {
  planId: number;
  v: PlanVersion;
  channels: ChannelInfo[];
  latest: boolean;
}) {
  const session = useSession();
  const accept = useAcceptPlanVersion(planId);
  const doc = v.document;
  const label = (key: string) => channels.find((c) => c.key === key)?.label ?? key;
  const st = PLAN_STATE[v.state];
  const unproven = doc.assumptions.filter((a) => a.class !== "cited").length;
  const canAccept = session.role === "admin" && v.state === "ready" && latest;
  return (
    <>
      <section className="grid animate-rise gap-8 border bg-card px-7 py-8 md:grid-cols-[1.2fr_1fr] md:px-9">
        <div>
          <StatusChip status={st.status}>{st.label}</StatusChip>
          <p className="mt-4 flex items-baseline gap-3 font-heading text-6xl font-semibold tracking-[-0.035em] tabular-nums md:text-7xl">
            <CountUp
              value={formatMoneyShort(doc.budgetMinor, doc.currency).replace(
                `${doc.currency} `,
                "",
              )}
              to={doc.budgetMinor / 100 / 1000}
            />
            <span className="text-xl font-medium tracking-normal text-muted-foreground">
              {doc.currency}
            </span>
          </p>
          <h2 className="mt-5 font-heading text-2xl font-semibold leading-tight">
            {doc.objective || "No objective set"}
          </h2>
          <p className="mt-2 max-w-[52ch] text-[0.95rem] leading-relaxed text-muted-foreground">
            {shortDate(doc.periodFrom)} to {shortDate(doc.periodTo)}
            {doc.notes && ` · ${doc.notes}`}
          </p>
          <div className="mt-5 flex flex-wrap items-center gap-2">
            {canAccept && (
              <Button disabled={accept.isPending} onClick={() => accept.mutate(v.version)}>
                Accept v{v.version}
              </Button>
            )}
            {v.state === "accepted" && (
              <Link to="/studio/launch" className={buttonVariants()}>
                Go to launch
              </Link>
            )}
            <span className="text-sm text-muted-foreground">
              {v.validation.blocking.length > 0
                ? `${v.validation.blocking.length} ${v.validation.blocking.length === 1 ? "thing blocks" : "things block"} acceptance`
                : unproven > 0
                  ? `${unproven} ${unproven === 1 ? "assumption" : "assumptions"} not backed by evidence`
                  : "Every assumption cites evidence"}
            </span>
            {accept.error && (
              <span className="text-sm text-destructive">{accept.error.message}</span>
            )}
          </div>
        </div>
        <div className="flex flex-col justify-center gap-3">
          <Eyebrow>By channel</Eyebrow>
          {doc.allocations.length === 0 ? (
            <p className="text-sm text-muted-foreground">No allocations yet.</p>
          ) : (
            <HBars
              rows={doc.allocations.map((a) => ({
                label: label(a.channel),
                value: `${Math.round((a.budgetMinor / Math.max(doc.budgetMinor, 1)) * 100)}%`,
                share: (a.budgetMinor / Math.max(doc.budgetMinor, 1)) * 100,
              }))}
              labelWidth="8rem"
            />
          )}
        </div>
      </section>

      {(v.validation.blocking.length > 0 || v.validation.warnings.length > 0) && (
        <Panel
          title="What the check found"
          description={`Allocated ${money(v.validation.allocatedMinor, doc.currency)} of ${money(doc.budgetMinor, doc.currency)}.`}
          className={v.validation.blocking.length > 0 ? "border-status-error/50" : undefined}
          bodyClassName="p-0"
        >
          <ul className="divide-y">
            {v.validation.blocking.map((i) => (
              <Issue key={`${i.code}-${i.path}`} issue={i} blocking />
            ))}
            {v.validation.warnings.map((i) => (
              <Issue key={`${i.code}-${i.path}`} issue={i} />
            ))}
          </ul>
        </Panel>
      )}

      <div className="grid gap-5 lg:grid-cols-2">
        <Panel title="Allocations" bodyClassName="p-0">
          <Table>
            <THead>
              <TR>
                <TH>Channel</TH>
                <TH>Objective</TH>
                <TH numeric>Budget</TH>
                <TH numeric>Expected per lead</TH>
              </TR>
            </THead>
            <TBody>
              {doc.allocations.map((a) => (
                <TR key={`${a.channel}|${a.objective}|${a.audience}|${a.geography}`}>
                  <TD>
                    <span className="font-medium">{label(a.channel)}</span>
                    {(a.geography || a.audience) && (
                      <span className="block text-xs text-muted-foreground">
                        {[a.geography, a.audience].filter(Boolean).join(" · ")}
                      </span>
                    )}
                  </TD>
                  <TD>{a.objective}</TD>
                  <TD numeric>{money(a.budgetMinor, doc.currency)}</TD>
                  <TD
                    numeric
                    className={a.expectedCplMinor === null ? "text-muted-foreground" : ""}
                  >
                    {a.expectedCplMinor === null
                      ? "Not available"
                      : money(a.expectedCplMinor, doc.currency)}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </Panel>

        <Panel
          title="Assumptions"
          description="What the plan relies on without proof. A cited one points at evidence."
          bodyClassName="p-0"
        >
          {doc.assumptions.length === 0 ? (
            <EmptyState title="None recorded" />
          ) : (
            <ul className="divide-y">
              {doc.assumptions.map((a) => (
                <li key={a.key} className="flex items-start gap-3 px-5 py-3 text-sm">
                  <StatusMark
                    status={
                      a.class === "cited"
                        ? "ready"
                        : a.class === "unavailable"
                          ? "pending"
                          : "caution"
                    }
                    className="mt-1.5"
                  />
                  <span className="min-w-0 flex-1">
                    {a.text}
                    {a.value && (
                      <span className="ml-2 font-mono text-xs text-muted-foreground">
                        {a.value}
                      </span>
                    )}
                  </span>
                  <span className="shrink-0 font-mono text-xs text-muted-foreground">
                    {a.class === "cited"
                      ? "Cited"
                      : a.class === "unavailable"
                        ? "Nobody has it"
                        : "Assumed"}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Panel>

        <Panel title="What must exist first" bodyClassName="p-0">
          <Requirements doc={doc} />
        </Panel>

        <Panel title="Tests and windows" bodyClassName="p-0">
          {doc.testHypotheses.length === 0 && doc.observationWindows.length === 0 ? (
            <EmptyState title="No tests planned" />
          ) : (
            <ul className="divide-y">
              {doc.testHypotheses.map((t) => (
                <li key={t.hypothesis} className="px-5 py-3 text-sm">
                  <p>{t.hypothesis}</p>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    Measured by {t.metric} over {t.windowDays} days
                  </p>
                </li>
              ))}
              {doc.observationWindows.map((w) => (
                <li key={w.name} className="flex justify-between px-5 py-3 text-sm">
                  <span>{w.name}</span>
                  <span className="font-mono text-xs text-muted-foreground">{w.days} days</span>
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>
    </>
  );
}

function Issue({ issue: i, blocking }: { issue: ValidationIssue; blocking?: boolean }) {
  return (
    <li className="flex items-start gap-3 px-5 py-2.5 text-sm">
      <StatusMark status={blocking ? "error" : "caution"} className="mt-1.5" />
      <span className="min-w-0 flex-1">{i.message}</span>
      {i.path && <span className="shrink-0 font-mono text-xs text-muted-foreground">{i.path}</span>}
    </li>
  );
}

function Requirements({ doc }: { doc: PlanDocument }) {
  const rows = [
    ...doc.assetRequirements.map((r) => ({
      k: "Creative",
      t: `${r.count} × ${r.format} · ${r.language}${r.note ? ` · ${r.note}` : ""}`,
    })),
    ...doc.pageRequirements.map((r) => ({
      k: r.kind === "thank_you" ? "Thank-you page" : "Landing page",
      t: `${r.language}${r.url ? ` · ${r.url}` : ""}${r.note ? ` · ${r.note}` : ""}`,
    })),
    ...doc.measurementRequirements.map((r) => ({
      k: "Measurement",
      t: `${r.name} (${r.kind}, ${r.platform})${r.required ? "" : " · optional"}`,
    })),
  ];
  if (rows.length === 0) return <EmptyState title="Nothing listed" />;
  return (
    <ul className="divide-y">
      {rows.map((r) => (
        <li key={`${r.k}:${r.t}`} className="flex gap-3 px-5 py-2.5 text-sm">
          <span className="w-28 shrink-0 text-muted-foreground">{r.k}</span>
          <span className="min-w-0">{r.t}</span>
        </li>
      ))}
    </ul>
  );
}

// Draft asks Adx for a version from the brand's research. The model fills a
// frame; code pins budget and period, drops evidence it can't find and
// rejects channels it can't run. Nothing drafted is accepted.
function Draft({
  planId,
  brandId,
  onDone,
}: {
  planId: number;
  brandId: number;
  onDone: (version: number) => void;
}) {
  const briefs = useBriefs();
  const generate = useGeneratePlan(planId);
  const [instructions, setInstructions] = useState("");
  const mine = briefs.data?.briefs.filter((b) => b.brandId === brandId) ?? [];
  const [chosen, setChosen] = useState<number[] | null>(null);
  const picked = chosen ?? mine.map((b) => b.id);
  return (
    <Panel
      title="Draft the next version"
      description="From the research briefs ticked. Adx proposes; the check and you decide."
    >
      <div className="grid gap-4 md:grid-cols-[1fr_auto]">
        <div className="space-y-3">
          {mine.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No research for this brand yet. Start it from the brand page.
            </p>
          ) : (
            <ul className="space-y-1.5">
              {mine.map((b) => (
                <li key={b.id}>
                  <label className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={picked.includes(b.id)}
                      onChange={(e) =>
                        setChosen(
                          e.target.checked ? [...picked, b.id] : picked.filter((x) => x !== b.id),
                        )
                      }
                    />
                    {b.title}
                  </label>
                </li>
              ))}
            </ul>
          )}
          <textarea
            value={instructions}
            onChange={(e) => setInstructions(e.target.value)}
            rows={2}
            placeholder="Anything to steer it: lean on Meta, keep Snapchat under 10%, test the cash offer first."
            className="w-full resize-y border border-input bg-transparent px-2.5 py-2 text-sm outline-none placeholder:text-muted-foreground hover:border-ring focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
          />
        </div>
        <div className="flex flex-col items-start gap-2">
          <Button
            disabled={generate.isPending || picked.length === 0}
            onClick={() =>
              generate.mutate(
                { briefIds: picked, instructions },
                { onSuccess: (g) => onDone(g.version.version) },
              )
            }
          >
            <Sparkles /> {generate.isPending ? "Drafting…" : "Draft a version"}
          </Button>
          {generate.error && <p className="text-xs text-destructive">{generate.error.message}</p>}
          {generate.data && (
            <ul className="text-xs text-muted-foreground">
              {generate.data.notes.map((n) => (
                <li key={n.code}>{n.message}</li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </Panel>
  );
}
