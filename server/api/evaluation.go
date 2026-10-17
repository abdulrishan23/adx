package api

import (
	"net/http"

	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/evaluation"
)

type VoiceInput struct {
	Transcript string `json:"transcript"`
}

// sendVoice is a spoken turn: the browser's transcript, through the same
// read-only assistant as typing, with the action preview in the reply.
func (h *handlers) sendVoice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "conversation")
	if !ok {
		return
	}
	var in VoiceInput
	if !readJSON(w, r, &in) {
		return
	}
	reply, err := h.assistant.SendVoice(r.Context(), actorFrom(r), id, in.Transcript)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reply)
}

// previewVoice shows what a transcript sounds like before it is sent.
func (h *handlers) previewVoice(w http.ResponseWriter, r *http.Request) {
	var in VoiceInput
	if !readJSON(w, r, &in) {
		return
	}
	writeJSON(w, http.StatusOK, assistant.Preview(in.Transcript))
}

func (h *handlers) evaluationReport(w http.ResponseWriter, r *http.Request) {
	rep, err := h.evaluation.Report(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (h *handlers) labelCall(w http.ResponseWriter, r *http.Request) {
	var in evaluation.LabelInput
	if !readJSON(w, r, &in) {
		return
	}
	l, err := h.evaluation.Label(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}
