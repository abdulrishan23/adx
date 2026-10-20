// The Brief, mocked from the campaign fixtures. Yesterday is compared with the
// 7 days before it; cost per qualified lead compares whole weeks.
//
// Guesses until Reportage confirms them (#34):
// - each project has its own monthly budget;
// - "Meeting" is filled in reliably enough to count;
// - a lead is untouched until its GoHighLevel status first changes;
// - junk means the statuses Junk, Wrong number and Duplicate.

import type {
  Brief,
  BriefProject,
  Comparable,
  DailyPoint,
  DayCounts,
  LeadHandling,
  MarketRow,
  SourceRow,
  StageCounts,
} from "../models";
import { CAMPAIGNS, OWNERS, PROJECTS, PROPOSALS, rng } from "./data";

const rand = rng(97);

// The fixtures end today, so yesterday is the second-to-last point.
const YESTERDAY = 2;

const at = (points: DailyPoint[], back: number) => points[points.length - back];
const span = (points: DailyPoint[], from: number, days: number) =>
  points.slice(points.length - from - days + 1, points.length - from + 1);

function counts(points: DailyPoint[]): DayCounts {
  return points.reduce(
    (a, p) => ({
      spendMinor: a.spendMinor + p.spendMinor,
      leads: a.leads + p.leads,
      qualified: a.qualified + p.qualified,
    }),
    { spendMinor: 0, leads: 0, qualified: 0 },
  );
}

function perDay(c: DayCounts, days: number): DayCounts {
  return {
    spendMinor: Math.round(c.spendMinor / days),
    leads: Math.round(c.leads / days),
    qualified: Math.round(c.qualified / days),
  };
}

const cpq = (c: DayCounts) => (c.qualified > 0 ? c.spendMinor / c.qualified : null);

function combine(lists: DailyPoint[][]): DailyPoint[] {
  const byDate = new Map<string, DailyPoint>();
  for (const list of lists) {
    for (const p of list) {
      const d = byDate.get(p.date) ?? { date: p.date, spendMinor: 0, leads: 0, qualified: 0 };
      d.spendMinor += p.spendMinor;
      d.leads += p.leads;
      d.qualified += p.qualified;
      byDate.set(p.date, d);
    }
  }
  return [...byDate.values()].sort((a, b) => a.date.localeCompare(b.date));
}

// Budgets a little under or over what each project is spending, so the
// table shows both. A project with no budget shows no pacing.
const BUDGET_FACTOR: Record<number, number | null> = {
  1: 1.04,
  2: 0.94,
  3: 1.0,
  4: 1.07,
  5: null,
  6: 0.95,
};

// Later stages from the qualified count, at rates near what Reportage sees.
function stages(c: DayCounts): StageCounts {
  const meetings = Math.round(c.qualified * (0.3 + rand() * 0.15));
  const offers = Math.round(meetings * (0.35 + rand() * 0.15));
  return { ...c, meetings, offers, deals: Math.round(offers * (0.25 + rand() * 0.15)) };
}

const addStages = (a: StageCounts, b: StageCounts): StageCounts => ({
  spendMinor: a.spendMinor + b.spendMinor,
  leads: a.leads + b.leads,
  qualified: a.qualified + b.qualified,
  meetings: a.meetings + b.meetings,
  offers: a.offers + b.offers,
  deals: a.deals + b.deals,
});

const NO_STAGES: StageCounts = {
  spendMinor: 0,
  leads: 0,
  qualified: 0,
  meetings: 0,
  offers: 0,
  deals: 0,
};

