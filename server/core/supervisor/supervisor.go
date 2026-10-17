// Package supervisor is the one place a person looks to see what ADX is
// waiting on: approvals, failures, uncertain actions, missing evidence,
// conflicts. It also holds the autonomy configuration (objective
// precedence and per-capability modes with caps) and arbitrates
// recommendations that compete for one budget envelope: precedence decides
// when it is configured, a person decides when it is not. Nothing here
// widens what policy allows; the kill switch it exposes is policy's own.
package supervisor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/calls"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/execution"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/core/qualification"
	"github.com/WokeWorks/adx/server/core/workflows"
	"github.com/WokeWorks/adx/server/store"
)

// AutonomyMode is how far an agent may go with a capability. Recommend: the
// recommendation is recorded as a proposal a person must approve, whatever
// policy would allow the role. ApprovalRequired: the same, but counted as
// an intended action. Allowed: policy alone decides.
type AutonomyMode string

const (
	ModeRecommend        AutonomyMode = "recommend"
	ModeApprovalRequired AutonomyMode = "approval_required"
	ModeAllowed          AutonomyMode = "allowed"
)

type AutonomyRule struct {
	Mode AutonomyMode `json:"mode"`
	// MaxPerDay caps recommendations of this capability per Dubai day.
	// Zero means no cap.
	MaxPerDay int `json:"maxPerDay"`
}

type SupervisorDocument struct {
	// Objectives in precedence order: the first outranks the rest when two
	// recommendations compete for one allocation. Empty means no precedence,
	// and every conflict goes to a person.
	Objectives []string                `json:"objectives"`
	Autonomy   map[string]AutonomyRule `json:"autonomy"`
	Note       string                  `json:"note"`
}

type SupervisorConfig struct {
	Version   int64              `json:"version"`
	Document  SupervisorDocument `json:"document"`
	CreatedBy *int64             `json:"createdBy"`
	CreatedAt time.Time          `json:"createdAt"`
}

type SupervisorState struct {
	Configured   bool                  `json:"configured"`
	Current      *SupervisorConfig     `json:"current"`
	Suggested    SupervisorDocument    `json:"suggested"`
	KillSwitches []policies.KillSwitch `json:"killSwitches"`
}

// InboxItem is one thing waiting for a person. Actions are what the Act
// endpoint accepts for it.
type InboxItem struct {
	Kind     string    `json:"kind"`
	Ref      int64     `json:"ref"`
	Severity string    `json:"severity"` // urgent, warning, info
	Title    string    `json:"title"`
	Detail   string    `json:"detail"`
	Actions  []string  `json:"actions"`
	Since    time.Time `json:"since"`
}

type ActionInput struct {
	Kind   string `json:"kind"`
	Ref    int64  `json:"ref"`
	Action string `json:"action"`
	Note   string `json:"note"`
	// Target is an extra id some actions need (a person to merge into, a
	// proposal to keep).
	Target int64 `json:"target"`
}

type Recommendation struct {
	ID         int64     `json:"id"`
	ProposalID int64     `json:"proposalId"`
	Source     string    `json:"source"`
	Capability string    `json:"capability"`
	Objective  string    `json:"objective"`
	CreatedAt  time.Time `json:"createdAt"`
}

type RecommendationConflict struct {
	ID          int64      `json:"id"`
	EnvelopeID  int64      `json:"envelopeId"`
	ProposalIDs []int64    `json:"proposalIds"`
	State       string     `json:"state"`
	Detail      string     `json:"detail"`
	Resolution  string     `json:"resolution"`
	CreatedAt   time.Time  `json:"createdAt"`
	ResolvedBy  *int64     `json:"resolvedBy"`
	ResolvedAt  *time.Time `json:"resolvedAt"`
}

type Config struct {
	Clock         core.Clock
	Proposals     *proposals.Proposals
	Execution     *execution.Dispatcher
	Workflows     *workflows.Workflows
	Calls         *calls.Calls
	Qualification *qualification.Qualification
	Outreach      *outreach.Outreach
	CRM           *crm.CRM
	Policies      *policies.Policies
}

type Supervisor struct {
	db    *store.DB
	cfg   Config
	clock core.Clock
}

func New(db *store.DB, cfg Config) *Supervisor {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Supervisor{db: db, cfg: cfg, clock: cfg.Clock}
}

