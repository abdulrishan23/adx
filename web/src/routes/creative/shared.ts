import { useQueries } from "@tanstack/react-query";
import { useCampaigns } from "@/api/campaigns";
import { getJSON } from "@/api/client";
import { useCreativeOverview } from "@/api/creative";
import type { CreativeProject } from "@/api/types.gen";

// useAllCreative reads every set-up project's concepts, ads and pictures, for
// the Creative screens that work across projects. Reportage has a handful of
// projects, so one request each is fine; the cache key matches
// useCreativeProject's, so opening a project reuses it.
export function useAllCreative() {
  const overview = useCreativeOverview();
  const ids = overview.data?.projects.map((p) => p.projectId) ?? [];
  const projects = useQueries({
    queries: ids.map((id) => ({
      queryKey: ["creative", "project", id],
      queryFn: () => getJSON<CreativeProject>(`/api/creative/projects/${id}`),
    })),
  });
  const campaigns = useCampaigns();
  const campaignName = (id: number | null) =>
    id === null ? null : (campaigns.data?.campaigns.find((c) => c.id === id)?.name ?? null);
  const loaded = projects.map((q) => q.data).filter((p): p is CreativeProject => !!p);
  return {
    isPending: overview.isPending || projects.some((q) => q.isPending),
    error: overview.error ?? projects.find((q) => q.error)?.error ?? null,
    projects: loaded,
    overview: overview.data,
    campaigns: campaigns.data?.campaigns ?? [],
    campaignName,
  };
}
