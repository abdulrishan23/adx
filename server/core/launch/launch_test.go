package launch_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/launch"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// The rules the owner decided on 2026-10-18 (BD-60). Each test names the
// wrong behaviour it catches.

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	plans     *plans.Plans
	brands    *brands.Brands
	launch    *launch.Service
	reportage int64
	wadi      int64
	verdana   int64
	googleAcc int64
	metaAcc   int64
}

// 12 October 2026 09:00 Dubai (05:00 UTC): a 31-day month with 20 days left.
var today = time.Date(2026, 10, 12, 5, 0, 0, 0, time.UTC)

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(today)}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	e.brands = brands.New(e.db, brands.Config{Clock: e.clock})
	e.plans = plans.New(e.db, e.brands, plans.Config{Clock: e.clock})
	rep := reporting.New(e.db, nil, nil, evidence.New(e.db, e.clock), reporting.Config{Clock: e.clock})
	e.launch = launch.New(e.db, e.plans, rep, launch.Config{Clock: e.clock})
	b, err := e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	e.reportage = b.ID
	for _, name := range []string{"The Wadi", "Verdana"} {
		p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: name, CreatedAt: e.clock.Now()})
		require.NoError(t, err)
		require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, p.ID, e.reportage))
		if name == "The Wadi" {
			e.wadi = p.ID
		} else {
			e.verdana = p.ID
		}
	}
	e.googleAcc = e.account(t, "google_ads", "1234567890")
	e.metaAcc = e.account(t, "meta", "act_42")
	return e
}

