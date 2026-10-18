import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  AssignProjectInput,
  Brand,
  BrandDetail,
  BrandInput,
  BrandsResponse,
  ClaimSet,
  Conflict,
  Fact,
  FactChange,
  FactInput,
  FactsResponse,
  Offer,
  OfferChange,
  OfferHistoryResponse,
  OfferInput,
  OfferState,
} from "@/api/types.gen";
import { getJSON, postJSON } from "./client";

export function useBrands() {
  return useQuery({ queryKey: ["brands"], queryFn: () => getJSON<BrandsResponse>("/api/brands") });
}

export function useBrand(id: number | null) {
  return useQuery({
    queryKey: ["brands", id],
    queryFn: () => getJSON<BrandDetail>(`/api/brands/${id}`),
    enabled: id !== null,
  });
}

export function useClaims(brandId: number | null, projectId: number) {
  return useQuery({
    queryKey: ["claims", brandId, projectId],
    queryFn: () => getJSON<ClaimSet>(`/api/brands/${brandId}/claims?project_id=${projectId}`),
    enabled: brandId !== null,
  });
}

export function useFactHistory(brandId: number | null, key: string | null) {
  return useQuery({
    queryKey: ["facts", brandId, key],
    queryFn: () =>
      getJSON<FactsResponse>(`/api/brands/${brandId}/facts?key=${encodeURIComponent(key ?? "")}`),
    enabled: brandId !== null && key !== null,
  });
}

export function useOffer(id: number | null) {
  return useQuery({
    queryKey: ["offers", id],
    queryFn: () => getJSON<OfferHistoryResponse>(`/api/offers/${id}`),
    enabled: id !== null,
  });
}

function invalidateBrands(qc: ReturnType<typeof useQueryClient>) {
  return () => {
    qc.invalidateQueries({ queryKey: ["brands"] });
    qc.invalidateQueries({ queryKey: ["claims"] });
    qc.invalidateQueries({ queryKey: ["facts"] });
    qc.invalidateQueries({ queryKey: ["offers"] });
  };
}

export function useCreateBrand() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: BrandInput) => postJSON<Brand>("/api/brands", input),
    onSuccess: invalidateBrands(qc),
  });
}

export function useAssignProject() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (input: AssignProjectInput) => {
      const res = await fetch("/api/brands/projects", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      });
      if (!res.ok) {
        const err = (await res.json().catch(() => null)) as { error?: string } | null;
        throw new Error(err?.error ?? res.statusText);
      }
    },
    onSuccess: invalidateBrands(qc),
  });
}

export function useAddFact(brandId: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: FactInput) => postJSON<FactChange>(`/api/brands/${brandId}/facts`, input),
    onSuccess: invalidateBrands(qc),
  });
}

export function useFactAction() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      action,
      reason,
    }: {
      id: number;
      action: "approve" | "retire";
      reason?: string;
    }) => postJSON<Fact>(`/api/facts/${id}/${action}`, action === "retire" ? { reason } : {}),
    onSuccess: invalidateBrands(qc),
  });
}

export function useResolveConflict() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      winnerId,
      resolution,
    }: {
      id: number;
      winnerId: number;
      resolution: string;
    }) => postJSON<Conflict>(`/api/fact-conflicts/${id}/resolve`, { winnerId, resolution }),
    onSuccess: invalidateBrands(qc),
  });
}

export function useCreateOffer(brandId: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: OfferInput) => postJSON<Offer>(`/api/brands/${brandId}/offers`, input),
    onSuccess: invalidateBrands(qc),
  });
}

export function useReviseOffer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: OfferInput }) =>
      postJSON<OfferChange>(`/api/offers/${id}/versions`, input),
    onSuccess: invalidateBrands(qc),
  });
}

export function useSetOfferState() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, state }: { id: number; state: OfferState }) =>
      postJSON<Offer>(`/api/offers/${id}/state`, { state }),
    onSuccess: invalidateBrands(qc),
  });
}
