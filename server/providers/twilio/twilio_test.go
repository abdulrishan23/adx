package twilio_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/voice"
	"github.com/WokeWorks/adx/server/providers/twilio"
)

// Catches: a call created without the stream or the status callback, wrong
// auth, a recording requested when the policy said no, and a 5xx treated as
// a placed call.
func TestStartCallSendsStreamAndCallbacks(t *testing.T) {
	var form url.Values
	var auth, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		path = r.URL.Path
		require.NoError(t, r.ParseForm())
		form = r.PostForm
		if r.PostForm.Get("To") == "+971500000500" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sid":"CA123","status":"queued"}`))
	}))
	defer server.Close()
	c, err := twilio.New(twilio.Config{AccountSID: "AC1", AuthToken: "tok", PublicURL: "https://adx.example.com", BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)
	id, err := c.StartCall(t.Context(), voice.CallRequest{OccurrenceKey: "enq:1", To: "+971501234567", From: "+97180012345", StreamURL: "wss://adx.example.com/webhooks/voice/stream/1", StatusCallbackURL: "https://adx.example.com/webhooks/voice/status", Record: false})
	require.NoError(t, err)
	assert.Equal(t, "CA123", id)
	assert.Equal(t, "/2010-04-01/Accounts/AC1/Calls.json", path)
	assert.True(t, strings.HasPrefix(auth, "Basic "))
	assert.Equal(t, "+971501234567", form.Get("To"))
	assert.Contains(t, form.Get("Twiml"), `<Stream url="wss://adx.example.com/webhooks/voice/stream/1">`)
	assert.Contains(t, form.Get("Twiml"), `<Hangup/>`)
	assert.Equal(t, "https://adx.example.com/webhooks/voice/status", form.Get("StatusCallback"))
	assert.ElementsMatch(t, []string{"initiated", "ringing", "answered", "completed"}, form["StatusCallbackEvent"])
	assert.Empty(t, form.Get("Record"), "no recording unless asked")
	_, err = c.StartCall(t.Context(), voice.CallRequest{OccurrenceKey: "enq:2", To: "+971500000500", From: "+97180012345", Record: true})
	require.Error(t, err, "a 5xx is an error the caller treats as unknown")
	assert.Equal(t, "true", form.Get("Record"))
	_, err = twilio.New(twilio.Config{AccountSID: "AC1", AuthToken: "tok"})
	require.Error(t, err, "public URL needed for signatures")
}

// Catches: an unsigned or wrongly signed callback accepted, a signed one
// refused, and Twilio's status spellings mapped wrongly.
func TestStatusCallbackSignatureAndMapping(t *testing.T) {
	c, err := twilio.New(twilio.Config{AccountSID: "AC1", AuthToken: "tok", PublicURL: "https://adx.example.com"})
	require.NoError(t, err)
	form := url.Values{"CallSid": {"CA123"}, "CallStatus": {"in-progress"}, "SequenceNumber": {"2"}, "AccountSid": {"AC1"}}
	req := httptest.NewRequest("POST", "/webhooks/voice/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err = c.ParseStatusCallback(req)
	assert.ErrorIs(t, err, voice.ErrBadSignature)
	req = httptest.NewRequest("POST", "/webhooks/voice/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", "bogus")
	_, err = c.ParseStatusCallback(req)
	assert.ErrorIs(t, err, voice.ErrBadSignature)

	req = httptest.NewRequest("POST", "/webhooks/voice/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", c.Sign("https://adx.example.com/webhooks/voice/status", form))
	ev, err := c.ParseStatusCallback(req)
	require.NoError(t, err)
	assert.Equal(t, voice.CallInProgress, ev.State)
	assert.Equal(t, "CA123", ev.ProviderCallID)
	assert.Equal(t, "2", ev.EventID)

	// A different token signs differently, so a replay against another
	// account is refused.
	other, _ := twilio.New(twilio.Config{AccountSID: "AC1", AuthToken: "other", PublicURL: "https://adx.example.com"})
	req = httptest.NewRequest("POST", "/webhooks/voice/status", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", other.Sign("https://adx.example.com/webhooks/voice/status", form))
	_, err = c.ParseStatusCallback(req)
	assert.ErrorIs(t, err, voice.ErrBadSignature)

	done := url.Values{"CallSid": {"CA123"}, "CallStatus": {"no-answer"}, "CallDuration": {"0"}, "SequenceNumber": {"3"}}
	req = httptest.NewRequest("POST", "/webhooks/voice/status", strings.NewReader(done.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", c.Sign("https://adx.example.com/webhooks/voice/status", done))
	ev, err = c.ParseStatusCallback(req)
	require.NoError(t, err)
	assert.Equal(t, voice.CallNoAnswer, ev.State)
	rec := url.Values{"CallSid": {"CA123"}, "RecordingSid": {"RE9"}, "RecordingStatus": {"completed"}}
	req = httptest.NewRequest("POST", "/webhooks/voice/status", strings.NewReader(rec.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", c.Sign("https://adx.example.com/webhooks/voice/status", rec))
	ev, err = c.ParseStatusCallback(req)
	require.NoError(t, err)
	assert.Equal(t, "RE9", ev.RecordingRef)
}

// Catches: a 404 on lookup being an error, and a found call mapped wrongly.
func TestLookupAndHangup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/Calls/CAgone.json"):
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/Calls/CA1.json") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"sid":"CA1","status":"busy","duration":"0"}`))
		case strings.HasSuffix(r.URL.Path, "/Calls/CA1.json") && r.Method == http.MethodPost:
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "completed", r.PostForm.Get("Status"))
			_, _ = w.Write([]byte(`{"sid":"CA1","status":"completed"}`))
		case strings.HasSuffix(r.URL.Path, "/Recordings/RE1.json") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	c, _ := twilio.New(twilio.Config{AccountSID: "AC1", AuthToken: "tok", PublicURL: "https://adx.example.com", BaseURL: server.URL, HTTPClient: server.Client()})
	_, found, err := c.LookupCall(t.Context(), "CAgone")
	require.NoError(t, err)
	assert.False(t, found)
	ev, found, err := c.LookupCall(t.Context(), "CA1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, voice.CallBusy, ev.State)
	require.NoError(t, c.Hangup(t.Context(), "CA1"))
	require.NoError(t, c.DeleteRecording(t.Context(), "RE1"))
}
