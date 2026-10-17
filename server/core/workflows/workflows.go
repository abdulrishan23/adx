// Package workflows runs registry workflow definitions as durable task
// graphs: tasks are leased, attempted and resumed; an external step's action
// happens once under a stable operation key however many times the task is
// retried or the process restarts; a changed upstream result invalidates
// what depended on it; loops are bounded by the definition; token budgets
// stop a run before it overspends. Every tool a step may call goes through
// the dispatcher, where a proposal is the only way to change the outside.
package workflows

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/registry"
	"github.com/WokeWorks/adx/server/store"
)

// Dispatcher runs one tool call for a skill. Read tools answer; the
// propose tools create a proposal and return its id and state. There is no
// path from here to an outside system except through a proposal.
type Dispatcher interface {
	Call(ctx context.Context, actor core.Actor, tool string, input map[string]any) (any, error)
}

// Runner produces a step's output from the skill, its input and the tools
// it may call. The model runner (in this package) asks a model; tests use a
// scripted one. A runner must only call tools through the dispatcher given.
type Runner interface {
	Run(ctx context.Context, actor core.Actor, req RunRequest) (RunResult, error)
}

type RunRequest struct {
	Skill    registry.SkillDocument
	SkillKey string
	Input    map[string]any
	Tools    []string
	// OperationKey is set for external steps; the runner passes it to any
	// proposing tool so the proposal is idempotent.
	OperationKey string
	Dispatch     func(ctx context.Context, tool string, input map[string]any) (any, error)
}

type RunResult struct {
	Output json.RawMessage
	Tokens int64
	// ExternalRef names the proposal (or other external record) a proposing
	// tool created, so the task remembers it across restarts.
	ExternalRef string
	// Blocked says the step is waiting on something outside the run (an
	// approval); the task stays blocked and is re-checked later.
	Blocked string
}

// ExternalChecker tells whether an external action recorded by ref has
// settled. The proposals service implements it.
type ExternalChecker interface {
	ExternalState(ctx context.Context, actor core.Actor, ref string) (state string, settled bool, failed bool, err error)
}

