package meta_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/providers/meta"
)

// Catches: the token in the URL, insights not asked per day at campaign
// level, string numbers read as zero, spend (decimal major units) and budgets
// (minor-unit strings) converted the same way, lead actions double counted,
// and paging.next not followed.
func TestInsightsAndCampaigns(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		assert.Empty(t, r.URL.Query().Get("access_token"), "the token never travels in the URL")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-business-use-case-usage", `{"42":[{"type":"ads_insights","call_count":12,"total_cputime":3,"total_time":4,"estimated_time_to_regain_access":0,"ads_api_access_tier":"standard_access"}]}`)
		switch r.URL.Path {
		case "/v26.0/act_42/insights":
			q := r.URL.Query()
			assert.Equal(t, "campaign", q.Get("level"))
			assert.Equal(t, "1", q.Get("time_increment"))
			assert.Equal(t, `{"since":"2026-10-14","until":"2026-10-15"}`, q.Get("time_range"))
			assert.Contains(t, q.Get("fields"), "actions")
			if q.Get("after") == "" {
				_, _ = w.Write([]byte(`{"data":[
					{"campaign_id":"120210","date_start":"2026-10-14","date_stop":"2026-10-14","spend":"5339.5","impressions":"361324","clicks":"4120","reach":"210000","account_currency":"AED",
					 "actions":[{"action_type":"lead","value":"37"},{"action_type":"onsite_conversion.lead_grouped","value":"30"},{"action_type":"link_click","value":"4000"}]},
					{"campaign_id":"120211","date_start":"2026-10-14","date_stop":"2026-10-14","spend":"0.01","impressions":"3","clicks":"0","account_currency":"AED"}
				],"paging":{"cursors":{"before":"a","after":"b"},"next":"` + "http://" + r.Host + `/v26.0/act_42/insights?after=b&level=campaign&time_increment=1&time_range=%7B%22since%22%3A%222026-10-14%22%2C%22until%22%3A%222026-10-15%22%7D&fields=actions"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"campaign_id":"120210","date_start":"2026-10-15","date_stop":"2026-10-15","spend":"100","impressions":"1","clicks":"1","account_currency":"AED"}],"paging":{"cursors":{"before":"c","after":"d"}}}`))
		case "/v26.0/act_42/campaigns":
			assert.Contains(t, r.URL.Query().Get("fields"), "daily_budget")
			_, _ = w.Write([]byte(`{"data":[
				{"id":"120210","name":"Wadi Leads","status":"ACTIVE","effective_status":"ACTIVE","objective":"OUTCOME_LEADS","daily_budget":"50000"},
				{"id":"120211","name":"Oceana Traffic","status":"ACTIVE","effective_status":"CAMPAIGN_PAUSED","objective":"OUTCOME_TRAFFIC","lifetime_budget":"2000000"},
				{"id":"120212","name":"Adset budgets","status":"ACTIVE","effective_status":"ACTIVE","objective":"OUTCOME_LEADS"}
			],"paging":{"cursors":{"before":"x","after":"y"}}}`))
		case "/v26.0/me/adaccounts":
			_, _ = w.Write([]byte(`{"data":[{"id":"act_42","account_id":"42","name":"Reportage","currency":"AED","timezone_name":"Asia/Dubai","account_status":1,"business":{"id":"b1","name":"Reportage LLC"}}],"paging":{"cursors":{}}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	client := meta.New(meta.Config{BaseURL: srv.URL, HTTPClient: srv.Client()})

	accounts, err := client.ListAccounts(t.Context(), "tok")
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	assert.Equal(t, "act_42", accounts[0].ExternalID)
	assert.Equal(t, "ACTIVE", accounts[0].Status)
	assert.Equal(t, "b1", accounts[0].ParentID)

	campaigns, err := client.ListCampaigns(t.Context(), "tok", "42", "")
	require.NoError(t, err)
	require.Len(t, campaigns.Items, 3)
	assert.Empty(t, campaigns.Next)
	assert.Equal(t, int64(50000), *campaigns.Items[0].BudgetMinor, "daily_budget is already minor units")
	assert.Equal(t, "daily", campaigns.Items[0].BudgetPeriod)
	assert.Equal(t, "CAMPAIGN_PAUSED", campaigns.Items[1].Status)
	assert.Equal(t, int64(2000000), *campaigns.Items[1].BudgetMinor)
	assert.Equal(t, "lifetime", campaigns.Items[1].BudgetPeriod)
	assert.Nil(t, campaigns.Items[2].BudgetMinor, "ad-set budgets mean no campaign budget, not zero")

	page, err := client.DailyMetrics(t.Context(), "tok", "act_42", "2026-10-14", "2026-10-15", "")
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.NotEmpty(t, page.Next)
	m := page.Items[0]
	assert.Equal(t, int64(533950), m.CostMinor, "5339.5 AED = 533950 fils")
	assert.Equal(t, int64(361324), m.Impressions)
	assert.Equal(t, int64(4120), m.Clicks)
	assert.Equal(t, int64(210000), *m.Reach)
	assert.Equal(t, 37.0, *m.PlatformLeads, "only the `lead` total counts; grouped types would double count")
	assert.Nil(t, m.Conversions, "Meta's leads are not Google-style conversions")
	assert.Equal(t, int64(1), page.Items[1].CostMinor, "0.01 AED = 1 fils")
	assert.Nil(t, page.Items[1].PlatformLeads)
	assert.Nil(t, page.Items[1].Reach)

	second, err := client.DailyMetrics(t.Context(), "tok", "act_42", "2026-10-14", "2026-10-15", page.Next)
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Empty(t, second.Next, "no paging.next means the last page")
	assert.Equal(t, int64(10000), second.Items[0].CostMinor)

	_, err = client.DailyMetrics(t.Context(), "tok", "act_42", "2026-10-14", "2026-10-15", "https://evil.example/steal?x=1")
	require.Error(t, err, "a cursor to another host is refused")
	_, err = client.ListCampaigns(t.Context(), "tok", "act_abc", "")
	require.Error(t, err)
}

// Catches: a throttle without a wait, an expired token shown as a generic
// error, and Meta's message or trace id reaching users.
func TestErrorMapping(t *testing.T) {
	status, body, usage := http.StatusBadRequest, `{"error":{"message":"Error validating access token: Session has expired","type":"OAuthException","code":190,"error_subcode":463,"fbtrace_id":"AbCdEf-secret"}}`, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if usage != "" {
			w.Header().Set("x-business-use-case-usage", usage)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	client := meta.New(meta.Config{BaseURL: srv.URL, HTTPClient: srv.Client()})

	_, err := client.ListCampaigns(t.Context(), "tok", "act_42", "")
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.Contains(t, ce.Message, "reconnect")
	assert.NotContains(t, ce.Message, "AbCdEf-secret")

	status, body, usage = http.StatusBadRequest, `{"error":{"message":"User request limit reached","type":"OAuthException","code":17,"fbtrace_id":"x"}}`,
		`{"42":[{"type":"ads_insights","call_count":100,"total_cputime":100,"total_time":100,"estimated_time_to_regain_access":12,"ads_api_access_tier":"standard_access"}]}`
	_, err = client.ListCampaigns(t.Context(), "tok", "act_42", "")
	require.ErrorAs(t, err, &ce)
	assert.Contains(t, ce.Message, "rate limit")
	var th meta.ThrottledError
	require.ErrorAs(t, err, &th)
	assert.Equal(t, "12m0s", th.Wait.String())

	status, body, usage = http.StatusForbidden, `{"error":{"message":"(#200) Requires ads_read","type":"OAuthException","code":200}}`, ""
	_, err = client.ListCampaigns(t.Context(), "tok", "act_42", "")
	require.ErrorAs(t, err, &ce)
	assert.Contains(t, ce.Message, "code 200")

	status, body = http.StatusBadRequest, `{"error":{"message":"Please reduce the amount of data","code":100,"error_subcode":1487534}}`
	_, err = client.DailyMetrics(t.Context(), "tok", "act_42", "2026-01-01", "2026-10-01", "")
	require.ErrorAs(t, err, &ce)
	assert.Contains(t, ce.Message, "too large")

	status, body = http.StatusInternalServerError, `{"error":{"message":"An unknown error occurred","code":1,"is_transient":true}}`
	_, err = client.ListCampaigns(t.Context(), "tok", "act_42", "")
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindUnavailable, ce.Kind)

	h := http.Header{}
	h.Set("x-business-use-case-usage", `{"42":[{"type":"ads_insights","call_count":12,"total_cputime":3,"total_time":4,"estimated_time_to_regain_access":0,"ads_api_access_tier":"standard_access"}]}`)
	u, ok := meta.ParseUsage(h, "42")
	require.True(t, ok)
	assert.Equal(t, 12, u.CallCount)
	assert.Equal(t, "standard_access", u.AccessTier)
}
