package qualification_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/assistant/assistanttest"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/calls"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/qualification"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/core/voice"
	"github.com/WokeWorks/adx/server/core/voice/voicetest"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db      *store.DB
	clock   *coretest.Clock
	crm     *crm.CRM
	brands  *brands.Brands
	signals *signals.Signals
	out     *outreach.Outreach
	calls   *calls.Calls
	tel     *voicetest.Telephony
	speech  *voicetest.Speech
	model   *assistanttest.Model
	qual    *qualification.Qualification
	project int64
}

func setup(t *testing.T, withModel bool) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)), tel: &voicetest.Telephony{Secret: "s3", Known: map[string]voice.StatusEvent{}}, speech: &voicetest.Speech{Codec: voice.MulawCodec}, model: &assistanttest.Model{}}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	ev := evidence.New(e.db, e.clock)
	e.crm = crm.New(e.db, ev, crm.Config{Clock: e.clock})
	e.brands = brands.New(e.db, brands.Config{Clock: e.clock})
	e.signals = signals.New(e.db, ev, e.crm, signals.Config{Clock: e.clock})
	pol := policies.New(e.db)
	e.out = outreach.New(e.db, ev, outreach.Config{Clock: e.clock, Brands: e.brands, Signals: e.signals, CRM: e.crm, Policies: pol})
	e.calls = calls.New(e.db, calls.Config{Clock: e.clock, Telephony: e.tel, Policies: pol, Outreach: e.out, CRM: e.crm, PublicURL: "https://adx.example.com"})
	cfg := qualification.Config{Clock: e.clock, Calls: e.calls, Speech: e.speech, Outreach: e.out, Signals: e.signals, CRM: e.crm, Bridge: voice.BridgeConfig{HangupGrace: 100 * time.Millisecond}, AmbienceGain: 0.2}
	if withModel {
		cfg.Model = e.model
	}
	e.qual = qualification.New(e.db, ev, cfg)
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.project = p.ID
	b, err := e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, e.project, b.ID))
	fc, err := e.brands.AddFact(ctx, coretest.Admin, b.ID, brands.FactInput{ProjectID: e.project, Key: "price.starting", Kind: brands.KindMoney, Value: "150000000", Currency: "AED", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceManual, SourceRef: "list"})
	require.NoError(t, err)
	_, err = e.brands.ApproveFact(ctx, coretest.Admin, fc.Fact.ID)
	require.NoError(t, err)
	doc := signals.SuggestedRules()
	_, err = e.signals.PutRules(ctx, coretest.Admin, doc)
	require.NoError(t, err)
	pdoc := calls.SuggestedPolicy()
	pdoc.Route = calls.Route{Provider: "fake", AuthorizedBy: "test", AuthorizedOn: "2026-10-17"}
	pdoc.FromNumber = "+97180012345"
	pdoc.Recording = calls.RecordingPolicy{Enabled: true, RetentionDays: 30, AccessRole: "admin"}
	_, err = e.calls.PutPolicy(ctx, coretest.Admin, pdoc)
	require.NoError(t, err)
	return e
}

// call creates a person and enquiry and places a call that is in progress.
func (e *env) call(t *testing.T, name, phone string) (int64, int64, calls.Call) {
	t.Helper()
	ctx := t.Context()
	d, err := e.crm.Create(ctx, coretest.Marketer, crm.PersonInput{DisplayName: name, Phone: phone})
	require.NoError(t, err)
	enq, err := e.db.Writer().CreateEnquiry(ctx, store.CreateEnquiryParams{WorkspaceID: 1, PersonID: d.Person.ID, ProjectID: sql.NullInt64{Int64: e.project, Valid: true}, SourceKind: "lead", SourceRef: "x", FirstContactAt: e.clock.Now(), LastActivityAt: e.clock.Now(), CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	c, err := e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq.ID, ConsentBasis: "form"})
	require.NoError(t, err)
	c, err = e.calls.Apply(ctx, voice.StatusEvent{ProviderCallID: c.ProviderCallID, State: voice.CallInProgress, EventID: "1"})
	require.NoError(t, err)
	return d.Person.ID, enq.ID, c
}

