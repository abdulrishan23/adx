package reporting_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
)

// seed loads one Google and one Meta campaign with two days of figures each,
// a project, a funnel mapping, and leads attributed by UTM campaign name.
func seed(t *testing.T, e *env) (googleCampaign, metaCampaign store.AdCampaign) {
	t.Helper()
	ctx := t.Context()
	project, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	now := e.clock.Now()
	googleCampaign, err = e.db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: e.googleAcc, Platform: "google_ads", ExternalID: "g1",
		Name: "The Wadi Search", Status: "ENABLED", Currency: sql.NullString{String: "AED", Valid: true}, FirstSeenAt: now, LastSeenAt: now})
	require.NoError(t, err)
	metaCampaign, err = e.db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: e.metaAcc, Platform: "meta", ExternalID: "m1",
		Name: "wadi leads", Status: "ACTIVE", Currency: sql.NullString{String: "AED", Valid: true}, FirstSeenAt: now, LastSeenAt: now})
	require.NoError(t, err)
	assigned, unresolved, err := e.reporting.AssignProjects(ctx, coretest.Admin)
	require.NoError(t, err)
	assert.Equal(t, 2, assigned)
	assert.Empty(t, unresolved)
	_ = project

	metric := func(c store.AdCampaign, day string, cost int64, conv *float64, metaLeads *float64) {
		require.NoError(t, e.db.Writer().UpsertCampaignDailyMetrics(ctx, store.UpsertCampaignDailyMetricsParams{
			WorkspaceID: 1, AccountID: c.AccountID, CampaignID: c.ID, Platform: c.Platform, Day: day, Currency: "AED", CostMinor: cost,
			Impressions: 1000, Clicks: 100, Conversions: nullF(conv), PlatformLeads: nullF(metaLeads), ObservedAt: now,
		}))
	}
	metric(googleCampaign, "2026-10-01", 100000, f64(4), nil)
	metric(googleCampaign, "2026-10-02", 100000, f64(2), nil)
	metric(metaCampaign, "2026-10-01", 300000, nil, f64(20))
	metric(metaCampaign, "2026-10-02", 300000, nil, f64(10))
	// Previous period: September 29 and 30. Google only; Meta has nothing.
	metric(googleCampaign, "2026-09-29", 50000, f64(1), nil)
	metric(googleCampaign, "2026-09-30", 50000, f64(1), nil)

	l := leads.New(e.db, nil, nil, nil, leads.Config{Clock: e.clock})
	_, err = l.PutFunnel(ctx, coretest.Admin, leads.FunnelDocument{
		StatusField: leads.FieldLocation{Kind: "custom_field", Ref: "status"},
		Statuses:    map[string]leads.Stage{"follow up": leads.StageEngaged, "done-deal": leads.StageWon, "no answer": leads.StageAttempted},
	})
	require.NoError(t, err)
	return googleCampaign, metaCampaign
}

func nullF(v *float64) sql.NullFloat64 {
	if v == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *v, Valid: true}
}

