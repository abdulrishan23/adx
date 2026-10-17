import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type { Conversation, ConversationsResponse, Reply, TurnsResponse } from "./types.gen";

export function useConversations() {
  return useQuery({
    queryKey: ["assistant", "conversations"],
    queryFn: () => getJSON<ConversationsResponse>("/api/assistant/conversations"),
  });
}

export function useTurns(id: number | null) {
  return useQuery({
    queryKey: ["assistant", "turns", id],
    queryFn: () => getJSON<TurnsResponse>(`/api/assistant/conversations/${id}`),
    enabled: id !== null,
  });
}

export function useCreateConversation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => postJSON<Conversation>("/api/assistant/conversations", {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["assistant", "conversations"] }),
  });
}

export function useSendMessage(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (text: string) =>
      postJSON<Reply>(`/api/assistant/conversations/${id}/messages`, { text }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["assistant", "turns", id] });
      qc.invalidateQueries({ queryKey: ["assistant", "conversations"] });
    },
  });
}
