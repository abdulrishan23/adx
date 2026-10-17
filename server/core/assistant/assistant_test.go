package assistant_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/alerts"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/assistant/assistanttest"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	model     *assistanttest.Model
	assistant *assistant.Assistant
}

func setup(t *testing.T, limits assistant.Limits) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)), model: &assistanttest.Model{}}
	coretest.SeedUsers(t, e.db)
	ev := evidence.New(e.db, e.clock)
	rep := reporting.New(e.db, nil, nil, ev, reporting.Config{Clock: e.clock})
	services := assistant.Services{Reporting: rep, Leads: leads.New(e.db, nil, nil, ev, leads.Config{Clock: e.clock}), Alerts: alerts.New(e.db, rep, alerts.Config{Clock: e.clock})}
	e.assistant = assistant.New(e.db, e.model, services, assistant.Config{Clock: e.clock, Limits: limits})
	// One campaign with 1234.56 AED spent on 2026-10-16.
	cred, err := e.db.Writer().UpsertCredential(t.Context(), store.UpsertCredentialParams{Provider: "google_ads", Kind: "oauth", Subject: "u", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(t.Context(), store.UpsertAccountParams{Type: "google_ads", ExternalID: "1", CredentialID: cred.ID, Currency: sql.NullString{String: "AED", Valid: true},
		TimeZone: sql.NullString{String: "Asia/Dubai", Valid: true}, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	c, err := e.db.Writer().UpsertAdCampaign(t.Context(), store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: acct.ID, Platform: "google_ads", ExternalID: "g1", Name: "Wadi Search", Status: "ENABLED",
		Currency: sql.NullString{String: "AED", Valid: true}, FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	require.NoError(t, e.db.Writer().UpsertCampaignDailyMetrics(t.Context(), store.UpsertCampaignDailyMetricsParams{WorkspaceID: 1, AccountID: acct.ID, CampaignID: c.ID, Platform: "google_ads",
		Day: "2026-10-16", Currency: "AED", CostMinor: 123456, Impressions: 10, Clicks: 2, ObservedAt: e.clock.Now()}))
	return e
}

func (e *env) ask(t *testing.T, actor core.Actor, text string) assistant.Reply {
	t.Helper()
	conv, err := e.assistant.CreateConversation(t.Context(), actor)
	require.NoError(t, err)
	reply, err := e.assistant.Send(t.Context(), actor, conv.ID, text)
	require.NoError(t, err)
	return reply
}

func last(r assistant.Reply) assistant.Turn { return r.Turns[len(r.Turns)-1] }

// Catches: a tool running with more than the asker's role, tool results not
// reaching the model as data, numbers in the answer that no tool produced
// passing silently, and the answer not carrying evidence references.
func TestAnswersAreGroundedInToolResults(t *testing.T) {
	e := setup(t, assistant.Limits{})
	e.model.Responses = []assistant.Response{
		assistanttest.ToolCall("call-1", "get_analytics", map[string]any{"from": "2026-10-16", "to": "2026-10-16", "group": "campaign"}),
		assistanttest.Text("Wadi Search spent 1234.56 AED on 2026-10-16 (123456 fils). Leads: 0. Revenue is not available because Done-Deal values don't arrive yet."),
	}
	reply := e.ask(t, coretest.Viewer, "How much did we spend yesterday?")
	require.Len(t, reply.Turns, 4, "user, assistant tool call, tool result, final answer")
	assert.Equal(t, "assistant", reply.Turns[1].Role)
	require.Len(t, reply.Turns[1].ToolCalls, 1)
	assert.Equal(t, "get_analytics", reply.Turns[1].ToolCalls[0].Name)
	toolTurn := reply.Turns[2]
	assert.Equal(t, "tool", toolTurn.Role)
	assert.Equal(t, "get_analytics", *toolTurn.ToolName)
	var result assistant.ToolResult
	require.NoError(t, json.Unmarshal(toolTurn.ToolResult, &result))
	assert.Empty(t, result.Error)
	assert.NotEmpty(t, toolTurn.EvidenceRefs)
	assert.Equal(t, "formula", toolTurn.EvidenceRefs[0].Kind)

	final := last(reply)
	assert.Equal(t, "complete", final.Status)
	assert.Contains(t, final.Content, "1234.56")
	assert.Empty(t, final.Warnings, "every number in the answer is in a tool result")
	assert.NotEmpty(t, final.EvidenceRefs, "the answer carries the refs of the tools it used")
	assert.Equal(t, int64(100), *final.InputTokens)
	assert.Nil(t, final.CostMicros, "no price configured: cost is not available, not zero")

	// The model saw the system rules, the tools and the tool result as data.
	require.Len(t, e.model.Requests, 2)
	assert.Contains(t, e.model.Requests[0].System, "Every number you state must come from a tool result")
	assert.Len(t, e.model.Requests[0].Tools, 4)
	second := e.model.Requests[1].Messages
	assert.Equal(t, "tool_result", second[len(second)-1].Content[0].Type)
	assert.Contains(t, second[len(second)-1].Content[0].Result, `"minor":123456`)

	// An invented number is flagged.
	e.model.Responses = []assistant.Response{
		assistanttest.ToolCall("call-2", "get_analytics", map[string]any{"from": "2026-10-16", "to": "2026-10-16"}),
		assistanttest.Text("You spent 9876 AED and got 42 leads."),
	}
	reply = e.ask(t, coretest.Viewer, "Again?")
	final = last(reply)
	require.Len(t, final.Warnings, 1)
	assert.Contains(t, final.Warnings[0], "ungrounded_numbers")
	assert.Contains(t, final.Warnings[0], "9876")
	assert.Contains(t, final.Warnings[0], "42")

	// A number with no tool call at all is flagged too.
	e.model.Responses = []assistant.Response{assistanttest.Text("Probably around 5000 AED.")}
	final = last(e.ask(t, coretest.Viewer, "Guess?"))
	assert.Contains(t, final.Warnings, "no_tool_evidence")
}

// Catches: a question the data can't answer getting a confident number
// instead of an explicit gap.
func TestQuestionWithoutDataProducesAnExplicitGap(t *testing.T) {
	e := setup(t, assistant.Limits{})
	e.model.Responses = []assistant.Response{
		assistanttest.ToolCall("c", "get_analytics", map[string]any{"from": "2026-10-01", "to": "2026-10-16", "compare": "previous"}),
		assistanttest.Text("ROAS is not available: Done-Deal values do not arrive from Akarati as a field yet, so revenue is unknown. Spend was 1234.56 AED."),
	}
	reply := e.ask(t, coretest.Marketer, "What was ROAS this month?")
	toolTurn := reply.Turns[2]
	assert.Contains(t, string(toolTurn.ToolResult), `"roas":{"value":null`)
	assert.Contains(t, string(toolTurn.ToolResult), "Done-Deal value does not arrive")
	assert.Empty(t, last(reply).Warnings)
}

// Catches: a write reaching anything. The model asks for a tool that isn't
// allowlisted (once prompted by injected text in a tool result); the request
// is refused, nothing is written, and the policy evaluator is never involved.
func TestWritesAndInjectedInstructionsAreRefused(t *testing.T) {
	e := setup(t, assistant.Limits{})
	e.model.Responses = []assistant.Response{
		assistanttest.ToolCall("c1", "set_budget", map[string]any{"campaign": "Wadi Search", "dailyBudgetMinor": 999999}),
		assistanttest.Text("I can't change budgets. Changes go through proposals and approvals in ADX."),
	}
	reply := e.ask(t, coretest.Admin, "Raise the Wadi Search budget to 9999 AED a day")
	toolTurn := reply.Turns[2]
	var result assistant.ToolResult
	require.NoError(t, json.Unmarshal(toolTurn.ToolResult, &result))
	assert.Contains(t, result.Error, `tool "set_budget" is not available`)
	assert.Contains(t, result.Error, "cannot change budgets")
	var budget sql.NullInt64
	require.NoError(t, e.db.Read.QueryRowContext(t.Context(), "SELECT budget_minor FROM ad_campaigns WHERE external_id = 'g1'").Scan(&budget))
	assert.False(t, budget.Valid, "nothing changed")
	var n int
	require.NoError(t, e.db.Read.QueryRowContext(t.Context(), "SELECT count(*) FROM policies").Scan(&n))
	assert.Zero(t, n, "no policy was consulted or created")

	// Content inside a tool result cannot widen the allowlist either: the
	// model, "following" instructions found in data, asks for a tool name
	// that doesn't exist and gets the same refusal.
	e.model.Responses = []assistant.Response{
		assistanttest.ToolCall("c2", "list_alerts", map[string]any{"state": "open"}),
		assistanttest.ToolCall("c3", "pause_campaign", map[string]any{"campaign": "g1"}),
		assistanttest.Text("No alerts are open, and I cannot pause campaigns."),
	}
	reply = e.ask(t, coretest.Marketer, "Anything urgent?")
	refused := reply.Turns[4]
	require.NoError(t, json.Unmarshal(refused.ToolResult, &result))
	assert.Contains(t, result.Error, `"pause_campaign" is not available`)

	// Bad input to a real tool is refused before it runs.
	e.model.Responses = []assistant.Response{
		assistanttest.ToolCall("c4", "get_analytics", map[string]any{"from": "2026-10-16"}),
		assistanttest.Text("I need a full period."),
	}
	reply = e.ask(t, coretest.Marketer, "Spend?")
	require.NoError(t, json.Unmarshal(reply.Turns[2].ToolResult, &result))
	assert.Contains(t, result.Error, `missing required field "to"`)
	e.model.Responses = []assistant.Response{
		assistanttest.ToolCall("c5", "get_analytics", map[string]any{"from": "2026-10-16", "to": "2026-10-16", "group": "colour", "extra": 1}),
		assistanttest.Text("Bad input."),
	}
	reply = e.ask(t, coretest.Marketer, "Spend?")
	require.NoError(t, json.Unmarshal(reply.Turns[2].ToolResult, &result))
	assert.Contains(t, result.Error, "invalid input")
}

// Catches: an outage turned into an invented answer, a non-retryable error
// marked retryable, and a refusal shown as an answer.
func TestModelOutageIsRetryableAndInventsNothing(t *testing.T) {
	e := setup(t, assistant.Limits{})
	e.model.Errors = []error{&assistant.ModelError{Retryable: true, Safe: "The model is busy or unavailable right now. Nothing was generated; try again in a moment.", Cause: errors.New("529 overloaded")}}
	reply := e.ask(t, coretest.Viewer, "Spend?")
	final := last(reply)
	assert.Equal(t, "unavailable", final.Status)
	assert.Contains(t, final.Content, "Nothing was generated")
	assert.Equal(t, []string{"model_unavailable", "retryable"}, final.Warnings)
	assert.NotContains(t, final.Content, "529")

	e.model.Errors = []error{&assistant.ModelError{Retryable: false, Safe: "The assistant's API key was rejected; an admin needs to fix the model configuration."}}
	final = last(e.ask(t, coretest.Viewer, "Spend?"))
	assert.Equal(t, "unavailable", final.Status)
	assert.Equal(t, []string{"model_error"}, final.Warnings)

	e.model.Responses = []assistant.Response{{Content: nil, StopReason: "refusal"}}
	final = last(e.ask(t, coretest.Viewer, "Spend?"))
	assert.Equal(t, "refused", final.Status)

	// The failed turns are not replayed to the model as history.
	conv, err := e.assistant.CreateConversation(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	e.model.Errors = []error{&assistant.ModelError{Retryable: true, Safe: "busy"}}
	_, err = e.assistant.Send(t.Context(), coretest.Viewer, conv.ID, "first")
	require.NoError(t, err)
	e.model.Responses = []assistant.Response{assistanttest.Text("Hello.")}
	_, err = e.assistant.Send(t.Context(), coretest.Viewer, conv.ID, "second")
	require.NoError(t, err)
	req := e.model.Requests[len(e.model.Requests)-1]
	assert.Len(t, req.Messages, 2, "two user messages, no unavailable assistant turn")

	// No model at all: an honest unavailable turn, no error.
	none := assistant.New(e.db, nil, assistant.Services{}, assistant.Config{Clock: e.clock})
	conv, err = none.CreateConversation(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	r, err := none.Send(t.Context(), coretest.Viewer, conv.ID, "hi")
	require.NoError(t, err)
	assert.Equal(t, "unavailable", last(r).Status)
	assert.Contains(t, last(r).Warnings, "model_not_configured")
}

// Catches: a tool loop that never ends, a daily budget that doesn't bite,
// and cost shown as zero when no price exists or wrong when one does.
func TestLimitsAndCost(t *testing.T) {
	e := setup(t, assistant.Limits{MaxToolRounds: 2, InputPriceMicros: 4_000_000, OutputPriceMicros: 20_000_000})
	e.model.Responses = []assistant.Response{
		assistanttest.ToolCall("a", "get_lead_coverage", nil),
		assistanttest.ToolCall("b", "get_lead_coverage", nil),
		assistanttest.ToolCall("c", "get_lead_coverage", nil),
	}
	reply := e.ask(t, coretest.Viewer, "loop")
	final := last(reply)
	assert.Contains(t, final.Warnings, "tool_round_limit")
	assert.Contains(t, final.Content, "maximum number of tool calls")
	// 100 input tokens at $4/M = 400 micros; 30 output at $20/M = 600 micros.
	assert.Equal(t, int64(1000), *reply.Turns[1].CostMicros)

	budgeted := assistant.New(e.db, e.model, assistant.Services{}, assistant.Config{Clock: e.clock, Limits: assistant.Limits{DailyTokenBudget: 100}})
	conv, err := budgeted.CreateConversation(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	r, err := budgeted.Send(t.Context(), coretest.Viewer, conv.ID, "more")
	require.NoError(t, err)
	assert.Equal(t, "unavailable", last(r).Status)
	assert.Contains(t, last(r).Warnings, "daily_budget_exhausted")
}

// Catches: one person reading another's conversation, and a viewer being
// refused what a viewer may read.
func TestConversationsBelongToTheirOwner(t *testing.T) {
	e := setup(t, assistant.Limits{})
	conv, err := e.assistant.CreateConversation(t.Context(), coretest.Marketer)
	require.NoError(t, err)
	_, err = e.assistant.Turns(t.Context(), coretest.Viewer, conv.ID)
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindNotFound, ce.Kind)
	_, err = e.assistant.Send(t.Context(), coretest.Viewer, conv.ID, "hi")
	require.ErrorAs(t, err, &ce)
	list, err := e.assistant.ListConversations(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	assert.Empty(t, list)
	list, err = e.assistant.ListConversations(t.Context(), coretest.Marketer)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	_, err = e.assistant.CreateConversation(t.Context(), core.Actor{})
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.assistant.Send(t.Context(), coretest.Marketer, conv.ID, "   ")
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindInvalid, ce.Kind)
}