// AttachWorkflows sets the workflow engine after it is built: the engine
// needs the supervisor as its recommender, so one of them is wired second.
func (s *Supervisor) AttachWorkflows(w *workflows.Workflows) { s.cfg.Workflows = w }

// SuggestedConfig: no precedence (a person decides every conflict) and
// every capability on recommend with a cap.
func SuggestedConfig() SupervisorDocument {
	return SupervisorDocument{Objectives: []string{}, Autonomy: map[string]AutonomyRule{
		"propose_change":        {Mode: ModeRecommend, MaxPerDay: 20},
		"propose_plan":          {Mode: ModeRecommend, MaxPerDay: 5},
		"propose_draft_message": {Mode: ModeRecommend, MaxPerDay: 50},
	}, Note: "Suggested: no objective precedence (conflicts go to a person) and every capability recommends only. Reportage names the objectives and what may run alone."}
}

func (s *Supervisor) GetConfig(ctx context.Context, actor core.Actor) (SupervisorState, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return SupervisorState{}, core.ErrForbidden
	}
	cur, err := s.current(ctx, actor.WorkspaceID)
	if err != nil {
		return SupervisorState{}, err
	}
	st := SupervisorState{Configured: cur != nil, Current: cur, Suggested: SuggestedConfig(), KillSwitches: []policies.KillSwitch{}}
	if s.cfg.Policies != nil {
		if ps, err := s.cfg.Policies.Get(ctx, actor); err == nil {
			st.KillSwitches = ps.KillSwitches
		}
	}
	return st, nil
}

func (s *Supervisor) PutConfig(ctx context.Context, actor core.Actor, doc SupervisorDocument) (SupervisorConfig, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return SupervisorConfig{}, core.ErrForbidden
	}
	seen := map[string]bool{}
	for _, o := range doc.Objectives {
		if strings.TrimSpace(o) == "" || seen[o] {
			return SupervisorConfig{}, core.Invalid("objectives must be unique and non-empty")
		}
		seen[o] = true
	}
	for c, r := range doc.Autonomy {
		switch r.Mode {
		case ModeRecommend, ModeApprovalRequired, ModeAllowed:
		default:
			return SupervisorConfig{}, core.Invalid("capability %q needs mode recommend, approval_required or allowed", c)
		}
		if r.MaxPerDay < 0 {
			return SupervisorConfig{}, core.Invalid("maxPerDay must not be negative")
		}
	}
	raw, _ := json.Marshal(doc)
	var out SupervisorConfig
	err := s.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var version int64 = 1
		if cur, err := q.GetCurrentSupervisorConfig(ctx, actor.WorkspaceID); err == nil {
			version = cur.Version + 1
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		row, err := q.CreateSupervisorConfig(ctx, store.CreateSupervisorConfigParams{WorkspaceID: actor.WorkspaceID, Version: version, Document: string(raw), CreatedBy: optionalID(actor.UserID), CreatedAt: s.clock.Now().UTC()})
		if err != nil {
			return err
		}
		out, err = configFromRow(row)
		return err
	})
	return out, err
}

// Allow is asked before an agent proposes with a capability. It answers
// whether a person must approve (recommend and approval-required modes, and
// the default when nothing is configured) and refuses when the day's cap
// is reached.
func (s *Supervisor) Allow(ctx context.Context, actor core.Actor, capability string) (bool, error) {
	cur, err := s.current(ctx, actor.WorkspaceID)
	if err != nil {
		return true, err
	}
	rule := AutonomyRule{Mode: ModeRecommend}
	if cur != nil {
		if r, ok := cur.Document.Autonomy[capability]; ok {
			rule = r
		}
	}
	if rule.MaxPerDay > 0 {
		n, err := s.db.Reader().CountRecommendationsSince(ctx, store.CountRecommendationsSinceParams{WorkspaceID: actor.WorkspaceID, Capability: capability, CreatedAt: dayStart(s.clock.Now())})
		if err != nil {
			return true, err
		}
		if n >= int64(rule.MaxPerDay) {
			return true, core.Conflict("the day's cap of %d %s recommendations is reached", rule.MaxPerDay, capability)
		}
	}
	return rule.Mode != ModeAllowed, nil
}

