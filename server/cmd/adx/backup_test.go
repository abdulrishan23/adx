package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/pipeline"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/providers/filestore"
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
	_, err = db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: time.Now().UTC()})
	require.NoError(t, err)
	key := bytes.Repeat([]byte{9}, 32)
	admin := core.Actor{UserID: 0, Role: core.RoleAdmin, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
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
	projects, err := restored.Reader().ListProjects(ctx, core.DefaultWorkspace)
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

// Catches: retention deleting a backup it should keep, touching a directory
// that isn't a backup, or pruning when retention is off.
func TestPruneKeepsRecentBackupsAndForeignDirectories(t *testing.T) {
	dest := t.TempDir()
	now := time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)
	for _, name := range []string{"20260901T023000Z", "20261001T023000Z", "20261017T023000Z", "notes"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dest, name), 0o700))
	}
	pruned, err := prune(dest, now, 0)
	require.NoError(t, err)
	assert.Empty(t, pruned, "retention off keeps everything")
	pruned, err = prune(dest, now, 30)
	require.NoError(t, err)
	assert.Equal(t, []string{"20260901T023000Z"}, pruned)
	for _, name := range []string{"20261001T023000Z", "20261017T023000Z", "notes"} {
		_, err := os.Stat(filepath.Join(dest, name))
		assert.NoError(t, err, "%s stays", name)
	}
}

// Catches: a restore that breaks the links between a person, their enquiry,
// the opportunity on it, its evidence and the asset on disk, or that loses
// an interrupted run instead of letting recovery requeue it.
func TestRestoreReproducesLinkedStateAndRecoversInterruptedRuns(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	dbPath := filepath.Join(root, "data", "adx.db")
	assetsDir := filepath.Join(root, "data", "assets")
	require.NoError(t, os.MkdirAll(assetsDir, 0o755))
	db, err := store.Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))
	coretest.SeedUsers(t, db)
	clock := coretest.NewClock(time.Date(2026, 10, 17, 1, 0, 0, 0, time.UTC))
	ev := evidence.New(db, clock)
	people := crm.New(db, ev, crm.Config{Clock: clock})
	pipe := pipeline.New(db, ev, pipeline.Config{Clock: clock})
	files, err := filestore.New(assetsDir)
	require.NoError(t, err)
	rs := runs.New(db, nil, runs.Config{Clock: clock})

	person, err := people.Create(ctx, coretest.Marketer, crm.PersonInput{DisplayName: "Ali", Phone: "+971501234567"})
	require.NoError(t, err)
	enq, err := db.Writer().CreateEnquiry(ctx, store.CreateEnquiryParams{WorkspaceID: 1, PersonID: person.Person.ID, SourceKind: "lead", SourceRef: "x", FirstContactAt: clock.Now(), LastActivityAt: clock.Now(), CreatedAt: clock.Now()})
	require.NoError(t, err)
	_, err = pipe.PutConfig(ctx, coretest.Admin, pipeline.SuggestedConfig())
	require.NoError(t, err)
	opp, _, err := pipe.Open(ctx, coretest.Marketer, enq.ID)
	require.NoError(t, err)
	obs, _, err := ev.Record(ctx, coretest.Marketer, evidence.Observation{Source: "manual", Kind: "note", SourceRef: "note:1", Scope: fmt.Sprintf("person:%d", person.Person.ID), Class: evidence.Observed, Payload: []byte(`{"text":"called"}`), Completeness: evidence.Complete})
	require.NoError(t, err)
	ref, err := files.Put(ctx, "image/jpeg", strings.NewReader("jpeg bytes"))
	require.NoError(t, err)
	run, _, err := rs.Start(ctx, coretest.Marketer, runs.StartInput{Kind: "test.refresh", OccurrenceKey: "k1"})
	require.NoError(t, err)
	_, err = rs.Begin(ctx, run.RowID(), "worker-a")
	require.NoError(t, err, "the run is leased, as it would be when the host dies mid-work")
	require.NoError(t, db.Close())

	dir, err := backup(ctx, dbPath, assetsDir, filepath.Join(root, "backups"), clock.Now())
	require.NoError(t, err)
	restoredPath := filepath.Join(root, "restored", "adx.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(restoredPath), 0o755))
	require.NoError(t, os.Rename(filepath.Join(dir, "adx.db"), restoredPath))
	restored, err := store.Open(restoredPath)
	require.NoError(t, err)
	t.Cleanup(func() { restored.Close() })
	require.NoError(t, restored.Migrate(ctx))
	restoredFiles, err := filestore.New(filepath.Join(dir, "assets"))
	require.NoError(t, err)

	rp, err := crm.New(restored, evidence.New(restored, clock), crm.Config{Clock: clock}).Get(ctx, coretest.Viewer, person.Person.ID)
	require.NoError(t, err)
	assert.Equal(t, "Ali", rp.Person.DisplayName)
	require.Len(t, rp.Enquiries, 1)
	assert.Equal(t, enq.ID, rp.Enquiries[0].ID)
	ro, err := pipeline.New(restored, nil, pipeline.Config{Clock: clock}).Get(ctx, coretest.Viewer, opp.ID)
	require.NoError(t, err)
	assert.Equal(t, enq.ID, ro.Opportunity.EnquiryID, "the opportunity still points at the same enquiry")
	var evidenceScope string
	require.NoError(t, restored.Read.QueryRowContext(ctx, "SELECT scope FROM evidence WHERE id = ?", obs.ID).Scan(&evidenceScope))
	assert.Equal(t, fmt.Sprintf("person:%d", person.Person.ID), evidenceScope)
	rc, err := restoredFiles.Open(ctx, ref)
	require.NoError(t, err)
	body, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, "jpeg bytes", string(body), "the asset the record refers to is in the backup")

	// Recovery: the lease the dead host held expires and the run is
	// interrupted, not lost and not silently completed.
	clock.Advance(time.Hour)
	recovered, err := runs.New(restored, nil, runs.Config{Clock: clock}).RecoverInterrupted(ctx)
	require.NoError(t, err)
	require.Len(t, recovered, 1)
	assert.Equal(t, run.ID, recovered[0].ID)
	assert.Equal(t, runs.StateInterrupted, recovered[0].State)
}

// Catches: the load harness recording a number without its conditions, or
// running against anything but a scratch database.
func TestLoadtestMeasuresAScratchWorkload(t *testing.T) {
	m, err := measureLoad(t.Context(), 300*time.Millisecond, 2)
	require.NoError(t, err)
	assert.Greater(t, m.Operations, int64(0))
	assert.Equal(t, int64(0), m.Errors, "every operation in the mix succeeds against a fresh database")
	assert.Len(t, m.Mix, 5)
	assert.NotEmpty(t, m.Host)
	assert.Contains(t, m.Notes, "scratch database")
}
