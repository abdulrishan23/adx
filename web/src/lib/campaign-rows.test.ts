import { describe, expect, it } from "vitest";
import { isLive } from "@/api/mock";
import { CAMPAIGNS } from "@/api/mock/data";
import type { CampaignCoverage, ReportRow } from "@/api/types.gen";
import {
  campaignRows,
  channelLabel,
  costPer,
  figuresOf,
  returnOnSpend,
  scopeWindow,
  sumFigures,
} from "./campaign-rows";
import { coverageFor } from "./coverage";

// The screen's join of the engine's two reads. What the engine says about a
// campaign (its figures, the plan behind it) is tested in Go; these catch
// the screen reading it wrongly.

function cov(over: Partial<CampaignCoverage>): CampaignCoverage {
  return {
    id: 1,
    accountId: 2,
    platform: "google_ads",
    externalId: "20100000008",
    name: "Search | Wadi | Brand",
    status: "ENABLED",
    running: true,
    channelType: "SEARCH",
    dailyBudgetMinor: 1_000_00,
    currency: "AED",
    projectId: 1,
    underPlan: false,
    planId: null,
    planVersion: null,
    line: null,
    kind: "",
    reason: "no accepted plan whose period has not ended resolves to this campaign",
    launch: {
      state: "none",
      action: "",
      proposalId: null,
      intentId: null,
      intentState: "",
      deliveryDay: null,
      note: "",
    },
    ...over,
  };
}

function row(campaignId: number, over: Partial<ReportRow["metrics"]> = {}): ReportRow {
  return {
    key: `campaign:${campaignId}`,
    label: "",
    platform: "google_ads",
    campaignId,
    projectId: 1,
    previous: null,
    changes: null,
    metrics: {
      spend: { minor: 12_000_00, currency: "AED" },
      impressions: 100,
      clicks: 10,
      googleConversions: null,
      metaLeads: null,
      leads: 4,
      qualifiedLeads: null,
      deals: null,
      revenue: { minor: null, currency: "AED" },
      costPerLead: { value: 3_000_00, undefined: false },
      costPerQualifiedLead: { value: null, undefined: false },
      roas: { value: null, undefined: false },
      ...over,
    },
  };
}

const all = { platform: "all", projectId: "all" } as const;

