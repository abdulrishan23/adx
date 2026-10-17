// Package outreach turns what ADX knows about a person and their enquiry
// into a next step, keeps the objections and preferences that shape it,
// drafts messages from approved facts only, and keeps GoHighLevel fields in
// step through a field-owned outbox. Nothing here contacts a customer
// unless a channel mode is configured, the person is not suppressed, and a
// messenger is wired; none is.
package outreach

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/store"
)

// FieldWriter sets custom fields on a CRM contact. providers/gohighlevel
// implements it; cmd/adx passes it in only when live writes are on.
type FieldWriter interface {
	Name() string
	SetContactFields(ctx context.Context, token, contactID string, fields map[string]string) error
}

// Tokens hands out the CRM access token for a location.
type Tokens interface {
	GHLToken(ctx context.Context, actor core.Actor, locationID string) (string, error)
}

// Messenger sends one message on one channel. No implementation is wired;
// the fake in outreachtest is the only one.
type Messenger interface {
	Name() string
	Channels() []string
	Send(ctx context.Context, msg OutboundMessage) (providerRef string, err error)
}

type OutboundMessage struct {
	Channel  string
	To       string
	Language string
	Body     string
	// IdempotencyKey is the draft's key so a retried send is the same send.
	IdempotencyKey string
}

// ChannelMode says what ADX may do on a channel. Off is the default and
// means drafts only. Approved means a marketer-approved draft can be sent.
// Automatic is accepted in the document but never acted on by this build:
// the policy owner has not supplied a messaging policy, so it behaves as
// approved and the config says so.
type ChannelMode string

const (
	ModeOff       ChannelMode = "off"
	ModeApproved  ChannelMode = "approved"
	ModeAutomatic ChannelMode = "automatic"
)

// FieldOwner says who wins when ADX and the CRM disagree on a field.
type FieldOwner string

const (
	OwnerADX FieldOwner = "adx"
	OwnerCRM FieldOwner = "crm"
)

type OutreachDocument struct {
	// Fields maps a CRM custom field id (or standard field) to its owner.
	Fields map[string]FieldOwner `json:"fields"`
	// Channels maps a channel to its mode. Missing is off.
	Channels map[string]ChannelMode `json:"channels"`
	// QuietHours in Dubai time, 24h clock; messages are held outside them.
	QuietStart int    `json:"quietStart"`
	QuietEnd   int    `json:"quietEnd"`
	Note       string `json:"note"`
}

type OutreachConfig struct {
	Version   int64            `json:"version"`
	Document  OutreachDocument `json:"document"`
	CreatedBy *int64           `json:"createdBy"`
	CreatedAt time.Time        `json:"createdAt"`
}

type OutreachState struct {
	Configured bool            `json:"configured"`
	Current    *OutreachConfig `json:"current"`
	// Messaging says whether any channel can send: a configured mode, a
	// wired messenger, and the channels it serves.
	Messaging MessagingState `json:"messaging"`
	// FieldWrites says whether the CRM outbox can dispatch.
	FieldWrites FieldWriteState  `json:"fieldWrites"`
	Suggested   OutreachDocument `json:"suggested"`
}

type MessagingState struct {
	Adapter   string   `json:"adapter"`
	Channels  []string `json:"channels"`
	Automatic bool     `json:"automatic"`
	Reason    string   `json:"reason"`
}

type FieldWriteState struct {
	Adapter string `json:"adapter"`
	Reason  string `json:"reason"`
}

type Objection struct {
	ID         int64      `json:"id"`
	PersonID   int64      `json:"personId"`
	EnquiryID  *int64     `json:"enquiryId"`
	Kind       string     `json:"kind"`
	Text       string     `json:"text"`
	Source     string     `json:"source"`
	EvidenceID *int64     `json:"evidenceId"`
	RecordedBy *int64     `json:"recordedBy"`
	RecordedAt time.Time  `json:"recordedAt"`
	ResolvedAt *time.Time `json:"resolvedAt"`
	Resolution string     `json:"resolution"`
}

type ObjectionInput struct {
	PersonID  int64  `json:"personId"`
	EnquiryID int64  `json:"enquiryId"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Source    string `json:"source"`
}

type Suppression struct {
	ID         int64      `json:"id"`
	PersonID   int64      `json:"personId"`
	Channel    string     `json:"channel"`
	Reason     string     `json:"reason"`
	Source     string     `json:"source"`
	Since      time.Time  `json:"since"`
	LiftedAt   *time.Time `json:"liftedAt"`
	LiftedBy   *int64     `json:"liftedBy"`
	LiftReason string     `json:"liftReason"`
}

type FieldWrite struct {
	ID             int64           `json:"id"`
	PersonID       int64           `json:"personId"`
	AccountID      int64           `json:"accountId"`
	ExternalID     string          `json:"externalId"`
	Field          string          `json:"field"`
	Value          string          `json:"value"`
	State          string          `json:"state"`
	Reason         string          `json:"reason"`
	Attempts       int64           `json:"attempts"`
	IdempotencyKey string          `json:"idempotencyKey"`
	Conflict       json.RawMessage `json:"conflict"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	SentAt         *time.Time      `json:"sentAt"`
	ConfirmedAt    *time.Time      `json:"confirmedAt"`
}

type MessageDraft struct {
	ID             int64               `json:"id"`
	PersonID       int64               `json:"personId"`
	EnquiryID      *int64              `json:"enquiryId"`
	Channel        string              `json:"channel"`
	Language       string              `json:"language"`
	Body           string              `json:"body"`
	Claims         *brands.ClaimReport `json:"claims"`
	State          string              `json:"state"`
	Reason         string              `json:"reason"`
	Attempts       int64               `json:"attempts"`
	IdempotencyKey string              `json:"idempotencyKey"`
	ProviderRef    string              `json:"providerRef"`
	CreatedBy      *int64              `json:"createdBy"`
	ApprovedBy     *int64              `json:"approvedBy"`
	CreatedAt      time.Time           `json:"createdAt"`
	UpdatedAt      time.Time           `json:"updatedAt"`
	SentAt         *time.Time          `json:"sentAt"`
}

type DraftInput struct {
	PersonID  int64  `json:"personId"`
	EnquiryID int64  `json:"enquiryId"`
	Channel   string `json:"channel"`
	// Body empty asks for a generated draft from the brief.
	Body string `json:"body"`
}

// NextStep is the recommended action and why.
type NextStep struct {
	Action string `json:"action"` // call, message, wait, stop, none
	Reason string `json:"reason"`
	// Draft is a suggested message when Action is message. It is not
	// stored until someone asks for a draft.
	Draft string `json:"draft"`
}

// Preferences are what the person told us about how to be contacted.
type Preferences struct {
	Language         string `json:"language"`
	PreferredChannel string `json:"preferredChannel"`
	DoNotContact     bool   `json:"doNotContact"`
}

