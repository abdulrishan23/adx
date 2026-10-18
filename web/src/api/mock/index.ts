// Which data the app reads, set with VITE_MOCK:
//   (unset)  Endpoints the Go server already has are real; the rest are mocked.
//   all      Everything is mocked, for design review without the server.
//   none     Everything is real. Screens without an endpoint yet will fail.
// VITE_USE_MOCK=0 still means none.
type MockMode = "partial" | "all" | "none";

const raw =
  import.meta.env?.VITE_MOCK ?? (import.meta.env?.VITE_USE_MOCK === "0" ? "none" : undefined);
export const MOCK_MODE: MockMode = raw === "all" || raw === "none" ? raw : "partial";

// Features the Go server serves today. Add one here when its endpoints land.
export type Feature = "projects" | "crm";
// Only features whose endpoints the engine server serves in the shape our
// screens expect. GoHighLevel leads and Creative Studio come back here as
// each is rewired to the engine's API (see docs/build/decisions.md).
const LIVE: ReadonlySet<Feature> = new Set<Feature>(["projects"]);

export const isLive = (f: Feature) =>
  MOCK_MODE === "none" || (MOCK_MODE === "partial" && LIVE.has(f));

// Everything not listed above.
export const USE_MOCK = MOCK_MODE !== "none";

// Answers like a network call would: asynchronously, after a short delay, and
// with a copy, so screens can't mutate the fixtures by accident.
export async function mockResponse<T>(answer: () => T, delayMs = 250): Promise<T> {
  await new Promise((resolve) => setTimeout(resolve, delayMs));
  return structuredClone(answer());
}
