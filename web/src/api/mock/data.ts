// Mock data for the design branch. Seeded, so every reload shows the same
// numbers, and shaped after Reportage's September 2026: about 3,400 leads a
// month, about 60 Done-Deals, Meta bringing most leads and Google few deals.
// Names of people and leads are invented.

import type {
  ActivityEvent,
  Alert,
  Campaign,
  Connection,
  DailyPoint,
  Lead,
  MatchLevel,
  Platform,
  Project,
  Proposal,
  Signal,
  Temperature,
} from "../models";

export function rng(seed: number) {
  let a = seed;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const rand = rng(43);
const pick = <T>(items: readonly T[]): T => items[Math.floor(rand() * items.length)] as T;
const between = (min: number, max: number) => min + rand() * (max - min);

export const DAY = 86_400_000;
export const now = new Date();

export function daysAgo(n: number, hour = 10): string {
  const d = new Date(now.getTime() - n * DAY);
  d.setUTCHours(hour, Math.floor(rand() * 60), 0, 0);
  // Today's fixed hour can still be ahead of now; keep fixtures in the past.
  if (d.getTime() > now.getTime()) d.setTime(now.getTime() - 45 * 60_000);
  return d.toISOString();
}

export function hoursAgo(n: number): string {
  return new Date(now.getTime() - n * 3_600_000).toISOString();
}

export const PROJECTS: Project[] = [
  { id: 1, name: "The Wadi", city: "Abu Dhabi" },
  { id: 2, name: "Verdana", city: "Dubai" },
  { id: 3, name: "Oceana", city: "Fujairah" },
  { id: 4, name: "Sila", city: "Abu Dhabi" },
  { id: 5, name: "Sensi", city: "Abu Dhabi" },
  { id: 6, name: "Selina Bay", city: "Abu Dhabi" },
];

export const OWNERS = ["Sara", "Omar", "Priya", "Khalid"] as const;

const projectName = (id: number) => PROJECTS.find((p) => p.id === id)?.name ?? "Unknown";

// 90 days of daily figures for one campaign, with weekends a little lower.
function series(dailySpend: number, cpl: number, qualRate: number): DailyPoint[] {
  const out: DailyPoint[] = [];
  for (let i = 89; i >= 0; i--) {
    const date = new Date(now.getTime() - i * DAY);
    const weekend = date.getUTCDay() === 5 || date.getUTCDay() === 6;
    const spend = dailySpend * between(0.8, 1.2) * (weekend ? 0.85 : 1);
    const leads = Math.max(0, Math.round((spend / cpl) * between(0.7, 1.3)));
    const qualified = Math.round(leads * qualRate * between(0.6, 1.4));
    out.push({
      date: date.toISOString().slice(0, 10),
      spendMinor: Math.round(spend * 100),
      leads,
      qualified,
    });
  }
  return out;
}

type CampaignSeed = {
  name: string;
  platform: Platform;
  type: string;
  projectId: number;
  dailySpend: number;
  cpl: number;
  qualRate: number;
  deals: number;
  status?: Campaign["status"];
};

const SEEDS: CampaignSeed[] = [
  {
    name: "Wadi | Leads | UAE AR",
    platform: "meta",
    type: "Leads, instant form",
    projectId: 1,
    dailySpend: 2600,
    cpl: 95,
    qualRate: 0.24,
    deals: 9,
  },
  {
    name: "Wadi | Leads | UAE EN",
    platform: "meta",
    type: "Leads, instant form",
    projectId: 1,
    dailySpend: 2100,
    cpl: 110,
    qualRate: 0.2,
    deals: 6,
  },
  {
    name: "Wadi | Leads | GCC",
    platform: "meta",
    type: "Leads, instant form",
    projectId: 1,
    dailySpend: 1200,
    cpl: 140,
    qualRate: 0.28,
    deals: 5,
  },
  {
    name: "Verdana | Leads | Offer 30%",
    platform: "meta",
    type: "Leads, instant form",
    projectId: 2,
    dailySpend: 1800,
    cpl: 85,
    qualRate: 0.14,
    deals: 3,
  },
  {
    name: "Oceana | Leads | GCC only",
    platform: "meta",
    type: "Leads, instant form",
    projectId: 3,
    dailySpend: 900,
    cpl: 160,
    qualRate: 0.18,
    deals: 2,
  },
  {
    name: "Sila | Leads | Villas",
    platform: "meta",
    type: "Leads, instant form",
    projectId: 4,
    dailySpend: 1100,
    cpl: 210,
    qualRate: 0.22,
    deals: 2,
    status: "limited",
  },
  {
    name: "Selina Bay | Retargeting",
    platform: "meta",
    type: "Leads, instant form",
    projectId: 6,
    dailySpend: 400,
    cpl: 75,
    qualRate: 0.3,
    deals: 1,
  },
  {
    name: "Search | Wadi | Brand",
    platform: "google",
    type: "Search",
    projectId: 1,
    dailySpend: 700,
    cpl: 180,
    qualRate: 0.2,
    deals: 1,
  },
  {
    name: "Search | Generic | Abu Dhabi off-plan",
    platform: "google",
    type: "Search",
    projectId: 1,
    dailySpend: 1500,
    cpl: 260,
    qualRate: 0.08,
    deals: 0,
  },
  {
    name: "PMax | Verdana",
    platform: "google",
    type: "Performance Max",
    projectId: 2,
    dailySpend: 1300,
    cpl: 150,
    qualRate: 0.07,
    deals: 1,
  },
  {
    name: "Demand Gen | Sensi",
    platform: "google",
    type: "Demand Gen",
    projectId: 5,
    dailySpend: 600,
    cpl: 220,
    qualRate: 0.05,
    deals: 0,
    status: "paused",
  },
];

const sum = (points: DailyPoint[], key: keyof Omit<DailyPoint, "date">) =>
  points.reduce((acc, p) => acc + p[key], 0);

export const CAMPAIGNS: Campaign[] = SEEDS.map((s, i) => {
  const daily = series(s.dailySpend, s.cpl, s.qualRate);
  const last30 = daily.slice(-30);
  const leads = sum(last30, "leads");
  const hot = Math.round(leads * s.qualRate * 0.5);
  const warm = Math.round(leads * 0.3);
  return {
    id: `c${i + 1}`,
    name: s.name,
    platform: s.platform,
    type: s.type,
    projectId: s.projectId,
    status: s.status ?? "active",
    spendMinor: sum(last30, "spendMinor"),
    leads,
    qualified: sum(last30, "qualified"),
    deals: s.deals,
    revenueMinor: s.deals > 0 ? Math.round(s.deals * between(1.4, 2.6) * 1_000_000 * 100) : 0,
    temperatureMix: { hot, warm, cold: Math.max(0, leads - hot - warm) },
    daily,
    adSets:
      s.platform === "meta"
        ? (
            [
              ["Broad 25-55", 0.45, 0.5, 0.35],
              ["Lookalike 1% deals", 0.35, 0.3, 0.45],
              ["Interest: real estate", 0.2, 0.2, 0.2],
            ] as const
          ).map(([name, spendShare, leadShare, qualShare]) => ({
            name,
            spendMinor: Math.round(sum(last30, "spendMinor") * spendShare),
            leads: Math.round(leads * leadShare),
            qualified: Math.round(sum(last30, "qualified") * qualShare),
          }))
        : [],
  };
});

export const projectOf = projectName;

// ——— Leads ———

// Names come from one background per lead, so first and last names belong
// together. The mix roughly follows who buys off-plan in the UAE.
type Background = { first: string[]; last: string[]; home: string };

const BACKGROUNDS: { weight: number; names: Background }[] = [
  {
    weight: 28,
    names: {
      first: [
        "Rahul",
        "Priyanka",
        "Vikram",
        "Ananya",
        "Arjun",
        "Ritu",
        "Sanjay",
        "Deepa",
        "Karthik",
        "Neha",
      ],
      last: [
        "Nair",
        "Patel",
        "Iyer",
        "Menon",
        "Sharma",
        "Reddy",
        "Kapoor",
        "Pillai",
        "Gupta",
        "Shah",
      ],
      home: "+91",
    },
  },
  {
    weight: 25,
    names: {
      first: [
        "Ahmed",
        "Fatima",
        "Yousef",
        "Mariam",
        "Khalid",
        "Noura",
        "Hassan",
        "Layla",
        "Faisal",
        "Reem",
      ],
      last: [
        "Al Mansouri",
        "Al Hashimi",
        "Haddad",
        "Al Suwaidi",
        "Saleh",
        "Al Ketbi",
        "Farouk",
        "Al Nuaimi",
        "Al Otaibi",
        "Khoury",
      ],
      home: "+966",
    },
  },
  {
    weight: 15,
    names: {
      first: [
        "Imran",
        "Ayesha",
        "Bilal",
        "Sana",
        "Usman",
        "Hira",
        "Kamran",
        "Zainab",
        "Hamza",
        "Mahnoor",
      ],
      last: [
        "Khan",
        "Qureshi",
        "Malik",
        "Chaudhry",
        "Butt",
        "Sheikh",
        "Mirza",
        "Siddiqui",
        "Abbasi",
        "Raza",
      ],
      home: "+92",
    },
  },
  {
    weight: 12,
    names: {
      first: [
        "James",
        "Emma",
        "Oliver",
        "Charlotte",
        "Harry",
        "Sophie",
        "Thomas",
        "Lucy",
        "William",
        "Grace",
      ],
      last: [
        "Smith",
        "Taylor",
        "Brown",
        "Wilson",
        "Evans",
        "Clarke",
        "Walker",
        "Wright",
        "Hughes",
        "Turner",
      ],
      home: "+44",
    },
  },
  {
    weight: 12,
    names: {
      first: [
        "Dmitry",
        "Anastasia",
        "Sergei",
        "Ekaterina",
        "Alexei",
        "Olga",
        "Ivan",
        "Natalia",
        "Mikhail",
        "Elena",
      ],
      last: [
        "Ivanov",
        "Petrova",
        "Smirnov",
        "Volkova",
        "Kuznetsov",
        "Sokolova",
        "Popov",
        "Morozova",
        "Lebedev",
        "Orlova",
      ],
      home: "+7",
    },
  },
  {
    weight: 8,
    names: {
      first: [
        "Javier",
        "Lucía",
        "Carlos",
        "María",
        "Alejandro",
        "Carmen",
        "Pablo",
        "Elena",
        "Diego",
        "Sofía",
      ],
      last: [
        "García",
        "Martínez",
        "López",
        "Sánchez",
        "Fernández",
        "Romero",
        "Navarro",
        "Torres",
        "Ruiz",
        "Moreno",
      ],
      home: "+34",
    },
  },
];

// Picks a background from the lead id rather than the random stream, so the
// rest of the fixtures stay as they were.
function backgroundFor(id: number): Background {
  const total = BACKGROUNDS.reduce((a, b) => a + b.weight, 0);
  let n = (id * 37 + 11) % total;
  for (const b of BACKGROUNDS) {
    if (n < b.weight) return b.names;
    n -= b.weight;
  }
  return (BACKGROUNDS[0] as (typeof BACKGROUNDS)[number]).names;
}

// Russian and Spanish surnames change with gender; keep them matched.
const FEMININE = new Set([
  "Priyanka",
  "Ananya",
  "Ritu",
  "Deepa",
  "Neha",
  "Fatima",
  "Mariam",
  "Noura",
  "Layla",
  "Reem",
  "Ayesha",
  "Sana",
  "Hira",
  "Zainab",
  "Mahnoor",
  "Emma",
  "Charlotte",
  "Sophie",
  "Lucy",
  "Grace",
  "Anastasia",
  "Ekaterina",
  "Olga",
  "Natalia",
  "Elena",
  "Lucía",
  "María",
  "Carmen",
  "Sofía",
]);

function russianSurname(last: string, female: boolean) {
  const base = last.replace(/a$/, "");
  return female ? `${base}a` : base;
}
const STATUSES = [
  null,
  null,
  null,
  "No Answer",
  "Follow Up",
  "Follow Up",
  "Potential",
  "Meeting",
  "Not Interested",
  "Sales offer",
] as const;
const SOURCES = [
  "Meta instant form",
  "Meta instant form",
  "Meta instant form",
  "Google search",
  "Website form",
  "WhatsApp",
  "Telesales",
  "Bayut",
] as const;

const SIGNAL_LIBRARY: Omit<Signal, "id" | "occurredAt" | "projectId">[] = [
  {
    code: "S03",
    label: "Tapped WhatsApp",
    role: "hot",
    evidence: "Header button on the project page",
  },
  { code: "S05", label: "Messaged us", role: "hot", evidence: "WhatsApp reply in GoHighLevel" },
  {
    code: "C09",
    label: "Ready to act",
    role: "hot",
    evidence: "Narrowed down, then tapped WhatsApp within 2 hours",
  },
  {
    code: "C02",
    label: "Narrowing down",
    role: "warm",
    evidence: "2BR floor plans and the unit-types PDF in one visit",
  },
  {
    code: "S07",
    label: "Took project material",
    role: "warm",
    evidence: "Opened the brochure PDF",
  },
  { code: "S09", label: "Viewed an offer", role: "warm", evidence: "Payment plan offer page" },
  { code: "C01", label: "Comparison shopping", role: "warm", evidence: "3 projects in one visit" },
  {
    code: "C03",
    label: "Coming back to one project",
    role: "warm",
    evidence: "Project page in 2 visits this week",
  },
  {
    code: "S16",
    label: "Site visit",
    role: "feature",
    evidence: "First project page in the visit",
  },
];

function temperatureFor(signals: Signal[], status: string | null): Temperature {
  if (status === "Not Interested") return "cold";
  const recent = (s: Signal, hours: number) =>
    now.getTime() - Date.parse(s.occurredAt) < hours * 3_600_000;
  if (signals.some((s) => s.role === "hot" && recent(s, 72))) return "hot";
  if (signals.some((s) => s.role !== "feature" && recent(s, 14 * 24))) return "warm";
  return "cold";
}

function reasonFor(signals: Signal[], temp: Temperature): string {
  const top = [...signals].sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt))[0];
  if (!top) return "Enquired, no activity since";
  const when = Math.round((now.getTime() - Date.parse(top.occurredAt)) / 3_600_000);
  const ago = when < 24 ? `${when}h ago` : `${Math.round(when / 24)}d ago`;
  return temp === "cold" ? `Last seen: ${top.label.toLowerCase()} ${ago}` : `${top.label} ${ago}`;
}