// run drives a scripted conversation through the bridge and returns the session.
func (e *env) run(t *testing.T, callID int64, script func(s *voicetest.Session), transport *voicetest.Transport) qualification.CallSession {
	t.Helper()
	done := make(chan struct{})
	var sess qualification.CallSession
	var err error
	go func() {
		defer close(done)
		sess, err = e.qual.Run(t.Context(), callID, transport)
	}()
	var s *voicetest.Session
	before := len(e.speech.Sessions)
	require.Eventually(t, func() bool { s = e.speech.Session(before); return s != nil }, 2*time.Second, 10*time.Millisecond)
	script(s)
	<-done
	require.NoError(t, err)
	return sess
}

func reading(outcome, confidence string, answers map[string]qualification.ReadingAnswer, objections []qualification.HeardObjection) assistant.Response {
	return assistanttest.ToolCall("t1", "record_qualification", qualification.Reading{Summary: "summary", Outcome: outcome, Answers: answers, Objections: objections, Confidence: confidence})
}

// Catches: stale queued speech playing on after a barge-in, the brief
// quoting an unapproved figure, and the transcript not kept as evidence.
func TestInterruptionClearsQueuedSpeechAndTranscriptIsEvidence(t *testing.T) {
	e := setup(t, true)
	ctx := t.Context()
	_, enq, c := e.call(t, "Amal Haddad", "0501234567")
	cfg, _, err := e.qual.Instructions(ctx, coretest.Viewer, enq)
	require.NoError(t, err)
	assert.Contains(t, cfg.Instructions, "price.starting: AED 150000000")
	assert.Contains(t, cfg.Instructions, "Never quote any other price")
	assert.Contains(t, cfg.FirstMessage, "Amal")
	assert.Equal(t, voice.MulawCodec, cfg.Codec)

	e.model.Responses = []assistant.Response{reading("interested", "high", map[string]qualification.ReadingAnswer{"purchase_timeline": {Value: "3_months", Quote: "in about three months"}}, nil)}
	tr := voicetest.NewTransport(voice.MulawCodec)
	sess := e.run(t, c.ID, func(s *voicetest.Session) {
		assert.Contains(t, s.Config.Instructions, "The Wadi")
		s.Emit(voice.SpeechEvent{Kind: voice.KindAudio, Audio: voice.Frame{Codec: voice.MulawCodec, Data: make([]byte, 640)}})
		require.Eventually(t, func() bool { return len(tr.SentFrames()) >= 4 }, 2*time.Second, 10*time.Millisecond)
		s.Emit(voice.SpeechEvent{Kind: voice.KindInterruption})
		require.Eventually(t, func() bool { return tr.ClearedCount() == 1 }, 2*time.Second, 10*time.Millisecond)
		s.Emit(voice.SpeechEvent{Kind: voice.KindTranscript, Speaker: "agent", Text: "Hello Amal"})
		s.Emit(voice.SpeechEvent{Kind: voice.KindTranscript, Speaker: "lead", Text: "in about three months"})
		s.Emit(voice.SpeechEvent{Kind: voice.KindEnd})
	}, tr)
	assert.Equal(t, "ended", sess.State)
	assert.Equal(t, int64(1), sess.Interruptions)
	assert.Len(t, sess.Transcript, 2)
	assert.NotNil(t, sess.TranscriptEvidenceID)
	assert.Equal(t, "auto", sess.ReviewStatus)
	assert.Equal(t, "interested", sess.Outcome)
	assert.Equal(t, []string{"purchase_timeline"}, sess.Applied.Attributes)
	assert.Equal(t, "qualified", sess.Applied.Stage)
	assert.NotNil(t, sess.Applied.SignalEventID)
	// The model saw the transcript, not a summary of it.
	require.Len(t, e.model.Requests, 1)
	assert.Equal(t, "record_qualification", e.model.Requests[0].ToolChoice)
	assert.Contains(t, e.model.Requests[0].Messages[0].Content[0].Text, "lead: in about three months")
	// A second conversation for the same call is refused.
	_, err = e.qual.Run(ctx, c.ID, voicetest.NewTransport(voice.MulawCodec))
	require.Error(t, err)
}

