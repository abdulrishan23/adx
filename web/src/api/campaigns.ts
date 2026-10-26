// Campaigns, straight from the engine: one push for a project with its
// stages. A campaign is read from the studio, pages and plans, so its
// queries sit under the creative key and refresh with them.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { Status } from "@/components/ui/status-chip";
import { getJSON, postJSON, putJSON } from "./client";
import type {
  CampaignDetail,
  CampaignInput,
  CampaignsResponse,
  ProjectProgress,
  ProjectsProgressResponse,
} from "./types.gen";

const KEY = ["creative", "campaigns"] as const;

export function useCampaigns() {
  return useQuery({
    queryKey: [...KEY],
    queryFn: () => getJSON<CampaignsResponse>("/api/campaigns"),
  });
}

// Where each of a project's five sections stands, and the measurement lines
// that gate Campaigns. Computed by the server from what the domains hold.
export function useProjectProgress(projectId: number | null) {
  return useQuery({
    queryKey: [...KEY, "progress", projectId],
    queryFn: () => getJSON<ProjectProgress>(`/api/projects/${projectId}/progress`),
    enabled: projectId !== null,
    // The sidebar keeps this query alive across screens, and facts, plans
    // and reviews change on other screens, so it re-reads on its own.
    refetchInterval: 30_000,
  });
}

// The same, for every project in one call: the Projects list and the project
// switcher show each project's health without a request per card.
export function useProjectsProgress() {
  return useQuery({
    queryKey: [...KEY, "progress", "all"],
    queryFn: () => getJSON<ProjectsProgressResponse>("/api/projects/progress"),
    refetchInterval: 30_000,
  });
}

export type ProjectHealth = {
  // Released sections that are ready, out of the released sections.
  ready: number;
  total: number;
  status: Status;
};

// healthOf reads a project's health from its progress: how many of the
// released sections are ready, and one status for the lot. A blocked project
// is an error whatever its sections say. Only released sections count, so a
// section hidden by the release phase neither helps nor hurts.
export function healthOf(progress: ProjectProgress, releasedIds: readonly string[]): ProjectHealth {
  const released = new Set(releasedIds);
  const ready = progress.sections.filter((s) => released.has(s.id) && s.status === "ready").length;
  const total = releasedIds.length;
  const status: Status = progress.blocked
    ? "error"
    : ready === total
      ? "ready"
      : ready > 0
        ? "caution"
        : "pending";
  return { ready, total, status };
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