// Record ties a proposal to where it came from and the objective it serves.
func (s *Supervisor) Record(ctx context.Context, actor core.Actor, proposalID int64, source, capability, objective string) error {
	if actor.WorkspaceID == 0 {
		return core.ErrForbidden
	}
	_, err := s.db.Writer().CreateRecommendation(ctx, store.CreateRecommendationParams{WorkspaceID: actor.WorkspaceID, ProposalID: proposalID, Source: source, Capability: capability, Objective: objective, CreatedAt: s.clock.Now().UTC()})
	return err
}

// Arbitrate finds live recommendations that together exceed their
// envelope's remaining budget. With precedence covering every objective it
// keeps the higher objectives (in order, while they fit) and rejects the
// rest, recording the conflict as arbitrated; otherwise it escalates the
// conflict to the inbox and touches nothing. Rejecting needs an admin with
// an identity; a job actor records the arbitration for an admin to apply.
func (s *Supervisor) Arbitrate(ctx context.Context, actor core.Actor) ([]RecommendationConflict, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || s.cfg.Proposals == nil {
		return nil, core.ErrForbidden
	}
	cur, err := s.current(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	precedence := map[string]int{}
	if cur != nil {
		for i, o := range cur.Document.Objectives {
			precedence[o] = i
		}
	}
	envelopes, err := s.cfg.Proposals.ListEnvelopes(ctx, actor)
	if err != nil {
		return nil, err
	}
	var out []RecommendationConflict
	now := s.clock.Now().UTC()
	for _, env := range envelopes {
		live, err := s.db.Reader().ListLiveProposalsForEnvelope(ctx, sql.NullInt64{Int64: env.ID, Valid: true})
		if err != nil {
			return nil, err
		}
		var asked int64
		for _, p := range live {
			asked += p.AmountMinor.Int64
		}
		remaining := env.RemainingMinor
		if len(live) < 2 || asked <= remaining {
			continue
		}
		if open, err := s.db.Reader().ListOpenSupervisorConflictsForEnvelope(ctx, env.ID); err == nil && len(open) > 0 {
			continue // already with a person
		}
		ids := make([]int64, 0, len(live))
		type ranked struct {
			p    store.Proposal
			rank int
			ok   bool
		}
		var rs []ranked
		allRanked := len(precedence) > 0
		for _, p := range live {
			ids = append(ids, p.ID)
			r := ranked{p: p}
			if rec, err := s.db.Reader().GetRecommendationByProposal(ctx, p.ID); err == nil {
				if i, ok := precedence[rec.Objective]; ok {
					r.rank, r.ok = i, true
				}
			}
			if !r.ok {
				allRanked = false
			}
			rs = append(rs, r)
		}
		idsJSON, _ := json.Marshal(ids)
		detail := fmt.Sprintf("%d proposals ask %d %s of %d remaining in envelope %d", len(live), asked, env.Currency, remaining, env.ID)
		if !allRanked {
			row, err := s.db.Writer().CreateSupervisorConflict(ctx, store.CreateSupervisorConflictParams{WorkspaceID: actor.WorkspaceID, EnvelopeID: env.ID, ProposalIds: string(idsJSON), State: "escalated", Detail: detail + "; no objective precedence covers them, so a person decides", CreatedAt: now})
			if err != nil {
				return nil, err
			}
			out = append(out, conflictFromRow(row))
			continue
		}
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].rank < rs[j].rank })
		var kept, rejected []string
		left := remaining
		for _, r := range rs {
			amt := r.p.AmountMinor.Int64
			if amt <= left {
				left -= amt
				kept = append(kept, strconv.FormatInt(r.p.ID, 10))
				continue
			}
			rejected = append(rejected, strconv.FormatInt(r.p.ID, 10))
			if actor.AtLeast(core.RoleAdmin) && actor.UserID != 0 {
				_, _ = s.cfg.Proposals.Reject(ctx, actor, r.p.ID, fmt.Sprintf("arbitrated: objective %q is outranked and the envelope cannot carry both", objectiveOf(precedence, r.rank)))
			}
		}
		resolution := fmt.Sprintf("kept %s; rejected %s by precedence %s", strings.Join(kept, ","), strings.Join(rejected, ","), strings.Join(cur.Document.Objectives, " > "))
		state := "arbitrated"
		if !actor.AtLeast(core.RoleAdmin) || actor.UserID == 0 {
			resolution += " (rejections pending an admin's Act)"
		}
		row, err := s.db.Writer().CreateSupervisorConflict(ctx, store.CreateSupervisorConflictParams{WorkspaceID: actor.WorkspaceID, EnvelopeID: env.ID, ProposalIds: string(idsJSON), State: state, Detail: detail, Resolution: resolution, CreatedAt: now})
		if err != nil {
			return nil, err
		}
		out = append(out, conflictFromRow(row))
	}
	if out == nil {
		out = []RecommendationConflict{}
	}
	return out, nil
}

