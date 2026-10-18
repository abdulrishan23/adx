// Pretend server for Create's phase 2 and 3 systems. Brands match the sample
// projects in data.ts. State lives in module variables, so decisions made on
// one screen show on the next until the page reloads, like a server would.

import type {
  Assumption,
  BrandKnowledge,
  BrandSetup,
  BuiltPage,
  Fact,
  Launch,
  LoopState,
  Plan,
  Research,
  SetupInput,
  SetupStep,
} from "../create";
import { daysAgo, hoursAgo, PROJECTS } from "./data";

const name = (id: number) => PROJECTS.find((p) => p.id === id)?.name ?? "Unknown";

// ---------------------------------------------------------------- brand knowledge

const brands = new Map<number, BrandKnowledge>();

function fact(
  id: string,
  topic: string,
  text: string,
  kind: Fact["source"]["kind"],
  label: string,
  days: number,
  status: Fact["status"] = "approved",
  conflict?: string,
): Fact {
  return { id, topic, text, source: { kind, label, seenAt: daysAgo(days) }, status, conflict };
}

brands.set(1, {
  projectId: 1,
  name: "The Wadi",
  capturedFrom: [
    { kind: "website", label: "reportagegroup.com/projects/the-wadi", items: 14 },
    { kind: "brochure", label: "The Wadi brochure, October 2026", items: 9 },
    { kind: "forms", label: "340 enquiry forms", items: 3 },
    { kind: "sales", label: "Sales notes, 38 calls", items: 4 },
  ],
  updatedAt: hoursAgo(5),
  facts: [
    fact(
      "w2",
      "Prices",
      "Townhouses from AED 1.35M",
      "brochure",
      "October brochure, p.4",
      11,
      "conflict",
      "price",
    ),
    fact(
      "w3",
      "Prices",
      "Townhouses from AED 1.2M",
      "website",
      "Project page",
      2,
      "conflict",
      "price",
    ),
    fact(
      "w4",
      "Payment plan",
      "10% to book, 50% during construction, 40% on handover",
      "brochure",
      "October brochure, p.12",
      11,
    ),
    fact(
      "w5",
      "Payment plan",
      "Buyers read the 60/40 split as two payments; sales explains it on most calls",
      "sales",
      "14 of 38 calls",
      1,
      "proposed",
    ),
    fact("w6", "Handover", "Q4 2027", "website", "Project page", 2),
    fact(
      "w7",
      "Location",
      "Al Shamkha, Abu Dhabi, 20 minutes from the airport",
      "website",
      "Project page",
      2,
    ),
    fact(
      "w8",
      "Units",
      "2, 3 and 4 bedroom townhouses, 1,800 to 2,900 sq ft",
      "brochure",
      "October brochure, p.6",
      11,
    ),
    fact("w9", "USPs", "Private garden with every unit", "brochure", "October brochure, p.2", 11),
    fact("w10", "USPs", "Community pool and gym", "website", "Project page", 2),
    fact(
      "w11",
      "USPs",
      "Cash buyers get 15% off, condition: full payment within 30 days",
      "sales",
      "Sales manager, 9 Oct",
      8,
    ),
    fact(
      "w12",
      "Audience",
      "Most enquiries come from families already renting in Abu Dhabi",
      "forms",
      "340 forms, 'current situation'",
      1,
    ),
    fact(
      "w13",
      "Audience",
      "Arabic speakers are 38% of enquiries",
      "forms",
      "340 forms, language",
      1,
    ),
    fact(
      "w14",
      "Offer",
      "Launch offer: AED 10K booking fee waived until 31 Oct",
      "brochure",
      "September brochure",
      40,
      "outdated",
    ),
  ],
});
brands.set(4, {
  projectId: 4,
  name: "Sila",
  capturedFrom: [
    { kind: "website", label: "reportagegroup.com/projects/sila", items: 11 },
    { kind: "brochure", label: "Sila launch brochure", items: 7 },
  ],
  updatedAt: daysAgo(3),
  facts: [
    fact(
      "s1",
      "Prices",
      "Apartments from AED 1.2M, launch prices for the first 50 units",
      "brochure",
      "Launch brochure, p.3",
      3,
    ),
    fact(
      "s2",
      "Payment plan",
      "20% to book, 80% on handover",
      "brochure",
      "Launch brochure, p.9",
      3,
    ),
    fact("s3", "Handover", "Q2 2028", "website", "Project page", 3),
    fact("s4", "Location", "Al Reem Island, waterfront", "website", "Project page", 3),
    fact("s5", "Units", "Studios to 3 bedrooms", "website", "Project page", 3),
    fact(
      "s6",
      "USPs",
      "Marina view from every tower apartment",
      "brochure",
      "Launch brochure, p.2",
      3,
      "proposed",
    ),
  ],
});
brands.set(3, {
  projectId: 3,
  name: "Oceana",
  capturedFrom: [{ kind: "website", label: "reportagegroup.com/projects/oceana", items: 8 }],
  updatedAt: daysAgo(6),
  facts: [
    fact(
      "o1",
      "Prices",
      "Apartments from AED 640K",
      "website",
      "Project page",
      6,
      "conflict",
      "price",
    ),
    fact(
      "o2",
      "Prices",
      "Apartments from AED 690K",
      "crm",
      "GoHighLevel pipeline notes",
      4,
      "conflict",
      "price",
    ),
    fact("o3", "Location", "Fujairah, beachfront", "website", "Project page", 6),
    fact("o4", "Handover", "Q3 2027", "website", "Project page", 6),
  ],
});

