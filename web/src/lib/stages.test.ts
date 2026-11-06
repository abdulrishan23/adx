import { describe, expect, it } from "vitest";
import type { FunnelState } from "@/api/types.gen";
import { blankCount, countsAs, emptyDocument, statusRows, unmappedIn, withStage } from "./stages";

const state = (over: Partial<FunnelState> = {}): FunnelState => ({
  configured: true,
  current: {
    version: 1,
    createdBy: null,
    createdAt: "2026-11-01T08:00:00Z",
    document: { ...emptyDocument(), statuses: { "follow up": "engaged" } },
  },
  stages: [
    "open",
    "attempted",
    "engaged",
    "meeting",
    "offer",
    "won",
    "disqualified",
    "excluded",
    "dead",
  ],
  observedStatuses: [
    { raw: "Follow Up", stage: "engaged", count: 5 },
    { raw: "follow  up", stage: "engaged", count: 1 },
    { raw: "Call Back", stage: null, count: 2 },
    { raw: null, stage: null, count: 3 },
  ],
  unmapped: ["Call Back"],
  ...over,
});

describe("rows", () => {
  it("folds spellings the engine treats as one status into one row", () => {
    const rows = statusRows(state(), { ...emptyDocument(), statuses: { "follow up": "engaged" } });
    expect(rows.map((r) => [r.label, r.leads, r.draft])).toEqual([
      ["Follow Up", 6, "engaged"],
      ["Call Back", 2, null],
    ]);
  });

  it("keeps the blank status out of the rows and counts it on its own", () => {
    const rows = statusRows(state(), emptyDocument());
    expect(rows.some((r) => r.label === "")).toBe(false);
    expect(blankCount(state())).toBe(3);
  });

  it("lists a status the table names before any lead has it, with no leads", () => {
    const rows = statusRows(state(), { ...emptyDocument(), statuses: { "Done-Deal": "won" } });
    const row = rows.find((r) => r.label === "Done-Deal");
    expect(row).toMatchObject({ leads: 0, observed: false, draft: "won" });
  });

  it("flags only statuses seen in the data as unmapped", () => {
    const draft = { ...emptyDocument(), statuses: { "Done-Deal": "won" as const } };
    expect(unmappedIn(state(), draft)).toEqual(["Follow Up", "Call Back"]);
  });
});

describe("editing", () => {
  it("replaces any spelling of the status when a stage is picked, and removes it on No stage", () => {
    let doc = withStage(
      { ...emptyDocument(), statuses: { "Follow  Up": "engaged" } },
      "follow up",
      "meeting",
    );
    expect(doc.statuses).toEqual({ "follow up": "meeting" });
    doc = withStage(doc, "FOLLOW UP", null);
    expect(doc.statuses).toEqual({});
  });

  it("counts engaged, meeting, offer and won as qualified and nothing else", () => {
    expect(countsAs("engaged")).toBe("Qualified");
    expect(countsAs("won")).toBe("Qualified");
    expect(countsAs("open")).toBe("Not qualified");
    expect(countsAs(null)).toBe("");
  });
});
