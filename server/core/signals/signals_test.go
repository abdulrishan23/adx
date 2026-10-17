package signals_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db    *store.DB
	clock *coretest.Clock
	crm   *crm.CRM
	sig   *signals.Signals
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))}
	coretest.SeedUsers(t, e.db)
	ev := evidence.New(e.db, e.clock)
	e.crm = crm.New(e.db, ev, crm.Config{Clock: e.clock})
	e.sig = signals.New(e.db, ev, e.crm, signals.Config{Clock: e.clock})
	return e
}

func (e *env) person(t *testing.T, name, phone, email string) int64 {
	t.Helper()
	d, err := e.crm.Create(t.Context(), coretest.Marketer, crm.PersonInput{DisplayName: name, Phone: phone, Email: email})
	require.NoError(t, err)
	return d.Person.ID
}

func configured(t *testing.T, e *env, stages *signals.StageCriteria) signals.ScoringVersion {
	t.Helper()
	doc := signals.SuggestedRules()
	doc.Stages = stages
	r, err := e.sig.PutRules(t.Context(), coretest.Admin, doc)
	require.NoError(t, err)
	return r
}

func ingest(t *testing.T, e *env, kind, key string, id signals.SignalIdentity, at time.Time) signals.SignalIngested {
	t.Helper()
	r, err := e.sig.Ingest(t.Context(), coretest.Marketer, signals.SignalInput{Source: "test", Kind: kind, OccurrenceKey: key, OccurredAt: at, Identity: id})
	require.NoError(t, err)
	return r
}

// Catches: a webhook delivered twice counting twice, and a duplicate
// triggering a second assessment.
func TestRepeatedWebhookScoresOnce(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	configured(t, e, nil)
	amal := e.person(t, "Amal Haddad", "0501234567", "amal@example.com")
	first := ingest(t, e, "form_submit", "wh-1", signals.SignalIdentity{Email: "AMAL@example.com"}, e.clock.Now())
	assert.False(t, first.Duplicate)
	assert.True(t, first.Scored)
	assert.Equal(t, "confirmed", first.Event.Confidence)
	again := ingest(t, e, "form_submit", "wh-1", signals.SignalIdentity{Email: "amal@example.com"}, e.clock.Now())
	assert.True(t, again.Duplicate)
	assert.False(t, again.Scored)
	assert.Equal(t, first.Event.ID, again.Event.ID)
	v, err := e.sig.Score(ctx, coretest.Viewer, amal)
	require.NoError(t, err)
	require.Len(t, v.History, 1, "one assessment, not two")
	require.NotNil(t, v.Latest.Intent)
	assert.Equal(t, int64(20), *v.Latest.Intent)
	assert.Len(t, v.Events, 1)
	require.Len(t, v.Latest.Explanation.Intent, 1)
	assert.Equal(t, 1, v.Latest.Explanation.Intent[0].Events)

	// A second distinct form is damped, not doubled; the cap holds a third.
	ingest(t, e, "form_submit", "wh-2", signals.SignalIdentity{Phone: "+971 50 123 4567"}, e.clock.Now())
	ingest(t, e, "form_submit", "wh-3", signals.SignalIdentity{Phone: "0501234567"}, e.clock.Now())
	v, _ = e.sig.Score(ctx, coretest.Viewer, amal)
	assert.Equal(t, int64(39), *v.Latest.Intent, "20 + 12 + 7.2 = 39.2, under the cap of 40")
	ingest(t, e, "form_submit", "wh-4", signals.SignalIdentity{Phone: "0501234567"}, e.clock.Now())
	v, _ = e.sig.Score(ctx, coretest.Viewer, amal)
	assert.Equal(t, int64(40), *v.Latest.Intent, "capped")
	assert.Equal(t, 40, v.Latest.Explanation.Intent[0].Cap)
	assert.Nil(t, v.Latest.Fit, "no fit attributes: not available, not zero")
	assert.False(t, v.Latest.Explanation.FitAvailable)
}

