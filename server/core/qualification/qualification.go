// Package qualification runs the conversation on a placed call and turns
// what was said into a reading: transcript as evidence, structured answers,
// a summary and a confidence. The reading is applied to the enquiry once,
// under the call's occurrence key, and only when it is trusted: a low-
// confidence reading waits for a person, and a figure the model marks
// uncertain is never written as fact. Corrections re-apply with history.
package qualification

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/calls"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/core/voice"
	"github.com/WokeWorks/adx/server/store"
)

// Outcomes a call can end with. Non-scoring outcomes (no_answer,
// voicemail, wrong_number) never change the enquiry.
var Outcomes = []string{"interested", "not_interested", "callback", "transfer", "no_answer", "voicemail", "wrong_number", "unclear"}

// Questions is what the agent tries to learn. Numeric answers are the ones
// a transcript gets wrong most, so budget carries its own uncertainty flag.
var Questions = []string{"purchase_timeline", "payment", "purpose", "unit_type", "budget"}

// ReadingAnswer is one structured answer with the model's own doubt.
type ReadingAnswer struct {
	Value     string `json:"value"`
	Uncertain bool   `json:"uncertain"`
	Quote     string `json:"quote"`
}

// Reading is the structured result of a conversation.
type Reading struct {
	Summary        string                   `json:"summary"`
	Outcome        string                   `json:"outcome"`
	Answers        map[string]ReadingAnswer `json:"answers"`
	Objections     []HeardObjection         `json:"objections"`
	RequestedHuman bool                     `json:"requestedHuman"`
	CallbackAt     string                   `json:"callbackAt"`
	Confidence     string                   `json:"confidence"`
}

type HeardObjection struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type CallSession struct {
	ID                   int64                    `json:"id"`
	CallID               int64                    `json:"callId"`
	SpeechProvider       string                   `json:"speechProvider"`
	State                string                   `json:"state"`
	EndedBy              string                   `json:"endedBy"`
	Error                string                   `json:"error"`
	Interruptions        int64                    `json:"interruptions"`
	FramesDropped        int64                    `json:"framesDropped"`
	Transcript           []voice.Segment          `json:"transcript"`
	TranscriptEvidenceID *int64                   `json:"transcriptEvidenceId"`
	Summary              string                   `json:"summary"`
	Answers              map[string]ReadingAnswer `json:"answers"`
	Outcome              string                   `json:"outcome"`
	Confidence           string                   `json:"confidence"`
	RequestedHuman       bool                     `json:"requestedHuman"`
	CallbackAt           string                   `json:"callbackAt"`
	ReviewStatus         string                   `json:"reviewStatus"`
	ReviewReason         string                   `json:"reviewReason"`
	AppliedKey           string                   `json:"appliedKey"`
	AppliedVersion       int64                    `json:"appliedVersion"`
	AppliedAt            *time.Time               `json:"appliedAt"`
	Applied              AppliedDetail            `json:"applied"`
	StartedAt            time.Time                `json:"startedAt"`
	EndedAt              *time.Time               `json:"endedAt"`
}

// AppliedDetail is what an application did and did not do, and why.
// ModelOutcome is the model's own first reading, kept here so a correction
// that overwrites the session's outcome does not erase what the model said.
type AppliedDetail struct {
	ModelOutcome  string   `json:"modelOutcome"`
	SignalEventID *int64   `json:"signalEventId"`
	Stage         string   `json:"stage"`
	Attributes    []string `json:"attributes"`
	Objections    int      `json:"objections"`
	Held          []string `json:"held"`
	Note          string   `json:"note"`
}

type ReadingCorrection struct {
	ID        int64                    `json:"id"`
	SessionID int64                    `json:"sessionId"`
	Outcome   string                   `json:"outcome"`
	Answers   map[string]ReadingAnswer `json:"answers"`
	Note      string                   `json:"note"`
	CreatedBy *int64                   `json:"createdBy"`
	CreatedAt time.Time                `json:"createdAt"`
}

type CorrectionInput struct {
	Outcome string                   `json:"outcome"`
	Answers map[string]ReadingAnswer `json:"answers"`
	Note    string                   `json:"note"`
}

