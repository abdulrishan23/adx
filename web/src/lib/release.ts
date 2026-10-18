// The December release ships build phases 0 and 1 (docs/screens.md). Screens
// from later phases stay in the code but are hidden and unreachable until
// their phase ships. VITE_PHASE=6 shows everything that exists.
export type Phase = 0 | 1 | 2 | 3 | 4 | 5 | 6;

export const RELEASE_PHASE = phaseFrom(import.meta.env.VITE_PHASE);

export function phaseFrom(raw: unknown): Phase {
  const n = Number(raw);
  return Number.isInteger(n) && n >= 0 && n <= 6 ? (n as Phase) : 1;
}

export function released(phase: Phase, release: Phase = RELEASE_PHASE) {
  return phase <= release;
}
