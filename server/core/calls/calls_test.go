package calls_test

import (
	"database/sql"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/calls"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/voice"
	"github.com/WokeWorks/adx/server/core/voice/voicetest"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db       *store.DB
	clock    *coretest.Clock
	crm      *crm.CRM
	out      *outreach.Outreach
	policies *policies.Policies
	tel      *voicetest.Telephony
	calls    *calls.Calls
	project  int64
}

// 09:00 UTC is 13:00 in Dubai: inside a 09:00-20:00 window.
func setup(t *testing.T, withRoute bool) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)), tel: &voicetest.Telephony{Secret: "s3", Known: map[string]voice.StatusEvent{}}}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	ev := evidence.New(e.db, e.clock)
	e.crm = crm.New(e.db, ev, crm.Config{Clock: e.clock})
	e.policies = policies.New(e.db)
	e.out = outreach.New(e.db, ev, outreach.Config{Clock: e.clock, CRM: e.crm})
	cfg := calls.Config{Clock: e.clock, Policies: e.policies, Outreach: e.out, CRM: e.crm, PublicURL: "https://adx.example.com", UnknownGrace: 2 * time.Minute}
	if withRoute {
		cfg.Telephony = e.tel
	}
	e.calls = calls.New(e.db, cfg)
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.project = p.ID
	return e
}

func (e *env) person(t *testing.T, name, phone string) (int64, int64) {
	t.Helper()
	ctx := t.Context()
	d, err := e.crm.Create(ctx, coretest.Marketer, crm.PersonInput{DisplayName: name, Phone: phone})
	require.NoError(t, err)
	enq, err := e.db.Writer().CreateEnquiry(ctx, store.CreateEnquiryParams{WorkspaceID: 1, PersonID: d.Person.ID, ProjectID: sql.NullInt64{Int64: e.project, Valid: true}, SourceKind: "lead", SourceRef: "x", FirstContactAt: e.clock.Now(), LastActivityAt: e.clock.Now(), CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	return d.Person.ID, enq.ID
}

func authorized() calls.CallPolicyDocument {
	doc := calls.SuggestedPolicy()
	doc.Route = calls.Route{Provider: "fake", AuthorizedBy: "test owner", AuthorizedOn: "2026-10-17", Note: "fake route for tests only"}
	doc.FromNumber = "+97180012345"
	doc.Recording = calls.RecordingPolicy{Enabled: true, RetentionDays: 30, AccessRole: "admin"}
	return doc
}

func (e *env) policy(t *testing.T, doc calls.CallPolicyDocument) {
	t.Helper()
	_, err := e.calls.PutPolicy(t.Context(), coretest.Admin, doc)
	require.NoError(t, err)
}

func (e *env) callback(t *testing.T, providerID, status, seq string, extra url.Values) calls.Call {
	t.Helper()
	form := url.Values{"CallSid": {providerID}, "CallStatus": {status}, "SequenceNumber": {seq}}
	for k, v := range extra {
		form[k] = v
	}
	req := httptest.NewRequest("POST", "/webhooks/voice/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Fake-Signature", "s3")
	c, err := e.calls.Callback(t.Context(), req)
	require.NoError(t, err)
	return c
}

// Catches: two requests for the same enquiry both ringing the phone, a
// third line opening past capacity, and a second attempt while the first is
// still live.
func TestSimultaneousRequestsCannotLaunchDuplicateCall(t *testing.T) {
	e := setup(t, true)
	ctx := t.Context()
	e.policy(t, authorized())
	_, enq := e.person(t, "Amal Haddad", "0501234567")
	var wg sync.WaitGroup
	results := make([]error, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq, ConsentBasis: "form consent 2026-10-10", Reason: "qualify"})
		}(i)
	}
	wg.Wait()
	placed := 0
	for _, err := range results {
		if err == nil {
			placed++
		} else {
			var ce *core.Error
			require.True(t, errors.As(err, &ce), err.Error())
			assert.Equal(t, core.KindConflict, ce.Kind)
		}
	}
	assert.Equal(t, 1, placed, "exactly one call")
	assert.Len(t, e.tel.Calls, 1, "the provider was asked once")
	assert.Equal(t, "wss://adx.example.com/webhooks/voice/stream/1", e.tel.Calls[0].StreamURL)
	assert.Equal(t, "https://adx.example.com/webhooks/voice/status", e.tel.Calls[0].StatusCallbackURL)
	assert.True(t, e.tel.Calls[0].Record)
	list, _ := e.calls.List(ctx, coretest.Viewer, 10)
	require.Len(t, list, 1)
	assert.Equal(t, "queued", list[0].State)
	assert.Equal(t, int64(1), list[0].Attempt)

	// Capacity: a second person fills the second line; a third is refused.
	_, enq2 := e.person(t, "Omar Haddad", "0507654321")
	_, enq3 := e.person(t, "Layla Haddad", "0509999999")
	_, err := e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq2, ConsentBasis: "c"})
	require.NoError(t, err)
	ct, err := e.calls.Contactability(ctx, coretest.Viewer, enq3, "c")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(ct.Blockers, ";"), "at capacity")
	_, err = e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq3, ConsentBasis: "c"})
	require.Error(t, err)
	assert.Len(t, e.tel.Calls, 2)
}

