package launch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/store"
)

// Submit writes the resolution out. Each line becomes one proposal under
// the plan version's envelope: a create as campaign.create_paused, an
// update as budget.raise or budget.lower, a resume as campaign.resume with
// the daily budget to set. What policy allows the actor is executed into the
// outbox; what needs approval waits in Perform; what is denied is reported.
// The proposal's idempotency key names the plan version, the line, the kind
// and the target, so a second Submit finds the first proposals instead of
// making more. Nothing here talks to a platform: the dispatcher does.

const (
	StateSubmitted        = "submitted"
	StateAwaitingApproval = "awaiting_approval"
	StateDenied           = "denied"
	StateHeld             = "held"
	StateUnchanged        = "unchanged"
	StateExhausted        = "exhausted"
	StateSkipped          = "skipped"
)

type SubmitInput struct {
	PlanID      int64 `json:"planId"`
	PlanVersion int64 `json:"planVersion"`
	// AccountByPlatform names the account each platform launches into. It
	// is used to open the version's envelopes when none are open yet, which
	// only an admin may do; with envelopes open it may be empty.
	AccountByPlatform map[string]int64 `json:"accountByPlatform"`
}

// SubmittedLine is what Submit did for one line.
type SubmittedLine struct {
	Line LaunchLine `json:"line"`
	Kind string     `json:"kind"`
	// State is submitted (an intent is in the outbox), awaiting_approval,
	// denied, held (approved but not executable now, with the reason),
	// unchanged (the campaign already has the daily budget), exhausted (the
	// line is spent this month, nothing to set) or skipped (not deployable).
	State    string              `json:"state"`
	Proposal *proposals.Proposal `json:"proposal"`
	Intent   *proposals.Intent   `json:"intent"`
	Note     string              `json:"note"`
}

type SubmitResult struct {
	PlanID      int64           `json:"planId"`
	PlanVersion int64           `json:"planVersion"`
	Lines       []SubmittedLine `json:"lines"`
	Note        string          `json:"note"`
}

// AttachProposals gives the service the proposals it submits through. It is
// separate from New because proposals are built after launch: they ask it
// for plan coverage.
func (s *Service) AttachProposals(p *proposals.Proposals) { s.proposals = p }

