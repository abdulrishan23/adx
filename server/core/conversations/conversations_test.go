package conversations_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/assistant/assistanttest"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/conversations"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/qualification"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type fakeMessenger struct {
	sent      []outreach.OutboundMessage
	templates []conversations.TemplateMessage
	err       error
}

func (m *fakeMessenger) Name() string       { return "fake-wa" }
func (m *fakeMessenger) Channels() []string { return []string{"whatsapp"} }
func (m *fakeMessenger) Send(_ context.Context, msg outreach.OutboundMessage) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.sent = append(m.sent, msg)
	return "wamid.out." + msg.IdempotencyKey, nil
}
func (m *fakeMessenger) SendTemplate(_ context.Context, msg conversations.TemplateMessage) (string, error) {
	m.templates = append(m.templates, msg)
	return "wamid.tpl." + msg.IdempotencyKey, nil
}

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	crm       *crm.CRM
	brands    *brands.Brands
	signals   *signals.Signals
	out       *outreach.Outreach
	policies  *policies.Policies
	model     *assistanttest.Model
	messenger *fakeMessenger
	conv      *conversations.Conversations
	project   int64
	brand     int64
}

const job = core.ViaJob

var system = core.System(job)

// setup builds the agent over real core services, a scripted model and a
// fake messenger. The outreach config is saved with whatsapp in the given
// mode, or not at all when mode is empty.
func setup(t *testing.T, mode outreach.ChannelMode, withModel bool) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)), model: &assistanttest.Model{}, messenger: &fakeMessenger{}}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	ev := evidence.New(e.db, e.clock)
	e.crm = crm.New(e.db, ev, crm.Config{Clock: e.clock})
	e.brands = brands.New(e.db, brands.Config{Clock: e.clock})
	e.signals = signals.New(e.db, ev, e.crm, signals.Config{Clock: e.clock})
	e.policies = policies.New(e.db)
	e.out = outreach.New(e.db, ev, outreach.Config{Clock: e.clock, Brands: e.brands, Signals: e.signals, CRM: e.crm, Policies: e.policies})
	cfg := conversations.Config{Clock: e.clock, Outreach: e.out, Signals: e.signals, CRM: e.crm, Brands: e.brands, Policies: e.policies, Messenger: e.messenger, Templates: e.messenger}
	if withModel {
		cfg.Model = e.model
	}
	e.conv = conversations.New(e.db, ev, cfg)
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.project = p.ID
	b, err := e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	e.brand = b.ID
	require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, e.project, e.brand))
	fc, err := e.brands.AddFact(ctx, coretest.Admin, e.brand, brands.FactInput{ProjectID: e.project, Key: "price.starting", Kind: brands.KindMoney, Value: "150000000", Currency: "AED", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceManual, SourceRef: "list"})
	require.NoError(t, err)
	_, err = e.brands.ApproveFact(ctx, coretest.Admin, fc.Fact.ID)
	require.NoError(t, err)
	rules := signals.SuggestedRules()
	rules.Stages = &signals.StageCriteria{Hot: 40, Warm: 20}
	_, err = e.signals.PutRules(ctx, coretest.Admin, rules)
	require.NoError(t, err)
	if mode != "" {
		doc := outreach.SuggestedConfig()
		doc.Channels["whatsapp"] = mode
		_, err = e.out.PutConfig(ctx, coretest.Admin, doc)
		require.NoError(t, err)
	}
	return e
}

