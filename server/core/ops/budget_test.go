package ops_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/assistant/assistanttest"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/ops"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: the cap not counting across calls, a call going through once the
// cap is reached, the refusal not being visible in the cost view, and the
// cap not resetting on the next day.
func TestMeterEnforcesTheServerCapAcrossCallsAndDays(t *testing.T) {
	db := storetest.Open(t)
	coretest.SeedUsers(t, db)
	clock := coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))
	fake := &assistanttest.Model{}
	for range 5 {
		r := assistanttest.Text("ok")
		r.Usage = assistant.Usage{InputTokens: 400, OutputTokens: 100}
		fake.Responses = append(fake.Responses, r)
	}
	meter := ops.NewMeter(db, fake, 1000, clock.Now)
	o := ops.New(slog.Default(), db).WithBudgets(ops.Budgets{ModelDailyTokenCap: 1000, AssistantDailyTokens: 50}, "claude-opus-5-5")
	ctx := t.Context()

	_, err := meter.Complete(ctx, assistant.Request{})
	require.NoError(t, err)
	_, err = meter.Complete(ctx, assistant.Request{})
	require.NoError(t, err, "second call brings the day to exactly the cap")
	_, err = meter.Complete(ctx, assistant.Request{})
	require.Error(t, err, "the third call is refused")
	var me *assistant.ModelError
	require.True(t, errors.As(err, &me))
	assert.False(t, me.Retryable, "a refusal is not retried")
	assert.Contains(t, me.Safe, "daily model cap")
	assert.Len(t, fake.Responses, 3, "the model was never called for the refused request")

	cost, err := o.Cost(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.Equal(t, int64(800), cost.Server.InputTokens)
	assert.Equal(t, int64(200), cost.Server.OutputTokens)
	assert.Equal(t, int64(2), cost.Server.Calls)
	assert.Equal(t, int64(1), cost.Server.Refused)
	assert.True(t, cost.Server.Exhausted)
	assert.Equal(t, int64(0), *cost.Server.Remaining)
	assert.Nil(t, cost.Workspace.CostMicros, "no price configured, no cost figure")
	assert.Nil(t, cost.Load, "no load measurement was ever recorded")
	assert.Equal(t, "claude-opus-5-5", cost.Running)

	clock.Advance(24 * time.Hour)
	_, err = meter.Complete(ctx, assistant.Request{})
	require.NoError(t, err, "a new day starts a new count")
	_, err = o.Cost(ctx, core.Actor{Role: core.RoleViewer, Via: core.ViaWeb})
	assert.ErrorIs(t, err, core.ErrForbidden)
}

// Catches: a measurement without conditions being recorded, a number quoted
// when nothing was measured, and the harness not counting every operation.
func TestLoadMeasurementsCarryTheirConditions(t *testing.T) {
	db := storetest.Open(t)
	coretest.SeedUsers(t, db)
	o := ops.New(slog.Default(), db)
	ctx := t.Context()
	none, err := o.LatestLoad(ctx)
	require.NoError(t, err)
	assert.Nil(t, none)

	var calls int64
	m, err := ops.RunLoad(ctx, ops.LoadPlan{Duration: 150 * time.Millisecond, Concurrency: 3, Notes: "unit", Ops: []ops.LoadOp{
		{Name: "ok", Run: func(context.Context) error { calls++; time.Sleep(time.Millisecond); return nil }},
		{Name: "fail", Run: func(context.Context) error { time.Sleep(time.Millisecond); return errors.New("x") }},
	}})
	require.NoError(t, err)
	assert.Greater(t, m.Operations, int64(10))
	assert.Equal(t, m.Operations, m.Mix["ok"]+m.Mix["fail"])
	assert.Equal(t, m.Errors, m.Mix["fail"])
	assert.NotEmpty(t, m.Host)
	assert.Greater(t, m.CPUs, 0)
	assert.Greater(t, m.P95Ms, 0.0)
	assert.GreaterOrEqual(t, m.MaxMs, m.P95Ms)
	assert.Greater(t, m.OpsPerSecond, 0.0)

	_, err = o.RecordLoad(ctx, coretest.Marketer, m)
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = o.RecordLoad(ctx, coretest.Admin, ops.LoadMeasurement{Operations: 5})
	require.Error(t, err, "no conditions, no measurement")
	saved, err := o.RecordLoad(ctx, coretest.Admin, m)
	require.NoError(t, err)
	assert.Equal(t, 3, saved.Concurrency)
	latest, err := o.LatestLoad(ctx)
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, m.Operations, latest.Operations)
	assert.Equal(t, "unit", latest.Notes)
	_, err = ops.RunLoad(ctx, ops.LoadPlan{})
	require.Error(t, err)
}