// Submit turns the resolution into proposals and intents. Marketers and
// admins may; the assistant may not. Every line must be resolved: a line
// still waiting on a pick stops the whole submission.
func (s *Service) Submit(ctx context.Context, actor core.Actor, in SubmitInput) (SubmitResult, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return SubmitResult{}, core.ErrForbidden
	}
	if s.proposals == nil {
		return SubmitResult{}, core.Conflict("proposals are not wired on this server; nothing can be submitted")
	}
	res, err := s.resolve(ctx, actor, in.PlanID, in.PlanVersion, s.clock.Now())
	if err != nil {
		return SubmitResult{}, err
	}
	var open []string
	for _, l := range res.Lines {
		if l.Kind == KindPick {
			open = append(open, fmt.Sprintf("%d (%s)", l.Line.Index, l.Line.Channel))
		}
	}
	if len(open) > 0 {
		return SubmitResult{}, core.Conflict("line %s still needs a person's pick before anything is submitted", strings.Join(open, ", "))
	}
	plan, err := s.db.Reader().GetPlan(ctx, store.GetPlanParams{ID: in.PlanID, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return SubmitResult{}, err
	}
	envelopes, err := s.envelopes(ctx, actor, res, in.AccountByPlatform)
	if err != nil {
		return SubmitResult{}, err
	}
	// All or nothing on coverage (BD-68 rule 11): every line is evaluated
	// before any is written, and a line policy would deny because its
	// campaign is not under a plan stops the whole submission. Other
	// denials stay per line, reported in the result.
	type prepared struct {
		line  LineResolution
		input *proposals.ProposalInput
		state string
		note  string
	}
	var lines []prepared
	var uncovered []string
	for _, l := range res.Lines {
		input, state, note := s.proposalFor(plan, l, envelopes[l.Line.Channel])
		lines = append(lines, prepared{line: l, input: input, state: state, note: note})
		if input == nil {
			continue
		}
		decision, err := s.proposals.Evaluate(ctx, actor, *input)
		if err != nil {
			return SubmitResult{}, fmt.Errorf("line %d: %w", l.Line.Index, err)
		}
		if decision.Reason == policies.ReasonUnplannedCampaign {
			why := "not under an accepted plan"
			if c, err := s.Coverage(ctx, actor.WorkspaceID, input.AccountID, input.TargetRef); err == nil && c.Reason != "" {
				why = c.Reason
			}
			uncovered = append(uncovered, fmt.Sprintf("line %d (%s) on campaign %s: %s", l.Line.Index, l.Line.Channel, input.TargetRef, why))
		}
	}
	if len(uncovered) > 0 {
		return SubmitResult{}, core.Conflict("nothing was submitted: %s", strings.Join(uncovered, "; "))
	}
	out := SubmitResult{PlanID: res.PlanID, PlanVersion: res.PlanVersion, Lines: []SubmittedLine{},
		Note: "Submitted means an intent is in the outbox; the dispatcher sends it when live writes are on and policy still allows it. Nothing here is delivery."}
	for _, pl := range lines {
		l, input := pl.line, pl.input
		sl := SubmittedLine{Line: l.Line, Kind: l.Kind}
		if input == nil {
			sl.State, sl.Note = pl.state, pl.note
			out.Lines = append(out.Lines, sl)
			continue
		}
		pr, _, err := s.proposals.Propose(ctx, actor, *input)
		if err != nil {
			return SubmitResult{}, fmt.Errorf("line %d: %w", l.Line.Index, err)
		}
		sl.Proposal = &pr
		switch pr.State {
		case "approved":
			intent, _, err := s.proposals.Execute(ctx, actor, pr.ID, "exec-"+input.IdempotencyKey)
			var ce *core.Error
			switch {
			case err == nil:
				sl.Intent, sl.State = &intent, StateSubmitted
			case errors.As(err, &ce):
				sl.State, sl.Note = StateHeld, ce.Message
			default:
				return SubmitResult{}, fmt.Errorf("line %d: %w", l.Line.Index, err)
			}
		case "executing", "done":
			sl.Intent, sl.State, sl.Note = pr.Intent, StateSubmitted, "already submitted by an earlier launch of this version"
		case "proposed":
			sl.State, sl.Note = StateAwaitingApproval, "waits for an admin in Perform"
		case "denied":
			sl.State, sl.Note = StateDenied, "policy: "+pr.PolicyReason
		default:
			sl.State, sl.Note = pr.State, fmt.Sprintf("an earlier submission of this line is %s; retry or repair it from the execution screen", pr.State)
		}
		out.Lines = append(out.Lines, sl)
	}
	return out, nil
}

// envelopes finds the version's envelopes by channel, opening them when an
// admin named the accounts and none are open yet.
func (s *Service) envelopes(ctx context.Context, actor core.Actor, res LaunchResolution, accountByPlatform map[string]int64) (map[string]proposals.Envelope, error) {
	list, err := s.proposals.ListEnvelopes(ctx, actor)
	if err != nil {
		return nil, err
	}
	byChannel := map[string]proposals.Envelope{}
	for _, e := range list {
		if e.PlanID == res.PlanID && e.PlanVersion == res.PlanVersion {
			byChannel[e.Channel] = e
		}
	}
	var missing []string
	for _, l := range res.Lines {
		if l.Kind == KindUnsupported {
			continue
		}
		if _, ok := byChannel[l.Line.Channel]; !ok {
			missing = append(missing, l.Line.Channel)
		}
	}
	if len(missing) == 0 {
		return byChannel, nil
	}
	if !actor.AtLeast(core.RoleAdmin) {
		return nil, core.Conflict("no budget envelope is open for %s; an admin opens the version's envelopes first", strings.Join(missing, ", "))
	}
	opened, err := s.proposals.OpenEnvelopes(ctx, actor, res.PlanID, res.PlanVersion, accountByPlatform)
	if err != nil {
		return nil, err
	}
	for _, e := range opened {
		byChannel[e.Channel] = e
	}
	return byChannel, nil
}

