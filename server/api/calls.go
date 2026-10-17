package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core/calls"
	"github.com/WokeWorks/adx/server/core/voice"
)

type CallsResponse struct {
	Calls []calls.Call `json:"calls"`
}

func (h *handlers) getCallPolicy(w http.ResponseWriter, r *http.Request) {
	st, err := h.calls.GetPolicy(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *handlers) putCallPolicy(w http.ResponseWriter, r *http.Request) {
	var doc calls.CallPolicyDocument
	if !readJSON(w, r, &doc) {
		return
	}
	p, err := h.calls.PutPolicy(r.Context(), actorFrom(r), doc)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *handlers) contactability(w http.ResponseWriter, r *http.Request) {
	enquiryID, _ := strconv.ParseInt(r.URL.Query().Get("enquiry_id"), 10, 64)
	if enquiryID == 0 {
		writeError(w, http.StatusBadRequest, "enquiry_id is required")
		return
	}
	ct, err := h.calls.Contactability(r.Context(), actorFrom(r), enquiryID, r.URL.Query().Get("consent"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ct)
}

func (h *handlers) listCalls(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.calls.List(r.Context(), actorFrom(r), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, CallsResponse{Calls: list})
}

func (h *handlers) requestCall(w http.ResponseWriter, r *http.Request) {
	var in calls.CallInput
	if !readJSON(w, r, &in) {
		return
	}
	c, err := h.calls.Request(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *handlers) getCall(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "call")
	if !ok {
		return
	}
	d, err := h.calls.Get(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) hangupCall(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "call")
	if !ok {
		return
	}
	var in ReasonInput
	if !readJSON(w, r, &in) {
		return
	}
	c, err := h.calls.Hangup(r.Context(), actorFrom(r), id, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) reconcileCall(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "call")
	if !ok {
		return
	}
	c, err := h.calls.Reconcile(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// voiceStatusWebhook is the telephony provider's status callback. The
// adapter verifies the signature; a bad one is 403 and nothing is recorded.
func (h *handlers) voiceStatusWebhook(w http.ResponseWriter, r *http.Request) {
	_, err := h.calls.Callback(r.Context(), r)
	switch {
	case errors.Is(err, voice.ErrBadSignature):
		writeError(w, http.StatusForbidden, "callback signature is invalid")
	case err != nil:
		h.fail(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