// CallEvidence is the call's page: the call, the session, corrections and
// whether this actor may open the recording.
type CallEvidence struct {
	Call            calls.Call          `json:"call"`
	Session         *CallSession        `json:"session"`
	Corrections     []ReadingCorrection `json:"corrections"`
	RecordingAccess bool                `json:"recordingAccess"`
	RecordingReason string              `json:"recordingReason"`
	RecordingRef    string              `json:"recordingRef"`
}

type Config struct {
	Clock    core.Clock
	Calls    *calls.Calls
	Speech   voice.Speech
	Model    assistant.Model
	Outreach *outreach.Outreach
	Signals  *signals.Signals
	CRM      *crm.CRM
	Bridge   voice.BridgeConfig
	// AmbienceGain mixes a generated room tone under the agent; 0 is off.
	AmbienceGain float64
}

type Qualification struct {
	db       *store.DB
	evidence *evidence.Store
	cfg      Config
	clock    core.Clock
	ambience []byte
}

func New(db *store.DB, ev *evidence.Store, cfg Config) *Qualification {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	q := &Qualification{db: db, evidence: ev, cfg: cfg, clock: cfg.Clock}
	if cfg.AmbienceGain > 0 {
		q.ambience = voice.AmbienceLoop(2, 42)
	}
	return q
}

// QualificationState says whether a conversation can run on this server.
type QualificationState struct {
	SpeechAdapter string   `json:"speechAdapter"`
	ModelReady    bool     `json:"modelReady"`
	Reasons       []string `json:"reasons"`
}

func (q *Qualification) State() QualificationState {
	st := QualificationState{Reasons: []string{}, ModelReady: q.cfg.Model != nil}
	if q.cfg.Speech == nil {
		st.Reasons = append(st.Reasons, "no speech adapter is wired (ADX_ELEVENLABS_API_KEY and ADX_ELEVENLABS_AGENT_ID)")
	} else {
		st.SpeechAdapter = q.cfg.Speech.Name()
	}
	if q.cfg.Model == nil {
		st.Reasons = append(st.Reasons, "no model is configured; transcripts are kept but readings wait for a person")
	}
	return st
}

// Instructions builds the agent's brief from the sales brief: the project,
// the approved facts it may quote, the questions to ask and the rules.
func (q *Qualification) Instructions(ctx context.Context, actor core.Actor, enquiryID int64) (voice.SessionConfig, outreach.SalesBrief, error) {
	if q.cfg.Outreach == nil {
		return voice.SessionConfig{}, outreach.SalesBrief{}, core.Unavailable(nil, "outreach is not wired")
	}
	b, err := q.cfg.Outreach.NextAction(ctx, actor, enquiryID)
	if err != nil {
		return voice.SessionConfig{}, outreach.SalesBrief{}, err
	}
	lang := b.Preferences.Language
	if lang == "" {
		lang = "en"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "You are calling %s on behalf of Reportage about %s. You are an assistant, not a salesperson; say so if asked.\n", firstName(b.Person.DisplayName), b.ProjectName)
	sb.WriteString("Goal: learn, politely and briefly, their purchase timeline, how they would pay, whether it is to live in or invest, the unit type they want and a budget range. Ask one question at a time. Accept 'I'd rather not say'.\n")
	sb.WriteString("You may state only these approved facts, exactly:\n")
	if len(b.Claims) == 0 {
		sb.WriteString("- (none approved; say you will have the team confirm figures)\n")
	}
	for _, f := range b.Claims {
		v := f.Value
		if f.Currency != nil && *f.Currency != "" {
			v = *f.Currency + " " + v
		}
		fmt.Fprintf(&sb, "- %s: %s\n", f.Key, v)
	}
	sb.WriteString("Never quote any other price, discount, date or percentage. If asked, say the team will confirm.\n")
	if len(b.Objections) > 0 {
		sb.WriteString("Known concerns from earlier conversations:\n")
		for _, o := range b.Objections {
			if o.ResolvedAt == nil {
				fmt.Fprintf(&sb, "- %s: %s\n", o.Kind, o.Text)
			}
		}
	}
	sb.WriteString("If they ask for a person, a call back at a time, or to stop, agree at once and end the call. End with thanks.\n")
	first := fmt.Sprintf("Hello %s, this is the Reportage assistant about %s. Is now a good moment for two quick questions?", firstName(b.Person.DisplayName), b.ProjectName)
	if strings.HasPrefix(lang, "ar") {
		first = fmt.Sprintf("مرحباً %s، معك مساعد ريبورتاج بخصوص %s. هل الوقت مناسب لسؤالين سريعين؟", firstName(b.Person.DisplayName), b.ProjectName)
	}
	return voice.SessionConfig{Codec: voice.MulawCodec, Language: lang, Instructions: sb.String(), FirstMessage: first}, b, nil
}

