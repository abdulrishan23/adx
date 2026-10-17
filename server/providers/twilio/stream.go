package twilio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/WokeWorks/adx/server/core/voice"
)

// streamMessage is Twilio's Media Streams envelope, inbound and outbound.
type streamMessage struct {
	Event     string `json:"event"`
	StreamSid string `json:"streamSid,omitempty"`
	Start     *struct {
		StreamSid        string            `json:"streamSid"`
		CallSid          string            `json:"callSid"`
		CustomParameters map[string]string `json:"customParameters"`
		MediaFormat      struct {
			Encoding   string `json:"encoding"`
			SampleRate int    `json:"sampleRate"`
			Channels   int    `json:"channels"`
		} `json:"mediaFormat"`
	} `json:"start,omitempty"`
	Media *struct {
		Payload string `json:"payload"`
	} `json:"media,omitempty"`
	Mark *struct {
		Name string `json:"name"`
	} `json:"mark,omitempty"`
}

// StreamExpectation is what the start message must say before any audio
// is accepted: the call SID ADX placed and the occurrence key it passed as
// a stream parameter. A stream for another call is closed.
type StreamExpectation struct {
	CallSid    string
	Occurrence string
}

// StreamTransport is one Media Streams connection as a voice.Transport.
type StreamTransport struct {
	conn      *websocket.Conn
	streamSid string
	incoming  chan voice.Frame
	marks     chan string
	closeOnce sync.Once
	err       error
	mu        sync.Mutex
}

// AcceptStream reads the connection up to and including Twilio's start
// message, checks it against the expectation, and returns a transport that
// decodes media into frames. It fails closed on a mismatch or on a format
// that is not 8 kHz µ-law.
func AcceptStream(ctx context.Context, conn *websocket.Conn, expect StreamExpectation, startTimeout time.Duration) (*StreamTransport, error) {
	if startTimeout == 0 {
		startTimeout = 10 * time.Second
	}
	startCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	conn.SetReadLimit(1 << 20)
	for {
		_, data, err := conn.Read(startCtx)
		if err != nil {
			return nil, fmt.Errorf("media stream ended before start: %w", err)
		}
		var m streamMessage
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		switch m.Event {
		case "connected":
			continue
		case "start":
			if m.Start == nil {
				return nil, errors.New("start message without a start body")
			}
			if expect.CallSid != "" && m.Start.CallSid != expect.CallSid {
				return nil, fmt.Errorf("stream is for call %q, not %q", m.Start.CallSid, expect.CallSid)
			}
			if expect.Occurrence != "" && m.Start.CustomParameters["occurrence"] != expect.Occurrence {
				return nil, errors.New("stream occurrence parameter does not match the call")
			}
			if enc := m.Start.MediaFormat.Encoding; enc != "" && enc != "audio/x-mulaw" {
				return nil, fmt.Errorf("stream encoding %q is not µ-law", enc)
			}
			if rate := m.Start.MediaFormat.SampleRate; rate != 0 && rate != 8000 {
				return nil, fmt.Errorf("stream sample rate %d is not 8000", rate)
			}
			t := &StreamTransport{conn: conn, streamSid: m.Start.StreamSid, incoming: make(chan voice.Frame, 256), marks: make(chan string, 16)}
			go t.readLoop(ctx)
			return t, nil
		default:
			return nil, fmt.Errorf("unexpected %q before start", m.Event)
		}
	}
}

func (t *StreamTransport) readLoop(ctx context.Context) {
	defer close(t.incoming)
	for {
		_, data, err := t.conn.Read(ctx)
		if err != nil {
			t.mu.Lock()
			if websocket.CloseStatus(err) == -1 && !errors.Is(err, context.Canceled) {
				t.err = err
			}
			t.mu.Unlock()
			return
		}
		var m streamMessage
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.Event {
		case "media":
			if m.Media == nil {
				continue
			}
			raw, err := base64.StdEncoding.DecodeString(m.Media.Payload)
			if err != nil || len(raw) == 0 {
				continue
			}
			select {
			case t.incoming <- voice.Frame{Codec: voice.MulawCodec, Data: raw}:
			case <-ctx.Done():
				return
			}
		case "mark":
			if m.Mark != nil {
				select {
				case t.marks <- m.Mark.Name:
				default:
				}
			}
		case "stop":
			return
		}
	}
}

func (t *StreamTransport) Incoming() <-chan voice.Frame { return t.incoming }
func (t *StreamTransport) Marks() <-chan string         { return t.marks }

func (t *StreamTransport) Send(ctx context.Context, f voice.Frame) error {
	if f.Codec != voice.MulawCodec {
		return fmt.Errorf("twilio streams carry %s, not %s", voice.MulawCodec, f.Codec)
	}
	msg, _ := json.Marshal(map[string]any{"event": "media", "streamSid": t.streamSid, "media": map[string]string{"payload": base64.StdEncoding.EncodeToString(f.Data)}})
	return t.conn.Write(ctx, websocket.MessageText, msg)
}

func (t *StreamTransport) Clear(ctx context.Context) error {
	msg, _ := json.Marshal(map[string]any{"event": "clear", "streamSid": t.streamSid})
	return t.conn.Write(ctx, websocket.MessageText, msg)
}

func (t *StreamTransport) Mark(ctx context.Context, name string) error {
	msg, _ := json.Marshal(map[string]any{"event": "mark", "streamSid": t.streamSid, "mark": map[string]string{"name": name}})
	return t.conn.Write(ctx, websocket.MessageText, msg)
}

func (t *StreamTransport) Close() error {
	t.closeOnce.Do(func() { _ = t.conn.Close(websocket.StatusNormalClosure, "call ended") })
	return nil
}

// Err is the read error that ended the stream, if it was not a clean close.
func (t *StreamTransport) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

var _ voice.Transport = (*StreamTransport)(nil)
