import { describe, expect, it } from "vitest";
import { formatMoney, NOT_AVAILABLE } from "./format";

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
