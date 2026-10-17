import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, getJSON, postJSON } from "./client";
import type { CostReport, LoadResponse, ModelRelease, Readiness, ReleaseReport } from "./types.gen";

// /api/ready answers 503 when a check fails, with the same body. The query
// treats both as data so the footer can show which check failed.
async function fetchReadiness(): Promise<Readiness> {
  const res = await fetch("/api/ready", { headers: { Accept: "application/json" } });
  if (res.status !== 200 && res.status !== 503) {
    throw new ApiError(res.status, res.statusText);
  }
  return (await res.json()) as Readiness;
}

export function useReadiness() {
  return useQuery({
    queryKey: ["ready"],
    queryFn: fetchReadiness,
    refetchInterval: 60_000,
    retry: false,
  });
}

export function useCost() {
  return useQuery({
    queryKey: ["ops", "cost"],
    queryFn: () => getJSON<CostReport>("/api/ops/cost"),
  });
}

export function useLoad() {
  return useQuery({
    queryKey: ["ops", "load"],
    queryFn: () => getJSON<LoadResponse>("/api/ops/load"),
  });
}

export function useReleases() {
  return useQuery({
    queryKey: ["ops", "releases"],
    queryFn: () => getJSON<ReleaseReport>("/api/ops/releases"),
  });
}

export function useEvaluateRelease() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (model: string) => postJSON<ModelRelease>("/api/ops/releases/evaluate", { model }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ops"] }),
  });
}

export function useDecideRelease() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ model, decision }: { model: string; decision: "promote" | "reject" }) =>
      postJSON<ModelRelease>(`/api/ops/releases/${encodeURIComponent(model)}/${decision}`, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ops"] }),
  });
}
