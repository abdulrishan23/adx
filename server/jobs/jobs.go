// Package jobs runs background work on River. Workers stay thin: they call
// core, the same way api handlers do.
package jobs

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riversqlite"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/WokeWorks/adx/server/store"
)

// Client inserts and works jobs. Insert with InsertTx inside store.DB.Tx when
// a job must exist only if the rest of the transaction commits.
type Client = river.Client[*sql.Tx]

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

func New(db *store.DB, log *slog.Logger) (*Client, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &optimizeWorker{db: db})

	// River shares the single write connection with the app, which is how
	// River's docs recommend running on SQLite.
	return river.NewClient(riversqlite.New(db.Write), &river.Config{
		Logger:  log,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}},
		Workers: workers,
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(6*time.Hour),
				func() (river.JobArgs, *river.InsertOpts) { return optimizeArgs{}, nil },
				nil,
			),
		},
	})
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
