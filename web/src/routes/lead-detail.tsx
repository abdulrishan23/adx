import { getRouteApi, Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { type ReactNode, useState } from "react";
import { useLead } from "@/api/leads";
import { useProjects } from "@/api/projects";
import type {
  Candidate,
  Enquiry,
  Lead,
  LeadAttribution,
  LeadDetail,
  PersonDetail,
  Link as PersonLink,
  ScoreView,
} from "@/api/types.gen";
import { useSession } from "@/app/session";
import { EmptyState, ErrorNote, LoadingRows, Panel } from "@/components/page";
import { SourceName } from "@/components/platform";
import { Button } from "@/components/ui/button";
import { StatusChip, StatusMark } from "@/components/ui/status-chip";
import {
  useDecideCandidate,
  usePerson,
  useReverseLink,
  useSyncCRM,
  useUnmergePerson,
} from "@/engine/api/crm";
import { usePersonScore } from "@/engine/api/signals";
import { BriefPanel } from "@/engine/routes/brief";
import { formatDate, formatDateTime, formatRelative, NOT_AVAILABLE } from "@/lib/format";
import { lastActivity, leadName, matchLabel, STAGE_LABEL } from "@/lib/leads";
import { StageCell, StatusCell, TemperatureCell } from "./leads";

const route = getRouteApi("/app/leads/$leadId");

// One lead as GoHighLevel gave it (core/leads), the person it resolved to
// with their enquiries, matches and what the customer said (core/crm), the
// score with its explanation and the events behind it (core/signals), and
// the sales brief per enquiry (core/outreach). A figure the engine does not
// have reads as not available; a lead nobody has resolved says so and offers
// to resolve the records.
export function LeadDetailPage() {
  const { leadId } = route.useParams();
  const lead = useLead(Number(leadId));
  return (
    <>
      <Link
        to="/leads"
        className="mb-6 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" /> All leads
      </Link>
      {lead.isPending && <LoadingRows rows={8} />}
      {lead.error && <ErrorNote error={lead.error} />}
      {lead.data === null && (
        <Panel>
          <EmptyState title="This lead doesn't exist">
            It may have been removed from GoHighLevel since the last import.
          </EmptyState>
        </Panel>
      )}
      {lead.data && <LeadBody detail={lead.data} />}
    </>
  );
}

function LeadBody({ detail }: { detail: LeadDetail }) {
  const lead = detail.lead;
  const projects = useProjects();
  const session = useSession();
  const canEdit = session.role === "admin" || session.role === "marketer";
  const person = usePerson(lead.personId);
  const score = usePersonScore(lead.personId ?? 0, lead.personId !== null);
  const project =
    projects.data?.projects.find((p) => p.id === lead.projectId)?.name ?? lead.projectRaw ?? null;
  return (
    <div className="space-y-6">
      <header className="space-y-2">
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-3xl font-semibold">{leadName(lead)}</h1>
          <TemperatureCell lead={lead} />
        </div>
        <p className="text-muted-foreground">
          {lead.stage ? STAGE_LABEL[lead.stage] : "No stage"}
          {" · "}
          {lead.statusRaw ?? "No status in GoHighLevel"}
          {" · "}
          {project ?? "No project"}
        </p>
      </header>

      <div className="grid gap-4 lg:grid-cols-3">
        <Panel title="Lead">
          <dl className="space-y-2.5 text-sm">
            <Field label="Phone">{lead.phone ?? <Muted>No phone</Muted>}</Field>
            <Field label="Email">{lead.email ?? <Muted>No email</Muted>}</Field>
            <Field label="Project">{project ?? <Muted>No project</Muted>}</Field>
            <Field label="Status">
              <StatusCell lead={lead} />
            </Field>
            <Field label="Stage">
              <StageCell lead={lead} />
            </Field>
            <Field label="Added in GoHighLevel">
              {lead.providerAddedAt ? (
                formatDate(lead.providerAddedAt, {
                  day: "numeric",
                  month: "short",
                  year: "numeric",
                })
              ) : (
                <Muted>{NOT_AVAILABLE}</Muted>
              )}
            </Field>
            <Field label="Last activity">{formatRelative(lastActivity(lead))}</Field>
            <Field label="Last read">{formatDateTime(lead.lastSeenAt)}</Field>
            <Field label="GoHighLevel id">
              <span className="font-mono text-xs">{lead.externalId}</span>
            </Field>
          </dl>
        </Panel>

        <Panel title="Fit, intent and temperature">
          {lead.personId === null ? (
            <Unresolved lead={lead} canEdit={canEdit} />
          ) : (
            <ScorePanel lead={lead} score={score.data ?? null} error={score.error} />
          )}
        </Panel>

        <Panel title="Source">
          <dl className="space-y-2.5 text-sm">
            <Field label="Source in GoHighLevel">
              {lead.sourceRaw ? <SourceName name={lead.sourceRaw} /> : <Muted>No source</Muted>}
            </Field>
            <Field label="Counted as">
              {lead.source ?? (
                <Muted>{lead.sourceRaw ? "Not in the sources table yet" : NOT_AVAILABLE}</Muted>
              )}
            </Field>
            <Field label="Match">{matchLabel(lead.matchLevel)}</Field>
          </dl>
          <Attributions attributions={detail.attributions} />
        </Panel>
      </div>

      {lead.personId !== null && (
        <>
          {person.isPending && <LoadingRows rows={4} />}
          {person.error && <ErrorNote error={person.error} />}
          {person.data && (
            <PersonPanels
              personId={lead.personId}
              person={person.data}
              score={score.data ?? null}
              canEdit={canEdit}
              isAdmin={session.role === "admin"}
            />
          )}
        </>
      )}
    </div>
  );
}

// A lead the resolver has not linked to a person yet: nothing is scored
// until it has. Resolving links every unlinked lead and form submission,
// conservatively (shared phones never merge on their own).
function Unresolved({ lead, canEdit }: { lead: Lead; canEdit: boolean }) {
  const sync = useSyncCRM();
  return (
    <div className="space-y-3 text-sm">
      <p className="text-muted-foreground">
        Not resolved to a person yet, so there is no score. {leadName(lead)} is linked to a person
        when the records are resolved.
      </p>
      {canEdit && (
        <Button size="sm" variant="outline" disabled={sync.isPending} onClick={() => sync.mutate()}>
          {sync.isPending ? "Resolving" : "Resolve new records"}
        </Button>
      )}
      {sync.data && (
        <p className="text-muted-foreground" role="status">
          Linked {sync.data.linked} record{sync.data.linked === 1 ? "" : "s"}.
        </p>
      )}
      {sync.error && <ErrorNote error={sync.error} />}
    </div>
  );
}

const figure = (n: number | null) => (n === null ? <Muted>{NOT_AVAILABLE}</Muted> : n);

function ScorePanel({
  lead,
  score,
  error,
}: {
  lead: Lead;
  score: ScoreView | null;
  error: Error | null;
}) {
  if (error) return <ErrorNote error={error} />;
  if (!score) return <LoadingRows rows={3} />;
  const latest = score.latest;
  if (!latest) {
    return <p className="text-sm text-muted-foreground">Not assessed yet: no signals recorded.</p>;
  }
  const drivers = [...latest.explanation.fit, ...latest.explanation.intent];
  return (
    <div className="space-y-3 text-sm">
      <div className="flex flex-wrap items-center gap-3">
        <TemperatureCell lead={lead} />
        <span className="text-muted-foreground">·</span>
        <span>
          Fit <strong className="font-medium">{figure(latest.fit)}</strong>
        </span>
        <span className="text-muted-foreground">·</span>
        <span>
          Intent <strong className="font-medium">{figure(latest.intent)}</strong>
        </span>
      </div>
      <p className="text-xs text-muted-foreground">
        Rules v{latest.rulesVersion ?? "none"} · {latest.trigger} ·{" "}
        {formatRelative(latest.assessedAt)}
      </p>
      {latest.explanation.unconfigured.length > 0 && (
        <p className="flex items-baseline gap-2 text-muted-foreground">
          <StatusMark status="caution" className="translate-y-[-1px]" />
          Not configured: {latest.explanation.unconfigured.join(", ")}.
        </p>
      )}
      {!latest.explanation.fitAvailable && latest.rulesVersion !== null && (
        <p className="text-muted-foreground">
          Fit is not available: none of the fit attributes are known for this person.
        </p>
      )}
      {drivers.length === 0 ? (
        <p className="text-muted-foreground">No drivers yet.</p>
      ) : (
        <ul className="space-y-2">
          {latest.explanation.fit.map((f) => (
            <li key={`fit-${f.attribute}`} className="flex items-start justify-between gap-3">
              <span>
                {f.attribute} = {f.value}
                <span className="block text-xs text-muted-foreground">Fit · {f.class}</span>
              </span>
              <span className="shrink-0 font-mono text-xs">{f.points}</span>
            </li>
          ))}
          {latest.explanation.intent.map((i) => (
            <li key={`intent-${i.kind}`} className="flex items-start justify-between gap-3">
              <span>
                {i.kind}
                <span className="block text-xs text-muted-foreground">
                  Intent · {i.events} event{i.events === 1 ? "" : "s"}, raw {i.raw}, cap {i.cap}
                </span>
              </span>
              <span className="shrink-0 font-mono text-xs">{i.points}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function Attributions({ attributions }: { attributions: LeadAttribution[] }) {
  if (attributions.length === 0) {
    return (
      <p className="mt-4 border-t pt-4 text-sm text-muted-foreground">
        No attribution record from GoHighLevel.
      </p>
    );
  }
  return (
    <ul className="mt-4 space-y-3 border-t pt-4 text-sm">
      {attributions.map((a) => (
        <li key={a.id}>
          <p className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{a.isFirst ? "First touch" : "Last touch"}</span>
            <span className="text-muted-foreground">
              {matchLabel(a.matchLevel)}
              {a.matchConfidence !== "none" && ` · ${a.matchConfidence} confidence`}
            </span>
          </p>
          <p className="text-xs text-muted-foreground">
            {[
              a.matchedPlatform && `Platform ${a.matchedPlatform}`,
              // A click id's reference is the id itself, not a campaign.
              a.matchedCampaignRef &&
                a.matchLevel !== "click_id" &&
                `Campaign ${a.matchedCampaignRef}`,
              a.utmSource && `utm_source ${a.utmSource}`,
              a.utmMedium && `utm_medium ${a.utmMedium}`,
              a.hasClickId && "Click id recorded",
              a.referrer && `From ${a.referrer}`,
            ]
              .filter(Boolean)
              .join(" · ") || "No campaign reference"}
          </p>
        </li>
      ))}
    </ul>
  );
}

function PersonPanels({
  personId,
  person: d,
  score,
  canEdit,
  isAdmin,
}: {
  personId: number;
  person: PersonDetail;
  score: ScoreView | null;
  canEdit: boolean;
  isAdmin: boolean;
}) {
  const projects = useProjects();
  const unmerge = useUnmergePerson();
  const projectName = (id: number | null) =>
    projects.data?.projects.find((p) => p.id === id)?.name ?? (id ? `Project ${id}` : "No project");
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Panel
        title="Person"
        description={
          <>
            {d.person.primaryPhone ?? "no phone"} · {d.person.primaryEmail ?? "no email"} · first
            seen{" "}
            {formatDate(d.person.createdAt, { day: "numeric", month: "short", year: "numeric" })}
          </>
        }
      >
        {d.person.mergedIntoId !== null && (
          <p className="mb-4 flex flex-wrap items-center gap-2 text-sm">
            <StatusChip status="caution">Merged</StatusChip>
            <span>Merged into person {d.person.mergedIntoId}.</span>
            {canEdit && (
              <Button
                size="xs"
                variant="outline"
                disabled={unmerge.isPending}
                onClick={() =>
                  unmerge.mutate({ id: personId, reason: "undone from the lead page" })
                }
              >
                Undo merge
              </Button>
            )}
          </p>
        )}
        {unmerge.error && <ErrorNote error={unmerge.error} />}
        <Enquiries
          enquiries={d.enquiries}
          projectName={projectName}
          canEdit={canEdit}
          isAdmin={isAdmin}
        />
        <Matches
          links={d.links}
          candidates={d.candidates}
          canEdit={canEdit}
          personName={d.person.displayName}
        />
        <Attributes person={d} />
      </Panel>

      <Panel title="Timeline">
        <Timeline score={score} />
      </Panel>
    </div>
  );
}

function Enquiries({
  enquiries,
  projectName,
  canEdit,
  isAdmin,
}: {
  enquiries: Enquiry[];
  projectName: (id: number | null) => string;
  canEdit: boolean;
  isAdmin: boolean;
}) {
  const [open, setOpen] = useState<number | null>(null);
  return (
    <section className="space-y-2">
      <h3 className="text-sm font-semibold">Enquiries</h3>
      {enquiries.length === 0 && <Muted>None yet.</Muted>}
      <ul className="space-y-3 text-sm">
        {enquiries.map((e) => (
          <li key={e.id} className="space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{projectName(e.projectId)}</span>
              <span className="text-muted-foreground">
                {e.state}
                {e.stage ? ` · ${e.stage}` : ""} · from {e.sourceKind} · first{" "}
                {formatDate(e.firstContactAt)} · last {formatRelative(e.lastActivityAt)}
              </span>
              <Button
                size="xs"
                variant="outline"
                onClick={() => setOpen(open === e.id ? null : e.id)}
              >
                {open === e.id ? "Hide brief" : "Sales brief"}
              </Button>
            </div>
            {open === e.id && <BriefPanel enquiryId={e.id} canEdit={canEdit} isAdmin={isAdmin} />}
          </li>
        ))}
      </ul>
    </section>
  );
}

// What matched this person: the records linked to them, each with the
// evidence and a way to say it was the wrong person, and the uncertain
// matches waiting for someone to decide. Nothing merges on its own.
function Matches({
  links,
  candidates,
  canEdit,
  personName,
}: {
  links: PersonLink[];
  candidates: Candidate[];
  canEdit: boolean;
  personName: string;
}) {
  const reverse = useReverseLink();
  const decide = useDecideCandidate();
  return (
    <section className="mt-5 space-y-2 border-t pt-4">
      <h3 className="text-sm font-semibold">Matches</h3>
      {candidates.length > 0 && (
        <div className="space-y-2 rounded-md border border-status-caution/60 p-3 text-sm">
          <p className="flex items-baseline gap-2">
            <StatusMark status="caution" className="translate-y-[-1px]" />
            {candidates.length === 1
              ? "1 record might be the same person."
              : `${candidates.length} records might be the same person.`}{" "}
            <span className="text-muted-foreground">
              They share something with {personName || "this person"} but not enough to be sure.
            </span>
          </p>
          {candidates.map((c) => (
            <div key={c.id} className="flex flex-wrap items-center gap-2 border-t pt-2">
              <span>
                {c.other
                  ? `${c.other.displayName || "(no name)"} · ${c.other.primaryPhone ?? ""} ${c.other.primaryEmail ?? ""}`
                  : `Person ${c.candidateId}`}
              </span>
              <span className="text-xs text-muted-foreground">Why: {reasonText(c.reason)}</span>
              {canEdit && (
                <>
                  <Button
                    size="xs"
                    disabled={decide.isPending}
                    onClick={() => decide.mutate({ id: c.id, accept: true, note: "same person" })}
                  >
                    Same person, merge
                  </Button>
                  <Button
                    size="xs"
                    variant="outline"
                    disabled={decide.isPending}
                    onClick={() =>
                      decide.mutate({ id: c.id, accept: false, note: "different people" })
                    }
                  >
                    Different people
                  </Button>
                </>
              )}
            </div>
          ))}
          {decide.error && <ErrorNote error={decide.error} />}
        </div>
      )}
      <ul className="space-y-1.5 text-sm">
        {links.map((l) => (
          <li
            key={l.id}
            className={
              l.reversedAt
                ? "flex flex-wrap items-center gap-2 text-muted-foreground line-through"
                : "flex flex-wrap items-center gap-2"
            }
          >
            <span>
              {l.kind === "lead" ? "GoHighLevel lead" : l.kind} {l.ref}
            </span>
            <span className="text-xs text-muted-foreground no-underline">
              matched as {l.matchLevel} · {reasonText(l.matchEvidence)}
              {l.reversedAt && ` · reversed: ${l.reversalReason}`}
            </span>
            {canEdit && !l.reversedAt && (
              <Button
                size="xs"
                variant="outline"
                disabled={reverse.isPending}
                onClick={() => reverse.mutate({ id: l.id, reason: "wrong person" })}
              >
                Not this person
              </Button>
            )}
          </li>
        ))}
      </ul>
      {reverse.error && <ErrorNote error={reverse.error} />}
    </section>
  );
}

// The engine's evidence is a small JSON object; shown as "key value" pairs
// rather than braces.
function reasonText(raw: unknown): string {
  if (raw && typeof raw === "object") {
    const pairs = Object.entries(raw as Record<string, unknown>).map(
      ([k, v]) => `${k} ${typeof v === "string" ? v : JSON.stringify(v)}`,
    );
    return pairs.join(", ") || "no detail";
  }
  return raw ? String(raw) : "no detail";
}

function Attributes({ person: d }: { person: PersonDetail }) {
  return (
    <section className="mt-5 space-y-2 border-t pt-4">
      <h3 className="text-sm font-semibold">What we know</h3>
      {d.attributes.length === 0 && <Muted>Nothing recorded.</Muted>}
      <dl className="space-y-1.5 text-sm">
        {d.attributes.map((a) => (
          <div key={a.key} className="flex flex-wrap items-baseline gap-2">
            <dt className="text-muted-foreground">{a.key}</dt>
            <dd>
              {a.value} <span className="text-xs text-muted-foreground">({a.class})</span>
              {a.enriched && a.class === "stated" && (
                <span className="text-xs text-muted-foreground">
                  {" "}
                  · provider says "{a.enriched.value}"{a.conflict ? ", which disagrees" : ""}
                </span>
              )}
            </dd>
          </div>
        ))}
      </dl>
      {!d.enrichment.configured && (
        <p className="text-xs text-muted-foreground">Enrichment: {d.enrichment.reason}</p>
      )}
    </section>
  );
}

// The events the score is built from, newest first, with the ones a person
// discarded struck through and the earlier assessments beneath.
function Timeline({ score }: { score: ScoreView | null }) {
  if (!score) return <LoadingRows rows={4} />;
  const events = [...score.events].sort(
    (a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt),
  );
  return (
    <div className="space-y-4">
      {events.length === 0 ? (
        <p className="text-sm text-muted-foreground">No events recorded yet.</p>
      ) : (
        <ol className="relative space-y-4 border-l border-dashed pl-5">
          {events.map((e) => (
            <li key={e.id} className="relative">
              <span className="absolute top-1.5 -left-[25px] size-2 rounded-none bg-foreground" />
              <div className="flex flex-wrap items-center gap-2">
                <span className={e.discardedAt ? "font-medium line-through" : "font-medium"}>
                  {e.kind}
                </span>
                <StatusChip
                  status={e.confidence === "confirmed" ? "ready" : "pending"}
                  className="h-5 px-1.5 text-[0.6rem]"
                >
                  {e.confidence}
                </StatusChip>
              </div>
              <p className="text-sm text-muted-foreground">
                From {e.source}
                {e.discardedAt && ` · discarded: ${e.discardReason}`}
              </p>
              <p className="font-mono text-[0.7rem] text-muted-foreground">
                {formatRelative(e.occurredAt)}
              </p>
            </li>
          ))}
        </ol>
      )}
      {score.history.length > 1 && (
        <div className="border-t pt-3">
          <h3 className="mb-1.5 text-sm font-semibold">Earlier assessments</h3>
          <ul className="space-y-1 text-xs text-muted-foreground">
            {score.history.slice(1).map((h) => (
              <li key={h.id}>
                {formatDateTime(h.assessedAt)} · fit {h.fit ?? NOT_AVAILABLE} · intent{" "}
                {h.intent ?? NOT_AVAILABLE} · {h.temperature ?? "not assessed"} · v
                {h.rulesVersion ?? "none"} · {h.trigger}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[8.5rem_1fr] gap-2">
      <dt className="text-muted-foreground">{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}

function Muted({ children }: { children: ReactNode }) {
  return <span className="text-muted-foreground">{children}</span>;
}
