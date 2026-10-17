package workflows_test

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
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/core/registry"
	"github.com/WokeWorks/adx/server/core/workflows"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// scriptedRunner answers per skill key; a script may call tools through
// the request's dispatcher, as the model runner would.
type scriptedRunner struct {
	scripts map[string]func(req workflows.RunRequest) (workflows.RunResult, error)
	calls   map[string]int
}

func (s *scriptedRunner) Run(_ context.Context, _ core.Actor, req workflows.RunRequest) (workflows.RunResult, error) {
	s.calls[req.SkillKey]++
	f, ok := s.scripts[req.SkillKey]
	if !ok {
		return workflows.RunResult{Output: json.RawMessage(`{}`), Tokens: 10}, nil
	}
	return f(req)
}

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	reg       *registry.Registry
	wf        *workflows.Workflows
	runner    *scriptedRunner
	dispatch  *workflows.CoreDispatcher
	proposals *proposals.Proposals
	policies  *policies.Policies
	acct      int64
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	e.reg = registry.New(e.db, registry.Config{Clock: e.clock})
	e.policies = policies.New(e.db)
	e.proposals = proposals.New(e.db, e.policies, nil, proposals.Config{Clock: e.clock})
	e.runner = &scriptedRunner{scripts: map[string]func(workflows.RunRequest) (workflows.RunResult, error){}, calls: map[string]int{}}
	e.dispatch = &workflows.CoreDispatcher{Proposals: e.proposals, Reads: map[string]workflows.ReadTool{
		"get_analytics": func(context.Context, core.Actor, map[string]any) (any, error) {
			return map[string]any{"spendMinor": 123456}, nil
		},
	}}
	e.wf = workflows.New(e.db, workflows.Config{Clock: e.clock, Registry: e.reg, Runner: e.runner, Dispatch: e.dispatch, External: e.dispatch, Lease: time.Minute, MaxAttempts: 2})
	cred, err := e.db.Writer().UpsertCredential(ctx, store.UpsertCredentialParams{Provider: "google_ads", Kind: "oauth", Subject: "u", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "x", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(ctx, store.UpsertAccountParams{WorkspaceID: core.DefaultWorkspace, Type: "google_ads", ExternalID: "1234567890", CredentialID: cred.ID, Currency: sql.NullString{String: "AED", Valid: true}, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	e.acct = acct.ID
	// Skills: a read skill and a proposing skill.
	out := map[string]any{"type": "object", "properties": map[string]any{"spend": map[string]any{"type": "number"}, "again": map[string]any{"type": "boolean"}, "note": map[string]any{"type": "string"}}}
	read := registry.SkillDocument{Name: "Read spend", Instructions: "Read spend.", InputSchema: map[string]any{"type": "object"}, OutputSchema: out, EvidenceScope: []string{"account:*"}, PermittedTools: []string{"get_analytics"}}
	e.activate(t, "spend.read", read)
	prop := registry.SkillDocument{Name: "Suggest budget", Instructions: "Propose.", InputSchema: map[string]any{"type": "object"}, OutputSchema: out, EvidenceScope: []string{"account:*"}, PermittedTools: []string{"get_analytics", "propose_change"}}
	e.activate(t, "budget.suggest", prop)
	return e
}

func (e *env) activate(t *testing.T, key string, doc registry.SkillDocument) {
	t.Helper()
	v, err := e.reg.SaveSkill(t.Context(), coretest.Admin, key, doc, false)
	require.NoError(t, err)
	_, err = e.reg.Activate(t.Context(), coretest.Admin, v.ID)
	require.NoError(t, err)
}

func (e *env) workflow(t *testing.T, key string, doc registry.WorkflowDocument) {
	t.Helper()
	w, err := e.reg.SaveWorkflow(t.Context(), coretest.Admin, key, doc, false)
	require.NoError(t, err)
	require.Empty(t, w.Validation, "%v", w.Validation)
	_, err = e.reg.Activate(t.Context(), coretest.Admin, w.ID)
	require.NoError(t, err)
}

func (e *env) tick(t *testing.T) {
	t.Helper()
	_, err := e.wf.Tick(t.Context())
	require.NoError(t, err)
}

func (e *env) task(t *testing.T, g workflows.WorkflowGraph, key string, iter int64) workflows.WorkflowTask {
	t.Helper()
	for _, x := range g.Tasks {
		if x.Key == key && x.Iteration == iter {
			return x
		}
	}
	t.Fatalf("task %s#%d not found", key, iter)
	return workflows.WorkflowTask{}
}

func (e *env) graph(t *testing.T, id int64) workflows.WorkflowGraph {
	t.Helper()
	g, err := e.wf.Graph(t.Context(), coretest.Viewer, id)
	require.NoError(t, err)
	return g
}

func (e *env) allowPolicy(t *testing.T) {
	t.Helper()
	doc := policies.SuggestedDefaults()
	pct, perDay := 20, 50
	doc.Caps = policies.PolicyCaps{DailySpendIncreasePct: &pct, MaxChangesPerDay: &perDay}
	doc.WriteAccounts = []int64{e.acct}
	doc.Actions[policies.BudgetLower] = policies.PolicyRule{Marketer: "needs_approval", Admin: "allowed"}
	_, err := e.policies.Put(t.Context(), coretest.Admin, doc)
	require.NoError(t, err)
	require.NoError(t, e.db.Writer().UpsertAccountCapability(t.Context(), store.UpsertAccountCapabilityParams{AccountID: e.acct, Capability: "write.budget", State: "verified", VerifiedAt: sql.NullTime{Time: e.clock.Now(), Valid: true}, CheckedAt: e.clock.Now()}))
}

func proposeScript(acct int64) func(workflows.RunRequest) (workflows.RunResult, error) {
	return func(req workflows.RunRequest) (workflows.RunResult, error) {
		out, err := req.Dispatch(context.Background(), "propose_change", map[string]any{"accountId": acct, "action": "budget.lower", "targetKind": "campaign", "targetRef": "g1", "targetVersion": "v1", "reason": "spend is high", "expectedCurrent": map[string]any{"dailyBudgetMinor": 20000}, "proposed": map[string]any{"dailyBudgetMinor": 10000}})
		if err != nil {
			return workflows.RunResult{Tokens: 5}, err
		}
		m := out.(map[string]any)
		return workflows.RunResult{Output: json.RawMessage(`{"note":"proposed"}`), Tokens: 20, ExternalRef: m["externalRef"].(string)}, nil
	}
}

// Catches: a step that proposed before a crash proposing again after the
// lease expires, and a settled proposal not completing the step.
func TestRestartDoesNotRepeatCompletedExternalAction(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.allowPolicy(t)
	e.workflow(t, "budget.review", registry.WorkflowDocument{Name: "Budget review", MaxSteps: 5, Steps: []registry.WorkflowStep{
		{Key: "read", Skill: "spend.read"},
		{Key: "suggest", Skill: "budget.suggest", DependsOn: []string{"read"}, External: true},
	}})
	e.runner.scripts["spend.read"] = func(req workflows.RunRequest) (workflows.RunResult, error) {
		out, err := req.Dispatch(ctx, "get_analytics", map[string]any{"period": "last_7"})
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"spendMinor": 123456}, out)
		return workflows.RunResult{Output: json.RawMessage(`{"spend":1234.56}`), Tokens: 30}, nil
	}
	// The proposing step records its proposal, then "crashes": the runner
	// returns an error after the dispatch succeeded.
	crashed := false
	e.runner.scripts["budget.suggest"] = func(req workflows.RunRequest) (workflows.RunResult, error) {
		_, err := req.Dispatch(ctx, "propose_change", map[string]any{"accountId": e.acct, "action": "budget.lower", "targetKind": "campaign", "targetRef": "g1", "targetVersion": "v1", "reason": "spend is high", "expectedCurrent": map[string]any{"dailyBudgetMinor": 20000}, "proposed": map[string]any{"dailyBudgetMinor": 10000}})
		require.NoError(t, err)
		if !crashed {
			crashed = true
			return workflows.RunResult{Tokens: 20}, errors.New("process died")
		}
		t.Fatal("the proposing step ran again after its proposal was recorded")
		return workflows.RunResult{}, nil
	}
	g, err := e.wf.Start(ctx, coretest.Marketer, workflows.WorkflowStartInput{Definition: "budget.review"})
	require.NoError(t, err)
	assert.Equal(t, "running", g.Run.State)
	assert.Equal(t, "ready", e.task(t, g, "read", 0).State)
	assert.Equal(t, "pending", e.task(t, g, "suggest", 0).State)
	e.tick(t) // read runs; suggest becomes ready
	e.tick(t) // suggest proposes then crashes: back to ready with the ref kept
	g = e.graph(t, g.Run.ID)
	read := e.task(t, g, "read", 0)
	assert.Equal(t, "done", read.State)
	assert.Equal(t, int64(1), read.OutputVersion)
	assert.Contains(t, string(read.Output), "1234.56")
	suggest := e.task(t, g, "suggest", 0)
	assert.Equal(t, "ready", suggest.State, "the failed attempt is retried")
	assert.Equal(t, "proposal:1", suggest.ExternalRef, "the proposal survived the crash")
	assert.Equal(t, "wf:1:suggest:0", *suggest.OperationKey)
	e.tick(t) // retry: the ref exists, so no second proposal; the step waits on it
	g = e.graph(t, g.Run.ID)
	suggest = e.task(t, g, "suggest", 0)
	assert.Equal(t, "blocked", suggest.State)
	assert.Contains(t, suggest.Reason, "waiting on external action proposal:1")
	assert.Equal(t, "blocked", g.Run.State)
	list, _ := e.proposals.List(ctx, coretest.Viewer, "", 10)
	require.Len(t, list, 1, "exactly one proposal")
	assert.Equal(t, "proposed", list[0].State, "a marketer-level actor never self-approves")
	assert.Equal(t, 1, e.runner.calls["budget.suggest"])
	kinds := map[string]int{}
	for _, ev := range g.Events {
		kinds[ev.Kind]++
	}
	assert.Equal(t, 1, kinds["not_repeated"])

	// A person approves and the outbox executes: the step completes.
	_, err = e.proposals.Approve(ctx, coretest.Admin, list[0].ID, "fine")
	require.NoError(t, err)
	_, _, err = e.proposals.Execute(ctx, coretest.Admin, list[0].ID, "exec-1")
	require.NoError(t, err)
	e.tick(t)
	g = e.graph(t, g.Run.ID)
	assert.Equal(t, "done", e.task(t, g, "suggest", 0).State)
	assert.Equal(t, "done", g.Run.State)
	assert.Equal(t, int64(50), g.Run.SpentTokens)
	// A done run cannot be paused; a lease expiry on nothing changes nothing.
	_, err = e.wf.Pause(ctx, coretest.Marketer, g.Run.ID, "x")
	require.Error(t, err)
}

// Catches: a corrected upstream result leaving downstream results standing,
// and an external dependent re-firing on its own.
func TestDependencyVersionChangeInvalidatesDownstream(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.allowPolicy(t)
	e.workflow(t, "chain", registry.WorkflowDocument{Name: "Chain", MaxSteps: 10, Steps: []registry.WorkflowStep{
		{Key: "a", Skill: "spend.read"},
		{Key: "b", Skill: "spend.read", DependsOn: []string{"a"}, Inputs: map[string]string{"from": "$a.spend"}},
		{Key: "c", Skill: "spend.read", DependsOn: []string{"b"}},
		{Key: "d", Skill: "budget.suggest", DependsOn: []string{"a"}, External: true},
	}})
	e.runner.scripts["spend.read"] = func(req workflows.RunRequest) (workflows.RunResult, error) {
		return workflows.RunResult{Output: json.RawMessage(`{"spend":100}`), Tokens: 1}, nil
	}
	e.runner.scripts["budget.suggest"] = proposeScript(e.acct)
	g, err := e.wf.Start(ctx, coretest.Marketer, workflows.WorkflowStartInput{Definition: "chain"})
	require.NoError(t, err)
	for i := 0; i < 4; i++ {
		e.tick(t)
	}
	g = e.graph(t, g.Run.ID)
	assert.Equal(t, "done", e.task(t, g, "c", 0).State)
	b := e.task(t, g, "b", 0)
	assert.Contains(t, string(b.ResolvedInput), `"from":100`, "inputs are resolved from upstream outputs")
	hashBefore := b.InputHash
	assert.Equal(t, "blocked", e.task(t, g, "d", 0).State, "the proposal waits for a person")
	// A person corrects a's output.
	_, err = e.wf.Correct(ctx, coretest.Viewer, e.task(t, g, "a", 0).ID, json.RawMessage(`{"spend":900}`), "wrong account")
	assert.ErrorIs(t, err, core.ErrForbidden)
	g, err = e.wf.Correct(ctx, coretest.Marketer, e.task(t, g, "a", 0).ID, json.RawMessage(`{"spend":900}`), "wrong account")
	require.NoError(t, err)
	a := e.task(t, g, "a", 0)
	assert.Equal(t, int64(2), a.OutputVersion)
	assert.Equal(t, "ready", e.task(t, g, "b", 0).State, "b is invalidated and re-queued")
	assert.Equal(t, "ready", e.task(t, g, "c", 0).State, "c too, transitively")
	d := e.task(t, g, "d", 0)
	assert.Equal(t, "blocked", d.State)
	assert.Contains(t, d.Reason, "a person decides", "an external step never re-fires on its own")
	assert.Equal(t, "running", g.Run.State)
	for i := 0; i < 3; i++ {
		e.tick(t)
	}
	g = e.graph(t, g.Run.ID)
	b = e.task(t, g, "b", 0)
	assert.Equal(t, "done", b.State)
	assert.NotEqual(t, hashBefore, b.InputHash, "the new result is tied to the new upstream version")
	assert.Contains(t, string(b.ResolvedInput), `"from":900`)
	assert.Equal(t, int64(2), b.OutputVersion)
	list, _ := e.proposals.List(ctx, coretest.Viewer, "", 10)
	assert.Len(t, list, 1, "still one proposal")
	// Retrying an external task that already acted is refused.
	_, err = e.wf.Retry(ctx, coretest.Marketer, d.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already acted externally")
}

// Catches: a run built from a cycle or an unbounded loop, a loop running
// past its bound, and a budget overrun.
func TestCyclesUnboundedLoopsAndBudgetsAreRefused(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.workflow(t, "loop", registry.WorkflowDocument{Name: "Loop", MaxSteps: 10, BudgetTokens: 25, Steps: []registry.WorkflowStep{
		{Key: "poll", Skill: "spend.read", Loop: &registry.LoopSpec{Max: 3}},
	}})
	e.runner.scripts["spend.read"] = func(req workflows.RunRequest) (workflows.RunResult, error) {
		return workflows.RunResult{Output: json.RawMessage(`{"spend":1,"again":true}`), Tokens: 10}, nil
	}
	g, err := e.wf.Start(ctx, coretest.Marketer, workflows.WorkflowStartInput{Definition: "loop"})
	require.NoError(t, err)
	for i := 0; i < 6; i++ {
		e.tick(t)
	}
	g = e.graph(t, g.Run.ID)
	iterations := 0
	for _, x := range g.Tasks {
		if x.Key == "poll" {
			iterations++
		}
	}
	assert.Equal(t, 3, iterations, "three passes, then the bound; the output still asked for more")
	loopBound := 0
	for _, ev := range g.Events {
		if ev.Kind == "loop_bound" {
			loopBound++
		}
	}
	// The budget of 25 tokens is spent after the third pass's start: passes
	// 1 and 2 cost 20; pass 3 ran (budget checked before, 20 < 25) and the
	// run then has 30 spent, so a fourth would have been blocked anyway.
	assert.Equal(t, int64(30), g.Run.SpentTokens)
	assert.Equal(t, 1, loopBound)
	assert.Equal(t, "done", g.Run.State)

	// A tighter budget blocks before the second pass.
	e.workflow(t, "tight", registry.WorkflowDocument{Name: "Tight", MaxSteps: 10, BudgetTokens: 10, Steps: []registry.WorkflowStep{{Key: "poll", Skill: "spend.read", Loop: &registry.LoopSpec{Max: 3}}}})
	g2, err := e.wf.Start(ctx, coretest.Marketer, workflows.WorkflowStartInput{Definition: "tight"})
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		e.tick(t)
	}
	g2 = e.graph(t, g2.Run.ID)
	assert.Equal(t, "blocked", g2.Run.State)
	second := e.task(t, g2, "poll", 1)
	assert.Equal(t, "blocked", second.State)
	assert.Contains(t, second.Reason, "budget of 10 tokens is spent")
	// A cycle cannot even be activated, so no run can be built from it; the
	// orchestrator checks again anyway.
	w, err := e.reg.SaveWorkflow(ctx, coretest.Admin, "cyclic", registry.WorkflowDocument{Name: "Cyclic", MaxSteps: 5, Steps: []registry.WorkflowStep{{Key: "x", Skill: "spend.read", DependsOn: []string{"y"}}, {Key: "y", Skill: "spend.read", DependsOn: []string{"x"}}}}, false)
	require.NoError(t, err)
	assert.NotEmpty(t, w.Validation)
	_, err = e.reg.Activate(ctx, coretest.Admin, w.ID)
	require.Error(t, err)
	_, err = e.wf.Start(ctx, coretest.Marketer, workflows.WorkflowStartInput{Definition: "cyclic"})
	require.Error(t, err)
	_, err = e.wf.Start(ctx, coretest.Viewer, workflows.WorkflowStartInput{Definition: "loop"})
	assert.ErrorIs(t, err, core.ErrForbidden)
	// Pause stops leasing; resume continues.
	e.workflow(t, "two", registry.WorkflowDocument{Name: "Two", MaxSteps: 5, Steps: []registry.WorkflowStep{{Key: "a", Skill: "spend.read"}, {Key: "b", Skill: "spend.read", DependsOn: []string{"a"}}}})
	g3, _ := e.wf.Start(ctx, coretest.Marketer, workflows.WorkflowStartInput{Definition: "two", BudgetTokens: 1000})
	g3, err = e.wf.Pause(ctx, coretest.Marketer, g3.Run.ID, "hold")
	require.NoError(t, err)
	e.tick(t)
	g3 = e.graph(t, g3.Run.ID)
	assert.Equal(t, "ready", e.task(t, g3, "a", 0).State, "paused runs lease nothing")
	_, err = e.wf.Resume(ctx, coretest.Marketer, g3.Run.ID)
	require.NoError(t, err)
	e.tick(t)
	e.tick(t)
	g3 = e.graph(t, g3.Run.ID)
	assert.Equal(t, "done", g3.Run.State)
}

// Catches: a step proposing a change without policy, a tool outside the
// skill's permissions being called, and anything reaching an outside system
// without a proposal.
func TestNoToolCallBypassesPolicy(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	// No policy is configured: the proposal is denied and the step fails
	// with the policy's reason; nothing is queued for execution.
	e.workflow(t, "suggest", registry.WorkflowDocument{Name: "Suggest", MaxSteps: 5, Steps: []registry.WorkflowStep{{Key: "s", Skill: "budget.suggest", External: true}}})
	e.runner.scripts["budget.suggest"] = proposeScript(e.acct)
	g, err := e.wf.Start(ctx, coretest.Marketer, workflows.WorkflowStartInput{Definition: "suggest"})
	require.NoError(t, err)
	e.tick(t)
	e.tick(t)
	g = e.graph(t, g.Run.ID)
	s := e.task(t, g, "s", 0)
	assert.Equal(t, "failed", s.State)
	assert.Contains(t, s.Reason, "denied")
	list, _ := e.proposals.List(ctx, coretest.Viewer, "", 10)
	require.Len(t, list, 1)
	assert.Equal(t, "denied", list[0].State)
	assert.Equal(t, policies.ReasonPolicyUnconfigured, list[0].PolicyReason)
	var intents int
	require.NoError(t, e.db.Read.QueryRowContext(ctx, "SELECT COUNT(*) FROM execution_intents").Scan(&intents))
	assert.Zero(t, intents, "nothing reached the outbox")
	assert.Equal(t, "failed", g.Run.State)

	// A skill that only reads cannot call the proposing tool, however the
	// runner was prompted.
	e.workflow(t, "sneaky", registry.WorkflowDocument{Name: "Sneaky", MaxSteps: 5, Steps: []registry.WorkflowStep{{Key: "r", Skill: "spend.read"}}})
	var toolErr error
	e.runner.scripts["spend.read"] = func(req workflows.RunRequest) (workflows.RunResult, error) {
		_, toolErr = req.Dispatch(ctx, "propose_change", map[string]any{"accountId": e.acct, "action": "budget.lower"})
		return workflows.RunResult{Output: json.RawMessage(`{"spend":1}`), Tokens: 1}, nil
	}
	g2, _ := e.wf.Start(ctx, coretest.Marketer, workflows.WorkflowStartInput{Definition: "sneaky"})
	e.tick(t)
	require.Error(t, toolErr)
	assert.Contains(t, toolErr.Error(), "not permitted for skill spend.read")
	list, _ = e.proposals.List(ctx, coretest.Viewer, "", 10)
	assert.Len(t, list, 1, "no new proposal")
	g2 = e.graph(t, g2.Run.ID)
	assert.Equal(t, "done", g2.Run.State)
	// An output that does not fit the skill's schema is rejected, then fails
	// after the attempts run out.
	e.workflow(t, "bad", registry.WorkflowDocument{Name: "Bad", MaxSteps: 5, Steps: []registry.WorkflowStep{{Key: "r", Skill: "spend.read"}}})
	e.runner.scripts["spend.read"] = func(req workflows.RunRequest) (workflows.RunResult, error) {
		return workflows.RunResult{Output: json.RawMessage(`{"spend":"lots"}`), Tokens: 1}, nil
	}
	g3, _ := e.wf.Start(ctx, coretest.Marketer, workflows.WorkflowStartInput{Definition: "bad"})
	e.tick(t)
	e.tick(t)
	g3 = e.graph(t, g3.Run.ID)
	r := e.task(t, g3, "r", 0)
	assert.Equal(t, "failed", r.State)
	assert.Contains(t, r.Reason, "schema")
	assert.Equal(t, int64(2), r.Attempt)
	// Retry gives it fresh attempts; a fixed runner completes it.
	e.runner.scripts["spend.read"] = func(req workflows.RunRequest) (workflows.RunResult, error) {
		return workflows.RunResult{Output: json.RawMessage(`{"spend":2}`), Tokens: 1}, nil
	}
	g3, err = e.wf.Retry(ctx, coretest.Marketer, r.ID)
	require.NoError(t, err)
	assert.Equal(t, "running", g3.Run.State)
	e.tick(t)
	g3 = e.graph(t, g3.Run.ID)
	assert.Equal(t, "done", g3.Run.State)
}
