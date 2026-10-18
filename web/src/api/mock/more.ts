// Mock data for the second batch of screens. Where a figure can be derived
// from the leads and campaigns in data.ts it is, so screens agree with each
// other; the rest is fixed and invented.

import type {
  Audience,
  CatalogueEntry,
  CoverageRow,
  DataQuality,
  LandingPage,
  Lists,
  MarketerRow,
  Organic,
  Person,
  PortalRow,
  Report,
  StageMapping,
  WriteAction,
} from "../models";
import { CAMPAIGNS, daysAgo, hoursAgo, LEADS, now, OWNERS, PROJECTS } from "./data";

function coverage(groupBy: (l: (typeof LEADS)[number]) => string): CoverageRow[] {
  const rows = new Map<string, CoverageRow>();
  for (const l of LEADS) {
    const key = groupBy(l);
    const row = rows.get(key) ?? { label: key, total: 0, covered: 0 };
    row.total += 1;
    if (l.status) row.covered += 1;
    rows.set(key, row);
  }
  return [...rows.values()].sort((a, b) => b.total - a.total);
}

export function dataQuality(): DataQuality {
  const levels = ["click_id", "lead_id", "contact", "utm", "none"] as const;
  return {
    syncGaps: [
      {
        form: "Wadi Launch AR",
        platform: "meta",
        projectId: 1,
        platformLeads: 1240,
        crmLeads: 1026,
      },
      { form: "Wadi Launch EN", platform: "meta", projectId: 1, platformLeads: 868, crmLeads: 861 },
      {
        form: "Verdana Offer 30%",
        platform: "meta",
        projectId: 2,
        platformLeads: 572,
        crmLeads: 566,
      },
      { form: "Oceana GCC", platform: "meta", projectId: 3, platformLeads: 170, crmLeads: 170 },
      { form: "Sila Villas", platform: "meta", projectId: 4, platformLeads: 151, crmLeads: 139 },
      {
        form: "Search lead form (website)",
        platform: "google",
        projectId: 1,
        platformLeads: 214,
        crmLeads: 188,
      },
    ],
    statusBySource: coverage((l) => l.originalSource),
    statusByOwner: coverage((l) => l.owner),
    matchLevels: levels.map((level) => ({
      level,
      count: LEADS.filter((l) => l.matchLevel === level).length,
    })),
    staleLeads: LEADS.filter(
      (l) => !l.status && now.getTime() - Date.parse(l.arrivedAt) > 7 * 86_400_000,
    ).length,
  };
}

// Which sources each marketer owns. The real list comes from the Generator
// column in Reportage's data.
const OWNED: Record<(typeof OWNERS)[number], string[]> = {
  Sara: ["Meta", "WhatsApp"],
  Omar: ["Meta"],
  Priya: ["Google", "Meta", "WhatsApp"],
  Khalid: ["Portals"],
};

export function marketers(): MarketerRow[] {
  const metaSpend = CAMPAIGNS.filter((c) => c.platform === "meta").reduce(
    (a, c) => a + c.spendMinor,
    0,
  );
  const googleSpend = CAMPAIGNS.filter((c) => c.platform === "google").reduce(
    (a, c) => a + c.spendMinor,
    0,
  );
  return OWNERS.map((name, i) => {
    const mine = LEADS.filter((l) => l.owner === name);
    const sources = OWNED[name];
    const spend =
      (sources.includes("Meta") ? metaSpend / 3 : 0) +
      (sources.includes("Google") ? googleSpend : 0);
    return {
      name,
      sources,
      spendMinor: spend > 0 ? Math.round(spend) : null,
      leads: mine.length * 21,
      qualified:
        mine.filter((l) => l.status && l.status !== "Not Interested" && l.status !== "No Answer")
          .length * 21,
      deals: [9, 6, 11, 0][i] ?? 0,
      statusCoverage: mine.filter((l) => l.status).length / Math.max(1, mine.length),
      weekly: Array.from({ length: 8 }, (_, w) => 120 + ((w * 37 + i * 53) % 90)),
    };
  });
}

