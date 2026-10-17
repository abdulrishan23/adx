package meta_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/providers/meta"
)

// Catches: the short-lived token stored instead of the long-lived one, the
// exchange sending the app secret anywhere but the token endpoints, and the
// subject not coming from /me.
func TestOAuthExchangeGetsLongLivedToken(t *testing.T) {
	var exchanges []url.Values
	expired := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			require.NoError(t, r.ParseForm())
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "short-1", "token_type": "bearer", "expires_in": 5000})
		case "/v26.0/oauth/access_token":
			exchanges = append(exchanges, r.URL.Query())
			if expired {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"Error validating access token","type":"OAuthException","code":190}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "long-1", "token_type": "bearer", "expires_in": 5183944})
		case "/v26.0/me":
			assert.Equal(t, "Bearer long-1", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "10001"})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	o, err := meta.NewOAuth(meta.OAuthConfig{AppID: "app", AppSecret: "secret", RedirectURL: "https://adx.example/api/connections/meta/callback",
		Endpoint: &oauth2.Endpoint{AuthURL: srv.URL + "/dialog", TokenURL: srv.URL + "/token"}, GraphBaseURL: srv.URL, HTTPClient: srv.Client()})
	require.NoError(t, err)

	u, _ := url.Parse(o.AuthorizationURL("st", ""))
	assert.Contains(t, u.Query().Get("scope"), "ads_read")
	assert.Equal(t, "st", u.Query().Get("state"))

	tok, err := o.Exchange(t.Context(), "code", "")
	require.NoError(t, err)
	assert.Equal(t, "long-1", tok.AccessToken)
	assert.Empty(t, tok.RefreshToken, "Meta has no refresh token")
	assert.Equal(t, "10001", tok.Subject)
	assert.False(t, tok.ExpiresAt.IsZero())
	require.Len(t, exchanges, 1)
	assert.Equal(t, "fb_exchange_token", exchanges[0].Get("grant_type"))
	assert.Equal(t, "short-1", exchanges[0].Get("fb_exchange_token"))
	assert.Equal(t, "secret", exchanges[0].Get("client_secret"))

	again, err := o.Refresh(t.Context(), "long-1")
	require.NoError(t, err)
	assert.Equal(t, "long-1", again.AccessToken)
	assert.Equal(t, "long-1", exchanges[1].Get("fb_exchange_token"))

	expired = true
	_, err = o.Refresh(t.Context(), "long-1")
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.Contains(t, ce.Message, "reconnect")
}
