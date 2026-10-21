package proposals_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/core/proposals/proposalstest"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	policies  *policies.Policies
	plans     *plans.Plans
	props     *proposals.Proposals
	brands    *brands.Brands
	reportage int64
	wadi      int64
	googleAcc int64
	plan      int64
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	e.policies = policies.New(e.db)
	e.brands = brands.New(e.db, brands.Config{Clock: e.clock})
	e.plans = plans.New(e.db, e.brands, plans.Config{Clock: e.clock})
	e.props = proposals.New(e.db, e.policies, e.plans, proposals.Config{Clock: e.clock, Coverage: proposalstest.Planned{}})
	b, err := e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	e.reportage = b.ID
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.wadi = p.ID
	require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, e.wadi, e.reportage))
	cred, err := e.db.Writer().UpsertCredential(ctx, store.UpsertCredentialParams{Provider: "google_ads", Kind: "oauth", Subject: "u", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "x", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(ctx, store.UpsertAccountParams{WorkspaceID: core.DefaultWorkspace, Type: "google_ads", ExternalID: "1234567890", CredentialID: cred.ID, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	e.googleAcc = acct.ID
	require.NoError(t, e.db.Writer().UpsertAccountCapability(ctx, store.UpsertAccountCapabilityParams{AccountID: acct.ID, Capability: "write.budget", State: "verified", CheckedAt: e.clock.Now()}))
	require.NoError(t, e.db.Writer().UpsertAccountCapability(ctx, store.UpsertAccountCapabilityParams{AccountID: acct.ID, Capability: "write.status", State: "verified", CheckedAt: e.clock.Now()}))
	cpl := int64(25000)
	d, err := e.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e.reportage, ProjectID: e.wadi, Title: "Wadi Nov", Document: plans.PlanDocument{Objective: "Leads", BudgetMinor: 10_000, Currency: "AED", PeriodFrom: "2026-11-01", PeriodTo: "2026-11-30",
		Allocations: []plans.Allocation{{Channel: "google_search", Objective: "leads", BudgetMinor: 10_000, Geography: "AE", ExpectedCPLMinor: &cpl}}}})
	require.NoError(t, err)
	e.plan = d.Plan.ID
	return e
}

func (e *env) enablePolicy(t *testing.T) {
	t.Helper()
	pct, changes := 50, 20
	doc := policies.SuggestedDefaults()
	doc.Caps = policies.PolicyCaps{DailySpendIncreasePct: &pct, MaxChangesPerDay: &changes}
	doc.WriteAccounts = []int64{e.googleAcc}
	_, err := e.policies.Put(t.Context(), coretest.Admin, doc)
	require.NoError(t, err)
}

func (e *env) envelope(t *testing.T) proposals.Envelope {
	t.Helper()
	_, err := e.plans.Accept(t.Context(), coretest.Admin, e.plan, 1)
	require.NoError(t, err)
	envs, err := e.props.OpenEnvelopes(t.Context(), coretest.Admin, e.plan, 1, map[string]int64{"google_ads": e.googleAcc})
	require.NoError(t, err)
	require.Len(t, envs, 1)
	return envs[0]
}

func raise(amount int64, env int64, key string) proposals.ProposalInput {
	return proposals.ProposalInput{AccountID: 0, Action: policies.BudgetRaise, TargetKind: "campaign", TargetRef: "111", TargetVersion: "budget:500000",
		ExpectedCurrent: json.RawMessage(`{"dailyBudgetMinor":500000}`), Proposed: json.RawMessage(`{"dailyBudgetMinor":600000}`), Reason: "CPL is below target and the search campaign is budget-limited",
		AmountMinor: amount, Currency: "AED", EnvelopeID: env, IdempotencyKey: key}
}

// Catches: two reservations both taking the same remaining budget, a
// repeated execute making a second intent, a cancel not freeing budget,
// and an envelope on an unaccepted plan.
func TestTwoAgentsCannotReserveTheSameBudgetTwice(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.enablePolicy(t)
	_, err := e.props.OpenEnvelopes(ctx, coretest.Admin, e.plan, 1, map[string]int64{"google_ads": e.googleAcc})
	require.Error(t, err, "the plan is not accepted")
	env := e.envelope(t)
	assert.Equal(t, int64(10_000), env.AmountMinor)
	assert.Equal(t, int64(10_000), env.RemainingMinor)

	a := raise(6_000, env.ID, "raise-a")
	a.AccountID = e.googleAcc
	pa, created, err := e.props.Propose(ctx, coretest.Admin, a)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "approved", pa.State, "an admin's own allowed action is self-approved: %s", pa.PolicyReason)
	b := raise(6_000, env.ID, "raise-b")
	b.AccountID = e.googleAcc
	pb, _, err := e.props.Propose(ctx, coretest.Admin, b)
	require.NoError(t, err)

	ia, created, err := e.props.Execute(ctx, coretest.Admin, pa.ID, "exec-a")
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "reserved", ia.State)
	require.NotNil(t, ia.ReservationID)
	_, _, err = e.props.Execute(ctx, coretest.Admin, pb.ID, "exec-b")
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.Contains(t, ce.Message, "insufficient remaining budget")
	again, created, err := e.props.Execute(ctx, coretest.Admin, pa.ID, "exec-a")
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, ia.ID, again.ID)
	envs, err := e.props.ListEnvelopes(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.Equal(t, int64(6_000), envs[0].ReservedMinor)
	assert.Equal(t, int64(4_000), envs[0].RemainingMinor)

	cancelled, err := e.props.Cancel(ctx, coretest.Admin, ia.ID, "changed our mind")
	require.NoError(t, err)
	assert.Equal(t, "cancelled", cancelled.State)
	ib, _, err := e.props.Execute(ctx, coretest.Admin, pb.ID, "exec-b")
	require.NoError(t, err)
	assert.Equal(t, "reserved", ib.State)

	// Ten agents race for 30 each against 40 remaining: one wins.
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		in := raise(3_000, env.ID, "race-"+strings.Repeat("x", i+1))
		in.AccountID = e.googleAcc
		pr, _, err := e.props.Propose(ctx, coretest.Admin, in)
		require.NoError(t, err)
		wg.Add(1)
		go func(id int64, key string) {
			defer wg.Done()
			_, _, err := e.props.Execute(context.Background(), coretest.Admin, id, key)
			results <- err
		}(pr.ID, "race-exec-"+in.IdempotencyKey)
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		}
	}
	assert.Equal(t, 1, wins, "40 remaining fits one 30, not two")
	envs, _ = e.props.ListEnvelopes(ctx, coretest.Viewer)
	assert.Equal(t, int64(9_000), envs[0].ReservedMinor)
}

