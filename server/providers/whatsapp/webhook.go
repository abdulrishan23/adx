package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core/conversations"
)

// Inbound side of the Cloud API: Meta posts a webhook for every message a
// customer sends to the Business number and for every status change of a
// message ADX sent. Deliveries are signed with the app secret
// (X-Hub-Signature-256) and the endpoint is verified once with a token.

// ErrBadSignature says the delivery was not signed with the app secret.
var ErrBadSignature = errors.New("whatsapp: webhook signature does not match")

// VerifySignature checks Meta's X-Hub-Signature-256 header over the raw
// body. An empty secret verifies nothing and refuses everything.
func VerifySignature(secret string, body []byte, header string) bool {
	if secret == "" {
		return false
	}
	given, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(strings.ToLower(given)), []byte(want))
}

// Sign produces the header Meta would send for body, for tests and the
// fake provider.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifyChallenge answers Meta's one-time subscription check: the token
// must match and the challenge is echoed back.
func VerifyChallenge(token string, query map[string]string) (string, bool) {
	if token == "" || query["hub.mode"] != "subscribe" || query["hub.verify_token"] != token {
		return "", false
	}
	return query["hub.challenge"], true
}

// Event is one webhook delivery, flattened: the Business number it was
// for, the messages customers sent and the statuses of messages ADX sent.
type Event struct {
	PhoneNumberID string
	Messages      []conversations.InboundMessage
	Statuses      []conversations.StatusUpdate
	// Ignored names what the delivery carried that ADX does not handle.
	Ignored []string
}

type webhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					WaID    string `json:"wa_id"`
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				} `json:"contacts"`
				Messages []struct {
					ID        string `json:"id"`
					From      string `json:"from"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text"`
					Button struct {
						Text string `json:"text"`
					} `json:"button"`
					Interactive struct {
						ButtonReply struct {
							Title string `json:"title"`
						} `json:"button_reply"`
						ListReply struct {
							Title string `json:"title"`
						} `json:"list_reply"`
					} `json:"interactive"`
				} `json:"messages"`
				Statuses []struct {
					ID          string `json:"id"`
					Status      string `json:"status"`
					Timestamp   string `json:"timestamp"`
					RecipientID string `json:"recipient_id"`
					Errors      []struct {
						Code    int    `json:"code"`
						Title   string `json:"title"`
						Message string `json:"message"`
					} `json:"errors"`
				} `json:"statuses"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

// ParseWebhook reads a delivery. Text, button and list replies are text
// messages; anything else (a picture, a voice note, a reaction) is an
// unsupported message with its type, so the thread shows it happened.
func ParseWebhook(body []byte) (Event, error) {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Event{}, fmt.Errorf("whatsapp: webhook body is not JSON: %w", err)
	}
	if p.Object != "whatsapp_business_account" {
		return Event{}, fmt.Errorf("whatsapp: webhook object %q is not a WhatsApp Business account", p.Object)
	}
	ev := Event{Messages: []conversations.InboundMessage{}, Statuses: []conversations.StatusUpdate{}, Ignored: []string{}}
	for _, e := range p.Entry {
		for _, ch := range e.Changes {
			if ch.Field != "messages" {
				ev.Ignored = append(ev.Ignored, "field "+ch.Field)
				continue
			}
			v := ch.Value
			if ev.PhoneNumberID == "" {
				ev.PhoneNumberID = v.Metadata.PhoneNumberID
			}
			names := map[string]string{}
			for _, c := range v.Contacts {
				names[c.WaID] = c.Profile.Name
			}
			for _, m := range v.Messages {
				in := conversations.InboundMessage{Channel: Channel, ProviderRef: m.ID, From: "+" + strings.TrimPrefix(m.From, "+"), ProfileName: names[m.From], ReceivedAt: unixTime(m.Timestamp)}
				switch m.Type {
				case "text":
					in.Kind, in.Body = "text", m.Text.Body
				case "button":
					in.Kind, in.Body = "text", m.Button.Text
				case "interactive":
					in.Kind, in.Body = "text", firstNonEmpty(m.Interactive.ButtonReply.Title, m.Interactive.ListReply.Title)
				default:
					in.Kind, in.Unsupported = "unsupported", firstNonEmpty(m.Type, "unknown")
				}
				if in.Kind == "text" && strings.TrimSpace(in.Body) == "" {
					in.Kind, in.Unsupported = "unsupported", "empty "+m.Type
				}
				ev.Messages = append(ev.Messages, in)
			}
			for _, s := range v.Statuses {
				st := conversations.StatusUpdate{Channel: Channel, ProviderRef: s.ID, Status: s.Status, At: unixTime(s.Timestamp)}
				for _, e := range s.Errors {
					st.Error = strings.TrimSpace(fmt.Sprintf("%d %s: %s", e.Code, e.Title, e.Message))
					break
				}
				ev.Statuses = append(ev.Statuses, st)
			}
		}
	}
	return ev, nil
}

func unixTime(s string) time.Time {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n == 0 {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// ReadWebhook verifies and parses an HTTP delivery in one step. The body
// is limited to a megabyte; a bad signature is ErrBadSignature.
func ReadWebhook(r *http.Request, appSecret string) (Event, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return Event{}, err
	}
	if !VerifySignature(appSecret, body, r.Header.Get("X-Hub-Signature-256")) {
		return Event{}, ErrBadSignature
	}
	return ParseWebhook(body)
}

// Templates.

type templateRequest struct {
	MessagingProduct string   `json:"messaging_product"`
	RecipientType    string   `json:"recipient_type"`
	To               string   `json:"to"`
	Type             string   `json:"type"`
	Template         template `json:"template"`
}

type template struct {
	Name     string   `json:"name"`
	Language language `json:"language"`
}

type language struct {
	Code string `json:"code"`
}

// SendTemplate posts a pre-approved template by name, with no parameters:
// the one shape a re-engagement message outside the window takes. The
// template must exist and be approved on the Business account; Meta
// refuses anything else and the refusal is the error.
func (c *Client) SendTemplate(ctx context.Context, msg conversations.TemplateMessage) (string, error) {
	to := strings.TrimPrefix(strings.TrimSpace(msg.To), "+")
	if to == "" || strings.Trim(to, "0123456789") != "" {
		return "", errors.New("whatsapp recipient must be an E.164 number")
	}
	if strings.TrimSpace(msg.Name) == "" {
		return "", errors.New("whatsapp template name is empty")
	}
	lang := msg.Language
	if lang == "" {
		lang = "en"
	}
	body, err := json.Marshal(templateRequest{MessagingProduct: "whatsapp", RecipientType: "individual", To: to, Type: "template", Template: template{Name: msg.Name, Language: language{Code: lang}}})
	if err != nil {
		return "", err
	}
	return c.post(ctx, body)
}

// post sends one messages request and returns the message id.
func (c *Client) post(ctx context.Context, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/"+c.cfg.PhoneNumberID+"/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("whatsapp: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("whatsapp: reading response: %w", err)
	}
	var out response
	_ = json.Unmarshal(raw, &out)
	if res.StatusCode >= 400 || out.Error != nil {
		if out.Error != nil {
			detail := out.Error.Message
			if out.Error.Details.Details != "" {
				detail += ": " + out.Error.Details.Details
			}
			return "", fmt.Errorf("whatsapp refused the message (%d %s): %s", out.Error.Code, out.Error.Type, detail)
		}
		return "", fmt.Errorf("whatsapp returned HTTP %d", res.StatusCode)
	}
	if len(out.Messages) == 0 || out.Messages[0].ID == "" {
		return "", errors.New("whatsapp accepted the request but returned no message id")
	}
	return out.Messages[0].ID, nil
}

var _ conversations.TemplateSender = (*Client)(nil)