describe("campaign rows", () => {
  // Catches: the screen still reading the sample rows after the rewire, so
  // sample campaigns show beside, or instead of, the engine's.
  it("is live in partial mock mode, with rows from the coverage list only", () => {
    expect(isLive("campaigns")).toBe(true);
    const rows = campaignRows([cov({ id: 7 }), cov({ id: 8, externalId: "x" })], { rows: [] }, all);
    expect(rows.map((r) => r.id)).toEqual(["7", "8"]);
    expect(CAMPAIGNS.some((c) => rows.some((r) => r.id === c.id))).toBe(false);
  });

  // Catches: a 0 shown for qualified, deals or revenue while the engine has
  // none, a 0 spend for a campaign without platform figures, and a leads
  // count invented for a campaign the report has no row for.
  it("keeps a figure the engine lacks null, never 0", () => {
    const withRow = campaignRows([cov({ id: 1 })], { rows: [row(1)] }, all)[0];
    expect(withRow).toMatchObject({
      spendMinor: 12_000_00,
      leads: 4,
      qualified: null,
      deals: null,
      revenueMinor: null,
    });
    const noSpend = figuresOf(row(1, { spend: { minor: null, currency: "AED" } }));
    expect(noSpend.spendMinor).toBeNull();
    expect(noSpend.leads).toBe(4);
    const noRow = campaignRows([cov({ id: 2 })], { rows: [row(1)] }, all)[0];
    expect(noRow).toMatchObject({
      spendMinor: null,
      leads: null,
      qualified: null,
      deals: null,
      revenueMinor: null,
    });
    const funnel = figuresOf(row(1, { qualifiedLeads: 0, deals: 0 }));
    expect(funnel.qualified).toBe(0);
    expect(funnel.deals).toBe(0);
  });

  // Catches: a chip lost in the rewire, because the row no longer carries
  // the platform word and the platform's own id the chips attach by.
  it("carries what the chips attach by, so coverage finds its own row", () => {
    const coverage = [
      cov({ id: 1, underPlan: true, planId: 3, planVersion: 2, line: 0, kind: "update" }),
      cov({ id: 2, platform: "meta", externalId: "120330000000101", name: "Wadi | Leads" }),
    ];
    const rows = campaignRows(coverage, { rows: [] }, all);
    for (const r of rows) {
      expect(coverageFor(coverage, r)?.id).toBe(Number(r.id));
    }
    expect(rows[1]?.platform).toBe("meta");
  });

  // Catches: a paused campaign shown as active, the platform's channel word
  // shown raw, and the sheet's sample sections shown on a live row.
  it("reads status, channel and project from the coverage entry", () => {
    const [paused] = campaignRows(
      [cov({ running: false, status: "PAUSED", channelType: "PERFORMANCE_MAX", projectId: null })],
      { rows: [] },
      all,
    );
    expect(paused?.status).toBe("paused");
    expect(paused?.type).toBe("Performance Max");
    expect(paused?.projectId).toBeNull();
    expect(paused?.daily).toBeNull();
    expect(paused?.adSets).toBeNull();
    expect(paused?.temperatureMix).toBeNull();
    expect(channelLabel("OUTCOME_LEADS")).toBe("Leads");
    expect(channelLabel("LOCAL_SERVICES")).toBe("Local services");
    expect(channelLabel("")).toBe("Campaign");
  });

  // Catches: the scope bar's platform or project ignored on live rows, and
  // the screen's platform word compared with the engine's as is.
  it("narrows the coverage list to the scope", () => {
    const coverage = [
      cov({ id: 1, platform: "google_ads", projectId: 1 }),
      cov({ id: 2, platform: "meta", externalId: "m", projectId: 2 }),
      cov({ id: 3, platform: "meta", externalId: "n", projectId: null }),
    ];
    const ids = (scope: Parameters<typeof campaignRows>[2]) =>
      campaignRows(coverage, { rows: [] }, scope).map((r) => r.id);
    expect(ids({ platform: "google", projectId: "all" })).toEqual(["1"]);
    expect(ids({ platform: "meta", projectId: "all" })).toEqual(["2", "3"]);
    expect(ids({ platform: "all", projectId: 2 })).toEqual(["2"]);
    expect(ids(all)).toEqual(["1", "2", "3"]);
  });

  // Catches: a total shown as 0 when no row has the figure, a known row's
  // spend dropped because another row's is unknown, and a cost per lead
  // computed from an unknown side or divided by nothing.
  it("totals the rows that have a figure and divides only by a count", () => {
    const total = sumFigures([
      { spendMinor: 100, leads: 2, qualified: null, deals: null, revenueMinor: null },
      { spendMinor: null, leads: 3, qualified: null, deals: null, revenueMinor: null },
    ]);
    expect(total).toEqual({
      spendMinor: 100,
      leads: 5,
      qualified: null,
      deals: null,
      revenueMinor: null,
    });
    expect(sumFigures([]).spendMinor).toBeNull();
    expect(costPer(100, 4)).toBe(25);
    expect(costPer(null, 4)).toBeNull();
    expect(costPer(100, null)).toBeNull();
    expect(costPer(100, 0)).toBeNull();
    expect(
      returnOnSpend({ spendMinor: 100, leads: 0, qualified: 0, deals: 0, revenueMinor: 250 }),
    ).toBe(2.5);
    expect(
      returnOnSpend({ spendMinor: 100, leads: 0, qualified: 0, deals: 0, revenueMinor: null }),
    ).toBeNull();
  });

  // Catches: a window that includes today, which Dubai has not finished,
  // or one day short of the scope.
  it("asks for the scope's completed days, ending yesterday in Dubai", () => {
    expect(scopeWindow({ days: 7 }, "2026-10-21")).toEqual({
      from: "2026-10-14",
      to: "2026-10-20",
    });
    expect(scopeWindow({ days: 30 }, "2026-03-01")).toEqual({
      from: "2026-01-30",
      to: "2026-02-28",
    });
  });
});