// Catches: a proposal executing with no policy, with the account not
// enabled for writes, with no approval where one is needed, with a kill
// switch engaged after approval, and with an unproven capability.
func TestMissingAuthorityDeniesExecution(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	in := raise(0, 0, "no-policy")
	in.AccountID = e.googleAcc
	pr, _, err := e.props.Propose(ctx, coretest.Marketer, in)
	require.NoError(t, err)
	assert.Equal(t, "denied", pr.State)
	assert.Equal(t, policies.ReasonPolicyUnconfigured, pr.PolicyReason)
	_, _, err = e.props.Execute(ctx, coretest.Admin, pr.ID, "x")
	require.Error(t, err, "a denied proposal never executes")

	pct, changes := 50, 20
	doc := policies.SuggestedDefaults()
	doc.Caps = policies.PolicyCaps{DailySpendIncreasePct: &pct, MaxChangesPerDay: &changes}
	_, err = e.policies.Put(ctx, coretest.Admin, doc)
	require.NoError(t, err)
	in.IdempotencyKey = "not-enabled"
	pr, _, err = e.props.Propose(ctx, coretest.Marketer, in)
	require.NoError(t, err)
	assert.Equal(t, "denied", pr.State)
	assert.Equal(t, policies.ReasonAccountNotEnabled, pr.PolicyReason)

	e.enablePolicy(t)
	// A marketer's budget raise needs approval.
	in.IdempotencyKey = "needs-approval"
	pr, _, err = e.props.Propose(ctx, coretest.Marketer, in)
	require.NoError(t, err)
	assert.Equal(t, "proposed", pr.State)
	assert.Equal(t, policies.ReasonOK, pr.PolicyReason)
	_, _, err = e.props.Execute(ctx, coretest.Marketer, pr.ID, "k1")
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
	_, err = e.props.Approve(ctx, coretest.Marketer, pr.ID, "")
	assert.ErrorIs(t, err, core.ErrForbidden)
	approved, err := e.props.Approve(ctx, coretest.Admin, pr.ID, "fine")
	require.NoError(t, err)
	assert.Equal(t, "approved", approved.State)
	require.Len(t, approved.Approvals, 1)
	assert.Equal(t, "budget:500000", approved.Approvals[0].TargetVersion)

	// Kill switch after approval: execution refused, nothing reserved.
	_, err = e.policies.SetKillSwitch(ctx, coretest.Admin, 0, true, "incident")
	require.NoError(t, err)
	_, _, err = e.props.Execute(ctx, coretest.Marketer, pr.ID, "k1")
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, policies.ReasonKillSwitch)
	intents, err := e.props.ListIntents(ctx, coretest.Viewer, "", 10)
	require.NoError(t, err)
	assert.Empty(t, intents)
	_, err = e.policies.SetKillSwitch(ctx, coretest.Admin, 0, false, "")
	require.NoError(t, err)
	it, created, err := e.props.Execute(ctx, coretest.Marketer, pr.ID, "k1")
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "reserved", it.State)
	assert.Nil(t, it.ReservationID, "no amount, no reservation")

	// An action whose write capability is unproven is denied.
	bid := raise(0, 0, "bids")
	bid.AccountID = e.googleAcc
	bid.Action = policies.BidsChange
	pr, _, err = e.props.Propose(ctx, coretest.Admin, bid)
	require.NoError(t, err)
	assert.Equal(t, "denied", pr.State)
	assert.Equal(t, policies.ReasonCapabilityUnproven, pr.PolicyReason)
	// Approving a proposed one once policy denies it is refused.
	in.IdempotencyKey = "late"
	pr, _, err = e.props.Propose(ctx, coretest.Marketer, in)
	require.NoError(t, err)
	_, err = e.policies.SetKillSwitch(ctx, coretest.Admin, e.googleAcc, true, "account incident")
	require.NoError(t, err)
	_, err = e.props.Approve(ctx, coretest.Admin, pr.ID, "")
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, ce.Message, policies.ReasonKillSwitch)
}

