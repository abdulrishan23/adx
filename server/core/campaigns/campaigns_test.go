package campaigns

import (
	"bytes"
	"image"
	"image/jpeg"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/pages"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/studio"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/blobs"
	"github.com/WokeWorks/adx/server/store/storetest"
)

var (
	admin    = core.Actor{UserID: 0, Role: core.RoleAdmin, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
	marketer = core.Actor{UserID: 0, Role: core.RoleMarketer, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
	viewer   = core.Actor{UserID: 0, Role: core.RoleViewer, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
)

type env struct {
	t       *testing.T
	db      *store.DB
	c       *Campaigns
	brands  *brands.Brands
	studio  *studio.Studio
	pages   *pages.Pages
	plans   *plans.Plans
	brand   int64
	project int64
	other   int64
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{t: t, db: storetest.Open(t)}
	clock := coretest.NewClock(time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC))
	e.brands = brands.New(e.db, brands.Config{Clock: clock})
	files, err := blobs.New(filepath.Join(t.TempDir(), "files"))
	if err != nil {
		t.Fatal(err)
	}
	e.studio = studio.New(e.db, files)
	e.pages = pages.New(e.db, e.brands, evidence.New(e.db, clock), pages.Config{Clock: clock, PublicBase: "https://adx.example", RatePerMinute: 3, DailyPerPage: 12, Salt: "test"})
	e.plans = plans.New(e.db, e.brands, plans.Config{Clock: clock})
	e.c = New(e.db, Deps{Brands: e.brands, Studio: e.studio, Pages: e.pages, Plans: e.plans})
	b, err := e.brands.Create(t.Context(), admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	if err != nil {
		t.Fatal(err)
	}
	e.brand = b.ID
	e.project = e.newProject("The Wadi")
	e.other = e.newProject("Verdana")
	return e
}

func (e *env) newProject(name string) int64 {
	e.t.Helper()
	p, err := e.db.Writer().CreateProject(e.t.Context(), store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: name, CreatedAt: time.Now()})
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.brands.AssignProject(e.t.Context(), admin, p.ID, e.brand); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.studio.SetUp(e.t.Context(), marketer, studio.SetupInput{ProjectID: p.ID}); err != nil {
		e.t.Fatal(err)
	}
	return p.ID
}

func (e *env) campaign(name string) CampaignDetail {
	e.t.Helper()
	d, err := e.c.Create(e.t.Context(), marketer, CampaignInput{ProjectID: e.project, Name: name, Goal: "150 leads", StartsOn: "2026-10-24", EndsOn: "2026-10-26", Channels: []string{"meta", "google"}, Markets: []string{"UAE"}})
	if err != nil {
		e.t.Fatal(err)
	}
	return d
}

func (e *env) jpg(w, h int, shade uint8) string {
	e.t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		e.t.Fatal(err)
	}
	ref, err := e.studio.Upload(e.t.Context(), marketer, &buf)
	if err != nil {
		e.t.Fatal(err)
	}
	return ref.SHA256
}

// Catches a campaign being created with dates the wrong way round, or a
// channel the platforms don't have, which would stall its plan later.
func TestCreateChecksTheBrief(t *testing.T) {
	e := setup(t)
	_, err := e.c.Create(t.Context(), marketer, CampaignInput{ProjectID: e.project, Name: "Bad", StartsOn: "2026-10-26", EndsOn: "2026-10-24"})
	if err == nil || !strings.Contains(err.Error(), "before the start") {
		t.Fatalf("want the date order refused, got %v", err)
	}
	_, err = e.c.Create(t.Context(), marketer, CampaignInput{ProjectID: e.project, Name: "Bad", Channels: []string{"tiktok"}})
	if err == nil || !strings.Contains(err.Error(), "meta and google") {
		t.Fatalf("want the channel refused, got %v", err)
	}
	if _, err := e.c.Create(t.Context(), viewer, CampaignInput{ProjectID: e.project, Name: "Nope"}); err != core.ErrForbidden {
		t.Fatalf("viewers can't create campaigns, got %v", err)
	}
	d := e.campaign("Launch weekend")
	if d.StartsOn != "2026-10-24" || d.EndsOn != "2026-10-26" || d.ProjectName != "The Wadi" || d.State != "planning" {
		t.Fatalf("campaign came back wrong: %+v", d.CampaignSummary)
	}
	if _, err := e.c.Create(t.Context(), marketer, CampaignInput{ProjectID: e.project, Name: "launch weekend"}); err == nil {
		t.Fatal("the same name twice in a project should conflict")
	}
}

// Catches ads made for one campaign showing up under another, and a concept
// being filed under a campaign of a different project.
func TestAdsBelongToTheirCampaign(t *testing.T) {
	e := setup(t)
	a := e.campaign("Launch weekend")
	b := e.campaign("Handover")
	if _, err := e.studio.CreateConcept(t.Context(), marketer, e.other, studio.NewConcept{Code: "C1", CampaignID: a.ID}); err == nil {
		t.Fatal("a campaign of another project should be refused")
	}
	k, err := e.studio.CreateConcept(t.Context(), marketer, e.project, studio.NewConcept{Code: "C1", Name: "Launch", CampaignID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	if k.CampaignID == nil || *k.CampaignID != a.ID {
		t.Fatalf("concept should carry its campaign, got %v", k.CampaignID)
	}
	if _, err := e.studio.AddUploads(t.Context(), marketer, e.project, []studio.UploadedAd{{
		Concept: "C1", Slug: "LAUNCH", Lang: studio.LangEN, Fmt: studio.FmtStory, File: e.jpg(1080, 1920, 10), Preview: e.jpg(64, 114, 10),
	}}); err != nil {
		t.Fatal(err)
	}
	got, err := e.c.Get(t.Context(), viewer, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Concepts) != 1 || got.Ads.Total != 1 || got.Ads.Waiting != 1 {
		t.Fatalf("campaign A should have the one ad awaiting review, got %d concepts, tally %+v", len(got.Concepts), got.Ads)
	}
	if s := got.Stages[1]; s.ID != "ads" || s.Status != "caution" || s.Note != "1 awaiting review" {
		t.Fatalf("ads stage: %+v", s)
	}
	if got.State != "in_production" {
		t.Fatalf("state should be in_production, got %s", got.State)
	}
	other, err := e.c.Get(t.Context(), viewer, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Concepts) != 0 || other.Ads.Total != 0 {
		t.Fatalf("campaign B should be empty, got %d concepts", len(other.Concepts))
	}
}

// Catches a page made for a campaign not showing in its Page stage, which
// happened once when the summaries left the campaign out.
func TestPagesAndPlansReachTheirStages(t *testing.T) {
	e := setup(t)
	a := e.campaign("Launch weekend")
	if _, err := e.pages.Create(t.Context(), marketer, pages.PageInput{
		BrandID: e.brand, ProjectID: e.project, CampaignID: a.ID, Slug: "wadi-launch", Name: "Launch", Language: "en",
		Document: pages.PageDocument{Title: "Launch", Sections: []pages.Section{{Kind: "text", Text: "Villas."}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.plans.Create(t.Context(), marketer, plans.PlanInput{
		BrandID: e.brand, ProjectID: e.project, CampaignID: a.ID, Title: "Launch",
		Document: plans.PlanDocument{Objective: "Leads", BudgetMinor: 100, Currency: "AED", PeriodFrom: "2026-10-24", PeriodTo: "2026-10-26",
			Allocations: []plans.Allocation{{Channel: "meta_lead_form", BudgetMinor: 100}}},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := e.c.Get(t.Context(), viewer, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Pages) != 1 || got.Stages[2].Note != "Drafted, not published" {
		t.Fatalf("page stage: %d pages, %+v", len(got.Pages), got.Stages[2])
	}
	if got.Plan == nil || got.Stages[3].Note != "Ready to accept" {
		t.Fatalf("plan stage: %+v %+v", got.Plan, got.Stages[3])
	}
}

// Catches a project's campaigns list leaking another project's, and the
// workspace list missing one.
func TestListsAreScoped(t *testing.T) {
	e := setup(t)
	e.campaign("Launch weekend")
	if _, err := e.c.Create(t.Context(), marketer, CampaignInput{ProjectID: e.other, Name: "Verdana spring"}); err != nil {
		t.Fatal(err)
	}
	mine, err := e.c.ListForProject(t.Context(), viewer, e.project)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].Name != "Launch weekend" {
		t.Fatalf("project list: %+v", mine)
	}
	all, err := e.c.List(t.Context(), viewer)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("workspace list should have both, got %d", len(all))
	}
}
