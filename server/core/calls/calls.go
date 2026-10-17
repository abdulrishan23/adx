// Package calls is the lifecycle of a qualification call: the policy that
// allows calling at all, the contactability checks that run before every
// attempt, an atomic reservation so two requests cannot ring one phone
// twice, provider callbacks applied in order with redeliveries ignored,
// unknown results reconciled before another attempt, and recording
// retention. The speech engine is B4.5's; this package never touches audio.
package calls

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/voice"
	"github.com/WokeWorks/adx/server/store"
)

// Route names the telecom provider and who authorized its live use. A
// policy whose route has no provider or no authorization allows no call.
type Route struct {
	Provider     string `json:"provider"`
	AuthorizedBy string `json:"authorizedBy"`
	AuthorizedOn string `json:"authorizedOn"`
	Note         string `json:"note"`
}

type CallingWindow struct {
	Start string `json:"start"` // HH:MM Dubai
	End   string `json:"end"`
}

type RecordingPolicy struct {
	Enabled       bool `json:"enabled"`
	RetentionDays int  `json:"retentionDays"`
	// AccessRole is the least role that may open a recording.
	AccessRole string `json:"accessRole"`
}

type CallPolicyDocument struct {
	Route             Route           `json:"route"`
	ConsentRequired   bool            `json:"consentRequired"`
	Window            CallingWindow   `json:"window"`
	MaxConcurrent     int             `json:"maxConcurrent"`
	MaxAttemptsPerDay int             `json:"maxAttemptsPerDay"`
	FromNumber        string          `json:"fromNumber"`
	Recording         RecordingPolicy `json:"recording"`
	Note              string          `json:"note"`
}

type CallPolicy struct {
	Version   int64              `json:"version"`
	Document  CallPolicyDocument `json:"document"`
	CreatedBy *int64             `json:"createdBy"`
	CreatedAt time.Time          `json:"createdAt"`
}

type CallPolicyState struct {
	Configured bool        `json:"configured"`
	Current    *CallPolicy `json:"current"`
	// Adapter names the telephony adapter wired on this server, or "".
	Adapter string `json:"adapter"`
	// LiveCalling says whether a live call can be placed right now and why not.
	LiveCalling bool               `json:"liveCalling"`
	Reasons     []string           `json:"reasons"`
	Suggested   CallPolicyDocument `json:"suggested"`
}

