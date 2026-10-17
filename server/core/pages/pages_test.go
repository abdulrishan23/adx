package pages_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/pages"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	brands    *brands.Brands
	pages     *pages.Pages
	reportage int64
	wadi      int64
	other     int64
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	e.brands = brands.New(e.db, brands.Config{Clock: e.clock})
	e.pages = pages.New(e.db, e.brands, evidence.New(e.db, e.clock), pages.Config{Clock: e.clock, PublicBase: "https://adx.example", RatePerMinute: 3, DailyPerPage: 12, Salt: "test"})
	b, err := e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	e.reportage = b.ID
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.wadi = p.ID
	require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, e.wadi, e.reportage))
	o, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "Oceana", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.other = o.ID
	require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, e.other, e.reportage))
	return e
}

func goodDoc() pages.PageDocument {
	return pages.PageDocument{
		Title: "The Wadi by Reportage", Headline: "Villas in Dubailand", Subheadline: "Register for the launch",
		Sections: []pages.Section{{Kind: "text", Text: "Three and four bedroom villas."}, {Kind: "bullets", Items: []string{"Gated community", "Handover 2027"}}, {Kind: "form"}},
		Form: &pages.FormConfig{Fields: []pages.FormField{{Name: "name", Label: "Name", Kind: "text", Required: true}, {Name: "phone", Label: "Phone", Kind: "phone", Required: true}, {Name: "email", Label: "Email", Kind: "email"}, {Name: "beds", Label: "Bedrooms", Kind: "select", Options: []string{"3", "4"}}},
			ConsentText: "By registering you agree to be contacted.", SuccessMessage: "Thanks, we will call you."},
	}
}

func (e *env) submit(t *testing.T, slug string, fields map[string]string, project int64, test string, ip string) (pages.Submission, error) {
	t.Helper()
	return e.pages.Submit(t.Context(), pages.SubmissionInput{Slug: slug, Fields: fields, ProjectID: project, TestToken: test, RemoteIP: ip, UserAgent: "test", Attribution: map[string]string{"utm_source": "google", "gclid": "abc", "secret": "no"}})
}

