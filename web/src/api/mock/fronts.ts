// The pretend server for the fronts. Spend, leads and alerts come from the same
// fixtures Campaigns, Leads and Alerts read, so a figure on Home matches the
// figure on the screen it opens. Create's loop, plans and creatives are
// scripted, since those systems arrive in phases 2 and 3.

import { formatMoneyShort, formatNumber } from "@/lib/format";
import type { Card, HomeDesk, ListItem, Template, WorkItem } from "../fronts";
import type { Scope } from "../models";
import { hoursAgo, now, PROJECTS } from "./data";
import { alerts, campaigns, leads, matchesFilter } from "./server";

const MONTH: Scope = { days: 30, platform: "all", projectId: "all" };
const BUDGET_MINOR = 950_000_00;
const sum = (xs: number[]) => xs.reduce((a, b) => a + b, 0);
const pct = (part: number, whole: number) => (whole > 0 ? Math.round((part / whole) * 100) : 0);

// ---------------------------------------------------------------- figures

function money() {
  const cs = campaigns(MONTH);
  // Month to date, so Home agrees with Analytics' default range.
  const monthStart = `${now.toISOString().slice(0, 8)}01`;
  const mtd = (key: "spendMinor" | "qualified") =>
    sum(cs.map((c) => sum(c.daily.filter((d) => d.date >= monthStart).map((d) => d[key]))));
  const spend = mtd("spendMinor");
  const qualified = mtd("qualified");
  const dayOfMonth = now.getUTCDate();
  const daysInMonth = new Date(now.getUTCFullYear(), now.getUTCMonth() + 1, 0).getUTCDate();
  const elapsed = pct(dayOfMonth, daysInMonth);
  const spent = pct(spend, BUDGET_MINOR);
  const byPlatform = (p: "google" | "meta") =>
    sum(
      cs
        .filter((c) => c.platform === p)
        .map((c) => sum(c.daily.filter((d) => d.date >= monthStart).map((d) => d.spendMinor))),
    );
  const byProject = PROJECTS.map((p) => ({
    project: p,
    spend: sum(cs.filter((c) => c.projectId === p.id).map((c) => c.spendMinor)),
  }))
    .filter((r) => r.spend > 0)
    .sort((a, b) => b.spend - a.spend);
  // Qualified leads per day across every campaign, last 14 days.
  const days = cs[0]?.daily.slice(-14).map((d) => d.date) ?? [];
  const perDay = days.map((date) =>
    sum(cs.map((c) => c.daily.find((d) => d.date === date)?.qualified ?? 0)),
  );
  const last7 = sum(perDay.slice(-7));
  const prev7 = sum(perDay.slice(-14, -7));
  return {
    cs,
    spend,
    qualified,
    cpq: qualified > 0 ? Math.round(spend / qualified) : null,
    spent,
    elapsed,
    onPace: spent <= elapsed + 5,
    meta: byPlatform("meta"),
    google: byPlatform("google"),
    byProject,
    days,
    perDay,
    weekChange: prev7 > 0 ? Math.round(((last7 - prev7) / prev7) * 100) : 0,
    dayLabels: days.map((d) => String(Number(d.slice(8, 10)))),
  };
}

// waited says how long a lead has sat, in the unit that reads naturally.
const waited = (ms: number) => {
  const hours = Math.round(ms / 3_600_000);
  return hours < 48 ? `${hours} h` : `${Math.round(hours / 24)} days`;
};

function people() {
  const all = leads(MONTH);
  const hot = all.filter((l) => matchesFilter(l, "hot_no_status"));
  hot.sort((a, b) => new Date(a.arrivedAt).getTime() - new Date(b.arrivedAt).getTime());
  const oldest = hot.reduce(
    (acc, l) => Math.max(acc, now.getTime() - new Date(l.arrivedAt).getTime()),
    0,
  );
  const sql = all.filter(
    (l) => l.status !== null && ["Meeting", "Sales offer"].includes(l.status),
  ).length;
  return { hot, oldest: waited(oldest), sqlText: formatNumber(sql) };
}

const DATE = new Intl.DateTimeFormat("en-GB", {
  weekday: "long",
  day: "numeric",
  month: "long",
  timeZone: "Asia/Dubai",
}).format(now);

// ---------------------------------------------------------------- shared blocks

function changedList(): ListItem[] {
  const top = alerts()
    .filter((a) => a.severity !== "low")
    .slice(0, 3);
  return top.map((a) => ({
    status: a.severity === "high" ? "error" : "caution",
    title: a.title,
    detail: a.detail,
    tag: a.target,
    action: { label: "Open", to: "/inbox" },
  }));
}

