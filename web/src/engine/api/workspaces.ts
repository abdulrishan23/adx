import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  IndustryConfig,
  IndustryDocument,
  IndustryState,
  Member,
  MemberInput,
  MembersResponse,
  SessionResponse,
  Workspace,
  WorkspaceInput,
  WorkspacesResponse,
} from "@/api/types.gen";
import { ApiError, getJSON, postJSON, putJSON } from "./client";

export function useWorkspaces() {
  return useQuery({
    queryKey: ["workspaces"],
    queryFn: () => getJSON<WorkspacesResponse>("/api/workspaces"),
  });
}

export function useCreateWorkspace() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: WorkspaceInput) => postJSON<Workspace>("/api/workspaces", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["workspaces"] }),
  });
}

// Switching clears every cached query: everything on screen belonged to the
// workspace that was current, and none of it may survive into the next one.
export function useSwitchWorkspace() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (workspaceId: number) =>
      postJSON<SessionResponse>("/api/session/switch", { workspaceId }),
    onSuccess: () => qc.clear(),
  });
}

export function useMembers() {
  return useQuery({
    queryKey: ["workspaces", "members"],
    queryFn: () => getJSON<MembersResponse>("/api/workspaces/members"),
  });
}

export function useAddMember() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: MemberInput) => postJSON<Member>("/api/workspaces/members", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["workspaces", "members"] }),
  });
}

export function useRemoveMember() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (userId: number) => {
      const res = await fetch(`/api/workspaces/members/${userId}`, { method: "DELETE" });
      if (!res.ok) throw new ApiError(res.status, res.statusText);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["workspaces", "members"] }),
  });
}

export function useIndustry() {
  return useQuery({
    queryKey: ["workspaces", "industry"],
    queryFn: () => getJSON<IndustryState>("/api/settings/industry"),
  });
}

export function usePutIndustry() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (doc: IndustryDocument) => putJSON<IndustryConfig>("/api/settings/industry", doc),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["workspaces", "industry"] }),
  });
}
