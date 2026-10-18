package reporting_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
)

// Catches: a campaign with no figures in the month shown as having spent
// zero, last month's days counted, a paused campaign reported as running,
// another project's campaign in the list, and a lifetime budget shown as
// a daily one.
func TestProjectCampaignsTypeStatusAndMonthSpend(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	wadi, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	verdana, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "Verdana", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	now := e.clock.Now()
	campaign := func(acct int64, platform, ext, name, status, typ string, project int64, budget int64, period string) store.AdCampaign {
		c, err := e.db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: acct, Platform: platform, ExternalID: ext, Name: name, Status: status,
			ChannelType: sql.NullString{String: typ, Valid: typ != ""}, BudgetMinor: sql.NullInt64{Int64: budget, Valid: budget != 0}, BudgetPeriod: sql.NullString{String: period, Valid: period != ""},
			Currency: sql.NullString{String: "AED", Valid: true}, FirstSeenAt: now, LastSeenAt: now})
		require.NoError(t, err)
		require.NoError(t, e.db.Writer().SetAdCampaignProject(ctx, store.SetAdCampaignProjectParams{ProjectID: sql.NullInt64{Int64: project, Valid: true}, ID: c.ID}))
		return c
	}
	search := campaign(e.googleAcc, "google_ads", "g1", "Search | Wadi", "ENABLED", "SEARCH", wadi.ID, 50000, "daily")
	paused := campaign(e.metaAcc, "meta", "m1", "Wadi | Leads", "CAMPAIGN_PAUSED", "OUTCOME_LEADS", wadi.ID, 2000000, "lifetime")
	quiet := campaign(e.metaAcc, "meta", "m2", "Wadi | Traffic", "ACTIVE", "OUTCOME_TRAFFIC", wadi.ID, 0, "")
	campaign(e.googleAcc, "google_ads", "v1", "Search | Verdana", "ENABLED", "SEARCH", verdana.ID, 0, "")
	metric := func(c store.AdCampaign, day string, cost int64) {
		require.NoError(t, e.db.Writer().UpsertCampaignDailyMetrics(ctx, store.UpsertCampaignDailyMetricsParams{WorkspaceID: 1, AccountID: c.AccountID, CampaignID: c.ID, Platform: c.Platform, Day: day, Currency: "AED", CostMinor: cost, Impressions: 10, Clicks: 1, ObservedAt: now}))
	}
	metric(search, "2026-09-30", 999_00)
	metric(search, "2026-10-01", 100_00)
	metric(search, "2026-10-12", 200_00)
	metric(paused, "2026-10-03", 50_00)

	_, err = e.reporting.ProjectCampaigns(ctx, coretest.Viewer, 0, "2026-10-01", "2026-10-31")
	require.Error(t, err)
	_, err = e.reporting.ProjectCampaigns(ctx, core.Actor{}, wadi.ID, "2026-10-01", "2026-10-31")
	assert.ErrorIs(t, err, core.ErrForbidden)

	list, err := e.reporting.ProjectCampaigns(ctx, coretest.Viewer, wadi.ID, "2026-10-01", "2026-10-31")
	require.NoError(t, err)
	require.Len(t, list, 3, "only The Wadi's campaigns")
	byRef := map[string]reporting.ProjectCampaign{}
	for _, c := range list {
		byRef[c.ExternalID] = c
	}
	g := byRef["g1"]
	assert.Equal(t, "SEARCH", g.Type)
	assert.True(t, g.Running)
	require.NotNil(t, g.SpentMinor)
	assert.Equal(t, int64(300_00), *g.SpentMinor, "September's day is not this month")
	assert.Equal(t, int64(2), g.SpendDays)
	require.NotNil(t, g.DailyBudgetMinor)
	assert.Equal(t, int64(50000), *g.DailyBudgetMinor)
	assert.Equal(t, wadi.ID, *g.ProjectID)

	m1 := byRef["m1"]
	assert.False(t, m1.Running, "a paused campaign is not running")
	assert.Nil(t, m1.DailyBudgetMinor, "a lifetime budget is not a daily one")
	assert.Equal(t, int64(50_00), *m1.SpentMinor)

	m2 := byRef["m2"]
	assert.True(t, m2.Running)
	assert.Nil(t, m2.SpentMinor, "no figures read this month is not zero")
	assert.Equal(t, int64(0), m2.SpendDays)
	_ = quiet
}
