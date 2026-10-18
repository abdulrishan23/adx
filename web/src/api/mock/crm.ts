// The GoHighLevel connection, mocked with the same rules the Go core applies
// (server/core/connections): admin only, a clean workspace ID, a token with no
// whitespace, and a reconnect that only replaces that workspace.

import type { Connection as CrmConnection, Lead, LeadList, LeadSync } from "../types.legacy";
import { daysAgo, hoursAgo } from "./data";

export const REQUIRED_SCOPES = [
  "contacts.readonly",
  "locations.readonly",
  "locations/customFields.readonly",
  "opportunities.readonly",
];

const crm: CrmConnection[] = [
  {
    provider: "gohighlevel",
    locationId: "rPq7xK2mV9aLtB4wZ8nD",
    companyId: "c0mpReportage01",
    name: "Reportage Properties",
    scopes: REQUIRED_SCOPES,
    tokenExpiresAt: null,
    authMethod: "private_token",
    connectedAt: daysAgo(12),
    verifiedAt: hoursAgo(2),
  },
];

const validId = (id: string) => /^[A-Za-z0-9_-]{1,100}$/.test(id);

export function listCrm(): CrmConnection[] {
  return crm;
}

export function connectPrivate(locationId: string, token: string): CrmConnection {
  const t = token.trim();
  if (!validId(locationId) || t === "" || /\s/.test(t)) {
    throw new Error("A workspace ID and valid private integration token are required");
  }
  // Stands in for GoHighLevel refusing the token or its scopes.
  if (!t.startsWith("pit-")) {
    throw new Error(
      "GoHighLevel refused the token. Check it was copied whole and has the read scopes.",
    );
  }
  const now = new Date().toISOString();
  const existing = crm.find((c) => c.locationId === locationId);
  const row: CrmConnection = {
    provider: "gohighlevel",
    locationId,
    companyId: existing?.companyId ?? "c0mpReportage01",
    name:
      existing?.name ?? (locationId.toLowerCase().includes("test") ? "ADX Test" : "New workspace"),
    scopes: REQUIRED_SCOPES,
    tokenExpiresAt: null,
    authMethod: "private_token",
    connectedAt: now,
    verifiedAt: now,
  };
  if (existing) Object.assign(existing, row);
  else crm.push(row);
  return row;
}

export function checkCrm(locationId: string): CrmConnection {
  const row = crm.find((c) => c.locationId === locationId);
  if (!row) throw new Error("GoHighLevel location is not connected");
  row.verifiedAt = new Date().toISOString();
  return row;
}

// The lead copy, mocked with the rules in server/core/leads: a sync reads the
// whole workspace, keeps the source a lead first arrived with, and records
// status and source changes it notices.

const lead = (
  id: number,
  name: string,
  source: string,
  status: string | null,
  added: string,
  extra: Partial<Lead> = {},
): Lead => ({
  id,
  locationId: "rPq7xK2mV9aLtB4wZ8nD",
  contactId: `ct${id.toString(36).padStart(6, "0")}`,
  name,
  email: `${(name.split(" ")[0] ?? "lead").toLowerCase()}@example.com`,
  phone: `+97150${String(1000000 + id * 7919).slice(-7)}`,
  source,
  originalSource: source,
  status,
  tags: [],
  customFields: status ? { "Lead Status": status } : {},
  utmSource: source === "Meta form" ? "facebook" : "",
  utmMedium: source === "Meta form" ? "paid" : "",
  utmCampaign: "",
  gclid: "",
  fbclid: "",
  createdAt: added,
  updatedAt: added,
  firstSeenAt: added,
  removedAt: null,
  ...extra,
});

const leadCopy: Lead[] = [
  lead(1, "Rahul Mehta", "Meta form", "Follow Up", hoursAgo(3)),
  lead(2, "Ayesha Siddiqui", "Website", "Meeting Booked", hoursAgo(9)),
  lead(3, "Omar Al Mansoori", "WhatsApp Camp.", "New", hoursAgo(20), {
    originalSource: "Meta form",
  }),
  lead(4, "James Whitfield", "Bayut", null, daysAgo(1)),
  lead(5, "Lucía Fernández", "Google Ads", "Not Interested", daysAgo(2)),
  lead(6, "Dmitri Volkov", "Property Finder", "Follow Up", daysAgo(3)),
  lead(7, "Priya Nair", "Meta form", "Done-Deal", daysAgo(5)),
  lead(8, "Bilal Qureshi", "Referral", "Follow Up", daysAgo(6)),
];

let lastSync: LeadSync | null = {
  id: 1,
  locationId: "rPq7xK2mV9aLtB4wZ8nD",
  state: "done",
  startedAt: hoursAgo(1),
  finishedAt: hoursAgo(1),
  seen: leadCopy.length,
  created: 0,
  changed: 2,
  removed: 0,
  statusFieldFound: true,
  error: "",
};

export function listLeads(locationId: string): LeadList {
  const leads = leadCopy.filter((l) => l.locationId === locationId && !l.removedAt);
  return {
    leads,
    total: leads.length,
    lastSync: lastSync?.locationId === locationId ? lastSync : null,
  };
}

export function startLeadSync(locationId: string): LeadSync {
  if (!crm.some((c) => c.locationId === locationId)) {
    throw new Error("GoHighLevel location is not connected");
  }
  if (lastSync?.locationId === locationId && lastSync.state === "running") {
    throw new Error("a sync for this workspace is already running");
  }
  const started = new Date().toISOString();
  const sync: LeadSync = {
    id: (lastSync?.id ?? 0) + 1,
    locationId,
    state: "running",
    startedAt: started,
    finishedAt: null,
    seen: 0,
    created: 0,
    changed: 0,
    removed: 0,
    statusFieldFound: true,
    error: "",
  };
  lastSync = sync;
  setTimeout(() => {
    const seen = leadCopy.filter((l) => l.locationId === locationId).length;
    lastSync = { ...sync, state: "done", finishedAt: new Date().toISOString(), seen };
  }, 2500);
  return sync;
}