export function brand(projectId: number): BrandKnowledge | null {
  return brands.get(projectId) ?? null;
}

export function approveFact(projectId: number, factId: string): BrandKnowledge | null {
  const b = brands.get(projectId);
  if (!b) return null;
  const chosen = b.facts.find((f) => f.id === factId);
  if (!chosen) return b;
  for (const f of b.facts) {
    if (chosen.conflict && f.conflict === chosen.conflict && f.id !== factId) {
      f.status = "outdated";
      f.conflict = undefined;
    }
  }
  chosen.status = "approved";
  chosen.conflict = undefined;
  b.updatedAt = new Date().toISOString();
  return b;
}

// ---------------------------------------------------------------- research

const researches = new Map<number, Research>();

researches.set(1, {
  projectId: 1,
  refreshedAt: daysAgo(9),
  nextRefreshAt: daysAgo(-21),
  competitors: [
    {
      name: "Aldar · Yas Park Gate",
      positioning: "Townhouses on Yas, from AED 1.5M",
      priceFrom: "AED 1.5M",
      adsSeen: 23,
      channels: ["Meta", "Google", "Snapchat"],
      note: "Leads with the Yas lifestyle, not price. 12 of 23 ads are video.",
    },
    {
      name: "Bloom Living",
      positioning: "Family community in Zayed City",
      priceFrom: "AED 1.1M",
      adsSeen: 15,
      channels: ["Meta", "Google"],
      note: "Started bidding on 'wadi abu dhabi' brand terms on 11 Oct.",
    },
    {
      name: "Reportage · Verdana",
      positioning: "Our own, Dubai Investments Park",
      priceFrom: "AED 480K",
      adsSeen: 31,
      channels: ["Meta", "Google", "Portals"],
      note: "Shares the GCC audience; keep exclusions in place.",
    },
  ],
  keywords: [
    { term: "townhouse abu dhabi", volume: 4400, cpcMinor: 620, intent: "buy", included: true },
    { term: "the wadi abu dhabi", volume: 1900, cpcMinor: 410, intent: "buy", included: true },
    {
      term: "townhouse for sale al shamkha",
      volume: 720,
      cpcMinor: 580,
      intent: "buy",
      included: true,
    },
    {
      term: "payment plan townhouse abu dhabi",
      volume: 590,
      cpcMinor: 450,
      intent: "buy",
      included: true,
    },
    { term: "wadi dubai rent", volume: 2900, cpcMinor: 180, intent: "rent", included: false },
    {
      term: "abu dhabi property prices 2027",
      volume: 3200,
      cpcMinor: 210,
      intent: "explore",
      included: false,
    },
    { term: "reportage properties", volume: 2400, cpcMinor: 320, intent: "buy", included: true },
  ],
  audiences: [
    {
      name: "Families renting in Abu Dhabi",
      size: "180K to 210K",
      why: "Most enquiries come from renters; the payment plan is the lever.",
      platforms: ["Meta", "Snapchat"],
    },
    {
      name: "Arabic speakers, GCC, 30 to 50",
      size: "1.1M to 1.3M",
      why: "38% of enquiries; separate ads, not translated ones.",
      platforms: ["Meta", "Google"],
    },
    {
      name: "Investors, India and Pakistan",
      size: "420K to 500K",
      why: "Cash offer converts well here; 21% of Done-Deals.",
      platforms: ["Meta", "Google"],
    },
  ],
  gaps: [
    "Competitor spend is estimated from ad libraries, not reported.",
    "Keyword volumes are Google's rounded ranges; CPCs are last 30 days.",
    "No SEO baseline yet: Search Console isn't connected (phase 6).",
  ],
});
researches.set(4, {
  projectId: 4,
  refreshedAt: daysAgo(2),
  nextRefreshAt: daysAgo(-28),
  competitors: [
    {
      name: "Aldar · Reem Hills",
      positioning: "Villas and apartments on Reem",
      priceFrom: "AED 1.4M",
      adsSeen: 19,
      channels: ["Meta", "Google"],
      note: "Heavy on view shots; same angle as ours.",
    },
    {
      name: "Imkan · Pixel",
      positioning: "Reem Island apartments, ready",
      priceFrom: "AED 900K",
      adsSeen: 8,
      channels: ["Meta"],
      note: "Ready units, competes on move-in date.",
    },
  ],
  keywords: [
    {
      term: "apartment al reem island",
      volume: 3600,
      cpcMinor: 700,
      intent: "buy",
      included: true,
    },
    { term: "sila abu dhabi", volume: 480, cpcMinor: 300, intent: "buy", included: true },
    {
      term: "waterfront apartment abu dhabi",
      volume: 1300,
      cpcMinor: 650,
      intent: "buy",
      included: true,
    },
    { term: "reem island rent", volume: 5400, cpcMinor: 160, intent: "rent", included: false },
  ],
  audiences: [
    {
      name: "Professionals on Reem and Maryah",
      size: "90K to 110K",
      why: "Waterfront, close to work.",
      platforms: ["Meta", "Google"],
    },
    {
      name: "Investors, GCC",
      size: "800K to 950K",
      why: "Launch pricing on 50 units is the hook.",
      platforms: ["Meta", "Snapchat"],
    },
  ],
  gaps: ["Launch-week research only; refresh after 4 weeks of delivery."],
});

