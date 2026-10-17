package api

import (
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core/outreach"
)

type FieldWritesResponse struct {
	Writes []outreach.FieldWrite `json:"writes"`
}

type DraftsResponse struct {
	Drafts []outreach.MessageDraft `json:"drafts"`
}

type SuppressInput struct {
	PersonID int64  `json:"personId"`
	Channel  string `json:"channel"`
	Reason   string `json:"reason"`
	Source   string `json:"source"`
}

type QueueFieldInput struct {
	PersonID int64  `json:"personId"`
	Field    string `json:"field"`
	Value    string `json:"value"`
}

type ResolveFieldConflictInput struct {
	KeepOurs bool   `json:"keepOurs"`
	Reason   string `json:"reason"`
}

type DispatchResponse struct {
	Sent int `json:"sent"`
}

type QueuedResponse struct {
	Queued int `json:"queued"`
}

func (h *handlers) getOutreachConfig(w http.ResponseWriter, r *http.Request) {
	st, err := h.outreach.GetConfig(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *handlers) putOutreachConfig(w http.ResponseWriter, r *http.Request) {
	var doc outreach.OutreachDocument
	if !readJSON(w, r, &doc) {
		return
	}
	c, err := h.outreach.PutConfig(r.Context(), actorFrom(r), doc)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *handlers) enquiryBrief(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "enquiry")
	if !ok {
		return
	}
	b, err := h.outreach.NextAction(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *handlers) recordObjection(w http.ResponseWriter, r *http.Request) {
	var in outreach.ObjectionInput
	if !readJSON(w, r, &in) {
		return
	}
	o, err := h.outreach.RecordObjection(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (h *handlers) resolveObjection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "objection")
	if !ok {
		return
	}
	var in ReasonInput
	if !readJSON(w, r, &in) {
		return
	}
	o, err := h.outreach.ResolveObjection(r.Context(), actorFrom(r), id, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *handlers) suppress(w http.ResponseWriter, r *http.Request) {
	var in SuppressInput
	if !readJSON(w, r, &in) {
		return
	}
	s, err := h.outreach.Suppress(r.Context(), actorFrom(r), in.PersonID, in.Channel, in.Reason, in.Source)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

func (h *handlers) liftSuppression(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "suppression")
	if !ok {
		return
	}
	var in ReasonInput
	if !readJSON(w, r, &in) {
		return
	}
	s, err := h.outreach.Lift(r.Context(), actorFrom(r), id, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *handlers) createDraft(w http.ResponseWriter, r *http.Request) {
	var in outreach.DraftInput
	if !readJSON(w, r, &in) {
		return
	}
	d, err := h.outreach.Draft(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (h *handlers) listDrafts(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.outreach.ListDrafts(r.Context(), actorFrom(r), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, DraftsResponse{Drafts: list})
}

func (h *handlers) approveDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "draft")
	if !ok {
		return
	}
	d, err := h.outreach.Approve(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) sendDraft(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "draft")
	if !ok {
		return
	}
	d, err := h.outreach.Send(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) listFieldWrites(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.outreach.ListFieldWrites(r.Context(), actorFrom(r), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, FieldWritesResponse{Writes: list})
}

func (h *handlers) queueFieldWrite(w http.ResponseWriter, r *http.Request) {
	var in QueueFieldInput
	if !readJSON(w, r, &in) {
		return
	}
	list, err := h.outreach.QueueFieldWrite(r.Context(), actorFrom(r), in.PersonID, in.Field, in.Value)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, FieldWritesResponse{Writes: list})
}

func (h *handlers) resolveFieldConflict(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "field write")
	if !ok {
		return
	}
	var in ResolveFieldConflictInput
	if !readJSON(w, r, &in) {
		return
	}
	fw, err := h.outreach.ResolveConflict(r.Context(), actorFrom(r), id, in.KeepOurs, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, fw)
}

func (h *handlers) dispatchFieldWrites(w http.ResponseWriter, r *http.Request) {
	n, err := h.outreach.DispatchFields(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, DispatchResponse{Sent: n})
}

func (h *handlers) syncScores(w http.ResponseWriter, r *http.Request) {
	n, err := h.outreach.SyncScores(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, QueuedResponse{Queued: n})
}
