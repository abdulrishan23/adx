// Package conversations runs the text qualification agent (BD-69). An
// inbound message on a channel becomes a conversation with the person who
// wrote it; the agent answers from the sales brief and the approved facts,
// one reply per customer message, and hands to a person when the customer
// asks for one, pushes back on something no fact answers, says stop, or
// asks something the facts cannot carry. Every send goes through the same
// gates as outreach drafts: suppression, the kill switch, the channel mode
// in the outreach config (absent or off sends nothing), a wired messenger,
// and the provider's customer-service window. The conversation is read
// with the call reading prompt and applied to the enquiry once per version.
package conversations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/qualification"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/store"
)

// Channel is the one channel the agent serves in this build.
const Channel = "whatsapp"

// States of a conversation.
const (
	StateAgent   = "agent"   // the agent replies on its own, within the gates
	StateWaiting = "waiting" // handed off; nobody has taken it yet
	StatePerson  = "person"  // a person took it; the agent is off
	StateClosed  = "closed"
)

// Handoff kinds, in the order the inbox ranks them.
const (
	HandoffRequestedHuman = "requested_human"
	HandoffObjection      = "objection"
	HandoffOutsideFacts   = "outside_facts"
	HandoffQuestion       = "question"
	HandoffStop           = "stop"
	HandoffNoModel        = "no_model"
)

// TemplateMessage is a pre-approved message sent outside the provider's
// window. Nothing in this build sends one: Reportage has supplied no
// catalogue, so the seam exists and the reply is held with the reason.
type TemplateMessage struct {
	To       string
	Name     string
	Language string
	// IdempotencyKey is the turn's key so a retried send is the same send.
	IdempotencyKey string
}

// TemplateSender sends a template on the channel. Optional.
type TemplateSender interface {
	SendTemplate(ctx context.Context, msg TemplateMessage) (providerRef string, err error)
}

// InboundMessage is one message as the channel adapter reports it.
type InboundMessage struct {
	Channel string `json:"channel"`
	// ProviderRef is the provider's message id; a repeated delivery of the
	// same id is the same turn.
	ProviderRef string `json:"providerRef"`
	From        string `json:"from"`
	ProfileName string `json:"profileName"`
	// Kind is text or unsupported. An unsupported message (a picture, a
	// voice note) is kept as a turn with its type, and the agent says it
	// cannot read it.
	Kind string `json:"kind"`
	Body string `json:"body"`
	// Unsupported names the provider's message type when Kind is unsupported.
	Unsupported string    `json:"unsupported"`
	ReceivedAt  time.Time `json:"receivedAt"`
}

// StatusUpdate is the provider's word on a sent message.
type StatusUpdate struct {
	Channel     string    `json:"channel"`
	ProviderRef string    `json:"providerRef"`
	Status      string    `json:"status"` // sent, delivered, read, failed
	Error       string    `json:"error"`
	At          time.Time `json:"at"`
}

type Thread struct {
	ID             int64      `json:"id"`
	PersonID       int64      `json:"personId"`
	EnquiryID      *int64     `json:"enquiryId"`
	Channel        string     `json:"channel"`
	Address        string     `json:"address"`
	State          string     `json:"state"`
	StateReason    string     `json:"stateReason"`
	HandoffKind    string     `json:"handoffKind"`
	HandoffAt      *time.Time `json:"handoffAt"`
	Language       string     `json:"language"`
	LastInboundAt  *time.Time `json:"lastInboundAt"`
	LastOutboundAt *time.Time `json:"lastOutboundAt"`
	// WindowClosesAt is when a free-text reply stops being possible; nil
	// when the customer has never written.
	WindowClosesAt *time.Time                             `json:"windowClosesAt"`
	Summary        string                                 `json:"summary"`
	Answers        map[string]qualification.ReadingAnswer `json:"answers"`
	Outcome        string                                 `json:"outcome"`
	Confidence     string                                 `json:"confidence"`
	RequestedHuman bool                                   `json:"requestedHuman"`
	ReviewStatus   string                                 `json:"reviewStatus"`
	ReviewReason   string                                 `json:"reviewReason"`
	ReadAt         *time.Time                             `json:"readAt"`
	ReadTurns      int64                                  `json:"readTurns"`
	AppliedKey     string                                 `json:"appliedKey"`
	AppliedVersion int64                                  `json:"appliedVersion"`
	AppliedAt      *time.Time                             `json:"appliedAt"`
	Applied        qualification.AppliedDetail            `json:"applied"`
	CreatedAt      time.Time                              `json:"createdAt"`
	UpdatedAt      time.Time                              `json:"updatedAt"`
}

type ThreadTurn struct {
	ID             int64               `json:"id"`
	ConversationID int64               `json:"conversationId"`
	Direction      string              `json:"direction"`
	Author         string              `json:"author"`
	Kind           string              `json:"kind"`
	Body           string              `json:"body"`
	State          string              `json:"state"`
	Reason         string              `json:"reason"`
	Handoff        string              `json:"handoff"`
	ProviderRef    string              `json:"providerRef"`
	Claims         *brands.ClaimReport `json:"claims"`
	EvidenceID     *int64              `json:"evidenceId"`
	CreatedBy      *int64              `json:"createdBy"`
	ApprovedBy     *int64              `json:"approvedBy"`
	OccurredAt     time.Time           `json:"occurredAt"`
	SentAt         *time.Time          `json:"sentAt"`
}

type ThreadCorrection struct {
	ID             int64                                  `json:"id"`
	ConversationID int64                                  `json:"conversationId"`
	Outcome        string                                 `json:"outcome"`
	Answers        map[string]qualification.ReadingAnswer `json:"answers"`
	Note           string                                 `json:"note"`
	CreatedBy      *int64                                 `json:"createdBy"`
	CreatedAt      time.Time                              `json:"createdAt"`
}

// ThreadSummary is a conversation in a list, with the person and the last words.
type ThreadSummary struct {
	Thread
	PersonName  string `json:"personName"`
	ProjectName string `json:"projectName"`
	LastBody    string `json:"lastBody"`
	LastFrom    string `json:"lastFrom"`
	// Pending counts the agent's drafts waiting for a person.
	Pending int `json:"pending"`
}

