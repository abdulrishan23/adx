package authentik

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/identity"
)

// OIDCConfig is the `adx-auth` provider in authentik (D-24): confidential
// client, subject mode "user's UUID", RS256, with the groups scope mapping on.
type OIDCConfig struct {
	// Issuer is the provider's issuer URL, such as
	// https://authentik.example/application/o/adx-auth/. Discovery runs
	// against it at startup.
	Issuer       string
	ClientID     string
	ClientSecret string
	// RedirectURL is ADX's callback, derived from the public URL.
	RedirectURL string
	// HTTPClient overrides the client, for tests against a local issuer.
	HTTPClient *http.Client
}

// OIDC implements identity.OIDC against authentik's OpenID Connect provider.
type OIDC struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    oauth2.Config
	http     *http.Client
}

// NewOIDC runs discovery and prepares the verifier. It fails if the issuer
// can't be reached, which is better than a server that accepts no sign-ins
// without saying why.
func NewOIDC(ctx context.Context, cfg OIDCConfig) (*OIDC, error) {
	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURL == "" {
		return nil, errors.New("OIDC issuer, client ID, client secret and redirect URL are required")
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	ctx = oidc.ClientContext(ctx, httpClient)
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery at %s: %w", cfg.Issuer, err)
	}
	return &OIDC{
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		oauth: oauth2.Config{
			ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, RedirectURL: cfg.RedirectURL,
			Endpoint: provider.Endpoint(),
			Scopes:   []string{oidc.ScopeOpenID, "profile", "email", "groups"},
		},
		http: httpClient,
	}, nil
}

func (o *OIDC) AuthorizationURL(state, nonce, codeVerifier string) string {
	return o.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(codeVerifier))
}

// claims is the subset of the ID token ADX reads.
type claims struct {
	Email  string   `json:"email"`
	Name   string   `json:"name"`
	Groups []string `json:"groups"`
}

// Exchange trades the code for tokens, verifies the ID token (signature,
// issuer, audience, expiry) and checks the nonce against nonceHash. Groups
// come from the ID token when authentik puts them there, else from userinfo.
func (o *OIDC) Exchange(ctx context.Context, code, codeVerifier, nonceHash string) (identity.Claims, error) {
	ctx = oidc.ClientContext(ctx, o.http)
	token, err := o.oauth.Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		return identity.Claims{}, core.Unavailable(err, "authentik didn't complete the sign-in, try again")
	}
	rawID, _ := token.Extra("id_token").(string)
	if rawID == "" {
		return identity.Claims{}, core.Unavailable(errors.New("no id_token in response"), "authentik didn't complete the sign-in, try again")
	}
	idToken, err := o.verifier.Verify(ctx, rawID)
	if err != nil {
		return identity.Claims{}, core.Invalid("sign-in token was not valid; start again")
	}
	if idToken.Nonce == "" || identity.Hash(idToken.Nonce) != nonceHash {
		return identity.Claims{}, core.Invalid("sign-in token did not match this browser; start again")
	}
	var c claims
	if err := idToken.Claims(&c); err != nil {
		return identity.Claims{}, core.Invalid("sign-in token was not valid; start again")
	}
	if c.Groups == nil {
		info, err := o.provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
		if err != nil {
			return identity.Claims{}, core.Unavailable(err, "authentik didn't return the account's groups, try again")
		}
		var more claims
		if err := info.Claims(&more); err != nil {
			return identity.Claims{}, core.Unavailable(err, "authentik didn't return the account's groups, try again")
		}
		c.Groups = more.Groups
		if c.Email == "" {
			c.Email = info.Email
		}
		if c.Name == "" {
			c.Name = more.Name
		}
		if c.Groups == nil {
			c.Groups = []string{}
		}
	}
	return identity.Claims{Subject: idToken.Subject, Email: strings.ToLower(c.Email), Name: c.Name, Groups: c.Groups}, nil
}

// LogoutURL is authentik's end-session page, so "sign out everywhere" can
// send the browser there after ADX ends its own session. Empty if the
// provider doesn't advertise one.
func (o *OIDC) LogoutURL() string {
	var extra struct {
		EndSession string `json:"end_session_endpoint"`
	}
	if err := o.provider.Claims(&extra); err != nil {
		return ""
	}
	if u, err := url.Parse(extra.EndSession); err != nil || u.Scheme == "" {
		return ""
	}
	return extra.EndSession
}
