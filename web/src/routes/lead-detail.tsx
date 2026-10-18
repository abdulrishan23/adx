import { getRouteApi, Link } from "@tanstack/react-router";
import {
  ArrowLeft,
  Database,
  FileText,
  Globe,
  Megaphone,
  MessageCircle,
  Phone,
  Share2,
} from "lucide-react";
import type { ReactNode } from "react";
import { useLead } from "@/api/adx";
import { CAMPAIGNS, projectOf } from "@/api/mock/data";
import type { ActivityEvent, Lead, Signal } from "@/api/models";
import { EmptyState, ErrorNote, Eyebrow, LoadingRows, Panel } from "@/components/page";
import { SourceName } from "@/components/platform";
import { TemperatureMeter } from "@/components/temperature";
import { StatusChip } from "@/components/ui/status-chip";
import { formatDate, formatRelative } from "@/lib/format";
import { released } from "@/lib/release";
import { cn } from "@/lib/utils";
import { LeadBriefPanels } from "./lead-brief";
import { MATCH_LABEL } from "./leads";

const route = getRouteApi("/app/leads/$leadId");

const ROLE_TAG: Record<Signal["role"], { label: string; chip: "ready" | "pending" }> = {
  hot: { label: "Hot trigger", chip: "ready" },
  warm: { label: "Warm trigger", chip: "pending" },
  feature: { label: "Context", chip: "pending" },
};

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
            It may have been merged into another lead as a duplicate.
          </EmptyState>
        </Panel>
      )}
      {lead.data && <LeadBody lead={lead.data} />}
    </>
  );
}

function LeadBody({ lead }: { lead: Lead }) {
  const campaign = CAMPAIGNS.find((c) => c.id === lead.campaignId);
  const project = lead.projectId ? projectOf(lead.projectId) : null;
  const drivers = lead.signals.filter((s) => s.role !== "feature").slice(0, 3);
  return (
    <div className="space-y-6">
      <header className="space-y-2">
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-3xl font-semibold">{lead.name}</h1>
          <TemperatureMeter temperature={lead.temperature} />
        </div>
        <p className="text-muted-foreground">
          {lead.temperature === "cold"
            ? "Cold"
            : `${lead.temperature === "hot" ? "Hot" : "Warm"} for ${project ?? "no project"}`}
          {" · "}
          {lead.reason}
        </p>
      </header>

      {/* Enrichment and the lead brief are phase 4. */}
      {released(4) && <LeadBriefPanels leadId={lead.id} />}

      <div className="grid gap-4 lg:grid-cols-3">
        <Panel title="Lead">
          <dl className="space-y-2.5 text-sm">
            <Field label="Phone code">{lead.phoneCountry}</Field>
            <Field label="Project">{project ?? "—"}</Field>
            <Field label="Status">
              {lead.status ?? <span className="text-muted-foreground">No status yet</span>}
            </Field>
            <Field label="Owner">{lead.owner}</Field>
            <Field label="Arrived">
              {formatDate(lead.arrivedAt, { day: "numeric", month: "short", year: "numeric" })}
            </Field>
          </dl>
        </Panel>

        <Panel title="Drivers">
          <div className="mb-4 flex items-center gap-3">
            <TemperatureMeter temperature={lead.temperature} />
            <span className="text-sm text-muted-foreground">·</span>
            <span className="text-sm">
              Fit <strong className="font-medium capitalize">{lead.fit}</strong>
            </span>
          </div>
          {drivers.length === 0 ? (
            <p className="text-sm text-muted-foreground">No signals yet.</p>
          ) : (
            <ul className="space-y-2.5">
              {drivers.map((s) => (
                <li key={s.id} className="flex items-start justify-between gap-3 text-sm">
                  <span>
                    {s.label}
                    <span className="block text-xs text-muted-foreground">{s.evidence}</span>
                  </span>
                  <span className="shrink-0 font-mono text-xs text-muted-foreground">
                    {formatRelative(s.occurredAt)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Panel>

        <Panel title="Source">
          <dl className="space-y-2.5 text-sm">
            <Field label="Original source">
              <SourceName name={lead.originalSource} />
            </Field>
            {lead.source !== lead.originalSource && (
              <Field label="Labelled in GoHighLevel as">
                {lead.source}{" "}
                <StatusChip status="caution" className="ml-1 h-5 px-1.5 text-[0.6rem]">
                  Recovered
                </StatusChip>
              </Field>
            )}
            <Field label="Campaign">{campaign?.name ?? "—"}</Field>
            <Field label="Match">{MATCH_LABEL[lead.matchLevel]}</Field>
          </dl>
        </Panel>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Panel title="Signal timeline">
          {lead.signals.length === 0 ? (
            <p className="text-sm text-muted-foreground">No signals yet.</p>
          ) : (
            <ol className="relative space-y-4 border-l border-dashed pl-5">
              {lead.signals.map((s, i) => (
                <li key={s.id} className="relative">
                  <span className="absolute top-1.5 -left-[25px] size-2 rounded-none bg-foreground" />
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{s.label}</span>
                    <StatusChip status={ROLE_TAG[s.role].chip} className="h-5 px-1.5 text-[0.6rem]">
                      {ROLE_TAG[s.role].label}
                    </StatusChip>
                    {i === 0 && s.role === "hot" && (
                      <StatusChip status="error" className="h-5 px-1.5 text-[0.6rem]">
                        Act now
                      </StatusChip>
                    )}
                  </div>
                  <p className="text-sm text-muted-foreground">{s.evidence}</p>
                  <p className="font-mono text-[0.7rem] text-muted-foreground">
                    {formatRelative(s.occurredAt)}
                  </p>
                </li>
              ))}
            </ol>
          )}
        </Panel>

        <Panel title="Activity">
          <ul className="space-y-3">
            {lead.activity.map((a) => (
              <ActivityRow key={a.id} event={a} />
            ))}
          </ul>
        </Panel>
      </div>

      {lead.formAnswers.length > 0 && (
        <Panel title="Form answers">
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {lead.formAnswers.map((f) => (
              <div key={f.question}>
                <dt>
                  <Eyebrow>{f.question}</Eyebrow>
                </dt>
                <dd className="mt-1">{f.answer}</dd>
              </div>
            ))}
          </dl>
        </Panel>
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

const CHANNEL_ICON: Record<ActivityEvent["channel"], typeof Globe> = {
  web: Globe,
  whatsapp: MessageCircle,
  call: Phone,
  form: FileText,
  crm: Database,
  social: Share2,
  ad: Megaphone,
};

function ActivityRow({ event }: { event: ActivityEvent }) {
  const Icon = CHANNEL_ICON[event.channel];
  return (
    <li className="flex items-start gap-3 text-sm">
      <span
        className={cn(
          "mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md border bg-background",
        )}
      >
        <Icon className="size-3.5" />
      </span>
      <div className="min-w-0 flex-1">
        <p>{event.label}</p>
        {event.detail && <p className="text-xs text-muted-foreground">{event.detail}</p>}
      </div>
      <time
        className="shrink-0 font-mono text-xs text-muted-foreground"
        dateTime={event.occurredAt}
        title={event.occurredAt}
      >
        {formatRelative(event.occurredAt)}
      </time>
    </li>
  );
}
