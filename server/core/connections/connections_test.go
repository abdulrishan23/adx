package connections_test

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

var (
	admin   = core.Actor{UserID: 1, Role: core.RoleAdmin, Via: core.ViaWeb}
	viewer  = core.Actor{UserID: 2, Role: core.RoleViewer, Via: core.ViaWeb}
	binding = strings.Repeat("browser", 8)
)

type fakeGHL struct {
	tokens, refreshed                                   connections.Tokens
	exchangeErr, refreshErr, locationErr, readAccessErr error
	locationID, timeZone, country                       string
	exchanges, refreshes, reads                         int
	lastAccess, lastRefresh                             string
}

func (f *fakeGHL) AuthorizationURL(state string) string {
	return "https://example.com/install?state=" + state
}
func (f *fakeGHL) Exchange(context.Context, string) (connections.Tokens, error) {
	f.exchanges++
	return f.tokens, f.exchangeErr
}
func (f *fakeGHL) Refresh(_ context.Context, token string) (connections.Tokens, error) {
	f.refreshes++
	f.lastRefresh = token
	return f.refreshed, f.refreshErr
}
func (f *fakeGHL) Location(_ context.Context, token, id string) (connections.Location, error) {
	f.reads++
	f.lastAccess = token
	if f.locationID != "" {
		id = f.locationID
	}
	return connections.Location{ID: id, Name: "Sandbox " + id, TimeZone: f.timeZone, Country: f.country}, f.locationErr
}

func (f *fakeGHL) ReadAccess(_ context.Context, token, _ string) error {
	f.lastAccess = token
	return f.readAccessErr
}

type fixture struct {
	db       *store.DB
	service  *connections.Connections
	provider *fakeGHL
	now      time.Time
	key      []byte
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		db: storetest.Open(t), now: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
		key:      bytes.Repeat([]byte{7}, 32),
		provider: &fakeGHL{},
	}
	f.provider.tokens = validTokens("location-1", "original")
	f.provider.refreshed = validTokens("location-1", "rotated")
	user, err := f.db.Writer().CreateUser(t.Context(), store.CreateUserParams{
		Email: "admin@example.com", Role: string(core.RoleAdmin), Status: "active",
		AuthentikPk: 1, AuthentikUuid: "admin-uuid", CreatedAt: f.now,
	})
	require.NoError(t, err)
	require.Equal(t, admin.UserID, user.ID)
	f.service, err = connections.New(f.db, f.provider, connections.Config{EncryptionKey: f.key, Now: func() time.Time { return f.now }})
	require.NoError(t, err)
	return f
}

func validTokens(location, suffix string) connections.Tokens {
	return connections.Tokens{
		AccessToken: "access-" + suffix, RefreshToken: "refresh-" + suffix, ExpiresIn: 3600,
		Scope: strings.Join(connections.RequiredScopes, " "), UserType: "Location", LocationID: location, CompanyID: "company-1",
	}
}

func (f *fixture) start(t *testing.T) connections.FinishInput {
	t.Helper()
	start, err := f.service.Start(t.Context(), admin, binding)
	require.NoError(t, err)
	u, err := url.Parse(start.AuthorizationURL)
	require.NoError(t, err)
	return connections.FinishInput{State: u.Query().Get("state"), BrowserBinding: binding, Code: "one-time-code"}
}

func (f *fixture) connect(t *testing.T) connections.Connection {
	t.Helper()
	result, err := f.service.Finish(t.Context(), admin, f.start(t))
	require.NoError(t, err)
	return result
}

func (f *fixture) stored(t *testing.T, id string) store.GetAccountRow {
	t.Helper()
	row, err := f.db.Reader().GetAccount(t.Context(), store.GetAccountParams{Type: connections.Provider, ExternalID: id})
	require.NoError(t, err)
	return row
}

func (f *fixture) credentialCount(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, f.db.Read.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM credentials").Scan(&n))
	return n
}

func TestConnectKeepsSecretsEncryptedAndSurvivesRestart(t *testing.T) {
	f := newFixture(t)
	result := f.connect(t)
	require.Equal(t, "location-1", result.LocationID)
	assert.Equal(t, "Sandbox location-1", result.Name)
	assert.Equal(t, f.now, result.VerifiedAt)
	row := f.stored(t, result.LocationID)
	assert.NotContains(t, string(row.Credential.EncryptedSecret), f.provider.tokens.AccessToken)
	assert.NotContains(t, string(row.Credential.EncryptedSecret), f.provider.tokens.RefreshToken)
	assert.Equal(t, sql.NullInt64{Int64: admin.UserID, Valid: true}, row.Credential.ConnectedBy)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "access-")
	assert.NotContains(t, string(body), "refresh-")
	restarted, err := connections.New(f.db, f.provider, connections.Config{EncryptionKey: f.key, Now: func() time.Time { return f.now }})
	require.NoError(t, err)
	_, err = restarted.Check(t.Context(), admin, result.LocationID)
	require.NoError(t, err)
	assert.Equal(t, "access-original", f.provider.lastAccess)
}

