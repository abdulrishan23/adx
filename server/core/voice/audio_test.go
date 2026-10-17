package voice_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/voice"
	"github.com/WokeWorks/adx/server/core/voice/voicetest"
)

// Catches: a codec that turns speech into static, frames longer than one
// barge-in can cut, and ambience leaking into the caller's leg.
func TestMulawRoundTripFramesAndAmbience(t *testing.T) {
	for _, s := range []int16{0, 1, -1, 100, -100, 1000, -1000, 8000, -8000, 30000, -30000, 32767, -32768} {
		back := voice.DecodeMulaw(voice.EncodeMulaw(s))
		tol := int32(1)
		if s != 0 {
			tol = int32(abs(int32(s))/16 + 8)
		}
		assert.LessOrEqual(t, abs(int32(back)-int32(s)), tol, "sample %d came back as %d", s, back)
	}
	frames := voice.SplitFrames(make([]byte, 500), voice.FrameBytes)
	require.Len(t, frames, 4)
	assert.Len(t, frames[0], 160)
	assert.Len(t, frames[3], 20)
	assert.Empty(t, voice.SplitFrames(nil, 160))

	loop := voice.AmbienceLoop(1, 7)
	assert.Len(t, loop, 8000)
	assert.Equal(t, loop, voice.AmbienceLoop(1, 7), "deterministic")
	// Silence mixed with the floor is the floor, not silence; and the floor is quiet.
	inner := voicetest.NewTransport(voice.MulawCodec)
	amb := voice.WithAmbience(inner, loop, 0.5)
	silence := make([]byte, 320)
	for i := range silence {
		silence[i] = voice.EncodeMulaw(0)
	}
	require.NoError(t, amb.Send(t.Context(), voice.Frame{Codec: voice.MulawCodec, Data: silence}))
	sent := inner.SentFrames()
	require.Len(t, sent, 2, "320 bytes is two 20 ms frames")
	loud := 0
	for _, f := range sent {
		for _, b := range f.Data {
			v := voice.DecodeMulaw(b)
			if v != 0 {
				loud++
			}
			assert.Less(t, abs(int32(v)), int32(1200), "ambience stays quiet")
		}
	}
	assert.Greater(t, loud, 100, "the floor is audible, not digital silence")
	// Gain 0 leaves audio untouched and still frames it.
	off := voice.WithAmbience(voicetest.NewTransport(voice.MulawCodec), loop, 0)
	require.NoError(t, off.Send(t.Context(), voice.Frame{Codec: voice.MulawCodec, Data: silence}))
	assert.Equal(t, silence[:160], off.Transport.(*voicetest.Transport).SentFrames()[0].Data)
}

// Catches: a barge-in that clears the transport but leaves the mixer and
// queued frames playing stale speech.
func TestBridgeWithAmbienceClearsOnInterruption(t *testing.T) {
	inner := voicetest.NewTransport(voice.MulawCodec)
	inner.EchoMarks = true
	amb := voice.WithAmbience(inner, voice.AmbienceLoop(1, 1), 0.3)
	speech := &voicetest.Speech{Codec: voice.MulawCodec}
	done := make(chan voice.BridgeResult, 1)
	go func() {
		done <- voice.Bridge(t.Context(), amb, speech, voice.SessionConfig{Codec: voice.MulawCodec, Language: "en", Instructions: "x"}, voice.BridgeConfig{HangupGrace: 200 * time.Millisecond})
	}()
	var sess *voicetest.Session
	require.Eventually(t, func() bool { sess = speech.Session(0); return sess != nil }, 2*time.Second, 10*time.Millisecond)
	sess.Emit(voice.SpeechEvent{Kind: voice.KindAudio, Audio: voice.Frame{Codec: voice.MulawCodec, Data: make([]byte, 480)}})
	require.Eventually(t, func() bool { return len(inner.SentFrames()) >= 3 }, 2*time.Second, 10*time.Millisecond)
	sess.Emit(voice.SpeechEvent{Kind: voice.KindInterruption})
	require.Eventually(t, func() bool { return inner.ClearedCount() == 1 }, 2*time.Second, 10*time.Millisecond)
	sess.Emit(voice.SpeechEvent{Kind: voice.KindTranscript, Text: "hello", Speaker: "agent"})
	sess.Emit(voice.SpeechEvent{Kind: voice.KindEnd})
	res := <-done
	assert.Equal(t, "agent", res.EndedBy)
	assert.Equal(t, 1, res.Interruptions)
	assert.Contains(t, inner.MarkedNames(), "end-of-call")
}

func abs(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}