// Catches: a submission for another project being stored as this one's, an
// unpublished page taking submissions, required and typed fields not
// checked, the honeypot being recorded as received, and the test token
// marking anything but a test.
func TestSubmissionsAreScopedValidatedAndRated(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	d, err := e.pages.Create(ctx, coretest.Marketer, pages.PageInput{BrandID: e.reportage, ProjectID: e.wadi, Slug: "wadi-launch", Name: "Wadi launch", Language: "en", Document: goodDoc()})
	require.NoError(t, err)
	assert.Equal(t, "https://adx.example/p/wadi-launch", d.PublicURL)
	assert.NotEmpty(t, d.TestToken)
	viewerView, err := e.pages.Get(ctx, coretest.Viewer, d.Page.ID)
	require.NoError(t, err)
	assert.Empty(t, viewerView.TestToken, "viewers don't get the test token")

	_, err = e.submit(t, "wadi-launch", map[string]string{"name": "A", "phone": "+971501234567"}, e.wadi, "", "1.1.1.1")
	var se *pages.SubmitError
	require.True(t, errors.As(err, &se))
	assert.Equal(t, "not_published", se.Reason)

	_, err = e.pages.Publish(ctx, coretest.Marketer, d.Page.ID, 1)
	require.NoError(t, err)
	_, err = e.pages.ServePublished(ctx, "wadi-launch")
	require.NoError(t, err)

	// Wrong project: rejected and recorded as rejected.
	s, err := e.submit(t, "wadi-launch", map[string]string{"name": "A", "phone": "+971501234567"}, e.other, "", "1.1.1.1")
	require.True(t, errors.As(err, &se))
	assert.Equal(t, "wrong_project", se.Reason)
	assert.Equal(t, "rejected", s.State)
	assert.Equal(t, e.wadi, *s.ProjectID, "recorded against the page's project, never the posted one")

	// Validation.
	_, err = e.submit(t, "wadi-launch", map[string]string{"name": "A"}, e.wadi, "", "1.1.1.2")
	require.True(t, errors.As(err, &se))
	assert.Equal(t, "missing:phone", se.Reason)
	_, err = e.submit(t, "wadi-launch", map[string]string{"name": "A", "phone": "call me"}, e.wadi, "", "1.1.1.3")
	require.True(t, errors.As(err, &se))
	assert.Equal(t, "invalid:phone", se.Reason)
	_, err = e.submit(t, "wadi-launch", map[string]string{"name": "A", "phone": "+971501234567", "email": "not-an-email"}, e.wadi, "", "1.1.1.4")
	require.True(t, errors.As(err, &se))
	assert.Equal(t, "invalid:email", se.Reason)
	_, err = e.submit(t, "wadi-launch", map[string]string{"name": "A", "phone": "+971501234567", "beds": "7"}, e.wadi, "", "1.1.1.5")
	require.True(t, errors.As(err, &se))
	assert.Equal(t, "invalid:beds", se.Reason)

	// A good one: fields cleaned, attribution filtered, evidence recorded.
	s, err = e.submit(t, "wadi-launch", map[string]string{"name": " Amal ", "phone": "+971501234567", "beds": "4", "extra": "ignored"}, e.wadi, "", "1.1.1.6")
	require.NoError(t, err)
	assert.Equal(t, "received", s.State)
	assert.False(t, s.Test)
	assert.Equal(t, map[string]string{"name": "Amal", "phone": "+971501234567", "beds": "4"}, s.Fields)
	assert.Equal(t, map[string]string{"utm_source": "google", "gclid": "abc"}, s.Attribution)
	require.NotNil(t, s.EvidenceID)
	assert.Equal(t, e.wadi, *s.ProjectID)

	// Honeypot: recorded as rejected, visitor sees a thank you.
	_, err = e.pages.Submit(ctx, pages.SubmissionInput{Slug: "wadi-launch", Fields: map[string]string{"name": "Bot", "phone": "+971501234567"}, ProjectID: e.wadi, Honeypot: "http://spam", RemoteIP: "9.9.9.9"})
	require.True(t, errors.As(err, &se))
	assert.Equal(t, "honeypot", se.Reason)
	assert.Equal(t, 200, se.Status)

	// Rate: three per minute per address, then refused; a minute later fine.
	for i := 0; i < 3; i++ {
		_, err = e.submit(t, "wadi-launch", map[string]string{"name": "A", "phone": "+971501234567"}, e.wadi, "", "5.5.5.5")
		require.NoError(t, err)
	}
	_, err = e.submit(t, "wadi-launch", map[string]string{"name": "A", "phone": "+971501234567"}, e.wadi, "", "5.5.5.5")
	require.True(t, errors.As(err, &se))
	assert.Equal(t, "rate_limited", se.Reason)
	e.clock.Advance(61 * time.Second)
	_, err = e.submit(t, "wadi-launch", map[string]string{"name": "A", "phone": "+971501234567"}, e.wadi, "", "5.5.5.5")
	require.NoError(t, err)

	// Daily cap per page (12 in this test, counting rejected rows too).
	_, err = e.submit(t, "wadi-launch", map[string]string{"name": "A", "phone": "+971501234567"}, e.wadi, "", "7.7.7.7")
	require.True(t, errors.As(err, &se))
	assert.Equal(t, "daily_cap", se.Reason)

	// A test token bypasses the limits and marks the row.
	ts, err := e.submit(t, "wadi-launch", map[string]string{"name": "QA", "phone": "+971501234567"}, e.wadi, d.TestToken, "7.7.7.7")
	require.NoError(t, err)
	assert.True(t, ts.Test)
	_, err = e.submit(t, "wadi-launch", map[string]string{"name": "QA", "phone": "+971501234567"}, e.wadi, "wrong-token", "8.8.8.8")
	require.True(t, errors.As(err, &se), "a wrong token is an ordinary visitor and hits the cap")
	assert.Equal(t, "daily_cap", se.Reason)

	subs, err := e.pages.Submissions(ctx, coretest.Viewer, d.Page.ID, 50)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(subs), 10)
	_, err = e.pages.Submissions(ctx, core.Actor{Role: core.RoleViewer}, d.Page.ID, 50)
	assert.ErrorIs(t, err, core.ErrForbidden)
}

