package evaluation_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/assistant/assistanttest"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evaluation"
	"github.com/WokeWorks/adx/server/core/voice"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// scripted answers each replayed transcript with the outcome its text
// names, offset by how many it should get wrong.
type scripted struct {
	name  string
	wrong int
	seen  int
}

func (m *scripted) Name() string { return m.name }
func (m *scripted) Complete(_ context.Context, req assistant.Request) (assistant.Response, error) {
	m.seen++
	text := req.Messages[0].Content[0].Text
	outcome := "interested"
	if strings.Contains(text, "no thanks") {
		outcome = "not_interested"
	}
	if m.seen <= m.wrong {
		outcome = "callback"
	}
	return assistanttest.ToolCall("t1", "record_qualification", map[string]any{"summary": "s", "outcome": outcome, "answers": map[string]any{}, "objections": []any{}, "requestedHuman": false, "callbackAt": "", "confidence": "high"}), nil
}

func transcriptSession(t *testing.T, db *store.DB, at time.Time, n int, said string) int64 {
	t.Helper()
	call := session(t, db, at, n, "interested", "auto")
	segs, _ := json.Marshal([]voice.Segment{{Speaker: "customer", Text: said}})
	var sessionID int64
	require.NoError(t, db.Read.QueryRowContext(t.Context(), "SELECT id FROM call_sessions WHERE call_id = ?", call).Scan(&sessionID))
	require.NoError(t, db.Writer().FinishCallSession(t.Context(), store.FinishCallSessionParams{State: "ended", EndedBy: "agent", Transcript: string(segs), EndedAt: sql.NullTime{Time: at, Valid: true}, UpdatedAt: at, ID: sessionID}))
	return call
}

// Catches: a candidate promoted on too few labels, below the required
// agreement, or below the approved release; a promotion by anyone but a
// person who is an admin; a replay changing any call; the running model
// shown as approved when it is not.
func TestReleaseRegressionIsNeverPromoted(t *testing.T) {
	db := storetest.Open(t)
	coretest.SeedUsers(t, db)
	clock := coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))
	models := map[string]*scripted{"good": {name: "good"}, "worse": {name: "worse", wrong: 2}, "bad": {name: "bad", wrong: 3}}
	ev := evaluation.New(db, evaluation.Config{Clock: clock, Requirements: evaluation.Requirements{MinLabels: 3, MinAgreement: 0.6},
		Running: "worse", Models: func(name string) (assistant.Model, error) {
			m, ok := models[name]
			if !ok {
				return nil, errors.New("unknown model")
			}
			return m, nil
		}})
	ctx := t.Context()
	for i, said := range []string{"yes I want the two bedroom", "no thanks", "yes send me the brochure", "yes"} {
		call := transcriptSession(t, db, clock.Now(), i, said)
		outcome := "interested"
		if said == "no thanks" {
			outcome = "not_interested"
		}
		_, err := ev.Label(ctx, coretest.Marketer, evaluation.LabelInput{CallID: call, Outcome: outcome})
		require.NoError(t, err)
	}
	session(t, db, clock.Now(), 9, "interested", "auto") // unlabelled: never replayed

	r, err := ev.Releases(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.Nil(t, r.Approved)
	assert.Contains(t, r.Warning, "no release is approved")

	_, err = ev.EvaluateRelease(ctx, coretest.Viewer, "good")
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = ev.EvaluateRelease(ctx, coretest.Marketer, "nope")
	require.Error(t, err)
	good, err := ev.EvaluateRelease(ctx, coretest.Marketer, "good")
	require.NoError(t, err)
	assert.Equal(t, "candidate", good.State)
	assert.Equal(t, 4, good.Labels)
	assert.Equal(t, 4, good.Agreements)
	assert.Equal(t, 1.0, *good.Agreement)
	assert.Equal(t, 4, models["good"].seen, "only labelled calls with transcripts are replayed")
	var changed int
	require.NoError(t, db.Read.QueryRowContext(ctx, "SELECT count(*) FROM call_sessions WHERE outcome <> 'interested' OR review_status <> 'auto'").Scan(&changed))
	assert.Zero(t, changed, "a replay writes nothing to any call")

	_, err = ev.Promote(ctx, coretest.Marketer, "good")
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = ev.Promote(ctx, core.SystemIn(core.ViaJob, 1), "good")
	assert.ErrorIs(t, err, core.ErrForbidden, "a job never promotes")
	approved, err := ev.Promote(ctx, coretest.Admin, "good")
	require.NoError(t, err)
	assert.Equal(t, "approved", approved.State)

	worse, err := ev.EvaluateRelease(ctx, coretest.Marketer, "worse")
	require.NoError(t, err)
	assert.Equal(t, 0.5, *worse.Agreement)
	_, err = ev.Promote(ctx, coretest.Admin, "worse")
	requireKind(t, err, core.KindConflict)
	assert.Contains(t, err.Error(), "at least 0.60")
	bad, err := ev.EvaluateRelease(ctx, coretest.Marketer, "bad")
	require.NoError(t, err)
	assert.Equal(t, 0.25, *bad.Agreement)

	// A candidate above the floor but below the approved release is a
	// regression and stays a candidate.
	models["worse"].seen, models["worse"].wrong = 0, 1
	worse, err = ev.EvaluateRelease(ctx, coretest.Marketer, "worse")
	require.NoError(t, err)
	assert.Equal(t, 0.75, *worse.Agreement)
	_, err = ev.Promote(ctx, coretest.Admin, "worse")
	requireKind(t, err, core.KindConflict)
	assert.Contains(t, err.Error(), "regression")

	r, err = ev.Releases(ctx, coretest.Viewer)
	require.NoError(t, err)
	require.NotNil(t, r.Approved)
	assert.Equal(t, "good", r.Approved.Model)
	assert.Contains(t, r.Warning, "running model worse is not the approved release good")
	rejected, err := ev.Reject(ctx, coretest.Admin, "bad")
	require.NoError(t, err)
	assert.Equal(t, "rejected", rejected.State)
	_, err = ev.Promote(ctx, coretest.Admin, "bad")
	requireKind(t, err, core.KindConflict)

	// Too few labels: a fresh workspace-like state is simulated by a strict
	// requirement.
	strict := evaluation.New(db, evaluation.Config{Clock: clock, Requirements: evaluation.Requirements{MinLabels: 50, MinAgreement: 0.6}, Models: func(name string) (assistant.Model, error) { return models[name], nil }})
	_, err = strict.EvaluateRelease(ctx, coretest.Marketer, "good")
	require.NoError(t, err)
	_, err = strict.Promote(ctx, coretest.Admin, "good")
	requireKind(t, err, core.KindConflict)
	assert.Contains(t, err.Error(), "50 are needed")
}

func requireKind(t *testing.T, err error, kind core.Kind) {
	t.Helper()
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, kind, ce.Kind)
}
