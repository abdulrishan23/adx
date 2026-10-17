package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core/assistant"
)

type ConversationsResponse struct {
	Conversations []assistant.Conversation `json:"conversations"`
}

type TurnsResponse struct {
	Turns []assistant.Turn `json:"turns"`
}

type SendMessageInput struct {
	Text string `json:"text"`
}

func (h *handlers) listConversations(w http.ResponseWriter, r *http.Request) {
	list, err := h.assistant.ListConversations(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ConversationsResponse{Conversations: list})
}

func (h *handlers) createConversation(w http.ResponseWriter, r *http.Request) {
	conv, err := h.assistant.CreateConversation(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, conv)
}

func (h *handlers) conversationTurns(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	turns, err := h.assistant.Turns(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, TurnsResponse{Turns: turns})
}

func (h *handlers) sendMessage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	var in SendMessageInput
	if !readJSON(w, r, &in) {
		return
	}
	reply, err := h.assistant.Send(r.Context(), actorFrom(r), id, in.Text)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reply)
}
