import { describe, expect, it } from "vitest";
import { ANALYTICS_CAMPAIGNS, analytics } from "@/api/mock/analytics";
import { projectPerformance } from "@/api/mock/project-performance";
import type { Campaign } from "@/api/models";
import {
  type AnalyticsCampaign,
  analyticsPeriodLabel,
  analyticsSearch,
  buildAnalytics,
  comparisonRange,
  dateShift,
  defaultAnalyticsDates,
  dubaiToday,
  monthEarlier,
  rangeError,
  validDate,
} from "./analytics";

const projects = [
  { id: 1, name: "The Wadi" },
  { id: 2, name: "Verdana" },
];
function campaign(
  id: string,
  projectId: number,
  platform: Campaign["platform"],
  multiplier = 1,
): Campaign {
  return {
    id,
    projectId,
    platform,
    name: id,
    type: "Leads",
    status: "active",
    spendMinor: 99999,
    leads: 999,
    qualified: 999,
    deals: 2,
    revenueMinor: null,
    temperatureMix: { hot: 0, warm: 0, cold: 0 },
    adSets: [],
    daily: [1, 2, 3, 4].map((day) => ({
      date: `2026-01-0${day}`,
      spendMinor: day * 10000 * multiplier,
      leads: day * 10 * multiplier,
      qualified: day * multiplier,
    })),
  };
}
const filters = {
  ...analyticsSearch({ compare: "previous" }),
  from: "2026-01-03",
  to: "2026-01-04",
};

