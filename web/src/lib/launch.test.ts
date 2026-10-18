import { describe, expect, it } from "vitest";
import {
  daysLeftInMonth,
  type LaunchCampaign,
  monthBudget,
  pickCampaign,
  resolveLaunch,
  spentThisMonth,
} from "./launch";

// The rules the owner decided on 2026-10-18 (BD-58). Each test names the
// wrong behaviour it catches.

const today = new Date(2026, 9, 12); // 12 October, 31-day month, 20 days left

const campaigns: LaunchCampaign[] = [
  {
    id: "m1",
    name: "Wadi | Leads | UAE AR",
    platform: "meta",
    type: "Leads, instant form",
    projectId: 1,
    spentThisMonthMinor: 50_000_00,
  },
  {
    id: "m2",
    name: "Wadi | Leads | UAE EN",
    platform: "meta",
    type: "Leads, instant form",
    projectId: 1,
    spentThisMonthMinor: 30_000_00,
  },
  {
    id: "g1",
    name: "Search | Wadi | Brand",
    platform: "google",
    type: "Search",
    projectId: 1,
    spentThisMonthMinor: 40_000_00,
  },
  {
    id: "v1",
    name: "Verdana | Leads",
    platform: "google",
    type: "Search",
    projectId: 2,
    spentThisMonthMinor: 90_000_00,
  },
];

describe("resolveLaunch", () => {
  // Catches ADX updating both matches without asking.
  it("asks when a line matches more than one campaign", () => {
    const [r] = resolveLaunch([{ platform: "meta", budgetMinor: 200_000_00 }], campaigns, 1, today);
    expect(r?.kind).toBe("pick");
    if (r?.kind === "pick") expect(r.candidates.map((c) => c.id)).toEqual(["m1", "m2"]);
  });

  // Catches spend to date being ignored, or another project's campaign
  // counting as a match.
  it("updates a single match and nets off this month's spend", () => {
    const [r] = resolveLaunch(
      [{ platform: "google", budgetMinor: 152_000_00 }],
      campaigns,
      1,
      today,
    );
    expect(r).toMatchObject({
      kind: "update",
      campaign: { id: "g1" },
      spentMinor: 40_000_00,
      remainingMinor: 112_000_00,
      dailyMinor: 5_600_00,
    });
  });

  // Catches a line being matched to a campaign of another type.
  it("matches the campaign type when the line names one", () => {
    const [r] = resolveLaunch(
      [{ platform: "google", type: "Performance Max", budgetMinor: 10_000_00 }],
      campaigns,
      1,
      today,
    );
    expect(r?.kind).toBe("create");
  });

  // Catches a created campaign getting the whole month's budget per day.
  it("creates when nothing matches, spreading the budget over the days left", () => {
    const [r] = resolveLaunch(
      [{ platform: "snapchat", budgetMinor: 20_000_00 }],
      campaigns,
      1,
      today,
    );
    expect(r).toMatchObject({
      kind: "create",
      spentMinor: 0,
      remainingMinor: 20_000_00,
      dailyMinor: 1_000_00,
    });
  });
});

describe("monthBudget", () => {
  // Catches a negative budget when the campaign has already spent past the plan.
  it("never goes below zero", () => {
    expect(monthBudget({ platform: "meta", budgetMinor: 30_000_00 }, 45_000_00, today)).toEqual({
      spentMinor: 45_000_00,
      remainingMinor: 0,
      dailyMinor: 0,
    });
  });

  it("counts today among the days left", () => {
    expect(daysLeftInMonth(new Date(2026, 9, 31))).toBe(1);
    expect(daysLeftInMonth(new Date(2026, 1, 1))).toBe(28);
  });
});

describe("pickCampaign", () => {
  const [r] = resolveLaunch([{ platform: "meta", budgetMinor: 200_000_00 }], campaigns, 1, today);

  it("turns the person's choice into an update of that campaign only", () => {
    if (r?.kind !== "pick") throw new Error("expected a pick");
    expect(pickCampaign(r, "m2", today)).toMatchObject({
      kind: "update",
      campaign: { id: "m2" },
      remainingMinor: 170_000_00,
    });
  });

  // Catches a choice outside the candidates slipping through.
  it("refuses a campaign that was not a candidate", () => {
    if (r?.kind !== "pick") throw new Error("expected a pick");
    expect(pickCampaign(r, "g1", today)).toBeNull();
  });
});

describe("spentThisMonth", () => {
  // Catches last month's spend netting off this month's plan.
  it("sums only this month's days", () => {
    const daily = [
      { date: "2026-09-30", spendMinor: 5_000_00 },
      { date: "2026-10-01", spendMinor: 1_000_00 },
      { date: "2026-10-12", spendMinor: 2_000_00 },
    ];
    expect(spentThisMonth(daily, today)).toBe(3_000_00);
  });
});
