package api

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core/plans"
)

type PlansResponse struct {
	Plans     []plans.PlanSummary `json:"plans"`
	Channels  []plans.ChannelInfo `json:"channels"`
	Templates plans.TemplateGate  `json:"templates"`
}

type ValidatePlanInput struct {
	BrandID  int64              `json:"brandId"`
	Document plans.PlanDocument `json:"document"`
}

func (h *handlers) listPlans(w http.ResponseWriter, r *http.Request) {
	list, err := h.plans.List(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, PlansResponse{Plans: list, Channels: plans.SortedChannels(), Templates: h.plans.Templates()})
}

func (h *handlers) createPlan(w http.ResponseWriter, r *http.Request) {
	var in plans.PlanInput
	if !readJSON(w, r, &in) {
		return
	}
	d, err := h.plans.Create(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (h *handlers) getPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "plan")
	if !ok {
		return
	}
	d, err := h.plans.Get(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) savePlanVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "plan")
	if !ok {
		return
	}
	var in plans.VersionInput
	if !readJSON(w, r, &in) {
		return
	}
	v, err := h.plans.SaveVersion(r.Context(), actorFrom(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *handlers) planVersionIDs(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	id, ok := pathID(w, r, "id", "plan")
	if !ok {
		return 0, 0, false
	}
	version, err := strconv.ParseInt(chi.URLParam(r, "version"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid version")
		return 0, 0, false
	}
	return id, version, true
}

func (h *handlers) acceptPlanVersion(w http.ResponseWriter, r *http.Request) {
	id, version, ok := h.planVersionIDs(w, r)
	if !ok {
		return
	}
	v, err := h.plans.Accept(r.Context(), actorFrom(r), id, version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *handlers) exportPlanVersion(w http.ResponseWriter, r *http.Request) {
	id, version, ok := h.planVersionIDs(w, r)
	if !ok {
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}
	data, ct, err := h.plans.Export(r.Context(), actorFrom(r), id, version, format)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename=\"plan-"+strconv.FormatInt(id, 10)+"-v"+strconv.FormatInt(version, 10)+"."+format+"\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *handlers) importPlanVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "plan")
	if !ok {
		return
	}
	format := r.URL.Query().Get("format")
	data, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read the upload")
		return
	}
	v, err := h.plans.Import(r.Context(), actorFrom(r), id, format, data, r.URL.Query().Get("note"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *handlers) generatePlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "plan")
	if !ok {
		return
	}
	var in plans.GenerateInput
	if !readJSON(w, r, &in) {
		return
	}
	g, err := h.plans.Generate(r.Context(), actorFrom(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *handlers) diffPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "plan")
	if !ok {
		return
	}
	from, err1 := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, err2 := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "from and to must be version numbers")
		return
	}
	d, err := h.plans.Diff(r.Context(), actorFrom(r), id, from, to)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) validatePlan(w http.ResponseWriter, r *http.Request) {
	var in ValidatePlanInput
	if !readJSON(w, r, &in) {
		return
	}
	writeJSON(w, http.StatusOK, h.plans.Validate(r.Context(), actorFrom(r), in.BrandID, in.Document))
}