func TestReconnectUpdatesOnlyThatLocation(t *testing.T) {
	f := newFixture(t)
	first := f.connect(t)
	before := f.stored(t, first.LocationID)
	f.now = f.now.Add(time.Hour)
	f.provider.tokens = validTokens("location-1", "replacement")
	second := f.connect(t)
	assert.Equal(t, first.ConnectedAt, second.ConnectedAt)
	assert.Equal(t, f.now, second.VerifiedAt)
	assert.NotEqual(t, before.Credential.EncryptedSecret, f.stored(t, "location-1").Credential.EncryptedSecret)
	f.provider.tokens = validTokens("location-2", "second-location")
	f.connect(t)
	assert.Equal(t, 2, f.credentialCount(t))
	list, err := f.service.List(t.Context(), viewer)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "location-1", list[0].LocationID)
	assert.Equal(t, "location-2", list[1].LocationID)
	_, err = f.service.Check(t.Context(), admin, "location-1")
	require.NoError(t, err)
	assert.Equal(t, "access-replacement", f.provider.lastAccess)
}

func TestInvalidCallbacksDoNotConnect(t *testing.T) {
	tests := []struct {
		name     string
		change   func(*fixture, *connections.FinishInput, *core.Actor)
		wantKind core.Kind
	}{
		{"viewer", func(_ *fixture, _ *connections.FinishInput, a *core.Actor) { *a = viewer }, core.KindForbidden},
		{"unknown state", func(_ *fixture, in *connections.FinishInput, _ *core.Actor) { in.State = strings.Repeat("x", 26) }, core.KindInvalid},
		{"missing browser", func(_ *fixture, in *connections.FinishInput, _ *core.Actor) { in.BrowserBinding = "" }, core.KindInvalid},
		{"different browser", func(_ *fixture, in *connections.FinishInput, _ *core.Actor) {
			in.BrowserBinding = strings.Repeat("other", 10)
		}, core.KindInvalid},
		{"different admin", func(_ *fixture, _ *connections.FinishInput, a *core.Actor) { a.UserID = 42 }, core.KindInvalid},
		{"expired state", func(f *fixture, _ *connections.FinishInput, _ *core.Actor) { f.now = f.now.Add(10 * time.Minute) }, core.KindInvalid},
		{"denied access", func(_ *fixture, in *connections.FinishInput, _ *core.Actor) { in.ProviderError = "access_denied" }, core.KindInvalid},
		{"missing code", func(_ *fixture, in *connections.FinishInput, _ *core.Actor) { in.Code = "" }, core.KindInvalid},
		{"missing scope", func(f *fixture, _ *connections.FinishInput, _ *core.Actor) {
			f.provider.tokens.Scope = "contacts.readonly"
		}, core.KindInvalid},
		{"agency token", func(f *fixture, _ *connections.FinishInput, _ *core.Actor) { f.provider.tokens.UserType = "Company" }, core.KindInvalid},
		{"invalid location", func(f *fixture, _ *connections.FinishInput, _ *core.Actor) { f.provider.tokens.LocationID = "../other" }, core.KindInvalid},
		{"incomplete tokens", func(f *fixture, _ *connections.FinishInput, _ *core.Actor) { f.provider.tokens.RefreshToken = "" }, core.KindInvalid},
		{"expired token", func(f *fixture, _ *connections.FinishInput, _ *core.Actor) { f.provider.tokens.ExpiresIn = 0 }, core.KindInvalid},
		{"wrong read location", func(f *fixture, _ *connections.FinishInput, _ *core.Actor) { f.provider.locationID = "location-2" }, core.KindInvalid},
		{"exchange failure", func(f *fixture, _ *connections.FinishInput, _ *core.Actor) {
			f.provider.exchangeErr = errors.New("provider unavailable")
		}, 0},
		{"location read failure", func(f *fixture, _ *connections.FinishInput, _ *core.Actor) {
			f.provider.locationErr = errors.New("provider unavailable")
		}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			in := f.start(t)
			actor := admin
			tt.change(f, &in, &actor)
			_, err := f.service.Finish(t.Context(), actor, in)
			require.Error(t, err)
			if tt.wantKind != 0 {
				var ce *core.Error
				require.ErrorAs(t, err, &ce)
				assert.Equal(t, tt.wantKind, ce.Kind)
			}
			list, err := f.service.List(t.Context(), admin)
			require.NoError(t, err)
			assert.Empty(t, list)
		})
	}
}