func (e *env) account(t *testing.T, typ, external string) int64 {
	t.Helper()
	cred, err := e.db.Writer().UpsertCredential(t.Context(), store.UpsertCredentialParams{Provider: typ, Kind: "oauth", Subject: "u-" + typ, EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "x", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(t.Context(), store.UpsertAccountParams{WorkspaceID: core.DefaultWorkspace, Type: typ, ExternalID: external, CredentialID: cred.ID, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	return acct.ID
}

// campaign seeds a platform campaign with its type and status, assigned to
// a project, with one day of spend this month when spent is not nil.
func (e *env) campaign(t *testing.T, acct int64, platform, ext, name, typ, status string, project int64, spent *int64) store.AdCampaign {
	t.Helper()
	ctx := t.Context()
	now := e.clock.Now()
	c, err := e.db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: acct, Platform: platform, ExternalID: ext, Name: name, Status: status,
		ChannelType: sql.NullString{String: typ, Valid: true}, Currency: sql.NullString{String: "AED", Valid: true}, FirstSeenAt: now, LastSeenAt: now})
	require.NoError(t, err)
	require.NoError(t, e.db.Writer().SetAdCampaignProject(ctx, store.SetAdCampaignProjectParams{ProjectID: sql.NullInt64{Int64: project, Valid: true}, ID: c.ID}))
	if spent != nil {
		e.metric(t, c, "2026-10-05", *spent)
	}
	return c
}

func (e *env) metric(t *testing.T, c store.AdCampaign, day string, cost int64) {
	t.Helper()
	require.NoError(t, e.db.Writer().UpsertCampaignDailyMetrics(t.Context(), store.UpsertCampaignDailyMetricsParams{WorkspaceID: 1, AccountID: c.AccountID, CampaignID: c.ID, Platform: c.Platform, Day: day, Currency: "AED", CostMinor: cost, Impressions: 10, Clicks: 1, ObservedAt: e.clock.Now()}))
}

// plan creates and accepts a one-version plan for The Wadi with the given
// allocations over October 2026, returning its id.
func (e *env) plan(t *testing.T, allocations ...plans.Allocation) int64 {
	t.Helper()
	return e.planFor(t, e.wadi, "2026-10-01", "2026-10-31", allocations...)
}

func (e *env) planFor(t *testing.T, project int64, from, to string, allocations ...plans.Allocation) int64 {
	t.Helper()
	var total int64
	for _, a := range allocations {
		total += a.BudgetMinor
	}
	d, err := e.plans.Create(t.Context(), coretest.Marketer, plans.PlanInput{BrandID: e.reportage, ProjectID: project, Title: "Wadi Oct", Document: plans.PlanDocument{Objective: "Leads", BudgetMinor: total, Currency: "AED", PeriodFrom: from, PeriodTo: to, Allocations: allocations}})
	require.NoError(t, err)
	require.Equal(t, plans.StateReady, d.Versions[0].State, "%+v", d.Versions[0].Validation)
	_, err = e.plans.Accept(t.Context(), coretest.Admin, d.Plan.ID, 1)
	require.NoError(t, err)
	return d.Plan.ID
}

func alloc(channel string, budget int64) plans.Allocation {
	cpl := int64(25000)
	return plans.Allocation{Channel: channel, Objective: "leads", BudgetMinor: budget, Geography: "AE", ExpectedCPLMinor: &cpl}
}

func i64(v int64) *int64 { return &v }

// Catches: ADX updating both matches without asking, and a pick slipping
// in from a viewer or from the assistant.
func TestTwoMatchesNeedAPersonsPick(t *testing.T) {
	e := setup(t)
	e.campaign(t, e.metaAcc, "meta", "m1", "Wadi | Leads | AR", "OUTCOME_LEADS", "ACTIVE", e.wadi, i64(50_000_00))
	e.campaign(t, e.metaAcc, "meta", "m2", "Wadi | Leads | EN", "OUTCOME_LEADS", "ACTIVE", e.wadi, i64(30_000_00))
	plan := e.plan(t, alloc("meta_lead_form", 200_000_00))

	res, err := e.launch.Resolve(t.Context(), coretest.Viewer, plan, 1)
	require.NoError(t, err)
	require.Len(t, res.Lines, 1)
	l := res.Lines[0]
	assert.Equal(t, launch.KindPick, l.Kind)
	assert.Nil(t, l.Campaign)
	assert.Nil(t, l.Budget, "no budget is computed until a person picks")
	require.Len(t, l.Candidates, 2)
	assert.Len(t, res.Unmatched, 2, "neither is under the plan until picked")

	_, err = e.launch.Pick(t.Context(), coretest.Viewer, plan, 1, 0, l.Candidates[0].ID)
	assert.ErrorIs(t, err, core.ErrForbidden)
	bot := coretest.Marketer
	bot.Via = core.ViaAssistant
	_, err = e.launch.Pick(t.Context(), bot, plan, 1, 0, l.Candidates[0].ID)
	assert.ErrorIs(t, err, core.ErrForbidden)
}

// Catches: the spend to date being ignored, a campaign of another project
// counting as a match, and the daily figure being the month's total.
func TestOneMatchUpdatesAndNetsThisMonthsSpend(t *testing.T) {
	e := setup(t)
	g := e.campaign(t, e.googleAcc, "google_ads", "g1", "Search | Wadi", "SEARCH", "ENABLED", e.wadi, i64(40_000_00))
	e.campaign(t, e.googleAcc, "google_ads", "v1", "Search | Verdana", "SEARCH", "ENABLED", e.verdana, i64(90_000_00))
	plan := e.plan(t, alloc("google_search", 152_000_00))

	res, err := e.launch.Resolve(t.Context(), coretest.Viewer, plan, 1)
	require.NoError(t, err)
	assert.Equal(t, reporting.Day("2026-10-12"), res.Today)
	assert.Equal(t, reporting.Day("2026-10-31"), res.MonthEnd)
	assert.Equal(t, 20, res.DaysLeft)
	l := res.Lines[0]
	assert.Equal(t, launch.KindUpdate, l.Kind)
	require.NotNil(t, l.Campaign)
	assert.Equal(t, g.ID, l.Campaign.ID)
	assert.Equal(t, "SEARCH", l.Line.CampaignType)
	assert.Equal(t, "google_ads", l.Line.Platform)
	assert.Equal(t, launch.MonthBudget{SpentMinor: 40_000_00, SpendKnown: true, RemainingMinor: 112_000_00, DailyMinor: 5_600_00, DaysLeft: 20}, *l.Budget)
	assert.Empty(t, res.Unmatched)
}

// Catches: a line matched to a running campaign of another type, and a
// created campaign getting the whole month per day.
func TestTypeMustMatchAndCreateSpreadsOverDaysLeft(t *testing.T) {
	e := setup(t)
	e.campaign(t, e.googleAcc, "google_ads", "g1", "Search | Wadi", "SEARCH", "ENABLED", e.wadi, i64(40_000_00))
	plan := e.plan(t, alloc("google_pmax", 20_000_00), alloc("tiktok", 5_000_00))

	res, err := e.launch.Resolve(t.Context(), coretest.Viewer, plan, 1)
	require.NoError(t, err)
	require.Len(t, res.Lines, 2)
	assert.Equal(t, launch.KindCreate, res.Lines[0].Kind)
	assert.Equal(t, launch.MonthBudget{SpentMinor: 0, SpendKnown: true, RemainingMinor: 20_000_00, DailyMinor: 1_000_00, DaysLeft: 20}, *res.Lines[0].Budget)
	assert.Equal(t, launch.KindUnsupported, res.Lines[1].Kind, "TikTok cannot be deployed, so the line is not launched")
	require.Len(t, res.Unmatched, 1, "the Search campaign is under no line of this plan")
	assert.Equal(t, "g1", res.Unmatched[0].ExternalID)
}

// Catches: a negative budget when the campaign has spent past the plan,
// last month's spend netting off this month's line, and a campaign with no
// figures this month being treated as having spent zero without saying so.
func TestNettingNeverGoesBelowZeroAndCountsOnlyThisMonth(t *testing.T) {
	e := setup(t)
	g := e.campaign(t, e.googleAcc, "google_ads", "g1", "Search | Wadi", "SEARCH", "ENABLED", e.wadi, i64(45_000_00))
	e.metric(t, g, "2026-09-30", 500_000_00)
	e.campaign(t, e.metaAcc, "meta", "m1", "Wadi | Leads", "OUTCOME_LEADS", "ACTIVE", e.wadi, nil)
	plan := e.plan(t, alloc("google_search", 30_000_00), alloc("meta_lead_form", 40_000_00))

	res, err := e.launch.Resolve(t.Context(), coretest.Viewer, plan, 1)
	require.NoError(t, err)
	assert.Equal(t, launch.MonthBudget{SpentMinor: 45_000_00, SpendKnown: true, RemainingMinor: 0, DailyMinor: 0, DaysLeft: 20}, *res.Lines[0].Budget)
	m := res.Lines[1]
	assert.Equal(t, launch.KindUpdate, m.Kind)
	assert.False(t, m.Budget.SpendKnown, "no figures read this month is not zero")
	assert.Equal(t, int64(40_000_00), m.Budget.RemainingMinor)
	assert.Equal(t, int64(2_000_00), m.Budget.DailyMinor)
}

// Catches: the month being taken in UTC. At 21:00 UTC on 31 October it is
// already 1 November in Dubai: November's days are left and October's spend
// is last month's.
func TestMonthIsTheDubaiMonth(t *testing.T) {
	e := setup(t)
	g := e.campaign(t, e.googleAcc, "google_ads", "g1", "Search | Wadi", "SEARCH", "ENABLED", e.wadi, nil)
	e.metric(t, g, "2026-10-31", 10_000_00)
	e.metric(t, g, "2026-11-01", 1_000_00)
	plan := e.planFor(t, e.wadi, "2026-11-01", "2026-11-30", alloc("google_search", 31_000_00))
	e.clock.Set(time.Date(2026, 10, 31, 21, 0, 0, 0, time.UTC))

	res, err := e.launch.Resolve(t.Context(), coretest.Viewer, plan, 1)
	require.NoError(t, err)
	assert.Equal(t, reporting.Day("2026-11-01"), res.Today)
	assert.Equal(t, reporting.Day("2026-11-30"), res.MonthEnd)
	assert.Equal(t, 30, res.DaysLeft)
	assert.Equal(t, launch.MonthBudget{SpentMinor: 1_000_00, SpendKnown: true, RemainingMinor: 30_000_00, DailyMinor: 1_000_00, DaysLeft: 30}, *res.Lines[0].Budget)
}

// Catches: a second campaign of the type being created beside a paused one
// (the owner, 2026-10-19: the launch resumes it; accepting the plan
// authorised its spend), a paused campaign resumed while a running one of
// the type exists, and two paused ones resumed without a person choosing.
func TestPausedOnlyMatchIsResumedNotDuplicated(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	m1 := e.campaign(t, e.metaAcc, "meta", "m1", "Wadi | Leads", "OUTCOME_LEADS", "CAMPAIGN_PAUSED", e.wadi, i64(5_000_00))
	e.campaign(t, e.googleAcc, "google_ads", "g1", "Search | Wadi", "SEARCH", "ENABLED", e.wadi, nil)
	e.campaign(t, e.googleAcc, "google_ads", "g0", "Search | Wadi old", "SEARCH", "PAUSED", e.wadi, nil)
	plan := e.plan(t, alloc("meta_lead_form", 40_000_00), alloc("google_search", 10_000_00))

	res, err := e.launch.Resolve(ctx, coretest.Viewer, plan, 1)
	require.NoError(t, err)
	l := res.Lines[0]
	assert.Equal(t, launch.KindResume, l.Kind)
	require.NotNil(t, l.Campaign)
	assert.Equal(t, m1.ID, l.Campaign.ID)
	assert.Equal(t, launch.MonthBudget{SpentMinor: 5_000_00, SpendKnown: true, RemainingMinor: 35_000_00, DailyMinor: 1_750_00, DaysLeft: 20}, *l.Budget, "what it spent before pausing nets off")
	assert.Contains(t, l.Note, "resumed")
	g := res.Lines[1]
	assert.Equal(t, launch.KindUpdate, g.Kind, "a running campaign takes the line; the paused one is left aside")
	assert.Equal(t, "g1", g.Campaign.ExternalID)
	require.Len(t, g.Paused, 1)
	assert.Equal(t, "g0", g.Paused[0].ExternalID)
	assert.Empty(t, res.Unmatched)
	c, err := e.launch.Coverage(ctx, 1, e.metaAcc, "m1")
	require.NoError(t, err)
	assert.True(t, c.UnderPlan, "the plan stands behind the campaign it resumes")

	// Two paused, none running: a person picks which one comes back.
	m2 := e.campaign(t, e.metaAcc, "meta", "m2", "Wadi | Leads | EN", "OUTCOME_LEADS", "PAUSED", e.wadi, nil)
	res, err = e.launch.Resolve(ctx, coretest.Viewer, plan, 1)
	require.NoError(t, err)
	assert.Equal(t, launch.KindPick, res.Lines[0].Kind)
	assert.Len(t, res.Lines[0].Candidates, 2)
	assert.Contains(t, res.Lines[0].Note, "paused")
	res, err = e.launch.Pick(ctx, coretest.Marketer, plan, 1, 0, m2.ID)
	require.NoError(t, err)
	assert.Equal(t, launch.KindResume, res.Lines[0].Kind)
	assert.Equal(t, m2.ID, res.Lines[0].Campaign.ID)
}

// Catches: two lines of one channel both updating the one campaign that
// matches them, which counts its budget twice (the owner, 2026-10-19: a
// person decides which line the campaign is for, the other creates).
func TestOneCampaignIsTheTargetOfOneLineOnly(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	g := e.campaign(t, e.googleAcc, "google_ads", "g1", "Search | Wadi", "SEARCH", "ENABLED", e.wadi, i64(2_000_00))
	ae, ksa := alloc("google_search", 12_000_00), alloc("google_search", 6_000_00)
	ksa.Geography = "SA"
	plan := e.plan(t, ae, ksa)

	res, err := e.launch.Resolve(ctx, coretest.Viewer, plan, 1)
	require.NoError(t, err)
	for _, l := range res.Lines {
		assert.Equal(t, launch.KindPick, l.Kind, "neither line takes the campaign unasked")
		assert.Nil(t, l.Campaign)
		require.Len(t, l.Candidates, 1)
		assert.Contains(t, l.Note, "which line it is for")
	}
	require.Len(t, res.Unmatched, 1)
	c, err := e.launch.Coverage(ctx, 1, e.googleAcc, "g1")
	require.NoError(t, err)
	assert.False(t, c.UnderPlan, "not under the plan until a line is chosen")

	res, err = e.launch.Pick(ctx, coretest.Marketer, plan, 1, 1, g.ID)
	require.NoError(t, err)
	assert.Equal(t, launch.KindUpdate, res.Lines[1].Kind)
	assert.Equal(t, g.ID, res.Lines[1].Campaign.ID)
	assert.Equal(t, int64(4_000_00), res.Lines[1].Budget.RemainingMinor, "the KSA line nets the spend")
	assert.Equal(t, launch.KindCreate, res.Lines[0].Kind, "the other line creates")
	assert.Equal(t, launch.MonthBudget{SpentMinor: 0, SpendKnown: true, RemainingMinor: 12_000_00, DailyMinor: 600_00, DaysLeft: 20}, *res.Lines[0].Budget)
	assert.Contains(t, res.Lines[0].Note, "another line's")
	assert.Empty(t, res.Unmatched)
	c, err = e.launch.Coverage(ctx, 1, e.googleAcc, "g1")
	require.NoError(t, err)
	assert.True(t, c.UnderPlan)

	// A pick for the same campaign on the other line too: the lower line
	// keeps it and the other creates, never both.
	_, err = e.db.Writer().UpsertLaunchPick(ctx, store.UpsertLaunchPickParams{WorkspaceID: 1, PlanID: plan, PlanVersion: 1, LineIndex: 0, Channel: "google_search", CampaignID: g.ID, PickedAt: e.clock.Now()})
	require.NoError(t, err)
	res, err = e.launch.Resolve(ctx, coretest.Viewer, plan, 1)
	require.NoError(t, err)
	assert.Equal(t, launch.KindUpdate, res.Lines[0].Kind)
	assert.Equal(t, launch.KindCreate, res.Lines[1].Kind)
	assert.Contains(t, res.Lines[1].Note, "is line 0's")
	updates := 0
	for _, l := range res.Lines {
		if l.Campaign != nil {
			updates++
		}
	}
	assert.Equal(t, 1, updates)
}

// Catches: a pick that applies to a campaign outside the candidates, a pick
// updating more than the chosen campaign, and a stale pick (the chosen
// campaign has since been paused) being applied silently.
func TestPickIsRecordedAppliedAndChecked(t *testing.T) {
	e := setup(t)
	m1 := e.campaign(t, e.metaAcc, "meta", "m1", "Wadi | Leads | AR", "OUTCOME_LEADS", "ACTIVE", e.wadi, i64(50_000_00))
	m2 := e.campaign(t, e.metaAcc, "meta", "m2", "Wadi | Leads | EN", "OUTCOME_LEADS", "ACTIVE", e.wadi, i64(30_000_00))
	g := e.campaign(t, e.googleAcc, "google_ads", "g1", "Search | Wadi", "SEARCH", "ENABLED", e.wadi, i64(1_00))
	plan := e.plan(t, alloc("meta_lead_form", 200_000_00), alloc("google_search", 10_000_00))
	ctx := t.Context()

	_, err := e.launch.Pick(ctx, coretest.Marketer, plan, 1, 0, g.ID)
	requireKind(t, err, core.KindInvalid)
	_, err = e.launch.Pick(ctx, coretest.Marketer, plan, 1, 1, g.ID)
	requireKind(t, err, core.KindConflict)
	_, err = e.launch.Pick(ctx, coretest.Marketer, plan, 1, 7, g.ID)
	requireKind(t, err, core.KindInvalid)

	res, err := e.launch.Pick(ctx, coretest.Marketer, plan, 1, 0, m2.ID)
	require.NoError(t, err)
	l := res.Lines[0]
	assert.Equal(t, launch.KindUpdate, l.Kind)
	assert.Equal(t, m2.ID, l.Campaign.ID)
	require.NotNil(t, l.Pick)
	assert.Equal(t, int64(2), *l.Pick.PickedBy)
	assert.Equal(t, int64(170_000_00), l.Budget.RemainingMinor)
	assert.Len(t, l.Candidates, 2, "the candidates stay listed so the choice can change")
	require.Len(t, res.Unmatched, 1, "the campaign not picked stays unplanned")
	assert.Equal(t, m1.ID, res.Unmatched[0].ID)
	picks, err := e.db.Reader().ListLaunchPicks(ctx, store.ListLaunchPicksParams{WorkspaceID: 1, PlanID: plan, PlanVersion: 1})
	require.NoError(t, err)
	require.Len(t, picks, 1)
	assert.Equal(t, "meta_lead_form", picks[0].Channel)

	// A later read sees the same choice without picking again.
	res, err = e.launch.Resolve(ctx, coretest.Viewer, plan, 1)
	require.NoError(t, err)
	assert.Equal(t, m2.ID, res.Lines[0].Campaign.ID)

	// Changing the choice replaces it.
	res, err = e.launch.Pick(ctx, coretest.Admin, plan, 1, 0, m1.ID)
	require.NoError(t, err)
	assert.Equal(t, m1.ID, res.Lines[0].Campaign.ID)
	assert.Equal(t, int64(1), *res.Lines[0].Pick.PickedBy)

	// The chosen campaign is paused: the line asks again instead of
	// quietly matching something else.
	_, err = e.db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: e.metaAcc, Platform: "meta", ExternalID: "m1", Name: "Wadi | Leads | AR", Status: "CAMPAIGN_PAUSED",
		ChannelType: sql.NullString{String: "OUTCOME_LEADS", Valid: true}, FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	res, err = e.launch.Resolve(ctx, coretest.Viewer, plan, 1)
	require.NoError(t, err)
	assert.Equal(t, launch.KindUpdate, res.Lines[0].Kind, "one running campaign is left, so it is the match")
	assert.Equal(t, m2.ID, res.Lines[0].Campaign.ID)
	assert.Nil(t, res.Lines[0].Pick)
}

// Catches: a draft or ready version launching, a plan of another workspace
// being readable, and a plan with no project resolving against nothing.
func TestOnlyAnAcceptedVersionOfAPlanWithAProjectResolves(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	d, err := e.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e.reportage, ProjectID: e.wadi, Title: "Draft", Document: plans.PlanDocument{Objective: "Leads", BudgetMinor: 1_00, Currency: "AED", PeriodFrom: "2026-10-01", PeriodTo: "2026-10-31", Allocations: []plans.Allocation{alloc("google_search", 1_00)}}})
	require.NoError(t, err)
	_, err = e.launch.Resolve(ctx, coretest.Viewer, d.Plan.ID, 1)
	requireKind(t, err, core.KindConflict)
	_, err = e.launch.Resolve(ctx, coretest.Viewer, 999, 1)
	requireKind(t, err, core.KindNotFound)
	other := coretest.Viewer
	other.WorkspaceID = 2
	_, err = e.launch.Resolve(ctx, other, d.Plan.ID, 1)
	requireKind(t, err, core.KindNotFound)

	noProject, err := e.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e.reportage, Title: "Brand-wide", Document: plans.PlanDocument{Objective: "Leads", BudgetMinor: 1_00, Currency: "AED", PeriodFrom: "2026-10-01", PeriodTo: "2026-10-31", Allocations: []plans.Allocation{alloc("google_search", 1_00)}}})
	require.NoError(t, err)
	_, err = e.plans.Accept(ctx, coretest.Admin, noProject.Plan.ID, 1)
	require.NoError(t, err)
	res, err := e.launch.Resolve(ctx, coretest.Viewer, noProject.Plan.ID, 1)
	require.NoError(t, err)
	assert.Nil(t, res.ProjectID)
	assert.Equal(t, launch.KindUnsupported, res.Lines[0].Kind)
	assert.Contains(t, res.Lines[0].Note, "names no project")
}