// SalesBrief is the sales brief for one enquiry.
type SalesBrief struct {
	Enquiry      crm.Enquiry              `json:"enquiry"`
	Person       crm.Person               `json:"person"`
	ProjectName  string                   `json:"projectName"`
	Score        *signals.ScoreAssessment `json:"score"`
	Claims       []brands.Fact            `json:"claims"`
	ClaimsNote   string                   `json:"claimsNote"`
	Objections   []Objection              `json:"objections"`
	Preferences  Preferences              `json:"preferences"`
	Suppressions []Suppression            `json:"suppressions"`
	Attributes   []crm.AttributeView      `json:"attributes"`
	Drafts       []MessageDraft           `json:"drafts"`
	FieldWrites  []FieldWrite             `json:"fieldWrites"`
	Next         NextStep                 `json:"next"`
	LastActivity time.Time                `json:"lastActivity"`
	DaysSince    int                      `json:"daysSinceActivity"`
}

type Config struct {
	Clock     core.Clock
	Brands    *brands.Brands
	Signals   *signals.Signals
	CRM       *crm.CRM
	Policies  *policies.Policies
	Fields    FieldWriter
	Tokens    Tokens
	Messenger Messenger
	// Location returns the GoHighLevel location id for an account; the
	// account's external id is the location.
}

type Outreach struct {
	db       *store.DB
	evidence *evidence.Store
	cfg      Config
	clock    core.Clock
}

func New(db *store.DB, ev *evidence.Store, cfg Config) *Outreach {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Outreach{db: db, evidence: ev, cfg: cfg, clock: cfg.Clock}
}

// Attach sets the dependencies that are built after the leads import that
// observes through this service. It is called once at startup.
func (o *Outreach) Attach(b *brands.Brands, s *signals.Signals, c *crm.CRM, p *policies.Policies) {
	o.cfg.Brands, o.cfg.Signals, o.cfg.CRM, o.cfg.Policies = b, s, c, p
}

// SuggestedConfig is a starting point; nothing uses it until saved.
func SuggestedConfig() OutreachDocument {
	return OutreachDocument{
		Fields:     map[string]FieldOwner{"adx_temperature": OwnerADX, "adx_intent": OwnerADX, "adx_fit": OwnerADX},
		Channels:   map[string]ChannelMode{"whatsapp": ModeOff, "sms": ModeOff, "email": ModeOff},
		QuietStart: 21, QuietEnd: 9,
		Note: "Suggested: ADX owns its own score fields; every channel is off until Reportage supplies a messaging policy.",
	}
}

func (o *Outreach) GetConfig(ctx context.Context, actor core.Actor) (OutreachState, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return OutreachState{}, core.ErrForbidden
	}
	cur, err := o.current(ctx, actor.WorkspaceID)
	if err != nil {
		return OutreachState{}, err
	}
	st := OutreachState{Configured: cur != nil, Current: cur, Suggested: SuggestedConfig()}
	st.Messaging = o.messagingState(cur)
	st.FieldWrites = o.fieldWriteState()
	return st, nil
}

func (o *Outreach) messagingState(cur *OutreachConfig) MessagingState {
	ms := MessagingState{Channels: []string{}}
	if o.cfg.Messenger == nil {
		ms.Reason = "No messaging adapter is wired on this server; drafts can be written and approved but not sent."
		return ms
	}
	ms.Adapter = o.cfg.Messenger.Name()
	ms.Channels = o.cfg.Messenger.Channels()
	if cur == nil {
		ms.Reason = "Outreach is not configured; every channel is off."
		return ms
	}
	on := 0
	for _, ch := range ms.Channels {
		if m := cur.Document.Channels[ch]; m == ModeApproved || m == ModeAutomatic {
			on++
		}
	}
	if on == 0 {
		ms.Reason = "Every channel is off in the current configuration."
	}
	return ms
}

func (o *Outreach) fieldWriteState() FieldWriteState {
	if o.cfg.Fields == nil {
		return FieldWriteState{Reason: "No CRM field writer is wired (ADX_LIVE_WRITES is off); queued field values are held."}
	}
	return FieldWriteState{Adapter: o.cfg.Fields.Name()}
}

func (o *Outreach) PutConfig(ctx context.Context, actor core.Actor, doc OutreachDocument) (OutreachConfig, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return OutreachConfig{}, core.ErrForbidden
	}
	for f, owner := range doc.Fields {
		if strings.TrimSpace(f) == "" || (owner != OwnerADX && owner != OwnerCRM) {
			return OutreachConfig{}, core.Invalid("field %q needs owner adx or crm", f)
		}
	}
	for ch, mode := range doc.Channels {
		switch ch {
		case "whatsapp", "sms", "email":
		default:
			return OutreachConfig{}, core.Invalid("unknown channel %q", ch)
		}
		switch mode {
		case ModeOff, ModeApproved, ModeAutomatic:
		default:
			return OutreachConfig{}, core.Invalid("channel %q needs mode off, approved or automatic", ch)
		}
	}
	if doc.QuietStart < 0 || doc.QuietStart > 23 || doc.QuietEnd < 0 || doc.QuietEnd > 23 {
		return OutreachConfig{}, core.Invalid("quiet hours are 0-23")
	}
	raw, _ := json.Marshal(doc)
	var out OutreachConfig
	err := o.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var version int64 = 1
		if cur, err := q.GetCurrentOutreachConfig(ctx, actor.WorkspaceID); err == nil {
			version = cur.Version + 1
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		row, err := q.CreateOutreachConfig(ctx, store.CreateOutreachConfigParams{WorkspaceID: actor.WorkspaceID, Version: version, Document: string(raw), CreatedBy: optionalID(actor.UserID), CreatedAt: o.clock.Now().UTC()})
		if err != nil {
			return err
		}
		out, err = configFromRow(row)
		return err
	})
	return out, err
}

// RecordObjection notes something the customer pushed back on.
func (o *Outreach) RecordObjection(ctx context.Context, actor core.Actor, in ObjectionInput) (Objection, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return Objection{}, core.ErrForbidden
	}
	switch in.Kind {
	case "price", "location", "timing", "payment", "trust", "unit", "other":
	default:
		return Objection{}, core.Invalid("objection kind must be price, location, timing, payment, trust, unit or other")
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" {
		return Objection{}, core.Invalid("say what the customer objected to")
	}
	if in.Source == "" {
		in.Source = "note"
	}
	if _, err := o.person(ctx, actor.WorkspaceID, in.PersonID); err != nil {
		return Objection{}, err
	}
	if in.EnquiryID != 0 {
		enq, err := o.enquiry(ctx, actor.WorkspaceID, in.EnquiryID)
		if err != nil {
			return Objection{}, err
		}
		if enq.PersonID != in.PersonID {
			return Objection{}, core.Invalid("enquiry %d belongs to another person", in.EnquiryID)
		}
	}
	now := o.clock.Now().UTC()
	var evID int64
	if o.evidence != nil {
		ev, _, err := o.evidence.Record(ctx, actor, evidence.Observation{Source: "manual", Kind: "objection", SourceRef: fmt.Sprintf("person:%d/%s", in.PersonID, now.Format(time.RFC3339Nano)), Scope: fmt.Sprintf("person:%d", in.PersonID), Class: evidence.Observed, EventAt: &now,
			Payload: map[string]any{"kind": in.Kind, "text": in.Text, "source": in.Source}, Completeness: evidence.Complete})
		if err != nil {
			return Objection{}, err
		}
		evID = ev.ID
	}
	row, err := o.db.Writer().CreateObjection(ctx, store.CreateObjectionParams{WorkspaceID: actor.WorkspaceID, PersonID: in.PersonID, EnquiryID: optionalID(in.EnquiryID), Kind: in.Kind, Text: in.Text, Source: in.Source, EvidenceID: optionalID(evID), RecordedBy: optionalID(actor.UserID), RecordedAt: now})
	if err != nil {
		return Objection{}, err
	}
	return objectionFromRow(row), nil
}

