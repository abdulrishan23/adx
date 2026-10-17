package creative_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assets"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/creative"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/providers/filestore"
	"github.com/WokeWorks/adx/server/providers/svgrender"
)

// flakyRenderer writes the file like the real renderer and then fails the
// first N times, the way a crash between the file write and the row update
// would look to the next attempt.
type flakyRenderer struct {
	inner    assets.Renderer
	failures int
	calls    int
}

func (f *flakyRenderer) Name() string { return f.inner.Name() }
func (f *flakyRenderer) Render(ctx context.Context, st assets.Store, doc assets.Document) (assets.Rendered, error) {
	f.calls++
	out, err := f.inner.Render(ctx, st, doc)
	if err != nil {
		return out, err
	}
	if f.failures > 0 {
		f.failures--
		return assets.Rendered{}, errors.New("disk full at /var/lib/adx: secret path")
	}
	return out, nil
}

type prodEnv struct {
	*env
	runs     *runs.Runs
	brands   *brands.Brands
	renderer *flakyRenderer
	brand    int64
}

func setupProd(t *testing.T) *prodEnv {
	t.Helper()
	e := setup(t)
	p := &prodEnv{env: e, renderer: &flakyRenderer{inner: svgrender.New()}}
	p.runs = runs.New(e.db, nil, runs.Config{Clock: e.clock})
	p.brands = brands.New(e.db, brands.Config{Clock: e.clock})
	st, err := filestore.New(t.TempDir())
	require.NoError(t, err)
	p.svc = creative.New(e.db, st, p.renderer, creative.Config{Clock: e.clock, Brands: p.brands})
	b, err := p.brands.Create(t.Context(), coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	p.brand = b.ID
	require.NoError(t, p.brands.AssignProject(t.Context(), coretest.Admin, e.project, b.ID))
	return p
}

func (p *prodEnv) work(t *testing.T, run runs.Run) runs.Run {
	t.Helper()
	h, err := p.runs.Begin(t.Context(), run.RowID(), "test")
	require.NoError(t, err)
	completion, execErr := p.svc.Execute(t.Context(), h)
	if execErr != nil {
		require.NoError(t, h.Finish(t.Context(), runs.Completion{State: runs.StateFailed, Completeness: runs.CompletenessPartial, Error: execErr.Error()}))
	} else {
		require.NoError(t, h.Finish(t.Context(), completion))
	}
	got, err := p.runs.Get(t.Context(), coretest.Viewer, run.ID)
	require.NoError(t, err)
	return got
}

// Catches: a render failure leaving a half-updated version, the failure's
// raw text (with a path) reaching the row, a retry producing a second file,
// and two clicks rendering twice.
func TestRenderJobFailureIsRecoverable(t *testing.T) {
	p := setupProd(t)
	ctx := t.Context()
	c, _, err := p.svc.Create(ctx, coretest.Marketer, creative.CreativeInput{ProjectID: p.project, Name: "Wadi", Format: assets.Format1x1, Language: assets.English, Copy: assets.Copy{Title: "Villas", CTA: "Register"}})
	require.NoError(t, err)
	p.renderer.failures = 1
	run, created, err := p.svc.StartRender(ctx, coretest.Marketer, p.runs, c.ID)
	require.NoError(t, err)
	assert.True(t, created)
	_, again, err := p.svc.StartRender(ctx, coretest.Marketer, p.runs, c.ID)
	require.NoError(t, err)
	assert.False(t, again, "the same document renders once")

	failed := p.work(t, run)
	assert.Equal(t, runs.StateFailed, failed.State)
	assert.NotContains(t, *failed.Error, "/var/lib", "the path stays out of the safe message")
	_, versions, err := p.svc.Get(ctx, coretest.Viewer, c.ID)
	require.NoError(t, err)
	v := versions[0]
	assert.Equal(t, assets.Draft, v.ReviewState, "nothing half-rendered")
	assert.Nil(t, v.Output)
	assert.NotEmpty(t, v.RenderError)
	assert.NotContains(t, v.RenderError, "/var/lib")

	// The same call requeues the failed run; the retry lands on the same key.
	retry, created, err := p.svc.StartRender(ctx, coretest.Marketer, p.runs, c.ID)
	require.NoError(t, err)
	assert.True(t, created)
	done := p.work(t, retry)
	assert.Equal(t, runs.StateSucceeded, done.State)
	_, versions, err = p.svc.Get(ctx, coretest.Viewer, c.ID)
	require.NoError(t, err)
	v = versions[0]
	assert.Equal(t, assets.RenderedSt, v.ReviewState)
	require.NotNil(t, v.Output)
	assert.Empty(t, v.RenderError)
	assert.Equal(t, 2, p.renderer.calls)
	assert.Equal(t, "template", *v.Renderer)
	// The failed attempt's bytes are the same bytes: one file, verified.
	rc, _, err := p.svc.Export(ctx, coretest.Viewer, c.ID, 1)
	require.NoError(t, err)
	rc.Close()

	// An upload that never reached the database converges on re-upload.
	a1 := p.upload(t, "render", "wadi.jpg", "image/jpeg", "same bytes")
	a2 := p.upload(t, "render", "wadi-again.jpg", "image/jpeg", "same bytes")
	assert.Equal(t, a1.Ref.Key, a2.Ref.Key)
	assert.Equal(t, "uploaded", a1.Provenance)
}

// Catches: an output being called AI-generated, the capability report
// claiming a generator that isn't there, and provenance missing from
// uploads.
func TestTemplateLayoutIsNotAdvertisedAsAIGeneration(t *testing.T) {
	p := setupProd(t)
	caps := p.svc.Capabilities()
	assert.Equal(t, "template", caps.Renderer)
	assert.False(t, caps.AIGeneration.Available)
	assert.Contains(t, caps.AIGeneration.Reason, "template layout")
	c, _, err := p.svc.Create(t.Context(), coretest.Marketer, creative.CreativeInput{ProjectID: p.project, Name: "Wadi", Format: assets.Format4x5, Language: assets.English, Copy: assets.Copy{Title: "Villas"}})
	require.NoError(t, err)
	v, err := p.svc.Render(t.Context(), coretest.Marketer, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "template", *v.Renderer)
	rc, _, err := p.svc.Export(t.Context(), coretest.Viewer, c.ID, 1)
	require.NoError(t, err)
	defer rc.Close()
	buf := new(strings.Builder)
	b := make([]byte, 4096)
	n, _ := rc.Read(b)
	buf.Write(b[:n])
	assert.Contains(t, buf.String(), `data-renderer="template"`)
}

// Catches: a creative with a discount nobody approved being ready, an
// approved fact making it ready, an offer change after the build passing,
// an unapproved version passing, and a figure the checker can't read being
// silently fine.
func TestUnapprovedClaimsFailReadiness(t *testing.T) {
	p := setupProd(t)
	ctx := t.Context()
	offer, err := p.brands.CreateOffer(ctx, coretest.Marketer, p.brand, brands.OfferInput{ProjectID: p.project, Name: "Launch", Document: brands.OfferDocument{Headline: "Up to 15% off and more", DiscountPct: f64(15)}, EffectiveFrom: "2026-10-01"})
	require.NoError(t, err)
	c, _, err := p.svc.Create(ctx, coretest.Marketer, creative.CreativeInput{ProjectID: p.project, Name: "Wadi", Format: assets.Format1x1, Language: assets.English,
		Copy: assets.Copy{Eyebrow: "Up to 15% off and more", Title: "Villas from AED 1.2M", Note: "on selected units", CTA: "Register"}, OfferID: offer.ID})
	require.NoError(t, err)
	_, err = p.svc.Render(ctx, coretest.Marketer, c.ID)
	require.NoError(t, err)
	_, err = p.svc.Submit(ctx, coretest.Marketer, c.ID)
	require.NoError(t, err)
	_, err = p.svc.ReviewVersion(ctx, coretest.Admin, c.ID, "approved", "")
	require.NoError(t, err)

	r, err := p.svc.Readiness(ctx, coretest.Viewer, c.ID)
	require.NoError(t, err)
	assert.False(t, r.Ready)
	codes := map[string]int{}
	for _, b := range r.Blockers {
		codes[b.Code]++
	}
	assert.Equal(t, 2, codes["unapproved_claim"], "15%% (offer not approved) and AED 1.2M (no fact): %+v", r.Blockers)
	assert.Equal(t, 1, codes["offer_not_approved"])
	assert.Equal(t, 0, codes["content_not_approved"])
	claimStatus := map[string]string{}
	for _, cl := range r.Claims {
		claimStatus[cl.Text] = cl.Status
	}
	assert.Equal(t, "unapproved", claimStatus["15%"])
	assert.Equal(t, "unapproved", claimStatus["AED 1.2M"])

	// Approve the offer and the price fact: the same copy becomes ready.
	_, err = p.brands.SetOfferState(ctx, coretest.Admin, offer.ID, brands.OfferApproved)
	require.NoError(t, err)
	fact, err := p.brands.AddFact(ctx, coretest.Marketer, p.brand, brands.FactInput{ProjectID: p.project, Key: "price.from", Kind: brands.KindMoney, Value: "120000000", Currency: "AED", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceDocument, SourceRef: "price list"})
	require.NoError(t, err)
	_, err = p.brands.ApproveFact(ctx, coretest.Admin, fact.Fact.ID)
	require.NoError(t, err)
	r, err = p.svc.Readiness(ctx, coretest.Viewer, c.ID)
	require.NoError(t, err)
	assert.True(t, r.Ready, "%+v", r.Blockers)
	for _, cl := range r.Claims {
		assert.Equal(t, "approved", cl.Status, cl.Text)
	}
	assert.Equal(t, int64(1), r.Version)

	// The offer changes: the approved creative is no longer ready.
	_, err = p.brands.ReviseOffer(ctx, coretest.Marketer, offer.ID, brands.OfferInput{Document: brands.OfferDocument{Headline: "12% off", DiscountPct: f64(12)}, EffectiveFrom: "2026-11-01"})
	require.NoError(t, err)
	r, err = p.svc.Readiness(ctx, coretest.Viewer, c.ID)
	require.NoError(t, err)
	assert.False(t, r.Ready)
	codes = map[string]int{}
	for _, b := range r.Blockers {
		codes[b.Code]++
	}
	assert.Equal(t, 1, codes["offer_changed"])

	// An edit makes an unapproved version: not ready, and the old approval
	// doesn't carry.
	_, versions, err := p.svc.Get(ctx, coretest.Viewer, c.ID)
	require.NoError(t, err)
	doc := versions[0].Document
	doc.Layers[0].Text = "Villas from AED 1.25M"
	v2, err := p.svc.SaveDocument(ctx, coretest.Marketer, c.ID, doc)
	require.NoError(t, err)
	assert.Equal(t, int64(2), v2.Version)
	r, err = p.svc.Readiness(ctx, coretest.Viewer, c.ID)
	require.NoError(t, err)
	assert.False(t, r.Ready)
	codes = map[string]int{}
	for _, b := range r.Blockers {
		codes[b.Code]++
	}
	assert.Equal(t, 1, codes["content_not_approved"])
	assert.GreaterOrEqual(t, codes["unapproved_claim"], 1, "AED 1.25M is not approved")

	// Arabic digits and the Arabic percent sign are read.
	ar, _, err := p.svc.Create(ctx, coretest.Marketer, creative.CreativeInput{ProjectID: p.project, Name: "Wadi AR", Format: assets.Format1x1, Language: assets.Arabic, Copy: assets.Copy{Title: "خصم ١٥٪", CTA: "سجل"}})
	require.NoError(t, err)
	r, err = p.svc.Readiness(ctx, coretest.Viewer, ar.ID)
	require.NoError(t, err)
	require.Len(t, r.Claims, 1)
	assert.Equal(t, "percent", r.Claims[0].Kind)
	assert.Equal(t, "unapproved", r.Claims[0].Status, "the creative has no offer, so the 15 is not an approved claim")
	warned := false
	for _, w := range r.Warnings {
		if w.Code == "arabic_review" {
			warned = true
		}
	}
	assert.True(t, warned)

	// Roles.
	_, err = p.svc.Readiness(ctx, core.Actor{Role: core.RoleViewer}, c.ID)
	assert.ErrorIs(t, err, core.ErrForbidden)
}

func f64(f float64) *float64 { return &f }
