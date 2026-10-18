import { describe, expect, it } from "vitest";
import { NAV } from "@/app/nav";
import { phaseFrom, released } from "./release";

describe("release phase", () => {
  // A typo in VITE_PHASE must not ship every unfinished screen.
  it("falls back to phase 1 for a missing or invalid value", () => {
    expect(phaseFrom(undefined)).toBe(1);
    expect(phaseFrom("seven")).toBe(1);
    expect(phaseFrom("9")).toBe(1);
    expect(phaseFrom("6")).toBe(6);
  });

  it("releases a phase and everything before it", () => {
    expect(released(0, 1)).toBe(true);
    expect(released(1, 1)).toBe(true);
    expect(released(3, 1)).toBe(false);
  });

  // The December release must not show screens that write to outside systems.
  it("keeps write screens out of the phase 1 navigation", () => {
    const shipped = NAV.flatMap((g) => g.items).filter((i) => released(i.phase, 1));
    expect(shipped.map((i) => i.to)).not.toContain("/proposals");
    expect(shipped.map((i) => i.to)).not.toContain("/audiences");
  });
});