func (o *Outreach) ResolveObjection(ctx context.Context, actor core.Actor, id int64, resolution string) (Objection, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return Objection{}, core.ErrForbidden
	}
	resolution = strings.TrimSpace(resolution)
	if resolution == "" {
		return Objection{}, core.Invalid("say how it was resolved")
	}
	if _, err := o.db.Reader().GetObjection(ctx, store.GetObjectionParams{ID: id, WorkspaceID: actor.WorkspaceID}); errors.Is(err, sql.ErrNoRows) {
		return Objection{}, core.NotFound("objection %d not found", id)
	} else if err != nil {
		return Objection{}, err
	}
	n, err := o.db.Writer().ResolveObjection(ctx, store.ResolveObjectionParams{ResolvedAt: sql.NullTime{Time: o.clock.Now().UTC(), Valid: true}, Resolution: resolution, ID: id})
	if err != nil {
		return Objection{}, err
	}
	if n != 1 {
		return Objection{}, core.Conflict("objection %d is already resolved", id)
	}
	row, err := o.db.Reader().GetObjection(ctx, store.GetObjectionParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return Objection{}, err
	}
	return objectionFromRow(row), nil
}

// Suppress records an opt-out. Source says where it came from (the
// customer's message, a call, a complaint). It is checked at every dispatch.
func (o *Outreach) Suppress(ctx context.Context, actor core.Actor, personID int64, channel, reason, source string) (Suppression, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Suppression{}, core.ErrForbidden
	}
	switch channel {
	case "all", "whatsapp", "sms", "email", "call":
	default:
		return Suppression{}, core.Invalid("channel must be all, whatsapp, sms, email or call")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Suppression{}, core.Invalid("a reason is required")
	}
	if source == "" {
		source = "manual"
	}
	if _, err := o.person(ctx, actor.WorkspaceID, personID); err != nil {
		return Suppression{}, err
	}
	row, err := o.db.Writer().CreateSuppression(ctx, store.CreateSuppressionParams{WorkspaceID: actor.WorkspaceID, PersonID: personID, Channel: channel, Reason: reason, Source: source, Since: o.clock.Now().UTC()})
	if err != nil {
		return Suppression{}, err
	}
	return suppressionFromRow(row), nil
}

// Lift ends a suppression. Admin only: an opt-out is the customer's, and
// undoing it needs their renewed consent on record in the reason.
func (o *Outreach) Lift(ctx context.Context, actor core.Actor, id int64, reason string) (Suppression, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return Suppression{}, core.ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Suppression{}, core.Invalid("say how consent was renewed")
	}
	if _, err := o.db.Reader().GetSuppression(ctx, store.GetSuppressionParams{ID: id, WorkspaceID: actor.WorkspaceID}); errors.Is(err, sql.ErrNoRows) {
		return Suppression{}, core.NotFound("suppression %d not found", id)
	} else if err != nil {
		return Suppression{}, err
	}
	n, err := o.db.Writer().LiftSuppression(ctx, store.LiftSuppressionParams{LiftedAt: sql.NullTime{Time: o.clock.Now().UTC(), Valid: true}, LiftedBy: optionalID(actor.UserID), LiftReason: reason, ID: id})
	if err != nil {
		return Suppression{}, err
	}
	if n != 1 {
		return Suppression{}, core.Conflict("suppression %d is already lifted", id)
	}
	row, err := o.db.Reader().GetSuppression(ctx, store.GetSuppressionParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return Suppression{}, err
	}
	return suppressionFromRow(row), nil
}

// Suppressed says whether a channel is blocked for the person right now,
// for other domains (calling) that must check at dispatch.
func (o *Outreach) Suppressed(ctx context.Context, actor core.Actor, personID int64, channel string) (bool, string, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return false, "", core.ErrForbidden
	}
	return o.suppressed(ctx, personID, channel)
}

// suppressed says whether a channel is blocked for the person right now.
func (o *Outreach) suppressed(ctx context.Context, personID int64, channel string) (bool, string, error) {
	rows, err := o.db.Reader().ListActiveSuppressions(ctx, personID)
	if err != nil {
		return false, "", err
	}
	for _, r := range rows {
		if r.Channel == "all" || r.Channel == channel {
			return true, fmt.Sprintf("suppressed on %s since %s: %s", r.Channel, r.Since.UTC().Format("2006-01-02"), r.Reason), nil
		}
	}
	return false, "", nil
}

