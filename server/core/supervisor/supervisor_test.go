package supervisor_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/calls"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/core/proposals/proposalstest"
	"github.com/WokeWorks/adx/server/core/registry"
	"github.com/WokeWorks/adx/server/core/supervisor"
	"github.com/WokeWorks/adx/server/core/voice"
	"github.com/WokeWorks/adx/server/core/voice/voicetest"
	"github.com/WokeWorks/adx/server/core/workflows"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type scriptedRunner struct {
	run func(req workflows.RunRequest) (workflows.RunResult, error)
	n   int
}

func (s *scriptedRunner) Run(_ context.Context, _ core.Actor, req workflows.RunRequest) (workflows.RunResult, error) {
	s.n++
	if s.run == nil {
		return workflows.RunResult{Output: json.RawMessage(`{}`), Tokens: 1}, nil
	}
	return s.run(req)
}

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	policies  *policies.Policies
	proposals *proposals.Proposals
	sup       *supervisor.Supervisor
	reg       *registry.Registry
	wf        *workflows.Workflows
	dispatch  *workflows.CoreDispatcher
	runner    *scriptedRunner
	out       *outreach.Outreach
	crm       *crm.CRM
	calls     *calls.Calls
	tel       *voicetest.Telephony
	acct      int64
	envelope  int64
	project   int64
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)), tel: &voicetest.Telephony{Secret: "s", Known: map[string]voice.StatusEvent{}}}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	ev := evidence.New(e.db, e.clock)
	e.policies = policies.New(e.db)
	e.proposals = proposals.New(e.db, e.policies, nil, proposals.Config{Clock: e.clock, Coverage: proposalstest.Planned{}})
	e.crm = crm.New(e.db, ev, crm.Config{Clock: e.clock})
	e.out = outreach.New(e.db, ev, outreach.Config{Clock: e.clock, CRM: e.crm, Policies: e.policies})
	e.calls = calls.New(e.db, calls.Config{Clock: e.clock, Telephony: e.tel, Policies: e.policies, Outreach: e.out, CRM: e.crm, PublicURL: "https://adx.example.com"})
	e.reg = registry.New(e.db, registry.Config{Clock: e.clock})
	e.runner = &scriptedRunner{}
	e.sup = supervisor.New(e.db, supervisor.Config{Clock: e.clock, Proposals: e.proposals, Outreach: e.out, CRM: e.crm, Policies: e.policies, Calls: e.calls})
	e.dispatch = &workflows.CoreDispatcher{Proposals: e.proposals, Recommender: e.sup}
	e.wf = workflows.New(e.db, workflows.Config{Clock: e.clock, Gate: e.policies, Registry: e.reg, Runner: e.runner, Dispatch: e.dispatch, External: e.dispatch, Lease: time.Minute, MaxAttempts: 2})
	// An ad account, a plan and an envelope of AED 10,000.
	cred, err := e.db.Writer().UpsertCredential(ctx, store.UpsertCredentialParams{Provider: "google_ads", Kind: "oauth", Subject: "u", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "x", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(ctx, store.UpsertAccountParams{WorkspaceID: core.DefaultWorkspace, Type: "google_ads", ExternalID: "1234567890", CredentialID: cred.ID, Currency: sql.NullString{String: "AED", Valid: true}, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	e.acct = acct.ID
	brand, err := e.db.Writer().CreateBrand(ctx, store.CreateBrandParams{WorkspaceID: 1, Name: "Reportage", Kind: "own", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.project = p.ID
	plan, err := e.db.Writer().CreatePlan(ctx, store.CreatePlanParams{WorkspaceID: 1, BrandID: brand.ID, ProjectID: sql.NullInt64{Int64: p.ID, Valid: true}, Title: "Q4", CreatedAt: e.clock.Now(), UpdatedAt: e.clock.Now()})
	require.NoError(t, err)
	envRow, err := e.db.Writer().CreateBudgetEnvelope(ctx, store.CreateBudgetEnvelopeParams{WorkspaceID: 1, PlanID: plan.ID, PlanVersion: 1, Channel: "google_search", AccountID: sql.NullInt64{Int64: acct.ID, Valid: true}, Currency: "AED", AmountMinor: 1000000, OpenedAt: e.clock.Now()})
	require.NoError(t, err)
	e.envelope = envRow.ID
	return e
}

func (e *env) allowPolicy(t *testing.T, marketer policies.PolicyMode) {
	t.Helper()
	doc := policies.SuggestedDefaults()
	pct, perDay := 50, 50
	doc.Caps = policies.PolicyCaps{DailySpendIncreasePct: &pct, MaxChangesPerDay: &perDay}
	doc.WriteAccounts = []int64{e.acct}
	doc.Actions[policies.BudgetRaise] = policies.PolicyRule{Marketer: marketer, Admin: "allowed"}
	_, err := e.policies.Put(t.Context(), coretest.Admin, doc)
	require.NoError(t, err)
	require.NoError(t, e.db.Writer().UpsertAccountCapability(t.Context(), store.UpsertAccountCapabilityParams{AccountID: e.acct, Capability: "write.budget", State: "verified", VerifiedAt: sql.NullTime{Time: e.clock.Now(), Valid: true}, CheckedAt: e.clock.Now()}))
}

func (e *env) propose(t *testing.T, actor core.Actor, key string, amount int64) proposals.Proposal {
	t.Helper()
	p, _, err := e.proposals.Propose(t.Context(), actor, proposals.ProposalInput{AccountID: e.acct, Action: policies.BudgetRaise, TargetKind: "campaign", TargetRef: "g1", TargetVersion: "v1", ExpectedCurrent: json.RawMessage(`{"dailyBudgetMinor":100000}`), Proposed: json.RawMessage(`{"dailyBudgetMinor":200000}`),
		Reason: "more leads", AmountMinor: amount, Currency: "AED", EnvelopeID: e.envelope, IdempotencyKey: key})
	require.NoError(t, err)
	return p
}

// Catches: two approved recommendations both reserving one allocation, a
// conflict with no precedence being decided by code, and a conflict with
// precedence left to a person.
func TestConflictingRecommendationsCannotBothConsumeTheAllocation(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.allowPolicy(t, "needs_approval")
	a := e.propose(t, coretest.Marketer, "rec-a", 700000)
	b := e.propose(t, coretest.Marketer, "rec-b", 700000)
	require.NoError(t, e.sup.Record(ctx, coretest.Marketer, a.ID, "agent:cpl", "propose_change", "cpl"))
	require.NoError(t, e.sup.Record(ctx, coretest.Marketer, b.ID, "agent:volume", "propose_change", "volume"))
	// Both approved by a person: the envelope cannot carry both.
	_, err := e.proposals.Approve(ctx, coretest.Admin, a.ID, "ok")
	require.NoError(t, err)
	_, err = e.proposals.Approve(ctx, coretest.Admin, b.ID, "ok")
	require.NoError(t, err)
	_, _, err = e.proposals.Execute(ctx, coretest.Admin, a.ID, "exec-a")
	require.NoError(t, err)
	_, _, err = e.proposals.Execute(ctx, coretest.Admin, b.ID, "exec-b")
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.Contains(t, err.Error(), "insufficient remaining budget")
	var reservations int
	require.NoError(t, e.db.Read.QueryRowContext(ctx, "SELECT COUNT(*) FROM budget_reservations").Scan(&reservations))
	assert.Equal(t, 1, reservations, "one reservation, not two")

	// No precedence configured: a fresh pair of competing proposals is
	// escalated, nothing is rejected by code.
	e2 := setup(t)
	e2.allowPolicy(t, "needs_approval")
	c := e2.propose(t, coretest.Marketer, "rec-c", 700000)
	d := e2.propose(t, coretest.Marketer, "rec-d", 700000)
	require.NoError(t, e2.sup.Record(ctx, coretest.Marketer, c.ID, "agent:cpl", "propose_change", "cpl"))
	require.NoError(t, e2.sup.Record(ctx, coretest.Marketer, d.ID, "agent:volume", "propose_change", "volume"))
	conflicts, err := e2.sup.Arbitrate(ctx, coretest.Admin)
	require.NoError(t, err)
	require.Len(t, conflicts, 1)
	assert.Equal(t, "escalated", conflicts[0].State)
	assert.Contains(t, conflicts[0].Detail, "a person decides")
	assert.ElementsMatch(t, []int64{c.ID, d.ID}, conflicts[0].ProposalIDs)
	for _, id := range []int64{c.ID, d.ID} {
		p, _ := e2.proposals.Get(ctx, coretest.Viewer, id)
		assert.Equal(t, "proposed", p.State, "nothing decided by code")
	}
	inbox, err := e2.sup.Inbox(ctx, coretest.Viewer)
	require.NoError(t, err)
	found := false
	for _, it := range inbox {
		if it.Kind == "conflict" && it.Ref == conflicts[0].ID {
			found = true
			assert.Equal(t, "urgent", it.Severity)
			assert.Equal(t, []string{"keep"}, it.Actions)
		}
	}
	assert.True(t, found, "the conflict is in the inbox")
	again, _ := e2.sup.Arbitrate(ctx, coretest.Admin)
	assert.Empty(t, again, "an escalated conflict is not raised twice")
	// A person keeps c; d is rejected with the reason.
	_, err = e2.sup.Act(ctx, coretest.Admin, supervisor.ActionInput{Kind: "conflict", Ref: conflicts[0].ID, Action: "keep", Target: c.ID, Note: "cost per lead matters more this month"})
	require.NoError(t, err)
	pd, _ := e2.proposals.Get(ctx, coretest.Viewer, d.ID)
	assert.Equal(t, "rejected", pd.State)
	pc, _ := e2.proposals.Get(ctx, coretest.Viewer, c.ID)
	assert.Equal(t, "proposed", pc.State)

	// With precedence configured, code arbitrates: cpl outranks volume.
	e3 := setup(t)
	e3.allowPolicy(t, "needs_approval")
	_, err = e3.sup.PutConfig(ctx, coretest.Admin, supervisor.SupervisorDocument{Objectives: []string{"cpl", "volume"}, Autonomy: map[string]supervisor.AutonomyRule{}})
	require.NoError(t, err)
	f := e3.propose(t, coretest.Marketer, "rec-f", 700000)
	g := e3.propose(t, coretest.Marketer, "rec-g", 700000)
	require.NoError(t, e3.sup.Record(ctx, coretest.Marketer, f.ID, "agent:volume", "propose_change", "volume"))
	require.NoError(t, e3.sup.Record(ctx, coretest.Marketer, g.ID, "agent:cpl", "propose_change", "cpl"))
	conflicts, err = e3.sup.Arbitrate(ctx, coretest.Admin)
	require.NoError(t, err)
	require.Len(t, conflicts, 1)
	assert.Equal(t, "arbitrated", conflicts[0].State)
	assert.Contains(t, conflicts[0].Resolution, "cpl > volume")
	pf, _ := e3.proposals.Get(ctx, coretest.Viewer, f.ID)
	pg, _ := e3.proposals.Get(ctx, coretest.Viewer, g.ID)
	assert.Equal(t, "rejected", pf.State, "volume is outranked")
	assert.Equal(t, "proposed", pg.State, "cpl stays for approval")
	// An unknown objective among them falls back to a person.
	h := e3.propose(t, coretest.Marketer, "rec-h", 700000)
	require.NoError(t, e3.sup.Record(ctx, coretest.Marketer, h.ID, "agent:brand", "propose_change", "brand"))
	conflicts, _ = e3.sup.Arbitrate(ctx, coretest.Admin)
	require.Len(t, conflicts, 1)
	assert.Equal(t, "escalated", conflicts[0].State)
}

// Catches: a proposal denied under one policy becoming approvable or
// executable after the policy is loosened.
func TestChangingPolicyCannotRetroactivelyAuthorize(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	denied := e.propose(t, coretest.Marketer, "early", 100000)
	assert.Equal(t, "denied", denied.State)
	assert.Equal(t, policies.ReasonPolicyUnconfigured, denied.PolicyReason)
	e.allowPolicy(t, "allowed")
	_, err := e.proposals.Approve(ctx, coretest.Admin, denied.ID, "now allowed")
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind, "a denied proposal cannot be approved after the fact")
	_, _, err = e.proposals.Execute(ctx, coretest.Admin, denied.ID, "exec-early")
	require.Error(t, err)
	still, _ := e.proposals.Get(ctx, coretest.Viewer, denied.ID)
	assert.Equal(t, "denied", still.State)
	assert.Nil(t, still.PolicyVersion, "the decision keeps the policy it was made under: none")
	// The way forward is a new proposal, evaluated now, that points back.
	fresh, err := e.sup.Reconsider(ctx, coretest.Marketer, denied.ID, "policy is configured now")
	require.NoError(t, err)
	assert.NotEqual(t, denied.ID, fresh.ID)
	assert.Equal(t, "approved", fresh.State, "a marketer may raise budgets under the new policy")
	assert.Contains(t, fresh.Reason, "re-asks proposal 1")
	assert.NotNil(t, fresh.PolicyVersion)
	still, _ = e.proposals.Get(ctx, coretest.Viewer, denied.ID)
	assert.Equal(t, "denied", still.State, "the old one is untouched")
	_, err = e.sup.Reconsider(ctx, coretest.Marketer, fresh.ID, "again")
	require.Error(t, err, "only denied, rejected or stale proposals are re-asked")
	// Autonomy modes: under "recommend", an agent's proposal waits even
	// though policy allows the role; the cap stops the second one.
	_, err = e.sup.PutConfig(ctx, coretest.Admin, supervisor.SupervisorDocument{Autonomy: map[string]supervisor.AutonomyRule{"propose_change": {Mode: supervisor.ModeRecommend, MaxPerDay: 1}}})
	require.NoError(t, err)
	wfActor := core.Actor{Role: core.RoleMarketer, Via: core.ViaJob, WorkspaceID: 1}
	out, err := e.dispatch.Call(ctx, wfActor, "propose_change", map[string]any{"accountId": e.acct, "action": "budget.raise", "targetKind": "campaign", "targetRef": "g1", "targetVersion": "v1", "operationKey": "wf:9:s:0", "objective": "cpl", "expectedCurrent": map[string]any{"a": 1}, "proposed": map[string]any{"a": 2}})
	require.NoError(t, err)
	m := out.(map[string]any)
	assert.Equal(t, "proposed", m["state"], "recommend mode: a person approves")
	assert.Contains(t, m["policyReason"], "autonomy mode")
	recs, _ := e.sup.Recommendations(ctx, coretest.Viewer, 10)
	require.Len(t, recs, 1)
	assert.Equal(t, "cpl", recs[0].Objective)
	assert.Equal(t, "workflow:wf:9:s:0", recs[0].Source)
	_, err = e.dispatch.Call(ctx, wfActor, "propose_change", map[string]any{"accountId": e.acct, "action": "budget.raise", "targetKind": "campaign", "targetRef": "g1", "targetVersion": "v1", "operationKey": "wf:9:t:0", "expectedCurrent": map[string]any{"a": 1}, "proposed": map[string]any{"a": 2}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cap of 1")
	// With no supervisor config at all, the default is recommend.
	e2 := setup(t)
	e2.allowPolicy(t, "allowed")
	out, err = e2.dispatch.Call(ctx, wfActor, "propose_change", map[string]any{"accountId": e2.acct, "action": "budget.raise", "targetKind": "campaign", "targetRef": "g1", "targetVersion": "v1", "operationKey": "wf:1:s:0", "expectedCurrent": map[string]any{"a": 1}, "proposed": map[string]any{"a": 2}})
	require.NoError(t, err)
	assert.Equal(t, "proposed", out.(map[string]any)["state"])
}

// Catches: an entry point that still acts while the kill switch is on.
func TestKillSwitchAppliesToEveryEntryPoint(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.allowPolicy(t, "allowed")
	// Set up each entry point so it would act without the switch.
	d, err := e.crm.Create(ctx, coretest.Marketer, crm.PersonInput{DisplayName: "Amal Haddad", Phone: "0501234567"})
	require.NoError(t, err)
	enq, err := e.db.Writer().CreateEnquiry(ctx, store.CreateEnquiryParams{WorkspaceID: 1, PersonID: d.Person.ID, ProjectID: sql.NullInt64{Int64: e.project, Valid: true}, SourceKind: "manual", SourceRef: "x", FirstContactAt: e.clock.Now(), LastActivityAt: e.clock.Now(), CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	_, err = e.out.PutConfig(ctx, coretest.Admin, outreach.OutreachDocument{Channels: map[string]outreach.ChannelMode{"whatsapp": outreach.ModeApproved}})
	require.NoError(t, err)
	draft, err := e.out.Draft(ctx, coretest.Marketer, outreach.DraftInput{EnquiryID: enq.ID, Channel: "whatsapp", Body: "Hi Amal"})
	require.NoError(t, err)
	_, err = e.out.Approve(ctx, coretest.Marketer, draft.ID)
	require.NoError(t, err)
	pdoc := calls.SuggestedPolicy()
	pdoc.Route = calls.Route{Provider: "fake", AuthorizedBy: "t", AuthorizedOn: "2026-10-17"}
	pdoc.FromNumber = "+97180012345"
	_, err = e.calls.PutPolicy(ctx, coretest.Admin, pdoc)
	require.NoError(t, err)
	skill := registry.SkillDocument{Name: "Read", Instructions: "x", InputSchema: map[string]any{"type": "object"}, OutputSchema: map[string]any{"type": "object"}, EvidenceScope: []string{"public"}, PermittedTools: []string{"get_analytics"}}
	v, err := e.reg.SaveSkill(ctx, coretest.Admin, "read", skill, false)
	require.NoError(t, err)
	_, err = e.reg.Activate(ctx, coretest.Admin, v.ID)
	require.NoError(t, err)
	w, err := e.reg.SaveWorkflow(ctx, coretest.Admin, "wf", registry.WorkflowDocument{Name: "W", MaxSteps: 3, Steps: []registry.WorkflowStep{{Key: "a", Skill: "read"}}}, false)
	require.NoError(t, err)
	_, err = e.reg.Activate(ctx, coretest.Admin, w.ID)
	require.NoError(t, err)
	g, err := e.wf.Start(ctx, coretest.Marketer, workflows.WorkflowStartInput{Definition: "wf"})
	require.NoError(t, err)

	// Engage the workspace switch through the supervisor.
	_, err = e.sup.Halt(ctx, coretest.Marketer, 0, true, "x")
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.sup.Halt(ctx, coretest.Admin, 0, true, "incident")
	require.NoError(t, err)

	// 1. Proposals: denied by policy.
	p := e.propose(t, coretest.Admin, "during", 100000)
	assert.Equal(t, "denied", p.State)
	assert.Equal(t, policies.ReasonKillSwitch, p.PolicyReason)
	// 2. Messaging: held.
	sent, err := e.out.Send(ctx, coretest.Marketer, draft.ID)
	require.NoError(t, err)
	assert.Equal(t, "held", sent.State)
	assert.Contains(t, sent.Reason, "kill switch")
	// 3. CRM field dispatch: held by policy.
	_, err = e.db.Writer().UpsertLead(ctx, store.UpsertLeadParams{WorkspaceID: 1, AccountID: e.acct, ExternalID: "c1", FirstName: ns("Amal"), Phone: ns("+971501234567"), PhoneE164: ns("+971501234567"), FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	_, err = e.crm.Sync(ctx, coretest.Marketer)
	require.NoError(t, err)
	people, _ := e.crm.List(ctx, coretest.Viewer, "Amal", 5)
	_, err = e.out.PutConfig(ctx, coretest.Admin, outreach.OutreachDocument{Fields: map[string]outreach.FieldOwner{"adx_temperature": outreach.OwnerADX}, Channels: map[string]outreach.ChannelMode{"whatsapp": outreach.ModeApproved}})
	require.NoError(t, err)
	_, err = e.out.QueueFieldWrite(ctx, coretest.Marketer, people[0].ID, "adx_temperature", "hot")
	require.NoError(t, err)
	n, err := e.out.DispatchFields(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	writes, _ := e.out.ListFieldWrites(ctx, coretest.Viewer, 10)
	require.NotEmpty(t, writes)
	assert.Equal(t, "held", writes[0].State)
	assert.Contains(t, writes[0].Reason, policies.ReasonKillSwitch)
	// 4. Workflows: no task leased.
	_, err = e.wf.Tick(ctx)
	require.NoError(t, err)
	g = mustGraph(t, e, g.Run.ID)
	assert.Equal(t, "ready", g.Tasks[0].State)
	assert.Zero(t, e.runner.n, "the runner was never asked")
	held := false
	for _, ev := range g.Events {
		if ev.Kind == "held" {
			held = true
		}
	}
	assert.True(t, held)
	// 5. Calls: blocked.
	ct, err := e.calls.Contactability(ctx, coretest.Viewer, enq.ID, "form")
	require.NoError(t, err)
	joined := ""
	for _, b := range ct.Blockers {
		joined += b + ";"
	}
	assert.Contains(t, joined, "kill switch")
	_, err = e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq.ID, ConsentBasis: "form"})
	require.Error(t, err)
	assert.Empty(t, e.tel.Calls)
	// The inbox shows the switch; releasing it lets the workflow run.
	inbox, _ := e.sup.Inbox(ctx, coretest.Viewer)
	kinds := map[string]int{}
	for _, it := range inbox {
		kinds[it.Kind]++
	}
	assert.Equal(t, 1, kinds["kill_switch"])
	_, err = e.sup.Act(ctx, coretest.Admin, supervisor.ActionInput{Kind: "kill_switch", Ref: 0, Action: "release", Note: "resolved"})
	require.NoError(t, err)
	_, err = e.wf.Tick(ctx)
	require.NoError(t, err)
	g = mustGraph(t, e, g.Run.ID)
	assert.Equal(t, "done", g.Tasks[0].State)
	assert.Equal(t, 1, e.runner.n)
}

// Catches: inbox items missing their domain, and an action the supervisor
// performs with more authority than the person has.
func TestInboxUnifiesWaitingWorkAndActsThroughDomains(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.allowPolicy(t, "needs_approval")
	p := e.propose(t, coretest.Marketer, "await", 100000)
	assert.Equal(t, "proposed", p.State)
	inbox, err := e.sup.Inbox(ctx, coretest.Viewer)
	require.NoError(t, err)
	var item *supervisor.InboxItem
	for i := range inbox {
		if inbox[i].Kind == "proposal" && inbox[i].Ref == p.ID {
			item = &inbox[i]
		}
	}
	require.NotNil(t, item)
	assert.Equal(t, []string{"approve", "reject"}, item.Actions)
	_, err = e.sup.Act(ctx, coretest.Marketer, supervisor.ActionInput{Kind: "proposal", Ref: p.ID, Action: "approve", Note: "x"})
	assert.ErrorIs(t, err, core.ErrForbidden, "a marketer cannot approve through the supervisor either")
	_, err = e.sup.Act(ctx, coretest.Admin, supervisor.ActionInput{Kind: "proposal", Ref: p.ID, Action: "approve", Note: "fine"})
	require.NoError(t, err)
	after, _ := e.proposals.Get(ctx, coretest.Viewer, p.ID)
	assert.Equal(t, "approved", after.State)
	_, err = e.sup.Act(ctx, coretest.Admin, supervisor.ActionInput{Kind: "proposal", Ref: p.ID, Action: "dance"})
	require.Error(t, err)
	_, err = e.sup.PutConfig(ctx, coretest.Admin, supervisor.SupervisorDocument{Autonomy: map[string]supervisor.AutonomyRule{"propose_change": {Mode: "whatever"}}})
	require.Error(t, err)
	st, err := e.sup.GetConfig(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.False(t, st.Configured)
	assert.Equal(t, supervisor.ModeRecommend, st.Suggested.Autonomy["propose_change"].Mode)
}

func mustGraph(t *testing.T, e *env, id int64) workflows.WorkflowGraph {
	t.Helper()
	g, err := e.wf.Graph(t.Context(), coretest.Viewer, id)
	require.NoError(t, err)
	return g
}

func ns(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
