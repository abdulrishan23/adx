// Package evaluation measures the lead loop against what people say
// happened: qualification readings against agreed labels, how often
// readings were corrected, how alerts were handled, and the feedback the
// team recorded. Predictive claims stay off until the evidence is there.
package evaluation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

// Requirements are what a predictive claim needs before it is allowed. They
// are deliberately plain: enough labelled calls, and agreement high enough.
type Requirements struct {
	MinLabels    int     `json:"minLabels"`
	MinAgreement float64 `json:"minAgreement"`
}

type QualificationLabel struct {
	ID        int64             `json:"id"`
	CallID    int64             `json:"callId"`
	Outcome   string            `json:"outcome"`
	Answers   map[string]string `json:"answers"`
	Note      string            `json:"note"`
	LabeledBy *int64            `json:"labeledBy"`
	LabeledAt time.Time         `json:"labeledAt"`
}

type LabelInput struct {
	CallID  int64             `json:"callId"`
	Outcome string            `json:"outcome"`
	Answers map[string]string `json:"answers"`
	Note    string            `json:"note"`
}

type QualificationQuality struct {
	Sessions       int      `json:"sessions"`
	Read           int      `json:"read"`
	Pending        int      `json:"pending"`
	Labeled        int      `json:"labeled"`
	Agreements     int      `json:"agreements"`
	AgreementRate  *float64 `json:"agreementRate"`
	Corrections    int      `json:"corrections"`
	CorrectionRate *float64 `json:"correctionRate"`
	Note           string   `json:"note"`
}

type AlertQuality struct {
	Total        int      `json:"total"`
	Open         int      `json:"open"`
	Acknowledged int      `json:"acknowledged"`
	Resolved     int      `json:"resolved"`
	ResolvedRate *float64 `json:"resolvedRate"`
	RaisedLast7d int      `json:"raisedLast7Days"`
}

type Feedback struct {
	Objections         int `json:"objections"`
	ActiveSuppressions int `json:"activeSuppressions"`
	DraftsHeld         int `json:"draftsHeld"`
	DraftsSent         int `json:"draftsSent"`
	FieldConflicts     int `json:"fieldConflicts"`
	SignalCorrections  int `json:"signalCorrections"`
}

type PredictiveGate struct {
	Enabled      bool         `json:"enabled"`
	Reason       string       `json:"reason"`
	Requirements Requirements `json:"requirements"`
}

type EvaluationReport struct {
	Qualification QualificationQuality `json:"qualification"`
	Alerts        AlertQuality         `json:"alerts"`
	Feedback      Feedback             `json:"feedback"`
	Predictive    PredictiveGate       `json:"predictive"`
	Labels        []QualificationLabel `json:"labels"`
	GeneratedAt   time.Time            `json:"generatedAt"`
}

type Config struct {
	Clock        core.Clock
	Requirements Requirements
	// Models builds a candidate model by name for release evaluation. Nil
	// means releases cannot be evaluated on this server.
	Models ModelFactory
	// Running is the model this server is configured with.
	Running string
}

type Evaluation struct {
	db      *store.DB
	clock   core.Clock
	req     Requirements
	models  ModelFactory
	running string
}

func New(db *store.DB, cfg Config) *Evaluation {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	if cfg.Requirements.MinLabels == 0 {
		cfg.Requirements.MinLabels = 50
	}
	if cfg.Requirements.MinAgreement == 0 {
		cfg.Requirements.MinAgreement = 0.8
	}
	return &Evaluation{db: db, clock: cfg.Clock, req: cfg.Requirements, models: cfg.Models, running: cfg.Running}
}