// Catches: a campaign ADX never read, one with no project, one of another
// project, or one matched only by a plan whose period is over counting as
// under a plan; and a sole match or a recorded pick not counting.
func TestCoverageFollowsTheResolution(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	g := e.campaign(t, e.googleAcc, "google_ads", "g1", "Search | Wadi", "SEARCH", "ENABLED", e.wadi, nil)
	m1 := e.campaign(t, e.metaAcc, "meta", "m1", "Wadi | Leads | AR", "OUTCOME_LEADS", "ACTIVE", e.wadi, nil)
	m2 := e.campaign(t, e.metaAcc, "meta", "m2", "Wadi | Leads | EN", "OUTCOME_LEADS", "ACTIVE", e.wadi, nil)
	e.campaign(t, e.googleAcc, "google_ads", "v1", "Search | Verdana", "SEARCH", "ENABLED", e.verdana, nil)
	orphan, err := e.db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: e.googleAcc, Platform: "google_ads", ExternalID: "x1", Name: "Unknown project", Status: "ENABLED", FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)

	under := func(acct int64, ext string) launch.PlanCoverage {
		c, err := e.launch.Coverage(ctx, 1, acct, ext)
		require.NoError(t, err)
		return c
	}
	assert.False(t, under(e.googleAcc, "g1").UnderPlan, "no plan at all")
	assert.Contains(t, under(e.googleAcc, "nope").Reason, "has not read")
	assert.Contains(t, under(e.googleAcc, orphan.ExternalID).Reason, "no project")

	// Last month's plan is over: nothing is under it now.
	e.planFor(t, e.wadi, "2026-09-01", "2026-09-30", alloc("google_search", 10_000_00))
	assert.False(t, under(e.googleAcc, "g1").UnderPlan)

	plan := e.plan(t, alloc("google_search", 10_000_00), alloc("meta_lead_form", 20_000_00))
	c := under(e.googleAcc, "g1")
	assert.True(t, c.UnderPlan, c.Reason)
	assert.Equal(t, plan, *c.PlanID)
	assert.False(t, under(e.googleAcc, "v1").UnderPlan, "Verdana's campaign is not under The Wadi's plan")
	assert.False(t, under(e.metaAcc, "m1").UnderPlan, "two matches: neither is under the plan until a person picks")
	assert.False(t, under(e.metaAcc, "m2").UnderPlan)

	_, err = e.launch.Pick(ctx, coretest.Marketer, plan, 1, 1, m2.ID)
	require.NoError(t, err)
	assert.True(t, under(e.metaAcc, "m2").UnderPlan)
	assert.False(t, under(e.metaAcc, "m1").UnderPlan, "the campaign not picked stays unplanned")
	ok, _, err := e.launch.UnderPlan(ctx, 1, e.googleAcc, g.ExternalID)
	require.NoError(t, err)
	assert.True(t, ok)
	_ = m1
}

