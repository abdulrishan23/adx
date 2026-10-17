package googleads

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/connections"
)

// OAuthConfig is the Google Cloud OAuth client. The Cloud project's Google
// Ads API access level decides what the token may read (developer tokens
// are sunset).
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// Endpoint and UserInfoURL override Google's, for tests.
	Endpoint    *oauth2.Endpoint
	UserInfoURL string
	HTTPClient  *http.Client
}

// OAuth implements connections.AdsIdentity for Google: authorization code
// with PKCE, offline access for a refresh token, and the person's stable
// subject from the OpenID userinfo endpoint.
type OAuth struct {
	cfg      oauth2.Config
	userInfo string
	http     *http.Client
}

func NewOAuth(c OAuthConfig) (*OAuth, error) {
	if c.ClientID == "" || c.ClientSecret == "" || c.RedirectURL == "" {
		return nil, errors.New("google OAuth client ID, secret and redirect URL are required")
	}
	endpoint := endpoints.Google
	if c.Endpoint != nil {
		endpoint = *c.Endpoint
	}
	userInfo := c.UserInfoURL
	if userInfo == "" {
		userInfo = "https://openidconnect.googleapis.com/v1/userinfo"
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &OAuth{cfg: oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, RedirectURL: c.RedirectURL, Endpoint: endpoint,
		Scopes: []string{Scope, "openid"}}, userInfo: userInfo, http: httpClient}, nil
}

func (o *OAuth) AuthorizationURL(state, codeVerifier string) string {
	return o.cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(codeVerifier))
}

func (o *OAuth) Exchange(ctx context.Context, code, codeVerifier string) (connections.AdsTokens, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, o.http)
	tok, err := o.cfg.Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		return connections.AdsTokens{}, core.Unavailable(err, "Google didn't complete the sign-in, try again")
	}
	if tok.RefreshToken == "" {
		return connections.AdsTokens{}, core.Invalid("Google returned no refresh token; remove ADX's access in the Google account and connect again")
	}
	subject, err := o.subject(ctx, tok.AccessToken)
	if err != nil {
		return connections.AdsTokens{}, err
	}
	return connections.AdsTokens{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, ExpiresAt: tok.Expiry, Scopes: scopesOf(tok), Subject: subject}, nil
}

func (o *OAuth) Refresh(ctx context.Context, refreshToken string) (connections.AdsTokens, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, o.http)
	tok, err := o.cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken, Expiry: time.Unix(1, 0)}).Token()
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && (re.ErrorCode == "invalid_grant" || re.Response != nil && re.Response.StatusCode == http.StatusBadRequest) {
			return connections.AdsTokens{}, core.Conflict("Google Ads authorization expired or was revoked; reconnect")
		}
		return connections.AdsTokens{}, core.Unavailable(err, "Google didn't refresh the credentials, try again")
	}
	return connections.AdsTokens{AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, ExpiresAt: tok.Expiry, Scopes: scopesOf(tok)}, nil
}

// subject asks the userinfo endpoint who the token belongs to.
func (o *OAuth) subject(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.userInfo, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	res, err := o.http.Do(req)
	if err != nil {
		return "", core.Unavailable(err, "Google didn't say who signed in, try again")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", core.Unavailable(errors.New("userinfo HTTP "+res.Status), "Google didn't say who signed in, try again")
	}
	var info struct {
		Sub string `json:"sub"`
	}
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil || info.Sub == "" {
		return "", core.Unavailable(err, "Google didn't say who signed in, try again")
	}
	return info.Sub, nil
}

func scopesOf(tok *oauth2.Token) []string {
	if s, ok := tok.Extra("scope").(string); ok && s != "" {
		return strings.Fields(s)
	}
	return nil
}

var _ connections.AdsIdentity = (*OAuth)(nil)
