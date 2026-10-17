package ops_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/ops"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: readiness saying "ready" with a failing worker, or leaking the
// failure's text (which can hold a file path or a token) to whoever asks.
func TestReadinessReportsNamesNotCauses(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	secretish := errors.New("dial tcp: /var/lib/adx/secret-key-file denied")
	o := ops.New(log, storetest.Open(t), ops.Check{Name: "jobs", Run: func(context.Context) error { return secretish }})

	r := o.Readiness(t.Context())
	assert.False(t, r.Ready)
	require.Len(t, r.Checks, 3)
	assert.Equal(t, "database", r.Checks[0].Name)
	assert.True(t, r.Checks[0].Ok)
	assert.True(t, r.Checks[1].Ok, "a migrated test database has nothing pending")
	assert.Equal(t, "jobs", r.Checks[2].Name)
	assert.False(t, r.Checks[2].Ok)

	body, err := json.Marshal(r)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "secret-key-file")
	assert.Contains(t, logs.String(), "secret-key-file", "the cause goes to the log")
}

func TestReadinessIsReadyWhenEverythingHolds(t *testing.T) {
	o := ops.New(slog.New(slog.DiscardHandler), storetest.Open(t))
	assert.True(t, o.Readiness(t.Context()).Ready)
}