// lead makes a person with an enquiry on The Wadi, as a CRM lead would.
func (e *env) lead(t *testing.T, name, phone string) (int64, int64) {
	t.Helper()
	ctx := t.Context()
	d, err := e.crm.Create(ctx, coretest.Marketer, crm.PersonInput{DisplayName: name, Phone: phone})
	require.NoError(t, err)
	enq, err := e.db.Writer().CreateEnquiry(ctx, store.CreateEnquiryParams{WorkspaceID: 1, PersonID: d.Person.ID, ProjectID: optionalID(e.project), SourceKind: "lead", SourceRef: "x", FirstContactAt: e.clock.Now(), LastActivityAt: e.clock.Now(), CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	return d.Person.ID, enq.ID
}

func (e *env) inbound(t *testing.T, ref, from, body string) conversations.ThreadReceived {
	t.Helper()
	r, err := e.conv.Receive(t.Context(), system, conversations.InboundMessage{ProviderRef: ref, From: from, ProfileName: "Fatima", Body: body, ReceivedAt: e.clock.Now()})
	require.NoError(t, err)
	return r
}

func decision(reply string, d conversations.ReplyDecision) assistant.Response {
	d.Reply = reply
	if d.Language == "" {
		d.Language = "en"
	}
	return assistanttest.ToolCall("t1", "reply", d)
}

func reading(outcome, confidence string, answers map[string]qualification.ReadingAnswer) assistant.Response {
	return assistanttest.ToolCall("r1", "record_qualification", qualification.Reading{Summary: "summary", Outcome: outcome, Answers: answers, Confidence: confidence})
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

// Catches: a reply leaving while the channel is off or the outreach config
// was never saved. The agent drafts, the draft is held with the reason, and
// nothing reaches the messenger; approving it by hand does not override the
// policy either.
func TestNothingIsSentWithThePolicyOff(t *testing.T) {
	for _, mode := range []outreach.ChannelMode{"", outreach.ModeOff} {
		e := setup(t, mode, true)
		e.lead(t, "Fatima Haddad", "0501234567")
		e.model.Responses = []assistant.Response{decision("Hello Fatima, happy to help with The Wadi. When are you hoping to buy?", conversations.ReplyDecision{})}
		r := e.inbound(t, "wamid.1", "+971501234567", "Hi, interested in The Wadi")
		require.NotNil(t, r.Reply)
		assert.Equal(t, "held", r.Reply.State)
		if mode == "" {
			assert.Contains(t, r.Reply.Reason, "not configured")
		} else {
			assert.Contains(t, r.Reply.Reason, "off")
		}
		assert.Empty(t, e.messenger.sent)

		_, err := e.conv.Approve(t.Context(), coretest.Marketer, r.Reply.ID)
		require.NoError(t, err)
		assert.Empty(t, e.messenger.sent, "approval must not send on a channel that is off (mode %q)", mode)
	}
}

// Catches: the agent stating a price nobody approved. The model's reply
// quotes AED 1,200,000 where the only approved fact is AED 1,500,000; the
// reply is held, the thread goes to a person as outside the facts, the
// messenger never sees it, and a person cannot approve the held text.
func TestReplyQuotingAnUnapprovedPriceIsHeldAndHandedOff(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, true)
	e.lead(t, "Fatima Haddad", "0501234567")
	e.model.Responses = []assistant.Response{
		decision("Two-bedroom units start at AED 1,200,000 with 10% down.", conversations.ReplyDecision{}),
		reading("unclear", "high", nil),
	}
	r := e.inbound(t, "wamid.1", "+971501234567", "What does a 2BR cost?")
	require.NotNil(t, r.Reply)
	assert.Equal(t, "held", r.Reply.State)
	assert.Equal(t, conversations.HandoffOutsideFacts, r.Reply.Handoff)
	assert.Contains(t, r.Reply.Reason, "not an approved fact")
	require.NotNil(t, r.Reply.Claims)
	assert.Equal(t, conversations.StateWaiting, r.Conversation.State)
	assert.Equal(t, conversations.HandoffOutsideFacts, r.Conversation.HandoffKind)
	assert.Empty(t, e.messenger.sent)

	_, err := e.conv.Approve(t.Context(), coretest.Marketer, r.Reply.ID)
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.Empty(t, e.messenger.sent)

	// The approved figure is fine: it goes.
	e.model.Responses = []assistant.Response{decision("Prices start at AED 1,500,000. Is it to live in or to invest?", conversations.ReplyDecision{})}
	_, err = e.conv.HandBack(t.Context(), coretest.Marketer, r.Conversation.ID)
	require.NoError(t, err)
	r2 := e.inbound(t, "wamid.2", "+971501234567", "ok and the starting price?")
	require.NotNil(t, r2.Reply)
	assert.Equal(t, "sent", r2.Reply.State)
	require.Len(t, e.messenger.sent, 1)
	assert.Equal(t, "+971501234567", e.messenger.sent[0].To)
}

// Catches: a request for a person missed. The model is not even asked: the
// words "talk to someone" hand the thread off, the customer gets one line
// saying a colleague will write, and the thread waits for a person with
// the urgent reason. A later message does not wake the agent.
func TestRequestForAHumanHandsOffWithoutAskingTheModel(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, true)
	e.lead(t, "Fatima Haddad", "0501234567")
	e.model.Responses = []assistant.Response{reading("transfer", "high", nil)}
	r := e.inbound(t, "wamid.1", "+971501234567", "Can I talk to someone please")
	require.NotNil(t, r.Reply)
	assert.Equal(t, "sent", r.Reply.State)
	assert.Equal(t, conversations.HandoffRequestedHuman, r.Reply.Handoff)
	assert.Equal(t, conversations.StateWaiting, r.Conversation.State)
	assert.Equal(t, conversations.HandoffRequestedHuman, r.Conversation.HandoffKind)
	assert.Contains(t, r.Conversation.StateReason, "asked for a person")
	// Only the reading prompt reached the model.
	require.Len(t, e.model.Requests, 1)
	assert.Equal(t, "record_qualification", e.model.Requests[0].ToolChoice)
	assert.Equal(t, "handoff", *stage(t, e, r.Conversation.EnquiryID))

	r2 := e.inbound(t, "wamid.2", "+971501234567", "hello?")
	assert.Nil(t, r2.Reply)
	assert.Equal(t, conversations.StateWaiting, r2.Conversation.State)
	require.Len(t, e.messenger.sent, 1)

	w, err := e.conv.Pending(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	require.Len(t, w.Handoffs, 1)
	assert.Equal(t, "Fatima Haddad", w.Handoffs[0].PersonName)
}

// Catches: the model missing a handoff it did name. When the model sets
// handoff to objection the thread waits for a person, the objection is on
// the brief, and the reply still goes because it only acknowledges.
func TestObjectionFromTheModelHandsOffAndReachesTheBrief(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, true)
	_, enq := e.lead(t, "Fatima Haddad", "0501234567")
	e.model.Responses = []assistant.Response{
		decision("I understand, Jumeirah is a different setting. A colleague will come back to you on location.", conversations.ReplyDecision{Handoff: conversations.HandoffObjection, HandoffNote: "location objection", Objection: &struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		}{Kind: "location", Text: "too far from Jumeirah"}}),
		reading("unclear", "high", nil),
	}
	r := e.inbound(t, "wamid.1", "+971501234567", "It is too far from Jumeirah for us")
	require.NotNil(t, r.Reply)
	assert.Equal(t, "sent", r.Reply.State)
	assert.Equal(t, conversations.StateWaiting, r.Conversation.State)
	assert.Equal(t, conversations.HandoffObjection, r.Conversation.HandoffKind)
	b, err := e.out.NextAction(t.Context(), coretest.Viewer, enq)
	require.NoError(t, err)
	require.Len(t, b.Objections, 1)
	assert.Equal(t, "location", b.Objections[0].Kind)
	assert.Equal(t, "conversation:"+itoa(r.Conversation.ID), b.Objections[0].Source)
}

