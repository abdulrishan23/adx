// Shapes the screens read. They're written by hand while the design branch
// runs on mock data. As each endpoint lands in Go, its struct generates a type
// into types.gen.ts and the matching type here is deleted in favour of it.
//
// Rules from CLAUDE.md apply to mocks too: money is minor units plus a
// currency, times are UTC ISO strings, and a missing figure is null.

export type Currency = "AED";
export type Platform = "google" | "meta";
export type Temperature = "hot" | "warm" | "cold";
export type Severity = "high" | "medium" | "low";

export type Money = { minor: number | null; currency: Currency };

// A figure with the period before it, so tiles can show change. `previous` is
// null when there's nothing to compare against.
export type Comparable = { current: number | null; previous: number | null };

export type DailyPoint = { date: string; spendMinor: number; leads: number; qualified: number };

export type Scope = {
  days: 7 | 30 | 90;
  platform: Platform | "all";
  projectId: number | "all";
};

export type Project = { id: number; name: string; city: string };

export type Pacing = {
  spentMinor: number;
  budgetMinor: number;
  projectedMinor: number;
  dayOfMonth: number;
  daysInMonth: number;
};

// The Brief compares yesterday with the 7 days before it, because that's the
// morning question: what changed, and where do I act. Cost per qualified lead
// uses whole weeks, since a single day's is too noisy to act on.
export type DayCounts = { spendMinor: number; leads: number; qualified: number };

// Leads reaching each stage in a week. A stage counts when a lead gets there,
// so a week's Done-Deals mostly come from leads that arrived weeks earlier.
export type StageCounts = {
  spendMinor: number;
  leads: number;
  qualified: number;
  meetings: number;
  offers: number;
  deals: number;
};

export type Brief = {
  date: string; // the day the Brief describes: yesterday, Dubai time
  summary: {
    text: string;
    yesterday: DayCounts;
    average: DayCounts; // a day's average over the 7 days before yesterday
    costPerQualified: Comparable; // last 7 days against the 7 before
  };
  pendingProposals: number;
  funnel: { week: StageCounts; previous: StageCounts };
  projects: BriefProject[];
  total: BriefProject;
  handling: LeadHandling;
  sources: SourceRow[];
  markets: MarketRow[];
  // Share of leads with a GoHighLevel status. Qualified counts depend on it.
  statusCoverage: number;
};

export type BriefProject = {
  projectId: number | null; // null on the total row
  name: string;
  yesterday: { leads: number; qualified: number };
  average: { leads: number; qualified: number };
  costPerQualified: Comparable;
  meetings: number; // last 7 days
  costPerMeeting: number | null;
  // Null when the project has no monthly budget set (open: #34).
  pacing: Pacing | null;
  qualifiedTrend: number[]; // 14 days
  week: StageCounts; // last 7 days
  previousWeek: StageCounts;
};

// Yesterday's leads that nobody has touched: no status change since they arrived.
export type LeadHandling = {
  arrived: number;
  untouched: number;
  untouchedOverAnHour: number;
  medianFirstTouchMinutes: number | null;
  bySource: { source: string; untouched: number }[];
  byOwner: { owner: string; untouched: number }[];
  // Share of the last 7 days' leads marked junk, wrong number or duplicate.
  junk: { source: string; share: number | null; leads: number }[];
};

export type SourceRow = {
  source: string;
  kind: "paid" | "portal" | "organic";
  spendMinor: number | null; // portals: the monthly fee spread over the days
  leads: number;
  qualified: number;
  costPerQualified: Comparable;
};

// Where leads live, from the phone country code. It says nothing about
// nationality, and no spend maps to it, so it shows volume and quality only.
export type MarketRow = {
  market: string;
  code: string;
  leads: number;
  qualified: number;
};

export type Campaign = {
  id: string;
  name: string;
  platform: Platform;
  type: string;
  projectId: number;
  status: "active" | "paused" | "limited";
  spendMinor: number;
  leads: number;
  qualified: number;
  deals: number;
  revenueMinor: number | null;
  temperatureMix: { hot: number; warm: number; cold: number };
  daily: DailyPoint[];
  adSets: { name: string; spendMinor: number; leads: number; qualified: number }[];
};