func objectiveOf(precedence map[string]int, rank int) string {
	for o, i := range precedence {
		if i == rank {
			return o
		}
	}
	return "?"
}

// ResolveConflict is a person's answer to an escalated conflict: keep one
// proposal, reject the others.
func (s *Supervisor) ResolveConflict(ctx context.Context, actor core.Actor, id, keepProposalID int64, note string) (RecommendationConflict, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return RecommendationConflict{}, core.ErrForbidden
	}
	row, err := s.db.Reader().GetSupervisorConflict(ctx, store.GetSupervisorConflictParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return RecommendationConflict{}, core.NotFound("conflict %d not found", id)
	}
	if err != nil {
		return RecommendationConflict{}, err
	}
	c := conflictFromRow(row)
	if c.State != "escalated" {
		return RecommendationConflict{}, core.Conflict("conflict %d is %s", id, c.State)
	}
	keep := false
	for _, pid := range c.ProposalIDs {
		if pid == keepProposalID {
			keep = true
		}
	}
	if !keep {
		return RecommendationConflict{}, core.Invalid("proposal %d is not part of this conflict", keepProposalID)
	}
	for _, pid := range c.ProposalIDs {
		if pid != keepProposalID {
			_, _ = s.cfg.Proposals.Reject(ctx, actor, pid, "conflict "+strconv.FormatInt(id, 10)+" resolved for proposal "+strconv.FormatInt(keepProposalID, 10)+": "+note)
		}
	}
	n, err := s.db.Writer().ResolveSupervisorConflict(ctx, store.ResolveSupervisorConflictParams{Resolution: fmt.Sprintf("kept %d: %s", keepProposalID, note), ResolvedBy: optionalID(actor.UserID), ResolvedAt: sql.NullTime{Time: s.clock.Now().UTC(), Valid: true}, ID: id})
	if err != nil {
		return RecommendationConflict{}, err
	}
	if n != 1 {
		return RecommendationConflict{}, core.Conflict("conflict %d changed underneath", id)
	}
	row, err = s.db.Reader().GetSupervisorConflict(ctx, store.GetSupervisorConflictParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return RecommendationConflict{}, err
	}
	return conflictFromRow(row), nil
}

// Reconsider re-asks a denied or rejected proposal as a new one, evaluated
// under today's policy. The earlier proposal keeps its decision: a policy
// change never flips what was refused.
func (s *Supervisor) Reconsider(ctx context.Context, actor core.Actor, proposalID int64, reason string) (proposals.Proposal, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return proposals.Proposal{}, core.ErrForbidden
	}
	if strings.TrimSpace(reason) == "" {
		return proposals.Proposal{}, core.Invalid("say what changed")
	}
	old, err := s.cfg.Proposals.Get(ctx, actor, proposalID)
	if err != nil {
		return proposals.Proposal{}, err
	}
	if old.State != "denied" && old.State != "rejected" && old.State != "stale" {
		return proposals.Proposal{}, core.Conflict("proposal %d is %s; only denied, rejected or stale proposals are re-asked", proposalID, old.State)
	}
	var envelope, amount int64
	var currency string
	if old.EnvelopeID != nil {
		envelope = *old.EnvelopeID
	}
	if old.AmountMinor != nil {
		amount = *old.AmountMinor
	}
	if old.Currency != nil {
		currency = *old.Currency
	}
	p, _, err := s.cfg.Proposals.Propose(ctx, actor, proposals.ProposalInput{AccountID: old.AccountID, Action: old.Action, TargetKind: old.TargetKind, TargetRef: old.TargetRef, TargetVersion: old.TargetVersion, ExpectedCurrent: old.ExpectedCurrent, Proposed: old.Proposed,
		Reason: reason, EvidenceRefs: old.EvidenceRefs, AmountMinor: amount, Currency: currency, EnvelopeID: envelope, IdempotencyKey: fmt.Sprintf("reconsider:%d:%d", proposalID, s.clock.Now().UTC().UnixNano()), Supersedes: proposalID})
	return p, err
}

