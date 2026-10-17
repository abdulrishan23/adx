// Package twilio is a reference telecom transport for core/voice. It places
// calls that connect a media stream to ADX, parses and verifies Twilio's
// status callbacks, looks a call up for reconciliation and deletes
// recordings. It is wired only when the owner names Twilio as the
// authorized route; the UAE route is not chosen (BD-45).
package twilio

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // Twilio's request signature is HMAC-SHA1 by their specification.
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core/voice"
)

const defaultBaseURL = "https://api.twilio.com"

type Config struct {
	AccountSID string
	AuthToken  string
	// PublicURL is the origin Twilio calls back to, used to rebuild the
	// signed URL behind the reverse proxy.
	PublicURL  string
	BaseURL    string
	HTTPClient *http.Client
}

type Client struct {
	sid, token, public, base string
	http                     *http.Client
}

func New(cfg Config) (*Client, error) {
	if cfg.AccountSID == "" || cfg.AuthToken == "" {
		return nil, errors.New("twilio account SID and auth token are required")
	}
	if cfg.PublicURL == "" {
		return nil, errors.New("twilio needs ADX_PUBLIC_URL to verify callback signatures")
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	h := cfg.HTTPClient
	if h == nil {
		h = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{sid: cfg.AccountSID, token: cfg.AuthToken, public: strings.TrimRight(cfg.PublicURL, "/"), base: strings.TrimRight(base, "/"), http: h}, nil
}

func (c *Client) Name() string { return "twilio" }

// StartCall creates an outbound call whose TwiML connects a bidirectional
// media stream to ADX and hangs up when the stream ends. Status callbacks
// go to the request's URL for every lifecycle event. A transport error or a
// 5xx is returned as is: the caller treats it as unknown and reconciles.
func (c *Client) StartCall(ctx context.Context, req voice.CallRequest) (string, error) {
	twiml := fmt.Sprintf(`<Response><Connect><Stream url="%s"><Parameter name="occurrence" value="%s"/></Stream></Connect><Hangup/></Response>`, html.EscapeString(req.StreamURL), html.EscapeString(req.OccurrenceKey))
	form := url.Values{"To": {req.To}, "From": {req.From}, "Twiml": {twiml}, "StatusCallback": {req.StatusCallbackURL}, "StatusCallbackMethod": {"POST"}, "StatusCallbackEvent": {"initiated", "ringing", "answered", "completed"}}
	if req.Record {
		form.Set("Record", "true")
		form.Set("RecordingChannels", "dual")
		form.Set("RecordingStatusCallback", req.StatusCallbackURL)
	}
	var out struct {
		Sid    string `json:"sid"`
		Status string `json:"status"`
	}
	if err := c.do(ctx, http.MethodPost, "/2010-04-01/Accounts/"+c.sid+"/Calls.json", form, &out); err != nil {
		return "", err
	}
	if out.Sid == "" {
		return "", errors.New("twilio returned no call SID")
	}
	return out.Sid, nil
}

func (c *Client) Hangup(ctx context.Context, providerCallID string) error {
	return c.do(ctx, http.MethodPost, "/2010-04-01/Accounts/"+c.sid+"/Calls/"+url.PathEscape(providerCallID)+".json", url.Values{"Status": {"completed"}}, nil)
}

// LookupCall reads a call back. A 404 is "not found", not an error.
func (c *Client) LookupCall(ctx context.Context, providerCallID string) (voice.StatusEvent, bool, error) {
	var out struct {
		Sid      string `json:"sid"`
		Status   string `json:"status"`
		Duration string `json:"duration"`
	}
	err := c.do(ctx, http.MethodGet, "/2010-04-01/Accounts/"+c.sid+"/Calls/"+url.PathEscape(providerCallID)+".json", nil, &out)
	if errors.Is(err, errNotFound) {
		return voice.StatusEvent{}, false, nil
	}
	if err != nil {
		return voice.StatusEvent{}, false, err
	}
	state, ok := mapStatus(out.Status)
	if !ok {
		return voice.StatusEvent{}, false, fmt.Errorf("twilio reported an unknown status %q", out.Status)
	}
	ev := voice.StatusEvent{ProviderCallID: out.Sid, State: state, At: time.Now().UTC()}
	if n, err := strconv.Atoi(out.Duration); err == nil {
		ev.Duration = time.Duration(n) * time.Second
	}
	return ev, true, nil
}

func (c *Client) DeleteRecording(ctx context.Context, recordingRef string) error {
	err := c.do(ctx, http.MethodDelete, "/2010-04-01/Accounts/"+c.sid+"/Recordings/"+url.PathEscape(recordingRef)+".json", nil, nil)
	if errors.Is(err, errNotFound) {
		return nil
	}
	return err
}

// ParseStatusCallback verifies X-Twilio-Signature (HMAC-SHA1 over the full
// public URL plus the sorted POST parameters, base64) and decodes the call
// status. The URL is rebuilt from the configured public origin, since the
// request reaches ADX through a proxy.
func (c *Client) ParseStatusCallback(r *http.Request) (voice.StatusEvent, error) {
	if err := r.ParseForm(); err != nil {
		return voice.StatusEvent{}, voice.ErrBadSignature
	}
	expected := c.Sign(c.public+r.URL.RequestURI(), r.PostForm)
	given := r.Header.Get("X-Twilio-Signature")
	if given == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(given)) != 1 {
		return voice.StatusEvent{}, voice.ErrBadSignature
	}
	state, ok := mapStatus(r.PostForm.Get("CallStatus"))
	if !ok {
		// A recording callback carries RecordingStatus instead of CallStatus.
		if r.PostForm.Get("RecordingSid") != "" && r.PostForm.Get("RecordingStatus") == "completed" {
			return voice.StatusEvent{ProviderCallID: r.PostForm.Get("CallSid"), State: voice.CallCompleted, RecordingRef: r.PostForm.Get("RecordingSid"), At: time.Now().UTC(), EventID: "recording:" + r.PostForm.Get("RecordingSid")}, nil
		}
		return voice.StatusEvent{}, fmt.Errorf("twilio callback has unknown CallStatus %q", r.PostForm.Get("CallStatus"))
	}
	ev := voice.StatusEvent{ProviderCallID: r.PostForm.Get("CallSid"), State: state, RecordingRef: r.PostForm.Get("RecordingSid"), At: time.Now().UTC(), EventID: r.PostForm.Get("SequenceNumber")}
	if n, err := strconv.Atoi(r.PostForm.Get("CallDuration")); err == nil {
		ev.Duration = time.Duration(n) * time.Second
	}
	if ts := r.PostForm.Get("Timestamp"); ts != "" {
		if t, err := time.Parse(time.RFC1123Z, ts); err == nil {
			ev.At = t.UTC()
		}
	}
	return ev, nil
}

// Sign computes Twilio's request signature, exported for tests and for
// anyone replaying a callback.
func (c *Client) Sign(fullURL string, form url.Values) string {
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(fullURL)
	for _, k := range keys {
		vals := append([]string(nil), form[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			b.WriteString(k)
			b.WriteString(v)
		}
	}
	mac := hmac.New(sha1.New, []byte(c.token))
	mac.Write([]byte(b.String()))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func mapStatus(s string) (voice.CallState, bool) {
	switch s {
	case "queued", "initiated":
		return voice.CallQueued, true
	case "ringing":
		return voice.CallRinging, true
	case "in-progress", "answered":
		return voice.CallInProgress, true
	case "completed":
		return voice.CallCompleted, true
	case "busy":
		return voice.CallBusy, true
	case "failed":
		return voice.CallFailed, true
	case "no-answer":
		return voice.CallNoAnswer, true
	case "canceled":
		return voice.CallCanceled, true
	}
	return "", false
}

var errNotFound = errors.New("not found")

func (c *Client) do(ctx context.Context, method, path string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.sid, c.token)
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("twilio request failed: %w", err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		return errNotFound
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return errors.New("twilio rejected the credentials")
	case res.StatusCode >= 500:
		return fmt.Errorf("twilio returned HTTP %d", res.StatusCode)
	case res.StatusCode < 200 || res.StatusCode >= 300:
		// Bodies can name numbers and account details; keep the code only.
		return fmt.Errorf("twilio returned HTTP %d", res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}

var (
	_ voice.Telephony  = (*Client)(nil)
	_ voice.Reconciler = (*Client)(nil)
	_ voice.Deleter    = (*Client)(nil)
)
