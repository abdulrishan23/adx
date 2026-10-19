// Package proposals is where a change to an outside account is written
// down before anything happens: what the target is now, what it should
// become, why, under which policy version, and who approved it. Approval
// binds to the target's version, so a target that changed underneath makes
// the proposal stale instead of letting an old approval carry a new change.
// Executing a proposal reserves budget from an accepted plan's envelope and
// writes an execution intent in one transaction, with no provider call
// inside; a worker (B3.5) dispatches intents. The language model can
// propose nothing here: calls that arrive through the assistant are refused.
package proposals

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/store"
)

type Envelope struct {
	ID             int64     `json:"id"`
	PlanID         int64     `json:"planId"`
	PlanVersion    int64     `json:"planVersion"`
	Channel        string    `json:"channel"`
	AccountID      *int64    `json:"accountId"`
	Currency       string    `json:"currency"`
	AmountMinor    int64     `json:"amountMinor"`
	ReservedMinor  int64     `json:"reservedMinor"`
	ConsumedMinor  int64     `json:"consumedMinor"`
	RemainingMinor int64     `json:"remainingMinor"`
	OpenedBy       *int64    `json:"openedBy"`
	OpenedAt       time.Time `json:"openedAt"`
}

type Approval struct {
	ID            int64     `json:"id"`
	TargetVersion string    `json:"targetVersion"`
	Decision      string    `json:"decision"`
	PolicyVersion *int64    `json:"policyVersion"`
	Note          string    `json:"note"`
	DecidedBy     int64     `json:"decidedBy"`
	DecidedAt     time.Time `json:"decidedAt"`
}

// ProviderRef is an id a platform gave back for something this intent made
// or changed.
type ProviderRef struct {
	Platform   string    `json:"platform"`
	Kind       string    `json:"kind"`
	ExternalID string    `json:"externalId"`
	At         time.Time `json:"at"`
}

type Intent struct {
	ID             int64         `json:"id"`
	ProposalID     int64         `json:"proposalId"`
	AccountID      int64         `json:"accountId"`
	IdempotencyKey string        `json:"idempotencyKey"`
	ReservationID  *int64        `json:"reservationId"`
	State          string        `json:"state"`
	Attempts       int64         `json:"attempts"`
	ProviderRefs   []ProviderRef `json:"providerRefs"`
	LastError      string        `json:"lastError"`
	ReconcileAfter *time.Time    `json:"reconcileAfter"`
	AuthorizedBy   *int64        `json:"authorizedBy"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
	DispatchedAt   *time.Time    `json:"dispatchedAt"`
	ConfirmedAt    *time.Time    `json:"confirmedAt"`
}

type Proposal struct {
	ID              int64                `json:"id"`
	AccountID       int64                `json:"accountId"`
	Action          policies.WriteAction `json:"action"`
	TargetKind      string               `json:"targetKind"`
	TargetRef       string               `json:"targetRef"`
	TargetVersion   string               `json:"targetVersion"`
	ExpectedCurrent json.RawMessage      `json:"expectedCurrent"`
	Proposed        json.RawMessage      `json:"proposed"`
	Reason          string               `json:"reason"`
	EvidenceRefs    []int64              `json:"evidenceRefs"`
	AmountMinor     *int64               `json:"amountMinor"`
	Currency        *string              `json:"currency"`
	EnvelopeID      *int64               `json:"envelopeId"`
	PolicyVersion   *int64               `json:"policyVersion"`
	PolicyOutcome   string               `json:"policyOutcome"`
	PolicyReason    string               `json:"policyReason"`
	State           string               `json:"state"`
	IdempotencyKey  string               `json:"idempotencyKey"`
	ProposedBy      *int64               `json:"proposedBy"`
	ProposedVia     string               `json:"proposedVia"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
	Approvals       []Approval           `json:"approvals"`
	Intent          *Intent              `json:"intent"`
}

