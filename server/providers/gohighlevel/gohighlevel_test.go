package gohighlevel_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/providers/gohighlevel"
)

const callback = "http://localhost:8080/api/connections/crm/callback"

func newClient(t *testing.T, server *httptest.Server) *gohighlevel.Client {
	t.Helper()
	client, err := gohighlevel.New(gohighlevel.Config{
		ClientID: "test-client", ClientSecret: "test-secret", RedirectURI: callback, VersionID: "test-version",
		APIBaseURL: server.URL, AuthorizeURL: server.URL + "/install", HTTPClient: server.Client(),
	})
	require.NoError(t, err)
	return client
}

func TestAuthorizationURLAndTokenExchange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/oauth/token", r.URL.Path)
		assert.Equal(t, "v3", r.Header.Get("Version"))
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "test-client", r.Form.Get("client_id"))
		assert.Equal(t, "test-secret", r.Form.Get("client_secret"))
		assert.Equal(t, callback, r.Form.Get("redirect_uri"))
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			assert.Empty(t, r.Form.Get("user_type"))
			assert.Equal(t, "a code+with/symbols", r.Form.Get("code"))
			assert.Empty(t, r.Form.Get("refresh_token"))
		case "refresh_token":
			assert.Equal(t, "Location", r.Form.Get("user_type"))
			assert.Equal(t, "previous-refresh", r.Form.Get("refresh_token"))
			assert.Empty(t, r.Form.Get("code"))
		default:
			t.Error("unexpected grant")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "new-access", "refresh_token": "new-refresh", "token_type": "Bearer", "expires_in": 86400,
			"scope": strings.Join(connections.RequiredScopes, " "), "userType": "Location", "locationId": "loc-1", "companyId": "company-1",
		})
	}))
	defer server.Close()
	client := newClient(t, server)
	u, err := url.Parse(client.AuthorizationURL("state+value"))
	require.NoError(t, err)
	assert.Equal(t, "test-client", u.Query().Get("client_id"))
	assert.Equal(t, callback, u.Query().Get("redirect_uri"))
	assert.Equal(t, "test-version", u.Query().Get("version_id"))
	assert.Equal(t, "state+value", u.Query().Get("state"))
	assert.Equal(t, strings.Join(connections.RequiredScopes, " "), u.Query().Get("scope"))
	assert.NotContains(t, u.String(), "test-secret")
	tokens, err := client.Exchange(t.Context(), "a code+with/symbols")
	require.NoError(t, err)
	assert.Equal(t, "new-access", tokens.AccessToken)
	assert.Equal(t, "new-refresh", tokens.RefreshToken)
	assert.Equal(t, "loc-1", tokens.LocationID)
	assert.Equal(t, "Location", tokens.UserType)
	tokens, err = client.Refresh(t.Context(), "previous-refresh")
	require.NoError(t, err)
	assert.Equal(t, "new-refresh", tokens.RefreshToken)
}

func TestAgencyInstallationExchangesOnlyTheApprovedWorkspace(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.NoError(t, r.ParseForm())
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "v3", r.Header.Get("Version"))
		switch r.URL.Path {
		case "/oauth/token":
			assert.Empty(t, r.Form.Get("user_type"))
			_, _ = io.WriteString(w, `{"access_token":"agency-access","refresh_token":"agency-refresh","token_type":"Bearer","expires_in":86400,"userType":"Company","companyId":"agency-1","approvedLocations":["loc-1"]}`)
		case "/oauth/location-token":
			assert.Equal(t, "Bearer agency-access", r.Header.Get("Authorization"))
			assert.Equal(t, "agency-1", r.Form.Get("companyId"))
			assert.Equal(t, "loc-1", r.Form.Get("locationId"))
			assert.Empty(t, r.Form.Get("client_secret"))
			assert.Empty(t, r.Form.Get("refresh_token"))
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "location-access", "refresh_token": "location-refresh", "token_type": "Bearer",
				"expires_in": 86400, "locationId": "loc-1", "scope": strings.Join(connections.RequiredScopes, " "),
			})
		default:
			t.Error("unexpected request")
		}
	}))
	defer server.Close()
	tokens, err := newClient(t, server).Exchange(t.Context(), "agency-code")
	require.NoError(t, err)
	assert.Equal(t, 2, requests)
	assert.Equal(t, "Location", tokens.UserType)
	assert.Equal(t, "agency-1", tokens.CompanyID)
	assert.Equal(t, "loc-1", tokens.LocationID)
	assert.Equal(t, "location-access", tokens.AccessToken)
	assert.Equal(t, "location-refresh", tokens.RefreshToken)
}

func TestAgencyInstallationRejectsAmbiguousOrAutomaticWorkspaceAccess(t *testing.T) {
	for _, extra := range []string{
		`"approvedLocations":[]`,
		`"approvedLocations":["loc-1","loc-2"]`,
		`"approvedLocations":[""]`,
		`"approvedLocations":["loc-1"],"approveAllLocations":true`,
		`"approvedLocations":["loc-1"],"installToFutureLocations":true`,
	} {
		t.Run(extra, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				assert.Equal(t, "/oauth/token", r.URL.Path)
				_, _ = io.WriteString(w, `{"access_token":"agency-access","token_type":"Bearer","expires_in":86400,"userType":"Company","companyId":"agency-1",`+extra+`}`)
			}))
			defer server.Close()
			_, err := newClient(t, server).Exchange(t.Context(), "agency-code")
			var ce *core.Error
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, core.KindInvalid, ce.Kind)
			assert.Equal(t, 1, requests)
		})
	}
}

