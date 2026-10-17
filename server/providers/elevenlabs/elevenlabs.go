// Package elevenlabs adapts ElevenLabs Conversational AI to core/voice.Speech
// over its WebSocket protocol: ADX sends µ-law audio as user_audio_chunk
// messages and receives audio, transcripts, interruptions and pings. The
// agent (voice, audio format, language) is configured in the ElevenLabs
// dashboard; ADX overrides only the prompt and the first message per call.
// Wired only when ADX_ELEVENLABS_API_KEY and ADX_ELEVENLABS_AGENT_ID are set.
package elevenlabs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/WokeWorks/adx/server/core/voice"
)

const defaultURL = "wss://api.elevenlabs.io/v1/convai/conversation"

type Config struct {
	APIKey  string
	AgentID string
	// URL overrides the conversation endpoint, for tests.
	URL        string
	HTTPClient *http.Client
	// HandshakeTimeout bounds the wait for conversation_initiation_metadata.
	HandshakeTimeout time.Duration
}

type Speech struct {
	cfg Config
}

func New(cfg Config) (*Speech, error) {
	if cfg.APIKey == "" || cfg.AgentID == "" {
		return nil, errors.New("elevenlabs API key and agent id are required")
	}
	if cfg.URL == "" {
		cfg.URL = defaultURL
	}
	if cfg.HandshakeTimeout == 0 {
		cfg.HandshakeTimeout = 10 * time.Second
	}
	return &Speech{cfg: cfg}, nil
}

func (s *Speech) Name() string { return "elevenlabs" }

// Start opens the conversation, sends the per-call overrides and waits for
// the initiation metadata. A negotiated audio format other than ulaw_8000
// is an error: it would be static on the phone, not speech.
func (s *Speech) Start(ctx context.Context, cfg voice.SessionConfig) (voice.Session, error) {
	if cfg.Codec != voice.MulawCodec {
		return nil, fmt.Errorf("elevenlabs adapter speaks %s, not %s", voice.MulawCodec, cfg.Codec)
	}
	u, err := url.Parse(s.cfg.URL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("agent_id", s.cfg.AgentID)
	u.RawQuery = q.Encode()
	opts := &websocket.DialOptions{HTTPHeader: http.Header{"xi-api-key": {s.cfg.APIKey}}, HTTPClient: s.cfg.HTTPClient}
	conn, _, err := websocket.Dial(ctx, u.String(), opts)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs connection failed: %w", err)
	}
	conn.SetReadLimit(4 << 20)
	init := map[string]any{"type": "conversation_initiation_client_data", "conversation_config_override": map[string]any{"agent": map[string]any{"prompt": map[string]any{"prompt": cfg.Instructions}, "first_message": cfg.FirstMessage, "language": cfg.Language}}}
	raw, _ := json.Marshal(init)
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		_ = conn.CloseNow()
		return nil, err
	}
	sess := &session{conn: conn, events: make(chan voice.SpeechEvent, 256), started: time.Now()}
	hctx, cancel := context.WithTimeout(ctx, s.cfg.HandshakeTimeout)
	defer cancel()
	for {
		_, data, err := conn.Read(hctx)
		if err != nil {
			_ = conn.CloseNow()
			return nil, fmt.Errorf("elevenlabs handshake failed: %w", err)
		}
		var m serverEvent
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		if m.Type == "conversation_initiation_metadata" {
			if f := m.Metadata.AgentOutputAudioFormat; f != "" && f != "ulaw_8000" {
				conn.Close(websocket.StatusPolicyViolation, "audio format")
				return nil, fmt.Errorf("elevenlabs agent outputs %s; the agent must be set to ulaw_8000", f)
			}
			if f := m.Metadata.UserInputAudioFormat; f != "" && f != "ulaw_8000" {
				conn.Close(websocket.StatusPolicyViolation, "audio format")
				return nil, fmt.Errorf("elevenlabs agent expects %s input; the agent must be set to ulaw_8000", f)
			}
			sess.conversationID = m.Metadata.ConversationID
			break
		}
		if m.Type == "ping" {
			sess.pong(ctx, m.Ping.EventID)
		}
	}
	go sess.readLoop(context.WithoutCancel(ctx))
	return sess, nil
}

