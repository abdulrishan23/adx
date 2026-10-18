import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  BriefDetail,
  BriefInput,
  BriefsResponse,
  ComparativeReport,
  Evidence,
  GenerateReportInput,
  HypothesisReviewInput,
  MarketEvent,
  MarketEventInput,
  MarketEventsResponse,
  ResearchReportsResponse,
  ResearchSchedulesResponse,
  ResolveCompetitorInput,
  Schedule,
  ScheduleInput,
  StartRunResponse,
} from "@/api/types.gen";
import { getJSON, postJSON } from "./client";

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

export function useResearchSchedules() {
  return useQuery({
    queryKey: ["research", "schedules"],
    queryFn: () => getJSON<ResearchSchedulesResponse>("/api/research/schedules"),
  });
}

export function useCreateResearchSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ScheduleInput) => postJSON<Schedule>("/api/research/schedules", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["research", "schedules"] }),
  });
}

export function useUpdateResearchSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, input }: { id: number; input: ScheduleInput }) => {
      const res = await fetch(`/api/research/schedules/${id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      });
      if (!res.ok) {
        const err = (await res.json().catch(() => null)) as { error?: string } | null;
        throw new Error(err?.error ?? res.statusText);
      }
      return (await res.json()) as Schedule;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["research", "schedules"] }),
  });
}

export function useResearchReports(briefId: number | null) {
  return useQuery({
    queryKey: ["research", "reports", briefId],
    queryFn: () => getJSON<ResearchReportsResponse>(`/api/research/briefs/${briefId}/reports`),
    enabled: briefId !== null,
  });
}

export function useGenerateResearchReport(briefId: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: GenerateReportInput) =>
      postJSON<ComparativeReport>(`/api/research/briefs/${briefId}/reports`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["research", "reports"] }),
  });
}

export function useReviewHypothesis() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ reportId, input }: { reportId: number; input: HypothesisReviewInput }) =>
      postJSON<ComparativeReport>(`/api/research/reports/${reportId}/review`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["research", "reports"] }),
  });
}

export function useMarketEvents() {
  return useQuery({
    queryKey: ["research", "events"],
    queryFn: () => getJSON<MarketEventsResponse>("/api/research/events"),
  });
}

export function useRecordMarketEvent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: MarketEventInput) => postJSON<MarketEvent>("/api/research/events", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["research", "events"] }),
  });
}