describe("analytics reporting", () => {
  it("parses multiple project IDs without duplicates and preserves single-project links", () => {
    expect(analyticsSearch({ project: [2, 1, 2] }).project).toEqual([1, 2]);
    expect(analyticsSearch({ project: "2,1,2" }).project).toEqual([1, 2]);
    expect(analyticsSearch({ project: "1" }).project).toBe(1);
    expect(analyticsSearch({ project: [null, false, "bad", -1, 0] }).project).toBe("all");
    expect(analyticsSearch({ project: [] }).project).toBe("all");
  });
  it("combines selected projects once across totals, daily points and breakdowns", () => {
    const data = buildAnalytics(
      [
        campaign("a", 1, "meta"),
        campaign("b", 2, "meta", 2),
        campaign("c", 3, "meta", 4),
        campaign("d", 1, "google", 8),
      ],
      [...projects, { id: 3, name: "Other" }],
      { ...filters, project: [1, 2, 2], platform: "meta" },
    );
    expect(data.current.spendMinor).toBe(210000);
    expect(data.previous.spendMinor).toBe(90000);
    expect(data.daily.map((day) => day.current?.spendMinor)).toEqual([90000, 120000]);
    expect(data.projects.map((p) => p.id).sort()).toEqual(["1", "2"]);
    expect(data.sources).toHaveLength(1);
    expect(data.sources[0]?.current.spendMinor).toBe(210000);
    expect(data.projectOptions).toHaveLength(3);
  });
  it("defaults to month to date with a calendar-month comparison while preserving explicit choices", () => {
    expect(analyticsSearch({}).compare).toBe("month");
    expect(analyticsSearch({})).toMatchObject(defaultAnalyticsDates());
    expect(analyticsSearch({ compare: "none" }).compare).toBe("none");
    expect(analyticsSearch({ compare: "previous" }).compare).toBe("previous");
    expect(analyticsSearch({ from: "2026-01-01", to: "2026-01-16" }).compare).toBe("none");
    expect(defaultAnalyticsDates("2026-10-17")).toEqual({ from: "2026-10-01", to: "2026-10-16" });
    expect(analyticsPeriodLabel({ from: "2026-10-01", to: "2026-10-16" }, "2026-10-17")).toBe(
      "Month to date",
    );
    expect(defaultAnalyticsDates("2026-01-01")).toEqual({ from: "2025-12-01", to: "2025-12-31" });
    expect(analyticsPeriodLabel({ from: "2025-12-01", to: "2025-12-31" }, "2026-01-01")).toBe(
      "Last month",
    );
    expect(monthEarlier("2026-03-31")).toBe("2026-02-28");
    expect(monthEarlier("2024-03-31")).toBe("2024-02-29");
    const data = buildAnalytics([campaign("a", 1, "meta")], projects, {
      ...filters,
      from: "2026-03-01",
      to: "2026-03-30",
      compare: "month",
    });
    expect(data.previousFrom).toBe("2026-02-01");
    expect(data.previousTo).toBe("2026-02-28");
    expect(data.daily[28]?.previous).toBeNull();
  });
  it("compares full months with full months and preserves explicitly chosen comparison dates", () => {
    expect(
      comparisonRange({ ...filters, from: "2026-09-01", to: "2026-09-30", compare: "month" }),
    ).toEqual({ from: "2026-08-01", to: "2026-08-31" });
    const custom = {
      ...filters,
      compare: "custom" as const,
      comparisonFrom: "2026-01-02",
      comparisonTo: "2026-01-03",
    };
    const data = buildAnalytics([campaign("a", 1, "meta")], projects, custom);
    expect(data.previousFrom).toBe("2026-01-02");
    expect(data.previousTo).toBe("2026-01-03");
    expect(data.previous.spendMinor).toBe(50000);
    expect(analyticsSearch(custom)).toMatchObject({
      compare: "custom",
      comparisonFrom: "2026-01-02",
      comparisonTo: "2026-01-03",
    });
    expect(rangeError({ ...custom, comparisonFrom: "2026-01-04" })).toMatch(/comparison end date/);
    expect(rangeError({ ...custom, comparisonTo: undefined })).toMatch(/valid comparison dates/);
    expect(analyticsSearch({ ...custom, compare: "none" }).comparisonFrom).toBeUndefined();
  });
  it("keeps totals, chart and breakdowns on the same dates and filters", () => {
    const data = buildAnalytics(
      [campaign("a", 1, "meta"), campaign("b", 1, "google", 2), campaign("c", 2, "meta", 3)],
      projects,
      { ...filters, project: 1, platform: "meta" },
    );
    expect(data.current).toEqual({
      spendMinor: 70000,
      leads: 70,
      qualified: 7,
      cpq: 10000,
      deals: null,
      revenueMinor: null,
      roas: null,
    });
    expect(data.daily.reduce((n, d) => n + (d.current?.spendMinor ?? 0), 0)).toBe(
      data.current.spendMinor,
    );
    expect(data.projects.map((r) => r.name)).toEqual(["The Wadi"]);
    expect(data.sources.map((r) => r.name)).toEqual(["Meta"]);
    expect(data.projects[0]?.current).toEqual(data.current);
    expect(data.sources[0]?.current).toEqual(data.current);
    expect(data.previousFrom).toBe("2026-01-01");
    expect(data.previousTo).toBe("2026-01-02");
    expect(data.previous.spendMinor).toBe(30000);
  });

  it("does not present a partial previous period as a complete comparison", () => {
    const c = campaign("a", 1, "meta");
    c.daily = c.daily.slice(1);
    const data = buildAnalytics([c], projects, filters);
    expect(data.previous.spendMinor).toBeNull();
    expect(data.previous.qualified).toBeNull();
    expect(data.current.spendMinor).toBe(70000);
    expect(data.daily[0]?.previous).toBeNull();
  });

  it("does not turn a missing current day or empty selection into zero spend", () => {
    const c = campaign("a", 1, "meta");
    c.daily = c.daily.filter((d) => d.date !== "2026-01-04");
    const data = buildAnalytics([c], projects, filters);
    expect(data.availableDays).toBe(1);
    expect(data.current.spendMinor).toBeNull();
    const empty = buildAnalytics([c], projects, { ...filters, project: 2 });
    expect(empty.current.spendMinor).toBeNull();
    expect(empty.projects).toEqual([]);
  });

  it("preserves measured zero while refusing cost per qualified when none qualify", () => {
    const c = campaign("a", 1, "meta");
    c.daily = c.daily.map((d) => ({ ...d, spendMinor: 0, leads: 0, qualified: 0 }));
    const data = buildAnalytics([c], projects, filters);
    expect(data.current.spendMinor).toBe(0);
    expect(data.current.leads).toBe(0);
    expect(data.current.cpq).toBeNull();
  });

  it("calculates ROAS from total revenue and spend rather than averaging campaign returns", () => {
    const campaigns: AnalyticsCampaign[] = [
      campaign("a", 1, "meta"),
      campaign("b", 2, "google", 2),
    ].map((c, i) => ({
      ...c,
      daily: c.daily.map((d) => ({ ...d, deals: 1, revenueMinor: i === 0 ? 100000 : 300000 })),
    }));
    const data = buildAnalytics(campaigns, projects, filters);
    expect(data.current.deals).toBe(4);
    expect(data.current.revenueMinor).toBe(800000);
    expect(data.current.roas).toBeCloseTo(800000 / 210000);
    expect(data.projects.reduce((sum, r) => sum + (r.current.revenueMinor ?? 0), 0)).toBe(
      data.current.revenueMinor,
    );
    const meta = buildAnalytics(campaigns, projects, { ...filters, platform: "meta" });
    expect(meta.current.deals).toBe(2);
    expect(meta.current.revenueMinor).toBe(200000);
    const noSpend = buildAnalytics(
      campaigns.map((c) => ({ ...c, daily: c.daily.map((d) => ({ ...d, spendMinor: 0 })) })),
      projects,
      filters,
    );
    expect(noSpend.current.roas).toBeNull();
  });

  it("keeps sample sales and revenue consistent across daily points and both breakdowns", () => {
    const dates = ANALYTICS_CAMPAIGNS[0]?.daily.map((d) => d.date) ?? [];
    const to = dates[dates.length - 2] ?? "2026-01-01";
    const data = buildAnalytics(
      ANALYTICS_CAMPAIGNS,
      [
        { id: 1, name: "The Wadi" },
        { id: 2, name: "Verdana" },
        { id: 3, name: "Oceana" },
        { id: 4, name: "Sila" },
        { id: 5, name: "Sensi" },
        { id: 6, name: "Selina Bay" },
      ],
      { ...filters, from: dateShift(to, -29), to },
    );
    expect(data.current.deals).toBeGreaterThan(0);
    expect(data.current.revenueMinor).toBeGreaterThan(0);
    expect(data.daily.reduce((sum, d) => sum + (d.current?.revenueMinor ?? 0), 0)).toBe(
      data.current.revenueMinor,
    );
    expect(data.projects.reduce((sum, r) => sum + (r.current.deals ?? 0), 0)).toBe(
      data.current.deals,
    );
    expect(data.sources.reduce((sum, r) => sum + (r.current.revenueMinor ?? 0), 0)).toBe(
      data.current.revenueMinor,
    );
  });

  it("changes project figures with the picker dates and reconciles them with Analytics", () => {
    const dates = ANALYTICS_CAMPAIGNS[0]?.daily.map((d) => d.date) ?? [];
    const to = dates[dates.length - 2] ?? "2026-01-01";
    const weekSearch = { ...filters, from: dateShift(to, -6), to };
    const monthSearch = { ...filters, from: dateShift(to, -29), to };
    const week = projectPerformance(weekSearch);
    const month = projectPerformance(monthSearch);
    const report = analytics(monthSearch);
    for (const p of month) {
      const row = report.projects.find((r) => Number(r.id) === p.projectId);
      expect(p.current).toEqual(row?.current);
      expect(p.previous).toEqual(row?.previous);
      expect(p.qualifiedTrend).toHaveLength(30);
      expect(p.pacing).toEqual(week.find((r) => r.projectId === p.projectId)?.pacing);
    }
    expect(week.reduce((sum, p) => sum + (p.current.spendMinor ?? 0), 0)).toBeLessThan(
      month.reduce((sum, p) => sum + (p.current.spendMinor ?? 0), 0),
    );
    const earliest = dates[0] ?? "2026-01-01";
    const missing = projectPerformance({ ...filters, from: dateShift(earliest, -1), to: earliest });
    expect(
      missing.every(
        (p) => p.current.leads === null && p.meetings === null && p.qualifiedTrend === null,
      ),
    ).toBe(true);
  });

  it("uses Dubai reporting dates and refuses invalid, reversed or unfinished ranges", () => {
    expect(dubaiToday(new Date("2026-01-01T21:00:00Z"))).toBe("2026-01-02");
    expect(validDate("2026-02-30")).toBe(false);
    expect(validDate("2026-02-28")).toBe(true);
    expect(rangeError({ ...filters, from: "2026-02-30" })).toMatch(/valid start/);
    expect(rangeError({ ...filters, from: "2026-01-04", to: "2026-01-03" })).toMatch(/end date/);
    expect(rangeError({ ...filters, to: dubaiToday() })).toMatch(/completed/);
    expect(rangeError({ ...filters, from: "2025-01-01" })).toMatch(/90 days/);
    expect(
      analyticsSearch({ project: "NaN", platform: "bogus", metric: "revenue", from: "2026-02-30" }),
    ).toMatchObject({ project: "all", platform: "all", metric: "spendMinor" });
  });
});
