package fronts_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/alerts"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/execution"
	"github.com/WokeWorks/adx/server/core/fronts"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/core/proposals/proposalstest"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/core/supervisor"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// 2026-10-17, 09:00 in Dubai.
var now = time.Date(2026, 10, 17, 5, 0, 0, 0, time.UTC)

type env struct {
	db       *store.DB
	clock    *coretest.Clock
	fronts   *fronts.Service
	alerts   *alerts.Alerts
	runs     *runs.Runs
	plans    *plans.Plans
	brands   *brands.Brands
	brand    int64
	account  int64
	campaign store.AdCampaign
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(now)}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	ev := evidence.New(e.db, e.clock)
	rep := reporting.New(e.db, nil, nil, ev, reporting.Config{Clock: e.clock})
	e.alerts = alerts.New(e.db, rep, alerts.Config{Clock: e.clock})
	e.runs = runs.New(e.db, nil, runs.Config{Clock: e.clock})
	br := brands.New(e.db, brands.Config{Clock: e.clock})
	e.brands = br
	e.plans = plans.New(e.db, br, plans.Config{Clock: e.clock})
	pol := policies.New(e.db)
	props := proposals.New(e.db, pol, e.plans, proposals.Config{Clock: e.clock, Coverage: proposalstest.Planned{}})
	exec := execution.New(e.db, pol, props, nil, nil, execution.Config{Clock: e.clock})
	sup := supervisor.New(e.db, supervisor.Config{Clock: e.clock, Proposals: props, Policies: pol})
	e.fronts = fronts.New(fronts.Config{Clock: e.clock, Alerts: e.alerts, Supervisor: sup, Reporting: rep, Runs: e.runs, Plans: e.plans, Execution: exec})

	b, err := br.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	e.brand = b.ID
	cred, err := e.db.Writer().UpsertCredential(ctx, store.UpsertCredentialParams{Provider: "google_ads", Kind: "oauth", Subject: "u", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(ctx, store.UpsertAccountParams{WorkspaceID: core.DefaultWorkspace, Type: "google_ads", ExternalID: "1", CredentialID: cred.ID, Name: sql.NullString{String: "Reportage", Valid: true},
		Currency: sql.NullString{String: "AED", Valid: true}, TimeZone: sql.NullString{String: "Asia/Dubai", Valid: true}, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	e.account = acct.ID
	e.campaign, err = e.db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: acct.ID, Platform: "google_ads", ExternalID: "c1", Name: "Wadi Search", Status: "ENABLED",
		Currency: sql.NullString{String: "AED", Valid: true}, FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	return e
}

func (e *env) metric(t *testing.T, day string, cost int64) {
	t.Helper()
	require.NoError(t, e.db.Writer().UpsertCampaignDailyMetrics(t.Context(), store.UpsertCampaignDailyMetricsParams{WorkspaceID: 1, AccountID: e.account, CampaignID: e.campaign.ID, Platform: "google_ads",
		Day: day, Currency: "AED", CostMinor: cost, Impressions: 10, Clicks: 1, ObservedAt: e.clock.Now()}))
}

func (e *env) fresh(t *testing.T, through string) {
	t.Helper()
	require.NoError(t, e.db.Writer().UpsertAccountFreshness(t.Context(), store.UpsertAccountFreshnessParams{AccountID: e.account, WorkspaceID: 1,
		LastSuccessAt: sql.NullTime{Time: e.clock.Now(), Valid: true}, ObservedThrough: sql.NullString{String: through, Valid: true}}))
}

func (e *env) desk(t *testing.T) fronts.Desk {
	t.Helper()
	d, err := e.fronts.Desk(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	return d
}

func find(cards []fronts.Card, id string) *fronts.Card {
	for i := range cards {
		if cards[i].ID == id {
			return &cards[i]
		}
	}
	return nil
}

func i64(v int64) *int64 { return &v }

// Catches: a 0 shown where no figure exists (nothing read yet, or nothing
// read inside the month), and a figure card counted as needing a person.
func TestMissingFiguresAreNullNotZero(t *testing.T) {
	e := setup(t)
	d := e.desk(t)
	spend := find(d.Cards, "spend")
	require.NotNil(t, spend)
	require.NotNil(t, spend.Figure)
	assert.Nil(t, spend.Figure.Value, "no ad account read: the value must be null, not 0")
	assert.Nil(t, spend.Figure.Count)
	assert.Equal(t, fronts.StatusPending, spend.Status)
	assert.False(t, spend.NeedsYou)
	assert.Contains(t, spend.Why, "No ad account has been read yet")
	assert.Nil(t, find(d.Cards, "leads"), "no leads at all: nothing to say")
	assert.Nil(t, find(d.Cards, "channels"))
	assert.Equal(t, "1 thing needs your attention.", d.Summary, "the missing write policy, nothing else")
	assert.Len(t, d.Templates, 18)

	// Read, but only last month's figures: still not a 0 for this month.
	e.metric(t, "2026-09-30", 50000)
	e.fresh(t, "2026-09-30")
	d = e.desk(t)
	spend = find(d.Cards, "spend")
	require.NotNil(t, spend)
	assert.Nil(t, spend.Figure.Value)
	assert.Contains(t, spend.Why, "Nothing read inside this month")

	e.metric(t, "2026-10-15", 120050)
	e.metric(t, "2026-10-16", 80000)
	e.fresh(t, "2026-10-16")
	d = e.desk(t)
	spend = find(d.Cards, "spend")
	require.NotNil(t, spend)
	require.NotNil(t, spend.Figure.Value)
	assert.Equal(t, "AED 2,000", *spend.Figure.Value)
	assert.Equal(t, float64(2000), *spend.Figure.Count)
	assert.Equal(t, fronts.StatusReady, spend.Status)
	assert.Contains(t, spend.Why, "Figures through 16 Oct")
	require.NotNil(t, spend.Picture)
	assert.Equal(t, "bars", spend.Picture.Kind)
	assert.Len(t, spend.Picture.Values, 14)
	assert.Equal(t, int64(800), spend.Picture.Values[12], "yesterday's bar, in whole AED")
	assert.Equal(t, int64(0), spend.Picture.Values[13], "today has nothing yet")
	channels := find(d.Cards, "channels")
	require.NotNil(t, channels)
	assert.Equal(t, []fronts.Slice{{Label: "Google", Value: 200050}}, channels.Picture.Slices)
	assert.Contains(t, channels.Why, "Google brings 100% of spend")
}

// Catches: an update ranked above something that needs a person, a ready
// card above a caution one, an older card above a newer one of the same
// rank, and the summary counting updates as things that need attention.
func TestNeedsYouComesFirstWorstFirstThenNewest(t *testing.T) {
	cards := []fronts.Card{
		{ID: "update-new", Status: fronts.StatusReady, At: now},
		{ID: "info", Status: fronts.StatusPending, NeedsYou: true, At: now.Add(-time.Hour)},
		{ID: "update-caution", Status: fronts.StatusCaution, At: now.Add(-2 * time.Hour)},
		{ID: "warning-old", Status: fronts.StatusCaution, NeedsYou: true, At: now.Add(-3 * time.Hour)},
		{ID: "urgent", Status: fronts.StatusError, NeedsYou: true, At: now.Add(-4 * time.Hour)},
		{ID: "warning-new", Status: fronts.StatusCaution, NeedsYou: true, At: now.Add(-time.Hour)},
		{ID: "update-old", Status: fronts.StatusReady, At: now.Add(-5 * time.Hour)},
	}
	fronts.Order(cards)
	var ids []string
	for _, c := range cards {
		ids = append(ids, c.ID)
	}
	assert.Equal(t, []string{"urgent", "warning-new", "warning-old", "info", "update-caution", "update-new", "update-old"}, ids)
	assert.Equal(t, "4 things need your attention.", fronts.Summary(cards))
	assert.Equal(t, "1 thing needs your attention.", fronts.Summary(cards[:1]))

	// The same order comes out of the desk: a warning alert above the
	// pending "no write policy" item above the figures.
	e := setup(t)
	e.metric(t, "2026-10-16", 80000)
	e.fresh(t, "2026-10-16")
	_, err := e.alerts.PutConfig(t.Context(), coretest.Admin, alerts.AlertDocument{ZeroLeads: alerts.ZeroLeadsConfig{MinSpendMinor: i64(1000)}})
	require.NoError(t, err)
	_, err = e.alerts.Evaluate(t.Context(), coretest.Marketer, "2026-10-16")
	require.NoError(t, err)
	d := e.desk(t)
	require.GreaterOrEqual(t, len(d.Cards), 3)
	assert.Equal(t, "alert-1", d.Cards[0].ID)
	assert.Equal(t, fronts.StatusCaution, d.Cards[0].Status)
	assert.Equal(t, "Spend with no leads: Wadi Search", d.Cards[0].Title)
	assert.Equal(t, "AED 800", *d.Cards[0].Figure.Value)
	assert.Equal(t, "on 16 Oct", d.Cards[0].Figure.Unit)
	assert.Equal(t, int64(1), *d.Cards[0].AlertID)
	assert.Equal(t, "inbox-missing_config-0", d.Cards[1].ID)
	assert.True(t, d.Cards[1].NeedsYou)
	assert.Equal(t, fronts.StatusPending, d.Cards[1].Status)
	assert.Equal(t, "spend", d.Cards[2].ID)
	assert.Equal(t, "2 things need your attention.", d.Summary)
}

// Catches: a dismissed alert coming back as a card, a snoozed one showing
// before its date or never coming back, and the Inbox filters and counts
// disagreeing with the cards.
func TestDismissedAndSnoozedAlertsLeaveTheFronts(t *testing.T) {
	e := setup(t)
	e.metric(t, "2026-10-16", 80000)
	e.fresh(t, "2026-10-16")
	_, err := e.alerts.PutConfig(t.Context(), coretest.Admin, alerts.AlertDocument{ZeroLeads: alerts.ZeroLeadsConfig{MinSpendMinor: i64(1000)}})
	require.NoError(t, err)
	_, err = e.alerts.Evaluate(t.Context(), coretest.Marketer, "2026-10-16")
	require.NoError(t, err)
	require.NotNil(t, find(e.desk(t).Cards, "alert-1"))

	_, err = e.alerts.Snooze(t.Context(), coretest.Marketer, 1, 7)
	require.NoError(t, err)
	assert.Nil(t, find(e.desk(t).Cards, "alert-1"), "snoozed: off the desk")
	in, err := e.fronts.Inbox(t.Context(), coretest.Viewer, fronts.InboxFilter{Show: "needs"})
	require.NoError(t, err)
	assert.Nil(t, find(in.Cards, "alert-1"))
	e.clock.Advance(8 * 24 * time.Hour)
	require.NotNil(t, find(e.desk(t).Cards, "alert-1"), "the snooze ended: back")

	_, err = e.alerts.Dismiss(t.Context(), coretest.Marketer, 1, "Expected")
	require.NoError(t, err)
	d := e.desk(t)
	assert.Nil(t, find(d.Cards, "alert-1"), "dismissed: gone for good")
	assert.Equal(t, "1 thing needs your attention.", d.Summary, "only the missing write policy")

	in, err = e.fronts.Inbox(t.Context(), coretest.Viewer, fronts.InboxFilter{})
	require.NoError(t, err)
	assert.Equal(t, fronts.Counts{All: 3, NeedsYou: 1, Updates: 2}, in.Counts)
	assert.Len(t, in.Cards, 3)
	in, err = e.fronts.Inbox(t.Context(), coretest.Viewer, fronts.InboxFilter{Show: "updates", Door: fronts.DoorPerform})
	require.NoError(t, err)
	assert.Equal(t, fronts.Counts{All: 3, NeedsYou: 1, Updates: 2}, in.Counts, "counts are of every card, before the filter")
	assert.Len(t, in.Cards, 2)
	for _, c := range in.Cards {
		assert.False(t, c.NeedsYou)
		assert.Equal(t, fronts.DoorPerform, c.Door)
	}
	in, err = e.fronts.Inbox(t.Context(), coretest.Viewer, fronts.InboxFilter{Show: "needs", Door: fronts.DoorConvert})
	require.NoError(t, err)
	assert.Empty(t, in.Cards)
	_, err = e.fronts.Inbox(t.Context(), coretest.Viewer, fronts.InboxFilter{Show: "mine"})
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	_, err = e.fronts.Desk(t.Context(), core.Actor{})
	assert.ErrorIs(t, err, core.ErrForbidden)
}

// Catches: a ready plan not asking for acceptance, an accepted plan with
// nothing sent not asking for its launch, a plan whose period ended still
// on the desk, and a queued job shown as a card instead of a count.
func TestPlansAndJobsArriveAsCardsAndWork(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	project, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, project.ID, e.brand))
	cpl := int64(25000)
	doc := plans.PlanDocument{Objective: "Leads", BudgetMinor: 45_000_000, Currency: "AED", PeriodFrom: "2026-10-01", PeriodTo: "2026-11-30",
		Allocations: []plans.Allocation{{Channel: "google_search", Objective: "leads", BudgetMinor: 30_000_000, Geography: "AE", ExpectedCPLMinor: &cpl}, {Channel: "meta_lead_form", Objective: "leads", BudgetMinor: 15_000_000, Geography: "AE", ExpectedCPLMinor: &cpl}}}
	pd, err := e.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e.brand, ProjectID: project.ID, Title: "Wadi Q4", Document: doc})
	require.NoError(t, err)
	require.Equal(t, plans.StateReady, pd.Versions[0].State)
	ended := doc
	ended.PeriodFrom, ended.PeriodTo = "2026-08-01", "2026-08-31"
	_, err = e.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e.brand, ProjectID: project.ID, Title: "Wadi Aug", Document: ended})
	require.NoError(t, err)

	d := e.desk(t)
	accept := find(d.Cards, "plan-1-accept")
	require.NotNil(t, accept)
	assert.True(t, accept.NeedsYou)
	assert.Equal(t, fronts.DoorStudio, accept.Door)
	assert.Equal(t, "Plan v1 for Wadi Q4 is waiting on you", accept.Title)
	assert.Equal(t, "AED 450K from 1 Oct to 30 Nov, 0 assumptions. Accepting it starts the launch.", accept.Why)
	assert.Equal(t, &fronts.CardAction{Label: "Review", To: "/studio/plans/1", Primary: true}, accept.Action)
	require.NotNil(t, accept.Picture)
	assert.Equal(t, []fronts.Slice{{Label: "Google Search", Value: 30_000_000}, {Label: "Meta lead form", Value: 15_000_000}}, accept.Picture.Slices)
	assert.Nil(t, find(d.Cards, "plan-2-accept"), "a plan whose period ended is not on the desk")
	assert.Nil(t, find(d.Cards, "plan-1-launch"))

	_, err = e.plans.Accept(ctx, coretest.Admin, pd.Plan.ID, 1)
	require.NoError(t, err)
	e.clock.Advance(3 * time.Hour)
	d = e.desk(t)
	assert.Nil(t, find(d.Cards, "plan-1-accept"))
	launch := find(d.Cards, "plan-1-launch")
	require.NotNil(t, launch)
	assert.True(t, launch.NeedsYou)
	assert.Equal(t, fronts.StatusCaution, launch.Status)
	assert.Equal(t, "Wadi Q4 is accepted, not launched", launch.Title)
	assert.Equal(t, "Plan v1 was accepted 3 h ago and nothing has been sent to a platform.", launch.Why)
	assert.Equal(t, "/studio/launch", launch.Action.To)

	// A queued job is a count, not a card; its result is a card.
	run, _, err := e.runs.Start(ctx, coretest.Marketer, runs.StartInput{Kind: "ads.refresh", OccurrenceKey: "2026-10-17", AccountID: e.account})
	require.NoError(t, err)
	d = e.desk(t)
	require.Len(t, d.Working, 1)
	assert.Equal(t, fronts.WorkItem{ID: run.ID, Title: "Refreshing the figures", Detail: "Queued.", To: "/engine/runs"}, d.Working[0])
	assert.Nil(t, find(d.Cards, "run-"+run.ID))
	h, err := e.runs.Begin(ctx, run.RowID(), "worker")
	require.NoError(t, err)
	e.clock.Advance(2 * time.Hour)
	require.NoError(t, h.Finish(ctx, runs.Completion{State: runs.StateSucceeded, Completeness: runs.CompletenessComplete}))
	d = e.desk(t)
	assert.Empty(t, d.Working)
	done := find(d.Cards, "run-"+run.ID)
	require.NotNil(t, done)
	assert.False(t, done.NeedsYou)
	assert.Equal(t, fronts.StatusReady, done.Status)
	assert.Equal(t, "Figures refreshed", done.Title)
	assert.Equal(t, "Finished under an hour ago.", done.Why)
	assert.Equal(t, fronts.DoorPerform, done.Door)
	e.clock.Advance(25 * time.Hour)
	assert.Nil(t, find(e.desk(t).Cards, "run-"+run.ID), "a day later the result is old news")
}