func TestWrongBrowserDoesNotConsumeLegitimateAttempt(t *testing.T) {
	f := newFixture(t)
	in := f.start(t)
	wrong := in
	wrong.BrowserBinding = strings.Repeat("different", 6)
	_, err := f.service.Finish(t.Context(), admin, wrong)
	require.Error(t, err)
	assert.Zero(t, f.provider.exchanges)
	_, err = f.service.Finish(t.Context(), admin, in)
	require.NoError(t, err)
}

func TestOnlyOneConcurrentCallbackExchangesTheCode(t *testing.T) {
	f := newFixture(t)
	in := f.start(t)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { _, err := f.service.Finish(t.Context(), admin, in); results <- err })
	}
	wg.Wait()
	close(results)
	succeeded, failed := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			failed++
		}
	}
	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 1, failed)
	assert.Equal(t, 1, f.provider.exchanges)
}

func TestFailedReconnectPreservesExistingConnectionAndConsumesState(t *testing.T) {
	f := newFixture(t)
	f.connect(t)
	before := f.stored(t, "location-1")
	in := f.start(t)
	f.provider.locationErr = errors.New("temporary read failure")
	_, err := f.service.Finish(t.Context(), admin, in)
	require.Error(t, err)
	assert.Equal(t, before.Credential.EncryptedSecret, f.stored(t, "location-1").Credential.EncryptedSecret)
	f.provider.locationErr = nil
	_, err = f.service.Finish(t.Context(), admin, in)
	require.Error(t, err)
	assert.Equal(t, 2, f.provider.exchanges)
}

func TestRefreshPersistsRotationEvenWhenTheFollowingReadFails(t *testing.T) {
	f := newFixture(t)
	f.connect(t)
	f.now = f.now.Add(time.Hour)
	f.provider.locationErr = errors.New("temporary read failure")
	_, err := f.service.Check(t.Context(), admin, "location-1")
	require.Error(t, err)
	assert.Equal(t, 1, f.provider.refreshes)
	assert.Equal(t, "refresh-original", f.provider.lastRefresh)
	assert.True(t, f.stored(t, "location-1").Credential.TokenExpiresAt.Time.After(f.now))
	f.provider.locationErr = nil
	_, err = f.service.Check(t.Context(), admin, "location-1")
	require.NoError(t, err)
	assert.Equal(t, 1, f.provider.refreshes)
	assert.Equal(t, "access-rotated", f.provider.lastAccess)
}

func TestConcurrentChecksRefreshOnlyOnce(t *testing.T) {
	f := newFixture(t)
	f.connect(t)
	f.now = f.now.Add(time.Hour)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := f.service.Check(t.Context(), admin, "location-1"); errs <- err })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, 1, f.provider.refreshes)
}

func TestRefreshCannotChangeLocation(t *testing.T) {
	f := newFixture(t)
	f.connect(t)
	before := f.stored(t, "location-1")
	f.now = f.now.Add(time.Hour)
	f.provider.refreshed = validTokens("location-2", "wrong-location")
	_, err := f.service.Check(t.Context(), admin, "location-1")
	require.Error(t, err)
	assert.Equal(t, before.Credential.EncryptedSecret, f.stored(t, "location-1").Credential.EncryptedSecret)
	assert.Equal(t, 1, f.provider.reads)
}

func TestCredentialsCannotBeDecryptedWithAnotherKeyOrLocation(t *testing.T) {
	t.Run("different key", func(t *testing.T) {
		f := newFixture(t)
		f.connect(t)
		other, err := connections.New(f.db, f.provider, connections.Config{EncryptionKey: bytes.Repeat([]byte{8}, 32)})
		require.NoError(t, err)
		_, err = other.Check(t.Context(), admin, "location-1")
		require.Error(t, err)
		assert.Equal(t, 1, f.provider.reads)
	})
	t.Run("different location", func(t *testing.T) {
		f := newFixture(t)
		f.connect(t)
		first := f.stored(t, "location-1")
		f.provider.tokens = validTokens("location-2", "second")
		f.connect(t)
		_, err := f.db.Write.ExecContext(t.Context(), "UPDATE credentials SET encrypted_secret = ? WHERE subject = ?", first.Credential.EncryptedSecret, "location-2")
		require.NoError(t, err)
		_, err = f.service.Check(t.Context(), admin, "location-2")
		require.Error(t, err)
		assert.Equal(t, 2, f.provider.reads)
	})
}

