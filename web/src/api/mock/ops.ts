// Outcome sync, guardrails, scoring, revenue and ads for the design branch.
// Figures come from the shared fixtures where a screen elsewhere shows the same
// thing, so the numbers agree across pages.

import type {
  AdRow,
  Calibration,
  Guardrails,
  Outcomes,
  Revenue,
  RevenueRow,
  Scoring,
} from "../models";
import { CAMPAIGNS, DAY, daysAgo, hoursAgo, LEADS, now } from "./data";
import { portalRows } from "./more";

// ——— Outcome sync ———

export function outcomes(): Outcomes {
  const daily = Array.from({ length: 30 }, (_, i) => {
    const d = new Date(now.getTime() - (29 - i) * DAY);
    const weekday = d.getUTCDay();
    const base = weekday === 5 ? 3 : weekday === 6 ? 6 : 12;
    return {
      date: d.toISOString().slice(0, 10),
      qualified: base + ((i * 7) % 5),
      deals: i % 4 === 0 ? 2 : i % 3 === 0 ? 1 : 0,
    };
  });
  // Google stopped receiving uploads four days ago, when the OAuth token expired.
  const sum = (k: "qualified" | "deals") => daily.reduce((a, d) => a + d[k], 0);
  const q = sum("qualified");
  const deals = sum("deals");
  const lead = (i: number) => LEADS[i * 11] ?? (LEADS[0] as (typeof LEADS)[number]);
  return {
    destinations: [
      {
        key: "meta-capi",
        platform: "meta",
        name: "Meta Conversions API",
        event: "Qualified lead (Lead, CRM)",
        sent30d: q,
        matched30d: Math.round(q * 0.91),
        failed30d: 3,
        lastSentAt: hoursAgo(1),
        state: "ready",
        stateLabel: "Sending",
      },
      {
        key: "meta-capi-deal",
        platform: "meta",
        name: "Meta Conversions API",
        event: "Done-Deal (Purchase, with value)",
        sent30d: deals,
        matched30d: deals - 1,
        failed30d: 0,
        lastSentAt: daysAgo(2),
        state: "ready",
        stateLabel: "Sending",
      },
      {
        key: "google-oci",
        platform: "google",
        name: "Google offline conversions",
        event: "Qualified lead (GCLID)",
        sent30d: Math.round(q * 0.22),
        matched30d: null,
        failed30d: 41,
        lastSentAt: daysAgo(4),
        state: "error",
        stateLabel: "Stopped",
      },
      {
        key: "google-ec",
        platform: "google",
        name: "Google enhanced conversions for leads",
        event: "Qualified lead (hashed phone and email)",
        sent30d: 0,
        matched30d: null,
        failed30d: 0,
        lastSentAt: null,
        state: "pending",
        stateLabel: "Not set up",
      },
    ],
    daily,
    failures: [
      {
        id: "f1",
        leadId: lead(1).id,
        leadName: lead(1).name,
        destination: "Google offline conversions",
        reason: "Google sign-in expired. Reconnect Google Ads in Connections.",
        at: hoursAgo(3),
      },
      {
        id: "f2",
        leadId: lead(2).id,
        leadName: lead(2).name,
        destination: "Google offline conversions",
        reason: "Click is older than 90 days, so Google won't accept it.",
        at: hoursAgo(20),
      },
      {
        id: "f3",
        leadId: lead(3).id,
        leadName: lead(3).name,
        destination: "Meta Conversions API",
        reason: "No Meta lead ID, phone or email to match on.",
        at: daysAgo(1),
      },
      {
        id: "f4",
        leadId: lead(4).id,
        leadName: lead(4).name,
        destination: "Meta Conversions API",
        reason: "Sent twice by Reportage's own Zapier, so Meta dropped the copy.",
        at: daysAgo(3),
      },
    ],
    audit: [
      {
        id: "au1",
        title: "The website pixel fires Lead on every thank-you page view",
        finding:
          "Refreshing the thank-you page sends another Lead. 14% of last month's Leads were repeats from the same browser.",
        fix: "Send Lead from GoHighLevel once per contact, with an event_id so Meta drops repeats.",
        severity: "high",
        status: "open",
      },
      {
        id: "au2",
        title: "Zapier also sends Conversions API events",
        finding:
          "A Zap from GoHighLevel sends Lead with no event_id, so Meta can't tell it apart from the pixel's.",
        fix: "Turn the Zap off once Adx sends the same events.",
        severity: "high",
        status: "open",
      },
      {
        id: "au3",
        title: "Instant form leads go back without the Meta lead ID",
        finding:
          "Without lead_id, Meta matches on phone and email only and drops about a fifth of them.",
        fix: "Adx sends lead_id with every outcome for instant form leads.",
        severity: "medium",
        status: "fixed",
      },
      {
        id: "au4",
        title: "Google's primary conversion is the form submit",
        finding:
          "Smart bidding optimises for form fills, so it learns from junk leads as much as from buyers.",
        fix: "Make the qualified lead upload primary and the form submit secondary, after two weeks of uploads.",
        severity: "medium",
        status: "open",
      },
    ],
  };
}

