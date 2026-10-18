import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  AgentCapabilitiesResponse,
  AgentDocument,
  RegistryEntriesResponse,
  RegistryEntry,
  SkillDocument,
  ValidationResponse,
  WorkflowDocument,
} from "@/api/types.gen";
import { getJSON, postJSON } from "./client";

function invalidate(qc: ReturnType<typeof useQueryClient>) {
  return () => qc.invalidateQueries({ queryKey: ["registry"] });
}

export function useCapabilities() {
  return useQuery({
    queryKey: ["registry", "capabilities"],
    queryFn: () => getJSON<AgentCapabilitiesResponse>("/api/agents/capabilities"),
  });
}

export function useRegistry(kind: string) {
  return useQuery({
    queryKey: ["registry", kind],
    queryFn: () => getJSON<RegistryEntriesResponse>(`/api/registry/${kind}`),
  });
}

export function useSaveSkill() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { key: string; document: SkillDocument; fromModel: boolean }) =>
      postJSON<RegistryEntry>("/api/skills", input),
    onSuccess: invalidate(qc),
  });
}

export function useSaveAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { key: string; document: AgentDocument; fromModel: boolean }) =>
      postJSON<RegistryEntry>("/api/agents", input),
    onSuccess: invalidate(qc),
  });
}

export function useSaveWorkflow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { key: string; document: WorkflowDocument; fromModel: boolean }) =>
      postJSON<RegistryEntry>("/api/workflows/definitions", input),
    onSuccess: invalidate(qc),
  });
}

export function useValidateWorkflow() {
  return useMutation({
    mutationFn: (doc: WorkflowDocument) =>
      postJSON<ValidationResponse>("/api/workflows/definitions/validate", doc),
  });
}

export function useActivateEntry() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => postJSON<RegistryEntry>(`/api/registry/${id}/activate`, {}),
    onSuccess: invalidate(qc),
  });
}

export function useRetireEntry() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => postJSON<RegistryEntry>(`/api/registry/${id}/retire`, {}),
    onSuccess: invalidate(qc),
  });
}
