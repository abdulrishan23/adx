package googleads_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/providers/googleads"
)

type captured struct {
	path, login, auth, devToken string
	query                       string
}

func server(t *testing.T, handler func(c captured, w http.ResponseWriter)) (*httptest.Server, *[]captured) {
	t.Helper()
	var calls []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := captured{path: r.URL.Path, login: r.Header.Get("login-customer-id"), auth: r.Header.Get("Authorization"), devToken: r.Header.Get("developer-token")}
		if r.Method == http.MethodPost {
			var body map[string]string
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			c.query = body["query"]
		}
		calls = append(calls, c)
		w.Header().Set("Content-Type", "application/json")
		handler(c, w)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// Catches: sending the sunset developer token, missing login-customer-id
// through a manager, not decoding the searchStream batch array, micros not
// converted to minor units, int64 strings read as zero, and null
// conversions becoming zero.
func TestCampaignsAndDailyMetrics(t *testing.T) {
	srv, calls := server(t, func(c captured, w http.ResponseWriter) {
		assert.Equal(t, "/v26/customers/1234567890/googleAds:searchStream", c.path)
		assert.Equal(t, "Bearer tok", c.auth)
		assert.Equal(t, "9876543210", c.login)
		assert.Empty(t, c.devToken)
		switch {
		case strings.HasPrefix(c.query, "SELECT campaign.id, campaign.name"):
			assert.Contains(t, c.query, "campaign_budget.amount_micros")
			_, _ = w.Write([]byte(`[{"results":[
				{"campaign":{"id":"111","name":"Wadi Search","status":"ENABLED","advertisingChannelType":"SEARCH"},"campaignBudget":{"amountMicros":"500000000","period":"DAILY"},"customer":{"currencyCode":"AED"}},
				{"campaign":{"id":"222","name":"Oceana PMax","status":"PAUSED","advertisingChannelType":"PERFORMANCE_MAX"},"customer":{"currencyCode":"AED"}}
			],"fieldMask":"campaign.id","requestId":"r1"},{"results":[
				{"campaign":{"id":"333","name":"Verdana Video","status":"ENABLED","advertisingChannelType":"VIDEO"},"campaignBudget":{"amountMicros":"1234567","period":"DAILY"},"customer":{"currencyCode":"AED"}}
			],"requestId":"r2"}]`))
		case strings.HasPrefix(c.query, "SELECT segments.date"):
			assert.Contains(t, c.query, "segments.date BETWEEN '2026-10-14' AND '2026-10-15'")
			_, _ = w.Write([]byte(`[{"results":[
				{"segments":{"date":"2026-10-14"},"campaign":{"id":"111"},"customer":{"currencyCode":"AED"},"metrics":{"costMicros":"123456789","impressions":"10000","clicks":"250","conversions":3.5,"conversionsValue":1200.5,"allConversions":4}},
				{"segments":{"date":"2026-10-14"},"campaign":{"id":"222"},"customer":{"currencyCode":"AED"},"metrics":{"costMicros":"0","impressions":"5","clicks":"0"}}
			]}]`))
		default:
			t.Errorf("unexpected query %q", c.query)
		}
	})
	client := googleads.New(googleads.Config{BaseURL: srv.URL, HTTPClient: srv.Client(), LoginCustomerID: "987-654-3210"})

	campaigns, err := client.ListCampaigns(t.Context(), "tok", "1234567890", "")
	require.NoError(t, err)
	require.Len(t, campaigns.Items, 3, "both batches are read")
	assert.Empty(t, campaigns.Next)
	assert.Equal(t, "Wadi Search", campaigns.Items[0].Name)
	assert.Equal(t, int64(50000), *campaigns.Items[0].BudgetMinor, "500 AED daily = 50000 fils")
	assert.Equal(t, "daily", campaigns.Items[0].BudgetPeriod)
	assert.Nil(t, campaigns.Items[1].BudgetMinor)
	assert.Equal(t, int64(123), *campaigns.Items[2].BudgetMinor, "1.234567 AED rounds to 123 fils")

	metrics, err := client.DailyMetrics(t.Context(), "tok", "1234567890", "2026-10-14", "2026-10-15", "")
	require.NoError(t, err)
	require.Len(t, metrics.Items, 2)
	m := metrics.Items[0]
	assert.Equal(t, "111", m.CampaignExternalID)
	assert.Equal(t, int64(12346), m.CostMinor, "123.456789 AED rounds to 12346 fils")
	assert.Equal(t, int64(10000), m.Impressions)
	assert.Equal(t, int64(250), m.Clicks)
	assert.Equal(t, 3.5, *m.Conversions)
	assert.Equal(t, int64(120050), *m.ConversionsValue)
	assert.Equal(t, 4.0, *m.AllConversions)
	assert.Nil(t, m.PlatformLeads, "Google doesn't separate leads; stays null")
	assert.Nil(t, metrics.Items[1].Conversions, "absent metric is null, not zero")
	assert.Len(t, *calls, 2)

	_, err = client.DailyMetrics(t.Context(), "tok", "12-34", "2026-10-14", "2026-10-15", "")
	require.Error(t, err)
	_, err = client.DailyMetrics(t.Context(), "tok", "1234567890", "14/10/2026", "2026-10-15", "")
	require.Error(t, err)
}

func TestAccountsWalkManagers(t *testing.T) {
	srv, calls := server(t, func(c captured, w http.ResponseWriter) {
		switch c.path {
		case "/v26/customers:listAccessibleCustomers":
			assert.Empty(t, c.login, "listAccessibleCustomers ignores the manager header")
			_, _ = w.Write([]byte(`{"resourceNames":["customers/9876543210","customers/5550001111"]}`))
		case "/v26/customers/9876543210/googleAds:searchStream":
			assert.Equal(t, "9876543210", c.login)
			_, _ = w.Write([]byte(`[{"results":[
				{"customerClient":{"id":"9876543210","descriptiveName":"WokeWorks MCC","level":"0","manager":true,"currencyCode":"AED","timeZone":"Asia/Dubai","status":"ENABLED"}},
				{"customerClient":{"id":"1234567890","descriptiveName":"Reportage Felix Properties","level":"1","manager":false,"currencyCode":"AED","timeZone":"Asia/Dubai","status":"ENABLED"}}
			]}]`))
		case "/v26/customers/5550001111/googleAds:searchStream":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":7,"message":"The caller does not have permission","status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.ads.googleads.v26.errors.GoogleAdsFailure","errors":[{"errorCode":{"authorizationError":"USER_PERMISSION_DENIED"},"message":"User doesn't have permission to access customer."}],"requestId":"x"}]}}`))
		}
	})
	client := googleads.New(googleads.Config{BaseURL: srv.URL, HTTPClient: srv.Client()})
	accounts, err := client.ListAccounts(t.Context(), "tok")
	require.NoError(t, err)
	require.Len(t, accounts, 2, "the forbidden customer is skipped, the manager and its client are kept")
	assert.True(t, accounts[0].Manager)
	assert.Equal(t, "Reportage Felix Properties", accounts[1].Name)
	assert.Equal(t, "9876543210", accounts[1].ParentID)
	assert.Equal(t, "AED", accounts[1].Currency)
	assert.Len(t, *calls, 3)
}

// Catches: a quota error without its retry delay, an auth error shown as a
// generic failure, and Google's message or request id reaching users.
func TestErrorMapping(t *testing.T) {
	status, body := http.StatusTooManyRequests, `{"error":{"code":8,"message":"Resource exhausted. secret-request-id-xyz","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.ads.googleads.v26.errors.GoogleAdsFailure","errors":[{"errorCode":{"quotaError":"RESOURCE_TEMPORARILY_EXHAUSTED"},"message":"Too many requests in a short amount of time.","details":{"quotaErrorDetails":{"rateScope":"ACCOUNT","rateName":"Account QPS","retryDelay":"42s"}}}],"requestId":"secret-request-id-xyz"}]}}`
	srv, _ := server(t, func(_ captured, w http.ResponseWriter) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	client := googleads.New(googleads.Config{BaseURL: srv.URL, HTTPClient: srv.Client()})
	_, err := client.ListCampaigns(t.Context(), "tok", "1234567890", "")
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.Contains(t, ce.Message, "quota")
	assert.NotContains(t, ce.Message, "secret-request-id")
	var ra googleads.RetryAfter
	assert.ErrorAs(t, err, &ra)
	assert.Equal(t, "42s", ra.Delay.String())

	status, body = http.StatusUnauthorized, `{"error":{"code":16,"message":"Request had invalid authentication credentials.","status":"UNAUTHENTICATED","details":[{"@type":"type.googleapis.com/google.ads.googleads.v26.errors.GoogleAdsFailure","errors":[{"errorCode":{"authenticationError":"OAUTH_TOKEN_EXPIRED"},"message":"expired"}]}]}}`
	_, err = client.ListCampaigns(t.Context(), "tok", "1234567890", "")
	require.ErrorAs(t, err, &ce)
	assert.Contains(t, ce.Message, "reconnect")

	status, body = http.StatusForbidden, `{"error":{"code":7,"status":"PERMISSION_DENIED","details":[{"@type":"x","errors":[{"errorCode":{"authorizationError":"CLOUD_PROJECT_NOT_APPROVED_FOR_PRODUCTION"}}]}]}}`
	_, err = client.ListCampaigns(t.Context(), "tok", "1234567890", "")
	require.ErrorAs(t, err, &ce)
	assert.Contains(t, ce.Message, "CLOUD_PROJECT_NOT_APPROVED_FOR_PRODUCTION")

	status, body = http.StatusServiceUnavailable, `{"error":{"code":14,"status":"UNAVAILABLE"}}`
	_, err = client.ListCampaigns(t.Context(), "tok", "1234567890", "")
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindUnavailable, ce.Kind)
}
