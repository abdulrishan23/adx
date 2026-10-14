package api

import (
	"crypto/rand"
	"html"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/connections"
)

const connectionCookie = "adx_crm_oauth"

type ConnectionsResponse struct {
	Connections []connections.Connection `json:"connections"`
}

func connectionHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func (h *handlers) connectionAvailable(w http.ResponseWriter, r *http.Request) bool {
	connectionHeaders(w)
	if h.connections == nil {
		h.fail(w, r, core.Conflict("GoHighLevel connection is not configured"))
		return false
	}
	return true
}

// This local form is the connection entry point until the settings screen exists.
func (h *handlers) connectGHLPage(w http.ResponseWriter, r *http.Request) {
	if !h.connectionAvailable(w, r) {
		return
	}
	if !actorFrom(r).AtLeast(core.RoleAdmin) {
		h.fail(w, r, core.ErrForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	locationID := html.EscapeString(r.URL.Query().Get("locationId"))
	_, _ = w.Write([]byte("<!doctype html><html lang=\"en\"><meta charset=\"utf-8\"><title>Connect GoHighLevel</title>" +
		"<h1>Connect GoHighLevel</h1><h2>Private integration token</h2>" +
		"<p>Create a token in your GoHighLevel workspace under Settings &rarr; Private Integrations. " +
		"Select read access to contacts, opportunities, locations and custom fields.</p>" +
		"<form method=\"post\" action=\"/api/connections/crm/private\">" +
		"<p><label>Workspace ID <input name=\"locationId\" required value=\"" + locationID + "\"></label></p>" +
		"<p><label>Private integration token <input type=\"password\" name=\"token\" required autocomplete=\"off\"></label></p>" +
		"<button>Connect and test</button></form>" +
		"<p><a href=\"/api/connections/crm\">View connected locations</a></p></html>"))
}

func (h *handlers) connectPrivateGHL(w http.ResponseWriter, r *http.Request) {
	if !h.connectionAvailable(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<10)
	if err := r.ParseForm(); err != nil || len(r.PostForm["locationId"]) != 1 || len(r.PostForm["token"]) != 1 {
		h.fail(w, r, core.Invalid("a workspace ID and private integration token are required"))
		return
	}
	result, err := h.connections.ConnectPrivate(r.Context(), actorFrom(r), r.PostForm.Get("locationId"), r.PostForm.Get("token"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handlers) startGHL(w http.ResponseWriter, r *http.Request) {
	if !h.connectionAvailable(w, r) {
		return
	}
	binding := ""
	if cookie, err := r.Cookie(connectionCookie); err == nil && len(cookie.Value) >= 32 && len(cookie.Value) <= 128 {
		binding = cookie.Value
	}
	if binding == "" {
		binding = rand.Text() + rand.Text()
	}
	result, err := h.connections.Start(r.Context(), actorFrom(r), binding)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: connectionCookie, Value: binding, Path: "/api/connections/crm",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.TLS != nil,
		MaxAge: int((10 * time.Minute).Seconds()),
	})
	http.Redirect(w, r, result.AuthorizationURL, http.StatusSeeOther)
}

func (h *handlers) finishGHL(w http.ResponseWriter, r *http.Request) {
	if !h.connectionAvailable(w, r) {
		return
	}
	q := r.URL.Query()
	if len(q["state"]) != 1 || len(q["code"]) > 1 || len(q["error"]) > 1 ||
		(q.Get("code") != "" && q.Get("error") != "") {
		h.fail(w, r, core.Invalid("GoHighLevel callback parameters are invalid"))
		return
	}
	binding := ""
	if cookie, err := r.Cookie(connectionCookie); err == nil {
		binding = cookie.Value
	}
	result, err := h.connections.Finish(r.Context(), actorFrom(r), connections.FinishInput{
		State: q.Get("state"), BrowserBinding: binding, Code: q.Get("code"), ProviderError: q.Get("error"),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *handlers) listConnections(w http.ResponseWriter, r *http.Request) {
	if !h.connectionAvailable(w, r) {
		return
	}
	list, err := h.connections.List(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ConnectionsResponse{Connections: list})
}

func (h *handlers) checkGHL(w http.ResponseWriter, r *http.Request) {
	if !h.connectionAvailable(w, r) {
		return
	}
	result, err := h.connections.Check(r.Context(), actorFrom(r), chi.URLParam(r, "locationID"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