// NextAction builds the sales brief for an enquiry and recommends one step.
// The recommendation is a rule, not a model, and every input is on the
// brief so a salesperson can disagree with it.
func (o *Outreach) NextAction(ctx context.Context, actor core.Actor, enquiryID int64) (SalesBrief, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return SalesBrief{}, core.ErrForbidden
	}
	enq, err := o.enquiry(ctx, actor.WorkspaceID, enquiryID)
	if err != nil {
		return SalesBrief{}, err
	}
	b := SalesBrief{Enquiry: enq, Claims: []brands.Fact{}, Objections: []Objection{}, Suppressions: []Suppression{}, Attributes: []crm.AttributeView{}, Drafts: []MessageDraft{}, FieldWrites: []FieldWrite{}}
	if o.cfg.CRM != nil {
		d, err := o.cfg.CRM.Get(ctx, actor, enq.PersonID)
		if err != nil {
			return SalesBrief{}, err
		}
		b.Person = d.Person
		b.Attributes = d.Attributes
		for _, a := range d.Attributes {
			switch a.Key {
			case "language":
				b.Preferences.Language = a.Value
			case "preferred_channel":
				b.Preferences.PreferredChannel = a.Value
			case "do_not_contact":
				b.Preferences.DoNotContact = strings.EqualFold(a.Value, "yes") || strings.EqualFold(a.Value, "true")
			}
		}
		if b.Preferences.Language == "" && d.Person.Language != nil {
			b.Preferences.Language = *d.Person.Language
		}
	}
	if b.Preferences.Language == "" {
		b.Preferences.Language = "en"
	}
	if enq.ProjectID != nil {
		if p, err := o.db.Reader().GetProject(ctx, *enq.ProjectID); err == nil {
			b.ProjectName = p.Name
		}
		if o.cfg.Brands != nil {
			brandID, err := o.cfg.Brands.BrandOfProject(ctx, actor, *enq.ProjectID)
			switch {
			case err == nil && brandID != 0:
				cs, err := o.cfg.Brands.Claims(ctx, actor, brandID, *enq.ProjectID, o.clock.Now())
				if err != nil {
					return SalesBrief{}, err
				}
				b.Claims = cs.Claims
				if len(cs.Pending) > 0 {
					b.ClaimsNote = "Facts awaiting approval are not usable: " + strings.Join(cs.Pending, ", ")
				}
			case err == nil:
				b.ClaimsNote = "The project has no brand, so there are no approved facts to quote."
			default:
				var ce *core.Error
				if !errors.As(err, &ce) {
					return SalesBrief{}, err
				}
				b.ClaimsNote = "The project has no brand, so there are no approved facts to quote."
			}
		}
	} else {
		b.ClaimsNote = "The enquiry names no project, so there are no project facts to quote."
	}
	if o.cfg.Signals != nil {
		if v, err := o.cfg.Signals.Score(ctx, actor, enq.PersonID); err == nil {
			b.Score = v.Latest
		}
	}
	objs, err := o.db.Reader().ListObjectionsForPerson(ctx, enq.PersonID)
	if err != nil {
		return SalesBrief{}, err
	}
	for _, r := range objs {
		b.Objections = append(b.Objections, objectionFromRow(r))
	}
	sups, err := o.db.Reader().ListSuppressionsForPerson(ctx, enq.PersonID)
	if err != nil {
		return SalesBrief{}, err
	}
	for _, r := range sups {
		b.Suppressions = append(b.Suppressions, suppressionFromRow(r))
	}
	drafts, err := o.db.Reader().ListMessageDraftsForPerson(ctx, enq.PersonID)
	if err != nil {
		return SalesBrief{}, err
	}
	for _, r := range drafts {
		b.Drafts = append(b.Drafts, draftFromRow(r))
	}
	fw, err := o.db.Reader().ListFieldOutboxForPerson(ctx, enq.PersonID)
	if err != nil {
		return SalesBrief{}, err
	}
	for _, r := range fw {
		b.FieldWrites = append(b.FieldWrites, fieldWriteFromRow(r))
	}
	b.LastActivity = enq.LastActivityAt
	b.DaysSince = int(o.clock.Now().UTC().Sub(enq.LastActivityAt).Hours() / 24)
	b.Next = o.recommend(ctx, b)
	return b, nil
}

// recommend is the rule. Stop beats everything; then an open objection with
// an approved fact that answers it; then temperature and silence.
func (o *Outreach) recommend(ctx context.Context, b SalesBrief) NextStep {
	if b.Enquiry.State == "closed" {
		return NextStep{Action: "none", Reason: "The enquiry is closed."}
	}
	if b.Preferences.DoNotContact {
		return NextStep{Action: "stop", Reason: "The customer asked not to be contacted."}
	}
	for _, s := range b.Suppressions {
		if s.LiftedAt == nil && s.Channel == "all" {
			return NextStep{Action: "stop", Reason: "Opted out of all contact: " + s.Reason}
		}
	}
	for _, d := range b.Drafts {
		if d.State == "draft" || d.State == "approved" {
			return NextStep{Action: "message", Reason: fmt.Sprintf("A %s draft is waiting (%s).", d.Channel, d.State)}
		}
	}
	for _, obj := range b.Objections {
		if obj.ResolvedAt != nil {
			continue
		}
		if fact := answering(obj.Kind, b.Claims); fact != nil {
			return NextStep{Action: "message", Reason: fmt.Sprintf("Open %s objection; the approved fact %s answers it.", obj.Kind, fact.Key), Draft: o.composeAnswer(b, *obj.ID64(), *fact)}
		}
		return NextStep{Action: "call", Reason: fmt.Sprintf("Open %s objection with no approved fact to answer it; a conversation is needed.", obj.Kind)}
	}
	hot := b.Score != nil && b.Score.Temperature != nil && *b.Score.Temperature == "hot"
	switch {
	case hot && b.DaysSince >= 1:
		return NextStep{Action: "call", Reason: "Hot and quiet for a day or more."}
	case b.DaysSince < 2:
		return NextStep{Action: "wait", Reason: "Active in the last two days."}
	case b.DaysSince >= 7:
		return NextStep{Action: "message", Reason: fmt.Sprintf("No activity for %d days.", b.DaysSince), Draft: o.composeFollowUp(b)}
	default:
		return NextStep{Action: "wait", Reason: fmt.Sprintf("Quiet for %d days; follow up at seven.", b.DaysSince)}
	}
}

// answering finds an approved fact that speaks to an objection kind.
func answering(kind string, claims []brands.Fact) *brands.Fact {
	prefixes := map[string][]string{
		"price":    {"price", "offer.price", "unit.price", "starting_price"},
		"payment":  {"payment", "offer.payment", "payment_plan"},
		"timing":   {"handover", "completion", "launch"},
		"location": {"location", "address", "community", "district"},
		"unit":     {"unit", "sizes", "bedrooms"},
	}
	for _, f := range claims {
		if f.Class != brands.Approved {
			continue
		}
		for _, p := range prefixes[kind] {
			if f.Key == p || strings.HasPrefix(f.Key, p+".") || strings.HasPrefix(f.Key, p+"_") {
				ff := f
				return &ff
			}
		}
	}
	return nil
}

func (o *Outreach) composeAnswer(b SalesBrief, _ int64, fact brands.Fact) string {
	name := firstName(b.Person.DisplayName)
	value := fact.Value
	if fact.Kind == brands.KindMoney {
		if minor, err := strconv.ParseInt(fact.Value, 10, 64); err == nil {
			value = formatMoney(minor)
		}
	}
	if fact.Currency != nil && *fact.Currency != "" {
		value = *fact.Currency + " " + value
	}
	if strings.HasPrefix(b.Preferences.Language, "ar") {
		return fmt.Sprintf("مرحباً %s، بخصوص سؤالك عن %s: %s هو %s. هل تود أن نتصل بك لتوضيح التفاصيل؟", name, b.ProjectName, fact.Key, value)
	}
	return fmt.Sprintf("Hi %s, on your question about %s: the %s is %s. Would a short call help with the details?", name, b.ProjectName, strings.ReplaceAll(fact.Key, "_", " "), value)
}

func (o *Outreach) composeFollowUp(b SalesBrief) string {
	name := firstName(b.Person.DisplayName)
	if strings.HasPrefix(b.Preferences.Language, "ar") {
		return fmt.Sprintf("مرحباً %s، تواصلنا سابقاً بخصوص %s. هل ما زلت مهتماً؟ يسعدنا الإجابة على أي سؤال.", name, b.ProjectName)
	}
	return fmt.Sprintf("Hi %s, we spoke about %s a little while ago. Is it still of interest? Happy to answer any questions.", name, b.ProjectName)
}