type ProposalInput struct {
	AccountID     int64                `json:"accountId"`
	Action        policies.WriteAction `json:"action"`
	TargetKind    string               `json:"targetKind"`
	TargetRef     string               `json:"targetRef"`
	TargetVersion string               `json:"targetVersion"`
	// ExpectedCurrent is what the target looks like now; Proposed is what it
	// should become. Both are kept as given.
	ExpectedCurrent json.RawMessage `json:"expectedCurrent"`
	Proposed        json.RawMessage `json:"proposed"`
	Reason          string          `json:"reason"`
	EvidenceRefs    []int64         `json:"evidenceRefs"`
	// AmountMinor and EnvelopeID tie a spending change to an accepted plan's
	// budget. Zero means the change spends nothing new.
	AmountMinor    int64  `json:"amountMinor"`
	Currency       string `json:"currency"`
	EnvelopeID     int64  `json:"envelopeId"`
	IdempotencyKey string `json:"idempotencyKey"`
	// RequireApproval keeps the proposal waiting for a person even when
	// policy would allow the proposer's role to act alone. The supervisor
	// sets it for recommendations under a "recommend" or "approval required"
	// autonomy mode.
	RequireApproval bool `json:"requireApproval"`
	// Supersedes names an earlier proposal this one re-asks, after a policy
	// or evidence change. The earlier one keeps its own decision.
	Supersedes int64 `json:"supersedes"`
}

// PlanCoverage says whether a campaign, by its account and the platform's
// id, has an accepted plan behind it (BD-60 rule 3). core/launch implements
// it; cmd/adx passes it in.
type PlanCoverage interface {
	UnderPlan(ctx context.Context, workspaceID, accountID int64, campaignRef string) (bool, string, error)
}

type Config struct {
	Clock core.Clock
	// Coverage answers whether a campaign is under a plan. Nil means no
	// source of coverage on this server, and then every running campaign
	// counts as not under a plan: only pause and lower budget go through.
	Coverage PlanCoverage
}

type Proposals struct {
	db       *store.DB
	clock    core.Clock
	policies *policies.Policies
	plans    *plans.Plans
	coverage PlanCoverage
}

func New(db *store.DB, pol *policies.Policies, pl *plans.Plans, cfg Config) *Proposals {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Proposals{db: db, clock: cfg.Clock, policies: pol, plans: pl, coverage: cfg.Coverage}
}

// PolicyRequestFor is the policy question one change asks, with the target's
// plan coverage filled in. A change to a campaign (other than creating one)
// is marked unplanned when no accepted plan stands behind it, so policy lets
// only pause and lower budget through. Creates are covered by their envelope.
func (p *Proposals) PolicyRequestFor(ctx context.Context, workspaceID, accountID int64, action policies.WriteAction, targetKind, targetRef string) (policies.PolicyRequest, error) {
	req := policies.PolicyRequest{AccountID: accountID, Action: action, WriteCapability: capabilityFor(action)}
	if targetKind != "campaign" || creates(action) {
		return req, nil
	}
	if p.coverage == nil {
		req.Unplanned = true
		return req, nil
	}
	under, _, err := p.coverage.UnderPlan(ctx, workspaceID, accountID, targetRef)
	if err != nil {
		return policies.PolicyRequest{}, err
	}
	req.Unplanned = !under
	return req, nil
}

func creates(a policies.WriteAction) bool {
	return a == policies.CampaignCreatePaus || a == policies.Launch
}

// capabilityFor is the account write capability an action needs proven.
func capabilityFor(a policies.WriteAction) string {
	switch a {
	case policies.BudgetLower, policies.BudgetRaise:
		return "write.budget"
	case policies.CampaignCreatePaus, policies.Launch:
		return "write.campaign"
	case policies.AdPause, policies.AdSetPause, policies.CampaignPause, policies.CampaignResume:
		return "write.status"
	case policies.AudiencesUpsert:
		return "write.audiences"
	case policies.ConversionsUpload:
		return "write.conversions"
	case policies.ExclusionsUpdate:
		return "write.exclusions"
	case policies.BidsChange:
		return "write.bids"
	case policies.TargetingChange:
		return "write.targeting"
	case policies.LeadFieldsSet:
		return "write.lead_fields"
	case policies.OrganicPost:
		return "write.organic"
	}
	return ""
}

// notThroughTheModel refuses anything the assistant asked for.
func notThroughTheModel(actor core.Actor) error {
	if actor.Via == core.ViaAssistant {
		return core.ErrForbidden
	}
	return nil
}

// Envelopes.