// Catches: a misheard budget written as fact, a low-confidence reading
// changing the stage, and an unreviewed reading scoring.
func TestUncertainFiguresDoNotAssertStageOrFacts(t *testing.T) {
	e := setup(t, true)
	ctx := t.Context()
	person, enq, c := e.call(t, "Amal Haddad", "0501234567")
	e.model.Responses = []assistant.Response{reading("interested", "low", map[string]qualification.ReadingAnswer{"budget": {Value: "1500000", Uncertain: true, Quote: "one point five... or fifteen"}, "purpose": {Value: "live", Quote: "to live in"}}, []qualification.HeardObjection{{Kind: "price", Text: "a bit high"}})}
	sess := e.run(t, c.ID, func(s *voicetest.Session) {
		s.Emit(voice.SpeechEvent{Kind: voice.KindTranscript, Speaker: "lead", Text: "one point five or fifteen"})
		s.Emit(voice.SpeechEvent{Kind: voice.KindEnd})
	}, voicetest.NewTransport(voice.MulawCodec))
	assert.Equal(t, "pending", sess.ReviewStatus)
	assert.Equal(t, "low", sess.Confidence)
	assert.Equal(t, int64(0), sess.AppliedVersion, "nothing applied while pending")
	enqRow, _ := e.db.Reader().GetEnquiry(ctx, store.GetEnquiryParams{ID: enq, WorkspaceID: 1})
	assert.False(t, enqRow.Stage.Valid, "stage untouched")
	d, _ := e.crm.Get(ctx, coretest.Viewer, person)
	assert.Empty(t, d.Attributes, "no fact written from an unconfirmed reading")
	v, _ := e.signals.Score(ctx, coretest.Viewer, person)
	assert.Empty(t, v.Events, "no scoring signal")
	brief, _ := e.out.NextAction(ctx, coretest.Viewer, enq)
	require.Len(t, brief.Objections, 1, "the objection is the customer's words and is kept")
	assert.Contains(t, brief.Objections[0].Source, "call:")

	// A person confirms: the sure answer is written, the uncertain budget is held and named.
	sess, err := e.qual.Confirm(ctx, coretest.Marketer, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "confirmed", sess.ReviewStatus)
	assert.Equal(t, int64(1), sess.AppliedVersion)
	assert.Equal(t, []string{"purpose"}, sess.Applied.Attributes)
	require.Len(t, sess.Applied.Held, 1)
	assert.Contains(t, sess.Applied.Held[0], "budget")
	assert.Equal(t, "qualified", sess.Applied.Stage)
	d, _ = e.crm.Get(ctx, coretest.Viewer, person)
	keys := []string{}
	for _, a := range d.Attributes {
		keys = append(keys, a.Key)
	}
	assert.Equal(t, []string{"purpose"}, keys)
	_, err = e.qual.Confirm(ctx, coretest.Marketer, c.ID)
	require.Error(t, err, "confirmed twice is refused")
}

// Catches: a completion applied twice (two signals, two stage writes), and
// a correction that leaves the old score standing.
func TestRepeatedCompletionAppliesOnceAndCorrectionsReapply(t *testing.T) {
	e := setup(t, true)
	ctx := t.Context()
	person, _, c := e.call(t, "Amal Haddad", "0501234567")
	e.model.Responses = []assistant.Response{reading("interested", "high", nil, nil)}
	sess := e.run(t, c.ID, func(s *voicetest.Session) {
		s.Emit(voice.SpeechEvent{Kind: voice.KindTranscript, Speaker: "lead", Text: "yes interested"})
		s.Emit(voice.SpeechEvent{Kind: voice.KindEnd})
	}, voicetest.NewTransport(voice.MulawCodec))
	assert.Equal(t, int64(1), sess.AppliedVersion)
	v, _ := e.signals.Score(ctx, coretest.Viewer, person)
	require.Len(t, v.Events, 1)
	assert.Equal(t, int64(25), *v.Latest.Intent)
	// The provider's completed callback arriving twice changes nothing here.
	_, err := e.calls.Apply(ctx, voice.StatusEvent{ProviderCallID: c.ProviderCallID, State: voice.CallCompleted, EventID: "9"})
	require.NoError(t, err)
	_, err = e.calls.Apply(ctx, voice.StatusEvent{ProviderCallID: c.ProviderCallID, State: voice.CallCompleted, EventID: "9"})
	require.NoError(t, err)
	_, err = e.qual.Run(ctx, c.ID, voicetest.NewTransport(voice.MulawCodec))
	require.Error(t, err, "a finished call runs no second conversation")
	v, _ = e.signals.Score(ctx, coretest.Viewer, person)
	assert.Len(t, v.Events, 1, "still one scoring event")

	// Correction: the customer was actually asking for a callback.
	_, err = e.qual.Correct(ctx, coretest.Marketer, c.ID, qualification.CorrectionInput{Outcome: "callback", Note: ""})
	require.Error(t, err, "reason required")
	_, err = e.qual.Correct(ctx, coretest.Viewer, c.ID, qualification.CorrectionInput{Outcome: "callback", Note: "x"})
	assert.ErrorIs(t, err, core.ErrForbidden)
	sess, err = e.qual.Correct(ctx, coretest.Marketer, c.ID, qualification.CorrectionInput{Outcome: "callback", Answers: map[string]qualification.ReadingAnswer{"purchase_timeline": {Value: "6_months"}}, Note: "they asked us to call next week"})
	require.NoError(t, err)
	assert.Equal(t, "corrected", sess.ReviewStatus)
	assert.Equal(t, int64(2), sess.AppliedVersion)
	assert.Equal(t, "callback", sess.Applied.Stage)
	v, _ = e.signals.Score(ctx, coretest.Viewer, person)
	require.Len(t, v.Events, 2, "old event kept on the timeline")
	discarded := 0
	for _, ev := range v.Events {
		if ev.DiscardedAt != nil {
			discarded++
		}
	}
	assert.Equal(t, 1, discarded, "the superseded reading no longer scores")
	assert.Equal(t, int64(25), *v.Latest.Intent, "one call outcome counts, not two")
	evd, err := e.qual.Evidence(ctx, coretest.Viewer, c.ID)
	require.NoError(t, err)
	require.Len(t, evd.Corrections, 1)
	assert.Equal(t, "callback", evd.Corrections[0].Outcome)
	assert.Equal(t, "summary", evd.Session.Summary, "the model's summary stays on the session")
}

