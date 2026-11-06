package whatsapp_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/conversations"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/providers/whatsapp"
)

// Catches: the request not carrying the token, the recipient keeping its
// plus, a non-text payload, and the message id not coming back.
func TestSendPostsATextMessage(t *testing.T) {
	var got map[string]any
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &got))
		_, _ = w.Write([]byte(`{"messaging_product":"whatsapp","messages":[{"id":"wamid.ABC"}]}`))
	}))
	defer srv.Close()
	c, err := whatsapp.New(whatsapp.Config{AccessToken: "tok-1", PhoneNumberID: "555", BaseURL: srv.URL, HTTPClient: srv.Client()})
	require.NoError(t, err)

	ref, err := c.Send(t.Context(), outreach.OutboundMessage{Channel: "whatsapp", To: "+971501234567", Body: "Salam Ali", IdempotencyKey: "k1"})
	require.NoError(t, err)
	assert.Equal(t, "wamid.ABC", ref)
	assert.Equal(t, "Bearer tok-1", auth)
	assert.Equal(t, "/555/messages", path)
	assert.Equal(t, "971501234567", got["to"])
	assert.Equal(t, "text", got["type"])
	assert.Equal(t, "whatsapp", got["messaging_product"])
	assert.Equal(t, "Salam Ali", got["text"].(map[string]any)["body"])
}

// Catches: a Graph error reported as success, the token leaking into the
// error, a wrong channel or a non-numeric recipient reaching the API.
func TestSendRefusalsAndErrors(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"(#131047) Re-engagement message","type":"OAuthException","code":131047,"error_data":{"details":"Message failed to send because more than 24 hours have passed since the customer last replied to this number."}}}`))
	}))
	defer srv.Close()
	c, err := whatsapp.New(whatsapp.Config{AccessToken: "secret-token", PhoneNumberID: "555", BaseURL: srv.URL, HTTPClient: srv.Client()})
	require.NoError(t, err)

	_, err = c.Send(t.Context(), outreach.OutboundMessage{Channel: "whatsapp", To: "+971501234567", Body: "hi"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "131047")
	assert.Contains(t, err.Error(), "24 hours")
	assert.NotContains(t, err.Error(), "secret-token")

	_, err = c.Send(t.Context(), outreach.OutboundMessage{Channel: "email", To: "a@b.c", Body: "hi"})
	require.Error(t, err)
	_, err = c.Send(t.Context(), outreach.OutboundMessage{Channel: "whatsapp", To: "not a number", Body: "hi"})
	require.Error(t, err)
	_, err = c.Send(t.Context(), outreach.OutboundMessage{Channel: "whatsapp", To: "+971501234567", Body: "  "})
	require.Error(t, err)
	assert.Equal(t, 1, calls, "refused inputs never reach the API")

	_, err = whatsapp.New(whatsapp.Config{AccessToken: "x"})
	require.Error(t, err)
}

// Catches: a delivery parsed into the wrong number or body, a picture
// passed off as text, statuses and errors lost, and a non-WhatsApp object
// accepted.
func TestParseWebhookReadsMessagesAndStatuses(t *testing.T) {
	body := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"1","changes":[{"field":"messages","value":{"messaging_product":"whatsapp","metadata":{"phone_number_id":"555"},
	"contacts":[{"profile":{"name":"Fatima"},"wa_id":"971501234567"}],
	"messages":[{"from":"971501234567","id":"wamid.1","timestamp":"1792000000","type":"text","text":{"body":"مرحبا"}},
	            {"from":"971501234567","id":"wamid.2","timestamp":"1792000001","type":"image","image":{"id":"m1"}},
	            {"from":"971501234567","id":"wamid.3","timestamp":"1792000002","type":"interactive","interactive":{"type":"button_reply","button_reply":{"id":"b1","title":"Yes please"}}}],
	"statuses":[{"id":"wamid.out.1","status":"failed","timestamp":"1792000003","recipient_id":"971501234567","errors":[{"code":131047,"title":"Re-engagement message","message":"More than 24 hours"}]},
	            {"id":"wamid.out.2","status":"read","timestamp":"1792000004","recipient_id":"971501234567"}]}},
	{"field":"account_update","value":{}}]}]}`)
	ev, err := whatsapp.ParseWebhook(body)
	require.NoError(t, err)
	assert.Equal(t, "555", ev.PhoneNumberID)
	require.Len(t, ev.Messages, 3)
	assert.Equal(t, "+971501234567", ev.Messages[0].From)
	assert.Equal(t, "Fatima", ev.Messages[0].ProfileName)
	assert.Equal(t, "text", ev.Messages[0].Kind)
	assert.Equal(t, "مرحبا", ev.Messages[0].Body)
	assert.Equal(t, int64(1792000000), ev.Messages[0].ReceivedAt.Unix())
	assert.Equal(t, "unsupported", ev.Messages[1].Kind)
	assert.Equal(t, "image", ev.Messages[1].Unsupported)
	assert.Equal(t, "Yes please", ev.Messages[2].Body)
	require.Len(t, ev.Statuses, 2)
	assert.Equal(t, "failed", ev.Statuses[0].Status)
	assert.Contains(t, ev.Statuses[0].Error, "131047")
	assert.Equal(t, "read", ev.Statuses[1].Status)
	assert.Equal(t, []string{"field account_update"}, ev.Ignored)

	_, err = whatsapp.ParseWebhook([]byte(`{"object":"page","entry":[]}`))
	require.Error(t, err)

	assert.True(t, whatsapp.VerifySignature("s3", body, whatsapp.Sign("s3", body)))
	assert.False(t, whatsapp.VerifySignature("s3", body, whatsapp.Sign("s4", body)))
	assert.False(t, whatsapp.VerifySignature("", body, whatsapp.Sign("", body)), "no secret verifies nothing")
	ch, ok := whatsapp.VerifyChallenge("tok", map[string]string{"hub.mode": "subscribe", "hub.verify_token": "tok", "hub.challenge": "42"})
	assert.True(t, ok)
	assert.Equal(t, "42", ch)
	_, ok = whatsapp.VerifyChallenge("tok", map[string]string{"hub.mode": "subscribe", "hub.verify_token": "nope", "hub.challenge": "42"})
	assert.False(t, ok)
}

// Catches: a template sent as text, the language dropped, and a template
// with no name reaching the API.
func TestSendTemplatePostsATemplate(t *testing.T) {
	var got map[string]any
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &got))
		_, _ = w.Write([]byte(`{"messaging_product":"whatsapp","messages":[{"id":"wamid.T"}]}`))
	}))
	defer srv.Close()
	c, err := whatsapp.New(whatsapp.Config{AccessToken: "tok-1", PhoneNumberID: "555", BaseURL: srv.URL, HTTPClient: srv.Client()})
	require.NoError(t, err)
	ref, err := c.SendTemplate(t.Context(), conversations.TemplateMessage{To: "+971501234567", Name: "wadi_follow_up", Language: "ar", IdempotencyKey: "k"})
	require.NoError(t, err)
	assert.Equal(t, "wamid.T", ref)
	assert.Equal(t, "template", got["type"])
	tpl := got["template"].(map[string]any)
	assert.Equal(t, "wadi_follow_up", tpl["name"])
	assert.Equal(t, "ar", tpl["language"].(map[string]any)["code"])
	_, err = c.SendTemplate(t.Context(), conversations.TemplateMessage{To: "+971501234567"})
	require.Error(t, err)
	assert.Equal(t, 1, calls)
}