// Catches: an approval surviving a change to the target, a stale proposal
// executing, and the approval record disappearing.
func TestChangingTheTargetInvalidatesTheApproval(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.enablePolicy(t)
	in := raise(0, 0, "raise")
	in.AccountID = e.googleAcc
	pr, _, err := e.props.Propose(ctx, coretest.Marketer, in)
	require.NoError(t, err)
	approved, err := e.props.Approve(ctx, coretest.Admin, pr.ID, "ok")
	require.NoError(t, err)
	assert.Equal(t, "approved", approved.State)

	// The refresh sees the campaign's budget changed by hand in Google Ads.
	n, err := e.props.Observe(ctx, core.System(core.ViaJob), "campaign", "111", "budget:550000")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	stale, err := e.props.Get(ctx, coretest.Viewer, pr.ID)
	require.NoError(t, err)
	assert.Equal(t, "stale", stale.State)
	assert.Len(t, stale.Approvals, 1, "the approval stays on file")
	_, _, err = e.props.Execute(ctx, coretest.Admin, pr.ID, "k")
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
	// The same observation again changes nothing; a matching version is not stale.
	n, err = e.props.Observe(ctx, core.System(core.ViaJob), "campaign", "111", "budget:550000")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	fresh := raise(0, 0, "raise-2")
	fresh.AccountID, fresh.TargetVersion = e.googleAcc, "budget:550000"
	p2, _, err := e.props.Propose(ctx, coretest.Marketer, fresh)
	require.NoError(t, err)
	n, err = e.props.Observe(ctx, core.System(core.ViaJob), "campaign", "111", "budget:550000")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	got, _ := e.props.Get(ctx, coretest.Viewer, p2.ID)
	assert.Equal(t, "proposed", got.State)
	// Same idempotency key returns the first proposal.
	dup, created, err := e.props.Propose(ctx, coretest.Marketer, fresh)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, p2.ID, dup.ID)
}

