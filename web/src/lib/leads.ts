// What the Leads screens work out from the engine's rows (core/leads). The
// rules the engine owns (stage, match level, temperature) arrive on the row;
// this file only reads them, counts them and turns them into words.
import type { Temperature } from "@/api/models";
import type { Coverage, Lead, Stage } from "@/api/types.gen";
import { NOT_AVAILABLE } from "@/lib/format";

export const MATCH_LABEL: Record<string, string> = {
  click_id: "Click ID",
  platform_lead_id: "Platform lead ID",
  hashed_contact: "Phone or email",
  utm: "UTM only",
  none: "Unknown",
};

export const STAGE_LABEL: Record<Stage, string> = {
  open: "Open",
  attempted: "Attempted",
  engaged: "Engaged",
  meeting: "Meeting",
  offer: "Offer",
  won: "Won",
  disqualified: "Disqualified",
  excluded: "Excluded",
  dead: "Dead",
};

export const matchLabel = (level: string) => MATCH_LABEL[level] ?? level;

export function leadName(
  l: Pick<Lead, "firstName" | "lastName" | "email" | "phone" | "externalId">,
) {
  return (
    [l.firstName, l.lastName].filter(Boolean).join(" ").trim() ||
    l.email ||
    l.phone ||
    `Contact ${l.externalId}`
  );
}

// The engine writes hot, warm or cold; anything else (or nothing) is not an
// assessment and never reads as cold.
export function temperatureOf(l: Pick<Lead, "temperature">): Temperature | null {
  return l.temperature === "hot" || l.temperature === "warm" || l.temperature === "cold"
    ? l.temperature
    : null;
}

export type LeadFilter = "no_status" | "no_stage" | "no_project" | "status_missing";

export function matchesFilter(l: Lead, f: LeadFilter): boolean {
  switch (f) {
    case "no_status":
      return l.statusRaw === null;
    case "no_stage":
      return l.stage === null;
    case "no_project":
      return l.projectId === null;
    case "status_missing":
      return l.statusMissingSince !== null;
  }
}

export type TemperatureTab = Temperature | "all";

export function matchesTab(l: Lead, tab: TemperatureTab): boolean {
  return tab === "all" || temperatureOf(l) === tab;
}

// A search spans every temperature, so a name is never hidden by the tab. A
// number typed the local way (050...) finds the international one GoHighLevel
// keeps (+97150...).
export function matchesQuery(l: Lead, q: string): boolean {
  const needle = q.trim().toLowerCase();
  if (!needle) return true;
  if ([leadName(l), l.email ?? "", l.externalId].some((s) => s.toLowerCase().includes(needle))) {
    return true;
  }
  const digits = needle.replace(/\D/g, "").replace(/^0/, "");
  return digits.length >= 4 && (l.phone ?? "").replace(/\D/g, "").includes(digits);
}

export function filterLeads(
  all: Lead[],
  by: { tab: TemperatureTab; filter: LeadFilter | null; query: string },
): Lead[] {
  const q = by.query.trim();
  return all.filter(
    (l) =>
      (q ? matchesQuery(l, q) : matchesTab(l, by.tab)) &&
      (!by.filter || matchesFilter(l, by.filter)),
  );
}

export function temperatureCounts(all: Lead[]): Record<TemperatureTab, number> {
  const counts = { hot: 0, warm: 0, cold: 0, all: all.length };
  for (const l of all) {
    const t = temperatureOf(l);
    if (t) counts[t]++;
  }
  return counts;
}

export type CoverageTile = {
  key: LeadFilter;
  // Null is a figure the engine cannot give yet, shown as Not available.
  count: number | null;
  label: string;
  detail: string;
  urgent: boolean;
};

const share = (part: number, total: number) =>
  total === 0 ? NOT_AVAILABLE : `${Math.round((part / total) * 100)}%`;

// The coverage figures as the four tiles under the headline: each one is a
// gap (leads missing what a figure depends on) with the share that has it.
export function coverageTiles(c: Coverage): CoverageTile[] {
  const noStatus = c.total - c.withStatus;
  return [
    {
      key: "no_status",
      count: noStatus,
      label: "no status in GoHighLevel",
      detail: c.total === 0 ? NOT_AVAILABLE : `${share(c.withStatus, c.total)} have one`,
      urgent: noStatus > 0,
    },
    {
      key: "no_stage",
      count: c.funnelConfigured ? c.total - c.withStage : null,
      label: c.funnelConfigured ? "no stage" : "no stage: table not saved",
      detail: c.funnelConfigured
        ? `${share(c.withStage, c.total)} have one`
        : "Save the status-to-stage table in Stages and lists",
      urgent: false,
    },
    {
      key: "no_project",
      count: c.total - c.withProject,
      label: "no project",
      detail: c.total === 0 ? NOT_AVAILABLE : `${share(c.withProject, c.total)} have one`,
      urgent: false,
    },
    {
      key: "status_missing",
      count: c.statusMissingOnLastScan,
      label: "status field missing on last scan",
      detail:
        c.statusMissingOnLastScan > 0
          ? "GoHighLevel did not send the field; the last known status is kept"
          : "Every lead carried the field",
      urgent: false,
    },
  ];
}

export function headline(c: Coverage): { text: string; filter: LeadFilter | null } {
  if (c.total === 0) return { text: "No leads yet", filter: null };
  const noStatus = c.total - c.withStatus;
  if (noStatus === 0) return { text: "Every lead has a status in GoHighLevel", filter: null };
  return {
    text:
      noStatus === 1
        ? "1 lead has no status in GoHighLevel"
        : `${noStatus} leads have no status in GoHighLevel`,
    filter: "no_status",
  };
}

// The last time GoHighLevel changed the contact, or the last time ADX saw it.
export function lastActivity(l: Pick<Lead, "providerUpdatedAt" | "lastSeenAt">): string {
  return l.providerUpdatedAt ?? l.lastSeenAt;
}
