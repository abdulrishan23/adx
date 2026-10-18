import { describe, expect, it } from "vitest";
import {
  change,
  formatChange,
  formatMoney,
  formatMoneyShort,
  formatPercent,
  formatRelative,
  NOT_AVAILABLE,
} from "./format";

describe("formatMoney", () => {
  it("formats AED from fils", () => {
    expect(formatMoney(1234550, "AED")).toBe("AED 12,345.50");
  });

  it("shows a real zero as zero", () => {
    expect(formatMoney(0, "AED")).toBe("AED 0.00");
  });

  it("shows a missing amount as not available", () => {
    expect(formatMoney(null, "AED")).toBe(NOT_AVAILABLE);
    expect(formatMoney(undefined, "AED")).toBe(NOT_AVAILABLE);
  });

  it("uses the currency's own minor unit", () => {
    expect(formatMoney(1500, "JPY")).toBe("JPY 1,500");
  });
});

describe("formatMoneyShort", () => {
  it("keeps small amounts whole and compacts large ones", () => {
    expect(formatMoneyShort(648_000)).toBe("AED 6,480");
    expect(formatMoneyShort(43_700_000)).toBe("AED 437K");
  });

  it("shows a missing amount as not available", () => {
    expect(formatMoneyShort(null)).toBe(NOT_AVAILABLE);
  });
});

describe("change", () => {
  // A percentage change from nothing isn't a number anyone can act on.
  it("has no change when the earlier figure is missing or zero", () => {
    expect(change(10, null)).toBeNull();
    expect(change(10, 0)).toBeNull();
    expect(formatChange(null)).toBe("No earlier period");
  });

  it("signs the change and treats tiny moves as flat", () => {
    expect(formatChange(change(120, 100))).toBe("+20%");
    expect(formatChange(change(80, 100))).toBe("−20%");
    expect(change(1000, 1001)?.direction).toBe("flat");
  });
});

describe("formatPercent", () => {
  it("shows a missing or infinite ratio as not available", () => {
    expect(formatPercent(null)).toBe(NOT_AVAILABLE);
    expect(formatPercent(Number.POSITIVE_INFINITY)).toBe(NOT_AVAILABLE);
  });
});

describe("formatRelative", () => {
  const now = new Date("2026-10-14T12:00:00Z");
  it("reads past and future times", () => {
    expect(formatRelative("2026-10-14T09:00:00Z", now)).toBe("3h ago");
    expect(formatRelative("2026-10-16T12:00:00Z", now)).toBe("in 2d");
  });
});
