// Enrichment for the design branch, modelled on stafford-dead-leads: Adx looks
// a lead up beyond the form (WhatsApp, LinkedIn, the broker register, earlier
// enquiries in GoHighLevel), then writes a short brief for whoever calls them.
// Every fact here is derived from the lead's id so it stays put between visits.

import type {
  CandidateFilter,
  EnrichedFact,
  EnrichmentJob,
  EnrichmentQueue,
  Lead,
  LeadBrief,
  LeadEnrichment,
} from "../models";
import { daysAgo, hoursAgo, LEADS, OWNERS, PROJECTS, projectOf } from "./data";

function rngFor(seed: number) {
  let s = seed * 9301 + 49297;
  return () => {
    s = (s * 9301 + 49297) % 233280;
    return s / 233280;
  };
}

const JOBS_LIST = [
  ["Head of Finance", "Emirates NBD"],
  ["Senior Engineer", "ADNOC"],
  ["Founder", "a logistics company"],
  ["Doctor", "Cleveland Clinic Abu Dhabi"],
  ["Sales Manager", "Al-Futtaim"],
  ["Consultant", "PwC Middle East"],
  ["Teacher", "GEMS Education"],
  ["Operations Lead", "Careem"],
] as const;

const CITY: Record<string, string[]> = {
  "+971": ["Abu Dhabi", "Dubai", "Sharjah"],
  "+966": ["Riyadh", "Jeddah"],
  "+91": ["Mumbai", "Bengaluru", "Kochi"],
  "+44": ["London", "Manchester"],
  "+92": ["Karachi", "Lahore"],
  "+7": ["Moscow", "Saint Petersburg"],
  "+34": ["Madrid", "Barcelona"],
};

const pick = <T>(r: () => number, xs: readonly T[]): T => xs[Math.floor(r() * xs.length)] as T;

type Profile = {
  whatsapp: boolean;
  corporateEmail: boolean;
  job: (typeof JOBS_LIST)[number] | null;
  jobConfidence: "high" | "medium" | "low";
  city: string;
  earlier: { project: string; via: string; owner: string; monthsAgo: number } | null;
  owns: string | null;
  broker: boolean;
};

function profile(lead: Lead): Profile {
  const r = rngFor(lead.id);
  const cities = CITY[lead.phoneCountry] ?? ["Not available"];
  const other = PROJECTS.filter((p) => p.id !== lead.projectId);
  return {
    whatsapp: r() < 0.85,
    corporateEmail: r() < 0.4,
    job: r() < 0.7 ? pick(r, JOBS_LIST) : null,
    jobConfidence: r() < 0.6 ? "high" : r() < 0.7 ? "medium" : "low",
    city: pick(r, cities),
    earlier:
      r() < 0.3
        ? {
            project: pick(r, other).name,
            via: pick(r, ["Bayut", "Property Finder", "a Meta form", "the website"]),
            owner: pick(r, OWNERS),
            monthsAgo: 2 + Math.floor(r() * 9),
          }
        : null,
    owns: r() < 0.08 ? pick(r, other).name : null,
    broker: r() < 0.05,
  };
}

// Leads Adx hasn't looked up yet: new ones still in their first day, and a
// share of cold ones nobody asked to enrich.
function stateFor(lead: Lead): LeadEnrichment["state"] {
  const r = rngFor(lead.id * 7)();
  if (Date.parse(lead.arrivedAt) > Date.now() - 86_400_000) return r < 0.5 ? "running" : "queued";
  if (lead.temperature === "cold" && r < 0.45) return "never";
  if (r < 0.06) return "failed";
  return "done";
}