// ThreadDetail is the thread: the conversation, its turns, corrections and who
// it is with.
type ThreadDetail struct {
	Conversation Thread             `json:"conversation"`
	Person       crm.Person         `json:"person"`
	ProjectName  string             `json:"projectName"`
	Turns        []ThreadTurn       `json:"turns"`
	Corrections  []ThreadCorrection `json:"corrections"`
}

// ThreadReceived is what one inbound message did.
type ThreadReceived struct {
	Conversation Thread     `json:"conversation"`
	Turn         ThreadTurn `json:"turn"`
	Duplicate    bool       `json:"duplicate"`
	// Reply is the agent's turn, when it drafted one.
	Reply *ThreadTurn `json:"reply"`
}

// ThreadsReadiness says whether the agent can run on this server.
type ThreadsReadiness struct {
	ModelReady  bool     `json:"modelReady"`
	Messenger   string   `json:"messenger"`
	ChannelMode string   `json:"channelMode"`
	Reasons     []string `json:"reasons"`
}

type Config struct {
	Clock     core.Clock
	Model     assistant.Model
	Outreach  *outreach.Outreach
	Signals   *signals.Signals
	CRM       *crm.CRM
	Brands    *brands.Brands
	Policies  *policies.Policies
	Messenger outreach.Messenger
	Templates TemplateSender
	// Window is the provider's customer-service window after the customer's
	// last message, inside which free text may be sent. Default 24h.
	Window time.Duration
	// MaxRepliesPerDay caps what the agent or a person sends on one
	// conversation per Dubai day. Default 20.
	MaxRepliesPerDay int
}

type Conversations struct {
	db       *store.DB
	evidence *evidence.Store
	cfg      Config
	clock    core.Clock
}

func New(db *store.DB, ev *evidence.Store, cfg Config) *Conversations {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	if cfg.Window <= 0 {
		cfg.Window = 24 * time.Hour
	}
	if cfg.MaxRepliesPerDay <= 0 {
		cfg.MaxRepliesPerDay = 20
	}
	return &Conversations{db: db, evidence: ev, cfg: cfg, clock: cfg.Clock}
}

// Readiness is what the screen shows about this server.
func (c *Conversations) Readiness(ctx context.Context, actor core.Actor) (ThreadsReadiness, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return ThreadsReadiness{}, core.ErrForbidden
	}
	st := ThreadsReadiness{ModelReady: c.cfg.Model != nil, Reasons: []string{}}
	if c.cfg.Model == nil {
		st.Reasons = append(st.Reasons, "no model is configured; every conversation waits for a person")
	}
	if c.cfg.Messenger == nil {
		st.Reasons = append(st.Reasons, "no messaging adapter is wired; replies are drafted and held")
	} else {
		st.Messenger = c.cfg.Messenger.Name()
	}
	mode, version, err := c.channelMode(ctx, actor.WorkspaceID)
	if err != nil {
		return ThreadsReadiness{}, err
	}
	st.ChannelMode = string(mode)
	switch mode {
	case outreach.ModeAutomatic:
	case outreach.ModeApproved:
		st.Reasons = append(st.Reasons, fmt.Sprintf("channel %s is on approval in outreach config v%d; a person approves every reply", Channel, version))
	default:
		st.Reasons = append(st.Reasons, "channel "+Channel+" is off in the outreach config; replies are drafted and nothing is sent")
	}
	return st, nil
}