// OpenEnvelopes creates the budget envelopes of an accepted plan version,
// one per allocation ADX can deploy, each tied to the account named for
// its platform. Admins only. Already-open envelopes are kept.
func (p *Proposals) OpenEnvelopes(ctx context.Context, actor core.Actor, planID, version int64, accountByPlatform map[string]int64) ([]Envelope, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if err := notThroughTheModel(actor); err != nil {
		return nil, err
	}
	pv, err := p.plans.Version(ctx, actor, planID, version)
	if err != nil {
		return nil, err
	}
	if pv.State != plans.StateAccepted {
		return nil, core.Conflict("plan version %d is %s; only an accepted version can open budget", version, pv.State)
	}
	now := p.clock.Now().UTC()
	for _, a := range pv.Document.Allocations {
		platform := ""
		switch {
		case strings.HasPrefix(a.Channel, "google_"):
			platform = "google_ads"
		case strings.HasPrefix(a.Channel, "meta_"):
			platform = "meta"
		default:
			continue
		}
		acct := accountByPlatform[platform]
		if acct == 0 {
			return nil, core.Invalid("no %s account named for channel %s", platform, a.Channel)
		}
		if _, err := p.db.Reader().GetAccountByID(ctx, acct); err != nil {
			return nil, core.NotFound("account %d is not connected", acct)
		}
		_, err := p.db.Writer().CreateBudgetEnvelope(ctx, store.CreateBudgetEnvelopeParams{WorkspaceID: actor.WorkspaceID, PlanID: planID, PlanVersion: pv.Version, Channel: a.Channel, AccountID: sql.NullInt64{Int64: acct, Valid: true}, Currency: pv.Document.Currency, AmountMinor: a.BudgetMinor, OpenedBy: optionalID(actor.UserID), OpenedAt: now})
		if err != nil && !store.IsUniqueViolation(err) {
			return nil, err
		}
	}
	return p.envelopesForPlan(ctx, actor, planID, pv.Version)
}