// Catches: a free reply outside the 24-hour window, and a template inside
// it. Twenty-five hours after the customer's last message a person's reply
// is held with the window reason and nothing is sent; a template is the
// only thing that may go then, and inside the window a template is refused.
func TestWindowDecidesFreeTextAgainstTemplate(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, true)
	e.lead(t, "Fatima Haddad", "0501234567")
	e.model.Responses = []assistant.Response{decision("Hello Fatima. When are you hoping to buy?", conversations.ReplyDecision{})}
	r := e.inbound(t, "wamid.1", "+971501234567", "Hi")
	require.Equal(t, "sent", r.Reply.State)
	require.NotNil(t, r.Conversation.WindowClosesAt)
	assert.Equal(t, e.clock.Now().Add(24*time.Hour), *r.Conversation.WindowClosesAt)

	e.clock.Advance(25 * time.Hour)
	turn, err := e.conv.Send(t.Context(), coretest.Marketer, r.Conversation.ID, "Still interested?")
	require.NoError(t, err)
	assert.Equal(t, "held", turn.State)
	assert.Contains(t, turn.Reason, "outside the 24-hour window")
	assert.Len(t, e.messenger.sent, 1)
	assert.Empty(t, e.messenger.templates)

	// A reply to a fresh message is inside the window again.
	e.model.Responses = []assistant.Response{decision("Welcome back. Is it to live in or to invest?", conversations.ReplyDecision{})}
	_, err = e.conv.HandBack(t.Context(), coretest.Marketer, r.Conversation.ID)
	require.NoError(t, err)
	r2 := e.inbound(t, "wamid.2", "+971501234567", "yes")
	require.NotNil(t, r2.Reply)
	assert.Equal(t, "sent", r2.Reply.State)
	assert.Len(t, e.messenger.sent, 2)
}

