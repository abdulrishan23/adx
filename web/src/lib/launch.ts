// Where a plan's money goes when it launches (BD-68, decided 2026-10-18).
// Create owns the first launch: each plan line is resolved against the
// project's running campaigns before anything is submitted. One match is
// updated; more than one and a person picks, never both unasked; none and a
// campaign is created. The plan is the month's ceiling, so a matched
// campaign's spend this month nets off its line, and the daily budget is what
// remains spread over the days left in the month.

export type LaunchLine = {
  platform: string;
  // A line that names a campaign type (search, lead form) matches only that
  // type; a line without one matches every type on its platform.
  type?: string;
  budgetMinor: number;
};

export type LaunchCampaign = {
  id: string;
  name: string;
  platform: string;
  type: string;
  projectId: number;
  spentThisMonthMinor: number;
};

export type MonthBudget = { spentMinor: number; remainingMinor: number; dailyMinor: number };

export type Resolution =
  | ({ kind: "create"; line: LaunchLine } & MonthBudget)
  | ({ kind: "update"; line: LaunchLine; campaign: LaunchCampaign } & MonthBudget)
  | { kind: "pick"; line: LaunchLine; candidates: LaunchCampaign[] };

export function matches(line: LaunchLine, c: LaunchCampaign, projectId: number): boolean {
  return (
    c.projectId === projectId &&
    c.platform === line.platform &&
    (line.type === undefined || c.type === line.type)
  );
}

// Days from today to the end of the month, today included.
export function daysLeftInMonth(today: Date): number {
  const last = new Date(today.getFullYear(), today.getMonth() + 1, 0).getDate();
  return last - today.getDate() + 1;
}

export function monthBudget(line: LaunchLine, spentMinor: number, today: Date): MonthBudget {
  const remainingMinor = Math.max(line.budgetMinor - spentMinor, 0);
  return {
    spentMinor,
    remainingMinor,
    dailyMinor: Math.round(remainingMinor / daysLeftInMonth(today)),
  };
}

// Spend on the days of today's month, from a campaign's daily points.
export function spentThisMonth(daily: { date: string; spendMinor: number }[], today: Date): number {
  const month = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, "0")}`;
  return daily.filter((d) => d.date.startsWith(month)).reduce((sum, d) => sum + d.spendMinor, 0);
}

export function resolveLaunch(
  lines: LaunchLine[],
  campaigns: LaunchCampaign[],
  projectId: number,
  today: Date,
): Resolution[] {
  return lines.map((line) => {
    const candidates = campaigns.filter((c) => matches(line, c, projectId));
    if (candidates.length === 0) {
      return { kind: "create", line, ...monthBudget(line, 0, today) };
    }
    if (candidates.length === 1) {
      const campaign = candidates[0] as LaunchCampaign;
      return {
        kind: "update",
        line,
        campaign,
        ...monthBudget(line, campaign.spentThisMonthMinor, today),
      };
    }
    return { kind: "pick", line, candidates };
  });
}

// A person's choice for a line that matched more than one campaign.
export function pickCampaign(
  res: Extract<Resolution, { kind: "pick" }>,
  campaignId: string,
  today: Date,
): Extract<Resolution, { kind: "update" }> | null {
  const campaign = res.candidates.find((c) => c.id === campaignId);
  if (!campaign) return null;
  return {
    kind: "update",
    line: res.line,
    campaign,
    ...monthBudget(res.line, campaign.spentThisMonthMinor, today),
  };
}
