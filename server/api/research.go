package api

import (
	"net/http"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/reports"
	"github.com/WokeWorks/adx/server/core/research"
)

type BriefsResponse struct {
	Briefs []research.Brief `json:"briefs"`
}

type ResolveCompetitorInput struct {
	Name    string `json:"name"`
	BrandID int64  `json:"brandId"`
}

type FindingsResponse struct {
	Findings []research.Finding `json:"findings"`
}

type ResearchSchedulesResponse struct {
	Schedules []research.Schedule `json:"schedules"`
}

type ResearchReportsResponse struct {
	Reports []research.ComparativeReport `json:"reports"`
}

type GenerateReportInput struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type HypothesisReviewInput struct {
	Key    string `json:"key"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

type MarketEventsResponse struct {
	Events []research.MarketEvent `json:"events"`
}

func (h *handlers) listBriefs(w http.ResponseWriter, r *http.Request) {
	list, err := h.research.ListBriefs(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, BriefsResponse{Briefs: list})
}

func (h *handlers) createBrief(w http.ResponseWriter, r *http.Request) {
	var in research.BriefInput
	if !readJSON(w, r, &in) {
		return
	}
	d, err := h.research.CreateBrief(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (h *handlers) getBrief(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "brief")
	if !ok {
		return
	}
	d, err := h.research.GetBrief(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) startResearchRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "brief")
	if !ok {
		return
	}
	if h.runs == nil {
		h.fail(w, r, core.Conflict("background runs are not available on this server"))
		return
	}
	run, created, err := h.research.StartRun(r.Context(), actorFrom(r), h.runs, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	writeJSON(w, status, StartRunResponse{Run: run, Created: created})
}

func (h *handlers) resolveCompetitor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "brief")
	if !ok {
		return
	}
	var in ResolveCompetitorInput
	if !readJSON(w, r, &in) {
		return
	}
	d, err := h.research.ResolveCompetitor(r.Context(), actorFrom(r), id, in.Name, in.BrandID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) listFindings(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "brief")
	if !ok {
		return
	}
	scope := research.Scope(r.URL.Query().Get("scope"))
	if scope != "" && scope != research.ScopeAccount && scope != research.ScopePublic {
		writeError(w, http.StatusBadRequest, "scope must be account or public")
		return
	}
	list, err := h.research.ListFindings(r.Context(), actorFrom(r), id, scope)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, FindingsResponse{Findings: list})
}

func (h *handlers) researchEvidence(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "evidence")
	if !ok {
		return
	}
	ev, err := h.research.Evidence(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

func (h *handlers) listResearchSchedules(w http.ResponseWriter, r *http.Request) {
	list, err := h.research.ListSchedules(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ResearchSchedulesResponse{Schedules: list})
}

func (h *handlers) createResearchSchedule(w http.ResponseWriter, r *http.Request) {
	var in research.ScheduleInput
	if !readJSON(w, r, &in) {
		return
	}
	s, err := h.research.CreateSchedule(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

func (h *handlers) updateResearchSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "schedule")
	if !ok {
		return
	}
	var in research.ScheduleInput
	if !readJSON(w, r, &in) {
		return
	}
	s, err := h.research.UpdateSchedule(r.Context(), actorFrom(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *handlers) listResearchReports(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "brief")
	if !ok {
		return
	}
	list, err := h.research.ListReports(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ResearchReportsResponse{Reports: list})
}

// generateResearchReport writes the comparative report for a period now;
// with no period it takes the last full month.
func (h *handlers) generateResearchReport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "brief")
	if !ok {
		return
	}
	var in GenerateReportInput
	if !readJSON(w, r, &in) {
		return
	}
	from, to := reporting.Day(in.From), reporting.Day(in.To)
	if in.From == "" || in.To == "" {
		from, to = reports.Period(reports.Monthly, time.Now())
	}
	rep, created, err := h.research.GenerateReport(r.Context(), actorFrom(r), id, 0, from, to)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, rep)
}

func (h *handlers) getResearchReport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "report")
	if !ok {
		return
	}
	rep, err := h.research.GetReport(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (h *handlers) reviewHypothesis(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "report")
	if !ok {
		return
	}
	var in HypothesisReviewInput
	if !readJSON(w, r, &in) {
		return
	}
	rep, err := h.research.ReviewHypothesis(r.Context(), actorFrom(r), id, in.Key, in.Status, in.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (h *handlers) listMarketEvents(w http.ResponseWriter, r *http.Request) {
	list, err := h.research.ListMarketEvents(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, MarketEventsResponse{Events: list})
}

func (h *handlers) recordMarketEvent(w http.ResponseWriter, r *http.Request) {
	var in research.MarketEventInput
	if !readJSON(w, r, &in) {
		return
	}
	ev, err := h.research.RecordMarketEvent(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, ev)
}