// Catches: a broken form passing readiness, an untested form passing, a
// rejected test counting as a test, an unapproved claim passing, and an
// offer change after publishing passing.
func TestBrokenFormCannotPassReadiness(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	offer, err := e.brands.CreateOffer(ctx, coretest.Marketer, e.reportage, brands.OfferInput{ProjectID: e.wadi, Name: "Launch", Document: brands.OfferDocument{Headline: "Up to 15% off and more", Terms: "Selected units", DiscountPct: f64(15)}, EffectiveFrom: "2026-10-01"})
	require.NoError(t, err)
	broken := goodDoc()
	broken.Form.Fields = []pages.FormField{{Name: "name", Label: "Name", Kind: "text", Required: true}, {Name: "email", Label: "Email", Kind: "email"}}
	broken.Headline = "Villas from AED 1.2M, 15% off"
	broken.OfferID = &offer.ID
	broken.Sections = append(broken.Sections, pages.Section{Kind: "offer"})
	d, err := e.pages.Create(ctx, coretest.Marketer, pages.PageInput{BrandID: e.reportage, ProjectID: e.wadi, Slug: "wadi", Name: "Wadi", Language: "en", Document: broken})
	require.NoError(t, err)
	r, err := e.pages.Readiness(ctx, coretest.Viewer, d.Page.ID)
	require.NoError(t, err)
	assert.False(t, r.Ready)
	assert.Equal(t, []string{"not_published"}, codes(r.Blockers))

	_, err = e.pages.Publish(ctx, coretest.Marketer, d.Page.ID, 1)
	require.NoError(t, err)
	html, err := e.pages.ServePublished(ctx, "wadi")
	require.NoError(t, err)
	assert.Contains(t, string(html), "Offer details pending approval", "an unapproved offer's terms are not shown")
	r, err = e.pages.Readiness(ctx, coretest.Viewer, d.Page.ID)
	require.NoError(t, err)
	assert.False(t, r.Ready)
	got := codes(r.Blockers)
	assert.Contains(t, got, "form_contact_missing")
	assert.Contains(t, got, "untested_form")
	assert.Contains(t, got, "offer_not_approved")
	assert.Equal(t, 2, count(got, "unapproved_claim"), "AED 1.2M and 15%%: %v", got)

	// Fix the form and the facts; publish v2; test it; approve the offer.
	fixed := broken
	fixed.Form = &pages.FormConfig{Fields: []pages.FormField{{Name: "name", Label: "Name", Kind: "text", Required: true}, {Name: "phone", Label: "Phone", Kind: "phone", Required: true}}, ConsentText: "ok"}
	v2, err := e.pages.SaveVersion(ctx, coretest.Marketer, d.Page.ID, pages.PageVersionInput{Document: fixed, Note: "phone required"})
	require.NoError(t, err)
	_, err = e.pages.Publish(ctx, coretest.Marketer, d.Page.ID, v2.Version)
	require.NoError(t, err)
	_, err = e.brands.SetOfferState(ctx, coretest.Admin, offer.ID, brands.OfferApproved)
	require.NoError(t, err)
	fact, err := e.brands.AddFact(ctx, coretest.Marketer, e.reportage, brands.FactInput{ProjectID: e.wadi, Key: "price.from", Kind: brands.KindMoney, Value: "120000000", Currency: "AED", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceDocument, SourceRef: "price list"})
	require.NoError(t, err)
	_, err = e.brands.ApproveFact(ctx, coretest.Admin, fact.Fact.ID)
	require.NoError(t, err)
	html, err = e.pages.ServePublished(ctx, "wadi")
	require.NoError(t, err)
	assert.Contains(t, string(html), "Up to 15% off and more")
	assert.Contains(t, string(html), "Selected units")

	// A failed test is not a passed test.
	_, err = e.submit(t, "wadi", map[string]string{"name": "QA"}, e.wadi, d.TestToken, "1.1.1.1")
	require.Error(t, err)
	r, err = e.pages.Readiness(ctx, coretest.Viewer, d.Page.ID)
	require.NoError(t, err)
	assert.Contains(t, codes(r.Blockers), "test_failed")
	ts, err := e.submit(t, "wadi", map[string]string{"name": "QA", "phone": "+971501234567"}, e.wadi, d.TestToken, "1.1.1.1")
	require.NoError(t, err)
	assert.True(t, ts.Test)
	r, err = e.pages.Readiness(ctx, coretest.Viewer, d.Page.ID)
	require.NoError(t, err)
	assert.True(t, r.Ready, "%v", r.Blockers)
	assert.Equal(t, int64(2), *r.PublishedVersion)
	require.NotNil(t, r.LastTest)
	assert.True(t, r.LastTest.Test)

	// The offer changes under the page: not ready.
	_, err = e.brands.ReviseOffer(ctx, coretest.Marketer, offer.ID, brands.OfferInput{Document: brands.OfferDocument{Headline: "12% off", DiscountPct: f64(12)}, EffectiveFrom: "2026-11-01"})
	require.NoError(t, err)
	r, err = e.pages.Readiness(ctx, coretest.Viewer, d.Page.ID)
	require.NoError(t, err)
	assert.Contains(t, codes(r.Blockers), "offer_changed")

	// A new draft after publishing is a warning, and its test doesn't count
	// for the live version.
	v3, err := e.pages.SaveVersion(ctx, coretest.Marketer, d.Page.ID, pages.PageVersionInput{Document: fixed})
	require.NoError(t, err)
	assert.Equal(t, int64(3), v3.Version)
	r, err = e.pages.Readiness(ctx, coretest.Viewer, d.Page.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), *r.PublishedVersion)
	assert.Contains(t, codes(r.Warnings), "newer_draft")
}

