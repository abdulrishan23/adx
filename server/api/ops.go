package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core/ops"
)

type EvaluateReleaseInput struct {
	Model string `json:"model"`
}

type LoadResponse struct {
	Measurement *ops.LoadMeasurement `json:"measurement"`
	Note        string               `json:"note"`
}

func (h *handlers) opsCost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	report, err := h.ops.Cost(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *handlers) opsLoad(w http.ResponseWriter, r *http.Request) {
	m, err := h.ops.LatestLoad(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := LoadResponse{Measurement: m}
	if m == nil {
		out.Note = "No load measurement has been recorded on this server; run `adx loadtest DURATION CONCURRENCY`. No throughput is claimed without one."
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) listReleases(w http.ResponseWriter, r *http.Request) {
	report, err := h.evaluation.Releases(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *handlers) evaluateRelease(w http.ResponseWriter, r *http.Request) {
	var in EvaluateReleaseInput
	if !readJSON(w, r, &in) {
		return
	}
	rel, err := h.evaluation.EvaluateRelease(r.Context(), actorFrom(r), in.Model)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *handlers) promoteRelease(w http.ResponseWriter, r *http.Request) {
	rel, err := h.evaluation.Promote(r.Context(), actorFrom(r), chi.URLParam(r, "model"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

func (h *handlers) rejectRelease(w http.ResponseWriter, r *http.Request) {
	rel, err := h.evaluation.Reject(r.Context(), actorFrom(r), chi.URLParam(r, "model"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rel)
}
