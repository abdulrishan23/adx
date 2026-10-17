package reporting_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// fakeProvider serves campaigns and metrics in pages and counts every page
// it was asked for, so the test can prove each was read exactly once.
type fakeProvider struct {
	platform     string
	campaigns    [][]reporting.Campaign
	metrics      [][]reporting.DailyMetric
	actions      []reporting.ConversionAction
	failMetrics  int // fail this metrics page (1-based) once
	campaignHits map[string]int
	metricHits   map[string]int
}

func (f *fakeProvider) Platform() string { return f.platform }
func (f *fakeProvider) ListAccounts(context.Context, string) ([]reporting.ProviderAccount, error) {
	return nil, nil
}
func (f *fakeProvider) ListCampaigns(_ context.Context, token, _ string, cursor string) (reporting.Page[reporting.Campaign], error) {
	if token != "tok" {
		return reporting.Page[reporting.Campaign]{}, core.Conflict("Google Ads rejected the credentials")
	}
	if f.campaignHits == nil {
		f.campaignHits = map[string]int{}
	}
	f.campaignHits[cursor]++
	idx := index(cursor)
	page := reporting.Page[reporting.Campaign]{Items: f.campaigns[idx]}
	if idx+1 < len(f.campaigns) {
		page.Next = fmt.Sprintf("p%d", idx+1)
	}
	return page, nil
}
func (f *fakeProvider) DailyMetrics(_ context.Context, _, _ string, _, _ reporting.Day, cursor string) (reporting.Page[reporting.DailyMetric], error) {
	if f.metricHits == nil {
		f.metricHits = map[string]int{}
	}
	f.metricHits[cursor]++
	idx := index(cursor)
	if f.failMetrics == idx+1 {
		f.failMetrics = 0
		return reporting.Page[reporting.DailyMetric]{}, core.Conflict("Google Ads quota exhausted; retry later")
	}
	page := reporting.Page[reporting.DailyMetric]{Items: f.metrics[idx]}
	if idx+1 < len(f.metrics) {
		page.Next = fmt.Sprintf("p%d", idx+1)
	}
	return page, nil
}
func (f *fakeProvider) ListConversionActions(context.Context, string, string) ([]reporting.ConversionAction, error) {
	return f.actions, nil
}

func index(cursor string) int {
	if cursor == "" {
		return 0
	}
	var i int
	_, _ = fmt.Sscanf(cursor, "p%d", &i)
	return i
}

type fakeTokens struct{}

func (fakeTokens) ProviderToken(context.Context, core.Actor, int64) (string, error) {
	return "tok", nil
}

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	google    *fakeProvider
	meta      *fakeProvider
	reporting *reporting.Reporting
	runs      *runs.Runs
	googleAcc int64
	metaAcc   int64
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)),
		google: &fakeProvider{platform: "google_ads"}, meta: &fakeProvider{platform: "meta"}}
	coretest.SeedUsers(t, e.db)
	e.runs = runs.New(e.db, nil, runs.Config{Clock: e.clock})
	e.reporting = reporting.New(e.db, map[string]reporting.Provider{"google_ads": e.google, "meta": e.meta}, fakeTokens{}, evidence.New(e.db, e.clock), reporting.Config{Clock: e.clock})
	e.googleAcc = e.account(t, "google_ads", "1234567890", "AED", "Asia/Dubai")
	e.metaAcc = e.account(t, "meta", "act_42", "AED", "Asia/Dubai")
	return e
}