function nameFor(bg: Background) {
  const first = pick(bg.first);
  const last = pick(bg.last);
  return `${first} ${bg.home === "+7" ? russianSurname(last, FEMININE.has(first)) : last}`;
}

function makeLead(id: number): Lead {
  const bg = backgroundFor(id);
  const projectId = pick(PROJECTS).id;
  const source = pick(SOURCES);
  const relabelled = source === "Meta instant form" && rand() < 0.12;
  const status = pick(STATUSES);
  const arrivedDays = Math.floor(between(0, 40));
  const signalCount = Math.floor(between(0, 5));
  const signals: Signal[] = Array.from({ length: signalCount }, (_, k) => {
    const s = pick(SIGNAL_LIBRARY);
    return {
      ...s,
      id: `${id}-s${k}`,
      projectId,
      occurredAt: hoursAgo(between(1, Math.max(2, arrivedDays * 24))),
    };
  }).sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt));
  const temperature = temperatureFor(signals, status);
  const matchLevel: MatchLevel = source.startsWith("Meta")
    ? "lead_id"
    : source === "Google search"
      ? pick(["click_id", "utm", "none"] as const)
      : source === "Website form"
        ? pick(["utm", "none"] as const)
        : source === "WhatsApp"
          ? "contact"
          : "none";
  const arrivedAt = daysAgo(arrivedDays);
  const events: ActivityEvent[] = [
    {
      id: `${id}-a0`,
      channel: "form",
      label: "Enquired",
      detail: `${source} · ${projectName(projectId)}`,
      occurredAt: arrivedAt,
    },
    ...signals.map((s, k) => ({
      id: `${id}-a${k + 1}`,
      channel: s.code === "S05" ? ("whatsapp" as const) : ("web" as const),
      label: s.label,
      detail: s.evidence,
      occurredAt: s.occurredAt,
    })),
    ...(status
      ? [
          {
            id: `${id}-crm`,
            channel: "crm" as const,
            label: `Status set to ${status}`,
            // Sales sets the status after the lead arrives, never before.
            occurredAt: new Date((Date.parse(arrivedAt) + now.getTime()) / 2).toISOString(),
          },
        ]
      : []),
  ];
  const activity = events.sort((a, b) => Date.parse(b.occurredAt) - Date.parse(a.occurredAt));

  return {
    id,
    name: nameFor(bg),
    // Most buyers live in the UAE whatever their background (D-10).
    phoneCountry: rand() < 0.7 ? "+971" : bg.home,
    source: relabelled ? "WhatsApp Camp." : source,
    originalSource: source,
    matchLevel,
    campaignId:
      source.startsWith("Meta") || source === "Google search"
        ? (CAMPAIGNS.find(
            (c) =>
              c.projectId === projectId &&
              c.platform === (source.startsWith("Meta") ? "meta" : "google"),
          )?.id ?? null)
        : null,
    projectId,
    status,
    owner: pick(OWNERS),
    temperature,
    fit: source.startsWith("Meta") ? pick(["high", "medium", "medium", "low"] as const) : "unknown",
    reason: reasonFor(signals, temperature),
    arrivedAt,
    lastActivityAt: activity[0]?.occurredAt ?? arrivedAt,
    formAnswers: source.startsWith("Meta")
      ? [
          {
            question: "When are you planning to buy?",
            answer: pick(["This month (launch offer)", "Within 3 months", "Just exploring"]),
          },
          {
            question: "How will you pay?",
            answer: pick(["Cash", "Payment plan", "Mortgage", "Not decided"]),
          },
          {
            question: "Which unit type?",
            answer: pick(["1–2 BR", "3–4 BR", "Duplex / townhouse", "Not sure"]),
          },
          { question: "Purpose", answer: pick(["Live", "Invest", "Both"]) },
        ]
      : [],
    signals,
    activity,
  };
}

