import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type {
  EnvelopesResponse,
  ExecutionsResponse,
  Intent,
  OpenEnvelopesInput,
  Proposal,
  ProposalInput,
  ProposalResponse,
  ProposalsResponse,
} from "./types.gen";

export function useProposals(state: string) {
  return useQuery({
    queryKey: ["proposals", state],
    queryFn: () => getJSON<ProposalsResponse>(`/api/proposals?state=${state}&limit=100`),
  });
}

export function useProposal(id: number | null) {
  return useQuery({
    queryKey: ["proposals", "one", id],
    queryFn: () => getJSON<ProposalResponse>(`/api/proposals/${id}`),
    enabled: id !== null,
  });
}

export function useExecutions() {
  return useQuery({
    queryKey: ["executions"],
    queryFn: () => getJSON<ExecutionsResponse>("/api/executions?limit=100"),
  });
}

export function useEnvelopes() {
  return useQuery({
    queryKey: ["envelopes"],
    queryFn: () => getJSON<EnvelopesResponse>("/api/budgets/envelopes"),
  });
}

function invalidateAll(qc: ReturnType<typeof useQueryClient>) {
  return () => {
    qc.invalidateQueries({ queryKey: ["proposals"] });
    qc.invalidateQueries({ queryKey: ["executions"] });
    qc.invalidateQueries({ queryKey: ["envelopes"] });
  };
}

export function useCreateProposal() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ProposalInput) => postJSON<Proposal>("/api/proposals", input),
    onSuccess: invalidateAll(qc),
  });
}

export function useDecideProposal() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      decision,
      note,
    }: {
      id: number;
      decision: "approve" | "reject";
      note: string;
    }) => postJSON<Proposal>(`/api/proposals/${id}/${decision}`, { note }),
    onSuccess: invalidateAll(qc),
  });
}

export function useExecuteProposal() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, idempotencyKey }: { id: number; idempotencyKey: string }) =>
      postJSON<Intent>(`/api/proposals/${id}/execute`, { idempotencyKey }),
    onSuccess: invalidateAll(qc),
  });
}

export function useCancelExecution() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) =>
      postJSON<Intent>(`/api/executions/${id}/cancel`, { reason }),
    onSuccess: invalidateAll(qc),
  });
}

export function useOpenEnvelopes() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: OpenEnvelopesInput) =>
      postJSON<EnvelopesResponse>("/api/budgets/envelopes", input),
    onSuccess: invalidateAll(qc),
  });
}