func firstName(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return "there"
	}
	return f[0]
}

// Run bridges a connected transport to the speech engine for the call,
// persists the transcript, reads it and applies the reading. It is what the
// media stream endpoint calls once the provider's start message checks out.
func (q *Qualification) Run(ctx context.Context, callID int64, transport voice.Transport) (CallSession, error) {
	actor := core.System(core.ViaJob)
	detail, err := q.cfg.Calls.Get(ctx, actor, callID)
	if err != nil {
		return CallSession{}, err
	}
	call := detail.Call
	switch call.State {
	case "queued", "ringing", "in_progress":
	default:
		return CallSession{}, core.Conflict("call %d is %s; no conversation runs", callID, call.State)
	}
	if q.cfg.Speech == nil {
		return CallSession{}, core.Unavailable(nil, "no speech adapter is wired")
	}
	cfg, _, err := q.Instructions(ctx, actor, call.EnquiryID)
	if err != nil {
		return CallSession{}, err
	}
	now := q.clock.Now().UTC()
	row, err := q.db.Writer().CreateCallSession(ctx, store.CreateCallSessionParams{WorkspaceID: actor.WorkspaceID, CallID: callID, SpeechProvider: q.cfg.Speech.Name(), StartedAt: now, UpdatedAt: now})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return CallSession{}, core.Conflict("call %d already has a conversation", callID)
		}
		return CallSession{}, err
	}
	t := transport
	if q.ambience != nil {
		t = voice.WithAmbience(transport, q.ambience, q.cfg.AmbienceGain)
	}
	res := voice.Bridge(ctx, t, q.cfg.Speech, cfg, q.cfg.Bridge)
	segs := res.Transcript.Segments
	if segs == nil {
		segs = []voice.Segment{}
	}
	raw, _ := json.Marshal(segs)
	var evID int64
	if q.evidence != nil && len(segs) > 0 {
		ev, _, err := q.evidence.Record(ctx, actor, evidence.Observation{Source: q.cfg.Speech.Name(), Kind: "call_transcript", SourceRef: fmt.Sprintf("call:%d", callID), Scope: fmt.Sprintf("person:%d", call.PersonID), Class: evidence.Observed, Payload: json.RawMessage(raw), Completeness: evidence.Complete})
		if err == nil {
			evID = ev.ID
		}
	}
	state, errText := "ended", ""
	if res.Err != nil {
		state, errText = "failed", res.Err.Error()
	}
	end := q.clock.Now().UTC()
	if err := q.db.Writer().FinishCallSession(ctx, store.FinishCallSessionParams{State: state, EndedBy: res.EndedBy, Error: errText, Interruptions: int64(res.Interruptions), FramesDropped: int64(res.FramesDropped), Transcript: string(raw), TranscriptEvidenceID: optionalID(evID), EndedAt: sql.NullTime{Time: end, Valid: true}, UpdatedAt: end, ID: row.ID}); err != nil {
		return CallSession{}, err
	}
	if err := q.read(ctx, actor, row.ID, call, segs); err != nil {
		return CallSession{}, err
	}
	if _, err := q.apply(ctx, actor, callID); err != nil {
		return CallSession{}, err
	}
	return q.session(ctx, actor.WorkspaceID, callID)
}

const readTool = "record_qualification"

