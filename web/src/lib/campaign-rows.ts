// Perform › Campaigns rows from the engine. Identity, status, project and
// the plan behind each campaign come from GET /api/launch/coverage
// (server/core/launch/coverage.go); the figures from GET /api/analytics
// grouped by campaign (server/core/reporting/metrics.go) for the scope's
// window. The two are joined here by the engine's campaign id, so the chips
// and the figures on a row describe the same campaign. What the engine does
// not have is null, shown as "Not available", never 0.

import type { Campaign, CampaignFigures, Platform, Scope } from "@/api/models";
import type { CampaignCoverage, ReportingResult, ReportRow } from "@/api/types.gen";
import { dateShift, dubaiToday } from "./analytics";
import { ENGINE_PLATFORM } from "./coverage";

// The scope's window: the last N completed reporting days, ending yesterday
// in Dubai, since today is still in progress there.
export function scopeWindow(scope: Pick<Scope, "days">, today = dubaiToday()) {
  const to = dateShift(today, -1);
  return { from: dateShift(to, -(scope.days - 1)), to };
}

const SCREEN_PLATFORM: Record<string, Platform> = Object.fromEntries(
  Object.entries(ENGINE_PLATFORM).map(([screen, engine]) => [engine, screen as Platform]),
);

// The platform's channel type as a marketer says it. Anything unlisted is
// shown as the platform wrote it, in words.
const CHANNEL_LABEL: Record<string, string> = {
  SEARCH: "Search",
  PERFORMANCE_MAX: "Performance Max",
  DEMAND_GEN: "Demand Gen",
  DISPLAY: "Display",
  VIDEO: "Video",
  SHOPPING: "Shopping",
  OUTCOME_LEADS: "Leads",
  OUTCOME_SALES: "Sales",
  OUTCOME_TRAFFIC: "Traffic",
  OUTCOME_AWARENESS: "Awareness",
  OUTCOME_ENGAGEMENT: "Engagement",
  OUTCOME_APP_PROMOTION: "App promotion",
};

export function channelLabel(channelType: string): string {
  if (!channelType) return "Campaign";
  const known = CHANNEL_LABEL[channelType];
  if (known) return known;
  const words = channelType
    .toLowerCase()
    .replace(/^outcome_/, "")
    .split("_");
  return words.map((w, i) => (i === 0 ? w.charAt(0).toUpperCase() + w.slice(1) : w)).join(" ");
}

// Figures from one report row. A null in the engine's row stays null. A
// campaign with no row at all got no figures from the engine in the window:
// everything is unknown, including leads, since the engine made no claim.
export function figuresOf(row: ReportRow | undefined): CampaignFigures {
  if (!row)
    return { spendMinor: null, leads: null, qualified: null, deals: null, revenueMinor: null };
  const m = row.metrics;
  return {
    spendMinor: m.spend.minor,
    leads: m.leads,
    qualified: m.qualifiedLeads,
    deals: m.deals,
    revenueMinor: m.revenue.minor,
  };
}

// Whether a coverage entry is in the scope. The engine's analytics are
// already asked for the scope's platform and project; the coverage list is
// the whole workspace, so it is narrowed here the same way.
export function inScope(cov: CampaignCoverage, scope: Pick<Scope, "platform" | "projectId">) {
  if (scope.platform !== "all" && SCREEN_PLATFORM[cov.platform] !== scope.platform) return false;
  if (scope.projectId !== "all" && cov.projectId !== scope.projectId) return false;
  return true;
}

// The screen's rows: one per campaign the engine has read in the scope,
// with its figures from the report. A campaign the platform reports as not
// running is paused; the engine has no "limited by budget" reading yet.
export function campaignRows(
  coverage: CampaignCoverage[],
  report: Pick<ReportingResult, "rows">,
  scope: Pick<Scope, "platform" | "projectId">,
): Campaign[] {
  const byId = new Map<number, ReportRow>();
  for (const row of report.rows) {
    if (row.campaignId !== null) byId.set(row.campaignId, row);
  }
  return coverage
    .filter((cov) => inScope(cov, scope))
    .map((cov) => ({
      id: String(cov.id),
      name: cov.name,
      platform: SCREEN_PLATFORM[cov.platform] ?? "google",
      externalId: cov.externalId,
      type: channelLabel(cov.channelType),
      projectId: cov.projectId,
      status: cov.running ? "active" : "paused",
      ...figuresOf(byId.get(cov.id)),
      temperatureMix: null,
      daily: null,
      adSets: null,
    }));
}

// A group's or the screen's figures: the sum of the rows that have one,
// unknown only when none has it. This is the engine's own summary rule
// (a row without platform figures adds nothing; mixed currencies make the
// sum unknown), so a project's total agrees with Analytics.
export function sumFigures(list: CampaignFigures[]): CampaignFigures {
  const sum = (key: keyof CampaignFigures) => {
    const known = list.map((c) => c[key]).filter((v): v is number => v !== null);
    return known.length ? known.reduce((a, v) => a + v, 0) : null;
  };
  return {
    spendMinor: sum("spendMinor"),
    leads: sum("leads"),
    qualified: sum("qualified"),
    deals: sum("deals"),
    revenueMinor: sum("revenueMinor"),
  };
}

// Cost per lead or per qualified lead. Null when either side is unknown or
// there is nothing to divide by; the cell says which.
export function costPer(spendMinor: number | null, count: number | null): number | null {
  if (spendMinor === null || count === null || count <= 0) return null;
  return Math.round(spendMinor / count);
}

export function returnOnSpend(c: CampaignFigures): number | null {
  if (c.revenueMinor === null || c.spendMinor === null || c.spendMinor <= 0) return null;
  return c.revenueMinor / c.spendMinor;
}