// Halt engages or clears the kill switch (policy's own) for an account or
// the workspace.
func (s *Supervisor) Halt(ctx context.Context, actor core.Actor, accountID int64, engaged bool, reason string) (policies.KillSwitch, error) {
	if s.cfg.Policies == nil {
		return policies.KillSwitch{}, core.Unavailable(nil, "policies are not wired")
	}
	return s.cfg.Policies.SetKillSwitch(ctx, actor, accountID, engaged, reason)
}

// Inbox gathers everything waiting on a person, most urgent first.
func (s *Supervisor) Inbox(ctx context.Context, actor core.Actor) ([]InboxItem, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	ws := actor.WorkspaceID
	var items []InboxItem
	if s.cfg.Proposals != nil {
		if list, err := s.cfg.Proposals.List(ctx, actor, "proposed", 200); err == nil {
			for _, p := range list {
				items = append(items, InboxItem{Kind: "proposal", Ref: p.ID, Severity: "warning", Title: fmt.Sprintf("Approve %s on %s %s?", p.Action, p.TargetKind, p.TargetRef), Detail: p.Reason, Actions: []string{"approve", "reject"}, Since: p.CreatedAt})
			}
		}
		for _, state := range []string{"uncertain", "failed", "reserved"} {
			if list, err := s.cfg.Proposals.ListIntents(ctx, actor, state, 200); err == nil {
				for _, i := range list {
					sev, title, actions := "warning", "Execution "+state, []string{"retry"}
					switch state {
					case "uncertain":
						sev, title, actions = "urgent", "Uncertain action: the provider did not confirm", []string{"reconcile"}
					case "reserved":
						if i.LastError == "" {
							continue
						}
						title, actions = "Held: "+i.LastError, []string{"retry"}
					case "failed":
						actions = []string{"retry", "cancel"}
					}
					items = append(items, InboxItem{Kind: "intent", Ref: i.ID, Severity: sev, Title: title, Detail: i.LastError, Actions: actions, Since: i.UpdatedAt})
				}
			}
		}
	}
	if rows, err := s.db.Reader().ListWorkflowTasksNeedingPeople(ctx, store.ListWorkflowTasksNeedingPeopleParams{WorkspaceID: ws, Limit: 200}); err == nil {
		for _, t := range rows {
			sev := "warning"
			if t.State == "failed" {
				sev = "urgent"
			}
			actions := []string{"retry"}
			if t.External == 1 && t.ExternalRef != "" {
				actions = []string{"inspect"}
			}
			items = append(items, InboxItem{Kind: "workflow_task", Ref: t.ID, Severity: sev, Title: fmt.Sprintf("Workflow step %s is %s", t.Key, t.State), Detail: t.Reason, Actions: actions, Since: t.UpdatedAt})
		}
	}
	if rows, err := s.db.Reader().ListFailedRuns(ctx, store.ListFailedRunsParams{WorkspaceID: ws, Limit: 100}); err == nil {
		for _, r := range rows {
			items = append(items, InboxItem{Kind: "run", Ref: r.ID, Severity: "warning", Title: fmt.Sprintf("Run %s %s", r.Kind, r.State), Detail: r.ErrorSafe.String, Actions: []string{"inspect"}, Since: r.CreatedAt})
		}
	}
	if rows, err := s.db.Reader().ListPendingCallSessions(ctx, store.ListPendingCallSessionsParams{WorkspaceID: ws, Limit: 100}); err == nil {
		for _, c := range rows {
			items = append(items, InboxItem{Kind: "call_reading", Ref: c.CallID, Severity: "info", Title: "A call's reading waits for a person", Detail: c.ReviewReason, Actions: []string{"confirm", "correct"}, Since: c.UpdatedAt})
		}
	}
	if rows, err := s.db.Reader().ListCallsByState(ctx, store.ListCallsByStateParams{WorkspaceID: ws, State: "unknown", Limit: 100}); err == nil {
		for _, c := range rows {
			items = append(items, InboxItem{Kind: "call", Ref: c.ID, Severity: "urgent", Title: "A call has an unknown result", Detail: c.Reason, Actions: []string{"reconcile"}, Since: c.UpdatedAt})
		}
	}
	if rows, err := s.db.Reader().ListFieldOutboxConflicts(ctx, store.ListFieldOutboxConflictsParams{WorkspaceID: ws, Limit: 100}); err == nil {
		for _, f := range rows {
			items = append(items, InboxItem{Kind: "field_conflict", Ref: f.ID, Severity: "warning", Title: fmt.Sprintf("CRM field %s disagrees", f.Field), Detail: f.Reason, Actions: []string{"keep_ours", "take_theirs"}, Since: f.UpdatedAt})
		}
	}
	if s.cfg.CRM != nil {
		if cands, err := s.cfg.CRM.OpenCandidates(ctx, actor, 100); err == nil {
			for _, c := range cands {
				items = append(items, InboxItem{Kind: "match_candidate", Ref: c.ID, Severity: "info", Title: "Two records may be one person", Detail: string(c.Reason), Actions: []string{"accept", "reject"}, Since: c.CreatedAt})
			}
		}
	}
	if rows, err := s.db.Reader().ListSupervisorConflicts(ctx, store.ListSupervisorConflictsParams{WorkspaceID: ws, Limit: 100}); err == nil {
		for _, c := range rows {
			if c.State != "escalated" {
				continue
			}
			items = append(items, InboxItem{Kind: "conflict", Ref: c.ID, Severity: "urgent", Title: "Recommendations compete for one budget", Detail: c.Detail, Actions: []string{"keep"}, Since: c.CreatedAt})
		}
	}
	if s.cfg.Policies != nil {
		if ps, err := s.cfg.Policies.Get(ctx, actor); err == nil {
			for _, k := range ps.KillSwitches {
				if k.Engaged {
					items = append(items, InboxItem{Kind: "kill_switch", Ref: k.AccountID, Severity: "urgent", Title: "Kill switch engaged", Detail: k.Reason, Actions: []string{"release"}, Since: k.SetAt})
				}
			}
			if !ps.Configured {
				items = append(items, InboxItem{Kind: "missing_config", Ref: 0, Severity: "info", Title: "No write policy is configured", Detail: "Every external change is denied until an admin saves one.", Actions: []string{"inspect"}, Since: s.clock.Now().UTC()})
			}
		}
	}
	rank := map[string]int{"urgent": 0, "warning": 1, "info": 2}
	sort.SliceStable(items, func(i, j int) bool {
		if rank[items[i].Severity] != rank[items[j].Severity] {
			return rank[items[i].Severity] < rank[items[j].Severity]
		}
		return items[i].Since.Before(items[j].Since)
	})
	if items == nil {
		items = []InboxItem{}
	}
	return items, nil
}

