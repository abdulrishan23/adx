// The status-to-stage table (core/leads: the funnel settings). Always the Go
// server: a mocked table would look like Reportage's real definitions.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, putJSON } from "./client";
import type { FunnelDocument, FunnelMapping, FunnelState } from "./types.gen";

const KEY = ["stages"] as const;

export function useFunnelSettings() {
  return useQuery({
    queryKey: KEY,
    queryFn: () => getJSON<FunnelState>("/api/settings/funnel"),
  });
}

// Saving writes the next version and re-stages every lead from it, so the
// leads and their coverage are read again.
export function useSaveFunnel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (document: FunnelDocument) =>
      putJSON<FunnelMapping>("/api/settings/funnel", { document }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY });
      qc.invalidateQueries({ queryKey: ["leads"] });
    },
  });
}
