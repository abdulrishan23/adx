// Campaigns, straight from the engine: one push for a project with its
// stages. A campaign is read from the studio, pages and plans, so its
// queries sit under the creative key and refresh with them.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON, putJSON } from "./client";
import type { CampaignDetail, CampaignInput, CampaignsResponse } from "./types.gen";

const KEY = ["creative", "campaigns"] as const;

export function useCampaigns() {
  return useQuery({
    queryKey: [...KEY],
    queryFn: () => getJSON<CampaignsResponse>("/api/campaigns"),
  });
}

export function useProjectCampaigns(projectId: number) {
  return useQuery({
    queryKey: [...KEY, "project", projectId],
    queryFn: () => getJSON<CampaignsResponse>(`/api/projects/${projectId}/campaigns`),
  });
}

export function useCampaign(id: number) {
  return useQuery({
    queryKey: [...KEY, id],
    queryFn: () => getJSON<CampaignDetail>(`/api/campaigns/${id}`),
    enabled: id > 0,
  });
}

function useCampaignMutation<I>(fn: (input: I) => Promise<CampaignDetail>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["creative"] });
      void qc.invalidateQueries({ queryKey: ["pages"] });
      void qc.invalidateQueries({ queryKey: ["plans"] });
    },
  });
}

export const useCreateCampaign = () =>
  useCampaignMutation((input: CampaignInput) => postJSON<CampaignDetail>("/api/campaigns", input));

export const useUpdateCampaign = (id: number) =>
  useCampaignMutation((input: CampaignInput) =>
    putJSON<CampaignDetail>(`/api/campaigns/${id}`, input),
  );
