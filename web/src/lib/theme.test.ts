import { describe, expect, it } from "vitest";
import { nextPreference, parsePreference, resolveTheme } from "./theme";

describe("parsePreference", () => {
  it("falls back to system for missing or unknown values", () => {
    expect(parsePreference(null)).toBe("system");
    expect(parsePreference("sepia")).toBe("system");
  });

  it("keeps a known preference", () => {
    expect(parsePreference("dark")).toBe("dark");
  });
});

describe("resolveTheme", () => {
  it("follows the system only when asked to", () => {
    expect(resolveTheme("system", true)).toBe("dark");
    expect(resolveTheme("system", false)).toBe("light");
    expect(resolveTheme("light", true)).toBe("light");
  });
});

describe("nextPreference", () => {
  it("cycles through every preference and wraps around", () => {
    expect(nextPreference("system")).toBe("light");
    expect(nextPreference("light")).toBe("dark");
    expect(nextPreference("dark")).toBe("system");
  });
});
