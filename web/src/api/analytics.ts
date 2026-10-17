import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type {
  AssignProjectsResponse,
  FreshnessResponse,
  RefreshInput,
  ReportingResult,
  StartRunResponse,
} from "./types.gen";

export function useFreshness() {
  return useQuery({
    queryKey: ["analytics", "freshness"],
    queryFn: () => getJSON<FreshnessResponse>("/api/analytics/freshness"),
    refetchInterval: 30_000,
  });
}

export function useRefreshAnalytics() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: RefreshInput) =>
      postJSON<StartRunResponse>("/api/analytics/refresh", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["runs"] });
      queryClient.invalidateQueries({ queryKey: ["analytics"] });
    },
  });
}

export type AnalyticsParams = {
  from: string;
  to: string;
  group: "campaign" | "project" | "platform" | "day";
  compare: "" | "previous" | "year";
  platform: "" | "google_ads" | "meta";
};

export function useAnalytics(p: AnalyticsParams) {
  const search = new URLSearchParams({ from: p.from, to: p.to, group: p.group });
  if (p.compare) search.set("compare", p.compare);
  if (p.platform) search.set("platform", p.platform);
  return useQuery({
    queryKey: ["analytics", p],
    queryFn: () => getJSON<ReportingResult>(`/api/analytics?${search.toString()}`),
  });
}

export function useAssignProjects() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => postJSON<AssignProjectsResponse>("/api/analytics/assign-projects", {}),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["analytics"] }),
  });
}