// Catches: a rollback that doesn't say which version came back, a rollback
// with nothing to go to, and the live page not following the rollback.
func TestRollbackNamesTheExactOldVersion(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	d, err := e.pages.Create(ctx, coretest.Marketer, pages.PageInput{BrandID: e.reportage, ProjectID: e.wadi, Slug: "wadi", Name: "Wadi", Language: "en", Document: goodDoc()})
	require.NoError(t, err)
	_, err = e.pages.Rollback(ctx, coretest.Marketer, d.Page.ID)
	require.Error(t, err, "not published")
	dep1, err := e.pages.Publish(ctx, coretest.Marketer, d.Page.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, "published", dep1.State)
	assert.Equal(t, "adx", dep1.Adapter)
	assert.Len(t, dep1.OutputSHA256, 64)
	_, err = e.pages.Rollback(ctx, coretest.Marketer, d.Page.ID)
	require.Error(t, err, "only one version ever published")

	doc2 := goodDoc()
	doc2.Headline = "Villas in Dubailand, new phase"
	v2, err := e.pages.SaveVersion(ctx, coretest.Marketer, d.Page.ID, pages.PageVersionInput{Document: doc2})
	require.NoError(t, err)
	dep2, err := e.pages.Publish(ctx, coretest.Marketer, d.Page.ID, v2.Version)
	require.NoError(t, err)
	html, err := e.pages.ServePublished(ctx, "wadi")
	require.NoError(t, err)
	assert.Contains(t, string(html), "new phase")

	rb, err := e.pages.Rollback(ctx, coretest.Marketer, d.Page.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), rb.Version, "back to version 1")
	assert.Equal(t, dep2.ID, *rb.RolledBackFromID)
	assert.Equal(t, int64(2), *rb.RolledBackFromVersion)
	assert.Equal(t, dep1.OutputSHA256, rb.OutputSHA256, "the same bytes as the first deployment")
	html, err = e.pages.ServePublished(ctx, "wadi")
	require.NoError(t, err)
	assert.NotContains(t, string(html), "new phase")
	detail, err := e.pages.Get(ctx, coretest.Viewer, d.Page.ID)
	require.NoError(t, err)
	require.NotNil(t, detail.Current)
	assert.Equal(t, rb.ID, detail.Current.ID)
	states := map[int64]string{}
	for _, dp := range detail.Deployments {
		states[dp.ID] = dp.State
	}
	assert.Equal(t, "superseded", states[dep1.ID])
	assert.Equal(t, "superseded", states[dep2.ID])
	assert.Equal(t, "published", states[rb.ID])
	_, err = e.pages.Rollback(ctx, coretest.Viewer, d.Page.ID)
	assert.ErrorIs(t, err, core.ErrForbidden)
}