// Receive stores an inbound message on the person's conversation, scores
// it as a reply, and asks the agent for its answer when the agent is on.
// A repeated delivery of the same provider id is the same turn and asks
// nothing. Jobs and marketers may.
func (c *Conversations) Receive(ctx context.Context, actor core.Actor, in InboundMessage) (ThreadReceived, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return ThreadReceived{}, core.ErrForbidden
	}
	if in.Channel == "" {
		in.Channel = Channel
	}
	if in.Channel != Channel {
		return ThreadReceived{}, core.Invalid("channel must be %s", Channel)
	}
	in.ProviderRef = strings.TrimSpace(in.ProviderRef)
	if in.ProviderRef == "" {
		return ThreadReceived{}, core.Invalid("a message needs the provider's id")
	}
	if in.Kind == "" {
		in.Kind = "text"
	}
	if in.Kind != "text" && in.Kind != "unsupported" {
		return ThreadReceived{}, core.Invalid("message kind must be text or unsupported")
	}
	in.Body = strings.TrimSpace(in.Body)
	if in.Kind == "text" && in.Body == "" {
		return ThreadReceived{}, core.Invalid("a text message needs a body")
	}
	if c.cfg.CRM == nil {
		return ThreadReceived{}, core.Unavailable(nil, "crm is not wired")
	}
	at := in.ReceivedAt
	if at.IsZero() {
		at = c.clock.Now()
	}
	at = at.UTC()
	now := c.clock.Now().UTC()
	phone := c.cfg.CRM.NormalizePhone(in.From)
	if phone == "" {
		return ThreadReceived{}, core.Invalid("the sender %q is not a phone number", in.From)
	}
	row, err := c.db.Reader().GetThreadByAddress(ctx, store.GetThreadByAddressParams{WorkspaceID: actor.WorkspaceID, Channel: in.Channel, Address: phone})
	if errors.Is(err, sql.ErrNoRows) {
		contact, err := c.cfg.CRM.ResolveContact(ctx, actor, crm.ContactInput{Phone: phone, Name: in.ProfileName, Source: in.Channel, At: at})
		if err != nil {
			return ThreadReceived{}, err
		}
		lang := "en"
		if contact.Person.Language != nil && *contact.Person.Language != "" {
			lang = *contact.Person.Language
		}
		state, reason := StateAgent, "the agent replies"
		if c.cfg.Model == nil {
			state, reason = StateWaiting, "no model is configured; a person replies"
		}
		row, err = c.db.Writer().CreateThread(ctx, store.CreateThreadParams{WorkspaceID: actor.WorkspaceID, PersonID: contact.Person.ID, EnquiryID: optionalID(contact.Enquiry.ID), Channel: in.Channel, Address: phone, State: state, StateReason: reason, Language: lang, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			if store.IsUniqueViolation(err) {
				row, err = c.db.Reader().GetThreadByAddress(ctx, store.GetThreadByAddressParams{WorkspaceID: actor.WorkspaceID, Channel: in.Channel, Address: phone})
			}
			if err != nil {
				return ThreadReceived{}, err
			}
		} else if state == StateWaiting {
			if err := c.db.Writer().SetThreadStateAny(ctx, store.SetThreadStateAnyParams{State: state, StateReason: reason, HandoffKind: HandoffNoModel, HandoffAt: sql.NullTime{Time: now, Valid: true}, UpdatedAt: now, ID: row.ID}); err != nil {
				return ThreadReceived{}, err
			}
		}
	} else if err != nil {
		return ThreadReceived{}, err
	}
	key := "in:" + in.ProviderRef
	if existing, err := c.db.Reader().GetThreadTurnByKey(ctx, store.GetThreadTurnByKeyParams{ThreadID: row.ID, IdempotencyKey: key}); err == nil {
		conv, err := c.get(ctx, actor.WorkspaceID, row.ID)
		if err != nil {
			return ThreadReceived{}, err
		}
		return ThreadReceived{Conversation: conv, Turn: turnFromRow(existing), Duplicate: true}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ThreadReceived{}, err
	}
	body := in.Body
	if in.Kind == "unsupported" {
		body = "(" + strings.TrimSpace(in.Unsupported) + " message; ADX cannot read it)"
	}
	var evID int64
	if c.evidence != nil {
		ev, _, err := c.evidence.Record(ctx, actor, evidence.Observation{Source: in.Channel, Kind: "message_in", SourceRef: in.ProviderRef, Scope: fmt.Sprintf("person:%d", row.PersonID), Class: evidence.Observed, EventAt: &at,
			Payload: map[string]any{"kind": in.Kind, "body": in.Body, "unsupported": in.Unsupported, "from": phone}, Completeness: evidence.Complete})
		if err != nil {
			return ThreadReceived{}, err
		}
		evID = ev.ID
	}
	trow, err := c.db.Writer().CreateThreadTurn(ctx, store.CreateThreadTurnParams{ThreadID: row.ID, Direction: "in", Author: "customer", Kind: in.Kind, Body: body, State: "received", ProviderRef: in.ProviderRef, IdempotencyKey: key, Claims: "{}", EvidenceID: optionalID(evID), OccurredAt: at, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		if store.IsUniqueViolation(err) {
			existing, err := c.db.Reader().GetThreadTurnByKey(ctx, store.GetThreadTurnByKeyParams{ThreadID: row.ID, IdempotencyKey: key})
			if err != nil {
				return ThreadReceived{}, err
			}
			conv, err := c.get(ctx, actor.WorkspaceID, row.ID)
			return ThreadReceived{Conversation: conv, Turn: turnFromRow(existing), Duplicate: true}, err
		}
		return ThreadReceived{}, err
	}
	lang := row.Language
	if in.Kind == "text" && looksArabic(in.Body) {
		lang = "ar"
	}
	if err := c.db.Writer().TouchThreadInbound(ctx, store.TouchThreadInboundParams{LastInboundAt: sql.NullTime{Time: at, Valid: true}, Language: lang, UpdatedAt: now, ID: row.ID}); err != nil {
		return ThreadReceived{}, err
	}
	if row.EnquiryID.Valid {
		_ = c.db.Writer().TouchEnquiry(ctx, store.TouchEnquiryParams{LastActivityAt: at, ID: row.EnquiryID.Int64})
	}
	if c.cfg.Signals != nil && in.Kind == "text" {
		payload, _ := json.Marshal(map[string]any{"conversationId": row.ID, "channel": in.Channel})
		_, _ = c.cfg.Signals.Ingest(ctx, actor, signals.SignalInput{Source: in.Channel, Kind: "message_reply", OccurrenceKey: in.ProviderRef, OccurredAt: at, Identity: signals.SignalIdentity{PersonID: row.PersonID}, Payload: payload})
	}
	out := ThreadReceived{Turn: turnFromRow(trow)}
	// A stop is honoured before anything else, whoever is running the thread.
	if in.Kind == "text" && saysStop(in.Body) {
		if err := c.stop(ctx, actor, row.ID, row.PersonID, in.Body); err != nil {
			return ThreadReceived{}, err
		}
	} else if row.State == StateAgent {
		reply, err := c.reply(ctx, actor, row.ID)
		if err != nil {
			return ThreadReceived{}, err
		}
		if reply != nil {
			out.Reply = reply
		}
	} else if row.State == StateWaiting && in.Kind == "text" && asksForHuman(in.Body) {
		// A thread already waiting (no model, a question nobody answered)
		// whose customer now asks for a person is urgent, with or without
		// an agent to say so.
		reply, err := c.acknowledgeHuman(ctx, actor, row, in.Body)
		if err != nil {
			return ThreadReceived{}, err
		}
		out.Reply = reply
	}
	out.Conversation, err = c.get(ctx, actor.WorkspaceID, row.ID)
	return out, err
}

// acknowledgeHuman records a request for a person on a thread, drafts the
// one line that says a colleague will write, and sends it when the gates
// allow. It asks no model: the customer said it.
func (c *Conversations) acknowledgeHuman(ctx context.Context, actor core.Actor, row store.Thread, words string) (*ThreadTurn, error) {
	now := c.clock.Now().UTC()
	body := "Of course. A colleague from Reportage will message you here shortly."
	if strings.HasPrefix(row.Language, "ar") {
		body = "بالتأكيد. سيراسلك زميل من ريبورتاج هنا قريباً."
	}
	t, err := c.db.Writer().CreateThreadTurn(ctx, store.CreateThreadTurnParams{ThreadID: row.ID, Direction: "out", Author: "agent", Kind: "text", Body: body, State: "draft", Handoff: HandoffRequestedHuman, IdempotencyKey: outKey(row.ID, now), Claims: "{}", OccurredAt: now, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return nil, err
	}
	reason := "the customer asked for a person: " + truncate(words, 120)
	if err := c.db.Writer().SetThreadStateAny(ctx, store.SetThreadStateAnyParams{State: StateWaiting, StateReason: reason, HandoffKind: HandoffRequestedHuman, HandoffAt: sql.NullTime{Time: now, Valid: true}, UpdatedAt: now, ID: row.ID}); err != nil {
		return nil, err
	}
	if err := c.read(ctx, actor, row.ID); err != nil {
		return nil, err
	}
	sent, err := c.dispatch(ctx, actor, row, t, false)
	if err != nil {
		return nil, err
	}
	return &sent, nil
}

// UpdateStatus applies the provider's delivery status to the sent turn it
// names. An unknown reference is ignored: it may be a message sent by a
// person from the Business app, which ADX does not track.
func (c *Conversations) UpdateStatus(ctx context.Context, actor core.Actor, in StatusUpdate) (bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return false, core.ErrForbidden
	}
	if strings.TrimSpace(in.ProviderRef) == "" {
		return false, core.Invalid("a status needs the provider's message id")
	}
	row, err := c.db.Reader().GetThreadTurnByProviderRef(ctx, store.GetThreadTurnByProviderRefParams{ProviderRef: in.ProviderRef, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.Direction != "out" {
		return false, nil
	}
	rank := map[string]int{"sent": 1, "delivered": 2, "read": 3}
	now := c.clock.Now().UTC()
	switch in.Status {
	case "failed":
		reason := strings.TrimSpace(in.Error)
		if reason == "" {
			reason = "the provider reported the message failed"
		}
		_, err = c.db.Writer().SetThreadTurnState(ctx, store.SetThreadTurnStateParams{State: "failed", Reason: reason, UpdatedAt: now, ID: row.ID, FromState: row.State})
		return err == nil, err
	case "sent", "delivered", "read":
		if rank[in.Status] <= rank[row.State] {
			return false, nil
		}
		_, err = c.db.Writer().SetThreadTurnState(ctx, store.SetThreadTurnStateParams{State: in.Status, Reason: "", UpdatedAt: now, ID: row.ID, FromState: row.State})
		return err == nil, err
	}
	return false, core.Invalid("unknown status %q", in.Status)
}

// Reply asks the agent for its next turn now. Marketers may, for a thread
// the agent runs; it is what the webhook does after each customer message.
func (c *Conversations) Reply(ctx context.Context, actor core.Actor, id int64) (ThreadTurn, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return ThreadTurn{}, core.ErrForbidden
	}
	row, err := c.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return ThreadTurn{}, err
	}
	if row.State != StateAgent {
		return ThreadTurn{}, core.Conflict("conversation %d is %s; the agent only replies while it runs the thread", id, row.State)
	}
	t, err := c.reply(ctx, actor, id)
	if err != nil {
		return ThreadTurn{}, err
	}
	if t == nil {
		return ThreadTurn{}, core.Conflict("the agent has nothing to answer: the last word is its own")
	}
	return *t, nil
}

// Send is a person's own message on the thread. It goes through the same
// gates as the agent's, except that a person's words are their own and are
// not checked against the facts. Marketers may.
func (c *Conversations) Send(ctx context.Context, actor core.Actor, id int64, body string) (ThreadTurn, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return ThreadTurn{}, core.ErrForbidden
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return ThreadTurn{}, core.Invalid("say something")
	}
	if len(body) > 4000 {
		return ThreadTurn{}, core.Invalid("a message is 4000 characters or fewer")
	}
	row, err := c.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return ThreadTurn{}, err
	}
	if row.State == StateClosed {
		return ThreadTurn{}, core.Conflict("conversation %d is closed", id)
	}
	now := c.clock.Now().UTC()
	if row.State != StatePerson {
		if err := c.db.Writer().SetThreadStateAny(ctx, store.SetThreadStateAnyParams{State: StatePerson, StateReason: fmt.Sprintf("user %d is replying", actor.UserID), HandoffKind: "", UpdatedAt: now, ID: id}); err != nil {
			return ThreadTurn{}, err
		}
	}
	t, err := c.db.Writer().CreateThreadTurn(ctx, store.CreateThreadTurnParams{ThreadID: id, Direction: "out", Author: fmt.Sprintf("user:%d", actor.UserID), Kind: "text", Body: body, State: "approved", IdempotencyKey: outKey(id, now), Claims: "{}", CreatedBy: optionalID(actor.UserID), OccurredAt: now, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return ThreadTurn{}, err
	}
	return c.dispatch(ctx, actor, row, t, true)
}

