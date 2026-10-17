package connections_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/connections"
)

// Catches: an unsupported capability without a reason, a publish marked
// supported, a write wired without live writes, a duplicate entry, and a
// connector that is present but whose writes claim to be wired.
func TestCatalogNeverImpliesUnsupportedPublishing(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range connections.Catalog(connections.Wiring{}) {
		key := c.Provider + "/" + c.Capability
		require.False(t, seen[key], "duplicate %s", key)
		seen[key] = true
		assert.NotEmpty(t, c.Reason, "%s needs a reason or a gate", key)
		if c.Direction == "publish" {
			assert.False(t, c.Supported, "%s: ADX publishes nothing", key)
		}
		assert.False(t, c.Wired, "nothing is wired on an empty server: %s", key)
	}
	for _, p := range []string{"meta", "google_ads", "gohighlevel", "whatsapp", "email"} {
		found := false
		for _, c := range connections.Catalog(connections.Wiring{}) {
			if c.Provider == p && !c.Supported {
				found = true
			}
		}
		assert.True(t, found, "%s states at least one thing it does not do", p)
	}

	readOnly := connections.Catalog(connections.Wiring{Providers: []string{"meta", "google_ads"}, LiveWrites: false})
	for _, c := range readOnly {
		if c.Provider == "meta" || c.Provider == "google_ads" {
			if c.Direction == "read" && c.Supported {
				assert.True(t, c.Wired, "%s reads are wired with credentials", c.Capability)
			} else {
				assert.False(t, c.Wired, "%s/%s: no write is wired without ADX_LIVE_WRITES", c.Provider, c.Capability)
			}
		}
	}
	live := connections.Catalog(connections.Wiring{Providers: []string{"meta"}, LiveWrites: true, Channels: []string{"whatsapp"}})
	wired := map[string]bool{}
	for _, c := range live {
		wired[c.Provider+"/"+c.Capability] = c.Wired
	}
	assert.True(t, wired["meta/write.budget"])
	assert.False(t, wired["google_ads/write.budget"], "no Google credentials, nothing wired")
	assert.False(t, wired["meta/publish.creative"])
	assert.True(t, wired["whatsapp/write.message"])
	assert.False(t, wired["email/write.message"])
}
