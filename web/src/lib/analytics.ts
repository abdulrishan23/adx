import type { Campaign, DailyPoint, Platform, Project } from "@/api/models";
import { TIME_ZONE } from "./format";

const DAY = 86_400_000;
export type AnalyticsMetric =
  | "spendMinor"
  | "leads"
  | "qualified"
  | "cpq"
  | "deals"
  | "revenueMinor"
  | "roas";
export type AnalyticsSearch = {
  compare: "none" | "previous" | "month" | "custom";
  comparisonFrom?: string;
  comparisonTo?: string;
  from: string;
  to: string;
  platform: Platform | "all";
  project: number | number[] | "all";
  metric: AnalyticsMetric;
};
export type AnalyticsPoint = DailyPoint & { deals: number | null; revenueMinor: number | null };
export type AnalyticsCampaign = Omit<Campaign, "daily"> & {
  daily: (DailyPoint & { deals?: number | null; revenueMinor?: number | null })[];
};
export type AnalyticsTotals = {
  spendMinor: number | null;
  leads: number | null;
  qualified: number | null;
  cpq: number | null;
  deals: number | null;
  revenueMinor: number | null;
  roas: number | null;
};
export type AnalyticsRow = {
  id: string;
  name: string;
  platform?: Platform;
  current: AnalyticsTotals;
  previous: AnalyticsTotals;
};
export type Analytics = {
  current: AnalyticsTotals;
  previous: AnalyticsTotals;
  projectOptions: Pick<Project, "id" | "name">[];
  daily: { date: string; current: AnalyticsPoint | null; previous: AnalyticsPoint | null }[];
  projects: AnalyticsRow[];
  sources: AnalyticsRow[];
  days: number;
  availableDays: number;
  previousFrom: string;
  previousTo: string;
};

export function dateShift(date: string, days: number): string {
  return new Date(Date.parse(`${date}T00:00:00Z`) + days * DAY).toISOString().slice(0, 10);
}

export function dubaiToday(now = new Date()): string {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: TIME_ZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(now);
}

export function validDate(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^\d{4}-\d{2}-\d{2}$/.test(value) &&
    Number.isFinite(Date.parse(`${value}T00:00:00Z`)) &&
    new Date(`${value}T00:00:00Z`).toISOString().slice(0, 10) === value
  );
}

export function monthEarlier(date: string): string {
  const d = new Date(`${date}T00:00:00Z`);
  const end = new Date(Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), 0));
  const day = Math.min(d.getUTCDate(), end.getUTCDate());
  return `${end.toISOString().slice(0, 7)}-${String(day).padStart(2, "0")}`;
}

export function defaultAnalyticsDates(today = dubaiToday()) {
  const to = dateShift(today, -1);
  return { from: `${(today.endsWith("-01") ? to : today).slice(0, 7)}-01`, to };
}

export function analyticsPeriodLabel(
  search: Pick<AnalyticsSearch, "from" | "to">,
  today = dubaiToday(),
): string | null {
  const defaults = defaultAnalyticsDates(today);
  if (search.from === defaults.from && search.to === defaults.to)
    return today.endsWith("-01") ? "Last month" : "Month to date";
  return null;
}

function projectSelection(raw: unknown): AnalyticsSearch["project"] {
  const values = Array.isArray(raw) ? raw : typeof raw === "string" ? raw.split(",") : [raw];
  const ids = [
    ...new Set(
      values
        .filter((v) => typeof v === "number" || typeof v === "string")
        .map(Number)
        .filter((id) => Number.isSafeInteger(id) && id > 0),
    ),
  ].sort((a, b) => a - b);
  return ids.length === 0 ? "all" : ids.length === 1 ? (ids[0] ?? "all") : ids;
}

export function analyticsSearch(raw: Record<string, unknown>): AnalyticsSearch {
  const defaults = defaultAnalyticsDates();
  const explicitDates = validDate(raw.from) || validDate(raw.to);
  return {
    compare:
      raw.compare === "month" ||
      raw.compare === "previous" ||
      raw.compare === "none" ||
      raw.compare === "custom"
        ? raw.compare
        : explicitDates
          ? "none"
          : "month",
    ...(raw.compare === "custom"
      ? {
          comparisonFrom: validDate(raw.comparisonFrom) ? raw.comparisonFrom : undefined,
          comparisonTo: validDate(raw.comparisonTo) ? raw.comparisonTo : undefined,
        }
      : {}),
    from: validDate(raw.from) ? raw.from : defaults.from,
    to: validDate(raw.to) ? raw.to : defaults.to,
    platform: raw.platform === "google" || raw.platform === "meta" ? raw.platform : "all",
    project: projectSelection(raw.project),
    metric:
      raw.metric === "leads" ||
      raw.metric === "qualified" ||
      raw.metric === "cpq" ||
      raw.metric === "deals" ||
      raw.metric === "revenueMinor" ||
      raw.metric === "roas"
        ? raw.metric
        : "spendMinor",
  };
}

export function comparisonRange(search: AnalyticsSearch): { from: string; to: string } {
  if (
    search.compare === "custom" &&
    validDate(search.comparisonFrom) &&
    validDate(search.comparisonTo)
  )
    return { from: search.comparisonFrom, to: search.comparisonTo };
  if (search.compare === "month") {
    const from = monthEarlier(search.from);
    // A full calendar month compares with the full preceding month.
    const fullMonth =
      search.from.slice(0, 7) === search.to.slice(0, 7) &&
      search.from.endsWith("-01") &&
      dateShift(search.to, 1).endsWith("-01");
    return { from, to: fullMonth ? dateShift(search.from, -1) : monthEarlier(search.to) };
  }
  const days = Math.round((Date.parse(search.to) - Date.parse(search.from)) / DAY) + 1;
  return { from: dateShift(search.from, -days), to: dateShift(search.from, -1) };
}