// ——— Guardrails ———

const guard: Guardrails = {
  dailyIncreaseCapPct: 20,
  changesPerDay: 15,
  changesToday: 6,
  proposalHours: 48,
  accounts: [
    {
      key: "meta",
      platform: "meta",
      name: "Reportage Properties (Meta)",
      monthlyBudgetMinor: 26_000_000,
      killed: false,
      killedBy: null,
      killedAt: null,
    },
    {
      key: "google",
      platform: "google",
      name: "Reportage Properties (Google Ads)",
      monthlyBudgetMinor: 14_000_000,
      killed: false,
      killedBy: null,
      killedAt: null,
    },
  ],
  blocked: [
    {
      id: "b1",
      at: hoursAgo(26),
      action: "Raise Search | Wadi | Brand budget by 40%",
      reason: "Over the 20% daily increase cap. Split into two days, or raise the cap.",
      by: "Ask Adx draft",
    },
    {
      id: "b2",
      at: daysAgo(5),
      action: "Pause 3 Meta ads",
      reason: "The 15 changes for the day were already used.",
      by: "Omar",
    },
  ],
};

export function guardrails(): Guardrails {
  return guard;
}

export function updateGuardrails(input: {
  dailyIncreaseCapPct?: number;
  changesPerDay?: number;
  account?: { key: string; killed?: boolean; monthlyBudgetMinor?: number | null };
}): Guardrails {
  if (input.dailyIncreaseCapPct !== undefined) {
    if (input.dailyIncreaseCapPct < 0 || input.dailyIncreaseCapPct > 100)
      throw new Error("The cap must be between 0% and 100%");
    guard.dailyIncreaseCapPct = input.dailyIncreaseCapPct;
  }
  if (input.changesPerDay !== undefined) {
    if (input.changesPerDay < 1) throw new Error("Allow at least one change a day");
    guard.changesPerDay = input.changesPerDay;
  }
  if (input.account) {
    const a = guard.accounts.find((x) => x.key === input.account?.key);
    if (!a) throw new Error("That account isn't connected any more");
    if (input.account.killed !== undefined) {
      a.killed = input.account.killed;
      a.killedBy = a.killed ? "Hisham Ahamad" : null;
      a.killedAt = a.killed ? new Date().toISOString() : null;
    }
    if (input.account.monthlyBudgetMinor !== undefined) {
      if (input.account.monthlyBudgetMinor !== null && input.account.monthlyBudgetMinor <= 0)
        throw new Error("The budget must be more than zero");
      a.monthlyBudgetMinor = input.account.monthlyBudgetMinor;
    }
  }
  return guard;
}

// ——— Scoring ———

