import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  Assignment,
  ExportResponse,
  FollowUpTask,
  ImportBatch,
  ImportsResponse,
  OpportunitiesResponse,
  Opportunity,
  OpportunityDetail,
  PipelineConfig,
  PipelineDocument,
  PipelineState,
  TaskInput,
  TasksResponse,
} from "@/api/types.gen";
import { getJSON, postJSON, putJSON } from "./client";

function invalidate(qc: ReturnType<typeof useQueryClient>) {
  return () => qc.invalidateQueries({ queryKey: ["pipeline"] });
}

export function usePipelineConfig() {
  return useQuery({
    queryKey: ["pipeline", "config"],
    queryFn: () => getJSON<PipelineState>("/api/crm/pipeline/config"),
  });
}

export function usePutPipelineConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (doc: PipelineDocument) => putJSON<PipelineConfig>("/api/crm/pipeline/config", doc),
    onSuccess: invalidate(qc),
  });
}

export function useOpportunities(stage: string, state: string) {
  return useQuery({
    queryKey: ["pipeline", "opportunities", stage, state],
    queryFn: () =>
      getJSON<OpportunitiesResponse>(
        `/api/crm/opportunities?stage=${encodeURIComponent(stage)}&state=${encodeURIComponent(state)}&limit=200`,
      ),
  });
}

export function useOpportunity(id: number | null) {
  return useQuery({
    queryKey: ["pipeline", "opportunity", id],
    queryFn: () => getJSON<OpportunityDetail>(`/api/crm/opportunities/${id}`),
    enabled: id !== null,
  });
}

export function useOpenOpportunity() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (enquiryId: number) =>
      postJSON<Opportunity>("/api/crm/opportunities", { enquiryId }),
    onSuccess: invalidate(qc),
  });
}

export function useSetStage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      stage,
      version,
      reason,
    }: {
      id: number;
      stage: string;
      version: number;
      reason: string;
    }) => postJSON<Opportunity>(`/api/crm/opportunities/${id}/stage`, { stage, version, reason }),
    onSuccess: invalidate(qc),
  });
}

export function useAssign() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, userId, reason }: { id: number; userId: number; reason: string }) =>
      postJSON<Assignment>(`/api/crm/opportunities/${id}/assign`, { userId, reason }),
    onSuccess: invalidate(qc),
  });
}

export function useTasks(state: string) {
  return useQuery({
    queryKey: ["pipeline", "tasks", state],
    queryFn: () => getJSON<TasksResponse>(`/api/crm/tasks?state=${state}&limit=200`),
  });
}

export function useCreateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: TaskInput) => postJSON<FollowUpTask>("/api/crm/tasks", input),
    onSuccess: invalidate(qc),
  });
}

export function useCompleteTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, canceled }: { id: number; canceled: boolean }) =>
      postJSON<FollowUpTask>(`/api/crm/tasks/${id}/complete`, { canceled }),
    onSuccess: invalidate(qc),
  });
}

export function useImports() {
  return useQuery({
    queryKey: ["pipeline", "imports"],
    queryFn: () => getJSON<ImportsResponse>("/api/crm/imports?limit=10"),
  });
}

export function usePreviewImport() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (source: string) => postJSON<ImportBatch>("/api/crm/imports/preview", { source }),
    onSuccess: invalidate(qc),
  });
}

export function useApplyImport() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => postJSON<ImportBatch>(`/api/crm/imports/${id}/apply`, {}),
    onSuccess: invalidate(qc),
  });
}

export function useExport() {
  return useQuery({
    queryKey: ["pipeline", "export"],
    queryFn: () => getJSON<ExportResponse>("/api/crm/export"),
    enabled: false,
  });
}
