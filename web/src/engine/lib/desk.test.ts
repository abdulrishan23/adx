import { describe, expect, it } from "vitest";
import { dayAgo, greeting, summaryLine } from "./desk";

// Catches: a greeting that uses the full name or an empty comma, a summary
// that says "1 things", shows a missing count as 0, or hides a failed check.
describe("greeting", () => {
  it("uses the time of day and the first name", () => {
    expect(greeting(8, "Hisham Al Mansoori")).toBe("Good morning, Hisham");
    expect(greeting(13, "Sam")).toBe("Good afternoon, Sam");
    expect(greeting(20, null)).toBe("Good evening");
    expect(greeting(20, "  ")).toBe("Good evening");
  });
});

describe("summaryLine", () => {
  it("counts what needs attention and says the server state", () => {
    expect(summaryLine({ waiting: 1, openAlerts: 0, jobs: 0, server: "ready" })).toBe(
      "1 thing needs your attention. No jobs under way. Server ready.",
    );
    expect(summaryLine({ waiting: 3, openAlerts: 2, jobs: 4, server: "ready" })).toBe(
      "5 things need your attention. 4 jobs under way. Server ready.",
    );
    expect(summaryLine({ waiting: 0, openAlerts: 0, jobs: 1, server: "ready" })).toBe(
      "Nothing needs your attention. 1 job under way. Server ready.",
    );
  });
  it("never shows a missing count as zero", () => {
    expect(summaryLine({ waiting: null, openAlerts: null, jobs: null, server: "checking" })).toBe(
      "Attention count not available. Jobs not available. Checking the server.",
    );
    expect(summaryLine({ waiting: null, openAlerts: 2, jobs: 0, server: "ready" })).toContain(
      "2 things need",
    );
  });
  it("names the failed checks", () => {
    expect(
      summaryLine({
        waiting: 0,
        openAlerts: 0,
        jobs: 0,
        server: "not ready",
        failedChecks: ["database"],
      }),
    ).toBe("Nothing needs your attention. No jobs under way. Server not ready: database.");
    expect(summaryLine({ waiting: 0, openAlerts: 0, jobs: 0, server: "unreachable" })).toContain(
      "Server unreachable.",
    );
  });
});

describe("dayAgo", () => {
  it("counts back in UTC days", () => {
    expect(dayAgo(1, new Date("2026-10-18T03:00:00Z"))).toBe("2026-10-17");
    expect(dayAgo(30, new Date("2026-10-18T03:00:00Z"))).toBe("2026-09-18");
  });
});