// lead inserts a GoHighLevel lead added at a Dubai time with a UTM campaign
// reference.
func lead(t *testing.T, e *env, id string, addedDubai time.Time, status, campaignRef string) {
	t.Helper()
	ctx := t.Context()
	stage := sql.NullString{}
	switch status {
	case "Follow Up":
		stage = sql.NullString{String: "engaged", Valid: true}
	case "Done-Deal":
		stage = sql.NullString{String: "won", Valid: true}
	case "No Answer":
		stage = sql.NullString{String: "attempted", Valid: true}
	}
	row, err := e.db.Writer().UpsertLead(ctx, store.UpsertLeadParams{WorkspaceID: 1, AccountID: e.googleAcc, ExternalID: id,
		StatusRaw: sql.NullString{String: status, Valid: status != ""}, Stage: stage,
		ProviderAddedAt: sql.NullTime{Time: addedDubai.UTC(), Valid: true}, FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	if campaignRef != "" {
		require.NoError(t, e.db.Writer().InsertLeadAttribution(ctx, store.InsertLeadAttributionParams{LeadID: row.ID, ObservedAt: e.clock.Now(), IsFirst: 1,
			UtmCampaign: sql.NullString{String: campaignRef, Valid: true}, MatchLevel: "utm", MatchConfidence: "low",
			MatchedCampaignRef: sql.NullString{String: campaignRef, Valid: true}, PayloadHash: id}))
	}
}

// Catches: ROAS or revenue shown as zero when no value arrived, cost per
// lead shown as zero or infinity with no leads, a previous-period zero
// denominator producing an infinite change, Google and Meta claims summed,
// and a summary that doesn't add up to its rows.
func TestAnalyticsNullsRatiosAndComparison(t *testing.T) {
	e := setup(t)
	g, m := seed(t, e)
	dubai := reporting.ReportingZone
	// Two leads on Oct 1 Dubai time, one at 02:00 Dubai (Sep 30 22:00 UTC),
	// which must still count as Oct 1. One Done-Deal, one unattributed.
	lead(t, e, "l1", time.Date(2026, 10, 1, 2, 0, 0, 0, dubai), "Follow Up", "The Wadi Search")
	lead(t, e, "l2", time.Date(2026, 10, 1, 14, 0, 0, 0, dubai), "Done-Deal", "g1")
	lead(t, e, "l3", time.Date(2026, 10, 2, 9, 0, 0, 0, dubai), "No Answer", "WADI LEADS")
	lead(t, e, "l4", time.Date(2026, 10, 2, 9, 0, 0, 0, dubai), "", "")
	lead(t, e, "l5", time.Date(2026, 10, 3, 9, 0, 0, 0, dubai), "Follow Up", "g1")

	res, err := e.reporting.Analytics(t.Context(), coretest.Viewer, reporting.AnalyticsQuery{From: "2026-10-01", To: "2026-10-02", Compare: "previous"})
	require.NoError(t, err)
	assert.Equal(t, reporting.FormulaVersion, res.FormulaVersion)
	assert.Equal(t, "Asia/Dubai", res.Period.TimeZone)
	require.NotNil(t, res.Period.Comparison)
	assert.Equal(t, reporting.Day("2026-09-29"), res.Period.Comparison.Start)
	assert.Equal(t, reporting.Day("2026-09-30"), res.Period.Comparison.End)

	require.Len(t, res.Rows, 2)
	byKey := map[string]reporting.ReportRow{}
	for _, r := range res.Rows {
		byKey[r.Label] = r
	}
	gr, mr := byKey["The Wadi Search"], byKey["wadi leads"]
	assert.Equal(t, int64(200000), *gr.Metrics.Spend.Minor)
	assert.Equal(t, "AED", gr.Metrics.Spend.Currency)
	assert.Equal(t, int64(2), gr.Metrics.Leads, "l1 (02:00 Dubai) and l2")
	assert.Equal(t, int64(2), *gr.Metrics.QualifiedLeads, "Follow Up and Done-Deal are qualified")
	assert.Equal(t, int64(1), *gr.Metrics.Deals)
	assert.Equal(t, 6.0, *gr.Metrics.GoogleConversions)
	assert.Nil(t, gr.Metrics.MetaLeads)
	assert.Equal(t, 100000.0, *gr.Metrics.CostPerLead.Value, "200000 fils / 2 leads")
	assert.Nil(t, gr.Metrics.Revenue.Minor, "no Done-Deal value arrives yet")
	assert.Nil(t, gr.Metrics.ROAS.Value)
	assert.False(t, gr.Metrics.ROAS.Undefined, "ROAS is not available, which is different from undefined")
	assert.Equal(t, int64(g.ID), *gr.CampaignID)
	require.NotNil(t, gr.ProjectID)

	assert.Equal(t, int64(600000), *mr.Metrics.Spend.Minor)
	assert.Equal(t, 30.0, *mr.Metrics.MetaLeads)
	assert.Nil(t, mr.Metrics.GoogleConversions)
	assert.Equal(t, int64(1), mr.Metrics.Leads, "WADI LEADS matched the campaign name case-insensitively")
	assert.Equal(t, int64(0), *mr.Metrics.QualifiedLeads)
	assert.True(t, mr.Metrics.CostPerQualifiedLead.Undefined, "no qualified leads: undefined, not zero or infinity")
	assert.Nil(t, mr.Metrics.CostPerQualifiedLead.Value)
	_ = m

	// Summary reconciles with rows and keeps the platform claims apart.
	s := res.Summary.Metrics
	assert.Equal(t, int64(800000), *s.Spend.Minor)
	assert.Equal(t, int64(3), s.Leads)
	assert.Equal(t, 6.0, *s.GoogleConversions)
	assert.Equal(t, 30.0, *s.MetaLeads)
	assert.Equal(t, int64(4), res.Coverage.EligibleCount, "l5 is on Oct 3, outside the period")
	assert.Equal(t, int64(3), res.Coverage.MatchedCount)
	assert.Equal(t, int64(1), res.Coverage.UnclassifiedCount)
	assert.Equal(t, int64(3), res.Coverage.WithStatus)
	assert.Contains(t, res.Coverage.DependenciesMissing[0], "Done-Deal value")

	// Comparison: Google spent 100000 before (200000 now → +100%); Meta had
	// nothing before, so its relative change is undefined, not infinite.
	assert.Equal(t, 1.0, *gr.Changes["spend"].Relative)
	assert.Equal(t, 100000.0, *gr.Changes["spend"].Absolute)
	assert.True(t, mr.Changes["spend"].Undefined)
	assert.Nil(t, mr.Changes["spend"].Relative)
	assert.Nil(t, mr.Changes["spend"].Absolute, "no figures before is not available, not zero")
	assert.True(t, gr.Changes["leads"].Undefined, "zero leads before")
	assert.Equal(t, 2.0, *gr.Changes["leads"].Absolute, "a count of zero before is a real zero")
	require.NotNil(t, gr.Previous)
	assert.Equal(t, int64(100000), *gr.Previous.Spend.Minor)
	assert.Contains(t, codes(res.Warnings), "comparison_incomplete", "Meta has no rows in the comparison period")
	assert.Empty(t, res.Freshness.MissingPeriods)
}

func codes(ws []reporting.ReportWarning) []string {
	out := []string{}
	for _, w := range ws {
		out = append(out, w.Code)
	}
	return out
}

// Catches: grouping that changes totals, project filter leaking other
// campaigns, missing days going unreported, and figures appearing before a
// funnel exists.
func TestAnalyticsGroupingFiltersAndMissingDays(t *testing.T) {
	e := setup(t)
	seed(t, e)
	byProject, err := e.reporting.Analytics(t.Context(), coretest.Viewer, reporting.AnalyticsQuery{From: "2026-10-01", To: "2026-10-02", GroupBy: "project"})
	require.NoError(t, err)
	require.Len(t, byProject.Rows, 1)
	assert.Equal(t, int64(800000), *byProject.Rows[0].Metrics.Spend.Minor)
	assert.Equal(t, *byProject.Summary.Metrics.Spend.Minor, *byProject.Rows[0].Metrics.Spend.Minor)

	byDay, err := e.reporting.Analytics(t.Context(), coretest.Viewer, reporting.AnalyticsQuery{From: "2026-09-29", To: "2026-10-02", GroupBy: "day", Platform: "google_ads"})
	require.NoError(t, err)
	require.Len(t, byDay.Rows, 4)
	assert.Equal(t, "2026-09-29", byDay.Rows[0].Label)
	var total int64
	for _, r := range byDay.Rows {
		total += *r.Metrics.Spend.Minor
	}
	assert.Equal(t, total, *byDay.Summary.Metrics.Spend.Minor)
	assert.Nil(t, byDay.Summary.Metrics.MetaLeads, "platform filter keeps Meta out")

	// A day with no Google rows in range is reported missing for that account.
	gap, err := e.reporting.Analytics(t.Context(), coretest.Viewer, reporting.AnalyticsQuery{From: "2026-09-28", To: "2026-10-02", Platform: "google_ads"})
	require.NoError(t, err)
	require.Len(t, gap.Freshness.MissingPeriods, 1)
	assert.Contains(t, gap.Freshness.MissingPeriods[0], "2026-09-28")

	byPlatform, err := e.reporting.Analytics(t.Context(), coretest.Viewer, reporting.AnalyticsQuery{From: "2026-10-01", To: "2026-10-02", GroupBy: "platform"})
	require.NoError(t, err)
	require.Len(t, byPlatform.Rows, 2)

	// No rows at all: an empty, honest result.
	empty, err := e.reporting.Analytics(t.Context(), coretest.Viewer, reporting.AnalyticsQuery{From: "2025-01-01", To: "2025-01-07"})
	require.NoError(t, err)
	assert.Empty(t, empty.Rows)
	assert.Nil(t, empty.Summary.Metrics.Spend.Minor)
	assert.Equal(t, int64(0), empty.Summary.Metrics.Leads)

	_, err = e.reporting.Analytics(t.Context(), core.Actor{}, reporting.AnalyticsQuery{From: "2026-10-01", To: "2026-10-02"})
	assert.ErrorIs(t, err, core.ErrForbidden)
	var ce *core.Error
	_, err = e.reporting.Analytics(t.Context(), coretest.Viewer, reporting.AnalyticsQuery{From: "2026-10-02", To: "2026-10-01"})
	require.ErrorAs(t, err, &ce)
	_, err = e.reporting.Analytics(t.Context(), coretest.Viewer, reporting.AnalyticsQuery{From: "2026-10-01", To: "2026-10-02", GroupBy: "colour"})
	require.ErrorAs(t, err, &ce)
	_, err = e.reporting.Analytics(t.Context(), coretest.Viewer, reporting.AnalyticsQuery{From: "2026-10-01", To: "2026-10-02", Compare: "sometime"})
	require.ErrorAs(t, err, &ce)

	yoy, err := e.reporting.Analytics(t.Context(), coretest.Viewer, reporting.AnalyticsQuery{From: "2026-10-01", To: "2026-10-02", Compare: "year"})
	require.NoError(t, err)
	assert.Equal(t, reporting.Day("2025-10-01"), yoy.Period.Comparison.Start)
	assert.Equal(t, reporting.Day("2025-10-02"), yoy.Period.Comparison.End)
}

// Catches: a funnel-less workspace showing qualified leads and deals as
// zero instead of not available.
func TestNoFunnelMeansNoQualifiedFigures(t *testing.T) {
	e := setup(t)
	now := e.clock.Now()
	c, err := e.db.Writer().UpsertAdCampaign(t.Context(), store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: e.googleAcc, Platform: "google_ads", ExternalID: "g1",
		Name: "Search", Status: "ENABLED", FirstSeenAt: now, LastSeenAt: now})
	require.NoError(t, err)
	require.NoError(t, e.db.Writer().UpsertCampaignDailyMetrics(t.Context(), store.UpsertCampaignDailyMetricsParams{WorkspaceID: 1, AccountID: e.googleAcc, CampaignID: c.ID,
		Platform: "google_ads", Day: "2026-10-01", Currency: "AED", CostMinor: 1000, Impressions: 1, Clicks: 1, ObservedAt: now}))
	res, err := e.reporting.Analytics(t.Context(), coretest.Viewer, reporting.AnalyticsQuery{From: "2026-10-01", To: "2026-10-01"})
	require.NoError(t, err)
	require.Len(t, res.Rows, 1)
	assert.Nil(t, res.Rows[0].Metrics.QualifiedLeads)
	assert.Nil(t, res.Rows[0].Metrics.Deals)
	assert.Nil(t, res.Rows[0].Metrics.CostPerQualifiedLead.Value)
	assert.True(t, res.Rows[0].Metrics.CostPerLead.Undefined, "spend but no leads")
	assert.Contains(t, res.Coverage.DependenciesMissing[0], "funnel mapping")
}
