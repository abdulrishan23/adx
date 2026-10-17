package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/qualification"
	"github.com/WokeWorks/adx/server/providers/twilio"
)

type RecordingResponse struct {
	Ref string `json:"ref"`
}

func (h *handlers) callEvidence(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "call")
	if !ok {
		return
	}
	ev, err := h.qualification.Evidence(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

func (h *handlers) confirmReading(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "call")
	if !ok {
		return
	}
	s, err := h.qualification.Confirm(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *handlers) correctReading(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "call")
	if !ok {
		return
	}
	var in qualification.CorrectionInput
	if !readJSON(w, r, &in) {
		return
	}
	s, err := h.qualification.Correct(r.Context(), actorFrom(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *handlers) callRecording(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "call")
	if !ok {
		return
	}
	ref, err := h.qualification.Recording(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, RecordingResponse{Ref: ref})
}

// voiceStream is the telephony provider's media stream for one call. It is
// public: the start message must name the call SID ADX placed and carry
// the call's occurrence key, or the socket is closed before any audio.
func (h *handlers) voiceStream(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid call id")
		return
	}
	detail, err := h.calls.Get(r.Context(), core.System(core.ViaJob), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		return
	}
	ctx := r.Context()
	transport, err := twilio.AcceptStream(ctx, conn, twilio.StreamExpectation{CallSid: detail.Call.ProviderCallID, Occurrence: detail.Call.OccurrenceKey}, 10*time.Second)
	if err != nil {
		conn.Close(websocket.StatusPolicyViolation, "stream does not match the call")
		return
	}
	if _, err := h.qualification.Run(ctx, id, transport); err != nil {
		h.log.WarnContext(ctx, "conversation failed", "call", id, "error", err)
	}
	_ = transport.Close()
}
