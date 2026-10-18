package api_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/registry"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type noAccounts struct{}

func (noAccounts) ConnectedAccountIDs(context.Context, core.Actor) ([]int64, error) { return nil, nil }

// newDevServer is a dev server with the registry, acting as the dev person
// startup makes when withDevUser is set, else as the bare dev admin.
func newDevServer(t *testing.T, withDevUser bool) (*httptest.Server, *store.DB, *users.User) {
	t.Helper()
	db := storetest.Open(t)
	var devUser *users.User
	if withDevUser {
		u, err := users.New(db, userstest.NewAuthentik()).EnsureDev(t.Context(), core.System(core.ViaStartup))
		require.NoError(t, err)
		devUser = &u
	}
	srv := httptest.NewServer(api.NewRouter(api.Deps{
		Log: slog.New(slog.DiscardHandler), Dev: true, DevUser: devUser,
		Registry: registry.New(db, registry.Config{Accounts: noAccounts{}}),
	}))
	t.Cleanup(srv.Close)
	return srv, db, devUser
}

const skillDraft = `{"key":"research-pass","document":{"name":"Research pass","instructions":"Read the competitor page and list offers.","evidenceScope":["public"],"permittedTools":[],
"inputSchema":{"type":"object","required":["url"],"properties":{"url":{"type":"string"}}},
"outputSchema":{"type":"object","required":["offers"],"additionalProperties":false,"properties":{"offers":{"type":"array","items":{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}}}}}}`

// Catches: the dev admin seeing Activate and being told it has no
// permission, an activation with nobody recorded as having done it, and
// the session not naming the dev person.
func TestDevAdminActivatesAsTheDevPerson(t *testing.T) {
	srv, db, dev := newDevServer(t, true)

	res, body := get(t, srv, "/api/session", nil, nil)
	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, body, `"dev":true`)
	assert.Contains(t, body, `"email":"dev@adx.localhost"`)

	res, err := http.Post(srv.URL+"/api/skills", "application/json", strings.NewReader(skillDraft))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, res.StatusCode)

	res, err = http.Post(srv.URL+"/api/registry/1/activate", "application/json", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	row, err := db.Reader().GetRegistryEntry(t.Context(), store.GetRegistryEntryParams{ID: 1, WorkspaceID: core.DefaultWorkspace})
	require.NoError(t, err)
	assert.Equal(t, "active", row.State)
	require.True(t, row.ActivatedBy.Valid, "the activation must record who did it")
	assert.Equal(t, dev.ID, row.ActivatedBy.Int64)
}

// Catches: a dev server without a dev person quietly activating with no
// actor, which would leave activated_by empty.
func TestDevAdminWithoutAPersonCannotActivate(t *testing.T) {
	srv, _, _ := newDevServer(t, false)
	res, err := http.Post(srv.URL+"/api/skills", "application/json", strings.NewReader(skillDraft))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, res.StatusCode)
	res, err = http.Post(srv.URL+"/api/registry/1/activate", "application/json", nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
}
