import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  CapabilitiesResponse,
  Intent,
  IntentWithEvents,
  LaunchStatus,
  Milestones,
  Proposal,
  Reversibility,
} from "@/api/types.gen";
import { getJSON, postJSON } from "./client";

export function useExecutionCapabilities() {
  return useQuery({
    queryKey: ["executions", "capabilities"],
    queryFn: () => getJSON<CapabilitiesResponse>("/api/executions/capabilities"),
  });
}

export function useLaunchStatus(planId: number | null, version: number | null) {
  return useQuery({
    queryKey: ["executions", "launch", planId, version],
    queryFn: () =>
      getJSON<LaunchStatus>(`/api/executions/launch?plan_id=${planId}&version=${version}`),
    enabled: planId !== null && version !== null,
    refetchInterval: 15000,
  });
}

export function useExecutionTimeline(id: number | null) {
  return useQuery({
    queryKey: ["executions", "timeline", id],
    queryFn: () => getJSON<IntentWithEvents>(`/api/executions/${id}/timeline`),
    enabled: id !== null,
  });
}

export function useExecutionAction() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, action }: { id: number; action: "retry" | "reconcile" }) =>
      postJSON<Intent>(`/api/executions/${id}/${action}`, {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["executions"] });
      qc.invalidateQueries({ queryKey: ["proposals"] });
      qc.invalidateQueries({ queryKey: ["envelopes"] });
    },
  });
}

export function useMilestones(planId: number | null, version: number | null) {
  return useQuery({
    queryKey: ["executions", "milestones", planId, version],
    queryFn: () =>
      getJSON<Milestones>(`/api/executions/milestones?plan_id=${planId}&version=${version}`),
    enabled: planId !== null && version !== null,
    refetchInterval: 30000,
  });
}

export function useReversibility(id: number | null) {
  return useQuery({
    queryKey: ["executions", "reversibility", id],
    queryFn: () => getJSON<Reversibility>(`/api/executions/${id}/reversibility`),
    enabled: id !== null,
  });
}

export function useCompensate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) =>
      postJSON<Intent>(`/api/executions/${id}/compensate`, { reason }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["executions"] });
      qc.invalidateQueries({ queryKey: ["proposals"] });
    },
  });
}

export function useRepair() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, proposed, note }: { id: number; proposed: unknown; note: string }) =>
      postJSON<Proposal>(`/api/executions/${id}/repair`, { proposed, note }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["executions"] });
      qc.invalidateQueries({ queryKey: ["proposals"] });
    },
  });
}
