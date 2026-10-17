package execution_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/execution"
	"github.com/WokeWorks/adx/server/core/execution/executiontest"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type fakeTokens struct{}

func (fakeTokens) ProviderToken(context.Context, core.Actor, int64) (string, error) {
	return "tok", nil
}

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	policies  *policies.Policies
	plans     *plans.Plans
	props     *proposals.Proposals
	google    *executiontest.Writer
	meta      *executiontest.Writer
	d         *execution.Dispatcher
	reportage int64
	wadi      int64
	googleAcc int64
	metaAcc   int64
	plan      int64
	envelopes map[string]int64
}

func setup(t *testing.T, writers map[string]execution.Writer) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)), envelopes: map[string]int64{}}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	e.policies = policies.New(e.db)
	br := brands.New(e.db, brands.Config{Clock: e.clock})
	e.plans = plans.New(e.db, br, plans.Config{Clock: e.clock})
	e.props = proposals.New(e.db, e.policies, e.plans, proposals.Config{Clock: e.clock})
	e.google = executiontest.New("google_ads", policies.CampaignCreatePaus, policies.BudgetRaise, policies.BudgetLower, policies.CampaignPause)
	e.meta = executiontest.New("meta", policies.CampaignCreatePaus, policies.BudgetRaise, policies.CampaignPause)
	if writers == nil {
		writers = map[string]execution.Writer{"google_ads": e.google, "meta": e.meta}
	}
	e.d = execution.New(e.db, e.policies, e.props, fakeTokens{}, writers, execution.Config{Clock: e.clock, MaxAttempts: 3, ReconcileAfter: time.Minute, LiveWritesEnabled: len(writers) > 0})
	b, err := br.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	e.reportage = b.ID
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.wadi = p.ID
	require.NoError(t, br.AssignProject(ctx, coretest.Admin, e.wadi, e.reportage))
	e.googleAcc = e.account(t, "google_ads", "1234567890")
	e.metaAcc = e.account(t, "meta", "act_42")
	for _, acc := range []int64{e.googleAcc, e.metaAcc} {
		for _, c := range []string{"write.budget", "write.status", "write.campaign"} {
			require.NoError(t, e.db.Writer().UpsertAccountCapability(ctx, store.UpsertAccountCapabilityParams{AccountID: acc, Capability: c, State: "verified", CheckedAt: e.clock.Now()}))
		}
	}
	pct, changes := 50, 20
	doc := policies.SuggestedDefaults()
	doc.Caps = policies.PolicyCaps{DailySpendIncreasePct: &pct, MaxChangesPerDay: &changes}
	doc.WriteAccounts = []int64{e.googleAcc, e.metaAcc}
	_, err = e.policies.Put(ctx, coretest.Admin, doc)
	require.NoError(t, err)
	cpl := int64(25000)
	d, err := e.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e.reportage, ProjectID: e.wadi, Title: "Wadi Nov", Document: plans.PlanDocument{Objective: "Leads", BudgetMinor: 20_000, Currency: "AED", PeriodFrom: "2026-11-01", PeriodTo: "2026-11-30",
		Allocations: []plans.Allocation{{Channel: "google_search", Objective: "leads", BudgetMinor: 10_000, Geography: "AE", ExpectedCPLMinor: &cpl}, {Channel: "meta_lead_form", Objective: "leads", BudgetMinor: 10_000, Geography: "AE", ExpectedCPLMinor: &cpl}}}})
	require.NoError(t, err)
	e.plan = d.Plan.ID
	_, err = e.plans.Accept(ctx, coretest.Admin, e.plan, 1)
	require.NoError(t, err)
	envs, err := e.props.OpenEnvelopes(ctx, coretest.Admin, e.plan, 1, map[string]int64{"google_ads": e.googleAcc, "meta": e.metaAcc})
	require.NoError(t, err)
	for _, en := range envs {
		e.envelopes[en.Channel] = en.ID
	}
	return e
}

