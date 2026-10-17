package plans_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	brands    *brands.Brands
	plans     *plans.Plans
	reportage int64
	rival     int64
	wadi      int64
	rivalProj int64
}

func setup(t *testing.T, exact plans.TemplateAdapter) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	e.brands = brands.New(e.db, brands.Config{Clock: e.clock})
	e.plans = plans.New(e.db, e.brands, plans.Config{Clock: e.clock, Exact: exact})
	r, err := e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	e.reportage = r.ID
	c, err := e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Rival", Kind: brands.Competitor})
	require.NoError(t, err)
	e.rival = c.ID
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.wadi = p.ID
	require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, e.wadi, e.reportage))
	rp, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "Rival Towers", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.rivalProj = rp.ID
	require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, e.rivalProj, e.rival))
	return e
}

func good() plans.PlanDocument {
	cpl := int64(25000)
	return plans.PlanDocument{
		Objective: "Qualified leads for The Wadi launch", BudgetMinor: 10_000_000, Currency: "AED", PeriodFrom: "2026-11-01", PeriodTo: "2026-11-30",
		Allocations: []plans.Allocation{
			{Channel: "google_search", Objective: "leads", BudgetMinor: 6_000_000, Geography: "AE", ExpectedCPLMinor: &cpl},
			{Channel: "meta_lead_form", Objective: "leads", BudgetMinor: 4_000_000, Geography: "AE", ExpectedCPLMinor: &cpl},
		},
		AssetRequirements:       []plans.AssetRequirement{{Format: "9:16", Language: "en", Count: 3}, {Format: "1:1", Language: "ar", Count: 2}},
		PageRequirements:        []plans.PageRequirement{{Kind: "landing", Language: "en", URL: "https://reportageoffer.com/wadi"}},
		MeasurementRequirements: []plans.MeasurementRequirement{{Kind: "conversion_action", Platform: "google_ads", Name: "Lead form submit", Required: true}},
		TestHypotheses:          []plans.TestHypothesis{{Hypothesis: "Arabic creative lowers CPL", Metric: "cost_per_lead", WindowDays: 14}},
		Assumptions:             []plans.Assumption{{Key: "cpl", Text: "AED 250 per lead from last quarter", Class: "assumption"}},
	}
}

func codes(issues []plans.ValidationIssue) []string {
	out := []string{}
	for _, i := range issues {
		out = append(out, i.Code)
	}
	return out
}

// Catches: allocations that don't add up being accepted, the allocated sum
// not being reported, and a mismatched plan reaching ready.
func TestBudgetRowsMustTotalTheBudget(t *testing.T) {
	e := setup(t, nil)
	ctx := t.Context()
	d, err := e.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e.reportage, ProjectID: e.wadi, Title: "Wadi November", Document: good()})
	require.NoError(t, err)
	v1 := d.Versions[0]
	assert.Equal(t, plans.StateReady, v1.State)
	assert.Empty(t, v1.Validation.Blocking)
	assert.Equal(t, int64(10_000_000), v1.Validation.AllocatedMinor)
	assert.Nil(t, v1.ExecutionAuthorization)

	off := good()
	off.Allocations[1].BudgetMinor = 3_500_000
	v2, err := e.plans.SaveVersion(ctx, coretest.Marketer, d.Plan.ID, plans.VersionInput{Document: off, Note: "trimmed Meta"})
	require.NoError(t, err)
	assert.Equal(t, plans.StateInvalid, v2.State)
	assert.Equal(t, []string{"budget_mismatch"}, codes(v2.Validation.Blocking))
	assert.Contains(t, v2.Validation.Blocking[0].Message, "AED 95000.00")
	assert.Equal(t, int64(9_500_000), v2.Validation.AllocatedMinor)
	_, err = e.plans.Accept(ctx, coretest.Admin, d.Plan.ID, 2)
	require.Error(t, err, "an invalid version can't be accepted")
}

