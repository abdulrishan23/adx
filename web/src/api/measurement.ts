import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type {
  ChecksResponse,
  MeasurementCatalog,
  MeasurementCheck,
  MeasurementCheckInput,
  MeasurementEventsResponse,
  PlanLaunchReadiness,
} from "./types.gen";

export function useMeasurementCatalog() {
  return useQuery({
    queryKey: ["measurement", "catalog"],
    queryFn: () => getJSON<MeasurementCatalog>("/api/measurement/catalog"),
  });
}

export function useLaunchReadiness(planId: number | null, version: number | null) {
  return useQuery({
    queryKey: ["measurement", "readiness", planId, version],
    queryFn: () =>
      getJSON<PlanLaunchReadiness>(
        `/api/measurement/readiness?plan_id=${planId}&version=${version}`,
      ),
    enabled: planId !== null && version !== null,
  });
}

export function useMeasurementChecks(planId: number | null, version: number | null) {
  return useQuery({
    queryKey: ["measurement", "checks", planId, version],
    queryFn: () =>
      getJSON<ChecksResponse>(`/api/measurement/checks?plan_id=${planId}&version=${version}`),
    enabled: planId !== null && version !== null,
  });
}

export function useMeasurementEvents(pageId: number) {
  return useQuery({
    queryKey: ["measurement", "events", pageId],
    queryFn: () =>
      getJSON<MeasurementEventsResponse>(`/api/measurement/events?page_id=${pageId}&limit=50`),
  });
}

export function useRunCheck() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: MeasurementCheckInput) =>
      postJSON<MeasurementCheck>("/api/measurement/checks", input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["measurement"] });
      qc.invalidateQueries({ queryKey: ["pages"] });
    },
  });
}