export type FunnelStage = { key: string; label: string; count: number };

export type FunnelRow = {
  projectId: number;
  spendMinor: number;
  stages: FunnelStage[];
  revenueMinor: number | null;
};

export type Funnel = {
  stages: FunnelStage[];
  spendMinor: number;
  revenueMinor: number | null;
  byProject: FunnelRow[];
  // Revenue and spend by month, newest last.
  months: { month: string; spendMinor: number; revenueMinor: number | null }[];
};

export type MatchLevel = "click_id" | "lead_id" | "contact" | "utm" | "none";

export type Signal = {
  id: string;
  code: string;
  label: string;
  role: "hot" | "warm" | "feature";
  occurredAt: string;
  evidence: string;
  projectId: number | null;
};

export type ActivityEvent = {
  id: string;
  channel: "web" | "whatsapp" | "call" | "form" | "crm" | "social" | "ad";
  label: string;
  detail?: string;
  occurredAt: string;
};

export type Lead = {
  id: number;
  name: string;
  phoneCountry: string;
  source: string;
  originalSource: string;
  matchLevel: MatchLevel;
  campaignId: string | null;
  projectId: number | null;
  status: string | null;
  owner: string;
  temperature: Temperature;
  fit: "high" | "medium" | "low" | "unknown";
  reason: string;
  arrivedAt: string;
  lastActivityAt: string;
  formAnswers: { question: string; answer: string }[];
  signals: Signal[];
  activity: ActivityEvent[];
};

export type LeadsSummary = {
  headline: { text: string; filter: LeadFilter | null };
  stats: { key: LeadFilter; label: string; count: number; urgent?: boolean }[];
};

export type LeadFilter = "hot_no_status" | "no_status_7d" | "back_in_play" | "relabelled";

export type ProposalStatus =
  | "pending"
  | "applying"
  | "applied"
  | "failed"
  | "undone"
  | "rejected"
  | "expired";

export type Proposal = {
  id: string;
  title: string;
  action: string;
  platform: Platform | "ghl";
  target: string;
  projectId: number | null;
  status: ProposalStatus;
  createdAt: string;
  expiresAt: string;
  source: "alert" | "marketer" | "copilot";
  requestedBy: string;
  problem: string;
  changes: { field: string; before: string; after: string }[];
  evidence: { label: string; value: string }[];
  impact: { label: string; kind: "estimate" | "measured" };
  reversible: boolean;
  needsApproval: boolean;
  decidedBy?: string;
  note?: string;
  // Why the last attempt to apply failed. Nothing was changed when it did.
  failure?: string;
  log?: { at: string; text: string }[];
};

export type Alert = {
  id: string;
  check: string;
  severity: Severity;
  title: string;
  detail: string;
  platform: Platform | "ghl" | "web";
  target: string;
  costMinor: number | null;
  detectedAt: string;
  proposalId: string | null;
};

export type ConnectionState = "ready" | "caution" | "error" | "pending";

export type Connection = {
  key: string;
  name: string;
  group: "Advertising" | "CRM" | "Analytics" | "Organic";
  state: ConnectionState;
  stateLabel: string;
  detail: string;
  accounts: string[];
  lastSyncAt: string | null;
};

export type SyncGap = {
  form: string;
  platform: Platform;
  projectId: number;
  platformLeads: number;
  crmLeads: number;
};

export type CoverageRow = { label: string; total: number; covered: number };

export type DataQuality = {
  syncGaps: SyncGap[];
  statusBySource: CoverageRow[];
  statusByOwner: CoverageRow[];
  matchLevels: { level: MatchLevel; count: number }[];
  staleLeads: number;
};

export type MarketerRow = {
  name: string;
  sources: string[];
  spendMinor: number | null;
  leads: number;
  qualified: number;
  deals: number;
  statusCoverage: number;
  weekly: number[];
};

export type PortalRow = {
  key: string;
  name: string;
  leads: number;
  qualified: number;
  deals: number;
  // Typed in by hand each month; null until someone enters it.
  spendMinor: number | null;
  spendMonth: string;
};

export type Audience = {
  id: string;
  name: string;
  platform: Platform;
  purpose: string;
  rule: string;
  size: number;
  matchRate: number | null;
  refreshedAt: string;
  state: "ready" | "caution" | "pending";
  stateLabel: string;
  lift: { reached: number; holdout: number; returnedRate: number; holdoutRate: number } | null;
};

