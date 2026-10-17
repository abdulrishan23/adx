package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core/alerts"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/reports"
)

type SchedulesResponse struct {
	Schedules []reports.ReportSchedule `json:"schedules"`
}

type ReportsResponse struct {
	Reports []reports.Report `json:"reports"`
}

func (h *handlers) listSchedules(w http.ResponseWriter, r *http.Request) {
	list, err := h.reports.ListSchedules(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, SchedulesResponse{Schedules: list})
}

func (h *handlers) createSchedule(w http.ResponseWriter, r *http.Request) {
	var in reports.ReportScheduleInput
	if !readJSON(w, r, &in) {
		return
	}
	s, err := h.reports.CreateSchedule(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

func (h *handlers) updateSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid schedule id")
		return
	}
	var in reports.ReportScheduleInput
	if !readJSON(w, r, &in) {
		return
	}
	s, err := h.reports.UpdateSchedule(r.Context(), actorFrom(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

type GenerateReportResponse struct {
	Report  reports.Report `json:"report"`
	Created bool           `json:"created"`
}

func (h *handlers) generateScheduled(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid schedule id")
		return
	}
	rep, created, err := h.reports.Generate(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, GenerateReportResponse{Report: rep, Created: created})
}

type AdHocReportInput struct {
	From   reporting.Day        `json:"from"`
	To     reporting.Day        `json:"to"`
	Params reports.ReportParams `json:"params"`
}

func (h *handlers) generateAdHoc(w http.ResponseWriter, r *http.Request) {
	var in AdHocReportInput
	if !readJSON(w, r, &in) {
		return
	}
	rep, created, err := h.reports.GenerateAdHoc(r.Context(), actorFrom(r), in.From, in.To, in.Params)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, GenerateReportResponse{Report: rep, Created: created})
}

func (h *handlers) listReports(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.reports.List(r.Context(), actorFrom(r), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ReportsResponse{Reports: list})
}

func (h *handlers) getReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid report id")
		return
	}
	rep, err := h.reports.Get(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

type AlertsResponse struct {
	Alerts []alerts.Alert `json:"alerts"`
}

func (h *handlers) listAlerts(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.alerts.List(r.Context(), actorFrom(r), r.URL.Query().Get("state"), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, AlertsResponse{Alerts: list})
}

func (h *handlers) alertAction(action func(*alerts.Alerts) func(r *http.Request, id int64) (alerts.Alert, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid alert id")
			return
		}
		a, err := action(h.alerts)(r, id)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}
}

func (h *handlers) getAlertConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	state, err := h.alerts.GetConfig(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

type PutAlertConfigInput struct {
	Document alerts.AlertDocument `json:"document"`
}

func (h *handlers) putAlertConfig(w http.ResponseWriter, r *http.Request) {
	var in PutAlertConfigInput
	if !readJSON(w, r, &in) {
		return
	}
	cfg, err := h.alerts.PutConfig(r.Context(), actorFrom(r), in.Document)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, cfg)
}

type EvaluateAlertsInput struct {
	Day reporting.Day `json:"day"`
}

func (h *handlers) evaluateAlerts(w http.ResponseWriter, r *http.Request) {
	var in EvaluateAlertsInput
	if !readJSON(w, r, &in) {
		return
	}
	ev, err := h.alerts.Evaluate(r.Context(), actorFrom(r), in.Day)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ev)
}
