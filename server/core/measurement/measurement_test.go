package measurement_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assets"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/creative"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/measurement"
	"github.com/WokeWorks/adx/server/core/pages"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/providers/filestore"
	"github.com/WokeWorks/adx/server/providers/svgrender"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	brands    *brands.Brands
	plans     *plans.Plans
	pages     *pages.Pages
	creative  *creative.Service
	m         *measurement.Measurement
	reportage int64
	wadi      int64
	googleAcc int64
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	ev := evidence.New(e.db, e.clock)
	e.brands = brands.New(e.db, brands.Config{Clock: e.clock})
	e.plans = plans.New(e.db, e.brands, plans.Config{Clock: e.clock})
	e.pages = pages.New(e.db, e.brands, ev, pages.Config{Clock: e.clock, PublicBase: "https://adx.example", Salt: "t"})
	st, err := filestore.New(t.TempDir())
	require.NoError(t, err)
	e.creative = creative.New(e.db, st, svgrender.New(), creative.Config{Clock: e.clock, Brands: e.brands, Plans: e.plans})
	e.m = measurement.New(e.db, ev, e.pages, e.creative, e.plans, nil, measurement.Config{Clock: e.clock})
	b, err := e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	e.reportage = b.ID
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.wadi = p.ID
	require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, e.wadi, e.reportage))
	cred, err := e.db.Writer().UpsertCredential(ctx, store.UpsertCredentialParams{Provider: "google_ads", Kind: "oauth", Subject: "u", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "x", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(ctx, store.UpsertAccountParams{Type: "google_ads", ExternalID: "1234567890", CredentialID: cred.ID, Currency: sql.NullString{String: "AED", Valid: true}, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	e.googleAcc = acct.ID
	return e
}

func (e *env) plan(t *testing.T) plans.PlanDetail {
	t.Helper()
	cpl := int64(25000)
	d, err := e.plans.Create(t.Context(), coretest.Marketer, plans.PlanInput{BrandID: e.reportage, ProjectID: e.wadi, Title: "Wadi Nov", Document: plans.PlanDocument{
		Objective: "Leads", BudgetMinor: 1_000_000, Currency: "AED", PeriodFrom: "2026-11-01", PeriodTo: "2026-11-30",
		Allocations:             []plans.Allocation{{Channel: "google_search", Objective: "leads", BudgetMinor: 1_000_000, Geography: "AE", ExpectedCPLMinor: &cpl}},
		AssetRequirements:       []plans.AssetRequirement{{Format: "1:1", Language: "en", Count: 1}},
		PageRequirements:        []plans.PageRequirement{{Kind: "landing", Language: "en"}},
		MeasurementRequirements: []plans.MeasurementRequirement{{Kind: "conversion_action", Platform: "google_ads", Name: "Lead form submit", Required: true}, {Kind: "utm_scheme", Platform: "all", Name: "ADX scheme"}},
	}})
	require.NoError(t, err)
	return d
}

func (e *env) page(t *testing.T, planID int64) pages.PageDetail {
	t.Helper()
	d, err := e.pages.Create(t.Context(), coretest.Marketer, pages.PageInput{BrandID: e.reportage, ProjectID: e.wadi, PlanID: planID, PlanVersion: 1, Slug: "wadi", Name: "Wadi", Language: "en", Document: pages.PageDocument{
		Title: "Wadi", Headline: "Villas", Sections: []pages.Section{{Kind: "form"}},
		Form: &pages.FormConfig{Fields: []pages.FormField{{Name: "name", Label: "Name", Kind: "text", Required: true}, {Name: "phone", Label: "Phone", Kind: "phone", Required: true}, {Name: "email", Label: "Email", Kind: "email"}}, ConsentText: "ok"},
	}})
	require.NoError(t, err)
	_, err = e.pages.Publish(t.Context(), coretest.Marketer, d.Page.ID, 1)
	require.NoError(t, err)
	return d
}

// Catches: a submission becoming two events, a repeat of the same person
// counting twice, a test counting as an enquiry, and a controlled check
// creating anything but a test event.
func TestOneSubmissionIsOneEventAndTestsNeverCount(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	pd := e.plan(t)
	pg := e.page(t, pd.Plan.ID)
	fields := map[string]string{"name": "Amal", "phone": "+971 50 123 4567", "email": "amal@example.com"}
	_, err := e.pages.Submit(ctx, pages.SubmissionInput{Slug: "wadi", Fields: fields, ProjectID: e.wadi, RemoteIP: "1.1.1.1"})
	require.NoError(t, err)
	n, err := e.m.RecordPending(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	n, err = e.m.RecordPending(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "recording is idempotent")
	count, err := e.m.CountEnquiries(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Same person, same page, same day, different formatting: a duplicate.
	_, err = e.pages.Submit(ctx, pages.SubmissionInput{Slug: "wadi", Fields: map[string]string{"name": "Amal A.", "phone": "0501234567"}, ProjectID: e.wadi, RemoteIP: "1.1.1.2"})
	require.NoError(t, err)
	_, err = e.m.RecordPending(ctx, coretest.Marketer)
	require.NoError(t, err)
	count, _ = e.m.CountEnquiries(ctx, coretest.Viewer)
	assert.Equal(t, int64(1), count, "the repeat does not count")
	events, err := e.m.ListEvents(ctx, coretest.Viewer, pg.Page.ID, 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.False(t, events[0].Counts)
	assert.NotNil(t, events[0].DuplicateOfID)
	assert.Equal(t, events[1].ID, *events[0].DuplicateOfID)
	assert.Equal(t, events[1].DedupeKey, events[0].DedupeKey)

	// Another person the same day counts; the same person the next day counts again.
	_, err = e.pages.Submit(ctx, pages.SubmissionInput{Slug: "wadi", Fields: map[string]string{"name": "Bilal", "phone": "+971559999999"}, ProjectID: e.wadi, RemoteIP: "1.1.1.3"})
	require.NoError(t, err)
	e.clock.Advance(26 * time.Hour)
	_, err = e.pages.Submit(ctx, pages.SubmissionInput{Slug: "wadi", Fields: fields, ProjectID: e.wadi, RemoteIP: "1.1.1.1"})
	require.NoError(t, err)
	_, err = e.m.RecordPending(ctx, coretest.Marketer)
	require.NoError(t, err)
	count, _ = e.m.CountEnquiries(ctx, coretest.Viewer)
	assert.Equal(t, int64(3), count)

	// A controlled check: one test submission, one non-counting test event.
	before := count
	chk, err := e.m.RunCheck(ctx, coretest.Marketer, measurement.MeasurementCheckInput{PlanID: pd.Plan.ID, PlanVersion: 1, Kind: "form_submit", PageID: pg.Page.ID})
	require.NoError(t, err)
	assert.Equal(t, "passed", chk.State, chk.Detail)
	require.NotNil(t, chk.EventID)
	require.NotNil(t, chk.SubmissionID)
	require.NotNil(t, chk.EvidenceID)
	ev, err := e.db.Reader().GetMeasurementEvent(ctx, store.GetMeasurementEventParams{ID: *chk.EventID, WorkspaceID: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(1), ev.Test)
	assert.Equal(t, int64(0), ev.Counts)
	after, _ := e.m.CountEnquiries(ctx, coretest.Viewer)
	assert.Equal(t, before, after, "verification fabricated no enquiry")
	// Running it again makes another test event, still nothing counted.
	chk2, err := e.m.RunCheck(ctx, coretest.Marketer, measurement.MeasurementCheckInput{PlanID: pd.Plan.ID, PlanVersion: 1, Kind: "form_submit", PageID: pg.Page.ID})
	require.NoError(t, err)
	assert.Equal(t, "passed", chk2.State, chk2.Detail)
	after, _ = e.m.CountEnquiries(ctx, coretest.Viewer)
	assert.Equal(t, before, after)
	_, err = e.m.RunCheck(ctx, coretest.Viewer, measurement.MeasurementCheckInput{PlanID: pd.Plan.ID, PlanVersion: 1, Kind: "form_submit", PageID: pg.Page.ID})
	assert.ErrorIs(t, err, core.ErrForbidden)
}

// Catches: a check passing on an account with no conversion actions read, a
// disabled action passing, a Meta pixel being "verified", and an unknown
// check kind.
func TestChecksSayWhatTheyCannotVerify(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	pd := e.plan(t)
	chk, err := e.m.RunCheck(ctx, coretest.Marketer, measurement.MeasurementCheckInput{PlanID: pd.Plan.ID, PlanVersion: 1, Kind: "conversion_action", AccountID: e.googleAcc, Name: "Lead form submit"})
	require.NoError(t, err)
	assert.Equal(t, "failed", chk.State)
	assert.Contains(t, chk.Detail, "no conversion actions have been read")
	now := e.clock.Now()
	require.NoError(t, e.db.Writer().UpsertConversionAction(ctx, store.UpsertConversionActionParams{WorkspaceID: 1, AccountID: e.googleAcc, ExternalID: "1", Name: "Lead form submit", Type: sql.NullString{String: "WEBPAGE", Valid: true}, Status: sql.NullString{String: "REMOVED", Valid: true}, IncludeInConversions: 1, LastSeenAt: now}))
	chk, err = e.m.RunCheck(ctx, coretest.Marketer, measurement.MeasurementCheckInput{PlanID: pd.Plan.ID, PlanVersion: 1, Kind: "conversion_action", AccountID: e.googleAcc, Name: "lead form submit"})
	require.NoError(t, err)
	assert.Equal(t, "failed", chk.State)
	assert.Contains(t, chk.Detail, "REMOVED")
	require.NoError(t, e.db.Writer().UpsertConversionAction(ctx, store.UpsertConversionActionParams{WorkspaceID: 1, AccountID: e.googleAcc, ExternalID: "1", Name: "Lead form submit", Type: sql.NullString{String: "WEBPAGE", Valid: true}, Status: sql.NullString{String: "ENABLED", Valid: true}, IncludeInConversions: 1, LastSeenAt: now}))
	chk, err = e.m.RunCheck(ctx, coretest.Marketer, measurement.MeasurementCheckInput{PlanID: pd.Plan.ID, PlanVersion: 1, Kind: "conversion_action", AccountID: e.googleAcc, Name: "Lead form submit"})
	require.NoError(t, err)
	assert.Equal(t, "passed", chk.State, chk.Detail)
	chk, err = e.m.RunCheck(ctx, coretest.Marketer, measurement.MeasurementCheckInput{PlanID: pd.Plan.ID, PlanVersion: 1, Kind: "pixel_event", Name: "Lead"})
	require.NoError(t, err)
	assert.Equal(t, "unsupported", chk.State)
	assert.Contains(t, chk.Detail, "cannot verify")
	_, err = e.m.RunCheck(ctx, coretest.Marketer, measurement.MeasurementCheckInput{PlanID: pd.Plan.ID, PlanVersion: 1, Kind: "scroll_depth"})
	require.Error(t, err)
	chk, err = e.m.RunCheck(ctx, coretest.Marketer, measurement.MeasurementCheckInput{PlanID: pd.Plan.ID, PlanVersion: 1, Kind: "account_permission", AccountID: e.googleAcc})
	require.NoError(t, err)
	assert.Equal(t, "failed", chk.State, "connections not wired in this test")
	list, err := e.m.ListChecks(ctx, coretest.Viewer, pd.Plan.ID, 1)
	require.NoError(t, err)
	assert.Len(t, list, 5)
	cat := measurement.GetCatalog()
	unsupported := 0
	for _, ev := range cat.Events {
		if !ev.Supported {
			unsupported++
			assert.NotEmpty(t, ev.How)
		}
	}
	assert.Equal(t, 3, unsupported, "page views, WhatsApp and call clicks are explicitly unsupported")
	u, err := measurement.TrackedURL("https://adx.example/p/wadi?ref=x", measurement.UTM{Source: "google", Medium: "cpc", Campaign: "plan-1-v1-google_search", Content: "creative-3-v2"})
	require.NoError(t, err)
	assert.Equal(t, "https://adx.example/p/wadi?ref=x&utm_campaign=plan-1-v1-google_search&utm_content=creative-3-v2&utm_medium=cpc&utm_source=google", u)
}

// Catches: readiness without an accepted plan, without a ready creative or
// page, without a passed check; readiness claiming live; and an unsupported
// channel passing silently.
func TestLaunchReadinessJoinsPlanAssetsPagesAccountsAndChecks(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	pd := e.plan(t)
	r, err := e.m.Readiness(ctx, coretest.Viewer, pd.Plan.ID, 1)
	require.NoError(t, err)
	assert.False(t, r.Ready)
	assert.False(t, r.Live)
	assert.Nil(t, r.ExecutionAuthorization)
	codes := codesOf(r)
	assert.Contains(t, codes, "plan_not_accepted")
	assert.Contains(t, codes, "assets_short")
	assert.Contains(t, codes, "no_ready_page")
	assert.Contains(t, codes, "account_missing", "no connections service: no account is known")
	assert.Contains(t, codes, "check_missing")
	assert.Contains(t, codes, "not_submitted")

	// Accept the plan, bind a ready creative and a ready page, pass the checks.
	_, err = e.plans.Accept(ctx, coretest.Admin, pd.Plan.ID, 1)
	require.NoError(t, err)
	c, _, err := e.creative.Create(ctx, coretest.Marketer, creative.CreativeInput{ProjectID: e.wadi, Name: "Square", Format: assets.Format1x1, Language: assets.English, Copy: assets.Copy{Title: "Villas", CTA: "Register"}, PlanID: pd.Plan.ID, PlanVersion: 1})
	require.NoError(t, err)
	_, err = e.creative.Render(ctx, coretest.Marketer, c.ID)
	require.NoError(t, err)
	_, err = e.creative.Submit(ctx, coretest.Marketer, c.ID)
	require.NoError(t, err)
	r, err = e.m.Readiness(ctx, coretest.Viewer, pd.Plan.ID, 1)
	require.NoError(t, err)
	assert.Contains(t, codesOf(r), "creative_content_not_approved", "a creative waiting for review is not ready")
	_, err = e.creative.ReviewVersion(ctx, coretest.Admin, c.ID, "approved", "")
	require.NoError(t, err)
	pg := e.page(t, pd.Plan.ID)
	r, err = e.m.Readiness(ctx, coretest.Viewer, pd.Plan.ID, 1)
	require.NoError(t, err)
	assert.Contains(t, codesOf(r), "page_untested_form")
	chk, err := e.m.RunCheck(ctx, coretest.Marketer, measurement.MeasurementCheckInput{PlanID: pd.Plan.ID, PlanVersion: 1, Kind: "form_submit", PageID: pg.Page.ID})
	require.NoError(t, err)
	require.Equal(t, "passed", chk.State, chk.Detail)
	require.NoError(t, e.db.Writer().UpsertConversionAction(ctx, store.UpsertConversionActionParams{WorkspaceID: 1, AccountID: e.googleAcc, ExternalID: "1", Name: "Lead form submit", Status: sql.NullString{String: "ENABLED", Valid: true}, IncludeInConversions: 1, LastSeenAt: e.clock.Now()}))
	_, err = e.m.RunCheck(ctx, coretest.Marketer, measurement.MeasurementCheckInput{PlanID: pd.Plan.ID, PlanVersion: 1, Kind: "conversion_action", AccountID: e.googleAcc, Name: "Lead form submit"})
	require.NoError(t, err)
	r, err = e.m.Readiness(ctx, coretest.Viewer, pd.Plan.ID, 1)
	require.NoError(t, err)
	codes = codesOf(r)
	assert.Contains(t, codes, "plan_accepted")
	assert.Contains(t, codes, "creative_ready")
	assert.Contains(t, codes, "page_ready")
	assert.Contains(t, codes, "check_passed")
	assert.Contains(t, codes, "form_check_passed")
	assert.Contains(t, codes, "utm_scheme")
	assert.Contains(t, codes, "account_missing", "still no connected account in this test")
	assert.False(t, r.Ready, "an account gate still blocks")
	assert.False(t, r.Live)
	require.Len(t, r.Links, 1)
	assert.Contains(t, r.Links[0].URL, "utm_campaign=plan-1-v1-google_search")
	assert.Contains(t, r.Links[0].URL, "utm_content=creative-1-v1")
	assert.Contains(t, r.Links[0].URL, "utm_source=google")

	// Everything but the account passes: remove the account requirement by
	// planning an unsupported channel instead and see it called out.
	states := map[string]string{}
	for _, it := range r.Items {
		states[it.Code] = it.State
	}
	assert.Equal(t, "warning", states["not_submitted"])
	_, err = e.m.Readiness(ctx, core.Actor{Role: core.RoleViewer}, pd.Plan.ID, 1)
	assert.ErrorIs(t, err, core.ErrForbidden)
}

func codesOf(r measurement.PlanLaunchReadiness) []string {
	out := []string{}
	for _, it := range r.Items {
		out = append(out, it.Code)
	}
	return out
}
