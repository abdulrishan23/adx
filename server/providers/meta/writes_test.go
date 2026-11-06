package meta_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/execution"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/providers/meta"
)

// Catches: the token in the URL, a create without PAUSED or the marker, the
// budget in the wrong unit, special_ad_categories missing, a lookup without
// the name filter, a 5xx treated as a refusal, and a 4xx as uncertain.
func TestMetaWritesSendTheDocumentedShapes(t *testing.T) {
	var forms []url.Values
	var paths []string
	mode := "ok"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		assert.NotContains(t, r.URL.RawQuery, "access_token")
		raw, _ := io.ReadAll(r.Body)
		f, _ := url.ParseQuery(string(raw))
		forms = append(forms, f)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case mode == "down":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"unknown","code":1}}`))
		case mode == "refused":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid parameter","type":"OAuthException","code":100}}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/campaigns"):
			_, _ = w.Write([]byte(`{"id":"120330000000001"}`))
		case r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"success":true}`))
		case strings.HasSuffix(r.URL.Path, "/campaigns"):
			_, _ = w.Write([]byte(`{"data":[{"id":"120330000000001","name":"Wadi [adx:7]","status":"PAUSED","effective_status":"PAUSED"}]}`))
		default:
			_, _ = w.Write([]byte(`{"id":"120330000000001","status":"PAUSED","effective_status":"PAUSED"}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := meta.New(meta.Config{BaseURL: srv.URL})
	assert.ElementsMatch(t, []policies.WriteAction{policies.CampaignCreatePaus, policies.BudgetRaise, policies.BudgetLower, policies.CampaignPause, policies.CampaignResume}, c.Capabilities())

	res, err := c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "act_42", Action: policies.CampaignCreatePaus, Marker: "adx:7", Proposed: []byte(`{"name":"Wadi","dailyBudgetMinor":50000,"objective":"outcome_leads"}`)})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(paths[0], "/v26.0/act_42/campaigns"), paths[0])
	assert.Equal(t, "Wadi [adx:7]", forms[0].Get("name"))
	assert.Equal(t, "PAUSED", forms[0].Get("status"))
	assert.Equal(t, "OUTCOME_LEADS", forms[0].Get("objective"))
	assert.Equal(t, "50000", forms[0].Get("daily_budget"), "Meta budgets are minor units")
	assert.Equal(t, "[]", forms[0].Get("special_ad_categories"))
	require.Len(t, res.Refs, 1)
	assert.Equal(t, "120330000000001", res.Refs[0].ExternalID)

	_, err = c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "act_42", Action: policies.BudgetLower, TargetRef: "120330000000001", Proposed: []byte(`{"dailyBudgetMinor":40000}`)})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(paths[1], "/v26.0/120330000000001"))
	assert.Equal(t, "40000", forms[1].Get("daily_budget"))
	_, err = c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "act_42", Action: policies.CampaignPause, TargetRef: "120330000000001"})
	require.NoError(t, err)
	assert.Equal(t, "PAUSED", forms[2].Get("status"))
	_, err = c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "act_42", Action: policies.AdSetPause, TargetRef: "1"})
	require.Error(t, err, "ad sets are not claimed")

	found, err := c.Lookup(t.Context(), "tok", "act_42", "adx:7", policies.CampaignCreatePaus, "")
	require.NoError(t, err)
	assert.True(t, found.Found)
	assert.Contains(t, paths[len(paths)-1], "filtering=")
	assert.Contains(t, paths[len(paths)-1], url.QueryEscape("[adx:7]"))
	v, err := c.Verify(t.Context(), "tok", "act_42", []proposals.ProviderRef{{Kind: "campaign", ExternalID: "120330000000001"}})
	require.NoError(t, err)
	assert.True(t, v.Exists)
	assert.Equal(t, "PAUSED", v.Status)

	mode = "down"
	_, err = c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "act_42", Action: policies.CampaignPause, TargetRef: "120330000000001"})
	var ue *execution.UncertainError
	assert.True(t, errors.As(err, &ue))
	mode = "refused"
	_, err = c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "act_42", Action: policies.CampaignPause, TargetRef: "120330000000001"})
	require.Error(t, err)
	assert.False(t, errors.As(err, &ue))
}

// Catches: a resume sent as two requests (status, then budget), where a
// failure between them resumes the campaign at its old budget; a resume
// sent twice after a lost answer, because Lookup cannot see that the first
// one landed; the budget sent in anything but minor units; a status other
// than ACTIVE accepted; a timeout treated as a refusal rather than left
// uncertain.
func TestMetaResumeIsOnePostAndIsFoundOnceItLanded(t *testing.T) {
	status := "PAUSED"
	posts := 0
	var lastForm url.Values
	slow := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v26.0/120330000000001":
			if slow {
				select {
				case <-time.After(500 * time.Millisecond):
				case <-r.Context().Done():
				}
				return
			}
			posts++
			lastForm, _ = url.ParseQuery(string(raw))
			status = "ACTIVE"
			_, _ = w.Write([]byte(`{"success":true}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v26.0/120330000000001":
			_, _ = w.Write([]byte(`{"id":"120330000000001","status":"` + status + `","effective_status":"` + status + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"Unsupported get request.","type":"GraphMethodException","code":100,"error_subcode":33}}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := meta.New(meta.Config{BaseURL: srv.URL})

	found, err := c.Lookup(t.Context(), "tok", "act_42", "adx:9", policies.CampaignResume, "120330000000001")
	require.NoError(t, err)
	assert.False(t, found.Found)
	assert.Equal(t, "PAUSED", found.Status)
	assert.Equal(t, 0, posts)

	res, err := c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "act_42", Action: policies.CampaignResume, TargetRef: "120330000000001", Marker: "adx:9", Proposed: []byte(`{"status":"ACTIVE","dailyBudgetMinor":56000}`)})
	require.NoError(t, err)
	assert.Equal(t, 1, posts, "status and budget travel in one POST")
	assert.Equal(t, "ACTIVE", lastForm.Get("status"))
	assert.Equal(t, "56000", lastForm.Get("daily_budget"), "Meta budgets are minor units")
	require.Len(t, res.Refs, 1)
	assert.Equal(t, "120330000000001", res.Refs[0].ExternalID)
	assert.Contains(t, res.Detail, "56000")

	found, err = c.Lookup(t.Context(), "tok", "act_42", "adx:9", policies.CampaignResume, "120330000000001")
	require.NoError(t, err)
	assert.True(t, found.Found)
	assert.Equal(t, "ACTIVE", found.Status)
	require.Len(t, found.Refs, 1)
	assert.Equal(t, 1, posts, "a lookup sends no second resume")
	v, err := c.Verify(t.Context(), "tok", "act_42", res.Refs)
	require.NoError(t, err)
	assert.True(t, v.Exists)
	assert.Equal(t, "ACTIVE", v.Status)

	_, err = c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "act_42", Action: policies.CampaignResume, TargetRef: "120330000000001", Proposed: []byte(`{"status":"ACTIVE"}`)})
	require.NoError(t, err)
	assert.Empty(t, lastForm.Get("daily_budget"), "no budget in the intent, none sent")
	_, err = c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "act_42", Action: policies.CampaignResume, TargetRef: "120330000000001", Proposed: []byte(`{"status":"ARCHIVED"}`)})
	require.Error(t, err, "a resume only sets ACTIVE")
	assert.Equal(t, 2, posts)

	slow = true
	quick := meta.New(meta.Config{BaseURL: srv.URL, HTTPClient: &http.Client{Timeout: 50 * time.Millisecond}})
	_, err = quick.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "act_42", Action: policies.CampaignResume, TargetRef: "120330000000001", Proposed: []byte(`{"status":"ACTIVE","dailyBudgetMinor":56000}`)})
	var ue *execution.UncertainError
	assert.True(t, errors.As(err, &ue), "a timeout leaves the intent uncertain: %v", err)
}