func requireKind(t *testing.T, err error, kind core.Kind, msg ...any) {
	t.Helper()
	var ce *core.Error
	require.ErrorAs(t, err, &ce, "%v", err)
	assert.Equal(t, kind, ce.Kind, append([]any{ce.Message}, msg...)...)
}

// writes configures policy with the suggested defaults, verified write
// capabilities on both accounts, and proposals that ask this launch service
// for plan coverage.
func (e *env) writes(t *testing.T) *proposals.Proposals {
	t.Helper()
	ctx := t.Context()
	pol := policies.New(e.db)
	for _, acc := range []int64{e.googleAcc, e.metaAcc} {
		for _, c := range []string{"write.budget", "write.status", "write.campaign"} {
			require.NoError(t, e.db.Writer().UpsertAccountCapability(ctx, store.UpsertAccountCapabilityParams{AccountID: acc, Capability: c, State: "verified", CheckedAt: e.clock.Now()}))
		}
	}
	pct, changes := 50, 20
	doc := policies.SuggestedDefaults()
	doc.Caps = policies.PolicyCaps{DailySpendIncreasePct: &pct, MaxChangesPerDay: &changes}
	doc.WriteAccounts = []int64{e.googleAcc, e.metaAcc}
	_, err := pol.Put(ctx, coretest.Admin, doc)
	require.NoError(t, err)
	props := proposals.New(e.db, pol, e.plans, proposals.Config{Clock: e.clock, Coverage: e.launch})
	e.launch.AttachProposals(props)
	return props
}