// Label records the team's agreed reading of a call. One label per call;
// a new one replaces the old. It never changes the call or the enquiry:
// that is a correction, which is the qualification domain's.
func (e *Evaluation) Label(ctx context.Context, actor core.Actor, in LabelInput) (QualificationLabel, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return QualificationLabel{}, core.ErrForbidden
	}
	in.Outcome = strings.TrimSpace(in.Outcome)
	if in.Outcome == "" {
		return QualificationLabel{}, core.Invalid("an outcome is required")
	}
	if _, err := e.db.Reader().GetCallSession(ctx, store.GetCallSessionParams{CallID: in.CallID, WorkspaceID: actor.WorkspaceID}); errors.Is(err, sql.ErrNoRows) {
		return QualificationLabel{}, core.NotFound("call %d has no conversation to label", in.CallID)
	} else if err != nil {
		return QualificationLabel{}, err
	}
	if in.Answers == nil {
		in.Answers = map[string]string{}
	}
	answers, _ := json.Marshal(in.Answers)
	row, err := e.db.Writer().UpsertQualificationLabel(ctx, store.UpsertQualificationLabelParams{WorkspaceID: actor.WorkspaceID, CallID: in.CallID, Outcome: in.Outcome, Answers: string(answers), Note: strings.TrimSpace(in.Note), LabeledBy: optionalID(actor.UserID), LabeledAt: e.clock.Now().UTC()})
	if err != nil {
		return QualificationLabel{}, err
	}
	return labelFromRow(row), nil
}

// Report computes the figures. Rates are nil, not zero, when there is
// nothing to divide by.
func (e *Evaluation) Report(ctx context.Context, actor core.Actor) (EvaluationReport, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return EvaluationReport{}, core.ErrForbidden
	}
	ws := actor.WorkspaceID
	r := EvaluationReport{GeneratedAt: e.clock.Now().UTC(), Labels: []QualificationLabel{}}
	sessions, err := e.db.Reader().ListEndedCallSessions(ctx, ws)
	if err != nil {
		return r, err
	}
	labels, err := e.db.Reader().ListQualificationLabels(ctx, ws)
	if err != nil {
		return r, err
	}
	labelFor := map[int64]store.QualificationLabel{}
	for _, l := range labels {
		labelFor[l.CallID] = l
		r.Labels = append(r.Labels, labelFromRow(l))
	}
	q := &r.Qualification
	q.Sessions = len(sessions)
	for _, s := range sessions {
		if s.ReviewStatus == "pending" {
			q.Pending++
		} else if s.Outcome != "" {
			q.Read++
		}
		if l, ok := labelFor[s.CallID]; ok {
			q.Labeled++
			// The model's own reading is what is measured. A corrected
			// session's current outcome is a person's, so the comparison
			// uses the first reading when a correction exists: the
			// correction row keeps the person's, the session keeps the
			// model's summary but the outcome field was overwritten, so the
			// model's outcome is recovered from the applied detail of v1
			// when present; otherwise the current outcome stands.
			if modelOutcome(s) == l.Outcome {
				q.Agreements++
			}
		}
	}
	if q.Labeled > 0 {
		rate := float64(q.Agreements) / float64(q.Labeled)
		q.AgreementRate = &rate
	}
	corrections, err := e.db.Reader().CountCallCorrections(ctx, ws)
	if err != nil {
		return r, err
	}
	q.Corrections = int(corrections)
	if q.Sessions > 0 {
		rate := float64(q.Corrections) / float64(q.Sessions)
		q.CorrectionRate = &rate
	}
	if q.Labeled == 0 {
		q.Note = "No labelled calls yet: accuracy is not available."
	}

	states, err := e.db.Reader().CountAlertsByState(ctx, ws)
	if err != nil {
		return r, err
	}
	for _, st := range states {
		n := int(st.N)
		r.Alerts.Total += n
		switch st.State {
		case "open":
			r.Alerts.Open = n
		case "acknowledged":
			r.Alerts.Acknowledged = n
		case "resolved":
			r.Alerts.Resolved = n
		}
	}
	if r.Alerts.Total > 0 {
		rate := float64(r.Alerts.Resolved) / float64(r.Alerts.Total)
		r.Alerts.ResolvedRate = &rate
	}
	recent, err := e.db.Reader().CountAlertsSince(ctx, store.CountAlertsSinceParams{WorkspaceID: ws, FirstSeenAt: e.clock.Now().UTC().Add(-7 * 24 * time.Hour)})
	if err != nil {
		return r, err
	}
	r.Alerts.RaisedLast7d = int(recent)

	obj, err := e.db.Reader().CountObjections(ctx, ws)
	if err != nil {
		return r, err
	}
	r.Feedback.Objections = int(obj)
	sup, err := e.db.Reader().CountActiveSuppressions(ctx, ws)
	if err != nil {
		return r, err
	}
	r.Feedback.ActiveSuppressions = int(sup)
	drafts, err := e.db.Reader().CountMessageDraftsByState(ctx, ws)
	if err != nil {
		return r, err
	}
	for _, d := range drafts {
		switch d.State {
		case "held", "suppressed":
			r.Feedback.DraftsHeld += int(d.N)
		case "sent":
			r.Feedback.DraftsSent += int(d.N)
		}
	}
	fields, err := e.db.Reader().CountFieldOutboxByState(ctx, ws)
	if err != nil {
		return r, err
	}
	for _, f := range fields {
		if f.State == "conflict" {
			r.Feedback.FieldConflicts += int(f.N)
		}
	}
	sc, err := e.db.Reader().CountSignalCorrections(ctx, ws)
	if err != nil {
		return r, err
	}
	r.Feedback.SignalCorrections = int(sc)

	r.Predictive = e.gate(q)
	return r, nil
}