func TestPermissionsAndMissingConnection(t *testing.T) {
	f := newFixture(t)
	for _, actor := range []core.Actor{viewer, {Role: core.RoleMarketer}, {}} {
		_, err := f.service.Start(t.Context(), actor, binding)
		assert.ErrorIs(t, err, core.ErrForbidden)
		_, err = f.service.Check(t.Context(), actor, "location-1")
		assert.ErrorIs(t, err, core.ErrForbidden)
	}
	_, err := f.service.List(t.Context(), core.Actor{})
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = f.service.Check(t.Context(), admin, "unknown")
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindNotFound, ce.Kind)
}

func TestPrivateTokenIsEncryptedAndNeverUsesOAuthRefresh(t *testing.T) {
	f := newFixture(t)
	result, err := f.service.ConnectPrivate(t.Context(), admin, "location-1", "private-secret")
	require.NoError(t, err)
	assert.Equal(t, "private_token", result.AuthMethod)
	assert.Nil(t, result.TokenExpiresAt)
	assert.False(t, f.stored(t, "location-1").Credential.TokenExpiresAt.Valid)
	assert.NotContains(t, string(f.stored(t, "location-1").Credential.EncryptedSecret), "private-secret")
	body, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "private-secret")
	f.now = f.now.AddDate(1, 0, 0)
	f.provider.refreshErr = errors.New("private tokens must never refresh")
	restarted, err := connections.New(f.db, f.provider, connections.Config{EncryptionKey: f.key, Now: func() time.Time { return f.now }})
	require.NoError(t, err)
	_, err = restarted.Check(t.Context(), admin, "location-1")
	require.NoError(t, err)
	assert.Zero(t, f.provider.refreshes)
	assert.Equal(t, "private-secret", f.provider.lastAccess)
}

func TestPrivateTokenReplacementRequiresAllReadPermissionsAndKeepsOtherLocations(t *testing.T) {
	f := newFixture(t)
	f.connect(t)
	first := f.stored(t, "location-1")
	f.provider.readAccessErr = errors.New("custom fields access denied")
	_, err := f.service.ConnectPrivate(t.Context(), admin, "location-1", "rejected-private-secret")
	require.Error(t, err)
	assert.Equal(t, first.Credential.EncryptedSecret, f.stored(t, "location-1").Credential.EncryptedSecret)
	f.provider.readAccessErr = nil
	_, err = f.service.ConnectPrivate(t.Context(), admin, "location-2", "other-private-secret")
	require.NoError(t, err)
	assert.Equal(t, first.Credential.EncryptedSecret, f.stored(t, "location-1").Credential.EncryptedSecret)
	_, err = f.service.ConnectPrivate(t.Context(), admin, "location-1", "replacement-private-secret")
	require.NoError(t, err)
	assert.Equal(t, "private_token", f.stored(t, "location-1").Credential.Kind)
	assert.False(t, f.stored(t, "location-1").Credential.TokenExpiresAt.Valid)
	list, err := f.service.List(t.Context(), viewer)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestPrivateConnectionRejectsUnauthorizedActorAndUnexpectedWorkspace(t *testing.T) {
	f := newFixture(t)
	_, err := f.service.ConnectPrivate(t.Context(), viewer, "location-1", "secret")
	require.ErrorIs(t, err, core.ErrForbidden)
	assert.Zero(t, f.provider.reads)
	f.provider.locationID = "location-2"
	_, err = f.service.ConnectPrivate(t.Context(), admin, "location-1", "secret")
	require.Error(t, err)
	list, err := f.service.List(t.Context(), admin)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestCheckRefreshesLocationTimeZoneAndCountry(t *testing.T) {
	f := newFixture(t)
	f.provider.timeZone, f.provider.country = "Asia/Dubai", "AE"
	f.connect(t)
	row := f.stored(t, "location-1")
	assert.Equal(t, "Asia/Dubai", row.Account.TimeZone.String)
	assert.JSONEq(t, `{"country":"AE"}`, row.Account.Details)
	f.provider.timeZone, f.provider.country = "Asia/Kolkata", "IN"
	_, err := f.service.Check(t.Context(), admin, "location-1")
	require.NoError(t, err)
	row = f.stored(t, "location-1")
	assert.Equal(t, "Asia/Kolkata", row.Account.TimeZone.String)
	assert.JSONEq(t, `{"country":"IN"}`, row.Account.Details)
}

// The GHL-only table sealed tokens with "gohighlevel:<location>". A location
// saved that way must keep working after the move to credentials and accounts.
func TestLocationSavedBeforeCredentialsAndAccountsStillWorks(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.MigrateTo(ctx, 3))
	key := bytes.Repeat([]byte{7}, 32)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	aead, err := cipher.NewGCMWithRandomNonce(block)
	require.NoError(t, err)
	sealed := aead.Seal(nil, nil, []byte(`{"AccessToken":"private-secret","RefreshToken":""}`), []byte("gohighlevel:location-1"))
	connectedAt := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	_, err = db.Write.ExecContext(ctx, `INSERT INTO connections
		(provider, location_id, company_id, name, encrypted_tokens, scope, connected_at, verified_at, auth_method)
		VALUES ('gohighlevel', 'location-1', 'company-1', 'Wokeworks', ?, ?, ?, ?, 'private_token')`,
		sealed, strings.Join(connections.RequiredScopes, " "), connectedAt, connectedAt)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))

	provider := &fakeGHL{}
	service, err := connections.New(db, provider, connections.Config{EncryptionKey: key})
	require.NoError(t, err)
	list, err := service.List(ctx, viewer)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, connections.Connection{
		Provider: connections.Provider, LocationID: "location-1", CompanyID: "company-1", Name: "Wokeworks",
		Scopes: connections.RequiredScopes, AuthMethod: "private_token", ConnectedAt: connectedAt, VerifiedAt: connectedAt,
	}, list[0])
	_, err = service.Check(ctx, admin, "location-1")
	require.NoError(t, err)
	assert.Equal(t, "private-secret", provider.lastAccess)
}