// read asks the model for the structured reading, with a forced tool call.
// Without a model, or on a model failure, the session waits for a person.
func (q *Qualification) read(ctx context.Context, actor core.Actor, sessionID int64, call calls.Call, segs []voice.Segment) error {
	now := q.clock.Now().UTC()
	pending := func(reason string) error {
		return q.db.Writer().SetCallSessionReading(ctx, store.SetCallSessionReadingParams{Summary: "", Answers: "{}", Outcome: "", Confidence: "", RequestedHuman: 0, CallbackAt: "", ReviewStatus: "pending", ReviewReason: reason, UpdatedAt: now, ID: sessionID})
	}
	if len(segs) == 0 {
		return q.db.Writer().SetCallSessionReading(ctx, store.SetCallSessionReadingParams{Summary: "No conversation was transcribed.", Answers: "{}", Outcome: "no_answer", Confidence: "high", ReviewStatus: "auto", UpdatedAt: now, ID: sessionID})
	}
	if q.cfg.Model == nil {
		return pending("no model is configured to read the transcript")
	}
	var tb strings.Builder
	for _, s := range segs {
		fmt.Fprintf(&tb, "%s: %s\n", s.Speaker, s.Text)
	}
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"summary", "outcome", "answers", "objections", "requestedHuman", "callbackAt", "confidence"},
		"properties": map[string]any{
			"summary":        map[string]any{"type": "string"},
			"outcome":        map[string]any{"type": "string", "enum": Outcomes},
			"answers":        map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "object", "required": []string{"value", "uncertain", "quote"}, "properties": map[string]any{"value": map[string]any{"type": "string"}, "uncertain": map[string]any{"type": "boolean"}, "quote": map[string]any{"type": "string"}}}},
			"objections":     map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"kind", "text"}, "properties": map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"price", "location", "timing", "payment", "trust", "unit", "other"}}, "text": map[string]any{"type": "string"}}}},
			"requestedHuman": map[string]any{"type": "boolean"},
			"callbackAt":     map[string]any{"type": "string"},
			"confidence":     map[string]any{"type": "string", "enum": []string{"high", "low"}},
		}}
	system := "You read call transcripts for a property developer. Record only what was said. For every answer quote the words it came from. Mark an answer uncertain when the transcript could be misheard, especially numbers, prices and dates. Confidence is low when the outcome is not plainly stated by the customer. Never infer nationality or anything personal beyond the questions."
	req := assistant.Request{System: system, Messages: []assistant.Message{{Role: assistant.RoleUser, Content: []assistant.Block{{Type: "text", Text: "Transcript:\n" + tb.String() + "\nQuestions: " + strings.Join(Questions, ", ")}}}},
		Tools: []assistant.ToolSpec{{Name: readTool, Description: "Record the structured reading of the call.", InputSchema: schema}}, ToolChoice: readTool, MaxTokens: 1500}
	resp, err := q.cfg.Model.Complete(ctx, req)
	if err != nil {
		return pending("the model did not answer: " + safe(err))
	}
	var r Reading
	found := false
	for _, b := range resp.Content {
		if b.Type == "tool_use" && b.ToolName == readTool {
			if err := json.Unmarshal(b.Input, &r); err != nil {
				return pending("the model's reading was not valid")
			}
			found = true
		}
	}
	if !found {
		return pending("the model returned no reading")
	}
	if !contains(Outcomes, r.Outcome) {
		return pending(fmt.Sprintf("the model named an unknown outcome %q", r.Outcome))
	}
	if r.Confidence != "high" && r.Confidence != "low" {
		r.Confidence = "low"
	}
	if r.Answers == nil {
		r.Answers = map[string]ReadingAnswer{}
	}
	review, reason := "auto", ""
	if r.Confidence == "low" {
		review, reason = "pending", "the model was not sure of its reading; a person confirms or corrects it"
	}
	answers, _ := json.Marshal(r.Answers)
	if err := q.db.Writer().SetCallSessionReading(ctx, store.SetCallSessionReadingParams{Summary: r.Summary, Answers: string(answers), Outcome: r.Outcome, Confidence: r.Confidence, RequestedHuman: boolInt(r.RequestedHuman), CallbackAt: r.CallbackAt, ReviewStatus: review, ReviewReason: reason, UpdatedAt: now, ID: sessionID}); err != nil {
		return err
	}
	// Objections are recorded whatever the confidence: they are the
	// customer's words, and the brief shows where they came from.
	if q.cfg.Outreach != nil {
		for _, o := range r.Objections {
			_, _ = q.cfg.Outreach.RecordObjection(ctx, actor, outreach.ObjectionInput{PersonID: call.PersonID, EnquiryID: call.EnquiryID, Kind: o.Kind, Text: o.Text, Source: fmt.Sprintf("call:%d", call.ID)})
		}
	}
	return nil
}

