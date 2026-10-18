import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  ActionInput,
  ConflictsResponse,
  InboxResponse,
  RecommendationsResponse,
  SupervisorConfig,
  SupervisorDocument,
  SupervisorState,
} from "@/api/types.gen";
import { getJSON, postJSON, putJSON } from "./client";

function invalidateAll(qc: ReturnType<typeof useQueryClient>) {
  return () => qc.invalidateQueries();
}

export function useInbox() {
  return useQuery({
    queryKey: ["supervisor", "inbox"],
    queryFn: () => getJSON<InboxResponse>("/api/supervisor/inbox"),
    refetchInterval: 15000,
  });
}

export function useSupervisorAct() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ActionInput) => postJSON<unknown>("/api/supervisor/act", input),
    onSuccess: invalidateAll(qc),
  });
}

export function useSupervisorConfig() {
  return useQuery({
    queryKey: ["supervisor", "config"],
    queryFn: () => getJSON<SupervisorState>("/api/supervisor/config"),
  });
}

export function usePutSupervisorConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (doc: SupervisorDocument) =>
      putJSON<SupervisorConfig>("/api/supervisor/config", doc),
    onSuccess: invalidateAll(qc),
  });
}

export function useConflicts() {
  return useQuery({
    queryKey: ["supervisor", "conflicts"],
    queryFn: () => getJSON<ConflictsResponse>("/api/supervisor/conflicts?limit=50"),
  });
}

export function useRecommendations() {
  return useQuery({
    queryKey: ["supervisor", "recommendations"],
    queryFn: () => getJSON<RecommendationsResponse>("/api/supervisor/recommendations?limit=100"),
  });
}

export function useArbitrate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => postJSON<ConflictsResponse>("/api/supervisor/arbitrate", {}),
    onSuccess: invalidateAll(qc),
  });
}

export function useHalt() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { accountId: number; engaged: boolean; reason: string }) =>
      postJSON<unknown>("/api/supervisor/halt", input),
    onSuccess: invalidateAll(qc),
  });
}
