package googleads_test

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
	"github.com/WokeWorks/adx/server/providers/googleads"
)

// Catches: the consent URL missing offline access, PKCE or the adwords
// scope; the exchange not sending the verifier; a missing refresh token
// being accepted; and the subject not coming from userinfo.
func TestOAuthExchangeAndRefresh(t *testing.T) {
	var tokenForms []url.Values
	refreshFails := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			require.NoError(t, r.ParseForm())
			tokenForms = append(tokenForms, r.PostForm)
			if r.PostForm.Get("grant_type") == "refresh_token" {
				if refreshFails {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at-2", "token_type": "Bearer", "expires_in": 3599, "scope": "https://www.googleapis.com/auth/adwords openid"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at-1", "refresh_token": "rt-1", "token_type": "Bearer", "expires_in": 3599, "scope": "https://www.googleapis.com/auth/adwords openid"})
		case "/userinfo":
			assert.Equal(t, "Bearer at-1", r.Header.Get("Authorization"))
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": "109876", "email": "ads@reportage.ae"})
		}
	}))
	defer srv.Close()
	o, err := googleads.NewOAuth(googleads.OAuthConfig{ClientID: "cid", ClientSecret: "secret", RedirectURL: "https://adx.example/api/connections/google_ads/callback",
		Endpoint: &oauth2.Endpoint{AuthURL: srv.URL + "/auth", TokenURL: srv.URL + "/token"}, UserInfoURL: srv.URL + "/userinfo", HTTPClient: srv.Client()})
	require.NoError(t, err)

	u, err := url.Parse(o.AuthorizationURL("st", "verifier-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))
	require.NoError(t, err)
	q := u.Query()
	assert.Equal(t, "offline", q.Get("access_type"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.Contains(t, q.Get("scope"), "https://www.googleapis.com/auth/adwords")
	assert.Equal(t, "st", q.Get("state"))
	assert.NotContains(t, u.String(), "verifier-x")

	tok, err := o.Exchange(t.Context(), "code", "verifier-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	require.NoError(t, err)
	assert.Equal(t, "at-1", tok.AccessToken)
	assert.Equal(t, "rt-1", tok.RefreshToken)
	assert.Equal(t, "109876", tok.Subject)
	assert.Contains(t, tok.Scopes, googleads.Scope)
	assert.False(t, tok.ExpiresAt.IsZero())
	require.Len(t, tokenForms, 1)
	assert.Equal(t, "verifier-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", tokenForms[0].Get("code_verifier"))
	assert.Equal(t, "https://adx.example/api/connections/google_ads/callback", tokenForms[0].Get("redirect_uri"))

	refreshed, err := o.Refresh(t.Context(), "rt-1")
	require.NoError(t, err)
	assert.Equal(t, "at-2", refreshed.AccessToken)
	assert.Equal(t, "rt-1", tokenForms[1].Get("refresh_token"))

	refreshFails = true
	_, err = o.Refresh(t.Context(), "rt-1")
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.Contains(t, ce.Message, "reconnect")

	_, err = googleads.NewOAuth(googleads.OAuthConfig{})
	require.Error(t, err)
}
