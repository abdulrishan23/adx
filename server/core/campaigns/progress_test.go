package campaigns

import (
	"context"
	"testing"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/measurement"
	"github.com/WokeWorks/adx/server/core/pages"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/research"
)

// fakeResearch answers with scripted briefs. finished marks briefs whose
// items have all run.
type fakeResearch struct {
	briefs   []research.Brief
	finished map[int64]bool
}

func (f *fakeResearch) ListBriefs(context.Context, core.Actor) ([]research.Brief, error) {
	return f.briefs, nil
}

func (f *fakeResearch) GetBrief(_ context.Context, _ core.Actor, id int64) (research.BriefDetail, error) {
	state := research.ItemPending
	if f.finished[id] {
		state = research.ItemFetched
	}
	return research.BriefDetail{Items: []research.Item{{ID: 1, BriefID: id, State: state}, {ID: 2, BriefID: id, State: state}}}, nil
}

// fakeReadiness answers every plan with the same items.
type fakeReadiness struct {
	items []measurement.LaunchItem
	calls int
}

func (f *fakeReadiness) Readiness(_ context.Context, _ core.Actor, planID, version int64) (measurement.PlanLaunchReadiness, error) {
	f.calls++
	return measurement.PlanLaunchReadiness{PlanID: planID, PlanVersion: version, Items: f.items}, nil
}

func section(p ProjectProgress, id string) SectionState {
	for _, s := range p.Sections {
		if s.ID == id {
			return s
		}
	}
	return SectionState{}
}