let portals: PortalRow[] = [
  {
    key: "bayut",
    name: "Bayut",
    leads: 98,
    qualified: 9,
    deals: 0,
    spendMinor: 4_500_000,
    spendMonth: "",
  },
  {
    key: "pf",
    name: "Property Finder",
    leads: 80,
    qualified: 7,
    deals: 0,
    spendMinor: 3_800_000,
    spendMonth: "",
  },
  {
    key: "guru",
    name: "PropertyGuru",
    leads: 113,
    qualified: 5,
    deals: 0,
    spendMinor: null,
    spendMonth: "",
  },
  {
    key: "dubizzle",
    name: "dubizzle",
    leads: 41,
    qualified: 3,
    deals: 0,
    spendMinor: null,
    spendMonth: "",
  },
];
const thisMonth = now.toISOString().slice(0, 7);
portals = portals.map((p) => ({ ...p, spendMonth: thisMonth }));

export function portalRows(): PortalRow[] {
  return portals;
}

export function setPortalSpend(key: string, spendMinor: number | null): PortalRow {
  const row = portals.find((p) => p.key === key);
  if (!row) throw new Error("That portal doesn't exist");
  if (spendMinor !== null && (!Number.isFinite(spendMinor) || spendMinor < 0)) {
    throw new Error("Spend must be zero or more");
  }
  row.spendMinor = spendMinor;
  return row;
}

export const AUDIENCES: Audience[] = [
  {
    id: "au1",
    name: "adx_reengage_cold_30d",
    platform: "meta",
    purpose: "Re-engagement",
    rule: "Leads that went cold in the last 30 days, excluding brokers, wrong numbers, duplicates and opt-outs",
    size: 2292,
    matchRate: 0.71,
    refreshedAt: daysAgo(2),
    state: "ready",
    stateLabel: "Ready",
    lift: { reached: 2063, holdout: 229, returnedRate: 0.058, holdoutRate: 0.022 },
  },
  {
    id: "au2",
    name: "adx_reengage_no_answer",
    platform: "google",
    purpose: "Re-engagement",
    rule: "Leads with 3 or more No Answer attempts in 30 days",
    size: 940,
    matchRate: 0.48,
    refreshedAt: daysAgo(1),
    state: "ready",
    stateLabel: "Ready",
    lift: { reached: 846, holdout: 94, returnedRate: 0.041, holdoutRate: 0.032 },
  },
  {
    id: "au3",
    name: "adx_lookalike_seed_deals",
    platform: "meta",
    purpose: "Lookalike seed",
    rule: "Done-Deal buyers from the last 12 months",
    size: 612,
    matchRate: 0.83,
    refreshedAt: daysAgo(6),
    state: "caution",
    stateLabel: "Small",
    lift: null,
  },
  {
    id: "au4",
    name: "adx_exclude_active_pipeline",
    platform: "meta",
    purpose: "Exclusion",
    rule: "Leads in Follow Up, Potential, Meeting or Sales offer, kept out of prospecting",
    size: 1488,
    matchRate: 0.74,
    refreshedAt: hoursAgo(20),
    state: "ready",
    stateLabel: "Ready",
    lift: null,
  },
  {
    id: "au5",
    name: "adx_exclude_brokers",
    platform: "google",
    purpose: "Exclusion",
    rule: "Leads marked Broker or answering Broker on a form",
    size: 0,
    matchRate: null,
    refreshedAt: hoursAgo(20),
    state: "pending",
    stateLabel: "Pending",
    lift: null,
  },
];