func (e *env) account(t *testing.T, typ, external string) int64 {
	t.Helper()
	cred, err := e.db.Writer().UpsertCredential(t.Context(), store.UpsertCredentialParams{Provider: typ, Kind: "oauth", Subject: "u-" + typ, EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "x", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(t.Context(), store.UpsertAccountParams{Type: typ, ExternalID: external, CredentialID: cred.ID, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	return acct.ID
}

// create proposes and executes a paused campaign create, returning the intent.
func (e *env) create(t *testing.T, account int64, channel, key string, amount int64) proposals.Intent {
	t.Helper()
	pr, _, err := e.props.Propose(t.Context(), coretest.Admin, proposals.ProposalInput{AccountID: account, Action: policies.CampaignCreatePaus, TargetKind: "campaign", TargetRef: "new:" + key, TargetVersion: "none",
		Proposed: json.RawMessage(`{"name":"Wadi launch","dailyBudgetMinor":500000,"channelType":"SEARCH"}`), Reason: "launch the Wadi plan", AmountMinor: amount, Currency: "AED", EnvelopeID: e.envelopes[channel], IdempotencyKey: key})
	require.NoError(t, err)
	require.Equal(t, "approved", pr.State, pr.PolicyReason)
	in, _, err := e.props.Execute(t.Context(), coretest.Admin, pr.ID, "exec-"+key)
	require.NoError(t, err)
	return in
}

func (e *env) intent(t *testing.T, id int64) proposals.Intent {
	t.Helper()
	in, err := e.props.GetIntent(t.Context(), coretest.Viewer, id)
	require.NoError(t, err)
	return in
}

// Catches: a timeout after a real create being retried into a duplicate, a
// lost request not being retried, and the reconciliation path skipping the
// lookup.
func TestTimeoutAfterCreateDoesNotDuplicateOnRetry(t *testing.T) {
	e := setup(t, nil)
	ctx := t.Context()
	in := e.create(t, e.googleAcc, "google_search", "g1", 5_000)
	assert.Equal(t, "reserved", in.State)

	e.google.UncertainNext = true
	worked, errs := e.d.Tick(ctx)
	assert.Empty(t, errs)
	assert.Equal(t, 1, worked)
	in = e.intent(t, in.ID)
	assert.Equal(t, "uncertain", in.State)
	assert.Equal(t, int64(1), in.Attempts)
	assert.Equal(t, 1, e.google.Count(), "the create landed")
	assert.Empty(t, in.ProviderRefs, "no response, no refs")

	// Not due yet: nothing happens. A minute later it is looked up, found,
	// and recorded as submitted without a second Apply.
	_, _ = e.d.Tick(ctx)
	assert.Equal(t, 1, e.google.Applies)
	e.clock.Advance(61 * time.Second)
	_, errs = e.d.Tick(ctx)
	assert.Empty(t, errs)
	in = e.intent(t, in.ID)
	assert.Equal(t, "submitted", in.State)
	assert.Equal(t, 1, e.google.Applies, "no retry of a create that landed")
	assert.Equal(t, 1, e.google.Lookups)
	assert.Equal(t, 1, e.google.Count())
	require.Len(t, in.ProviderRefs, 1)
	assert.Equal(t, "adx:1-a", in.ProviderRefs[0].ExternalID)
	envs, _ := e.props.ListEnvelopes(ctx, coretest.Viewer)
	for _, en := range envs {
		if en.Channel == "google_search" {
			assert.Equal(t, int64(5_000), en.ConsumedMinor, "submitted spend is consumed, not reserved")
		}
	}

	// A lost request (nothing landed) is retried once the lookup finds nothing.
	in2 := e.create(t, e.googleAcc, "google_search", "g2", 1_000)
	e.google.LostNext = true
	_, _ = e.d.Tick(ctx)
	assert.Equal(t, "uncertain", e.intent(t, in2.ID).State)
	e.clock.Advance(61 * time.Second)
	_, _ = e.d.Tick(ctx)
	assert.Equal(t, "reserved", e.intent(t, in2.ID).State, "not found: back to the outbox")
	_, _ = e.d.Tick(ctx)
	got := e.intent(t, in2.ID)
	assert.Equal(t, "submitted", got.State)
	assert.Equal(t, int64(2), got.Attempts)
	assert.Equal(t, 2, e.google.Count(), "one object for g1, one for g2")

	// Submitted is not confirmed: verification reads it back later.
	assert.Nil(t, got.ConfirmedAt)
	_, _ = e.d.Tick(ctx)
	assert.Equal(t, "submitted", e.intent(t, got.ID).State, "not verified in the same minute")
	e.clock.Advance(61 * time.Second)
	_, _ = e.d.Tick(ctx)
	got = e.intent(t, got.ID)
	assert.Equal(t, "externally_confirmed", got.State)
	assert.NotNil(t, got.ConfirmedAt)
	tl, err := e.d.Timeline(ctx, coretest.Viewer, got.ID)
	require.NoError(t, err)
	states := []string{}
	for _, ev := range tl.Events {
		states = append(states, ev.ToState)
	}
	assert.Equal(t, []string{"dispatching", "uncertain", "reconciling", "reserved", "dispatching", "submitted", "externally_confirmed"}, states)
}

// Catches: a launch with one platform failed being reported as confirmed or
// submitted, a submission being reported as delivery, and a failed intent
// keeping its budget.
func TestPartialSuccessAcrossChannelsStaysPartial(t *testing.T) {
	e := setup(t, nil)
	ctx := t.Context()
	g := e.create(t, e.googleAcc, "google_search", "g", 5_000)
	m := e.create(t, e.metaAcc, "meta_lead_form", "m", 5_000)
	e.meta.FailNext = &core.Error{Kind: core.KindConflict, Message: "Meta access was rejected; check the app roles and permissions (code 200)"}
	_, errs := e.d.Tick(ctx)
	assert.Empty(t, errs)
	assert.Equal(t, "submitted", e.intent(t, g.ID).State)
	mi := e.intent(t, m.ID)
	assert.Equal(t, "failed", mi.State)
	assert.Contains(t, mi.LastError, "Meta access was rejected")
	ls, err := e.d.Launch(ctx, coretest.Viewer, e.plan, 1)
	require.NoError(t, err)
	assert.Equal(t, "partial", ls.State)
	assert.False(t, ls.Delivered)
	byP := map[string]string{}
	for _, p := range ls.Platforms {
		byP[p.Platform] = p.State
	}
	assert.Equal(t, "submitted", byP["google_ads"])
	assert.Equal(t, "failed", byP["meta"])
	envs, _ := e.props.ListEnvelopes(ctx, coretest.Viewer)
	for _, en := range envs {
		if en.Channel == "meta_lead_form" {
			assert.Equal(t, int64(0), en.ReservedMinor, "a failed intent gives its budget back")
			assert.Equal(t, int64(10_000), en.RemainingMinor)
		}
	}
	// Verification confirms Google; the launch is still partial, never
	// "confirmed" while Meta failed, and never delivered.
	e.clock.Advance(61 * time.Second)
	_, _ = e.d.Tick(ctx)
	ls, _ = e.d.Launch(ctx, coretest.Viewer, e.plan, 1)
	assert.Equal(t, "partial", ls.State)
	assert.False(t, ls.Delivered)
	assert.Contains(t, ls.Note, "Neither is delivery")

	// Retry Meta: reserved again, dispatched, submitted; the launch becomes
	// submitted/confirmed only now.
	_, err = e.d.Retry(ctx, coretest.Marketer, m.ID)
	assert.ErrorIs(t, err, core.ErrForbidden)
	retried, err := e.d.Retry(ctx, coretest.Admin, m.ID)
	require.NoError(t, err)
	assert.Equal(t, "reserved", retried.State)
	_, _ = e.d.Tick(ctx)
	assert.Equal(t, "submitted", e.intent(t, m.ID).State)
	ls, _ = e.d.Launch(ctx, coretest.Viewer, e.plan, 1)
	assert.Equal(t, "submitted", ls.State, "Google confirmed, Meta submitted: the whole is submitted, not confirmed")
	e.clock.Advance(61 * time.Second)
	_, _ = e.d.Tick(ctx)
	ls, _ = e.d.Launch(ctx, coretest.Viewer, e.plan, 1)
	assert.Equal(t, "confirmed", ls.State)
	assert.False(t, ls.Delivered, "confirmed existence is still not delivery")
}

// Catches: an intent being sent while no live adapter is enabled, an
// unimplemented action being sent, a kill switch not holding undispatched
// work, and a hold being recorded as a failure.
func TestUnauthorizedLiveAccountStaysDisabled(t *testing.T) {
	// No writers at all: the server has live writes off.
	e := setup(t, map[string]execution.Writer{})
	ctx := t.Context()
	in := e.create(t, e.googleAcc, "google_search", "g", 1_000)
	_, errs := e.d.Tick(ctx)
	assert.Empty(t, errs)
	got := e.intent(t, in.ID)
	assert.Equal(t, "reserved", got.State, "held, not failed and not sent")
	assert.Contains(t, got.LastError, "no live write adapter")
	assert.Equal(t, int64(0), got.Attempts)
	caps := e.d.Capabilities()
	for _, c := range caps {
		assert.False(t, c.Live)
		assert.Contains(t, c.Note, "ADX_LIVE_WRITES")
		assert.Empty(t, c.Implemented)
	}

	// With a fake writer that only knows some actions, the rest fail honestly.
	e2 := setup(t, nil)
	pr, _, err := e2.props.Propose(ctx, coretest.Admin, proposals.ProposalInput{AccountID: e2.googleAcc, Action: policies.AudiencesUpsert, TargetKind: "audience", TargetRef: "new", TargetVersion: "none", Proposed: json.RawMessage(`{"name":"Buyers"}`), Reason: "test", IdempotencyKey: "aud"})
	require.NoError(t, err)
	assert.Equal(t, "denied", pr.State, "write.audiences is not a verified capability here")
	require.NoError(t, e2.db.Writer().UpsertAccountCapability(ctx, store.UpsertAccountCapabilityParams{AccountID: e2.googleAcc, Capability: "write.audiences", State: "verified", CheckedAt: e2.clock.Now()}))
	pr, _, err = e2.props.Propose(ctx, coretest.Admin, proposals.ProposalInput{AccountID: e2.googleAcc, Action: policies.AudiencesUpsert, TargetKind: "audience", TargetRef: "new", TargetVersion: "none", Proposed: json.RawMessage(`{"name":"Buyers"}`), Reason: "test", IdempotencyKey: "aud2"})
	require.NoError(t, err)
	require.Equal(t, "approved", pr.State)
	ai, _, err := e2.props.Execute(ctx, coretest.Admin, pr.ID, "aud-exec")
	require.NoError(t, err)
	_, _ = e2.d.Tick(ctx)
	got = e2.intent(t, ai.ID)
	assert.Equal(t, "failed", got.State)
	assert.Contains(t, got.LastError, "not implemented for google_ads")
	assert.Equal(t, 0, e2.google.Applies)
	caps = e2.d.Capabilities()
	for _, c := range caps {
		if c.Platform == "google_ads" {
			assert.Contains(t, c.Implemented, "campaign.create_paused")
			assert.Contains(t, c.NotImplemented, "audiences.upsert")
		}
	}

	// A kill switch holds anything not yet sent; clearing it lets it go.
	held := e2.create(t, e2.googleAcc, "google_search", "held", 1_000)
	_, err = e2.policies.SetKillSwitch(ctx, coretest.Admin, 0, true, "incident")
	require.NoError(t, err)
	_, _ = e2.d.Tick(ctx)
	got = e2.intent(t, held.ID)
	assert.Equal(t, "reserved", got.State)
	assert.Contains(t, got.LastError, "kill_switch")
	assert.Equal(t, 0, e2.google.Applies)
	_, err = e2.policies.SetKillSwitch(ctx, coretest.Admin, 0, false, "")
	require.NoError(t, err)
	e2.clock.Advance(11 * time.Minute)
	_, _ = e2.d.Tick(ctx)
	assert.Equal(t, "submitted", e2.intent(t, held.ID).State)
	assert.Equal(t, 1, e2.google.Applies)

	// Reconcile now, by hand; the assistant can't.
	asModel := coretest.Admin
	asModel.Via = core.ViaAssistant
	_, err = e2.d.Reconcile(ctx, asModel, held.ID)
	assert.ErrorIs(t, err, core.ErrForbidden)
	now, err := e2.d.Reconcile(ctx, coretest.Marketer, held.ID)
	require.NoError(t, err)
	assert.Equal(t, "externally_confirmed", now.State)
	var ce *core.Error
	_, err = e2.d.Reconcile(ctx, coretest.Marketer, held.ID)
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
}
