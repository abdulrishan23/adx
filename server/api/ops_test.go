package api_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core/ops"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: readiness answering 200 while a check fails, needing a sign-in
// (the proxy has none), or being left out of the router.
func TestReadyNeedsNoSessionAndFailsWith503(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	var workerDown bool
	o := ops.New(log, storetest.Open(t), ops.Check{Name: "jobs", Run: func(context.Context) error {
		if workerDown {
			return errors.New("river stopped: /srv/adx/data/adx.db")
		}
		return nil
	}})
	srv := httptest.NewServer(api.NewRouter(api.Deps{Log: log, Dev: false, Ops: o}))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/api/ready")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode, "outside dev the readiness route still answers without a session")

	workerDown = true
	res, err = http.Get(srv.URL + "/api/ready")
	require.NoError(t, err)
	body, _ := io.ReadAll(res.Body)
	assert.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
	assert.Contains(t, string(body), `"name":"jobs","ok":false`)
	assert.NotContains(t, string(body), "/srv/adx")
}
