package api

import (
	"net/http"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/users"
)

// SessionResponse is GET /api/session. Dev is true for the dev admin, which
// nobody signed in as; User is then the dev person, or null on a server
// without one.
type SessionResponse struct {
	User        *users.User `json:"user"`
	Role        core.Role   `json:"role"`
	WorkspaceID int64       `json:"workspaceId"`
	Workspace   string      `json:"workspace"`
	ExpiresAt   *string     `json:"expiresAt"`
	Dev         bool        `json:"dev"`
}

func (h *handlers) session(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s, ok := sessionFrom(r); ok {
		expires := s.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		writeJSON(w, http.StatusOK, SessionResponse{
			User: &s.User, Role: s.Role, WorkspaceID: s.WorkspaceID, Workspace: s.Workspace, ExpiresAt: &expires,
		})
		return
	}
	actor := actorFrom(r)
	writeJSON(w, http.StatusOK, SessionResponse{User: h.devUser, Role: actor.Role, WorkspaceID: actor.WorkspaceID, Workspace: "Reportage (dev)", Dev: true})
}

// login sends the browser to authentik. These routes sit outside withActor:
// the person has no session yet.
func (h *handlers) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if h.identity == nil {
		h.fail(w, r, core.Conflict("sign-in is not configured on this server"))
		return
	}
	target, err := h.identity.StartSignIn(r.Context(), r.URL.Query().Get("return_to"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *handlers) authCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if h.identity == nil {
		h.fail(w, r, core.Conflict("sign-in is not configured on this server"))
		return
	}
	q := r.URL.Query()
	if len(q["state"]) != 1 || len(q["code"]) > 1 || len(q["error"]) > 1 {
		h.fail(w, r, core.Invalid("sign-in callback parameters are invalid"))
		return
	}
	done, err := h.identity.FinishSignIn(r.Context(), core.ViaWeb, identity.FinishInput{
		State: q.Get("state"), Code: q.Get("code"), ProviderError: q.Get("error"),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	setSessionCookie(w, done.Token, h.dev)
	http.Redirect(w, r, done.ReturnTo, http.StatusSeeOther)
}

// logout ends the ADX session only (D-24). It's a POST so a cross-site image
// tag can't sign people out; the cross-origin protection covers it.
func (h *handlers) logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if cookie, err := r.Cookie(sessionCookie); err == nil && h.identity != nil {
		if err := h.identity.SignOut(r.Context(), cookie.Value); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	clearSessionCookie(w, h.dev)
	w.WriteHeader(http.StatusNoContent)
}