// Catches: a provider error being retried blindly, a reconcile that
// invents a result, and a second attempt before the first is settled.
func TestUnknownResultReconcilesBeforeRepeat(t *testing.T) {
	e := setup(t, true)
	ctx := t.Context()
	e.policy(t, authorized())
	_, enq := e.person(t, "Amal Haddad", "0501234567")
	e.tel.StartErr = errors.New("connection reset")
	c, err := e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq, ConsentBasis: "c"})
	require.NoError(t, err)
	assert.Equal(t, "unknown", c.State)
	assert.NotNil(t, c.UnknownSince)
	assert.Len(t, e.tel.Calls, 1)
	e.tel.StartErr = nil

	// No repeat while unknown.
	_, err = e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq, ConsentBasis: "c"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reconcile")
	assert.Len(t, e.tel.Calls, 1, "nothing re-sent")

	// Within the grace period and not found: still unknown.
	c, err = e.calls.Reconcile(ctx, coretest.Marketer, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "unknown", c.State)
	// The provider did place it after all: its state is applied.
	e.tel.Known[c.OccurrenceKey] = voice.StatusEvent{ProviderCallID: "pc-77", State: voice.CallNoAnswer, At: e.clock.Now()}
	c, err = e.calls.Reconcile(ctx, coretest.Marketer, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "no_answer", c.State)
	assert.Equal(t, "pc-77", c.ProviderCallID)
	assert.NotNil(t, c.EndedAt)
	_, err = e.calls.Reconcile(ctx, coretest.Marketer, c.ID)
	require.Error(t, err, "settled calls are not reconciled again")

	// Now a second attempt is allowed, and counted.
	c2, err := e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq, ConsentBasis: "c"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), c2.Attempt)
	assert.Equal(t, "queued", c2.State)
	// A third today is over the daily limit of two.
	e.callback(t, c2.ProviderCallID, "completed", "3", nil)
	_, err = e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq, ConsentBasis: "c"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attempt")

	// Unknown past the grace period with nothing at the provider: failed.
	_, enq2 := e.person(t, "Omar Haddad", "0507654321")
	e.tel.StartErr = errors.New("timeout")
	c3, err := e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq2, ConsentBasis: "c"})
	require.NoError(t, err)
	e.tel.StartErr = nil
	e.clock.Advance(3 * time.Minute)
	c3, err = e.calls.Reconcile(ctx, coretest.Marketer, c3.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", c3.State)
	assert.Contains(t, c3.Reason, "no record")
}

