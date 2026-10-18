// Package execution carries an approved, reserved intent to a platform and
// records what the platform said. It is the only place ADX writes to an ad
// account. A write adapter per platform declares which actions it really
// implements; everything else fails as not implemented rather than being
// faked. Before each dispatch the policy is checked again, so a kill switch
// holds anything not yet sent. A response that may or may not have landed
// leaves the intent uncertain; it is looked up by its marker before any
// retry, so a timeout after a real create never creates a second object.
// "Submitted" means the platform accepted the request. It is not delivery:
// that is observed later from the platform's own data (B3.6).
package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
)

// ApplyRequest is one change for a platform adapter.
type ApplyRequest struct {
	Token             string
	AccountExternalID string
	Action            policies.WriteAction
	TargetKind        string
	TargetRef         string
	ExpectedCurrent   json.RawMessage
	Proposed          json.RawMessage
	// Marker is written into anything the adapter creates (in its name), so
	// a lost response can be found again by Lookup.
	Marker string
}

type ApplyResult struct {
	Refs   []proposals.ProviderRef
	Detail string
}

type LookupResult struct {
	Found  bool
	Refs   []proposals.ProviderRef
	Status string
	Detail string
}

type VerifyResult struct {
	Exists bool
	Status string
	Detail string
}

// UncertainError says the request was sent and no usable answer came back:
// the change may or may not have happened. Never retried blindly.
type UncertainError struct{ Cause error }

func (e *UncertainError) Error() string {
	if e.Cause == nil {
		return "uncertain outcome"
	}
	return "uncertain outcome: " + e.Cause.Error()
}
func (e *UncertainError) Unwrap() error { return e.Cause }

// Writer is a platform's write adapter. Implementations live in
// server/providers and are registered only when live writes are enabled.
type Writer interface {
	Platform() string
	// Capabilities lists the actions this adapter implements. Honest and
	// short: no feature parity is claimed.
	Capabilities() []policies.WriteAction
	Apply(ctx context.Context, req ApplyRequest) (ApplyResult, error)
	// Lookup finds what an earlier attempt with this marker created.
	Lookup(ctx context.Context, token, accountExternalID, marker string, action policies.WriteAction, targetRef string) (LookupResult, error)
	// Verify reads the objects behind refs back from the platform.
	Verify(ctx context.Context, token, accountExternalID string, refs []proposals.ProviderRef) (VerifyResult, error)
}

type Event struct {
	ID        int64     `json:"id"`
	IntentID  int64     `json:"intentId"`
	Attempt   int64     `json:"attempt"`
	FromState string    `json:"fromState"`
	ToState   string    `json:"toState"`
	Detail    string    `json:"detail"`
	ByUser    *int64    `json:"byUser"`
	At        time.Time `json:"at"`
}

type PlatformCapabilities struct {
	Platform       string   `json:"platform"`
	Live           bool     `json:"live"`
	Implemented    []string `json:"implemented"`
	NotImplemented []string `json:"notImplemented"`
	Note           string   `json:"note"`
}

type IntentWithEvents struct {
	Intent proposals.Intent `json:"intent"`
	Events []Event          `json:"events"`
}

type PlatformLaunch struct {
	Platform string             `json:"platform"`
	Intents  []IntentWithEvents `json:"intents"`
	State    string             `json:"state"`
}

// LaunchStatus is where a plan version's deployment stands across
// platforms. Delivered is never set here: delivery is observed from
// platform data, not inferred from a submission.
type LaunchStatus struct {
	PlanID      int64            `json:"planId"`
	PlanVersion int64            `json:"planVersion"`
	State       string           `json:"state"` // none, in_progress, partial, submitted, confirmed, failed
	Platforms   []PlatformLaunch `json:"platforms"`
	Delivered   bool             `json:"delivered"`
	Note        string           `json:"note"`
}

type Config struct {
	Clock core.Clock
	// MaxAttempts caps dispatch attempts per intent. Default 3.
	MaxAttempts int
	// ReconcileAfter is how long an uncertain intent waits before lookup.
	// Default a minute.
	ReconcileAfter time.Duration
	// LiveWritesEnabled is the server switch. With it off no writer is
	// registered and every intent is held with that reason.
	LiveWritesEnabled bool
}

type Dispatcher struct {
	db        *store.DB
	clock     core.Clock
	policies  *policies.Policies
	proposals *proposals.Proposals
	tokens    reporting.Tokens
	writers   map[string]Writer
	cfg       Config
}

