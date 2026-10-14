package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
)

// Catches: a deployed server starting without authentik, sending its token
// to a plain http address, or hiding why adding people fails in dev.
func TestDirectoryConfig(t *testing.T) {
	full := config{
		AuthentikIssuer:       "https://authentik.example/application/o/adx-auth/",
		AuthentikAPIToken:     "token",
		AuthentikGroupID:      "group",
		AuthentikEmailStageID: "stage",
	}

	_, err := directory(config{AuthentikIssuer: full.AuthentikIssuer}, false)
	require.Error(t, err)
	assert.ErrorContains(t, err, "ADX_AUTHENTIK_API_TOKEN, ADX_AUTHENTIK_EMAIL_STAGE_ID, ADX_AUTHENTIK_GROUP_ID")

	plain := full
	plain.AuthentikIssuer = "http://authentik.example/application/o/adx-auth/"
	_, err = directory(plain, false)
	assert.ErrorContains(t, err, "must use https")

	_, err = directory(full, false)
	require.NoError(t, err)

	dir, err := directory(config{}, true)
	require.NoError(t, err, "dev starts without authentik")
	_, err = dir.FindByEmail(t.Context(), "sara@reportage.ae")
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