export function research(projectId: number): Research | null {
  return researches.get(projectId) ?? null;
}

export function toggleKeyword(projectId: number, term: string): Research | null {
  const r = researches.get(projectId);
  if (!r) return null;
  const k = r.keywords.find((x) => x.term === term);
  if (k) k.included = !k.included;
  return r;
}

export function refreshResearch(projectId: number): Research | null {
  const r = researches.get(projectId);
  if (!r) return null;
  r.refreshedAt = new Date().toISOString();
  r.nextRefreshAt = daysAgo(-30);
  for (const c of r.competitors) c.adsSeen += 1;
  return r;
}

// ---------------------------------------------------------------- media plans

const plansById = new Map<string, Plan>();

function assumption(id: string, text: string, source: string, confirmed = false): Assumption {
  return { id, text, source, confirmed };
}

const sila3: Plan = {
  id: "plan-sila-3",
  projectId: 4,
  projectName: "Sila",
  version: 3,
  status: "waiting",
  budgetMinor: 450_000_00,
  from: "2026-11-01",
  to: "2026-12-12",
  channels: [
    {
      platform: "meta",
      share: 55,
      budgetMinor: 247_500_00,
      expectedQualified: 660,
      expectedCpqMinor: 375_00,
    },
    {
      platform: "google",
      share: 35,
      budgetMinor: 157_500_00,
      expectedQualified: 340,
      expectedCpqMinor: 463_00,
    },
    {
      platform: "snapchat",
      share: 10,
      budgetMinor: 45_000_00,
      expectedQualified: 100,
      expectedCpqMinor: 450_00,
    },
  ],
  weekly: [
    { week: "1 Nov", budgetMinor: 95_000_00, note: "Launch week, 40% above run rate" },
    { week: "8 Nov", budgetMinor: 80_000_00 },
    { week: "15 Nov", budgetMinor: 70_000_00 },
    { week: "22 Nov", budgetMinor: 70_000_00, note: "First optimisation" },
    { week: "29 Nov", budgetMinor: 70_000_00 },
    { week: "6 Dec", budgetMinor: 65_000_00 },
  ],
  assumptions: [
    assumption(
      "a1",
      "Generic search CPC stays under AED 7 through launch week",
      "Research, 15 Oct: AED 6.50 to 7.00",
    ),
    assumption(
      "a2",
      "Arabic speakers are 35% or more of enquiries, so Arabic ads get 35% of Meta",
      "The Wadi forms, 38%",
    ),
    assumption(
      "a3",
      "50 launch-priced units stay available through November",
      "Sales, 14 Oct",
      true,
    ),
  ],
  kpis: [
    { label: "Qualified leads", target: "1,100 by 12 Dec" },
    { label: "Cost per qualified", target: "Under AED 410" },
    { label: "Meetings (SQL)", target: "300" },
    { label: "Time to first call", target: "Under 1 hour, 90% of hot leads" },
  ],
  expectedQualified: 1100,
  expectedCpqMinor: 409_00,
  changes:
    "v3 moves 10% from Google to Meta after The Wadi's cash offer outperformed on Meta, and adds Snapchat for the GCC investor audience.",
  author: "Adx",
  createdAt: daysAgo(4),
};
const sila2: Plan = {
  ...sila3,
  id: "plan-sila-2",
  version: 2,
  status: "superseded",
  channels: [
    {
      platform: "meta",
      share: 45,
      budgetMinor: 202_500_00,
      expectedQualified: 540,
      expectedCpqMinor: 375_00,
    },
    {
      platform: "google",
      share: 45,
      budgetMinor: 202_500_00,
      expectedQualified: 437,
      expectedCpqMinor: 463_00,
    },
    {
      platform: "snapchat",
      share: 10,
      budgetMinor: 45_000_00,
      expectedQualified: 100,
      expectedCpqMinor: 450_00,
    },
  ],
  expectedQualified: 1077,
  expectedCpqMinor: 418_00,
  changes: "v2 added the Snapchat line after research found the GCC investor audience there.",
  createdAt: daysAgo(7),
  assumptions: sila3.assumptions.map((a) => ({ ...a })),
};
const wadi1: Plan = {
  id: "plan-wadi-1",
  projectId: 1,
  projectName: "The Wadi",
  version: 1,
  status: "approved",
  budgetMinor: 380_000_00,
  from: "2026-10-01",
  to: "2026-10-31",
  channels: [
    {
      platform: "meta",
      share: 60,
      budgetMinor: 228_000_00,
      expectedQualified: 600,
      expectedCpqMinor: 380_00,
    },
    {
      platform: "google",
      share: 40,
      budgetMinor: 152_000_00,
      expectedQualified: 320,
      expectedCpqMinor: 475_00,
    },
  ],
  weekly: [
    { week: "1 Oct", budgetMinor: 95_000_00 },
    { week: "8 Oct", budgetMinor: 95_000_00 },
    { week: "15 Oct", budgetMinor: 95_000_00 },
    { week: "22 Oct", budgetMinor: 95_000_00 },
  ],
  assumptions: [
    assumption("b1", "Brand CPC stays near AED 4", "Account history, September", true),
    assumption("b2", "Cash offer runs all month", "Sales, 28 Sept", true),
  ],
  kpis: [
    { label: "Qualified leads", target: "920 in October" },
    { label: "Cost per qualified", target: "Under AED 420" },
  ],
  expectedQualified: 920,
  expectedCpqMinor: 413_00,
  changes: "First plan for October, built from the September audit.",
  author: "Adx",
  createdAt: daysAgo(20),
  decidedAt: daysAgo(19),
  decidedBy: "CMO",
};
const verdana2: Plan = {
  id: "plan-verdana-2",
  projectId: 2,
  projectName: "Verdana",
  version: 2,
  status: "draft",
  budgetMinor: 200_000_00,
  from: "2026-11-01",
  to: "2026-11-30",
  channels: [
    {
      platform: "meta",
      share: 70,
      budgetMinor: 140_000_00,
      expectedQualified: 350,
      expectedCpqMinor: 400_00,
    },
    {
      platform: "google",
      share: 30,
      budgetMinor: 60_000_00,
      expectedQualified: 120,
      expectedCpqMinor: 500_00,
    },
  ],
  weekly: [
    { week: "1 Nov", budgetMinor: 50_000_00 },
    { week: "8 Nov", budgetMinor: 50_000_00 },
    { week: "15 Nov", budgetMinor: 50_000_00 },
    { week: "22 Nov", budgetMinor: 50_000_00 },
  ],
  assumptions: [assumption("c1", "PMax 'Generic' stays paused", "Alert, 9 Oct")],
  kpis: [{ label: "Cost per qualified", target: "Under AED 450, from AED 512" }],
  expectedQualified: 470,
  expectedCpqMinor: 426_00,
  changes: "Drafting. Waits on the research refresh due 20 Oct.",
  author: "Adx",
  createdAt: hoursAgo(6),
};
for (const p of [sila3, sila2, wadi1, verdana2]) plansById.set(p.id, p);