func New(db *store.DB, pol *policies.Policies, props *proposals.Proposals, tokens reporting.Tokens, writers map[string]Writer, cfg Config) *Dispatcher {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.ReconcileAfter <= 0 {
		cfg.ReconcileAfter = time.Minute
	}
	if writers == nil {
		writers = map[string]Writer{}
	}
	return &Dispatcher{db: db, clock: cfg.Clock, policies: pol, proposals: props, tokens: tokens, writers: writers, cfg: cfg}
}

// Capabilities says, per platform, what ADX can write right now.
func (d *Dispatcher) Capabilities() []PlatformCapabilities {
	out := []PlatformCapabilities{}
	for _, platform := range []string{"google_ads", "meta"} {
		pc := PlatformCapabilities{Platform: platform, Implemented: []string{}, NotImplemented: []string{}}
		w, ok := d.writers[platform]
		have := map[policies.WriteAction]bool{}
		if ok {
			pc.Live = true
			for _, a := range w.Capabilities() {
				have[a] = true
				pc.Implemented = append(pc.Implemented, string(a))
			}
		} else {
			pc.Note = "no write adapter is enabled on this server (ADX_LIVE_WRITES off or the platform is not connected); intents are held, not sent"
		}
		for _, a := range policies.Catalog {
			if !have[a.Action] {
				pc.NotImplemented = append(pc.NotImplemented, string(a.Action))
			}
		}
		sort.Strings(pc.Implemented)
		out = append(out, pc)
	}
	return out
}

// Tick works every intent that is due: reserved ones are dispatched,
// uncertain ones reconciled, submitted ones verified. One pass, no retries
// inside.
func (d *Dispatcher) Tick(ctx context.Context) (worked int, errs []error) {
	now := d.clock.Now().UTC()
	rows, err := d.db.Reader().ListDispatchableIntents(ctx, store.ListDispatchableIntentsParams{ReconcileAfter: sql.NullTime{Time: now, Valid: true}, Limit: 50})
	if err != nil {
		return 0, []error{err}
	}
	for _, row := range rows {
		if err := d.work(ctx, row, 0); err != nil {
			errs = append(errs, fmt.Errorf("intent %d: %w", row.ID, err))
			continue
		}
		worked++
	}
	submitted, err := d.db.Reader().ListIntentsByStateAll(ctx, store.ListIntentsByStateAllParams{State: "submitted", Limit: 50})
	if err != nil {
		return worked, append(errs, err)
	}
	for _, row := range submitted {
		if row.ReconcileAfter.Valid && row.ReconcileAfter.Time.After(now) {
			continue
		}
		if err := d.verify(ctx, row, 0); err != nil {
			errs = append(errs, fmt.Errorf("intent %d: %w", row.ID, err))
			continue
		}
		worked++
	}
	return worked, errs
}

func (d *Dispatcher) work(ctx context.Context, row store.ExecutionIntent, byUser int64) error {
	switch row.State {
	case "reserved":
		return d.dispatch(ctx, row, byUser)
	case "uncertain", "reconciling":
		return d.reconcile(ctx, row, byUser)
	case "submitted":
		return d.verify(ctx, row, byUser)
	}
	return nil
}

type target struct {
	proposal store.Proposal
	account  store.GetAccountByIDRow
	platform string
	writer   Writer
	token    string
}

// prepare re-checks authority and finds the adapter and token. A hold is
// not a failure: the intent waits with the reason on it.
func (d *Dispatcher) prepare(ctx context.Context, row store.ExecutionIntent) (target, string, error) {
	actor := core.SystemIn(core.ViaJob, row.WorkspaceID)
	prop, err := d.db.Reader().GetProposal(ctx, store.GetProposalParams{ID: row.ProposalID, WorkspaceID: row.WorkspaceID})
	if err != nil {
		return target{}, "", err
	}
	acct, err := d.db.Reader().GetAccountByID(ctx, row.AccountID)
	if err != nil {
		return target{}, "", err
	}
	t := target{proposal: prop, account: acct, platform: acct.Account.Type}
	if acct.Account.WorkspaceID != row.WorkspaceID {
		return t, "held: the account belongs to another workspace; nothing was sent", nil
	}
	req, err := d.proposals.PolicyRequestFor(ctx, row.WorkspaceID, row.AccountID, policies.WriteAction(prop.Action), prop.TargetKind, prop.TargetRef)
	if err != nil {
		return t, "", err
	}
	decision, err := d.policies.Evaluate(ctx, actor, req)
	if err != nil {
		return t, "", err
	}
	if decision.Outcome == policies.Denied {
		return t, "held: policy denies this action now (" + decision.Reason + "); nothing was sent", nil
	}
	w, ok := d.writers[t.platform]
	if !ok {
		return t, "held: no live write adapter for " + t.platform + " on this server; nothing was sent", nil
	}
	t.writer = w
	if d.tokens == nil {
		return t, "held: no credential source on this server", nil
	}
	token, err := d.tokens.ProviderToken(ctx, actor, row.AccountID)
	if err != nil {
		return t, "held: the account's credential is unavailable (" + safe(err) + ")", nil
	}
	t.token = token
	return t, "", nil
}

