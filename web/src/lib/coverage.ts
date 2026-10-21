// What Perform › Campaigns says about a campaign's plan and launch (BD-68).
// The rules are the server's: whether an accepted plan stands behind a
// campaign and where its launch intent is come from GET /api/launch/coverage
// (server/core/launch/coverage.go). What stays here is presentation: which
// row a coverage entry belongs to, which chip it earns, and what a marketer
// may change from the row.

import type { Platform } from "@/api/models";
import type { CampaignCoverage } from "@/api/types.gen";
import type { Status } from "@/components/ui/status-chip";

// The engine names platforms by their API; the screens by their brand.
export const ENGINE_PLATFORM: Record<Platform, string> = { google: "google_ads", meta: "meta" };

// The coverage entry for a row, by platform and the platform's own id. Null
// when Adx has not read the campaign: a sample row, or one the next refresh
// brings in. Null is unknown, not "not under a plan".
export function coverageFor(
  rows: CampaignCoverage[] | undefined,
  c: { platform: Platform; externalId: string },
): CampaignCoverage | null {
  if (!rows) return null;
  const platform = ENGINE_PLATFORM[c.platform];
  return rows.find((r) => r.platform === platform && r.externalId === c.externalId) ?? null;
}

export type Chip = { status: Status; label: string; note: string };

// Launching while the plan's intent waits for approval, sits in the outbox,
// has been sent or has been confirmed; Delivering once the platform's own
// figures show impressions after the send. Nothing for a launch that did
// not go through, or that never started.
export function launchChip(cov: CampaignCoverage | null): Chip | null {
  if (!cov) return null;
  switch (cov.launch.state) {
    case "in_review":
    case "submitted":
    case "confirmed":
      return { status: "caution", label: "Launching", note: cov.launch.note };
    case "delivering":
      return {
        status: "ready",
        label: "Delivering",
        note: cov.launch.deliveryDay
          ? `Delivering since ${cov.launch.deliveryDay}. ${cov.launch.note}`
          : cov.launch.note,
      };
    default:
      return null;
  }
}

// "Not under a plan" for a campaign Adx has read that no accepted plan
// resolves to. Nothing when coverage is unknown.
export function planChip(cov: CampaignCoverage | null): Chip | null {
  if (!cov || cov.underPlan) return null;
  return { status: "pending", label: "Not under a plan", note: cov.reason };
}

export type AllowedChanges = { raise: boolean; lower: boolean; pause: boolean };

// What a marketer may change from the row. A campaign not under a plan
// takes only a pause and a budget cut until a plan adopts it (BD-68 rule
// 4); the server refuses the rest, so the screen does not offer it. Unknown
// coverage limits nothing: the server decides.
export function allowedChanges(cov: CampaignCoverage | null): AllowedChanges {
  if (cov && !cov.underPlan) return { raise: false, lower: true, pause: true };
  return { raise: true, lower: true, pause: true };
}

// One line for the sheet on which plan line stands behind the campaign.
export function planLine(cov: CampaignCoverage | null): string | null {
  if (!cov) return null;
  if (!cov.underPlan || cov.planId === null || cov.planVersion === null) {
    return `Not under a plan: ${cov.reason}.`;
  }
  const line = cov.line === null ? "" : `, line ${cov.line + 1}`;
  const does = cov.kind === "resume" ? "resumes" : "updates";
  return `Plan ${cov.planId} v${cov.planVersion}${line} ${does} it.`;
}