// Approve accepts the agent's draft and sends it. A held draft that quotes
// something not approved cannot be approved; the person writes their own.
func (c *Conversations) Approve(ctx context.Context, actor core.Actor, turnID int64) (ThreadTurn, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return ThreadTurn{}, core.ErrForbidden
	}
	t, err := c.turn(ctx, actor.WorkspaceID, turnID)
	if err != nil {
		return ThreadTurn{}, err
	}
	if t.Direction != "out" {
		return ThreadTurn{}, core.Conflict("turn %d is the customer's", turnID)
	}
	switch t.State {
	case "draft", "held", "failed":
	default:
		return ThreadTurn{}, core.Conflict("turn %d is %s, not waiting", turnID, t.State)
	}
	if t.Handoff == HandoffOutsideFacts {
		return ThreadTurn{}, core.Conflict("turn %d is held: %s", turnID, t.Reason)
	}
	row, err := c.row(ctx, actor.WorkspaceID, t.ThreadID)
	if err != nil {
		return ThreadTurn{}, err
	}
	if row.State == StateClosed {
		return ThreadTurn{}, core.Conflict("conversation %d is closed", row.ID)
	}
	now := c.clock.Now().UTC()
	n, err := c.db.Writer().ApproveThreadTurn(ctx, store.ApproveThreadTurnParams{ApprovedBy: optionalID(actor.UserID), UpdatedAt: now, ID: t.ID, FromState: t.State})
	if err != nil {
		return ThreadTurn{}, err
	}
	if n != 1 {
		return ThreadTurn{}, core.Conflict("turn %d changed underneath", turnID)
	}
	t.State = "approved"
	return c.dispatch(ctx, actor, row, t, true)
}

