package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core/workflows"
)

type WorkflowRunsResponse struct {
	Runs []workflows.WorkflowRun `json:"runs"`
}

type CorrectTaskInput struct {
	Output json.RawMessage `json:"output"`
	Reason string          `json:"reason"`
}

func (h *handlers) listWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.workflows.List(r.Context(), actorFrom(r), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, WorkflowRunsResponse{Runs: list})
}

func (h *handlers) startWorkflowRun(w http.ResponseWriter, r *http.Request) {
	var in workflows.WorkflowStartInput
	if !readJSON(w, r, &in) {
		return
	}
	g, err := h.workflows.Start(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *handlers) workflowGraph(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "workflow run")
	if !ok {
		return
	}
	g, err := h.workflows.Graph(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *handlers) workflowTransition(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id", "workflow run")
		if !ok {
			return
		}
		var in ReasonInput
		if r.ContentLength != 0 && !readJSON(w, r, &in) {
			return
		}
		var g workflows.WorkflowGraph
		var err error
		switch kind {
		case "pause":
			g, err = h.workflows.Pause(r.Context(), actorFrom(r), id, in.Reason)
		case "resume":
			g, err = h.workflows.Resume(r.Context(), actorFrom(r), id)
		default:
			g, err = h.workflows.Cancel(r.Context(), actorFrom(r), id, in.Reason)
		}
		if err != nil {
			h.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, g)
	}
}

func (h *handlers) retryWorkflowTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "task")
	if !ok {
		return
	}
	g, err := h.workflows.Retry(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *handlers) correctWorkflowTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "task")
	if !ok {
		return
	}
	var in CorrectTaskInput
	if !readJSON(w, r, &in) {
		return
	}
	g, err := h.workflows.Correct(r.Context(), actorFrom(r), id, in.Output, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *handlers) tickWorkflows(w http.ResponseWriter, r *http.Request) {
	if !actorFrom(r).AtLeast("marketer") {
		writeError(w, http.StatusForbidden, "marketers and admins only")
		return
	}
	n, err := h.workflows.Tick(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"worked": n})
}