export type WriteAction = {
  key: string;
  label: string;
  detail: string;
  defaultMode: PermissionMode;
};

export type PermissionMode = "allowed" | "approval" | "off";

export type Person = {
  id: number;
  name: string;
  email: string;
  role: "admin" | "marketer" | "viewer";
  approver: boolean;
  lastActiveAt: string | null;
  overrides: Record<string, PermissionMode>;
};

export type StageMapping = { status: string; stage: string | null; leads: number };

export type CatalogueEntry = {
  url: string;
  project: string | null;
  pageType: string;
  weight: number;
  confirmed: boolean;
};

export type Lists = {
  stages: string[];
  mappings: StageMapping[];
  aliases: { projectId: number; aliases: string[] }[];
  catalogue: CatalogueEntry[];
};

export type LandingPage = {
  url: string;
  site: string;
  projectId: number | null;
  sessions: number;
  leads: number;
  bounceRate: number;
  flag: "ok" | "not_found" | "password" | "slow";
};

export type OrganicAccount = {
  key: string;
  name: string;
  followers: number | null;
  followersChange: number | null;
  reach: number | null;
  engagementRate: number | null;
  state: "ready" | "pending";
};

export type OrganicPost = {
  id: string;
  account: string;
  caption: string;
  postedAt: string;
  reach: number;
  engagements: number;
  comments: number;
  buyingQuestions: number;
};

export type SearchQuery = { query: string; clicks: number; impressions: number; position: number };

export type Organic = { accounts: OrganicAccount[]; posts: OrganicPost[]; queries: SearchQuery[] };

export type Report = {
  id: string;
  title: string;
  kind: "monthly" | "weekly";
  period: string;
  createdAt: string;
  state: "ready" | "pending" | "caution";
  stateLabel: string;
  summary: string[];
  figures: { label: string; value: string }[];
};

// Ask Adx: a question answered from connected data, with the steps it took and
// the figures behind the answer.
export type AskStep = { label: string; rows?: number };

export type AskTable = {
  columns: { label: string; numeric?: boolean }[];
  rows: { cells: string[]; leadId?: number }[];
};

export type AskAnswer = {
  id: string;
  question: string;
  askedAt: string;
  steps: AskStep[];
  // Paragraphs; **text** is bold.
  paragraphs: string[];
  table?: AskTable;
  proposal?: { id: string; title: string };
  // A change Adx drafted from the answer. Nothing happens until someone sends it.
  draft?: {
    title: string;
    request: ChangeRequest;
    changes: { field: string; before: string; after: string }[];
    needsApproval: boolean;
  };
  followUps: string[];
  sources: string[];
};

export type Conversation = { id: string; title: string; updatedAt: string; turns: AskAnswer[] };

// Enrichment: what Adx found out about a lead beyond the form, each fact with
// where it came from and how sure Adx is. Nationality is never inferred.
export type Confidence = "high" | "medium" | "low";

export type EnrichedFact = {
  label: string;
  value: string | null;
  source: string;
  confidence: Confidence;
};

export type EnrichmentState = "done" | "running" | "queued" | "failed" | "never";

export type LeadBrief = {
  doThis: string;
  angles: string[];
  watchOut: string | null;
};

export type LeadEnrichment = {
  leadId: number;
  state: EnrichmentState;
  enrichedAt: string | null;
  failure: string | null;
  facts: EnrichedFact[];
  brief: LeadBrief;
};

export type EnrichmentJob = {
  id: string;
  label: string;
  requestedBy: string;
  createdAt: string;
  total: number;
  done: number;
  failed: number;
  state: "running" | "queued" | "paused" | "done";
};

export type EnrichmentQueue = {
  paused: boolean;
  monthUsed: number;
  monthLimit: number;
  jobs: EnrichmentJob[];
  candidates: { cold: number; noStatus: number; neverEnriched: number };
};

// Which leads to send for enrichment.
export type CandidateFilter = {
  temperature: "cold" | "warm" | "any";
  noStatus: boolean;
  projectId: number | "all";
  olderThanDays: 0 | 30 | 90;
  skipEnriched: boolean;
};

