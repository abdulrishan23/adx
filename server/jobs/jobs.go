// Package jobs runs background work on River. Workers stay thin: they call
// core, the same way api handlers do. The one generic worker here works a
// core/runs run by handing it to the executor registered for its kind.
package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riversqlite"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/store"
)

// Client inserts and works jobs. Insert with InsertTx inside store.DB.Tx when
// a job must exist only if the rest of the transaction commits.
type Client = river.Client[*sql.Tx]

// Executor does the work of one run kind. It reads and writes through the
// handle, which refuses writes once the lease is lost, and returns how the
// run ended. Returning an error ends the run as failed with a safe message.
type Executor interface {
	Execute(ctx context.Context, h *runs.Handle) (runs.Completion, error)
}

// ExecutorFunc adapts a function to Executor.
type ExecutorFunc func(ctx context.Context, h *runs.Handle) (runs.Completion, error)

func (f ExecutorFunc) Execute(ctx context.Context, h *runs.Handle) (runs.Completion, error) {
	return f(ctx, h)
}

// Queue is the runs.Enqueuer core gets. It is created before the River
// client, which needs the runs service that needs the queue, and bound to
// the client once that exists.
type Queue struct{ client *Client }

func (q *Queue) Bind(w *Worker) { q.client = w.Client }

// Enqueue inserts the run's job in the caller's transaction. River's unique
// option on run_id means a run that is already queued, running or retrying
// gets no second job.
func (q *Queue) Enqueue(ctx context.Context, tx *sql.Tx, runID int64) error {
	if q.client == nil {
		return errors.New("job queue is not started")
	}
	_, err := q.client.InsertTx(ctx, tx, runArgs{RunID: runID}, &river.InsertOpts{
		UniqueOpts: river.UniqueOpts{ByArgs: true},
	})
	return err
}

// Migrate creates or updates River's own tables. They live next to ours in
// the same SQLite file but outside our migrations.
func Migrate(ctx context.Context, db *store.DB) error {
	m, err := rivermigrate.New(riversqlite.New(db.Write), nil)
	if err != nil {
		return err
	}
	_, err = m.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

type Deps struct {
	Runs      *runs.Runs
	Executors map[string]Executor
	// Owner names this process in leases. Defaults to the hostname and pid.
	Owner string
	// RecoverEvery is how often expired leases are swept. Default a minute.
	RecoverEvery time.Duration
}

// Worker is the River client plus what the run worker needs.
type Worker struct {
	Client *Client
	runs   *runs.Runs
	log    *slog.Logger
}

func New(db *store.DB, log *slog.Logger, d Deps) (*Worker, error) {
	if d.Owner == "" {
		host, _ := os.Hostname()
		d.Owner = fmt.Sprintf("%s/%d", host, os.Getpid())
	}
	if d.RecoverEvery == 0 {
		d.RecoverEvery = time.Minute
	}
	w := &Worker{runs: d.Runs, log: log}
	workers := river.NewWorkers()
	river.AddWorker(workers, &optimizeWorker{db: db})
	periodic := []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(6*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return optimizeArgs{}, nil }, nil),
	}
	if d.Runs != nil {
		river.AddWorker(workers, &runWorker{runs: d.Runs, executors: d.Executors, owner: d.Owner, log: log})
		river.AddWorker(workers, &recoverWorker{runs: d.Runs, log: log})
		periodic = append(periodic, river.NewPeriodicJob(river.PeriodicInterval(d.RecoverEvery),
			func() (river.JobArgs, *river.InsertOpts) { return recoverArgs{}, nil }, &river.PeriodicJobOpts{RunOnStart: true}))
	}

	// River shares the single write connection with the app, which is how
	// River's docs recommend running on SQLite.
	client, err := river.NewClient(riversqlite.New(db.Write), &river.Config{
		Logger:       log,
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}},
		Workers:      workers,
		PeriodicJobs: periodic,
	})
	if err != nil {
		return nil, err
	}
	w.Client = client
	return w, nil
}

type optimizeArgs struct{}

func (optimizeArgs) Kind() string { return "store_optimize" }

type optimizeWorker struct {
	river.WorkerDefaults[optimizeArgs]
	db *store.DB
}

func (w *optimizeWorker) Work(ctx context.Context, _ *river.Job[optimizeArgs]) error {
	return w.db.Optimize(ctx)
}

// runArgs works one core/runs run. The run id is the whole identity.
type runArgs struct {
	RunID int64 `json:"run_id" river:"unique"`
}

func (runArgs) Kind() string { return "run.execute" }

type runWorker struct {
	river.WorkerDefaults[runArgs]
	runs      *runs.Runs
	executors map[string]Executor
	owner     string
	log       *slog.Logger
}

func (w *runWorker) Timeout(*river.Job[runArgs]) time.Duration { return 2 * time.Hour }

func (w *runWorker) Work(ctx context.Context, job *river.Job[runArgs]) error {
	owner := fmt.Sprintf("%s/job-%d", w.owner, job.ID)
	h, err := w.runs.Begin(ctx, job.Args.RunID, owner)
	switch {
	case errors.Is(err, runs.ErrLeased):
		// Someone else holds it, or it already finished. Let River retry
		// later; if it has finished, Begin keeps refusing and the job is
		// discarded after its attempts.
		return err
	case err != nil:
		return err
	}
	exec, ok := w.executors[h.Run.Kind]
	if !ok {
		return h.Finish(ctx, runs.Completion{State: runs.StateFailed, Error: "no worker knows how to run " + h.Run.Kind})
	}
	completion, execErr := exec.Execute(ctx, h)
	if execErr != nil {
		w.log.ErrorContext(ctx, "run failed", "run", h.Run.ID, "kind", h.Run.Kind, "error", execErr)
		if errors.Is(execErr, runs.ErrLeased) {
			// The lease was taken away: recovery already marked the run
			// interrupted. Nothing to record.
			return nil
		}
		completion = runs.Completion{State: runs.StateFailed, Error: safeMessage(execErr)}
	}
	if err := h.Finish(ctx, completion); err != nil && !errors.Is(err, runs.ErrLeased) {
		return err
	}
	return nil
}

func safeMessage(err error) string {
	var ce *core.Error
	if errors.As(err, &ce) {
		return ce.Message
	}
	return "the run failed; see the server log"
}

type recoverArgs struct{}

func (recoverArgs) Kind() string { return "runs.recover" }

type recoverWorker struct {
	river.WorkerDefaults[recoverArgs]
	runs *runs.Runs
	log  *slog.Logger
}

// Work marks runs whose worker died as interrupted and retries them while
// they have attempts left.
func (w *recoverWorker) Work(ctx context.Context, _ *river.Job[recoverArgs]) error {
	interrupted, err := w.runs.RecoverInterrupted(ctx)
	if err != nil {
		return err
	}
	for _, run := range interrupted {
		if run.Attempt+1 >= runs.MaxAutoAttempts {
			w.log.WarnContext(ctx, "interrupted run is out of automatic attempts; start it again by hand", "run", run.ID, "attempt", run.Attempt)
			continue
		}
		if _, err := w.runs.Retry(ctx, run.RowID()); err != nil {
			w.log.WarnContext(ctx, "could not retry interrupted run", "run", run.ID, "error", err)
		}
	}
	return nil
}
