package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
)

// Catches: a deployed server starting without authentik, sending its token
// to a plain http address, or hiding why adding people fails in dev.
func TestAuthentikConfig(t *testing.T) {
	full := config{
		AuthentikIssuer:       "https://authentik.example/application/o/adx-auth/",
		AuthentikAPIToken:     "token",
		AuthentikGroupID:      "group",
		AuthentikEmailStageID: "stage",
	}

	_, err := authentikAccounts(config{AuthentikIssuer: full.AuthentikIssuer}, false)
	require.Error(t, err)
	assert.ErrorContains(t, err, "ADX_AUTHENTIK_API_TOKEN, ADX_AUTHENTIK_EMAIL_STAGE_ID, ADX_AUTHENTIK_GROUP_ID")

	plain := full
	plain.AuthentikIssuer = "http://authentik.example/application/o/adx-auth/"
	_, err = authentikAccounts(plain, false)
	assert.ErrorContains(t, err, "must use https")

	_, err = authentikAccounts(full, false)
	require.NoError(t, err)

	accounts, err := authentikAccounts(config{}, true)
	require.NoError(t, err, "dev starts without authentik")
	_, err = accounts.FindByEmail(t.Context(), "sara@reportage.ae")
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Contains(t, ce.Message, "authentik isn't configured", "the reason reaches the web, not only the CLI")
}

// Catches: dev mode, which has no sign-in, reachable from other machines.
func TestDevOnlyListensLocally(t *testing.T) {
	for _, addr := range []string{"localhost:8080", "127.0.0.1:8090", "[::1]:8080"} {
		assert.NoError(t, devAddrIsLocal(addr), addr)
	}
	for _, addr := range []string{"0.0.0.0:8080", ":8080", "192.168.1.5:8080"} {
		assert.Error(t, devAddrIsLocal(addr), addr)
	}
}

// Catches: a deployed server starting without sign-in settings, a callback
// behind plain http, or dev refusing to start without authentik.
func TestOIDCConfig(t *testing.T) {
	full := config{
		AuthentikIssuer: "https://authentik.example/application/o/adx-auth/", PublicURL: "https://adx.example",
		OIDCClientID: "adx", OIDCClientSecret: "secret",
	}
	_, err := configuredOIDC(t.Context(), config{AuthentikIssuer: full.AuthentikIssuer}, false)
	require.Error(t, err)
	assert.ErrorContains(t, err, "ADX_OIDC_CLIENT_ID, ADX_OIDC_CLIENT_SECRET, ADX_PUBLIC_URL")

	plain := full
	plain.PublicURL = "http://adx.example"
	_, err = configuredOIDC(t.Context(), plain, false)
	assert.ErrorContains(t, err, "must use https")

	bad := full
	bad.PublicURL = "adx.example/?x=1"
	_, err = configuredOIDC(t.Context(), bad, false)
	assert.ErrorContains(t, err, "origin URL")

	provider, err := configuredOIDC(t.Context(), config{}, true)
	require.NoError(t, err, "dev starts without sign-in")
	assert.Nil(t, provider)

	// With everything set, discovery must reach the issuer; an unreachable one fails loudly.
	unreachable := full
	unreachable.AuthentikIssuer = "https://127.0.0.1:1/application/o/adx-auth/"
	_, err = configuredOIDC(t.Context(), unreachable, false)
	require.Error(t, err)
	assert.ErrorContains(t, err, "discovery")
}
