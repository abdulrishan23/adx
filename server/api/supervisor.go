package api

import (
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core/supervisor"
)

type InboxResponse struct {
	Items []supervisor.InboxItem `json:"items"`
}

type ConflictsResponse struct {
	Conflicts []supervisor.RecommendationConflict `json:"conflicts"`
}

type RecommendationsResponse struct {
	Recommendations []supervisor.Recommendation `json:"recommendations"`
}

type HaltInput struct {
	AccountID int64  `json:"accountId"`
	Engaged   bool   `json:"engaged"`
	Reason    string `json:"reason"`
}

func (h *handlers) supervisorInbox(w http.ResponseWriter, r *http.Request) {
	items, err := h.supervisor.Inbox(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, InboxResponse{Items: items})
}

func (h *handlers) supervisorAct(w http.ResponseWriter, r *http.Request) {
	var in supervisor.ActionInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.supervisor.Act(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) getSupervisorConfig(w http.ResponseWriter, r *http.Request) {
	st, err := h.supervisor.GetConfig(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *handlers) putSupervisorConfig(w http.ResponseWriter, r *http.Request) {
	var doc supervisor.SupervisorDocument
	if !readJSON(w, r, &doc) {
		return
	}
	c, err := h.supervisor.PutConfig(r.Context(), actorFrom(r), doc)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *handlers) supervisorArbitrate(w http.ResponseWriter, r *http.Request) {
	list, err := h.supervisor.Arbitrate(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ConflictsResponse{Conflicts: list})
}

func (h *handlers) supervisorConflicts(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.supervisor.Conflicts(r.Context(), actorFrom(r), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ConflictsResponse{Conflicts: list})
}

func (h *handlers) supervisorRecommendations(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.supervisor.Recommendations(r.Context(), actorFrom(r), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, RecommendationsResponse{Recommendations: list})
}

func (h *handlers) supervisorHalt(w http.ResponseWriter, r *http.Request) {
	var in HaltInput
	if !readJSON(w, r, &in) {
		return
	}
	k, err := h.supervisor.Halt(r.Context(), actorFrom(r), in.AccountID, in.Engaged, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}