function row(projectId: number | null, name: string, daily: DailyPoint[]): BriefProject {
  const yesterday = at(daily, YESTERDAY) ?? { date: "", spendMinor: 0, leads: 0, qualified: 0 };
  const before = perDay(counts(span(daily, YESTERDAY + 1, 7)), 7);
  const week = counts(span(daily, YESTERDAY, 7));
  const prior = counts(span(daily, YESTERDAY + 7, 7));
  const thisWeek = stages(week);
  const lastWeek = stages(prior);
  const meetings = thisWeek.meetings;

  const today = new Date().getUTCDate();
  const daysInMonth = new Date(
    new Date().getUTCFullYear(),
    new Date().getUTCMonth() + 1,
    0,
  ).getDate();
  const month = counts(daily.slice(-today));
  const projected = Math.round((month.spendMinor / today) * daysInMonth);
  const factor = projectId === null ? 1 : BUDGET_FACTOR[projectId];
  const budget =
    factor === null || factor === undefined || month.spendMinor === 0
      ? null
      : Math.round(projected / factor);

  return {
    projectId,
    name,
    yesterday: { leads: yesterday.leads, qualified: yesterday.qualified },
    average: { leads: before.leads, qualified: before.qualified },
    costPerQualified: { current: cpq(week), previous: cpq(prior) },
    meetings,
    costPerMeeting: meetings > 0 ? week.spendMinor / meetings : null,
    pacing:
      budget === null
        ? null
        : {
            spentMinor: month.spendMinor,
            budgetMinor: budget,
            projectedMinor: projected,
            dayOfMonth: today,
            daysInMonth,
          },
    qualifiedTrend: span(daily, YESTERDAY, 14).map((p) => p.qualified),
    week: thisWeek,
    previousWeek: lastWeek,
  };
}

function handling(arrived: number): LeadHandling {
  const untouched = Math.round(arrived * 0.21);
  const share = (weights: [string, number][]) => {
    let left = untouched;
    return weights.map(([key, w], i) => {
      const n = i === weights.length - 1 ? left : Math.min(left, Math.round(untouched * w));
      left -= n;
      return [key, n] as const;
    });
  };
  return {
    arrived,
    untouched,
    untouchedOverAnHour: Math.round(untouched * 0.74),
    medianFirstTouchMinutes: 52,
    bySource: share([
      ["Meta", 0.48],
      ["Bayut", 0.18],
      ["Property Finder", 0.14],
      ["Google", 0.12],
      ["Website", 0.08],
    ])
      .filter(([, n]) => n > 0)
      .map(([source, n]) => ({ source, untouched: n })),
    byOwner: share(OWNERS.map((o, i) => [o, [0.4, 0.27, 0.2, 0.13][i] ?? 0]))
      .filter(([, n]) => n > 0)
      .map(([owner, n]) => ({ owner, untouched: n })),
    junk: [
      { source: "Meta", share: 0.14, leads: 0 },
      { source: "Bayut", share: 0.09, leads: 0 },
      { source: "Property Finder", share: 0.08, leads: 0 },
      { source: "Google", share: 0.06, leads: 0 },
      { source: "Website", share: 0.04, leads: 0 },
    ],
  };
}

const PORTAL_MONTHLY_MINOR = { Bayut: 4_500_000, "Property Finder": 3_800_000 };

function sources(meta: DayCounts, metaPrior: DayCounts, google: DayCounts, googlePrior: DayCounts) {
  const row = (
    source: string,
    kind: SourceRow["kind"],
    spendMinor: number | null,
    leads: number,
    qualified: number,
    previous: Comparable["previous"],
  ): SourceRow => ({
    source,
    kind,
    spendMinor,
    leads,
    qualified,
    costPerQualified: {
      current: spendMinor !== null && qualified > 0 ? spendMinor / qualified : null,
      previous,
    },
  });
  const portal = (name: keyof typeof PORTAL_MONTHLY_MINOR, leads: number, rate: number) => {
    const spend = Math.round((PORTAL_MONTHLY_MINOR[name] * 7) / 30);
    const q = Math.round(leads * rate);
    return row(name, "portal", spend, leads, q, spend / Math.max(1, Math.round(q * 1.1)));
  };
  return [
    row("Meta", "paid", meta.spendMinor, meta.leads, meta.qualified, cpq(metaPrior)),
    row("Google", "paid", google.spendMinor, google.leads, google.qualified, cpq(googlePrior)),
    portal("Bayut", 148, 0.11),
    portal("Property Finder", 121, 0.12),
    row("Website", "organic", null, 64, 12, null),
    row("WhatsApp", "organic", null, 38, 9, null),
    row("Referral", "organic", null, 11, 5, null),
  ];
}