export const LEADS: Lead[] = Array.from({ length: 160 }, (_, i) => makeLead(1001 + i)).sort(
  (a, b) => Date.parse(b.lastActivityAt) - Date.parse(a.lastActivityAt),
);

// ——— Proposals ———

export const PROPOSALS: Proposal[] = [
  {
    id: "p1",
    title: "Add 14 negative keywords that spent AED 6,480 with no leads",
    action: "Add negative keywords",
    platform: "google",
    target: "Search | Generic | Abu Dhabi off-plan",
    projectId: 1,
    status: "pending",
    createdAt: hoursAgo(5),
    expiresAt: hoursAgo(-43),
    source: "alert",
    requestedBy: "Search-term check",
    problem:
      '14 search terms such as "abu dhabi rent apartment" and "jobs reportage" took 9% of this campaign\'s spend in 30 days and brought no leads.',
    changes: [
      { field: "Negative keywords (phrase)", before: "38 terms", after: "52 terms" },
      {
        field: "Includes",
        before: "—",
        after: '"rent", "jobs", "careers", "for sale by owner" +10',
      },
    ],
    evidence: [
      { label: "Spend on these terms, 30 days", value: "AED 6,480" },
      { label: "Leads from these terms", value: "0" },
      { label: "Clicks", value: "1,212" },
    ],
    impact: { label: "Saves about AED 6,500 a month", kind: "estimate" },
    reversible: true,
    needsApproval: false,
  },
  {
    id: "p2",
    title: "Move AED 600/day from Verdana Offer to Wadi GCC",
    action: "Change budgets",
    platform: "meta",
    target: "Verdana | Leads | Offer 30% → Wadi | Leads | GCC",
    projectId: null,
    status: "pending",
    createdAt: hoursAgo(9),
    expiresAt: hoursAgo(-39),
    source: "copilot",
    requestedBy: "Sara via the copilot",
    problem:
      "Verdana's offer campaign brings cheap leads but few reach a meeting. Wadi GCC costs more per lead and qualifies at twice the rate.",
    changes: [
      {
        field: "Verdana | Leads | Offer 30% daily budget",
        before: "AED 1,800",
        after: "AED 1,200",
      },
      { field: "Wadi | Leads | GCC daily budget", before: "AED 1,200", after: "AED 1,800" },
    ],
    evidence: [
      { label: "Cost per qualified lead, Verdana", value: "AED 607" },
      { label: "Cost per qualified lead, Wadi GCC", value: "AED 500" },
      { label: "Deals in 30 days", value: "3 vs 5" },
    ],
    impact: { label: "About 5 more qualified leads a month at the same spend", kind: "estimate" },
    reversible: true,
    needsApproval: true,
  },
  {
    id: "p3",
    title: "Pause 2 Meta ads with frequency above 6 and falling CTR",
    action: "Pause ads",
    platform: "meta",
    target: "Wadi | Leads | UAE EN · 2 ads",
    projectId: 1,
    status: "pending",
    createdAt: hoursAgo(20),
    expiresAt: hoursAgo(-28),
    source: "alert",
    requestedBy: "Creative fatigue check",
    problem:
      "Two video ads have been shown to the same people more than 6 times on average and their click rate fell by half over 14 days.",
    changes: [
      { field: '"Valley at dusk" video', before: "Active", after: "Paused" },
      { field: '"Family walk" video', before: "Active", after: "Paused" },
    ],
    evidence: [
      { label: "Frequency, 14 days", value: "6.4 and 7.1" },
      { label: "CTR change, 14 days", value: "−48% and −52%" },
    ],
    impact: { label: "Lower cost per lead on this ad set", kind: "estimate" },
    reversible: true,
    needsApproval: false,
  },
  {
    id: "p4",
    title: "Refresh the re-engagement audience with 412 leads that went cold",
    action: "Update audience",
    platform: "meta",
    target: "adx_reengage_cold_30d",
    projectId: null,
    status: "applied",
    createdAt: daysAgo(2),
    expiresAt: daysAgo(0),
    source: "alert",
    requestedBy: "Audience refresh",
    problem:
      "412 leads went cold in the last 30 days and aren't in the re-engagement audience yet.",
    changes: [{ field: "Audience size", before: "1,880", after: "2,292" }],
    evidence: [{ label: "Excluded: brokers, wrong numbers, opted out", value: "57" }],
    impact: { label: "12 came back to the site in 2 days", kind: "measured" },
    reversible: true,
    needsApproval: false,
    decidedBy: "Omar",
  },
  {
    id: "p5",
    title: "Raise Search | Wadi | Brand budget by 25%",
    action: "Change budget",
    platform: "google",
    target: "Search | Wadi | Brand",
    projectId: 1,
    status: "rejected",
    createdAt: daysAgo(4),
    expiresAt: daysAgo(2),
    source: "copilot",
    requestedBy: "Khalid via the copilot",
    problem: "The brand campaign is limited by budget on weekday evenings.",
    changes: [{ field: "Daily budget", before: "AED 700", after: "AED 875" }],
    evidence: [{ label: "Impression share lost to budget", value: "22%" }],
    impact: { label: "About 8 more leads a month", kind: "estimate" },
    reversible: true,
    needsApproval: true,
    decidedBy: "CMO",
    note: "Hold until the Wadi phase 2 launch next month.",
  },
];

