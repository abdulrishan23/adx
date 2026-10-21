// Convert › Inbox: the WhatsApp conversations the text agent runs (BD-69).
// The server owns every rule here: who the agent may answer, what it may
// say, when it hands to a person and whether a reply may leave. The screen
// shows the thread, the agent's drafts with their hold reasons, the
// handoffs and the reading, and sends a person's own words.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  CorrectionInput,
  InstructionsResponse,
  Thread,
  ThreadDetail,
  ThreadsReadiness,
  ThreadsResponse,
  ThreadTurn,
} from "@/api/types.gen";
import { getJSON, postJSON } from "./client";

export type ThreadState = Thread["state"];

const key = {
  list: (state: string) => ["conversations", "list", state] as const,
  one: (id: number) => ["conversations", "one", id] as const,
  readiness: ["conversations", "readiness"] as const,
  instructions: (id: number) => ["conversations", "instructions", id] as const,
};

export function useThreads(state: string) {
  const q = state ? `?state=${state}&limit=200` : "?limit=200";
  return useQuery({
    queryKey: key.list(state),
    queryFn: () => getJSON<ThreadsResponse>(`/api/conversations${q}`),
    refetchInterval: 15000,
  });
}

export function useThread(id: number | null) {
  return useQuery({
    queryKey: key.one(id ?? 0),
    queryFn: () => getJSON<ThreadDetail>(`/api/conversations/${id}`),
    enabled: id !== null,
    refetchInterval: 10000,
  });
}

export function useThreadsReadiness() {
  return useQuery({
    queryKey: key.readiness,
    queryFn: () => getJSON<ThreadsReadiness>("/api/conversations/readiness"),
  });
}

export function useInstructions(id: number | null) {
  return useQuery({
    queryKey: key.instructions(id ?? 0),
    queryFn: () => getJSON<InstructionsResponse>(`/api/conversations/${id}/instructions`),
    enabled: id !== null,
  });
}

function useInvalidate() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: ["conversations"] });
}

export type ThreadAction = "take" | "hand-back" | "close" | "read" | "confirm" | "reply";

// One mutation for the thread's own actions: each is a POST on the thread
// with an optional note, and every one refreshes the thread and the list.
export function useThreadAction() {
  const refresh = useInvalidate();
  return useMutation({
    mutationFn: ({ id, action, note }: { id: number; action: ThreadAction; note?: string }) =>
      postJSON<Thread | ThreadTurn>(`/api/conversations/${id}/${action}`, { note: note ?? "" }),
    onSuccess: refresh,
  });
}

export function useCorrectReading() {
  const refresh = useInvalidate();
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: CorrectionInput }) =>
      postJSON<Thread>(`/api/conversations/${id}/outcome`, input),
    onSuccess: refresh,
  });
}

export function useSendMessage() {
  const refresh = useInvalidate();
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: string }) =>
      postJSON<ThreadTurn>(`/api/conversations/${id}/messages`, { body }),
    onSuccess: refresh,
  });
}

export function useTurnAction() {
  const refresh = useInvalidate();
  return useMutation({
    mutationFn: ({
      id,
      action,
      note,
    }: {
      id: number;
      action: "approve" | "discard";
      note?: string;
    }) => postJSON<ThreadTurn>(`/api/conversations/turns/${id}/${action}`, { note: note ?? "" }),
    onSuccess: refresh,
  });
}