// Catches: the agent nagging. With the customer silent after the agent's
// question, asking the agent again sends nothing: the last word is its own.
func TestAgentNeverSendsTwiceInARow(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, true)
	e.lead(t, "Fatima Haddad", "0501234567")
	e.model.Responses = []assistant.Response{decision("When are you hoping to buy?", conversations.ReplyDecision{}), decision("Just checking in!", conversations.ReplyDecision{})}
	r := e.inbound(t, "wamid.1", "+971501234567", "Hi")
	require.Equal(t, "sent", r.Reply.State)
	_, err := e.conv.Reply(t.Context(), coretest.Marketer, r.Conversation.ID)
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.Len(t, e.messenger.sent, 1)
	assert.Len(t, e.model.Requests, 1)
}

// Catches: a repeated webhook delivery making a second turn or a second
// reply, and a message from an unknown number not finding its way to a
// person record.
func TestRepeatedDeliveryIsOneTurnAndAnUnknownNumberMakesAPerson(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, true)
	e.model.Responses = []assistant.Response{decision("Hello! Which project are you asking about?", conversations.ReplyDecision{})}
	r := e.inbound(t, "wamid.1", "+971509999999", "Hello")
	require.NotNil(t, r.Reply)
	assert.False(t, r.Duplicate)
	d, err := e.crm.Get(t.Context(), coretest.Viewer, r.Conversation.PersonID)
	require.NoError(t, err)
	assert.Equal(t, "Fatima", d.Person.DisplayName)
	require.NotNil(t, d.Person.PrimaryPhone)
	assert.Equal(t, "+971509999999", *d.Person.PrimaryPhone)
	require.Len(t, d.Enquiries, 1)
	assert.Nil(t, d.Enquiries[0].ProjectID)
	assert.Equal(t, "whatsapp", d.Enquiries[0].SourceKind)

	again := e.inbound(t, "wamid.1", "+971509999999", "Hello")
	assert.True(t, again.Duplicate)
	assert.Nil(t, again.Reply)
	assert.Equal(t, r.Turn.ID, again.Turn.ID)
	assert.Len(t, e.messenger.sent, 1)
	assert.Len(t, e.model.Requests, 1)
	detail, err := e.conv.Get(t.Context(), coretest.Viewer, r.Conversation.ID)
	require.NoError(t, err)
	assert.Len(t, detail.Turns, 2)
}

