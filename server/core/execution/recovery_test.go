package execution_test

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/store"
)

// Catches: an intent left in dispatching being sent again after a restart,
// recovery firing on an intent that is still being sent, and the kill
// switch not holding what recovery put back.
func TestRestartResumesSafelyAcrossTheDispatchBoundary(t *testing.T) {
	e := setup(t, nil)
	ctx := t.Context()
	in := e.create(t, e.googleAcc, "google_search", "g", 5_000)
	// Simulate the process dying mid-send: the fake writer created the
	// object, and the row sits in "dispatching".
	e.google.UncertainNext = true
	_, _ = e.d.Tick(ctx)
	require.NoError(t, e.db.Writer().SetProposalStateAny(ctx, store.SetProposalStateAnyParams{State: "executing", UpdatedAt: e.clock.Now(), ID: in.ProposalID}))
	_, err := e.db.Write.ExecContext(ctx, `UPDATE execution_intents SET state = 'dispatching', reconcile_after = NULL WHERE id = ?`, in.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, e.google.Count())

	// Too fresh to be considered interrupted.
	n, err := e.d.RecoverInterrupted(ctx, 5*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	e.clock.Advance(6 * time.Minute)
	n, err = e.d.RecoverInterrupted(ctx, 5*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	got := e.intent(t, in.ID)
	assert.Equal(t, "uncertain", got.State)
	assert.Contains(t, got.LastError, "process stopped")

	// The next tick looks it up and finds the object: no second send.
	_, errs := e.d.Tick(ctx)
	assert.Empty(t, errs)
	got = e.intent(t, in.ID)
	assert.Equal(t, "submitted", got.State)
	assert.Equal(t, 1, e.google.Applies)
	assert.Equal(t, 1, e.google.Count())

	// A kill switch holds everything not yet sent, including after recovery.
	held := e.create(t, e.googleAcc, "google_search", "h", 1_000)
	_, err = e.policies.SetKillSwitch(ctx, coretest.Admin, e.googleAcc, true, "stop")
	require.NoError(t, err)
	_, _ = e.d.Tick(ctx)
	assert.Equal(t, "reserved", e.intent(t, held.ID).State)
	assert.Equal(t, 1, e.google.Applies)
	rev, err := e.d.Reversibility(ctx, coretest.Viewer, held.ID)
	require.NoError(t, err)
	assert.True(t, rev.Reversible)
	assert.Contains(t, rev.How, "nothing reached the platform")
}

// Catches: compensation described as a refund or an undo, compensation
// bypassing policy, compensation of something that never reached the
// platform, and the spend figure disappearing.
func TestSpentMoneyIsNeverDescribedAsUndone(t *testing.T) {
	e := setup(t, nil)
	ctx := t.Context()
	in := e.create(t, e.googleAcc, "google_search", "g", 5_000)
	_, _ = e.d.Tick(ctx)
	require.Equal(t, "submitted", e.intent(t, in.ID).State)

	rev, err := e.d.Reversibility(ctx, coretest.Viewer, in.ID)
	require.NoError(t, err)
	assert.True(t, rev.Reversible)
	require.NotNil(t, rev.SpentMinor)
	assert.Equal(t, int64(5_000), *rev.SpentMinor)
	for _, bad := range []string{"refund", "undo", "undone", "recovered."} {
		assert.False(t, strings.Contains(strings.ToLower(rev.How), bad) && !strings.Contains(strings.ToLower(rev.How), "not recovered"), rev.How)
	}
	assert.Contains(t, rev.How, "nothing spent is recovered")

	_, err = e.d.Compensate(ctx, coretest.Marketer, in.ID, "wrong campaign")
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.d.Compensate(ctx, coretest.Admin, in.ID, "")
	require.Error(t, err)
	comp, err := e.d.Compensate(ctx, coretest.Admin, in.ID, "wrong campaign")
	require.NoError(t, err)
	assert.Equal(t, "reserved", comp.State, "the compensation is in the outbox, not yet sent")
	orig := e.intent(t, in.ID)
	assert.Equal(t, "compensated", orig.State)
	assert.Contains(t, orig.LastError, "nothing already spent is recovered")
	assert.NotContains(t, strings.ToLower(orig.LastError), "refund")
	cp, err := e.props.Get(ctx, coretest.Viewer, comp.ProposalID)
	require.NoError(t, err)
	assert.Equal(t, policies.CampaignPause, cp.Action)
	assert.Equal(t, "adx:1-a", cp.TargetRef, "pauses the campaign that was created")
	assert.Contains(t, cp.Reason, "money already spent is not recovered")
	envs, _ := e.props.ListEnvelopes(ctx, coretest.Viewer)
	for _, en := range envs {
		if en.Channel == "google_search" {
			assert.Equal(t, int64(5_000), en.ConsumedMinor, "consumed stays consumed")
		}
	}
	// The compensation dispatches like any change.
	_, _ = e.d.Tick(ctx)
	assert.Equal(t, "submitted", e.intent(t, comp.ID).State)
	assert.Equal(t, "UPDATED", e.google.Objects["adx:"+itoa(comp.ID)].Status)

	// Something never sent cannot be compensated; it is cancelled instead.
	held := e.create(t, e.googleAcc, "google_search", "h", 1_000)
	_, err = e.d.Compensate(ctx, coretest.Admin, held.ID, "x")
	require.Error(t, err)
	rev, _ = e.d.Reversibility(ctx, coretest.Viewer, held.ID)
	assert.Contains(t, rev.How, "cancelling")

	// Repair of a refused intent: a new proposal, the old one stays failed.
	e.meta.FailNext = &core.Error{Kind: core.KindInvalid, Message: "Meta refused: special_ad_categories is required"}
	m := e.create(t, e.metaAcc, "meta_lead_form", "m", 2_000)
	_, _ = e.d.Tick(ctx)
	require.Equal(t, "failed", e.intent(t, m.ID).State)
	repaired, err := e.d.Repair(ctx, coretest.Marketer, m.ID, json.RawMessage(`{"name":"Wadi launch","dailyBudgetMinor":500000,"specialAdCategories":["HOUSING"]}`), "added the category")
	require.NoError(t, err)
	assert.Equal(t, "approved", repaired.State, "create_paused is allowed for marketers under the suggested defaults")
	assert.Contains(t, repaired.Reason, "repair of execution")
	assert.Equal(t, "failed", e.intent(t, m.ID).State)
	tl, _ := e.d.Timeline(ctx, coretest.Viewer, m.ID)
	assert.Contains(t, tl.Events[len(tl.Events)-1].Detail, "repaired by proposal")
}

// Catches: delivery claimed from a submission or confirmation, the one-hour
// mark reported without impressions, and delivery read from anything but
// the platform's figures.
func TestOneHourIsNotAchievedWithoutDeliveryEvidence(t *testing.T) {
	e := setup(t, nil)
	ctx := t.Context()
	in := e.create(t, e.googleAcc, "google_search", "g", 5_000)
	ms, err := e.d.Milestones(ctx, coretest.Viewer, e.plan, 1)
	require.NoError(t, err)
	assert.NotNil(t, ms.PlanAcceptedAt)
	assert.False(t, ms.WithinOneHour)
	assert.Nil(t, ms.FirstDeliveryDay)
	assert.Contains(t, ms.Note, "none has reached the platform")

	_, _ = e.d.Tick(ctx)
	e.clock.Advance(61 * time.Second)
	_, _ = e.d.Tick(ctx)
	require.Equal(t, "externally_confirmed", e.intent(t, in.ID).State)
	ms, _ = e.d.Milestones(ctx, coretest.Viewer, e.plan, 1)
	assert.NotNil(t, ms.FirstConfirmed)
	assert.False(t, ms.WithinOneHour, "confirmed is not delivered")
	assert.Nil(t, ms.ReadyToDeliveryHours)
	assert.Contains(t, ms.Note, "delivery is not claimed")

	// The platform's figures later show impressions for the created campaign.
	camp, err := e.db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: e.googleAcc, Platform: "google_ads", ExternalID: "adx:1-a", Name: "Wadi [adx:1]", Status: "ENABLED", FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	require.NoError(t, e.db.Writer().UpsertCampaignDailyMetrics(ctx, store.UpsertCampaignDailyMetricsParams{WorkspaceID: 1, AccountID: e.googleAcc, CampaignID: camp.ID, Platform: "google_ads", Day: "2026-10-17", Currency: "AED", CostMinor: 1200, Impressions: 0, Clicks: 0, ObservedAt: e.clock.Now()}))
	ms, _ = e.d.Milestones(ctx, coretest.Viewer, e.plan, 1)
	assert.Nil(t, ms.FirstDeliveryDay, "a day with no impressions is not delivery")
	e.clock.Advance(70 * time.Minute)
	require.NoError(t, e.db.Writer().UpsertCampaignDailyMetrics(ctx, store.UpsertCampaignDailyMetricsParams{WorkspaceID: 1, AccountID: e.googleAcc, CampaignID: camp.ID, Platform: "google_ads", Day: "2026-10-18", Currency: "AED", CostMinor: 3400, Impressions: 120, Clicks: 3, ObservedAt: e.clock.Now()}))
	ms, _ = e.d.Milestones(ctx, coretest.Viewer, e.plan, 1)
	require.NotNil(t, ms.FirstDeliveryDay)
	assert.Equal(t, "2026-10-18", *ms.FirstDeliveryDay)
	require.NotNil(t, ms.ReadyToDeliveryHours)
	assert.Greater(t, *ms.ReadyToDeliveryHours, 1.0)
	assert.False(t, ms.WithinOneHour, "accepted at 09:00, observed after more than an hour of clock")
	assert.Contains(t, ms.Note, "impressions recorded")
	_ = sql.NullTime{}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
