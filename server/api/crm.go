package api

import (
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core/crm"
)

type PeopleResponse struct {
	People []crm.PersonSummary `json:"people"`
}

type EnquiriesResponse struct {
	Enquiries []crm.Enquiry `json:"enquiries"`
}

type CandidatesResponse struct {
	Candidates []crm.Candidate `json:"candidates"`
}

type DecideCandidateInput struct {
	Accept bool   `json:"accept"`
	Note   string `json:"note"`
}

type MergeInput struct {
	IntoID int64  `json:"intoId"`
	Reason string `json:"reason"`
}

type ReasonInput struct {
	Reason string `json:"reason"`
}

type ManualLinkInput struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

type StatedAttributeInput struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

type SyncResponse struct {
	Linked int `json:"linked"`
}

func (h *handlers) listPeople(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.crm.List(r.Context(), actorFrom(r), r.URL.Query().Get("q"), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, PeopleResponse{People: list})
}

func (h *handlers) createPerson(w http.ResponseWriter, r *http.Request) {
	var in crm.PersonInput
	if !readJSON(w, r, &in) {
		return
	}
	d, err := h.crm.Create(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (h *handlers) getPerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "person")
	if !ok {
		return
	}
	d, err := h.crm.Get(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) mergePerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "person")
	if !ok {
		return
	}
	var in MergeInput
	if !readJSON(w, r, &in) {
		return
	}
	d, err := h.crm.Merge(r.Context(), actorFrom(r), id, in.IntoID, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) unmergePerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "person")
	if !ok {
		return
	}
	var in ReasonInput
	if !readJSON(w, r, &in) {
		return
	}
	d, err := h.crm.Unmerge(r.Context(), actorFrom(r), id, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) enrichPerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "person")
	if !ok {
		return
	}
	d, err := h.crm.Enrich(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) setStatedAttribute(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "person")
	if !ok {
		return
	}
	var in StatedAttributeInput
	if !readJSON(w, r, &in) {
		return
	}
	d, err := h.crm.SetStated(r.Context(), actorFrom(r), id, in.Key, in.Value, in.Source)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) linkPerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "person")
	if !ok {
		return
	}
	var in ManualLinkInput
	if !readJSON(w, r, &in) {
		return
	}
	l, err := h.crm.LinkManually(r.Context(), actorFrom(r), id, in.Kind, in.Ref)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (h *handlers) reverseLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "link")
	if !ok {
		return
	}
	var in ReasonInput
	if !readJSON(w, r, &in) {
		return
	}
	l, err := h.crm.ReverseLink(r.Context(), actorFrom(r), id, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (h *handlers) listCandidates(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.crm.OpenCandidates(r.Context(), actorFrom(r), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, CandidatesResponse{Candidates: list})
}

func (h *handlers) decideCandidate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "candidate")
	if !ok {
		return
	}
	var in DecideCandidateInput
	if !readJSON(w, r, &in) {
		return
	}
	d, err := h.crm.Decide(r.Context(), actorFrom(r), id, in.Accept, in.Note)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *handlers) listEnquiries(w http.ResponseWriter, r *http.Request) {
	projectID, _ := strconv.ParseInt(r.URL.Query().Get("project_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.crm.ListEnquiries(r.Context(), actorFrom(r), projectID, limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, EnquiriesResponse{Enquiries: list})
}

func (h *handlers) syncCRM(w http.ResponseWriter, r *http.Request) {
	n, err := h.crm.Sync(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, SyncResponse{Linked: n})
}
