package api_test

import (
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
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/pages"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: a public page served without the no-script policy, editor text
// reaching a visitor as markup, a form post for another project accepted, a
// visitor's post needing a session, and the preview served without a
// sandbox.
func TestPublicPagesAndForms(t *testing.T) {
	db := storetest.Open(t)
	authn := userstest.NewAuthentik()
	people := users.New(db, authn)
	oidc := &apiOIDC{}
	id := identity.New(db, people, oidc, identity.Config{AllowedGroup: "adx-users"})
	br := brands.New(db, brands.Config{})
	svc := pages.New(db, br, evidence.New(db, nil), pages.Config{PublicBase: "https://adx.example"})
	e := &authEnv{db: db, users: people, authn: authn, oidc: oidc}
	e.srv = httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Identity: id, Brands: br, Pages: svc}))
	t.Cleanup(e.srv.Close)
	e.addAndPoint(t, "sara@reportage.ae", core.RoleMarketer)
	marketer := e.signIn(t)
	admin := core.Actor{UserID: 1, Role: core.RoleAdmin, WorkspaceID: 1}
	b, err := br.Create(t.Context(), admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	p, err := db.Writer().CreateProject(t.Context(), store.CreateProjectParams{Name: "The Wadi"})
	require.NoError(t, err)
	require.NoError(t, br.AssignProject(t.Context(), admin, p.ID, b.ID))

	post := func(cookie *http.Cookie, path, json string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, e.srv.URL+path, strings.NewReader(json))
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		body, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(body)
	}
	doc := `{"title":"Wadi","headline":"<script>alert(1)</script> Villas","subheadline":"","sections":[{"kind":"text","text":"Register","items":[],"imageKey":"","alt":""},{"kind":"form","text":"","items":[],"imageKey":"","alt":""}],"offerId":null,"offerVersion":null,"form":{"fields":[{"name":"phone","label":"Phone","kind":"phone","required":true,"options":[]}],"consentText":"ok","submitLabel":"Go","successMessage":"Thanks"},"footer":""}`
	code, body := post(marketer, "/api/pages", `{"brandId":1,"projectId":1,"planId":0,"planVersion":0,"slug":"wadi","name":"Wadi","language":"en","document":`+doc+`}`)
	require.Equal(t, http.StatusCreated, code, body)
	assert.Contains(t, body, `"testToken":"`)
	token := strings.SplitN(strings.SplitN(body, `"testToken":"`, 2)[1], `"`, 2)[0]

	// Not published yet: the public URL is 404 and so is the form.
	res, err := http.Get(e.srv.URL + "/p/wadi")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, res.StatusCode)

	res, body = get(t, e.srv, "/api/pages/1/versions/1/preview", marketer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, res.Header.Get("Content-Security-Policy"), "sandbox")
	assert.Contains(t, body, "&lt;script&gt;")
	assert.NotContains(t, body, "<script>")

	code, body = post(marketer, "/api/pages/1/publish", `{"version":1}`)
	require.Equal(t, http.StatusOK, code, body)
	res, err = http.Get(e.srv.URL + "/p/wadi")
	require.NoError(t, err)
	raw, _ := io.ReadAll(res.Body)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	csp := res.Header.Get("Content-Security-Policy")
	assert.Contains(t, csp, "default-src 'none'")
	assert.NotContains(t, csp, "script-src")
	assert.Contains(t, string(raw), "&lt;script&gt;alert(1)&lt;/script&gt; Villas")
	assert.Contains(t, string(raw), `name="_project" value="1"`)

	// A visitor posts without any session. Wrong project: 400. Right: 303 to thanks.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	form := url.Values{"phone": {"+971501234567"}, "_project": {"2"}, "utm_source": {"google"}}
	res, err = client.PostForm(e.srv.URL+"/p/wadi/form", form)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	form.Set("_project", "1")
	res, err = client.PostForm(e.srv.URL+"/p/wadi/form", form)
	require.NoError(t, err)
	assert.Equal(t, http.StatusSeeOther, res.StatusCode)
	assert.Equal(t, "/p/wadi/thanks", res.Header.Get("Location"))
	res, err = http.Get(e.srv.URL + "/p/wadi/thanks")
	require.NoError(t, err)
	raw, _ = io.ReadAll(res.Body)
	assert.Contains(t, string(raw), "Thanks")

	// JSON callers get the submission; a test token marks it.
	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/p/wadi/form", strings.NewReader(url.Values{"phone": {"+971501234567"}, "_project": {"1"}, "_test": {token}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	raw, _ = io.ReadAll(res.Body)
	assert.Equal(t, http.StatusCreated, res.StatusCode, string(raw))
	assert.Contains(t, string(raw), `"test":true`)
	req, _ = http.NewRequest(http.MethodPost, e.srv.URL+"/p/wadi/form", strings.NewReader(url.Values{"phone": {"nope"}, "_project": {"1"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	raw, _ = io.ReadAll(res.Body)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	assert.Contains(t, string(raw), `"reason":"invalid:phone"`)

	res, body = get(t, e.srv, "/api/pages/1/readiness", marketer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"ready":true`, body)
	res, body = get(t, e.srv, "/api/pages/1/submissions", marketer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"wrong_project"`)
}