// proposalFor builds the line's proposal, or says why there is none.
func (s *Service) proposalFor(plan store.Plan, l LineResolution, env proposals.Envelope) (*proposals.ProposalInput, string, string) {
	if l.Kind == KindUnsupported {
		return nil, StateSkipped, l.Note
	}
	if env.ID == 0 || env.AccountID == nil {
		return nil, StateSkipped, "no budget envelope with an account is open for " + l.Line.Channel
	}
	if l.Budget == nil || l.Budget.DailyMinor <= 0 {
		return nil, StateExhausted, "the line's budget for the period is spent; nothing is set. Pause the campaign from Perform if it should stop."
	}
	daily := l.Budget.DailyMinor
	key := fmt.Sprintf("launch:%d:%d:%d:%s:", plan.ID, env.PlanVersion, l.Line.Index, l.Kind)
	reason := fmt.Sprintf("launch of plan %d version %d, line %d (%s): %d of the line's %d %s is left in the period, %d a day over %d days", plan.ID, env.PlanVersion, l.Line.Index, l.Line.Channel, l.Budget.RemainingMinor, l.Line.BudgetMinor, l.Line.Currency, daily, l.Budget.DaysLeft)
	base := proposals.ProposalInput{TargetKind: "campaign", Reason: reason, AmountMinor: l.Budget.RemainingMinor, Currency: env.Currency, EnvelopeID: env.ID}
	switch l.Kind {
	case KindCreate:
		ch, _ := plans.Channel(l.Line.Channel)
		name := strings.TrimSpace(strings.Join([]string{plan.Title, ch.Label, l.Line.Geography}, " | "))
		proposed, _ := json.Marshal(map[string]any{"name": name, "dailyBudgetMinor": daily, "channelType": ch.CampaignType, "objective": ch.CampaignType})
		base.AccountID, base.Action = *env.AccountID, policies.CampaignCreatePaus
		base.TargetRef, base.TargetVersion = fmt.Sprintf("new:launch:%d:%d:%d", plan.ID, env.PlanVersion, l.Line.Index), "none"
		base.Proposed, base.IdempotencyKey = proposed, key+base.TargetRef
		return &base, "", ""
	case KindUpdate:
		c := l.Campaign
		base.AccountID, base.TargetRef = c.AccountID, c.ExternalID
		proposed, _ := json.Marshal(map[string]any{"dailyBudgetMinor": daily})
		base.Proposed = proposed
		switch {
		case c.DailyBudgetMinor == nil:
			base.Action, base.TargetVersion, base.ExpectedCurrent = policies.BudgetRaise, "budget:unknown", json.RawMessage(`{}`)
		case *c.DailyBudgetMinor == daily:
			return nil, StateUnchanged, fmt.Sprintf("campaign %s already spends %d a day", c.ExternalID, daily)
		default:
			base.Action = policies.BudgetRaise
			if daily < *c.DailyBudgetMinor {
				base.Action = policies.BudgetLower
			}
			base.TargetVersion = fmt.Sprintf("budget:%d", *c.DailyBudgetMinor)
			base.ExpectedCurrent, _ = json.Marshal(map[string]any{"dailyBudgetMinor": *c.DailyBudgetMinor})
		}
		base.IdempotencyKey = key + c.ExternalID
		return &base, "", ""
	case KindResume:
		c := l.Campaign
		status := "ENABLED"
		if c.Platform == "meta" {
			status = "ACTIVE"
		}
		base.AccountID, base.Action, base.TargetRef, base.TargetVersion = c.AccountID, policies.CampaignResume, c.ExternalID, "status:"+c.Status
		base.ExpectedCurrent, _ = json.Marshal(map[string]any{"status": c.Status})
		base.Proposed, _ = json.Marshal(map[string]any{"status": status, "dailyBudgetMinor": daily})
		base.IdempotencyKey = key + c.ExternalID
		return &base, "", ""
	}
	return nil, StateSkipped, "nothing to submit for a " + l.Kind + " line"
}