// Catches: a call going out with no route, with a route nobody authorized,
// with an adapter that is not the named provider, and the fake being
// blocked in tests by the same rule.
func TestNoAuthorizedRouteBlocksLiveCallingNotFakeTests(t *testing.T) {
	e := setup(t, false)
	ctx := t.Context()
	_, enq := e.person(t, "Amal Haddad", "0501234567")
	st, err := e.calls.GetPolicy(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.False(t, st.LiveCalling)
	assert.Contains(t, strings.Join(st.Reasons, ";"), "no calling policy")
	_, err = e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq, ConsentBasis: "c"})
	require.Error(t, err)

	// Policy without a route or authorization, and no adapter.
	doc := calls.SuggestedPolicy()
	doc.FromNumber = "+97180012345"
	e.policy(t, doc)
	st, _ = e.calls.GetPolicy(ctx, coretest.Viewer)
	assert.False(t, st.LiveCalling)
	joined := strings.Join(st.Reasons, ";")
	assert.Contains(t, joined, "no telecom route is chosen")
	assert.Contains(t, joined, "not authorized in writing")
	assert.Contains(t, joined, "no telephony adapter")
	_, err = e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq, ConsentBasis: "c"})
	require.Error(t, err)

	// The fake route, authorized, with the fake adapter: allowed.
	e2 := setup(t, true)
	_, enq2 := e2.person(t, "Amal Haddad", "0501234567")
	e2.policy(t, authorized())
	st, _ = e2.calls.GetPolicy(ctx, coretest.Viewer)
	assert.True(t, st.LiveCalling)
	assert.Equal(t, "fake", st.Adapter)
	_, err = e2.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq2, ConsentBasis: "c"})
	require.NoError(t, err)

	// The policy names twilio but the adapter is the fake: blocked.
	doc = authorized()
	doc.Route.Provider = "twilio"
	e2.policy(t, doc)
	st, _ = e2.calls.GetPolicy(ctx, coretest.Viewer)
	assert.False(t, st.LiveCalling)
	assert.Contains(t, strings.Join(st.Reasons, ";"), "wired adapter is fake but the policy names twilio")
	// Authorization needs both fields; a marketer can't save policy.
	doc = authorized()
	doc.Route.AuthorizedOn = ""
	_, err = e2.calls.PutPolicy(ctx, coretest.Admin, doc)
	require.Error(t, err)
	_, err = e2.calls.PutPolicy(ctx, coretest.Marketer, authorized())
	assert.ErrorIs(t, err, core.ErrForbidden)
}

// Catches: contactability checks being skipped: consent, opt-out, do not
// contact, hours and the kill switch.
func TestContactabilityChecksRunBeforeEveryCall(t *testing.T) {
	e := setup(t, true)
	ctx := t.Context()
	e.policy(t, authorized())
	person, enq := e.person(t, "Amal Haddad", "0501234567")
	ct, err := e.calls.Contactability(ctx, coretest.Viewer, enq, "")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(ct.Blockers, ";"), "consent basis is required")
	_, err = e.out.Suppress(ctx, coretest.Marketer, person, "call", "asked not to be called", "call")
	require.NoError(t, err)
	ct, _ = e.calls.Contactability(ctx, coretest.Viewer, enq, "form")
	assert.Contains(t, strings.Join(ct.Blockers, ";"), "opted out")
	_, err = e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq, ConsentBasis: "form"})
	require.Error(t, err)
	assert.Empty(t, e.tel.Calls)

	_, enq2 := e.person(t, "Omar Haddad", "0507654321")
	e.clock.Set(time.Date(2026, 10, 17, 18, 30, 0, 0, time.UTC)) // 22:30 Dubai
	ct, _ = e.calls.Contactability(ctx, coretest.Viewer, enq2, "form")
	assert.Contains(t, strings.Join(ct.Blockers, ";"), "outside calling hours")
	e.clock.Set(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))
	ct, _ = e.calls.Contactability(ctx, coretest.Viewer, enq2, "form")
	assert.Empty(t, ct.Blockers)

	// Kill switch for the workspace stops calls too.
	doc := policies.SuggestedDefaults()
	pct, perDay := 20, 50
	doc.Caps = policies.PolicyCaps{DailySpendIncreasePct: &pct, MaxChangesPerDay: &perDay}
	_, err = e.policies.Put(ctx, coretest.Admin, doc)
	require.NoError(t, err)
	_, err = e.policies.SetKillSwitch(ctx, coretest.Admin, 0, true, "incident")
	require.NoError(t, err)
	ct, _ = e.calls.Contactability(ctx, coretest.Viewer, enq2, "form")
	assert.Contains(t, strings.Join(ct.Blockers, ";"), "kill switch")
	_, err = e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq2, ConsentBasis: "form"})
	require.Error(t, err)
	assert.Empty(t, e.tel.Calls)
	_, err = e.calls.Request(ctx, coretest.Viewer, calls.CallInput{EnquiryID: enq2, ConsentBasis: "form"})
	assert.ErrorIs(t, err, core.ErrForbidden)
}

