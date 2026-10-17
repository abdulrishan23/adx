package alerts_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/alerts"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	alerts    *alerts.Alerts
	account   int64
	campaigns map[string]int64
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 5, 0, 0, 0, time.UTC)), campaigns: map[string]int64{}}
	coretest.SeedUsers(t, e.db)
	r := reporting.New(e.db, nil, nil, evidence.New(e.db, e.clock), reporting.Config{Clock: e.clock})
	e.alerts = alerts.New(e.db, r, alerts.Config{Clock: e.clock})
	cred, err := e.db.Writer().UpsertCredential(t.Context(), store.UpsertCredentialParams{Provider: "google_ads", Kind: "oauth", Subject: "u", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(t.Context(), store.UpsertAccountParams{Type: "google_ads", ExternalID: "1", CredentialID: cred.ID, Name: sql.NullString{String: "Reportage", Valid: true},
		Currency: sql.NullString{String: "AED", Valid: true}, TimeZone: sql.NullString{String: "Asia/Dubai", Valid: true}, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	e.account = acct.ID
	for _, name := range []string{"Steady", "Spiky", "Quiet"} {
		c, err := e.db.Writer().UpsertAdCampaign(t.Context(), store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: acct.ID, Platform: "google_ads", ExternalID: name, Name: name, Status: "ENABLED",
			Currency: sql.NullString{String: "AED", Valid: true}, FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
		require.NoError(t, err)
		e.campaigns[name] = c.ID
	}
	// Seven baseline days at 100 AED for Steady and Spiky; day 16: Steady 110, Spiky 400, Quiet 50 with no leads.
	for d := 9; d <= 15; d++ {
		day := time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		e.metric(t, "Steady", day, 10000)
		e.metric(t, "Spiky", day, 10000)
	}
	e.metric(t, "Steady", "2026-10-16", 11000)
	e.metric(t, "Spiky", "2026-10-16", 40000)
	e.metric(t, "Quiet", "2026-10-16", 5000)
	return e
}

func (e *env) metric(t *testing.T, campaign, day string, cost int64) {
	t.Helper()
	require.NoError(t, e.db.Writer().UpsertCampaignDailyMetrics(t.Context(), store.UpsertCampaignDailyMetricsParams{WorkspaceID: 1, AccountID: e.account, CampaignID: e.campaigns[campaign], Platform: "google_ads",
		Day: day, Currency: "AED", CostMinor: cost, Impressions: 10, Clicks: 1, ObservedAt: e.clock.Now()}))
}

func (e *env) fresh(t *testing.T, through string, partial bool, at time.Time) {
	t.Helper()
	require.NoError(t, e.db.Writer().UpsertAccountFreshness(t.Context(), store.UpsertAccountFreshnessParams{AccountID: e.account, WorkspaceID: 1,
		LastSuccessAt: sql.NullTime{Time: at, Valid: true}, ObservedThrough: sql.NullString{String: through, Valid: true}, LastPartial: boolInt(partial)}))
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func i(v int) *int       { return &v }
func i64(v int64) *int64 { return &v }

// Catches: checks firing before anyone set a threshold, a spike missed or a
// steady campaign flagged, a day without complete figures being reported as
// zero leads, and the same anomaly opening a new alert on every run.
func TestChecksNeedThresholdsAndCompleteFigures(t *testing.T) {
	e := setup(t)
	e.fresh(t, "2026-10-16", false, e.clock.Now())

	ev, err := e.alerts.Evaluate(t.Context(), coretest.Marketer, "2026-10-16")
	require.NoError(t, err)
	assert.False(t, ev.Evaluated)
	assert.Equal(t, 0, ev.Fired)
	state, err := e.alerts.GetConfig(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	assert.False(t, state.Configured)
	assert.Equal(t, []string{"alert configuration"}, state.Unconfigured)

	// Only the spike threshold is set: zero-lead and stale checks stay off.
	_, err = e.alerts.PutConfig(t.Context(), coretest.Admin, alerts.AlertDocument{CostSpike: alerts.CostSpikeConfig{ThresholdPct: i(50), MinSpendMinor: i64(1000)}})
	require.NoError(t, err)
	ev, err = e.alerts.Evaluate(t.Context(), coretest.Marketer, "2026-10-16")
	require.NoError(t, err)
	assert.True(t, ev.Evaluated)
	assert.Equal(t, 1, ev.Fired, "only Spiky: 400 vs a 100 average")
	assert.Contains(t, ev.Skipped, "stale_ingestion: threshold not configured")
	open, err := e.alerts.List(t.Context(), coretest.Viewer, "open", 10)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, alerts.CheckCostSpike, open[0].Check)
	assert.Equal(t, "Spiky", open[0].SubjectLabel)
	assert.Contains(t, open[0].Message, "300% above")
	assert.Contains(t, open[0].Message, "No change was made")
	assert.Equal(t, int64(1), open[0].ConfigVersion)

	// Re-running the same day adds no occurrence and no alert.
	ev, err = e.alerts.Evaluate(t.Context(), coretest.Marketer, "2026-10-16")
	require.NoError(t, err)
	assert.Equal(t, 0, ev.Fired)
	assert.Equal(t, 1, ev.Repeated)
	open, _ = e.alerts.List(t.Context(), coretest.Viewer, "open", 10)
	require.Len(t, open, 1)
	assert.Equal(t, int64(1), open[0].Occurrences)

	// Spiky spikes again the next day: same alert, one more occurrence.
	e.metric(t, "Spiky", "2026-10-17", 40000)
	e.metric(t, "Steady", "2026-10-17", 10000)
	e.fresh(t, "2026-10-17", false, e.clock.Now())
	ev, err = e.alerts.Evaluate(t.Context(), coretest.Marketer, "2026-10-17")
	require.NoError(t, err)
	assert.Equal(t, 0, ev.Fired)
	assert.Equal(t, 1, ev.Repeated)
	open, _ = e.alerts.List(t.Context(), coretest.Viewer, "open", 10)
	require.Len(t, open, 1)
	assert.Equal(t, int64(2), open[0].Occurrences)

	// Zero leads: Quiet spent 50 AED on the 16th with no lead, and the day
	// is complete, so it fires. Steady and Spiky also have no leads.
	_, err = e.alerts.PutConfig(t.Context(), coretest.Admin, alerts.AlertDocument{CostSpike: alerts.CostSpikeConfig{ThresholdPct: i(50)}, ZeroLeads: alerts.ZeroLeadsConfig{MinSpendMinor: i64(4000)}})
	require.NoError(t, err)
	ev, err = e.alerts.Evaluate(t.Context(), coretest.Marketer, "2026-10-16")
	require.NoError(t, err)
	assert.Equal(t, 3, ev.Fired, "zero_leads for Steady, Spiky and Quiet")
	open, _ = e.alerts.List(t.Context(), coretest.Viewer, "open", 10)
	assert.Len(t, open, 4)

	// A day the figures are not complete for: no zero-lead alert, a skip note.
	e.metric(t, "Quiet", "2026-10-18", 5000)
	e.fresh(t, "2026-10-17", true, e.clock.Now())
	ev, err = e.alerts.Evaluate(t.Context(), coretest.Marketer, "2026-10-18")
	require.NoError(t, err)
	assert.Equal(t, 0, ev.Fired)
	assert.NotEmpty(t, ev.Skipped)
	assert.Contains(t, ev.Skipped[len(ev.Skipped)-1], "not complete yet")
}

// Catches: stale figures reported as a quiet account instead of a stale one,
// and acknowledge/resolve ignoring roles.
func TestStaleIngestionAndLifecycle(t *testing.T) {
	e := setup(t)
	e.fresh(t, "2026-10-14", true, e.clock.Now().Add(-40*time.Hour))
	_, err := e.alerts.PutConfig(t.Context(), coretest.Admin, alerts.AlertDocument{StaleIngestion: alerts.StaleIngestionConfig{MaxAgeHours: i(24)}, ZeroLeads: alerts.ZeroLeadsConfig{MinSpendMinor: i64(1000)}})
	require.NoError(t, err)
	ev, err := e.alerts.Evaluate(t.Context(), coretest.Marketer, "2026-10-16")
	require.NoError(t, err)
	assert.Equal(t, 1, ev.Fired, "stale ingestion only")
	open, err := e.alerts.List(t.Context(), coretest.Viewer, "open", 10)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, alerts.CheckStaleIngestion, open[0].Check)
	assert.Contains(t, open[0].Message, "40 hours ago")
	for _, al := range open {
		assert.NotEqual(t, alerts.CheckZeroLeads, al.Check, "zero leads must not fire on stale data")
	}
	assert.Contains(t, ev.Skipped[len(ev.Skipped)-1], "not complete yet")

	_, err = e.alerts.Acknowledge(t.Context(), coretest.Viewer, open[0].ID)
	assert.ErrorIs(t, err, core.ErrForbidden)
	acked, err := e.alerts.Acknowledge(t.Context(), coretest.Marketer, open[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "acknowledged", acked.State)
	assert.Equal(t, int64(2), *acked.AcknowledgedBy)
	_, err = e.alerts.Acknowledge(t.Context(), coretest.Marketer, open[0].ID)
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	resolved, err := e.alerts.Resolve(t.Context(), coretest.Marketer, open[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "resolved", resolved.State)
	open, _ = e.alerts.List(t.Context(), coretest.Viewer, "open", 10)
	assert.Empty(t, open)

	_, err = e.alerts.PutConfig(t.Context(), coretest.Marketer, alerts.AlertDocument{})
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.alerts.PutConfig(t.Context(), coretest.Admin, alerts.AlertDocument{CostSpike: alerts.CostSpikeConfig{ThresholdPct: i(0)}})
	require.ErrorAs(t, err, &ce)
}