type serverEvent struct {
	Type     string `json:"type"`
	Metadata struct {
		ConversationID         string `json:"conversation_id"`
		AgentOutputAudioFormat string `json:"agent_output_audio_format"`
		UserInputAudioFormat   string `json:"user_input_audio_format"`
	} `json:"conversation_initiation_metadata_event"`
	Ping struct {
		EventID int64 `json:"event_id"`
	} `json:"ping_event"`
	Audio struct {
		Base64 string `json:"audio_base_64"`
	} `json:"audio_event"`
	UserTranscript struct {
		Text string `json:"user_transcript"`
	} `json:"user_transcription_event"`
	AgentResponse struct {
		Text string `json:"agent_response"`
	} `json:"agent_response_event"`
}

type session struct {
	conn           *websocket.Conn
	conversationID string
	events         chan voice.SpeechEvent
	started        time.Time
	mu             sync.Mutex
	segments       []voice.Segment
	ended          bool
	closeOnce      sync.Once
}

func (s *session) pong(ctx context.Context, id int64) {
	raw, _ := json.Marshal(map[string]any{"type": "pong", "event_id": id})
	_ = s.conn.Write(ctx, websocket.MessageText, raw)
}

func (s *session) readLoop(ctx context.Context) {
	defer close(s.events)
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			// The agent closing the socket is how a conversation ends.
			s.emit(voice.SpeechEvent{Kind: voice.KindEnd})
			return
		}
		var m serverEvent
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.Type {
		case "ping":
			s.pong(ctx, m.Ping.EventID)
		case "audio":
			raw, err := base64.StdEncoding.DecodeString(m.Audio.Base64)
			if err != nil || len(raw) == 0 {
				continue
			}
			s.emit(voice.SpeechEvent{Kind: voice.KindAudio, Audio: voice.Frame{Codec: voice.MulawCodec, Data: raw}})
		case "user_transcript":
			s.transcript("lead", m.UserTranscript.Text)
		case "agent_response":
			s.transcript("agent", m.AgentResponse.Text)
		case "interruption":
			s.emit(voice.SpeechEvent{Kind: voice.KindInterruption})
		case "close", "conversation_end":
			s.emit(voice.SpeechEvent{Kind: voice.KindEnd})
			return
		}
	}
}

func (s *session) transcript(speaker, text string) {
	if text == "" {
		return
	}
	at := time.Since(s.started)
	s.mu.Lock()
	s.segments = append(s.segments, voice.Segment{Speaker: speaker, Text: text, Start: at, End: at})
	s.mu.Unlock()
	s.emit(voice.SpeechEvent{Kind: voice.KindTranscript, Speaker: speaker, Text: text})
}

func (s *session) emit(ev voice.SpeechEvent) {
	s.mu.Lock()
	ended := s.ended
	s.mu.Unlock()
	if ended {
		return
	}
	select {
	case s.events <- ev:
	default:
		// A full buffer means the bridge is gone; drop rather than block.
	}
}

func (s *session) SendAudio(ctx context.Context, f voice.Frame) error {
	if f.Codec != voice.MulawCodec {
		return fmt.Errorf("elevenlabs expects %s, got %s", voice.MulawCodec, f.Codec)
	}
	raw, _ := json.Marshal(map[string]string{"user_audio_chunk": base64.StdEncoding.EncodeToString(f.Data)})
	return s.conn.Write(ctx, websocket.MessageText, raw)
}

func (s *session) Events() <-chan voice.SpeechEvent { return s.events }

func (s *session) End(context.Context) (voice.Transcript, error) {
	s.mu.Lock()
	s.ended = true
	segs := append([]voice.Segment(nil), s.segments...)
	s.mu.Unlock()
	s.closeOnce.Do(func() { _ = s.conn.Close(websocket.StatusNormalClosure, "call ended") })
	return voice.Transcript{Segments: segs, Duration: time.Since(s.started)}, nil
}

var _ voice.Speech = (*Speech)(nil)
