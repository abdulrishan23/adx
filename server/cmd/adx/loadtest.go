package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/ops"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/providers/filestore"
	"github.com/WokeWorks/adx/server/store"
)

// runLoadtest is `adx loadtest DURATION CONCURRENCY`: a representative mix
// of reads, writes, a job-style ingest and a media write against a fresh
// temporary database, measured, then recorded with its conditions in the
// configured database so the operations screen can show it. It never
// touches production records: the workload runs in a scratch directory.
func runLoadtest(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: adx loadtest DURATION CONCURRENCY (for example 10s 8)")
	}
	duration, err := time.ParseDuration(args[0])
	if err != nil {
		return err
	}
	concurrency, err := strconv.Atoi(args[1])
	if err != nil || concurrency <= 0 {
		return errors.New("concurrency must be a positive number")
	}
	cfg, err := env.ParseAs[config]()
	if err != nil {
		return err
	}
	m, err := measureLoad(context.Background(), duration, concurrency)
	if err != nil {
		return err
	}
	fmt.Printf("%d operations in %dms with %d workers on %s (%d cpus, %s): %.0f ops/s, p50 %.1fms, p95 %.1fms, max %.1fms, %d errors\n",
		m.Operations, m.DurationMs, m.Concurrency, m.Host, m.CPUs, m.GoVersion, m.OpsPerSecond, m.P50Ms, m.P95Ms, m.MaxMs, m.Errors)
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		return fmt.Errorf("measured but not recorded: %w", err)
	}
	if _, err := ops.New(slog.Default(), db).RecordLoad(context.Background(), core.System(core.ViaStartup), m); err != nil {
		return fmt.Errorf("measured but not recorded: %w", err)
	}
	fmt.Println("recorded")
	return nil
}

// measureLoad builds the workload against a scratch database and runs it.
func measureLoad(ctx context.Context, duration time.Duration, concurrency int) (ops.LoadMeasurement, error) {
	dir, err := os.MkdirTemp("", "adx-loadtest-")
	if err != nil {
		return ops.LoadMeasurement{}, err
	}
	defer os.RemoveAll(dir)
	db, err := store.Open(filepath.Join(dir, "load.db"))
	if err != nil {
		return ops.LoadMeasurement{}, err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return ops.LoadMeasurement{}, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o700); err != nil {
		return ops.LoadMeasurement{}, err
	}
	files, err := filestore.New(filepath.Join(dir, "assets"))
	if err != nil {
		return ops.LoadMeasurement{}, err
	}
	ev := evidence.New(db, core.SystemClock{})
	people := crm.New(db, ev, crm.Config{})
	sig := signals.New(db, ev, people, signals.Config{})
	actor := core.SystemIn(core.ViaJob, core.DefaultWorkspace)
	var n atomic.Int64
	next := func() int64 { return n.Add(1) }
	blob := bytes.Repeat([]byte("adx"), 64*1024/3)
	plan := ops.LoadPlan{Duration: duration, Concurrency: concurrency, Notes: "scratch database; mix of person create, person list, signal ingest, evidence record, 64KB asset write", Ops: []ops.LoadOp{
		{Name: "person.create", Run: func(ctx context.Context) error {
			i := next()
			_, err := people.Create(ctx, actor, crm.PersonInput{DisplayName: fmt.Sprintf("Load %d", i), Phone: fmt.Sprintf("+9715%08d", i%100000000)})
			return err
		}},
		{Name: "person.list", Run: func(ctx context.Context) error {
			_, err := people.List(ctx, actor, "Load", 50)
			return err
		}},
		{Name: "signal.ingest", Run: func(ctx context.Context) error {
			i := next()
			_, err := sig.Ingest(ctx, actor, signals.SignalInput{Source: "loadtest", Kind: "form_submit", OccurrenceKey: fmt.Sprintf("load-%d", i), OccurredAt: time.Now().UTC(),
				Identity: signals.SignalIdentity{Phone: fmt.Sprintf("+9715%08d", i%100000000)}})
			return err
		}},
		{Name: "evidence.record", Run: func(ctx context.Context) error {
			i := next()
			_, _, err := ev.Record(ctx, actor, evidence.Observation{Source: "loadtest", Kind: "probe", SourceRef: fmt.Sprintf("probe:%d", i), Scope: "load", Class: evidence.Observed, Payload: []byte(`{"i":1}`), Completeness: evidence.Complete})
			return err
		}},
		{Name: "asset.put", Run: func(ctx context.Context) error {
			_, err := files.Put(ctx, "application/octet-stream", bytes.NewReader(blob))
			return err
		}},
	}}
	return ops.RunLoad(ctx, plan)
}