// Discard drops a draft with a note.
func (c *Conversations) Discard(ctx context.Context, actor core.Actor, turnID int64, note string) (ThreadTurn, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return ThreadTurn{}, core.ErrForbidden
	}
	t, err := c.turn(ctx, actor.WorkspaceID, turnID)
	if err != nil {
		return ThreadTurn{}, err
	}
	switch t.State {
	case "draft", "held", "failed":
	default:
		return ThreadTurn{}, core.Conflict("turn %d is %s, not waiting", turnID, t.State)
	}
	n, err := c.db.Writer().SetThreadTurnState(ctx, store.SetThreadTurnStateParams{State: "discarded", Reason: "discarded by user " + itoa(actor.UserID) + ": " + strings.TrimSpace(note), UpdatedAt: c.clock.Now().UTC(), ID: t.ID, FromState: t.State})
	if err != nil {
		return ThreadTurn{}, err
	}
	if n != 1 {
		return ThreadTurn{}, core.Conflict("turn %d changed underneath", turnID)
	}
	return c.getTurn(ctx, actor.WorkspaceID, turnID)
}

// Take puts a person on the thread: the agent stops drafting.
func (c *Conversations) Take(ctx context.Context, actor core.Actor, id int64, note string) (Thread, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return Thread{}, core.ErrForbidden
	}
	row, err := c.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return Thread{}, err
	}
	if row.State == StateClosed {
		return Thread{}, core.Conflict("conversation %d is closed", id)
	}
	reason := fmt.Sprintf("taken by user %d", actor.UserID)
	if n := strings.TrimSpace(note); n != "" {
		reason += ": " + n
	}
	if err := c.db.Writer().SetThreadStateAny(ctx, store.SetThreadStateAnyParams{State: StatePerson, StateReason: reason, HandoffKind: row.HandoffKind, HandoffAt: row.HandoffAt, UpdatedAt: c.clock.Now().UTC(), ID: id}); err != nil {
		return Thread{}, err
	}
	return c.get(ctx, actor.WorkspaceID, id)
}

// HandBack gives the thread back to the agent, which answers the next
// customer message. It needs a model; without one the thread waits.
func (c *Conversations) HandBack(ctx context.Context, actor core.Actor, id int64) (Thread, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return Thread{}, core.ErrForbidden
	}
	row, err := c.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return Thread{}, err
	}
	if row.State == StateClosed {
		return Thread{}, core.Conflict("conversation %d is closed", id)
	}
	if c.cfg.Model == nil {
		return Thread{}, core.Conflict("no model is configured; the agent cannot take the thread")
	}
	if err := c.db.Writer().SetThreadStateAny(ctx, store.SetThreadStateAnyParams{State: StateAgent, StateReason: fmt.Sprintf("handed back to the agent by user %d", actor.UserID), HandoffKind: "", UpdatedAt: c.clock.Now().UTC(), ID: id}); err != nil {
		return Thread{}, err
	}
	return c.get(ctx, actor.WorkspaceID, id)
}

// Close ends the thread and reads it, so what was learned reaches the
// enquiry. A new message from the customer reopens it to the agent.
func (c *Conversations) Close(ctx context.Context, actor core.Actor, id int64, note string) (Thread, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return Thread{}, core.ErrForbidden
	}
	row, err := c.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return Thread{}, err
	}
	if row.State == StateClosed {
		return Thread{}, core.Conflict("conversation %d is already closed", id)
	}
	reason := fmt.Sprintf("closed by user %d", actor.UserID)
	if n := strings.TrimSpace(note); n != "" {
		reason += ": " + n
	}
	if err := c.db.Writer().SetThreadStateAny(ctx, store.SetThreadStateAnyParams{State: StateClosed, StateReason: reason, HandoffKind: row.HandoffKind, HandoffAt: row.HandoffAt, UpdatedAt: c.clock.Now().UTC(), ID: id}); err != nil {
		return Thread{}, err
	}
	if err := c.read(ctx, actor, id); err != nil {
		return Thread{}, err
	}
	return c.get(ctx, actor.WorkspaceID, id)
}

