// Package storetest gives tests a fresh, migrated database.
package storetest

import (
	"path/filepath"
	"testing"

	"github.com/WokeWorks/adx/server/store"
)

func Open(t testing.TB) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}
