package ops

import (
	"context"
	"errors"
	"time"

	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/store"
)

// Meter wraps the model every domain calls and enforces one server-wide
// daily token cap across all of them: the assistant, plan generation,
// research, call readings and workflow steps. Usage is written per UTC day
// and model so the cap survives a restart. Once the cap is reached every
// call is refused with a plain reason until the day turns; nothing retries
// on its own, so a runaway loop costs at most one day's cap.
type Meter struct {
	db    *store.DB
	model assistant.Model
	cap   int64
	now   func() time.Time
}

// NewMeter wraps model. A cap of zero meters without refusing.
func NewMeter(db *store.DB, model assistant.Model, dailyCap int64, now func() time.Time) *Meter {
	if now == nil {
		now = time.Now
	}
	return &Meter{db: db, model: model, cap: dailyCap, now: now}
}

func (m *Meter) Name() string { return m.model.Name() }

var errCapReached = errors.New("daily model token cap reached")

func (m *Meter) Complete(ctx context.Context, req assistant.Request) (assistant.Response, error) {
	day := m.now().UTC().Format("2006-01-02")
	if m.cap > 0 {
		used, err := m.db.Reader().SumModelUsageDay(ctx, day)
		if err != nil {
			return assistant.Response{}, err
		}
		if used.InputTokens+used.OutputTokens >= m.cap {
			_ = m.db.Writer().AddModelUsage(ctx, store.AddModelUsageParams{Day: day, Model: m.model.Name(), Refused: 1})
			return assistant.Response{}, &assistant.ModelError{Retryable: false, Cause: errCapReached,
				Safe: "the server's daily model cap is reached; nothing more is generated until tomorrow unless an admin raises ADX_MODEL_DAILY_TOKEN_CAP"}
		}
	}
	resp, err := m.model.Complete(ctx, req)
	if err != nil {
		return resp, err
	}
	name := resp.Model
	if name == "" {
		name = m.model.Name()
	}
	if werr := m.db.Writer().AddModelUsage(ctx, store.AddModelUsageParams{Day: day, Model: name, InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens, Calls: 1}); werr != nil {
		return resp, werr
	}
	return resp, nil
}

var _ assistant.Model = (*Meter)(nil)
