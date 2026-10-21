package api

import (
	"net/http"

	"github.com/WokeWorks/adx/server/core/fronts"
)

// SnoozeAlertInput is how long an alert card stays off the fronts.
type SnoozeAlertInput struct {
	Days int `json:"days"`
}

// DismissAlertInput is why a person dismissed an alert card.
type DismissAlertInput struct {
	Reason string `json:"reason"`
}

// desk is GET /api/fronts: Home's cards in order, the jobs under way and
// the prompt templates.
func (h *handlers) desk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	d, err := h.fronts.Desk(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// inbox is GET /api/inbox?show=all|needs|updates&door=any|studio|perform|convert.
func (h *handlers) inbox(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	q := r.URL.Query()
	in, err := h.fronts.Inbox(r.Context(), actorFrom(r), fronts.InboxFilter{Show: q.Get("show"), Door: fronts.Door(q.Get("door"))})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}
