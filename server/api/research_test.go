package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/research"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/providers/webfetch"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: a viewer creating a brief or starting a run, a private address
// accepted over HTTP, an invalid scope filter, and the run route returning
// 202 for a new run.
func TestResearchRoutes(t *testing.T) {
	db := storetest.Open(t)
	authn := userstest.NewAuthentik()
	people := users.New(db, authn)
	oidc := &apiOIDC{}
	id := identity.New(db, people, oidc, identity.Config{AllowedGroup: "adx-users"})
	br := brands.New(db, brands.Config{})
	rs := runs.New(db, nil, runs.Config{})
	// The real fetcher's guard refuses the loopback address without any
	// network, which is the point of the second request below.
	svc := research.New(db, evidence.New(db, nil), br, research.Config{Fetcher: webfetch.New(webfetch.Config{})})
	e := &authEnv{db: db, users: people, authn: authn, oidc: oidc}
	e.srv = httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Identity: id, Brands: br, Research: svc, Runs: rs}))
	t.Cleanup(e.srv.Close)
	e.addAndPoint(t, "ceo@reportage.ae", core.RoleViewer)
	viewer := e.signIn(t)
	e.addAndPoint(t, "cmo@reportage.ae", core.RoleAdmin)
	admin := e.signIn(t)
	_, err := br.Create(t.Context(), core.Actor{UserID: 1, Role: core.RoleAdmin, WorkspaceID: 1}, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)

	post := func(cookie *http.Cookie, path, json string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, e.srv.URL+path, strings.NewReader(json))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	brief := `{"brandId":1,"title":"Q4","geography":"AE","urls":["https://example.com/"],"auditAccount":true}`
	code, _ := post(viewer, "/api/research/briefs", brief)
	assert.Equal(t, http.StatusForbidden, code)
	code, body := post(admin, "/api/research/briefs", `{"brandId":1,"title":"Q4","urls":["http://127.0.0.1/"]}`)
	assert.Equal(t, http.StatusBadRequest, code, body)
	code, body = post(admin, "/api/research/briefs", brief)
	require.Equal(t, http.StatusCreated, code, body)
	assert.Contains(t, body, `"state":"pending"`)
	code, _ = post(viewer, "/api/research/briefs/1/runs", `{}`)
	assert.Equal(t, http.StatusForbidden, code)
	code, body = post(admin, "/api/research/briefs/1/runs", `{}`)
	assert.Equal(t, http.StatusAccepted, code, body)
	assert.Contains(t, body, `"kind":"research.run"`)
	code, _ = post(admin, "/api/research/briefs/1/runs", `{}`)
	assert.Equal(t, http.StatusOK, code, "same day, same run")

	res, body := get(t, e.srv, "/api/research/briefs/1", viewer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"title":"Q4"`)
	res, _ = get(t, e.srv, "/api/research/briefs/1/findings?scope=secret", viewer, nil)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	res, body = get(t, e.srv, "/api/research/briefs/1/findings?scope=account", viewer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, `{"findings":[]}`+"\n", body)
	res, _ = get(t, e.srv, "/api/research/evidence/99", viewer, nil)
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}
