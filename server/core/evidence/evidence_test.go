package evidence_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: the same observation stored twice after repeated work, and a
// changed observation overwriting the original instead of versioning it.
func TestRepeatedObservationsDoNotDuplicate(t *testing.T) {
	clock := coretest.NewClock(time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC))
	s := evidence.New(storetest.Open(t), clock)
	contact := evidence.Observation{Source: "gohighlevel", Kind: "contact", SourceRef: "c-1", Class: evidence.Observed,
		Completeness: evidence.Complete, Payload: map[string]any{"name": "Sara", "status": "New"}}

	first, created, err := s.Record(t.Context(), coretest.Marketer, contact)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, int64(1), first.PayloadVersion)

	for range 3 {
		again, created, err := s.Record(t.Context(), coretest.Marketer, contact)
		require.NoError(t, err)
		assert.False(t, created)
		assert.Equal(t, first.ID, again.ID)
	}
	n, err := s.Count(t.Context(), coretest.Viewer, "gohighlevel", "contact")
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	clock.Advance(time.Hour)
	changed := contact
	changed.Payload = map[string]any{"name": "Sara", "status": "Follow Up"}
	second, created, err := s.Record(t.Context(), coretest.Marketer, changed)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, int64(2), second.PayloadVersion)

	current, err := s.Lookup(t.Context(), coretest.Viewer, evidence.Filter{Source: "gohighlevel", Kind: "contact"})
	require.NoError(t, err)
	require.Len(t, current, 1, "only the current version is listed")
	assert.Equal(t, second.ID, current[0].ID)
	assert.JSONEq(t, `{"name":"Sara","status":"Follow Up"}`, string(current[0].Payload))
	n, err = s.Count(t.Context(), coretest.Viewer, "gohighlevel", "contact")
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "the original is kept")
}

// Catches: evidence leaking across workspaces, or a viewer being refused the
// evidence behind a figure they can see.
func TestLookupIsScopedAndValidated(t *testing.T) {
	s := evidence.New(storetest.Open(t), nil)
	_, _, err := s.Record(t.Context(), coretest.Marketer, evidence.Observation{Source: "adx", Kind: "cpl", SourceRef: "camp-1/2026-10-16",
		Class: evidence.Inferred, Completeness: evidence.Partial, Payload: map[string]any{"minor": 12000, "currency": "AED"}})
	require.NoError(t, err)

	other := coretest.Viewer
	other.WorkspaceID = 2
	rows, err := s.Lookup(t.Context(), other, evidence.Filter{Source: "adx", Kind: "cpl"})
	require.NoError(t, err)
	assert.Empty(t, rows)
	rows, err = s.Lookup(t.Context(), coretest.Viewer, evidence.Filter{Source: "adx", Kind: "cpl"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, evidence.Inferred, rows[0].Class)
	assert.Equal(t, evidence.Partial, rows[0].Completeness)

	_, _, err = s.Record(t.Context(), coretest.Viewer, evidence.Observation{Source: "adx", Kind: "cpl", SourceRef: "x", Class: evidence.Observed, Completeness: evidence.Complete})
	assert.ErrorIs(t, err, core.ErrForbidden)
	for _, bad := range []evidence.Observation{
		{Source: "adx", Kind: "cpl", SourceRef: "x", Class: "guess", Completeness: evidence.Complete},
		{Source: "adx", Kind: "cpl", SourceRef: "x", Class: evidence.Observed, Completeness: "mostly"},
		{Source: "adx", Kind: "", SourceRef: "x", Class: evidence.Observed, Completeness: evidence.Complete},
	} {
		_, _, err = s.Record(t.Context(), coretest.Marketer, bad)
		var ce *core.Error
		require.ErrorAs(t, err, &ce)
		assert.Equal(t, core.KindInvalid, ce.Kind)
	}
}