export function plans(): Plan[] {
  return [...plansById.values()].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}

export function plan(id: string): Plan | null {
  return plansById.get(id) ?? null;
}

export function confirmAssumption(
  planId: string,
  assumptionId: string,
  confirmed: boolean,
): Plan | null {
  const p = plansById.get(planId);
  if (!p) return null;
  const a = p.assumptions.find((x) => x.id === assumptionId);
  if (a) a.confirmed = confirmed;
  return p;
}

export function decidePlan(
  planId: string,
  decision: "approve" | "changes",
  note?: string,
): Plan | null {
  const p = plansById.get(planId);
  if (!p) return null;
  if (decision === "approve") {
    p.status = "approved";
    p.decidedAt = new Date().toISOString();
    p.decidedBy = "You";
    for (const other of plansById.values()) {
      if (other.projectId === p.projectId && other.id !== p.id && other.status === "approved") {
        other.status = "superseded";
      }
    }
  } else {
    p.status = "draft";
    p.changes = note ? `Changes asked for: ${note}` : "Changes asked for.";
  }
  return p;
}

// ---------------------------------------------------------------- pages

const builtPages: BuiltPage[] = [
  {
    id: "pg1",
    projectId: 1,
    projectName: "The Wadi",
    title: "The Wadi · Townhouses from AED 1.2M",
    url: "/lp/the-wadi",
    status: "live",
    factsUsed: 9,
    form: "Wadi Launch EN",
    language: "en",
    updatedAt: daysAgo(3),
  },
  {
    id: "pg2",
    projectId: 1,
    projectName: "The Wadi",
    title: "الوادي · تاون هاوس من 1.2 مليون درهم",
    url: "/lp/the-wadi-ar",
    status: "live",
    factsUsed: 9,
    form: "Wadi Launch AR",
    language: "ar",
    updatedAt: daysAgo(3),
  },
  {
    id: "pg3",
    projectId: 1,
    projectName: "The Wadi",
    title: "The Wadi · Cash offer",
    url: "/lp/the-wadi-cash",
    status: "live",
    factsUsed: 6,
    form: "Wadi Cash",
    language: "en",
    updatedAt: daysAgo(7),
  },
  {
    id: "pg4",
    projectId: 4,
    projectName: "Sila",
    title: "Sila · Launch, from AED 1.2M",
    url: "/lp/sila",
    status: "draft",
    factsUsed: 5,
    form: "Sila Launch EN",
    language: "en",
    updatedAt: hoursAgo(20),
  },
  {
    id: "pg5",
    projectId: 2,
    projectName: "Verdana",
    title: "Verdana · Offer",
    url: "/offer/verdana-offer/",
    status: "broken",
    factsUsed: 4,
    form: "Verdana Offer",
    language: "en",
    updatedAt: daysAgo(12),
  },
];

