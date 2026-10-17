package ops

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

// LoadOp is one operation the harness repeats: a name and a call.
type LoadOp struct {
	Name string
	Run  func(ctx context.Context) error
}

// LoadPlan is a bounded, measured run: a mix of operations, how many
// workers, for how long.
type LoadPlan struct {
	Duration    time.Duration
	Concurrency int
	Ops         []LoadOp
	Notes       string
}

// LoadMeasurement is one measured run with the conditions it ran under. A
// throughput figure means nothing without the host, the mix and the
// concurrency, so they are stored together and shown together.
type LoadMeasurement struct {
	RanAt       time.Time        `json:"ranAt"`
	Host        string           `json:"host"`
	CPUs        int              `json:"cpus"`
	GoVersion   string           `json:"goVersion"`
	DurationMs  int64            `json:"durationMs"`
	Concurrency int              `json:"concurrency"`
	Operations  int64            `json:"operations"`
	Errors      int64            `json:"errors"`
	P50Ms       float64          `json:"p50Ms"`
	P95Ms       float64          `json:"p95Ms"`
	MaxMs       float64          `json:"maxMs"`
	Mix         map[string]int64 `json:"mix"`
	Notes       string           `json:"notes"`
	// OpsPerSecond is derived from operations and duration.
	OpsPerSecond float64 `json:"opsPerSecond"`
}

// RunLoad runs the plan and measures it. It records nothing: the caller
// decides where a measurement belongs.
func RunLoad(ctx context.Context, plan LoadPlan) (LoadMeasurement, error) {
	if plan.Duration <= 0 || plan.Concurrency <= 0 || len(plan.Ops) == 0 {
		return LoadMeasurement{}, errors.New("a load plan needs a duration, workers and operations")
	}
	start := time.Now()
	deadline, cancel := context.WithTimeout(ctx, plan.Duration)
	defer cancel()
	var mu sync.Mutex
	var latencies []float64
	mix := map[string]int64{}
	var errs int64
	var wg sync.WaitGroup
	for w := 0; w < plan.Concurrency; w++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for i := offset; ; i++ {
				if deadline.Err() != nil {
					return
				}
				op := plan.Ops[i%len(plan.Ops)]
				t0 := time.Now()
				err := op.Run(ctx)
				ms := float64(time.Since(t0).Microseconds()) / 1000
				mu.Lock()
				latencies = append(latencies, ms)
				mix[op.Name]++
				if err != nil {
					errs++
				}
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	elapsed := time.Since(start)
	sort.Float64s(latencies)
	host, _ := os.Hostname()
	m := LoadMeasurement{RanAt: start.UTC(), Host: host, CPUs: runtime.NumCPU(), GoVersion: runtime.Version(), DurationMs: elapsed.Milliseconds(),
		Concurrency: plan.Concurrency, Operations: int64(len(latencies)), Errors: errs, Mix: mix, Notes: plan.Notes}
	if len(latencies) > 0 {
		m.P50Ms, m.P95Ms, m.MaxMs = percentile(latencies, 0.5), percentile(latencies, 0.95), latencies[len(latencies)-1]
		m.OpsPerSecond = float64(len(latencies)) / elapsed.Seconds()
	}
	return m, nil
}

func percentile(sorted []float64, p float64) float64 {
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

// RecordLoad stores a measurement. Admins only, and only through the
// harness: a hand-typed number is not a measurement.
func (o *Ops) RecordLoad(ctx context.Context, actor core.Actor, m LoadMeasurement) (LoadMeasurement, error) {
	if !actor.AtLeast(core.RoleAdmin) || o.db == nil {
		return LoadMeasurement{}, core.ErrForbidden
	}
	if m.Operations == 0 || m.DurationMs == 0 || m.Host == "" {
		return LoadMeasurement{}, core.Invalid("a measurement needs operations, a duration and a host")
	}
	mix, _ := json.Marshal(m.Mix)
	row, err := o.db.Writer().CreateLoadMeasurement(ctx, store.CreateLoadMeasurementParams{RanAt: m.RanAt.UTC(), Host: m.Host, Cpus: int64(m.CPUs), GoVersion: m.GoVersion,
		DurationMs: m.DurationMs, Concurrency: int64(m.Concurrency), Operations: m.Operations, Errors: m.Errors, P50Ms: m.P50Ms, P95Ms: m.P95Ms, MaxMs: m.MaxMs, Mix: string(mix), Notes: m.Notes})
	if err != nil {
		return LoadMeasurement{}, err
	}
	return loadFromRow(row), nil
}

// LatestLoad is the newest measurement, or nil when none was ever run: the
// screen then says so instead of quoting a number.
func (o *Ops) LatestLoad(ctx context.Context) (*LoadMeasurement, error) {
	if o.db == nil {
		return nil, nil
	}
	row, err := o.db.Reader().LatestLoadMeasurement(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m := loadFromRow(row)
	return &m, nil
}

func loadFromRow(r store.LoadMeasurement) LoadMeasurement {
	m := LoadMeasurement{RanAt: r.RanAt, Host: r.Host, CPUs: int(r.Cpus), GoVersion: r.GoVersion, DurationMs: r.DurationMs, Concurrency: int(r.Concurrency),
		Operations: r.Operations, Errors: r.Errors, P50Ms: r.P50Ms, P95Ms: r.P95Ms, MaxMs: r.MaxMs, Notes: r.Notes, Mix: map[string]int64{}}
	_ = json.Unmarshal([]byte(r.Mix), &m.Mix)
	if m.DurationMs > 0 {
		m.OpsPerSecond = float64(m.Operations) / (float64(m.DurationMs) / 1000)
	}
	return m
}
