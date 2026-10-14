package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type apiGHL struct{ exchanges int }

func (*apiGHL) AuthorizationURL(state string) string {
	return "https://example.com/install?state=" + state
}
func (p *apiGHL) Exchange(context.Context, string) (connections.Tokens, error) {
	p.exchanges++
	return connections.Tokens{
		AccessToken: "never-expose-access", RefreshToken: "never-expose-refresh", ExpiresIn: 3600,
		UserType: "Location", LocationID: "loc-1", CompanyID: "company-1", Scope: strings.Join(connections.RequiredScopes, " "),
	}, nil
}
func (p *apiGHL) Refresh(ctx context.Context, token string) (connections.Tokens, error) {
	return p.Exchange(ctx, token)
}
func (*apiGHL) Location(context.Context, string, string) (connections.Location, error) {
	return connections.Location{ID: "loc-1", Name: "Sandbox"}, nil
}

func (*apiGHL) ReadAccess(context.Context, string, string) error { return nil }

func connectionServer(t *testing.T, dev bool) (*httptest.Server, *apiGHL) {
	t.Helper()
	provider := &apiGHL{}
	service, err := connections.New(storetest.Open(t), provider, connections.Config{EncryptionKey: bytes.Repeat([]byte{1}, 32)})
	require.NoError(t, err)
	srv := httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Dev: dev, Connections: service}))
	t.Cleanup(srv.Close)
	return srv, provider
}

func flowClient(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()
	client := *srv.Client()
	var err error
	client.Jar, err = cookiejar.New(nil)
	require.NoError(t, err)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}

func startFlow(t *testing.T, client *http.Client, base string) string {
	t.Helper()
	res, err := client.Post(base+"/api/connections/crm/start", "application/x-www-form-urlencoded", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	assert.Equal(t, "no-store", res.Header.Get("Cache-Control"))
	u, err := url.Parse(res.Header.Get("Location"))
	require.NoError(t, err)
	for _, cookie := range res.Cookies() {
		assert.True(t, cookie.HttpOnly)
		assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	}
	return u.Query().Get("state")
}

func responseBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return string(body)
}

func TestConnectionHTTPRoundTripIncludesCoreAndNeverReturnsTokens(t *testing.T) {
	srv, provider := connectionServer(t, true)
	client := flowClient(t, srv)
	state := startFlow(t, client, srv.URL)
	res, err := client.Get(srv.URL + "/api/connections/crm/callback?state=" + url.QueryEscape(state) + "&code=valid")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "no-referrer", res.Header.Get("Referrer-Policy"))
	body := responseBody(t, res)
	assert.Contains(t, body, "loc-1")
	assert.NotContains(t, body, "never-expose")
	assert.Equal(t, 1, provider.exchanges)
	res, err = client.Get(srv.URL + "/api/connections/crm")
	require.NoError(t, err)
	var list api.ConnectionsResponse
	require.NoError(t, json.Unmarshal([]byte(responseBody(t, res)), &list))
	require.Len(t, list.Connections, 1)
	assert.Equal(t, "Sandbox", list.Connections[0].Name)
	res, err = client.Post(srv.URL+"/api/connections/crm/loc-1/check", "application/json", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.NotContains(t, responseBody(t, res), "never-expose")
	res, err = client.Get(srv.URL + "/api/connections/crm/callback?state=" + url.QueryEscape(state) + "&code=valid")
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	responseBody(t, res)
	assert.Equal(t, 1, provider.exchanges)
}

func TestPrivateConnectionFormIncludesCoreAndDoesNotExposeTheToken(t *testing.T) {
	srv, provider := connectionServer(t, true)
	res, err := srv.Client().PostForm(srv.URL+"/api/connections/crm/private", url.Values{
		"locationId": {"loc-1"}, "token": {"private-token-secret"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	body := responseBody(t, res)
	assert.Contains(t, body, `"authMethod":"private_token"`)
	assert.Contains(t, body, `"tokenExpiresAt":null`)
	assert.NotContains(t, body, "private-token-secret")
	assert.Zero(t, provider.exchanges)
	res, err = srv.Client().Post(srv.URL+"/api/connections/crm/loc-1/check", "application/json", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	responseBody(t, res)
	assert.Zero(t, provider.exchanges)
	prod, _ := connectionServer(t, false)
	res, err = prod.Client().PostForm(prod.URL+"/api/connections/crm/private", url.Values{
		"locationId": {"loc-1"}, "token": {"private-token-secret"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	assert.NotContains(t, responseBody(t, res), "private-token-secret")
}

func TestCallbackNeedsItsBrowserCookie(t *testing.T) {
	srv, provider := connectionServer(t, true)
	client := flowClient(t, srv)
	state := startFlow(t, client, srv.URL)
	res, err := srv.Client().Get(srv.URL + "/api/connections/crm/callback?state=" + url.QueryEscape(state) + "&code=valid")
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	responseBody(t, res)
	assert.Zero(t, provider.exchanges)
	res, err = client.Get(srv.URL + "/api/connections/crm/callback?state=" + url.QueryEscape(state) + "&code=valid")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	responseBody(t, res)
}

func TestParallelFlowsReuseCookieAndRejectDuplicateCallbackFields(t *testing.T) {
	srv, provider := connectionServer(t, true)
	client := flowClient(t, srv)
	first := startFlow(t, client, srv.URL)
	second := startFlow(t, client, srv.URL)
	res, err := client.Get(srv.URL + "/api/connections/crm/callback?state=" + first + "&state=" + second + "&code=valid")
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	responseBody(t, res)
	assert.Zero(t, provider.exchanges)
	for _, state := range []string{first, second} {
		res, err = client.Get(srv.URL + "/api/connections/crm/callback?state=" + state + "&code=valid")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, res.StatusCode)
		responseBody(t, res)
	}
	assert.Equal(t, 2, provider.exchanges)
}

func TestConnectionRoutesRefuseProductionAndCrossSiteStarts(t *testing.T) {
	srv, provider := connectionServer(t, false)
	for _, path := range []string{"/api/connections/crm", "/api/connections/crm/connect", "/api/connections/crm/callback?state=example&code=example"} {
		res, err := srv.Client().Get(srv.URL + path)
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
		responseBody(t, res)
	}
	res, err := srv.Client().Post(srv.URL+"/api/connections/crm/start", "application/json", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	responseBody(t, res)
	assert.Zero(t, provider.exchanges)
	dev, _ := connectionServer(t, true)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, dev.URL+"/api/connections/crm/start", nil)
	require.NoError(t, err)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res, err = dev.Client().Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
	responseBody(t, res)
}
