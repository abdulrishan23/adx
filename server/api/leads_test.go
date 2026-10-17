package api_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: the funnel saved by a non-admin, an import started by a viewer, an
// import for an unconnected account answering 500 instead of 404, and the
// unconfigured funnel not reaching the screen.
func TestLeadsRoutes(t *testing.T) {
	db := storetest.Open(t)
	authn := userstest.NewAuthentik()
	people := users.New(db, authn)
	oidc := &apiOIDC{}
	id := identity.New(db, people, oidc, identity.Config{AllowedGroup: "adx-users"})
	e := &authEnv{db: db, users: people, authn: authn, oidc: oidc}
	l := leads.New(db, nil, nil, evidence.New(db, nil), leads.Config{})
	e.srv = httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Identity: id, Leads: l, Runs: runs.New(db, nil, runs.Config{})}))
	t.Cleanup(e.srv.Close)
	e.addAndPoint(t, "ceo@reportage.ae", core.RoleViewer)
	viewer := e.signIn(t)
	e.addAndPoint(t, "sara@reportage.ae", core.RoleMarketer)
	marketer := e.signIn(t)
	e.addAndPoint(t, "cmo@reportage.ae", core.RoleAdmin)
	admin := e.signIn(t)

	res, body := get(t, e.srv, "/api/leads", viewer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "{\"leads\":[],\"nextAfterId\":null}\n", body)
	res, body = get(t, e.srv, "/api/leads/coverage", viewer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"funnelConfigured":false`)
	res, body = get(t, e.srv, "/api/settings/funnel", viewer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"configured":false`)

	send := func(method string, cookie *http.Cookie, path, json string) *http.Response {
		req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(json))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return res
	}
	doc := `{"document":{"statusField":{"kind":"custom_field","ref":"cf1"},"projectField":{"kind":"","ref":""},"statuses":{"Follow Up":"engaged"},"blankStatusStage":"open","sources":{},"note":""}}`
	assert.Equal(t, http.StatusForbidden, send(http.MethodPut, marketer, "/api/settings/funnel", doc).StatusCode)
	assert.Equal(t, http.StatusCreated, send(http.MethodPut, admin, "/api/settings/funnel", doc).StatusCode)
	assert.Equal(t, http.StatusBadRequest, send(http.MethodPut, admin, "/api/settings/funnel", `{"document":{"statuses":{"x":"hot"}}}`).StatusCode)

	assert.Equal(t, http.StatusForbidden, send(http.MethodPost, viewer, "/api/leads/import", `{"accountId":1,"mode":"full","since":null}`).StatusCode)
	assert.Equal(t, http.StatusNotFound, send(http.MethodPost, marketer, "/api/leads/import", `{"accountId":1,"mode":"full","since":null}`).StatusCode)
}
