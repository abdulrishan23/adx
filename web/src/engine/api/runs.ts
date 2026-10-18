import { useQuery } from "@tanstack/react-query";
import type { Run, RunsResponse } from "@/api/types.gen";
import { getJSON } from "./client";

export function useRuns() {
  return useQuery({
    queryKey: ["runs"],
    queryFn: () => getJSON<RunsResponse>("/api/runs"),
    refetchInterval: 10_000,
  });
}

export function useRun(id: string) {
  return useQuery({
    queryKey: ["runs", id],
    queryFn: () => getJSON<Run>(`/api/runs/${encodeURIComponent(id)}`),
    enabled: id !== "",
    refetchInterval: (query) => {
      const state = query.state.data?.state;
      return state === "queued" || state === "running" ? 2_000 : false;
    },
  });
}