// apply writes the reading to the enquiry once per version: the scoring
// signal under the call's occurrence key, stated attributes for answers the
// model was sure of, a stage for a clear outcome. A pending reading applies
// nothing; a non-scoring outcome scores nothing; an uncertain figure is
// held, named, and never written.
func (q *Qualification) apply(ctx context.Context, actor core.Actor, callID int64) (AppliedDetail, error) {
	row, err := q.db.Reader().GetCallSession(ctx, store.GetCallSessionParams{CallID: callID, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return AppliedDetail{}, err
	}
	d := AppliedDetail{Attributes: []string{}, Held: []string{}}
	var prev AppliedDetail
	_ = json.Unmarshal([]byte(row.AppliedDetail), &prev)
	switch {
	case prev.ModelOutcome != "":
		d.ModelOutcome = prev.ModelOutcome
	case row.ReviewStatus == "corrected":
		d.ModelOutcome = ""
	default:
		d.ModelOutcome = row.Outcome
	}
	if row.State != "ended" {
		d.Note = "the conversation did not end cleanly; nothing applied"
		return d, nil
	}
	if row.ReviewStatus == "pending" {
		d.Note = "reading pending review; nothing applied"
		return d, nil
	}
	version := row.AppliedVersion + 1
	key := fmt.Sprintf("call:%d:v%d", callID, version)
	call, err := q.cfg.Calls.Get(ctx, actor, callID)
	if err != nil {
		return d, err
	}
	var answers map[string]ReadingAnswer
	_ = json.Unmarshal([]byte(row.Answers), &answers)
	scoring := row.Outcome == "interested" || row.Outcome == "not_interested" || row.Outcome == "callback" || row.Outcome == "transfer"
	if scoring && q.cfg.Signals != nil {
		payload, _ := json.Marshal(map[string]any{"callId": callID, "outcome": row.Outcome, "version": version})
		res, err := q.cfg.Signals.Ingest(ctx, actor, signals.SignalInput{Source: "voice", Kind: "call_outcome", OccurrenceKey: key, OccurredAt: q.clock.Now().UTC(), Identity: signals.SignalIdentity{PersonID: call.Call.PersonID}, ProjectID: 0, Payload: payload})
		if err == nil {
			id := res.Event.ID
			d.SignalEventID = &id
		}
	}
	if !scoring {
		d.Note = fmt.Sprintf("outcome %s does not change the enquiry", row.Outcome)
	}
	keys := make([]string, 0, len(answers))
	for k := range answers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a := answers[k]
		if strings.TrimSpace(a.Value) == "" {
			continue
		}
		if a.Uncertain {
			d.Held = append(d.Held, fmt.Sprintf("%s=%q marked uncertain; not recorded", k, a.Value))
			continue
		}
		if q.cfg.CRM != nil {
			if _, err := q.cfg.CRM.SetStated(ctx, actor, call.Call.PersonID, k, a.Value, fmt.Sprintf("call:%d", callID)); err == nil {
				d.Attributes = append(d.Attributes, k)
			}
		}
	}
	switch row.Outcome {
	case "interested":
		d.Stage = "qualified"
	case "not_interested":
		d.Stage = "not_interested"
	case "callback":
		d.Stage = "callback"
	case "transfer":
		d.Stage = "handoff"
	}
	if d.Stage != "" {
		if err := q.db.Writer().TouchEnquiry(ctx, store.TouchEnquiryParams{LastActivityAt: q.clock.Now().UTC(), Stage: sql.NullString{String: d.Stage, Valid: true}, ID: call.Call.EnquiryID}); err != nil {
			return d, err
		}
	}
	detail, _ := json.Marshal(d)
	n, err := q.db.Writer().SetCallSessionApplied(ctx, store.SetCallSessionAppliedParams{AppliedKey: key, AppliedVersion: version, AppliedAt: sql.NullTime{Time: q.clock.Now().UTC(), Valid: true}, AppliedDetail: string(detail), UpdatedAt: q.clock.Now().UTC(), ID: row.ID, AppliedVersion_2: row.AppliedVersion})
	if err != nil {
		return d, err
	}
	if n != 1 {
		return d, core.Conflict("call %d was applied by someone else meanwhile", callID)
	}
	return d, nil
}