export function pages(): BuiltPage[] {
  return builtPages;
}

// ---------------------------------------------------------------- launch

const launchesById = new Map<number, Launch>();
launchesById.set(1, {
  projectId: 1,
  projectName: "The Wadi",
  planId: "plan-wadi-1",
  readiness: [
    { label: "Plan approved", ok: true, note: "v1, by the CMO" },
    {
      label: "Creatives approved per channel",
      ok: true,
      note: "C1 Cash, 2 of 8 approved; the rest waiting",
    },
    { label: "Landing pages live with forms", ok: true, note: "3 pages, forms reach GoHighLevel" },
    {
      label: "Tracking verified",
      ok: true,
      note: "Meta pixel and Google tag fire on the thank-you page",
    },
    {
      label: "Special ad category set",
      ok: false,
      note: "Housing category missing on 'Wadi | Cash | UAE'",
    },
  ],
  steps: [
    {
      label: "Campaigns generated from the plan",
      status: "ready",
      at: hoursAgo(10),
      note: "2 Meta, 2 Google",
    },
    { label: "Validated against guardrails", status: "ready", at: hoursAgo(10) },
    { label: "Submitted", status: "ready", at: hoursAgo(10) },
    { label: "Meta review", status: "error", at: hoursAgo(9), note: "2 ads rejected" },
    { label: "Google review", status: "pending", note: "In review, nothing needed" },
    { label: "Delivering", status: "pending", note: "Not yet" },
  ],
  blockers: [
    {
      id: "bl1",
      platform: "meta",
      target: "Wadi | Cash | UAE",
      reason: "Housing special ad category missing",
      fix: "Set the special ad category and resubmit",
      fixed: false,
    },
    {
      id: "bl2",
      platform: "meta",
      target: "Wadi | Cash | UAE · THEWADI_C1_CASH_AR_9x16",
      reason: "Text covers more than 20% of the image",
      fix: "Use the 1:1 layout's text block on 9:16",
      fixed: false,
    },
  ],
});
launchesById.set(4, {
  projectId: 4,
  projectName: "Sila",
  planId: "plan-sila-3",
  readiness: [
    { label: "Plan approved", ok: false, note: "v3 waiting since Tuesday" },
    { label: "Creatives approved per channel", ok: false, note: "6 waiting" },
    { label: "Landing pages live with forms", ok: false, note: "1 draft" },
    { label: "Tracking verified", ok: false, note: "Not yet, page isn't live" },
    { label: "Special ad category set", ok: true, note: "Housing, on the template" },
  ],
  steps: [
    {
      label: "Campaigns generated from the plan",
      status: "pending",
      note: "Starts when the plan is approved",
    },
    { label: "Validated against guardrails", status: "pending" },
    { label: "Submitted", status: "pending" },
    { label: "Meta review", status: "pending" },
    { label: "Google review", status: "pending" },
    { label: "Delivering", status: "pending" },
  ],
  blockers: [],
});

