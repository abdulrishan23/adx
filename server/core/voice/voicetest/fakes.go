// Package voicetest holds fakes for core/voice: a telephony provider, a
// media transport and a speech engine that need no credentials, so the whole
// bridge runs in tests and in an isolated demo.
package voicetest

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/WokeWorks/adx/server/core/voice"
)

// Telephony records calls instead of placing them.
type Telephony struct {
	mu     sync.Mutex
	Calls  []voice.CallRequest
	Hungup []string
	// Secret is what ParseStatusCallback expects in the X-Fake-Signature
	// header, standing in for a provider's HMAC.
	Secret string
	// StartErr makes StartCall fail after recording the request, standing in
	// for a transport failure whose outcome is unknown.
	StartErr error
	// Known is what LookupCall answers, by provider call id.
	Known   map[string]voice.StatusEvent
	Deleted []string
}

func (t *Telephony) Name() string { return "fake" }

func (t *Telephony) StartCall(_ context.Context, req voice.CallRequest) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Calls = append(t.Calls, req)
	if t.StartErr != nil {
		return "", t.StartErr
	}
	return "fake-call-" + req.OccurrenceKey, nil
}

// LookupCall answers from Known; a call the fake was never asked about is
// not found.
func (t *Telephony) LookupCall(_ context.Context, id string) (voice.StatusEvent, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	ev, ok := t.Known[id]
	return ev, ok, nil
}

func (t *Telephony) DeleteRecording(_ context.Context, ref string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Deleted = append(t.Deleted, ref)
	return nil
}

func (t *Telephony) Hangup(_ context.Context, id string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Hungup = append(t.Hungup, id)
	return nil
}

func (t *Telephony) ParseStatusCallback(r *http.Request) (voice.StatusEvent, error) {
	if r.Header.Get("X-Fake-Signature") != t.Secret || t.Secret == "" {
		return voice.StatusEvent{}, voice.ErrBadSignature
	}
	var d time.Duration
	if secs := r.FormValue("CallDuration"); secs != "" {
		if n, err := strconv.Atoi(secs); err == nil {
			d = time.Duration(n) * time.Second
		}
	}
	return voice.StatusEvent{ProviderCallID: r.FormValue("CallSid"), State: voice.CallState(r.FormValue("CallStatus")), Duration: d, RecordingRef: r.FormValue("RecordingSid"), At: time.Now(), EventID: r.FormValue("SequenceNumber")}, nil
}

// Transport is an in-memory media stream. The test feeds caller audio with
// Hear and reads what the agent sent from Sent.
type Transport struct {
	Codec    string
	incoming chan voice.Frame
	marks    chan string
	mu       sync.Mutex
	Sent     []voice.Frame
	Cleared  int
	Marked   []string
	closed   bool
	// EchoMarks makes Mark echo the name back, as a real provider does once
	// the audio before it has played.
	EchoMarks bool
}

func NewTransport(codec string) *Transport {
	return &Transport{Codec: codec, incoming: make(chan voice.Frame, 64), marks: make(chan string, 8), EchoMarks: true}
}

// Hear is the caller speaking.
func (t *Transport) Hear(data []byte) { t.incoming <- voice.Frame{Codec: t.Codec, Data: data} }

// HangUp is the caller ending the call.
func (t *Transport) HangUp() { close(t.incoming) }

func (t *Transport) Incoming() <-chan voice.Frame { return t.incoming }

func (t *Transport) Send(_ context.Context, f voice.Frame) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return errors.New("transport closed")
	}
	t.Sent = append(t.Sent, f)
	return nil
}

func (t *Transport) Clear(context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Cleared++
	return nil
}

func (t *Transport) Mark(_ context.Context, name string) error {
	t.mu.Lock()
	t.Marked = append(t.Marked, name)
	echo := t.EchoMarks
	t.mu.Unlock()
	if echo {
		t.marks <- name
	}
	return nil
}

func (t *Transport) Marks() <-chan string { return t.marks }

// SentFrames, ClearedCount and MarkedNames read the fake's state safely from
// the test goroutine.
func (t *Transport) SentFrames() []voice.Frame {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]voice.Frame(nil), t.Sent...)
}

func (t *Transport) ClearedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Cleared
}

func (t *Transport) MarkedNames() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.Marked...)
}

func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

// Speech is a scripted engine. Script is what happens, in order, when the
// session starts; the session then waits for the test to drive more events
// through Emit, and End returns the transcript.
type Speech struct {
	Codec string
	// StartErr makes Start fail, for the start-timeout and error paths.
	StartErr error
	mu       sync.Mutex
	Sessions []*Session
}

func (s *Speech) Name() string { return "fake" }

// Session returns the i-th session started, or nil.
func (s *Speech) Session(i int) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i >= len(s.Sessions) {
		return nil
	}
	return s.Sessions[i]
}

func (s *Speech) Start(_ context.Context, cfg voice.SessionConfig) (voice.Session, error) {
	if s.StartErr != nil {
		return nil, s.StartErr
	}
	if cfg.Codec != s.Codec {
		return nil, errors.New("fake speech only speaks " + s.Codec)
	}
	sess := &Session{events: make(chan voice.SpeechEvent, 64), Config: cfg}
	s.mu.Lock()
	s.Sessions = append(s.Sessions, sess)
	s.mu.Unlock()
	return sess, nil
}

type Session struct {
	Config   voice.SessionConfig
	events   chan voice.SpeechEvent
	mu       sync.Mutex
	Heard    []voice.Frame
	ended    bool
	segments []voice.Segment
}

func (s *Session) SendAudio(_ context.Context, f voice.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return errors.New("session ended")
	}
	s.Heard = append(s.Heard, f)
	return nil
}

func (s *Session) Events() <-chan voice.SpeechEvent { return s.events }

func (s *Session) HeardCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Heard)
}

// Emit is the engine doing something: speaking, noticing an interruption,
// producing a transcript line, or ending.
func (s *Session) Emit(ev voice.SpeechEvent) {
	if ev.Kind == voice.KindTranscript {
		s.mu.Lock()
		s.segments = append(s.segments, voice.Segment{Speaker: ev.Speaker, Text: ev.Text})
		s.mu.Unlock()
	}
	s.events <- ev
}

// Say emits n audio frames then the transcript line for them.
func (s *Session) Say(text string, frames int) {
	for i := 0; i < frames; i++ {
		s.Emit(voice.SpeechEvent{Kind: voice.KindAudio, Audio: voice.Frame{Codec: s.Config.Codec, Data: []byte{byte(i)}}})
	}
	s.Emit(voice.SpeechEvent{Kind: voice.KindTranscript, Speaker: "agent", Text: text})
}

func (s *Session) End(context.Context) (voice.Transcript, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended = true
	return voice.Transcript{Segments: s.segments}, nil
}
