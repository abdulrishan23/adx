import { describe, expect, it } from "vitest";
import type { LineResolution, ProjectCampaign } from "@/api/types.gen";
import {
  accountsByPlatform,
  lineFigures,
  pickCaption,
  plainNote,
  platformsWithoutAccount,
  submitBlocker,
} from "./launch";

// The screen's own reading of the server's resolution (BD-68). The rules
// (what a line matches, how spend nets off) are tested in
// server/core/launch/launch_test.go; these catch the screen misreading them.

function campaign(over: Partial<ProjectCampaign>): ProjectCampaign {
  return {
    id: 1,
    accountId: 2,
    platform: "google_ads",
    externalId: "g-1",
    name: "Search | Wadi | Brand",
    type: "SEARCH",
    status: "ENABLED",
    running: true,
    projectId: 1,
    currency: "AED",
    dailyBudgetMinor: 150000,
    spentMinor: 2400000,
    spendDays: 20,
    ...over,
  };
}

function line(
  over: Partial<LineResolution> & { index?: number; platform?: string },
): LineResolution {
  const { index = 0, platform = "google_ads", ...rest } = over;
  return {
    kind: "create",
    line: {
      index,
      channel: platform === "meta" ? "meta_lead_form" : "google_search",
      platform,
      campaignType: platform === "meta" ? "OUTCOME_LEADS" : "SEARCH",
      projectId: 1,
      budgetMinor: 30000000,
      currency: "AED",
      objective: "leads",
      geography: "AE",
    },
    campaign: null,
    candidates: [],
    budget: {
      spentMinor: 0,
      spendKnown: true,
      remainingMinor: 30000000,
      dailyMinor: 2727273,
      daysLeft: 11,
    },
    pick: null,
    paused: [],
    note: "",
    ...rest,
  };
}

describe("lineFigures", () => {
  // Catches: a campaign with no figures read this month shown as AED 0 spent.
  it("says Not available when the spend is unknown", () => {
    const f = lineFigures(
      line({
        kind: "update",
        campaign: campaign({ spentMinor: null, spendDays: 0 }),
        budget: {
          spentMinor: 0,
          spendKnown: false,
          remainingMinor: 30000000,
          dailyMinor: 2727273,
          daysLeft: 11,
        },
      }),
    );
    expect(f?.spent).toBe("Not available");
    expect(f?.left).toBe("AED 300K");
    expect(f?.daily).toBe("AED 27.3K");
  });

  // Catches: figures invented for a line the server could not budget.
  it("has no figures for a pick or an unsupported line", () => {
    expect(lineFigures(line({ kind: "pick", budget: null }))).toBeNull();
    expect(lineFigures(line({ kind: "unsupported", budget: null }))).toBeNull();
  });
});

describe("submitBlocker", () => {
  // Catches: Submit enabled while a line still waits on a person's choice.
  it("names the lines still waiting on a pick", () => {
    const lines = [
      line({ kind: "pick", budget: null, candidates: [campaign({}), campaign({ id: 2 })] }),
      line({
        index: 1,
        platform: "meta",
        kind: "resume",
        campaign: campaign({ id: 3, platform: "meta" }),
      }),
    ];
    expect(submitBlocker(lines, (c) => (c === "google_search" ? "Google Search" : c))).toBe(
      "Line 1 (Google Search) still needs a pick.",
    );
  });

  // Catches: a submission of nothing, when no line can be deployed.
  it("blocks a version with nothing Adx can launch", () => {
    expect(submitBlocker([line({ kind: "unsupported", budget: null })])).toBe(
      "Nothing in this version can be launched by Adx.",
    );
    expect(submitBlocker([])).toBe("The version has no lines.");
  });

  it("lets a resolved version through", () => {
    expect(
      submitBlocker([line({}), line({ index: 1, kind: "unsupported", budget: null })]),
    ).toBeNull();
  });
});

describe("accountsByPlatform", () => {
  // Catches: asking a person for an account the matched campaign already names.
  it("takes the account from the line's campaign or candidates", () => {
    const lines = [
      line({ kind: "pick", budget: null, candidates: [campaign({}), campaign({ id: 2 })] }),
      line({
        index: 1,
        platform: "meta",
        kind: "resume",
        campaign: campaign({ id: 3, platform: "meta", accountId: 3 }),
      }),
    ];
    expect(accountsByPlatform(lines, [])).toEqual({ google_ads: 2, meta: 3 });
  });

  // Catches: Adx guessing between two connected accounts for a create.
  it("uses the only connected account for a create, and none when there are two", () => {
    const create = [line({})];
    expect(accountsByPlatform(create, [{ id: 7, provider: "google_ads" }])).toEqual({
      google_ads: 7,
    });
    const two = accountsByPlatform(create, [
      { id: 7, provider: "google_ads" },
      { id: 8, provider: "google_ads" },
    ]);
    expect(two).toEqual({});
    expect(platformsWithoutAccount(create, two)).toEqual(["google_ads"]);
  });

  // Catches: an unsupported line demanding an account it will never use.
  it("ignores unsupported lines", () => {
    const lines = [line({ kind: "unsupported", budget: null, platform: "meta" })];
    expect(platformsWithoutAccount(lines, accountsByPlatform(lines, []))).toEqual([]);
  });
});

describe("plainNote", () => {
  // Catches: a bare policy code shown as the whole reason for a denial.
  it("spells a policy denial out and keeps the code", () => {
    expect(plainNote("policy: campaign_not_under_a_plan")).toBe(
      "Denied because the campaign is not under an accepted plan whose period covers today (campaign_not_under_a_plan).",
    );
    expect(plainNote("policy: something_new")).toBe("Denied by policy: something_new.");
  });

  it("leaves a sentence alone", () => {
    expect(plainNote("waits for an admin in Perform")).toBe("waits for an admin in Perform");
  });
});

describe("pickCaption", () => {
  const rel = () => "just now";
  // Catches: a user id shown as a name, or another person's pick credited to you.
  it("says you only for your own pick, and never shows an id", () => {
    expect(pickCaption({ pickedBy: 2, pickedAt: "2026-10-21T13:00:00Z" }, 2, rel)).toBe(
      "Picked by you · just now",
    );
    expect(pickCaption({ pickedBy: 2, pickedAt: "2026-10-21T13:00:00Z" }, 1, rel)).toBe(
      "Picked · just now",
    );
    expect(pickCaption({ pickedBy: null, pickedAt: "2026-10-21T13:00:00Z" }, null, rel)).toBe(
      "Picked · just now",
    );
  });
});
