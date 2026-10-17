package creative_test

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assets"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/creative"
	"github.com/WokeWorks/adx/server/providers/filestore"
	"github.com/WokeWorks/adx/server/providers/svgrender"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db      *store.DB
	clock   *coretest.Clock
	svc     *creative.Service
	project int64
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))}
	coretest.SeedUsers(t, e.db)
	st, err := filestore.New(t.TempDir())
	require.NoError(t, err)
	e.svc = creative.New(e.db, st, svgrender.New(), creative.Config{Clock: e.clock})
	p, err := e.db.Writer().CreateProject(t.Context(), store.CreateProjectParams{Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.project = p.ID
	return e
}

func (e *env) upload(t *testing.T, kind, name, ct, body string) creative.Asset {
	t.Helper()
	a, err := e.svc.UploadAsset(t.Context(), coretest.Marketer, e.project, kind, name, ct, strings.NewReader(body))
	require.NoError(t, err)
	return a
}

// Catches: an approval surviving an edit, an approval covering a different
// output than it was given for, a draft being skipped past review, and the
// board step (in_review) being mistaken for delivery.
func TestEditingApprovedArtworkInvalidatesTheApproval(t *testing.T) {
	e := setup(t)
	render := e.upload(t, "render", "wadi.jpg", "image/jpeg", "jpeg bytes")
	logo := e.upload(t, "logo", "logo.png", "image/png", "png bytes")
	kit, err := e.svc.PutBrandKit(t.Context(), coretest.Marketer, e.project, assets.BrandKit{Logos: map[string]*assets.Ref{"onDark": &logo.Ref}, Colors: map[string]string{"inkLight": "#FFFFFF", "accent": "#C9A227"}})
	require.NoError(t, err)
	assert.Equal(t, int64(1), kit.Version)

	c, v1, err := e.svc.Create(t.Context(), coretest.Marketer, creative.CreativeInput{ProjectID: e.project, Name: "Wadi launch", Format: assets.Format9x16, Language: assets.English,
		Copy: assets.Copy{Eyebrow: "The Wadi", Title: "Villas from AED 1.2M", Note: "Up to 15% off when you pay cash. T&C apply.", CTA: "Register"}, RenderAssetID: render.ID})
	require.NoError(t, err)
	assert.Equal(t, assets.Draft, v1.ReviewState)
	assert.Equal(t, int64(1), v1.Version)
	assert.Nil(t, v1.Output)
	assert.False(t, assets.HasErrors(v1.Checks))

	// Can't submit or approve a draft.
	_, err = e.svc.Submit(t.Context(), coretest.Marketer, c.ID)
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
	_, err = e.svc.ReviewVersion(t.Context(), coretest.Admin, c.ID, "approved", "")
	require.ErrorAs(t, err, &ce)

	rendered, err := e.svc.Render(t.Context(), coretest.Marketer, c.ID)
	require.NoError(t, err)
	assert.Equal(t, assets.RenderedSt, rendered.ReviewState)
	require.NotNil(t, rendered.Output)
	assert.Equal(t, "template", *rendered.Renderer, "template layout, never labelled as generation")
	firstOutput := *rendered.Output

	inReview, err := e.svc.Submit(t.Context(), coretest.Marketer, c.ID)
	require.NoError(t, err)
	assert.Equal(t, assets.InReview, inReview.ReviewState, "on the board; nothing was delivered")

	_, err = e.svc.ReviewVersion(t.Context(), coretest.Marketer, c.ID, "approved", "")
	assert.ErrorIs(t, err, core.ErrForbidden, "only an admin approves")
	approved, err := e.svc.ReviewVersion(t.Context(), coretest.Admin, c.ID, "approved", "Looks good")
	require.NoError(t, err)
	assert.Equal(t, assets.Approved, approved.ReviewState)
	require.NotNil(t, approved.Acceptance)
	assert.True(t, approved.Acceptance.Covers(firstOutput))
	assert.False(t, approved.Acceptance.GrantsDelivery())
	assert.Equal(t, int64(1), approved.Acceptance.AcceptedBy)
	require.Len(t, approved.Reviews, 1)
	assert.Equal(t, firstOutput.SHA256, approved.Reviews[0].OutputSHA256)

	// Edit the approved artwork: a new draft version, no approval.
	doc := approved.Document
	doc.Layers[1].Text = "Villas from AED 1.1M"
	v2, err := e.svc.SaveDocument(t.Context(), coretest.Marketer, c.ID, doc)
	require.NoError(t, err)
	assert.Equal(t, int64(2), v2.Version)
	assert.Equal(t, assets.Draft, v2.ReviewState)
	assert.Nil(t, v2.Acceptance)
	assert.Nil(t, v2.Output)
	_, versions, err := e.svc.Get(t.Context(), coretest.Viewer, c.ID)
	require.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, assets.Approved, versions[0].ReviewState, "version 1 stays approved as it was")
	assert.True(t, versions[0].Acceptance.Covers(firstOutput))

	v2r, err := e.svc.Render(t.Context(), coretest.Marketer, c.ID)
	require.NoError(t, err)
	assert.NotEqual(t, firstOutput.SHA256, v2r.Output.SHA256, "changed copy, changed output")
	assert.False(t, versions[0].Acceptance.Covers(*v2r.Output), "the old approval does not cover the new output")

	// A draft edit stays in place; a revise decision sends it back.
	doc = v2r.Document
	doc.Layers[1].Text = "Villas from AED 1.15M"
	v3, err := e.svc.SaveDocument(t.Context(), coretest.Marketer, c.ID, doc)
	require.NoError(t, err)
	assert.Equal(t, int64(3), v3.Version, "rendered is not draft: editing makes version 3")
	v3b, err := e.svc.SaveDocument(t.Context(), coretest.Marketer, c.ID, doc)
	require.NoError(t, err)
	assert.Equal(t, int64(3), v3b.Version, "a draft is edited in place")
	_, err = e.svc.Render(t.Context(), coretest.Marketer, c.ID)
	require.NoError(t, err)
	_, err = e.svc.Submit(t.Context(), coretest.Marketer, c.ID)
	require.NoError(t, err)
	revised, err := e.svc.ReviewVersion(t.Context(), coretest.Admin, c.ID, "revise", "Smaller title")
	require.NoError(t, err)
	assert.Equal(t, assets.Revise, revised.ReviewState)
	assert.Nil(t, revised.Acceptance)
}

