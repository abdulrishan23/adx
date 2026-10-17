package api

import (
	"crypto/rand"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/core/reporting"
)

func validAdsProvider(p string) bool {
	return p == connections.ProviderGoogleAds || p == connections.ProviderMeta
}

// startAds sends an admin to the ad platform's consent screen. The browser
// binding cookie ties the callback to this browser, as the GoHighLevel flow
// does.
func (h *handlers) startAds(w http.ResponseWriter, r *http.Request) {
	connectionHeaders(w)
	provider := chi.URLParam(r, "provider")
	if !validAdsProvider(provider) {
		h.fail(w, r, core.NotFound("unknown provider"))
		return
	}
	if h.connections == nil {
		h.fail(w, r, core.Conflict("connections are not configured on this server"))
		return
	}
	binding := ""
	if cookie, err := r.Cookie(connectionCookie); err == nil && len(cookie.Value) >= 32 && len(cookie.Value) <= 128 {
		binding = cookie.Value
	}
	if binding == "" {
		binding = rand.Text() + rand.Text()
	}
	result, err := h.connections.StartAds(r.Context(), actorFrom(r), provider, binding)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: connectionCookie, Value: binding, Path: "/api/connections",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: !h.dev, MaxAge: int((10 * time.Minute).Seconds()),
	})
	http.Redirect(w, r, result.AuthorizationURL, http.StatusSeeOther)
}

func (h *handlers) finishAds(w http.ResponseWriter, r *http.Request) {
	connectionHeaders(w)
	provider := chi.URLParam(r, "provider")
	if !validAdsProvider(provider) {
		h.fail(w, r, core.NotFound("unknown provider"))
		return
	}
	if h.connections == nil {
		h.fail(w, r, core.Conflict("connections are not configured on this server"))
		return
	}
	q := r.URL.Query()
	if len(q["state"]) != 1 || len(q["code"]) > 1 || len(q["error"]) > 1 {
		h.fail(w, r, core.Invalid("callback parameters are invalid"))
		return
	}
	binding := ""
	if cookie, err := r.Cookie(connectionCookie); err == nil {
		binding = cookie.Value
	}
	accounts, err := h.connections.FinishAds(r.Context(), actorFrom(r), provider, connections.FinishInput{
		State: q.Get("state"), BrowserBinding: binding, Code: q.Get("code"), ProviderError: q.Get("error"),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	// The browser came from the provider; land it on the connections screen.
	if r.Header.Get("Accept") == "application/json" {
		writeJSON(w, http.StatusOK, AccountsResponse{Accounts: accounts})
		return
	}
	http.Redirect(w, r, "/connections?connected="+provider, http.StatusSeeOther)
}

type RefreshInput struct {
	AccountID int64         `json:"accountId"`
	From      reporting.Day `json:"from"`
	To        reporting.Day `json:"to"`
}

func (h *handlers) refreshAnalytics(w http.ResponseWriter, r *http.Request) {
	var in RefreshInput
	if !readJSON(w, r, &in) {
		return
	}
	if h.runs == nil {
		h.fail(w, r, core.Conflict("background runs are not available on this server"))
		return
	}
	run, created, err := h.reporting.StartRefresh(r.Context(), actorFrom(r), h.runs, in.AccountID, in.From, in.To)
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

type FreshnessResponse struct {
	Accounts []reporting.Freshness `json:"accounts"`
}

func (h *handlers) analyticsFreshness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	list, err := h.reporting.ListFreshness(r.Context(), actorFrom(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, FreshnessResponse{Accounts: list})
}
