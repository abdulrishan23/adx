// Package anthropic adapts Claude (the Anthropic Messages API, through the
// official Go SDK) to core/assistant.Model. The model name, API key, base
// URL, timeout and effort are configuration; nothing here is specific to
// one Claude model beyond the default id.
package anthropic

import (
	"context"
	"errors"
	"net/http"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/WokeWorks/adx/server/core/assistant"
)

// DefaultModel is Claude Opus 5.5, the current default for new work.
const DefaultModel = "claude-opus-5-5"

type Config struct {
	APIKey  string
	Model   string
	BaseURL string
	Timeout time.Duration
	// Effort is low, medium, high, xhigh or max; empty leaves the model's default.
	Effort     string
	HTTPClient *http.Client
}

type Model struct {
	client sdk.Client
	model  string
	effort string
}

func New(cfg Config) (*Model, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("anthropic API key is required")
	}
	model := cfg.Model
	if model == "" {
		model = DefaultModel
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey), option.WithRequestTimeout(timeout), option.WithMaxRetries(2)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cfg.HTTPClient))
	}
	switch cfg.Effort {
	case "", "low", "medium", "high", "xhigh", "max":
	default:
		return nil, errors.New("anthropic effort must be low, medium, high, xhigh or max")
	}
	return &Model{client: sdk.NewClient(opts...), model: model, effort: cfg.Effort}, nil
}

func (m *Model) Name() string { return m.model }

// Complete maps the request to the Messages API. Thinking is left adaptive
// (the model's default); tool use is "auto" with the tools core allows.
func (m *Model) Complete(ctx context.Context, req assistant.Request) (assistant.Response, error) {
	params := sdk.MessageNewParams{Model: sdk.Model(m.model), MaxTokens: int64(req.MaxTokens)}
	if params.MaxTokens <= 0 {
		params.MaxTokens = 4000
	}
	if req.System != "" {
		params.System = []sdk.TextBlockParam{{Text: req.System}}
	}
	if m.effort != "" {
		params.OutputConfig = sdk.OutputConfigParam{Effort: sdk.OutputConfigEffort(m.effort)}
	}
	for _, t := range req.Tools {
		props, _ := t.InputSchema["properties"]
		required, _ := t.InputSchema["required"].([]string)
		tool := sdk.ToolParam{Name: t.Name, Description: sdk.String(t.Description), InputSchema: sdk.ToolInputSchemaParam{Properties: props, Required: required}}
		if ap, ok := t.InputSchema["additionalProperties"]; ok {
			tool.InputSchema.ExtraFields = map[string]any{"additionalProperties": ap}
		}
		params.Tools = append(params.Tools, sdk.ToolUnionParam{OfTool: &tool})
	}
	for _, msg := range req.Messages {
		var blocks []sdk.ContentBlockParamUnion
		for _, b := range msg.Content {
			switch b.Type {
			case "text":
				if b.Text != "" {
					blocks = append(blocks, sdk.NewTextBlock(b.Text))
				}
			case "tool_use":
				var input any
				_ = jsonUnmarshal(b.Input, &input)
				blocks = append(blocks, sdk.NewToolUseBlock(b.ToolUseID, input, b.ToolName))
			case "tool_result":
				blocks = append(blocks, sdk.NewToolResultBlock(b.ToolUseID, b.Result, b.IsError))
			}
		}
		if len(blocks) == 0 {
			continue
		}
		if msg.Role == assistant.RoleAssistant {
			params.Messages = append(params.Messages, sdk.NewAssistantMessage(blocks...))
		} else {
			params.Messages = append(params.Messages, sdk.NewUserMessage(blocks...))
		}
	}

	resp, err := m.client.Messages.New(ctx, params)
	if err != nil {
		return assistant.Response{}, mapError(err)
	}
	out := assistant.Response{StopReason: string(resp.StopReason), Model: string(resp.Model), Usage: assistant.Usage{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}}
	for _, block := range resp.Content {
		switch v := block.AsAny().(type) {
		case sdk.TextBlock:
			out.Content = append(out.Content, assistant.Block{Type: "text", Text: v.Text})
		case sdk.ToolUseBlock:
			out.Content = append(out.Content, assistant.Block{Type: "tool_use", ToolUseID: v.ID, ToolName: v.Name, Input: []byte(v.JSON.Input.Raw())})
		}
	}
	return out, nil
}

// mapError turns SDK failures into assistant.ModelError with a safe message.
// Rate limits, overload, server errors and network failures are retryable;
// authentication and bad requests are not and name the fix.
func mapError(err error) error {
	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden:
			return &assistant.ModelError{Retryable: false, Safe: "The assistant's API key was rejected; an admin needs to fix the model configuration.", Cause: err}
		case apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode == 529 || apiErr.StatusCode >= 500:
			return &assistant.ModelError{Retryable: true, Safe: "The model is busy or unavailable right now. Nothing was generated; try again in a moment.", Cause: err}
		case apiErr.StatusCode == http.StatusBadRequest || apiErr.StatusCode == http.StatusNotFound:
			return &assistant.ModelError{Retryable: false, Safe: "The model rejected the request; an admin needs to check the model configuration.", Cause: err}
		}
		return &assistant.ModelError{Retryable: apiErr.StatusCode >= 500, Safe: "The model did not answer. Nothing was generated.", Cause: err}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &assistant.ModelError{Retryable: true, Safe: "The model took too long to answer. Nothing was generated; try again.", Cause: err}
	}
	return &assistant.ModelError{Retryable: true, Safe: "The model could not be reached. Nothing was generated; try again.", Cause: err}
}

var _ assistant.Model = (*Model)(nil)
