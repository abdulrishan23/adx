package jobs_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/jobs"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db       *store.DB
	runs     *runs.Runs
	evidence *evidence.Store
	worker   *jobs.Worker
	executed chan string
}

func start(t *testing.T, executors map[string]jobs.Executor) *env {
	t.Helper()
	db := storetest.Open(t)
	coretest.SeedUsers(t, db)
	require.NoError(t, jobs.Migrate(t.Context(), db))
	queue := &jobs.Queue{}
	e := &env{db: db, executed: make(chan string, 16)}
	e.runs = runs.New(db, queue, runs.Config{})
	e.evidence = evidence.New(db, nil)
	var err error
	e.worker, err = jobs.New(db, slog.New(slog.DiscardHandler), jobs.Deps{Runs: e.runs, Executors: executors, Owner: "test", RecoverEvery: time.Hour})
	require.NoError(t, err)
	queue.Bind(e.worker)
	require.NoError(t, e.worker.Client.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = e.worker.Client.Stop(ctx)
	})
	return e
}

func (e *env) waitFor(t *testing.T, id string, want runs.State) runs.Run {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		run, err := e.runs.Get(t.Context(), coretest.Viewer, id)
		require.NoError(t, err)
		if run.State == want {
			return run
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run %s never reached %s", id, want)
	return runs.Run{}
}

// Catches: a run committed without its job, the same run worked twice, the
// executor's evidence not landing in the run's workspace, and a run with no
// executor hanging instead of failing visibly.
func TestRunsAreWorkedOnceThroughRiver(t *testing.T) {
	var e *env
	e = start(t, map[string]jobs.Executor{
		"fake.import": jobs.ExecutorFunc(func(ctx context.Context, h *runs.Handle) (runs.Completion, error) {
			e.executed <- h.Run.OccurrenceKey
			for i := range 3 {
				_, _, err := e.evidence.Record(ctx, h.Actor, evidence.Observation{RunID: h.Run.RowID(), Source: "fake", Kind: "page",
					SourceRef: string(rune('a' + i)), Class: evidence.Observed, Completeness: evidence.Complete, Payload: map[string]int{"n": i}})
				if err != nil {
					return runs.Completion{}, err
				}
				if err := h.Progress(ctx, map[string]int{"pages": i + 1}, nil); err != nil {
					return runs.Completion{}, err
				}
			}
			return runs.Completion{State: runs.StateSucceeded, Completeness: runs.CompletenessComplete}, nil
		}),
		"fake.broken": jobs.ExecutorFunc(func(context.Context, *runs.Handle) (runs.Completion, error) {
			return runs.Completion{}, core.Conflict("GoHighLevel request limit reached; try again later")
		}),
		"fake.panics": jobs.ExecutorFunc(func(context.Context, *runs.Handle) (runs.Completion, error) {
			return runs.Completion{}, errors.New("nil pointer at /srv/adx/secret")
		}),
	})

	run, created, err := e.runs.Start(t.Context(), coretest.Marketer, runs.StartInput{Kind: "fake.import", OccurrenceKey: "loc-1/day-1"})
	require.NoError(t, err)
	require.True(t, created)
	_, created, err = e.runs.Start(t.Context(), coretest.Marketer, runs.StartInput{Kind: "fake.import", OccurrenceKey: "loc-1/day-1"})
	require.NoError(t, err)
	assert.False(t, created)

	done := e.waitFor(t, run.ID, runs.StateSucceeded)
	assert.Equal(t, runs.CompletenessComplete, done.Completeness)
	assert.JSONEq(t, `{"pages":3}`, string(done.Progress))
	assert.Equal(t, "loc-1/day-1", <-e.executed)
	select {
	case key := <-e.executed:
		t.Fatalf("executed twice: %s", key)
	case <-time.After(300 * time.Millisecond):
	}
	n, err := e.evidence.Count(t.Context(), coretest.Viewer, "fake", "page")
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)

	broken, _, err := e.runs.Start(t.Context(), coretest.Marketer, runs.StartInput{Kind: "fake.broken", OccurrenceKey: "x"})
	require.NoError(t, err)
	failed := e.waitFor(t, broken.ID, runs.StateFailed)
	assert.Equal(t, "GoHighLevel request limit reached; try again later", *failed.Error)

	internal, _, err := e.runs.Start(t.Context(), coretest.Marketer, runs.StartInput{Kind: "fake.panics", OccurrenceKey: "y"})
	require.NoError(t, err)
	failed = e.waitFor(t, internal.ID, runs.StateFailed)
	assert.NotContains(t, *failed.Error, "/srv/adx", "raw errors stay in the log")

	unknown, _, err := e.runs.Start(t.Context(), coretest.Marketer, runs.StartInput{Kind: "nobody.knows", OccurrenceKey: "z"})
	require.NoError(t, err)
	failed = e.waitFor(t, unknown.ID, runs.StateFailed)
	assert.Contains(t, *failed.Error, "no worker knows")
}