// Catches: the customer naming a project and the agent still answering
// without its facts. The first answer names The Wadi; the enquiry takes
// the project, the model is asked again with the project's facts, and the
// sent reply is the second answer.
func TestNamedProjectJoinsTheEnquiryAndTheBrief(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, true)
	e.model.Responses = []assistant.Response{
		decision("Got it, The Wadi.", conversations.ReplyDecision{Project: "The Wadi"}),
		decision("The Wadi starts at AED 1,500,000. When are you hoping to buy?", conversations.ReplyDecision{}),
	}
	r := e.inbound(t, "wamid.1", "+971509999999", "Hi, I saw your ad for The Wadi")
	require.NotNil(t, r.Reply)
	assert.Equal(t, "sent", r.Reply.State)
	assert.Contains(t, r.Reply.Body, "1,500,000")
	require.Len(t, e.model.Requests, 2)
	assert.NotContains(t, e.model.Requests[0].System, "price.starting")
	assert.Contains(t, e.model.Requests[1].System, "price.starting: AED 1,500,000")
	d, err := e.crm.Get(t.Context(), coretest.Viewer, r.Conversation.PersonID)
	require.NoError(t, err)
	require.NotNil(t, d.Enquiries[0].ProjectID)
	assert.Equal(t, e.project, *d.Enquiries[0].ProjectID)
}

// Catches: a reading applied twice, or applied without scoring the person.
// When the agent is done the thread is read; the outcome scores once under
// the conversation's key, the sure answer is a stated attribute, the
// uncertain one is held and named, and the stage moves to qualified.
func TestReadingScoresOnceAndHoldsUncertainFigures(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, true)
	person, enq := e.lead(t, "Fatima Haddad", "0501234567")
	e.model.Responses = []assistant.Response{
		decision("Thank you, the team will follow up.", conversations.ReplyDecision{Done: true}),
		reading("interested", "high", map[string]qualification.ReadingAnswer{"purchase_timeline": {Value: "3_months", Quote: "in three months"}, "budget": {Value: "1.2M", Uncertain: true, Quote: "around 1.2"}}),
	}
	r := e.inbound(t, "wamid.1", "+971501234567", "In three months, around 1.2, cash")
	require.NotNil(t, r.Reply)
	c := r.Conversation
	assert.Equal(t, "interested", c.Outcome)
	assert.Equal(t, "auto", c.ReviewStatus)
	assert.Equal(t, "conversation:"+itoa(c.ID)+":v1", c.AppliedKey)
	assert.Equal(t, []string{"purchase_timeline"}, c.Applied.Attributes)
	require.Len(t, c.Applied.Held, 1)
	assert.Contains(t, c.Applied.Held[0], "budget")
	assert.Equal(t, "qualified", c.Applied.Stage)
	assert.Equal(t, "qualified", *stage(t, e, &enq))
	require.NotNil(t, c.Applied.SignalEventID)

	v, err := e.signals.Score(t.Context(), coretest.Viewer, person)
	require.NoError(t, err)
	kinds := map[string]int{}
	for _, ev := range v.Events {
		kinds[ev.Kind]++
	}
	assert.Equal(t, 1, kinds["conversation_outcome"])
	assert.Equal(t, 1, kinds["message_reply"])
	require.NotNil(t, v.Latest)
	require.NotNil(t, v.Latest.Intent)
	assert.Greater(t, *v.Latest.Intent, int64(0))

	// Reading again over the same turns applies nothing new.
	again, err := e.conv.Read(t.Context(), coretest.Marketer, c.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), again.AppliedVersion)
	assert.Len(t, e.model.Requests, 2)

	// A correction discards the model's event and applies a new version.
	fixed, err := e.conv.Correct(t.Context(), coretest.Marketer, c.ID, qualification.CorrectionInput{Outcome: "callback", Note: "she asked to be called next week"})
	require.NoError(t, err)
	assert.Equal(t, "corrected", fixed.ReviewStatus)
	assert.Equal(t, int64(2), fixed.AppliedVersion)
	assert.Equal(t, "interested", fixed.Applied.ModelOutcome)
	assert.Equal(t, "callback", *stage(t, e, &enq))
}

