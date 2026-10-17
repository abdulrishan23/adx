import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON, putJSON } from "./client";
import type {
  DeriveResponse,
  ScoreView,
  ScoringDocument,
  ScoringState,
  ScoringVersion,
  SignalCorrection,
  SignalEventsResponse,
} from "./types.gen";

export function useScoringRules() {
  return useQuery({
    queryKey: ["signals", "rules"],
    queryFn: () => getJSON<ScoringState>("/api/signals/rules"),
  });
}

export function usePutScoringRules() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (doc: ScoringDocument) => putJSON<ScoringVersion>("/api/signals/rules", doc),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["signals"] }),
  });
}

export function usePersonScore(id: number) {
  return useQuery({
    queryKey: ["signals", "person", id],
    queryFn: () => getJSON<ScoreView>(`/api/signals/people/${id}`),
  });
}

export function useUnknownEvents() {
  return useQuery({
    queryKey: ["signals", "unknown"],
    queryFn: () => getJSON<SignalEventsResponse>("/api/signals/events/unknown?limit=100"),
  });
}

export function useCorrectEvent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      action,
      personId,
      reason,
    }: {
      id: number;
      action: string;
      personId: number;
      reason: string;
    }) =>
      postJSON<SignalCorrection>(`/api/signals/events/${id}/correct`, { action, personId, reason }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["signals"] }),
  });
}

export function useDeriveSignals() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => postJSON<DeriveResponse>("/api/signals/derive", {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["signals"] }),
  });
}
