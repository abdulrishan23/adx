package identity_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// fakeOIDC hands back whatever claims the test sets. It records what it was
// asked so the test can check the PKCE verifier and nonce round trip.
type fakeOIDC struct {
	claims                  identity.Claims
	err                     error
	lastVerifier, lastNonce string
	lastState               string
}

func (f *fakeOIDC) AuthorizationURL(state, nonce, verifier string) string {
	f.lastState = state
	return "https://authentik.example/authorize?state=" + url.QueryEscape(state) + "&nonce=" + url.QueryEscape(nonce) + "&v=" + verifier
}

func (f *fakeOIDC) Exchange(_ context.Context, _, verifier, nonceHash string) (identity.Claims, error) {
	f.lastVerifier, f.lastNonce = verifier, nonceHash
	return f.claims, f.err
}

type env struct {
	db       *store.DB
	users    *users.Users
	authn    *userstest.Authentik
	oidc     *fakeOIDC
	identity *identity.Identity
	now      time.Time
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), authn: userstest.NewAuthentik(), oidc: &fakeOIDC{},
		now: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)}
	e.users = users.New(e.db, e.authn)
	e.identity = identity.New(e.db, e.users, e.oidc, identity.Config{
		AllowedGroup: "adx-users", Now: func() time.Time { return e.now },
	})
	return e
}

// add invites someone and points the fake provider at their authentik account.
func (e *env) add(t *testing.T, email string, role core.Role) users.User {
	t.Helper()
	u, err := e.users.Add(t.Context(), core.System(core.ViaStartup), users.AddInput{Email: email, Role: role})
	require.NoError(t, err)
	acct := e.authn.ByEmail(email)
	e.oidc.claims = identity.Claims{Subject: acct.UUID, Email: email, Name: "Sara", Groups: []string{"adx-users"}}
	return u
}

func (e *env) signIn(t *testing.T) identity.Finished {
	t.Helper()
	authURL, err := e.identity.StartSignIn(t.Context(), "/projects?tab=all")
	require.NoError(t, err)
	u, err := url.Parse(authURL)
	require.NoError(t, err)
	done, err := e.identity.FinishSignIn(t.Context(), core.ViaWeb, identity.FinishInput{State: u.Query().Get("state"), Code: "code-1"})
	require.NoError(t, err)
	return done
}

// Catches: a session that doesn't carry the membership's role and
// workspace, a cookie value stored in clear, or the PKCE secret and nonce
// not reaching the exchange.
func TestSignInBeginsAScopedSession(t *testing.T) {
	e := setup(t)
	sara := e.add(t, "sara@reportage.ae", core.RoleMarketer)
	done := e.signIn(t)

	assert.Equal(t, "/projects?tab=all", done.ReturnTo)
	assert.Equal(t, core.RoleMarketer, done.Session.Role)
	assert.Equal(t, core.DefaultWorkspace, done.Session.WorkspaceID)
	assert.Equal(t, "Reportage", done.Session.Workspace)
	assert.Equal(t, sara.ID, done.Session.User.ID)
	assert.NotEmpty(t, e.oidc.lastVerifier)
	assert.Len(t, e.oidc.lastNonce, 64, "the adapter gets the nonce hash to compare")

	var stored int
	require.NoError(t, e.db.Read.QueryRowContext(t.Context(), "SELECT count(*) FROM sessions WHERE id_hash = ?", done.Token).Scan(&stored))
	assert.Zero(t, stored, "the cookie value is never stored, only its hash")

	actor, session, err := e.identity.Resolve(t.Context(), core.ViaWeb, done.Token)
	require.NoError(t, err)
	assert.Equal(t, core.Actor{UserID: sara.ID, Role: core.RoleMarketer, Via: core.ViaWeb, WorkspaceID: 1}, actor)
	assert.Equal(t, "sara@reportage.ae", session.User.Email)
	assert.Equal(t, users.StatusActive, session.User.Status)
}

