// What the stage table works out from the funnel settings (core/leads). The
// engine owns the rules: which raw status means which stage, that matching
// ignores case and spacing, and that an unmapped status leaves the stage
// null. This file mirrors the key rule so the editor's rows line up with
// the engine's, and turns the state into rows.
import type { FunnelDocument, FunnelState, Stage } from "@/api/types.gen";

// Stages reference.md counts as qualified (core/leads.QualifiedStages).
export const QUALIFIED: ReadonlySet<Stage> = new Set<Stage>(["engaged", "meeting", "offer", "won"]);

export const countsAs = (stage: Stage | null) =>
  stage === null ? "" : QUALIFIED.has(stage) ? "Qualified" : "Not qualified";

// The engine's key for a raw status: trimmed, lower case, one space between
// words, so "Follow  Up" and "follow up" are one row.
export const normalizeKey = (raw: string) => raw.trim().toLowerCase().split(/\s+/).join(" ");

export function emptyDocument(): FunnelDocument {
  return {
    statusField: { kind: "", ref: "" },
    projectField: { kind: "", ref: "" },
    statuses: {},
    blankStatusStage: null,
    sources: {},
    note: "",
  };
}

export type StatusRow = {
  key: string;
  // How GoHighLevel spells it, from the data when seen there.
  label: string;
  leads: number;
  // Seen in the data: false for a status the table names that no lead has.
  observed: boolean;
  // The stage leads with this status have now, from the saved version.
  current: Stage | null;
  // The stage the draft gives it.
  draft: Stage | null;
};

// One row per raw status seen in the data or named in the draft, most
// leads first, the blank status left out (it has its own row).
export function statusRows(state: FunnelState, draft: FunnelDocument): StatusRow[] {
  const rows = new Map<string, StatusRow>();
  for (const o of state.observedStatuses) {
    if (o.raw === null || o.raw.trim() === "") continue;
    const key = normalizeKey(o.raw);
    const row = rows.get(key);
    if (row) {
      row.leads += o.count;
      row.current = row.current ?? o.stage;
    } else {
      rows.set(key, {
        key,
        label: o.raw,
        leads: o.count,
        observed: true,
        current: o.stage,
        draft: null,
      });
    }
  }
  for (const [raw, stage] of Object.entries(draft.statuses ?? {})) {
    const key = normalizeKey(raw);
    const row = rows.get(key);
    if (row) row.draft = stage;
    else rows.set(key, { key, label: raw, leads: 0, observed: false, current: null, draft: stage });
  }
  return [...rows.values()].sort((a, b) => b.leads - a.leads || a.label.localeCompare(b.label));
}

export const blankCount = (state: FunnelState) =>
  state.observedStatuses
    .filter((o) => o.raw === null || o.raw.trim() === "")
    .reduce((n, o) => n + o.count, 0);

// Statuses seen in the data that the draft gives no stage.
export function unmappedIn(state: FunnelState, draft: FunnelDocument): string[] {
  return statusRows(state, draft)
    .filter((r) => r.observed && r.draft === null)
    .map((r) => r.label);
}

export function withStage(doc: FunnelDocument, raw: string, stage: Stage | null): FunnelDocument {
  const statuses = { ...(doc.statuses ?? {}) };
  const key = normalizeKey(raw);
  for (const k of Object.keys(statuses)) if (normalizeKey(k) === key) delete statuses[k];
  if (stage !== null) statuses[key] = stage;
  return { ...doc, statuses };
}

export const draftOf = (state: FunnelState): FunnelDocument =>
  state.current ? structuredClone(state.current.document) : emptyDocument();
