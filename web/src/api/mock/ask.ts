// Scripted Ask Adx answers for the design branch. The real copilot runs on the
// latest Sonnet (architecture.md, D-18). Each one is worked out from
// the same fixtures the other screens use, so a figure in an answer matches the
// figure on Campaigns or Leads. A question that matches none of them gets an
// honest "can't answer that yet" with suggestions, never an invented figure.

import { formatMoneyShort, formatNumber, formatPercent } from "@/lib/format";
import type { AskAnswer, Conversation, DailyPoint } from "../models";
import { dailyBudget } from "./actions";
import { CAMPAIGNS, daysAgo, hoursAgo, LEADS, PROJECTS, PROPOSALS, projectOf } from "./data";
import { dataQuality, portalRows } from "./more";
import { ads, guardrails } from "./ops";

export const SUGGESTIONS = [
  "Why did cost per qualified lead change this week?",
  "Which leads should sales call first today?",
  "Which project is cheapest per Done-Deal?",
  "Are any forms losing leads before GoHighLevel?",
  "What are the portals costing us this month?",
  "Which ads are fatigued?",
  "Give Wadi GCC more budget",
];

const sum = (points: DailyPoint[], key: "spendMinor" | "leads" | "qualified") =>
  points.reduce((a, p) => a + p[key], 0);

const per = (minor: number, n: number) => (n > 0 ? Math.round(minor / n) : null);

let seq = 0;
const nextId = () => `ans-${Date.now().toString(36)}-${seq++}`;

type Script = {
  match: RegExp;
  answer: (question: string) => Omit<AskAnswer, "id" | "question" | "askedAt">;
};