func marker(row store.ExecutionIntent) string { return fmt.Sprintf("adx:%d", row.ID) }

func (d *Dispatcher) dispatch(ctx context.Context, row store.ExecutionIntent, byUser int64) error {
	t, hold, err := d.prepare(ctx, row)
	if err != nil {
		return err
	}
	now := d.clock.Now().UTC()
	if hold != "" {
		return d.record(ctx, row, row.State, "reserved", 0, hold, byUser, sql.NullTime{Time: now.Add(10 * time.Minute), Valid: true}, row.ProviderRefs, false)
	}
	action := policies.WriteAction(t.proposal.Action)
	if !has(t.writer.Capabilities(), action) {
		return d.fail(ctx, row, fmt.Sprintf("action %s is not implemented for %s; nothing was sent", action, t.platform), byUser)
	}
	if row.Attempts >= int64(d.cfg.MaxAttempts) {
		return d.fail(ctx, row, fmt.Sprintf("gave up after %d attempts", row.Attempts), byUser)
	}
	if err := d.record(ctx, row, "reserved", "dispatching", 1, "sending to "+t.platform, byUser, sql.NullTime{}, row.ProviderRefs, true); err != nil {
		return err
	}
	cur, _ := d.db.Reader().GetExecutionIntentAny(ctx, row.ID)
	res, applyErr := t.writer.Apply(ctx, ApplyRequest{Token: t.token, AccountExternalID: t.account.Account.ExternalID, Action: action, TargetKind: t.proposal.TargetKind, TargetRef: t.proposal.TargetRef,
		ExpectedCurrent: json.RawMessage(t.proposal.ExpectedCurrent), Proposed: json.RawMessage(t.proposal.Proposed), Marker: marker(row)})
	var ue *UncertainError
	switch {
	case applyErr == nil:
		refs, _ := json.Marshal(res.Refs)
		return d.record(ctx, cur, "dispatching", "submitted", 0, "submitted: "+res.Detail+" (accepted by the platform, not yet confirmed, not delivery)", byUser, sql.NullTime{Time: d.clock.Now().UTC().Add(d.cfg.ReconcileAfter), Valid: true}, string(refs), false)
	case errors.As(applyErr, &ue):
		return d.record(ctx, cur, "dispatching", "uncertain", 0, "uncertain: the request was sent and no usable answer came back; it will be looked up by its marker before any retry", byUser, sql.NullTime{Time: d.clock.Now().UTC().Add(d.cfg.ReconcileAfter), Valid: true}, cur.ProviderRefs, false)
	default:
		return d.fail(ctx, cur, "the platform refused: "+safe(applyErr), byUser)
	}
}

func (d *Dispatcher) reconcile(ctx context.Context, row store.ExecutionIntent, byUser int64) error {
	t, hold, err := d.prepare(ctx, row)
	if err != nil {
		return err
	}
	now := d.clock.Now().UTC()
	if hold != "" {
		return d.record(ctx, row, row.State, row.State, 0, hold, byUser, sql.NullTime{Time: now.Add(10 * time.Minute), Valid: true}, row.ProviderRefs, false)
	}
	if row.State == "uncertain" {
		if err := d.record(ctx, row, "uncertain", "reconciling", 0, "looking the request up by its marker", byUser, sql.NullTime{}, row.ProviderRefs, false); err != nil {
			return err
		}
		row.State = "reconciling"
	}
	found, err := t.writer.Lookup(ctx, t.token, t.account.Account.ExternalID, marker(row), policies.WriteAction(t.proposal.Action), t.proposal.TargetRef)
	if err != nil {
		return d.record(ctx, row, "reconciling", "uncertain", 0, "lookup failed: "+safe(err)+"; will look again", byUser, sql.NullTime{Time: now.Add(d.cfg.ReconcileAfter), Valid: true}, row.ProviderRefs, false)
	}
	if found.Found {
		refs, _ := json.Marshal(found.Refs)
		return d.record(ctx, row, "reconciling", "submitted", 0, "found on the platform: "+found.Detail+" (the earlier request had landed; no second object was made)", byUser, sql.NullTime{Time: now.Add(d.cfg.ReconcileAfter), Valid: true}, string(refs), false)
	}
	if row.Attempts >= int64(d.cfg.MaxAttempts) {
		return d.fail(ctx, row, fmt.Sprintf("not found on the platform after %d attempts; gave up", row.Attempts), byUser)
	}
	return d.record(ctx, row, "reconciling", "reserved", 0, "not found on the platform; the request did not land and will be sent again", byUser, sql.NullTime{}, row.ProviderRefs, false)
}