// formatMoney writes minor units as a whole-unit figure with thousands
// separators, the way a price is quoted to a customer (AED 1,500,000).
func formatMoney(minor int64) string {
	whole := minor / 100
	neg := whole < 0
	if neg {
		whole = -whole
	}
	s := strconv.FormatInt(whole, 10)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func firstName(display string) string {
	parts := strings.Fields(display)
	if len(parts) == 0 {
		return "there"
	}
	return parts[0]
}

// Draft stores a message with its claim check. A body with an unapproved
// figure is stored held, with the claims that failed; it cannot be approved.
func (o *Outreach) Draft(ctx context.Context, actor core.Actor, in DraftInput) (MessageDraft, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return MessageDraft{}, core.ErrForbidden
	}
	switch in.Channel {
	case "whatsapp", "sms", "email":
	default:
		return MessageDraft{}, core.Invalid("channel must be whatsapp, sms or email")
	}
	if in.EnquiryID == 0 {
		return MessageDraft{}, core.Invalid("a draft belongs to an enquiry")
	}
	b, err := o.NextAction(ctx, actor, in.EnquiryID)
	if err != nil {
		return MessageDraft{}, err
	}
	if in.PersonID != 0 && in.PersonID != b.Enquiry.PersonID {
		return MessageDraft{}, core.Invalid("enquiry %d belongs to another person", in.EnquiryID)
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		body = b.Next.Draft
		if body == "" {
			body = o.composeFollowUp(b)
		}
	}
	state, reason := "draft", ""
	var report *brands.ClaimReport
	if o.cfg.Brands != nil && b.Enquiry.ProjectID != nil {
		if brandID, err := o.cfg.Brands.BrandOfProject(ctx, actor, *b.Enquiry.ProjectID); err == nil && brandID != 0 {
			r, err := o.cfg.Brands.CheckCopy(ctx, actor, brandID, *b.Enquiry.ProjectID, 0, 0, body)
			if err != nil {
				return MessageDraft{}, err
			}
			report = &r
			for _, c := range r.Claims {
				if c.Status != "approved" {
					state, reason = "held", "the draft quotes "+c.Text+" which is not an approved fact"
					break
				}
			}
		}
	}
	claims, _ := json.Marshal(report)
	if report == nil {
		claims = []byte("{}")
	}
	now := o.clock.Now().UTC()
	key := fmt.Sprintf("draft:%d:%s:%d:%s", b.Enquiry.PersonID, in.Channel, now.UnixNano(), nonce())
	row, err := o.db.Writer().CreateMessageDraft(ctx, store.CreateMessageDraftParams{WorkspaceID: actor.WorkspaceID, PersonID: b.Enquiry.PersonID, EnquiryID: optionalID(in.EnquiryID), Channel: in.Channel, Language: b.Preferences.Language, Body: body, Claims: string(claims), State: state, Reason: reason, IdempotencyKey: key, CreatedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return MessageDraft{}, err
	}
	return draftFromRow(row), nil
}

func (o *Outreach) Approve(ctx context.Context, actor core.Actor, draftID int64) (MessageDraft, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return MessageDraft{}, core.ErrForbidden
	}
	row, err := o.draft(ctx, actor.WorkspaceID, draftID)
	if err != nil {
		return MessageDraft{}, err
	}
	if row.State == "held" {
		return MessageDraft{}, core.Conflict("draft %d is held: %s", draftID, row.Reason)
	}
	if row.State != "draft" {
		return MessageDraft{}, core.Conflict("draft %d is %s, not draft", draftID, row.State)
	}
	now := o.clock.Now().UTC()
	n, err := o.db.Writer().SetMessageDraftState(ctx, store.SetMessageDraftStateParams{State: "approved", Reason: "", ApprovedBy: optionalID(actor.UserID), UpdatedAt: now, ID: draftID, State_2: "draft"})
	if err != nil {
		return MessageDraft{}, err
	}
	if n != 1 {
		return MessageDraft{}, core.Conflict("draft %d changed underneath", draftID)
	}
	return o.get(ctx, actor.WorkspaceID, draftID)
}

// Send dispatches an approved draft if, and only if: the channel's mode is
// configured on, a messenger serving the channel is wired, and the person
// is not suppressed on the channel. Every check runs again on a retry.
func (o *Outreach) Send(ctx context.Context, actor core.Actor, draftID int64) (MessageDraft, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return MessageDraft{}, core.ErrForbidden
	}
	row, err := o.draft(ctx, actor.WorkspaceID, draftID)
	if err != nil {
		return MessageDraft{}, err
	}
	switch row.State {
	case "approved", "held", "failed":
	case "suppressed":
		// Re-check: a lifted suppression may allow it now.
	default:
		return MessageDraft{}, core.Conflict("draft %d is %s; only an approved draft is sent", draftID, row.State)
	}
	if !row.ApprovedBy.Valid {
		return MessageDraft{}, core.Conflict("draft %d was never approved", draftID)
	}
	now := o.clock.Now().UTC()
	hold := func(state, reason string) (MessageDraft, error) {
		if _, err := o.db.Writer().SetMessageDraftState(ctx, store.SetMessageDraftStateParams{State: state, Reason: reason, ApprovedBy: sql.NullInt64{}, UpdatedAt: now, ID: draftID, State_2: row.State}); err != nil {
			return MessageDraft{}, err
		}
		return o.get(ctx, actor.WorkspaceID, draftID)
	}
	if blocked, why, err := o.suppressed(ctx, row.PersonID, row.Channel); err != nil {
		return MessageDraft{}, err
	} else if blocked {
		return hold("suppressed", why+"; not sent")
	}
	if o.cfg.Policies != nil {
		if engaged, why, err := o.cfg.Policies.KillSwitchEngaged(ctx, actor.WorkspaceID, 0); err != nil {
			return MessageDraft{}, err
		} else if engaged {
			return hold("held", "kill switch engaged: "+why+"; nothing is sent")
		}
	}
	cur, err := o.current(ctx, actor.WorkspaceID)
	if err != nil {
		return MessageDraft{}, err
	}
	if cur == nil {
		return hold("held", "outreach is not configured: no channel may send")
	}
	mode := cur.Document.Channels[row.Channel]
	if mode != ModeApproved && mode != ModeAutomatic {
		return hold("held", fmt.Sprintf("channel %s is off in outreach config v%d", row.Channel, cur.Version))
	}
	if o.cfg.Messenger == nil {
		return hold("held", "no messaging adapter is wired on this server")
	}
	serves := false
	for _, ch := range o.cfg.Messenger.Channels() {
		if ch == row.Channel {
			serves = true
		}
	}
	if !serves {
		return hold("held", fmt.Sprintf("adapter %s does not serve %s", o.cfg.Messenger.Name(), row.Channel))
	}
	if quiet(cur.Document, now) {
		return hold("held", "quiet hours in Dubai; try again in the morning")
	}
	to, err := o.address(ctx, row.PersonID, row.Channel)
	if err != nil {
		return hold("held", err.Error())
	}
	ref, sendErr := o.cfg.Messenger.Send(ctx, OutboundMessage{Channel: row.Channel, To: to, Language: row.Language, Body: row.Body, IdempotencyKey: row.IdempotencyKey})
	if sendErr != nil {
		if _, err := o.db.Writer().SetMessageDraftState(ctx, store.SetMessageDraftStateParams{State: "failed", Reason: safe(sendErr), Attempts: 1, ApprovedBy: sql.NullInt64{}, UpdatedAt: now, ID: draftID, State_2: row.State}); err != nil {
			return MessageDraft{}, err
		}
		return o.get(ctx, actor.WorkspaceID, draftID)
	}
	if _, err := o.db.Writer().SetMessageDraftState(ctx, store.SetMessageDraftStateParams{State: "sent", Reason: "", Attempts: 1, ApprovedBy: sql.NullInt64{}, UpdatedAt: now, SentAt: sql.NullTime{Time: now, Valid: true}, ID: draftID, State_2: row.State}); err != nil {
		return MessageDraft{}, err
	}
	if err := o.db.Writer().SetMessageDraftProviderRef(ctx, store.SetMessageDraftProviderRefParams{ProviderRef: ref, ID: draftID}); err != nil {
		return MessageDraft{}, err
	}
	if o.cfg.Signals != nil {
		payload, _ := json.Marshal(map[string]any{"draftId": draftID, "channel": row.Channel})
		_, _ = o.cfg.Signals.Ingest(ctx, actor, signals.SignalInput{Source: "messaging", Kind: "message_reply", OccurrenceKey: "sent:" + row.IdempotencyKey, OccurredAt: now, Identity: signals.SignalIdentity{PersonID: row.PersonID}, Payload: payload})
	}
	return o.get(ctx, actor.WorkspaceID, draftID)
}

