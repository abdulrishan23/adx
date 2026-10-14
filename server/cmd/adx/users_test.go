package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: a subcommand wired to the wrong core function or flag, and core
// errors from the CLI being swallowed.
func TestUsersCommand(t *testing.T) {
	dir := userstest.NewDirectory()
	u := users.New(storetest.Open(t), dir)
	run := func(args ...string) (string, error) {
		var out strings.Builder
		err := runUsers(t.Context(), u, args, &out)
		return out.String(), err
	}

	_, err := run("add", "--email", "cmo@reportage.ae", "--role", "admin")
	require.NoError(t, err)
	_, err = run("add", "--email", "sara@reportage.ae", "--role", "marketer")
	require.NoError(t, err)
	_, err = run("set-role", "--email", "sara@reportage.ae", "--role", "viewer")
	require.NoError(t, err)
	out, err := run("password-link", "--email", "sara@reportage.ae")
	require.NoError(t, err)
	assert.Contains(t, out, "sent the set-password email")
	assert.Equal(t, 2, dir.ByEmail("sara@reportage.ae").Invites)
	_, err = run("disable", "--email", "sara@reportage.ae")
	require.NoError(t, err)

	out, err = run("list")
	require.NoError(t, err)
	assert.Regexp(t, `sara@reportage\.ae\s+-\s+viewer\s+disabled`, out)

	var ce *core.Error
	_, err = run("disable", "--email", "cmo@reportage.ae")
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)

	_, err = run("enable", "--email", "nobody@reportage.ae")
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindNotFound, ce.Kind)

	dir.Fail["SendSetPasswordEmail"] = true
	out, err = run("add", "--email", "omar@reportage.ae", "--role", "viewer")
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindUnavailable, ce.Kind)
	assert.Contains(t, out, "omar@reportage.ae", "a person added without their email is still shown")

	dir.Fail["SendSetPasswordEmail"] = false
	uuid := dir.Existing("tamim@wokeworks.ai", true)
	_, err = run("add", "--email", "tamim@wokeworks.ai", "--role", "admin")
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
	out, err = run("find-account", "--email", "tamim@wokeworks.ai")
	require.NoError(t, err)
	assert.Contains(t, out, uuid)
	out, err = run("add", "--email", "tamim@wokeworks.ai", "--role", "admin", "--authentik-uuid", uuid)
	require.NoError(t, err)
	assert.Contains(t, out, uuid)

	_, err = run("set-role", "--id", "2", "--email", "sara@reportage.ae", "--role", "viewer")
	assert.ErrorContains(t, err, "not both")
	out, err = run("enable", "--id", "2")
	require.NoError(t, err)
	assert.Regexp(t, `sara@reportage\.ae\s+-\s+viewer\s+invited`, out, "she never signed in, so she's invited again")

	_, err = run("promote")
	assert.ErrorContains(t, err, "usage:")
}