func (d *Dispatcher) verify(ctx context.Context, row store.ExecutionIntent, byUser int64) error {
	t, hold, err := d.prepare(ctx, row)
	if err != nil {
		return err
	}
	now := d.clock.Now().UTC()
	if hold != "" {
		return d.record(ctx, row, "submitted", "submitted", 0, hold, byUser, sql.NullTime{Time: now.Add(10 * time.Minute), Valid: true}, row.ProviderRefs, false)
	}
	var refs []proposals.ProviderRef
	_ = json.Unmarshal([]byte(row.ProviderRefs), &refs)
	v, err := t.writer.Verify(ctx, t.token, t.account.Account.ExternalID, refs)
	if err != nil {
		return d.record(ctx, row, "submitted", "submitted", 0, "verification failed: "+safe(err), byUser, sql.NullTime{Time: now.Add(d.cfg.ReconcileAfter), Valid: true}, row.ProviderRefs, false)
	}
	if !v.Exists {
		return d.record(ctx, row, "submitted", "submitted", 0, "not visible on the platform yet: "+v.Detail, byUser, sql.NullTime{Time: now.Add(d.cfg.ReconcileAfter), Valid: true}, row.ProviderRefs, false)
	}
	if err := d.record(ctx, row, "submitted", "externally_confirmed", 0, "confirmed on the platform with status "+v.Status+" (existence and status; delivery is observed separately)", byUser, sql.NullTime{}, row.ProviderRefs, false); err != nil {
		return err
	}
	return d.db.Writer().SetProposalStateAny(ctx, store.SetProposalStateAnyParams{State: "done", UpdatedAt: now, ID: row.ProposalID})
}

// fail ends an intent, releases its reservation and marks the proposal.
func (d *Dispatcher) fail(ctx context.Context, row store.ExecutionIntent, detail string, byUser int64) error {
	now := d.clock.Now().UTC()
	return d.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		n, err := q.UpdateExecutionIntent(ctx, store.UpdateExecutionIntentParams{ToState: "failed", ProviderRefs: row.ProviderRefs, LastErrorSafe: detail, UpdatedAt: now, ID: row.ID, FromState: row.State})
		if err != nil {
			return err
		}
		if n != 1 {
			return core.Conflict("the execution changed underneath")
		}
		if _, err := q.CreateExecutionEvent(ctx, store.CreateExecutionEventParams{IntentID: row.ID, Attempt: row.Attempts, FromState: row.State, ToState: "failed", Detail: detail, ByUser: optionalID(byUser), At: now}); err != nil {
			return err
		}
		if row.ReservationID.Valid {
			if err := q.SetBudgetReservationState(ctx, store.SetBudgetReservationStateParams{State: "released", UpdatedAt: now, ID: row.ReservationID.Int64}); err != nil {
				return err
			}
		}
		return q.SetProposalStateAny(ctx, store.SetProposalStateAnyParams{State: "failed", UpdatedAt: now, ID: row.ProposalID})
	})
}

