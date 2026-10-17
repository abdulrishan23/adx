import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type {
  BriefDetail,
  BriefInput,
  BriefsResponse,
  Evidence,
  ResolveCompetitorInput,
  StartRunResponse,
} from "./types.gen";

export function useBriefs() {
  return useQuery({
    queryKey: ["research", "briefs"],
    queryFn: () => getJSON<BriefsResponse>("/api/research/briefs"),
  });
}

export function useBrief(id: number | null) {
  return useQuery({
    queryKey: ["research", "briefs", id],
    queryFn: () => getJSON<BriefDetail>(`/api/research/briefs/${id}`),
    enabled: id !== null,
    refetchInterval: (q) => (q.state.data?.items.some((i) => i.state === "pending") ? 3000 : false),
  });
}

export function useResearchEvidence(id: number | null) {
  return useQuery({
    queryKey: ["research", "evidence", id],
    queryFn: () => getJSON<Evidence>(`/api/research/evidence/${id}`),
    enabled: id !== null,
  });
}

export function useCreateBrief() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: BriefInput) => postJSON<BriefDetail>("/api/research/briefs", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["research"] }),
  });
}

export function useStartResearchRun(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => postJSON<StartRunResponse>(`/api/research/briefs/${id}/runs`, {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["research"] });
      qc.invalidateQueries({ queryKey: ["runs"] });
    },
  });
}

export function useResolveCompetitor(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ResolveCompetitorInput) =>
      postJSON<BriefDetail>(`/api/research/briefs/${id}/competitors`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["research"] }),
  });
}