// Outcome sync: qualified leads and Done-Deals sent back to Google and Meta, so
// the platforms optimise for buyers instead of form fills.
export type OutcomeDestination = {
  key: string;
  platform: Platform;
  name: string;
  event: string;
  sent30d: number;
  matched30d: number | null;
  failed30d: number;
  lastSentAt: string | null;
  state: "ready" | "caution" | "error" | "pending";
  stateLabel: string;
};

export type OutcomeDay = { date: string; qualified: number; deals: number };

export type OutcomeFailure = {
  id: string;
  leadId: number;
  leadName: string;
  destination: string;
  reason: string;
  at: string;
};

export type AuditItem = {
  id: string;
  title: string;
  finding: string;
  fix: string;
  severity: Severity;
  status: "open" | "fixed";
};

export type Outcomes = {
  destinations: OutcomeDestination[];
  daily: OutcomeDay[];
  failures: OutcomeFailure[];
  audit: AuditItem[];
};

// Guardrails: hard limits the admin sets that no proposal can pass (D-13).
export type AccountGuard = {
  key: string;
  platform: Platform;
  name: string;
  monthlyBudgetMinor: number | null;
  killed: boolean;
  killedBy: string | null;
  killedAt: string | null;
};

export type Guardrails = {
  dailyIncreaseCapPct: number;
  changesPerDay: number;
  changesToday: number;
  proposalHours: number;
  accounts: AccountGuard[];
  blocked: { id: string; at: string; action: string; reason: string; by: string }[];
};

// Scoring: how fit and intent turn into a temperature, and how well each
// temperature predicted what sales did with it.
export type FitWeight = { key: string; question: string; answer: string; points: number };
export type SignalWeight = {
  code: string;
  label: string;
  points: number;
  cap: number;
  halfLifeDays: number;
};
export type Calibration = {
  temperature: Temperature;
  leads: number;
  qualifiedRate: number | null;
  dealRate: number | null;
};
export type Scoring = {
  fit: FitWeight[];
  signals: SignalWeight[];
  thresholds: { hot: number; warm: number };
  calibration: Calibration[];
  coverage: number;
  lastChangedBy: string;
  lastChangedAt: string;
};

// Revenue by channel: what each source brought in deals and Done-Deal value.
// Channels without ad spend (telesales, walk-ins) still count toward revenue.
export type RevenueRow = {
  key: string;
  label: string;
  spendMinor: number;
  leads: number;
  qualified: number;
  deals: number;
  revenueMinor: number | null;
};
export type Revenue = {
  byChannel: RevenueRow[];
  dealsMissingValue: number;
};

// Ads inside a campaign, with what fatigue looks like.
export type AdRow = {
  id: string;
  name: string;
  adSet: string;
  status: "active" | "paused" | "disapproved";
  spendMinor: number;
  leads: number;
  frequency: number | null;
  ctr: number | null;
  ctrWeeks: number[];
  flag: "fatigued" | "disapproved" | "sold_out" | null;
};

// A change a marketer proposes from a campaign. The permission checklist and
// guardrails decide whether it applies now or waits for approval.
export type ChangeRequest =
  | { kind: "pause_ad"; campaignId: string; adId: string }
  | { kind: "daily_budget"; campaignId: string; dailyBudgetMinor: number };

export type ProjectSummary = {
  id: number;
  name: string;
  city: string;
  aliases: string[];
  spendMinor: number;
  leads: number;
  qualified: number;
  deals: number;
  revenueMinor: number | null;
  daily: DailyPoint[];
  campaigns: Campaign[];
  hotLeads: Lead[];
  temperatureMix: { hot: number; warm: number; cold: number };
};

// One outside source and the accounts Adx reads from it.
export type SourceAccount = {
  id: string;
  name: string;
  connected: boolean;
  lastSyncAt: string | null;
};

export type SourceDetail = {
  key: string;
  name: string;
  method: "oauth" | "key";
  signInWith: string | null;
  reads: string[];
  writes: string[];
  access: string[];
  state: ConnectionState;
  stateLabel: string;
  issue: string | null;
  // Accounts found after signing in. Empty until someone signs in.
  accounts: SourceAccount[];
  signedInAs: string | null;
};
