import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type {
  InsightsResponse,
  LearningDecisionInput,
  LearningOutcome,
  LearningOutcomeInput,
  LearningRecord,
  LearningRecordInput,
  LearningRecordsResponse,
  LearningReport,
  MemoryFindingsResponse,
  MemoryInsight,
  MemoryInsightInput,
} from "./types.gen";

function invalidate(qc: ReturnType<typeof useQueryClient>) {
  return () => qc.invalidateQueries({ queryKey: ["learning"] });
}

export function useLearningRecords() {
  return useQuery({
    queryKey: ["learning", "records"],
    queryFn: () => getJSON<LearningRecordsResponse>("/api/learning/records?limit=100"),
  });
}

export function useLearningReport() {
  return useQuery({
    queryKey: ["learning", "report"],
    queryFn: () => getJSON<LearningReport>("/api/learning/report"),
  });
}

export function useInsights() {
  return useQuery({
    queryKey: ["learning", "insights"],
    queryFn: () => getJSON<InsightsResponse>("/api/learning/insights"),
  });
}

export function useFindings() {
  return useQuery({
    queryKey: ["learning", "findings"],
    queryFn: () => getJSON<MemoryFindingsResponse>("/api/learning/findings"),
  });
}

export function useCreateRecord() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: LearningRecordInput) =>
      postJSON<LearningRecord>("/api/learning/records", input),
    onSuccess: invalidate(qc),
  });
}

export function useDecideRecord() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: { id: number } & LearningDecisionInput) =>
      postJSON<LearningRecord>(`/api/learning/records/${id}/decide`, input),
    onSuccess: invalidate(qc),
  });
}

export function useObserveOutcome() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: LearningOutcomeInput) =>
      postJSON<LearningOutcome>("/api/learning/outcomes", input),
    onSuccess: invalidate(qc),
  });
}

export function useProposeInsight() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: MemoryInsightInput) =>
      postJSON<MemoryInsight>("/api/learning/insights", input),
    onSuccess: invalidate(qc),
  });
}

export function usePromoteInsight() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      to,
      controlEvidenceId,
    }: {
      id: number;
      to: string;
      controlEvidenceId: number;
    }) =>
      postJSON<MemoryInsight>(`/api/learning/insights/${id}/promote`, { to, controlEvidenceId }),
    onSuccess: invalidate(qc),
  });
}

export function useReviseInsight() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, statement, scope }: { id: number; statement: string; scope: string }) =>
      postJSON<MemoryInsight>(`/api/learning/insights/${id}/revise`, { statement, scope }),
    onSuccess: invalidate(qc),
  });
}
