package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core/projects"
	"github.com/WokeWorks/adx/server/store/storetest"
)

func newServer(t *testing.T, dev bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(api.NewRouter(api.Deps{
		Log:      slog.New(slog.DiscardHandler),
		Dev:      dev,
		Projects: projects.New(storetest.Open(t)),
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestProjectsRoundTrip(t *testing.T) {
	srv := newServer(t, true)

	res, err := http.Post(srv.URL+"/api/projects", "application/json", strings.NewReader(`{"name":"The Wadi"}`))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, res.StatusCode)

	res, err = http.Post(srv.URL+"/api/projects", "application/json", strings.NewReader(`{"name":"the wadi"}`))
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, res.StatusCode)

	res, err = http.Get(srv.URL + "/api/projects")
	require.NoError(t, err)
	body, _ := io.ReadAll(res.Body)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, string(body), `"name":"The Wadi"`)
}

func TestUnknownFieldIsRejected(t *testing.T) {
	srv := newServer(t, true)
	res, err := http.Post(srv.URL+"/api/projects", "application/json", strings.NewReader(`{"nmae":"Oceana"}`))
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestOutsideDevEverythingIsRefused(t *testing.T) {
	srv := newServer(t, false)
	res, err := http.Get(srv.URL + "/api/projects")
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

// Catches: another website making a signed-in browser change data, which
// http.NewCrossOriginProtection in the router stops.
func TestCrossOriginWritesAreRefused(t *testing.T) {
	srv := newServer(t, true)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/projects", strings.NewReader(`{"name":"The Wadi"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
}
