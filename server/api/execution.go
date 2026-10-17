package api

import (
	"encoding/json"
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

type CompensateInput struct {
	Reason string `json:"reason"`
}

type RepairInput struct {
	Proposed json.RawMessage `json:"proposed"`
	Note     string          `json:"note"`
}

func (h *handlers) compensateExecution(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "execution")
	if !ok {
		return
	}
	var in CompensateInput
	if !readJSON(w, r, &in) {
		return
	}
	intent, err := h.execution.Compensate(r.Context(), actorFrom(r), id, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, intent)
}

func (h *handlers) repairExecution(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "execution")
	if !ok {
		return
	}
	var in RepairInput
	if !readJSON(w, r, &in) {
		return
	}
	p, err := h.execution.Repair(r.Context(), actorFrom(r), id, in.Proposed, in.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *handlers) executionReversibility(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "execution")
	if !ok {
		return
	}
	rev, err := h.execution.Reversibility(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rev)
}

func (h *handlers) launchMilestones(w http.ResponseWriter, r *http.Request) {
	planID, err1 := strconv.ParseInt(r.URL.Query().Get("plan_id"), 10, 64)
	version, err2 := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "plan_id and version are required")
		return
	}
	ms, err := h.execution.Milestones(r.Context(), actorFrom(r), planID, version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ms)
}
