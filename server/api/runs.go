package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core/runs"
)

type RunsResponse struct {
	Runs []runs.Run `json:"runs"`
}

func (h *handlers) getRun(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	run, err := h.runs.Get(r.Context(), actorFrom(r), chi.URLParam(r, "id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *handlers) listRuns(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.runs.List(r.Context(), actorFrom(r), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, RunsResponse{Runs: list})
}
