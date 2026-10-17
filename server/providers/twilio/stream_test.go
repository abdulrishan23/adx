package twilio_test

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
	"github.com/WokeWorks/adx/server/providers/twilio"
)

// Catches: audio accepted for a call ADX did not place, media frames
// decoded wrongly, and outbound clear/mark/media in the wrong shape.
func TestStreamValidatesStartAndCarriesMedia(t *testing.T) {
	type result struct {
		tr  *twilio.StreamTransport
		err error
	}
	results := make(chan result, 1)
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		require.NoError(t, err)
		tr, err := twilio.AcceptStream(r.Context(), conn, twilio.StreamExpectation{CallSid: "CA1", Occurrence: "enq:1"}, 2*time.Second)
		results <- result{tr, err}
		if err != nil {
			conn.Close(websocket.StatusPolicyViolation, err.Error())
			return
		}
		// Play something back, clear, mark, then hold the connection until the
		// test is done reading from the transport.
		_ = tr.Send(r.Context(), voice.Frame{Codec: voice.MulawCodec, Data: []byte{1, 2, 3}})
		_ = tr.Clear(r.Context())
		_ = tr.Mark(r.Context(), "end-of-call")
		<-done
		_ = tr.Close()
	}))
	defer close(done)
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http")

	// Wrong call: refused before any media.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	require.NoError(t, err)
	send := func(c *websocket.Conn, v any) {
		b, _ := json.Marshal(v)
		require.NoError(t, c.Write(ctx, websocket.MessageText, b))
	}
	send(c, map[string]any{"event": "connected"})
	send(c, map[string]any{"event": "start", "start": map[string]any{"streamSid": "MZ1", "callSid": "CAother", "customParameters": map[string]string{"occurrence": "enq:1"}}})
	res := <-results
	require.Error(t, res.err)
	assert.Contains(t, res.err.Error(), "not \"CA1\"")
	_ = c.CloseNow()

	// Right call: media flows both ways.
	c, _, err = websocket.Dial(ctx, url, nil)
	require.NoError(t, err)
	send(c, map[string]any{"event": "connected"})
	send(c, map[string]any{"event": "start", "start": map[string]any{"streamSid": "MZ2", "callSid": "CA1", "customParameters": map[string]string{"occurrence": "enq:1"}, "mediaFormat": map[string]any{"encoding": "audio/x-mulaw", "sampleRate": 8000, "channels": 1}}})
	res = <-results
	require.NoError(t, res.err)
	send(c, map[string]any{"event": "media", "streamSid": "MZ2", "media": map[string]string{"payload": base64.StdEncoding.EncodeToString([]byte{9, 8, 7, 6})}})
	frame := <-res.tr.Incoming()
	assert.Equal(t, voice.MulawCodec, frame.Codec)
	assert.Equal(t, []byte{9, 8, 7, 6}, frame.Data)
	var got []map[string]any
	for len(got) < 3 {
		_, data, err := c.Read(ctx)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))
		got = append(got, m)
	}
	assert.Equal(t, "media", got[0]["event"])
	assert.Equal(t, "MZ2", got[0]["streamSid"])
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte{1, 2, 3}), got[0]["media"].(map[string]any)["payload"])
	assert.Equal(t, "clear", got[1]["event"])
	assert.Equal(t, "mark", got[2]["event"])
	assert.Equal(t, "end-of-call", got[2]["mark"].(map[string]any)["name"])
	send(c, map[string]any{"event": "mark", "streamSid": "MZ2", "mark": map[string]string{"name": "end-of-call"}})
	assert.Equal(t, "end-of-call", <-res.tr.Marks())
	send(c, map[string]any{"event": "stop", "streamSid": "MZ2"})
	_, ok := <-res.tr.Incoming()
	assert.False(t, ok, "stop closes the incoming channel")
	_ = c.CloseNow()

	// A non-µ-law stream is refused.
	c, _, err = websocket.Dial(ctx, url, nil)
	require.NoError(t, err)
	send(c, map[string]any{"event": "start", "start": map[string]any{"streamSid": "MZ3", "callSid": "CA1", "customParameters": map[string]string{"occurrence": "enq:1"}, "mediaFormat": map[string]any{"encoding": "audio/l16", "sampleRate": 16000}}})
	res = <-results
	require.Error(t, res.err)
	_ = c.CloseNow()
}
