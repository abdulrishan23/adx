import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type { CapabilitiesResponse, Intent, IntentWithEvents, LaunchStatus } from "./types.gen";

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
