import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { WorkflowGraph, WorkflowRunsResponse, WorkflowStartInput } from "@/api/types.gen";
import { getJSON, postJSON } from "./client";

function invalidate(qc: ReturnType<typeof useQueryClient>) {
  return () => qc.invalidateQueries({ queryKey: ["workflows"] });
}

export function useWorkflowRuns() {
  return useQuery({
    queryKey: ["workflows", "runs"],
    queryFn: () => getJSON<WorkflowRunsResponse>("/api/workflows/runs?limit=50"),
  });
}

export function useWorkflowGraph(id: number | null) {
  return useQuery({
    queryKey: ["workflows", "graph", id],
    queryFn: () => getJSON<WorkflowGraph>(`/api/workflows/runs/${id}`),
    enabled: id !== null,
    refetchInterval: 10000,
  });
}

export function useStartWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: WorkflowStartInput) =>
      postJSON<WorkflowGraph>("/api/workflows/runs", input),
    onSuccess: invalidate(qc),
  });
}

export function useWorkflowTransition() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      kind,
      reason,
    }: {
      id: number;
      kind: "pause" | "resume" | "cancel";
      reason: string;
    }) => postJSON<WorkflowGraph>(`/api/workflows/runs/${id}/${kind}`, { reason }),
    onSuccess: invalidate(qc),
  });
}

export function useRetryTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => postJSON<WorkflowGraph>(`/api/workflows/tasks/${id}/retry`, {}),
    onSuccess: invalidate(qc),
  });
}

export function useCorrectTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, output, reason }: { id: number; output: unknown; reason: string }) =>
      postJSON<WorkflowGraph>(`/api/workflows/tasks/${id}/correct`, { output, reason }),
    onSuccess: invalidate(qc),
  });
}

export function useTickWorkflows() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => postJSON<{ worked: number }>("/api/workflows/tick", {}),
    onSuccess: invalidate(qc),
  });
}