// quiet says whether now falls in the configured quiet hours, Dubai time.
func quiet(doc OutreachDocument, now time.Time) bool {
	if doc.QuietStart == doc.QuietEnd {
		return false
	}
	h := now.In(reporting.ReportingZone).Hour()
	if doc.QuietStart < doc.QuietEnd {
		return h >= doc.QuietStart && h < doc.QuietEnd
	}
	return h >= doc.QuietStart || h < doc.QuietEnd
}

func (o *Outreach) address(ctx context.Context, personID int64, channel string) (string, error) {
	p, err := o.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: personID, WorkspaceID: core.DefaultWorkspace})
	if err != nil {
		return "", err
	}
	switch channel {
	case "email":
		if !p.PrimaryEmail.Valid {
			return "", errors.New("the person has no email on record")
		}
		return p.PrimaryEmail.String, nil
	default:
		if !p.PrimaryPhone.Valid {
			return "", errors.New("the person has no phone on record")
		}
		return p.PrimaryPhone.String, nil
	}
}

// QueueFieldWrite asks for a CRM field to carry a value for every lead
// linked to the person. The same value twice is the same row.
func (o *Outreach) QueueFieldWrite(ctx context.Context, actor core.Actor, personID int64, field, value string) ([]FieldWrite, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return nil, core.ErrForbidden
	}
	field = strings.TrimSpace(field)
	if field == "" {
		return nil, core.Invalid("a field is required")
	}
	if _, err := o.person(ctx, actor.WorkspaceID, personID); err != nil {
		return nil, err
	}
	leads, err := o.db.Reader().ListLeadsForPerson(ctx, personID)
	if err != nil {
		return nil, err
	}
	if len(leads) == 0 {
		return nil, core.Conflict("person %d has no CRM record to write to", personID)
	}
	now := o.clock.Now().UTC()
	out := []FieldWrite{}
	for _, l := range leads {
		key := fmt.Sprintf("field:%d:%s:%s:%s", l.AccountID, l.ExternalID, field, value)
		if existing, err := o.db.Reader().GetFieldOutboxByKey(ctx, store.GetFieldOutboxByKeyParams{WorkspaceID: actor.WorkspaceID, IdempotencyKey: key}); err == nil {
			out = append(out, fieldWriteFromRow(existing))
			continue
		}
		// An older pending value for the same field is superseded by this one.
		olds, err := o.db.Reader().ListFieldOutboxForTarget(ctx, store.ListFieldOutboxForTargetParams{AccountID: l.AccountID, ExternalID: l.ExternalID, Field: field})
		if err != nil {
			return nil, err
		}
		for _, old := range olds {
			if old.State == "pending" || old.State == "held" {
				if _, err := o.db.Writer().SetFieldOutboxState(ctx, store.SetFieldOutboxStateParams{State: "superseded", Reason: "a newer value was queued", Conflict: "{}", UpdatedAt: now, ID: old.ID, State_2: old.State}); err != nil {
					return nil, err
				}
			}
		}
		row, err := o.db.Writer().CreateFieldOutbox(ctx, store.CreateFieldOutboxParams{WorkspaceID: actor.WorkspaceID, PersonID: personID, AccountID: l.AccountID, ExternalID: l.ExternalID, Field: field, Value: value, State: "pending", IdempotencyKey: key, CreatedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return nil, err
		}
		out = append(out, fieldWriteFromRow(row))
	}
	return out, nil
}

// DispatchFields sends pending field values. Each needs: the policy to
// allow ghl.lead_fields.set for the account, a field writer, and a token.
// Anything missing holds the row with the reason; nothing is sent.
func (o *Outreach) DispatchFields(ctx context.Context, actor core.Actor) (int, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return 0, core.ErrForbidden
	}
	sent := 0
	for _, state := range []string{"pending", "held"} {
		rows, err := o.db.Reader().ListFieldOutboxByState(ctx, store.ListFieldOutboxByStateParams{WorkspaceID: actor.WorkspaceID, State: state, Limit: 200})
		if err != nil {
			return sent, err
		}
		for _, r := range rows {
			ok, err := o.dispatchField(ctx, actor, r)
			if err != nil {
				return sent, err
			}
			if ok {
				sent++
			}
		}
	}
	return sent, nil
}