// gate decides whether ADX may make predictive claims (a lead "will
// convert", a score "predicts" a sale). It needs enough labels and enough
// agreement; the reason says what is missing.
func (e *Evaluation) gate(q *QualificationQuality) PredictiveGate {
	g := PredictiveGate{Requirements: e.req}
	switch {
	case q.Labeled < e.req.MinLabels:
		g.Reason = "disabled: " + itoa(q.Labeled) + " labelled calls; " + itoa(e.req.MinLabels) + " are needed before any predictive claim"
	case q.AgreementRate == nil || *q.AgreementRate < e.req.MinAgreement:
		g.Reason = "disabled: agreement with labels is below the required " + ftoa(e.req.MinAgreement)
	default:
		g.Enabled = true
		g.Reason = "enabled: " + itoa(q.Labeled) + " labelled calls with agreement " + ftoa(*q.AgreementRate)
	}
	return g
}

// Predictive is the gate alone, for other domains that want to phrase a
// claim and must check first.
func (e *Evaluation) Predictive(ctx context.Context, actor core.Actor) (PredictiveGate, error) {
	r, err := e.Report(ctx, actor)
	if err != nil {
		return PredictiveGate{}, err
	}
	return r.Predictive, nil
}

// modelOutcome recovers the model's reading for a session. The session's
// outcome field is the current reading, which a correction overwrites, so
// the first application's record is preferred; it carries the outcome the
// model gave.
func modelOutcome(s store.CallSession) string {
	if s.ReviewStatus != "corrected" {
		return s.Outcome
	}
	var d struct {
		ModelOutcome string `json:"modelOutcome"`
	}
	if json.Unmarshal([]byte(s.AppliedDetail), &d) == nil && d.ModelOutcome != "" {
		return d.ModelOutcome
	}
	return s.Outcome
}

func labelFromRow(r store.QualificationLabel) QualificationLabel {
	l := QualificationLabel{ID: r.ID, CallID: r.CallID, Outcome: r.Outcome, Note: r.Note, LabeledBy: idPtr(r.LabeledBy), LabeledAt: r.LabeledAt.UTC(), Answers: map[string]string{}}
	_ = json.Unmarshal([]byte(r.Answers), &l.Answers)
	return l
}

func itoa(n int) string { return strconv.Itoa(n) }

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