func (e *env) accounts() map[string]int64 {
	return map[string]int64{"google_ads": e.googleAcc, "meta": e.metaAcc}
}

// Catches: a second Submit creating a second campaign or a second intent,
// an update proposing a raise when the budget should fall (or the reverse),
// a create proposed without the line's envelope and amount, an unchanged
// budget proposed anyway, and an unsupported line reaching proposals.
func TestSubmitWritesOneProposalPerLineOnce(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	props := e.writes(t)
	// Search runs at 10,000 a day and the line leaves 5,600 a day: a lower.
	g := e.campaign(t, e.googleAcc, "google_ads", "g1", "Search | Wadi", "SEARCH", "ENABLED", e.wadi, i64(40_000_00))
	_, err := e.db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: e.googleAcc, Platform: "google_ads", ExternalID: "g1", Name: "Search | Wadi", Status: "ENABLED",
		ChannelType: sql.NullString{String: "SEARCH", Valid: true}, BudgetMinor: sql.NullInt64{Int64: 10_000_00, Valid: true}, BudgetPeriod: sql.NullString{String: "daily", Valid: true}, FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	// Leads runs at 1,000 a day and the line leaves 2,000: a raise.
	m := e.campaign(t, e.metaAcc, "meta", "m1", "Wadi | Leads", "OUTCOME_LEADS", "ACTIVE", e.wadi, nil)
	_, err = e.db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: e.metaAcc, Platform: "meta", ExternalID: "m1", Name: "Wadi | Leads", Status: "ACTIVE",
		ChannelType: sql.NullString{String: "OUTCOME_LEADS", Valid: true}, BudgetMinor: sql.NullInt64{Int64: 1_000_00, Valid: true}, BudgetPeriod: sql.NullString{String: "daily", Valid: true}, FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	// Traffic already spends exactly what the line leaves: unchanged.
	e.campaign(t, e.metaAcc, "meta", "m2", "Wadi | Traffic", "OUTCOME_TRAFFIC", "ACTIVE", e.wadi, nil)
	_, err = e.db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: e.metaAcc, Platform: "meta", ExternalID: "m2", Name: "Wadi | Traffic", Status: "ACTIVE",
		ChannelType: sql.NullString{String: "OUTCOME_TRAFFIC", Valid: true}, BudgetMinor: sql.NullInt64{Int64: 500_00, Valid: true}, BudgetPeriod: sql.NullString{String: "daily", Valid: true}, FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	plan := e.plan(t, alloc("google_search", 152_000_00), alloc("meta_lead_form", 40_000_00), alloc("meta_traffic", 10_000_00), alloc("google_pmax", 20_000_00), alloc("tiktok", 5_000_00))

	res, err := e.launch.Submit(ctx, coretest.Admin, launch.SubmitInput{PlanID: plan, PlanVersion: 1, AccountByPlatform: e.accounts()})
	require.NoError(t, err)
	require.Len(t, res.Lines, 5)
	byIndex := map[int]launch.SubmittedLine{}
	for _, l := range res.Lines {
		byIndex[l.Line.Index] = l
	}
	lower := byIndex[0]
	assert.Equal(t, launch.StateSubmitted, lower.State, lower.Note)
	assert.Equal(t, policies.BudgetLower, lower.Proposal.Action)
	assert.Equal(t, g.ExternalID, lower.Proposal.TargetRef)
	assert.Equal(t, "budget:1000000", lower.Proposal.TargetVersion)
	assert.JSONEq(t, `{"dailyBudgetMinor":560000}`, string(lower.Proposal.Proposed))
	assert.Equal(t, int64(112_000_00), *lower.Proposal.AmountMinor)
	require.NotNil(t, lower.Intent)
	assert.Equal(t, "reserved", lower.Intent.State)
	raise := byIndex[1]
	assert.Equal(t, policies.BudgetRaise, raise.Proposal.Action)
	assert.Equal(t, m.ExternalID, raise.Proposal.TargetRef)
	assert.Equal(t, launch.StateSubmitted, raise.State, "the admin self-approves a raise")
	assert.Equal(t, launch.StateUnchanged, byIndex[2].State)
	assert.Nil(t, byIndex[2].Proposal)
	create := byIndex[3]
	assert.Equal(t, launch.StateSubmitted, create.State, create.Note)
	assert.Equal(t, policies.CampaignCreatePaus, create.Proposal.Action)
	assert.Equal(t, "new:launch:1:1:3", create.Proposal.TargetRef)
	assert.Equal(t, int64(20_000_00), *create.Proposal.AmountMinor)
	require.NotNil(t, create.Proposal.EnvelopeID)
	assert.Contains(t, string(create.Proposal.Proposed), `"channelType":"PERFORMANCE_MAX"`)
	assert.Contains(t, string(create.Proposal.Proposed), `"dailyBudgetMinor":100000`)
	assert.Contains(t, create.Proposal.Reason, "plan 1 version 1, line 3")
	assert.Equal(t, launch.StateSkipped, byIndex[4].State)
	assert.Nil(t, byIndex[4].Proposal)

	intents, err := props.ListIntents(ctx, coretest.Viewer, "", 0)
	require.NoError(t, err)
	assert.Len(t, intents, 3)
	envs, err := props.ListEnvelopes(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.Len(t, envs, 4, "one envelope per channel ADX can deploy; TikTok gets none")

	// Again: the same proposals come back, nothing new is made.
	again, err := e.launch.Submit(ctx, coretest.Marketer, launch.SubmitInput{PlanID: plan, PlanVersion: 1})
	require.NoError(t, err)
	for i := range res.Lines {
		if res.Lines[i].Proposal != nil {
			assert.Equal(t, res.Lines[i].Proposal.ID, again.Lines[i].Proposal.ID, "line %d", i)
			assert.Equal(t, launch.StateSubmitted, again.Lines[i].State)
		}
	}
	intents, err = props.ListIntents(ctx, coretest.Viewer, "", 0)
	require.NoError(t, err)
	assert.Len(t, intents, 3)
	proposalsNow, err := props.List(ctx, coretest.Viewer, "", 0)
	require.NoError(t, err)
	assert.Len(t, proposalsNow, 3)
}

// Catches: a submission going ahead while a line still waits on a pick, a
// marketer opening envelopes or submitting with none open, and a marketer's
// raise executing without the admin.
func TestSubmitNeedsEveryPickAndOpenEnvelopes(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	props := e.writes(t)
	m1 := e.campaign(t, e.metaAcc, "meta", "m1", "Wadi | Leads | AR", "OUTCOME_LEADS", "ACTIVE", e.wadi, nil)
	e.campaign(t, e.metaAcc, "meta", "m2", "Wadi | Leads | EN", "OUTCOME_LEADS", "ACTIVE", e.wadi, nil)
	plan := e.plan(t, alloc("meta_lead_form", 40_000_00))

	_, err := e.launch.Submit(ctx, coretest.Admin, launch.SubmitInput{PlanID: plan, PlanVersion: 1, AccountByPlatform: e.accounts()})
	requireKind(t, err, core.KindConflict)
	assert.Contains(t, err.Error(), "needs a person's pick")
	_, err = e.launch.Pick(ctx, coretest.Marketer, plan, 1, 0, m1.ID)
	require.NoError(t, err)

	_, err = e.launch.Submit(ctx, coretest.Marketer, launch.SubmitInput{PlanID: plan, PlanVersion: 1, AccountByPlatform: e.accounts()})
	requireKind(t, err, core.KindConflict)
	assert.Contains(t, err.Error(), "an admin opens")
	bot := coretest.Marketer
	bot.Via = core.ViaAssistant
	_, err = e.launch.Submit(ctx, bot, launch.SubmitInput{PlanID: plan, PlanVersion: 1})
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.launch.Submit(ctx, coretest.Admin, launch.SubmitInput{PlanID: plan, PlanVersion: 1})
	requireKind(t, err, core.KindInvalid, "an admin with no accounts named cannot open envelopes either")

	// Envelopes open; the marketer's raise waits for the admin.
	_, err = props.OpenEnvelopes(ctx, coretest.Admin, plan, 1, e.accounts())
	require.NoError(t, err)
	res, err := e.launch.Submit(ctx, coretest.Marketer, launch.SubmitInput{PlanID: plan, PlanVersion: 1})
	require.NoError(t, err)
	require.Len(t, res.Lines, 1)
	assert.Equal(t, launch.StateAwaitingApproval, res.Lines[0].State)
	assert.Equal(t, policies.BudgetRaise, res.Lines[0].Proposal.Action, "no daily budget read: proposed as a raise to the plan's figure")
	assert.Equal(t, "budget:unknown", res.Lines[0].Proposal.TargetVersion)
	assert.Nil(t, res.Lines[0].Intent)
}

// Catches: a paused-only match being created again instead of resumed, and
// a spent line setting a zero budget.
func TestSubmitResumesAndStopsAtTheCeiling(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.writes(t)
	e.campaign(t, e.metaAcc, "meta", "m1", "Wadi | Leads", "OUTCOME_LEADS", "CAMPAIGN_PAUSED", e.wadi, i64(5_000_00))
	e.campaign(t, e.googleAcc, "google_ads", "g1", "Search | Wadi", "SEARCH", "ENABLED", e.wadi, i64(45_000_00))
	plan := e.plan(t, alloc("meta_lead_form", 40_000_00), alloc("google_search", 30_000_00))

	res, err := e.launch.Submit(ctx, coretest.Admin, launch.SubmitInput{PlanID: plan, PlanVersion: 1, AccountByPlatform: e.accounts()})
	require.NoError(t, err)
	resume := res.Lines[0]
	assert.Equal(t, launch.KindResume, resume.Kind)
	assert.Equal(t, launch.StateSubmitted, resume.State, resume.Note)
	assert.Equal(t, policies.CampaignResume, resume.Proposal.Action)
	assert.Equal(t, "m1", resume.Proposal.TargetRef)
	assert.Equal(t, "status:CAMPAIGN_PAUSED", resume.Proposal.TargetVersion)
	assert.JSONEq(t, `{"status":"ACTIVE","dailyBudgetMinor":175000}`, string(resume.Proposal.Proposed))
	assert.Equal(t, int64(35_000_00), *resume.Proposal.AmountMinor)
	assert.Equal(t, launch.StateExhausted, res.Lines[1].State)
	assert.Nil(t, res.Lines[1].Proposal)
}
