// Package assistant is Ask ADX: a conversation with a language model that may
// only read, through an allowlist of tools that call the same core functions
// with the same actor as the person asking. The model proposes; deterministic
// code calculates every figure and decides what runs. Numbers in an answer are
// checked against the tool results that produced them. Nothing here can
// change an ad account, a lead or a setting: there is no write tool, and a
// tool name the allowlist doesn't know is refused whoever asks for it,
// including text inside a tool result.
package assistant

import (
	"context"
	"encoding/json"
	"errors"
)

// Role of a turn as the model sees it.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ToolSpec describes a tool to the model: name, what it does, and the JSON
// schema of its input.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// Block is one piece of a message: text, a tool call the model asks for, or
// a tool result ADX supplies.
type Block struct {
	Type string // "text", "tool_use", "tool_result"
	Text string
	// tool_use
	ToolUseID string
	ToolName  string
	Input     json.RawMessage
	// tool_result
	Result  string
	IsError bool
}

type Message struct {
	Role    Role
	Content []Block
}

// Request is one model call.
type Request struct {
	System    string
	Messages  []Message
	Tools     []ToolSpec
	MaxTokens int
}

type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

// Response is what the model returned: blocks, why it stopped, and usage.
// StopReason is "end_turn", "tool_use", "max_tokens" or "refusal".
type Response struct {
	Content    []Block
	StopReason string
	Usage      Usage
	Model      string
}

// Model is the language model behind the assistant. The real adapter lives
// in providers/anthropic; tests use assistanttest.Model. Any error that is
// the provider's fault (outage, rate limit, timeout) is a *ModelError with
// Retryable set, so the conversation records an unavailable turn instead of
// an invented one.
type Model interface {
	Name() string
	Complete(ctx context.Context, req Request) (Response, error)
}

// ModelError is a failure to get a response.
type ModelError struct {
	Retryable bool
	// Safe is shown to the person; Cause goes to the log.
	Safe  string
	Cause error
}

func (e *ModelError) Error() string {
	if e.Cause != nil {
		return e.Safe + ": " + e.Cause.Error()
	}
	return e.Safe
}

func (e *ModelError) Unwrap() error { return e.Cause }

// IsRetryable reports whether err is a retryable model failure.
func IsRetryable(err error) bool {
	var me *ModelError
	return errors.As(err, &me) && me.Retryable
}