// Catches: a section counted done from another project's facts, plans or
// pages; research counted done while its items still run; a blocked
// measurement line not gating Campaigns; lines outside measurement's areas
// (the plan's own issues) leaking into Measurement; and progress on an
// unknown project or for an outsider.
func TestProgressFollowsTheProjectThroughItsSections(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	rs := &fakeResearch{finished: map[int64]bool{}}
	rd := &fakeReadiness{}
	e.c.AttachReaders(rs, rd)

	p, err := e.c.Progress(ctx, viewer, e.project)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Sections) != 5 || p.Blocked {
		t.Fatalf("empty project: %+v", p)
	}
	for _, s := range p.Sections {
		if s.Status != "pending" {
			t.Fatalf("empty project has %s %s", s.ID, s.Status)
		}
	}
	if section(p, "measurement").Note != "Nothing to check until a campaign has a plan" {
		t.Fatalf("measurement with no plan: %+v", section(p, "measurement"))
	}

	// A fact for the other project changes nothing here; one for this
	// project needs review until approved, and a running brief is not done.
	otherFact, err := e.brands.AddFact(ctx, marketer, e.brand, brands.FactInput{ProjectID: e.other, Key: "price.from", Kind: brands.KindMoney, Value: "90000000", Currency: "AED", EffectiveFrom: "2026-10-01", SourceKind: brands.SourceDocument, SourceRef: "verdana list"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.brands.ApproveFact(ctx, admin, otherFact.Fact.ID); err != nil {
		t.Fatal(err)
	}
	p, _ = e.c.Progress(ctx, viewer, e.project)
	if section(p, "brand").Status != "pending" {
		t.Fatalf("another project's fact counted: %+v", section(p, "brand"))
	}
	mine, err := e.brands.AddFact(ctx, marketer, e.brand, brands.FactInput{ProjectID: e.project, Key: "price.from", Kind: brands.KindMoney, Value: "120000000", Currency: "AED", EffectiveFrom: "2026-10-01", SourceKind: brands.SourceDocument, SourceRef: "wadi list"})
	if err != nil {
		t.Fatal(err)
	}
	p, _ = e.c.Progress(ctx, viewer, e.project)
	if s := section(p, "brand"); s.Status != "caution" || s.Note != "1 fact needs review" {
		t.Fatalf("unapproved fact: %+v", s)
	}
	if _, err := e.brands.ApproveFact(ctx, admin, mine.Fact.ID); err != nil {
		t.Fatal(err)
	}
	pid := e.project
	rs.briefs = []research.Brief{{ID: 7, BrandID: e.brand, ProjectID: &pid, Title: "Wadi"}}
	p, _ = e.c.Progress(ctx, viewer, e.project)
	if s := section(p, "brand"); s.Status != "caution" || s.Note != "1 approved fact, no research yet" {
		t.Fatalf("research still running: %+v", s)
	}
	rs.finished[7] = true
	p, _ = e.c.Progress(ctx, viewer, e.project)
	if s := section(p, "brand"); s.Status != "ready" {
		t.Fatalf("approved fact and finished brief: %+v", s)
	}

	// A campaign with a page and a plan moves Studio, Media and Campaigns.
	a := e.campaign("Launch weekend")
	if _, err := e.pages.Create(ctx, marketer, pages.PageInput{
		BrandID: e.brand, ProjectID: e.project, CampaignID: a.ID, Slug: "wadi-launch", Name: "Launch", Language: "en",
		Document: pages.PageDocument{Title: "Launch", Sections: []pages.Section{{Kind: "text", Text: "Villas."}}},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := e.plans.Create(ctx, marketer, plans.PlanInput{
		BrandID: e.brand, ProjectID: e.project, CampaignID: a.ID, Title: "Launch",
		Document: plans.PlanDocument{Objective: "Leads", BudgetMinor: 100, Currency: "AED", PeriodFrom: "2026-10-24", PeriodTo: "2026-10-26",
			Allocations: []plans.Allocation{{Channel: "meta_lead_form", BudgetMinor: 100}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rd.items = []measurement.LaunchItem{
		{Area: "plan", Code: "plan_issue", State: "blocked", Message: "the plan's own problem"},
		{Area: "pages", Code: "page_unpublished", State: "warning", Message: "page not published", RefKind: "page", RefID: 1},
	}
	p, err = e.c.Progress(ctx, viewer, e.project)
	if err != nil {
		t.Fatal(err)
	}
	if s := section(p, "studio"); s.Status != "caution" || s.Note != "Started, no ads approved" {
		t.Fatalf("studio with a page and no ads: %+v", s)
	}
	if s := section(p, "media"); s.Status != "caution" || s.Note != "1 plan drafted" {
		t.Fatalf("media with a draft plan: %+v", s)
	}
	if s := section(p, "campaigns"); s.Status != "caution" || s.Note != "1 in progress" {
		t.Fatalf("campaigns: %+v", s)
	}
	if len(p.Measurement) != 1 || p.Measurement[0].Code != "page_unpublished" || p.Measurement[0].CampaignName != "Launch weekend" {
		t.Fatalf("the plan's own issue leaked into measurement, or the campaign isn't named: %+v", p.Measurement)
	}
	if p.Blocked || section(p, "measurement").Status != "caution" {
		t.Fatalf("a warning is not a blocker: %+v", p)
	}

	// A blocked measurement line gates Campaigns.
	rd.items = append(rd.items, measurement.LaunchItem{Area: "accounts", Code: "conversion_action_missing", State: "blocked", Message: "no conversion action", RefKind: "account", RefID: 3})
	p, _ = e.c.Progress(ctx, viewer, e.project)
	if !p.Blocked || section(p, "measurement").Status != "error" || section(p, "measurement").Note != "1 blocker" {
		t.Fatalf("blocked measurement: %+v", p)
	}
	if s := section(p, "campaigns"); s.Status != "error" || s.Note != "Blocked by measurement" {
		t.Fatalf("campaigns not gated: %+v", s)
	}

	// Accepting the plan finishes Media, and clean checks finish Measurement.
	if _, err := e.plans.Accept(ctx, admin, plan.Plan.ID, 1); err != nil {
		t.Fatal(err)
	}
	rd.items = []measurement.LaunchItem{{Area: "measurement", Code: "form_submit", State: "ok", Message: "test submission arrived"}}
	p, _ = e.c.Progress(ctx, viewer, e.project)
	if s := section(p, "media"); s.Status != "ready" || s.Note != "1 plan accepted" {
		t.Fatalf("accepted plan: %+v", s)
	}
	if s := section(p, "measurement"); s.Status != "ready" || p.Blocked {
		t.Fatalf("clean checks: %+v", s)
	}

	// The other project has none of this.
	q, err := e.c.Progress(ctx, viewer, e.other)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range q.Sections {
		if s.ID != "brand" && s.Status != "pending" {
			t.Fatalf("the other project borrowed %s: %+v", s.ID, s)
		}
	}

	if _, err := e.c.Progress(ctx, viewer, 999); err == nil {
		t.Fatal("unknown project should be not found")
	}
	if _, err := e.c.Progress(ctx, core.Actor{}, e.project); err != core.ErrForbidden {
		t.Fatalf("outsider: %v", err)
	}
}

// Catches Measurement claiming an answer when nothing can check it.
func TestProgressWithoutReadersSaysNotAvailable(t *testing.T) {
	e := setup(t)
	p, err := e.c.Progress(t.Context(), viewer, e.project)
	if err != nil {
		t.Fatal(err)
	}
	if s := section(p, "measurement"); s.Status != "pending" || s.Note != "Not available" {
		t.Fatalf("no reader: %+v", s)
	}
}