function facts(lead: Lead, p: Profile): EnrichedFact[] {
  return [
    { label: "Phone code", value: lead.phoneCountry, source: "GoHighLevel", confidence: "high" },
    {
      label: "WhatsApp",
      value: p.whatsapp ? "Active on this number" : "Not on WhatsApp",
      source: "WhatsApp",
      confidence: "high",
    },
    {
      label: "Email",
      value: p.corporateEmail ? "Work address" : "Personal address",
      source: "GoHighLevel",
      confidence: "high",
    },
    {
      label: "Work",
      value: p.job ? `${p.job[0]}, ${p.job[1]}` : null,
      source: "LinkedIn",
      confidence: p.job ? p.jobConfidence : "low",
    },
    {
      label: "Based in",
      value: p.job ? p.city : null,
      source: "LinkedIn",
      confidence: p.job ? p.jobConfidence : "low",
    },
    {
      label: "Earlier enquiries",
      value: p.earlier
        ? `${p.earlier.project} via ${p.earlier.via}, ${p.earlier.monthsAgo} months ago`
        : "None",
      source: "GoHighLevel",
      confidence: "high",
    },
    {
      label: "Already owns",
      value: p.owns ? `A unit at ${p.owns}` : "Nothing with Reportage",
      source: "GoHighLevel",
      confidence: "high",
    },
    {
      label: "Broker register",
      value: p.broker ? "Listed as a broker" : "Not listed",
      source: "Broker register",
      confidence: "high",
    },
  ];
}

function brief(lead: Lead, p: Profile): LeadBrief {
  const project = lead.projectId ? projectOf(lead.projectId) : "the project they asked about";
  const latest = lead.signals[0];

  if (p.broker) {
    return {
      doThis: "Pass to the broker desk. Don't count this lead toward campaign results.",
      angles: [],
      watchOut: "Their number is on the broker register, so this is an agent, not a buyer.",
    };
  }

  const doThis =
    lead.temperature === "hot" && lead.status === null
      ? `Call today. ${latest ? `${latest.label} ${formatAgo(latest.occurredAt)}` : "They're active again"}, and nobody has spoken to them yet.`
      : lead.temperature === "hot"
        ? `Call today about ${project}. They came back after being marked ${lead.status}.`
        : lead.temperature === "warm"
          ? `Send the ${project} payment plan on WhatsApp, then call in two days if they open it.`
          : "Leave this one to the re-engagement audience. Nothing in the last 30 days suggests they're looking now.";

  const angles: string[] = [];
  if (p.owns)
    angles.push(
      `Already owns at ${p.owns}: open with the owners' priority allocation, not the brochure.`,
    );
  if (p.job && p.jobConfidence !== "low" && /Head|Founder|Senior|Doctor|Consultant/.test(p.job[0]))
    angles.push(
      `${p.job[0]} at ${p.job[1]}: rental yield and handover date will land better than lifestyle.`,
    );
  if (lead.phoneCountry !== "+971")
    angles.push(
      `Phone code ${lead.phoneCountry}: offer a video viewing; they likely can't visit the sales centre.`,
    );
  const floorPlan = lead.signals.find((s) => /floor plan/i.test(s.evidence));
  if (floorPlan)
    angles.push(`Looked at ${floorPlan.evidence.toLowerCase()}: start from that unit type.`);
  if (!p.whatsapp) angles.push("Not on WhatsApp: call or email instead of messaging.");

  const watchOut = p.earlier
    ? `Also enquired about ${p.earlier.project} via ${p.earlier.via}. ${p.earlier.owner} owns that lead${p.earlier.owner === lead.owner ? "" : `, so agree with ${p.earlier.owner} who calls`}.`
    : p.job && p.jobConfidence === "low"
      ? "The LinkedIn match is a guess from the name alone. Don't bring up their job."
      : null;

  return { doThis, angles: angles.slice(0, 3), watchOut };
}

function formatAgo(iso: string) {
  const h = Math.round((Date.now() - Date.parse(iso)) / 3_600_000);
  return h < 24 ? `${h}h ago` : `${Math.round(h / 24)}d ago`;
}

// Re-enrich requests made in this session, so the lead page shows them queued.
const requested = new Map<number, string>();