// Confirm accepts a pending reading as read and applies it.
func (q *Qualification) Confirm(ctx context.Context, actor core.Actor, callID int64) (CallSession, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return CallSession{}, core.ErrForbidden
	}
	row, err := q.row(ctx, actor.WorkspaceID, callID)
	if err != nil {
		return CallSession{}, err
	}
	if row.ReviewStatus != "pending" {
		return CallSession{}, core.Conflict("call %d is %s, not pending", callID, row.ReviewStatus)
	}
	if row.Outcome == "" {
		return CallSession{}, core.Conflict("call %d has no reading to confirm; correct it with an outcome instead", callID)
	}
	if err := q.db.Writer().SetCallSessionReading(ctx, store.SetCallSessionReadingParams{Summary: row.Summary, Answers: row.Answers, Outcome: row.Outcome, Confidence: "high", RequestedHuman: row.RequestedHuman, CallbackAt: row.CallbackAt, ReviewStatus: "confirmed", ReviewReason: fmt.Sprintf("confirmed by user %d", actor.UserID), UpdatedAt: q.clock.Now().UTC(), ID: row.ID}); err != nil {
		return CallSession{}, err
	}
	if _, err := q.apply(ctx, actor, callID); err != nil {
		return CallSession{}, err
	}
	return q.session(ctx, actor.WorkspaceID, callID)
}

// Correct replaces the reading with a person's, keeps the model's in the
// correction history, discards the earlier scoring event and applies again
// under a new version.
func (q *Qualification) Correct(ctx context.Context, actor core.Actor, callID int64, in CorrectionInput) (CallSession, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return CallSession{}, core.ErrForbidden
	}
	if !contains(Outcomes, in.Outcome) {
		return CallSession{}, core.Invalid("outcome must be one of %s", strings.Join(Outcomes, ", "))
	}
	in.Note = strings.TrimSpace(in.Note)
	if in.Note == "" {
		return CallSession{}, core.Invalid("say why the reading was wrong")
	}
	row, err := q.row(ctx, actor.WorkspaceID, callID)
	if err != nil {
		return CallSession{}, err
	}
	if row.State != "ended" {
		return CallSession{}, core.Conflict("call %d has no finished conversation", callID)
	}
	if in.Answers == nil {
		in.Answers = map[string]ReadingAnswer{}
	}
	answers, _ := json.Marshal(in.Answers)
	now := q.clock.Now().UTC()
	if _, err := q.db.Writer().CreateCallCorrection(ctx, store.CreateCallCorrectionParams{SessionID: row.ID, Outcome: in.Outcome, Answers: string(answers), Note: in.Note, CreatedBy: optionalID(actor.UserID), CreatedAt: now}); err != nil {
		return CallSession{}, err
	}
	// The earlier scoring event, if any, is discarded with the reason.
	var prev AppliedDetail
	_ = json.Unmarshal([]byte(row.AppliedDetail), &prev)
	if prev.SignalEventID != nil && q.cfg.Signals != nil {
		_, _ = q.cfg.Signals.Correct(ctx, actor, *prev.SignalEventID, "discard", 0, "call reading corrected: "+in.Note)
	}
	if err := q.db.Writer().SetCallSessionReading(ctx, store.SetCallSessionReadingParams{Summary: row.Summary, Answers: string(answers), Outcome: in.Outcome, Confidence: "high", RequestedHuman: row.RequestedHuman, CallbackAt: row.CallbackAt, ReviewStatus: "corrected", ReviewReason: in.Note, UpdatedAt: now, ID: row.ID}); err != nil {
		return CallSession{}, err
	}
	if _, err := q.apply(ctx, actor, callID); err != nil {
		return CallSession{}, err
	}
	return q.session(ctx, actor.WorkspaceID, callID)
}