// Act performs one inbox action through the owning domain, which applies
// its own permissions and rules. The supervisor adds no authority.
func (s *Supervisor) Act(ctx context.Context, actor core.Actor, in ActionInput) (any, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return nil, core.ErrForbidden
	}
	switch in.Kind + "/" + in.Action {
	case "proposal/approve":
		return s.cfg.Proposals.Approve(ctx, actor, in.Ref, in.Note)
	case "proposal/reject":
		return s.cfg.Proposals.Reject(ctx, actor, in.Ref, in.Note)
	case "proposal/reconsider":
		return s.Reconsider(ctx, actor, in.Ref, in.Note)
	case "intent/retry":
		if s.cfg.Execution == nil {
			return nil, core.Unavailable(nil, "execution is not wired")
		}
		return s.cfg.Execution.Retry(ctx, actor, in.Ref)
	case "intent/reconcile":
		if s.cfg.Execution == nil {
			return nil, core.Unavailable(nil, "execution is not wired")
		}
		return s.cfg.Execution.Reconcile(ctx, actor, in.Ref)
	case "intent/cancel":
		return s.cfg.Proposals.Cancel(ctx, actor, in.Ref, in.Note)
	case "workflow_task/retry":
		if s.cfg.Workflows == nil {
			return nil, core.Unavailable(nil, "workflows are not wired")
		}
		return s.cfg.Workflows.Retry(ctx, actor, in.Ref)
	case "call/reconcile":
		if s.cfg.Calls == nil {
			return nil, core.Unavailable(nil, "calls are not wired")
		}
		return s.cfg.Calls.Reconcile(ctx, actor, in.Ref)
	case "call_reading/confirm":
		if s.cfg.Qualification == nil {
			return nil, core.Unavailable(nil, "qualification is not wired")
		}
		return s.cfg.Qualification.Confirm(ctx, actor, in.Ref)
	case "field_conflict/keep_ours", "field_conflict/take_theirs":
		if s.cfg.Outreach == nil {
			return nil, core.Unavailable(nil, "outreach is not wired")
		}
		return s.cfg.Outreach.ResolveConflict(ctx, actor, in.Ref, in.Action == "keep_ours", in.Note)
	case "match_candidate/accept", "match_candidate/reject":
		if s.cfg.CRM == nil {
			return nil, core.Unavailable(nil, "crm is not wired")
		}
		return s.cfg.CRM.Decide(ctx, actor, in.Ref, in.Action == "accept", in.Note)
	case "conflict/keep":
		return s.ResolveConflict(ctx, actor, in.Ref, in.Target, in.Note)
	case "kill_switch/release":
		return s.Halt(ctx, actor, in.Ref, false, in.Note)
	case "kill_switch/engage":
		return s.Halt(ctx, actor, in.Ref, true, in.Note)
	}
	return nil, core.Invalid("no action %q for %s", in.Action, in.Kind)
}

