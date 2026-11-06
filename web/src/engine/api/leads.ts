import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { FunnelDocument, FunnelMapping, FunnelState } from "@/api/types.gen";
import { getJSON } from "./client";

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
