// Package runs is the record of durable work: imports, refreshes, report
// builds. A run is named by an occurrence key (what the work is, not which
// attempt), so asking for the same work twice reuses the run instead of
// doing it twice. Workers lease a run and, when it touches a provider
// account, the account too, so one account is worked by one run at a time.
// A worker that dies leaves an expiring lease; recovery marks the run
// interrupted and a new attempt starts from what was recorded. A run that
// did not read everything can never be marked complete.
//
// The job queue is behind the Enqueuer interface. jobs implements it with
// River and inserts the job in the same transaction as the run row.
package runs

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

type State string

const (
	StateQueued      State = "queued"
	StateRunning     State = "running"
	StateSucceeded   State = "succeeded"
	StatePartial     State = "partial"
	StateFailed      State = "failed"
	StateInterrupted State = "interrupted"
)

type Completeness string

const (
	CompletenessUnknown  Completeness = "unknown"
	CompletenessComplete Completeness = "complete"
	CompletenessPartial  Completeness = "partial"
)

// Run is a run as the API shows it. Params and Progress are JSON documents
// the run kind owns.
type Run struct {
	ID              string          `json:"id"`
	Kind            string          `json:"kind"`
	OccurrenceKey   string          `json:"occurrenceKey"`
	AccountID       *int64          `json:"accountId"`
	State           State           `json:"state"`
	Completeness    Completeness    `json:"completeness"`
	Params          json.RawMessage `json:"params"`
	Progress        json.RawMessage `json:"progress"`
	Error           *string         `json:"error"`
	Attempt         int64           `json:"attempt"`
	CreatedAt       time.Time       `json:"createdAt"`
	StartedAt       *time.Time      `json:"startedAt"`
	FinishedAt      *time.Time      `json:"finishedAt"`
	ObservedThrough *time.Time      `json:"observedThrough"`
	// internal id for workers
	rowID int64
}

// RowID is the database id, for the job that works the run.
func (r Run) RowID() int64 { return r.rowID }

type StartInput struct {
	Kind          string
	OccurrenceKey string
	// AccountID is the provider account the run reads or writes, or 0.
	AccountID int64
	Params    any
}

// Completion is how a worker ends its run.
type Completion struct {
	State        State
	Completeness Completeness
	// Error is a message safe to show. Required for failed.
	Error string
}

// Enqueuer schedules a run to be worked, inside the transaction that created
// it, so a committed run always has its job and a rolled-back one never does.
type Enqueuer interface {
	Enqueue(ctx context.Context, tx *sql.Tx, runID int64) error
}

// NoQueue records runs without working them, for tests and for the CLI.
type NoQueue struct{}

func (NoQueue) Enqueue(context.Context, *sql.Tx, int64) error { return nil }

type Config struct {
	Clock core.Clock
	// LeaseFor is how long a worker owns a run between renewals.
	LeaseFor time.Duration
}

type Runs struct {
	db    *store.DB
	queue Enqueuer
	clock core.Clock
	lease time.Duration
}

func New(db *store.DB, queue Enqueuer, cfg Config) *Runs {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	if cfg.LeaseFor == 0 {
		cfg.LeaseFor = 5 * time.Minute
	}
	if queue == nil {
		queue = NoQueue{}
	}
	return &Runs{db: db, queue: queue, clock: cfg.Clock, lease: cfg.LeaseFor}
}

// Start records a run and queues it. A second Start with the same kind and
// occurrence key returns the existing run with created=false while it is
// queued, running or finished well. A failed or interrupted run is requeued
// as a new attempt. Marketers may start runs; viewers may not.
func (r *Runs) Start(ctx context.Context, actor core.Actor, in StartInput) (Run, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Run{}, false, core.ErrForbidden
	}
	if in.Kind == "" || len(in.Kind) > 100 || in.OccurrenceKey == "" || len(in.OccurrenceKey) > 500 {
		return Run{}, false, core.Invalid("run kind and occurrence key are required")
	}
	params := []byte("{}")
	if in.Params != nil {
		var err error
		if params, err = json.Marshal(in.Params); err != nil {
			return Run{}, false, err
		}
	}
	now := r.clock.Now().UTC()
	var (
		row     store.Run
		created bool
	)
	err := r.db.Tx(ctx, func(tx *sql.Tx, q *store.Queries) error {
		existing, err := q.GetRunByOccurrence(ctx, store.GetRunByOccurrenceParams{WorkspaceID: actor.WorkspaceID, Kind: in.Kind, OccurrenceKey: in.OccurrenceKey})
		switch {
		case errors.Is(err, sql.ErrNoRows):
			row, err = q.CreateRun(ctx, store.CreateRunParams{
				PublicID: "run_" + rand.Text(), WorkspaceID: actor.WorkspaceID, Kind: in.Kind, OccurrenceKey: in.OccurrenceKey,
				AccountID: optionalID(in.AccountID), Params: string(params), RequestedBy: optionalID(actor.UserID), CreatedAt: now,
			})
			if err != nil {
				return err
			}
			created = true
		case err != nil:
			return err
		case State(existing.State) == StateFailed || State(existing.State) == StateInterrupted:
			n, err := q.RequeueRun(ctx, store.RequeueRunParams{RequestedBy: optionalID(actor.UserID), CreatedAt: now, ID: existing.ID})
			if err != nil {
				return err
			}
			if n != 1 {
				return core.Conflict("run changed while requeuing; try again")
			}
			if row, err = q.GetRun(ctx, existing.ID); err != nil {
				return err
			}
			created = true
		default:
			row = existing
			return nil
		}
		return r.queue.Enqueue(ctx, tx, row.ID)
	})
	if store.IsUniqueViolation(err) {
		return Run{}, false, core.Conflict("that run was just started; refresh to see it")
	}
	if err != nil {
		return Run{}, false, err
	}
	return fromRow(row), created, nil
}

