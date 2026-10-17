// Package whatsapp sends outreach messages through the WhatsApp Business
// Cloud API (Meta Graph). It implements outreach.Messenger for the
// "whatsapp" channel and sends plain text only: a free-form text reaches a
// person only inside the 24-hour customer-service window Meta keeps per
// conversation, and anything outside it needs a pre-approved template,
// which this adapter does not send. The API reports that case as an error
// and the draft fails with the reason; nothing is retried blindly.
package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core/outreach"
)

const (
	// DefaultBaseURL is the Graph API host and version this adapter was
	// written against.
	DefaultBaseURL = "https://graph.facebook.com/v21.0"
	// Channel is the outreach channel this adapter serves.
	Channel = "whatsapp"
)

type Config struct {
	// AccessToken is a system-user token with whatsapp_business_messaging.
	AccessToken string
	// PhoneNumberID is the sender: the WhatsApp Business phone number id,
	// not the number itself.
	PhoneNumberID string
	BaseURL       string
	HTTPClient    *http.Client
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.AccessToken) == "" || strings.TrimSpace(cfg.PhoneNumberID) == "" {
		return nil, errors.New("whatsapp access token and phone number id are required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	h := cfg.HTTPClient
	if h == nil {
		h = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{cfg: cfg, http: h}, nil
}

func (c *Client) Name() string       { return "whatsapp-cloud" }
func (c *Client) Channels() []string { return []string{Channel} }

type request struct {
	MessagingProduct string `json:"messaging_product"`
	RecipientType    string `json:"recipient_type"`
	To               string `json:"to"`
	Type             string `json:"type"`
	Text             text   `json:"text"`
}

type text struct {
	PreviewURL bool   `json:"preview_url"`
	Body       string `json:"body"`
}

type response struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
	Error *apiError `json:"error"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    int    `json:"code"`
	Details struct {
		Details string `json:"details"`
	} `json:"error_data"`
}

// Send posts one text message and returns Meta's message id. The recipient
// is the person's E.164 number; the API wants it without the plus.
func (c *Client) Send(ctx context.Context, msg outreach.OutboundMessage) (string, error) {
	if msg.Channel != Channel {
		return "", fmt.Errorf("whatsapp adapter does not serve channel %q", msg.Channel)
	}
	to := strings.TrimPrefix(strings.TrimSpace(msg.To), "+")
	if to == "" || strings.Trim(to, "0123456789") != "" {
		return "", errors.New("whatsapp recipient must be an E.164 number")
	}
	if strings.TrimSpace(msg.Body) == "" {
		return "", errors.New("whatsapp message body is empty")
	}
	body, err := json.Marshal(request{MessagingProduct: "whatsapp", RecipientType: "individual", To: to, Type: "text", Text: text{Body: msg.Body}})
	if err != nil {
		return "", err
	}
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

var _ outreach.Messenger = (*Client)(nil)