// Catches: a replayed or forged callback starting a session, or someone
// outside adx-users getting in with a valid authentik account.
func TestCallbackIsSingleUseAndGroupIsRequired(t *testing.T) {
	e := setup(t)
	e.add(t, "sara@reportage.ae", core.RoleViewer)
	authURL, err := e.identity.StartSignIn(t.Context(), "/")
	require.NoError(t, err)
	state := mustQuery(t, authURL, "state")

	_, err = e.identity.FinishSignIn(t.Context(), core.ViaWeb, identity.FinishInput{State: state, Code: "code"})
	require.NoError(t, err)
	_, err = e.identity.FinishSignIn(t.Context(), core.ViaWeb, identity.FinishInput{State: state, Code: "code"})
	requireKind(t, err, core.KindInvalid)
	_, err = e.identity.FinishSignIn(t.Context(), core.ViaWeb, identity.FinishInput{State: "forged-state-value-that-is-long", Code: "code"})
	requireKind(t, err, core.KindInvalid)

	// Expired attempt.
	authURL, err = e.identity.StartSignIn(t.Context(), "/")
	require.NoError(t, err)
	e.now = e.now.Add(11 * time.Minute)
	_, err = e.identity.FinishSignIn(t.Context(), core.ViaWeb, identity.FinishInput{State: mustQuery(t, authURL, "state"), Code: "code"})
	requireKind(t, err, core.KindInvalid)

	// Provider refused, and outside the group.
	authURL, err = e.identity.StartSignIn(t.Context(), "/")
	require.NoError(t, err)
	_, err = e.identity.FinishSignIn(t.Context(), core.ViaWeb, identity.FinishInput{State: mustQuery(t, authURL, "state"), ProviderError: "access_denied"})
	requireKind(t, err, core.KindInvalid)

	e.oidc.claims.Groups = nil
	authURL, err = e.identity.StartSignIn(t.Context(), "/")
	require.NoError(t, err)
	_, err = e.identity.FinishSignIn(t.Context(), core.ViaWeb, identity.FinishInput{State: mustQuery(t, authURL, "state"), Code: "code"})
	assert.ErrorIs(t, err, users.ErrNoAccess)

	// Unknown subject: an authentik account ADX never added.
	e.oidc.claims = identity.Claims{Subject: "stranger-uuid", Email: "sara@reportage.ae", Groups: []string{"adx-users"}}
	authURL, err = e.identity.StartSignIn(t.Context(), "/")
	require.NoError(t, err)
	_, err = e.identity.FinishSignIn(t.Context(), core.ViaWeb, identity.FinishInput{State: mustQuery(t, authURL, "state"), Code: "code"})
	assert.ErrorIs(t, err, users.ErrNoAccess, "the email matching isn't enough; the subject must")
}

// Catches: a disabled person keeping a working session, a role change not
// applying until re-login, and a session outliving the idle or absolute limit.
func TestSessionsFollowTheUserAndExpire(t *testing.T) {
	e := setup(t)
	sara := e.add(t, "sara@reportage.ae", core.RoleViewer)
	done := e.signIn(t)
	admin := core.System(core.ViaStartup)

	_, err := e.users.SetRole(t.Context(), admin, sara.ID, core.RoleMarketer)
	require.NoError(t, err)
	actor, _, err := e.identity.Resolve(t.Context(), core.ViaWeb, done.Token)
	require.NoError(t, err)
	assert.Equal(t, core.RoleMarketer, actor.Role, "the new role applies on the next request")

	_, err = e.users.Disable(t.Context(), admin, sara.ID)
	require.NoError(t, err)
	_, _, err = e.identity.Resolve(t.Context(), core.ViaWeb, done.Token)
	assert.ErrorIs(t, err, identity.ErrNoSession)

	_, err = e.users.Enable(t.Context(), admin, sara.ID)
	require.NoError(t, err)
	_, _, err = e.identity.Resolve(t.Context(), core.ViaWeb, done.Token)
	assert.ErrorIs(t, err, identity.ErrNoSession, "disabling ended the session for good; they sign in again")

	// Idle expiry: 7 days without a request.
	done = e.signIn(t)
	e.now = e.now.Add(6 * 24 * time.Hour)
	_, _, err = e.identity.Resolve(t.Context(), core.ViaWeb, done.Token)
	require.NoError(t, err, "day 6 keeps it alive and touches last_seen")
	e.now = e.now.Add(6 * 24 * time.Hour)
	_, _, err = e.identity.Resolve(t.Context(), core.ViaWeb, done.Token)
	require.NoError(t, err, "day 12, within 7 days of the last request")
	e.now = e.now.Add(8 * 24 * time.Hour)
	_, _, err = e.identity.Resolve(t.Context(), core.ViaWeb, done.Token)
	assert.ErrorIs(t, err, identity.ErrNoSession, "day 20, idle for 8 days")

	// Absolute expiry: 30 days even when used every day.
	done = e.signIn(t)
	for day := 1; day <= 29; day++ {
		e.now = e.now.Add(24 * time.Hour)
		_, _, err = e.identity.Resolve(t.Context(), core.ViaWeb, done.Token)
		require.NoError(t, err, "day %d", day)
	}
	e.now = e.now.Add(24 * time.Hour)
	_, _, err = e.identity.Resolve(t.Context(), core.ViaWeb, done.Token)
	assert.ErrorIs(t, err, identity.ErrNoSession)

	// Sign-out.
	done = e.signIn(t)
	require.NoError(t, e.identity.SignOut(t.Context(), done.Token))
	_, _, err = e.identity.Resolve(t.Context(), core.ViaWeb, done.Token)
	assert.ErrorIs(t, err, identity.ErrNoSession)
	_, _, err = e.identity.Resolve(t.Context(), core.ViaWeb, "not-a-token-at-all-but-long-enough-xx")
	assert.ErrorIs(t, err, identity.ErrNoSession)
}

