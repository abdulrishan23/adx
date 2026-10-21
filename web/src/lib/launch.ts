// What the Launch screen says about where a plan version's money goes
// (BD-68). The rules themselves moved to the server on 2026-10-19: which
// campaign a line updates, resumes or picks from, how this month's spend
// nets off the line and spreads over the days left, all live in
// server/core/launch and are tested in launch_test.go. The browser decides
// nothing; it reads GET /api/launch/resolution. What stays here is
// presentation: the figures a resolved line shows, why Submit is disabled,
// which account each platform launches into, and what each submitted
// line's state is called.

import type { LineResolution, ProjectCampaign } from "@/api/types.gen";
import { formatMoneyShort, NOT_AVAILABLE } from "./format";

export type LineFigures = { spent: string; left: string; daily: string };

// The month's figures for an update, resume or create. Spend is "Not
// available" when the campaign has no figures read this month, never 0.
export function lineFigures(l: LineResolution): LineFigures | null {
  if (!l.budget) return null;
  const cur = l.line.currency;
  return {
    spent: l.budget.spendKnown ? formatMoneyShort(l.budget.spentMinor, cur) : NOT_AVAILABLE,
    left: formatMoneyShort(l.budget.remainingMinor, cur),
    daily: formatMoneyShort(l.budget.dailyMinor, cur),
  };
}

// A candidate's spend this month, for the pick list.
export function campaignSpend(c: ProjectCampaign, currency: string): string {
  return c.spentMinor === null
    ? NOT_AVAILABLE
    : formatMoneyShort(c.spentMinor, c.currency || currency);
}

// Why nothing can be submitted yet, or null when it can. A line still
// waiting on a pick stops the whole submission (the server refuses it too),
// and a version with nothing ADX can deploy has nothing to submit.
export function submitBlocker(
  lines: LineResolution[],
  label: (channel: string) => string = (c) => c,
): string | null {
  const open = lines.filter((l) => l.kind === "pick");
  if (open.length > 0) {
    const names = open.map((l) => `line ${l.line.index + 1} (${label(l.line.channel)})`);
    return `${capitalize(names.join(" and "))} still ${open.length === 1 ? "needs" : "need"} a pick.`;
  }
  if (lines.length === 0) {
    return "The version has no lines.";
  }
  if (lines.every((l) => l.kind === "unsupported")) {
    return "Nothing in this version can be launched by Adx.";
  }
  return null;
}

// The account each platform launches into, for opening the version's
// envelopes when none are open yet: the account the line's matched or
// candidate campaign sits in, else the only connected account of the
// platform. A platform with no campaign to go by and more than one account
// is left out, so a person names it rather than Adx guessing.
export function accountsByPlatform(
  lines: LineResolution[],
  accounts: { id: number; provider: string }[],
): Record<string, number> {
  const out: Record<string, number> = {};
  for (const l of lines) {
    if (l.kind === "unsupported" || out[l.line.platform]) continue;
    const known = [l.campaign, ...l.candidates].filter((c): c is ProjectCampaign => c !== null);
    const ids = new Set(known.map((c) => c.accountId));
    if (ids.size === 1) {
      out[l.line.platform] = known[0]?.accountId as number;
      continue;
    }
    const connected = accounts.filter((a) => a.provider === l.line.platform);
    if (connected.length === 1) {
      out[l.line.platform] = connected[0]?.id as number;
    }
  }
  return out;
}

// Platforms a deployable line needs that no account was found for.
export function platformsWithoutAccount(
  lines: LineResolution[],
  byPlatform: Record<string, number>,
): string[] {
  const missing = new Set<string>();
  for (const l of lines) {
    if (l.kind !== "unsupported" && !byPlatform[l.line.platform]) missing.add(l.line.platform);
  }
  return [...missing];
}

export type SubmitTone = "ready" | "caution" | "error" | "pending";

// What Submit did for a line, as the screen names it. Executed means an
// intent is in the outbox, not that anything is live.
export const SUBMIT_STATE: Record<string, { status: SubmitTone; label: string }> = {
  submitted: { status: "ready", label: "Executed" },
  awaiting_approval: { status: "pending", label: "Waiting for approval" },
  denied: { status: "error", label: "Denied" },
  held: { status: "caution", label: "Held" },
  unchanged: { status: "pending", label: "Unchanged" },
  exhausted: { status: "caution", label: "Spent for the month" },
  skipped: { status: "pending", label: "Not launched" },
};

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
