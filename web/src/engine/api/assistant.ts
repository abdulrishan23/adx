import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  ActionPreview,
  Conversation,
  ConversationsResponse,
  Reply,
  TurnsResponse,
  VoiceReply,
} from "@/api/types.gen";
import { getJSON, postJSON } from "./client";

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

// Sends into a conversation named per call, for a question that arrives
// before the page has a current conversation.
export function useSendMessageTo() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, text }: { id: number; text: string }) =>
      postJSON<Reply>(`/api/assistant/conversations/${id}/messages`, { text }),
    onSuccess: (_reply, { id }) => {
      qc.invalidateQueries({ queryKey: ["assistant", "turns", id] });
      qc.invalidateQueries({ queryKey: ["assistant", "conversations"] });
    },
  });
}

export function useSendVoice(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (transcript: string) =>
      postJSON<VoiceReply>(`/api/assistant/conversations/${id}/voice`, { transcript }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["assistant", "turns", id] });
      qc.invalidateQueries({ queryKey: ["assistant", "conversations"] });
    },
  });
}

export function usePreviewVoice() {
  return useMutation({
    mutationFn: (transcript: string) =>
      postJSON<ActionPreview>("/api/assistant/voice/preview", { transcript }),
  });
}
