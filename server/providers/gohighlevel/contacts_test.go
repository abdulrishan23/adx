package gohighlevel_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
)

// Catches: the search request missing the location, page size or sort; the
// cursor not being the last contact's searchAfter; a short last page being
// treated as having more; lowercase names, multi-select custom fields and
// the two attribution objects being read wrongly; and a 429 or 401 being
// turned into something other than a safe conflict.
func TestListContactsPagesWithSearchAfter(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/contacts/search", r.URL.Path)
		assert.Equal(t, "Bearer pit-token", r.Header.Get("Authorization"))
		assert.Equal(t, "2021-07-28", r.Header.Get("Version"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		require.NoError(t, json.Unmarshal(raw, &body))
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		if body["searchAfter"] == nil {
			// First page: full page of 100 identical-shaped contacts, the
			// last carrying the cursor.
			contacts := make([]map[string]any, 0, 100)
			for i := 0; i < 100; i++ {
				contacts = append(contacts, map[string]any{"id": "c-" + string(rune('A'+i%26)) + string(rune('a'+i/26)), "firstNameLowerCase": "sara", "lastNameLowerCase": "k",
					"phone": "+971501234567", "source": "Meta", "dateAdded": "2026-10-01T08:00:00.000Z", "dateUpdated": "2026-10-02T09:30:00.000Z",
					"tags": []string{"status:follow up"}, "customFields": []map[string]any{{"id": "cf_project", "value": "The Wadi"}, {"id": "cf_units", "value": []string{"1-2 BR", "Duplex"}}, {"id": "cf_budget", "value": 1500000}},
					"attributionSource":     map[string]any{"utmSource": "facebook", "campaign": "wadi-launch", "campaignId": "120210", "fbclid": "fb.1.x", "url": "https://reportage.ae/wadi"},
					"lastAttributionSource": map[string]any{"utmSource": "google", "utmMedium": "cpc", "campaign": "wadi-search", "gclid": "Cj0K", "utmKeyword": "wadi villas"},
					"searchAfter":           []any{1759305600000, "c-last"}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"contacts": contacts, "total": 101})
			return
		}
		assert.Equal(t, []any{float64(1759305600000), "c-last"}, body["searchAfter"])
		_ = json.NewEncoder(w).Encode(map[string]any{"contacts": []map[string]any{{"id": "c-101", "firstName": "Omar", "searchAfter": []any{1, "c-101"}}}, "total": 101})
	}))
	defer server.Close()
	client := newClient(t, server)

	page, err := client.ListContacts(t.Context(), "pit-token", "loc-1", "", nil)
	require.NoError(t, err)
	require.Len(t, page.Contacts, 100)
	assert.Equal(t, int64(101), *page.Total)
	assert.Equal(t, `[1759305600000,"c-last"]`, page.NextCursor)
	first := page.Contacts[0]
	assert.Equal(t, "sara", first.FirstName)
	assert.Equal(t, "Meta", first.Source)
	assert.Equal(t, time.Date(2026, 10, 2, 9, 30, 0, 0, time.UTC), *first.DateUpdated)
	assert.Equal(t, "The Wadi", first.CustomFields["cf_project"])
	assert.Equal(t, "1-2 BR, Duplex", first.CustomFields["cf_units"])
	assert.Equal(t, "1500000", first.CustomFields["cf_budget"])
	require.Len(t, first.Attributions, 2)
	assert.True(t, first.Attributions[0].IsFirst)
	assert.Equal(t, "wadi-launch", first.Attributions[0].UTMCampaign)
	assert.Equal(t, "120210", first.Attributions[0].CampaignID)
	assert.Equal(t, "fb.1.x", first.Attributions[0].FBCLID)
	assert.Equal(t, "https://reportage.ae/wadi", first.Attributions[0].LandingURL)
	assert.False(t, first.Attributions[1].IsFirst)
	assert.Equal(t, "Cj0K", first.Attributions[1].GCLID)
	assert.Equal(t, "wadi villas", first.Attributions[1].UTMTerm)
	assert.Contains(t, string(first.Raw), `"firstNameLowerCase":"sara"`)

	second, err := client.ListContacts(t.Context(), "pit-token", "loc-1", page.NextCursor, nil)
	require.NoError(t, err)
	require.Len(t, second.Contacts, 1)
	assert.Equal(t, "Omar", second.Contacts[0].FirstName)
	assert.Empty(t, second.NextCursor, "a short page is the last page")

	require.Len(t, bodies, 2)
	assert.Equal(t, "loc-1", bodies[0]["locationId"])
	assert.Equal(t, float64(100), bodies[0]["pageLimit"])
	assert.Nil(t, bodies[0]["page"], "page numbers stop at 10,000 records; searchAfter is used instead")
	assert.Equal(t, []any{map[string]any{"field": "dateAdded", "direction": "asc"}, map[string]any{"field": "id", "direction": "asc"}}, bodies[0]["sort"])
	assert.Nil(t, bodies[0]["filters"])

	// Incremental: a dateUpdated range filter and sort.
	since := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	_, err = client.ListContacts(t.Context(), "pit-token", "loc-1", "", &since)
	require.NoError(t, err)
	filters := bodies[2]["filters"].([]any)
	require.Len(t, filters, 1)
	assert.Equal(t, map[string]any{"field": "dateUpdated", "operator": "range", "value": map[string]any{"gte": "2026-10-16T00:00:00Z"}}, filters[0])
	assert.Equal(t, "dateUpdated", bodies[2]["sort"].([]any)[0].(map[string]any)["field"])

	_, err = client.ListContacts(t.Context(), "pit-token", "loc-1", "not json", nil)
	require.Error(t, err)
}

func TestListContactsMapsErrors(t *testing.T) {
	status := http.StatusTooManyRequests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"statusCode":401,"message":"Invalid token: access token is invalid","error":"Unauthorized"}`))
	}))
	defer server.Close()
	client := newClient(t, server)
	_, err := client.ListContacts(t.Context(), "pit-token", "loc-1", "", nil)
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.Contains(t, ce.Message, "limit")

	status = http.StatusUnauthorized
	_, err = client.ListContacts(t.Context(), "pit-token", "loc-1", "", nil)
	require.ErrorAs(t, err, &ce)
	assert.Contains(t, ce.Message, "rejected")
	assert.NotContains(t, err.Error(), "Invalid token", "provider bodies never reach users")
}
