package api_test

import (
	"bytes"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/creative"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/providers/filestore"
	"github.com/WokeWorks/adx/server/providers/svgrender"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: an upload accepted without a session or from a viewer, a served
// file without a sandboxing content security policy, and the review route
// letting a marketer approve.
func TestCreativeRoutes(t *testing.T) {
	db := storetest.Open(t)
	authn := userstest.NewAuthentik()
	people := users.New(db, authn)
	oidc := &apiOIDC{}
	id := identity.New(db, people, oidc, identity.Config{AllowedGroup: "adx-users"})
	st, err := filestore.New(t.TempDir())
	require.NoError(t, err)
	svc := creative.New(db, st, svgrender.New(), creative.Config{})
	e := &authEnv{db: db, users: people, authn: authn, oidc: oidc}
	e.srv = httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Identity: id, Creative: svc}))
	t.Cleanup(e.srv.Close)
	e.addAndPoint(t, "ceo@reportage.ae", core.RoleViewer)
	viewer := e.signIn(t)
	e.addAndPoint(t, "sara@reportage.ae", core.RoleMarketer)
	marketer := e.signIn(t)

	upload := func(cookie *http.Cookie) *http.Response {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		part, err := mw.CreatePart(map[string][]string{"Content-Disposition": {`form-data; name="file"; filename="wadi.jpg"`}, "Content-Type": {"image/jpeg"}})
		require.NoError(t, err)
		_, _ = part.Write([]byte("jpeg bytes"))
		_ = mw.WriteField("kind", "render")
		require.NoError(t, mw.Close())
		req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/api/assets", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return res
	}
	assert.Equal(t, http.StatusUnauthorized, upload(nil).StatusCode)
	assert.Equal(t, http.StatusForbidden, upload(viewer).StatusCode)
	res := upload(marketer)
	raw, _ := io.ReadAll(res.Body)
	uploaded := string(raw)
	require.Equal(t, http.StatusCreated, res.StatusCode, uploaded)
	assert.Contains(t, uploaded, `"key":"sha256-`)
	key := strings.SplitN(strings.SplitN(uploaded, `"key":"`, 2)[1], `"`, 2)[0]

	res, _ = get(t, e.srv, "/api/assets/files/"+key, viewer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "image/jpeg", res.Header.Get("Content-Type"))
	assert.Contains(t, res.Header.Get("Content-Security-Policy"), "sandbox")
	assert.Equal(t, "nosniff", res.Header.Get("X-Content-Type-Options"))
	res, _ = get(t, e.srv, "/api/assets/files/sha256-"+strings.Repeat("0", 64), viewer, nil)
	assert.Equal(t, http.StatusNotFound, res.StatusCode)

	post := func(cookie *http.Cookie, path, json string) (*http.Response, string) {
		req, _ := http.NewRequest(http.MethodPost, e.srv.URL+path, strings.NewReader(json))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	res, body := post(marketer, "/api/creatives", `{"projectId":0,"name":"Wadi","format":"1:1","language":"en","copy":{"eyebrow":"","title":"Villas","text":"","note":"","cta":"Register"},"renderAssetId":1}`)
	require.Equal(t, http.StatusCreated, res.StatusCode, body)
	assert.Contains(t, body, `"reviewState":"draft"`)
	res, body = post(marketer, "/api/creatives/1/render", `{}`)
	require.Equal(t, http.StatusOK, res.StatusCode, body)
	assert.Contains(t, body, `"renderer":"template"`)
	res, _ = post(marketer, "/api/creatives/1/submit", `{}`)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	res, _ = post(marketer, "/api/creatives/1/review", `{"decision":"approved","note":""}`)
	assert.Equal(t, http.StatusForbidden, res.StatusCode, "marketers don't approve")
	res, svg := get(t, e.srv, "/api/creatives/1/export/1", viewer, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "image/svg+xml", res.Header.Get("Content-Type"))
	assert.Contains(t, svg, `data-renderer="template"`)
}