// stop honours an opt-out: the channel is suppressed for the person, the
// thread closes, and one closing line goes out if the gates allow it.
func (c *Conversations) stop(ctx context.Context, actor core.Actor, id, personID int64, words string) error {
	now := c.clock.Now().UTC()
	if c.cfg.Outreach != nil {
		if _, err := c.cfg.Outreach.Suppress(ctx, actor, personID, Channel, "the customer wrote: "+truncate(words, 120), fmt.Sprintf("conversation:%d", id)); err != nil {
			return err
		}
	}
	if err := c.db.Writer().SetThreadStateAny(ctx, store.SetThreadStateAnyParams{State: StateClosed, StateReason: "the customer asked to stop", HandoffKind: HandoffStop, HandoffAt: sql.NullTime{Time: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
		return err
	}
	row, err := c.db.Reader().GetThread(ctx, store.GetThreadParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return err
	}
	body := "Understood, we won't message you again. Thank you."
	if strings.HasPrefix(row.Language, "ar") {
		body = "تمام، لن نراسلك مرة أخرى. شكراً لك."
	}
	t, err := c.db.Writer().CreateThreadTurn(ctx, store.CreateThreadTurnParams{ThreadID: id, Direction: "out", Author: "agent", Kind: "text", Body: body, State: "draft", Handoff: HandoffStop, IdempotencyKey: outKey(id, now), Claims: "{}", OccurredAt: now, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return err
	}
	// The closing line is the one message that goes to a suppressed person:
	// it is the acknowledgement, sent now or never.
	_, err = c.dispatch(ctx, actor, row, t, false)
	if err != nil {
		var ce *core.Error
		if !errors.As(err, &ce) {
			return err
		}
	}
	return c.read(ctx, actor, id)
}

// handoff moves a running thread to waiting with the reason, once.
func (c *Conversations) handoff(ctx context.Context, actor core.Actor, id int64, kind, reason string) error {
	now := c.clock.Now().UTC()
	n, err := c.db.Writer().SetThreadState(ctx, store.SetThreadStateParams{State: StateWaiting, StateReason: reason, HandoffKind: kind, HandoffAt: sql.NullTime{Time: now, Valid: true}, UpdatedAt: now, ID: id, FromState: StateAgent})
	if err != nil {
		return err
	}
	if n == 1 && kind != HandoffQuestion {
		return c.read(ctx, actor, id)
	}
	return nil
}

// dispatch sends an outbound turn if every gate allows it, and otherwise
// leaves it where a person can see why. agentAuto says the turn is the
// agent's own and goes only when the channel is automatic; a person's
// approval (approved mode) or own message goes when the channel is on at
// all. ack is the stop acknowledgement, which ignores the suppression it
// just created and the mode's approval step.
func (c *Conversations) dispatch(ctx context.Context, actor core.Actor, row store.Thread, t store.ThreadTurn, byPerson bool) (ThreadTurn, error) {
	now := c.clock.Now().UTC()
	hold := func(state, reason string) (ThreadTurn, error) {
		if _, err := c.db.Writer().SetThreadTurnState(ctx, store.SetThreadTurnStateParams{State: state, Reason: reason, UpdatedAt: now, ID: t.ID, FromState: t.State}); err != nil {
			return ThreadTurn{}, err
		}
		return c.getTurn(ctx, row.WorkspaceID, t.ID)
	}
	ack := t.Handoff == HandoffStop
	if !ack && c.cfg.Outreach != nil {
		if blocked, why, err := c.cfg.Outreach.Suppressed(ctx, actor, row.PersonID, Channel); err != nil {
			return ThreadTurn{}, err
		} else if blocked {
			return hold("held", why+"; not sent")
		}
	}
	if c.cfg.Policies != nil {
		if engaged, why, err := c.cfg.Policies.KillSwitchEngaged(ctx, row.WorkspaceID, 0); err != nil {
			return ThreadTurn{}, err
		} else if engaged {
			return hold("held", "kill switch engaged: "+why+"; nothing is sent")
		}
	}
	mode, version, err := c.channelMode(ctx, row.WorkspaceID)
	if err != nil {
		return ThreadTurn{}, err
	}
	switch {
	case mode == outreach.ModeAutomatic:
	case mode == outreach.ModeApproved && (byPerson || ack):
	case mode == outreach.ModeApproved:
		return hold("draft", fmt.Sprintf("waiting for a person: channel %s needs approval in outreach config v%d", Channel, version))
	case version == 0:
		return hold("held", "outreach is not configured: no channel may send")
	default:
		return hold("held", fmt.Sprintf("channel %s is off in outreach config v%d; nothing is sent", Channel, version))
	}
	if c.cfg.Messenger == nil {
		return hold("held", "no messaging adapter is wired on this server")
	}
	serves := false
	for _, ch := range c.cfg.Messenger.Channels() {
		if ch == Channel {
			serves = true
		}
	}
	if !serves {
		return hold("held", fmt.Sprintf("adapter %s does not serve %s", c.cfg.Messenger.Name(), Channel))
	}
	if t.Author == "agent" && !ack {
		if last, err := c.db.Reader().LastThreadTurn(ctx, row.ID); err == nil && last.ID != t.ID && last.Direction == "out" && (last.State == "sent" || last.State == "delivered" || last.State == "read") {
			return hold("held", "the customer has not replied to the last message; the agent never sends twice in a row")
		}
	}
	n, err := c.db.Reader().CountThreadTurnsSince(ctx, store.CountThreadTurnsSinceParams{ThreadID: row.ID, SentAt: sql.NullTime{Time: dayStart(now), Valid: true}})
	if err != nil {
		return ThreadTurn{}, err
	}
	if int(n) >= c.cfg.MaxRepliesPerDay {
		return hold("held", fmt.Sprintf("today's cap of %d messages on this conversation is reached", c.cfg.MaxRepliesPerDay))
	}
	if !row.LastInboundAt.Valid || now.Sub(row.LastInboundAt.Time.UTC()) >= c.cfg.Window {
		if t.Kind != "template" {
			return hold("held", fmt.Sprintf("outside the %d-hour window since the customer's last message; only a template may be sent, and none is approved", int(c.cfg.Window.Hours())))
		}
		if c.cfg.Templates == nil {
			return hold("held", "no template sender is wired")
		}
	} else if t.Kind == "template" {
		return hold("held", "inside the window a free reply is sent, not a template")
	}
	var ref string
	var sendErr error
	key := t.IdempotencyKey
	if t.Kind == "template" {
		ref, sendErr = c.cfg.Templates.SendTemplate(ctx, TemplateMessage{To: row.Address, Name: t.Body, Language: row.Language, IdempotencyKey: key})
	} else {
		ref, sendErr = c.cfg.Messenger.Send(ctx, outreach.OutboundMessage{Channel: Channel, To: row.Address, Language: row.Language, Body: t.Body, IdempotencyKey: key})
	}
	if sendErr != nil {
		return hold("failed", safe(sendErr))
	}
	if _, err := c.db.Writer().MarkThreadTurnSent(ctx, store.MarkThreadTurnSentParams{ProviderRef: ref, SentAt: sql.NullTime{Time: now, Valid: true}, UpdatedAt: now, ID: t.ID, FromState: t.State}); err != nil {
		return ThreadTurn{}, err
	}
	if err := c.db.Writer().TouchThreadOutbound(ctx, store.TouchThreadOutboundParams{LastOutboundAt: sql.NullTime{Time: now, Valid: true}, UpdatedAt: now, ID: row.ID}); err != nil {
		return ThreadTurn{}, err
	}
	if c.evidence != nil {
		if ev, _, err := c.evidence.Record(ctx, actor, evidence.Observation{Source: Channel, Kind: "message_out", SourceRef: key, Scope: fmt.Sprintf("person:%d", row.PersonID), Class: evidence.Observed, EventAt: &now,
			Payload: map[string]any{"body": t.Body, "author": t.Author, "providerRef": ref}, Completeness: evidence.Complete}); err == nil {
			_ = c.db.Writer().SetThreadTurnEvidence(ctx, store.SetThreadTurnEvidenceParams{EvidenceID: optionalID(ev.ID), ID: t.ID})
		}
	}
	return c.getTurn(ctx, row.WorkspaceID, t.ID)
}

// channelMode is what the outreach config says for the channel: "" when
// nothing is configured, with the config version.
func (c *Conversations) channelMode(ctx context.Context, workspaceID int64) (outreach.ChannelMode, int64, error) {
	if c.cfg.Outreach == nil {
		return "", 0, nil
	}
	st, err := c.cfg.Outreach.GetConfig(ctx, core.SystemIn(core.ViaJob, workspaceID))
	if err != nil {
		return "", 0, err
	}
	if st.Current == nil {
		return "", 0, nil
	}
	return st.Current.Document.Channels[Channel], st.Current.Version, nil
}

// Lists and reads.

func (c *Conversations) List(ctx context.Context, actor core.Actor, state string, limit int) ([]ThreadSummary, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	switch state {
	case "", StateAgent, StateWaiting, StatePerson, StateClosed:
	default:
		return nil, core.Invalid("state must be agent, waiting, person or closed")
	}
	rows, err := c.db.Reader().ListThreads(ctx, store.ListThreadsParams{WorkspaceID: actor.WorkspaceID, State: nullStr(state), Max: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]ThreadSummary, 0, len(rows))
	for _, r := range rows {
		s := ThreadSummary{Thread: c.fromRow(r)}
		if p, err := c.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: r.PersonID, WorkspaceID: actor.WorkspaceID}); err == nil {
			s.PersonName = p.DisplayName
		}
		s.ProjectName = c.projectName(ctx, actor.WorkspaceID, r.EnquiryID)
		turns, err := c.db.Reader().ListThreadTurns(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		for _, t := range turns {
			if t.State == "discarded" {
				continue
			}
			if t.Direction == "out" && (t.State == "draft" || t.State == "held" || t.State == "failed") {
				s.Pending++
				continue
			}
			s.LastBody, s.LastFrom = truncate(t.Body, 140), t.Author
		}
		out = append(out, s)
	}
	return out, nil
}

func (c *Conversations) Get(ctx context.Context, actor core.Actor, id int64) (ThreadDetail, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return ThreadDetail{}, core.ErrForbidden
	}
	row, err := c.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return ThreadDetail{}, err
	}
	d := ThreadDetail{Conversation: c.fromRow(row), Turns: []ThreadTurn{}, Corrections: []ThreadCorrection{}}
	if p, err := c.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: row.PersonID, WorkspaceID: actor.WorkspaceID}); err == nil {
		d.Person = crm.Person{ID: p.ID, DisplayName: p.DisplayName, PrimaryPhone: strPtr(p.PrimaryPhone), PrimaryEmail: strPtr(p.PrimaryEmail), Country: strPtr(p.Country), Language: strPtr(p.Language), Notes: p.Notes, CreatedAt: p.CreatedAt.UTC(), UpdatedAt: p.UpdatedAt.UTC()}
	}
	d.ProjectName = c.projectName(ctx, actor.WorkspaceID, row.EnquiryID)
	turns, err := c.db.Reader().ListThreadTurns(ctx, id)
	if err != nil {
		return ThreadDetail{}, err
	}
	for _, t := range turns {
		d.Turns = append(d.Turns, turnFromRow(t))
	}
	cs, err := c.db.Reader().ListThreadCorrections(ctx, id)
	if err != nil {
		return ThreadDetail{}, err
	}
	for _, r := range cs {
		cc := ThreadCorrection{ID: r.ID, ConversationID: r.ThreadID, Outcome: r.Outcome, Note: r.Note, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC(), Answers: map[string]qualification.ReadingAnswer{}}
		_ = json.Unmarshal([]byte(r.Answers), &cc.Answers)
		d.Corrections = append(d.Corrections, cc)
	}
	return d, nil
}

