// A pretend server for the design branch. Each function answers what its
// endpoint will answer, computed from the fixtures in data.ts, so screens
// exercise real filtering and maths instead of hard-coded tiles.

import type {
  Campaign,
  DailyPoint,
  Funnel,
  FunnelRow,
  FunnelStage,
  Lead,
  LeadFilter,
  LeadsSummary,
  Proposal,
  Scope,
} from "../models";
import { ALERTS, CAMPAIGNS, CONNECTIONS, DAY, LEADS, now, PROJECTS, PROPOSALS } from "./data";
import { source } from "./sources";

const inScope = (c: Campaign, scope: Scope) =>
  (scope.platform === "all" || c.platform === scope.platform) &&
  (scope.projectId === "all" || c.projectId === scope.projectId);

function window(points: DailyPoint[], days: number, offset = 0) {
  const end = points.length - offset;
  return points.slice(Math.max(0, end - days), end);
}

function total(points: DailyPoint[], key: "spendMinor" | "leads" | "qualified") {
  return points.reduce((acc, p) => acc + p[key], 0);
}

export function campaigns(scope: Scope): Campaign[] {
  return CAMPAIGNS.filter((c) => inScope(c, scope)).map((c) => {
    const w = window(c.daily, scope.days);
    const factor = scope.days / 30;
    return {
      ...c,
      spendMinor: total(w, "spendMinor"),
      leads: total(w, "leads"),
      qualified: total(w, "qualified"),
      deals: Math.round(c.deals * factor),
      revenueMinor: c.revenueMinor === null ? null : Math.round(c.revenueMinor * factor),
      daily: w,
    };
  });
}

const STAGES: { key: string; label: string }[] = [
  { key: "leads", label: "Leads" },
  { key: "qualified", label: "Qualified" },
  { key: "meeting", label: "Meeting" },
  { key: "offer", label: "Offer" },
  { key: "deal", label: "Done-Deal" },
];

export function funnel(scope: Scope): Funnel {
  const rows: FunnelRow[] = PROJECTS.map((p) => {
    const cs = campaigns({ ...scope, projectId: p.id });
    const leads = cs.reduce((a, c) => a + c.leads, 0);
    const qualified = cs.reduce((a, c) => a + c.qualified, 0);
    const deals = cs.reduce((a, c) => a + c.deals, 0);
    // Each stage can hold at most as many leads as the one before it.
    const meeting = Math.min(qualified, Math.max(deals, Math.round(leads * 0.06)));
    const offer = Math.min(meeting, Math.max(deals, Math.round(leads * 0.035)));
    const counts: Record<string, number> = {
      leads,
      qualified,
      meeting,
      offer,
      deal: Math.min(offer, deals),
    };
    const stages: FunnelStage[] = STAGES.map((s) => ({
      key: s.key,
      label: s.label,
      count: counts[s.key] ?? 0,
    }));
    const revenue = cs.reduce((a, c) => a + (c.revenueMinor ?? 0), 0);
    return {
      projectId: p.id,
      spendMinor: cs.reduce((a, c) => a + c.spendMinor, 0),
      stages,
      revenueMinor: deals > 0 ? revenue : null,
    };
  }).filter(
    (r) => r.spendMinor > 0 && (scope.projectId === "all" || r.projectId === scope.projectId),
  );

  const stages = STAGES.map((s, i) => ({
    key: s.key,
    label: s.label,
    count: rows.reduce((a, r) => a + (r.stages[i]?.count ?? 0), 0),
  }));
  const months = [5, 4, 3, 2, 1, 0].map((back) => {
    const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - back, 1));
    const spend = rows.reduce((a, r) => a + r.spendMinor, 0) * (0.85 + ((back * 7) % 5) / 20);
    return {
      month: d.toISOString().slice(0, 7),
      spendMinor: Math.round(spend),
      // The current month's deals are still closing, so revenue is partial.
      revenueMinor: back === 0 ? null : Math.round(spend * (4 + ((back * 3) % 4))),
    };
  });
  return {
    stages,
    spendMinor: rows.reduce((a, r) => a + r.spendMinor, 0),
    revenueMinor: rows.reduce((a, r) => a + (r.revenueMinor ?? 0), 0),
    byProject: rows,
    months,
  };
}

const olderThan = (iso: string, days: number) => now.getTime() - Date.parse(iso) > days * DAY;

export function leads(scope: Scope): Lead[] {
  return LEADS.filter((l) => scope.projectId === "all" || l.projectId === scope.projectId);
}

export function matchesFilter(l: Lead, f: LeadFilter): boolean {
  switch (f) {
    case "hot_no_status":
      return l.temperature === "hot" && !l.status;
    case "no_status_7d":
      return !l.status && olderThan(l.arrivedAt, 7);
    case "back_in_play":
      return l.status === "Not Interested" && l.signals.some((s) => s.role === "hot");
    case "relabelled":
      return l.source !== l.originalSource;
  }
}

