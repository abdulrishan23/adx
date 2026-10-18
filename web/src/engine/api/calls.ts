import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  Call,
  CallDetail,
  CallEvidence,
  CallInput,
  CallPolicy,
  CallPolicyDocument,
  CallPolicyState,
  CallSession,
  CallsResponse,
  Contactability,
  CorrectionInput,
} from "@/api/types.gen";
import { getJSON, postJSON, putJSON } from "./client";

function invalidate(qc: ReturnType<typeof useQueryClient>) {
  return () => qc.invalidateQueries({ queryKey: ["calls"] });
}

export function useCallPolicy() {
  return useQuery({
    queryKey: ["calls", "policy"],
    queryFn: () => getJSON<CallPolicyState>("/api/voice/policy"),
  });
}

export function usePutCallPolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (doc: CallPolicyDocument) => putJSON<CallPolicy>("/api/voice/policy", doc),
    onSuccess: invalidate(qc),
  });
}

export function useContactability(enquiryId: number, consent: string) {
  return useQuery({
    queryKey: ["calls", "contactability", enquiryId, consent],
    queryFn: () =>
      getJSON<Contactability>(
        `/api/voice/contactability?enquiry_id=${enquiryId}&consent=${encodeURIComponent(consent)}`,
      ),
  });
}

export function useCalls() {
  return useQuery({
    queryKey: ["calls", "list"],
    queryFn: () => getJSON<CallsResponse>("/api/voice/calls?limit=100"),
  });
}

export function useCall(id: number | null) {
  return useQuery({
    queryKey: ["calls", "one", id],
    queryFn: () => getJSON<CallDetail>(`/api/voice/calls/${id}`),
    enabled: id !== null,
  });
}

export function useRequestCall() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CallInput) => postJSON<Call>("/api/voice/calls", input),
    onSuccess: invalidate(qc),
  });
}

export function useHangup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) =>
      postJSON<Call>(`/api/voice/calls/${id}/hangup`, { reason }),
    onSuccess: invalidate(qc),
  });
}

export function useReconcile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => postJSON<Call>(`/api/voice/calls/${id}/reconcile`, {}),
    onSuccess: invalidate(qc),
  });
}

export function useEvidence(id: number | null) {
  return useQuery({
    queryKey: ["calls", "evidence", id],
    queryFn: () => getJSON<CallEvidence>(`/api/voice/calls/${id}/evidence`),
    enabled: id !== null,
  });
}

export function useConfirmReading() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => postJSON<CallSession>(`/api/voice/calls/${id}/confirm`, {}),
    onSuccess: invalidate(qc),
  });
}

export function useCorrectReading() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: { id: number } & CorrectionInput) =>
      postJSON<CallSession>(`/api/voice/calls/${id}/outcome`, input),
    onSuccess: invalidate(qc),
  });
}