// record moves an intent and writes the event. attemptDelta counts a send.
func (d *Dispatcher) record(ctx context.Context, row store.ExecutionIntent, from, to string, attemptDelta int64, detail string, byUser int64, reconcileAfter sql.NullTime, refs string, setDispatched bool) error {
	now := d.clock.Now().UTC()
	return d.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		params := store.UpdateExecutionIntentParams{ToState: to, AttemptDelta: attemptDelta, ProviderRefs: refs, LastErrorSafe: detail, ReconcileAfter: reconcileAfter, UpdatedAt: now, ID: row.ID, FromState: from}
		if setDispatched {
			params.DispatchedAt = sql.NullTime{Time: now, Valid: true}
		}
		if to == "externally_confirmed" {
			params.ConfirmedAt = sql.NullTime{Time: now, Valid: true}
		}
		n, err := q.UpdateExecutionIntent(ctx, params)
		if err != nil {
			return err
		}
		if n != 1 {
			return core.Conflict("the execution changed underneath")
		}
		if to == "submitted" && row.ReservationID.Valid {
			if err := q.SetBudgetReservationState(ctx, store.SetBudgetReservationStateParams{State: "consumed", UpdatedAt: now, ID: row.ReservationID.Int64}); err != nil {
				return err
			}
		}
		_, err = q.CreateExecutionEvent(ctx, store.CreateExecutionEventParams{IntentID: row.ID, Attempt: row.Attempts + attemptDelta, FromState: from, ToState: to, Detail: detail, ByUser: optionalID(byUser), At: now})
		return err
	})
}

// Operator actions.

// Retry puts a failed intent back in the outbox with its budget reserved
// again. Admins only. Authority is checked again at dispatch.
func (d *Dispatcher) Retry(ctx context.Context, actor core.Actor, intentID int64) (proposals.Intent, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return proposals.Intent{}, core.ErrForbidden
	}
	row, err := d.intent(ctx, actor.WorkspaceID, intentID)
	if err != nil {
		return proposals.Intent{}, err
	}
	if row.State != "failed" {
		return proposals.Intent{}, core.Conflict("execution %d is %s; only a failed execution is retried", intentID, row.State)
	}
	prop, err := d.db.Reader().GetProposal(ctx, store.GetProposalParams{ID: row.ProposalID, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return proposals.Intent{}, err
	}
	now := d.clock.Now().UTC()
	err = d.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		if prop.AmountMinor.Valid && prop.AmountMinor.Int64 > 0 && row.ReservationID.Valid {
			env, err := q.GetBudgetEnvelope(ctx, store.GetBudgetEnvelopeParams{ID: prop.EnvelopeID.Int64, WorkspaceID: actor.WorkspaceID})
			if err != nil {
				return core.Conflict("the budget envelope is gone")
			}
			taken, err := q.SumActiveReservations(ctx, env.ID)
			if err != nil {
				return err
			}
			if toInt64(taken)+prop.AmountMinor.Int64 > env.AmountMinor {
				return core.Conflict("insufficient remaining budget to retry")
			}
			if err := q.SetBudgetReservationState(ctx, store.SetBudgetReservationStateParams{State: "reserved", UpdatedAt: now, ID: row.ReservationID.Int64}); err != nil {
				return err
			}
		}
		n, err := q.UpdateExecutionIntent(ctx, store.UpdateExecutionIntentParams{ToState: "reserved", ProviderRefs: row.ProviderRefs, LastErrorSafe: "retry requested", UpdatedAt: now, ID: row.ID, FromState: "failed"})
		if err != nil {
			return err
		}
		if n != 1 {
			return core.Conflict("the execution changed; reload and try again")
		}
		if _, err := q.CreateExecutionEvent(ctx, store.CreateExecutionEventParams{IntentID: row.ID, Attempt: row.Attempts, FromState: "failed", ToState: "reserved", Detail: "retry requested by an admin", ByUser: optionalID(actor.UserID), At: now}); err != nil {
			return err
		}
		return q.SetProposalStateAny(ctx, store.SetProposalStateAnyParams{State: "executing", UpdatedAt: now, ID: row.ProposalID})
	})
	if err != nil {
		return proposals.Intent{}, err
	}
	return d.proposals.GetIntent(ctx, actor, intentID)
}

// Reconcile works one intent now instead of waiting for the tick. Admins
// and marketers may; the assistant may not.
func (d *Dispatcher) Reconcile(ctx context.Context, actor core.Actor, intentID int64) (proposals.Intent, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return proposals.Intent{}, core.ErrForbidden
	}
	row, err := d.intent(ctx, actor.WorkspaceID, intentID)
	if err != nil {
		return proposals.Intent{}, err
	}
	switch row.State {
	case "reserved", "uncertain", "reconciling", "submitted":
	default:
		return proposals.Intent{}, core.Conflict("execution %d is %s; nothing to reconcile", intentID, row.State)
	}
	if err := d.work(ctx, row, actor.UserID); err != nil {
		return proposals.Intent{}, err
	}
	return d.proposals.GetIntent(ctx, actor, intentID)
}

