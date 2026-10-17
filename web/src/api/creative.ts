import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type {
  Asset,
  AssetsResponse,
  BrandKitResponse,
  CreativeInput,
  CreativeResponse,
  CreativesResponse,
  Document,
  ProjectOffersResponse,
  Version,
} from "./types.gen";

export function useAssets(kind?: string) {
  return useQuery({
    queryKey: ["assets", kind],
    queryFn: () => getJSON<AssetsResponse>(`/api/assets${kind ? `?kind=${kind}` : ""}`),
  });
}

export function useUploadAsset() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({
      file,
      kind,
      projectId,
    }: {
      file: File;
      kind: string;
      projectId: number;
    }) => {
      const form = new FormData();
      form.append("file", file);
      form.append("kind", kind);
      form.append("project_id", String(projectId));
      const res = await fetch("/api/assets", { method: "POST", body: form });
      if (!res.ok) {
        const err = (await res.json().catch(() => null)) as { error?: string } | null;
        throw new Error(err?.error ?? res.statusText);
      }
      return (await res.json()) as Asset;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["assets"] }),
  });
}

export function useProjectOffers(projectId: number) {
  return useQuery({
    queryKey: ["project-offers", projectId],
    queryFn: () => getJSON<ProjectOffersResponse>(`/api/projects/${projectId}/offers`),
    enabled: projectId > 0,
  });
}

export function useCreatives() {
  return useQuery({
    queryKey: ["creatives"],
    queryFn: () => getJSON<CreativesResponse>("/api/creatives"),
  });
}

export function useCreative(id: number | null) {
  return useQuery({
    queryKey: ["creatives", id],
    queryFn: () => getJSON<CreativeResponse>(`/api/creatives/${id}`),
    enabled: id !== null,
  });
}

export function useBrandKit(projectId: number) {
  return useQuery({
    queryKey: ["brand", projectId],
    queryFn: () => getJSON<BrandKitResponse>(`/api/projects/${projectId}/brand-kit`),
    enabled: projectId > 0,
  });
}

export function useCreateCreative() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreativeInput) => postJSON<CreativeResponse>("/api/creatives", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["creatives"] }),
  });
}

export function useCreativeAction(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ action, body }: { action: "render" | "submit" | "review"; body?: unknown }) =>
      postJSON<Version>(`/api/creatives/${id}/${action}`, body ?? {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["creatives"] }),
  });
}

export function useSaveDocument(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (document: Document) => {
      const res = await fetch(`/api/creatives/${id}/document`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ document }),
      });
      if (!res.ok) {
        const err = (await res.json().catch(() => null)) as { error?: string } | null;
        throw new Error(err?.error ?? res.statusText);
      }
      return (await res.json()) as Version;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["creatives"] }),
  });
}
