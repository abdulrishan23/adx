package gohighlevel_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/providers/gohighlevel"
)

// Catches: the wrong verb or path, a missing token, fields sent under the
// wrong key, and a provider rejection reported as success.
func TestSetContactFieldsSendsOnlyTheGivenFields(t *testing.T) {
	var got map[string]any
	var method, path, auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &got))
		if path == "/contacts/bad" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"unknown field"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"contact":{"id":"c-1"}}`))
	}))
	defer server.Close()
	client, err := gohighlevel.New(gohighlevel.Config{ClientID: "id", ClientSecret: "secret", RedirectURI: "https://adx.example.com/cb", APIBaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)
	require.NoError(t, client.SetContactFields(t.Context(), "tok", "c-1", map[string]string{"adx_temperature": "hot"}))
	assert.Equal(t, http.MethodPut, method)
	assert.Equal(t, "/contacts/c-1", path)
	assert.Equal(t, "Bearer tok", auth)
	cf := got["customFields"].([]any)
	require.Len(t, cf, 1)
	assert.Equal(t, map[string]any{"id": "adx_temperature", "field_value": "hot"}, cf[0])
	assert.Len(t, got, 1, "nothing but customFields is sent")
	err = client.SetContactFields(t.Context(), "tok", "bad", map[string]string{"x": "y"})
	require.Error(t, err)
	assert.Error(t, client.SetContactFields(t.Context(), "tok", "", map[string]string{"x": "y"}))
	assert.Error(t, client.SetContactFields(t.Context(), "tok", "c-1", nil))
}
