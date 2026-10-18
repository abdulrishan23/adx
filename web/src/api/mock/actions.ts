// Proposing changes and project pages for the design branch. Proposals follow
// D-13: allowed actions apply straight away and are logged, the rest wait for
// approval, and nothing gets past the guardrails.

import type { ChangeRequest, Lead, ProjectSummary, Proposal } from "../models";
import { CAMPAIGNS, DAY, LEADS, PROJECTS, PROPOSALS } from "./data";
import { lists } from "./more";
import { ads, guardrails, pausedAds } from "./ops";

const fmt = (minor: number) => `AED ${Math.round(minor / 100).toLocaleString("en")}`;

// Budgets changed through an applied proposal in this session.
const budgets = new Map<string, number>();

// The current daily budget, read from the last week's spend.
export function dailyBudget(campaignId: string): number {
  const c = CAMPAIGNS.find((x) => x.id === campaignId);
  if (!c) throw new Error("That campaign no longer exists");
  const set = budgets.get(campaignId);
  if (set !== undefined) return set;
  const week = c.daily.slice(-7);
  return (
    Math.round(week.reduce((a, p) => a + p.spendMinor, 0) / Math.max(1, week.length) / 100) * 100
  );
}

let seq = PROPOSALS.length;

export function proposeChange(req: ChangeRequest): Proposal {
  const c = CAMPAIGNS.find((x) => x.id === req.campaignId);
  if (!c) throw new Error("That campaign no longer exists");
  const g = guardrails();
  const account = g.accounts.find((a) => a.platform === c.platform);
  if (account?.killed) throw new Error(`The kill switch is on for ${account.name}`);
  if (g.changesToday >= g.changesPerDay) throw new Error("Today's change limit is used up");

  const now = new Date();
  const base = {
    id: `p${++seq}`,
    platform: c.platform,
    projectId: c.projectId,
    createdAt: now.toISOString(),
    expiresAt: new Date(now.getTime() + g.proposalHours * 3_600_000).toISOString(),
    source: "marketer" as const,
    requestedBy: "Hisham Ahamad",
    evidence: [] as Proposal["evidence"],
    reversible: true,
  };

  let p: Proposal;
  if (req.kind === "pause_ad") {
    const ad = ads(c.id).find((a) => a.id === req.adId);
    if (!ad) throw new Error("That ad no longer exists");
    if (ad.status !== "active") throw new Error("That ad isn't running");
    p = {
      ...base,
      title: `Pause ${ad.name}`,
      action: "Pause an ad",
      target: `${c.name} · ${ad.adSet}`,
      status: "applied",
      problem:
        ad.flag === "fatigued" ? "Frequency over 6 and click rate falling." : "Paused by hand.",
      changes: [{ field: "Status", before: "Active", after: "Paused" }],
      evidence: [
        { label: "Spend, 30 days", value: fmt(ad.spendMinor) },
        { label: "Leads, 30 days", value: String(ad.leads) },
      ],
      impact: { label: `Saves about ${fmt(ad.spendMinor)} a month`, kind: "estimate" },
      needsApproval: false,
      decidedBy: "Allowed by the checklist",
    };
  } else {
    const current = dailyBudget(c.id);
    const next = req.dailyBudgetMinor;
    if (!Number.isFinite(next) || next <= 0) throw new Error("The budget must be more than zero");
    if (next === current) throw new Error("That's the current budget");
    const raise = next > current;
    const pct = Math.round(((next - current) / current) * 100);
    if (raise && pct > g.dailyIncreaseCapPct) {
      throw new Error(
        `That's a ${pct}% raise. The cap is ${g.dailyIncreaseCapPct}% a day, so the most today is ${fmt(Math.floor(current * (1 + g.dailyIncreaseCapPct / 100)))}.`,
      );
    }
    p = {
      ...base,
      title: `${raise ? "Raise" : "Lower"} ${c.name} to ${fmt(next)} a day`,
      action: raise ? "Raise a budget within the cap" : "Lower a budget",
      target: c.name,
      status: raise ? "pending" : "applied",
      problem: raise ? "Proposed from the campaign page." : "Lowered by hand.",
      changes: [{ field: "Daily budget", before: fmt(current), after: fmt(next) }],
      evidence: [
        {
          label: "Cost per qualified, 30 days",
          value: c.qualified ? fmt(c.spendMinor / c.qualified) : "Not available",
        },
        { label: "Change", value: `${pct > 0 ? "+" : ""}${pct}%` },
      ],
      impact: {
        label: `${raise ? "About" : "Saves about"} ${fmt(Math.abs(next - current) * 30)} a month`,
        kind: "estimate",
      },
      needsApproval: raise,
      ...(raise ? {} : { decidedBy: "Allowed by the checklist" }),
    };
  }
  p.log = [
    { at: p.createdAt, text: "Proposed by Hisham Ahamad" },
    ...(p.status === "applied"
      ? [{ at: p.createdAt, text: "Applied: allowed by the checklist" }]
      : []),
  ];
  if (req.kind === "pause_ad") pausedAds.add(req.adId);
  if (req.kind === "daily_budget" && p.status === "applied")
    budgets.set(c.id, req.dailyBudgetMinor);
  PROPOSALS.unshift(p);
  g.changesToday += p.status === "applied" ? 1 : 0;
  return p;
}

export function projectSummary(id: number, days: number): ProjectSummary | null {
  const project = PROJECTS.find((p) => p.id === id);
  if (!project) return null;
  const cs = CAMPAIGNS.filter((c) => c.projectId === id);
  const daily = new Map<
    string,
    { date: string; spendMinor: number; leads: number; qualified: number }
  >();
  for (const c of cs) {
    for (const p of c.daily.slice(-days)) {
      const d = daily.get(p.date) ?? { date: p.date, spendMinor: 0, leads: 0, qualified: 0 };
      d.spendMinor += p.spendMinor;
      d.leads += p.leads;
      d.qualified += p.qualified;
      daily.set(p.date, d);
    }
  }
  const points = [...daily.values()].sort((a, b) => a.date.localeCompare(b.date));
  const sum = (k: "spendMinor" | "leads" | "qualified") => points.reduce((a, p) => a + p[k], 0);
  const leads = LEADS.filter((l) => l.projectId === id);
  const cutoff = Date.now() - days * DAY;
  const recent = leads.filter((l) => Date.parse(l.arrivedAt) >= cutoff);
  const mix = { hot: 0, warm: 0, cold: 0 };
  for (const l of recent) mix[l.temperature] += 1;
  const deals = cs.reduce((a, c) => a + c.deals, 0);
  return {
    id,
    name: project.name,
    city: project.city,
    aliases: lists().aliases.find((a) => a.projectId === id)?.aliases ?? [project.name],
    spendMinor: sum("spendMinor"),
    leads: sum("leads"),
    qualified: sum("qualified"),
    deals,
    revenueMinor: deals > 0 ? cs.reduce((a, c) => a + (c.revenueMinor ?? 0), 0) : null,
    daily: points,
    campaigns: cs,
    hotLeads: leads
      .filter((l: Lead) => l.temperature === "hot")
      .sort((a, b) => Date.parse(b.lastActivityAt) - Date.parse(a.lastActivityAt))
      .slice(0, 6),
    temperatureMix: mix,
  };
}
