// Package evidence keeps what ADX observed: a contact as GoHighLevel returned
// it, a day of campaign metrics, a figure ADX computed from them. Every
// figure a screen shows traces back to rows here. An observation is never
// edited: re-observing the same thing with the same content is a no-op, and
// with different content it becomes a new version that supersedes the old
// one, which stays. Observed, estimated and inferred are kept apart so an
// estimate can't be shown as a count.
package evidence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

type Class string

const (
	// Observed came straight from a source.
	Observed Class = "observed"
	// Estimated is a figure a source or ADX modelled, such as a platform's
	// estimated conversions.
	Estimated Class = "estimated"
	// Inferred is ADX's own conclusion from other evidence, such as a
	// campaign match by UTM.
	Inferred Class = "inferred"
)

type Completeness string

const (
	Complete Completeness = "complete"
	Partial  Completeness = "partial"
)

// Observation is one thing to record.
type Observation struct {
	// RunID is the internal id of the run that made the observation, or 0.
	RunID     int64
	AccountID int64
	// Source names the system: "gohighlevel", "google_ads", "meta",
	// "manual", "adx". Kind names the thing: "contact", "campaign_day".
	Source string
	Kind   string
	// SourceRef is the source's stable id for the thing, so a re-read lands
	// on the same row.
	SourceRef string
	// Scope is a path for lookups, such as "account:12/campaign:345".
	Scope string
	Class Class
	// EventAt is when it happened at the source, if the source says.
	EventAt      *time.Time
	Payload      any
	Completeness Completeness
}

// Evidence is a stored observation.
type Evidence struct {
	ID             int64           `json:"id"`
	RunID          *int64          `json:"runId"`
	AccountID      *int64          `json:"accountId"`
	Source         string          `json:"source"`
	Kind           string          `json:"kind"`
	SourceRef      string          `json:"sourceRef"`
	Scope          string          `json:"scope"`
	Class          Class           `json:"class"`
	ObservedAt     time.Time       `json:"observedAt"`
	EventAt        *time.Time      `json:"eventAt"`
	PayloadVersion int64           `json:"payloadVersion"`
	Payload        json.RawMessage `json:"payload"`
	Completeness   Completeness    `json:"completeness"`
	RetainUntil    *time.Time      `json:"retainUntil"`
	SupersededByID *int64          `json:"supersededById"`
}

type Filter struct {
	Source    string
	Kind      string
	AccountID int64
	Since     *time.Time
	Limit     int
}

type Store struct {
	db    *store.DB
	clock core.Clock
}

func New(db *store.DB, clock core.Clock) *Store {
	if clock == nil {
		clock = core.SystemClock{}
	}
	return &Store{db: db, clock: clock}
}

// Record stores an observation in the actor's workspace. It returns the row
// and whether a new row was written: false means the same content was
// already there. Jobs record as the system actor in the run's workspace;
// people need at least marketer.
func (s *Store) Record(ctx context.Context, actor core.Actor, o Observation) (Evidence, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Evidence{}, false, core.ErrForbidden
	}
	if o.Source == "" || o.Kind == "" || o.SourceRef == "" {
		return Evidence{}, false, core.Invalid("evidence needs a source, a kind and a source reference")
	}
	switch o.Class {
	case Observed, Estimated, Inferred:
	default:
		return Evidence{}, false, core.Invalid("evidence class must be observed, estimated or inferred")
	}
	if o.Completeness != Complete && o.Completeness != Partial {
		return Evidence{}, false, core.Invalid("evidence completeness must be complete or partial")
	}
	raw, err := json.Marshal(o.Payload)
	if err != nil {
		return Evidence{}, false, err
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	now := s.clock.Now().UTC()
	var (
		row     store.Evidence
		created bool
	)
	err = s.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		latest, err := q.GetLatestEvidence(ctx, store.GetLatestEvidenceParams{WorkspaceID: actor.WorkspaceID, Source: o.Source, Kind: o.Kind, SourceRef: o.SourceRef})
		version := int64(1)
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return err
		case latest.PayloadHash == hash:
			row = latest
			return nil
		default:
			version = latest.PayloadVersion + 1
		}
		row, err = q.CreateEvidence(ctx, store.CreateEvidenceParams{
			WorkspaceID: actor.WorkspaceID, RunID: optionalID(o.RunID), AccountID: optionalID(o.AccountID),
			Source: o.Source, Kind: o.Kind, SourceRef: o.SourceRef, Scope: o.Scope, Class: string(o.Class),
			ObservedAt: now, EventAt: nullTimePtr(o.EventAt), PayloadVersion: version, Payload: string(raw), PayloadHash: hash,
			Completeness: string(o.Completeness),
		})
		if err != nil {
			return err
		}
		created = true
		if version > 1 {
			return q.SupersedeEvidence(ctx, store.SupersedeEvidenceParams{SupersededBy: sql.NullInt64{Int64: row.ID, Valid: true}, ID: latest.ID})
		}
		return nil
	})
	if err != nil {
		return Evidence{}, false, err
	}
	return fromRow(row), created, nil
}

// Lookup returns current (not superseded) evidence in the actor's workspace.
func (s *Store) Lookup(ctx context.Context, actor core.Actor, f Filter) ([]Evidence, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if f.Source == "" || f.Kind == "" {
		return nil, core.Invalid("a source and kind are required")
	}
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 200
	}
	rows, err := s.db.Reader().ListEvidence(ctx, store.ListEvidenceParams{
		WorkspaceID: actor.WorkspaceID, Source: f.Source, Kind: f.Kind, AccountID: optionalID(f.AccountID),
		Since: nullTimePtr(f.Since), Max: int64(f.Limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Evidence, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

// Get is one evidence row in the actor's workspace, for drill-down.
func (s *Store) Get(ctx context.Context, actor core.Actor, id int64) (Evidence, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return Evidence{}, core.ErrForbidden
	}
	row, err := s.db.Reader().GetEvidenceByID(ctx, store.GetEvidenceByIDParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Evidence{}, core.NotFound("evidence %d not found", id)
	}
	if err != nil {
		return Evidence{}, err
	}
	return fromRow(row), nil
}

// Count is how many rows (all versions) exist for a source and kind.
func (s *Store) Count(ctx context.Context, actor core.Actor, source, kind string) (int64, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return 0, core.ErrForbidden
	}
	return s.db.Reader().CountEvidence(ctx, store.CountEvidenceParams{WorkspaceID: actor.WorkspaceID, Source: source, Kind: kind})
}

func fromRow(r store.Evidence) Evidence {
	out := Evidence{
		ID: r.ID, Source: r.Source, Kind: r.Kind, SourceRef: r.SourceRef, Scope: r.Scope, Class: Class(r.Class),
		ObservedAt: r.ObservedAt.UTC(), PayloadVersion: r.PayloadVersion, Payload: json.RawMessage(r.Payload),
		Completeness: Completeness(r.Completeness),
	}
	if r.RunID.Valid {
		out.RunID = &r.RunID.Int64
	}
	if r.AccountID.Valid {
		out.AccountID = &r.AccountID.Int64
	}
	if r.EventAt.Valid {
		t := r.EventAt.Time.UTC()
		out.EventAt = &t
	}
	if r.RetainUntil.Valid {
		t := r.RetainUntil.Time.UTC()
		out.RetainUntil = &t
	}
	if r.SupersededBy.Valid {
		out.SupersededByID = &r.SupersededBy.Int64
	}
	return out
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func nullTimePtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}