export function leadsSummary(scope: Scope): LeadsSummary {
  const all = leads(scope);
  const count = (f: LeadFilter) => all.filter((l) => matchesFilter(l, f)).length;
  const hotNoStatus = count("hot_no_status");
  return {
    headline:
      hotNoStatus > 0
        ? { text: `${hotNoStatus} hot leads have no status yet`, filter: "hot_no_status" }
        : { text: "Every hot lead has been picked up", filter: null },
    stats: [
      {
        key: "hot_no_status",
        label: "hot, no status",
        count: hotNoStatus,
        urgent: hotNoStatus > 0,
      },
      { key: "no_status_7d", label: "no status after 7 days", count: count("no_status_7d") },
      { key: "back_in_play", label: "back in play", count: count("back_in_play") },
      { key: "relabelled", label: "source recovered", count: count("relabelled") },
    ],
  };
}

export function lead(id: number): Lead | null {
  return LEADS.find((l) => l.id === id) ?? null;
}

// Approved changes take a few seconds to apply, as a real API call would.
// One of them fails the first time, to show what a failure looks like.
const applying = new Map<string, number>();
const failOnce = new Set(["p3"]);
const APPLY_MS = 4000;

function log(p: Proposal, text: string) {
  p.log = [...(p.log ?? []), { at: new Date().toISOString(), text }];
}

function settle() {
  const t = Date.now();
  for (const [id, at] of applying) {
    if (at > t) continue;
    applying.delete(id);
    const p = PROPOSALS.find((x) => x.id === id);
    if (!p) continue;
    if (failOnce.delete(id)) {
      p.status = "failed";
      p.failure = "Meta didn't answer in time. Nothing was changed.";
      log(p, "Failed: Meta didn't answer in time");
    } else {
      p.status = "applied";
      p.failure = undefined;
      log(p, "Applied");
    }
  }
}

export function proposals(): Proposal[] {
  settle();
  return [...PROPOSALS].sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt));
}

function startApplying(p: Proposal) {
  p.status = "applying";
  log(p, "Applying");
  applying.set(p.id, Date.now() + APPLY_MS);
}

export function decide(id: string, decision: "approve" | "reject", note?: string): Proposal {
  settle();
  const p = PROPOSALS.find((x) => x.id === id);
  if (!p) throw new Error("That proposal no longer exists");
  if (p.status !== "pending") throw new Error("That proposal was already decided");
  p.decidedBy = "Abdul";
  if (note) p.note = note;
  if (decision === "reject") {
    p.status = "rejected";
    log(p, `Rejected by Abdul${note ? `: ${note}` : ""}`);
  } else {
    log(p, `Approved by Abdul${note ? `: ${note}` : ""}`);
    startApplying(p);
  }
  return p;
}

export function retry(id: string): Proposal {
  settle();
  const p = PROPOSALS.find((x) => x.id === id);
  if (!p) throw new Error("That proposal no longer exists");
  if (p.status !== "failed") throw new Error("Only a failed change can be retried");
  log(p, "Retried by Abdul");
  startApplying(p);
  return p;
}

export function undo(id: string): Proposal {
  settle();
  const p = PROPOSALS.find((x) => x.id === id);
  if (!p) throw new Error("That proposal no longer exists");
  if (p.status !== "applied") throw new Error("Only an applied change can be undone");
  if (!p.reversible) throw new Error("This change can't be undone automatically");
  p.status = "undone";
  log(p, "Undone by Abdul. The previous values are back.");
  return p;
}

// Dismissed alerts leave the list; snoozed ones come back when the snooze ends.
const hidden = new Map<string, { until: number | null; reason: string }>();

export function alerts() {
  const t = Date.now();
  return ALERTS.filter((a) => {
    const h = hidden.get(a.id);
    return !h || (h.until !== null && h.until <= t);
  });
}

export function dismissAlert(id: string, reason: string) {
  if (!ALERTS.some((a) => a.id === id)) throw new Error("That alert no longer exists");
  hidden.set(id, { until: null, reason });
  return { id };
}

export function snoozeAlert(id: string, days: number) {
  if (!ALERTS.some((a) => a.id === id)) throw new Error("That alert no longer exists");
  hidden.set(id, { until: Date.now() + days * DAY, reason: "snoozed" });
  return { id };
}

// Cards follow each source's connect page, so connecting there shows here.
export function connections() {
  return CONNECTIONS.map((c) => {
    const s = source(c.key);
    if (!s) return c;
    const accounts = s.accounts.filter((a) => a.connected);
    const synced = accounts
      .map((a) => a.lastSyncAt)
      .filter((t): t is string => t !== null)
      .sort();
    return {
      ...c,
      state: s.state,
      stateLabel: s.stateLabel,
      detail: s.issue ?? c.detail,
      accounts: accounts.map((a) => a.name),
      lastSyncAt: synced[synced.length - 1] ?? null,
    };
  });
}