// Timeline is an intent with its events.
func (d *Dispatcher) Timeline(ctx context.Context, actor core.Actor, intentID int64) (IntentWithEvents, error) {
	in, err := d.proposals.GetIntent(ctx, actor, intentID)
	if err != nil {
		return IntentWithEvents{}, err
	}
	rows, err := d.db.Reader().ListExecutionEvents(ctx, intentID)
	if err != nil {
		return IntentWithEvents{}, err
	}
	out := IntentWithEvents{Intent: in, Events: []Event{}}
	for _, r := range rows {
		out.Events = append(out.Events, Event{ID: r.ID, IntentID: r.IntentID, Attempt: r.Attempt, FromState: r.FromState, ToState: r.ToState, Detail: r.Detail, ByUser: idPtr(r.ByUser), At: r.At.UTC()})
	}
	return out, nil
}

// Launch is the per-platform state of a plan version's intents.
func (d *Dispatcher) Launch(ctx context.Context, actor core.Actor, planID, version int64) (LaunchStatus, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return LaunchStatus{}, core.ErrForbidden
	}
	rows, err := d.db.Reader().ListIntentsForPlanVersion(ctx, store.ListIntentsForPlanVersionParams{WorkspaceID: actor.WorkspaceID, PlanID: planID, PlanVersion: version})
	if err != nil {
		return LaunchStatus{}, err
	}
	ls := LaunchStatus{PlanID: planID, PlanVersion: version, Platforms: []PlatformLaunch{}, Note: "Submitted means the platform accepted the request. Confirmed means the object exists there. Neither is delivery; delivery is read from the platform's own figures."}
	byPlatform := map[string]*PlatformLaunch{}
	order := []string{}
	for _, r := range rows {
		acct, err := d.db.Reader().GetAccountByID(ctx, r.AccountID)
		if err != nil {
			return LaunchStatus{}, err
		}
		pl, ok := byPlatform[acct.Account.Type]
		if !ok {
			pl = &PlatformLaunch{Platform: acct.Account.Type, Intents: []IntentWithEvents{}}
			byPlatform[acct.Account.Type] = pl
			order = append(order, acct.Account.Type)
		}
		tl, err := d.Timeline(ctx, actor, r.ID)
		if err != nil {
			return LaunchStatus{}, err
		}
		pl.Intents = append(pl.Intents, tl)
	}
	counts := map[string]int{}
	for _, p := range order {
		pl := byPlatform[p]
		pl.State = rollup(pl.Intents)
		counts[pl.State]++
		ls.Platforms = append(ls.Platforms, *pl)
	}
	switch {
	case len(rows) == 0:
		ls.State = "none"
	case counts["confirmed"] == len(order):
		ls.State = "confirmed"
	case counts["failed"] == len(order):
		ls.State = "failed"
	case counts["failed"] > 0 || (counts["partial"] > 0):
		ls.State = "partial"
	case counts["submitted"]+counts["confirmed"] == len(order):
		ls.State = "submitted"
	default:
		ls.State = "in_progress"
	}
	return ls, nil
}

func rollup(intents []IntentWithEvents) string {
	c := map[string]int{}
	for _, i := range intents {
		switch i.Intent.State {
		case "externally_confirmed", "confirmed":
			c["confirmed"]++
		case "submitted":
			c["submitted"]++
		case "failed", "cancelled":
			c["failed"]++
		default:
			c["in_progress"]++
		}
	}
	n := len(intents)
	switch {
	case n == 0:
		return "none"
	case c["confirmed"] == n:
		return "confirmed"
	case c["failed"] == n:
		return "failed"
	case c["failed"] > 0:
		return "partial"
	case c["submitted"]+c["confirmed"] == n:
		return "submitted"
	}
	return "in_progress"
}

func (d *Dispatcher) intent(ctx context.Context, workspaceID, id int64) (store.ExecutionIntent, error) {
	row, err := d.db.Reader().GetExecutionIntent(ctx, store.GetExecutionIntentParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.ExecutionIntent{}, core.NotFound("execution %d not found", id)
	}
	return row, err
}

func has(xs []policies.WriteAction, a policies.WriteAction) bool {
	for _, x := range xs {
		if x == a {
			return true
		}
	}
	return false
}

// safe is the message shown for a provider error: ours when it is a
// core.Error, generic otherwise.
func safe(err error) string {
	var ce *core.Error
	if errors.As(err, &ce) {
		return ce.Message
	}
	s := err.Error()
	if len(s) > 160 || strings.Contains(s, "http") {
		return "the platform returned an error; see the server log"
	}
	return s
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
