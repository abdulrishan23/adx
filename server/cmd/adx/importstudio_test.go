package main

import (
	"bytes"
	"database/sql"
	"image"
	"image/jpeg"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/studio"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/blobs"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: an import that loses the picture behind an ad or its review, one
// that lands ads on a different project than the old server's, and a second
// run doubling everything.
func TestImportStudioCarriesAdsReviewsAndFilesOnce(t *testing.T) {
	ctx := t.Context()
	oldDir := t.TempDir()
	oldDB, err := store.Open(filepath.Join(oldDir, "adx.db"))
	require.NoError(t, err)
	t.Cleanup(func() { oldDB.Close() })
	require.NoError(t, oldDB.Migrate(ctx))
	oldFiles, err := blobs.New(filepath.Join(oldDir, "files"))
	require.NoError(t, err)
	old := studio.New(oldDB, oldFiles)
	marketer := core.Actor{Role: core.RoleMarketer, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
	p, err := oldDB.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: time.Now().UTC()})
	require.NoError(t, err)
	settings, err := old.SetUp(ctx, marketer, studio.SetupInput{ProjectID: p.ID})
	require.NoError(t, err)
	pic := func(w, h int, shade uint8) string {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for i := range img.Pix {
			img.Pix[i] = shade
		}
		var buf bytes.Buffer
		require.NoError(t, jpeg.Encode(&buf, img, nil))
		ref, err := old.Upload(ctx, marketer, &buf)
		require.NoError(t, err)
		return ref.SHA256
	}
	_, err = old.AddUploads(ctx, marketer, p.ID, []studio.UploadedAd{{
		Concept: "C1", Slug: "CASH", Lang: studio.LangEN, Fmt: studio.FmtStory, File: pic(1080, 1920, 40), Preview: pic(270, 480, 41),
	}})
	require.NoError(t, err)
	require.NoError(t, old.Decide(ctx, core.Anonymous(core.ViaWeb), settings.ReviewKey, studio.BoardDecision{Ad: "C1_CASH_EN_9x16", Name: "Tamim", Status: "approved"}))

	newDir := t.TempDir()
	db := storetest.Open(t)
	files, err := blobs.New(filepath.Join(newDir, "files"))
	require.NoError(t, err)
	// The target already has the project, under another id than the old one.
	_, err = db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "Sila", CreatedAt: time.Now().UTC()})
	require.NoError(t, err)
	wadi, err := db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: time.Now().UTC()})
	require.NoError(t, err)
	oldConn, err := sql.Open("sqlite", "file:"+filepath.Join(oldDir, "adx.db")+"?_pragma=query_only(1)&_time_format=sqlite")
	require.NoError(t, err)
	t.Cleanup(func() { oldConn.Close() })

	run := func() importCounts {
		imp := &studioImport{ctx: ctx, old: oldConn, oldFiles: oldFiles, q: db.Writer(), files: files, fileIDs: map[int64]int64{}}
		n, err := imp.run()
		require.NoError(t, err)
		return n
	}
	n := run()
	assert.Equal(t, importCounts{projects: 1, files: 2, ads: 1, versions: 1}, n)

	got := studio.New(db, files)
	proj, err := got.Project(ctx, marketer, wadi.ID)
	require.NoError(t, err)
	require.Len(t, proj.Concepts, 1)
	require.Len(t, proj.Concepts[0].Ads, 1)
	ad := proj.Concepts[0].Ads[0]
	assert.Equal(t, "C1_CASH_EN_9x16", ad.Code)
	assert.Equal(t, studio.StatusApproved, ad.Status, "the review came with it")
	require.NotNil(t, ad.Current)
	f, _, err := got.OpenFile(ctx, core.Anonymous(core.ViaWeb), ad.Current.File.SHA256)
	require.NoError(t, err, "the picture's bytes are in the new store")
	f.Close()
	assert.Equal(t, "THEWADI", proj.Settings.Prefix)

	again := run()
	assert.Equal(t, importCounts{skipped: 1}, again, "a second run adds nothing")
	proj, err = got.Project(ctx, marketer, wadi.ID)
	require.NoError(t, err)
	assert.Len(t, proj.Concepts[0].Ads, 1)
}