export function markets(leads: number, qualified: number): MarketRow[] {
  const split: [string, string, number, number][] = [
    ["UAE", "+971", 0.52, 1.0],
    ["Saudi Arabia", "+966", 0.11, 1.25],
    ["India", "+91", 0.12, 0.8],
    ["Pakistan", "+92", 0.07, 0.55],
    ["United Kingdom", "+44", 0.06, 1.4],
    ["Russia", "+7", 0.05, 1.2],
    ["Other", "", 0.07, 0.6],
  ];
  const rate = qualified / Math.max(1, leads);
  return split.map(([market, code, s, q]) => {
    const n = Math.round(leads * s);
    return { market, code, leads: n, qualified: Math.round(n * rate * q) };
  });
}

function sentence(total: BriefProject, rows: BriefProject[], y: DayCounts): string {
  const c = total.costPerQualified;
  const movers = rows
    // A project with a handful of qualified leads swings by chance, so only
    // name one with enough volume for the move to mean something.
    .filter(
      (r) =>
        r.costPerQualified.current !== null &&
        r.costPerQualified.previous &&
        r.average.qualified * 7 >= 10,
    )
    .map((r) => ({
      name: r.name,
      ratio: (r.costPerQualified.current as number) / (r.costPerQualified.previous as number) - 1,
    }))
    .sort((a, b) => Math.abs(b.ratio) - Math.abs(a.ratio));
  const aed = (minor: number) => `AED ${Math.round(minor / 100).toLocaleString("en")}`;
  const parts = [`Yesterday brought ${y.leads} leads, ${y.qualified} of them qualified.`];
  if (c.current !== null && c.previous) {
    const r = c.current / c.previous - 1;
    const pct = Math.round(Math.abs(r) * 100);
    parts.push(
      `A qualified lead cost ${aed(c.current)} over the last 7 days, ${
        pct < 1 ? "about the same as" : `${pct}% ${r < 0 ? "less than" : "more than"}`
      } the week before.`,
    );
  }
  const top = movers[0];
  if (top && Math.abs(top.ratio) >= 0.15) {
    parts.push(
      `${top.name} moved most: ${Math.round(Math.abs(top.ratio) * 100)}% ${
        top.ratio > 0 ? "more" : "less"
      } per qualified lead.`,
    );
  }
  return parts.join(" ");
}

// Built once, so the Brief and the Projects page show the same figures.
let cached: Brief | null = null;

export function brief(): Brief {
  cached ??= build();
  return cached;
}

function build(): Brief {
  const byProject = PROJECTS.map((p) =>
    combine(CAMPAIGNS.filter((c) => c.projectId === p.id).map((c) => c.daily)),
  );
  const all = combine(CAMPAIGNS.map((c) => c.daily));
  const projects = PROJECTS.map((p, i) => row(p.id, p.name, byProject[i] ?? []));
  const total = row(null, "All projects", all);

  const platform = (p: "meta" | "google", from: number) =>
    counts(span(combine(CAMPAIGNS.filter((c) => c.platform === p).map((c) => c.daily)), from, 7));
  const week = counts(span(all, YESTERDAY, 7));
  const yesterday = counts(span(all, YESTERDAY, 1));

  return {
    date: (at(all, YESTERDAY)?.date ?? "") as string,
    summary: {
      text: sentence(total, projects, yesterday),
      yesterday,
      average: perDay(counts(span(all, YESTERDAY + 1, 7)), 7),
      costPerQualified: total.costPerQualified,
    },
    pendingProposals: PROPOSALS.filter((p) => p.status === "pending").length,
    // The sum of the projects, so the Brief and the Projects page agree.
    funnel: {
      week: projects.reduce((a, p) => addStages(a, p.week), NO_STAGES),
      previous: projects.reduce((a, p) => addStages(a, p.previousWeek), NO_STAGES),
    },
    projects: projects.sort((a, b) => b.yesterday.leads - a.yesterday.leads),
    total,
    handling: handling(yesterday.leads),
    sources: sources(
      platform("meta", YESTERDAY),
      platform("meta", YESTERDAY + 7),
      platform("google", YESTERDAY),
      platform("google", YESTERDAY + 7),
    ),
    markets: markets(week.leads, week.qualified),
    statusCoverage: 0.53,
  };
}
