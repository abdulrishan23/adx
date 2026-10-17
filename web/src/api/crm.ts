import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type {
  CandidatesResponse,
  EnquiriesResponse,
  Link,
  PeopleResponse,
  PersonDetail,
  PersonInput,
  SyncResponse,
} from "./types.gen";

export function usePeople(q: string) {
  return useQuery({
    queryKey: ["crm", "people", q],
    queryFn: () => getJSON<PeopleResponse>(`/api/crm/people?q=${encodeURIComponent(q)}&limit=100`),
  });
}

export function usePerson(id: number | null) {
  return useQuery({
    queryKey: ["crm", "person", id],
    queryFn: () => getJSON<PersonDetail>(`/api/crm/people/${id}`),
    enabled: id !== null,
  });
}

export function useCandidates() {
  return useQuery({
    queryKey: ["crm", "candidates"],
    queryFn: () => getJSON<CandidatesResponse>("/api/crm/candidates?limit=100"),
  });
}

export function useEnquiries(projectId: number) {
  return useQuery({
    queryKey: ["crm", "enquiries", projectId],
    queryFn: () =>
      getJSON<EnquiriesResponse>(`/api/crm/enquiries?project_id=${projectId}&limit=100`),
  });
}

function invalidate(qc: ReturnType<typeof useQueryClient>) {
  return () => qc.invalidateQueries({ queryKey: ["crm"] });
}

export function useSyncCRM() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => postJSON<SyncResponse>("/api/crm/sync", {}),
    onSuccess: invalidate(qc),
  });
}

export function useCreatePerson() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: PersonInput) => postJSON<PersonDetail>("/api/crm/people", input),
    onSuccess: invalidate(qc),
  });
}

export function useDecideCandidate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, accept, note }: { id: number; accept: boolean; note: string }) =>
      postJSON<PersonDetail>(`/api/crm/candidates/${id}/decide`, { accept, note }),
    onSuccess: invalidate(qc),
  });
}

export function useMergePerson() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, intoId, reason }: { id: number; intoId: number; reason: string }) =>
      postJSON<PersonDetail>(`/api/crm/people/${id}/merge`, { intoId, reason }),
    onSuccess: invalidate(qc),
  });
}

export function useUnmergePerson() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) =>
      postJSON<PersonDetail>(`/api/crm/people/${id}/unmerge`, { reason }),
    onSuccess: invalidate(qc),
  });
}

export function useEnrichPerson() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => postJSON<PersonDetail>(`/api/crm/people/${id}/enrich`, {}),
    onSuccess: invalidate(qc),
  });
}

export function useSetStated() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      key,
      value,
      source,
    }: {
      id: number;
      key: string;
      value: string;
      source: string;
    }) => postJSON<PersonDetail>(`/api/crm/people/${id}/attributes`, { key, value, source }),
    onSuccess: invalidate(qc),
  });
}

export function useReverseLink() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, reason }: { id: number; reason: string }) =>
      postJSON<Link>(`/api/crm/links/${id}/reverse`, { reason }),
    onSuccess: invalidate(qc),
  });
}