export function rangeError(search: AnalyticsSearch): string | null {
  if (!validDate(search.from) || !validDate(search.to)) return "Choose valid start and end dates.";
  if (search.from > search.to) return "Choose an end date on or after the start date.";
  if (search.to >= dubaiToday())
    return "Choose a completed reporting day. Today is still in progress in Dubai.";
  if ((Date.parse(search.to) - Date.parse(search.from)) / DAY >= 90)
    return "Choose up to 90 days at a time.";
  if (search.compare === "custom") {
    if (!validDate(search.comparisonFrom) || !validDate(search.comparisonTo))
      return "Choose valid comparison dates.";
    if (search.comparisonFrom > search.comparisonTo)
      return "The comparison end date must be on or after its start date.";
    if (search.comparisonTo >= dubaiToday()) return "The comparison must end before today.";
    if ((Date.parse(search.comparisonTo) - Date.parse(search.comparisonFrom)) / DAY >= 90)
      return "Choose up to 90 comparison days.";
  }
  return null;
}

const unavailable = (): AnalyticsTotals => ({
  spendMinor: null,
  leads: null,
  qualified: null,
  cpq: null,
  deals: null,
  revenueMinor: null,
  roas: null,
});

// One calculation supplies the chart, totals and both breakdowns. A missing
// day invalidates its period total; a recorded zero remains a real zero.
export function buildAnalytics(
  campaigns: AnalyticsCampaign[],
  projects: Pick<Project, "id" | "name">[],
  search: AnalyticsSearch,
): Analytics {
  if (rangeError(search)) throw new Error(rangeError(search) ?? "Invalid period");
  const days = Math.round((Date.parse(search.to) - Date.parse(search.from)) / DAY) + 1;
  const { from: previousFrom, to: previousTo } = comparisonRange(search);
  const previousDays = Math.round((Date.parse(previousTo) - Date.parse(previousFrom)) / DAY) + 1;
  const selected = campaigns.filter(
    (c) =>
      (search.platform === "all" || c.platform === search.platform) &&
      (search.project === "all" ||
        (Array.isArray(search.project)
          ? search.project.includes(c.projectId)
          : c.projectId === search.project)),
  );
  function period(
    rows: AnalyticsCampaign[],
    from: string,
    length = days,
  ): (AnalyticsPoint | null)[] {
    const lookup = rows.map((c) => new Map(c.daily.map((d) => [d.date, d])));
    return Array.from({ length }, (_, i) => {
      const date = dateShift(from, i);
      const points = lookup.map((m) => m.get(date));
      if (!rows.length || points.some((p) => p === undefined)) return null;
      return points.reduce<AnalyticsPoint>(
        (a, p) => ({
          date,
          spendMinor: a.spendMinor + (p?.spendMinor ?? 0),
          leads: a.leads + (p?.leads ?? 0),
          qualified: a.qualified + (p?.qualified ?? 0),
          deals: a.deals === null || p?.deals == null ? null : a.deals + p.deals,
          revenueMinor:
            a.revenueMinor === null || p?.revenueMinor == null
              ? null
              : a.revenueMinor + p.revenueMinor,
        }),
        { date, spendMinor: 0, leads: 0, qualified: 0, deals: 0, revenueMinor: 0 },
      );
    });
  }
  function totals(points: (AnalyticsPoint | null)[]): AnalyticsTotals {
    if (points.some((p) => p === null)) return unavailable();
    const total = points.reduce<Omit<AnalyticsTotals, "cpq" | "roas">>(
      (a, p) => ({
        spendMinor: (a.spendMinor ?? 0) + (p?.spendMinor ?? 0),
        leads: (a.leads ?? 0) + (p?.leads ?? 0),
        qualified: (a.qualified ?? 0) + (p?.qualified ?? 0),
        deals: a.deals === null || p?.deals == null ? null : a.deals + p.deals,
        revenueMinor:
          a.revenueMinor === null || p?.revenueMinor == null
            ? null
            : a.revenueMinor + p.revenueMinor,
      }),
      { spendMinor: 0, leads: 0, qualified: 0, deals: 0, revenueMinor: 0 },
    );
    return {
      ...total,
      cpq:
        total.qualified !== null && total.qualified > 0 && total.spendMinor !== null
          ? total.spendMinor / total.qualified
          : null,
      roas:
        total.revenueMinor !== null && total.spendMinor !== null && total.spendMinor > 0
          ? total.revenueMinor / total.spendMinor
          : null,
    };
  }
  function row(
    id: string,
    name: string,
    rows: AnalyticsCampaign[],
    platform?: Platform,
  ): AnalyticsRow {
    return {
      id,
      name,
      platform,
      current: totals(period(rows, search.from)),
      previous: totals(period(rows, previousFrom, previousDays)),
    };
  }
  const current = period(selected, search.from);
  const previous = period(selected, previousFrom, previousDays);
  return {
    current: totals(current),
    previous: totals(previous),
    days,
    projectOptions: projects,
    availableDays: current.filter((p) => p !== null).length,
    previousFrom,
    previousTo,
    daily: current.map((p, i) => ({
      date: dateShift(search.from, i),
      current: p,
      previous: previous[i] ?? null,
    })),
    projects: projects
      .filter((p) => selected.some((c) => c.projectId === p.id))
      .map((p) =>
        row(
          String(p.id),
          p.name,
          selected.filter((c) => c.projectId === p.id),
        ),
      ),
    sources: (["meta", "google"] as const)
      .filter((p) => selected.some((c) => c.platform === p))
      .map((p) =>
        row(
          p,
          p === "meta" ? "Meta" : "Google",
          selected.filter((c) => c.platform === p),
          p,
        ),
      ),
  };
}