// Catches: editor text reaching the page as markup, an attribute escape,
// and a slug or field name that could carry a path or script.
func TestUntrustedContentIsEscaped(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	doc := goodDoc()
	doc.Headline = `<script>alert(1)</script> Villas`
	doc.Sections[0].Text = `"><img src=x onerror=alert(2)>`
	doc.Sections[1].Items = []string{"<b>bold</b>", "javascript:alert(3)"}
	doc.Form.Fields[0].Label = `Name" onfocus="alert(4)`
	doc.Form.SuccessMessage = "<i>thanks</i>"
	d, err := e.pages.Create(ctx, coretest.Marketer, pages.PageInput{BrandID: e.reportage, ProjectID: e.wadi, Slug: "safe", Name: "Safe", Language: "ar", Document: doc})
	require.NoError(t, err)
	html, err := e.pages.Render(ctx, coretest.Viewer, d.Page.ID, 1)
	require.NoError(t, err)
	s := string(html)
	assert.NotContains(t, s, "<script>")
	assert.Contains(t, s, "&lt;script&gt;alert(1)&lt;/script&gt; Villas")
	assert.NotContains(t, s, "<img src=x")
	assert.NotContains(t, s, "<b>bold</b>")
	assert.NotContains(t, s, `onfocus="alert(4)`)
	assert.Contains(t, s, `dir="rtl"`)
	assert.Contains(t, s, `action="/p/safe/form"`)
	assert.Contains(t, s, `name="website"`, "honeypot present")
	assert.NotContains(t, s, "<script", "no script anywhere in a page")
	_, err = e.pages.Publish(ctx, coretest.Marketer, d.Page.ID, 1)
	require.NoError(t, err)
	thanks, err := e.pages.ThankYou(ctx, "safe", true)
	require.NoError(t, err)
	assert.Contains(t, string(thanks), "&lt;i&gt;thanks&lt;/i&gt;")
	assert.Contains(t, string(thanks), "test submission")

	for _, in := range []pages.PageInput{
		{BrandID: e.reportage, Slug: "../etc", Name: "x", Language: "en", Document: goodDoc()},
		{BrandID: e.reportage, Slug: "Has Space", Name: "x", Language: "en", Document: goodDoc()},
		{BrandID: e.reportage, Slug: "ok-slug", Name: "x", Language: "fr", Document: goodDoc()},
		{BrandID: e.reportage, ProjectID: e.wadi, Slug: "ok-slug", Name: "x", Language: "en", Document: pages.PageDocument{Title: "t", Sections: []pages.Section{{Kind: "html", Text: "<b>"}}}},
		{BrandID: e.reportage, Slug: "ok-slug", Name: "x", Language: "en", Document: pages.PageDocument{Title: "t", Form: &pages.FormConfig{Fields: []pages.FormField{{Name: "_project", Kind: "text"}}}}},
		{BrandID: e.reportage, Slug: "ok-slug", Name: "x", Language: "en", Document: pages.PageDocument{Title: "t", Form: &pages.FormConfig{Fields: []pages.FormField{{Name: "website", Kind: "text"}}}}},
		{BrandID: e.reportage, Slug: "ok-slug", Name: "x", Language: "en", Document: pages.PageDocument{Title: "t", Sections: []pages.Section{{Kind: "image", ImageKey: "sha256-" + strings.Repeat("0", 64)}}}},
	} {
		_, err := e.pages.Create(ctx, coretest.Marketer, in)
		require.Error(t, err, "%+v", in)
	}
	_, err = e.pages.Create(ctx, coretest.Viewer, pages.PageInput{BrandID: e.reportage, Slug: "v", Name: "x", Language: "en", Document: goodDoc()})
	assert.ErrorIs(t, err, core.ErrForbidden)
	gate := e.pages.Publishing()
	assert.Equal(t, "adx", gate.Local)
	assert.False(t, gate.External.Available)
}

func codes(issues []pages.PageReadinessIssue) []string {
	out := []string{}
	for _, i := range issues {
		out = append(out, i.Code)
	}
	return out
}

func count(xs []string, x string) int {
	n := 0
	for _, s := range xs {
		if s == x {
			n++
		}
	}
	return n
}

func f64(f float64) *float64 { return &f }
