package api

import (
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core/execution"
)

type CapabilitiesResponse struct {
	Platforms []execution.PlatformCapabilities `json:"platforms"`
}

func (h *handlers) executionCapabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, CapabilitiesResponse{Platforms: h.execution.Capabilities()})
}

func (h *handlers) executionTimeline(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "execution")
	if !ok {
		return
	}
	tl, err := h.execution.Timeline(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tl)
}

func (h *handlers) retryExecution(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "execution")
	if !ok {
		return
	}
	in, err := h.execution.Retry(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (h *handlers) reconcileExecution(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "execution")
	if !ok {
		return
	}
	in, err := h.execution.Reconcile(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (h *handlers) launchStatus(w http.ResponseWriter, r *http.Request) {
	planID, err1 := strconv.ParseInt(r.URL.Query().Get("plan_id"), 10, 64)
	version, err2 := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "plan_id and version are required")
		return
	}
	ls, err := h.execution.Launch(r.Context(), actorFrom(r), planID, version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ls)
}