const scoring: Omit<Scoring, "calibration" | "coverage"> = {
  fit: [
    {
      key: "when-now",
      question: "When are you planning to buy?",
      answer: "This month (launch offer)",
      points: 30,
    },
    {
      key: "when-3m",
      question: "When are you planning to buy?",
      answer: "Within 3 months",
      points: 20,
    },
    {
      key: "when-explore",
      question: "When are you planning to buy?",
      answer: "Just exploring",
      points: 0,
    },
    { key: "pay-cash", question: "How will you pay?", answer: "Cash", points: 20 },
    { key: "pay-plan", question: "How will you pay?", answer: "Payment plan", points: 15 },
    { key: "pay-mortgage", question: "How will you pay?", answer: "Mortgage", points: 10 },
    { key: "unit-large", question: "Which unit type?", answer: "3–4 BR or townhouse", points: 10 },
    { key: "phone-uae", question: "Phone code", answer: "+971 (lives in the UAE)", points: 10 },
    { key: "phone-gcc", question: "Phone code", answer: "Other GCC", points: 8 },
  ],
  signals: [
    { code: "S05", label: "Messaged us", points: 30, cap: 30, halfLifeDays: 7 },
    { code: "S03", label: "Tapped WhatsApp", points: 20, cap: 40, halfLifeDays: 5 },
    { code: "C09", label: "Ready to act", points: 25, cap: 25, halfLifeDays: 5 },
    { code: "S16", label: "Site visit", points: 35, cap: 35, halfLifeDays: 14 },
    { code: "S07", label: "Took project material", points: 12, cap: 24, halfLifeDays: 10 },
    { code: "S09", label: "Viewed an offer", points: 10, cap: 20, halfLifeDays: 10 },
    { code: "C02", label: "Narrowing down", points: 10, cap: 10, halfLifeDays: 10 },
    { code: "C01", label: "Comparison shopping", points: 8, cap: 8, halfLifeDays: 14 },
    { code: "C03", label: "Coming back to one project", points: 8, cap: 16, halfLifeDays: 14 },
  ],
  thresholds: { hot: 60, warm: 30 },
  lastChangedBy: "Head of Marketing & AI",
  lastChangedAt: daysAgo(6),
};

// The fixtures' statuses are random, so calibration from them would be noise.
// These are the shape a working score should show: each step up qualifies
// noticeably more often.
const CALIBRATION: Calibration[] = [
  { temperature: "hot", leads: 412, qualifiedRate: 0.41, dealRate: 0.041 },
  { temperature: "warm", leads: 1_288, qualifiedRate: 0.18, dealRate: 0.012 },
  { temperature: "cold", leads: 1_745, qualifiedRate: 0.06, dealRate: 0.002 },
];

export function scoringSettings(): Scoring {
  const withStatus = LEADS.filter((l) => l.status !== null);
  return { ...scoring, calibration: CALIBRATION, coverage: withStatus.length / LEADS.length };
}

export function updateScoring(input: {
  fit?: { key: string; points: number };
  signal?: { code: string; points?: number; halfLifeDays?: number };
  thresholds?: { hot: number; warm: number };
}): Scoring {
  if (input.fit) {
    const f = scoring.fit.find((x) => x.key === input.fit?.key);
    if (!f) throw new Error("That answer isn't scored any more");
    f.points = input.fit.points;
  }
  if (input.signal) {
    const s = scoring.signals.find((x) => x.code === input.signal?.code);
    if (!s) throw new Error("That signal isn't scored any more");
    if (input.signal.points !== undefined) s.points = input.signal.points;
    if (input.signal.halfLifeDays !== undefined) {
      if (input.signal.halfLifeDays < 1) throw new Error("A half-life is at least one day");
      s.halfLifeDays = input.signal.halfLifeDays;
    }
  }
  if (input.thresholds) {
    if (input.thresholds.warm >= input.thresholds.hot)
      throw new Error("Warm has to start below hot");
    scoring.thresholds = input.thresholds;
  }
  scoring.lastChangedBy = "Hisham Ahamad";
  scoring.lastChangedAt = new Date().toISOString();
  return scoringSettings();
}

// ——— Revenue ———

const DEAL_VALUE = 1_900_000_00; // AED 1.9M average, in fils

