package plans_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/assistant/assistanttest"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/store"
)

// genEnv adds a model, a brief with findings, a claim and an offer.
type genEnv struct {
	*env
	model   *assistanttest.Model
	brief   int64
	finding int64
	fact    int64
	offer   int64
	plan    int64
}

func setupGen(t *testing.T) *genEnv {
	t.Helper()
	g := &genEnv{model: &assistanttest.Model{}}
	g.env = setup(t, nil)
	ctx := t.Context()
	ev := evidence.New(g.db, g.clock)
	g.plans = plans.New(g.db, g.brands, plans.Config{Clock: g.clock, Model: g.model, Evidence: ev, MaxGenerationsPerDay: 3})
	brief, err := g.db.Writer().CreateResearchBrief(ctx, store.CreateResearchBriefParams{WorkspaceID: 1, BrandID: g.reportage, ProjectID: optional(g.wadi), Title: "Q4", Geography: "AE", Language: "en", CreatedAt: g.clock.Now()})
	require.NoError(t, err)
	g.brief = brief.ID
	item, err := g.db.Writer().CreateResearchItem(ctx, store.CreateResearchItemParams{WorkspaceID: 1, BriefID: brief.ID, Kind: "keywords", Target: "villas dubai", IdentityState: "n/a", Candidates: "[]", State: "fetched", CreatedAt: g.clock.Now()})
	require.NoError(t, err)
	f, err := g.db.Writer().CreateResearchFinding(ctx, store.CreateResearchFindingParams{WorkspaceID: 1, BriefID: brief.ID, ItemID: item.ID, Scope: "public", Kind: "keyword", Class: "estimated", Label: "villas dubai",
		Payload: `{"idea":{"avgMonthlySearches":12100,"lowTopOfPageBidMinor":245}}`, SourceObservedAt: g.clock.Now(), Geography: "AE", CreatedAt: g.clock.Now()})
	require.NoError(t, err)
	g.finding = f.ID
	fact, err := g.brands.AddFact(ctx, coretest.Marketer, g.reportage, brands.FactInput{ProjectID: g.wadi, Key: "price.from", Kind: brands.KindMoney, Value: "120000000", Currency: "AED", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceDocument, SourceRef: "price list"})
	require.NoError(t, err)
	_, err = g.brands.ApproveFact(ctx, coretest.Admin, fact.Fact.ID)
	require.NoError(t, err)
	g.fact = fact.Fact.ID
	offer, err := g.brands.CreateOffer(ctx, coretest.Marketer, g.reportage, brands.OfferInput{ProjectID: g.wadi, Name: "Launch", Document: brands.OfferDocument{Headline: "Up to 15% off and more"}, EffectiveFrom: "2026-10-01"})
	require.NoError(t, err)
	g.offer = offer.ID
	frame := plans.PlanDocument{Objective: "Leads for The Wadi", BudgetMinor: 10_000_000, Currency: "AED", PeriodFrom: "2026-11-01", PeriodTo: "2026-11-30"}
	d, err := g.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: g.reportage, ProjectID: g.wadi, Title: "Wadi Nov", Document: frame})
	require.NoError(t, err)
	g.plan = d.Plan.ID
	return g
}

