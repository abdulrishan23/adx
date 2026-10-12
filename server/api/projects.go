package api

import (
	"net/http"

	"github.com/WokeWorks/adx/server/core/projects"
)

type ProjectsResponse struct {
	Projects []projects.Project `json:"projects"`
}

func (h *handlers) listProjects(w http.ResponseWriter, r *http.Request) {
	list, err := h.projects.List(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ProjectsResponse{Projects: list})
}

func (h *handlers) createProject(w http.ResponseWriter, r *http.Request) {
	var in projects.CreateInput
	if !readJSON(w, r, &in) {
		return
	}
	p, err := h.projects.Create(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}