// Catches: a connection whose read failed still showing as connected, health
// being stored instead of derived from the last check, or a failed check
// inventing a placeholder account.
func TestAccountsShowHealthFromProvenCapabilities(t *testing.T) {
	f := newFixture(t)
	_, err := f.service.ConnectPrivate(t.Context(), admin, "loc-1", "pit-token")
	require.NoError(t, err)

	accounts, err := f.service.ListAccounts(t.Context(), viewer)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	assert.Equal(t, connections.HealthConnected, accounts[0].Health)
	require.Len(t, accounts[0].Capabilities, 1)
	assert.Equal(t, connections.CapabilityReadLeads, accounts[0].Capabilities[0].Name)
	assert.Equal(t, "verified", accounts[0].Capabilities[0].State)
	assert.Nil(t, accounts[0].Capabilities[0].LastError)

	f.now = f.now.Add(25 * time.Hour)
	accounts, err = f.service.ListAccounts(t.Context(), viewer)
	require.NoError(t, err)
	assert.Equal(t, connections.HealthStale, accounts[0].Health, "a day without a check is stale")

	f.provider.readAccessErr = core.Conflict("GoHighLevel access was rejected; check the connection token and permissions")
	_, err = f.service.Check(t.Context(), admin, "loc-1")
	require.Error(t, err)
	accounts, err = f.service.ListAccounts(t.Context(), viewer)
	require.NoError(t, err)
	require.Len(t, accounts, 1, "no fixture account appears when the real one fails")
	assert.Equal(t, connections.HealthNeedsPermission, accounts[0].Health)
	assert.Equal(t, "needs_permission", accounts[0].Capabilities[0].State)
	assert.Contains(t, *accounts[0].Capabilities[0].LastError, "rejected")
	assert.NotNil(t, accounts[0].Capabilities[0].VerifiedAt, "the last success is kept")

	f.provider.readAccessErr = errors.New("dial tcp: connection refused to https://token-in-url")
	_, err = f.service.Check(t.Context(), admin, "loc-1")
	require.Error(t, err)
	accounts, err = f.service.ListAccounts(t.Context(), viewer)
	require.NoError(t, err)
	assert.Equal(t, connections.HealthFailed, accounts[0].Health)
	assert.Equal(t, "the provider request failed", *accounts[0].Capabilities[0].LastError, "raw errors never reach the screen")

	f.provider.readAccessErr = nil
	_, err = f.service.Check(t.Context(), admin, "loc-1")
	require.NoError(t, err)
	accounts, err = f.service.ListAccounts(t.Context(), viewer)
	require.NoError(t, err)
	assert.Equal(t, connections.HealthConnected, accounts[0].Health)

	_, err = f.service.ListAccounts(t.Context(), core.Actor{})
	assert.ErrorIs(t, err, core.ErrForbidden)
}
