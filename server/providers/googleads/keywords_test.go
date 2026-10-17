package googleads_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/providers/googleads"
)

// Catches: the wrong geo or language constant, seeds sent untrimmed, a
// network other than search, int64 strings read as zero, micros not
// converted, an idea without metrics becoming zeros, and an unknown country
// being guessed.
func TestKeywordIdeas(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v26/customers/1234567890:generateKeywordIdeas", r.URL.Path)
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		assert.Equal(t, "9876543210", r.Header.Get("login-customer-id"))
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[
			{"text":"villas in dubai","keywordIdeaMetrics":{"avgMonthlySearches":"12100","competition":"HIGH","competitionIndex":"87","lowTopOfPageBidMicros":"2450000","highTopOfPageBidMicros":"9800000"}},
			{"text":"the wadi reportage"}
		]}`))
	}))
	t.Cleanup(srv.Close)
	c := googleads.New(googleads.Config{BaseURL: srv.URL, LoginCustomerID: "987-654-3210"})
	ideas, err := c.KeywordIdeas(t.Context(), "tok", "1234567890", []string{" villas dubai ", "", "wadi"}, "ae", "EN")
	require.NoError(t, err)
	assert.Equal(t, []any{"villas dubai", "wadi"}, got["keywordSeed"].(map[string]any)["keywords"])
	assert.Equal(t, []any{"geoTargetConstants/2784"}, got["geoTargetConstants"])
	assert.Equal(t, "languageConstants/1000", got["language"])
	assert.Equal(t, "GOOGLE_SEARCH", got["keywordPlanNetwork"])
	require.Len(t, ideas, 2)
	assert.Equal(t, int64(12100), *ideas[0].AvgMonthlySearches)
	assert.Equal(t, "HIGH", ideas[0].Competition)
	assert.Equal(t, int64(87), *ideas[0].CompetitionIndex)
	assert.Equal(t, int64(245), *ideas[0].LowTopOfPageBidMinor)
	assert.Equal(t, int64(980), *ideas[0].HighTopOfPageBidMinor)
	assert.Nil(t, ideas[1].AvgMonthlySearches, "no metrics stays null, not zero")
	assert.Nil(t, ideas[1].LowTopOfPageBidMinor)

	_, err = c.KeywordIdeas(t.Context(), "tok", "1234567890", []string{"x"}, "FR", "en")
	require.Error(t, err)
	_, err = c.KeywordIdeas(t.Context(), "tok", "1234567890", []string{"x"}, "AE", "fr")
	require.Error(t, err)
	_, err = c.KeywordIdeas(t.Context(), "tok", "1234567890", nil, "AE", "en")
	require.Error(t, err)
}
