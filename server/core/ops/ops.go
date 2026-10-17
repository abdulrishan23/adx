// Package ops answers "is this server able to serve?" for the load balancer
// and the operations screen. Liveness is the process answering at all.
// Readiness is the database reachable, migrated and the workers running. A
// failed check names itself and nothing else, so readiness never leaks a path
// or a secret.
package ops

import (
	"context"
	"log/slog"
	"time"

	"github.com/WokeWorks/adx/server/store"
)

// Check is one readiness condition. Run returns nil when it holds.
type Check struct {
	Name string
	Run  func(context.Context) error
}

type CheckResult struct {
	Name string `json:"name"`
	Ok   bool   `json:"ok"`
}

type Readiness struct {
	Ready     bool          `json:"ready"`
	Checks    []CheckResult `json:"checks"`
	CheckedAt time.Time     `json:"checkedAt"`
}

type Ops struct {
	log     *slog.Logger
	db      *store.DB
	checks  []Check
	now     func() time.Time
	budgets Budgets
	running string
}

// New builds the readiness checks for db plus any extra checks the caller
// adds, such as the job worker.
func New(log *slog.Logger, db *store.DB, extra ...Check) *Ops {
	checks := []Check{
		{Name: "database", Run: func(ctx context.Context) error {
			var one int
			return db.Read.QueryRowContext(ctx, "SELECT 1").Scan(&one)
		}},
		{Name: "migrations", Run: func(ctx context.Context) error {
			pending, err := db.Pending(ctx)
			if err != nil {
				return err
			}
			if pending {
				return errPending
			}
			return nil
		}},
	}
	return &Ops{log: log, db: db, checks: append(checks, extra...), now: time.Now}
}

type constError string

func (e constError) Error() string { return string(e) }

const errPending = constError("migrations pending")

// Readiness runs every check with a short deadline. Failures are logged with
// their cause and reported by name only.
func (o *Ops) Readiness(ctx context.Context) Readiness {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out := Readiness{Ready: true, Checks: make([]CheckResult, 0, len(o.checks)), CheckedAt: o.now().UTC()}
	for _, c := range o.checks {
		err := c.Run(ctx)
		if err != nil {
			out.Ready = false
			o.log.WarnContext(ctx, "readiness check failed", "check", c.Name, "error", err)
		}
		out.Checks = append(out.Checks, CheckResult{Name: c.Name, Ok: err == nil})
	}
	return out
}
