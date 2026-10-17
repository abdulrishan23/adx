package outreach_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type fakeFields struct {
	calls []map[string]string
	err   error
}

func (f *fakeFields) Name() string { return "fake-ghl" }
func (f *fakeFields) SetContactFields(_ context.Context, _, _ string, fields map[string]string) error {
	f.calls = append(f.calls, fields)
	return f.err
}

type fakeTokens struct{}

func (fakeTokens) GHLToken(context.Context, core.Actor, string) (string, error) { return "tok", nil }

type fakeMessenger struct {
	sent []outreach.OutboundMessage
	err  error
}

func (m *fakeMessenger) Name() string       { return "fake-wa" }
func (m *fakeMessenger) Channels() []string { return []string{"whatsapp"} }
func (m *fakeMessenger) Send(_ context.Context, msg outreach.OutboundMessage) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.sent = append(m.sent, msg)
	return "wamid-1", nil
}

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	crm       *crm.CRM
	brands    *brands.Brands
	signals   *signals.Signals
	policies  *policies.Policies
	fields    *fakeFields
	messenger *fakeMessenger
	out       *outreach.Outreach
	acct      int64
	project   int64
	brand     int64
}

func setup(t *testing.T, withAdapters bool) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)), fields: &fakeFields{}, messenger: &fakeMessenger{}}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	ev := evidence.New(e.db, e.clock)
	e.crm = crm.New(e.db, ev, crm.Config{Clock: e.clock})
	e.brands = brands.New(e.db, brands.Config{Clock: e.clock})
	e.signals = signals.New(e.db, ev, e.crm, signals.Config{Clock: e.clock})
	e.policies = policies.New(e.db)
	cfg := outreach.Config{Clock: e.clock, Brands: e.brands, Signals: e.signals, CRM: e.crm, Policies: e.policies}
	if withAdapters {
		cfg.Fields, cfg.Tokens, cfg.Messenger = e.fields, fakeTokens{}, e.messenger
	}
	e.out = outreach.New(e.db, ev, cfg)
	cred, err := e.db.Writer().UpsertCredential(ctx, store.UpsertCredentialParams{Provider: "gohighlevel", Kind: "private_token", Subject: "loc", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "x", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(ctx, store.UpsertAccountParams{WorkspaceID: core.DefaultWorkspace, Type: "gohighlevel", ExternalID: "loc-1", CredentialID: cred.ID, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	e.acct = acct.ID
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.project = p.ID
	b, err := e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	e.brand = b.ID
	require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, e.project, e.brand))
	return e
}

// lead creates a CRM lead and resolves it to a person; returns the person
// and enquiry ids.
func (e *env) lead(t *testing.T, ext, first, phone string) (int64, int64) {
	t.Helper()
	ctx := t.Context()
	_, err := e.db.Writer().UpsertLead(ctx, store.UpsertLeadParams{WorkspaceID: 1, AccountID: e.acct, ExternalID: ext, FirstName: ns(first), LastName: ns("Haddad"), Phone: ns(phone), PhoneE164: ns(e.crm.NormalizePhone(phone)),
		ProjectID: sql.NullInt64{Int64: e.project, Valid: true}, FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	_, err = e.crm.Sync(ctx, coretest.Marketer)
	require.NoError(t, err)
	people, err := e.crm.List(ctx, coretest.Viewer, first, 10)
	require.NoError(t, err)
	require.Len(t, people, 1)
	d, err := e.crm.Get(ctx, coretest.Viewer, people[0].ID)
	require.NoError(t, err)
	require.NotEmpty(t, d.Enquiries)
	return d.Person.ID, d.Enquiries[0].ID
}

func ns(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func (e *env) configure(t *testing.T, doc outreach.OutreachDocument) {
	t.Helper()
	_, err := e.out.PutConfig(t.Context(), coretest.Admin, doc)
	require.NoError(t, err)
}

func (e *env) allowFieldWrites(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	doc := policies.SuggestedDefaults()
	pct, perDay := 20, 50
	doc.Caps = policies.PolicyCaps{DailySpendIncreasePct: &pct, MaxChangesPerDay: &perDay}
	doc.WriteAccounts = []int64{e.acct}
	doc.Actions[policies.LeadFieldsSet] = policies.PolicyRule{Marketer: "allowed", Admin: "allowed"}
	_, err := e.policies.Put(ctx, coretest.Admin, doc)
	require.NoError(t, err)
	require.NoError(t, e.db.Writer().UpsertAccountCapability(ctx, store.UpsertAccountCapabilityParams{AccountID: e.acct, Capability: "write.lead_fields", State: "verified", VerifiedAt: sql.NullTime{Time: e.clock.Now(), Valid: true}, CheckedAt: e.clock.Now()}))
}

// Catches: an outside edit of a field ADX owns being overwritten or
// silently accepted, a CRM-owned field being fought over, an unconfigured
// field being resolved by guessing, and a held row re-sending on its own.
func TestExternalEditRacingADXBecomesConflictOrFollowsOwnership(t *testing.T) {
	e := setup(t, true)
	ctx := t.Context()
	person, _ := e.lead(t, "c1", "Amal", "0501234567")
	e.configure(t, outreach.OutreachDocument{Fields: map[string]outreach.FieldOwner{"adx_temperature": outreach.OwnerADX, "notes": outreach.OwnerCRM}})
	e.allowFieldWrites(t)

	temp, err := e.out.QueueFieldWrite(ctx, coretest.Marketer, person, "adx_temperature", "hot")
	require.NoError(t, err)
	require.Len(t, temp, 1)
	notes, err := e.out.QueueFieldWrite(ctx, coretest.Marketer, person, "notes", "ADX: called twice")
	require.NoError(t, err)
	other, err := e.out.QueueFieldWrite(ctx, coretest.Marketer, person, "budget", "1.5m")
	require.NoError(t, err)
	n, err := e.out.DispatchFields(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "only the ADX-owned field is sent")
	require.Len(t, e.fields.calls, 1)
	assert.Equal(t, map[string]string{"adx_temperature": "hot"}, e.fields.calls[0])
	writes, _ := e.out.ListFieldWrites(ctx, coretest.Viewer, 10)
	byID := map[int64]outreach.FieldWrite{}
	for _, w := range writes {
		byID[w.ID] = w
	}
	assert.Equal(t, "sent", byID[temp[0].ID].State)
	assert.Equal(t, "held", byID[notes[0].ID].State)
	assert.Contains(t, byID[notes[0].ID].Reason, "not owned by ADX")
	assert.Equal(t, "held", byID[other[0].ID].State)

	// The CRM comes back: temperature edited by a salesperson, notes theirs,
	// budget theirs.
	e.clock.Advance(time.Minute)
	require.NoError(t, e.out.ObserveFields(ctx, coretest.Marketer, e.acct, "c1", map[string]string{"adx_temperature": "warm", "notes": "Spoke to the husband", "budget": "2m"}, e.clock.Now()))
	writes, _ = e.out.ListFieldWrites(ctx, coretest.Viewer, 10)
	for _, w := range writes {
		byID[w.ID] = w
	}
	assert.Equal(t, "conflict", byID[temp[0].ID].State, "ADX owns it and the CRM disagrees: a person decides")
	assert.Contains(t, string(byID[temp[0].ID].Conflict), `"theirs":"warm"`)
	assert.Equal(t, "superseded", byID[notes[0].ID].State, "the CRM owns notes; theirs stands")
	assert.Equal(t, "conflict", byID[other[0].ID].State)
	assert.Contains(t, byID[other[0].ID].Reason, "not configured")
	assert.Len(t, e.fields.calls, 1, "nothing was re-sent")
	n, _ = e.out.DispatchFields(ctx, coretest.Marketer)
	assert.Equal(t, 0, n, "a conflict never dispatches by itself")

	// Resolve: keep ours re-queues a fresh row; the conflict row keeps its record.
	kept, err := e.out.ResolveConflict(ctx, coretest.Marketer, temp[0].ID, true, "the salesperson agreed the score is right")
	require.NoError(t, err)
	assert.Equal(t, "pending", kept.State)
	assert.NotEqual(t, temp[0].ID, kept.ID)
	n, _ = e.out.DispatchFields(ctx, coretest.Marketer)
	assert.Equal(t, 1, n)
	_, err = e.out.ResolveConflict(ctx, coretest.Marketer, temp[0].ID, true, "again")
	require.Error(t, err)
	taken, err := e.out.ResolveConflict(ctx, coretest.Marketer, other[0].ID, false, "their budget is newer")
	require.NoError(t, err)
	assert.Equal(t, "superseded", taken.State)
}

// Catches: ADX's own write coming back from the CRM being treated as an
// outside change, and a repeated queue creating a second row.
func TestProviderEchoCreatesNoNewIntent(t *testing.T) {
	e := setup(t, true)
	ctx := t.Context()
	person, _ := e.lead(t, "c1", "Amal", "0501234567")
	e.configure(t, outreach.OutreachDocument{Fields: map[string]outreach.FieldOwner{"adx_temperature": outreach.OwnerADX}})
	e.allowFieldWrites(t)
	rows, err := e.out.QueueFieldWrite(ctx, coretest.Marketer, person, "adx_temperature", "hot")
	require.NoError(t, err)
	again, err := e.out.QueueFieldWrite(ctx, coretest.Marketer, person, "adx_temperature", "hot")
	require.NoError(t, err)
	assert.Equal(t, rows[0].ID, again[0].ID, "same value, same row")
	n, _ := e.out.DispatchFields(ctx, coretest.Marketer)
	assert.Equal(t, 1, n)
	// The import sees our value: an echo.
	require.NoError(t, e.out.ObserveFields(ctx, coretest.Marketer, e.acct, "c1", map[string]string{"adx_temperature": "hot"}, e.clock.Now()))
	writes, _ := e.out.ListFieldWrites(ctx, coretest.Viewer, 10)
	require.Len(t, writes, 1, "no new intent")
	assert.Equal(t, "confirmed", writes[0].State)
	assert.NotNil(t, writes[0].ConfirmedAt)
	n, _ = e.out.DispatchFields(ctx, coretest.Marketer)
	assert.Equal(t, 0, n)
	assert.Len(t, e.fields.calls, 1)
	// A newer value supersedes the pending one; the old sent row is history.
	newer, err := e.out.QueueFieldWrite(ctx, coretest.Marketer, person, "adx_temperature", "warm")
	require.NoError(t, err)
	assert.Equal(t, "pending", newer[0].State)
	writes, _ = e.out.ListFieldWrites(ctx, coretest.Viewer, 10)
	assert.Len(t, writes, 2)
}

// Catches: an opt-out being bypassed by a retry, a lifted opt-out without
// a reason, and a marketer lifting an opt-out.
func TestSuppressedPersonCannotBeContactedByRetry(t *testing.T) {
	e := setup(t, true)
	ctx := t.Context()
	person, enquiry := e.lead(t, "c1", "Amal", "0501234567")
	e.configure(t, outreach.OutreachDocument{Channels: map[string]outreach.ChannelMode{"whatsapp": outreach.ModeApproved}, QuietStart: 21, QuietEnd: 9})
	d, err := e.out.Draft(ctx, coretest.Marketer, outreach.DraftInput{EnquiryID: enquiry, Channel: "whatsapp", Body: "Hi Amal, is The Wadi still of interest?"})
	require.NoError(t, err)
	assert.Equal(t, "draft", d.State)
	_, err = e.out.Send(ctx, coretest.Marketer, d.ID)
	require.Error(t, err, "unapproved drafts don't send")
	d, err = e.out.Approve(ctx, coretest.Marketer, d.ID)
	require.NoError(t, err)
	assert.Equal(t, "approved", d.State)

	_, err = e.out.Suppress(ctx, coretest.Marketer, person, "whatsapp", "replied STOP", "message")
	require.NoError(t, err)
	d, err = e.out.Send(ctx, coretest.Marketer, d.ID)
	require.NoError(t, err)
	assert.Equal(t, "suppressed", d.State)
	assert.Contains(t, d.Reason, "replied STOP")
	assert.Empty(t, e.messenger.sent)
	d, err = e.out.Send(ctx, coretest.Marketer, d.ID)
	require.NoError(t, err, "a retry runs the same checks")
	assert.Equal(t, "suppressed", d.State)
	assert.Empty(t, e.messenger.sent, "retry did not contact the person")
	b, err := e.out.NextAction(ctx, coretest.Viewer, enquiry)
	require.NoError(t, err)
	assert.Len(t, b.Suppressions, 1)

	sups := b.Suppressions
	_, err = e.out.Lift(ctx, coretest.Marketer, sups[0].ID, "they asked again")
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.out.Lift(ctx, coretest.Admin, sups[0].ID, "")
	require.Error(t, err)
	_, err = e.out.Lift(ctx, coretest.Admin, sups[0].ID, "customer wrote in on 2026-10-17 asking for updates")
	require.NoError(t, err)
	d, err = e.out.Send(ctx, coretest.Marketer, d.ID)
	require.NoError(t, err)
	assert.Equal(t, "sent", d.State)
	assert.Equal(t, "wamid-1", d.ProviderRef)
	require.Len(t, e.messenger.sent, 1)
	assert.Equal(t, "+971501234567", e.messenger.sent[0].To)
	assert.Equal(t, d.IdempotencyKey, e.messenger.sent[0].IdempotencyKey)
	_, err = e.out.Send(ctx, coretest.Marketer, d.ID)
	require.Error(t, err, "sent is final")
	// An all-channel opt-out makes the next step stop.
	_, err = e.out.Suppress(ctx, coretest.Marketer, person, "all", "asked never to be contacted", "call")
	require.NoError(t, err)
	b, _ = e.out.NextAction(ctx, coretest.Viewer, enquiry)
	assert.Equal(t, "stop", b.Next.Action)
}

// Catches: a message going out with no configuration, with a channel off,
// or with no adapter; and a draft quoting an unapproved figure being
// approvable.
func TestAutomaticMessagingStaysOffUntilConfigured(t *testing.T) {
	e := setup(t, false)
	ctx := t.Context()
	_, enquiry := e.lead(t, "c1", "Amal", "0501234567")
	st, err := e.out.GetConfig(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.False(t, st.Configured)
	assert.Contains(t, st.Messaging.Reason, "No messaging adapter")
	assert.Contains(t, st.FieldWrites.Reason, "No CRM field writer")

	d, err := e.out.Draft(ctx, coretest.Marketer, outreach.DraftInput{EnquiryID: enquiry, Channel: "whatsapp", Body: "Hi Amal, still interested in The Wadi?"})
	require.NoError(t, err)
	d, err = e.out.Approve(ctx, coretest.Marketer, d.ID)
	require.NoError(t, err)
	d, err = e.out.Send(ctx, coretest.Marketer, d.ID)
	require.NoError(t, err)
	assert.Equal(t, "held", d.State)
	assert.Contains(t, d.Reason, "not configured")

	e.configure(t, outreach.OutreachDocument{Channels: map[string]outreach.ChannelMode{"whatsapp": outreach.ModeOff}})
	d, _ = e.out.Send(ctx, coretest.Marketer, d.ID)
	assert.Equal(t, "held", d.State)
	assert.Contains(t, d.Reason, "is off")

	e.configure(t, outreach.OutreachDocument{Channels: map[string]outreach.ChannelMode{"whatsapp": outreach.ModeAutomatic}})
	d, _ = e.out.Send(ctx, coretest.Marketer, d.ID)
	assert.Equal(t, "held", d.State)
	assert.Contains(t, d.Reason, "no messaging adapter")
	st, _ = e.out.GetConfig(ctx, coretest.Viewer)
	assert.False(t, st.Messaging.Automatic, "automatic is never claimed")

	// Unapproved figure: the draft is held and can't be approved.
	held, err := e.out.Draft(ctx, coretest.Marketer, outreach.DraftInput{EnquiryID: enquiry, Channel: "whatsapp", Body: "Prices start from AED 1,200,000 with 10% down."})
	require.NoError(t, err)
	assert.Equal(t, "held", held.State)
	assert.Contains(t, held.Reason, "not an approved fact")
	require.NotNil(t, held.Claims)
	_, err = e.out.Approve(ctx, coretest.Marketer, held.ID)
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
	_, err = e.out.PutConfig(ctx, coretest.Marketer, outreach.SuggestedConfig())
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.out.PutConfig(ctx, coretest.Admin, outreach.OutreachDocument{Channels: map[string]outreach.ChannelMode{"fax": outreach.ModeOff}})
	require.Error(t, err)
}

// Catches: a next step that ignores an open objection, a draft quoting a
// figure that isn't approved, a preference not changing the step, and a
// closed enquiry still being worked.
func TestNextActionUsesFactsObjectionsAndPreferences(t *testing.T) {
	e := setup(t, false)
	ctx := t.Context()
	person, enquiry := e.lead(t, "c1", "Amal", "0501234567")
	b, err := e.out.NextAction(ctx, coretest.Viewer, enquiry)
	require.NoError(t, err)
	assert.Equal(t, "wait", b.Next.Action, "active today")
	assert.Equal(t, "The Wadi", b.ProjectName)
	assert.Empty(t, b.Claims)

	// Price objection with no approved fact: call.
	obj, err := e.out.RecordObjection(ctx, coretest.Marketer, outreach.ObjectionInput{PersonID: person, EnquiryID: enquiry, Kind: "price", Text: "too expensive for a 1BR", Source: "call"})
	require.NoError(t, err)
	assert.NotNil(t, obj.EvidenceID)
	b, _ = e.out.NextAction(ctx, coretest.Viewer, enquiry)
	assert.Equal(t, "call", b.Next.Action)
	assert.Contains(t, b.Next.Reason, "no approved fact")

	// An approved price fact answers it: message, with the figure.
	fc, err := e.brands.AddFact(ctx, coretest.Admin, e.brand, brands.FactInput{ProjectID: e.project, Key: "price.starting", Kind: brands.KindMoney, Value: "150000000", Currency: "AED", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceManual, SourceRef: "price list"})
	require.NoError(t, err)
	_, err = e.brands.ApproveFact(ctx, coretest.Admin, fc.Fact.ID)
	require.NoError(t, err)
	b, _ = e.out.NextAction(ctx, coretest.Viewer, enquiry)
	assert.Equal(t, "message", b.Next.Action)
	assert.Contains(t, b.Next.Draft, "AED 1,500,000")
	assert.Len(t, b.Claims, 1)
	d, err := e.out.Draft(ctx, coretest.Marketer, outreach.DraftInput{EnquiryID: enquiry, Channel: "whatsapp"})
	require.NoError(t, err)
	assert.Equal(t, "draft", d.State, "the generated draft quotes an approved figure")
	assert.Equal(t, b.Next.Draft, d.Body)
	require.NotNil(t, d.Claims)
	assert.Equal(t, "approved", d.Claims.Claims[0].Status)
	b, _ = e.out.NextAction(ctx, coretest.Viewer, enquiry)
	assert.Equal(t, "message", b.Next.Action)
	assert.Contains(t, b.Next.Reason, "draft is waiting")

	// Arabic preference changes the draft language.
	_, err = e.crm.SetStated(ctx, coretest.Marketer, person, "language", "ar", "call")
	require.NoError(t, err)
	_, err = e.out.ResolveObjection(ctx, coretest.Marketer, obj.ID, "sent the price list")
	require.NoError(t, err)
	e.clock.Advance(8 * 24 * time.Hour)
	b, _ = e.out.NextAction(ctx, coretest.Viewer, enquiry)
	assert.Equal(t, "message", b.Next.Action, "a draft is still waiting")
	assert.Equal(t, "ar", b.Preferences.Language)
	assert.Equal(t, 8, b.DaysSince)

	// Do not contact stops everything; a closed enquiry has no step.
	_, err = e.crm.SetStated(ctx, coretest.Marketer, person, "do_not_contact", "yes", "call")
	require.NoError(t, err)
	b, _ = e.out.NextAction(ctx, coretest.Viewer, enquiry)
	assert.Equal(t, "stop", b.Next.Action)
	assert.True(t, b.Preferences.DoNotContact)
	_, err = e.out.NextAction(ctx, coretest.Viewer, 999)
	require.Error(t, err)
}

// Catches: a combined adapter sending on a channel nobody serves, the first
// adapter not winning a shared channel, and nil adapters counting.
func TestCombineRoutesByChannel(t *testing.T) {
	wa := &fakeMessenger{}
	mail := &namedMessenger{name: "smtp", channels: []string{"email", "whatsapp"}}
	assert.Nil(t, outreach.Combine(nil, nil))
	assert.Same(t, wa, outreach.Combine(wa).(*fakeMessenger))
	both := outreach.Combine(nil, wa, mail)
	assert.Equal(t, "fake-wa+smtp", both.Name())
	assert.Equal(t, []string{"email", "whatsapp"}, both.Channels())

	ref, err := both.Send(t.Context(), outreach.OutboundMessage{Channel: "whatsapp", To: "+971500000000", Body: "x"})
	require.NoError(t, err)
	assert.Equal(t, "wamid-1", ref)
	require.Len(t, wa.sent, 1, "the first adapter given keeps a shared channel")
	assert.Empty(t, mail.sent)
	_, err = both.Send(t.Context(), outreach.OutboundMessage{Channel: "email", To: "a@b.c", Body: "x"})
	require.NoError(t, err)
	assert.Len(t, mail.sent, 1)
	_, err = both.Send(t.Context(), outreach.OutboundMessage{Channel: "sms", To: "+971500000000", Body: "x"})
	require.Error(t, err)
}

type namedMessenger struct {
	name     string
	channels []string
	sent     []outreach.OutboundMessage
}

func (m *namedMessenger) Name() string       { return m.name }
func (m *namedMessenger) Channels() []string { return m.channels }
func (m *namedMessenger) Send(_ context.Context, msg outreach.OutboundMessage) (string, error) {
	m.sent = append(m.sent, msg)
	return m.name + "-ref", nil
}
