package api

import (
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core/launch"
)

// CoverageResponse lists the campaigns ADX has read with the plan behind
// each and where its launch stands.
type CoverageResponse struct {
	Campaigns []launch.CampaignCoverage `json:"campaigns"`
}

// LaunchPickInput is a person's choice for a plan line that matched more
// than one running campaign.
type LaunchPickInput struct {
	PlanID      int64 `json:"planId"`
	PlanVersion int64 `json:"planVersion"`
	Line        int   `json:"line"`
	CampaignID  int64 `json:"campaignId"`
}

// launchResolution is GET /api/launch/resolution?plan_id=&version=: what
// launching the version would do today, line by line.
func (h *handlers) launchResolution(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	planID, err1 := strconv.ParseInt(r.URL.Query().Get("plan_id"), 10, 64)
	version, err2 := strconv.ParseInt(r.URL.Query().Get("version"), 10, 64)
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "plan_id and version are required")
		return
	}
	res, err := h.launch.Resolve(r.Context(), actorFrom(r), planID, version)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// launchPick is POST /api/launch/picks: records the choice and answers with
// the resolution it applies to.
func (h *handlers) launchPick(w http.ResponseWriter, r *http.Request) {
	var in LaunchPickInput
	if !readJSON(w, r, &in) {
		return
	}
	res, err := h.launch.Pick(r.Context(), actorFrom(r), in.PlanID, in.PlanVersion, in.Line, in.CampaignID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// launchSubmit is POST /api/launch/submit: writes the resolution out as
// proposals and intents under the version's envelopes.
func (h *handlers) launchSubmit(w http.ResponseWriter, r *http.Request) {
	var in launch.SubmitInput
	if !readJSON(w, r, &in) {
		return
	}
	res, err := h.launch.Submit(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, res)
}

// launchCoverage is GET /api/launch/coverage?project=: every campaign ADX
// has read (a project's, when given) with whether an accepted plan stands
// behind it, which line of which version, and where its launch stands.
func (h *handlers) launchCoverage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var project int64
	if raw := r.URL.Query().Get("project"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "project must be a positive integer")
			return
		}
		project = id
	}
	list, err := h.launch.Campaigns(r.Context(), actorFrom(r), project)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, CoverageResponse{Campaigns: list})
}
