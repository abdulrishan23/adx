package api

import (
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
)

type ProposalsResponse struct {
	Proposals []proposals.Proposal `json:"proposals"`
}

type ProposalResponse struct {
	Proposal  proposals.Proposal      `json:"proposal"`
	Authority policies.PolicyDecision `json:"authority"`
}

type DecisionInput struct {
	Note string `json:"note"`
}

type ExecuteInput struct {
	IdempotencyKey string `json:"idempotencyKey"`
}

type CancelInput struct {
	Reason string `json:"reason"`
}

type ExecutionsResponse struct {
	Executions []proposals.Intent `json:"executions"`
}

type EnvelopesResponse struct {
	Envelopes []proposals.Envelope `json:"envelopes"`
}

type OpenEnvelopesInput struct {
	PlanID            int64            `json:"planId"`
	PlanVersion       int64            `json:"planVersion"`
	AccountByPlatform map[string]int64 `json:"accountByPlatform"`
}

func (h *handlers) listProposals(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.proposals.List(r.Context(), actorFrom(r), r.URL.Query().Get("state"), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ProposalsResponse{Proposals: list})
}

func (h *handlers) createProposal(w http.ResponseWriter, r *http.Request) {
	var in proposals.ProposalInput
	if !readJSON(w, r, &in) {
		return
	}
	p, created, err := h.proposals.Propose(r.Context(), actorFrom(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, p)
}

func (h *handlers) getProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "proposal")
	if !ok {
		return
	}
	actor := actorFrom(r)
	p, err := h.proposals.Get(r.Context(), actor, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	authority, err := h.proposals.Authority(r.Context(), actor, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ProposalResponse{Proposal: p, Authority: authority})
}

func (h *handlers) decideProposal(decision string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id", "proposal")
		if !ok {
			return
		}
		var in DecisionInput
		if !readJSON(w, r, &in) {
			return
		}
		var (
			p   proposals.Proposal
			err error
		)
		if decision == "approve" {
			p, err = h.proposals.Approve(r.Context(), actorFrom(r), id, in.Note)
		} else {
			p, err = h.proposals.Reject(r.Context(), actorFrom(r), id, in.Note)
		}
		if err != nil {
			h.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func (h *handlers) executeProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "proposal")
	if !ok {
		return
	}
	var in ExecuteInput
	if !readJSON(w, r, &in) {
		return
	}
	intent, created, err := h.proposals.Execute(r.Context(), actorFrom(r), id, in.IdempotencyKey)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	writeJSON(w, status, intent)
}

func (h *handlers) listExecutions(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := h.proposals.ListIntents(r.Context(), actorFrom(r), r.URL.Query().Get("state"), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ExecutionsResponse{Executions: list})
}

func (h *handlers) getExecution(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "execution")
	if !ok {
		return
	}
	intent, err := h.proposals.GetIntent(r.Context(), actorFrom(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, intent)
}

func (h *handlers) cancelExecution(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id", "execution")
	if !ok {
		return
	}
	var in CancelInput
	if !readJSON(w, r, &in) {
		return
	}
	intent, err := h.proposals.Cancel(r.Context(), actorFrom(r), id, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, intent)
}

func (h *handlers) listEnvelopes(w http.ResponseWriter, r *http.Request) {
	list, err := h.proposals.ListEnvelopes(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, EnvelopesResponse{Envelopes: list})
}

func (h *handlers) openEnvelopes(w http.ResponseWriter, r *http.Request) {
	var in OpenEnvelopesInput
	if !readJSON(w, r, &in) {
		return
	}
	list, err := h.proposals.OpenEnvelopes(r.Context(), actorFrom(r), in.PlanID, in.PlanVersion, in.AccountByPlatform)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, EnvelopesResponse{Envelopes: list})
}
