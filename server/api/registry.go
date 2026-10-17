package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core/registry"
)

type RegistryEntriesResponse struct {
	Entries []registry.RegistryEntry `json:"entries"`
}

type AgentCapabilitiesResponse struct {
	Capabilities []registry.AgentCapability `json:"capabilities"`
}

type SaveSkillInput struct {
	Key       string                 `json:"key"`
	Document  registry.SkillDocument `json:"document"`
	FromModel bool                   `json:"fromModel"`
}

type SaveAgentInput struct {
	Key       string                 `json:"key"`
	Document  registry.AgentDocument `json:"document"`
	FromModel bool                   `json:"fromModel"`
}

type SaveWorkflowInput struct {
	Key       string                    `json:"key"`
	Document  registry.WorkflowDocument `json:"document"`
	FromModel bool                      `json:"fromModel"`
}

type ValidationResponse struct {
	Problems []string `json:"problems"`
}

type CheckOutputInput struct {
	Output json.RawMessage `json:"output"`
}

func (h *handlers) capabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, AgentCapabilitiesResponse{Capabilities: h.registry.Catalog()})
}

func (h *handlers) listRegistry(w http.ResponseWriter, r *http.Request) {
	list, err := h.registry.List(r.Context(), actorFrom(r), chi.URLParam(r, "kind"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, RegistryEntriesResponse{Entries: list})
}

func (h *handlers) saveSkill(w http.ResponseWriter, r *http.Request) {
	var in SaveSkillInput
	if !readJSON(w, r, &in) {
		return
	}
	e, err := h.registry.SaveSkill(r.Context(), actorFrom(r), in.Key, in.Document, in.FromModel)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *handlers) saveAgent(w http.ResponseWriter, r *http.Request) {
	var in SaveAgentInput
	if !readJSON(w, r, &in) {
		return
	}
	e, err := h.registry.SaveAgent(r.Context(), actorFrom(r), in.Key, in.Document, in.FromModel)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *handlers) saveWorkflow(w http.ResponseWriter, r *http.Request) {
	var in SaveWorkflowInput
	if !readJSON(w, r, &in) {
		return
	}
	e, err := h.registry.SaveWorkflow(r.Context(), actorFrom(r), in.Key, in.Document, in.FromModel)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *handlers) validateWorkflow(w http.ResponseWriter, r *http.Request) {
	var doc registry.WorkflowDocument
	if !readJSON(w, r, &doc) {
		return
	}
	problems, err := h.registry.ValidateWorkflow(r.Context(), actorFrom(r), doc)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if problems == nil {
		problems = []string{}
	}
	writeJSON(w, http.StatusOK, ValidationResponse{Problems: problems})
}

func (h *handlers) activateRegistry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "registry entry")
	if !ok {
		return
	}
	e, err := h.registry.Activate(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *handlers) retireRegistry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "registry entry")
	if !ok {
		return
	}
	e, err := h.registry.Retire(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *handlers) checkSkillOutput(w http.ResponseWriter, r *http.Request) {
	var in CheckOutputInput
	if !readJSON(w, r, &in) {
		return
	}
	problems, err := h.registry.CheckOutput(r.Context(), actorFrom(r), chi.URLParam(r, "key"), in.Output)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if problems == nil {
		problems = []string{}
	}
	writeJSON(w, http.StatusOK, ValidationResponse{Problems: problems})
}