export const WRITE_ACTIONS: WriteAction[] = [
  {
    key: "audiences",
    label: "Create or update audiences",
    detail: "Meta custom audiences and Google Customer Match",
    defaultMode: "allowed",
  },
  {
    key: "outcomes",
    label: "Send outcomes to Google and Meta",
    detail: "Qualified leads and Done-Deals, sent automatically",
    defaultMode: "allowed",
  },
  {
    key: "negatives",
    label: "Exclusions, labels and negative keywords",
    detail: "Things that only stop spend",
    defaultMode: "allowed",
  },
  {
    key: "ghl_fields",
    label: "Set temperature and score in GoHighLevel",
    detail: "Adx's own fields only",
    defaultMode: "allowed",
  },
  { key: "pause_ad", label: "Pause an ad", detail: "One ad at a time", defaultMode: "allowed" },
  {
    key: "lower_budget",
    label: "Lower a budget",
    detail: "Never below the cap's floor",
    defaultMode: "allowed",
  },
  {
    key: "create_paused",
    label: "Create a campaign or ad, paused",
    detail: "Launching is separate",
    defaultMode: "allowed",
  },
  {
    key: "pause_campaign",
    label: "Pause an ad set or campaign",
    detail: "Can cut off many leads at once",
    defaultMode: "approval",
  },
  {
    key: "raise_budget",
    label: "Raise a budget within the cap",
    detail: "Spends more",
    defaultMode: "approval",
  },
  {
    key: "bids",
    label: "Change bids or targets",
    detail: "Target cost or target return",
    defaultMode: "approval",
  },
  {
    key: "launch",
    label: "Launch anything",
    detail: "Turns paused work live",
    defaultMode: "approval",
  },
  {
    key: "targeting",
    label: "Change targeting or geography",
    detail: "Including GCC-only rules",
    defaultMode: "approval",
  },
  { key: "organic", label: "Post on organic accounts", detail: "Coming later", defaultMode: "off" },
];

export const PEOPLE: Person[] = [
  {
    id: 1,
    name: "CMO",
    email: "cmo@reportage.example",
    role: "admin",
    approver: true,
    lastActiveAt: hoursAgo(3),
    overrides: {},
  },
  {
    id: 2,
    name: "Head of Marketing & AI",
    email: "head.ai@reportage.example",
    role: "marketer",
    approver: false,
    lastActiveAt: hoursAgo(1),
    overrides: { raise_budget: "allowed" },
  },
  {
    id: 3,
    name: "Sara",
    email: "sara@reportage.example",
    role: "marketer",
    approver: false,
    lastActiveAt: hoursAgo(5),
    overrides: {},
  },
  {
    id: 4,
    name: "Omar",
    email: "omar@reportage.example",
    role: "marketer",
    approver: false,
    lastActiveAt: daysAgo(1),
    overrides: { pause_ad: "approval" },
  },
  {
    id: 5,
    name: "Priya",
    email: "priya@reportage.example",
    role: "marketer",
    approver: false,
    lastActiveAt: hoursAgo(26),
    overrides: {},
  },
  {
    id: 6,
    name: "Khalid",
    email: "khalid@reportage.example",
    role: "marketer",
    approver: false,
    lastActiveAt: null,
    overrides: {},
  },
  {
    id: 7,
    name: "CEO",
    email: "ceo@reportage.example",
    role: "viewer",
    approver: false,
    lastActiveAt: daysAgo(4),
    overrides: {},
  },
];

const STAGES = [
  "Open",
  "Attempted",
  "Engaged",
  "Meeting",
  "Offer",
  "Won",
  "Disqualified",
  "Dead",
  "Excluded",
];

const MAPPINGS: StageMapping[] = [
  { status: "(blank)", stage: "Open", leads: 1813 },
  { status: "No Answer", stage: "Attempted", leads: 596 },
  { status: "Follow Up", stage: "Engaged", leads: 565 },
  { status: "Not Interested", stage: "Disqualified", leads: 168 },
  { status: "Unreachable", stage: "Dead", leads: 96 },
  { status: "Done-Deal", stage: "Won", leads: 58 },
  { status: "Duplicated", stage: "Excluded", leads: 41 },
  { status: "Meeting", stage: "Meeting", leads: 35 },
  { status: "Potential", stage: "Engaged", leads: 27 },
  { status: "Broker", stage: "Excluded", leads: 25 },
  { status: "Ready To Move", stage: "Disqualified", leads: 16 },
  { status: "Sales offer", stage: "Offer", leads: 4 },
  { status: "Low Budget", stage: "Disqualified", leads: 4 },
  { status: "No availability", stage: "Disqualified", leads: 3 },
  { status: "Site Visit Booked", stage: null, leads: 6 },
];

