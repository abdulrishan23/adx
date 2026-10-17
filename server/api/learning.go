package api

import (
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core/learning"
)

type LearningRecordsResponse struct {
	Records []learning.LearningRecord `json:"records"`
}

type InsightsResponse struct {
	Insights []learning.MemoryInsight `json:"insights"`
}

type MemoryFindingsResponse struct {
	Findings []learning.MemoryFinding `json:"findings"`
}

type PromoteInsightInput struct {
	To                string `json:"to"`
	ControlEvidenceID int64  `json:"controlEvidenceId"`
}

type ReviseInsightInput struct {
	Statement string `json:"statement"`
	Scope     string `json:"scope"`
}

func (h *handlers) listLearningRecords(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.learning.List(r.Context(), actorFrom(r), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, LearningRecordsResponse{Records: list})
}

func (h *handlers) createLearningRecord(w http.ResponseWriter, r *http.Request) {
	var in learning.LearningRecordInput
	if !readJSON(w, r, &in) {
		return
	}
	rec, created, err := h.learning.Record(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, rec)
}

func (h *handlers) decideLearningRecord(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "learning record")
	if !ok {
		return
	}
	var in learning.LearningDecisionInput
	if !readJSON(w, r, &in) {
		return
	}
	rec, err := h.learning.Decide(r.Context(), actorFrom(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (h *handlers) observeOutcome(w http.ResponseWriter, r *http.Request) {
	var in learning.LearningOutcomeInput
	if !readJSON(w, r, &in) {
		return
	}
	o, err := h.learning.Observe(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (h *handlers) listInsights(w http.ResponseWriter, r *http.Request) {
	list, err := h.learning.ListInsights(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, InsightsResponse{Insights: list})
}

func (h *handlers) proposeInsight(w http.ResponseWriter, r *http.Request) {
	var in learning.MemoryInsightInput
	if !readJSON(w, r, &in) {
		return
	}
	i, err := h.learning.ProposeInsight(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, i)
}

func (h *handlers) promoteInsight(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "insight")
	if !ok {
		return
	}
	var in PromoteInsightInput
	if !readJSON(w, r, &in) {
		return
	}
	i, err := h.learning.Promote(r.Context(), actorFrom(r), id, in.To, in.ControlEvidenceID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, i)
}

func (h *handlers) reviseInsight(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "insight")
	if !ok {
		return
	}
	var in ReviseInsightInput
	if !readJSON(w, r, &in) {
		return
	}
	i, err := h.learning.Revise(r.Context(), actorFrom(r), id, in.Statement, in.Scope)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, i)
}

func (h *handlers) learningFindings(w http.ResponseWriter, r *http.Request) {
	list, err := h.learning.Findings(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, MemoryFindingsResponse{Findings: list})
}

func (h *handlers) learningReport(w http.ResponseWriter, r *http.Request) {
	rep, err := h.learning.Report(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
