import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type {
  Coverage,
  FunnelDocument,
  FunnelMapping,
  FunnelState,
  ImportLeadsInput,
  LeadsResponse,
  StartRunResponse,
} from "./types.gen";

export function useLeads(params: { stage?: string; status?: string; accountId?: number }) {
  const search = new URLSearchParams();
  if (params.stage) search.set("stage", params.stage);
  if (params.status) search.set("status", params.status);
  if (params.accountId) search.set("account_id", String(params.accountId));
  const qs = search.toString();
  return useQuery({
    queryKey: ["leads", params],
    queryFn: () => getJSON<LeadsResponse>(`/api/leads${qs ? `?${qs}` : ""}`),
  });
}

export function useLeadCoverage() {
  return useQuery({
    queryKey: ["leads", "coverage"],
    queryFn: () => getJSON<Coverage>("/api/leads/coverage"),
  });
}

export function useFunnel() {
  return useQuery({
    queryKey: ["funnel"],
    queryFn: () => getJSON<FunnelState>("/api/settings/funnel"),
  });
}

async function putJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const err = (await res.json().catch(() => null)) as { error?: string } | null;
    throw new Error(err?.error ?? res.statusText);
  }
  return (await res.json()) as T;
}

export function usePutFunnel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (document: FunnelDocument) =>
      putJSON<FunnelMapping>("/api/settings/funnel", { document }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["funnel"] });
      queryClient.invalidateQueries({ queryKey: ["leads"] });
    },
  });
}

export function useImportLeads() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ImportLeadsInput) => postJSON<StartRunResponse>("/api/leads/import", input),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["runs"] }),
  });
}