func (e *env) account(t *testing.T, typ, external, currency, tz string) int64 {
	t.Helper()
	cred, err := e.db.Writer().UpsertCredential(t.Context(), store.UpsertCredentialParams{Provider: typ, Kind: "oauth", Subject: "user-" + typ,
		EncryptedSecret: []byte("sealed"), KeyVersion: 1, Scopes: "x", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(t.Context(), store.UpsertAccountParams{WorkspaceID: core.DefaultWorkspace, Type: typ, ExternalID: external, CredentialID: cred.ID,
		Currency: sql.NullString{String: currency, Valid: currency != ""}, TimeZone: sql.NullString{String: tz, Valid: tz != ""}, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	return acct.ID
}

func (e *env) refresh(t *testing.T, account int64, from, to reporting.Day) runs.Run {
	t.Helper()
	run, _, err := e.reporting.StartRefresh(t.Context(), coretest.Marketer, e.runs, account, from, to)
	require.NoError(t, err)
	h, err := e.runs.Begin(t.Context(), run.RowID(), "test")
	require.NoError(t, err)
	completion, execErr := e.reporting.Execute(t.Context(), h)
	if execErr != nil {
		require.NoError(t, h.Finish(t.Context(), runs.Completion{State: runs.StateFailed, Error: execErr.Error()}))
	} else {
		require.NoError(t, h.Finish(t.Context(), completion))
	}
	got, err := e.runs.Get(t.Context(), coretest.Viewer, run.ID)
	require.NoError(t, err)
	return got
}

func f64(v float64) *float64 { return &v }
func i64(v int64) *int64     { return &v }

// Catches: a page read twice or skipped, a retry starting over, a failed
// page losing what came before it, rows for unknown campaigns being invented,
// and a read that includes today being called complete.
func TestFakePaginatedProviderImportsEveryPageOnce(t *testing.T) {
	e := setup(t)
	e.google.campaigns = [][]reporting.Campaign{
		{{ExternalID: "c1", Name: "Wadi Search", Status: "ENABLED", ChannelType: "SEARCH", BudgetMinor: i64(50000), BudgetPeriod: "daily"}},
		{{ExternalID: "c2", Name: "Oceana PMax", Status: "PAUSED", ChannelType: "PERFORMANCE_MAX"}},
	}
	e.google.metrics = [][]reporting.DailyMetric{
		{{CampaignExternalID: "c1", Day: "2026-10-14", Currency: "AED", CostMinor: 12345, Impressions: 1000, Clicks: 50, Conversions: f64(3), ConversionsValue: i64(0)},
			{CampaignExternalID: "c2", Day: "2026-10-14", Currency: "AED", CostMinor: 500, Impressions: 10, Clicks: 1}},
		{{CampaignExternalID: "c1", Day: "2026-10-15", Currency: "AED", CostMinor: 20000, Impressions: 2000, Clicks: 80, Conversions: f64(4.5)},
			{CampaignExternalID: "ghost", Day: "2026-10-15", Currency: "AED", CostMinor: 1, Impressions: 1, Clicks: 0}},
	}
	e.google.actions = []reporting.ConversionAction{{ExternalID: "ca1", Name: "Lead form", Type: "WEBPAGE", Category: "SUBMIT_LEAD_FORM", Status: "ENABLED", CountingType: "ONE_PER_CLICK", IncludeInConversions: true}}

	// Page 2 of metrics fails first time.
	e.google.failMetrics = 2
	failed := e.refresh(t, e.googleAcc, "2026-10-14", "2026-10-15")
	assert.Equal(t, runs.StateFailed, failed.State)
	assert.Contains(t, *failed.Error, "quota")
	var progress reporting.RefreshProgress
	require.NoError(t, json.Unmarshal(failed.Progress, &progress))
	assert.Equal(t, "metrics", progress.Stage)
	assert.Equal(t, "p1", progress.Cursor)
	assert.Equal(t, 2, progress.Campaigns)
	assert.Equal(t, 2, progress.MetricRows, "page 1 was kept")
	fresh, err := e.reporting.ListFreshness(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	require.Len(t, fresh, 1)
	assert.True(t, fresh[0].LastPartial)
	assert.Nil(t, fresh[0].LastSuccessAt)
	assert.Contains(t, *fresh[0].LastError, "quota")

	retried := e.refresh(t, e.googleAcc, "2026-10-14", "2026-10-15")
	assert.Equal(t, runs.StateSucceeded, retried.State)
	assert.Equal(t, runs.CompletenessComplete, retried.Completeness)
	assert.Equal(t, map[string]int{"": 1, "p1": 1}, e.google.campaignHits, "campaigns read once; the retry skipped them")
	assert.Equal(t, map[string]int{"": 1, "p1": 2}, e.google.metricHits, "page 2 was asked for twice: once failed, once read")
	require.NoError(t, json.Unmarshal(retried.Progress, &progress))
	assert.Equal(t, 3, progress.MetricRows)
	assert.Equal(t, 1, progress.UnknownCampaign, "the ghost campaign's row is counted, not invented")
	assert.Equal(t, reporting.Day("2026-10-15"), progress.ThroughDay)

	n, err := e.db.Reader().CountMetricRows(t.Context(), e.googleAcc)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
	rows, err := e.db.Reader().ListCampaignDailyMetrics(t.Context(), store.ListCampaignDailyMetricsParams{WorkspaceID: 1, FromDay: "2026-10-14", ToDay: "2026-10-15"})
	require.NoError(t, err)
	require.Len(t, rows, 3)
	byName := map[string]store.ListCampaignDailyMetricsRow{}
	for _, r := range rows {
		byName[r.CampaignName+"/"+r.Day] = r
	}
	assert.Equal(t, int64(12345), byName["Wadi Search/2026-10-14"].CostMinor)
	assert.Equal(t, "google_ads", byName["Wadi Search/2026-10-14"].Platform)
	assert.False(t, byName["Oceana PMax/2026-10-14"].Conversions.Valid, "no claim means null, not zero")
	actions, err := e.db.Reader().ListConversionActions(t.Context(), e.googleAcc)
	require.NoError(t, err)
	require.Len(t, actions, 1)
	fresh, err = e.reporting.ListFreshness(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	assert.False(t, fresh[0].LastPartial)
	assert.Equal(t, reporting.Day("2026-10-15"), *fresh[0].ObservedThrough)
	assert.Nil(t, fresh[0].LastError)

	// Re-reading replaces revised rows instead of duplicating them.
	e.google.metrics[0][0].CostMinor = 12999
	e.refresh(t, e.googleAcc, "2026-10-14", "2026-10-14")
	n, err = e.db.Reader().CountMetricRows(t.Context(), e.googleAcc)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)
	rows, err = e.db.Reader().ListCampaignDailyMetrics(t.Context(), store.ListCampaignDailyMetricsParams{WorkspaceID: 1, FromDay: "2026-10-14", ToDay: "2026-10-14"})
	require.NoError(t, err)
	for _, r := range rows {
		if r.CampaignName == "Wadi Search" {
			assert.Equal(t, int64(12999), r.CostMinor)
		}
	}

	// A range that includes today (Dubai) is partial and observed through yesterday.
	e.google.metrics = [][]reporting.DailyMetric{{{CampaignExternalID: "c1", Day: "2026-10-17", Currency: "AED", CostMinor: 1, Impressions: 1, Clicks: 1}}}
	today := e.refresh(t, e.googleAcc, "2026-10-16", "2026-10-17")
	assert.Equal(t, runs.StatePartial, today.State)
	fresh, err = e.reporting.ListFreshness(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	assert.True(t, fresh[0].LastPartial)
	assert.Equal(t, reporting.Day("2026-10-16"), *fresh[0].ObservedThrough)
}

// Catches: Google and Meta conversions landing in one pile.
func TestPlatformClaimsStaySeparate(t *testing.T) {
	e := setup(t)
	e.google.campaigns = [][]reporting.Campaign{{{ExternalID: "g1", Name: "Wadi Search", Status: "ENABLED"}}}
	e.google.metrics = [][]reporting.DailyMetric{{{CampaignExternalID: "g1", Day: "2026-10-10", Currency: "AED", CostMinor: 100, Impressions: 1, Clicks: 1, Conversions: f64(2)}}}
	e.meta.campaigns = [][]reporting.Campaign{{{ExternalID: "m1", Name: "Wadi Leads", Status: "ACTIVE", ChannelType: "OUTCOME_LEADS"}}}
	e.meta.metrics = [][]reporting.DailyMetric{{{CampaignExternalID: "m1", Day: "2026-10-10", Currency: "AED", CostMinor: 300, Impressions: 1, Clicks: 1, PlatformLeads: f64(5), Reach: i64(900)}}}
	assert.Equal(t, runs.StateSucceeded, e.refresh(t, e.googleAcc, "2026-10-10", "2026-10-10").State)
	assert.Equal(t, runs.StateSucceeded, e.refresh(t, e.metaAcc, "2026-10-10", "2026-10-10").State)

	rows, err := e.db.Reader().ListCampaignDailyMetrics(t.Context(), store.ListCampaignDailyMetricsParams{WorkspaceID: 1, FromDay: "2026-10-10", ToDay: "2026-10-10"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	byPlatform := map[string]store.ListCampaignDailyMetricsRow{}
	for _, r := range rows {
		byPlatform[r.Platform] = r
	}
	assert.Equal(t, 2.0, byPlatform["google_ads"].Conversions.Float64)
	assert.False(t, byPlatform["google_ads"].PlatformLeads.Valid)
	assert.Equal(t, 5.0, byPlatform["meta"].PlatformLeads.Float64)
	assert.False(t, byPlatform["meta"].Conversions.Valid)
	assert.Equal(t, int64(900), byPlatform["meta"].Reach.Int64)

	metaOnly, err := e.db.Reader().ListCampaignDailyMetrics(t.Context(), store.ListCampaignDailyMetricsParams{WorkspaceID: 1, FromDay: "2026-10-10", ToDay: "2026-10-10", Platform: sql.NullString{String: "meta", Valid: true}})
	require.NoError(t, err)
	assert.Len(t, metaOnly, 1)
}

func TestRefreshValidation(t *testing.T) {
	e := setup(t)
	_, _, err := e.reporting.StartRefresh(t.Context(), coretest.Viewer, e.runs, e.googleAcc, "2026-10-01", "2026-10-02")
	assert.ErrorIs(t, err, core.ErrForbidden)
	var ce *core.Error
	_, _, err = e.reporting.StartRefresh(t.Context(), coretest.Marketer, e.runs, e.googleAcc, "2026-10-02", "2026-10-01")
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindInvalid, ce.Kind)
	_, _, err = e.reporting.StartRefresh(t.Context(), coretest.Marketer, e.runs, 999, "2026-10-01", "2026-10-02")
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindNotFound, ce.Kind)

	unconfigured := reporting.New(e.db, nil, nil, evidence.New(e.db, e.clock), reporting.Config{Clock: e.clock})
	run, _, err := unconfigured.StartRefresh(t.Context(), coretest.Marketer, e.runs, e.googleAcc, "2026-10-01", "2026-10-02")
	require.NoError(t, err)
	h, err := e.runs.Begin(t.Context(), run.RowID(), "test")
	require.NoError(t, err)
	_, err = unconfigured.Execute(t.Context(), h)
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
}