// ——— Alerts ———

export const ALERTS: Alert[] = [
  {
    id: "a1",
    check: "Search terms",
    severity: "high",
    title: "AED 6,480 on search terms with no leads",
    detail: "14 terms in Search | Generic | Abu Dhabi off-plan, last 30 days.",
    platform: "google",
    target: "Search | Generic | Abu Dhabi off-plan",
    costMinor: 648_000,
    detectedAt: hoursAgo(5),
    proposalId: "p1",
  },
  {
    id: "a2",
    check: "Sync gap",
    severity: "high",
    title: "Meta delivered 214 leads that never reached GoHighLevel",
    detail: 'Form "Wadi Launch AR" since Monday. The GoHighLevel sync for this form may be broken.',
    platform: "ghl",
    target: "Wadi Launch AR",
    costMinor: null,
    detectedAt: hoursAgo(7),
    proposalId: null,
  },
  {
    id: "a3",
    check: "Creative fatigue",
    severity: "medium",
    title: "2 ads past frequency 6 with CTR down by half",
    detail: "Wadi | Leads | UAE EN, last 14 days.",
    platform: "meta",
    target: "Wadi | Leads | UAE EN",
    costMinor: null,
    detectedAt: hoursAgo(20),
    proposalId: "p3",
  },
  {
    id: "a4",
    check: "Pacing",
    severity: "medium",
    title: "Meta is pacing 14% over the monthly budget",
    detail: "Projected AED 498,000 against AED 437,000.",
    platform: "meta",
    target: "All Meta campaigns",
    costMinor: 6_100_000,
    detectedAt: hoursAgo(3),
    proposalId: null,
  },
  {
    id: "a5",
    check: "Landing page",
    severity: "medium",
    title: "An advertised URL returns 404",
    detail: "reportageoffer.com/offplan-dubai is in 2 Google ads.",
    platform: "web",
    target: "PMax | Verdana",
    costMinor: null,
    detectedAt: hoursAgo(26),
    proposalId: null,
  },
  {
    id: "a6",
    check: "Delivery",
    severity: "low",
    title: "Sila | Leads | Villas is limited by its bid cap",
    detail: "Spent 61% of budget over 7 days.",
    platform: "meta",
    target: "Sila | Leads | Villas",
    costMinor: null,
    detectedAt: daysAgo(1),
    proposalId: null,
  },
  {
    id: "a7",
    check: "Status coverage",
    severity: "low",
    title: "1,206 leads from the last 30 days have no status",
    detail: "Scores and cost per qualified lead are only as good as the updates.",
    platform: "ghl",
    target: "All leads",
    costMinor: null,
    detectedAt: daysAgo(1),
    proposalId: null,
  },
];

