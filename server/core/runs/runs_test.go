package runs_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type queue struct{ enqueued []int64 }

func (q *queue) Enqueue(_ context.Context, _ *sql.Tx, id int64) error {
	q.enqueued = append(q.enqueued, id)
	return nil
}

type env struct {
	db    *store.DB
	clock *coretest.Clock
	queue *queue
	runs  *runs.Runs
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)), queue: &queue{}}
	coretest.SeedUsers(t, e.db)
	e.runs = runs.New(e.db, e.queue, runs.Config{Clock: e.clock, LeaseFor: 5 * time.Minute})
	return e
}

func (e *env) account(t *testing.T, external string) int64 {
	t.Helper()
	cred, err := e.db.Writer().UpsertCredential(t.Context(), store.UpsertCredentialParams{Provider: "gohighlevel", Kind: "private_token", Subject: external,
		EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(t.Context(), store.UpsertAccountParams{Type: "gohighlevel", ExternalID: external, CredentialID: cred.ID,
		Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	return acct.ID
}

// Catches: the same work queued twice, a finished run being redone by a
// repeat request, and a failed run that can't be retried.
func TestSameOccurrenceIsReusedAndFailuresRetry(t *testing.T) {
	e := setup(t)
	in := runs.StartInput{Kind: "ghl.import", OccurrenceKey: "loc-1/2026-10-17", Params: map[string]string{"location": "loc-1"}}

	first, created, err := e.runs.Start(t.Context(), coretest.Marketer, in)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, runs.StateQueued, first.State)
	assert.Equal(t, []int64{first.RowID()}, e.queue.enqueued)

	again, created, err := e.runs.Start(t.Context(), coretest.Marketer, in)
	require.NoError(t, err)
	assert.False(t, created, "queued work is reused")
	assert.Equal(t, first.ID, again.ID)
	assert.Len(t, e.queue.enqueued, 1, "no second job")

	h, err := e.runs.Begin(t.Context(), first.RowID(), "worker-a")
	require.NoError(t, err)
	_, created, err = e.runs.Start(t.Context(), coretest.Marketer, in)
	require.NoError(t, err)
	assert.False(t, created, "running work is reused")

	require.NoError(t, h.Finish(t.Context(), runs.Completion{State: runs.StateFailed, Error: "GoHighLevel request limit reached"}))
	got, err := e.runs.Get(t.Context(), coretest.Viewer, first.ID)
	require.NoError(t, err)
	assert.Equal(t, runs.StateFailed, got.State)
	assert.Equal(t, "GoHighLevel request limit reached", *got.Error)

	retry, created, err := e.runs.Start(t.Context(), coretest.Marketer, in)
	require.NoError(t, err)
	assert.True(t, created, "a failed run is requeued as a new attempt")
	assert.Equal(t, first.ID, retry.ID, "same occurrence, same reference")
	assert.Equal(t, int64(1), retry.Attempt)
	assert.Equal(t, runs.StateQueued, retry.State)
	assert.Nil(t, retry.Error)
	assert.Len(t, e.queue.enqueued, 2)

	_, _, err = e.runs.Start(t.Context(), coretest.Viewer, in)
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, _, err = e.runs.Start(t.Context(), coretest.Marketer, runs.StartInput{Kind: "x"})
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindInvalid, ce.Kind)
}

// Catches: a run that read half the pages being recorded as complete, and
// the lease holder's bookkeeping being skipped.
func TestPartialReadsCannotBeMarkedComplete(t *testing.T) {
	e := setup(t)
	run, _, err := e.runs.Start(t.Context(), coretest.Marketer, runs.StartInput{Kind: "google.refresh", OccurrenceKey: "acct/2026-10"})
	require.NoError(t, err)
	h, err := e.runs.Begin(t.Context(), run.RowID(), "worker-a")
	require.NoError(t, err)
	through := e.clock.Now().Add(-time.Hour)
	require.NoError(t, h.Progress(t.Context(), map[string]int{"pages": 3, "of": 7}, &through))

	err = h.Finish(t.Context(), runs.Completion{State: runs.StateSucceeded, Completeness: runs.CompletenessPartial})
	require.Error(t, err)
	err = h.Finish(t.Context(), runs.Completion{State: runs.StateSucceeded, Completeness: runs.CompletenessUnknown})
	require.Error(t, err)
	err = h.Finish(t.Context(), runs.Completion{State: runs.StateFailed})
	require.Error(t, err, "a failure needs a message")

	require.NoError(t, h.Finish(t.Context(), runs.Completion{State: runs.StatePartial, Completeness: runs.CompletenessPartial}))
	got, err := e.runs.Get(t.Context(), coretest.Viewer, run.ID)
	require.NoError(t, err)
	assert.Equal(t, runs.StatePartial, got.State)
	assert.Equal(t, runs.CompletenessPartial, got.Completeness)
	assert.JSONEq(t, `{"pages":3,"of":7}`, string(got.Progress))
	assert.Equal(t, through, *got.ObservedThrough)
	assert.NotNil(t, got.FinishedAt)
}

// Catches: two workers taking one run, two runs reading one account at once,
// and a lease that never expires after a crash.
func TestLeasesAreExclusiveAndExpire(t *testing.T) {
	e := setup(t)
	acct := e.account(t, "loc-1")
	a, _, err := e.runs.Start(t.Context(), coretest.Marketer, runs.StartInput{Kind: "ghl.import", OccurrenceKey: "a", AccountID: acct})
	require.NoError(t, err)
	b, _, err := e.runs.Start(t.Context(), coretest.Marketer, runs.StartInput{Kind: "ghl.check", OccurrenceKey: "b", AccountID: acct})
	require.NoError(t, err)

	ha, err := e.runs.Begin(t.Context(), a.RowID(), "worker-1")
	require.NoError(t, err)
	assert.Equal(t, core.SystemIn(core.ViaJob, core.DefaultWorkspace), ha.Actor)
	_, err = e.runs.Begin(t.Context(), a.RowID(), "worker-2")
	assert.ErrorIs(t, err, runs.ErrLeased, "the run is taken")
	_, err = e.runs.Begin(t.Context(), b.RowID(), "worker-2")
	assert.ErrorIs(t, err, runs.ErrLeased, "the account is taken")

	// worker-1 dies. Six minutes later its lease is gone.
	e.clock.Advance(6 * time.Minute)
	interrupted, err := e.runs.RecoverInterrupted(t.Context())
	require.NoError(t, err)
	require.Len(t, interrupted, 1)
	assert.Equal(t, a.ID, interrupted[0].ID)
	got, err := e.runs.Get(t.Context(), coretest.Viewer, a.ID)
	require.NoError(t, err)
	assert.Equal(t, runs.StateInterrupted, got.State)
	assert.Contains(t, *got.Error, "worker stopped")

	// The dead worker's late writes are refused.
	assert.ErrorIs(t, ha.Progress(t.Context(), map[string]int{"pages": 9}, nil), runs.ErrLeased)
	assert.ErrorIs(t, ha.Finish(t.Context(), runs.Completion{State: runs.StateSucceeded, Completeness: runs.CompletenessComplete}), runs.ErrLeased)

	// Now b can take the account, and a can be retried afterwards.
	hb, err := e.runs.Begin(t.Context(), b.RowID(), "worker-2")
	require.NoError(t, err)
	require.NoError(t, hb.Finish(t.Context(), runs.Completion{State: runs.StateSucceeded, Completeness: runs.CompletenessComplete}))
	retry, created, err := e.runs.Start(t.Context(), coretest.Marketer, runs.StartInput{Kind: "ghl.import", OccurrenceKey: "a", AccountID: acct})
	require.NoError(t, err)
	assert.True(t, created)
	h2, err := e.runs.Begin(t.Context(), retry.RowID(), "worker-3")
	require.NoError(t, err)
	assert.Equal(t, int64(1), h2.Run.Attempt)
	require.NoError(t, h2.Finish(t.Context(), runs.Completion{State: runs.StateSucceeded, Completeness: runs.CompletenessComplete}))
}

// Catches: a run from one workspace visible from another.
func TestRunsAreScopedToTheWorkspace(t *testing.T) {
	e := setup(t)
	run, _, err := e.runs.Start(t.Context(), coretest.Marketer, runs.StartInput{Kind: "k", OccurrenceKey: "o"})
	require.NoError(t, err)
	other := coretest.Viewer
	other.WorkspaceID = 2
	_, err = e.runs.Get(t.Context(), other, run.ID)
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindNotFound, ce.Kind)
	list, err := e.runs.List(t.Context(), coretest.Viewer, 10)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	_, err = e.runs.List(t.Context(), core.Actor{}, 10)
	assert.ErrorIs(t, err, core.ErrForbidden)
}
