package api

import (
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/measurement"
)

const coreRoleMarketer = core.RoleMarketer

type ChecksResponse struct {
	Checks []measurement.MeasurementCheck `json:"checks"`
}

type MeasurementEventsResponse struct {
	Events    []measurement.MeasurementEvent `json:"events"`
	Enquiries int64                          `json:"enquiries"`
}

type TrackedURLInput struct {
	Base string          `json:"base"`
	UTM  measurement.UTM `json:"utm"`
}

type TrackedURLResponse struct {
	URL string `json:"url"`
}

func (h *handlers) measurementCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, measurement.GetCatalog())
}

func (h *handlers) runMeasurementCheck(w http.ResponseWriter, r *http.Request) {
	var in measurement.MeasurementCheckInput
	if !readJSON(w, r, &in) {
		return
	}
	c, err := h.measurement.RunCheck(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *handlers) listMeasurementChecks(w http.ResponseWriter, r *http.Request) {
	planID, _ := strconv.ParseInt(r.URL.Query().Get("plan_id"), 10, 64)
	version, _ := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	list, err := h.measurement.ListChecks(r.Context(), actorFrom(r), planID, version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ChecksResponse{Checks: list})
}

func (h *handlers) launchReadiness(w http.ResponseWriter, r *http.Request) {
	planID, err1 := strconv.ParseInt(r.URL.Query().Get("plan_id"), 10, 64)
	version, err2 := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "plan_id and version are required")
		return
	}
	rd, err := h.measurement.Readiness(r.Context(), actorFrom(r), planID, version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rd)
}

func (h *handlers) measurementEvents(w http.ResponseWriter, r *http.Request) {
	actor := actorFrom(r)
	pageID, _ := strconv.ParseInt(r.URL.Query().Get("page_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if actor.AtLeast(coreRoleMarketer) {
		if _, err := h.measurement.RecordPending(r.Context(), actor); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	list, err := h.measurement.ListEvents(r.Context(), actor, pageID, limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	n, err := h.measurement.CountEnquiries(r.Context(), actor)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, MeasurementEventsResponse{Events: list, Enquiries: n})
}

func (h *handlers) trackedURL(w http.ResponseWriter, r *http.Request) {
	var in TrackedURLInput
	if !readJSON(w, r, &in) {
		return
	}
	u, err := measurement.TrackedURL(in.Base, in.UTM)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, TrackedURLResponse{URL: u})
}