// Catches: an export serving bytes other than the approved ones, a document
// with house-rule errors being rendered, and files from another workspace
// being accepted into a layout.
func TestRenderBlocksRuleErrorsAndExportVerifiesBytes(t *testing.T) {
	e := setup(t)
	c, v1, err := e.svc.Create(t.Context(), coretest.Marketer, creative.CreativeInput{ProjectID: e.project, Name: "Oceana", Format: assets.Format1x1, Language: assets.English,
		Copy: assets.Copy{Title: "Up to 40% off", CTA: "Buy"}})
	require.NoError(t, err)
	assert.True(t, assets.HasErrors(v1.Checks))
	_, err = e.svc.Render(t.Context(), coretest.Marketer, c.ID)
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindInvalid, ce.Kind)
	assert.Contains(t, ce.Message, "15% limit")

	doc := v1.Document
	doc.Layers[0].Text = "Up to 15% off and more. T&C apply."
	_, err = e.svc.SaveDocument(t.Context(), coretest.Marketer, c.ID, doc)
	require.NoError(t, err)
	rendered, err := e.svc.Render(t.Context(), coretest.Marketer, c.ID)
	require.NoError(t, err)

	rc, v, err := e.svc.Export(t.Context(), coretest.Viewer, c.ID, 1)
	require.NoError(t, err)
	svg, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, rendered.Output.SHA256, v.Output.SHA256)
	assert.Contains(t, string(svg), "15% off and more")
	_, _, err = e.svc.Export(t.Context(), coretest.Viewer, c.ID, 9)
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindNotFound, ce.Kind)

	// A layer pointing at an unknown file is refused.
	doc = rendered.Document
	doc.Layers = append(doc.Layers, assets.Layer{ID: "x", Kind: "image", X: 10, Y: 500, Width: 100, Height: 100, Src: &assets.Ref{Key: "sha256-" + strings.Repeat("a", 64), SHA256: strings.Repeat("a", 64)}})
	_, err = e.svc.SaveDocument(t.Context(), coretest.Marketer, c.ID, doc)
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindInvalid, ce.Kind)

	// Uploads: type and role checks; unknown keys are not served.
	_, err = e.svc.UploadAsset(t.Context(), coretest.Viewer, e.project, "render", "x.jpg", "image/jpeg", strings.NewReader("x"))
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.svc.UploadAsset(t.Context(), coretest.Marketer, e.project, "render", "x.exe", "application/octet-stream", strings.NewReader("x"))
	require.ErrorAs(t, err, &ce)
	_, _, err = e.svc.Open(t.Context(), coretest.Viewer, "sha256-"+strings.Repeat("b", 64))
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindNotFound, ce.Kind)
	rc, ref, err := e.svc.Open(t.Context(), coretest.Viewer, rendered.Output.Key)
	require.NoError(t, err)
	rc.Close()
	assert.Equal(t, "image/svg+xml", ref.ContentType)

	// Unconfigured storage says so.
	bare := creative.New(e.db, nil, nil, creative.Config{Clock: e.clock})
	_, err = bare.UploadAsset(t.Context(), coretest.Marketer, 0, "render", "x.jpg", "image/jpeg", strings.NewReader("x"))
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
}

func TestBrandKitValidation(t *testing.T) {
	e := setup(t)
	_, err := e.svc.PutBrandKit(t.Context(), coretest.Marketer, e.project, assets.BrandKit{Colors: map[string]string{"accent": "gold"}})
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	_, err = e.svc.PutBrandKit(t.Context(), coretest.Marketer, e.project, assets.BrandKit{Logos: map[string]*assets.Ref{"onDark": {Key: "sha256-" + strings.Repeat("c", 64)}}})
	require.ErrorAs(t, err, &ce)
	_, err = e.svc.PutBrandKit(t.Context(), coretest.Marketer, 999, assets.BrandKit{})
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindNotFound, ce.Kind)
	kit, found, err := e.svc.GetBrandKit(t.Context(), coretest.Viewer, e.project)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, "Almarai", kit.Kit.Fonts["arTitle"].Family, "defaults until a kit is saved")
	_, err = e.svc.PutBrandKit(t.Context(), coretest.Marketer, e.project, assets.BrandKit{Colors: map[string]string{"accent": "#C9A227"}})
	require.NoError(t, err)
	v2, err := e.svc.PutBrandKit(t.Context(), coretest.Marketer, e.project, assets.BrandKit{Colors: map[string]string{"accent": "#FFFFFF"}})
	require.NoError(t, err)
	assert.Equal(t, int64(2), v2.Version)
}