export function launches(): Launch[] {
  return [...launchesById.values()];
}

export function fixBlocker(projectId: number, blockerId: string): Launch | null {
  const l = launchesById.get(projectId);
  if (!l) return null;
  const b = l.blockers.find((x) => x.id === blockerId);
  if (b) b.fixed = true;
  if (l.blockers.every((x) => x.fixed)) {
    const review = l.steps.find((s) => s.label === "Meta review");
    if (review) {
      review.status = "pending";
      review.note = "Resubmitted, in review";
      review.at = new Date().toISOString();
    }
    const cat = l.readiness.find((r) => r.label === "Special ad category set");
    if (cat) {
      cat.ok = true;
      cat.note = "Set on every ad set";
    }
  }
  return l;
}

// ---------------------------------------------------------------- loop

export function loopStates(): LoopState[] {
  return PROJECTS.map((p) => {
    const b = brands.get(p.id);
    const r = researches.get(p.id);
    const ps = plans().filter((x) => x.projectId === p.id);
    const l = launchesById.get(p.id);
    const pg = builtPages.filter((x) => x.projectId === p.id);
    const brandState = !b
      ? "pending"
      : b.facts.some((f) => f.status === "conflict")
        ? "caution"
        : "ready";
    const planState = ps.some((x) => x.status === "approved")
      ? "ready"
      : ps.some((x) => x.status === "waiting")
        ? "caution"
        : "pending";
    const pageState = pg.some((x) => x.status === "broken")
      ? "error"
      : pg.some((x) => x.status === "live")
        ? "ready"
        : pg.length
          ? "caution"
          : "pending";
    const launchState = !l
      ? "pending"
      : l.blockers.some((x) => !x.fixed)
        ? "error"
        : l.steps.at(-1)?.status === "ready"
          ? "ready"
          : "pending";
    const note = !b
      ? "Brand not captured"
      : brandState === "caution"
        ? "A fact to decide"
        : planState === "caution"
          ? "Plan waiting for approval"
          : launchState === "error"
            ? "Launch blocked"
            : planState === "pending"
              ? "No plan yet"
              : "Delivering";
    return {
      projectId: p.id,
      steps: {
        brand: brandState,
        research: !r ? "pending" : Date.parse(r.nextRefreshAt) < Date.now() ? "caution" : "ready",
        plan: planState,
        creative: "pending",
        page: pageState,
        launch: launchState,
      },
      note,
    };
  });
}

