package evaluation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/qualification"
	"github.com/WokeWorks/adx/server/core/voice"
	"github.com/WokeWorks/adx/server/store"
)

// ModelRelease is a candidate model's result against the team's labelled
// calls, and what a person decided about it.
type ModelRelease struct {
	Model       string     `json:"model"`
	State       string     `json:"state"`
	Labels      int        `json:"labels"`
	Agreements  int        `json:"agreements"`
	Agreement   *float64   `json:"agreement"`
	Note        string     `json:"note"`
	EvaluatedAt time.Time  `json:"evaluatedAt"`
	DecidedAt   *time.Time `json:"decidedAt"`
}

// ReleaseReport is the operations view of model releases: what runs, what
// is approved, and whether they differ.
type ReleaseReport struct {
	Running      string         `json:"running"`
	Approved     *ModelRelease  `json:"approved"`
	Releases     []ModelRelease `json:"releases"`
	Requirements Requirements   `json:"requirements"`
	// Warning is set when the running model is not the approved one.
	Warning string `json:"warning"`
}

// Releases lists release evaluations for the actor's workspace.
func (e *Evaluation) Releases(ctx context.Context, actor core.Actor) (ReleaseReport, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return ReleaseReport{}, core.ErrForbidden
	}
	rows, err := e.db.Reader().ListModelReleases(ctx, actor.WorkspaceID)
	if err != nil {
		return ReleaseReport{}, err
	}
	r := ReleaseReport{Running: e.running, Releases: []ModelRelease{}, Requirements: e.req}
	for _, row := range rows {
		rel := releaseFromRow(row)
		r.Releases = append(r.Releases, rel)
		if rel.State == "approved" {
			approved := rel
			r.Approved = &approved
		}
	}
	switch {
	case e.running == "":
		r.Warning = "no model is configured on this server"
	case r.Approved == nil:
		r.Warning = "no release is approved; the running model " + e.running + " has not been evaluated against labelled calls"
	case r.Approved.Model != e.running:
		r.Warning = fmt.Sprintf("the running model %s is not the approved release %s", e.running, r.Approved.Model)
	}
	return r, nil
}

// EvaluateRelease replays every labelled call's transcript through the named
// model with the same reading prompt live calls use, and records how often
// its outcome agreed with the team's label. It writes nothing to any call:
// the replay is a measurement. The result is a candidate; nothing is
// promoted here.
func (e *Evaluation) EvaluateRelease(ctx context.Context, actor core.Actor, model string) (ModelRelease, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return ModelRelease{}, core.ErrForbidden
	}
	model = strings.TrimSpace(model)
	if model == "" || len(model) > 80 {
		return ModelRelease{}, core.Invalid("model name is required")
	}
	if e.models == nil {
		return ModelRelease{}, core.Unavailable(errors.New("no model factory"), "release evaluation needs a configured model provider")
	}
	candidate, err := e.models(model)
	if err != nil {
		return ModelRelease{}, core.Invalid("model %q cannot be built: %s", model, err.Error())
	}
	labels, err := e.db.Reader().ListQualificationLabels(ctx, actor.WorkspaceID)
	if err != nil {
		return ModelRelease{}, err
	}
	sessions, err := e.db.Reader().ListEndedCallSessions(ctx, actor.WorkspaceID)
	if err != nil {
		return ModelRelease{}, err
	}
	byCall := map[int64]store.CallSession{}
	for _, s := range sessions {
		byCall[s.CallID] = s
	}
	replayed, agreements, skipped := 0, 0, 0
	for _, l := range labels {
		s, ok := byCall[l.CallID]
		if !ok {
			skipped++
			continue
		}
		var segs []voice.Segment
		_ = json.Unmarshal([]byte(s.Transcript), &segs)
		if len(segs) == 0 {
			skipped++
			continue
		}
		reading, err := qualification.ReadTranscript(ctx, candidate, segs)
		if err != nil {
			var re *qualification.ReadError
			if errors.As(err, &re) {
				replayed++ // an unusable reading is a disagreement, not a skip
				continue
			}
			return ModelRelease{}, core.Unavailable(err, "the candidate model did not answer; nothing was recorded")
		}
		replayed++
		if reading.Outcome == l.Outcome {
			agreements++
		}
	}
	var agreement sql.NullFloat64
	if replayed > 0 {
		agreement = sql.NullFloat64{Float64: float64(agreements) / float64(replayed), Valid: true}
	}
	note := fmt.Sprintf("%d labelled calls replayed, %d skipped without a transcript", replayed, skipped)
	row, err := e.db.Writer().UpsertModelRelease(ctx, store.UpsertModelReleaseParams{WorkspaceID: actor.WorkspaceID, Model: model, Labels: int64(replayed), Agreements: int64(agreements),
		Agreement: agreement, Note: note, EvaluatedBy: optionalID(actor.UserID), EvaluatedAt: e.clock.Now().UTC()})
	if err != nil {
		return ModelRelease{}, err
	}
	return releaseFromRow(row), nil
}