const CATALOGUE: CatalogueEntry[] = [
  {
    url: "/offer/verdana-offer/",
    project: "Verdana",
    pageType: "Offer",
    weight: 3,
    confirmed: true,
  },
  { url: "/offer/sensi-offer/", project: "Sensi", pageType: "Offer", weight: 3, confirmed: true },
  {
    url: "/projects/residential-communities/the-wadi/",
    project: "The Wadi",
    pageType: "Project",
    weight: 1,
    confirmed: true,
  },
  {
    url: "/projects/residential-apartments/oceana/",
    project: "Oceana",
    pageType: "Project",
    weight: 1,
    confirmed: true,
  },
  {
    url: "/projects/residential-apartments/selina-bay/",
    project: "Selina Bay",
    pageType: "Project",
    weight: 1,
    confirmed: false,
  },
  { url: "/invest-with-reportage/", project: null, pageType: "Invest", weight: 1, confirmed: true },
  {
    url: "/construction-updates/",
    project: null,
    pageType: "Construction",
    weight: 0,
    confirmed: true,
  },
  {
    url: "/projects/residential-communities/r-dot-hills/",
    project: "R. Hills",
    pageType: "Project",
    weight: 1,
    confirmed: false,
  },
  {
    url: "/news/sensi-launch-jan-19th/",
    project: "Sensi",
    pageType: "Content",
    weight: 0,
    confirmed: false,
  },
];

export function lists(): Lists {
  return {
    stages: STAGES,
    mappings: MAPPINGS,
    aliases: PROJECTS.map((p) => ({
      projectId: p.id,
      aliases:
        p.id === 1
          ? ["Wadi", "The Wadi", "WADI"]
          : p.id === 2
            ? ["Verdana", "Verdana DIP"]
            : [p.name],
    })),
    catalogue: CATALOGUE,
  };
}

export function setStage(status: string, stage: string): StageMapping {
  const row = MAPPINGS.find((m) => m.status === status);
  if (!row) throw new Error("That status isn't in GoHighLevel any more");
  if (!STAGES.includes(stage)) throw new Error("Pick one of the stages");
  row.stage = stage;
  return row;
}

export const LANDING_PAGES: LandingPage[] = [
  {
    url: "/projects/residential-communities/the-wadi/",
    site: "reportagegroup.com",
    projectId: 1,
    sessions: 18_420,
    leads: 212,
    bounceRate: 0.41,
    flag: "ok",
  },
  {
    url: "/offer/verdana-offer/",
    site: "reportagegroup.com",
    projectId: 2,
    sessions: 9_870,
    leads: 164,
    bounceRate: 0.38,
    flag: "ok",
  },
  {
    url: "/",
    site: "reportageoffer.com",
    projectId: null,
    sessions: 6_240,
    leads: 0,
    bounceRate: 0.92,
    flag: "password",
  },
  {
    url: "/projects/residential-apartments/oceana/",
    site: "reportagegroup.com",
    projectId: 3,
    sessions: 4_110,
    leads: 37,
    bounceRate: 0.49,
    flag: "ok",
  },
  {
    url: "/offplan-dubai",
    site: "reportageoffer.com",
    projectId: null,
    sessions: 1_380,
    leads: 0,
    bounceRate: 0.97,
    flag: "not_found",
  },
  {
    url: "/projects/residential-communities/sila/",
    site: "reportagegroup.com",
    projectId: 4,
    sessions: 2_960,
    leads: 22,
    bounceRate: 0.52,
    flag: "slow",
  },
  {
    url: "/offer/sensi-offer/",
    site: "reportagegroup.com",
    projectId: 5,
    sessions: 2_010,
    leads: 18,
    bounceRate: 0.47,
    flag: "ok",
  },
  {
    url: "/project-listing/",
    site: "reportagegroup.com",
    projectId: null,
    sessions: 5_640,
    leads: 9,
    bounceRate: 0.35,
    flag: "ok",
  },
];

