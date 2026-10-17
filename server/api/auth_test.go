package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/projects"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type apiOIDC struct{ claims identity.Claims }

func (o *apiOIDC) AuthorizationURL(state, nonce, _ string) string {
	return "https://authentik.example/authorize?state=" + url.QueryEscape(state) + "&nonce=" + nonce
}
func (o *apiOIDC) Exchange(context.Context, string, string, string) (identity.Claims, error) {
	return o.claims, nil
}

type authEnv struct {
	srv   *httptest.Server
	db    *store.DB
	users *users.Users
	authn *userstest.Authentik
	oidc  *apiOIDC
}

func newAuthServer(t *testing.T, dev bool) *authEnv {
	t.Helper()
	e := &authEnv{db: storetest.Open(t), authn: userstest.NewAuthentik(), oidc: &apiOIDC{}}
	e.users = users.New(e.db, e.authn)
	id := identity.New(e.db, e.users, e.oidc, identity.Config{AllowedGroup: "adx-users"})
	e.srv = httptest.NewServer(api.NewRouter(api.Deps{
		Log: slog.New(slog.DiscardHandler), Dev: dev, Projects: projects.New(e.db), Identity: id,
	}))
	t.Cleanup(e.srv.Close)
	return e
}

// noRedirect keeps 303s visible so the test can read Location and Set-Cookie.
func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (e *authEnv) addAndPoint(t *testing.T, email string, role core.Role) users.User {
	t.Helper()
	u, err := e.users.Add(t.Context(), core.System(core.ViaStartup), users.AddInput{Email: email, Role: role})
	require.NoError(t, err)
	e.oidc.claims = identity.Claims{Subject: e.authn.ByEmail(email).UUID, Email: email, Name: "Sara", Groups: []string{"adx-users"}}
	return u
}

// signIn walks login → callback and returns the session cookie.
func (e *authEnv) signIn(t *testing.T) *http.Cookie {
	t.Helper()
	c := noRedirect()
	res, err := c.Get(e.srv.URL + "/api/auth/login?return_to=/projects")
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	loc, err := url.Parse(res.Header.Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "authentik.example", loc.Host)

	res, err = c.Get(e.srv.URL + "/api/auth/callback?state=" + url.QueryEscape(loc.Query().Get("state")) + "&code=good")
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	assert.Equal(t, "/projects", res.Header.Get("Location"))
	for _, ck := range res.Cookies() {
		if ck.Name == "adx_session" {
			assert.True(t, ck.HttpOnly)
			return ck
		}
	}
	t.Fatal("no session cookie set")
	return nil
}

func get(t *testing.T, srv *httptest.Server, path string, cookie *http.Cookie, headers map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	require.NoError(t, err)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := noRedirect().Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(res.Body)
	return res, string(body)
}

// Catches: protected data reachable without a session outside dev, a role
// header being trusted, and the dev admin existing outside dev mode.
func TestUnauthenticatedAndForgedRequestsAreRefused(t *testing.T) {
	e := newAuthServer(t, false)
	e.addAndPoint(t, "sara@reportage.ae", core.RoleViewer)

	res, _ := get(t, e.srv, "/api/projects", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	res, _ = get(t, e.srv, "/api/session", nil, nil)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode, "no dev admin outside dev")

	forged := map[string]string{"X-Role": "admin", "X-User-Id": "1", "X-Forwarded-User": "cmo@reportage.ae", "Authorization": "Bearer admin"}
	res, _ = get(t, e.srv, "/api/projects", nil, forged)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	res, _ = get(t, e.srv, "/api/projects", &http.Cookie{Name: "adx_session", Value: strings.Repeat("x", 52)}, forged)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode, "an invented cookie is no session")
	assert.Contains(t, res.Header.Get("Set-Cookie"), "Max-Age=0", "a dead cookie is cleared")
}

// Catches: the callback not setting the cookie, the session not carrying
// the person and role, a viewer's forged header raising their role, and
// disabling not ending the session.
func TestSessionCarriesMembershipAndEndsOnDisable(t *testing.T) {
	e := newAuthServer(t, false)
	sara := e.addAndPoint(t, "sara@reportage.ae", core.RoleViewer)
	cookie := e.signIn(t)

	res, body := get(t, e.srv, "/api/session", cookie, map[string]string{"X-Role": "admin"})
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"role":"viewer"`)
	assert.Contains(t, body, `"email":"sara@reportage.ae"`)
	assert.Contains(t, body, `"workspaceId":1`)
	assert.Contains(t, body, `"dev":false`)
	assert.NotContains(t, body, cookie.Value)

	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/api/projects", strings.NewReader(`{"name":"Oceana"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Role", "admin")
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode, "a viewer stays a viewer whatever the header says")

	res, _ = get(t, e.srv, "/api/projects", cookie, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	_, err = e.users.Disable(t.Context(), core.System(core.ViaStartup), sara.ID)
	require.NoError(t, err)
	res, _ = get(t, e.srv, "/api/projects", cookie, nil)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestLogoutEndsOnlyThatSession(t *testing.T) {
	e := newAuthServer(t, false)
	e.addAndPoint(t, "sara@reportage.ae", core.RoleMarketer)
	first := e.signIn(t)
	second := e.signIn(t)

	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/api/auth/logout", nil)
	req.AddCookie(first)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, res.StatusCode)

	res, _ = get(t, e.srv, "/api/session", first, nil)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	res, _ = get(t, e.srv, "/api/session", second, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	// A cross-site POST can't sign someone out.
	req, _ = http.NewRequest(http.MethodPost, e.srv.URL+"/api/auth/logout", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.AddCookie(second)
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
}

// Catches: dev mode refusing to work without authentik, or a dev server
// with a real session ignoring it in favour of the dev admin.
func TestDevAdminOnlyWithoutASession(t *testing.T) {
	e := newAuthServer(t, true)
	res, body := get(t, e.srv, "/api/session", nil, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"dev":true`)
	assert.Contains(t, body, `"role":"admin"`)

	e.addAndPoint(t, "sara@reportage.ae", core.RoleViewer)
	cookie := e.signIn(t)
	res, body = get(t, e.srv, "/api/session", cookie, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"role":"viewer"`)
	assert.Contains(t, body, `"dev":false`)

	// No identity at all in dev: still the dev admin, and login says so.
	bare := httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Dev: true, Projects: projects.New(storetest.Open(t))}))
	t.Cleanup(bare.Close)
	res, _ = get(t, bare, "/api/projects", nil, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	res, body = get(t, bare, "/api/auth/login", nil, nil)
	assert.Equal(t, http.StatusConflict, res.StatusCode)
	assert.Contains(t, body, "not configured")
}
