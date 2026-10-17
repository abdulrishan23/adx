package learning_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/learning"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db    *store.DB
	clock *coretest.Clock
	ev    *evidence.Store
	l     *learning.Learning
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))}
	coretest.SeedUsers(t, e.db)
	e.ev = evidence.New(e.db, e.clock)
	e.l = learning.New(e.db, learning.Config{Clock: e.clock, Requirements: learning.PromotionRequirements{MinRecords: 2}})
	return e
}

// recordAdopted opens a record for a subject and adopts it at the clock's now.
func (e *env) recordAdopted(t *testing.T, ref, subject string, decision string) learning.LearningRecord {
	t.Helper()
	r, _, err := e.l.Record(t.Context(), coretest.Marketer, learning.LearningRecordInput{RecommendationRef: ref, Source: "agent:budget", SubjectKind: "campaign", SubjectRef: subject, Recommended: json.RawMessage(`{"raise":true}`), WindowDays: 14})
	require.NoError(t, err)
	in := learning.LearningDecisionInput{Decision: decision, Note: "n"}
	if decision != "ignored" {
		in.ActionRef = "intent:" + ref
	}
	r, err = e.l.Decide(t.Context(), coretest.Marketer, r.ID, in)
	require.NoError(t, err)
	return r
}

// Catches: a model's agreement counting as support, a hypothesis promoted
// without observed outcomes, and the model's actor promoting anything.
func TestModelAgreementIsNotFactualValidation(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	asModel := coretest.Marketer
	asModel.Via = core.ViaAssistant
	// A model proposes an insight backed only by its own agreement.
	ins, err := e.l.ProposeInsight(ctx, asModel, learning.MemoryInsightInput{Statement: "Raising budgets on Fridays lowers cost per lead.", Scope: "account:*", Support: []learning.SupportItem{{Kind: "model_agreement", Note: "the model is confident"}, {Kind: "model_agreement", Note: "a second run agreed"}}})
	require.NoError(t, err)
	assert.Equal(t, "model", ins.Source)
	assert.Equal(t, "hypothesis", ins.Class)
	assert.Equal(t, 0, ins.SupportingRecords, "agreement is listed, not counted")
	assert.Len(t, ins.Support, 2)
	_, err = e.l.Promote(ctx, coretest.Marketer, ins.ID, "supported", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model agreement does not count")
	// Two adopted records with observed outcomes make it supported; the
	// model's actor still cannot promote.
	r1 := e.recordAdopted(t, "p-1", "g1", "adopted")
	r2 := e.recordAdopted(t, "p-2", "g2", "modified")
	for _, r := range []learning.LearningRecord{r1, r2} {
		_, err := e.l.Observe(ctx, coretest.Marketer, learning.LearningOutcomeInput{SubjectKind: "campaign", SubjectRef: r.SubjectRef, Metric: "cpl_minor", Value: 9000, ObservedAt: e.clock.Now().Add(72 * time.Hour)})
		require.NoError(t, err)
	}
	ins2, err := e.l.ProposeInsight(ctx, coretest.Marketer, learning.MemoryInsightInput{Statement: "Raising budgets lowers cost per lead.", Support: []learning.SupportItem{{Kind: "record", RecordID: r1.ID}, {Kind: "record", RecordID: r2.ID}, {Kind: "model_agreement"}}})
	require.NoError(t, err)
	assert.Equal(t, "human", ins2.Source)
	assert.Equal(t, 2, ins2.SupportingRecords)
	_, err = e.l.Promote(ctx, asModel, ins2.ID, "supported", 0)
	assert.ErrorIs(t, err, core.ErrForbidden)
	ins2, err = e.l.Promote(ctx, coretest.Marketer, ins2.ID, "supported", 0)
	require.NoError(t, err)
	assert.Equal(t, "supported", ins2.Class)
	assert.True(t, ins2.Attributed)
	assert.False(t, ins2.Causal, "supported is attributed, not causal")
	_, err = e.l.Promote(ctx, coretest.Marketer, ins2.ID, "supported", 0)
	require.Error(t, err, "only a hypothesis becomes supported")
	// An unknown support record is refused at proposal time.
	_, err = e.l.ProposeInsight(ctx, coretest.Marketer, learning.MemoryInsightInput{Statement: "x", Support: []learning.SupportItem{{Kind: "record", RecordID: 999}}})
	require.Error(t, err)
}

// Catches: an outcome landing on the wrong intervention, on ignored advice,
// or on nothing when a record covers it.
func TestDelayedOutcomeAttachesToTheCorrectCohort(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	first := e.recordAdopted(t, "p-1", "g1", "adopted") // acted 1 Oct, window to 15 Oct
	e.clock.Advance(10 * 24 * time.Hour)
	second := e.recordAdopted(t, "p-2", "g1", "adopted") // acted 11 Oct, window to 25 Oct
	ignored := e.recordAdopted(t, "p-3", "g2", "ignored")
	assert.Nil(t, ignored.ActionAt)

	early, err := e.l.Observe(ctx, coretest.Marketer, learning.LearningOutcomeInput{SubjectKind: "campaign", SubjectRef: "g1", Metric: "cpl_minor", Value: 8500, ObservedAt: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	assert.Equal(t, first.ID, *early.RecordID, "5 Oct is inside the first window and before the second action")
	late, err := e.l.Observe(ctx, coretest.Marketer, learning.LearningOutcomeInput{SubjectKind: "campaign", SubjectRef: "g1", Metric: "cpl_minor", Value: 8000, ObservedAt: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	assert.Equal(t, second.ID, *late.RecordID, "13 Oct is in both windows; the most recent intervention takes it")
	after, err := e.l.Observe(ctx, coretest.Marketer, learning.LearningOutcomeInput{SubjectKind: "campaign", SubjectRef: "g1", Metric: "cpl_minor", Value: 7000, ObservedAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	assert.Nil(t, after.RecordID, "outside every window: unattached")
	forIgnored, err := e.l.Observe(ctx, coretest.Marketer, learning.LearningOutcomeInput{SubjectKind: "campaign", SubjectRef: "g2", Metric: "cpl_minor", Value: 7000, ObservedAt: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	assert.Nil(t, forIgnored.RecordID, "ignored advice acted on nothing, so nothing attaches to it")
	got, _ := e.l.Get(ctx, coretest.Viewer, first.ID)
	assert.Len(t, got.Outcomes, 1)
	got, _ = e.l.Get(ctx, coretest.Viewer, second.ID)
	assert.Len(t, got.Outcomes, 1)
	report, err := e.l.Report(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.Equal(t, 4, report.Outcomes)
	assert.Equal(t, 2, report.Unattached)
}

// Catches: ignored advice counted as an intervention, an ignored decision
// carrying an action, and a record decided twice.
func TestIgnoredAdviceIsNotAnExecutedIntervention(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.recordAdopted(t, "p-1", "g1", "adopted")
	e.recordAdopted(t, "p-2", "g2", "modified")
	e.recordAdopted(t, "p-3", "g3", "ignored")
	r, created, err := e.l.Record(ctx, coretest.Marketer, learning.LearningRecordInput{RecommendationRef: "p-4", Source: "alert", SubjectKind: "campaign", SubjectRef: "g4"})
	require.NoError(t, err)
	assert.True(t, created)
	_, created, _ = e.l.Record(ctx, coretest.Marketer, learning.LearningRecordInput{RecommendationRef: "p-4", Source: "alert", SubjectKind: "campaign", SubjectRef: "g4"})
	assert.False(t, created, "one record per recommendation")
	_, err = e.l.Decide(ctx, coretest.Marketer, r.ID, learning.LearningDecisionInput{Decision: "ignored", ActionRef: "intent:9"})
	require.Error(t, err, "ignored advice has no action")
	_, err = e.l.Decide(ctx, coretest.Marketer, r.ID, learning.LearningDecisionInput{Decision: "adopted"})
	require.Error(t, err, "adopted needs the action")
	_, err = e.l.Decide(ctx, coretest.Viewer, r.ID, learning.LearningDecisionInput{Decision: "ignored"})
	assert.ErrorIs(t, err, core.ErrForbidden)
	report, _ := e.l.Report(ctx, coretest.Viewer)
	assert.Equal(t, 4, report.Records)
	assert.Equal(t, 2, report.Interventions, "adopted and modified only")
	assert.Equal(t, 1, report.Ignored)
	assert.Equal(t, 1, report.Decisions["pending"])
	_, err = e.l.Decide(ctx, coretest.Marketer, r.ID, learning.LearningDecisionInput{Decision: "ignored", Note: "not this month"})
	require.NoError(t, err)
	_, err = e.l.Decide(ctx, coretest.Marketer, r.ID, learning.LearningDecisionInput{Decision: "adopted", ActionRef: "x"})
	require.Error(t, err, "decided once")
}

// Catches: a supported insight called causal, validation without a
// controlled comparison, and a revision keeping the old class.
func TestAttributedImprovementIsNotAutomaticallyCausal(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	r1 := e.recordAdopted(t, "p-1", "g1", "adopted")
	r2 := e.recordAdopted(t, "p-2", "g2", "adopted")
	for _, r := range []learning.LearningRecord{r1, r2} {
		_, err := e.l.Observe(ctx, coretest.Marketer, learning.LearningOutcomeInput{SubjectKind: "campaign", SubjectRef: r.SubjectRef, Metric: "leads", Value: 40, ObservedAt: e.clock.Now().Add(48 * time.Hour), Class: "attributed"})
		require.NoError(t, err)
	}
	ins, err := e.l.ProposeInsight(ctx, coretest.Marketer, learning.MemoryInsightInput{Statement: "More budget, more leads.", Support: []learning.SupportItem{{Kind: "record", RecordID: r1.ID}, {Kind: "record", RecordID: r2.ID}}})
	require.NoError(t, err)
	assert.Equal(t, 0, ins.SupportingRecords, "attributed outcomes are not observed outcomes")
	_, err = e.l.Promote(ctx, coretest.Marketer, ins.ID, "supported", 0)
	require.Error(t, err)
	for _, r := range []learning.LearningRecord{r1, r2} {
		_, err := e.l.Observe(ctx, coretest.Marketer, learning.LearningOutcomeInput{SubjectKind: "campaign", SubjectRef: r.SubjectRef, Metric: "leads", Value: 40, ObservedAt: e.clock.Now().Add(72 * time.Hour)})
		require.NoError(t, err)
	}
	ins, err = e.l.Promote(ctx, coretest.Marketer, ins.ID, "supported", 0)
	require.NoError(t, err)
	assert.False(t, ins.Causal)
	_, err = e.l.Promote(ctx, coretest.Marketer, ins.ID, "validated", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not causal lift")
	// Any observed evidence will not do: it must be a controlled comparison.
	plain, _, err := e.ev.Record(ctx, coretest.Marketer, evidence.Observation{Source: "google_ads", Kind: "campaign_day", SourceRef: "g1/2026-10-02", Scope: "account:1", Class: evidence.Observed, Payload: map[string]any{"leads": 40}, Completeness: evidence.Complete})
	require.NoError(t, err)
	_, err = e.l.Promote(ctx, coretest.Marketer, ins.ID, "validated", plain.ID)
	require.Error(t, err)
	control, _, err := e.ev.Record(ctx, coretest.Marketer, evidence.Observation{Source: "adx", Kind: "controlled_comparison", SourceRef: "holdout:g1:2026-10", Scope: "account:1", Class: evidence.Observed, Payload: map[string]any{"treated": 40, "holdout": 25}, Completeness: evidence.Complete})
	require.NoError(t, err)
	ins, err = e.l.Promote(ctx, coretest.Marketer, ins.ID, "validated", control.ID)
	require.NoError(t, err)
	assert.Equal(t, "validated", ins.Class)
	assert.True(t, ins.Causal)
	assert.Equal(t, control.ID, *ins.ControlEvidenceID)
	// Memory feeds validated and supported findings forward with lineage;
	// a revision is a new hypothesis in the same lineage.
	findings, err := e.l.Findings(ctx, coretest.Viewer)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, int64(1), findings[0].Lineage)
	assert.Equal(t, int64(1), findings[0].Version)
	assert.True(t, findings[0].Causal)
	rev, err := e.l.Revise(ctx, coretest.Marketer, ins.ID, "More budget, more leads, within the plan's cap.", "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), rev.Lineage)
	assert.Equal(t, int64(2), rev.Version)
	assert.Equal(t, "hypothesis", rev.Class)
	assert.False(t, rev.Causal)
	findings, _ = e.l.Findings(ctx, coretest.Viewer)
	assert.Empty(t, findings, "the retired version no longer feeds forward; the revision must earn it again")
	all, _ := e.l.ListInsights(ctx, coretest.Viewer)
	assert.Len(t, all, 2)
	report, _ := e.l.Report(ctx, coretest.Viewer)
	assert.Equal(t, 1, report.Insights["hypothesis"])
	assert.Equal(t, 0, report.Insights["validated"])
}