// Catches: a marketer accepting, a needs_input version being accepted, an
// accepted version being edited in place, acceptance carrying to the next
// version, and acceptance implying spend authority.
func TestAcceptedVersionIsImmutableAndGrantsNoSpend(t *testing.T) {
	e := setup(t, nil)
	ctx := t.Context()
	d, err := e.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e.reportage, Title: "Q4", Document: good()})
	require.NoError(t, err)
	_, err = e.plans.Accept(ctx, coretest.Marketer, d.Plan.ID, 1)
	assert.ErrorIs(t, err, core.ErrForbidden)
	accepted, err := e.plans.Accept(ctx, coretest.Admin, d.Plan.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, plans.StateAccepted, accepted.State)
	require.NotNil(t, accepted.Acceptance)
	assert.Equal(t, int64(1), accepted.Acceptance.AcceptedBy)
	assert.False(t, accepted.Acceptance.GrantsSpend())
	assert.Nil(t, accepted.ExecutionAuthorization)
	again, err := e.plans.Accept(ctx, coretest.Admin, d.Plan.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, accepted.Acceptance.AcceptedAt, again.Acceptance.AcceptedAt, "accepting twice is a no-op")

	// A change is a new version; version 1 stays as accepted.
	changed := good()
	changed.Objective = "Qualified leads and site visits"
	v2, err := e.plans.SaveVersion(ctx, coretest.Marketer, d.Plan.ID, plans.VersionInput{Document: changed})
	require.NoError(t, err)
	assert.Equal(t, int64(2), v2.Version)
	assert.Equal(t, plans.StateReady, v2.State)
	assert.Nil(t, v2.Acceptance)
	detail, err := e.plans.Get(ctx, coretest.Viewer, d.Plan.ID)
	require.NoError(t, err)
	assert.Equal(t, plans.StateAccepted, detail.Versions[0].State)
	assert.Equal(t, "Qualified leads for The Wadi launch", detail.Versions[0].Document.Objective)
	list, err := e.plans.List(ctx, coretest.Viewer)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.True(t, list[0].Accepted)
	assert.Equal(t, int64(2), list[0].LatestVersion)

	incomplete := good()
	incomplete.BudgetMinor = 0
	incomplete.Allocations = nil
	v3, err := e.plans.SaveVersion(ctx, coretest.Marketer, d.Plan.ID, plans.VersionInput{Document: incomplete})
	require.NoError(t, err)
	assert.Equal(t, plans.StateNeedsInput, v3.State)
	_, err = e.plans.Accept(ctx, coretest.Admin, d.Plan.ID, 3)
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
}

type fakeExact struct{}

func (fakeExact) Name() string { return "xlsx" }
func (fakeExact) ContentType() string {
	return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}
func (fakeExact) Export(v plans.PlanVersion) ([]byte, error) {
	return []byte("xlsx:" + v.Document.Objective), nil
}
func (fakeExact) Import([]byte) (plans.PlanDocument, error) { return good(), nil }

// Catches: a generic export losing meaning on the way back, the exact
// template being reported available without a file, and the blocked export
// surfacing as a server error instead of a stated gate.
func TestGenericRoundTripAndExactTemplateGate(t *testing.T) {
	e := setup(t, nil)
	ctx := t.Context()
	d, err := e.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e.reportage, ProjectID: e.wadi, Title: "Round trip", Document: good()})
	require.NoError(t, err)
	gate := e.plans.Templates()
	assert.Equal(t, []string{"json", "csv"}, gate.Generic)
	assert.False(t, gate.Exact.Available)
	assert.Contains(t, gate.Exact.Reason, "has not been supplied")
	assert.Contains(t, gate.Exact.Unblocks, ".xlsx")

	data, ct, err := e.plans.Export(ctx, coretest.Viewer, d.Plan.ID, 1, "json")
	require.NoError(t, err)
	assert.Equal(t, "application/json", ct)
	v2, err := e.plans.Import(ctx, coretest.Marketer, d.Plan.ID, "json", data, "re-imported")
	require.NoError(t, err)
	assert.Equal(t, "imported", v2.Source)
	assert.Equal(t, d.Versions[0].Document, v2.Document, "JSON carries the whole document")
	assert.Equal(t, plans.StateReady, v2.State)

	csvData, ct, err := e.plans.Export(ctx, coretest.Viewer, d.Plan.ID, 1, "csv")
	require.NoError(t, err)
	assert.Contains(t, ct, "text/csv")
	assert.Contains(t, string(csvData), "google_search,leads,6000000,AED,AE")
	v3, err := e.plans.Import(ctx, coretest.Marketer, d.Plan.ID, "csv", csvData, "")
	require.NoError(t, err)
	assert.Equal(t, d.Versions[0].Document.Allocations, v3.Document.Allocations, "CSV carries the allocation table")
	assert.Equal(t, d.Versions[0].Document.BudgetMinor, v3.Document.BudgetMinor)
	assert.Empty(t, v3.Document.AssetRequirements, "CSV does not carry requirements; that is stated, not hidden")

	_, _, err = e.plans.Export(ctx, coretest.Viewer, d.Plan.ID, 1, "xlsx")
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.Contains(t, ce.Message, "has not been supplied")
	_, err = e.plans.Import(ctx, coretest.Marketer, d.Plan.ID, "xlsx", []byte("x"), "")
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
	_, _, err = e.plans.Export(ctx, coretest.Viewer, d.Plan.ID, 1, "pdf")
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindInvalid, ce.Kind)

	// Once an adapter exists for the real template, the gate opens.
	e2 := setup(t, fakeExact{})
	gate = e2.plans.Templates()
	assert.True(t, gate.Exact.Available)
	d2, err := e2.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e2.reportage, Title: "x", Document: good()})
	require.NoError(t, err)
	out, _, err := e2.plans.Export(ctx, coretest.Viewer, d2.Plan.ID, 1, "xlsx")
	require.NoError(t, err)
	assert.Equal(t, "xlsx:Qualified leads for The Wadi launch", string(out))
}