export function enrichment(leadId: number): LeadEnrichment | null {
  const lead = LEADS.find((l) => l.id === leadId);
  if (!lead) return null;
  const p = profile(lead);
  const at = requested.get(leadId);
  const state = at ? "queued" : stateFor(lead);
  const hasFacts = state === "done" || (at !== undefined && stateFor(lead) === "done");
  const base = hasFacts ? facts(lead, p) : facts(lead, p).filter((f) => f.source === "GoHighLevel");
  return {
    leadId,
    state,
    enrichedAt: state === "done" ? hoursAgo(6 + (leadId % 70)) : null,
    failure: state === "failed" ? "No LinkedIn profile matched the name and phone code." : null,
    facts: base,
    brief: brief(lead, hasFacts ? p : { ...p, job: null, broker: false }),
  };
}

export function requestEnrichment(leadId: number): LeadEnrichment | null {
  requested.set(leadId, new Date().toISOString());
  return enrichment(leadId);
}

// ——— Queue ———

export function candidates(f: CandidateFilter): Lead[] {
  const cutoff = Date.now() - f.olderThanDays * 86_400_000;
  return LEADS.filter(
    (l) =>
      (f.temperature === "any" || l.temperature === f.temperature) &&
      (!f.noStatus || l.status === null) &&
      (f.projectId === "all" || l.projectId === f.projectId) &&
      Date.parse(l.arrivedAt) <= cutoff &&
      (!f.skipEnriched || stateFor(l) !== "done"),
  );
}

// The fixtures only hold 160 leads, so batch sizes are scaled to look like
// Reportage's real volume (about 3,400 leads a month).
const SCALE = 21;

const queue: EnrichmentQueue = {
  paused: false,
  monthUsed: 1_840,
  monthLimit: 5_000,
  candidates: { cold: 0, noStatus: 0, neverEnriched: 0 },
  jobs: [
    {
      id: "j4",
      label: "New leads, as they arrive",
      requestedBy: "Adx",
      createdAt: hoursAgo(1),
      total: 38,
      done: 31,
      failed: 1,
      state: "running",
    },
    {
      id: "j3",
      label: "Cold Wadi leads with no status, older than 30 days",
      requestedBy: "Sara",
      createdAt: hoursAgo(5),
      total: 420,
      done: 0,
      failed: 0,
      state: "queued",
    },
    {
      id: "j2",
      label: "Cold leads, older than 90 days",
      requestedBy: "Head of Marketing & AI",
      createdAt: daysAgo(3),
      total: 860,
      done: 812,
      failed: 48,
      state: "done",
    },
    {
      id: "j1",
      label: "Verdana leads with no status",
      requestedBy: "Omar",
      createdAt: daysAgo(9),
      total: 310,
      done: 297,
      failed: 13,
      state: "done",
    },
  ],
};

export function enrichmentQueue(): EnrichmentQueue {
  return {
    ...queue,
    candidates: {
      cold: LEADS.filter((l) => l.temperature === "cold").length * SCALE,
      noStatus: LEADS.filter((l) => l.status === null).length * SCALE,
      neverEnriched: LEADS.filter((l) => stateFor(l) === "never").length * SCALE,
    },
  };
}

export function countCandidates(f: CandidateFilter): number {
  return candidates(f).length * SCALE;
}

export function startJob(f: CandidateFilter, limit: number, label: string): EnrichmentJob {
  const available = countCandidates(f);
  if (available === 0) throw new Error("No leads match those filters");
  const left = queue.monthLimit - queue.monthUsed;
  const total = Math.min(limit, available);
  if (total > left) throw new Error(`Only ${left.toLocaleString("en")} lookups left this month`);
  const job: EnrichmentJob = {
    id: `j${queue.jobs.length + 1}`,
    label,
    requestedBy: "Hisham Ahamad",
    createdAt: new Date().toISOString(),
    total,
    done: 0,
    failed: 0,
    state: queue.paused ? "paused" : "queued",
  };
  queue.jobs.unshift(job);
  queue.monthUsed += total;
  return job;
}

export function setPaused(paused: boolean): EnrichmentQueue {
  queue.paused = paused;
  for (const j of queue.jobs) {
    if (paused && (j.state === "running" || j.state === "queued")) j.state = "paused";
    if (!paused && j.state === "paused") j.state = j.done > 0 ? "running" : "queued";
  }
  return enrichmentQueue();
}