// Catches: a dev server without authentik hanging on sign-in instead of
// saying what's missing, and an open redirect through return_to.
func TestUnconfiguredAndReturnTo(t *testing.T) {
	e := setup(t)
	unconfigured := identity.New(e.db, e.users, nil, identity.Config{})
	_, err := unconfigured.StartSignIn(t.Context(), "/")
	requireKind(t, err, core.KindConflict)

	e.add(t, "sara@reportage.ae", core.RoleViewer)
	for _, bad := range []string{"https://evil.example/", "//evil.example", "projects", ""} {
		authURL, err := e.identity.StartSignIn(t.Context(), bad)
		require.NoError(t, err)
		done, err := e.identity.FinishSignIn(t.Context(), core.ViaWeb, identity.FinishInput{State: mustQuery(t, authURL, "state"), Code: "c"})
		require.NoError(t, err)
		assert.Equal(t, "/", done.ReturnTo, "return_to %q", bad)
	}
}

func TestProviderFailureStartsNoSession(t *testing.T) {
	e := setup(t)
	e.add(t, "sara@reportage.ae", core.RoleViewer)
	e.oidc.err = errors.New("token endpoint 502")
	authURL, err := e.identity.StartSignIn(t.Context(), "/")
	require.NoError(t, err)
	_, err = e.identity.FinishSignIn(t.Context(), core.ViaWeb, identity.FinishInput{State: mustQuery(t, authURL, "state"), Code: "c"})
	require.Error(t, err)
	var n int
	require.NoError(t, e.db.Read.QueryRowContext(t.Context(), "SELECT count(*) FROM sessions").Scan(&n))
	assert.Zero(t, n)
}

func mustQuery(t *testing.T, raw, key string) string {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u.Query().Get(key)
}

func requireKind(t *testing.T, err error, kind core.Kind) {
	t.Helper()
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, kind, ce.Kind)
}

// Catches: a session landing in a workspace the person doesn't belong to, a
// switch to one they don't belong to, the old cookie surviving a switch, and
// the role not following the membership in the new workspace.
func TestSwitchFollowsMemberships(t *testing.T) {
	e := setup(t)
	sara := e.add(t, "sara@reportage.ae", core.RoleMarketer)
	acme, err := e.db.Writer().CreateWorkspace(t.Context(), store.CreateWorkspaceParams{Slug: "acme", Name: "Acme Homes", CreatedAt: e.now})
	require.NoError(t, err)
	done := e.signIn(t)

	_, _, err = e.identity.Switch(t.Context(), core.ViaWeb, done.Token, acme.ID)
	require.ErrorIs(t, err, users.ErrNoAccess, "no membership, no switch")
	actor, _, err := e.identity.Resolve(t.Context(), core.ViaWeb, done.Token)
	require.NoError(t, err)
	assert.Equal(t, core.DefaultWorkspace, actor.WorkspaceID, "a refused switch leaves the session where it was")

	require.NoError(t, e.db.Writer().CreateMembership(t.Context(), store.CreateMembershipParams{
		WorkspaceID: acme.ID, UserID: sara.ID, Role: string(core.RoleViewer), Enabled: 1, CreatedAt: e.now,
	}))
	token, session, err := e.identity.Switch(t.Context(), core.ViaWeb, done.Token, acme.ID)
	require.NoError(t, err)
	assert.NotEqual(t, done.Token, token)
	assert.Equal(t, acme.ID, session.WorkspaceID)
	assert.Equal(t, "Acme Homes", session.Workspace)
	assert.Equal(t, core.RoleViewer, session.Role, "the role is the membership's role there, not the one carried over")
	_, _, err = e.identity.Resolve(t.Context(), core.ViaWeb, done.Token)
	assert.ErrorIs(t, err, identity.ErrNoSession, "the old cookie opens nothing")
	actor, _, err = e.identity.Resolve(t.Context(), core.ViaWeb, token)
	require.NoError(t, err)
	assert.Equal(t, core.Actor{UserID: sara.ID, Role: core.RoleViewer, Via: core.ViaWeb, WorkspaceID: acme.ID}, actor)

	// Someone who belongs only to Acme signs in straight into Acme.
	require.NoError(t, e.db.Writer().SetMembershipEnabled(t.Context(), store.SetMembershipEnabledParams{Enabled: 0, WorkspaceID: core.DefaultWorkspace, UserID: sara.ID}))
	again := e.signIn(t)
	assert.Equal(t, acme.ID, again.Session.WorkspaceID)
}