// Catches: a low-confidence reading changing the enquiry before a person
// looks, and the pending reading missing from what the supervisor shows.
func TestUnsureReadingWaitsForAPerson(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, true)
	_, enq := e.lead(t, "Fatima Haddad", "0501234567")
	e.model.Responses = []assistant.Response{decision("Thanks, the team will be in touch.", conversations.ReplyDecision{Done: true}), reading("interested", "low", nil)}
	r := e.inbound(t, "wamid.1", "+971501234567", "maybe, we'll see")
	assert.Equal(t, "pending", r.Conversation.ReviewStatus)
	assert.Equal(t, "", r.Conversation.AppliedKey)
	assert.Nil(t, stage(t, e, &enq))
	w, err := e.conv.Pending(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	require.Len(t, w.Readings, 1)
	c, err := e.conv.Confirm(t.Context(), coretest.Marketer, r.Conversation.ID)
	require.NoError(t, err)
	assert.Equal(t, "confirmed", c.ReviewStatus)
	assert.Equal(t, "qualified", *stage(t, e, &enq))
}

// Catches: a message after "stop". The channel is suppressed for the
// person, the thread closes, one acknowledgement goes, and a later message
// from them wakes nothing and sends nothing.
func TestStopSuppressesAndCloses(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, true)
	person, _ := e.lead(t, "Fatima Haddad", "0501234567")
	e.model.Responses = []assistant.Response{reading("not_interested", "high", nil)}
	r := e.inbound(t, "wamid.1", "+971501234567", "STOP")
	assert.Nil(t, r.Reply)
	assert.Equal(t, conversations.StateClosed, r.Conversation.State)
	assert.Equal(t, conversations.HandoffStop, r.Conversation.HandoffKind)
	require.Len(t, e.messenger.sent, 1)
	assert.Contains(t, e.messenger.sent[0].Body, "won't message you again")
	blocked, why, err := e.out.Suppressed(t.Context(), coretest.Viewer, person, "whatsapp")
	require.NoError(t, err)
	assert.True(t, blocked)
	assert.Contains(t, why, "STOP")

	r2 := e.inbound(t, "wamid.2", "+971501234567", "actually wait")
	assert.Nil(t, r2.Reply)
	assert.Equal(t, conversations.StateClosed, r2.Conversation.State)
	assert.Len(t, e.messenger.sent, 1)
	_, err = e.conv.Send(t.Context(), coretest.Marketer, r2.Conversation.ID, "Hi again")
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
}