// ——— Connections ———

export const CONNECTIONS: Connection[] = [
  {
    key: "google",
    name: "Google Ads",
    group: "Advertising",
    state: "ready",
    stateLabel: "Ready",
    detail: "Synced daily performance and conversions.",
    accounts: ["Reportage Felix Properties"],
    lastSyncAt: hoursAgo(1),
  },
  {
    key: "meta",
    name: "Meta Ads",
    group: "Advertising",
    state: "pending",
    stateLabel: "Pending",
    detail: "Waiting for the Standard Access test before reading lead forms.",
    accounts: ["Reportage Properties"],
    lastSyncAt: null,
  },
  {
    key: "ghl",
    name: "GoHighLevel",
    group: "CRM",
    state: "caution",
    stateLabel: "Caution",
    detail: "Connected. 1 form has a sync gap with Meta.",
    accounts: ["Reportage Marketing"],
    lastSyncAt: hoursAgo(0.3),
  },
  {
    key: "ga4",
    name: "Google Analytics 4",
    group: "Analytics",
    state: "error",
    stateLabel: "Error",
    detail: "Access was removed. Reconnect with a Viewer account.",
    accounts: ["reportagegroup.com"],
    lastSyncAt: daysAgo(3),
  },
  {
    key: "gsc",
    name: "Search Console",
    group: "Analytics",
    state: "ready",
    stateLabel: "Ready",
    detail: "Queries and pages for 2 sites.",
    accounts: ["reportagegroup.com", "reportageoffer.com"],
    lastSyncAt: hoursAgo(6),
  },
  {
    key: "posthog",
    name: "PostHog",
    group: "Analytics",
    state: "pending",
    stateLabel: "Pending",
    detail: "Not installed on the sites yet.",
    accounts: [],
    lastSyncAt: null,
  },
  {
    key: "instagram",
    name: "Instagram",
    group: "Organic",
    state: "pending",
    stateLabel: "Pending",
    detail: "Comes with the Meta connection.",
    accounts: [],
    lastSyncAt: null,
  },
  {
    key: "linkedin",
    name: "LinkedIn Page",
    group: "Organic",
    state: "pending",
    stateLabel: "Pending",
    detail: "Waiting on LinkedIn's API approval.",
    accounts: [],
    lastSyncAt: null,
  },
];
