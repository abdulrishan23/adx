import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type {
  GenerateInput,
  Generation,
  PlanDetail,
  PlanDiff,
  PlanDocument,
  PlanInput,
  PlansResponse,
  PlanValidation,
  PlanVersion,
  VersionInput,
} from "./types.gen";

export function usePlans() {
  return useQuery({ queryKey: ["plans"], queryFn: () => getJSON<PlansResponse>("/api/plans") });
}

export function usePlan(id: number | null) {
  return useQuery({
    queryKey: ["plans", id],
    queryFn: () => getJSON<PlanDetail>(`/api/plans/${id}`),
    enabled: id !== null,
  });
}

export function useCreatePlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: PlanInput) => postJSON<PlanDetail>("/api/plans", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["plans"] }),
  });
}

export function useSavePlanVersion(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: VersionInput) => postJSON<PlanVersion>(`/api/plans/${id}/versions`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["plans"] }),
  });
}

export function useAcceptPlanVersion(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (version: number) =>
      postJSON<PlanVersion>(`/api/plans/${id}/versions/${version}/accept`, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["plans"] }),
  });
}

export function useValidatePlan() {
  return useMutation({
    mutationFn: (input: { brandId: number; document: PlanDocument }) =>
      postJSON<PlanValidation>("/api/plans/validate", input),
  });
}

export function useImportPlan(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ format, file }: { format: string; file: File }) => {
      const res = await fetch(`/api/plans/${id}/import?format=${format}`, {
        method: "POST",
        body: await file.text(),
      });
      if (!res.ok) {
        const err = (await res.json().catch(() => null)) as { error?: string } | null;
        throw new Error(err?.error ?? res.statusText);
      }
      return (await res.json()) as PlanVersion;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["plans"] }),
  });
}

export function useGeneratePlan(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: GenerateInput) => postJSON<Generation>(`/api/plans/${id}/generate`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["plans"] }),
  });
}

export function usePlanDiff(id: number | null, from: number | null, to: number | null) {
  return useQuery({
    queryKey: ["plans", id, "diff", from, to],
    queryFn: () => getJSON<PlanDiff>(`/api/plans/${id}/diff?from=${from}&to=${to}`),
    enabled: id !== null && from !== null && to !== null && from !== to,
  });
}