export function organic(): Organic {
  return {
    accounts: [
      {
        key: "instagram",
        name: "Instagram",
        followers: 48_210,
        followersChange: 0.021,
        reach: 312_000,
        engagementRate: 0.034,
        state: "ready",
      },
      {
        key: "facebook",
        name: "Facebook",
        followers: 131_400,
        followersChange: 0.004,
        reach: 205_000,
        engagementRate: 0.011,
        state: "ready",
      },
      {
        key: "linkedin",
        name: "LinkedIn",
        followers: null,
        followersChange: null,
        reach: null,
        engagementRate: null,
        state: "pending",
      },
      {
        key: "youtube",
        name: "YouTube",
        followers: 3_920,
        followersChange: 0.013,
        reach: 41_000,
        engagementRate: 0.027,
        state: "ready",
      },
    ],
    posts: [
      {
        id: "po1",
        account: "Instagram",
        caption: "The Wadi: a community drawn from the land",
        postedAt: daysAgo(3),
        reach: 48_200,
        engagements: 2_310,
        comments: 184,
        buyingQuestions: 23,
      },
      {
        id: "po2",
        account: "Instagram",
        caption: "Verdana offer: 30% down payment, 1% monthly",
        postedAt: daysAgo(6),
        reach: 39_800,
        engagements: 1_640,
        comments: 212,
        buyingQuestions: 41,
      },
      {
        id: "po3",
        account: "Facebook",
        caption: "Construction update: Perla Waves at 92%",
        postedAt: daysAgo(8),
        reach: 22_100,
        engagements: 610,
        comments: 37,
        buyingQuestions: 2,
      },
      {
        id: "po4",
        account: "YouTube",
        caption: "Oceana walk-through, 2BR Type B",
        postedAt: daysAgo(12),
        reach: 9_400,
        engagements: 520,
        comments: 61,
        buyingQuestions: 9,
      },
    ],
    queries: [
      { query: "reportage properties", clicks: 4_120, impressions: 18_300, position: 1.2 },
      { query: "the wadi abu dhabi", clicks: 1_870, impressions: 9_600, position: 2.1 },
      { query: "verdana dubai investments park", clicks: 960, impressions: 7_400, position: 3.4 },
      { query: "off plan abu dhabi payment plan", clicks: 310, impressions: 14_900, position: 8.7 },
      { query: "oceana fujairah price", clicks: 240, impressions: 3_100, position: 4.0 },
    ],
  };
}

const month = (back: number) =>
  new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - back, 1)).toLocaleDateString(
    "en-GB",
    {
      month: "long",
      year: "numeric",
      timeZone: "UTC",
    },
  );

export const REPORTS: Report[] = [
  {
    id: "r1",
    title: "Weekly marketing report",
    kind: "weekly",
    period: "This week so far",
    createdAt: hoursAgo(2),
    state: "pending",
    stateLabel: "Building",
    summary: [],
    figures: [],
  },
  {
    id: "r2",
    title: `Leadership report, ${month(1)}`,
    kind: "monthly",
    period: month(1),
    createdAt: daysAgo(13),
    state: "ready",
    stateLabel: "Ready",
    summary: [
      "Meta brought 61% of leads and 28 of 58 deals. Google brought 2 deals from 465 leads.",
      "The Wadi produced 23 deals, the most of any project.",
      "Half of all leads never got a status, so cost per qualified lead is understated.",
      "Portals brought no deals against AED 360,000 of budgeted spend.",
    ],
    figures: [
      { label: "Spend", value: "AED 1.21M" },
      { label: "Leads", value: "3,445" },
      { label: "Done-Deals", value: "58" },
      { label: "Revenue", value: "AED 112M" },
    ],
  },
  {
    id: "r3",
    title: "Weekly marketing report",
    kind: "weekly",
    period: "Last week",
    createdAt: daysAgo(3),
    state: "caution",
    stateLabel: "Partial",
    summary: [
      "Google Analytics was disconnected for 2 days, so website figures are incomplete.",
      "Cost per qualified lead fell 6% as Verdana budget moved to Wadi GCC.",
    ],
    figures: [
      { label: "Spend", value: "AED 94K" },
      { label: "Qualified leads", value: "151" },
      { label: "Per qualified", value: "AED 622" },
    ],
  },
];
