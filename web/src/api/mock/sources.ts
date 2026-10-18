// Connect flows for every source except GoHighLevel, which has its own page
// on the real API. Signing in is simulated: the "redirect" returns at once
// with the accounts that login can see.

import type { SourceDetail } from "../models";
import { daysAgo, hoursAgo } from "./data";

const SOURCES: SourceDetail[] = [
  {
    key: "google",
    name: "Google Ads",
    method: "oauth",
    signInWith: "Google",
    reads: [
      "Campaigns, ad groups, ads and keywords",
      "Daily spend and conversions",
      "Search terms",
    ],
    writes: ["Offline conversions", "Customer Match lists", "Approved campaign changes"],
    access: ["Standard access on the manager account", "Read and manage campaigns"],
    state: "ready",
    stateLabel: "Connected",
    issue: null,
    signedInAs: "ads@reportage.example",
    accounts: [
      {
        id: "123-456-7890",
        name: "Reportage Felix Properties",
        connected: true,
        lastSyncAt: hoursAgo(1),
      },
      { id: "234-567-8901", name: "Reportage Brand (old)", connected: false, lastSyncAt: null },
    ],
  },
  {
    key: "meta",
    name: "Meta Ads",
    method: "oauth",
    signInWith: "Facebook",
    reads: ["Campaigns, ad sets and ads", "Daily spend and results", "Instant form leads"],
    writes: ["Conversions API events", "Custom audiences", "Approved campaign changes"],
    access: ["ads_read", "leads_retrieval", "ads_management", "Admin on the Business Manager"],
    state: "pending",
    stateLabel: "Pending",
    issue: "Meta's Standard Access review isn't finished. Lead forms stay unread until it is.",
    signedInAs: "marketing@reportage.example",
    accounts: [
      {
        id: "act_1029384756",
        name: "Reportage Properties",
        connected: true,
        lastSyncAt: hoursAgo(3),
      },
      { id: "act_5647382910", name: "Reportage Hospitality", connected: false, lastSyncAt: null },
    ],
  },
  {
    key: "ga4",
    name: "Google Analytics 4",
    method: "oauth",
    signInWith: "Google",
    reads: ["Sessions and landing pages", "Events and conversions"],
    writes: [],
    access: ["Viewer on each property"],
    state: "error",
    stateLabel: "Reconnect",
    issue: "The account that connected GA4 lost access 2 days ago.",
    signedInAs: null,
    accounts: [],
  },
  {
    key: "gsc",
    name: "Search Console",
    method: "oauth",
    signInWith: "Google",
    reads: ["Search queries", "Pages and positions"],
    writes: [],
    access: ["Restricted user on each site"],
    state: "ready",
    stateLabel: "Connected",
    issue: null,
    signedInAs: "seo@reportage.example",
    accounts: [
      {
        id: "sc-domain:reportagegroup.com",
        name: "reportagegroup.com",
        connected: true,
        lastSyncAt: hoursAgo(6),
      },
      {
        id: "sc-domain:reportageoffer.com",
        name: "reportageoffer.com",
        connected: true,
        lastSyncAt: hoursAgo(6),
      },
    ],
  },
  {
    key: "posthog",
    name: "PostHog",
    method: "key",
    signInWith: null,
    reads: ["Page views and clicks on the websites", "Which visitor became which lead"],
    writes: [],
    access: ["A project API key", "The snippet on every page of both sites"],
    state: "pending",
    stateLabel: "Not set up",
    issue: null,
    signedInAs: null,
    accounts: [],
  },
  {
    key: "instagram",
    name: "Instagram",
    method: "oauth",
    signInWith: "Facebook",
    reads: ["Posts, reach and engagement", "Comments"],
    writes: [],
    access: ["instagram_basic", "instagram_manage_insights", "pages_read_engagement"],
    state: "pending",
    stateLabel: "Not set up",
    issue: null,
    signedInAs: null,
    accounts: [],
  },
  {
    key: "linkedin",
    name: "LinkedIn Page",
    method: "oauth",
    signInWith: "LinkedIn",
    reads: ["Posts, followers and engagement"],
    writes: [],
    access: ["r_organization_social", "Admin on the Reportage page"],
    state: "pending",
    stateLabel: "Waiting",
    issue: "LinkedIn hasn't approved API access yet.",
    signedInAs: null,
    accounts: [],
  },
];

// What each sign-in "finds".
const DISCOVERED: Record<string, SourceDetail["accounts"]> = {
  ga4: [
    { id: "properties/31415926", name: "reportagegroup.com", connected: false, lastSyncAt: null },
    { id: "properties/27182818", name: "reportageoffer.com", connected: false, lastSyncAt: null },
  ],
  instagram: [
    { id: "17841400000000001", name: "@reportageproperties", connected: false, lastSyncAt: null },
  ],
  linkedin: [
    {
      id: "urn:li:organization:1234567",
      name: "Reportage Properties",
      connected: false,
      lastSyncAt: null,
    },
  ],
};

const find = (key: string) => {
  const s = SOURCES.find((x) => x.key === key);
  if (!s) throw new Error("Adx doesn't connect to that source");
  return s;
};

export function source(key: string): SourceDetail | null {
  return SOURCES.find((x) => x.key === key) ?? null;
}

export function signIn(key: string): SourceDetail {
  const s = find(key);
  if (s.key === "linkedin")
    throw new Error("LinkedIn refused: the app isn't approved for this API yet");
  if (s.accounts.length === 0) s.accounts = structuredClone(DISCOVERED[key] ?? []);
  s.signedInAs = "dev.admin@reportage.example";
  if (s.state === "error") {
    s.issue = null;
    s.state = "caution";
    s.stateLabel = "Choose accounts";
  }
  return s;
}

export function setAccounts(key: string, ids: string[]): SourceDetail {
  const s = find(key);
  if (ids.length === 0) throw new Error("Choose at least one account, or disconnect instead");
  for (const a of s.accounts) {
    const on = ids.includes(a.id);
    if (on && !a.connected) a.lastSyncAt = null;
    a.connected = on;
  }
  if (s.state !== "pending" || s.key !== "meta") {
    s.state = "ready";
    s.stateLabel = "Connected";
  }
  return s;
}

export function syncNow(key: string): SourceDetail {
  const s = find(key);
  const t = new Date().toISOString();
  for (const a of s.accounts) if (a.connected) a.lastSyncAt = t;
  return s;
}

export function disconnect(key: string): SourceDetail {
  const s = find(key);
  s.accounts = [];
  s.signedInAs = null;
  s.state = "pending";
  s.stateLabel = "Not set up";
  s.issue = null;
  return s;
}

export function saveKey(key: string, apiKey: string): SourceDetail {
  const s = find(key);
  if (!/^phc_[A-Za-z0-9]{20,}$/.test(apiKey.trim()))
    throw new Error("That isn't a PostHog project key. It starts with phc_.");
  s.accounts = [
    { id: "project-1", name: "Reportage websites", connected: true, lastSyncAt: daysAgo(0) },
  ];
  s.state = "caution";
  s.stateLabel = "No events yet";
  s.issue = "The key works, but no page views have arrived. Add the snippet to both sites.";
  return s;
}