type WorkflowRun struct {
	ID                int64           `json:"id"`
	DefinitionKey     string          `json:"definitionKey"`
	DefinitionVersion int64           `json:"definitionVersion"`
	State             string          `json:"state"`
	Params            json.RawMessage `json:"params"`
	BudgetTokens      int64           `json:"budgetTokens"`
	SpentTokens       int64           `json:"spentTokens"`
	Reason            string          `json:"reason"`
	CreatedBy         *int64          `json:"createdBy"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	FinishedAt        *time.Time      `json:"finishedAt"`
}

type WorkflowTask struct {
	ID            int64           `json:"id"`
	RunID         int64           `json:"runId"`
	Key           string          `json:"key"`
	Iteration     int64           `json:"iteration"`
	Skill         string          `json:"skill"`
	Agent         string          `json:"agent"`
	DependsOn     []string        `json:"dependsOn"`
	State         string          `json:"state"`
	Attempt       int64           `json:"attempt"`
	MaxAttempts   int64           `json:"maxAttempts"`
	LeaseOwner    *string         `json:"leaseOwner"`
	LeaseUntil    *time.Time      `json:"leaseUntil"`
	Input         json.RawMessage `json:"input"`
	ResolvedInput json.RawMessage `json:"resolvedInput"`
	InputHash     string          `json:"inputHash"`
	Output        json.RawMessage `json:"output"`
	OutputVersion int64           `json:"outputVersion"`
	External      bool            `json:"external"`
	OperationKey  *string         `json:"operationKey"`
	ExternalRef   string          `json:"externalRef"`
	LoopMax       int64           `json:"loopMax"`
	BudgetTokens  int64           `json:"budgetTokens"`
	SpentTokens   int64           `json:"spentTokens"`
	Reason        string          `json:"reason"`
	CreatedAt     time.Time       `json:"createdAt"`
	StartedAt     *time.Time      `json:"startedAt"`
	FinishedAt    *time.Time      `json:"finishedAt"`
}

type WorkflowEvent struct {
	ID     int64     `json:"id"`
	TaskID *int64    `json:"taskId"`
	Kind   string    `json:"kind"`
	Detail string    `json:"detail"`
	At     time.Time `json:"at"`
}

type WorkflowGraph struct {
	Run    WorkflowRun     `json:"run"`
	Tasks  []WorkflowTask  `json:"tasks"`
	Events []WorkflowEvent `json:"events"`
}

type WorkflowStartInput struct {
	Definition   string          `json:"definition"`
	Params       json.RawMessage `json:"params"`
	BudgetTokens int64           `json:"budgetTokens"`
}

type Config struct {
	Clock    core.Clock
	Registry *registry.Registry
	Runner   Runner
	Dispatch Dispatcher
	External ExternalChecker
	// Owner names this process in leases.
	Owner string
	Lease time.Duration
	// MaxAttempts per task before it fails.
	MaxAttempts int64
	// DefaultBudget applies when a run names none. Zero means no limit.
	DefaultBudget int64
}

type Workflows struct {
	db    *store.DB
	cfg   Config
	clock core.Clock
}

func New(db *store.DB, cfg Config) *Workflows {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	if cfg.Owner == "" {
		cfg.Owner = "adx"
	}
	if cfg.Lease == 0 {
		cfg.Lease = 5 * time.Minute
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 3
	}
	return &Workflows{db: db, cfg: cfg, clock: cfg.Clock}
}

// workflowActor is who a run acts as: ADX's job identity at marketer
// level, so nothing it proposes is self-approved and every change waits for
// a person under policy.
func workflowActor(workspaceID int64) core.Actor {
	return core.Actor{UserID: 0, Role: core.RoleMarketer, Via: core.ViaJob, WorkspaceID: workspaceID}
}

// Start creates a run from the active definition and its first tasks.
// The definition was validated at activation; the graph is checked again
// here, because a run must never be built from a cycle or an unbounded loop.
func (w *Workflows) Start(ctx context.Context, actor core.Actor, in WorkflowStartInput) (WorkflowGraph, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return WorkflowGraph{}, core.ErrForbidden
	}
	if w.cfg.Registry == nil {
		return WorkflowGraph{}, core.Unavailable(nil, "the registry is not wired")
	}
	entry, doc, err := w.cfg.Registry.ActiveWorkflow(ctx, actor, in.Definition)
	if err != nil {
		return WorkflowGraph{}, err
	}
	if entry == nil {
		return WorkflowGraph{}, core.NotFound("workflow %q has no active version", in.Definition)
	}
	if problems, err := w.cfg.Registry.ValidateWorkflow(ctx, actor, doc); err != nil {
		return WorkflowGraph{}, err
	} else if len(problems) > 0 {
		return WorkflowGraph{}, core.Invalid("the definition is not runnable: %s", strings.Join(problems, "; "))
	}
	params := in.Params
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	if !json.Valid(params) {
		return WorkflowGraph{}, core.Invalid("params must be JSON")
	}
	budget := in.BudgetTokens
	if budget == 0 {
		budget = doc.BudgetTokens
	}
	if budget == 0 {
		budget = w.cfg.DefaultBudget
	}
	now := w.clock.Now().UTC()
	var run store.WorkflowRun
	err = w.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		run, err = q.CreateWorkflowRun(ctx, store.CreateWorkflowRunParams{WorkspaceID: actor.WorkspaceID, DefinitionKey: entry.Key, DefinitionVersion: entry.Version, Params: string(params), BudgetTokens: budget, CreatedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		for _, s := range doc.Steps {
			state := "pending"
			if len(s.DependsOn) == 0 {
				state = "ready"
			}
			deps, _ := json.Marshal(nonNil(s.DependsOn))
			var opKey sql.NullString
			if s.External {
				opKey = sql.NullString{String: fmt.Sprintf("wf:%d:%s:0", run.ID, s.Key), Valid: true}
			}
			var loopMax int64
			if s.Loop != nil {
				loopMax = int64(s.Loop.Max)
			}
			inputs, _ := json.Marshal(s.Inputs)
			if _, err := q.CreateWorkflowTask(ctx, store.CreateWorkflowTaskParams{RunID: run.ID, Key: s.Key, Iteration: 0, Skill: s.Skill, Agent: s.Agent, DependsOn: string(deps), State: state, MaxAttempts: w.cfg.MaxAttempts, Input: string(inputs), External: boolInt(s.External), OperationKey: opKey, LoopMax: loopMax, CreatedAt: now, UpdatedAt: now}); err != nil {
				return err
			}
		}
		return q.CreateWorkflowEvent(ctx, store.CreateWorkflowEventParams{RunID: run.ID, Kind: "started", Detail: fmt.Sprintf("%s v%d by user %d", entry.Key, entry.Version, actor.UserID), At: now})
	})
	if err != nil {
		return WorkflowGraph{}, err
	}
	return w.Graph(ctx, actor, run.ID)
}

// Tick advances every running run: resumes expired leases, settles blocked
// external tasks, runs ready tasks, and finishes runs whose tasks are all
// settled. It is what the periodic job calls; tests call it directly.
func (w *Workflows) Tick(ctx context.Context) (int, error) {
	now := w.clock.Now().UTC()
	resumed, err := w.db.Writer().ReleaseExpiredWorkflowLeases(ctx, store.ReleaseExpiredWorkflowLeasesParams{UpdatedAt: now, LeaseUntil: sql.NullTime{Time: now, Valid: true}})
	if err != nil {
		return 0, err
	}
	for _, t := range resumed {
		_ = w.db.Writer().CreateWorkflowEvent(ctx, store.CreateWorkflowEventParams{RunID: t.RunID, TaskID: sql.NullInt64{Int64: t.ID, Valid: true}, Kind: "resumed", Detail: fmt.Sprintf("lease expired after attempt %d", t.Attempt), At: now})
	}
	worked := 0
	for _, state := range []string{"running", "blocked"} {
		runs, err := w.db.Reader().ListWorkflowRunsByState(ctx, store.ListWorkflowRunsByStateParams{State: state, Limit: 100})
		if err != nil {
			return worked, err
		}
		for _, run := range runs {
			n, err := w.advance(ctx, run)
			if err != nil {
				return worked, err
			}
			worked += n
		}
	}
	return worked, nil
}

func (w *Workflows) advance(ctx context.Context, run store.WorkflowRun) (int, error) {
	actor := workflowActor(run.WorkspaceID)
	tasks, err := w.db.Reader().ListWorkflowTasks(ctx, run.ID)
	if err != nil {
		return 0, err
	}
	worked := 0
	// Blocked external tasks: has the proposal settled?
	for _, t := range tasks {
		if t.State == "blocked" && t.External == 1 && t.ExternalRef != "" && w.cfg.External != nil {
			state, settled, failed, err := w.cfg.External.ExternalState(ctx, actor, t.ExternalRef)
			if err != nil {
				continue
			}
			if !settled {
				continue
			}
			out, _ := json.Marshal(map[string]any{"externalRef": t.ExternalRef, "state": state})
			if failed {
				_ = w.setState(ctx, t, "failed", "external action "+t.ExternalRef+" ended as "+state)
			} else {
				_ = w.finish(ctx, run, t, out, 0, t.InputHash, true)
			}
			worked++
		}
	}
	// Ready tasks: lease and run, one at a time, in id order.
	tasks, err = w.db.Reader().ListWorkflowTasks(ctx, run.ID)
	if err != nil {
		return worked, err
	}
	if err := w.promote(ctx, run, tasks); err != nil {
		return worked, err
	}
	tasks, _ = w.db.Reader().ListWorkflowTasks(ctx, run.ID)
	if run.State == "running" || run.State == "blocked" {
		for _, t := range tasks {
			if t.State != "ready" {
				continue
			}
			if err := w.execute(ctx, run, t); err != nil {
				return worked, err
			}
			worked++
		}
	}
	return worked, w.settleRun(ctx, run)
}

// promote makes pending tasks ready once every dependency is done, and
// marks tasks whose dependencies failed as skipped.
func (w *Workflows) promote(ctx context.Context, run store.WorkflowRun, tasks []store.WorkflowTask) error {
	latest := map[string]store.WorkflowTask{}
	for _, t := range tasks {
		if cur, ok := latest[t.Key]; !ok || t.Iteration > cur.Iteration {
			latest[t.Key] = t
		}
	}
	now := w.clock.Now().UTC()
	for _, t := range tasks {
		if t.State != "pending" {
			continue
		}
		var deps []string
		_ = json.Unmarshal([]byte(t.DependsOn), &deps)
		ready, dead := true, ""
		for _, d := range deps {
			dt, ok := latest[d]
			if !ok {
				dead = "depends on unknown step " + d
				break
			}
			switch dt.State {
			case "done":
			case "failed", "skipped":
				dead = "dependency " + d + " " + dt.State
			default:
				ready = false
			}
		}
		if dead != "" {
			if _, err := w.db.Writer().SetWorkflowTaskState(ctx, store.SetWorkflowTaskStateParams{State: "skipped", Reason: dead, UpdatedAt: now, FinishedAt: sql.NullTime{Time: now, Valid: true}, ID: t.ID, State_2: "pending"}); err != nil {
				return err
			}
			continue
		}
		if ready {
			if _, err := w.db.Writer().SetWorkflowTaskState(ctx, store.SetWorkflowTaskStateParams{State: "ready", Reason: "", UpdatedAt: now, ID: t.ID, State_2: "pending"}); err != nil {
				return err
			}
		}
	}
	return nil
}

// execute leases one ready task and runs it. A proposing step that already
// recorded an external ref (a restart after the proposal was made) is not
// run again: it goes straight to checking the proposal.
func (w *Workflows) execute(ctx context.Context, run store.WorkflowRun, t store.WorkflowTask) error {
	actor := workflowActor(run.WorkspaceID)
	now := w.clock.Now().UTC()
	owner := fmt.Sprintf("%s/%d", w.cfg.Owner, now.UnixNano())
	n, err := w.db.Writer().LeaseWorkflowTask(ctx, store.LeaseWorkflowTaskParams{LeaseOwner: sql.NullString{String: owner, Valid: true}, LeaseUntil: sql.NullTime{Time: now.Add(w.cfg.Lease), Valid: true}, StartedAt: sql.NullTime{Time: now, Valid: true}, UpdatedAt: now, ID: t.ID})
	if err != nil || n != 1 {
		return err
	}
	t.Attempt++
	t.State = "leased"
	t.LeaseOwner = sql.NullString{String: owner, Valid: true}
	if t.Attempt > t.MaxAttempts {
		return w.setState(ctx, t, "failed", fmt.Sprintf("gave up after %d attempts", t.MaxAttempts))
	}
	// Budget: a run over budget blocks before spending more.
	if run.BudgetTokens > 0 && run.SpentTokens >= run.BudgetTokens {
		if err := w.setState(ctx, t, "blocked", fmt.Sprintf("run budget of %d tokens is spent (%d used); raise it or cancel", run.BudgetTokens, run.SpentTokens)); err != nil {
			return err
		}
		_, _ = w.db.Writer().SetWorkflowRunState(ctx, store.SetWorkflowRunStateParams{State: "blocked", Reason: "budget spent", UpdatedAt: now, ID: run.ID, State_2: "running"})
		return nil
	}
	if t.External == 1 && t.ExternalRef != "" {
		// Completed external action from an earlier attempt: never repeat it.
		_ = w.db.Writer().CreateWorkflowEvent(ctx, store.CreateWorkflowEventParams{RunID: run.ID, TaskID: sql.NullInt64{Int64: t.ID, Valid: true}, Kind: "not_repeated", Detail: "external action " + t.ExternalRef + " already recorded; checking its state instead of acting again", At: now})
		return w.setState(ctx, t, "blocked", "waiting on external action "+t.ExternalRef)
	}
	inputs, hash, err := w.resolveInputs(ctx, run, t)
	if err != nil {
		return w.setState(ctx, t, "failed", "inputs: "+err.Error())
	}
	raw, _ := json.Marshal(inputs)
	_ = w.db.Writer().SetWorkflowTaskResolvedInput(ctx, store.SetWorkflowTaskResolvedInputParams{ResolvedInput: string(raw), UpdatedAt: now, ID: t.ID})
	if w.cfg.Runner == nil || w.cfg.Registry == nil {
		return w.setState(ctx, t, "blocked", "no runner is wired on this server")
	}
	skillKey := t.Skill
	var tools []string
	if t.Agent != "" {
		_, ag, err := w.cfg.Registry.ActiveAgent(ctx, actor, t.Agent)
		if err != nil {
			return err
		}
		if len(ag.Skills) == 0 {
			return w.setState(ctx, t, "failed", "agent "+t.Agent+" has no skills to run")
		}
		skillKey = ag.Skills[0]
		tools, _ = w.cfg.Registry.AgentTools(ctx, actor, t.Agent)
	}
	entry, skill, err := w.cfg.Registry.ActiveSkill(ctx, actor, skillKey)
	if err != nil {
		return err
	}
	if entry == nil {
		return w.setState(ctx, t, "failed", "skill "+skillKey+" has no active version")
	}
	if tools == nil {
		tools = skill.PermittedTools
	}
	if problems := registry.ValidateJSON(skill.InputSchema, inputs); len(problems) > 0 && len(skill.InputSchema) > 0 {
		return w.setState(ctx, t, "failed", "input does not fit the skill: "+strings.Join(problems, "; "))
	}
	allowed := map[string]bool{}
	for _, x := range tools {
		allowed[x] = true
	}
	var externalRef string
	dispatch := func(ctx context.Context, tool string, input map[string]any) (any, error) {
		if !allowed[tool] {
			return nil, fmt.Errorf("tool %q is not permitted for skill %s", tool, skillKey)
		}
		if w.cfg.Dispatch == nil {
			return nil, errors.New("no tool dispatcher is wired")
		}
		if t.OperationKey.Valid {
			if input == nil {
				input = map[string]any{}
			}
			input["operationKey"] = t.OperationKey.String
		}
		out, err := w.cfg.Dispatch.Call(ctx, actor, tool, input)
		if err != nil {
			return nil, err
		}
		if m, ok := out.(map[string]any); ok {
			if ref, ok := m["externalRef"].(string); ok && ref != "" {
				externalRef = ref
				// Record at once: a crash after this line must not act again.
				_, _ = w.db.Writer().SetWorkflowTaskExternalRef(ctx, store.SetWorkflowTaskExternalRefParams{ExternalRef: ref, UpdatedAt: w.clock.Now().UTC(), ID: t.ID})
			}
		}
		return out, nil
	}
	res, runErr := w.cfg.Runner.Run(ctx, actor, RunRequest{Skill: skill, SkillKey: skillKey, Input: inputs, Tools: tools, OperationKey: t.OperationKey.String, Dispatch: dispatch})
	spent := res.Tokens
	if spent > 0 {
		_ = w.db.Writer().AddWorkflowRunSpend(ctx, store.AddWorkflowRunSpendParams{SpentTokens: spent, UpdatedAt: w.clock.Now().UTC(), ID: run.ID})
	}
	if runErr != nil {
		if t.Attempt >= t.MaxAttempts {
			return w.setState(ctx, t, "failed", safe(runErr))
		}
		return w.setState(ctx, t, "ready", "attempt "+fmt.Sprint(t.Attempt)+" failed: "+safe(runErr))
	}
	if res.ExternalRef != "" && externalRef == "" {
		externalRef = res.ExternalRef
		_, _ = w.db.Writer().SetWorkflowTaskExternalRef(ctx, store.SetWorkflowTaskExternalRefParams{ExternalRef: externalRef, UpdatedAt: w.clock.Now().UTC(), ID: t.ID})
	}
	if res.Blocked != "" || (t.External == 1 && externalRef != "") {
		why := res.Blocked
		if why == "" {
			why = "waiting on external action " + externalRef
		}
		return w.setState(ctx, t, "blocked", why)
	}
	output := res.Output
	if len(output) == 0 {
		output = json.RawMessage(`null`)
	}
	if problems := registry.ValidateRaw(skill.OutputSchema, output); len(problems) > 0 {
		if t.Attempt >= t.MaxAttempts {
			return w.setState(ctx, t, "failed", "output does not fit the skill's schema: "+strings.Join(problems, "; "))
		}
		return w.setState(ctx, t, "ready", "output rejected by schema: "+strings.Join(problems, "; "))
	}
	return w.finish(ctx, run, t, output, spent, hash, false)
}

// resolveInputs builds a task's input from the run params and the outputs
// of its dependencies, and hashes those outputs with their versions.
func (w *Workflows) resolveInputs(ctx context.Context, run store.WorkflowRun, t store.WorkflowTask) (map[string]any, string, error) {
	var deps []string
	_ = json.Unmarshal([]byte(t.DependsOn), &deps)
	tasks, err := w.db.Reader().ListWorkflowTasks(ctx, run.ID)
	if err != nil {
		return nil, "", err
	}
	latest := map[string]store.WorkflowTask{}
	for _, x := range tasks {
		if x.State != "done" {
			continue
		}
		if cur, ok := latest[x.Key]; !ok || x.Iteration > cur.Iteration {
			latest[x.Key] = x
		}
	}
	var params map[string]any
	_ = json.Unmarshal([]byte(run.Params), &params)
	inputs := map[string]any{"params": params}
	h := sha256.New()
	sort.Strings(deps)
	for _, d := range deps {
		dt, ok := latest[d]
		if !ok {
			return nil, "", fmt.Errorf("dependency %s has no result", d)
		}
		var out any
		_ = json.Unmarshal([]byte(dt.Output), &out)
		inputs[d] = out
		fmt.Fprintf(h, "%s@%d:%s\n", d, dt.OutputVersion, dt.Output)
	}
	var tmpl map[string]string
	_ = json.Unmarshal([]byte(t.Input), &tmpl)
	for k, v := range tmpl {
		if strings.HasPrefix(v, "$") {
			inputs[k] = lookup(inputs, strings.TrimPrefix(v, "$"))
		} else {
			inputs[k] = v
		}
	}
	// The previous iteration of a loop is an input too.
	if t.Iteration > 0 {
		for _, x := range tasks {
			if x.Key == t.Key && x.Iteration == t.Iteration-1 && x.State == "done" {
				var out any
				_ = json.Unmarshal([]byte(x.Output), &out)
				inputs["previous"] = out
				fmt.Fprintf(h, "previous@%d:%s\n", x.OutputVersion, x.Output)
			}
		}
	}
	return inputs, hex.EncodeToString(h.Sum(nil)), nil
}

func lookup(m map[string]any, path string) any {
	var cur any = m
	for _, p := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[p]
	}
	return cur
}

// finish records a task's output and, for loops, schedules the next pass.
func (w *Workflows) finish(ctx context.Context, run store.WorkflowRun, t store.WorkflowTask, output json.RawMessage, spent int64, hash string, fromBlocked bool) error {
	now := w.clock.Now().UTC()
	from := "leased"
	var owner sql.NullString
	if fromBlocked {
		from = "blocked"
	} else {
		owner = t.LeaseOwner
	}
	n, err := w.db.Writer().FinishWorkflowTask(ctx, store.FinishWorkflowTaskParams{Output: string(output), InputHash: hash, SpentTokens: spent, UpdatedAt: now, FinishedAt: sql.NullTime{Time: now, Valid: true}, ID: t.ID, State: from, LeaseOwner: owner})
	if err != nil {
		return err
	}
	if n != 1 {
		return nil // lease lost; the resumed attempt owns the task now
	}
	_ = w.db.Writer().CreateWorkflowEvent(ctx, store.CreateWorkflowEventParams{RunID: run.ID, TaskID: sql.NullInt64{Int64: t.ID, Valid: true}, Kind: "done", Detail: fmt.Sprintf("%s#%d attempt %d, %d tokens", t.Key, t.Iteration, t.Attempt, spent), At: now})
	// Loops: another pass while the output asks for one and the bound allows.
	if t.LoopMax > 0 {
		var out struct {
			Again bool `json:"again"`
		}
		_ = json.Unmarshal(output, &out)
		if out.Again && t.Iteration+1 < t.LoopMax {
			var opKey sql.NullString
			if t.External == 1 {
				opKey = sql.NullString{String: fmt.Sprintf("wf:%d:%s:%d", run.ID, t.Key, t.Iteration+1), Valid: true}
			}
			if _, err := w.db.Writer().CreateWorkflowTask(ctx, store.CreateWorkflowTaskParams{RunID: run.ID, Key: t.Key, Iteration: t.Iteration + 1, Skill: t.Skill, Agent: t.Agent, DependsOn: t.DependsOn, State: "ready", MaxAttempts: t.MaxAttempts, Input: t.Input, External: t.External, OperationKey: opKey, LoopMax: t.LoopMax, CreatedAt: now, UpdatedAt: now}); err != nil {
				return err
			}
		} else if out.Again {
			_ = w.db.Writer().CreateWorkflowEvent(ctx, store.CreateWorkflowEventParams{RunID: run.ID, TaskID: sql.NullInt64{Int64: t.ID, Valid: true}, Kind: "loop_bound", Detail: fmt.Sprintf("%s asked to run again but the bound of %d passes is reached", t.Key, t.LoopMax), At: now})
		}
	}
	return nil
}

func (w *Workflows) setState(ctx context.Context, t store.WorkflowTask, state, reason string) error {
	now := w.clock.Now().UTC()
	var finished sql.NullTime
	if state == "failed" || state == "skipped" {
		finished = sql.NullTime{Time: now, Valid: true}
	}
	from := "leased"
	if t.State != "leased" && t.State != "" {
		from = t.State
	}
	n, err := w.db.Writer().SetWorkflowTaskState(ctx, store.SetWorkflowTaskStateParams{State: state, Reason: reason, UpdatedAt: now, FinishedAt: finished, ID: t.ID, State_2: from})
	if err != nil {
		return err
	}
	if n == 1 {
		_ = w.db.Writer().CreateWorkflowEvent(ctx, store.CreateWorkflowEventParams{RunID: t.RunID, TaskID: sql.NullInt64{Int64: t.ID, Valid: true}, Kind: state, Detail: reason, At: now})
	}
	return nil
}

// settleRun finishes a run whose tasks are all settled, blocks one with a
// blocked task and nothing runnable, and fails one with a failed task.
func (w *Workflows) settleRun(ctx context.Context, run store.WorkflowRun) error {
	tasks, err := w.db.Reader().ListWorkflowTasks(ctx, run.ID)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, t := range tasks {
		counts[t.State]++
	}
	now := w.clock.Now().UTC()
	switch {
	case counts["failed"] > 0 && counts["ready"]+counts["leased"]+counts["pending"]+counts["blocked"] == 0:
		_, _ = w.db.Writer().SetWorkflowRunState(ctx, store.SetWorkflowRunStateParams{State: "failed", Reason: fmt.Sprintf("%d task(s) failed", counts["failed"]), UpdatedAt: now, FinishedAt: sql.NullTime{Time: now, Valid: true}, ID: run.ID, State_2: run.State})
	case counts["ready"]+counts["leased"]+counts["pending"]+counts["blocked"] == 0:
		_, _ = w.db.Writer().SetWorkflowRunState(ctx, store.SetWorkflowRunStateParams{State: "done", Reason: "", UpdatedAt: now, FinishedAt: sql.NullTime{Time: now, Valid: true}, ID: run.ID, State_2: run.State})
	case counts["blocked"] > 0 && counts["ready"]+counts["leased"]+counts["pending"] == 0 && run.State == "running":
		_, _ = w.db.Writer().SetWorkflowRunState(ctx, store.SetWorkflowRunStateParams{State: "blocked", Reason: "waiting on approvals or budget", UpdatedAt: now, ID: run.ID, State_2: "running"})
	case run.State == "blocked" && counts["blocked"] == 0:
		_, _ = w.db.Writer().SetWorkflowRunState(ctx, store.SetWorkflowRunStateParams{State: "running", Reason: "", UpdatedAt: now, ID: run.ID, State_2: "blocked"})
	}
	return nil
}

// Pause stops a run from leasing new tasks; a leased task finishes.
func (w *Workflows) Pause(ctx context.Context, actor core.Actor, runID int64, reason string) (WorkflowGraph, error) {
	return w.transition(ctx, actor, runID, []string{"running", "blocked"}, "paused", reason)
}

func (w *Workflows) Resume(ctx context.Context, actor core.Actor, runID int64) (WorkflowGraph, error) {
	return w.transition(ctx, actor, runID, []string{"paused", "blocked"}, "running", "")
}

func (w *Workflows) Cancel(ctx context.Context, actor core.Actor, runID int64, reason string) (WorkflowGraph, error) {
	return w.transition(ctx, actor, runID, []string{"running", "paused", "blocked"}, "canceled", reason)
}

func (w *Workflows) transition(ctx context.Context, actor core.Actor, runID int64, from []string, to, reason string) (WorkflowGraph, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return WorkflowGraph{}, core.ErrForbidden
	}
	run, err := w.run(ctx, actor.WorkspaceID, runID)
	if err != nil {
		return WorkflowGraph{}, err
	}
	ok := false
	for _, f := range from {
		if run.State == f {
			ok = true
		}
	}
	if !ok {
		return WorkflowGraph{}, core.Conflict("run %d is %s", runID, run.State)
	}
	now := w.clock.Now().UTC()
	var finished sql.NullTime
	if to == "canceled" {
		finished = sql.NullTime{Time: now, Valid: true}
	}
	if _, err := w.db.Writer().SetWorkflowRunState(ctx, store.SetWorkflowRunStateParams{State: to, Reason: reason, UpdatedAt: now, FinishedAt: finished, ID: runID, State_2: run.State}); err != nil {
		return WorkflowGraph{}, err
	}
	_ = w.db.Writer().CreateWorkflowEvent(ctx, store.CreateWorkflowEventParams{RunID: runID, Kind: to, Detail: fmt.Sprintf("by user %d: %s", actor.UserID, reason), At: now})
	return w.Graph(ctx, actor, runID)
}

// Retry puts a failed task back to ready with a fresh attempt budget.
func (w *Workflows) Retry(ctx context.Context, actor core.Actor, taskID int64) (WorkflowGraph, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return WorkflowGraph{}, core.ErrForbidden
	}
	t, run, err := w.task(ctx, actor.WorkspaceID, taskID)
	if err != nil {
		return WorkflowGraph{}, err
	}
	if t.State != "failed" && t.State != "blocked" {
		return WorkflowGraph{}, core.Conflict("task %d is %s, not failed or blocked", taskID, t.State)
	}
	if t.External == 1 && t.ExternalRef != "" {
		return WorkflowGraph{}, core.Conflict("task %d already acted externally (%s); it cannot be retried, only corrected", taskID, t.ExternalRef)
	}
	now := w.clock.Now().UTC()
	n, err := w.db.Writer().SetWorkflowTaskState(ctx, store.SetWorkflowTaskStateParams{State: "ready", Reason: fmt.Sprintf("retried by user %d", actor.UserID), UpdatedAt: now, ID: taskID, State_2: t.State})
	if err != nil {
		return WorkflowGraph{}, err
	}
	if n != 1 {
		return WorkflowGraph{}, core.Conflict("task %d changed underneath", taskID)
	}
	if _, err := w.db.Write.ExecContext(ctx, "UPDATE workflow_tasks SET attempt = 0, finished_at = NULL WHERE id = ?", taskID); err != nil {
		return WorkflowGraph{}, err
	}
	if run.State == "failed" || run.State == "blocked" {
		_, _ = w.db.Writer().SetWorkflowRunState(ctx, store.SetWorkflowRunStateParams{State: "running", Reason: "", UpdatedAt: now, ID: run.ID, State_2: run.State})
	}
	return w.Graph(ctx, actor, run.ID)
}

// Correct replaces a done task's output with a person's and invalidates
// everything downstream: read-only dependents are re-queued, external
// dependents are blocked for a person, because a changed input never
// re-fires an outside action on its own.
func (w *Workflows) Correct(ctx context.Context, actor core.Actor, taskID int64, output json.RawMessage, reason string) (WorkflowGraph, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return WorkflowGraph{}, core.ErrForbidden
	}
	if strings.TrimSpace(reason) == "" {
		return WorkflowGraph{}, core.Invalid("a reason is required")
	}
	if !json.Valid(output) {
		return WorkflowGraph{}, core.Invalid("output must be JSON")
	}
	t, run, err := w.task(ctx, actor.WorkspaceID, taskID)
	if err != nil {
		return WorkflowGraph{}, err
	}
	if t.State != "done" {
		return WorkflowGraph{}, core.Conflict("task %d is %s; only a done task's output can be corrected", taskID, t.State)
	}
	now := w.clock.Now().UTC()
	if _, err := w.db.Write.ExecContext(ctx, "UPDATE workflow_tasks SET output = ?, output_version = output_version + 1, reason = ?, updated_at = ? WHERE id = ?", string(output), "corrected by user "+fmt.Sprint(actor.UserID)+": "+reason, now, taskID); err != nil {
		return WorkflowGraph{}, err
	}
	_ = w.db.Writer().CreateWorkflowEvent(ctx, store.CreateWorkflowEventParams{RunID: run.ID, TaskID: sql.NullInt64{Int64: taskID, Valid: true}, Kind: "corrected", Detail: reason, At: now})
	if err := w.invalidateDownstream(ctx, run, t.Key); err != nil {
		return WorkflowGraph{}, err
	}
	if run.State == "done" || run.State == "blocked" {
		_, _ = w.db.Writer().SetWorkflowRunState(ctx, store.SetWorkflowRunStateParams{State: "running", Reason: "re-running after a correction", UpdatedAt: now, ID: run.ID, State_2: run.State})
	}
	return w.Graph(ctx, actor, run.ID)
}

// invalidateDownstream marks every task that depends, directly or through
// others, on key as invalidated and re-queues the read-only ones.
func (w *Workflows) invalidateDownstream(ctx context.Context, run store.WorkflowRun, key string) error {
	tasks, err := w.db.Reader().ListWorkflowTasks(ctx, run.ID)
	if err != nil {
		return err
	}
	affected := map[string]bool{key: true}
	changed := true
	for changed {
		changed = false
		for _, t := range tasks {
			if affected[t.Key] {
				continue
			}
			var deps []string
			_ = json.Unmarshal([]byte(t.DependsOn), &deps)
			for _, d := range deps {
				if affected[d] {
					affected[t.Key] = true
					changed = true
					break
				}
			}
		}
	}
	now := w.clock.Now().UTC()
	for _, t := range tasks {
		if t.Key == key || !affected[t.Key] {
			continue
		}
		if t.State == "failed" || t.State == "skipped" || t.State == "invalidated" {
			continue
		}
		state, reason := "ready", "upstream "+key+" changed; result invalidated and re-queued"
		if t.External == 1 {
			state, reason = "blocked", "upstream "+key+" changed after this step acted externally ("+t.ExternalRef+"); a person decides"
			if t.ExternalRef == "" {
				state = "ready"
				reason = "upstream " + key + " changed; re-queued before any external action"
			}
		}
		if t.State == "pending" {
			continue
		}
		if _, err := w.db.Writer().InvalidateWorkflowTask(ctx, store.InvalidateWorkflowTaskParams{State: state, Reason: reason, UpdatedAt: now, ID: t.ID}); err != nil {
			return err
		}
		_ = w.db.Writer().CreateWorkflowEvent(ctx, store.CreateWorkflowEventParams{RunID: run.ID, TaskID: sql.NullInt64{Int64: t.ID, Valid: true}, Kind: "invalidated", Detail: reason, At: now})
	}
	return nil
}

func (w *Workflows) Graph(ctx context.Context, actor core.Actor, runID int64) (WorkflowGraph, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return WorkflowGraph{}, core.ErrForbidden
	}
	run, err := w.run(ctx, actor.WorkspaceID, runID)
	if err != nil {
		return WorkflowGraph{}, err
	}
	g := WorkflowGraph{Run: runFromRow(run), Tasks: []WorkflowTask{}, Events: []WorkflowEvent{}}
	tasks, err := w.db.Reader().ListWorkflowTasks(ctx, runID)
	if err != nil {
		return WorkflowGraph{}, err
	}
	for _, t := range tasks {
		g.Tasks = append(g.Tasks, taskFromRow(t))
	}
	events, err := w.db.Reader().ListWorkflowEvents(ctx, runID)
	if err != nil {
		return WorkflowGraph{}, err
	}
	for _, e := range events {
		g.Events = append(g.Events, WorkflowEvent{ID: e.ID, TaskID: idPtr(e.TaskID), Kind: e.Kind, Detail: e.Detail, At: e.At.UTC()})
	}
	return g, nil
}

func (w *Workflows) List(ctx context.Context, actor core.Actor, limit int) ([]WorkflowRun, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := w.db.Reader().ListWorkflowRuns(ctx, store.ListWorkflowRunsParams{WorkspaceID: actor.WorkspaceID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]WorkflowRun, 0, len(rows))
	for _, r := range rows {
		out = append(out, runFromRow(r))
	}
	return out, nil
}

func (w *Workflows) run(ctx context.Context, workspaceID, id int64) (store.WorkflowRun, error) {
	row, err := w.db.Reader().GetWorkflowRun(ctx, store.GetWorkflowRunParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.WorkflowRun{}, core.NotFound("workflow run %d not found", id)
	}
	return row, err
}

func (w *Workflows) task(ctx context.Context, workspaceID, id int64) (store.WorkflowTask, store.WorkflowRun, error) {
	t, err := w.db.Reader().GetWorkflowTask(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return store.WorkflowTask{}, store.WorkflowRun{}, core.NotFound("task %d not found", id)
	}
	if err != nil {
		return store.WorkflowTask{}, store.WorkflowRun{}, err
	}
	run, err := w.run(ctx, workspaceID, t.RunID)
	if err != nil {
		return store.WorkflowTask{}, store.WorkflowRun{}, err
	}
	return t, run, nil
}

func runFromRow(r store.WorkflowRun) WorkflowRun {
	return WorkflowRun{ID: r.ID, DefinitionKey: r.DefinitionKey, DefinitionVersion: r.DefinitionVersion, State: r.State, Params: json.RawMessage(r.Params), BudgetTokens: r.BudgetTokens, SpentTokens: r.SpentTokens, Reason: r.Reason, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(), FinishedAt: timePtr(r.FinishedAt)}
}

func taskFromRow(r store.WorkflowTask) WorkflowTask {
	t := WorkflowTask{ID: r.ID, RunID: r.RunID, Key: r.Key, Iteration: r.Iteration, Skill: r.Skill, Agent: r.Agent, DependsOn: []string{}, State: r.State, Attempt: r.Attempt, MaxAttempts: r.MaxAttempts, Input: json.RawMessage(r.Input), ResolvedInput: json.RawMessage(r.ResolvedInput), InputHash: r.InputHash, Output: json.RawMessage(r.Output), OutputVersion: r.OutputVersion,
		External: r.External == 1, ExternalRef: r.ExternalRef, LoopMax: r.LoopMax, BudgetTokens: r.BudgetTokens, SpentTokens: r.SpentTokens, Reason: r.Reason, CreatedAt: r.CreatedAt.UTC(), StartedAt: timePtr(r.StartedAt), FinishedAt: timePtr(r.FinishedAt), LeaseUntil: timePtr(r.LeaseUntil)}
	_ = json.Unmarshal([]byte(r.DependsOn), &t.DependsOn)
	if r.LeaseOwner.Valid {
		s := r.LeaseOwner.String
		t.LeaseOwner = &s
	}
	if r.OperationKey.Valid {
		s := r.OperationKey.String
		t.OperationKey = &s
	}
	return t
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func safe(err error) string {
	s := err.Error()
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func timePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time.UTC()
	return &t
}
