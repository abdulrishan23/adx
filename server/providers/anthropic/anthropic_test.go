package anthropic_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/providers/anthropic"
)

// Catches: the system prompt or tools not reaching the API, tool results not
// sent as tool_result blocks, a tool_use block's input or id lost on the
// way back, and the API key travelling anywhere but the header.
func TestCompleteRoundTrip(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "sk-test", r.Header.Get("x-api-key"))
		assert.NotEmpty(t, r.Header.Get("anthropic-version"))
		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5",
			"content":[{"type":"text","text":"Let me check."},{"type":"tool_use","id":"toolu_1","name":"get_analytics","input":{"from":"2026-10-01","to":"2026-10-16"}}],
			"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":321,"output_tokens":45}}`))
	}))
	defer srv.Close()
	m, err := anthropic.New(anthropic.Config{APIKey: "sk-test", BaseURL: srv.URL, HTTPClient: srv.Client(), Effort: "medium"})
	require.NoError(t, err)
	assert.Equal(t, "claude-opus-5-5", m.Name())

	res, err := m.Complete(t.Context(), assistant.Request{
		System:    "You are Ask ADX.",
		MaxTokens: 2000,
		Tools:     []assistant.ToolSpec{{Name: "get_analytics", Description: "Figures", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"from": map[string]any{"type": "string"}}, "required": []string{"from"}, "additionalProperties": false}}},
		Messages: []assistant.Message{
			{Role: assistant.RoleUser, Content: []assistant.Block{{Type: "text", Text: "Spend?"}}},
			{Role: assistant.RoleAssistant, Content: []assistant.Block{{Type: "tool_use", ToolUseID: "toolu_0", ToolName: "get_analytics", Input: []byte(`{"from":"x"}`)}}},
			{Role: assistant.RoleUser, Content: []assistant.Block{{Type: "tool_result", ToolUseID: "toolu_0", Result: `{"ok":true}`, IsError: false}}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "tool_use", res.StopReason)
	assert.Equal(t, "claude-opus-5-5", res.Model)
	assert.Equal(t, int64(321), res.Usage.InputTokens)
	require.Len(t, res.Content, 2)
	assert.Equal(t, "Let me check.", res.Content[0].Text)
	assert.Equal(t, "tool_use", res.Content[1].Type)
	assert.Equal(t, "toolu_1", res.Content[1].ToolUseID)
	assert.Equal(t, "get_analytics", res.Content[1].ToolName)
	assert.JSONEq(t, `{"from":"2026-10-01","to":"2026-10-16"}`, string(res.Content[1].Input))

	assert.Equal(t, "claude-opus-5-5", body["model"])
	assert.Equal(t, float64(2000), body["max_tokens"])
	assert.Equal(t, "medium", body["output_config"].(map[string]any)["effort"])
	system := body["system"].([]any)[0].(map[string]any)
	assert.Equal(t, "You are Ask ADX.", system["text"])
	tools := body["tools"].([]any)
	require.Len(t, tools, 1)
	tool := tools[0].(map[string]any)
	assert.Equal(t, "get_analytics", tool["name"])
	schema := tool["input_schema"].(map[string]any)
	assert.Equal(t, "object", schema["type"])
	assert.Equal(t, false, schema["additionalProperties"])
	assert.Equal(t, []any{"from"}, schema["required"])
	msgs := body["messages"].([]any)
	require.Len(t, msgs, 3)
	assert.Equal(t, "assistant", msgs[1].(map[string]any)["role"])
	toolUse := msgs[1].(map[string]any)["content"].([]any)[0].(map[string]any)
	assert.Equal(t, "tool_use", toolUse["type"])
	assert.Equal(t, "toolu_0", toolUse["id"])
	toolResult := msgs[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	assert.Equal(t, "tool_result", toolResult["type"])
	assert.Equal(t, "toolu_0", toolResult["tool_use_id"])
	_, hasThinking := body["thinking"]
	assert.False(t, hasThinking, "thinking is left adaptive by omission")
}

// Catches: an outage or rate limit not marked retryable, an auth failure
// marked retryable, and the provider's message leaking into the safe text.
func TestErrorMapping(t *testing.T) {
	status := http.StatusTooManyRequests
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Rate limited for org secret-org-id"}}`))
	}))
	defer srv.Close()
	m, err := anthropic.New(anthropic.Config{APIKey: "sk-test", BaseURL: srv.URL, HTTPClient: srv.Client()})
	require.NoError(t, err)
	m2, err := anthropic.New(anthropic.Config{APIKey: "sk-test", BaseURL: srv.URL, HTTPClient: srv.Client()})
	require.NoError(t, err)

	_, err = m.Complete(t.Context(), assistant.Request{Messages: []assistant.Message{{Role: assistant.RoleUser, Content: []assistant.Block{{Type: "text", Text: "hi"}}}}})
	require.Error(t, err)
	assert.True(t, assistant.IsRetryable(err))
	var me *assistant.ModelError
	require.ErrorAs(t, err, &me)
	assert.NotContains(t, me.Safe, "secret-org-id")

	status = http.StatusUnauthorized
	_, err = m2.Complete(t.Context(), assistant.Request{Messages: []assistant.Message{{Role: assistant.RoleUser, Content: []assistant.Block{{Type: "text", Text: "hi"}}}}})
	require.ErrorAs(t, err, &me)
	assert.False(t, me.Retryable)
	assert.Contains(t, me.Safe, "API key")

	_, err = anthropic.New(anthropic.Config{})
	require.Error(t, err)
	_, err = anthropic.New(anthropic.Config{APIKey: "k", Effort: "ultra"})
	require.Error(t, err)
}
