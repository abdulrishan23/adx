package policies_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

var (
	admin    = core.Actor{UserID: 1, Role: core.RoleAdmin, Via: core.ViaWeb, WorkspaceID: 1}
	marketer = core.Actor{UserID: 2, Role: core.RoleMarketer, Via: core.ViaWeb, WorkspaceID: 1}
	viewer   = core.Actor{UserID: 3, Role: core.RoleViewer, Via: core.ViaWeb, WorkspaceID: 1}
)

type env struct {
	db *store.DB
	p  *policies.Policies
}

func setup(t *testing.T) env {
	t.Helper()
	db := storetest.Open(t)
	now := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	for i, email := range []string{"cmo@reportage.ae", "sara@reportage.ae", "ceo@reportage.ae"} {
		_, err := db.Writer().CreateUser(t.Context(), store.CreateUserParams{Email: email, Role: "admin", Status: "active",
			AuthentikPk: int64(i + 1), AuthentikUuid: email, CreatedAt: now})
		require.NoError(t, err)
	}
	return env{db: db, p: policies.New(db)}
}

// account connects a fake credential and account, and optionally marks a
// write capability verified. It returns the account ID.
func (e env) account(t *testing.T, external string, writeVerified bool) int64 {
	t.Helper()
	ctx := t.Context()
	cred, err := e.db.Writer().UpsertCredential(ctx, store.UpsertCredentialParams{Provider: "google_ads", Kind: "oauth", Subject: "sub-" + external,
		EncryptedSecret: []byte("sealed"), KeyVersion: 1, Scopes: "adwords", ConnectedAt: time.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(ctx, store.UpsertAccountParams{WorkspaceID: core.DefaultWorkspace, Type: "google_ads", ExternalID: external, CredentialID: cred.ID,
		Details: "{}", ConnectedAt: time.Now(), VerifiedAt: time.Now()})
	require.NoError(t, err)
	state := "unknown"
	if writeVerified {
		state = "verified"
	}
	require.NoError(t, e.db.Writer().UpsertAccountCapability(ctx, store.UpsertAccountCapabilityParams{AccountID: acct.ID, Capability: "write.budget",
		State: state, VerifiedAt: sql.NullTime{Time: time.Now(), Valid: writeVerified}, CheckedAt: time.Now()}))
	return acct.ID
}

func (e env) evaluate(t *testing.T, actor core.Actor, account int64, action policies.WriteAction) policies.PolicyDecision {
	t.Helper()
	d, err := e.p.Evaluate(t.Context(), actor, policies.PolicyRequest{AccountID: account, Action: action, WriteCapability: "write.budget"})
	require.NoError(t, err)
	return d
}

func configured(accounts ...int64) policies.PolicyDocument {
	doc := policies.SuggestedDefaults()
	pct, perDay := 20, 50
	doc.Caps = policies.PolicyCaps{DailySpendIncreasePct: &pct, MaxChangesPerDay: &perDay}
	doc.WriteAccounts = accounts
	return doc
}

// Catches: a connected account with a working token being writable before
// anyone configured anything, which is the spec's "token existing implies
// permission" failure.
func TestNoPolicyDeniesEverythingEvenWithAVerifiedToken(t *testing.T) {
	e := setup(t)
	acct := e.account(t, "123", true)
	d := e.evaluate(t, admin, acct, policies.BudgetLower)
	assert.Equal(t, policies.Denied, d.Outcome)
	assert.Equal(t, policies.ReasonPolicyUnconfigured, d.Reason)

	state, err := e.p.Get(t.Context(), viewer)
	require.NoError(t, err)
	assert.False(t, state.Configured)
	assert.Equal(t, []string{"policy document"}, state.Unconfigured)
	assert.Len(t, state.Suggested.Actions, len(policies.Catalog), "the D-13 table is offered, not applied")
}

// Catches: the checklist applying to an account nobody enabled writes for,
// or to a write the provider never proved (missing scope), or with no caps.
func TestScopeAndEnablementGateTheChecklist(t *testing.T) {
	e := setup(t)
	enabled := e.account(t, "enabled", true)
	notEnabled := e.account(t, "not-enabled", true)
	unproven := e.account(t, "unproven", false)

	doc := configured(enabled, unproven)
	_, err := e.p.Put(t.Context(), admin, doc)
	require.NoError(t, err)

	assert.Equal(t, policies.ReasonAccountNotEnabled, e.evaluate(t, marketer, notEnabled, policies.BudgetLower).Reason)
	assert.Equal(t, policies.ReasonCapabilityUnproven, e.evaluate(t, marketer, unproven, policies.BudgetLower).Reason)
	assert.Equal(t, policies.ReasonAccountNotConnected, e.evaluate(t, marketer, 999, policies.BudgetLower).Reason)

	d := e.evaluate(t, marketer, enabled, policies.BudgetLower)
	assert.Equal(t, policies.Allowed, d.Outcome)
	assert.Equal(t, int64(1), d.PolicyVersion)
	assert.Equal(t, policies.NeedsApproval, e.evaluate(t, marketer, enabled, policies.BudgetRaise).Outcome)
	assert.Equal(t, policies.Allowed, e.evaluate(t, admin, enabled, policies.BudgetRaise).Outcome, "the admin self-approves")
	assert.Equal(t, policies.ReasonActionOff, e.evaluate(t, admin, enabled, policies.OrganicPost).Reason)
	assert.Equal(t, policies.ReasonForbiddenRole, e.evaluate(t, viewer, enabled, policies.BudgetLower).Reason)
	assert.Equal(t, policies.ReasonUnknownAction, e.evaluate(t, admin, enabled, policies.WriteAction("delete")).Reason)

	// Caps missing: raises are denied even when the checklist allows them.
	noCaps := configured(enabled)
	noCaps.Caps = policies.PolicyCaps{}
	_, err = e.p.Put(t.Context(), admin, noCaps)
	require.NoError(t, err)
	d = e.evaluate(t, admin, enabled, policies.BudgetRaise)
	assert.Equal(t, policies.ReasonCapUnconfigured, d.Reason)
	assert.Equal(t, int64(2), d.PolicyVersion)

	// Another workspace evaluates against its own (absent) policy, never
	// against this one's. Accounts themselves get workspace scope in B6.1.
	other := admin
	other.WorkspaceID = 2
	assert.Equal(t, policies.ReasonPolicyUnconfigured, e.evaluate(t, other, enabled, policies.BudgetLower).Reason)
	none := admin
	none.WorkspaceID = 0
	assert.Equal(t, policies.ReasonWorkspaceMismatch, e.evaluate(t, none, enabled, policies.BudgetLower).Reason)
}

// Catches: a kill switch that only some entry point honours, or one that
// an account switch misses for other accounts.
func TestKillSwitches(t *testing.T) {
	e := setup(t)
	a, b := e.account(t, "a", true), e.account(t, "b", true)
	_, err := e.p.Put(t.Context(), admin, configured(a, b))
	require.NoError(t, err)

	_, err = e.p.SetKillSwitch(t.Context(), marketer, a, true, "")
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.p.SetKillSwitch(t.Context(), admin, 999, true, "")
	requireKind(t, err, core.KindNotFound)

	_, err = e.p.SetKillSwitch(t.Context(), admin, a, true, "spend spike")
	require.NoError(t, err)
	assert.Equal(t, policies.ReasonKillSwitch, e.evaluate(t, admin, a, policies.BudgetLower).Reason)
	assert.Equal(t, policies.Allowed, e.evaluate(t, admin, b, policies.BudgetLower).Outcome)

	_, err = e.p.SetKillSwitch(t.Context(), admin, 0, true, "everything off")
	require.NoError(t, err)
	assert.Equal(t, policies.ReasonKillSwitch, e.evaluate(t, admin, b, policies.BudgetLower).Reason)

	_, err = e.p.SetKillSwitch(t.Context(), admin, 0, false, "")
	require.NoError(t, err)
	_, err = e.p.SetKillSwitch(t.Context(), admin, a, false, "")
	require.NoError(t, err)
	assert.Equal(t, policies.Allowed, e.evaluate(t, admin, a, policies.BudgetLower).Outcome)

	state, err := e.p.Get(t.Context(), viewer)
	require.NoError(t, err)
	require.Len(t, state.KillSwitches, 2)
	assert.False(t, state.KillSwitches[0].Engaged)
}

// Catches: a non-admin saving policy, an invalid document being stored, and
// a version being edited in place.
func TestPutIsAdminOnlyValidatedAndVersioned(t *testing.T) {
	e := setup(t)
	_, err := e.p.Put(t.Context(), marketer, configured())
	assert.ErrorIs(t, err, core.ErrForbidden)

	bad := configured()
	bad.Actions[policies.WriteAction("campaign.delete")] = policies.PolicyRule{Marketer: policies.ModeAllowed}
	_, err = e.p.Put(t.Context(), admin, bad)
	requireKind(t, err, core.KindInvalid)
	bad = configured()
	bad.Actions[policies.AdPause] = policies.PolicyRule{Marketer: policies.PolicyMode("maybe")}
	_, err = e.p.Put(t.Context(), admin, bad)
	requireKind(t, err, core.KindInvalid)

	v1, err := e.p.Put(t.Context(), admin, configured())
	require.NoError(t, err)
	v2, err := e.p.Put(t.Context(), admin, configured())
	require.NoError(t, err)
	assert.Equal(t, int64(1), v1.Version)
	assert.Equal(t, int64(2), v2.Version)
	assert.Equal(t, int64(1), *v2.CreatedBy)

	state, err := e.p.Get(t.Context(), viewer)
	require.NoError(t, err)
	assert.Equal(t, int64(2), state.Current.Version)
	assert.Contains(t, state.Unconfigured, "write accounts: none enabled")

	_, err = e.p.Get(t.Context(), core.Actor{})
	assert.ErrorIs(t, err, core.ErrForbidden)
}

func requireKind(t *testing.T, err error, kind core.Kind) {
	t.Helper()
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, kind, ce.Kind)
}

var _ = context.Background

// Catches: resuming a paused campaign waiting on the CMO under the suggested
// defaults (BD-60 rule 5), and the new action missing from the catalog.
func TestResumeIsAMarketersOwnAction(t *testing.T) {
	e := setup(t)
	acct := e.account(t, "resume", true)
	_, err := e.p.Put(t.Context(), admin, configured(acct))
	require.NoError(t, err)
	d := e.evaluate(t, marketer, acct, policies.CampaignResume)
	assert.Equal(t, policies.Allowed, d.Outcome, d.Reason)
	assert.Equal(t, policies.NeedsApproval, e.evaluate(t, marketer, acct, policies.CampaignPause).Outcome, "pausing a planned campaign keeps its approval rule")
	found := false
	for _, a := range policies.Catalog {
		found = found || a.Action == policies.CampaignResume
	}
	assert.True(t, found)
}

// Catches: a campaign with no plan behind it taking a raise, a resume or a
// targeting change because the checklist allows the role to; and the safety
// actions (pause, lower) being blocked on it (BD-60 rules 3 and 4).
func TestUnplannedCampaignAllowsOnlyPauseAndLower(t *testing.T) {
	e := setup(t)
	acct := e.account(t, "unplanned", true)
	_, err := e.p.Put(t.Context(), admin, configured(acct))
	require.NoError(t, err)
	eval := func(actor core.Actor, action policies.WriteAction) policies.PolicyDecision {
		d, err := e.p.Evaluate(t.Context(), actor, policies.PolicyRequest{AccountID: acct, Action: action, WriteCapability: "write.budget", Unplanned: true})
		require.NoError(t, err)
		return d
	}
	for _, action := range []policies.WriteAction{policies.BudgetRaise, policies.CampaignResume, policies.TargetingChange, policies.BidsChange} {
		d := eval(admin, action)
		assert.Equal(t, policies.Denied, d.Outcome, string(action))
		assert.Equal(t, policies.ReasonUnplannedCampaign, d.Reason, string(action))
	}
	assert.Equal(t, policies.Allowed, eval(marketer, policies.BudgetLower).Outcome)
	assert.Equal(t, policies.NeedsApproval, eval(marketer, policies.CampaignPause).Outcome, "pause keeps the checklist's own rule")
	assert.Equal(t, policies.Allowed, eval(admin, policies.CampaignPause).Outcome)
	assert.True(t, policies.AllowedUnplanned(policies.BudgetLower))
	assert.False(t, policies.AllowedUnplanned(policies.BudgetRaise))
}
