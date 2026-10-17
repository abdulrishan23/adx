package ops

import (
	"context"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

// Budgets are the configured limits the cost view reports against.
type Budgets struct {
	// AssistantDailyTokens is the per-workspace assistant budget (24 hours).
	AssistantDailyTokens int64 `json:"assistantDailyTokens"`
	// ModelDailyTokenCap is the server-wide cap the Meter enforces.
	ModelDailyTokenCap int64 `json:"modelDailyTokenCap"`
	// WorkflowDefaultBudget is the per-run token budget when a run names none.
	WorkflowDefaultBudget int64 `json:"workflowDefaultBudget"`
	InputPriceMicros      int64 `json:"inputPriceMicros"`
	OutputPriceMicros     int64 `json:"outputPriceMicros"`
}

type AssistantUsage struct {
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	// CostMicros is null when no price is configured: no price, no figure.
	CostMicros *int64 `json:"costMicros"`
	Budget     int64  `json:"budget"`
	Remaining  *int64 `json:"remaining"`
	Exhausted  bool   `json:"exhausted"`
}

type ServerUsage struct {
	Day          string `json:"day"`
	InputTokens  int64  `json:"inputTokens"`
	OutputTokens int64  `json:"outputTokens"`
	Calls        int64  `json:"calls"`
	Refused      int64  `json:"refused"`
	Cap          int64  `json:"cap"`
	Remaining    *int64 `json:"remaining"`
	Exhausted    bool   `json:"exhausted"`
}

type WorkflowUsage struct {
	SpentTokens     int64 `json:"spentTokens"`
	BlockedOnBudget int64 `json:"blockedOnBudget"`
	DefaultBudget   int64 `json:"defaultBudget"`
}

// CostReport is what the operations screen shows about spend and limits.
// Every figure is for the last 24 hours except the server day, which is the
// UTC day the cap counts.
type CostReport struct {
	Workspace   AssistantUsage   `json:"workspace"`
	Server      ServerUsage      `json:"server"`
	Workflows   WorkflowUsage    `json:"workflows"`
	Budgets     Budgets          `json:"budgets"`
	Pricing     bool             `json:"pricing"`
	Running     string           `json:"running"`
	Load        *LoadMeasurement `json:"load"`
	GeneratedAt time.Time        `json:"generatedAt"`
}

// WithBudgets gives the ops service what it needs for the cost view: the
// limits in force and the model this server runs.
func (o *Ops) WithBudgets(b Budgets, running string) *Ops {
	o.budgets, o.running = b, running
	return o
}

// Cost reports usage against limits for the actor's workspace and the
// server. Viewers may read it: knowing the budget is spent is not a secret.
func (o *Ops) Cost(ctx context.Context, actor core.Actor) (CostReport, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 || o.db == nil {
		return CostReport{}, core.ErrForbidden
	}
	now := o.now().UTC()
	since := now.Add(-24 * time.Hour)
	r := CostReport{Budgets: o.budgets, Running: o.running, GeneratedAt: now, Pricing: o.budgets.InputPriceMicros > 0 || o.budgets.OutputPriceMicros > 0}
	tokens, err := o.db.Reader().SumAssistantTokensSince(ctx, store.SumAssistantTokensSinceParams{WorkspaceID: actor.WorkspaceID, CreatedAt: since})
	if err != nil {
		return CostReport{}, err
	}
	r.Workspace.InputTokens, r.Workspace.OutputTokens = toInt(tokens.InputTokens), toInt(tokens.OutputTokens)
	if r.Pricing {
		cost, err := o.db.Reader().SumAssistantCostSince(ctx, store.SumAssistantCostSinceParams{WorkspaceID: actor.WorkspaceID, CreatedAt: since})
		if err != nil {
			return CostReport{}, err
		}
		r.Workspace.CostMicros = &cost
	}
	r.Workspace.Budget = o.budgets.AssistantDailyTokens
	if r.Workspace.Budget > 0 {
		rem := max(r.Workspace.Budget-r.Workspace.InputTokens-r.Workspace.OutputTokens, 0)
		r.Workspace.Remaining = &rem
		r.Workspace.Exhausted = rem == 0
	}
	day := now.Format("2006-01-02")
	server, err := o.db.Reader().SumModelUsageDay(ctx, day)
	if err != nil {
		return CostReport{}, err
	}
	r.Server = ServerUsage{Day: day, InputTokens: server.InputTokens, OutputTokens: server.OutputTokens, Calls: server.Calls, Refused: server.Refused, Cap: o.budgets.ModelDailyTokenCap}
	if r.Server.Cap > 0 {
		rem := max(r.Server.Cap-r.Server.InputTokens-r.Server.OutputTokens, 0)
		r.Server.Remaining = &rem
		r.Server.Exhausted = rem == 0
	}
	wf, err := o.db.Reader().SumWorkflowSpendSince(ctx, store.SumWorkflowSpendSinceParams{WorkspaceID: actor.WorkspaceID, CreatedAt: since})
	if err != nil {
		return CostReport{}, err
	}
	r.Workflows = WorkflowUsage{SpentTokens: wf.SpentTokens, BlockedOnBudget: wf.BlockedOnBudget, DefaultBudget: o.budgets.WorkflowDefaultBudget}
	load, err := o.LatestLoad(ctx)
	if err != nil {
		return CostReport{}, err
	}
	r.Load = load
	return r, nil
}

func toInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}
