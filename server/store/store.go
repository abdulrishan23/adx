// Package store is ADX's SQLite database: migrations, queries and the
// code sqlc generates from them.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"strings"

	"github.com/pressly/goose/v3"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB holds two pools on one SQLite file. SQLite allows one writer at a time,
// so writes share a single connection and reads get their own pool.
type DB struct {
	Read  *sql.DB
	Write *sql.DB
}

// Open opens the database at path, creating it if needed.
func Open(path string) (*DB, error) {
	// _txlock=immediate makes a write transaction take the write lock when it
	// begins. Otherwise it can fail halfway with "database is locked".
	write, err := sql.Open("sqlite", "file:"+path+
		"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"+
		"&_pragma=foreign_keys(1)&_txlock=immediate&_time_format=sqlite")
	if err != nil {
		return nil, err
	}
	write.SetMaxOpenConns(1)

	read, err := sql.Open("sqlite", "file:"+path+
		"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=query_only(1)&_time_format=sqlite")
	if err != nil {
		write.Close()
		return nil, err
	}
	read.SetMaxOpenConns(max(4, runtime.NumCPU()))

	// Ping the writer first so it creates the file and switches it to WAL
	// before any reader opens it.
	if err := write.Ping(); err != nil {
		write.Close()
		read.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &DB{Read: read, Write: write}, nil
}

func (db *DB) Close() error {
	return errors.Join(db.Read.Close(), db.Write.Close())
}

// Migrate applies any migrations that haven't run yet.
func (db *DB) Migrate(ctx context.Context) error {
	p, err := db.migrator()
	if err != nil {
		return err
	}
	_, err = p.Up(ctx)
	return err
}

// MigrateTo applies migrations up to and including version. Tests use it to
// check that a later migration keeps existing rows working.
func (db *DB) MigrateTo(ctx context.Context, version int64) error {
	p, err := db.migrator()
	if err != nil {
		return err
	}
	_, err = p.UpTo(ctx, version)
	return err
}

func (db *DB) migrator() (*goose.Provider, error) {
	dir, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectSQLite3, db.Write, dir)
}

func (db *DB) Reader() *Queries { return New(db.Read) }
func (db *DB) Writer() *Queries { return New(db.Write) }

// Tx runs fn in one write transaction. It holds the only write connection
// until it returns, so keep it short and never call an outside API inside it.
func (db *DB) Tx(ctx context.Context, fn func(tx *sql.Tx, q *Queries) error) error {
	tx, err := db.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx, New(tx)); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

// Pending reports whether any migration hasn't run yet. Readiness uses it so
// a server started against an older database file says so instead of
// failing on the first query.
func (db *DB) Pending(ctx context.Context) (bool, error) {
	p, err := db.migrator()
	if err != nil {
		return false, err
	}
	return p.HasPending(ctx)
}

// BackupTo writes a consistent copy of the whole database to path using
// SQLite's VACUUM INTO, which works while the server keeps running. path must
// not exist yet. The copy holds the sealed credentials but not the key, so a
// restore needs the key file from its own backup.
func (db *DB) BackupTo(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("backup target %s already exists", path)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// VACUUM INTO takes a filename literal, so it can't be a bound parameter.
	// The path is escaped as an SQL string.
	_, err := db.Write.ExecContext(ctx, "VACUUM INTO '"+strings.ReplaceAll(path, "'", "''")+"'")
	if err != nil {
		return fmt.Errorf("backup to %s: %w", path, err)
	}
	return nil
}

// Optimize refreshes the statistics SQLite uses to plan queries. SQLite
// recommends running it every few hours on long-lived connections.
func (db *DB) Optimize(ctx context.Context) error {
	_, err := db.Write.ExecContext(ctx, "PRAGMA optimize")
	return err
}

func IsUniqueViolation(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}