// ---------------------------------------------------------------- home desk

// The first four are the ones Home shows as pills; the rest open with
// "Show more templates". Grouped by door in the catalog.
const t = (
  id: string,
  door: Template["door"],
  icon: Template["icon"],
  title: string,
  description: string,
  prompt = title,
): Template => ({ id, door, icon, title, description, prompt });

const TEMPLATES: Template[] = [
  t(
    "t1",
    "perform",
    "changes",
    "What changed since yesterday?",
    "The daily check in plain words: what moved, by how much, and whether it matters.",
  ),
  t(
    "t2",
    "perform",
    "cost",
    "Why did cost per qualified lead change?",
    "Breaks the change down by brand, channel and campaign, and names the cause.",
    "Why did cost per qualified lead change this week?",
  ),
  t(
    "t3",
    "perform",
    "waste",
    "Where is money going without leads?",
    "Ad sets, keywords and search terms spending with nothing to show for it.",
    "Where is money going without qualified leads?",
  ),
  t(
    "t4",
    "convert",
    "call",
    "Who should sales call first today?",
    "Hot leads ranked by signal and wait, with the reason for each.",
    "Which leads should sales call first today?",
  ),
  t(
    "t5",
    "perform",
    "pace",
    "Are we on pace this month?",
    "Spend against budget by brand and channel, and where the month will land.",
  ),
  t(
    "t6",
    "perform",
    "compare",
    "Compare the brands this month",
    "Spend, qualified leads and cost per qualified, side by side.",
    "Which project is cheapest per Done-Deal?",
  ),
  t(
    "t7",
    "perform",
    "budget",
    "Which campaigns should get more budget?",
    "Campaigns qualifying leads below target cost, with room to spend.",
  ),
  t(
    "t8",
    "perform",
    "exclude",
    "Search terms to exclude",
    "Terms with spend and no qualified leads, ready to add as negatives.",
  ),
  t(
    "t9",
    "convert",
    "revive",
    "Which leads are back in play?",
    "Cold leads that visited, replied or re-enquired this week.",
    "Which leads came back in play this week?",
  ),
  t(
    "t10",
    "convert",
    "reasons",
    "Why are leads not qualifying?",
    "The reasons sales gives most, by brand and source.",
  ),
  t(
    "t11",
    "convert",
    "sources",
    "Which sources bring buyers?",
    "Leads, qualified leads and Done-Deals by source, portals included.",
  ),
  t(
    "t12",
    "convert",
    "forms",
    "Are any forms losing leads?",
    "Platform leads that never reached GoHighLevel, by form, since when.",
    "Are any forms losing leads before GoHighLevel?",
  ),
  t(
    "t13",
    "studio",
    "launch",
    "What's blocking a launch?",
    "Every brand with a plan approved and campaigns not yet delivering, and why.",
    "What's blocking The Wadi launch?",
  ),
  t(
    "t14",
    "studio",
    "fatigue",
    "Which ads are fatigued?",
    "Ads shown too often with click rate falling, and what to swap in.",
  ),
  t(
    "t15",
    "studio",
    "review",
    "Which creatives are waiting on review?",
    "Ads with no decision on the review board, oldest first.",
  ),
  t(
    "t16",
    "studio",
    "plan",
    "Draft a media plan for a brand",
    "Budget by channel and week from the brand's facts and research.",
  ),
  t(
    "t17",
    "any",
    "report",
    "Draft the weekly report",
    "The CMO's Monday summary, from the same figures as the dashboard.",
    "Draft the weekly report for the CMO.",
  ),
  t(
    "t18",
    "any",
    "summary",
    "Summarise the month for the CEO",
    "Spend, qualified leads, Done-Deals and what changed, in one page.",
  ),
];

// The search terms behind the waste alert, largest first. Scripted until the
// search-term check returns its rows.
const WASTED_TERMS = [
  { term: "abu dhabi rent apartment", aed: 1_920 },
  { term: "jobs reportage", aed: 1_140 },
  { term: "for sale by owner", aed: 860 },
];

// What Adx is doing right now. Scripted until agents run for real.
// Adx's own jobs in progress. Its drafts that change an ad account or contact
// a customer arrive as cards waiting for approval: external writes and
// messaging stay off until an authorized person sets a policy (handover B0.4,
// B4.3).
const WORKING: WorkItem[] = [
  {
    id: "verdana-plan",
    title: "Drafting Verdana plan v2",
    detail: "From the September audit and this week's research.",
    to: "/studio/plans",
  },
  {
    id: "sila-research",
    title: "Refreshing Sila research",
    detail: "Monthly competitor and keyword pass.",
    to: "/studio/research",
  },
];

