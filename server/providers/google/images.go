// Package google is ADX's client for Google's generative media on the Gemini
// API: pictures through the image model and clips through Veo. One API key
// covers both. Nothing here
// decides what to make; core/studio writes the prompt and keeps the result.
package google

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL    = "https://generativelanguage.googleapis.com/v1beta"
	defaultImageModel = "gemini-3.1-flash-image"
	maxImageBody      = 40 << 20
)

type Config struct {
	APIKey     string
	ImageModel string // defaults to gemini-3.1-flash-image
	VideoModel string // defaults to veo-3.1-generate-preview
	BaseURL    string
	HTTPClient *http.Client
	// Poll is how often a video operation is checked; tests shorten it.
	Poll time.Duration
}

type Client struct {
	key, model, videoModel, base string
	http                         *http.Client
	poll                         time.Duration
}

func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("google: an API key is required")
	}
	c := &Client{key: cfg.APIKey, model: cfg.ImageModel, videoModel: cfg.VideoModel, base: strings.TrimRight(cfg.BaseURL, "/"), http: cfg.HTTPClient, poll: cfg.Poll}
	if c.model == "" {
		c.model = defaultImageModel
	}
	if c.base == "" {
		c.base = defaultBaseURL
	}
	if c.http == nil {
		c.http = &http.Client{Timeout: 120 * time.Second}
	}
	return c, nil
}

// ImageRequest is one picture to make. References are pictures the model
// should take its cue from, such as the project's own renders.
type ImageRequest struct {
	Prompt      string
	AspectRatio string // "9:16", "4:5", "1:1", "16:9"
	Size        string // "1K" or "2K"
	References  []Reference
}

type Reference struct {
	MIME string
	Data []byte
}

type Image struct {
	MIME string
	Data []byte
}

// Error is a failure the caller may show: the model refused, the key is
// wrong, the quota is spent. Retryable marks outages and rate limits.
type Error struct {
	Status    int
	Safe      string
	Retryable bool
	Cause     error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("google: %s: %v", e.Safe, e.Cause)
	}
	return "google: " + e.Safe
}

func (e *Error) Unwrap() error { return e.Cause }

// GenerateImage asks the image model for one picture.
func (c *Client) GenerateImage(ctx context.Context, in ImageRequest) (Image, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return Image{}, &Error{Safe: "the prompt is empty"}
	}
	parts := []map[string]any{}
	for _, r := range in.References {
		parts = append(parts, map[string]any{"inline_data": map[string]any{"mime_type": r.MIME, "data": base64.StdEncoding.EncodeToString(r.Data)}})
	}
	parts = append(parts, map[string]any{"text": in.Prompt})
	imageConfig := map[string]any{}
	if in.AspectRatio != "" {
		imageConfig["aspectRatio"] = in.AspectRatio
	}
	if in.Size != "" {
		imageConfig["imageSize"] = in.Size
	}
	body := map[string]any{
		"contents":         []map[string]any{{"role": "user", "parts": parts}},
		"generationConfig": map[string]any{"responseModalities": []string{"IMAGE"}, "imageConfig": imageConfig},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return Image{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/models/%s:generateContent", c.base, c.model), bytes.NewReader(raw))
	if err != nil {
		return Image{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.key)
	res, err := c.http.Do(req)
	if err != nil {
		return Image{}, &Error{Safe: "the image service did not answer", Retryable: true, Cause: err}
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxImageBody))
	if err != nil {
		return Image{}, &Error{Safe: "the image service's answer was cut off", Retryable: true, Cause: err}
	}
	if res.StatusCode != http.StatusOK {
		return Image{}, httpError(res.StatusCode, data)
	}
	var out struct {
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []struct {
					Text       string `json:"text"`
					InlineData *struct {
						MIME string `json:"mimeType"`
						Data string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		PromptFeedback *struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return Image{}, &Error{Safe: "the image service answered in an unexpected shape", Cause: err}
	}
	if out.PromptFeedback != nil && out.PromptFeedback.BlockReason != "" {
		return Image{}, &Error{Safe: "the image service declined the prompt (" + out.PromptFeedback.BlockReason + ")"}
	}
	for _, cand := range out.Candidates {
		for _, p := range cand.Content.Parts {
			if p.InlineData != nil && p.InlineData.Data != "" {
				b, err := base64.StdEncoding.DecodeString(p.InlineData.Data)
				if err != nil {
					return Image{}, &Error{Safe: "the picture could not be decoded", Cause: err}
				}
				return Image{MIME: p.InlineData.MIME, Data: b}, nil
			}
		}
		if cand.FinishReason != "" && cand.FinishReason != "STOP" {
			return Image{}, &Error{Safe: "the image service stopped without a picture (" + cand.FinishReason + ")"}
		}
	}
	return Image{}, &Error{Safe: "the image service answered without a picture"}
}

func httpError(status int, body []byte) *Error {
	var g struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &g)
	msg := g.Error.Message
	if len(msg) > 200 {
		msg = msg[:200]
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return &Error{Status: status, Safe: "the image service refused the API key"}
	case status == http.StatusTooManyRequests:
		return &Error{Status: status, Safe: "the image service's quota is used up for now", Retryable: true}
	case status >= 500:
		return &Error{Status: status, Safe: "the image service is having trouble", Retryable: true}
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	return &Error{Status: status, Safe: "the image service rejected the request: " + msg}
}