// Catches: an anonymous browser id becoming a confident signal, and
// identifiers that disagree being resolved by guessing.
func TestUnknownIdentityDoesNotScore(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	configured(t, e, nil)
	amal := e.person(t, "Amal Haddad", "0501234567", "amal@example.com")
	omar := e.person(t, "Omar Haddad", "0507654321", "omar@example.com")
	anon := ingest(t, e, "page_view", "pv-1", signals.SignalIdentity{AnonymousID: "device-abc"}, e.clock.Now())
	assert.False(t, anon.Scored)
	assert.Equal(t, "unknown", anon.Event.Confidence)
	assert.Nil(t, anon.Event.PersonID)
	assert.Contains(t, string(anon.Event.Identity), "not an identity")
	mixed := ingest(t, e, "page_view", "pv-2", signals.SignalIdentity{Email: "amal@example.com", Phone: "0507654321"}, e.clock.Now())
	assert.Equal(t, "unknown", mixed.Event.Confidence, "Amal's email and Omar's phone is not one person")
	assert.Contains(t, string(mixed.Event.Identity), "different people")
	unknown, err := e.sig.Unknown(ctx, coretest.Viewer, 10)
	require.NoError(t, err)
	assert.Len(t, unknown, 2, "uncertainty is visible")
	for _, id := range []int64{amal, omar} {
		v, _ := e.sig.Score(ctx, coretest.Viewer, id)
		assert.Nil(t, v.Latest, "nobody scored from unknown events")
	}
	// A marketer attributes the page view by hand; only then does it count.
	_, err = e.sig.Correct(ctx, coretest.Marketer, anon.Event.ID, "reattribute", amal, "same session as the form, confirmed on the call")
	require.NoError(t, err)
	v, _ := e.sig.Score(ctx, coretest.Viewer, amal)
	require.NotNil(t, v.Latest)
	assert.Equal(t, int64(5), *v.Latest.Intent)
	assert.Equal(t, "correction", v.Latest.Trigger)
	unknown, _ = e.sig.Unknown(ctx, coretest.Viewer, 10)
	assert.Len(t, unknown, 1)
	// A shared phone is ambiguous: the event is unknown, not assigned.
	_, err = e.crm.Create(ctx, coretest.Marketer, crm.PersonInput{DisplayName: "Layla Haddad", Phone: "0501234567"})
	require.NoError(t, err)
	shared := ingest(t, e, "whatsapp_tap", "wa-1", signals.SignalIdentity{Phone: "0501234567"}, e.clock.Now())
	assert.Equal(t, "unknown", shared.Event.Confidence)
	assert.Contains(t, string(shared.Event.Identity), "shared by 2 people")
}