// Which door an alert check belongs to.
const ALERT_DOOR: Record<string, Card["door"]> = {
  "Creative fatigue": "perform",
  Pacing: "perform",
  Delivery: "perform",
  "Landing page": "studio",
  "Status coverage": "convert",
};

const alertIdFor = (check: string) => alerts().find((a) => a.check === check)?.id;

export function desk(): HomeDesk {
  const m = money();
  const p = people();
  const changed = changedList();
  const spendShort = formatMoneyShort(m.spend);
  const waste = changed.find((c) => /no leads|no qualified/i.test(c.title));
  const lost = changed.find((c) => /never reached/i.test(c.title));
  const drafted: Omit<Card, "at">[] = [
    {
      id: "spend",
      door: "perform",
      status: m.onPace ? "ready" : "caution",
      title: "Spend this month",
      why: `${m.spent}% spent with ${m.elapsed}% of the month gone. ${m.onPace ? "On pace." : "Ahead of pace."}`,
      figure: {
        value: spendShort.replace("AED ", ""),
        count: Number(spendShort.replace(/[^\d.]/g, "")) || undefined,
        unit: "AED",
        change: `of ${formatMoneyShort(BUDGET_MINOR)}`,
      },
      evidence: {
        kind: "pace",
        rows: [
          { label: "Spent", value: `${m.spent}%`, share: m.spent },
          { label: "Month", value: `${m.elapsed}%`, share: m.elapsed },
        ],
        trend: m.perDay,
      },
      question: "Are we on pace this month, and which brand is pulling spend?",
      action: { label: "Analytics", to: "/analytics" },
    },
    {
      id: "qualified",
      door: "perform",
      status: m.weekChange >= 0 ? "ready" : "caution",
      title: "Qualified leads per day",
      why: `${m.weekChange >= 0 ? "Up" : "Down"} ${Math.abs(m.weekChange)}% on last week. Yesterday: ${m.perDay.at(-1)}.`,
      figure: {
        value: formatNumber(m.qualified),
        count: m.qualified,
        unit: "this month",
        change: m.cpq === null ? "cost not available" : `${formatMoneyShort(m.cpq)} each`,
      },
      evidence: { kind: "bars", values: m.perDay, labels: m.dayLabels },
      question: "Why are qualified leads up this week, and will it hold?",
    },
    ...(waste
      ? [
          {
            id: "waste",
            alertId: alertIdFor("Search terms"),
            door: "perform" as const,
            status: "error" as const,
            title: "Money with no leads",
            why: "14 search terms in Abu Dhabi off-plan Search, with no leads.",
            figure: { value: waste.title.match(/AED ([\d,]+)/)?.[1] ?? "", unit: "AED, 30 days" },
            // The three terms that took the most, out of the 14 in the alert.
            evidence: {
              kind: "rows" as const,
              rows: WASTED_TERMS.map((t) => ({
                label: t.term,
                value: formatNumber(t.aed),
                share: pct(t.aed, WASTED_TERMS[0]?.aed ?? 1),
              })),
            },
            question: "Where is money going without qualified leads, and what should we stop?",
            // Adx has drafted the 14 as negative keywords; nothing changes in
            // Google Ads until someone approves them.
            action: { label: "Review drafts", to: "/proposals" as const, primary: true },
          },
        ]
      : []),
    {
      id: "rejected",
      door: "studio",
      status: "error",
      title: "The Wadi launch is blocked",
      why: "Meta rejected two ads: a missing housing category and too much text on the Arabic story.",
      evidence: {
        kind: "ads",
        projectId: 1,
        items: [
          { code: "C1_CASH_AR_9x16", status: "error", note: "Text over 20%" },
          { code: "O1_PLAN10_EN_9x16", status: "caution", note: "Housing category" },
        ],
      },
      question: "What's blocking The Wadi launch?",
      action: { label: "Fix", to: "/studio/launch" },
    },
    {
      id: "hot",
      door: "convert",
      status: p.hot.length > 0 ? "caution" : "ready",
      title: "Hot leads nobody has picked up",
      why:
        p.hot.length > 0
          ? `No status in GoHighLevel. The oldest has waited ${p.oldest}.`
          : "Every hot lead has been picked up.",
      figure: { value: String(p.hot.length), count: p.hot.length, unit: "waiting" },
      evidence: {
        kind: "leads",
        items: p.hot.slice(0, 3).map((l) => ({
          id: l.id,
          name: l.name,
          detail: `${PROJECTS.find((x) => x.id === l.projectId)?.name ?? "No brand"} · ${l.reason}`,
          waited: waited(now.getTime() - new Date(l.arrivedAt).getTime()),
        })),
      },
      question: "Which leads should sales call first today?",
      action: { label: "Call list", to: "/leads" },
    },
    {
      id: "plan",
      door: "studio",
      status: "caution",
      title: "Plan v3 for Sila is waiting on you",
      why: "AED 450K over six weeks, two assumptions not confirmed. Approving it starts the launch.",
      evidence: {
        kind: "pie",
        slices: [
          { label: "Meta", value: 55 },
          { label: "Google", value: 35 },
          { label: "Snapchat", value: 10 },
        ],
      },
      question: "Walk me through plan v3 for Sila and what I'm being asked to assume.",
      action: { label: "Review", to: "/studio/plans", primary: true },
    },
    {
      id: "channels",
      door: "perform",
      status: "ready",
      title: "Spend by channel",
      why: `Meta brings ${pct(m.meta, m.spend)}% of spend this month.`,
      evidence: {
        kind: "pie",
        slices: [
          { label: "Meta", value: m.meta },
          { label: "Google", value: m.google },
        ],
      },
      question: "Is the Meta and Google split right for this month?",
    },
    ...(lost
      ? [
          {
            id: "lost",
            alertId: alertIdFor("Sync gap"),
            door: "convert" as const,
            status: "error" as const,
            title: "Leads that never reached GoHighLevel",
            why: lost.detail ?? "",
            figure: { value: lost.title.match(/[\d,]+/)?.[0] ?? "", unit: "leads" },
            question: "Which forms are losing leads before GoHighLevel, since when?",
            action: { label: "Data quality", to: "/data-quality" as const },
          },
        ]
      : []),
    {
      id: "drafts",
      door: "convert",
      status: "caution",
      title: "Follow-ups drafted, not sent",
      why: "A WhatsApp message for each hot lead nobody has picked up. Sending stays off until a contact policy is set.",
      figure: { value: String(p.hot.length), count: p.hot.length, unit: "drafts" },
      question: "Show me the follow-up drafts and why each lead got that message.",
      action: { label: "Review drafts", to: "/leads" },
    },
    {
      id: "research",
      door: "studio",
      status: "ready",
      title: "The Wadi research refreshed",
      why: "Bloom Living started bidding on 'wadi abu dhabi' on 11 Oct. Two terms added to the plan's watch list.",
      figure: { value: "3", count: 3, unit: "competitor moves" },
      question: "What changed in The Wadi's research, and does the plan need to move?",
      action: { label: "Research", to: "/studio/research" },
    },
  ];
  const ARRIVED: Record<string, number> = {};
  // Every other alert check is a card too, so the Inbox is the one place
  // things that need a person arrive. Search terms and Sync gap have their own
  // richer cards above.
  for (const a of alerts()) {
    if (a.check === "Search terms" || a.check === "Sync gap") continue;
    drafted.push({
      id: `alert-${a.id}`,
      alertId: a.id,
      door: ALERT_DOOR[a.check] ?? "perform",
      status: a.severity === "high" ? "error" : a.severity === "medium" ? "caution" : "pending",
      title: a.title,
      why: a.detail,
      ...(a.costMinor !== null
        ? {
            figure: {
              value: formatMoneyShort(a.costMinor).replace("AED ", ""),
              unit: "AED at stake",
            },
          }
        : {}),
      question: `Tell me more about this: ${a.title}`,
      ...(a.proposalId ? { action: { label: "Review fix", to: "/proposals" as const } } : {}),
    });
    ARRIVED[`alert-${a.id}`] = (now.getTime() - new Date(a.detectedAt).getTime()) / 3_600_000;
  }
  // When each card arrived. Scripted until the server stamps them.
  const ARRIVED_BASE: Record<string, number> = {
    waste: 5,
    rejected: 9,
    hot: 2,
    plan: 96,
    lost: 7,
    drafts: 1,
    research: 3,
    spend: 6,
    qualified: 6,
    channels: 6,
  };
  const cards: Card[] = drafted.map((c) => ({
    ...c,
    at: hoursAgo(ARRIVED[c.id] ?? ARRIVED_BASE[c.id] ?? 6),
  }));
  const needs = cards.filter((c) => c.status === "error" || c.status === "caution").length;
  const summary =
    needs === 0
      ? "Nothing needs your attention."
      : `${needs} ${needs === 1 ? "thing needs" : "things need"} your attention.`;
  return {
    checkedAt: `${DATE} · 08:00 daily check`,
    summary,
    sample: true,
    cards,
    working: WORKING,
    templates: TEMPLATES,
  };
}