// Get returns one run by its public reference, in the actor's workspace.
func (r *Runs) Get(ctx context.Context, actor core.Actor, publicID string) (Run, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return Run{}, core.ErrForbidden
	}
	row, err := r.db.Reader().GetRunByPublicID(ctx, store.GetRunByPublicIDParams{WorkspaceID: actor.WorkspaceID, PublicID: publicID})
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, core.NotFound("run %s not found", publicID)
	}
	if err != nil {
		return Run{}, err
	}
	return fromRow(row), nil
}

// List returns the most recent runs in the workspace.
func (r *Runs) List(ctx context.Context, actor core.Actor, limit int) ([]Run, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Reader().ListRecentRuns(ctx, store.ListRecentRunsParams{WorkspaceID: actor.WorkspaceID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromRow(row))
	}
	return out, nil
}

// Handle is what a worker holds while it works a run. Everything it writes
// is conditional on still owning the lease.
type Handle struct {
	runs  *Runs
	id    int64
	owner string
	// Actor is ADX acting in the run's workspace, for the core calls the
	// worker makes.
	Actor core.Actor
	// Run is the row as leased, including progress from an earlier attempt.
	Run Run
}

// ErrLeased means another worker holds the run or its account right now.
var ErrLeased = core.Conflict("this work is already running elsewhere")

// Begin leases run id for owner, and its account if it has one. A run that
// is queued, or running on a lease that expired, can be taken. Anything else
// is refused. The account lease is refused while another live run holds it.
func (r *Runs) Begin(ctx context.Context, id int64, owner string) (*Handle, error) {
	if owner == "" {
		return nil, errors.New("lease owner is required")
	}
	now := r.clock.Now().UTC()
	until := now.Add(r.lease)
	var row store.Run
	err := r.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		row, err = q.GetRun(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return core.NotFound("run %d not found", id)
		}
		if err != nil {
			return err
		}
		if row.AccountID.Valid {
			lease, err := q.GetAccountLease(ctx, row.AccountID.Int64)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if err == nil && lease.RunID != id && !lease.Until.Before(now) {
				return ErrLeased
			}
		}
		n, err := q.LeaseRun(ctx, store.LeaseRunParams{Owner: sql.NullString{String: owner, Valid: true},
			LeaseUntil: sql.NullTime{Time: until, Valid: true}, Now: sql.NullTime{Time: now, Valid: true}, ID: id})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrLeased
		}
		if row.AccountID.Valid {
			if err := q.UpsertAccountLease(ctx, store.UpsertAccountLeaseParams{AccountID: row.AccountID.Int64, RunID: id, Owner: owner, Until: until}); err != nil {
				return err
			}
		}
		row, err = q.GetRun(ctx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Handle{runs: r, id: id, owner: owner, Actor: core.SystemIn(core.ViaJob, row.WorkspaceID), Run: fromRow(row)}, nil
}

// Progress records how far the run got and through when it has observed
// data. It also renews the lease. Losing the lease is an error the worker
// must stop on.
func (h *Handle) Progress(ctx context.Context, progress any, observedThrough *time.Time) error {
	raw, err := json.Marshal(progress)
	if err != nil {
		return err
	}
	now := h.runs.clock.Now().UTC()
	until := now.Add(h.runs.lease)
	return h.runs.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		n, err := q.UpdateRunProgress(ctx, store.UpdateRunProgressParams{Progress: string(raw), ObservedThrough: nullTimePtr(observedThrough), ID: h.id, LeaseOwner: sql.NullString{String: h.owner, Valid: true}})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrLeased
		}
		if _, err := q.RenewRunLease(ctx, store.RenewRunLeaseParams{LeaseUntil: sql.NullTime{Time: until, Valid: true}, ID: h.id, LeaseOwner: sql.NullString{String: h.owner, Valid: true}}); err != nil {
			return err
		}
		if h.Run.AccountID != nil {
			return q.UpsertAccountLease(ctx, store.UpsertAccountLeaseParams{AccountID: *h.Run.AccountID, RunID: h.id, Owner: h.owner, Until: until})
		}
		return nil
	})
}