// Promote approves a candidate. It refuses a candidate with too few labelled
// calls, agreement below the requirement, or agreement below the approved
// release's: a regression never becomes the approved model, and nothing
// here is automatic. The previous approved release is superseded.
func (e *Evaluation) Promote(ctx context.Context, actor core.Actor, model string) (ModelRelease, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant || actor.UserID == 0 {
		return ModelRelease{}, core.ErrForbidden
	}
	row, err := e.db.Reader().GetModelRelease(ctx, store.GetModelReleaseParams{WorkspaceID: actor.WorkspaceID, Model: model})
	if errors.Is(err, sql.ErrNoRows) {
		return ModelRelease{}, core.NotFound("model %q has not been evaluated", model)
	}
	if err != nil {
		return ModelRelease{}, err
	}
	if row.State != "candidate" {
		return ModelRelease{}, core.Conflict("model %q is %s, not a candidate", model, row.State)
	}
	if int(row.Labels) < e.req.MinLabels {
		return ModelRelease{}, core.Conflict("model %q was measured on %d labelled calls; %d are needed", model, row.Labels, e.req.MinLabels)
	}
	if !row.Agreement.Valid || row.Agreement.Float64 < e.req.MinAgreement {
		return ModelRelease{}, core.Conflict("model %q agrees with the labels on %s of calls; at least %s is required", model, pct(row.Agreement), ftoa(e.req.MinAgreement))
	}
	rows, err := e.db.Reader().ListModelReleases(ctx, actor.WorkspaceID)
	if err != nil {
		return ModelRelease{}, err
	}
	var current *store.ModelRelease
	for i := range rows {
		if rows[i].State == "approved" {
			current = &rows[i]
		}
	}
	if current != nil && current.Agreement.Valid && row.Agreement.Float64 < current.Agreement.Float64 {
		return ModelRelease{}, core.Conflict("regression: %s agrees on %s of calls, the approved %s on %s; it is not promoted", model, pct(row.Agreement), current.Model, pct(current.Agreement))
	}
	now := e.clock.Now().UTC()
	err = e.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		if current != nil {
			if err := q.SetModelReleaseState(ctx, store.SetModelReleaseStateParams{State: "superseded", DecidedBy: optionalID(actor.UserID), DecidedAt: sql.NullTime{Time: now, Valid: true}, WorkspaceID: actor.WorkspaceID, Model: current.Model}); err != nil {
				return err
			}
		}
		return q.SetModelReleaseState(ctx, store.SetModelReleaseStateParams{State: "approved", DecidedBy: optionalID(actor.UserID), DecidedAt: sql.NullTime{Time: now, Valid: true}, WorkspaceID: actor.WorkspaceID, Model: model})
	})
	if err != nil {
		return ModelRelease{}, err
	}
	row, err = e.db.Reader().GetModelRelease(ctx, store.GetModelReleaseParams{WorkspaceID: actor.WorkspaceID, Model: model})
	if err != nil {
		return ModelRelease{}, err
	}
	return releaseFromRow(row), nil
}

// Reject marks a candidate rejected. Admins only.
func (e *Evaluation) Reject(ctx context.Context, actor core.Actor, model string) (ModelRelease, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return ModelRelease{}, core.ErrForbidden
	}
	row, err := e.db.Reader().GetModelRelease(ctx, store.GetModelReleaseParams{WorkspaceID: actor.WorkspaceID, Model: model})
	if errors.Is(err, sql.ErrNoRows) {
		return ModelRelease{}, core.NotFound("model %q has not been evaluated", model)
	}
	if err != nil {
		return ModelRelease{}, err
	}
	if row.State != "candidate" {
		return ModelRelease{}, core.Conflict("model %q is %s, not a candidate", model, row.State)
	}
	now := e.clock.Now().UTC()
	if err := e.db.Writer().SetModelReleaseState(ctx, store.SetModelReleaseStateParams{State: "rejected", DecidedBy: optionalID(actor.UserID), DecidedAt: sql.NullTime{Time: now, Valid: true}, WorkspaceID: actor.WorkspaceID, Model: model}); err != nil {
		return ModelRelease{}, err
	}
	row.State = "rejected"
	row.DecidedAt = sql.NullTime{Time: now, Valid: true}
	return releaseFromRow(row), nil
}

func releaseFromRow(r store.ModelRelease) ModelRelease {
	m := ModelRelease{Model: r.Model, State: r.State, Labels: int(r.Labels), Agreements: int(r.Agreements), Note: r.Note, EvaluatedAt: r.EvaluatedAt}
	if r.Agreement.Valid {
		a := r.Agreement.Float64
		m.Agreement = &a
	}
	if r.DecidedAt.Valid {
		d := r.DecidedAt.Time
		m.DecidedAt = &d
	}
	return m
}

func pct(a sql.NullFloat64) string {
	if !a.Valid {
		return "an unknown share"
	}
	return fmt.Sprintf("%d%%", int(a.Float64*100+0.5))
}

// ModelFactory builds a model client by name, so a candidate release can be
// evaluated before anyone changes ADX_MODEL_NAME.
type ModelFactory func(name string) (assistant.Model, error)
