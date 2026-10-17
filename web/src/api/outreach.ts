import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON, putJSON } from "./client";
import type {
  DispatchResponse,
  DraftInput,
  FieldWrite,
  FieldWritesResponse,
  MessageDraft,
  Objection,
  ObjectionInput,
  OutreachConfig,
  OutreachDocument,
  OutreachState,
  QueuedResponse,
  SalesBrief,
  Suppression,
} from "./types.gen";

function invalidate(qc: ReturnType<typeof useQueryClient>) {
  return () => {
    qc.invalidateQueries({ queryKey: ["outreach"] });
    qc.invalidateQueries({ queryKey: ["signals"] });
  };
}

export function useOutreachConfig() {
  return useQuery({
    queryKey: ["outreach", "config"],
    queryFn: () => getJSON<OutreachState>("/api/outreach/config"),
  });
}

export function usePutOutreachConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (doc: OutreachDocument) => putJSON<OutreachConfig>("/api/outreach/config", doc),
    onSuccess: invalidate(qc),
  });
}

export function useBrief(enquiryId: number | null) {
  return useQuery({
    queryKey: ["outreach", "brief", enquiryId],
    queryFn: () => getJSON<SalesBrief>(`/api/outreach/enquiries/${enquiryId}/brief`),
    enabled: enquiryId !== null,
  });
}

export function useRecordObjection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ObjectionInput) => postJSON<Objection>("/api/outreach/objections", input),
    onSuccess: invalidate(qc),
  });
}

export function useResolveObjection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) =>
      postJSON<Objection>(`/api/outreach/objections/${id}/resolve`, { reason }),
    onSuccess: invalidate(qc),
  });
}

export function useSuppress() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { personId: number; channel: string; reason: string; source: string }) =>
      postJSON<Suppression>("/api/outreach/suppressions", input),
    onSuccess: invalidate(qc),
  });
}

export function useLiftSuppression() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) =>
      postJSON<Suppression>(`/api/outreach/suppressions/${id}/lift`, { reason }),
    onSuccess: invalidate(qc),
  });
}

export function useCreateDraft() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: DraftInput) => postJSON<MessageDraft>("/api/outreach/drafts", input),
    onSuccess: invalidate(qc),
  });
}

export function useApproveDraft() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => postJSON<MessageDraft>(`/api/outreach/drafts/${id}/approve`, {}),
    onSuccess: invalidate(qc),
  });
}

export function useSendDraft() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => postJSON<MessageDraft>(`/api/outreach/drafts/${id}/send`, {}),
    onSuccess: invalidate(qc),
  });
}

export function useFieldWrites() {
  return useQuery({
    queryKey: ["outreach", "fields"],
    queryFn: () => getJSON<FieldWritesResponse>("/api/outreach/fields?limit=200"),
  });
}

export function useQueueFieldWrite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { personId: number; field: string; value: string }) =>
      postJSON<FieldWritesResponse>("/api/outreach/fields", input),
    onSuccess: invalidate(qc),
  });
}

export function useResolveFieldConflict() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, keepOurs, reason }: { id: number; keepOurs: boolean; reason: string }) =>
      postJSON<FieldWrite>(`/api/outreach/fields/${id}/resolve`, { keepOurs, reason }),
    onSuccess: invalidate(qc),
  });
}

export function useDispatchFields() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => postJSON<DispatchResponse>("/api/outreach/fields/dispatch", {}),
    onSuccess: invalidate(qc),
  });
}

export function useSyncScores() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => postJSON<QueuedResponse>("/api/outreach/fields/sync-scores", {}),
    onSuccess: invalidate(qc),
  });
}
