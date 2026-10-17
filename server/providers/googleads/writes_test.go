package googleads_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/execution"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/providers/googleads"
)

// Catches: a create without the marker in the name, a budget sent in the
// wrong unit, a missing PAUSED status, an update without its mask, a 5xx
// treated as a refusal, and a 4xx treated as uncertain.
func TestWritesSendTheDocumentedShapes(t *testing.T) {
	var bodies []map[string]any
	var paths []string
	mode := "ok"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case mode == "down":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":{"code":502,"message":"upstream"}}`))
		case mode == "refused":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":400,"message":"bad","status":"INVALID_ARGUMENT","details":[{"@type":"x","errors":[{"errorCode":{"fieldError":"REQUIRED"},"message":"Name is required"}]}]}}`))
		case strings.HasSuffix(r.URL.Path, "googleAds:mutate"):
			_, _ = w.Write([]byte(`{"mutateOperationResponses":[{"campaignBudgetResult":{"resourceName":"customers/1234567890/campaignBudgets/777"}},{"campaignResult":{"resourceName":"customers/1234567890/campaigns/999"}}]}`))
		case strings.HasSuffix(r.URL.Path, "googleAds:searchStream"):
			q, _ := body["query"].(string)
			if strings.Contains(q, "campaign.campaign_budget") {
				_, _ = w.Write([]byte(`[{"results":[{"campaign":{"id":"111","campaignBudget":"customers/1234567890/campaignBudgets/555"}}]}]`))
			} else if strings.Contains(q, "LIKE") {
				_, _ = w.Write([]byte(`[{"results":[{"campaign":{"id":"999","name":"Wadi [adx:7]","status":"PAUSED"}}]}]`))
			} else {
				_, _ = w.Write([]byte(`[{"results":[{"campaign":{"id":"999","status":"PAUSED"}}]}]`))
			}
		default:
			_, _ = w.Write([]byte(`{"results":[{"resourceName":"x"}]}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := googleads.New(googleads.Config{BaseURL: srv.URL, LoginCustomerID: "9876543210"})
	assert.ElementsMatch(t, []policies.WriteAction{policies.CampaignCreatePaus, policies.BudgetRaise, policies.BudgetLower, policies.CampaignPause}, c.Capabilities())

	res, err := c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "1234567890", Action: policies.CampaignCreatePaus, Marker: "adx:7", Proposed: json.RawMessage(`{"name":"Wadi","dailyBudgetMinor":50000,"channelType":"search"}`)})
	require.NoError(t, err)
	assert.Equal(t, "/v26/customers/1234567890/googleAds:mutate", paths[0])
	ops := bodies[0]["mutateOperations"].([]any)
	require.Len(t, ops, 2)
	budget := ops[0].(map[string]any)["campaignBudgetOperation"].(map[string]any)["create"].(map[string]any)
	assert.Equal(t, "500000000", budget["amountMicros"], "minor units times 10,000")
	camp := ops[1].(map[string]any)["campaignOperation"].(map[string]any)["create"].(map[string]any)
	assert.Equal(t, "Wadi [adx:7]", camp["name"])
	assert.Equal(t, "PAUSED", camp["status"])
	assert.Equal(t, "SEARCH", camp["advertisingChannelType"])
	assert.Equal(t, "customers/1234567890/campaignBudgets/-1", camp["campaignBudget"])
	require.Len(t, res.Refs, 2)
	assert.Equal(t, "777", res.Refs[0].ExternalID)
	assert.Equal(t, "999", res.Refs[1].ExternalID)

	_, err = c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "1234567890", Action: policies.BudgetRaise, TargetRef: "111", Proposed: json.RawMessage(`{"dailyBudgetMinor":60000}`)})
	require.NoError(t, err)
	assert.Equal(t, "/v26/customers/1234567890/campaignBudgets:mutate", paths[len(paths)-1])
	op := bodies[len(bodies)-1]["operations"].([]any)[0].(map[string]any)
	assert.Equal(t, "amount_micros", op["updateMask"])
	assert.Equal(t, "customers/1234567890/campaignBudgets/555", op["update"].(map[string]any)["resourceName"])
	assert.Equal(t, "600000000", op["update"].(map[string]any)["amountMicros"])

	_, err = c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "1234567890", Action: policies.CampaignPause, TargetRef: "111"})
	require.NoError(t, err)
	op = bodies[len(bodies)-1]["operations"].([]any)[0].(map[string]any)
	assert.Equal(t, "status", op["updateMask"])
	assert.Equal(t, "PAUSED", op["update"].(map[string]any)["status"])

	_, err = c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "1234567890", Action: policies.AudiencesUpsert, Proposed: json.RawMessage(`{}`)})
	require.Error(t, err)

	found, err := c.Lookup(t.Context(), "tok", "1234567890", "adx:7", policies.CampaignCreatePaus, "")
	require.NoError(t, err)
	assert.True(t, found.Found)
	assert.Equal(t, "999", found.Refs[0].ExternalID)
	assert.Contains(t, bodies[len(bodies)-1]["query"], "LIKE '%[adx:7]%'")
	v, err := c.Verify(t.Context(), "tok", "1234567890", []proposals.ProviderRef{{Kind: "campaign", ExternalID: "999"}})
	require.NoError(t, err)
	assert.True(t, v.Exists)
	assert.Equal(t, "PAUSED", v.Status)

	mode = "down"
	_, err = c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "1234567890", Action: policies.CampaignPause, TargetRef: "111"})
	var ue *execution.UncertainError
	assert.True(t, errors.As(err, &ue), "a 5xx may have applied: %v", err)
	mode = "refused"
	_, err = c.Apply(t.Context(), execution.ApplyRequest{Token: "tok", AccountExternalID: "1234567890", Action: policies.CampaignPause, TargetRef: "111"})
	require.Error(t, err)
	assert.False(t, errors.As(err, &ue), "a 4xx is a refusal, not uncertainty")
}