// Evidence is the call's page.
func (q *Qualification) Evidence(ctx context.Context, actor core.Actor, callID int64) (CallEvidence, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return CallEvidence{}, core.ErrForbidden
	}
	d, err := q.cfg.Calls.Get(ctx, actor, callID)
	if err != nil {
		return CallEvidence{}, err
	}
	out := CallEvidence{Call: d.Call, Corrections: []ReadingCorrection{}}
	s, err := q.session(ctx, actor.WorkspaceID, callID)
	if err == nil {
		out.Session = &s
		cs, err := q.db.Reader().ListCallCorrections(ctx, s.ID)
		if err != nil {
			return CallEvidence{}, err
		}
		for _, c := range cs {
			cc := ReadingCorrection{ID: c.ID, SessionID: c.SessionID, Outcome: c.Outcome, Note: c.Note, CreatedBy: idPtr(c.CreatedBy), CreatedAt: c.CreatedAt.UTC(), Answers: map[string]ReadingAnswer{}}
			_ = json.Unmarshal([]byte(c.Answers), &cc.Answers)
			out.Corrections = append(out.Corrections, cc)
		}
	} else {
		var ce *core.Error
		if !errors.As(err, &ce) {
			return CallEvidence{}, err
		}
	}
	ok, why, err := q.cfg.Calls.RecordingAccess(ctx, actor)
	if err != nil {
		return CallEvidence{}, err
	}
	out.RecordingAccess = ok && d.Call.RecordingRef != "" && d.Call.RecordingDeletedAt == nil
	switch {
	case d.Call.RecordingRef == "":
		out.RecordingReason = "no recording"
	case d.Call.RecordingDeletedAt != nil:
		out.RecordingReason = "recording deleted under the retention policy"
	case !ok:
		out.RecordingReason = why
	default:
		out.RecordingRef = d.Call.RecordingRef
	}
	return out, nil
}

// Recording returns the recording reference for an actor the policy
// allows, and nothing otherwise.
func (q *Qualification) Recording(ctx context.Context, actor core.Actor, callID int64) (string, error) {
	ev, err := q.Evidence(ctx, actor, callID)
	if err != nil {
		return "", err
	}
	if !ev.RecordingAccess {
		return "", &core.Error{Kind: core.KindForbidden, Message: ev.RecordingReason}
	}
	return ev.RecordingRef, nil
}

func (q *Qualification) row(ctx context.Context, workspaceID, callID int64) (store.CallSession, error) {
	row, err := q.db.Reader().GetCallSession(ctx, store.GetCallSessionParams{CallID: callID, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.CallSession{}, core.NotFound("call %d has no conversation", callID)
	}
	return row, err
}

func (q *Qualification) session(ctx context.Context, workspaceID, callID int64) (CallSession, error) {
	row, err := q.row(ctx, workspaceID, callID)
	if err != nil {
		return CallSession{}, err
	}
	s := CallSession{ID: row.ID, CallID: row.CallID, SpeechProvider: row.SpeechProvider, State: row.State, EndedBy: row.EndedBy, Error: row.Error, Interruptions: row.Interruptions, FramesDropped: row.FramesDropped, TranscriptEvidenceID: idPtr(row.TranscriptEvidenceID),
		Summary: row.Summary, Outcome: row.Outcome, Confidence: row.Confidence, RequestedHuman: row.RequestedHuman == 1, CallbackAt: row.CallbackAt, ReviewStatus: row.ReviewStatus, ReviewReason: row.ReviewReason, AppliedKey: row.AppliedKey, AppliedVersion: row.AppliedVersion, StartedAt: row.StartedAt.UTC(), Transcript: []voice.Segment{}, Answers: map[string]ReadingAnswer{}}
	_ = json.Unmarshal([]byte(row.Transcript), &s.Transcript)
	_ = json.Unmarshal([]byte(row.Answers), &s.Answers)
	_ = json.Unmarshal([]byte(row.AppliedDetail), &s.Applied)
	if s.Applied.Attributes == nil {
		s.Applied.Attributes = []string{}
	}
	if s.Applied.Held == nil {
		s.Applied.Held = []string{}
	}
	if row.AppliedAt.Valid {
		t := row.AppliedAt.Time.UTC()
		s.AppliedAt = &t
	}
	if row.EndedAt.Valid {
		t := row.EndedAt.Time.UTC()
		s.EndedAt = &t
	}
	return s, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func safe(err error) string {
	var me *assistant.ModelError
	if errors.As(err, &me) {
		return me.Safe
	}
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
