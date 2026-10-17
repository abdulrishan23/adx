package api

import (
	"net/http"

	"github.com/WokeWorks/adx/server/core"
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