func (o *Outreach) dispatchField(ctx context.Context, actor core.Actor, r store.CrmFieldOutbox) (bool, error) {
	now := o.clock.Now().UTC()
	hold := func(reason string) (bool, error) {
		if r.State == "held" && r.Reason == reason {
			return false, nil
		}
		_, err := o.db.Writer().SetFieldOutboxState(ctx, store.SetFieldOutboxStateParams{State: "held", Reason: reason, Conflict: "{}", UpdatedAt: now, ID: r.ID, State_2: r.State})
		return false, err
	}
	cur, err := o.current(ctx, actor.WorkspaceID)
	if err != nil {
		return false, err
	}
	if cur == nil {
		return hold("outreach is not configured: field ownership unknown")
	}
	if cur.Document.Fields[r.Field] != OwnerADX {
		return hold(fmt.Sprintf("field %s is not owned by ADX in config v%d", r.Field, cur.Version))
	}
	if o.cfg.Policies != nil {
		d, err := o.cfg.Policies.Evaluate(ctx, actor, policies.PolicyRequest{AccountID: r.AccountID, Action: policies.LeadFieldsSet, WriteCapability: "write.lead_fields"})
		if err != nil {
			return false, err
		}
		if d.Outcome != policies.Allowed {
			return hold("policy: " + d.Reason)
		}
	}
	if o.cfg.Fields == nil {
		return hold("no CRM field writer is wired (ADX_LIVE_WRITES is off)")
	}
	if o.cfg.Tokens == nil {
		return hold("no CRM token source")
	}
	acct, err := o.db.Reader().GetAccountByID(ctx, r.AccountID)
	if err != nil {
		return false, err
	}
	token, err := o.cfg.Tokens.GHLToken(ctx, core.SystemIn(core.ViaJob, actor.WorkspaceID), acct.Account.ExternalID)
	if err != nil {
		return hold("the location's token is unavailable: " + safe(err))
	}
	if err := o.cfg.Fields.SetContactFields(ctx, token, r.ExternalID, map[string]string{r.Field: r.Value}); err != nil {
		_, uerr := o.db.Writer().SetFieldOutboxState(ctx, store.SetFieldOutboxStateParams{State: "failed", Reason: safe(err), Attempts: 1, Conflict: "{}", UpdatedAt: now, ID: r.ID, State_2: r.State})
		return false, uerr
	}
	_, err = o.db.Writer().SetFieldOutboxState(ctx, store.SetFieldOutboxStateParams{State: "sent", Reason: "", Attempts: 1, Conflict: "{}", UpdatedAt: now, SentAt: sql.NullTime{Time: now, Valid: true}, ID: r.ID, State_2: r.State})
	return err == nil, err
}