// Catches: a temperature being assigned with no thresholds, and rules
// scoring with no rules at all.
func TestUnconfiguredCriteriaClassifyNobody(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	amal := e.person(t, "Amal Haddad", "0501234567", "")
	r := ingest(t, e, "form_submit", "f-1", signals.SignalIdentity{Phone: "0501234567"}, e.clock.Now())
	assert.True(t, r.Scored, "an assessment is recorded so the history says rules were missing")
	v, _ := e.sig.Score(ctx, coretest.Viewer, amal)
	require.NotNil(t, v.Latest)
	assert.Nil(t, v.Latest.Intent)
	assert.Nil(t, v.Latest.Fit)
	assert.Nil(t, v.Latest.Temperature)
	assert.Contains(t, v.Latest.Explanation.Unconfigured, "scoring rules")
	assert.False(t, v.Rules.Configured)

	configured(t, e, nil)
	_, err := e.sig.Assess(ctx, coretest.Marketer, amal, "manual")
	require.NoError(t, err)
	v, _ = e.sig.Score(ctx, coretest.Viewer, amal)
	assert.Equal(t, int64(20), *v.Latest.Intent)
	assert.Nil(t, v.Latest.Temperature, "stage criteria unset: no hot, warm or cold")
	assert.Contains(t, v.Latest.Explanation.Unconfigured, "stage criteria")

	_, err = e.sig.PutRules(ctx, coretest.Admin, signals.ScoringDocument{Stages: &signals.StageCriteria{Hot: 10, Warm: 20}})
	require.Error(t, err, "hot must be above warm")
	_, err = e.sig.PutRules(ctx, coretest.Admin, signals.ScoringDocument{Intent: []signals.IntentRule{{Kind: "email_open", Points: 1, Cap: 1, HalfLifeDays: 1, RepeatFactor: 1}}})
	require.Error(t, err, "unobservable kinds can't be scored")
	_, err = e.sig.PutRules(ctx, coretest.Marketer, signals.SuggestedRules())
	assert.ErrorIs(t, err, core.ErrForbidden)

	rules := configured(t, e, &signals.StageCriteria{Hot: 50, Warm: 20})
	assert.Equal(t, int64(2), rules.Version)
	_, err = e.sig.Assess(ctx, coretest.Marketer, amal, "manual")
	require.NoError(t, err)
	v, _ = e.sig.Score(ctx, coretest.Viewer, amal)
	assert.Equal(t, "warm", *v.Latest.Temperature)
	assert.Equal(t, int64(2), *v.Latest.RulesVersion)
	assert.Equal(t, int64(1), *v.History[1].RulesVersion, "the earlier assessment keeps the version it used")

	// Fit adds to intent; it never multiplies a missing fit to zero.
	_, err = e.crm.SetStated(ctx, coretest.Marketer, amal, "purchase_timeline", "now", "call")
	require.NoError(t, err)
	_, err = e.sig.Assess(ctx, coretest.Marketer, amal, "manual")
	require.NoError(t, err)
	v, _ = e.sig.Score(ctx, coretest.Viewer, amal)
	assert.Equal(t, int64(30), *v.Latest.Fit)
	assert.Equal(t, "hot", *v.Latest.Temperature, "20 + 30 crosses 50")
	assert.Equal(t, "stated", v.Latest.Explanation.Fit[0].Class)
	assert.Equal(t, "warm", *v.Latest.Explanation.PreviousTemp)
}

// Catches: a correction that leaves the old score standing, a reattributed
// event scoring on both people, and decay not showing without a new event.
func TestCorrectionsReassessWithHistory(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	configured(t, e, nil)
	amal := e.person(t, "Amal Haddad", "0501234567", "")
	omar := e.person(t, "Omar Haddad", "0507654321", "")
	ev := ingest(t, e, "call_outcome", "call-1", signals.SignalIdentity{PersonID: amal}, e.clock.Now())
	v, _ := e.sig.Score(ctx, coretest.Viewer, amal)
	assert.Equal(t, int64(25), *v.Latest.Intent)

	_, err := e.sig.Correct(ctx, coretest.Marketer, ev.Event.ID, "reattribute", omar, "")
	require.Error(t, err, "reason required")
	_, err = e.sig.Correct(ctx, coretest.Viewer, ev.Event.ID, "reattribute", omar, "x")
	assert.ErrorIs(t, err, core.ErrForbidden)
	c, err := e.sig.Correct(ctx, coretest.Marketer, ev.Event.ID, "reattribute", omar, "the call was with Omar")
	require.NoError(t, err)
	assert.Equal(t, amal, *c.FromPersonID)
	assert.Equal(t, omar, *c.ToPersonID)
	v, _ = e.sig.Score(ctx, coretest.Viewer, amal)
	assert.Equal(t, int64(0), *v.Latest.Intent)
	assert.Len(t, v.History, 2, "the earlier 25 stays in history")
	assert.Equal(t, int64(25), *v.History[1].Intent)
	assert.Empty(t, v.Events, "the event moved")
	ov, _ := e.sig.Score(ctx, coretest.Viewer, omar)
	assert.Equal(t, int64(25), *ov.Latest.Intent)
	require.Len(t, ov.Corrections, 1)
	assert.Equal(t, "reattribute", ov.Corrections[0].Action)

	// Discard with a reason: the event stays on the timeline, excluded.
	_, err = e.sig.Correct(ctx, coretest.Marketer, ev.Event.ID, "discard", 0, "test call by the team")
	require.NoError(t, err)
	ov, _ = e.sig.Score(ctx, coretest.Viewer, omar)
	assert.Equal(t, int64(0), *ov.Latest.Intent)
	require.Len(t, ov.Events, 1)
	assert.NotNil(t, ov.Events[0].DiscardedAt)
	require.Len(t, ov.Latest.Explanation.Excluded, 1)
	assert.Contains(t, ov.Latest.Explanation.Excluded[0].Reason, "discarded")
	_, err = e.sig.Correct(ctx, coretest.Marketer, ev.Event.ID, "discard", 0, "again")
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)

	// Decay: three weeks later the daily re-assessment halves a form's points.
	ingest(t, e, "form_submit", "f-1", signals.SignalIdentity{PersonID: amal}, e.clock.Now())
	e.clock.Advance(14 * 24 * time.Hour)
	n, err := e.sig.Reassess(ctx, coretest.Marketer, 90*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	v, _ = e.sig.Score(ctx, coretest.Viewer, amal)
	assert.Equal(t, int64(10), *v.Latest.Intent, "20 × 0.5 after one half-life")
	assert.Equal(t, "decay", v.Latest.Trigger)
}