// Catches: a missing input being called invalid (or the reverse), an
// unknown channel passing, an unsupported channel blocking, evidence that
// doesn't exist passing, a dangling citation passing, a foreign project
// passing, and a missing CPL being silently fine.
func TestValidationStatesAndCodes(t *testing.T) {
	e := setup(t, nil)
	ctx := t.Context()
	fact, err := e.brands.AddFact(ctx, coretest.Marketer, e.reportage, brands.FactInput{Key: "price.from", Kind: brands.KindMoney, Value: "120000000", Currency: "AED", EffectiveFrom: "2026-10-01", SourceKind: brands.SourceManual})
	require.NoError(t, err)

	empty := plans.PlanDocument{}
	v := e.plans.Validate(ctx, coretest.Marketer, e.reportage, empty)
	assert.Equal(t, plans.StateNeedsInput, stateOf(t, e, empty))
	assert.Subset(t, codes(v.Blocking), []string{"objective_missing", "budget_missing", "currency_missing", "period_missing", "no_allocations"})

	bad := good()
	bad.Allocations[0].Channel = "google_search"
	bad.Allocations[1].Channel = "carrier_pigeon"
	assert.Equal(t, plans.StateInvalid, stateOf(t, e, bad))
	assert.Contains(t, codes(e.plans.Validate(ctx, coretest.Marketer, e.reportage, bad).Blocking), "channel_unknown")

	manual := good()
	manual.Allocations[1].Channel = "tiktok"
	v = e.plans.Validate(ctx, coretest.Marketer, e.reportage, manual)
	assert.Empty(t, v.Blocking)
	assert.Contains(t, codes(v.Warnings), "channel_unsupported")
	assert.Equal(t, plans.StateReady, stateOf(t, e, manual))

	cited := good()
	cited.EvidenceRefs = []plans.PlanEvidenceRef{{Kind: "fact", ID: fact.Fact.ID, Note: "starting price"}}
	zero := 0
	cited.Assumptions = []plans.Assumption{{Key: "price", Text: "from AED 1.2M", Class: "cited", EvidenceRef: &zero}}
	v = e.plans.Validate(ctx, coretest.Marketer, e.reportage, cited)
	assert.Empty(t, v.Blocking)
	assert.NotContains(t, codes(v.Warnings), "no_evidence")

	missing := good()
	missing.EvidenceRefs = []plans.PlanEvidenceRef{{Kind: "fact", ID: 999}, {Kind: "offer", ID: 5}, {Kind: "research_finding", ID: 7}}
	v = e.plans.Validate(ctx, coretest.Marketer, e.reportage, missing)
	assert.Len(t, v.Blocking, 3)
	assert.Equal(t, plans.StateInvalid, stateOf(t, e, missing))

	dangling := good()
	one := 1
	dangling.Assumptions = []plans.Assumption{{Key: "x", Text: "y", Class: "cited", EvidenceRef: &one}}
	assert.Contains(t, codes(e.plans.Validate(ctx, coretest.Marketer, e.reportage, dangling).Blocking), "citation_dangling")

	foreign := good()
	foreign.Allocations[0].ProjectID = &e.rivalProj
	assert.Contains(t, codes(e.plans.Validate(ctx, coretest.Marketer, e.reportage, foreign).Blocking), "project_foreign")

	noCPL := good()
	noCPL.Allocations[0].ExpectedCPLMinor = nil
	v = e.plans.Validate(ctx, coretest.Marketer, e.reportage, noCPL)
	assert.Empty(t, v.Blocking)
	assert.Contains(t, codes(v.Warnings), "cpl_unavailable")

	_, err = e.plans.Create(ctx, coretest.Viewer, plans.PlanInput{BrandID: e.reportage, Title: "x", Document: good()})
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e.reportage, ProjectID: e.rivalProj, Title: "x", Document: good()})
	require.Error(t, err, "a plan for a project of another brand")
}

func stateOf(t *testing.T, e *env, d plans.PlanDocument) plans.PlanState {
	t.Helper()
	return plans.StateFor(e.plans.Validate(t.Context(), coretest.Marketer, e.reportage, d))
}