func (s *Supervisor) Conflicts(ctx context.Context, actor core.Actor, limit int) ([]RecommendationConflict, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Reader().ListSupervisorConflicts(ctx, store.ListSupervisorConflictsParams{WorkspaceID: actor.WorkspaceID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]RecommendationConflict, 0, len(rows))
	for _, r := range rows {
		out = append(out, conflictFromRow(r))
	}
	return out, nil
}

func (s *Supervisor) Recommendations(ctx context.Context, actor core.Actor, limit int) ([]Recommendation, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Reader().ListRecommendations(ctx, store.ListRecommendationsParams{WorkspaceID: actor.WorkspaceID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Recommendation, 0, len(rows))
	for _, r := range rows {
		out = append(out, Recommendation{ID: r.ID, ProposalID: r.ProposalID, Source: r.Source, Capability: r.Capability, Objective: r.Objective, CreatedAt: r.CreatedAt.UTC()})
	}
	return out, nil
}

func (s *Supervisor) current(ctx context.Context, workspaceID int64) (*SupervisorConfig, error) {
	row, err := s.db.Reader().GetCurrentSupervisorConfig(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := configFromRow(row)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func configFromRow(r store.SupervisorConfig) (SupervisorConfig, error) {
	var doc SupervisorDocument
	if err := json.Unmarshal([]byte(r.Document), &doc); err != nil {
		return SupervisorConfig{}, fmt.Errorf("supervisor config v%d: %w", r.Version, err)
	}
	if doc.Objectives == nil {
		doc.Objectives = []string{}
	}
	if doc.Autonomy == nil {
		doc.Autonomy = map[string]AutonomyRule{}
	}
	return SupervisorConfig{Version: r.Version, Document: doc, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC()}, nil
}

func conflictFromRow(r store.SupervisorConflict) RecommendationConflict {
	c := RecommendationConflict{ID: r.ID, EnvelopeID: r.EnvelopeID, ProposalIDs: []int64{}, State: r.State, Detail: r.Detail, Resolution: r.Resolution, CreatedAt: r.CreatedAt.UTC(), ResolvedBy: idPtr(r.ResolvedBy)}
	_ = json.Unmarshal([]byte(r.ProposalIds), &c.ProposalIDs)
	if r.ResolvedAt.Valid {
		t := r.ResolvedAt.Time.UTC()
		c.ResolvedAt = &t
	}
	return c
}

func dayStart(t time.Time) time.Time {
	l := t.In(dubai)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, dubai).UTC()
}

var dubai = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Dubai")
	if err != nil {
		return time.FixedZone("Dubai", 4*3600)
	}
	return loc
}()

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// Ensure the supervisor satisfies the workflow engine's recommender hooks.
var _ workflows.Recommender = (*Supervisor)(nil)
