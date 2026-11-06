package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/assistant/assistanttest"
	"github.com/WokeWorks/adx/server/core/conversations"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/providers/whatsapp"
	"github.com/WokeWorks/adx/server/store/storetest"
)

const appSecret = "app-secret-1"

func conversationsServer(t *testing.T, model *assistanttest.Model) *httptest.Server {
	t.Helper()
	db := storetest.Open(t)
	coretest.SeedUsers(t, db)
	clock := coretest.NewClock(time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC))
	ev := evidence.New(db, clock)
	c := crm.New(db, ev, crm.Config{Clock: clock})
	sig := signals.New(db, ev, c, signals.Config{Clock: clock})
	out := outreach.New(db, ev, outreach.Config{Clock: clock, Signals: sig, CRM: c})
	conv := conversations.New(db, ev, conversations.Config{Clock: clock, Model: model, Outreach: out, Signals: sig, CRM: c})
	srv := httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Dev: true, DevUser: &users.User{ID: 1}, Conversations: conv, CRM: c, Outreach: out,
		WhatsAppWebhook: api.WhatsAppWebhookConfig{AppSecret: appSecret, VerifyToken: "verify-me", PhoneNumberID: "555"}}))
	t.Cleanup(srv.Close)
	return srv
}

const delivery = `{"object":"whatsapp_business_account","entry":[{"id":"1","changes":[{"field":"messages","value":{"messaging_product":"whatsapp","metadata":{"display_phone_number":"97180012345","phone_number_id":"555"},"contacts":[{"profile":{"name":"Fatima"},"wa_id":"971501234567"}],"messages":[{"from":"971501234567","id":"wamid.HBgL1","timestamp":"1792000000","type":"text","text":{"body":"Hi, the Wadi price?"}}]}}]}]}`

func postWebhook(t *testing.T, srv *httptest.Server, body, signature string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/webhooks/whatsapp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("X-Hub-Signature-256", signature)
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	raw, _ := io.ReadAll(res.Body)
	return res, string(raw)
}

// Catches: an unsigned or wrongly signed delivery being recorded, Meta's
// verification answered without the token, a repeated delivery making a
// second turn, and a message for another Business number landing here.
func TestWhatsAppWebhookChecksSignatureAndVerifyToken(t *testing.T) {
	model := &assistanttest.Model{Responses: []assistant.Response{assistanttest.ToolCall("t1", "reply", conversations.ReplyDecision{Reply: "Hello Fatima, happy to help. Is it to live in or to invest?", Language: "en"})}}
	srv := conversationsServer(t, model)

	res, err := http.Get(srv.URL + "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=wrong&hub.challenge=123")
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
	res, err = http.Get(srv.URL + "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=verify-me&hub.challenge=123")
	require.NoError(t, err)
	raw, _ := io.ReadAll(res.Body)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "123", string(raw))

	res, _ = postWebhook(t, srv, delivery, "")
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
	res, _ = postWebhook(t, srv, delivery, whatsapp.Sign("other-secret", []byte(delivery)))
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
	list, _ := get(t, srv, "/api/conversations", nil, nil)
	assert.Equal(t, http.StatusOK, list.StatusCode)
	assert.Empty(t, model.Requests, "nothing reached the agent")

	res, body := postWebhook(t, srv, delivery, whatsapp.Sign(appSecret, []byte(delivery)))
	assert.Equal(t, http.StatusOK, res.StatusCode)
	var out api.WhatsAppWebhookResponse
	require.NoError(t, json.Unmarshal([]byte(body), &out))
	assert.Equal(t, 1, out.Messages)
	assert.Equal(t, 1, out.Replies)
	assert.Equal(t, 0, out.Duplicate)

	res, body = postWebhook(t, srv, delivery, whatsapp.Sign(appSecret, []byte(delivery)))
	assert.Equal(t, http.StatusOK, res.StatusCode)
	require.NoError(t, json.Unmarshal([]byte(body), &out))
	assert.Equal(t, 1, out.Duplicate)
	assert.Equal(t, 0, out.Replies)

	other := strings.Replace(delivery, `"phone_number_id":"555"`, `"phone_number_id":"999"`, 1)
	res, body = postWebhook(t, srv, other, whatsapp.Sign(appSecret, []byte(other)))
	assert.Equal(t, http.StatusAccepted, res.StatusCode)
	assert.Contains(t, body, "not this server's")

	res, body = get(t, srv, "/api/conversations", nil, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	var threads api.ThreadsResponse
	require.NoError(t, json.Unmarshal([]byte(body), &threads))
	require.Len(t, threads.Conversations, 1)
	assert.Equal(t, "Fatima", threads.Conversations[0].PersonName)
	assert.Equal(t, "+971501234567", threads.Conversations[0].Address)
	assert.Equal(t, 1, threads.Conversations[0].Pending, "the reply is held: outreach is not configured")

	detail, body := get(t, srv, "/api/conversations/1", nil, nil)
	assert.Equal(t, http.StatusOK, detail.StatusCode)
	assert.Contains(t, body, `"state":"held"`)
	assert.Contains(t, body, "not configured")
	res, _ = get(t, srv, "/api/conversations/1/instructions", nil, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	res, _ = get(t, srv, "/api/conversations/readiness", nil, nil)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

// Catches: the dev simulate endpoint mounted outside dev, and a person's
// own message on a thread not reaching the turn list.
func TestSimulateIsDevOnly(t *testing.T) {
	srv := conversationsServer(t, &assistanttest.Model{})
	res := postJSON(t, srv.URL+"/api/conversations/simulate", conversations.InboundMessage{ProviderRef: "wamid.sim.1", From: "+971501234567", ProfileName: "Fatima", Body: "talk to someone"})
	assert.Equal(t, http.StatusCreated, res.StatusCode)
	res = postJSON(t, srv.URL+"/api/conversations/1/messages", api.ConversationMessageInput{Body: "Hi Fatima, Sara here."})
	assert.Equal(t, http.StatusCreated, res.StatusCode)
	_, body := get(t, srv, "/api/conversations/1", nil, nil)
	assert.Contains(t, body, "Sara here")
	assert.Contains(t, body, `"state":"person"`)

	prod := httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Conversations: conversations.New(storetest.Open(t), nil, conversations.Config{})}))
	t.Cleanup(prod.Close)
	res = postJSON(t, prod.URL+"/api/conversations/simulate", conversations.InboundMessage{ProviderRef: "x", From: "+971501234567", Body: "hi"})
	assert.NotEqual(t, http.StatusCreated, res.StatusCode)
	res, err := http.Get(prod.URL + "/webhooks/whatsapp?hub.mode=subscribe&hub.verify_token=x&hub.challenge=1")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, res.StatusCode, "no app secret, no webhook")
}