func optional(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

// proposal is what a well-behaved model would send.
func proposal(g *genEnv) map[string]any {
	return map[string]any{
		"objective": "Qualified leads for The Wadi at a sustainable cost",
		"allocations": []map[string]any{
			{"channel": "google_search", "objective": "leads", "budgetMinor": 6_000_000, "geography": "AE", "audience": "high-intent buyers", "expectedCplMinor": 30000},
			{"channel": "meta_lead_form", "objective": "leads", "budgetMinor": 4_000_000, "geography": "AE", "audience": "investors 30-55"},
		},
		"assetRequirements":       []map[string]any{{"format": "9:16", "language": "ar", "count": 3, "offerId": g.offer}},
		"pageRequirements":        []map[string]any{{"kind": "landing", "language": "en", "url": "https://reportageoffer.com/wadi"}},
		"measurementRequirements": []map[string]any{{"kind": "conversion_action", "platform": "google_ads", "name": "Lead form submit", "required": true}},
		"testHypotheses":          []map[string]any{{"hypothesis": "Arabic creative lowers CPL", "metric": "cost_per_lead", "windowDays": 14}},
		"evidenceRefs": []map[string]any{
			{"kind": "research_finding", "id": g.finding, "note": "search demand"},
			{"kind": "fact", "id": g.fact, "note": "starting price"},
			{"kind": "offer", "id": g.offer},
			{"kind": "fact", "id": 999, "note": "made up"},
		},
		"assumptions": []map[string]any{
			{"key": "cpc.google_search", "text": "Top-of-page bid AED 2.45 from Keyword Planner", "class": "cited", "evidenceRef": 0},
			{"key": "price", "text": "from AED 1.2M", "class": "cited", "evidenceRef": 3},
		},
		"notes": "Budget split 60/40 toward search.",
	}
}

// Catches: the model moving the budget, citing evidence outside the
// context, typing a cost per lead with no source, a dangling citation
// surviving, and a generated version being accepted by being generated.
func TestGeneratedPlanIsConstrainedAndValidated(t *testing.T) {
	g := setupGen(t)
	ctx := t.Context()
	p := proposal(g)
	p["budgetMinor"] = 50_000_000
	g.model.Responses = []assistant.Response{assistanttest.ToolCall("t1", "propose_plan", p)}
	gen, err := g.plans.Generate(ctx, coretest.Marketer, g.plan, plans.GenerateInput{BriefIDs: []int64{g.brief}, Instructions: "Lean on search."})
	require.NoError(t, err)
	v := gen.Version
	assert.Equal(t, int64(2), v.Version)
	assert.Equal(t, "generated", v.Source)
	assert.Equal(t, plans.StateReady, v.State, "%+v", v.Validation.Blocking)
	assert.Nil(t, v.Acceptance)
	assert.Equal(t, int64(10_000_000), v.Document.BudgetMinor, "the frame is pinned")
	assert.Equal(t, "2026-11-01", v.Document.PeriodFrom)
	assert.Len(t, v.Document.EvidenceRefs, 3, "the made-up fact was dropped")
	assert.Nil(t, v.Document.Allocations[0].ExpectedCPLMinor, "a typed CPL with no source is unavailable")
	assert.Equal(t, g.wadi, *v.Document.Allocations[0].ProjectID)
	noteCodes := []string{}
	for _, n := range gen.Notes {
		noteCodes = append(noteCodes, n.Code)
	}
	assert.Subset(t, noteCodes, []string{"budget_pinned", "evidence_dropped", "cpl_unavailable", "citation_removed"})
	classes := map[string]string{}
	for _, a := range v.Document.Assumptions {
		classes[a.Key] = a.Class
	}
	assert.Equal(t, "cited", classes["cpc.google_search"])
	assert.Equal(t, "unavailable", classes["price"], "its reference pointed at the dropped ref")
	assert.Equal(t, "unavailable", classes["cpl.google_search"])

	// The request the model saw: frame, context, forced tool, no budget field in the schema.
	require.Len(t, g.model.Requests, 1)
	req := g.model.Requests[0]
	assert.Equal(t, "propose_plan", req.ToolChoice)
	props := req.Tools[0].InputSchema["properties"].(map[string]any)
	assert.NotContains(t, props, "budgetMinor")
	assert.Contains(t, req.Messages[0].Content[0].Text, `"budgetMinor":10000000`)
	assert.Contains(t, req.Messages[0].Content[0].Text, "villas dubai")
	assert.Contains(t, req.Messages[0].Content[0].Text, "Lean on search.")
	assert.NotContains(t, req.Messages[0].Content[0].Text, "tiktok")

	// Version 1 is untouched and the generation was recorded as evidence.
	detail, err := g.plans.Get(ctx, coretest.Viewer, g.plan)
	require.NoError(t, err)
	assert.Equal(t, "Leads for The Wadi", detail.Versions[0].Document.Objective)
	n, err := g.db.Reader().CountEvidence(ctx, store.CountEvidenceParams{WorkspaceID: 1, Source: "adx", Kind: "plan_generation"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}

// Catches: a channel ADX can't deploy being saved, an unknown channel being
// saved, prose instead of a document being saved, a model outage being
// reported as a plan, and a brief of another brand feeding the context.
func TestModelOutputThatBreaksTheRulesIsRejected(t *testing.T) {
	g := setupGen(t)
	ctx := t.Context()
	bad := proposal(g)
	bad["allocations"] = []map[string]any{{"channel": "tiktok", "objective": "leads", "budgetMinor": 10_000_000}}
	g.model.Responses = []assistant.Response{assistanttest.ToolCall("t1", "propose_plan", bad)}
	_, err := g.plans.Generate(ctx, coretest.Marketer, g.plan, plans.GenerateInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot deploy")
	unknown := proposal(g)
	unknown["allocations"] = []map[string]any{{"channel": "carrier_pigeon", "objective": "leads", "budgetMinor": 10_000_000}}
	g.model.Responses = []assistant.Response{assistanttest.ToolCall("t2", "propose_plan", unknown)}
	_, err = g.plans.Generate(ctx, coretest.Marketer, g.plan, plans.GenerateInput{})
	require.Error(t, err)
	g.model.Responses = []assistant.Response{assistanttest.Text("Here is my plan: spend it all on search.")}
	_, err = g.plans.Generate(ctx, coretest.Marketer, g.plan, plans.GenerateInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prose")
	detail, err := g.plans.Get(ctx, coretest.Viewer, g.plan)
	require.NoError(t, err)
	assert.Len(t, detail.Versions, 1, "nothing was saved")

	// The daily cap: three calls were made; the fourth is refused before the model.
	g.model.Responses = []assistant.Response{assistanttest.ToolCall("t3", "propose_plan", proposal(g))}
	_, err = g.plans.Generate(ctx, coretest.Marketer, g.plan, plans.GenerateInput{})
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.Len(t, g.model.Requests, 3)

	// An outage is unavailable, not a plan.
	g2 := setupGen(t)
	g2.model.Errors = []error{&assistant.ModelError{Retryable: true, Safe: "The model is busy; try again in a minute."}}
	_, err = g2.plans.Generate(ctx, coretest.Marketer, g2.plan, plans.GenerateInput{})
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindUnavailable, ce.Kind)
	assert.Contains(t, ce.Message, "busy")

	// A brief about another brand is refused before any model call.
	other, err := g2.db.Writer().CreateResearchBrief(ctx, store.CreateResearchBriefParams{WorkspaceID: 1, BrandID: g2.rival, Title: "Rival", Geography: "AE", Language: "en", CreatedAt: g2.clock.Now()})
	require.NoError(t, err)
	_, err = g2.plans.Generate(ctx, coretest.Marketer, g2.plan, plans.GenerateInput{BriefIDs: []int64{other.ID}})
	require.Error(t, err)
	assert.Len(t, g2.model.Requests, 1, "no second model call")

	// No model configured: a stated conflict.
	bare := plans.New(g2.db, g2.brands, plans.Config{Clock: g2.clock})
	_, err = bare.Generate(ctx, coretest.Marketer, g2.plan, plans.GenerateInput{})
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
}

// Catches: a diff missing a changed allocation, an accepted version not
// being flagged as superseded in content, and an offer change not naming
// the plans that cite it.
func TestDiffAndRevisionImpact(t *testing.T) {
	e := setup(t, nil)
	ctx := t.Context()
	offer, err := e.brands.CreateOffer(ctx, coretest.Marketer, e.reportage, brands.OfferInput{ProjectID: e.wadi, Name: "Launch", Document: brands.OfferDocument{Headline: "10% off"}, EffectiveFrom: "2026-10-01"})
	require.NoError(t, err)
	doc := good()
	doc.EvidenceRefs = []plans.PlanEvidenceRef{{Kind: "offer", ID: offer.ID, Note: "launch offer"}}
	d, err := e.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e.reportage, ProjectID: e.wadi, Title: "Wadi", Document: doc})
	require.NoError(t, err)
	_, err = e.plans.Accept(ctx, coretest.Admin, d.Plan.ID, 1)
	require.NoError(t, err)

	changed := doc
	changed.Allocations = []plans.Allocation{
		{Channel: "google_search", Objective: "leads", BudgetMinor: 5_000_000, Geography: "AE"},
		{Channel: "google_pmax", Objective: "leads", BudgetMinor: 5_000_000, Geography: "AE"},
	}
	changed.Objective = "Leads at lower CPL"
	changed.EvidenceRefs = nil
	v2, err := e.plans.SaveVersion(ctx, coretest.Marketer, d.Plan.ID, plans.VersionInput{Document: changed})
	require.NoError(t, err)

	diff, err := e.plans.Diff(ctx, coretest.Viewer, d.Plan.ID, 1, 2)
	require.NoError(t, err)
	fields := map[string]plans.DiffChange{}
	for _, f := range diff.Fields {
		fields[f.Path] = f
	}
	assert.Equal(t, "Leads at lower CPL", fields["objective"].After)
	byKey := map[string]string{}
	for _, a := range diff.Allocations {
		byKey[a.Channel] = a.Change
	}
	assert.Equal(t, map[string]string{"google_search": "changed", "meta_lead_form": "removed", "google_pmax": "added"}, byKey)
	require.NotNil(t, diff.Invalidates)
	assert.Equal(t, int64(1), diff.Invalidates.AcceptedVersion)
	raw, _ := json.Marshal(diff.Sections)
	assert.Contains(t, string(raw), `"evidenceRefs"`)

	same, err := e.plans.Diff(ctx, coretest.Viewer, d.Plan.ID, 2, 2)
	require.NoError(t, err)
	assert.Empty(t, same.Fields)
	assert.Nil(t, same.Invalidates)

	// The offer is cited by the accepted version 1 but not by the latest.
	affected, err := e.plans.PlansUsingOffer(ctx, coretest.Viewer, offer.ID)
	require.NoError(t, err)
	require.Len(t, affected, 1)
	assert.Equal(t, int64(1), affected[0].Version)
	assert.Equal(t, plans.StateAccepted, affected[0].State)
	_ = v2
}
