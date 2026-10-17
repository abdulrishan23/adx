package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/store"
)

// Recovery, compensation and milestones (B3.6).
//
// A process can die between sending a request and recording the answer.
// Such an intent is found in "dispatching" after a restart; it is never
// sent again blindly. It becomes uncertain and goes through the marker
// lookup like any other uncertain intent. Compensation is the honest
// reverse of what ADX can do: pausing stops further spend and nothing
// here recovers money already spent, and the text says so. Milestones are
// read from the platform's own figures: delivery is impressions on a day,
// never a submission.

// RecoverInterrupted turns intents left in "dispatching" for longer than
// grace into uncertain ones. Startup and each tick call it.
func (d *Dispatcher) RecoverInterrupted(ctx context.Context, grace time.Duration) (int, error) {
	now := d.clock.Now().UTC()
	rows, err := d.db.Reader().ListIntentsStuckDispatching(ctx, now.Add(-grace))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		err := d.record(ctx, row, "dispatching", "uncertain", 0, "the process stopped while this was being sent; the request may have landed and will be looked up by its marker before any retry", 0, sql.NullTime{Time: now, Valid: true}, row.ProviderRefs, false)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Reversibility says what undoing an intent can and cannot do.
type Reversibility struct {
	IntentID   int64  `json:"intentId"`
	State      string `json:"state"`
	Reversible bool   `json:"reversible"`
	How        string `json:"how"`
	// SpentMinor is what the intent's reservation consumed; it is reported,
	// never described as recoverable.
	SpentMinor *int64 `json:"spentMinor"`
	Currency   string `json:"currency"`
}

func (d *Dispatcher) Reversibility(ctx context.Context, actor core.Actor, intentID int64) (Reversibility, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return Reversibility{}, core.ErrForbidden
	}
	row, err := d.intent(ctx, actor.WorkspaceID, intentID)
	if err != nil {
		return Reversibility{}, err
	}
	prop, err := d.db.Reader().GetProposal(ctx, store.GetProposalParams{ID: row.ProposalID, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return Reversibility{}, err
	}
	r := Reversibility{IntentID: row.ID, State: row.State}
	if prop.Currency.Valid {
		r.Currency = prop.Currency.String
	}
	if row.ReservationID.Valid {
		res, err := d.db.Reader().GetBudgetReservation(ctx, row.ReservationID.Int64)
		if err == nil && res.State == "consumed" {
			v := res.AmountMinor
			r.SpentMinor = &v
		}
	}
	switch row.State {
	case "requested", "authorized", "reserved":
		r.Reversible, r.How = true, "not sent yet: cancelling removes it from the outbox and releases its budget; nothing reached the platform"
	case "dispatching", "uncertain", "reconciling":
		r.Reversible, r.How = false, "in flight: wait for the lookup to settle what landed; cancelling now could leave a live object nobody tracks"
	case "submitted", "externally_confirmed", "confirmed":
		switch policies.WriteAction(prop.Action) {
		case policies.CampaignCreatePaus:
			r.Reversible, r.How = true, "compensation pauses the created campaign (it was created paused and stays paused); nothing spent is recovered, and the campaign remains on the platform for a person to remove"
		case policies.BudgetRaise, policies.BudgetLower:
			r.Reversible, r.How = true, "compensation sets the budget back to what the proposal recorded as current; money spent at the raised budget is not recovered"
		case policies.CampaignPause:
			r.Reversible, r.How = false, "pausing is not reversed by ADX: resuming is a new decision and a new proposal, because spend would restart"
		default:
			r.Reversible, r.How = false, "no compensation is implemented for this action; a person reverses it on the platform"
		}
	case "failed", "cancelled":
		r.Reversible, r.How = false, "nothing to reverse: it did not happen"
	case "compensated":
		r.Reversible, r.How = false, "already compensated; any money spent before compensation is not recovered"
	}
	return r, nil
}

// Compensate proposes and executes the reverse of a submitted or confirmed
// intent through the ordinary proposal path, so it is policy-checked,
// recorded and dispatched like any change. Admins only. The original
// intent is marked compensated once the reverse is in the outbox. The
// detail never claims spent money comes back.
func (d *Dispatcher) Compensate(ctx context.Context, actor core.Actor, intentID int64, reason string) (proposals.Intent, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return proposals.Intent{}, core.ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return proposals.Intent{}, core.Invalid("a reason is required")
	}
	row, err := d.intent(ctx, actor.WorkspaceID, intentID)
	if err != nil {
		return proposals.Intent{}, err
	}
	if row.State != "submitted" && row.State != "externally_confirmed" && row.State != "confirmed" {
		return proposals.Intent{}, core.Conflict("execution %d is %s; only something that reached the platform can be compensated (cancel what has not been sent)", intentID, row.State)
	}
	prop, err := d.db.Reader().GetProposal(ctx, store.GetProposalParams{ID: row.ProposalID, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return proposals.Intent{}, err
	}
	var refs []proposals.ProviderRef
	_ = json.Unmarshal([]byte(row.ProviderRefs), &refs)
	var in proposals.ProposalInput
	switch policies.WriteAction(prop.Action) {
	case policies.CampaignCreatePaus:
		campaign := ""
		for _, r := range refs {
			if r.Kind == "campaign" {
				campaign = r.ExternalID
			}
		}
		if campaign == "" {
			return proposals.Intent{}, core.Conflict("no campaign reference is recorded; reconcile first")
		}
		in = proposals.ProposalInput{AccountID: row.AccountID, Action: policies.CampaignPause, TargetKind: "campaign", TargetRef: campaign, TargetVersion: "compensating:" + row.IdempotencyKey,
			ExpectedCurrent: json.RawMessage(`{"status":"PAUSED or ENABLED"}`), Proposed: json.RawMessage(`{"status":"PAUSED"}`)}
	case policies.BudgetRaise, policies.BudgetLower:
		in = proposals.ProposalInput{AccountID: row.AccountID, Action: policies.BudgetLower, TargetKind: prop.TargetKind, TargetRef: prop.TargetRef, TargetVersion: "compensating:" + row.IdempotencyKey,
			ExpectedCurrent: json.RawMessage(prop.Proposed), Proposed: json.RawMessage(prop.ExpectedCurrent)}
		if policies.WriteAction(prop.Action) == policies.BudgetLower {
			in.Action = policies.BudgetRaise
		}
	default:
		return proposals.Intent{}, core.Conflict("no compensation is implemented for %s; a person reverses it on the platform", prop.Action)
	}
	in.Reason = fmt.Sprintf("compensation for execution %d: %s. This stops further spend; money already spent is not recovered.", row.ID, reason)
	in.IdempotencyKey = fmt.Sprintf("compensate:%d", row.ID)
	comp, _, err := d.proposals.Propose(ctx, actor, in)
	if err != nil {
		return proposals.Intent{}, err
	}
	if comp.State == "denied" {
		return proposals.Intent{}, core.Conflict("policy denies the compensating %s now (%s); the platform object stays as it is", in.Action, comp.PolicyReason)
	}
	if comp.State == "proposed" {
		if _, err := d.proposals.Approve(ctx, actor, comp.ID, "compensation approved by the admin who asked for it"); err != nil {
			return proposals.Intent{}, err
		}
	}
	intent, _, err := d.proposals.Execute(ctx, actor, comp.ID, "compensate-exec:"+strings.TrimPrefix(in.IdempotencyKey, "compensate:"))
	if err != nil {
		return proposals.Intent{}, err
	}
	detail := fmt.Sprintf("compensated by execution %d (%s on %s %s). Further spend stops once it is dispatched; nothing already spent is recovered.", intent.ID, in.Action, in.TargetKind, in.TargetRef)
	if err := d.record(ctx, row, row.State, "compensated", 0, detail, actor.UserID, sql.NullTime{}, row.ProviderRefs, false); err != nil {
		return proposals.Intent{}, err
	}
	return intent, nil
}

// Repair makes a corrected proposal for an intent the platform refused: the
// same target and account, a new proposed document, a note pointing back.
// Admins and marketers may. The failed intent stays failed.
func (d *Dispatcher) Repair(ctx context.Context, actor core.Actor, intentID int64, proposed json.RawMessage, note string) (proposals.Proposal, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return proposals.Proposal{}, core.ErrForbidden
	}
	row, err := d.intent(ctx, actor.WorkspaceID, intentID)
	if err != nil {
		return proposals.Proposal{}, err
	}
	if row.State != "failed" {
		return proposals.Proposal{}, core.Conflict("execution %d is %s; only a refused execution is repaired", intentID, row.State)
	}
	if len(proposed) == 0 || !json.Valid(proposed) {
		return proposals.Proposal{}, core.Invalid("a corrected proposed document is required")
	}
	prop, err := d.db.Reader().GetProposal(ctx, store.GetProposalParams{ID: row.ProposalID, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return proposals.Proposal{}, err
	}
	var refs []int64
	_ = json.Unmarshal([]byte(prop.EvidenceRefs), &refs)
	in := proposals.ProposalInput{AccountID: prop.AccountID, Action: policies.WriteAction(prop.Action), TargetKind: prop.TargetKind, TargetRef: prop.TargetRef, TargetVersion: prop.TargetVersion,
		ExpectedCurrent: json.RawMessage(prop.ExpectedCurrent), Proposed: proposed, EvidenceRefs: refs, Currency: prop.Currency.String, IdempotencyKey: fmt.Sprintf("repair:%d:%d", row.ID, d.clock.Now().UTC().UnixNano()),
		Reason: fmt.Sprintf("repair of execution %d (refused: %s). %s", row.ID, row.LastErrorSafe, strings.TrimSpace(note))}
	if prop.AmountMinor.Valid {
		in.AmountMinor = prop.AmountMinor.Int64
		in.EnvelopeID = prop.EnvelopeID.Int64
	}
	repaired, _, err := d.proposals.Propose(ctx, actor, in)
	if err != nil {
		return proposals.Proposal{}, err
	}
	now := d.clock.Now().UTC()
	_, _ = d.db.Writer().CreateExecutionEvent(ctx, store.CreateExecutionEventParams{IntentID: row.ID, Attempt: row.Attempts, FromState: "failed", ToState: "failed", Detail: fmt.Sprintf("repaired by proposal %d", repaired.ID), ByUser: optionalID(actor.UserID), At: now})
	return repaired, nil
}

// Milestones are the moments between an accepted plan and observed
// delivery. Each is read from a record; none is inferred.
type Milestones struct {
	PlanID           int64      `json:"planId"`
	PlanVersion      int64      `json:"planVersion"`
	PlanAcceptedAt   *time.Time `json:"planAcceptedAt"`
	FirstDispatched  *time.Time `json:"firstDispatchedAt"`
	FirstSubmitted   *time.Time `json:"firstSubmittedAt"`
	FirstConfirmed   *time.Time `json:"firstConfirmedAt"`
	FirstDeliveryDay *string    `json:"firstDeliveryDay"`
	// DeliveryObservedAt is when ADX read the first day with impressions.
	DeliveryObservedAt *time.Time `json:"deliveryObservedAt"`
	// ReadyToDeliveryHours is accepted-plan to delivery-observed, or null.
	ReadyToDeliveryHours *float64 `json:"readyToDeliveryHours"`
	WithinOneHour        bool     `json:"withinOneHour"`
	Note                 string   `json:"note"`
}

func (d *Dispatcher) Milestones(ctx context.Context, actor core.Actor, planID, version int64) (Milestones, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return Milestones{}, core.ErrForbidden
	}
	m := Milestones{PlanID: planID, PlanVersion: version}
	pv, err := d.db.Reader().GetPlanVersion(ctx, store.GetPlanVersionParams{PlanID: planID, Version: version})
	if errors.Is(err, sql.ErrNoRows) {
		return Milestones{}, core.NotFound("plan %d has no version %d", planID, version)
	}
	if err != nil {
		return Milestones{}, err
	}
	if pv.AcceptedAt.Valid {
		t := pv.AcceptedAt.Time.UTC()
		m.PlanAcceptedAt = &t
	}
	rows, err := d.db.Reader().ListIntentsForPlanVersion(ctx, store.ListIntentsForPlanVersionParams{WorkspaceID: actor.WorkspaceID, PlanID: planID, PlanVersion: version})
	if err != nil {
		return Milestones{}, err
	}
	earliest := func(cur *time.Time, t sql.NullTime) *time.Time {
		if !t.Valid {
			return cur
		}
		u := t.Time.UTC()
		if cur == nil || u.Before(*cur) {
			return &u
		}
		return cur
	}
	for _, r := range rows {
		m.FirstDispatched = earliest(m.FirstDispatched, r.DispatchedAt)
		if r.State == "submitted" || r.State == "externally_confirmed" || r.State == "compensated" {
			m.FirstSubmitted = earliest(m.FirstSubmitted, sql.NullTime{Time: r.UpdatedAt, Valid: true})
		}
		m.FirstConfirmed = earliest(m.FirstConfirmed, r.ConfirmedAt)
		var refs []proposals.ProviderRef
		_ = json.Unmarshal([]byte(r.ProviderRefs), &refs)
		for _, ref := range refs {
			if ref.Kind != "campaign" {
				continue
			}
			day, err := d.db.Reader().FirstDeliveryDay(ctx, store.FirstDeliveryDayParams{AccountID: r.AccountID, ExternalID: ref.ExternalID})
			if err != nil {
				continue
			}
			if m.FirstDeliveryDay == nil || day.Day < *m.FirstDeliveryDay {
				dd := day.Day
				m.FirstDeliveryDay = &dd
				obs := day.ObservedAt.UTC()
				m.DeliveryObservedAt = &obs
			}
		}
	}
	switch {
	case m.DeliveryObservedAt != nil && m.PlanAcceptedAt != nil:
		h := m.DeliveryObservedAt.Sub(*m.PlanAcceptedAt).Hours()
		m.ReadyToDeliveryHours = &h
		m.WithinOneHour = h <= 1
		m.Note = fmt.Sprintf("delivery observed: impressions recorded for %s in the platform's figures", *m.FirstDeliveryDay)
	case m.FirstConfirmed != nil:
		m.Note = "confirmed on the platform; no impressions have been read yet, so delivery is not claimed and the one-hour mark is not met"
	case m.FirstSubmitted != nil:
		m.Note = "submitted; not confirmed and not delivered"
	case len(rows) > 0:
		m.Note = "executions exist but none has reached the platform"
	default:
		m.Note = "nothing has been executed for this plan version"
	}
	return m, nil
}