// Catches: recordings served to a role the policy excludes, served after
// retention deleted them, and readings produced with no model.
func TestRecordingAccessFollowsPolicyAndNoModelWaits(t *testing.T) {
	e := setup(t, false)
	ctx := t.Context()
	_, _, c := e.call(t, "Amal Haddad", "0501234567")
	sess := e.run(t, c.ID, func(s *voicetest.Session) {
		s.Emit(voice.SpeechEvent{Kind: voice.KindTranscript, Speaker: "lead", Text: "hello"})
		s.Emit(voice.SpeechEvent{Kind: voice.KindEnd})
	}, voicetest.NewTransport(voice.MulawCodec))
	assert.Equal(t, "pending", sess.ReviewStatus)
	assert.Contains(t, sess.ReviewReason, "no model")
	assert.Equal(t, "", sess.Outcome)
	_, err := e.qual.Confirm(ctx, coretest.Marketer, c.ID)
	require.Error(t, err, "nothing to confirm without a reading")
	st := e.qual.State()
	assert.False(t, st.ModelReady)
	assert.Equal(t, "fake", st.SpeechAdapter)

	_, err = e.calls.Apply(ctx, voice.StatusEvent{ProviderCallID: c.ProviderCallID, State: voice.CallCompleted, RecordingRef: "RE1", Duration: 90 * time.Second, EventID: "2"})
	require.NoError(t, err)
	ev, err := e.qual.Evidence(ctx, coretest.Marketer, c.ID)
	require.NoError(t, err)
	assert.False(t, ev.RecordingAccess)
	assert.Contains(t, ev.RecordingReason, "admin")
	assert.Empty(t, ev.RecordingRef)
	_, err = e.qual.Recording(ctx, coretest.Marketer, c.ID)
	assert.Error(t, err)
	ref, err := e.qual.Recording(ctx, coretest.Admin, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "RE1", ref)
	e.clock.Advance(31 * 24 * time.Hour)
	_, err = e.calls.ExpireRecordings(ctx, coretest.Marketer)
	require.NoError(t, err)
	ev, _ = e.qual.Evidence(ctx, coretest.Admin, c.ID)
	assert.False(t, ev.RecordingAccess)
	assert.Contains(t, ev.RecordingReason, "retention")
	// A corrected reading still works without a model: the person is the reader.
	sess, err = e.qual.Correct(ctx, coretest.Marketer, c.ID, qualification.CorrectionInput{Outcome: "not_interested", Note: "listened to the recording"})
	require.NoError(t, err)
	assert.Equal(t, "not_interested", sess.Applied.Stage)
}
