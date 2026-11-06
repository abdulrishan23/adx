import { describe, expect, it } from "vitest";
import type { Coverage, Lead } from "@/api/types.gen";
import {
  coverageTiles,
  filterLeads,
  headline,
  leadName,
  matchesFilter,
  temperatureCounts,
  temperatureOf,
} from "./leads";

function lead(over: Partial<Lead> = {}): Lead {
  return {
    id: 1,
    accountId: 1,
    externalId: "ct1",
    firstName: "Rahul",
    lastName: "Mehta",
    email: null,
    phone: "+971501234567",
    dialCode: "971",
    sourceRaw: "Meta",
    source: "meta",
    projectRaw: "The Wadi",
    projectId: 1,
    statusRaw: "Follow Up",
    statusObservedAt: "2026-11-01T08:00:00Z",
    statusMissingSince: null,
    stage: "engaged",
    providerAddedAt: "2026-10-30T08:00:00Z",
    providerUpdatedAt: "2026-11-01T08:00:00Z",
    firstSeenAt: "2026-10-30T09:00:00Z",
    lastSeenAt: "2026-11-02T09:00:00Z",
    evidenceId: null,
    matchLevel: "click_id",
    personId: null,
    temperature: null,
    ...over,
  };
}

const coverage = (over: Partial<Coverage> = {}): Coverage => ({
  total: 10,
  withStatus: 7,
  withStage: 6,
  withProject: 4,
  withPhone: 10,
  statusMissingOnLastScan: 2,
  sharedPhones: 1,
  matchLevels: { click_id: 3 },
  funnelConfigured: true,
  ...over,
});

describe("temperature", () => {
  it("never reads an unassessed lead as cold", () => {
    expect(temperatureOf(lead({ temperature: null }))).toBeNull();
    expect(temperatureOf(lead({ temperature: "lukewarm" }))).toBeNull();
    expect(temperatureOf(lead({ temperature: "cold" }))).toBe("cold");
  });

  it("counts unassessed leads under All only", () => {
    const counts = temperatureCounts([
      lead({ temperature: "hot" }),
      lead({ id: 2, temperature: null }),
      lead({ id: 3, temperature: "cold" }),
    ]);
    expect(counts).toEqual({ hot: 1, warm: 0, cold: 1, all: 3 });
  });

  it("keeps an unassessed lead out of the Hot tab but finds it by search", () => {
    const all = [
      lead({ temperature: "hot" }),
      lead({ id: 2, firstName: "Ayesha", temperature: null }),
    ];
    expect(filterLeads(all, { tab: "hot", filter: null, query: "" }).map((l) => l.id)).toEqual([1]);
    expect(filterLeads(all, { tab: "hot", filter: null, query: "aye" }).map((l) => l.id)).toEqual([
      2,
    ]);
  });
});

describe("filters", () => {
  it("tells no status from a status field missing on the last scan", () => {
    const kept = lead({ statusRaw: "Follow Up", statusMissingSince: "2026-11-02T09:00:00Z" });
    expect(matchesFilter(kept, "no_status")).toBe(false);
    expect(matchesFilter(kept, "status_missing")).toBe(true);
    expect(matchesFilter(lead({ statusRaw: null }), "no_status")).toBe(true);
  });

  it("treats an unmapped status as no stage, not as no status", () => {
    const l = lead({ statusRaw: "Potential", stage: null });
    expect(matchesFilter(l, "no_stage")).toBe(true);
    expect(matchesFilter(l, "no_status")).toBe(false);
  });

  it("searches phone and email as well as the name", () => {
    const all = [
      lead(),
      lead({
        id: 2,
        firstName: null,
        lastName: null,
        email: "ayesha@example.com",
        phone: "+971559876543",
      }),
    ];
    expect(
      filterLeads(all, { tab: "all", filter: null, query: "0501234" }).map((l) => l.id),
    ).toEqual([1]);
    expect(
      filterLeads(all, { tab: "all", filter: null, query: "ayesha@" }).map((l) => l.id),
    ).toEqual([2]);
  });
});

describe("names", () => {
  it("falls back to the email, the phone, then the GoHighLevel id", () => {
    expect(leadName(lead())).toBe("Rahul Mehta");
    expect(leadName(lead({ firstName: null, lastName: null, email: "a@b.c" }))).toBe("a@b.c");
    expect(leadName(lead({ firstName: null, lastName: null, email: null }))).toBe("+971501234567");
    expect(leadName(lead({ firstName: " ", lastName: null, email: null, phone: null }))).toBe(
      "Contact ct1",
    );
  });
});

describe("coverage", () => {
  it("shows the gaps, not the covered counts, and the share beside each", () => {
    const tiles = coverageTiles(coverage());
    expect(tiles.map((t) => [t.key, t.count])).toEqual([
      ["no_status", 3],
      ["no_stage", 4],
      ["no_project", 6],
      ["status_missing", 2],
    ]);
    expect(tiles[0]?.detail).toBe("70% have one");
    expect(tiles[0]?.urgent).toBe(true);
  });

  it("says Not available for the stage gap until the table is saved, never 0", () => {
    const tile = coverageTiles(coverage({ funnelConfigured: false, withStage: 0 }))[1];
    expect(tile?.count).toBeNull();
    expect(tile?.label).toContain("table not saved");
  });

  it("has no share and no headline filter when there are no leads", () => {
    const empty = coverage({
      total: 0,
      withStatus: 0,
      withStage: 0,
      withProject: 0,
      withPhone: 0,
      statusMissingOnLastScan: 0,
    });
    expect(coverageTiles(empty)[0]?.detail).toBe("Not available");
    expect(headline(empty)).toEqual({ text: "No leads yet", filter: null });
  });

  it("headlines the leads without a status and clears the filter once every lead has one", () => {
    expect(headline(coverage())).toEqual({
      text: "3 leads have no status in GoHighLevel",
      filter: "no_status",
    });
    expect(headline(coverage({ withStatus: 9 })).text).toBe("1 lead has no status in GoHighLevel");
    expect(headline(coverage({ withStatus: 10 })).filter).toBeNull();
  });
});