const SCRIPTS: Script[] = [
  {
    match: /fatigue|tired|frequency|ads? (are )?(worn|stale)/i,
    answer: () => {
      const tired = CAMPAIGNS.filter((c) => c.platform === "meta").flatMap((c) =>
        ads(c.id)
          .filter((a) => a.flag === "fatigued" && a.status === "active")
          .map((a) => ({ c, a })),
      );
      const first = tired[0];
      return {
        steps: [
          { label: "Read 6 weeks of ad delivery from Meta", rows: tired.length * 42 || 0 },
          { label: "Flagged ads with frequency over 6 and falling click rate" },
        ],
        paragraphs: tired.length
          ? [
              `**${tired.length} ${tired.length === 1 ? "ad is" : "ads are"} fatigued**: shown to the same people more than 6 times, with click rate down for 4 weeks.`,
              first ? `Pausing **${first.a.name}** in ${first.c.name} is the clearest case.` : "",
            ].filter(Boolean)
          : ["No running Meta ad is fatigued right now."],
        table: tired.length
          ? {
              columns: [
                { label: "Ad" },
                { label: "Campaign" },
                { label: "Frequency", numeric: true },
                { label: "CTR", numeric: true },
              ],
              rows: tired.map(({ c, a }) => ({
                cells: [
                  a.name,
                  c.name,
                  a.frequency?.toFixed(1) ?? "Not available",
                  formatPercent(a.ctr, 1),
                ],
              })),
            }
          : undefined,
        draft: first
          ? {
              title: `Pause ${first.a.name}`,
              request: { kind: "pause_ad", campaignId: first.c.id, adId: first.a.id },
              changes: [{ field: `${first.a.name} status`, before: "Active", after: "Paused" }],
              needsApproval: false,
            }
          : undefined,
        followUps: ["Why did cost per qualified lead change this week?"],
        sources: ["Meta"],
      };
    },
  },
  {
    match: /more budget|raise|increase.*budget|scale/i,
    answer: () => {
      const c = CAMPAIGNS.find((x) => x.name === "Wadi | Leads | GCC") ?? CAMPAIGNS[0];
      if (!c)
        return { steps: [], paragraphs: ["No campaign to raise."], followUps: [], sources: [] };
      const now = dailyBudget(c.id);
      const cap = guardrails().dailyIncreaseCapPct;
      const next = Math.round((now * (1 + Math.min(15, cap) / 100)) / 1000) * 1000;
      const cpq = c.qualified ? Math.round(c.spendMinor / c.qualified) : null;
      return {
        steps: [
          { label: "Read 30 days of spend and qualified leads for the campaign", rows: 30 },
          { label: `Checked the ${cap}% daily increase cap` },
        ],
        paragraphs: [
          `**${c.name}** qualifies leads at ${formatMoneyShort(cpq)} each, below the account average. A raise from ${formatMoneyShort(now)} to **${formatMoneyShort(next)} a day** stays inside the ${cap}% cap.`,
          "Watch cost per qualified lead for a week before raising again.",
        ],
        draft: {
          title: `Raise ${c.name} to ${formatMoneyShort(next)} a day`,
          request: { kind: "daily_budget", campaignId: c.id, dailyBudgetMinor: next },
          changes: [
            { field: "Daily budget", before: formatMoneyShort(now), after: formatMoneyShort(next) },
          ],
          needsApproval: true,
        },
        followUps: ["Which project is cheapest per Done-Deal?"],
        sources: ["Meta", "GoHighLevel"],
      };
    },
  },
  {
    match: /cost per qualified|cpql|cost.*(went|go) up|cost.*change/i,
    answer: () => {
      const rows = CAMPAIGNS.map((c) => {
        const now7 = c.daily.slice(-7);
        const prev7 = c.daily.slice(-14, -7);
        return {
          c,
          cur: per(sum(now7, "spendMinor"), sum(now7, "qualified")),
          prev: per(sum(prev7, "spendMinor"), sum(prev7, "qualified")),
          spend: sum(now7, "spendMinor"),
          qualified: sum(now7, "qualified"),
        };
      });
      const all7 = rows.reduce((a, r) => a + r.spend, 0);
      const allQ = rows.reduce((a, r) => a + r.qualified, 0);
      const prevSpend = CAMPAIGNS.reduce(
        (a, c) => a + sum(c.daily.slice(-14, -7), "spendMinor"),
        0,
      );
      const prevQ = CAMPAIGNS.reduce((a, c) => a + sum(c.daily.slice(-14, -7), "qualified"), 0);
      const cur = per(all7, allQ);
      const prev = per(prevSpend, prevQ);
      const moved = cur !== null && prev !== null && prev > 0 ? (cur - prev) / prev : null;
      const worst = [...rows]
        .filter((r) => r.cur !== null && r.prev !== null)
        .sort((a, b) => (b.cur ?? 0) / (b.prev || 1) - (a.cur ?? 0) / (a.prev || 1))
        .slice(0, 5);
      const top = worst[0];
      return {
        steps: [
          { label: "Read 14 days of spend from Google Ads and Meta", rows: CAMPAIGNS.length * 14 },
          { label: "Joined leads to their GoHighLevel status", rows: LEADS.length },
          { label: "Compared this week with last week, per campaign" },
        ],
        paragraphs: [
          moved === null
            ? "There isn't enough history to compare this week with last week."
            : `Cost per qualified lead is **${formatMoneyShort(cur)}** this week, ${moved >= 0 ? "up" : "down"} **${formatPercent(Math.abs(moved))}** on last week's ${formatMoneyShort(prev)}.`,
          top
            ? `The biggest mover is **${top.c.name}**: ${formatMoneyShort(top.prev)} to ${formatMoneyShort(top.cur)} per qualified lead. Its spend held steady while fewer of its leads were marked qualified, so check whether sales got to them before reading this as an ad problem.`
            : "",
          "Half of all leads still have no status, so qualified counts lag by a few days. The figure usually settles by Thursday.",
        ].filter(Boolean),
        table: {
          columns: [
            { label: "Campaign" },
            { label: "Last week", numeric: true },
            { label: "This week", numeric: true },
            { label: "Qualified", numeric: true },
          ],
          rows: worst.map((r) => ({
            cells: [
              r.c.name,
              formatMoneyShort(r.prev),
              formatMoneyShort(r.cur),
              formatNumber(r.qualified),
            ],
          })),
        },
        proposal: PROPOSALS[1] ? { id: PROPOSALS[1].id, title: PROPOSALS[1].title } : undefined,
        followUps: [
          "Which leads should sales call first today?",
          "Which project is cheapest per Done-Deal?",
        ],
        sources: ["Google Ads", "Meta", "GoHighLevel"],
      };
    },
  },
  {
    match: /call first|who.*call|which leads|hot leads|follow up/i,
    answer: () => {
      const hot = LEADS.filter((l) => l.temperature === "hot")
        .sort((a, b) => Date.parse(b.lastActivityAt) - Date.parse(a.lastActivityAt))
        .slice(0, 6);
      const blank = hot.filter((l) => l.status === null).length;
      return {
        steps: [
          { label: "Read leads and statuses from GoHighLevel", rows: LEADS.length },
          { label: "Scored website and WhatsApp behaviour from the last 14 days" },
          { label: "Ranked hot leads by their latest activity" },
        ],
        paragraphs: [
          `**${hot.length} hot leads** are worth a call today${blank ? `, and **${blank}** of them have no status yet, so nobody has spoken to them` : ""}.`,
          "Each one came back on their own: a repeat visit to a payment plan, a floor plan download or a WhatsApp reply. Open a lead to see exactly what they did.",
        ],
        table: {
          columns: [
            { label: "Lead" },
            { label: "Project" },
            { label: "Owner" },
            { label: "Why now" },
          ],
          rows: hot.map((l) => ({
            leadId: l.id,
            cells: [
              l.name,
              l.projectId === null ? "Not set" : projectOf(l.projectId),
              l.owner,
              l.reason,
            ],
          })),
        },
        followUps: [
          "Are any forms losing leads before GoHighLevel?",
          "Which project is cheapest per Done-Deal?",
        ],
        sources: ["GoHighLevel", "Website", "WhatsApp"],
      };
    },
  },
  {
    match: /cheapest|per (done-)?deal|cost per deal|best project|which project/i,
    answer: () => {
      const rows = PROJECTS.map((p) => {
        const cs = CAMPAIGNS.filter((c) => c.projectId === p.id);
        const spend = cs.reduce((a, c) => a + c.spendMinor, 0);
        const deals = cs.reduce((a, c) => a + c.deals, 0);
        const leads = cs.reduce((a, c) => a + c.leads, 0);
        return { p, spend, deals, leads, perDeal: per(spend, deals) };
      }).sort(
        (a, b) => (a.perDeal ?? Number.POSITIVE_INFINITY) - (b.perDeal ?? Number.POSITIVE_INFINITY),
      );
      const best = rows.find((r) => r.perDeal !== null);
      const none = rows.filter((r) => r.deals === 0 && r.spend > 0);
      return {
        steps: [
          { label: "Read 30 days of spend per campaign", rows: CAMPAIGNS.length * 30 },
          { label: "Counted Done-Deals in GoHighLevel by project" },
        ],
        paragraphs: [
          best
            ? `**${best.p.name}** is cheapest at **${formatMoneyShort(best.perDeal)} per Done-Deal** over the last 30 days, from ${best.deals} deals.`
            : "No project closed a deal in the last 30 days.",
          none.length
            ? `${none.map((r) => r.p.name).join(" and ")} spent ${formatMoneyShort(none.reduce((a, r) => a + r.spend, 0))} with no deals yet. Off-plan deals often take more than 30 days, so this is worth watching rather than cutting.`
            : "",
        ].filter(Boolean),
        table: {
          columns: [
            { label: "Project" },
            { label: "Spend", numeric: true },
            { label: "Leads", numeric: true },
            { label: "Deals", numeric: true },
            { label: "Per deal", numeric: true },
          ],
          rows: rows.map((r) => ({
            cells: [
              r.p.name,
              formatMoneyShort(r.spend),
              formatNumber(r.leads),
              formatNumber(r.deals),
              formatMoneyShort(r.perDeal),
            ],
          })),
        },
        followUps: ["Why did cost per qualified lead change this week?"],
        sources: ["Google Ads", "Meta", "GoHighLevel"],
      };
    },
  },
  {
    match: /losing leads|missing|sync|forms|reach(ed)? gohighlevel/i,
    answer: () => {
      const gaps = dataQuality()
        .syncGaps.map((g) => ({ ...g, gap: Math.max(0, g.platformLeads - g.crmLeads) }))
        .sort((a, b) => b.gap - a.gap);
      const total = gaps.reduce((a, g) => a + g.gap, 0);
      const worst = gaps[0];
      return {
        steps: [
          { label: "Read lead counts per form from Meta and Google", rows: gaps.length },
          { label: "Matched them to contacts in GoHighLevel" },
        ],
        paragraphs: [
          `**${formatNumber(total)} leads** that the platforms delivered never reached GoHighLevel in the last 30 days.`,
          worst
            ? `Most of them are from **${worst.form}** (${formatNumber(worst.gap)} of ${formatNumber(worst.platformLeads)}). That size of gap usually means the form's connection to GoHighLevel broke, not that people dropped off.`
            : "",
        ].filter(Boolean),
        table: {
          columns: [
            { label: "Form" },
            { label: "Platform", numeric: true },
            { label: "GoHighLevel", numeric: true },
            { label: "Missing", numeric: true },
          ],
          rows: gaps.map((g) => ({
            cells: [
              g.form,
              formatNumber(g.platformLeads),
              formatNumber(g.crmLeads),
              formatNumber(g.gap),
            ],
          })),
        },
        followUps: ["Which leads should sales call first today?"],
        sources: ["Meta", "Google Ads", "GoHighLevel"],
      };
    },
  },
  {
    match: /portal|bayut|property ?finder|dubizzle/i,
    answer: () => {
      const rows = portalRows();
      const spend = rows.reduce((a, r) => a + (r.spendMinor ?? 0), 0);
      const missing = rows.filter((r) => r.spendMinor === null);
      return {
        steps: [
          { label: "Read portal leads from GoHighLevel" },
          { label: "Read the spend typed in on Portals" },
        ],
        paragraphs: [
          `Portals have cost **${formatMoneyShort(spend)}** so far this month across ${rows.length - missing.length} portals.`,
          missing.length
            ? `${missing.map((r) => r.name).join(" and ")} ${missing.length === 1 ? "has" : "have"} no spend typed in, so ${missing.length === 1 ? "its" : "their"} cost per lead is Not available.`
            : "",
        ].filter(Boolean),
        table: {
          columns: [
            { label: "Portal" },
            { label: "Spend", numeric: true },
            { label: "Leads", numeric: true },
            { label: "Deals", numeric: true },
          ],
          rows: rows.map((r) => ({
            cells: [
              r.name,
              formatMoneyShort(r.spendMinor),
              formatNumber(r.leads),
              formatNumber(r.deals),
            ],
          })),
        },
        followUps: ["Which project is cheapest per Done-Deal?"],
        sources: ["GoHighLevel", "Portals spend"],
      };
    },
  },
];

