import { describe, expect, it } from "vitest";
import { router } from "@/router";
import { doorOf, HOME, NAV, navFor } from "./nav";

const items = [HOME, ...NAV.flatMap((g) => g.items)];

describe("nav", () => {
  // Catches a sidebar entry whose page was renamed or removed. Types catch
  // most of this, but not a route that exists in the type and not at runtime.
  it("points every entry at a real route", () => {
    const paths = Object.keys(router.routesByPath);
    for (const item of items) {
      expect(paths).toContain(item.to);
    }
  });

  it("opens each door at a live screen under it", () => {
    for (const group of NAV.filter((g) => g.door)) {
      const first = group.items.find((i) => i.to === group.door?.to);
      expect(first?.status).toBe("live");
    }
  });

  // Catches a screen under a door that leaves the sidebar closed, or Settings
  // opening a door by accident.
  it("knows which door a path belongs to", () => {
    expect(doorOf("/engine/creatives")?.label).toBe("Create");
    expect(doorOf("/brands/3")?.label).toBe("Create");
    expect(doorOf("/leads/1001")?.label).toBe("Convert");
    expect(doorOf("/perform/projects")?.label).toBe("Perform");
    expect(doorOf("/settings/people")).toBeUndefined();
    expect(doorOf("/")).toBeUndefined();
  });

  it("has no duplicate entries", () => {
    const paths = items.map((i) => i.to);
    expect(new Set(paths).size).toBe(paths.length);
  });

  it("hides admin-only entries from marketers and viewers", () => {
    const adminOnly = items.filter((i) => i.roles?.includes("admin") && i.roles.length === 1);
    expect(adminOnly.length).toBeGreaterThan(0);
    for (const role of ["marketer", "viewer"] as const) {
      const visible = navFor(role).flatMap((g) => g.items);
      for (const item of adminOnly) {
        expect(visible).not.toContain(item);
      }
    }
  });

  it("drops a group when a role can see none of its entries", () => {
    for (const group of navFor("viewer")) {
      expect(group.items.length).toBeGreaterThan(0);
    }
  });
});