// Finish ends the run and releases its leases. The state and completeness
// must agree: succeeded needs complete, partial needs partial, failed needs a
// message. A run can't be finished by anyone but its lease holder, so an
// interrupted run (whose lease was taken away) can't be marked done late.
func (h *Handle) Finish(ctx context.Context, c Completion) error {
	switch {
	case c.State == StateSucceeded && c.Completeness != CompletenessComplete:
		return core.Invalid("a run can only succeed when it read everything; use partial")
	case c.State == StatePartial && c.Completeness != CompletenessPartial:
		return core.Invalid("a partial run must say its completeness is partial")
	case c.State == StateFailed && c.Error == "":
		return core.Invalid("a failed run needs a safe error message")
	case c.State != StateSucceeded && c.State != StatePartial && c.State != StateFailed:
		return core.Invalid("a run finishes as succeeded, partial or failed")
	}
	if c.State == StateFailed && (c.Completeness == "" || c.Completeness == CompletenessUnknown) {
		c.Completeness = CompletenessPartial
	}
	now := h.runs.clock.Now().UTC()
	return h.runs.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		n, err := q.FinishRun(ctx, store.FinishRunParams{
			State: string(c.State), Completeness: string(c.Completeness), ErrorSafe: nullString(c.Error),
			FinishedAt: sql.NullTime{Time: now, Valid: true}, ID: h.id, LeaseOwner: sql.NullString{String: h.owner, Valid: true},
		})
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrLeased
		}
		if h.Run.AccountID != nil {
			return q.ReleaseAccountLease(ctx, store.ReleaseAccountLeaseParams{AccountID: *h.Run.AccountID, RunID: h.id})
		}
		return nil
	})
}

// MaxAutoAttempts is how many times recovery requeues an interrupted run on
// its own before leaving it for a person.
const MaxAutoAttempts = 3

// Retry requeues an interrupted or failed run as a new attempt, as ADX
// itself. Recovery uses it; people use Start, which does the same with their
// name on it.
func (r *Runs) Retry(ctx context.Context, id int64) (Run, error) {
	now := r.clock.Now().UTC()
	var row store.Run
	err := r.db.Tx(ctx, func(tx *sql.Tx, q *store.Queries) error {
		n, err := q.RequeueRun(ctx, store.RequeueRunParams{CreatedAt: now, ID: id})
		if err != nil {
			return err
		}
		if n != 1 {
			return core.Conflict("run %d is not failed or interrupted", id)
		}
		if row, err = q.GetRun(ctx, id); err != nil {
			return err
		}
		return r.queue.Enqueue(ctx, tx, id)
	})
	if err != nil {
		return Run{}, err
	}
	return fromRow(row), nil
}

// RecoverInterrupted marks every running run whose lease expired as
// interrupted and drops expired account leases. Startup and a periodic job
// call it. It returns the runs it interrupted so the caller can requeue or
// log them.
func (r *Runs) RecoverInterrupted(ctx context.Context) ([]Run, error) {
	now := r.clock.Now().UTC()
	out := []Run{}
	err := r.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		rows, err := q.InterruptExpiredRuns(ctx, store.InterruptExpiredRunsParams{FinishedAt: sql.NullTime{Time: now, Valid: true}, LeaseUntil: sql.NullTime{Time: now, Valid: true}})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out = append(out, fromRow(row))
		}
		return q.ReleaseExpiredAccountLeases(ctx, now)
	})
	return out, err
}

func fromRow(r store.Run) Run {
	out := Run{
		ID: r.PublicID, Kind: r.Kind, OccurrenceKey: r.OccurrenceKey, State: State(r.State), Completeness: Completeness(r.Completeness),
		Params: json.RawMessage(r.Params), Progress: json.RawMessage(r.Progress), Attempt: r.Attempt, CreatedAt: r.CreatedAt.UTC(),
		StartedAt: timePtr(r.StartedAt), FinishedAt: timePtr(r.FinishedAt), ObservedThrough: timePtr(r.ObservedThrough), rowID: r.ID,
	}
	if r.AccountID.Valid {
		out.AccountID = &r.AccountID.Int64
	}
	if r.ErrorSafe.Valid {
		out.Error = &r.ErrorSafe.String
	}
	return out
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	u := t.Time.UTC()
	return &u
}

func nullTimePtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
func optionalID(id int64) sql.NullInt64  { return sql.NullInt64{Int64: id, Valid: id != 0} }