// Catches: derived events duplicating across runs, a test submission
// becoming a signal, and an unchanged CRM status counted as a change.
func TestDeriveFromLinkedRecordsIsIdempotent(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	configured(t, e, nil)
	cred, err := e.db.Writer().UpsertCredential(ctx, store.UpsertCredentialParams{Provider: "gohighlevel", Kind: "private_token", Subject: "loc", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "x", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(ctx, store.UpsertAccountParams{Type: "gohighlevel", ExternalID: "loc-1", CredentialID: cred.ID, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	lead, err := e.db.Writer().UpsertLead(ctx, store.UpsertLeadParams{WorkspaceID: 1, AccountID: acct.ID, ExternalID: "c1", FirstName: ns("Amal"), LastName: ns("Haddad"), Phone: ns("+971501234567"), PhoneE164: ns("+971501234567"), StatusRaw: ns("New"), StatusObservedAt: sql.NullTime{Time: e.clock.Now(), Valid: true}, FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	_, err = e.crm.Sync(ctx, coretest.Marketer)
	require.NoError(t, err)
	n, err := e.sig.Derive(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the first status is recorded as the starting stage")
	n, _ = e.sig.Derive(ctx, coretest.Marketer)
	assert.Equal(t, 0, n, "nothing changed")
	// The import re-observes the same status later: still no change.
	e.clock.Advance(time.Hour)
	_, err = e.db.Writer().UpsertLead(ctx, store.UpsertLeadParams{WorkspaceID: 1, AccountID: acct.ID, ExternalID: "c1", FirstName: ns("Amal"), LastName: ns("Haddad"), Phone: ns("+971501234567"), PhoneE164: ns("+971501234567"), StatusRaw: ns("new"), StatusObservedAt: sql.NullTime{Time: e.clock.Now(), Valid: true}, FirstSeenAt: lead.FirstSeenAt, LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	n, _ = e.sig.Derive(ctx, coretest.Marketer)
	assert.Equal(t, 0, n)
	// A move to follow-up is one change.
	e.clock.Advance(time.Hour)
	_, err = e.db.Writer().UpsertLead(ctx, store.UpsertLeadParams{WorkspaceID: 1, AccountID: acct.ID, ExternalID: "c1", FirstName: ns("Amal"), LastName: ns("Haddad"), Phone: ns("+971501234567"), PhoneE164: ns("+971501234567"), StatusRaw: ns("Follow Up"), StatusObservedAt: sql.NullTime{Time: e.clock.Now(), Valid: true}, FirstSeenAt: lead.FirstSeenAt, LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	n, _ = e.sig.Derive(ctx, coretest.Marketer)
	assert.Equal(t, 1, n)
	people, _ := e.crm.List(ctx, coretest.Viewer, "Amal", 10)
	require.Len(t, people, 1)
	v, _ := e.sig.Score(ctx, coretest.Viewer, people[0].ID)
	require.Len(t, v.Events, 2)
	var p struct {
		Status, Previous string
	}
	require.NoError(t, json.Unmarshal(v.Events[0].Payload, &p))
	assert.Equal(t, "follow up", p.Status)
	assert.Equal(t, "new", p.Previous)
	assert.Equal(t, int64(17), *v.Latest.Intent, "10 + 7 for two stage changes")
}

func ns(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

var _ = context.Background