// ObserveFields is called with a contact's custom fields as the CRM shows
// them (by the import). For each field with an outbox row: the same value
// is an echo and confirms the row; a different value is a conflict when
// ADX owns the field, supersedes the row when the CRM owns it, and is a
// conflict for a person to look at when ownership is unconfigured. No
// observation ever creates a new write.
func (o *Outreach) ObserveFields(ctx context.Context, actor core.Actor, accountID int64, externalID string, fields map[string]string, at time.Time) error {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return core.ErrForbidden
	}
	cur, err := o.current(ctx, actor.WorkspaceID)
	if err != nil {
		return err
	}
	now := o.clock.Now().UTC()
	for field, theirs := range fields {
		rows, err := o.db.Reader().ListFieldOutboxForTarget(ctx, store.ListFieldOutboxForTargetParams{AccountID: accountID, ExternalID: externalID, Field: field})
		if err != nil {
			return err
		}
		for _, r := range rows {
			switch {
			case strings.TrimSpace(theirs) == strings.TrimSpace(r.Value):
				if r.State == "sent" || r.State == "pending" || r.State == "held" {
					if _, err := o.db.Writer().SetFieldOutboxState(ctx, store.SetFieldOutboxStateParams{State: "confirmed", Reason: "the CRM shows the value", Conflict: "{}", UpdatedAt: now, ConfirmedAt: sql.NullTime{Time: now, Valid: true}, ID: r.ID, State_2: r.State}); err != nil {
						return err
					}
				}
			default:
				owner := FieldOwner("")
				if cur != nil {
					owner = cur.Document.Fields[field]
				}
				detail, _ := json.Marshal(map[string]any{"ours": r.Value, "theirs": theirs, "observedAt": at.UTC(), "owner": owner})
				switch owner {
				case OwnerCRM:
					if _, err := o.db.Writer().SetFieldOutboxState(ctx, store.SetFieldOutboxStateParams{State: "superseded", Reason: "the CRM owns this field and shows a different value", Conflict: string(detail), UpdatedAt: now, ID: r.ID, State_2: r.State}); err != nil {
						return err
					}
				default:
					reason := "the CRM shows a different value for a field ADX owns"
					if owner == "" {
						reason = "the CRM shows a different value and the field's owner is not configured"
					}
					if _, err := o.db.Writer().SetFieldOutboxState(ctx, store.SetFieldOutboxStateParams{State: "conflict", Reason: reason, Conflict: string(detail), UpdatedAt: now, ID: r.ID, State_2: r.State}); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// ResolveConflict lets a marketer pick: keep ADX's value (re-queued as a
// new pending row) or take the CRM's (the row stays superseded). Either way
// the conflict row keeps its record.
func (o *Outreach) ResolveConflict(ctx context.Context, actor core.Actor, id int64, keepOurs bool, reason string) (FieldWrite, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return FieldWrite{}, core.ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return FieldWrite{}, core.Invalid("a reason is required")
	}
	r, err := o.db.Reader().GetFieldOutbox(ctx, store.GetFieldOutboxParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return FieldWrite{}, core.NotFound("field write %d not found", id)
	}
	if err != nil {
		return FieldWrite{}, err
	}
	if r.State != "conflict" {
		return FieldWrite{}, core.Conflict("field write %d is %s, not in conflict", id, r.State)
	}
	now := o.clock.Now().UTC()
	if _, err := o.db.Writer().SetFieldOutboxState(ctx, store.SetFieldOutboxStateParams{State: "superseded", Reason: "resolved: " + reason, Conflict: r.Conflict, UpdatedAt: now, ID: r.ID, State_2: "conflict"}); err != nil {
		return FieldWrite{}, err
	}
	if !keepOurs {
		row, err := o.db.Reader().GetFieldOutbox(ctx, store.GetFieldOutboxParams{ID: id, WorkspaceID: actor.WorkspaceID})
		if err != nil {
			return FieldWrite{}, err
		}
		return fieldWriteFromRow(row), nil
	}
	key := fmt.Sprintf("field:%d:%s:%s:%s@%d", r.AccountID, r.ExternalID, r.Field, r.Value, now.Unix())
	row, err := o.db.Writer().CreateFieldOutbox(ctx, store.CreateFieldOutboxParams{WorkspaceID: actor.WorkspaceID, PersonID: r.PersonID, AccountID: r.AccountID, ExternalID: r.ExternalID, Field: r.Field, Value: r.Value, State: "pending", Reason: "re-queued after conflict: " + reason, IdempotencyKey: key, CreatedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return FieldWrite{}, err
	}
	return fieldWriteFromRow(row), nil
}

// ListFieldWrites is the outbox view.
func (o *Outreach) ListFieldWrites(ctx context.Context, actor core.Actor, limit int) ([]FieldWrite, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := o.db.Reader().ListFieldOutbox(ctx, store.ListFieldOutboxParams{WorkspaceID: actor.WorkspaceID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]FieldWrite, 0, len(rows))
	for _, r := range rows {
		out = append(out, fieldWriteFromRow(r))
	}
	return out, nil
}

func (o *Outreach) ListDrafts(ctx context.Context, actor core.Actor, limit int) ([]MessageDraft, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := o.db.Reader().ListMessageDrafts(ctx, store.ListMessageDraftsParams{WorkspaceID: actor.WorkspaceID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]MessageDraft, 0, len(rows))
	for _, r := range rows {
		out = append(out, draftFromRow(r))
	}
	return out, nil
}

// SyncScores queues the latest temperature, intent and fit of every scored
// person into the ADX-owned fields, so the CRM shows what ADX knows. Rows
// dispatch only under policy.
func (o *Outreach) SyncScores(ctx context.Context, actor core.Actor) (int, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || o.cfg.Signals == nil {
		return 0, core.ErrForbidden
	}
	cur, err := o.current(ctx, actor.WorkspaceID)
	if err != nil || cur == nil {
		return 0, err
	}
	ids, err := o.db.Reader().ListPeopleWithEvents(ctx, store.ListPeopleWithEventsParams{WorkspaceID: actor.WorkspaceID, OccurredAt: o.clock.Now().UTC().Add(-365 * 24 * time.Hour)})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if !id.Valid {
			continue
		}
		v, err := o.cfg.Signals.Score(ctx, actor, id.Int64)
		if err != nil || v.Latest == nil {
			continue
		}
		values := map[string]string{}
		if v.Latest.Temperature != nil {
			values["adx_temperature"] = *v.Latest.Temperature
		}
		if v.Latest.Intent != nil {
			values["adx_intent"] = strconv.FormatInt(*v.Latest.Intent, 10)
		}
		if v.Latest.Fit != nil {
			values["adx_fit"] = strconv.FormatInt(*v.Latest.Fit, 10)
		}
		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if cur.Document.Fields[k] != OwnerADX {
				continue
			}
			rows, err := o.QueueFieldWrite(ctx, actor, id.Int64, k, values[k])
			if err != nil {
				var ce *core.Error
				if errors.As(err, &ce) {
					continue
				}
				return n, err
			}
			for _, r := range rows {
				if r.CreatedAt.Equal(o.clock.Now().UTC()) {
					n++
				}
			}
		}
	}
	return n, nil
}

func (o *Outreach) current(ctx context.Context, workspaceID int64) (*OutreachConfig, error) {
	row, err := o.db.Reader().GetCurrentOutreachConfig(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := configFromRow(row)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (o *Outreach) person(ctx context.Context, workspaceID, id int64) (store.Person, error) {
	p, err := o.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Person{}, core.NotFound("person %d not found", id)
	}
	return p, err
}

func (o *Outreach) enquiry(ctx context.Context, workspaceID, id int64) (crm.Enquiry, error) {
	r, err := o.db.Reader().GetEnquiry(ctx, store.GetEnquiryParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return crm.Enquiry{}, core.NotFound("enquiry %d not found", id)
	}
	if err != nil {
		return crm.Enquiry{}, err
	}
	return crm.Enquiry{ID: r.ID, PersonID: r.PersonID, ProjectID: idPtr(r.ProjectID), State: r.State, Stage: strPtr(r.Stage), SourceKind: r.SourceKind, SourceRef: r.SourceRef, FirstContactAt: r.FirstContactAt.UTC(), LastActivityAt: r.LastActivityAt.UTC()}, nil
}

func (o *Outreach) draft(ctx context.Context, workspaceID, id int64) (store.MessageDraft, error) {
	r, err := o.db.Reader().GetMessageDraft(ctx, store.GetMessageDraftParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.MessageDraft{}, core.NotFound("draft %d not found", id)
	}
	return r, err
}

func (o *Outreach) get(ctx context.Context, workspaceID, id int64) (MessageDraft, error) {
	r, err := o.draft(ctx, workspaceID, id)
	if err != nil {
		return MessageDraft{}, err
	}
	return draftFromRow(r), nil
}

func configFromRow(r store.OutreachConfig) (OutreachConfig, error) {
	var doc OutreachDocument
	if err := json.Unmarshal([]byte(r.Document), &doc); err != nil {
		return OutreachConfig{}, fmt.Errorf("outreach config v%d: %w", r.Version, err)
	}
	return OutreachConfig{Version: r.Version, Document: doc, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC()}, nil
}

func objectionFromRow(r store.Objection) Objection {
	o := Objection{ID: r.ID, PersonID: r.PersonID, EnquiryID: idPtr(r.EnquiryID), Kind: r.Kind, Text: r.Text, Source: r.Source, EvidenceID: idPtr(r.EvidenceID), RecordedBy: idPtr(r.RecordedBy), RecordedAt: r.RecordedAt.UTC(), Resolution: r.Resolution}
	o.ResolvedAt = timePtr(r.ResolvedAt)
	return o
}

// ID64 returns the objection id as a pointer, for callers that want one.
func (ob Objection) ID64() *int64 { id := ob.ID; return &id }

func suppressionFromRow(r store.ContactSuppression) Suppression {
	return Suppression{ID: r.ID, PersonID: r.PersonID, Channel: r.Channel, Reason: r.Reason, Source: r.Source, Since: r.Since.UTC(), LiftedAt: timePtr(r.LiftedAt), LiftedBy: idPtr(r.LiftedBy), LiftReason: r.LiftReason}
}

func fieldWriteFromRow(r store.CrmFieldOutbox) FieldWrite {
	return FieldWrite{ID: r.ID, PersonID: r.PersonID, AccountID: r.AccountID, ExternalID: r.ExternalID, Field: r.Field, Value: r.Value, State: r.State, Reason: r.Reason, Attempts: r.Attempts, IdempotencyKey: r.IdempotencyKey, Conflict: json.RawMessage(r.Conflict),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(), SentAt: timePtr(r.SentAt), ConfirmedAt: timePtr(r.ConfirmedAt)}
}

func draftFromRow(r store.MessageDraft) MessageDraft {
	d := MessageDraft{ID: r.ID, PersonID: r.PersonID, EnquiryID: idPtr(r.EnquiryID), Channel: r.Channel, Language: r.Language, Body: r.Body, State: r.State, Reason: r.Reason, Attempts: r.Attempts, IdempotencyKey: r.IdempotencyKey, ProviderRef: r.ProviderRef,
		CreatedBy: idPtr(r.CreatedBy), ApprovedBy: idPtr(r.ApprovedBy), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(), SentAt: timePtr(r.SentAt)}
	if r.Claims != "" && r.Claims != "{}" && r.Claims != "null" {
		var rep brands.ClaimReport
		if json.Unmarshal([]byte(r.Claims), &rep) == nil {
			d.Claims = &rep
		}
	}
	return d
}

func nonce() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func safe(err error) string {
	var ce *core.Error
	if errors.As(err, &ce) {
		return ce.Error()
	}
	s := err.Error()
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func strPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	v := n.String
	return &v
}

func timePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time.UTC()
	return &t
}
