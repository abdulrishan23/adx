package voice_test

import (
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/voice"
	"github.com/WokeWorks/adx/server/core/voice/voicetest"
)

const codec = "mulaw_8000"

// Catches: a barge-in that lets queued speech keep playing, an end that cuts
// the goodbye off instead of marking and waiting, and any need for
// credentials to run the bridge locally.
func TestBridgeStartsInterruptsAndEnds(t *testing.T) {
	transport := voicetest.NewTransport(codec)
	speech := &voicetest.Speech{Codec: codec}
	done := make(chan voice.BridgeResult, 1)
	go func() {
		done <- voice.Bridge(t.Context(), transport, speech, voice.SessionConfig{Codec: codec, Language: "en", FirstMessage: "Hello"},
			voice.BridgeConfig{HangupGrace: time.Second})
	}()

	var session *voicetest.Session
	require.Eventually(t, func() bool {
		session = speech.Session(0)
		return session != nil
	}, time.Second, 10*time.Millisecond)

	// Agent greets; the frames reach the caller in order.
	session.Say("Hello, this is ADX for Reportage.", 3)
	require.Eventually(t, func() bool { return len(transport.SentFrames()) == 3 }, time.Second, 10*time.Millisecond)
	assert.Equal(t, []byte{0}, transport.SentFrames()[0].Data)

	// Caller talks; it reaches the engine.
	transport.Hear([]byte("caller audio"))
	require.Eventually(t, func() bool { return session.HeardCount() == 1 }, time.Second, 10*time.Millisecond)

	// Barge-in: the far side is told to clear.
	session.Emit(voice.SpeechEvent{Kind: voice.KindInterruption})
	require.Eventually(t, func() bool { return transport.ClearedCount() == 1 }, time.Second, 10*time.Millisecond)

	// Agent says goodbye and ends; the mark is sent and echoed before hangup.
	session.Say("Goodbye.", 2)
	session.Emit(voice.SpeechEvent{Kind: voice.KindEnd})
	select {
	case result := <-done:
		assert.NoError(t, result.Err)
		assert.Equal(t, "agent", result.EndedBy)
		assert.Equal(t, 1, result.Interruptions)
		assert.Equal(t, []string{"end-of-call"}, transport.MarkedNames())
		assert.Len(t, transport.SentFrames(), 5)
		require.Len(t, result.Transcript.Segments, 2)
		assert.Equal(t, "agent", result.Transcript.Segments[0].Speaker)
		assert.Equal(t, "Goodbye.", result.Transcript.Segments[1].Text)
	case <-time.After(3 * time.Second):
		t.Fatal("bridge did not end")
	}
}

// Catches: a caller hangup leaving the session open, a codec mismatch being
// bridged as noise, and a dead engine hanging the call instead of failing.
func TestBridgeHandlesCallerHangupCodecMismatchAndEngineFailure(t *testing.T) {
	transport := voicetest.NewTransport(codec)
	speech := &voicetest.Speech{Codec: codec}
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.HangUp()
	}()
	result := voice.Bridge(t.Context(), transport, speech, voice.SessionConfig{Codec: codec}, voice.BridgeConfig{})
	assert.Equal(t, "caller", result.EndedBy)
	assert.NoError(t, result.Err)

	mismatch := voicetest.NewTransport("pcm_16000")
	speech = &voicetest.Speech{Codec: codec}
	go func() { mismatch.Hear([]byte("x")) }()
	result = voice.Bridge(t.Context(), mismatch, speech, voice.SessionConfig{Codec: codec}, voice.BridgeConfig{})
	assert.Equal(t, "error", result.EndedBy)
	assert.ErrorContains(t, result.Err, "codec")

	broken := &voicetest.Speech{Codec: codec, StartErr: errors.New("signed-url request failed")}
	result = voice.Bridge(t.Context(), voicetest.NewTransport(codec), broken, voice.SessionConfig{Codec: codec}, voice.BridgeConfig{})
	assert.Equal(t, "error", result.EndedBy)
	assert.Error(t, result.Err)

	// Max duration: the engine never ends, the bridge does.
	slow := voicetest.NewTransport(codec)
	result = voice.Bridge(t.Context(), slow, &voicetest.Speech{Codec: codec}, voice.SessionConfig{Codec: codec}, voice.BridgeConfig{MaxDuration: 100 * time.Millisecond})
	assert.Equal(t, "timeout", result.EndedBy)
}

// Catches: a callback without the provider's signature being accepted.
func TestFakeTelephonyRejectsUnsignedCallbacks(t *testing.T) {
	tel := &voicetest.Telephony{Secret: "s3"}
	form := url.Values{"CallSid": {"fake-call-1"}, "CallStatus": {"completed"}}
	req := httptest.NewRequest("POST", "/api/voice/callbacks/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err := tel.ParseStatusCallback(req)
	assert.ErrorIs(t, err, voice.ErrBadSignature)

	req = httptest.NewRequest("POST", "/api/voice/callbacks/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Fake-Signature", "s3")
	ev, err := tel.ParseStatusCallback(req)
	require.NoError(t, err)
	assert.Equal(t, voice.CallCompleted, ev.State)
	assert.True(t, ev.State.Terminal())
	assert.False(t, voice.CallRinging.Terminal())

	id, err := tel.StartCall(t.Context(), voice.CallRequest{OccurrenceKey: "enq-1/attempt-1", To: "+9715...", From: "+9718..."})
	require.NoError(t, err)
	assert.Equal(t, "fake-call-enq-1/attempt-1", id)
	assert.Len(t, tel.Calls, 1)
}
