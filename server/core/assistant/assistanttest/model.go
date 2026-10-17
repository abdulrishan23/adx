// Package assistanttest is a scripted model for tests: each Complete call
// pops the next scripted response, and every request is recorded.
package assistanttest

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/WokeWorks/adx/server/core/assistant"
)

type Model struct {
	mu        sync.Mutex
	Responses []assistant.Response
	Errors    []error
	Requests  []assistant.Request
}

func (m *Model) Name() string { return "fake-model" }

func (m *Model) Complete(_ context.Context, req assistant.Request) (assistant.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Requests = append(m.Requests, req)
	if len(m.Errors) > 0 {
		err := m.Errors[0]
		m.Errors = m.Errors[1:]
		if err != nil {
			return assistant.Response{}, err
		}
	}
	if len(m.Responses) == 0 {
		return assistant.Response{Content: []assistant.Block{{Type: "text", Text: "(no scripted response)"}}, StopReason: "end_turn", Model: m.Name()}, nil
	}
	r := m.Responses[0]
	m.Responses = m.Responses[1:]
	if r.Model == "" {
		r.Model = m.Name()
	}
	return r, nil
}

// Text scripts a final text answer.
func Text(text string) assistant.Response {
	return assistant.Response{Content: []assistant.Block{{Type: "text", Text: text}}, StopReason: "end_turn", Usage: assistant.Usage{InputTokens: 100, OutputTokens: 20}}
}

// ToolCall scripts a tool request.
func ToolCall(id, name string, input any) assistant.Response {
	raw, _ := json.Marshal(input)
	return assistant.Response{Content: []assistant.Block{{Type: "tool_use", ToolUseID: id, ToolName: name, Input: raw}}, StopReason: "tool_use", Usage: assistant.Usage{InputTokens: 100, OutputTokens: 30}}
}
