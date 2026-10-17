package evaluation_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evaluation"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// session writes a finished call and its session directly: the evaluation
// reads records, it does not run calls.
func session(t *testing.T, db *store.DB, at time.Time, n int, outcome, review string) int64 {
	t.Helper()
	ctx := t.Context()
	p, err := db.Writer().CreatePerson(ctx, store.CreatePersonParams{WorkspaceID: 1, DisplayName: "P", CreatedAt: at, UpdatedAt: at})
	require.NoError(t, err)
	enq, err := db.Writer().CreateEnquiry(ctx, store.CreateEnquiryParams{WorkspaceID: 1, PersonID: p.ID, SourceKind: "lead", SourceRef: "x", FirstContactAt: at, LastActivityAt: at, CreatedAt: at})
	require.NoError(t, err)
	c, err := db.Writer().CreateCall(ctx, store.CreateCallParams{WorkspaceID: 1, PersonID: p.ID, EnquiryID: enq.ID, OccurrenceKey: "k" + string(rune('a'+n)), Mode: "qualification", ToNumber: "+971500000000", FromNumber: "+971800", Provider: "fake", PolicyVersion: 1, ConsentBasis: "c", Attempt: 1, CreatedAt: at, UpdatedAt: at})
	require.NoError(t, err)
	s, err := db.Writer().CreateCallSession(ctx, store.CreateCallSessionParams{WorkspaceID: 1, CallID: c.ID, SpeechProvider: "fake", StartedAt: at, UpdatedAt: at})
	require.NoError(t, err)
	require.NoError(t, db.Writer().FinishCallSession(ctx, store.FinishCallSessionParams{State: "ended", EndedBy: "agent", Transcript: "[]", EndedAt: sql.NullTime{Time: at, Valid: true}, UpdatedAt: at, ID: s.ID}))
	require.NoError(t, db.Writer().SetCallSessionReading(ctx, store.SetCallSessionReadingParams{Summary: "s", Answers: "{}", Outcome: outcome, Confidence: "high", ReviewStatus: review, UpdatedAt: at, ID: s.ID}))
	return c.ID
}

// Catches: accuracy shown with no labels, a label counted as a correction,
// a predictive claim allowed on a handful of calls, and rates shown as
// zero when nothing exists to divide.
func TestQualityIsMeasuredAgainstLabelsAndPredictiveStaysOff(t *testing.T) {
	db := storetest.Open(t)
	coretest.SeedUsers(t, db)
	clock := coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))
	ev := evaluation.New(db, evaluation.Config{Clock: clock, Requirements: evaluation.Requirements{MinLabels: 3, MinAgreement: 0.8}})
	ctx := t.Context()
	r, err := ev.Report(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.Nil(t, r.Qualification.AgreementRate)
	assert.Nil(t, r.Qualification.CorrectionRate)
	assert.Nil(t, r.Alerts.ResolvedRate)
	assert.False(t, r.Predictive.Enabled)
	assert.Contains(t, r.Predictive.Reason, "0 labelled calls; 3 are needed")
	assert.Contains(t, r.Qualification.Note, "not available")

	c1 := session(t, db, clock.Now(), 0, "interested", "auto")
	c2 := session(t, db, clock.Now(), 1, "not_interested", "auto")
	c3 := session(t, db, clock.Now(), 2, "callback", "pending")
	_, err = ev.Label(ctx, coretest.Viewer, evaluation.LabelInput{CallID: c1, Outcome: "interested"})
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = ev.Label(ctx, coretest.Marketer, evaluation.LabelInput{CallID: 999, Outcome: "interested"})
	require.Error(t, err)
	_, err = ev.Label(ctx, coretest.Marketer, evaluation.LabelInput{CallID: c1, Outcome: "interested", Note: "listened"})
	require.NoError(t, err)
	_, err = ev.Label(ctx, coretest.Marketer, evaluation.LabelInput{CallID: c2, Outcome: "callback", Note: "they asked for Tuesday"})
	require.NoError(t, err)
	r, err = ev.Report(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.Equal(t, 3, r.Qualification.Sessions)
	assert.Equal(t, 2, r.Qualification.Read)
	assert.Equal(t, 1, r.Qualification.Pending)
	assert.Equal(t, 2, r.Qualification.Labeled)
	assert.Equal(t, 1, r.Qualification.Agreements)
	assert.Equal(t, 0.5, *r.Qualification.AgreementRate)
	assert.Equal(t, 0, r.Qualification.Corrections, "a label is not a correction")
	assert.Equal(t, 0.0, *r.Qualification.CorrectionRate)
	assert.False(t, r.Predictive.Enabled)
	assert.Contains(t, r.Predictive.Reason, "2 labelled calls")
	// Enough labels but too little agreement: still off.
	_, err = ev.Label(ctx, coretest.Marketer, evaluation.LabelInput{CallID: c3, Outcome: "interested"})
	require.NoError(t, err)
	r, _ = ev.Report(ctx, coretest.Viewer)
	assert.Equal(t, 3, r.Qualification.Labeled)
	assert.False(t, r.Predictive.Enabled)
	assert.Contains(t, r.Predictive.Reason, "below the required")
	// Relabel c2 to agree; relabel c3 to agree: the gate opens on evidence.
	_, err = ev.Label(ctx, coretest.Marketer, evaluation.LabelInput{CallID: c2, Outcome: "not_interested", Note: "re-listened"})
	require.NoError(t, err)
	_, err = ev.Label(ctx, coretest.Marketer, evaluation.LabelInput{CallID: c3, Outcome: "callback"})
	require.NoError(t, err)
	r, _ = ev.Report(ctx, coretest.Viewer)
	assert.Equal(t, 3, r.Qualification.Labeled, "one label per call")
	assert.Equal(t, 1.0, *r.Qualification.AgreementRate)
	assert.True(t, r.Predictive.Enabled)
	g, err := ev.Predictive(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.True(t, g.Enabled)
	// With the default requirement of 50 labels, three are nowhere near.
	strict := evaluation.New(db, evaluation.Config{Clock: clock})
	r, _ = strict.Report(ctx, coretest.Viewer)
	assert.False(t, r.Predictive.Enabled)
	assert.Equal(t, 50, r.Predictive.Requirements.MinLabels)
}
