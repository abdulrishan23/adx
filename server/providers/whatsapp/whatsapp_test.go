package whatsapp_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
