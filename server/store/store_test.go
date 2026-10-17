package store_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/store"
)

// Catches: a later migration that drops, renames or re-types existing
// projects, users or GoHighLevel credentials. It seeds rows at migration 3,
// where the old connections table still exists, then applies every later
// migration and checks the rows and the encrypted secret bytes come through
// unchanged. Every migration added after this test keeps being covered.
func TestMigrationsKeepExistingData(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(filepath.Join(t.TempDir(), "rehearsal.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	require.NoError(t, db.MigrateTo(ctx, 3))
	seededAt := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	secret := []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02}
	_, err = db.Write.ExecContext(ctx, `INSERT INTO projects (name, created_at) VALUES ('The Wadi', ?), ('Oceana', ?)`, seededAt, seededAt)
	require.NoError(t, err)
	_, err = db.Write.ExecContext(ctx, `INSERT INTO connections
		(provider, location_id, company_id, name, encrypted_tokens, scope, token_expires_at, connected_at, verified_at, auth_method)
		VALUES ('gohighlevel', 'loc-1', 'company-1', 'Reportage Dubai', ?, 'contacts.readonly locations.readonly', NULL, ?, ?, 'private_token')`,
		secret, seededAt, seededAt)
	require.NoError(t, err)

	require.NoError(t, db.MigrateTo(ctx, 4))
	_, err = db.Write.ExecContext(ctx, `INSERT INTO users (email, role, status, authentik_pk, authentik_uuid, created_at)
		VALUES ('cmo@reportage.ae', 'admin', 'active', 7, 'uuid-7', ?)`, seededAt)
	require.NoError(t, err)

	require.NoError(t, db.Migrate(ctx))

	q := db.Reader()
	projects, err := q.ListProjects(ctx, 1)
	require.NoError(t, err)
	require.Len(t, projects, 2)
	assert.Equal(t, "Oceana", projects[0].Name)
	assert.Equal(t, "The Wadi", projects[1].Name)
	assert.Equal(t, seededAt, projects[1].CreatedAt.UTC())

	user, err := q.GetUserByEmail(ctx, "cmo@reportage.ae")
	require.NoError(t, err)
	assert.Equal(t, "admin", user.Role)
	assert.Equal(t, "uuid-7", user.AuthentikUuid)

	row, err := q.GetAccount(ctx, store.GetAccountParams{Type: "gohighlevel", ExternalID: "loc-1"})
	require.NoError(t, err)
	assert.Equal(t, "Reportage Dubai", row.Account.Name.String)
	assert.Equal(t, "company-1", row.Account.ParentID.String)
	assert.Equal(t, "private_token", row.Credential.Kind)
	assert.Equal(t, "loc-1", row.Credential.Subject)
	assert.Equal(t, secret, row.Credential.EncryptedSecret, "sealed bytes must survive untouched or nothing decrypts")
	assert.False(t, row.Credential.TokenExpiresAt.Valid)

	// Migrating again is a no-op, so a restart never re-runs a migration.
	require.NoError(t, db.Migrate(ctx))
}

// Catches: a backup that is missing rows written just before it, that
// silently overwrites an existing file, or that can't be opened and
// migrated on its own.
func TestBackupToCopiesEveryCommittedRow(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "live.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.Migrate(ctx))
	_, err = db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: 1, Name: "Verdana", CreatedAt: time.Now().UTC()})
	require.NoError(t, err)

	target := filepath.Join(dir, "backup", "adx.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, db.BackupTo(ctx, target))
	require.Error(t, db.BackupTo(ctx, target), "a second backup must not overwrite the first")

	copied, err := store.Open(target)
	require.NoError(t, err)
	t.Cleanup(func() { copied.Close() })
	pending, err := copied.Pending(ctx)
	require.NoError(t, err)
	assert.False(t, pending)
	projects, err := copied.Reader().ListProjects(ctx, 1)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, "Verdana", projects[0].Name)
}