// Catches: the assistant proposing, approving or executing through a tool,
// and a write tool creeping into its allowlist.
func TestReadOnlyAssistantCannotInvokeTheExecutor(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.enablePolicy(t)
	asModel := coretest.Admin
	asModel.Via = core.ViaAssistant
	in := raise(0, 0, "model")
	in.AccountID = e.googleAcc
	_, _, err := e.props.Propose(ctx, asModel, in)
	assert.ErrorIs(t, err, core.ErrForbidden)
	pr, _, err := e.props.Propose(ctx, coretest.Marketer, in)
	require.NoError(t, err)
	_, err = e.props.Approve(ctx, asModel, pr.ID, "")
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.props.Approve(ctx, coretest.Admin, pr.ID, "")
	require.NoError(t, err)
	_, _, err = e.props.Execute(ctx, asModel, pr.ID, "k")
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.props.OpenEnvelopes(ctx, asModel, e.plan, 1, nil)
	assert.ErrorIs(t, err, core.ErrForbidden)
	intents, _ := e.props.ListIntents(ctx, coretest.Viewer, "", 10)
	assert.Empty(t, intents)

	a := assistant.New(e.db, nil, assistant.Services{}, assistant.Config{})
	for _, name := range a.ToolNames() {
		lower := strings.ToLower(name)
		for _, bad := range []string{"execute", "propose", "approve", "set_", "pause", "launch", "create", "update", "delete"} {
			assert.NotContains(t, lower, bad, "the assistant's allowlist must only read")
		}
	}
	assert.NotEmpty(t, a.ToolNames())
}

