// The status-to-stage table. Always the Go server: a mocked table would look
// like Reportage's real definitions.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON, putJSON, request } from "./client";
import type { StageMap, StageMapInput, StageOverview } from "./types.legacy";

const KEY = ["stages"] as const;

export function useStageOverview() {
  return useQuery({ queryKey: KEY, queryFn: () => getJSON<StageOverview>("/api/stages") });
}

function useStageMutation<I, O>(fn: (input: I) => Promise<O>) {
  const qc = useQueryClient();
  return useMutation({ mutationFn: fn, onSettled: () => qc.invalidateQueries({ queryKey: KEY }) });
}

export const useStartStageDraft = () =>
  useStageMutation(() => postJSON<StageMap>("/api/stages/draft", {}));

export const useSaveStageDraft = () =>
  useStageMutation((input: StageMapInput) => putJSON<StageMap>("/api/stages/draft", input));

export const useConfirmStageDraft = () =>
  useStageMutation(() => postJSON<StageMap>("/api/stages/draft/confirm", {}));

export const useDiscardStageDraft = () =>
  useStageMutation(() => request<void>("/api/stages/draft", { method: "DELETE" }));