func TestAgencyExchangeRejectsAnotherWorkspaceOrAgency(t *testing.T) {
	for _, extra := range []string{
		`"locationId":"loc-2"`,
		`"locationId":"loc-1","companyId":"agency-2"`,
		`"locationId":"loc-1","userType":"Company"`,
	} {
		t.Run(extra, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/oauth/token" {
					_, _ = io.WriteString(w, `{"access_token":"agency-access","token_type":"Bearer","expires_in":86400,"userType":"Company","companyId":"agency-1","approvedLocations":["loc-1"]}`)
					return
				}
				_, _ = io.WriteString(w, `{"access_token":"location-access","refresh_token":"location-refresh","token_type":"Bearer","expires_in":86400,`+extra+`}`)
			}))
			defer server.Close()
			_, err := newClient(t, server).Exchange(t.Context(), "agency-code")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "agency-access")
			assert.NotContains(t, err.Error(), "location-refresh")
		})
	}
}

func TestLocationUsesBearerAndVersionHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/locations/loc-1", r.URL.Path)
		assert.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		assert.Equal(t, "2021-07-28", r.Header.Get("Version"))
		_, _ = io.WriteString(w, `{"location":{"id":"loc-1","name":"Test sandbox","phone":"unused",`+
			`"timezone":"Asia/Dubai","country":"AE"}}`)
	}))
	defer server.Close()
	location, err := newClient(t, server).Location(t.Context(), "access-token", "loc-1")
	require.NoError(t, err)
	assert.Equal(t, "loc-1", location.ID)
	assert.Equal(t, "Test sandbox", location.Name)
	assert.Equal(t, "Asia/Dubai", location.TimeZone)
	assert.Equal(t, "AE", location.Country)
}

func TestReadAccessChecksEveryResourceAndStopsAtADeniedRead(t *testing.T) {
	for _, denyFields := range []bool{false, true} {
		t.Run(map[bool]string{false: "allowed", true: "denied fields"}[denyFields], func(t *testing.T) {
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "2021-07-28", r.Header.Get("Version"))
				assert.Equal(t, "ADX/0.1", r.Header.Get("User-Agent"))
				assert.Equal(t, "Bearer private-secret", r.Header.Get("Authorization"))
				switch r.URL.Path {
				case "/contacts/":
					assert.Equal(t, "loc-1", r.URL.Query().Get("locationId"))
					assert.Equal(t, "1", r.URL.Query().Get("limit"))
				case "/locations/loc-1/customFields":
					assert.Equal(t, "all", r.URL.Query().Get("model"))
					if denyFields {
						w.WriteHeader(http.StatusForbidden)
						return
					}
				case "/opportunities/pipelines":
					assert.Equal(t, "loc-1", r.URL.Query().Get("locationId"))
				default:
					t.Error("unexpected read")
				}
				_, _ = io.WriteString(w, `{}`)
			}))
			defer server.Close()
			err := newClient(t, server).ReadAccess(t.Context(), "private-secret", "loc-1")
			if denyFields {
				require.Error(t, err)
				assert.Len(t, paths, 2)
				assert.NotContains(t, err.Error(), "private-secret")
			} else {
				require.NoError(t, err)
				assert.Len(t, paths, 3)
			}
		})
	}
}

func TestFailuresNeverIncludeProviderBodies(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			redirected := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirect" {
					redirected = true
				}
				w.Header().Set("Location", "/redirect")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "access-secret refresh-secret")
			}))
			defer server.Close()
			_, err := newClient(t, server).Refresh(t.Context(), "refresh-secret")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "access-secret")
			assert.NotContains(t, err.Error(), "refresh-secret")
			assert.False(t, redirected)
			if status == 401 || status == 403 || status == 429 {
				var ce *core.Error
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, core.KindConflict, ce.Kind)
			}
		})
	}
}

func TestInvalidAndOversizedResponsesAreRejected(t *testing.T) {
	for _, body := range []string{"secret-response-not-json", strings.Repeat("x", 513<<10), `{"token_type":"MAC"}`} {
		t.Run(body[:min(len(body), 20)], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) }))
			defer server.Close()
			_, err := newClient(t, server).Exchange(t.Context(), "code")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "secret-response")
		})
	}
}

func TestRedirectURIRequiresHTTPSOrLoopback(t *testing.T) {
	for _, uri := range []string{"http://example.com/callback", "https://user:password@example.com/callback", "https://example.com/callback#fragment", "https://example.com/callback?redirect=elsewhere"} {
		_, err := gohighlevel.New(gohighlevel.Config{ClientID: "id", ClientSecret: "secret", RedirectURI: uri})
		require.Error(t, err, uri)
	}
	for _, uri := range []string{callback, "http://127.0.0.1:8080/callback", "https://example.com/callback"} {
		_, err := gohighlevel.New(gohighlevel.Config{ClientID: "id", ClientSecret: "secret", RedirectURI: uri})
		require.NoError(t, err, uri)
	}
}

func TestExpiredRefreshGrantRequiresReconnectWithoutExposingDescription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"private-refresh-token"}`)
	}))
	defer server.Close()
	_, err := newClient(t, server).Refresh(t.Context(), "expired-refresh")
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.Contains(t, ce.Message, "reconnect")
	assert.NotContains(t, err.Error(), "private-refresh-token")
}
