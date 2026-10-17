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
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: a marketer creating a brand or approving a fact over HTTP, input
// reaching core unparsed, and the error mapping for a conflicting approval.
func TestBrandRoutes(t *testing.T) {
	db := storetest.Open(t)
	authn := userstest.NewAuthentik()
	people := users.New(db, authn)
	oidc := &apiOIDC{}
	id := identity.New(db, people, oidc, identity.Config{AllowedGroup: "adx-users"})
	svc := brands.New(db, brands.Config{})
	e := &authEnv{db: db, users: people, authn: authn, oidc: oidc}
	e.srv = httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Identity: id, Brands: svc}))
	t.Cleanup(e.srv.Close)
	e.addAndPoint(t, "sara@reportage.ae", core.RoleMarketer)
	marketer := e.signIn(t)
	e.addAndPoint(t, "cmo@reportage.ae", core.RoleAdmin)
	admin := e.signIn(t)
	p, err := db.Writer().CreateProject(t.Context(), store.CreateProjectParams{Name: "The Wadi"})
	require.NoError(t, err)

	post := func(cookie *http.Cookie, method, path, json string) (int, string) {
		req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(json))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	code, _ := post(marketer, http.MethodPost, "/api/brands", `{"name":"Reportage","kind":"own"}`)
	assert.Equal(t, http.StatusForbidden, code)
	code, body := post(admin, http.MethodPost, "/api/brands", `{"name":"Reportage","kind":"own"}`)
	require.Equal(t, http.StatusCreated, code, body)
	code, body = post(admin, http.MethodPut, "/api/brands/projects", `{"projectId":1,"brandId":1}`)
	require.Equal(t, http.StatusOK, code, body)
	_ = p

	fact := `{"projectId":1,"key":"price.from","kind":"money","value":"120000000","currency":"AED","effectiveFrom":"2026-10-01","sourceKind":"document","sourceRef":"price list"}`
	code, body = post(marketer, http.MethodPost, "/api/brands/1/facts", fact)
	require.Equal(t, http.StatusCreated, code, body)
	assert.Contains(t, body, `"class":"needs_review"`)
	code, body = post(marketer, http.MethodPost, "/api/brands/1/facts", strings.Replace(fact, "120000000", "110000000", 1))
	require.Equal(t, http.StatusCreated, code, body)
	assert.Contains(t, body, `"inConflict":true`)
	code, _ = post(marketer, http.MethodPost, "/api/facts/1/approve", `{}`)
	assert.Equal(t, http.StatusForbidden, code)
	code, body = post(admin, http.MethodPost, "/api/facts/1/approve", `{}`)
	assert.Equal(t, http.StatusConflict, code, body)
	code, body = post(admin, http.MethodPost, "/api/fact-conflicts/1/resolve", `{"winnerId":1,"resolution":"price list wins"}`)
	require.Equal(t, http.StatusOK, code, body)
	code, body = post(admin, http.MethodPost, "/api/facts/1/approve", `{}`)
	require.Equal(t, http.StatusOK, code, body)

	res, body := get(t, e.srv, "/api/brands/1/claims?project_id=1&at=2026-10-17", marketer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"value":"120000000"`)
	res, _ = get(t, e.srv, "/api/brands/1/claims?project_id=1&at=yesterday", marketer, nil)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	res, body = get(t, e.srv, "/api/brands", marketer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"projectCount":1`)
	code, body = post(marketer, http.MethodPost, "/api/brands/1/offers", `{"projectId":1,"name":"Launch","document":{"headline":"Up to 15% off and more"},"effectiveFrom":"2026-10-01"}`)
	require.Equal(t, http.StatusCreated, code, body)
	code, body = post(marketer, http.MethodPost, "/api/offers/1/state", `{"state":"approved"}`)
	assert.Equal(t, http.StatusForbidden, code, body)
	code, body = post(admin, http.MethodPost, "/api/offers/1/state", `{"state":"approved"}`)
	require.Equal(t, http.StatusOK, code, body)
	code, body = post(marketer, http.MethodPost, "/api/offers/1/versions", `{"document":{"headline":"12% off"},"effectiveFrom":"2026-11-01"}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, `"version":2`)
	assert.Contains(t, body, `"affected":[]`)
}