export function ask(question: string): AskAnswer {
  const script = SCRIPTS.find((s) => s.match.test(question));
  const base = { id: nextId(), question, askedAt: new Date().toISOString() };
  if (script) return { ...base, ...script.answer(question) };
  return {
    ...base,
    steps: [{ label: "Looked for a matching report in the connected data" }],
    paragraphs: [
      "I can't answer that one yet. Right now I can explain cost changes, rank leads for sales, compare projects, find leads lost on the way to GoHighLevel and total up portal spend.",
    ],
    followUps: SUGGESTIONS.slice(0, 3),
    sources: [],
  };
}

const seeded = (question: string, at: string): AskAnswer => ({ ...ask(question), askedAt: at });

export const CONVERSATIONS: Conversation[] = [
  {
    id: "cv1",
    title: "Cost per qualified lead this week",
    updatedAt: hoursAgo(3),
    turns: [seeded(SUGGESTIONS[0] as string, hoursAgo(3))],
  },
  {
    id: "cv2",
    title: "Cheapest project per deal",
    updatedAt: daysAgo(1),
    turns: [seeded(SUGGESTIONS[2] as string, daysAgo(1))],
  },
  {
    id: "cv3",
    title: "Forms losing leads",
    updatedAt: daysAgo(4),
    turns: [seeded(SUGGESTIONS[3] as string, daysAgo(4))],
  },
];
