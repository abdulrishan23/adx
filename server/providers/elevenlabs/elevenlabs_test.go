package elevenlabs_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/voice"
	"github.com/WokeWorks/adx/server/providers/elevenlabs"
)

// Catches: the API key or agent id not sent, the prompt override missing,
// pings left unanswered, audio decoded wrongly, transcripts attributed to
// the wrong speaker, and an agent configured for the wrong audio format
// being accepted.
func TestSessionProtocol(t *testing.T) {
	received := make(chan map[string]any, 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "key", r.Header.Get("xi-api-key"))
		agent := r.URL.Query().Get("agent_id")
		conn, err := websocket.Accept(w, r, nil)
		require.NoError(t, err)
		ctx := r.Context()
		send := func(v any) {
			b, _ := json.Marshal(v)
			_ = conn.Write(ctx, websocket.MessageText, b)
		}
		_, data, err := conn.Read(ctx)
		require.NoError(t, err)
		var init map[string]any
		require.NoError(t, json.Unmarshal(data, &init))
		received <- init
		if agent == "wrong-format" {
			send(map[string]any{"type": "conversation_initiation_metadata", "conversation_initiation_metadata_event": map[string]any{"conversation_id": "c1", "agent_output_audio_format": "pcm_16000"}})
			return
		}
		send(map[string]any{"type": "ping", "ping_event": map[string]any{"event_id": 1}})
		send(map[string]any{"type": "conversation_initiation_metadata", "conversation_initiation_metadata_event": map[string]any{"conversation_id": "c1", "agent_output_audio_format": "ulaw_8000", "user_input_audio_format": "ulaw_8000"}})
		send(map[string]any{"type": "audio", "audio_event": map[string]any{"audio_base_64": base64.StdEncoding.EncodeToString([]byte{5, 6, 7})}})
		send(map[string]any{"type": "agent_response", "agent_response_event": map[string]any{"agent_response": "Hello Amal"}})
		send(map[string]any{"type": "ping", "ping_event": map[string]any{"event_id": 2}})
		send(map[string]any{"type": "user_transcript", "user_transcription_event": map[string]any{"user_transcript": "hi"}})
		send(map[string]any{"type": "interruption", "interruption_event": map[string]any{"event_id": 3}})
		// Read what the client sent: pongs and audio.
		for i := 0; i < 3; i++ {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var m map[string]any
			_ = json.Unmarshal(data, &m)
			received <- m
		}
		send(map[string]any{"type": "close"})
		<-ctx.Done()
	}))
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	bad, err := elevenlabs.New(elevenlabs.Config{APIKey: "key", AgentID: "wrong-format", URL: wsURL, HTTPClient: server.Client()})
	require.NoError(t, err)
	_, err = bad.Start(t.Context(), voice.SessionConfig{Codec: voice.MulawCodec, Instructions: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ulaw_8000")
	<-received

	sp, err := elevenlabs.New(elevenlabs.Config{APIKey: "key", AgentID: "agent-1", URL: wsURL, HTTPClient: server.Client()})
	require.NoError(t, err)
	_, err = sp.Start(t.Context(), voice.SessionConfig{Codec: "pcm_16000"})
	require.Error(t, err, "only µ-law")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	sess, err := sp.Start(ctx, voice.SessionConfig{Codec: voice.MulawCodec, Language: "en", Instructions: "Be brief.", FirstMessage: "Hello"})
	require.NoError(t, err)
	init := <-received
	assert.Equal(t, "conversation_initiation_client_data", init["type"])
	agent := init["conversation_config_override"].(map[string]any)["agent"].(map[string]any)
	assert.Equal(t, "Be brief.", agent["prompt"].(map[string]any)["prompt"])
	assert.Equal(t, "Hello", agent["first_message"])
	require.NoError(t, sess.SendAudio(ctx, voice.Frame{Codec: voice.MulawCodec, Data: []byte{1, 2}}))

	var kinds []voice.SpeechEventKind
	var audio []byte
	for ev := range sess.Events() {
		kinds = append(kinds, ev.Kind)
		if ev.Kind == voice.KindAudio {
			audio = ev.Audio.Data
		}
		if ev.Kind == voice.KindEnd {
			break
		}
	}
	assert.Equal(t, []voice.SpeechEventKind{voice.KindAudio, voice.KindTranscript, voice.KindTranscript, voice.KindInterruption, voice.KindEnd}, kinds)
	assert.Equal(t, []byte{5, 6, 7}, audio)
	tr, err := sess.End(ctx)
	require.NoError(t, err)
	require.Len(t, tr.Segments, 2)
	assert.Equal(t, "agent", tr.Segments[0].Speaker)
	assert.Equal(t, "Hello Amal", tr.Segments[0].Text)
	assert.Equal(t, "lead", tr.Segments[1].Speaker)

	// The client answered both pings and sent the audio chunk.
	var pongs, chunks int
	for i := 0; i < 3; i++ {
		m := <-received
		if m["type"] == "pong" {
			pongs++
		}
		if m["user_audio_chunk"] != nil {
			chunks++
			assert.Equal(t, base64.StdEncoding.EncodeToString([]byte{1, 2}), m["user_audio_chunk"])
		}
	}
	assert.Equal(t, 2, pongs)
	assert.Equal(t, 1, chunks)
}
