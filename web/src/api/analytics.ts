import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type { FreshnessResponse, RefreshInput, StartRunResponse } from "./types.gen";

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