func (p *Proposals) envelopesForPlan(ctx context.Context, actor core.Actor, planID, version int64) ([]Envelope, error) {
	rows, err := p.db.Reader().ListBudgetEnvelopesForPlan(ctx, store.ListBudgetEnvelopesForPlanParams{PlanID: planID, PlanVersion: version})
	if err != nil {
		return nil, err
	}
	out := make([]Envelope, 0, len(rows))
	for _, r := range rows {
		e, err := p.envelopeFromRow(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (p *Proposals) ListEnvelopes(ctx context.Context, actor core.Actor) ([]Envelope, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := p.db.Reader().ListBudgetEnvelopes(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]Envelope, 0, len(rows))
	for _, r := range rows {
		e, err := p.envelopeFromRow(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (p *Proposals) envelopeFromRow(ctx context.Context, r store.BudgetEnvelope) (Envelope, error) {
	reserved, err := p.db.Reader().SumReservationsByState(ctx, store.SumReservationsByStateParams{EnvelopeID: r.ID, State: "reserved"})
	if err != nil {
		return Envelope{}, err
	}
	consumed, err := p.db.Reader().SumReservationsByState(ctx, store.SumReservationsByStateParams{EnvelopeID: r.ID, State: "consumed"})
	if err != nil {
		return Envelope{}, err
	}
	rv, cv := toInt64(reserved), toInt64(consumed)
	return Envelope{ID: r.ID, PlanID: r.PlanID, PlanVersion: r.PlanVersion, Channel: r.Channel, AccountID: idPtr(r.AccountID), Currency: r.Currency, AmountMinor: r.AmountMinor, ReservedMinor: rv, ConsumedMinor: cv, RemainingMinor: r.AmountMinor - rv - cv, OpenedBy: idPtr(r.OpenedBy), OpenedAt: r.OpenedAt.UTC()}, nil
}

// Proposals.

// Propose records a change and evaluates it against policy. Marketers may;
// the assistant may not. A denied evaluation is recorded as a denied
// proposal with its reason, so what was asked and why it can't happen are
// both on file. The same idempotency key returns the first proposal.
func (p *Proposals) Propose(ctx context.Context, actor core.Actor, in ProposalInput) (Proposal, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Proposal{}, false, core.ErrForbidden
	}
	if err := notThroughTheModel(actor); err != nil {
		return Proposal{}, false, err
	}
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if in.IdempotencyKey == "" || len(in.IdempotencyKey) > 200 {
		return Proposal{}, false, core.Invalid("an idempotency key of up to 200 characters is required")
	}
	if existing, err := p.db.Reader().GetProposalByKey(ctx, store.GetProposalByKeyParams{WorkspaceID: actor.WorkspaceID, IdempotencyKey: in.IdempotencyKey}); err == nil {
		pr, err := p.proposalFromRow(ctx, existing)
		return pr, false, err
	}
	in.TargetKind, in.TargetRef, in.TargetVersion = strings.TrimSpace(in.TargetKind), strings.TrimSpace(in.TargetRef), strings.TrimSpace(in.TargetVersion)
	if in.TargetKind == "" || in.TargetRef == "" || in.TargetVersion == "" {
		return Proposal{}, false, core.Invalid("the target's kind, reference and version are required")
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || utf8.RuneCountInString(in.Reason) > 1000 {
		return Proposal{}, false, core.Invalid("a reason of up to 1000 characters is required")
	}
	if len(in.ExpectedCurrent) == 0 {
		in.ExpectedCurrent = json.RawMessage(`{}`)
	}
	if len(in.Proposed) == 0 || !json.Valid(in.Proposed) || !json.Valid(in.ExpectedCurrent) {
		return Proposal{}, false, core.Invalid("expectedCurrent and proposed must be JSON documents")
	}
	if in.AccountID == 0 {
		return Proposal{}, false, core.Invalid("an account is required")
	}
	if creates(in.Action) && in.EnvelopeID == 0 {
		return Proposal{}, false, core.Invalid("no campaign without a plan: creating one needs a budget envelope from an accepted plan version")
	}
	var envelope *store.BudgetEnvelope
	if in.AmountMinor < 0 {
		return Proposal{}, false, core.Invalid("amount can't be negative")
	}
	if in.AmountMinor > 0 {
		if in.EnvelopeID == 0 {
			return Proposal{}, false, core.Invalid("a change that spends needs a budget envelope from an accepted plan")
		}
		row, err := p.db.Reader().GetBudgetEnvelope(ctx, store.GetBudgetEnvelopeParams{ID: in.EnvelopeID, WorkspaceID: actor.WorkspaceID})
		if errors.Is(err, sql.ErrNoRows) {
			return Proposal{}, false, core.NotFound("budget envelope %d not found", in.EnvelopeID)
		}
		if err != nil {
			return Proposal{}, false, err
		}
		if !strings.EqualFold(row.Currency, in.Currency) {
			return Proposal{}, false, core.Invalid("the envelope is in %s, the proposal in %s", row.Currency, in.Currency)
		}
		if row.AccountID.Valid && row.AccountID.Int64 != in.AccountID {
			return Proposal{}, false, core.Invalid("the envelope is for account %d, not %d", row.AccountID.Int64, in.AccountID)
		}
		envelope = &row
	}
	req, err := p.PolicyRequestFor(ctx, actor.WorkspaceID, in.AccountID, in.Action, in.TargetKind, in.TargetRef)
	if err != nil {
		return Proposal{}, false, err
	}
	decision, err := p.policies.Evaluate(ctx, actor, req)
	if err != nil {
		return Proposal{}, false, err
	}
	state := "denied"
	switch decision.Outcome {
	case policies.NeedsApproval:
		state = "proposed"
	case policies.Allowed:
		state = "approved"
		if in.RequireApproval {
			state = "proposed"
			decision.Reason = decision.Reason + "; a person must approve under the autonomy mode"
		}
	}
	if in.Supersedes != 0 {
		in.Reason = fmt.Sprintf("%s (re-asks proposal %d, which keeps its own decision)", in.Reason, in.Supersedes)
	}
	refs, _ := json.Marshal(nonNil(in.EvidenceRefs))
	now := p.clock.Now().UTC()
	var row store.Proposal
	err = p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		row, err = q.CreateProposal(ctx, store.CreateProposalParams{WorkspaceID: actor.WorkspaceID, AccountID: in.AccountID, Action: string(in.Action), TargetKind: in.TargetKind, TargetRef: in.TargetRef, TargetVersion: in.TargetVersion,
			ExpectedCurrent: string(in.ExpectedCurrent), Proposed: string(in.Proposed), Reason: in.Reason, EvidenceRefs: string(refs), AmountMinor: sql.NullInt64{Int64: in.AmountMinor, Valid: in.AmountMinor > 0}, Currency: sql.NullString{String: strings.ToUpper(in.Currency), Valid: in.AmountMinor > 0},
			EnvelopeID: optionalID(in.EnvelopeID), PolicyVersion: optionalID(decision.PolicyVersion), PolicyOutcome: string(decision.Outcome), PolicyReason: decision.Reason, State: state, IdempotencyKey: in.IdempotencyKey,
			ProposedBy: optionalID(actor.UserID), ProposedVia: string(actor.Via), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		if state == "approved" && actor.UserID != 0 {
			_, err = q.CreateApproval(ctx, store.CreateApprovalParams{WorkspaceID: actor.WorkspaceID, ProposalID: row.ID, TargetVersion: in.TargetVersion, Decision: "approve", PolicyVersion: optionalID(decision.PolicyVersion), Note: "policy allows this action for the proposer's role", DecidedBy: actor.UserID, DecidedAt: now})
		}
		return err
	})
	if store.IsUniqueViolation(err) {
		existing, err := p.db.Reader().GetProposalByKey(ctx, store.GetProposalByKeyParams{WorkspaceID: actor.WorkspaceID, IdempotencyKey: in.IdempotencyKey})
		if err != nil {
			return Proposal{}, false, err
		}
		pr, err := p.proposalFromRow(ctx, existing)
		return pr, false, err
	}
	if err != nil {
		return Proposal{}, false, err
	}
	_ = envelope
	pr, err := p.proposalFromRow(ctx, row)
	return pr, true, err
}

// Approve binds an admin's approval to the proposal's current target
// version. Policy is evaluated again at this moment: a kill switch engaged
// since the proposal refuses the approval.
func (p *Proposals) Approve(ctx context.Context, actor core.Actor, id int64, note string) (Proposal, error) {
	return p.decide(ctx, actor, id, "approve", note)
}

func (p *Proposals) Reject(ctx context.Context, actor core.Actor, id int64, note string) (Proposal, error) {
	return p.decide(ctx, actor, id, "reject", note)
}

func (p *Proposals) decide(ctx context.Context, actor core.Actor, id int64, decision, note string) (Proposal, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.UserID == 0 {
		return Proposal{}, core.ErrForbidden
	}
	if err := notThroughTheModel(actor); err != nil {
		return Proposal{}, err
	}
	row, err := p.proposal(ctx, actor.WorkspaceID, id)
	if err != nil {
		return Proposal{}, err
	}
	if row.State != "proposed" {
		return Proposal{}, core.Conflict("proposal %d is %s, not awaiting approval", id, row.State)
	}
	d, err := p.evaluate(ctx, actor, row)
	if err != nil {
		return Proposal{}, err
	}
	if decision == "approve" && d.Outcome == policies.Denied {
		return Proposal{}, core.Conflict("policy denies this action now (%s); the approval was not recorded", d.Reason)
	}
	to := "approved"
	if decision == "reject" {
		to = "rejected"
	}
	now := p.clock.Now().UTC()
	err = p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		n, err := q.SetProposalState(ctx, store.SetProposalStateParams{ToState: to, UpdatedAt: now, ID: id, FromState: "proposed"})
		if err != nil {
			return err
		}
		if n != 1 {
			return core.Conflict("the proposal changed; reload and try again")
		}
		_, err = q.CreateApproval(ctx, store.CreateApprovalParams{WorkspaceID: actor.WorkspaceID, ProposalID: id, TargetVersion: row.TargetVersion, Decision: decision, PolicyVersion: optionalID(d.PolicyVersion), Note: strings.TrimSpace(note), DecidedBy: actor.UserID, DecidedAt: now})
		return err
	})
	if err != nil {
		return Proposal{}, err
	}
	return p.Get(ctx, actor, id)
}

// Observe records that a target now has a different version. Every live
// proposal on it whose version differs becomes stale: its approval, if
// any, stays on file but covers nothing. Marketers and jobs may.
func (p *Proposals) Observe(ctx context.Context, actor core.Actor, targetKind, targetRef, version string) (int, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return 0, core.ErrForbidden
	}
	rows, err := p.db.Reader().ListLiveProposalsForTarget(ctx, store.ListLiveProposalsForTargetParams{WorkspaceID: actor.WorkspaceID, TargetKind: targetKind, TargetRef: targetRef})
	if err != nil {
		return 0, err
	}
	n := 0
	now := p.clock.Now().UTC()
	for _, r := range rows {
		if r.TargetVersion == version {
			continue
		}
		if err := p.db.Writer().SetProposalStateAny(ctx, store.SetProposalStateAnyParams{State: "stale", UpdatedAt: now, ID: r.ID}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Execute turns an approved proposal into a durable execution intent,
// reserving its budget in the same transaction. Marketers may ask; the
// assistant may not. Authority is checked now, not remembered: the
// proposal must be approved for its current target version, policy must
// still allow the action (no kill switch, account still enabled), and the
// envelope must have the amount left. Nothing talks to a provider here.
func (p *Proposals) Execute(ctx context.Context, actor core.Actor, id int64, idempotencyKey string) (Intent, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Intent{}, false, core.ErrForbidden
	}
	if err := notThroughTheModel(actor); err != nil {
		return Intent{}, false, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" || len(idempotencyKey) > 200 {
		return Intent{}, false, core.Invalid("an idempotency key of up to 200 characters is required")
	}
	if existing, err := p.db.Reader().GetExecutionIntentByKey(ctx, store.GetExecutionIntentByKeyParams{WorkspaceID: actor.WorkspaceID, IdempotencyKey: idempotencyKey}); err == nil {
		if existing.ProposalID != id {
			return Intent{}, false, core.Conflict("idempotency key %q belongs to another proposal", idempotencyKey)
		}
		return intentFromRow(existing), false, nil
	}
	row, err := p.proposal(ctx, actor.WorkspaceID, id)
	if err != nil {
		return Intent{}, false, err
	}
	if row.State != "approved" {
		return Intent{}, false, core.Conflict("proposal %d is %s; only an approved proposal can be executed", id, row.State)
	}
	approvals, err := p.db.Reader().ListApprovals(ctx, id)
	if err != nil {
		return Intent{}, false, err
	}
	approved := false
	for _, a := range approvals {
		if a.Decision == "approve" && a.TargetVersion == row.TargetVersion {
			approved = true
		}
	}
	if !approved {
		_ = p.db.Writer().SetProposalStateAny(ctx, store.SetProposalStateAnyParams{State: "stale", UpdatedAt: p.clock.Now().UTC(), ID: id})
		return Intent{}, false, core.Conflict("no approval covers target version %s; the proposal is stale", row.TargetVersion)
	}
	d, err := p.evaluate(ctx, actor, row)
	if err != nil {
		return Intent{}, false, err
	}
	if d.Outcome == policies.Denied {
		return Intent{}, false, core.Conflict("authority denied: %s", d.Reason)
	}
	// NeedsApproval is satisfied by the approval checked above: a marketer
	// may carry out what an admin approved.
	now := p.clock.Now().UTC()
	var irow store.ExecutionIntent
	err = p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var reservation sql.NullInt64
		if row.AmountMinor.Valid && row.AmountMinor.Int64 > 0 {
			env, err := q.GetBudgetEnvelope(ctx, store.GetBudgetEnvelopeParams{ID: row.EnvelopeID.Int64, WorkspaceID: actor.WorkspaceID})
			if err != nil {
				return core.Conflict("the proposal's budget envelope is gone")
			}
			taken, err := q.SumActiveReservations(ctx, env.ID)
			if err != nil {
				return err
			}
			if toInt64(taken)+row.AmountMinor.Int64 > env.AmountMinor {
				return core.Conflict("insufficient remaining budget: %d of %d %s already reserved or spent, %d asked", toInt64(taken), env.AmountMinor, env.Currency, row.AmountMinor.Int64)
			}
			res, err := q.CreateBudgetReservation(ctx, store.CreateBudgetReservationParams{EnvelopeID: env.ID, ProposalID: id, AmountMinor: row.AmountMinor.Int64, CreatedAt: now, UpdatedAt: now})
			if err != nil {
				return err
			}
			reservation = sql.NullInt64{Int64: res.ID, Valid: true}
		}
		var err error
		irow, err = q.CreateExecutionIntent(ctx, store.CreateExecutionIntentParams{WorkspaceID: actor.WorkspaceID, ProposalID: id, AccountID: row.AccountID, IdempotencyKey: idempotencyKey, ReservationID: reservation, State: "reserved", AuthorizedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		n, err := q.SetProposalState(ctx, store.SetProposalStateParams{ToState: "executing", UpdatedAt: now, ID: id, FromState: "approved"})
		if err != nil {
			return err
		}
		if n != 1 {
			return core.Conflict("the proposal changed; reload and try again")
		}
		return nil
	})
	if store.IsUniqueViolation(err) {
		existing, err := p.db.Reader().GetExecutionIntentByKey(ctx, store.GetExecutionIntentByKeyParams{WorkspaceID: actor.WorkspaceID, IdempotencyKey: idempotencyKey})
		if err != nil {
			return Intent{}, false, err
		}
		return intentFromRow(existing), false, nil
	}
	if err != nil {
		return Intent{}, false, err
	}
	return intentFromRow(irow), true, nil
}

// Cancel withdraws an intent that has not been dispatched and releases its
// reservation. Admins only. Anything already sent is not undone here.
func (p *Proposals) Cancel(ctx context.Context, actor core.Actor, intentID int64, reason string) (Intent, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return Intent{}, core.ErrForbidden
	}
	if err := notThroughTheModel(actor); err != nil {
		return Intent{}, err
	}
	row, err := p.db.Reader().GetExecutionIntent(ctx, store.GetExecutionIntentParams{ID: intentID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Intent{}, core.NotFound("execution %d not found", intentID)
	}
	if err != nil {
		return Intent{}, err
	}
	switch row.State {
	case "requested", "authorized", "reserved":
	default:
		return Intent{}, core.Conflict("execution %d is %s; it has left the outbox and cannot be cancelled here", intentID, row.State)
	}
	now := p.clock.Now().UTC()
	err = p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		n, err := q.UpdateExecutionIntent(ctx, store.UpdateExecutionIntentParams{ToState: "cancelled", ProviderRefs: row.ProviderRefs, LastErrorSafe: "cancelled: " + strings.TrimSpace(reason), UpdatedAt: now, ID: row.ID, FromState: row.State})
		if err != nil {
			return err
		}
		if n != 1 {
			return core.Conflict("the execution changed; reload and try again")
		}
		if row.ReservationID.Valid {
			if err := q.SetBudgetReservationState(ctx, store.SetBudgetReservationStateParams{State: "released", UpdatedAt: now, ID: row.ReservationID.Int64}); err != nil {
				return err
			}
		}
		return q.SetProposalStateAny(ctx, store.SetProposalStateAnyParams{State: "cancelled", UpdatedAt: now, ID: row.ProposalID})
	})
	if err != nil {
		return Intent{}, err
	}
	out, err := p.db.Reader().GetExecutionIntent(ctx, store.GetExecutionIntentParams{ID: intentID, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return Intent{}, err
	}
	return intentFromRow(out), nil
}

// Reads.

func (p *Proposals) Get(ctx context.Context, actor core.Actor, id int64) (Proposal, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return Proposal{}, core.ErrForbidden
	}
	row, err := p.proposal(ctx, actor.WorkspaceID, id)
	if err != nil {
		return Proposal{}, err
	}
	return p.proposalFromRow(ctx, row)
}

func (p *Proposals) List(ctx context.Context, actor core.Actor, state string, limit int) ([]Proposal, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := p.db.Reader().ListProposals(ctx, store.ListProposalsParams{WorkspaceID: actor.WorkspaceID, State: sql.NullString{String: state, Valid: state != ""}, Max: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Proposal, 0, len(rows))
	for _, r := range rows {
		pr, err := p.proposalFromRow(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, nil
}

func (p *Proposals) GetIntent(ctx context.Context, actor core.Actor, id int64) (Intent, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return Intent{}, core.ErrForbidden
	}
	row, err := p.db.Reader().GetExecutionIntent(ctx, store.GetExecutionIntentParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Intent{}, core.NotFound("execution %d not found", id)
	}
	if err != nil {
		return Intent{}, err
	}
	return intentFromRow(row), nil
}

func (p *Proposals) ListIntents(ctx context.Context, actor core.Actor, state string, limit int) ([]Intent, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := p.db.Reader().ListExecutionIntents(ctx, store.ListExecutionIntentsParams{WorkspaceID: actor.WorkspaceID, State: sql.NullString{String: state, Valid: state != ""}, Max: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Intent, 0, len(rows))
	for _, r := range rows {
		out = append(out, intentFromRow(r))
	}
	return out, nil
}

// Authority is the policy's answer for a proposal right now, for the UI.
func (p *Proposals) Authority(ctx context.Context, actor core.Actor, id int64) (policies.PolicyDecision, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return policies.PolicyDecision{}, core.ErrForbidden
	}
	row, err := p.proposal(ctx, actor.WorkspaceID, id)
	if err != nil {
		return policies.PolicyDecision{}, err
	}
	eval := actor
	if !eval.AtLeast(core.RoleMarketer) {
		eval.Role = core.RoleMarketer
	}
	return p.evaluate(ctx, eval, row)
}

// evaluate asks policy about a stored proposal as it stands now.
func (p *Proposals) evaluate(ctx context.Context, actor core.Actor, row store.Proposal) (policies.PolicyDecision, error) {
	req, err := p.PolicyRequestFor(ctx, actor.WorkspaceID, row.AccountID, policies.WriteAction(row.Action), row.TargetKind, row.TargetRef)
	if err != nil {
		return policies.PolicyDecision{}, err
	}
	return p.policies.Evaluate(ctx, actor, req)
}

func (p *Proposals) proposal(ctx context.Context, workspaceID, id int64) (store.Proposal, error) {
	row, err := p.db.Reader().GetProposal(ctx, store.GetProposalParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Proposal{}, core.NotFound("proposal %d not found", id)
	}
	return row, err
}

func (p *Proposals) proposalFromRow(ctx context.Context, r store.Proposal) (Proposal, error) {
	pr := Proposal{ID: r.ID, AccountID: r.AccountID, Action: policies.WriteAction(r.Action), TargetKind: r.TargetKind, TargetRef: r.TargetRef, TargetVersion: r.TargetVersion,
		ExpectedCurrent: json.RawMessage(r.ExpectedCurrent), Proposed: json.RawMessage(r.Proposed), Reason: r.Reason, EvidenceRefs: []int64{}, AmountMinor: idPtr(r.AmountMinor), EnvelopeID: idPtr(r.EnvelopeID),
		PolicyVersion: idPtr(r.PolicyVersion), PolicyOutcome: r.PolicyOutcome, PolicyReason: r.PolicyReason, State: r.State, IdempotencyKey: r.IdempotencyKey, ProposedBy: idPtr(r.ProposedBy), ProposedVia: r.ProposedVia,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(), Approvals: []Approval{}}
	_ = json.Unmarshal([]byte(r.EvidenceRefs), &pr.EvidenceRefs)
	if r.Currency.Valid {
		c := r.Currency.String
		pr.Currency = &c
	}
	approvals, err := p.db.Reader().ListApprovals(ctx, r.ID)
	if err != nil {
		return Proposal{}, err
	}
	for _, a := range approvals {
		pr.Approvals = append(pr.Approvals, Approval{ID: a.ID, TargetVersion: a.TargetVersion, Decision: a.Decision, PolicyVersion: idPtr(a.PolicyVersion), Note: a.Note, DecidedBy: a.DecidedBy, DecidedAt: a.DecidedAt.UTC()})
	}
	if in, err := p.db.Reader().GetExecutionIntentForProposal(ctx, r.ID); err == nil {
		i := intentFromRow(in)
		pr.Intent = &i
	}
	return pr, nil
}

func intentFromRow(r store.ExecutionIntent) Intent {
	i := Intent{ID: r.ID, ProposalID: r.ProposalID, AccountID: r.AccountID, IdempotencyKey: r.IdempotencyKey, ReservationID: idPtr(r.ReservationID), State: r.State, Attempts: r.Attempts, ProviderRefs: []ProviderRef{}, LastError: r.LastErrorSafe, AuthorizedBy: idPtr(r.AuthorizedBy), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
	_ = json.Unmarshal([]byte(r.ProviderRefs), &i.ProviderRefs)
	if r.ReconcileAfter.Valid {
		t := r.ReconcileAfter.Time.UTC()
		i.ReconcileAfter = &t
	}
	if r.DispatchedAt.Valid {
		t := r.DispatchedAt.Time.UTC()
		i.DispatchedAt = &t
	}
	if r.ConfirmedAt.Valid {
		t := r.ConfirmedAt.Time.UTC()
		i.ConfirmedAt = &t
	}
	return i
}

func nonNil(xs []int64) []int64 {
	if xs == nil {
		return []int64{}
	}
	return xs
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case []byte:
		var out int64
		_, _ = fmt.Sscan(string(n), &out)
		return out
	case string:
		var out int64
		_, _ = fmt.Sscan(n, &out)
		return out
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