// ThreadsWaiting lists what the supervisor shows: handoffs nobody took, drafts
// waiting for a person, readings pending review and failed sends.
type ThreadsWaiting struct {
	Handoffs []ThreadSummary `json:"handoffs"`
	Drafts   []ThreadTurn    `json:"drafts"`
	Readings []Thread        `json:"readings"`
	Failed   []ThreadTurn    `json:"failed"`
}

func (c *Conversations) Pending(ctx context.Context, actor core.Actor) (ThreadsWaiting, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return ThreadsWaiting{}, core.ErrForbidden
	}
	w := ThreadsWaiting{Handoffs: []ThreadSummary{}, Drafts: []ThreadTurn{}, Readings: []Thread{}, Failed: []ThreadTurn{}}
	hs, err := c.List(ctx, actor, StateWaiting, 200)
	if err != nil {
		return ThreadsWaiting{}, err
	}
	w.Handoffs = hs
	for _, st := range []string{"draft", "held"} {
		rows, err := c.db.Reader().ListThreadTurnsByState(ctx, store.ListThreadTurnsByStateParams{WorkspaceID: actor.WorkspaceID, State: st, Limit: 200})
		if err != nil {
			return ThreadsWaiting{}, err
		}
		for _, r := range rows {
			w.Drafts = append(w.Drafts, turnFromRow(r))
		}
	}
	rows, err := c.db.Reader().ListThreadTurnsByState(ctx, store.ListThreadTurnsByStateParams{WorkspaceID: actor.WorkspaceID, State: "failed", Limit: 200})
	if err != nil {
		return ThreadsWaiting{}, err
	}
	for _, r := range rows {
		w.Failed = append(w.Failed, turnFromRow(r))
	}
	pend, err := c.db.Reader().ListThreadsPendingReview(ctx, store.ListThreadsPendingReviewParams{WorkspaceID: actor.WorkspaceID, Limit: 200})
	if err != nil {
		return ThreadsWaiting{}, err
	}
	for _, r := range pend {
		w.Readings = append(w.Readings, c.fromRow(r))
	}
	return w, nil
}

