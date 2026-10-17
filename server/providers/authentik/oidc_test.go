package authentik_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/providers/authentik"
)

// issuer is a tiny OpenID provider: discovery, JWKS, token and userinfo. It
// records the token request so the test can check what ADX sends.
type issuer struct {
	srv       *httptest.Server
	key       *rsa.PrivateKey
	signer    jose.Signer
	tokenForm url.Values
	// nonce and groups are what the next ID token carries.
	nonce       string
	groups      []string
	omitGroups  bool
	userinfoHit int
}

func newIssuer(t *testing.T) *issuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
	require.NoError(t, err)
	is := &issuer{key: key, signer: signer, groups: []string{"adx-users", "staff"}}
	mux := http.NewServeMux()
	is.srv = httptest.NewServer(mux)
	t.Cleanup(is.srv.Close)
	base := is.srv.URL + "/application/o/adx-auth/"
	mux.HandleFunc("/application/o/adx-auth/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": base, "authorization_endpoint": is.srv.URL + "/authorize", "token_endpoint": is.srv.URL + "/token",
			"userinfo_endpoint": is.srv.URL + "/userinfo", "jwks_uri": is.srv.URL + "/jwks", "end_session_endpoint": is.srv.URL + "/end-session",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		is.tokenForm = r.PostForm
		user, pass, _ := r.BasicAuth()
		if (user != "adx-client" || pass != "adx-secret") && r.PostForm.Get("client_secret") != "adx-secret" {
			http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
			return
		}
		if r.PostForm.Get("code") != "good-code" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		cl := map[string]any{
			"iss": base, "sub": "uuid-sara", "aud": "adx-client", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
			"nonce": is.nonce, "email": "Sara@Reportage.ae", "name": "Sara",
		}
		if !is.omitGroups {
			cl["groups"] = is.groups
		}
		raw, err := jwt.Signed(is.signer).Claims(cl).Serialize()
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at-1", "token_type": "Bearer", "expires_in": 300, "id_token": raw})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		is.userinfoHit++
		assert.Equal(t, "Bearer at-1", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "uuid-sara", "email": "sara@reportage.ae", "groups": is.groups})
	})
	return is
}

func (is *issuer) client(t *testing.T) *authentik.OIDC {
	t.Helper()
	c, err := authentik.NewOIDC(t.Context(), authentik.OIDCConfig{
		Issuer: is.srv.URL + "/application/o/adx-auth/", ClientID: "adx-client", ClientSecret: "adx-secret",
		RedirectURL: "https://adx.example/api/auth/callback", HTTPClient: is.srv.Client(),
	})
	require.NoError(t, err)
	return c
}

// Catches: the authorize URL missing PKCE or the nonce, the exchange not
// sending the verifier and redirect URI, groups or email read wrongly, and
// an ID token for another browser's nonce being accepted.
func TestOIDCRoundTrip(t *testing.T) {
	is := newIssuer(t)
	c := is.client(t)

	authURL := c.AuthorizationURL("state-1", "nonce-1", "verifier-value-that-is-long-enough-for-pkce-xxxxxxxxxx")
	u, err := url.Parse(authURL)
	require.NoError(t, err)
	q := u.Query()
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "adx-client", q.Get("client_id"))
	assert.Equal(t, "state-1", q.Get("state"))
	assert.Equal(t, "nonce-1", q.Get("nonce"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.NotEmpty(t, q.Get("code_challenge"))
	assert.NotContains(t, authURL, "verifier-value", "the verifier itself never leaves the server")
	assert.Contains(t, q.Get("scope"), "groups")
	assert.Equal(t, "https://adx.example/api/auth/callback", q.Get("redirect_uri"))

	is.nonce = "nonce-1"
	claims, err := c.Exchange(t.Context(), "good-code", "verifier-value-that-is-long-enough-for-pkce-xxxxxxxxxx", identity.Hash("nonce-1"))
	require.NoError(t, err)
	assert.Equal(t, identity.Claims{Subject: "uuid-sara", Email: "sara@reportage.ae", Name: "Sara", Groups: []string{"adx-users", "staff"}}, claims)
	assert.Equal(t, "good-code", is.tokenForm.Get("code"))
	assert.Equal(t, "verifier-value-that-is-long-enough-for-pkce-xxxxxxxxxx", is.tokenForm.Get("code_verifier"))
	assert.Equal(t, "https://adx.example/api/auth/callback", is.tokenForm.Get("redirect_uri"))
	assert.Zero(t, is.userinfoHit, "groups in the ID token mean no userinfo call")

	is.omitGroups = true
	claims, err = c.Exchange(t.Context(), "good-code", "verifier-value-that-is-long-enough-for-pkce-xxxxxxxxxx", identity.Hash("nonce-1"))
	require.NoError(t, err)
	assert.Equal(t, []string{"adx-users", "staff"}, claims.Groups)
	assert.Equal(t, 1, is.userinfoHit)

	_, err = c.Exchange(t.Context(), "good-code", "verifier-value-that-is-long-enough-for-pkce-xxxxxxxxxx", identity.Hash("someone-elses-nonce"))
	requireKind(t, err, core.KindInvalid)

	_, err = c.Exchange(t.Context(), "bad-code", "verifier-value-that-is-long-enough-for-pkce-xxxxxxxxxx", identity.Hash("nonce-1"))
	requireKind(t, err, core.KindUnavailable)

	assert.True(t, strings.HasSuffix(c.LogoutURL(), "/end-session"))
}

// Catches: a token signed by someone else being accepted.
func TestOIDCRejectsATokenSignedWithAnotherKey(t *testing.T) {
	is := newIssuer(t)
	c := is.client(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	is.signer, err = jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: other}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
	require.NoError(t, err)
	is.nonce = "n"
	_, err = c.Exchange(t.Context(), "good-code", "verifier-value-that-is-long-enough-for-pkce-xxxxxxxxxx", identity.Hash("n"))
	requireKind(t, err, core.KindInvalid)
}

func TestOIDCNeedsAReachableIssuer(t *testing.T) {
	_, err := authentik.NewOIDC(t.Context(), authentik.OIDCConfig{Issuer: "http://127.0.0.1:1/x/", ClientID: "a", ClientSecret: "b", RedirectURL: "https://adx.example/cb",
		HTTPClient: &http.Client{Timeout: time.Second}})
	require.Error(t, err)
	_, err = authentik.NewOIDC(t.Context(), authentik.OIDCConfig{})
	require.Error(t, err)
}

func requireKind(t *testing.T, err error, kind core.Kind) {
	t.Helper()
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, kind, ce.Kind)
}
