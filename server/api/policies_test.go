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
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: a viewer or marketer saving policy over HTTP, an unknown action
// slipping through parsing, and the unconfigured state not reaching the
// screen.
func TestPoliciesRoutesCheckRoleAndValidate(t *testing.T) {
	db := storetest.Open(t)
	authn := userstest.NewAuthentik()
	people := users.New(db, authn)
	oidc := &apiOIDC{}
	id := identity.New(db, people, oidc, identity.Config{AllowedGroup: "adx-users"})
	e := &authEnv{db: db, users: people, authn: authn, oidc: oidc}
	e.srv = httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Identity: id, Policies: policies.New(db)}))
	t.Cleanup(e.srv.Close)

	e.addAndPoint(t, "sara@reportage.ae", core.RoleMarketer)
	marketer := e.signIn(t)
	e.addAndPoint(t, "cmo@reportage.ae", core.RoleAdmin)
	admin := e.signIn(t)

	res, body := get(t, e.srv, "/api/policies", marketer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"configured":false`)
	assert.Contains(t, body, `"unconfigured":["policy document"]`)
	assert.Contains(t, body, `"suggested":{`)

	put := func(cookie *http.Cookie, json string) (*http.Response, string) {
		req, _ := http.NewRequest(http.MethodPut, e.srv.URL+"/api/policies", strings.NewReader(json))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	doc := `{"document":{"actions":{"ad.pause":{"marketer":"allowed","admin":"allowed"}},"caps":{"dailySpendIncreasePct":20,"maxChangesPerDay":30},"writeAccounts":[],"note":"first"}}`
	res, _ = put(marketer, doc)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
	res, _ = put(admin, `{"document":{"actions":{"campaign.delete":{"marketer":"allowed","admin":"allowed"}}}}`)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	res, body = put(admin, doc)
	assert.Equal(t, http.StatusCreated, res.StatusCode)
	assert.Contains(t, body, `"version":1`)

	res, body = get(t, e.srv, "/api/policies", marketer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"configured":true`)
	assert.Contains(t, body, `"write accounts: none enabled"`)

	req, _ := http.NewRequest(http.MethodPut, e.srv.URL+"/api/policies/kill-switch", strings.NewReader(`{"accountId":0,"engaged":true,"reason":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(admin)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	res, body = get(t, e.srv, "/api/accounts", marketer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "{\"accounts\":[]}\n", body, "no connections service means an honest empty list")
}
