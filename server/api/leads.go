package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/core/runs"
)

type LeadsResponse struct {
	Leads []leads.Lead `json:"leads"`
	// NextAfterID pages: pass it back as after_id for the next page. Null
	// on the last page.
	NextAfterID *int64 `json:"nextAfterId"`
}

func (h *handlers) listLeads(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	accountID, _ := strconv.ParseInt(q.Get("account_id"), 10, 64)
	afterID, _ := strconv.ParseInt(q.Get("after_id"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	list, err := h.leads.List(r.Context(), actorFrom(r), leads.ListFilter{
		AccountID: accountID, Stage: q.Get("stage"), StatusRaw: q.Get("status"), AfterID: afterID, Limit: limit,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := LeadsResponse{Leads: list}
	if len(list) == limit {
		last := list[len(list)-1].ID
		out.NextAfterID = &last
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) leadCoverage(w http.ResponseWriter, r *http.Request) {
	accountID, _ := strconv.ParseInt(r.URL.Query().Get("account_id"), 10, 64)
	cov, err := h.leads.Coverage(r.Context(), actorFrom(r), accountID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cov)
}

// ImportLeadsInput starts a GoHighLevel import. since is only for incremental.
type ImportLeadsInput struct {
	AccountID int64      `json:"accountId"`
	Mode      string     `json:"mode"`
	Since     *time.Time `json:"since"`
}

type StartRunResponse struct {
	Run     runs.Run `json:"run"`
	Created bool     `json:"created"`
}

func (h *handlers) importLeads(w http.ResponseWriter, r *http.Request) {
	var in ImportLeadsInput
	if !readJSON(w, r, &in) {
		return
	}
	if h.runs == nil {
		h.fail(w, r, core.Conflict("background runs are not available on this server"))
		return
	}
	run, created, err := h.leads.StartImport(r.Context(), actorFrom(r), h.runs, in.AccountID, in.Mode, in.Since)
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

func (h *handlers) getFunnel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	state, err := h.leads.GetFunnel(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

type PutFunnelInput struct {
	Document leads.FunnelDocument `json:"document"`
}

func (h *handlers) putFunnel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var in PutFunnelInput
	if !readJSON(w, r, &in) {
		return
	}
	m, err := h.leads.PutFunnel(r.Context(), actorFrom(r), in.Document)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}
