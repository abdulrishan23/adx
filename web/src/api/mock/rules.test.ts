import { afterEach, describe, expect, it, vi } from "vitest";
import { dailyBudget, proposeChange } from "./actions";
import { connectPrivate, listCrm, listLeads, startLeadSync } from "./crm";
import { CAMPAIGNS } from "./data";
import { countCandidates, startJob } from "./enrich";
import { ads, guardrails, updateGuardrails } from "./ops";
import { decide, proposals, retry, undo } from "./server";

// These rules belong to the server. The mocks enforce them so the design branch
// behaves like the real thing; each test names the wrong behaviour it catches.

const meta = CAMPAIGNS.find((c) => c.platform === "meta" && c.status === "active");
if (!meta) throw new Error("fixtures need an active Meta campaign");

describe("proposing a change", () => {
  afterEach(() => {
    const g = guardrails();
    g.changesToday = 0;
    for (const a of g.accounts) a.killed = false;
  });

  it("refuses a raise over the daily cap, so approval can't get past a guardrail", () => {
    const now = dailyBudget(meta.id);
    expect(() =>
      proposeChange({ kind: "daily_budget", campaignId: meta.id, dailyBudgetMinor: now * 2 }),
    ).toThrow(/cap is 20%/);
  });

  it("sends a raise inside the cap for approval instead of applying it", () => {
    const now = dailyBudget(meta.id);
    const p = proposeChange({
      kind: "daily_budget",
      campaignId: meta.id,
      dailyBudgetMinor: Math.round(now * 1.1),
    });
    expect(p.status).toBe("pending");
    expect(p.needsApproval).toBe(true);
    expect(dailyBudget(meta.id)).toBe(now);
  });

  it("applies a cut at once and records the new budget", () => {
    const now = dailyBudget(meta.id);
    const next = Math.round(now * 0.8);
    const p = proposeChange({ kind: "daily_budget", campaignId: meta.id, dailyBudgetMinor: next });
    expect(p.status).toBe("applied");
    expect(dailyBudget(meta.id)).toBe(next);
  });

  it("pauses an ad and won't pause it twice", () => {
    const ad = ads(meta.id).find((a) => a.status === "active");
    if (!ad) throw new Error("fixtures need an active ad");
    proposeChange({ kind: "pause_ad", campaignId: meta.id, adId: ad.id });
    expect(ads(meta.id).find((a) => a.id === ad.id)?.status).toBe("paused");
    expect(() => proposeChange({ kind: "pause_ad", campaignId: meta.id, adId: ad.id })).toThrow(
      /isn't running/,
    );
  });

  it("blocks every change while the account's kill switch is on", () => {
    updateGuardrails({ account: { key: "meta", killed: true } });
    expect(() =>
      proposeChange({ kind: "daily_budget", campaignId: meta.id, dailyBudgetMinor: 100 }),
    ).toThrow(/kill switch/);
  });

  it("stops at the daily change limit", () => {
    guardrails().changesToday = guardrails().changesPerDay;
    expect(() =>
      proposeChange({ kind: "daily_budget", campaignId: meta.id, dailyBudgetMinor: 100 }),
    ).toThrow(/change limit/);
  });
});

describe("guardrail settings", () => {
  it("rejects a cap outside 0–100%, which would make the cap meaningless", () => {
    expect(() => updateGuardrails({ dailyIncreaseCapPct: 150 })).toThrow();
    expect(() => updateGuardrails({ changesPerDay: 0 })).toThrow();
  });
});

describe("proposal lifecycle", () => {
  afterEach(() => vi.useRealTimers());

  it("applies an approved change after a delay, and a failure can be retried", () => {
    vi.useFakeTimers();
    const p = decide("p3", "approve");
    expect(p.status).toBe("applying");
    vi.advanceTimersByTime(5000);
    expect(proposals().find((x) => x.id === "p3")?.status).toBe("failed");
    retry("p3");
    vi.advanceTimersByTime(5000);
    expect(proposals().find((x) => x.id === "p3")?.status).toBe("applied");
  });

  it("only undoes applied changes, so a pending one can't be reverted before it runs", () => {
    const pending = proposals().find((x) => x.status === "pending");
    if (pending) expect(() => undo(pending.id)).toThrow(/Only an applied/);
    expect(undo("p3").status).toBe("undone");
  });

  it("won't decide a proposal twice", () => {
    expect(() => decide("p3", "approve")).toThrow(/already decided/);
  });
});

describe("GoHighLevel connect", () => {
  it("rejects workspace IDs the server would reject", () => {
    expect(() => connectPrivate("bad id!", "pit-abc")).toThrow(/workspace ID/);
    expect(() => connectPrivate("abc", "pit with space")).toThrow(/token/);
  });

  it("replaces a reconnected workspace instead of adding a second copy", () => {
    const before = listCrm().length;
    const id = listCrm()[0]?.locationId ?? "x";
    connectPrivate(id, "pit-new-token");
    expect(listCrm()).toHaveLength(before);
  });
});

describe("enrichment batches", () => {
  const filter = {
    temperature: "any",
    noStatus: false,
    projectId: "all",
    olderThanDays: 0,
    skipEnriched: false,
  } as const;

  it("refuses a batch bigger than what's left this month", () => {
    expect(() => startJob(filter, 1_000_000, "too many")).toThrow(/lookups left/);
  });

  it("refuses an empty batch", () => {
    expect(() => startJob({ ...filter, projectId: 999 }, 10, "nobody")).toThrow(/No leads match/);
    expect(countCandidates({ ...filter, projectId: 999 })).toBe(0);
  });
});

describe("lead copy", () => {
  it("refuses a second sync while one runs, then finishes", () => {
    vi.useFakeTimers();
    try {
      const id = "rPq7xK2mV9aLtB4wZ8nD";
      expect(startLeadSync(id).state).toBe("running");
      expect(() => startLeadSync(id)).toThrow(/already running/);
      vi.advanceTimersByTime(3000);
      expect(listLeads(id).lastSync?.state).toBe("done");
    } finally {
      vi.useRealTimers();
    }
  });

  it("refuses a workspace that isn't connected", () => {
    expect(() => startLeadSync("nope")).toThrow(/not connected/);
  });
});
