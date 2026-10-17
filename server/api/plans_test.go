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
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: a viewer creating or accepting a plan, the template gate missing
// from the list, generation without a model answering 500 instead of a
// stated 409, the exact export answering 500, and a bad diff query.
func TestPlanRoutes(t *testing.T) {
	db := storetest.Open(t)
	authn := userstest.NewAuthentik()
	people := users.New(db, authn)
	oidc := &apiOIDC{}
	id := identity.New(db, people, oidc, identity.Config{AllowedGroup: "adx-users"})
	br := brands.New(db, brands.Config{})
	svc := plans.New(db, br, plans.Config{})
	e := &authEnv{db: db, users: people, authn: authn, oidc: oidc}
	e.srv = httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Identity: id, Brands: br, Plans: svc}))
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
	doc := `{"objective":"Leads","budgetMinor":1000000,"currency":"AED","periodFrom":"2026-11-01","periodTo":"2026-11-30","allocations":[{"channel":"google_search","objective":"leads","budgetMinor":1000000,"geography":"AE","audience":"","projectId":null,"expectedCplMinor":null,"note":""}],"assetRequirements":[],"pageRequirements":[],"measurementRequirements":[],"testHypotheses":[],"observationWindows":[],"evidenceRefs":[],"assumptions":[],"notes":""}`
	code, _ := post(viewer, "/api/plans", `{"brandId":1,"title":"Q4","document":`+doc+`}`)
	assert.Equal(t, http.StatusForbidden, code)
	code, body := post(admin, "/api/plans", `{"brandId":1,"title":"Q4","document":`+doc+`}`)
	require.Equal(t, http.StatusCreated, code, body)
	assert.Contains(t, body, `"state":"ready"`)

	res, body := get(t, e.srv, "/api/plans", viewer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"available":false`)
	assert.Contains(t, body, `"key":"google_search"`)

	code, body = post(viewer, "/api/plans/1/versions/1/accept", `{}`)
	assert.Equal(t, http.StatusForbidden, code, body)
	code, body = post(admin, "/api/plans/1/versions/1/accept", `{}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, `"executionAuthorization":null`)

	code, body = post(admin, "/api/plans/1/generate", `{"briefIds":[],"instructions":""}`)
	assert.Equal(t, http.StatusConflict, code, body)
	assert.Contains(t, body, "no model is configured")

	res, body = get(t, e.srv, "/api/plans/1/versions/1/export?format=xlsx", viewer, nil)
	assert.Equal(t, http.StatusConflict, res.StatusCode, body)
	res, body = get(t, e.srv, "/api/plans/1/versions/1/export?format=csv", viewer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, res.Header.Get("Content-Disposition"), "plan-1-v1.csv")
	assert.Contains(t, body, "google_search,leads,1000000,AED,AE")

	res, _ = get(t, e.srv, "/api/plans/1/diff?from=1&to=x", viewer, nil)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	res, body = get(t, e.srv, "/api/plans/1/diff?from=1&to=1", viewer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"invalidates":null`)
	code, body = post(admin, "/api/plans/validate", `{"brandId":1,"document":{"objective":"","budgetMinor":0,"currency":"","periodFrom":"","periodTo":""}}`)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, `"budget_missing"`)
}