type Call struct {
	ID                 int64      `json:"id"`
	PersonID           int64      `json:"personId"`
	EnquiryID          int64      `json:"enquiryId"`
	OccurrenceKey      string     `json:"occurrenceKey"`
	Mode               string     `json:"mode"`
	To                 string     `json:"to"`
	From               string     `json:"from"`
	State              string     `json:"state"`
	Reason             string     `json:"reason"`
	Provider           string     `json:"provider"`
	ProviderCallID     string     `json:"providerCallId"`
	PolicyVersion      int64      `json:"policyVersion"`
	ConsentBasis       string     `json:"consentBasis"`
	Attempt            int64      `json:"attempt"`
	Record             bool       `json:"record"`
	RecordingRef       string     `json:"recordingRef"`
	RecordingDeletedAt *time.Time `json:"recordingDeletedAt"`
	DurationSeconds    *int64     `json:"durationSeconds"`
	RequestedBy        *int64     `json:"requestedBy"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	StartedAt          *time.Time `json:"startedAt"`
	EndedAt            *time.Time `json:"endedAt"`
	UnknownSince       *time.Time `json:"unknownSince"`
}

type CallEvent struct {
	ID         int64           `json:"id"`
	CallID     int64           `json:"callId"`
	Kind       string          `json:"kind"`
	FromState  string          `json:"fromState"`
	ToState    string          `json:"toState"`
	Detail     string          `json:"detail"`
	Payload    json.RawMessage `json:"payload"`
	ReceivedAt time.Time       `json:"receivedAt"`
}

type CallDetail struct {
	Call   Call        `json:"call"`
	Events []CallEvent `json:"events"`
}

type CallInput struct {
	EnquiryID    int64  `json:"enquiryId"`
	Mode         string `json:"mode"`
	ConsentBasis string `json:"consentBasis"`
	Reason       string `json:"reason"`
}

// Contactability is every reason a call may not be placed right now. Empty
// Blockers means it may.
type Contactability struct {
	EnquiryID int64    `json:"enquiryId"`
	PersonID  int64    `json:"personId"`
	To        string   `json:"to"`
	Blockers  []string `json:"blockers"`
	// Active is the call already running or unknown for this enquiry, if any.
	Active *Call `json:"active"`
}

type Config struct {
	Clock     core.Clock
	Telephony voice.Telephony
	Policies  *policies.Policies
	Outreach  *outreach.Outreach
	CRM       *crm.CRM
	// PublicURL is where the provider reaches ADX for the media stream and
	// status callbacks.
	PublicURL string
	// UnknownGrace is how long an unknown call waits before a reconcile that
	// finds nothing at the provider marks it failed.
	UnknownGrace time.Duration
}

type Calls struct {
	db    *store.DB
	cfg   Config
	clock core.Clock
}

func New(db *store.DB, cfg Config) *Calls {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	if cfg.UnknownGrace == 0 {
		cfg.UnknownGrace = 2 * time.Minute
	}
	return &Calls{db: db, cfg: cfg, clock: cfg.Clock}
}

// SuggestedPolicy is a starting point. Its route is empty on purpose: the
// provider and the authorization are the owner's to supply.
func SuggestedPolicy() CallPolicyDocument {
	return CallPolicyDocument{ConsentRequired: true, Window: CallingWindow{Start: "09:00", End: "20:00"}, MaxConcurrent: 2, MaxAttemptsPerDay: 2,
		Recording: RecordingPolicy{Enabled: false, RetentionDays: 90, AccessRole: "admin"},
		Note:      "Suggested: consent required, Dubai daytime only, two lines, two attempts a day, no recording. The route stays empty until Reportage picks and authorizes a telecom provider."}
}

func (c *Calls) GetPolicy(ctx context.Context, actor core.Actor) (CallPolicyState, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return CallPolicyState{}, core.ErrForbidden
	}
	cur, err := c.current(ctx, actor.WorkspaceID)
	if err != nil {
		return CallPolicyState{}, err
	}
	st := CallPolicyState{Configured: cur != nil, Current: cur, Suggested: SuggestedPolicy(), Reasons: []string{}}
	if c.cfg.Telephony != nil {
		st.Adapter = c.cfg.Telephony.Name()
	}
	st.Reasons = c.routeBlockers(cur)
	st.LiveCalling = len(st.Reasons) == 0
	return st, nil
}

// routeBlockers are the reasons live calling is off, independent of any
// person: no policy, no provider named, no authorization, no adapter, or an
// adapter that is not the named provider.
func (c *Calls) routeBlockers(cur *CallPolicy) []string {
	out := []string{}
	if cur == nil {
		return append(out, "no calling policy is saved")
	}
	r := cur.Document.Route
	if r.Provider == "" {
		out = append(out, "no telecom route is chosen in the calling policy")
	}
	if r.AuthorizedBy == "" || r.AuthorizedOn == "" {
		out = append(out, "the telecom route is not authorized in writing (authorizedBy and authorizedOn)")
	}
	switch {
	case c.cfg.Telephony == nil:
		out = append(out, "no telephony adapter is wired on this server")
	case r.Provider != "" && c.cfg.Telephony.Name() != r.Provider:
		out = append(out, fmt.Sprintf("the wired adapter is %s but the policy names %s", c.cfg.Telephony.Name(), r.Provider))
	}
	if cur.Document.FromNumber == "" {
		out = append(out, "no caller number is configured")
	}
	return out
}

func (c *Calls) PutPolicy(ctx context.Context, actor core.Actor, doc CallPolicyDocument) (CallPolicy, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return CallPolicy{}, core.ErrForbidden
	}
	if _, _, err := parseWindow(doc.Window); err != nil {
		return CallPolicy{}, err
	}
	if doc.MaxConcurrent < 0 || doc.MaxAttemptsPerDay < 0 || doc.Recording.RetentionDays < 0 {
		return CallPolicy{}, core.Invalid("limits must not be negative")
	}
	if doc.Recording.Enabled && doc.Recording.RetentionDays == 0 {
		return CallPolicy{}, core.Invalid("recording needs a retention period in days")
	}
	switch doc.Recording.AccessRole {
	case "", "admin", "marketer":
	default:
		return CallPolicy{}, core.Invalid("recording access role must be admin or marketer")
	}
	if (doc.Route.AuthorizedBy == "") != (doc.Route.AuthorizedOn == "") {
		return CallPolicy{}, core.Invalid("route authorization needs both who and when")
	}
	raw, _ := json.Marshal(doc)
	var out CallPolicy
	err := c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var version int64 = 1
		if cur, err := q.GetCurrentCallPolicy(ctx, actor.WorkspaceID); err == nil {
			version = cur.Version + 1
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		row, err := q.CreateCallPolicy(ctx, store.CreateCallPolicyParams{WorkspaceID: actor.WorkspaceID, Version: version, Document: string(raw), CreatedBy: optionalID(actor.UserID), CreatedAt: c.clock.Now().UTC()})
		if err != nil {
			return err
		}
		out, err = policyFromRow(row)
		return err
	})
	return out, err
}

func parseWindow(w CallingWindow) (start, end int, err error) {
	if w.Start == "" && w.End == "" {
		return 0, 0, nil
	}
	parse := func(s string) (int, error) {
		var h, m int
		if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
			return 0, core.Invalid("calling window times are HH:MM")
		}
		return h*60 + m, nil
	}
	if start, err = parse(w.Start); err != nil {
		return 0, 0, err
	}
	if end, err = parse(w.End); err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

// dayStart is midnight of the Dubai reporting day that contains t, in UTC.
func dayStart(t time.Time) time.Time {
	l := t.In(reporting.ReportingZone)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, reporting.ReportingZone).UTC()
}

// inWindow is start-inclusive, end-exclusive, Dubai time; an unset window
// is always open and an overnight window wraps.
func inWindow(w CallingWindow, now time.Time) bool {
	start, end, err := parseWindow(w)
	if err != nil || start == end {
		return true
	}
	local := now.In(reporting.ReportingZone)
	m := local.Hour()*60 + local.Minute()
	if start < end {
		return m >= start && m < end
	}
	return m >= start || m < end
}

// Contactability runs every check a call must pass, without placing one.
func (c *Calls) Contactability(ctx context.Context, actor core.Actor, enquiryID int64, consentBasis string) (Contactability, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return Contactability{}, core.ErrForbidden
	}
	cur, err := c.current(ctx, actor.WorkspaceID)
	if err != nil {
		return Contactability{}, err
	}
	return c.check(ctx, actor, cur, enquiryID, consentBasis)
}

func (c *Calls) check(ctx context.Context, actor core.Actor, cur *CallPolicy, enquiryID int64, consentBasis string) (Contactability, error) {
	out := Contactability{EnquiryID: enquiryID, Blockers: []string{}}
	enq, err := c.db.Reader().GetEnquiry(ctx, store.GetEnquiryParams{ID: enquiryID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return out, core.NotFound("enquiry %d not found", enquiryID)
	}
	if err != nil {
		return out, err
	}
	out.PersonID = enq.PersonID
	out.Blockers = append(out.Blockers, c.routeBlockers(cur)...)
	if enq.State == "closed" {
		out.Blockers = append(out.Blockers, "the enquiry is closed")
	}
	if c.cfg.Policies != nil {
		st, err := c.cfg.Policies.Get(ctx, actor)
		if err != nil {
			return out, err
		}
		for _, k := range st.KillSwitches {
			if k.Engaged && k.AccountID == 0 {
				out.Blockers = append(out.Blockers, "the workspace kill switch is engaged: "+k.Reason)
			}
		}
	}
	p, err := c.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: enq.PersonID, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return out, err
	}
	if p.PrimaryPhone.Valid {
		out.To = p.PrimaryPhone.String
	} else {
		out.Blockers = append(out.Blockers, "the person has no phone number on record")
	}
	if c.cfg.Outreach != nil {
		if blocked, why, err := c.cfg.Outreach.Suppressed(ctx, actor, enq.PersonID, "call"); err != nil {
			return out, err
		} else if blocked {
			out.Blockers = append(out.Blockers, "opted out: "+why)
		}
	}
	if c.cfg.CRM != nil {
		d, err := c.cfg.CRM.Get(ctx, actor, enq.PersonID)
		if err != nil {
			return out, err
		}
		for _, a := range d.Attributes {
			if a.Key == "do_not_contact" && (strings.EqualFold(a.Value, "yes") || strings.EqualFold(a.Value, "true")) {
				out.Blockers = append(out.Blockers, "the customer asked not to be contacted")
			}
		}
	}
	if cur != nil {
		if cur.Document.ConsentRequired && strings.TrimSpace(consentBasis) == "" {
			out.Blockers = append(out.Blockers, "consent basis is required by the calling policy (where the customer agreed to a call)")
		}
		if !inWindow(cur.Document.Window, c.clock.Now()) {
			out.Blockers = append(out.Blockers, fmt.Sprintf("outside calling hours %s-%s Dubai", cur.Document.Window.Start, cur.Document.Window.End))
		}
		if cur.Document.MaxConcurrent > 0 {
			n, err := c.db.Reader().CountActiveCalls(ctx, actor.WorkspaceID)
			if err != nil {
				return out, err
			}
			if n >= int64(cur.Document.MaxConcurrent) {
				out.Blockers = append(out.Blockers, fmt.Sprintf("at capacity: %d of %d lines in use", n, cur.Document.MaxConcurrent))
			}
		}
		if cur.Document.MaxAttemptsPerDay > 0 {
			n, err := c.db.Reader().CountCallsForEnquirySince(ctx, store.CountCallsForEnquirySinceParams{EnquiryID: enquiryID, CreatedAt: dayStart(c.clock.Now())})
			if err != nil {
				return out, err
			}
			if n >= int64(cur.Document.MaxAttemptsPerDay) {
				out.Blockers = append(out.Blockers, fmt.Sprintf("already %d attempt(s) today; the policy allows %d", n, cur.Document.MaxAttemptsPerDay))
			}
		}
	}
	active, err := c.db.Reader().ListActiveCallsForEnquiry(ctx, enquiryID)
	if err != nil {
		return out, err
	}
	if len(active) > 0 {
		a := callFromRow(active[0])
		out.Active = &a
		if a.State == "unknown" {
			out.Blockers = append(out.Blockers, fmt.Sprintf("call %d has an unknown result; reconcile it before another attempt", a.ID))
		} else {
			out.Blockers = append(out.Blockers, fmt.Sprintf("call %d is %s", a.ID, a.State))
		}
	}
	return out, nil
}

// Request reserves and places a call. The reservation is one transaction
// that re-counts active calls and refuses a second active call for the
// enquiry, so two simultaneous requests cannot ring the phone twice; the
// provider is asked only after the row exists. A provider error leaves the
// call unknown, to be reconciled, never silently retried.
func (c *Calls) Request(ctx context.Context, actor core.Actor, in CallInput) (Call, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return Call{}, core.ErrForbidden
	}
	if in.Mode == "" {
		in.Mode = "qualification"
	}
	if in.Mode != "qualification" && in.Mode != "callback" {
		return Call{}, core.Invalid("mode must be qualification or callback")
	}
	cur, err := c.current(ctx, actor.WorkspaceID)
	if err != nil {
		return Call{}, err
	}
	ct, err := c.check(ctx, actor, cur, in.EnquiryID, in.ConsentBasis)
	if err != nil {
		return Call{}, err
	}
	if len(ct.Blockers) > 0 {
		return Call{}, core.Conflict("call not placed: %s", strings.Join(ct.Blockers, "; "))
	}
	now := c.clock.Now().UTC()
	day := now.In(reporting.ReportingZone).Format("2006-01-02")
	var row store.Call
	err = c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		// Re-check the two things another request could have changed since
		// the read above: capacity and an active call for this enquiry.
		if cur.Document.MaxConcurrent > 0 {
			n, err := q.CountActiveCalls(ctx, actor.WorkspaceID)
			if err != nil {
				return err
			}
			if n >= int64(cur.Document.MaxConcurrent) {
				return core.Conflict("call not placed: at capacity")
			}
		}
		active, err := q.ListActiveCallsForEnquiry(ctx, in.EnquiryID)
		if err != nil {
			return err
		}
		if len(active) > 0 {
			return core.Conflict("call not placed: call %d is already %s for this enquiry", active[0].ID, active[0].State)
		}
		n, err := q.CountCallsForEnquirySince(ctx, store.CountCallsForEnquirySinceParams{EnquiryID: in.EnquiryID, CreatedAt: dayStart(now)})
		if err != nil {
			return err
		}
		key := fmt.Sprintf("enq:%d:%s:attempt-%d", in.EnquiryID, day, n+1)
		row, err = q.CreateCall(ctx, store.CreateCallParams{WorkspaceID: actor.WorkspaceID, PersonID: ct.PersonID, EnquiryID: in.EnquiryID, OccurrenceKey: key, Mode: in.Mode, ToNumber: ct.To, FromNumber: cur.Document.FromNumber,
			Reason: strings.TrimSpace(in.Reason), Provider: c.cfg.Telephony.Name(), PolicyVersion: cur.Version, ConsentBasis: strings.TrimSpace(in.ConsentBasis), Attempt: n + 1, Record: boolInt(cur.Document.Recording.Enabled), RequestedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		_, err = q.CreateCallEvent(ctx, store.CreateCallEventParams{CallID: row.ID, Kind: "reserved", FromState: "", ToState: "reserved", Detail: "reserved under policy v" + fmt.Sprint(cur.Version), Payload: "{}", EventKey: "reserved", ReceivedAt: now})
		return err
	})
	if err != nil {
		return Call{}, err
	}
	return c.place(ctx, row)
}

// place asks the provider for the reserved call. Success is queued; an
// error is unknown, because the provider may have placed it anyway.
func (c *Calls) place(ctx context.Context, row store.Call) (Call, error) {
	now := c.clock.Now().UTC()
	base := strings.TrimRight(c.cfg.PublicURL, "/")
	req := voice.CallRequest{OccurrenceKey: row.OccurrenceKey, To: row.ToNumber, From: row.FromNumber, StreamURL: streamURL(base, row.ID), StatusCallbackURL: base + "/webhooks/voice/status", Record: row.Record == 1}
	id, err := c.cfg.Telephony.StartCall(ctx, req)
	if err != nil {
		if terr := c.transition(ctx, row.ID, "reserved", "unknown", "provider", "the provider did not confirm the call: "+safe(err), "unknown:"+now.Format(time.RFC3339Nano), nil, sql.NullTime{Time: now, Valid: true}); terr != nil {
			return Call{}, terr
		}
		return c.get(ctx, row.WorkspaceID, row.ID)
	}
	if err := c.db.Writer().SetCallProviderID(ctx, store.SetCallProviderIDParams{ProviderCallID: id, UpdatedAt: now, ID: row.ID}); err != nil {
		return Call{}, err
	}
	if err := c.transition(ctx, row.ID, "reserved", "queued", "provider", "accepted by "+c.cfg.Telephony.Name(), "queued", nil, sql.NullTime{}); err != nil {
		return Call{}, err
	}
	return c.get(ctx, row.WorkspaceID, row.ID)
}

func streamURL(base string, callID int64) string {
	ws := strings.Replace(strings.Replace(base, "https://", "wss://", 1), "http://", "ws://", 1)
	return fmt.Sprintf("%s/webhooks/voice/stream/%d", ws, callID)
}

// transition moves a call from one state to another and records the event.
// A redelivered event (same key) is ignored; a move out of a terminal state
// is refused.
func (c *Calls) transition(ctx context.Context, callID int64, from, to, kind, detail, eventKey string, ev *voice.StatusEvent, unknownSince sql.NullTime) error {
	now := c.clock.Now().UTC()
	return c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		payload := []byte("{}")
		var started, ended sql.NullTime
		var dur sql.NullInt64
		if ev != nil {
			payload, _ = json.Marshal(map[string]any{"providerCallId": ev.ProviderCallID, "state": ev.State, "durationSeconds": int64(ev.Duration.Seconds()), "recordingRef": ev.RecordingRef, "at": ev.At.UTC(), "eventId": ev.EventID})
			if ev.Duration > 0 {
				dur = sql.NullInt64{Int64: int64(ev.Duration.Seconds()), Valid: true}
			}
		}
		if to == "in_progress" {
			started = sql.NullTime{Time: now, Valid: true}
		}
		if voice.CallState(to).Terminal() {
			ended = sql.NullTime{Time: now, Valid: true}
		}
		if _, err := q.CreateCallEvent(ctx, store.CreateCallEventParams{CallID: callID, Kind: kind, FromState: from, ToState: to, Detail: detail, Payload: string(payload), EventKey: eventKey, ReceivedAt: now}); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return errRedelivered
			}
			return err
		}
		n, err := q.SetCallState(ctx, store.SetCallStateParams{State: to, Reason: detail, UpdatedAt: now, StartedAt: started, EndedAt: ended, UnknownSince: unknownSince, DurationSeconds: dur, ID: callID, State_2: from})
		if err != nil {
			return err
		}
		if n != 1 {
			return core.Conflict("call %d is no longer %s", callID, from)
		}
		if ev != nil && ev.RecordingRef != "" {
			if err := q.SetCallRecording(ctx, store.SetCallRecordingParams{RecordingRef: ev.RecordingRef, UpdatedAt: now, ID: callID}); err != nil {
				return err
			}
		}
		return nil
	})
}

var errRedelivered = errors.New("event already applied")

// Callback applies a provider status callback. The adapter validates the
// signature; a bad one is refused with ErrBadSignature and nothing is
// recorded. Unknown calls are matched by provider id; events for calls ADX
// did not place are refused.
func (c *Calls) Callback(ctx context.Context, r *http.Request) (Call, error) {
	if c.cfg.Telephony == nil {
		return Call{}, core.Unavailable(nil, "no telephony adapter is wired")
	}
	ev, err := c.cfg.Telephony.ParseStatusCallback(r)
	if err != nil {
		return Call{}, err
	}
	return c.Apply(ctx, ev)
}

// Apply applies one provider event to the call it names.
func (c *Calls) Apply(ctx context.Context, ev voice.StatusEvent) (Call, error) {
	row, err := c.db.Reader().GetCallByProviderID(ctx, store.GetCallByProviderIDParams{Provider: c.cfg.Telephony.Name(), ProviderCallID: ev.ProviderCallID})
	if errors.Is(err, sql.ErrNoRows) {
		return Call{}, core.NotFound("no call with provider id %q", ev.ProviderCallID)
	}
	if err != nil {
		return Call{}, err
	}
	to := string(ev.State)
	if !validState(to) {
		return Call{}, core.Invalid("unknown call state %q", to)
	}
	key := "provider:" + ev.EventID
	if ev.EventID == "" {
		key = "provider:" + to
	}
	if voice.CallState(row.State).Terminal() {
		// Record the late event for the timeline without moving the call.
		payload, _ := json.Marshal(map[string]any{"state": to, "eventId": ev.EventID, "ignored": "call already " + row.State})
		_, _ = c.db.Writer().CreateCallEvent(ctx, store.CreateCallEventParams{CallID: row.ID, Kind: "late", FromState: row.State, ToState: row.State, Detail: "late event ignored: " + to, Payload: string(payload), EventKey: key, ReceivedAt: c.clock.Now().UTC()})
		return c.get(ctx, row.WorkspaceID, row.ID)
	}
	if to == row.State {
		return c.get(ctx, row.WorkspaceID, row.ID)
	}
	if rank(to) < rank(row.State) && !voice.CallState(to).Terminal() {
		// Out of order (ringing after in_progress): keep the later state.
		return c.get(ctx, row.WorkspaceID, row.ID)
	}
	err = c.transition(ctx, row.ID, row.State, to, "provider", "provider reported "+to, key, &ev, sql.NullTime{})
	if err != nil && !errors.Is(err, errRedelivered) {
		return Call{}, err
	}
	return c.get(ctx, row.WorkspaceID, row.ID)
}

func validState(s string) bool {
	switch voice.CallState(s) {
	case voice.CallQueued, voice.CallRinging, voice.CallInProgress, voice.CallCompleted, voice.CallNoAnswer, voice.CallBusy, voice.CallFailed, voice.CallCanceled:
		return true
	}
	return false
}

func rank(s string) int {
	switch s {
	case "reserved":
		return 0
	case "unknown":
		return 1
	case "queued":
		return 2
	case "ringing":
		return 3
	case "in_progress":
		return 4
	}
	return 5
}

// Reconcile settles an unknown call by asking the provider. Found: the
// provider's state is applied. Not found after the grace period: failed,
// nothing was placed. Not found within it: still unknown.
func (c *Calls) Reconcile(ctx context.Context, actor core.Actor, callID int64) (Call, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Call{}, core.ErrForbidden
	}
	row, err := c.row(ctx, actor.WorkspaceID, callID)
	if err != nil {
		return Call{}, err
	}
	if row.State != "unknown" {
		return Call{}, core.Conflict("call %d is %s, not unknown", callID, row.State)
	}
	rec, ok := c.cfg.Telephony.(voice.Reconciler)
	if !ok || c.cfg.Telephony == nil {
		return Call{}, core.Unavailable(nil, "the telephony adapter cannot look calls up; settle call %d by hand", callID)
	}
	now := c.clock.Now().UTC()
	// The provider may know the call by the occurrence key when no id came back.
	lookupID := row.ProviderCallID
	if lookupID == "" {
		lookupID = row.OccurrenceKey
	}
	ev, found, err := rec.LookupCall(ctx, lookupID)
	if err != nil {
		return Call{}, core.Unavailable(err, "the provider could not be reached; call %d stays unknown", callID)
	}
	if !found {
		if row.UnknownSince.Valid && now.Sub(row.UnknownSince.Time) < c.cfg.UnknownGrace {
			return callFromRow(row), nil
		}
		if err := c.transition(ctx, row.ID, "unknown", "failed", "reconcile", "the provider has no record of the call; nothing was placed", "reconcile:"+now.Format(time.RFC3339Nano), nil, sql.NullTime{}); err != nil {
			return Call{}, err
		}
		return c.get(ctx, actor.WorkspaceID, callID)
	}
	if ev.ProviderCallID != "" && row.ProviderCallID == "" {
		if err := c.db.Writer().SetCallProviderID(ctx, store.SetCallProviderIDParams{ProviderCallID: ev.ProviderCallID, UpdatedAt: now, ID: row.ID}); err != nil {
			return Call{}, err
		}
	}
	to := string(ev.State)
	if !validState(to) {
		return Call{}, core.Invalid("provider reported unknown state %q", to)
	}
	if err := c.transition(ctx, row.ID, "unknown", to, "reconcile", "the provider reports "+to, "reconcile:"+now.Format(time.RFC3339Nano), &ev, sql.NullTime{}); err != nil {
		return Call{}, err
	}
	return c.get(ctx, actor.WorkspaceID, callID)
}

// Hangup ends a live call. The provider's callback then settles the state;
// the request is recorded as an event.
func (c *Calls) Hangup(ctx context.Context, actor core.Actor, callID int64, reason string) (Call, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return Call{}, core.ErrForbidden
	}
	row, err := c.row(ctx, actor.WorkspaceID, callID)
	if err != nil {
		return Call{}, err
	}
	if voice.CallState(row.State).Terminal() || row.State == "reserved" || row.State == "unknown" {
		return Call{}, core.Conflict("call %d is %s; nothing to hang up", callID, row.State)
	}
	if c.cfg.Telephony == nil {
		return Call{}, core.Unavailable(nil, "no telephony adapter is wired")
	}
	if err := c.cfg.Telephony.Hangup(ctx, row.ProviderCallID); err != nil {
		return Call{}, core.Unavailable(err, "the provider refused the hangup")
	}
	now := c.clock.Now().UTC()
	payload, _ := json.Marshal(map[string]any{"by": actor.UserID, "reason": reason})
	_, _ = c.db.Writer().CreateCallEvent(ctx, store.CreateCallEventParams{CallID: row.ID, Kind: "hangup", FromState: row.State, ToState: row.State, Detail: "hangup requested", Payload: string(payload), EventKey: "hangup:" + now.Format(time.RFC3339Nano), ReceivedAt: now})
	return c.get(ctx, actor.WorkspaceID, callID)
}

func (c *Calls) Get(ctx context.Context, actor core.Actor, callID int64) (CallDetail, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return CallDetail{}, core.ErrForbidden
	}
	row, err := c.row(ctx, actor.WorkspaceID, callID)
	if err != nil {
		return CallDetail{}, err
	}
	evs, err := c.db.Reader().ListCallEvents(ctx, callID)
	if err != nil {
		return CallDetail{}, err
	}
	d := CallDetail{Call: callFromRow(row), Events: []CallEvent{}}
	for _, e := range evs {
		d.Events = append(d.Events, CallEvent{ID: e.ID, CallID: e.CallID, Kind: e.Kind, FromState: e.FromState, ToState: e.ToState, Detail: e.Detail, Payload: json.RawMessage(e.Payload), ReceivedAt: e.ReceivedAt.UTC()})
	}
	return d, nil
}

func (c *Calls) List(ctx context.Context, actor core.Actor, limit int) ([]Call, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := c.db.Reader().ListCalls(ctx, store.ListCallsParams{WorkspaceID: actor.WorkspaceID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Call, 0, len(rows))
	for _, r := range rows {
		out = append(out, callFromRow(r))
	}
	return out, nil
}

// ExpireRecordings deletes recordings past the policy's retention at the
// provider and marks them deleted. Transcripts and summaries are kept.
// Without a retention period nothing is deleted.
func (c *Calls) ExpireRecordings(ctx context.Context, actor core.Actor) (int, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return 0, core.ErrForbidden
	}
	cur, err := c.current(ctx, actor.WorkspaceID)
	if err != nil || cur == nil || cur.Document.Recording.RetentionDays <= 0 {
		return 0, err
	}
	del, ok := c.cfg.Telephony.(voice.Deleter)
	if !ok {
		return 0, nil
	}
	cutoff := c.clock.Now().UTC().Add(-time.Duration(cur.Document.Recording.RetentionDays) * 24 * time.Hour)
	rows, err := c.db.Reader().ListCallsWithRecordingsBefore(ctx, store.ListCallsWithRecordingsBeforeParams{WorkspaceID: actor.WorkspaceID, EndedAt: sql.NullTime{Time: cutoff, Valid: true}, Limit: 200})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if err := del.DeleteRecording(ctx, r.RecordingRef); err != nil {
			continue
		}
		now := c.clock.Now().UTC()
		if _, err := c.db.Writer().MarkRecordingDeleted(ctx, store.MarkRecordingDeletedParams{RecordingDeletedAt: sql.NullTime{Time: now, Valid: true}, UpdatedAt: now, ID: r.ID}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// RecordingAccess says whether the actor may open recordings under the
// policy. B4.5 uses it before serving audio.
func (c *Calls) RecordingAccess(ctx context.Context, actor core.Actor) (bool, string, error) {
	cur, err := c.current(ctx, actor.WorkspaceID)
	if err != nil {
		return false, "", err
	}
	if cur == nil {
		return false, "no calling policy; recordings are not served", nil
	}
	role := core.Role(cur.Document.Recording.AccessRole)
	if role == "" {
		role = core.RoleAdmin
	}
	if !actor.AtLeast(role) {
		return false, fmt.Sprintf("recordings need the %s role under policy v%d", role, cur.Version), nil
	}
	return true, "", nil
}

func (c *Calls) current(ctx context.Context, workspaceID int64) (*CallPolicy, error) {
	row, err := c.db.Reader().GetCurrentCallPolicy(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p, err := policyFromRow(row)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Calls) row(ctx context.Context, workspaceID, id int64) (store.Call, error) {
	row, err := c.db.Reader().GetCall(ctx, store.GetCallParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Call{}, core.NotFound("call %d not found", id)
	}
	return row, err
}

func (c *Calls) get(ctx context.Context, workspaceID, id int64) (Call, error) {
	row, err := c.row(ctx, workspaceID, id)
	if err != nil {
		return Call{}, err
	}
	return callFromRow(row), nil
}

func policyFromRow(r store.CallPolicy) (CallPolicy, error) {
	var doc CallPolicyDocument
	if err := json.Unmarshal([]byte(r.Document), &doc); err != nil {
		return CallPolicy{}, fmt.Errorf("call policy v%d: %w", r.Version, err)
	}
	return CallPolicy{Version: r.Version, Document: doc, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC()}, nil
}

func callFromRow(r store.Call) Call {
	return Call{ID: r.ID, PersonID: r.PersonID, EnquiryID: r.EnquiryID, OccurrenceKey: r.OccurrenceKey, Mode: r.Mode, To: r.ToNumber, From: r.FromNumber, State: r.State, Reason: r.Reason, Provider: r.Provider, ProviderCallID: r.ProviderCallID,
		PolicyVersion: r.PolicyVersion, ConsentBasis: r.ConsentBasis, Attempt: r.Attempt, Record: r.Record == 1, RecordingRef: r.RecordingRef, RecordingDeletedAt: timePtr(r.RecordingDeletedAt), DurationSeconds: idPtr(r.DurationSeconds), RequestedBy: idPtr(r.RequestedBy),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(), StartedAt: timePtr(r.StartedAt), EndedAt: timePtr(r.EndedAt), UnknownSince: timePtr(r.UnknownSince)}
}

func safe(err error) string {
	s := err.Error()
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func timePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time.UTC()
	return &t
}