export function revenue(): Revenue {
  const sumBy = (platform: "google" | "meta") => {
    const cs = CAMPAIGNS.filter((c) => c.platform === platform);
    return {
      spendMinor: cs.reduce((a, c) => a + c.spendMinor, 0),
      leads: cs.reduce((a, c) => a + c.leads, 0),
      qualified: cs.reduce((a, c) => a + c.qualified, 0),
      deals: cs.reduce((a, c) => a + c.deals, 0),
      revenueMinor: cs.reduce((a, c) => a + (c.revenueMinor ?? 0), 0),
    };
  };
  const portals = portalRows();
  const byChannel: RevenueRow[] = [
    { key: "meta", label: "Meta", ...sumBy("meta") },
    { key: "google", label: "Google", ...sumBy("google") },
    {
      key: "portals",
      label: "Portals",
      spendMinor: portals.reduce((a, p) => a + (p.spendMinor ?? 0), 0),
      leads: portals.reduce((a, p) => a + p.leads, 0),
      qualified: portals.reduce((a, p) => a + p.qualified, 0),
      deals: portals.reduce((a, p) => a + p.deals, 0),
      revenueMinor: portals.some((p) => p.deals > 0)
        ? portals.reduce((a, p) => a + p.deals, 0) * DEAL_VALUE
        : null,
    },
    {
      key: "telesales",
      label: "Telesales",
      spendMinor: 0,
      leads: 212,
      qualified: 41,
      deals: 6,
      revenueMinor: 6 * DEAL_VALUE,
    },
    {
      key: "walk-in",
      label: "Walk-ins and referrals",
      spendMinor: 0,
      leads: 64,
      qualified: 30,
      deals: 9,
      revenueMinor: 9 * DEAL_VALUE,
    },
  ];

  return { byChannel, dealsMissingValue: 7 };
}

// ——— Ads ———

// Ads paused through a proposal in this session.
export const pausedAds = new Set<string>();

const AD_NAMES = [
  "Aerial 15s",
  "Payment plan carousel",
  "Show unit walk-through",
  "Lifestyle reel",
  "Founder message",
];

export function ads(campaignId: string): AdRow[] {
  const c = CAMPAIGNS.find((x) => x.id === campaignId);
  if (!c) return [];
  const seed = Number(campaignId.replace(/\D/g, "")) || 1;
  const sets = c.adSets.length ? c.adSets.map((a) => a.name) : ["All"];
  const count = c.platform === "meta" ? 5 : 3;
  return Array.from({ length: count }, (_, i) => {
    const share = [0.34, 0.26, 0.18, 0.14, 0.08][i] ?? 0.1;
    const fatigued = c.platform === "meta" && (seed + i) % 4 === 0;
    const disapproved = (seed * 3 + i) % 11 === 0;
    const soldOut = c.platform === "meta" && i === 2 && seed % 3 === 0;
    const startCtr = 0.012 + ((seed + i) % 5) * 0.002;
    const ctrWeeks = Array.from({ length: 6 }, (_, w) =>
      fatigued ? startCtr * (1 - w * 0.11) : startCtr * (1 + Math.sin(w + i) * 0.05),
    );
    return {
      id: `${campaignId}-ad${i + 1}`,
      name:
        c.platform === "meta" ? (AD_NAMES[i] ?? `Ad ${i + 1}`) : `Responsive search ad ${i + 1}`,
      adSet: sets[i % sets.length] ?? "All",
      status: disapproved
        ? "disapproved"
        : c.status === "paused" || pausedAds.has(`${campaignId}-ad${i + 1}`)
          ? "paused"
          : "active",
      spendMinor: Math.round(c.spendMinor * share),
      leads: Math.round(c.leads * share),
      frequency: c.platform === "meta" ? (fatigued ? 6.4 : 1.8 + i * 0.6) : null,
      ctr: ctrWeeks[ctrWeeks.length - 1] ?? null,
      ctrWeeks,
      flag: disapproved ? "disapproved" : soldOut ? "sold_out" : fatigued ? "fatigued" : null,
    };
  });
}