export { name as projectName };

// ---------------------------------------------------------------- setup

const setups = new Map<number, { input: SetupInput; startedAt: number }>();

// Each step takes a few seconds here so the page can be watched moving. The
// real run takes as long as the research does.
const STEP_MS = 6000;

function setupSteps(input: SetupInput): Omit<SetupStep, "state">[] {
  return [
    {
      id: "analysis",
      label: "Brand analysis",
      detail: "What it sells, to whom, its look and voice, USPs, and what it isn't doing well",
    },
    input.hasHistory
      ? {
          id: "audit",
          label: "Account audit",
          detail: "Past campaigns, spend, leads and what worked, from the connected accounts",
        }
      : {
          id: "audit",
          label: "Outside audit",
          detail: "No history to audit, so Adx works from public data and comparable brands",
        },
    { id: "market", label: "Market research", detail: "Demand, pricing and audiences in the area" },
    {
      id: "competitors",
      label: "Competitor research",
      detail: "Their sites, socials and ads from the Meta, Google and TikTok ad libraries",
    },
    { id: "seo", label: "SEO and keywords", detail: "Search terms, volumes and costs by intent" },
    {
      id: "plan",
      label: "First media plan",
      detail: input.templateFile
        ? `In your template, ${input.templateFile}`
        : "In Adx's own layout until the media-plan template is added",
    },
  ];
}

export function startSetup(input: SetupInput): BrandSetup {
  setups.set(input.projectId, { input, startedAt: Date.now() });
  return brandSetup(input.projectId) as BrandSetup;
}

export function brandSetup(projectId: number): BrandSetup | null {
  const s = setups.get(projectId);
  if (!s) {
    return null;
  }
  const elapsed = Date.now() - s.startedAt;
  const current = Math.floor(elapsed / STEP_MS);
  const steps = setupSteps(s.input).map((step, i) => ({
    ...step,
    state: (i < current ? "done" : i === current ? "working" : "queued") as SetupStep["state"],
  }));
  return {
    ...s.input,
    startedAt: new Date(s.startedAt).toISOString(),
    steps,
    done: steps.every((x) => x.state === "done"),
  };
}
