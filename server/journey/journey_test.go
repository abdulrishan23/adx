// Package journey runs the cross-phase acceptance journey the build
// specification asks for at the final gate: brand to plan to reviewed asset
// to deployed page to enquiry to outcome, through the real core packages
// with fake providers, then proves a second client sees none of it.
package journey_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assets"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/creative"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/learning"
	"github.com/WokeWorks/adx/server/core/measurement"
	"github.com/WokeWorks/adx/server/core/pages"
	"github.com/WokeWorks/adx/server/core/pipeline"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/projects"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/core/workspaces"
	"github.com/WokeWorks/adx/server/providers/filestore"
	"github.com/WokeWorks/adx/server/providers/svgrender"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: a break anywhere in the chain from brand to outcome, an approval
// or deployment that claims to be live, an enquiry that does not become a
// person with a score and an opportunity, an outcome that attaches to the
// wrong intervention, and a second client seeing the first's records.
func TestBrandToPlanToAssetToPageToEnquiryToOutcome(t *testing.T) {
	db := storetest.Open(t)
	coretest.SeedUsers(t, db)
	ctx := t.Context()
	clock := coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))
	for _, m := range []struct {
		user int64
		role string
	}{{1, "admin"}, {2, "marketer"}, {3, "viewer"}} {
		require.NoError(t, db.Writer().CreateMembership(ctx, store.CreateMembershipParams{WorkspaceID: core.DefaultWorkspace, UserID: m.user, Role: m.role, Enabled: 1, CreatedAt: clock.Now()}))
	}
	ev := evidence.New(db, clock)
	brandSvc := brands.New(db, brands.Config{Clock: clock})
	planSvc := plans.New(db, brandSvc, plans.Config{Clock: clock})
	files, err := filestore.New(t.TempDir())
	require.NoError(t, err)
	creativeSvc := creative.New(db, files, svgrender.New(), creative.Config{Clock: clock, Brands: brandSvc, Plans: planSvc})
	pageSvc := pages.New(db, brandSvc, ev, pages.Config{Clock: clock, PublicBase: "https://adx.example", Salt: "j"})
	measure := measurement.New(db, ev, pageSvc, creativeSvc, planSvc, nil, measurement.Config{Clock: clock})
	people := crm.New(db, ev, crm.Config{Clock: clock})
	sig := signals.New(db, ev, people, signals.Config{Clock: clock})
	pipe := pipeline.New(db, ev, pipeline.Config{Clock: clock})
	memory := learning.New(db, learning.Config{Clock: clock})
	projectSvc := projects.New(db)

	// Phase 2: brand, project, typed plan, accepted version.
	brand, err := brandSvc.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	wadi, err := projectSvc.Create(ctx, coretest.Admin, projects.CreateInput{Name: "The Wadi"})
	require.NoError(t, err)
	require.NoError(t, brandSvc.AssignProject(ctx, coretest.Admin, wadi.ID, brand.ID))
	cpl := int64(25000)
	plan, err := planSvc.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: brand.ID, ProjectID: wadi.ID, Title: "Wadi launch", Document: plans.PlanDocument{
		Objective: "Qualified leads", BudgetMinor: 1_000_000, Currency: "AED", PeriodFrom: "2026-11-01", PeriodTo: "2026-11-30",
		Allocations:             []plans.Allocation{{Channel: "google_search", Objective: "leads", BudgetMinor: 1_000_000, Geography: "AE", ExpectedCPLMinor: &cpl}},
		AssetRequirements:       []plans.AssetRequirement{{Format: "1:1", Language: "en", Count: 1}},
		PageRequirements:        []plans.PageRequirement{{Kind: "landing", Language: "en"}},
		MeasurementRequirements: []plans.MeasurementRequirement{{Kind: "utm_scheme", Platform: "all", Name: "ADX scheme"}},
	}})
	require.NoError(t, err)
	accepted, err := planSvc.Accept(ctx, coretest.Admin, plan.Plan.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), accepted.Version)

	// Phase 1 and 3: a reviewed asset against the brand kit, approved by an admin.
	render, err := creativeSvc.UploadAsset(ctx, coretest.Marketer, wadi.ID, "render", "wadi.jpg", "image/jpeg", strings.NewReader("jpeg bytes"))
	require.NoError(t, err)
	logo, err := creativeSvc.UploadAsset(ctx, coretest.Marketer, wadi.ID, "logo", "logo.png", "image/png", strings.NewReader("png bytes"))
	require.NoError(t, err)
	_, err = creativeSvc.PutBrandKit(ctx, coretest.Marketer, wadi.ID, assets.BrandKit{Logos: map[string]*assets.Ref{"onDark": &logo.Ref}, Colors: map[string]string{"inkLight": "#FFFFFF", "accent": "#C9A227"}})
	require.NoError(t, err)
	cr, v1, err := creativeSvc.Create(ctx, coretest.Marketer, creative.CreativeInput{ProjectID: wadi.ID, Name: "Wadi 1:1", Format: assets.Format1x1, Language: assets.English,
		Copy: assets.Copy{Eyebrow: "The Wadi", Title: "Villas in Dubailand", CTA: "Register"}, RenderAssetID: render.ID})
	require.NoError(t, err)
	assert.Equal(t, assets.Draft, v1.ReviewState)
	rendered, err := creativeSvc.Render(ctx, coretest.Marketer, cr.ID)
	require.NoError(t, err)
	assert.Equal(t, "template", *rendered.Renderer, "template layout, never labelled as generation")
	_, err = creativeSvc.Submit(ctx, coretest.Marketer, cr.ID)
	require.NoError(t, err)
	approved, err := creativeSvc.ReviewVersion(ctx, coretest.Admin, cr.ID, "approved", "fine")
	require.NoError(t, err)
	assert.Equal(t, assets.Approved, approved.ReviewState)

	// Phase 3: a page bound to the plan, published on ADX's own host.
	page, err := pageSvc.Create(ctx, coretest.Marketer, pages.PageInput{BrandID: brand.ID, ProjectID: wadi.ID, PlanID: plan.Plan.ID, PlanVersion: 1, Slug: "wadi", Name: "Wadi", Language: "en", Document: pages.PageDocument{
		Title: "The Wadi", Headline: "Villas in Dubailand", Sections: []pages.Section{{Kind: "form"}},
		Form: &pages.FormConfig{Fields: []pages.FormField{{Name: "name", Label: "Name", Kind: "text", Required: true}, {Name: "phone", Label: "Phone", Kind: "phone", Required: true}}, ConsentText: "You agree to be contacted."},
	}})
	require.NoError(t, err)
	deployment, err := pageSvc.Publish(ctx, coretest.Marketer, page.Page.ID, 1)
	require.NoError(t, err)
	assert.NotEmpty(t, deployment.URL)
	readiness, err := measure.Readiness(ctx, coretest.Viewer, plan.Plan.ID, 1)
	require.NoError(t, err)
	assert.False(t, readiness.Live, "readiness is never delivery")
	assert.Nil(t, readiness.ExecutionAuthorization, "nothing here authorizes spend")

	// Phase 4: an enquiry arrives through the page and becomes one counted
	// event, one person, one scored signal.
	_, err = pageSvc.Submit(ctx, pages.SubmissionInput{Slug: "wadi", Fields: map[string]string{"name": "Amal", "phone": "+971 50 123 4567"}, ProjectID: wadi.ID, RemoteIP: "1.1.1.1"})
	require.NoError(t, err)
	recorded, err := measure.RecordPending(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 1, recorded)
	enquiries, err := measure.CountEnquiries(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.Equal(t, int64(1), enquiries)
	linked, err := people.Sync(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 1, linked)
	found, err := people.List(ctx, coretest.Viewer, "Amal", 10)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, 1, found[0].Enquiries)
	detail, err := people.Get(ctx, coretest.Viewer, found[0].ID)
	require.NoError(t, err)
	require.Len(t, detail.Enquiries, 1)
	derived, err := sig.Derive(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 1, derived, "the submission is one signal, derived once")
	derived, err = sig.Derive(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 0, derived)
	score, err := sig.Assess(ctx, coretest.Marketer, found[0].ID, "journey")
	require.NoError(t, err)
	assert.Equal(t, found[0].ID, score.PersonID)
	assert.Nil(t, score.Temperature, "no scoring rules are configured, so nobody is hot or warm")

	// Phase 5: the enquiry is an opportunity; the advice about it is a
	// learning record whose outcome attaches to the adopted intervention.
	_, err = pipe.PutConfig(ctx, coretest.Admin, pipeline.SuggestedConfig())
	require.NoError(t, err)
	opp, created, err := pipe.Open(ctx, coretest.Marketer, detail.Enquiries[0].ID)
	require.NoError(t, err)
	assert.True(t, created)
	_, created, err = pipe.Open(ctx, coretest.Marketer, detail.Enquiries[0].ID)
	require.NoError(t, err)
	assert.False(t, created, "one opportunity per enquiry")
	subject := "opportunity:" + itoa(opp.ID)
	rec, _, err := memory.Record(ctx, coretest.Marketer, learning.LearningRecordInput{RecommendationRef: "brief:" + itoa(opp.ID), Source: "brief", SubjectKind: "opportunity", SubjectRef: subject, Recommended: json.RawMessage(`{"step":"call"}`), WindowDays: 14})
	require.NoError(t, err)
	_, err = memory.Decide(ctx, coretest.Marketer, rec.ID, learning.LearningDecisionInput{Decision: "adopted", ActionRef: "call:1", ActionAt: clock.Now()})
	require.NoError(t, err)
	clock.Advance(72 * time.Hour)
	outcome, err := memory.Observe(ctx, coretest.Marketer, learning.LearningOutcomeInput{SubjectKind: "opportunity", SubjectRef: subject, Metric: "stage", Value: 1, Class: "observed", ObservedAt: clock.Now()})
	require.NoError(t, err)
	require.NotNil(t, outcome.RecordID)
	assert.Equal(t, rec.ID, *outcome.RecordID, "the outcome lands on the intervention whose window covers it")

	// Phase 6: a second client with the same project name sees none of it.
	ws := workspaces.New(db, clock.Now)
	acme, err := ws.Create(ctx, coretest.Admin, workspaces.WorkspaceInput{Slug: "acme", Name: "Acme Homes", Industry: "real_estate"})
	require.NoError(t, err)
	acmeAdmin := core.Actor{UserID: 1, Role: core.RoleAdmin, Via: core.ViaWeb, WorkspaceID: acme.ID}
	_, err = projectSvc.Create(ctx, acmeAdmin, projects.CreateInput{Name: "The Wadi"})
	require.NoError(t, err, "the same project name in another client is fine")
	theirPeople, err := people.List(ctx, acmeAdmin, "Amal", 10)
	require.NoError(t, err)
	assert.Empty(t, theirPeople)
	theirPages, err := pageSvc.List(ctx, acmeAdmin)
	require.NoError(t, err)
	assert.Empty(t, theirPages)
	theirPlans, err := planSvc.List(ctx, acmeAdmin)
	require.NoError(t, err)
	assert.Empty(t, theirPlans)
	_, err = people.Get(ctx, acmeAdmin, found[0].ID)
	require.Error(t, err)
	_, err = pipe.Get(ctx, acmeAdmin, opp.ID)
	require.Error(t, err)
	theirMemory, err := memory.List(ctx, acmeAdmin, 50)
	require.NoError(t, err)
	assert.Empty(t, theirMemory)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
