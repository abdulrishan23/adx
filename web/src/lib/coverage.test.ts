import { describe, expect, it } from "vitest";
import type { CampaignCoverage } from "@/api/types.gen";
import { allowedChanges, coverageFor, launchChip, planChip, planLine } from "./coverage";

// The screen's reading of the server's coverage (BD-68). Whether a plan
// stands behind a campaign and where its intent is are tested in
// server/core/launch/launch_test.go; these catch the screen misreading them.

function cov(
  over: Omit<Partial<CampaignCoverage>, "launch"> & {
    launch?: Partial<CampaignCoverage["launch"]>;
  },
) {
  const { launch, ...rest } = over;
  const c: CampaignCoverage = {
    id: 1,
    accountId: 2,
    platform: "google_ads",
    externalId: "20100000008",
    name: "Search | Wadi | Brand",
    status: "ENABLED",
    running: true,
    projectId: 1,
    underPlan: true,
    planId: 1,
    planVersion: 2,
    line: 0,
    kind: "update",
    reason: "plan 1 version 2 (2026-11-01 to 2026-11-30) line 0 updates it",
    launch: {
      state: "none",
      action: "",
      proposalId: null,
      intentId: null,
      intentState: "",
      deliveryDay: null,
      note: "",
      ...launch,
    },
    ...rest,
  };
  return c;
}

const unplanned = cov({
  underPlan: false,
  planId: null,
  planVersion: null,
  line: null,
  kind: "",
  reason: "no accepted plan whose period has not ended resolves to this campaign",
});

describe("coverageFor", () => {
  // Catches: a chip on a campaign Adx never read, a Google row taking a
  // Meta entry with the same id, and the screen's platform word sent as is.
  it("matches by platform and the platform's own id, else unknown", () => {
    const rows = [cov({}), cov({ platform: "meta", externalId: "20100000008", underPlan: false })];
    expect(coverageFor(rows, { platform: "google", externalId: "20100000008" })?.underPlan).toBe(
      true,
    );
    expect(coverageFor(rows, { platform: "meta", externalId: "20100000008" })?.underPlan).toBe(
      false,
    );
    expect(coverageFor(rows, { platform: "google", externalId: "nope" })).toBeNull();
    expect(coverageFor(undefined, { platform: "google", externalId: "20100000008" })).toBeNull();
  });
});

describe("launchChip", () => {
  // Catches: a Launching chip after delivery is observed, a Delivering chip
  // from a submission alone, and a failed or never-started launch shown as
  // launching.
  it("is Launching until delivery is observed, then Delivering", () => {
    for (const state of ["in_review", "submitted", "confirmed"]) {
      const chip = launchChip(cov({ launch: { state, note: "sent" } }));
      expect(chip?.label).toBe("Launching");
      expect(chip?.status).toBe("caution");
    }
    const delivering = launchChip(
      cov({ launch: { state: "delivering", deliveryDay: "2026-10-21", note: "impressions" } }),
    );
    expect(delivering?.label).toBe("Delivering");
    expect(delivering?.status).toBe("ready");
    expect(delivering?.note).toContain("2026-10-21");
    expect(launchChip(cov({ launch: { state: "none" } }))).toBeNull();
    expect(launchChip(cov({ launch: { state: "failed", note: "refused" } }))).toBeNull();
    expect(launchChip(null)).toBeNull();
  });
});

describe("planChip and allowedChanges", () => {
  // Catches: a raise offered on an unplanned campaign, a pause or a cut
  // withheld from one, "Not under a plan" on a campaign Adx never read, and
  // a planned campaign limited.
  it("limits an unplanned campaign to pausing and lowering", () => {
    expect(planChip(unplanned)?.label).toBe("Not under a plan");
    expect(planChip(unplanned)?.status).toBe("pending");
    expect(allowedChanges(unplanned)).toEqual({ raise: false, lower: true, pause: true });
    expect(planChip(cov({}))).toBeNull();
    expect(allowedChanges(cov({}))).toEqual({ raise: true, lower: true, pause: true });
    expect(planChip(null)).toBeNull();
    expect(allowedChanges(null)).toEqual({ raise: true, lower: true, pause: true });
  });

  it("names the plan line behind the campaign", () => {
    expect(planLine(cov({}))).toBe("Plan 1 v2, line 1 updates it.");
    expect(planLine(cov({ kind: "resume", line: 1 }))).toBe("Plan 1 v2, line 2 resumes it.");
    expect(planLine(unplanned)).toContain("Not under a plan");
    expect(planLine(null)).toBeNull();
  });
});
