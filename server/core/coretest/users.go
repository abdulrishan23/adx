package coretest

import (
	"testing"
	"time"

	"github.com/WokeWorks/adx/server/store"
)

// SeedUsers creates the rows behind Admin, Marketer and Viewer, so foreign
// keys to users hold in tests that record who did something.
func SeedUsers(t testing.TB, db *store.DB) {
	t.Helper()
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i, u := range []struct{ email, role string }{{"cmo@reportage.ae", "admin"}, {"sara@reportage.ae", "marketer"}, {"ceo@reportage.ae", "viewer"}} {
		if _, err := db.Writer().CreateUser(t.Context(), store.CreateUserParams{Email: u.email, Role: u.role, Status: "active",
			AuthentikPk: int64(i + 1), AuthentikUuid: "uuid-" + u.email, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
}
