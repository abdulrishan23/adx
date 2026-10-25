package api

import (
	"net/http"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/core/policies"
)

type AccountsResponse struct {
	Accounts []connections.Account `json:"accounts"`
}

func (h *handlers) listAccounts(w http.ResponseWriter, r *http.Request) {
	connectionHeaders(w)
	if h.connections == nil {
		// No connection service means no accounts, which is an honest empty
		// list, not a fixture.
		writeJSON(w, http.StatusOK, AccountsResponse{Accounts: []connections.Account{}})
		return
	}
	// ?all=1 is the admin's full list, including accounts that are off.
	list := h.connections.ListAccounts
	if r.URL.Query().Get("all") == "1" {
		list = h.connections.ListAllAccounts
	}
	accounts, err := list(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, AccountsResponse{Accounts: accounts})
}

// AccountEnabledInput is the body of PUT /api/accounts/{id}/enabled.
type AccountEnabledInput struct {
	Enabled bool `json:"enabled"`
}

func (h *handlers) setAccountEnabled(w http.ResponseWriter, r *http.Request) {
	connectionHeaders(w)
	if h.connections == nil {
		h.fail(w, r, core.Conflict("connections are not configured on this server"))
		return
	}
	id, ok := pathID(w, r, "id", "account")
	if !ok {
		return
	}
	var in AccountEnabledInput
	if !readJSON(w, r, &in) {
		return
	}
	account, err := h.connections.SetAccountEnabled(r.Context(), actorFrom(r), id, in.Enabled)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, account)
}

func (h *handlers) getPolicies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	state, err := h.policies.Get(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// PutPolicyInput is the body of PUT /api/policies: the whole next version.
type PutPolicyInput struct {
	Document policies.PolicyDocument `json:"document"`
}

func (h *handlers) putPolicies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var in PutPolicyInput
	if !readJSON(w, r, &in) {
		return
	}
	policy, err := h.policies.Put(r.Context(), actorFrom(r), in.Document)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, policy)
}

type KillSwitchInput struct {
	AccountID int64  `json:"accountId"`
	Engaged   bool   `json:"engaged"`
	Reason    string `json:"reason"`
}

func (h *handlers) putKillSwitch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var in KillSwitchInput
	if !readJSON(w, r, &in) {
		return
	}
	if in.AccountID < 0 {
		h.fail(w, r, core.Invalid("accountId must be 0 for the workspace or a connected account"))
		return
	}
	ks, err := h.policies.SetKillSwitch(r.Context(), actorFrom(r), in.AccountID, in.Engaged, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ks)
}