func (c *Conversations) projectName(ctx context.Context, workspaceID int64, enquiryID sql.NullInt64) string {
	if !enquiryID.Valid {
		return ""
	}
	enq, err := c.db.Reader().GetEnquiry(ctx, store.GetEnquiryParams{ID: enquiryID.Int64, WorkspaceID: workspaceID})
	if err != nil || !enq.ProjectID.Valid {
		return ""
	}
	p, err := c.db.Reader().GetProject(ctx, store.GetProjectParams{ID: enq.ProjectID.Int64, WorkspaceID: workspaceID})
	if err != nil {
		return ""
	}
	return p.Name
}

func (c *Conversations) row(ctx context.Context, workspaceID, id int64) (store.Thread, error) {
	row, err := c.db.Reader().GetThread(ctx, store.GetThreadParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Thread{}, core.NotFound("conversation %d not found", id)
	}
	return row, err
}

func (c *Conversations) turn(ctx context.Context, workspaceID, id int64) (store.ThreadTurn, error) {
	row, err := c.db.Reader().GetThreadTurn(ctx, store.GetThreadTurnParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.ThreadTurn{}, core.NotFound("turn %d not found", id)
	}
	return row, err
}

func (c *Conversations) get(ctx context.Context, workspaceID, id int64) (Thread, error) {
	row, err := c.row(ctx, workspaceID, id)
	if err != nil {
		return Thread{}, err
	}
	return c.fromRow(row), nil
}

func (c *Conversations) getTurn(ctx context.Context, workspaceID, id int64) (ThreadTurn, error) {
	row, err := c.turn(ctx, workspaceID, id)
	if err != nil {
		return ThreadTurn{}, err
	}
	return turnFromRow(row), nil
}

func (c *Conversations) fromRow(r store.Thread) Thread {
	v := Thread{ID: r.ID, PersonID: r.PersonID, EnquiryID: idPtr(r.EnquiryID), Channel: r.Channel, Address: r.Address, State: r.State, StateReason: r.StateReason, HandoffKind: r.HandoffKind, HandoffAt: timePtr(r.HandoffAt), Language: r.Language,
		LastInboundAt: timePtr(r.LastInboundAt), LastOutboundAt: timePtr(r.LastOutboundAt), Summary: r.Summary, Outcome: r.Outcome, Confidence: r.Confidence, RequestedHuman: r.RequestedHuman == 1, ReviewStatus: r.ReviewStatus, ReviewReason: r.ReviewReason,
		ReadAt: timePtr(r.ReadAt), ReadTurns: r.ReadTurns, AppliedKey: r.AppliedKey, AppliedVersion: r.AppliedVersion, AppliedAt: timePtr(r.AppliedAt), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(), Answers: map[string]qualification.ReadingAnswer{}}
	if r.LastInboundAt.Valid {
		w := r.LastInboundAt.Time.UTC().Add(c.cfg.Window)
		v.WindowClosesAt = &w
	}
	_ = json.Unmarshal([]byte(r.Answers), &v.Answers)
	_ = json.Unmarshal([]byte(r.AppliedDetail), &v.Applied)
	if v.Applied.Attributes == nil {
		v.Applied.Attributes = []string{}
	}
	if v.Applied.Held == nil {
		v.Applied.Held = []string{}
	}
	return v
}

func turnFromRow(r store.ThreadTurn) ThreadTurn {
	t := ThreadTurn{ID: r.ID, ConversationID: r.ThreadID, Direction: r.Direction, Author: r.Author, Kind: r.Kind, Body: r.Body, State: r.State, Reason: r.Reason, Handoff: r.Handoff, ProviderRef: r.ProviderRef, EvidenceID: idPtr(r.EvidenceID), CreatedBy: idPtr(r.CreatedBy), ApprovedBy: idPtr(r.ApprovedBy), OccurredAt: r.OccurredAt.UTC(), SentAt: timePtr(r.SentAt)}
	if r.Claims != "" && r.Claims != "{}" && r.Claims != "null" {
		var rep brands.ClaimReport
		if json.Unmarshal([]byte(r.Claims), &rep) == nil {
			t.Claims = &rep
		}
	}
	return t
}

// Words the agent must never miss, whatever the model says.

var stopWords = []string{"stop", "unsubscribe", "don't message", "do not message", "dont message", "don't contact", "do not contact", "remove me", "leave me alone", "توقف", "لا تراسلني", "لا تتصل", "أوقف", "الغاء الاشتراك", "إلغاء الاشتراك"}

var humanWords = []string{"human", "real person", "a person", "an agent", "someone", "call me", "speak to", "talk to", "representative", "sales person", "salesperson", "manager", "شخص", "موظف", "اتصل بي", "اتصلي بي", "كلمني", "أريد التحدث", "اريد التحدث", "بشر", "مندوب"}

func saysStop(body string) bool {
	b := strings.ToLower(strings.TrimSpace(body))
	if b == "stop" || b == "unsubscribe" {
		return true
	}
	if len([]rune(b)) > 60 {
		return false
	}
	for _, w := range stopWords {
		if strings.Contains(b, w) {
			return true
		}
	}
	return false
}

func asksForHuman(body string) bool {
	b := strings.ToLower(body)
	for _, w := range humanWords {
		if strings.Contains(b, w) {
			return true
		}
	}
	return false
}

func looksArabic(s string) bool {
	n := 0
	for _, r := range s {
		if r >= 0x0600 && r <= 0x06FF {
			n++
		}
	}
	return n > 0 && n*2 >= len([]rune(strings.ReplaceAll(s, " ", "")))
}

func dayStart(t time.Time) time.Time {
	l := t.In(dubai)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, dubai).UTC()
}

var dubai = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Dubai")
	if err != nil {
		return time.FixedZone("Dubai", 4*3600)
	}
	return loc
}()

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

func safe(err error) string {
	var me *assistant.ModelError
	if errors.As(err, &me) {
		return me.Safe
	}
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

func itoa(n int64) string { return fmt.Sprintf("%d", n) }

// outKey is an outbound turn's idempotency key: the thread, the instant
// and a nonce, so two drafts in one clock tick are two sends.
func outKey(id int64, now time.Time) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("out:%d:%d:%s", id, now.UnixNano(), hex.EncodeToString(b[:]))
}

func sortedKeys(m map[string]qualification.ReadingAnswer) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

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
