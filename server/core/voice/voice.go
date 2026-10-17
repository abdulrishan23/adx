// Package voice owns calls between ADX and a person: the call's lifecycle,
// the audio bridge between the phone network and the speech engine, and the
// transcript that comes out. This file fixes the boundary phase 0 needs; B4.4
// and B4.5 add the tables, policy checks and the real adapters.
//
// Three outside things are kept behind separate interfaces, because they are
// bought and verified separately: Telephony (who places the call and carries
// the audio, Twilio in the reference source; the UAE route is not chosen),
// Transport (one call's media stream, frames in and out), and Speech (the
// conversational engine, ElevenLabs in the reference source). Core never
// imports an adapter, and the fakes in voicetest run the whole bridge with no
// credentials at all.
//
// The lead-facing caller is not the operator voice interface (B4.6); that
// one reuses Speech but never Telephony.
package voice

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// CallState follows the reference source's state machine. Terminal states
// never move again.
type CallState string

const (
	CallQueued     CallState = "queued"
	CallRinging    CallState = "ringing"
	CallInProgress CallState = "in_progress"
	CallCompleted  CallState = "completed"
	CallNoAnswer   CallState = "no_answer"
	CallBusy       CallState = "busy"
	CallFailed     CallState = "failed"
	CallCanceled   CallState = "canceled"
)

func (s CallState) Terminal() bool {
	switch s {
	case CallCompleted, CallNoAnswer, CallBusy, CallFailed, CallCanceled:
		return true
	}
	return false
}

// CallRequest is what Telephony needs to place a call. Policy (consent,
// hours, caps) is checked by core before this is ever built.
type CallRequest struct {
	// OccurrenceKey is the call's stable key, so a retried dispatch after an
	// unknown result can be reconciled instead of placing a second call.
	OccurrenceKey string
	To            string
	From          string
	// StreamURL is where the provider connects the media stream, and
	// StatusCallbackURL where it posts state changes. Both are ADX's.
	StreamURL         string
	StatusCallbackURL string
	Record            bool
}

// StatusEvent is a provider callback, parsed and signature-checked by the
// adapter. ProviderCallID ties it to a placed call.
type StatusEvent struct {
	ProviderCallID string
	State          CallState
	Duration       time.Duration
	RecordingRef   string
	At             time.Time
	// EventID is the provider's id or sequence for this callback, so a
	// redelivery is the same event. Empty means the adapter has none.
	EventID string
}

// Reconciler is the optional read side of a telephony provider: what it
// knows about a call it was asked to place. The call lifecycle uses it to
// settle an unknown result before placing another attempt.
type Reconciler interface {
	LookupCall(ctx context.Context, providerCallID string) (StatusEvent, bool, error)
}

// Deleter removes a recording at the provider, for retention.
type Deleter interface {
	DeleteRecording(ctx context.Context, recordingRef string) error
}

// ErrBadSignature means a callback did not come from the provider.
var ErrBadSignature = errors.New("callback signature is invalid")

// Telephony places and ends calls and parses the provider's callbacks. An
// implementation is injected by cmd/adx only when a route is configured and
// authorized; otherwise live calling is disabled and only fakes run.
type Telephony interface {
	Name() string
	StartCall(ctx context.Context, req CallRequest) (providerCallID string, err error)
	Hangup(ctx context.Context, providerCallID string) error
	// ParseStatusCallback validates the request's signature against the
	// provider's secret and decodes it. Invalid requests return ErrBadSignature.
	ParseStatusCallback(r *http.Request) (StatusEvent, error)
}

// Frame is one slice of audio. The reference source runs 8 kHz mono µ-law
// end to end; the codec is named so a mismatch is an error, not static.
type Frame struct {
	Codec string // "mulaw_8000"
	Data  []byte
}

// Transport is one call's media stream: the caller's audio in, the agent's
// audio out, plus the two control messages the bridge needs. Clear drops
// audio the far side has queued but not played, which is how a barge-in
// stops the agent mid-sentence. Mark asks the far side to echo a name once
// everything before it has played, which is how a goodbye is allowed to
// finish before hangup.
type Transport interface {
	Incoming() <-chan Frame
	Send(ctx context.Context, f Frame) error
	Clear(ctx context.Context) error
	Mark(ctx context.Context, name string) error
	// Marks delivers the names the far side echoed back.
	Marks() <-chan string
	Close() error
}

// SpeechEvent is something the speech engine says or notices.
type SpeechEvent struct {
	Kind SpeechEventKind
	// Audio for KindAudio.
	Audio Frame
	// Text and Speaker for KindTranscript.
	Text    string
	Speaker string // "lead" or "agent"
}

type SpeechEventKind string

const (
	KindAudio        SpeechEventKind = "audio"
	KindTranscript   SpeechEventKind = "transcript"
	KindInterruption SpeechEventKind = "interruption"
	KindEnd          SpeechEventKind = "end"
)

// SessionConfig is what a conversation starts with. The prompt and facts
// come from core (approved brand facts, the enquiry); nothing in here is
// education-specific.
type SessionConfig struct {
	Codec        string
	Language     string
	Instructions string
	FirstMessage string
}

// Session is one live conversation with the speech engine.
type Session interface {
	SendAudio(ctx context.Context, f Frame) error
	Events() <-chan SpeechEvent
	// End closes the session and returns the transcript so far.
	End(ctx context.Context) (Transcript, error)
}

// Speech starts sessions. ElevenLabs is the reference implementation (B4.5);
// the fake in voicetest is the only one phase 0 has.
type Speech interface {
	Name() string
	Start(ctx context.Context, cfg SessionConfig) (Session, error)
}

// Segment is one line of transcript.
type Segment struct {
	Speaker string        `json:"speaker"`
	Text    string        `json:"text"`
	Start   time.Duration `json:"startMs"`
	End     time.Duration `json:"endMs"`
}

type Transcript struct {
	Segments []Segment     `json:"segments"`
	Duration time.Duration `json:"duration"`
}
