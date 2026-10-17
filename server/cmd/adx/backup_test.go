package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/store"
)

// Catches: production starting with an empty database on an unmounted disk,
// or dev refusing to start because data/ isn't there yet.
func TestProductionNeedsExistingStorage(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "volume")
	cfg := config{DBPath: filepath.Join(missing, "adx.db"), AssetsDir: filepath.Join(missing, "assets")}

	err := checkStorage(cfg, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
	assert.Contains(t, err.Error(), "ADX_")
	_, statErr := os.Stat(missing)
	assert.True(t, os.IsNotExist(statErr), "production must not create the directory")

	require.NoError(t, checkStorage(cfg, true), "dev creates it")
	require.NoError(t, checkStorage(cfg, false), "and production is happy once it exists")

	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	err = checkStorage(config{DBPath: filepath.Join(file, "adx.db"), AssetsDir: cfg.AssetsDir}, false)
	assert.Error(t, err)
}

type backupGHL struct{}

func (backupGHL) AuthorizationURL(string) string { return "" }
func (backupGHL) Exchange(context.Context, string) (connections.Tokens, error) {
	return connections.Tokens{}, nil
}
func (backupGHL) Refresh(context.Context, string) (connections.Tokens, error) {
	return connections.Tokens{}, nil
}
func (backupGHL) Location(_ context.Context, _, id string) (connections.Location, error) {
	return connections.Location{ID: id, Name: "Reportage", TimeZone: "Asia/Dubai"}, nil
}
func (backupGHL) ReadAccess(context.Context, string, string) error { return nil }

// Catches: a restore that loses projects or assets, or sealed credentials
// that don't open with the key kept outside the backup. The second half
// proves the backup alone is useless without that key.
func TestBackupRestoresRecordsAndCredentialsWithTheKey(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	dbPath := filepath.Join(root, "data", "adx.db")
	assetsDir := filepath.Join(root, "data", "assets")
	require.NoError(t, os.MkdirAll(filepath.Join(assetsDir, "renders"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(assetsDir, "renders", "wadi.jpg"), []byte("jpeg bytes"), 0o644))

	db, err := store.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))
	_, err = db.Writer().CreateProject(ctx, store.CreateProjectParams{Name: "The Wadi", CreatedAt: time.Now().UTC()})
	require.NoError(t, err)
	key := bytes.Repeat([]byte{9}, 32)
	admin := core.Actor{UserID: 0, Role: core.RoleAdmin, Via: core.ViaWeb}
	live, err := connections.New(db, backupGHL{}, connections.Config{EncryptionKey: key})
	require.NoError(t, err)
	_, err = live.ConnectPrivate(ctx, admin, "loc-1", "pit-token-value")
	require.NoError(t, err)
	require.NoError(t, db.Close())

	dir, err := backup(ctx, dbPath, assetsDir, filepath.Join(root, "backups"), time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(dir, "20261017T030000Z"))
	asset, err := os.ReadFile(filepath.Join(dir, "assets", "renders", "wadi.jpg"))
	require.NoError(t, err)
	assert.Equal(t, "jpeg bytes", string(asset))
	raw, err := os.ReadFile(filepath.Join(dir, "adx.db"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "pit-token-value", "the backup holds sealed bytes, never the token")

	// Restore: open the copy in a new place, as the runbook does.
	restoredPath := filepath.Join(root, "restored", "adx.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(restoredPath), 0o755))
	require.NoError(t, os.Rename(filepath.Join(dir, "adx.db"), restoredPath))
	restored, err := store.Open(restoredPath)
	require.NoError(t, err)
	t.Cleanup(func() { restored.Close() })
	require.NoError(t, restored.Migrate(ctx))
	projects, err := restored.Reader().ListProjects(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 1)

	withKey, err := connections.New(restored, backupGHL{}, connections.Config{EncryptionKey: key})
	require.NoError(t, err)
	_, err = withKey.Check(ctx, admin, "loc-1")
	require.NoError(t, err, "the separately supplied key opens the restored credential")

	wrongKey, err := connections.New(restored, backupGHL{}, connections.Config{EncryptionKey: bytes.Repeat([]byte{1}, 32)})
	require.NoError(t, err)
	_, err = wrongKey.Check(ctx, admin, "loc-1")
	require.Error(t, err, "without the key the credential stays sealed")
}
