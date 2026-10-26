package api

import (
	"net/http"

	"github.com/WokeWorks/adx/server/core/campaigns"
)

type CampaignsResponse struct {
	Campaigns []campaigns.CampaignSummary `json:"campaigns"`
}

func (h *handlers) listCampaigns(w http.ResponseWriter, r *http.Request) {
	list, err := h.campaigns.List(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, CampaignsResponse{Campaigns: list})
}

func (h *handlers) projectCampaigns(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "projectID", "project")
	if !ok {
		return
	}
	list, err := h.campaigns.ListForProject(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, CampaignsResponse{Campaigns: list})
}

// ProjectsProgressResponse is every project's progress, for the Projects
// page and the switcher.
type ProjectsProgressResponse struct {
	Projects []campaigns.ProjectProgress `json:"projects"`
}

func (h *handlers) projectsProgress(w http.ResponseWriter, r *http.Request) {
	list, err := h.campaigns.ProgressAll(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ProjectsProgressResponse{Projects: list})
}

func (h *handlers) projectProgress(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "projectID", "project")
	if !ok {
		return
	}
	progress, err := h.campaigns.Progress(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, progress)
}

func (h *handlers) createCampaign(w http.ResponseWriter, r *http.Request) {
	var in campaigns.CampaignInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.campaigns.Create(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *handlers) getCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "campaignID", "campaign")
	if !ok {
		return
	}
	out, err := h.campaigns.Get(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) updateCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "campaignID", "campaign")
	if !ok {
		return
	}
	var in campaigns.CampaignInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := h.campaigns.Update(r.Context(), actorFrom(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