// Catches: a change other than pause or lower budget going through on a
// campaign no accepted plan stands behind, a server with no coverage source
// treating campaigns as planned, and a campaign being created with no
// envelope, so no plan, behind it (BD-68 rules 3 and 4).
func TestUnplannedCampaignTakesOnlySafetyActions(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.enablePolicy(t)
	env := e.envelope(t)
	unplanned := proposals.New(e.db, e.policies, e.plans, proposals.Config{Clock: e.clock, Coverage: proposalstest.Unplanned{}})

	in := raise(1_000, env.ID, "raise-unplanned")
	in.AccountID = e.googleAcc
	pr, _, err := unplanned.Propose(ctx, coretest.Admin, in)
	require.NoError(t, err)
	assert.Equal(t, "denied", pr.State)
	assert.Equal(t, policies.ReasonUnplannedCampaign, pr.PolicyReason)

	lower := proposals.ProposalInput{AccountID: e.googleAcc, Action: policies.BudgetLower, TargetKind: "campaign", TargetRef: "111", TargetVersion: "budget:500000",
		ExpectedCurrent: json.RawMessage(`{"dailyBudgetMinor":500000}`), Proposed: json.RawMessage(`{"dailyBudgetMinor":100000}`), Reason: "spend with no plan behind it", IdempotencyKey: "lower-unplanned"}
	pr, _, err = unplanned.Propose(ctx, coretest.Marketer, lower)
	require.NoError(t, err)
	assert.Equal(t, "approved", pr.State, pr.PolicyReason)
	pause := lower
	pause.Action, pause.IdempotencyKey, pause.Proposed = policies.CampaignPause, "pause-unplanned", json.RawMessage(`{"status":"PAUSED"}`)
	pr, _, err = unplanned.Propose(ctx, coretest.Marketer, pause)
	require.NoError(t, err)
	assert.Equal(t, "proposed", pr.State, "a marketer's pause still needs the admin under the defaults")
	resume := lower
	resume.Action, resume.IdempotencyKey, resume.Proposed = policies.CampaignResume, "resume-unplanned", json.RawMessage(`{"status":"ENABLED"}`)
	pr, _, err = unplanned.Propose(ctx, coretest.Marketer, resume)
	require.NoError(t, err)
	assert.Equal(t, "denied", pr.State, "nothing else changes on it until a plan adopts it")

	// The same raise on a planned campaign goes through, and the authority
	// shown for it says so.
	in = raise(1_000, env.ID, "raise-planned")
	in.AccountID = e.googleAcc
	pr, _, err = e.props.Propose(ctx, coretest.Admin, in)
	require.NoError(t, err)
	assert.Equal(t, "approved", pr.State, pr.PolicyReason)
	auth, err := unplanned.Authority(ctx, coretest.Viewer, pr.ID)
	require.NoError(t, err)
	assert.Equal(t, policies.ReasonUnplannedCampaign, auth.Reason, "coverage is asked again each time, not remembered")

	// No coverage source means nothing is known to be under a plan.
	none := proposals.New(e.db, e.policies, e.plans, proposals.Config{Clock: e.clock})
	req, err := none.PolicyRequestFor(ctx, 1, e.googleAcc, policies.BudgetRaise, "campaign", "111")
	require.NoError(t, err)
	assert.True(t, req.Unplanned)
	req, err = none.PolicyRequestFor(ctx, 1, e.googleAcc, policies.CampaignCreatePaus, "campaign", "new:x")
	require.NoError(t, err)
	assert.False(t, req.Unplanned, "a create is covered by its envelope, not by an existing plan match")
	req, err = none.PolicyRequestFor(ctx, 1, e.googleAcc, policies.AudiencesUpsert, "audience", "new")
	require.NoError(t, err)
	assert.False(t, req.Unplanned, "the rule is about campaigns")

	create := proposals.ProposalInput{AccountID: e.googleAcc, Action: policies.CampaignCreatePaus, TargetKind: "campaign", TargetRef: "new:free", TargetVersion: "none",
		Proposed: json.RawMessage(`{"name":"No plan"}`), Reason: "a campaign with nothing behind it", IdempotencyKey: "create-free"}
	_, _, err = e.props.Propose(ctx, coretest.Admin, create)
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindInvalid, ce.Kind)
}

// Catches: two lines of one channel getting an envelope sized by the first
// line alone, which would refuse the second line's spend (BD-68 point 10).
func TestEnvelopeHoldsEveryLineOfItsChannel(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	cpl := int64(25000)
	ae := plans.Allocation{Channel: "google_search", Objective: "leads", BudgetMinor: 6_000, Geography: "AE", ExpectedCPLMinor: &cpl}
	ksa := ae
	ksa.Geography, ksa.BudgetMinor = "SA", 4_000
	d, err := e.plans.Create(ctx, coretest.Marketer, plans.PlanInput{BrandID: e.reportage, ProjectID: e.wadi, Title: "Two lines", Document: plans.PlanDocument{Objective: "Leads", BudgetMinor: 10_000, Currency: "AED", PeriodFrom: "2026-11-01", PeriodTo: "2026-11-30", Allocations: []plans.Allocation{ae, ksa}}})
	require.NoError(t, err)
	_, err = e.plans.Accept(ctx, coretest.Admin, d.Plan.ID, 1)
	require.NoError(t, err)
	envs, err := e.props.OpenEnvelopes(ctx, coretest.Admin, d.Plan.ID, 1, map[string]int64{"google_ads": e.googleAcc})
	require.NoError(t, err)
	require.Len(t, envs, 1)
	assert.Equal(t, int64(10_000), envs[0].AmountMinor)
}