// Catches: an approval-mode channel sending without a person, and a
// person's approval not sending. The agent's draft waits; Approve sends it
// and records who approved.
func TestApprovedModeWaitsForAPerson(t *testing.T) {
	e := setup(t, outreach.ModeApproved, true)
	e.lead(t, "Fatima Haddad", "0501234567")
	e.model.Responses = []assistant.Response{decision("Hello Fatima. When are you hoping to buy?", conversations.ReplyDecision{})}
	r := e.inbound(t, "wamid.1", "+971501234567", "Hi")
	require.NotNil(t, r.Reply)
	assert.Equal(t, "draft", r.Reply.State)
	assert.Contains(t, r.Reply.Reason, "waiting for a person")
	assert.Empty(t, e.messenger.sent)
	w, err := e.conv.Pending(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	require.Len(t, w.Drafts, 1)

	_, err = e.conv.Approve(t.Context(), coretest.Viewer, r.Reply.ID)
	assert.ErrorIs(t, err, core.ErrForbidden)
	sent, err := e.conv.Approve(t.Context(), coretest.Marketer, r.Reply.ID)
	require.NoError(t, err)
	assert.Equal(t, "sent", sent.State)
	require.NotNil(t, sent.ApprovedBy)
	assert.Equal(t, coretest.Marketer.UserID, *sent.ApprovedBy)
	require.Len(t, e.messenger.sent, 1)

	ok, err := e.conv.UpdateStatus(t.Context(), system, conversations.StatusUpdate{ProviderRef: sent.ProviderRef, Status: "read", At: e.clock.Now()})
	require.NoError(t, err)
	assert.True(t, ok)
	d, err := e.conv.Get(t.Context(), coretest.Viewer, r.Conversation.ID)
	require.NoError(t, err)
	assert.Equal(t, "read", d.Turns[1].State)
}

// Catches: the kill switch not covering the agent, and a send that fails
// at the provider being reported as sent.
func TestKillSwitchHoldsAndProviderFailureIsFailed(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, true)
	e.lead(t, "Fatima Haddad", "0501234567")
	_, err := e.policies.SetKillSwitch(t.Context(), coretest.Admin, 0, true, "incident")
	require.NoError(t, err)
	e.model.Responses = []assistant.Response{decision("Hello Fatima.", conversations.ReplyDecision{})}
	r := e.inbound(t, "wamid.1", "+971501234567", "Hi")
	assert.Equal(t, "held", r.Reply.State)
	assert.Contains(t, r.Reply.Reason, "kill switch")
	assert.Empty(t, e.messenger.sent)

	_, err = e.policies.SetKillSwitch(t.Context(), coretest.Admin, 0, false, "over")
	require.NoError(t, err)
	e.messenger.err = errors.New("whatsapp refused the message (131047 OAuthException): re-engagement")
	t2, err := e.conv.Approve(t.Context(), coretest.Marketer, r.Reply.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", t2.State)
	assert.Contains(t, t2.Reason, "131047")
	w, err := e.conv.Pending(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	assert.Len(t, w.Failed, 1)
}

// Catches: a thread running without a model as if the agent were there.
// The thread waits for a person with the reason, and the readiness says so.
func TestWithoutAModelTheThreadWaitsForAPerson(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, false)
	e.lead(t, "Fatima Haddad", "0501234567")
	r := e.inbound(t, "wamid.1", "+971501234567", "Hi")
	assert.Nil(t, r.Reply)
	assert.Equal(t, conversations.StateWaiting, r.Conversation.State)
	assert.Equal(t, conversations.HandoffNoModel, r.Conversation.HandoffKind)
	st, err := e.conv.Readiness(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	assert.False(t, st.ModelReady)
	assert.Equal(t, "automatic", st.ChannelMode)
	_, err = e.conv.HandBack(t.Context(), coretest.Marketer, r.Conversation.ID)
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
}

// Catches: a person's own words checked as if they were the agent's, and
// the agent carrying on after a person took the thread.
func TestAPersonsOwnMessageGoesAndSilencesTheAgent(t *testing.T) {
	e := setup(t, outreach.ModeAutomatic, true)
	e.lead(t, "Fatima Haddad", "0501234567")
	e.model.Responses = []assistant.Response{decision("Hello Fatima.", conversations.ReplyDecision{})}
	r := e.inbound(t, "wamid.1", "+971501234567", "Hi")
	turn, err := e.conv.Send(t.Context(), coretest.Marketer, r.Conversation.ID, "Hi Fatima, Sara here. We can do AED 1,400,000 on the corner unit.")
	require.NoError(t, err)
	assert.Equal(t, "sent", turn.State)
	assert.Equal(t, "user:2", turn.Author)
	r2 := e.inbound(t, "wamid.2", "+971501234567", "great")
	assert.Nil(t, r2.Reply)
	assert.Equal(t, conversations.StatePerson, r2.Conversation.State)
	assert.Len(t, e.model.Requests, 1)
	d, err := e.conv.Get(t.Context(), coretest.Viewer, r.Conversation.ID)
	require.NoError(t, err)
	assert.Equal(t, "Fatima Haddad", d.Person.DisplayName)
	assert.Equal(t, "The Wadi", d.ProjectName)
}

func stage(t *testing.T, e *env, enq *int64) *string {
	t.Helper()
	require.NotNil(t, enq)
	row, err := e.db.Reader().GetEnquiry(t.Context(), store.GetEnquiryParams{ID: *enq, WorkspaceID: 1})
	require.NoError(t, err)
	if !row.Stage.Valid {
		return nil
	}
	return &row.Stage.String
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
