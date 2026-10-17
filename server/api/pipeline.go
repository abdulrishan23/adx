package api

import (
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core/pipeline"
)

type OpportunitiesResponse struct {
	Opportunities []pipeline.Opportunity `json:"opportunities"`
}

type TasksResponse struct {
	Tasks []pipeline.FollowUpTask `json:"tasks"`
}

type ImportsResponse struct {
	Imports []pipeline.ImportBatch `json:"imports"`
}

type ExportResponse struct {
	Rows []pipeline.ExportRow `json:"rows"`
}

type OpenOpportunityInput struct {
	EnquiryID int64 `json:"enquiryId"`
}

type SetStageInput struct {
	Stage   string `json:"stage"`
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}

type SetValueInput struct {
	ValueMinor int64  `json:"valueMinor"`
	Currency   string `json:"currency"`
	Version    int64  `json:"version"`
}

type AssignInput struct {
	UserID int64  `json:"userId"`
	Reason string `json:"reason"`
}

type PreviewImportInput struct {
	Source string `json:"source"`
}

type CompleteTaskInput struct {
	Canceled bool `json:"canceled"`
}

func (h *handlers) getPipelineConfig(w http.ResponseWriter, r *http.Request) {
	st, err := h.pipeline.GetConfig(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *handlers) putPipelineConfig(w http.ResponseWriter, r *http.Request) {
	var doc pipeline.PipelineDocument
	if !readJSON(w, r, &doc) {
		return
	}
	c, err := h.pipeline.PutConfig(r.Context(), actorFrom(r), doc)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *handlers) listOpportunities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	owner, _ := strconv.ParseInt(q.Get("owner"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	list, err := h.pipeline.List(r.Context(), actorFrom(r), q.Get("stage"), q.Get("state"), owner, limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, OpportunitiesResponse{Opportunities: list})
}

func (h *handlers) openOpportunity(w http.ResponseWriter, r *http.Request) {
	var in OpenOpportunityInput
	if !readJSON(w, r, &in) {
		return
	}
	o, created, err := h.pipeline.Open(r.Context(), actorFrom(r), in.EnquiryID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, o)
}

func (h *handlers) getOpportunity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "opportunity")
	if !ok {
		return
	}
	d, err := h.pipeline.Get(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) setOpportunityStage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "opportunity")
	if !ok {
		return
	}
	var in SetStageInput
	if !readJSON(w, r, &in) {
		return
	}
	o, err := h.pipeline.SetStage(r.Context(), actorFrom(r), id, in.Stage, in.Version, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *handlers) setOpportunityValue(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "opportunity")
	if !ok {
		return
	}
	var in SetValueInput
	if !readJSON(w, r, &in) {
		return
	}
	o, err := h.pipeline.SetValue(r.Context(), actorFrom(r), id, in.ValueMinor, in.Currency, in.Version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *handlers) assignOpportunity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "opportunity")
	if !ok {
		return
	}
	var in AssignInput
	if !readJSON(w, r, &in) {
		return
	}
	a, err := h.pipeline.Assign(r.Context(), actorFrom(r), id, in.UserID, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (h *handlers) recordInteraction(w http.ResponseWriter, r *http.Request) {
	var in pipeline.InteractionInput
	if !readJSON(w, r, &in) {
		return
	}
	i, err := h.pipeline.RecordInteraction(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, i)
}

func (h *handlers) listTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	assignee, _ := strconv.ParseInt(q.Get("assignee"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	list, err := h.pipeline.ListTasks(r.Context(), actorFrom(r), q.Get("state"), assignee, limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, TasksResponse{Tasks: list})
}

func (h *handlers) createTask(w http.ResponseWriter, r *http.Request) {
	var in pipeline.TaskInput
	if !readJSON(w, r, &in) {
		return
	}
	t, created, err := h.pipeline.CreateTask(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, t)
}

func (h *handlers) completeTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "task")
	if !ok {
		return
	}
	var in CompleteTaskInput
	if !readJSON(w, r, &in) {
		return
	}
	t, err := h.pipeline.CompleteTask(r.Context(), actorFrom(r), id, in.Canceled)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *handlers) previewImport(w http.ResponseWriter, r *http.Request) {
	var in PreviewImportInput
	if !readJSON(w, r, &in) {
		return
	}
	b, err := h.pipeline.PreviewImport(r.Context(), actorFrom(r), in.Source)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (h *handlers) applyImport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "import")
	if !ok {
		return
	}
	b, err := h.pipeline.ApplyImport(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *handlers) getImport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "import")
	if !ok {
		return
	}
	b, err := h.pipeline.GetImport(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (h *handlers) listImports(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.pipeline.ListImports(r.Context(), actorFrom(r), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ImportsResponse{Imports: list})
}

func (h *handlers) exportPipeline(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pipeline.Export(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ExportResponse{Rows: rows})
}