// Catches: an unsigned callback changing a call, a redelivered callback
// counted twice, a late event moving a finished call, and recordings kept
// past retention.
func TestInvalidWebhookIsRejectedAndEventsApplyOnce(t *testing.T) {
	e := setup(t, true)
	ctx := t.Context()
	e.policy(t, authorized())
	_, enq := e.person(t, "Amal Haddad", "0501234567")
	c, err := e.calls.Request(ctx, coretest.Marketer, calls.CallInput{EnquiryID: enq, ConsentBasis: "c"})
	require.NoError(t, err)
	form := url.Values{"CallSid": {c.ProviderCallID}, "CallStatus": {"completed"}}
	req := httptest.NewRequest("POST", "/webhooks/voice/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err = e.calls.Callback(ctx, req)
	assert.ErrorIs(t, err, voice.ErrBadSignature)
	d, _ := e.calls.Get(ctx, coretest.Viewer, c.ID)
	assert.Equal(t, "queued", d.Call.State, "unsigned callback changed nothing")
	assert.Len(t, d.Events, 2, "reserved, queued")

	// Ringing, in progress, then completed with a duration and recording.
	c = e.callback(t, c.ProviderCallID, "ringing", "1", nil)
	assert.Equal(t, "ringing", c.State)
	bad := httptest.NewRequest("POST", "/webhooks/voice/status", strings.NewReader(url.Values{"CallSid": {c.ProviderCallID}, "CallStatus": {"in-progress"}, "SequenceNumber": {"2"}}.Encode()))
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	bad.Header.Set("X-Fake-Signature", "s3")
	_, err = e.calls.Callback(ctx, bad)
	require.Error(t, err, "an unknown state spelling is refused, not guessed")
	d, _ = e.calls.Get(ctx, coretest.Viewer, c.ID)
	assert.Equal(t, "ringing", d.Call.State)
	c = e.callback(t, c.ProviderCallID, "in_progress", "2", nil)
	assert.Equal(t, "in_progress", c.State)
	assert.NotNil(t, c.StartedAt)
	c = e.callback(t, c.ProviderCallID, "ringing", "1", nil)
	assert.Equal(t, "in_progress", c.State, "a redelivered earlier event does not move the call back")
	c = e.callback(t, c.ProviderCallID, "completed", "3", url.Values{"CallDuration": {"184"}, "RecordingSid": {"RE1"}})
	assert.Equal(t, "completed", c.State)
	assert.Equal(t, int64(184), *c.DurationSeconds)
	assert.Equal(t, "RE1", c.RecordingRef)
	c = e.callback(t, c.ProviderCallID, "completed", "3", nil)
	assert.Equal(t, "completed", c.State)
	c = e.callback(t, c.ProviderCallID, "failed", "4", nil)
	assert.Equal(t, "completed", c.State, "terminal states never move")
	d, _ = e.calls.Get(ctx, coretest.Viewer, c.ID)
	kinds := map[string]int{}
	for _, ev := range d.Events {
		kinds[ev.Kind]++
	}
	assert.Equal(t, 1, kinds["late"])
	assert.Equal(t, 4, kinds["provider"], "queued, ringing, in_progress, completed once each")
	// An event for a call ADX never placed is refused.
	req = httptest.NewRequest("POST", "/webhooks/voice/status", strings.NewReader(url.Values{"CallSid": {"stranger"}, "CallStatus": {"completed"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Fake-Signature", "s3")
	_, err = e.calls.Callback(ctx, req)
	require.Error(t, err)

	// Hangup on a finished call is refused; recording access follows policy.
	_, err = e.calls.Hangup(ctx, coretest.Marketer, c.ID, "x")
	require.Error(t, err)
	ok, why, err := e.calls.RecordingAccess(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, why, "admin")
	ok, _, _ = e.calls.RecordingAccess(ctx, coretest.Admin)
	assert.True(t, ok)

	// Retention: 30 days later the recording is deleted at the provider and marked.
	n, err := e.calls.ExpireRecordings(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	e.clock.Advance(31 * 24 * time.Hour)
	n, err = e.calls.ExpireRecordings(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, []string{"RE1"}, e.tel.Deleted)
	d, _ = e.calls.Get(ctx, coretest.Viewer, c.ID)
	assert.NotNil(t, d.Call.RecordingDeletedAt)
	assert.Equal(t, "RE1", d.Call.RecordingRef, "the reference stays for the record")
}
