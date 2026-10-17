// Package learning is ADX's memory of advice and what came of it: a record
// per recommendation with the person's decision (adopted, modified,
// ignored), the action that followed, and the outcomes observed later,
// attached to the intervention whose window covers them. Insights are
// statements that climb from hypothesis to supported to validated only
// through observed outcomes and a controlled comparison; a model agreeing
// is never evidence, and an attributed improvement is never called causal.
package learning

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

type LearningRecord struct {
	ID                int64             `json:"id"`
	RecommendationRef string            `json:"recommendationRef"`
	Source            string            `json:"source"`
	SubjectKind       string            `json:"subjectKind"`
	SubjectRef        string            `json:"subjectRef"`
	Recommended       json.RawMessage   `json:"recommended"`
	Decision          string            `json:"decision"`
	DecisionNote      string            `json:"decisionNote"`
	DecidedBy         *int64            `json:"decidedBy"`
	DecidedAt         *time.Time        `json:"decidedAt"`
	ActionRef         string            `json:"actionRef"`
	ActionAt          *time.Time        `json:"actionAt"`
	WindowDays        int64             `json:"windowDays"`
	CreatedAt         time.Time         `json:"createdAt"`
	Outcomes          []LearningOutcome `json:"outcomes"`
}

type LearningRecordInput struct {
	RecommendationRef string          `json:"recommendationRef"`
	Source            string          `json:"source"`
	SubjectKind       string          `json:"subjectKind"`
	SubjectRef        string          `json:"subjectRef"`
	Recommended       json.RawMessage `json:"recommended"`
	WindowDays        int             `json:"windowDays"`
}

type LearningDecisionInput struct {
	Decision  string    `json:"decision"` // adopted, modified, ignored
	Note      string    `json:"note"`
	ActionRef string    `json:"actionRef"`
	ActionAt  time.Time `json:"actionAt"`
}

type LearningOutcome struct {
	ID          int64     `json:"id"`
	RecordID    *int64    `json:"recordId"`
	SubjectKind string    `json:"subjectKind"`
	SubjectRef  string    `json:"subjectRef"`
	Metric      string    `json:"metric"`
	Value       float64   `json:"value"`
	Unit        string    `json:"unit"`
	Class       string    `json:"class"`
	ObservedAt  time.Time `json:"observedAt"`
	EvidenceID  *int64    `json:"evidenceId"`
	Note        string    `json:"note"`
}

type LearningOutcomeInput struct {
	SubjectKind string    `json:"subjectKind"`
	SubjectRef  string    `json:"subjectRef"`
	Metric      string    `json:"metric"`
	Value       float64   `json:"value"`
	Unit        string    `json:"unit"`
	Class       string    `json:"class"` // observed, attributed, control
	ObservedAt  time.Time `json:"observedAt"`
	EvidenceID  int64     `json:"evidenceId"`
	Note        string    `json:"note"`
}

// SupportItem backs an insight. Kind "record" is a learning record with
// observed outcomes; "model_agreement" is a model saying so, which is kept
// for the record and never counted.
type SupportItem struct {
	Kind     string `json:"kind"`
	RecordID int64  `json:"recordId"`
	Note     string `json:"note"`
}

type MemoryInsight struct {
	ID                int64         `json:"id"`
	Lineage           int64         `json:"lineage"`
	Version           int64         `json:"version"`
	Statement         string        `json:"statement"`
	Scope             string        `json:"scope"`
	Class             string        `json:"class"`
	Causal            bool          `json:"causal"`
	Attributed        bool          `json:"attributed"`
	Support           []SupportItem `json:"support"`
	SupportingRecords int           `json:"supportingRecords"`
	Source            string        `json:"source"`
	ControlEvidenceID *int64        `json:"controlEvidenceId"`
	CreatedBy         *int64        `json:"createdBy"`
	CreatedAt         time.Time     `json:"createdAt"`
	PromotedBy        *int64        `json:"promotedBy"`
	PromotedAt        *time.Time    `json:"promotedAt"`
	RetiredAt         *time.Time    `json:"retiredAt"`
}

type MemoryInsightInput struct {
	Statement string        `json:"statement"`
	Scope     string        `json:"scope"`
	Support   []SupportItem `json:"support"`
}

// MemoryFinding is what other domains receive from memory: a validated or
// supported statement with its lineage, so a brief can cite it and a reader
// can trace it.
type MemoryFinding struct {
	Statement string `json:"statement"`
	Class     string `json:"class"`
	Causal    bool   `json:"causal"`
	Lineage   int64  `json:"lineage"`
	Version   int64  `json:"version"`
	Scope     string `json:"scope"`
}

type LearningReport struct {
	Records       int            `json:"records"`
	Decisions     map[string]int `json:"decisions"`
	Interventions int            `json:"interventions"`
	Ignored       int            `json:"ignored"`
	Outcomes      int            `json:"outcomes"`
	Unattached    int            `json:"unattachedOutcomes"`
	Insights      map[string]int `json:"insights"`
	Note          string         `json:"note"`
}

// PromotionRequirements for promotion.
type PromotionRequirements struct {
	// MinRecords with observed outcomes before a hypothesis is supported.
	MinRecords int `json:"minRecords"`
}

type Config struct {
	Clock        core.Clock
	Requirements PromotionRequirements
}

type Learning struct {
	db    *store.DB
	clock core.Clock
	req   PromotionRequirements
}

func New(db *store.DB, cfg Config) *Learning {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	if cfg.Requirements.MinRecords == 0 {
		cfg.Requirements.MinRecords = 3
	}
	return &Learning{db: db, clock: cfg.Clock, req: cfg.Requirements}
}

// LearningRecord opens a learning record for a recommendation, once per reference.
func (l *Learning) Record(ctx context.Context, actor core.Actor, in LearningRecordInput) (LearningRecord, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return LearningRecord{}, false, core.ErrForbidden
	}
	in.RecommendationRef = strings.TrimSpace(in.RecommendationRef)
	if in.RecommendationRef == "" || strings.TrimSpace(in.SubjectKind) == "" || strings.TrimSpace(in.SubjectRef) == "" {
		return LearningRecord{}, false, core.Invalid("recommendationRef, subjectKind and subjectRef are required")
	}
	if in.WindowDays <= 0 {
		in.WindowDays = 14
	}
	if len(in.Recommended) == 0 {
		in.Recommended = json.RawMessage(`{}`)
	}
	if !json.Valid(in.Recommended) {
		return LearningRecord{}, false, core.Invalid("recommended must be JSON")
	}
	if existing, err := l.db.Reader().GetLearningRecordByRef(ctx, store.GetLearningRecordByRefParams{WorkspaceID: actor.WorkspaceID, RecommendationRef: in.RecommendationRef}); err == nil {
		r, err := l.record(ctx, existing)
		return r, false, err
	}
	row, err := l.db.Writer().CreateLearningRecord(ctx, store.CreateLearningRecordParams{WorkspaceID: actor.WorkspaceID, RecommendationRef: in.RecommendationRef, Source: in.Source, SubjectKind: in.SubjectKind, SubjectRef: in.SubjectRef, Recommended: string(in.Recommended), WindowDays: int64(in.WindowDays), CreatedAt: l.clock.Now().UTC()})
	if err != nil {
		if existing, e2 := l.db.Reader().GetLearningRecordByRef(ctx, store.GetLearningRecordByRefParams{WorkspaceID: actor.WorkspaceID, RecommendationRef: in.RecommendationRef}); e2 == nil {
			r, err := l.record(ctx, existing)
			return r, false, err
		}
		return LearningRecord{}, false, err
	}
	r, err := l.record(ctx, row)
	return r, true, err
}

// Decide records what a person did with the advice. Adopted and modified
// name the action that followed and when; ignored names none, and nothing
// is ever attributed to it.
func (l *Learning) Decide(ctx context.Context, actor core.Actor, id int64, in LearningDecisionInput) (LearningRecord, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return LearningRecord{}, core.ErrForbidden
	}
	switch in.Decision {
	case "adopted", "modified":
		if strings.TrimSpace(in.ActionRef) == "" {
			return LearningRecord{}, core.Invalid("%s needs the action that followed (actionRef)", in.Decision)
		}
		if in.ActionAt.IsZero() {
			in.ActionAt = l.clock.Now()
		}
	case "ignored":
		if strings.TrimSpace(in.ActionRef) != "" {
			return LearningRecord{}, core.Invalid("ignored advice has no action; record it as modified if something else was done")
		}
	default:
		return LearningRecord{}, core.Invalid("decision must be adopted, modified or ignored")
	}
	if _, err := l.db.Reader().GetLearningRecord(ctx, store.GetLearningRecordParams{ID: id, WorkspaceID: actor.WorkspaceID}); errors.Is(err, sql.ErrNoRows) {
		return LearningRecord{}, core.NotFound("learning record %d not found", id)
	} else if err != nil {
		return LearningRecord{}, err
	}
	now := l.clock.Now().UTC()
	var actionAt sql.NullTime
	if in.Decision != "ignored" {
		actionAt = sql.NullTime{Time: in.ActionAt.UTC(), Valid: true}
	}
	n, err := l.db.Writer().DecideLearningRecord(ctx, store.DecideLearningRecordParams{Decision: in.Decision, DecisionNote: strings.TrimSpace(in.Note), DecidedBy: optionalID(actor.UserID), DecidedAt: sql.NullTime{Time: now, Valid: true}, ActionRef: strings.TrimSpace(in.ActionRef), ActionAt: actionAt, ID: id})
	if err != nil {
		return LearningRecord{}, err
	}
	if n != 1 {
		return LearningRecord{}, core.Conflict("record %d was already decided", id)
	}
	row, err := l.db.Reader().GetLearningRecord(ctx, store.GetLearningRecordParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return LearningRecord{}, err
	}
	return l.record(ctx, row)
}

// Observe stores a delayed outcome and attaches it to the most recent acted
// record on the same subject whose window covers the observation. Ignored
// advice has no action and so never receives one; an outcome with no such
// record stays unattached and is reported as such.
func (l *Learning) Observe(ctx context.Context, actor core.Actor, in LearningOutcomeInput) (LearningOutcome, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return LearningOutcome{}, core.ErrForbidden
	}
	if strings.TrimSpace(in.Metric) == "" || strings.TrimSpace(in.SubjectKind) == "" || strings.TrimSpace(in.SubjectRef) == "" {
		return LearningOutcome{}, core.Invalid("subjectKind, subjectRef and metric are required")
	}
	switch in.Class {
	case "":
		in.Class = "observed"
	case "observed", "attributed", "control":
	default:
		return LearningOutcome{}, core.Invalid("class must be observed, attributed or control")
	}
	if in.ObservedAt.IsZero() {
		in.ObservedAt = l.clock.Now()
	}
	at := in.ObservedAt.UTC()
	records, err := l.db.Reader().ListActedRecordsForSubject(ctx, store.ListActedRecordsForSubjectParams{WorkspaceID: actor.WorkspaceID, SubjectKind: in.SubjectKind, SubjectRef: in.SubjectRef})
	if err != nil {
		return LearningOutcome{}, err
	}
	var recordID sql.NullInt64
	for _, r := range records { // most recent action first
		start := r.ActionAt.Time.UTC()
		end := start.Add(time.Duration(r.WindowDays) * 24 * time.Hour)
		if !at.Before(start) && at.Before(end) {
			recordID = sql.NullInt64{Int64: r.ID, Valid: true}
			break
		}
	}
	row, err := l.db.Writer().CreateLearningOutcome(ctx, store.CreateLearningOutcomeParams{WorkspaceID: actor.WorkspaceID, RecordID: recordID, SubjectKind: in.SubjectKind, SubjectRef: in.SubjectRef, Metric: in.Metric, Value: in.Value, Unit: in.Unit, Class: in.Class, ObservedAt: at, EvidenceID: optionalID(in.EvidenceID), Note: strings.TrimSpace(in.Note), CreatedAt: l.clock.Now().UTC()})
	if err != nil {
		return LearningOutcome{}, err
	}
	return outcomeFromRow(row), nil
}

func (l *Learning) Get(ctx context.Context, actor core.Actor, id int64) (LearningRecord, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return LearningRecord{}, core.ErrForbidden
	}
	row, err := l.db.Reader().GetLearningRecord(ctx, store.GetLearningRecordParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return LearningRecord{}, core.NotFound("learning record %d not found", id)
	}
	if err != nil {
		return LearningRecord{}, err
	}
	return l.record(ctx, row)
}

func (l *Learning) List(ctx context.Context, actor core.Actor, limit int) ([]LearningRecord, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := l.db.Reader().ListLearningRecords(ctx, store.ListLearningRecordsParams{WorkspaceID: actor.WorkspaceID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]LearningRecord, 0, len(rows))
	for _, r := range rows {
		rec, err := l.record(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// ProposeInsight opens a hypothesis. A model's statement is marked as such;
// its own agreement is not support.
func (l *Learning) ProposeInsight(ctx context.Context, actor core.Actor, in MemoryInsightInput) (MemoryInsight, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return MemoryInsight{}, core.ErrForbidden
	}
	in.Statement = strings.TrimSpace(in.Statement)
	if in.Statement == "" {
		return MemoryInsight{}, core.Invalid("a statement is required")
	}
	source := "human"
	if actor.Via == core.ViaAssistant || actor.UserID == 0 {
		source = "model"
	}
	for _, s := range in.Support {
		if s.Kind != "record" && s.Kind != "model_agreement" {
			return MemoryInsight{}, core.Invalid("support kind must be record or model_agreement")
		}
		if s.Kind == "record" {
			if _, err := l.db.Reader().GetLearningRecord(ctx, store.GetLearningRecordParams{ID: s.RecordID, WorkspaceID: actor.WorkspaceID}); err != nil {
				return MemoryInsight{}, core.Invalid("support names learning record %d, which does not exist", s.RecordID)
			}
		}
	}
	support, _ := json.Marshal(nonNil(in.Support))
	var out MemoryInsight
	err := l.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		lineage, err := q.NextInsightLineage(ctx, actor.WorkspaceID)
		if err != nil {
			return err
		}
		row, err := q.CreateInsight(ctx, store.CreateInsightParams{WorkspaceID: actor.WorkspaceID, Lineage: lineage, Version: 1, Statement: in.Statement, Scope: strings.TrimSpace(in.Scope), Class: "hypothesis", Causal: 0, Support: string(support), Source: source, CreatedBy: optionalID(actor.UserID), CreatedAt: l.clock.Now().UTC()})
		if err != nil {
			return err
		}
		out, err = l.insight(ctx, row)
		return err
	})
	return out, err
}

// Promote moves an insight up one class under the rules: to supported with
// at least MinRecords supporting records that have observed outcomes on an
// adopted or modified intervention; to validated only from supported and
// only with controlled-comparison evidence. Model agreement counts for
// nothing, and a model's actor cannot promote.
func (l *Learning) Promote(ctx context.Context, actor core.Actor, id int64, to string, controlEvidenceID int64) (MemoryInsight, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return MemoryInsight{}, core.ErrForbidden
	}
	row, err := l.db.Reader().GetInsight(ctx, store.GetInsightParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return MemoryInsight{}, core.NotFound("insight %d not found", id)
	}
	if err != nil {
		return MemoryInsight{}, err
	}
	cur, err := l.insight(ctx, row)
	if err != nil {
		return MemoryInsight{}, err
	}
	causal := int64(0)
	var control sql.NullInt64
	switch to {
	case "supported":
		if cur.Class != "hypothesis" {
			return MemoryInsight{}, core.Conflict("insight %d is %s; only a hypothesis becomes supported", id, cur.Class)
		}
		if cur.SupportingRecords < l.req.MinRecords {
			return MemoryInsight{}, core.Invalid("supported needs %d learning records with observed outcomes on an adopted or modified intervention; %d qualify (model agreement does not count)", l.req.MinRecords, cur.SupportingRecords)
		}
	case "validated":
		if cur.Class != "supported" {
			return MemoryInsight{}, core.Conflict("insight %d is %s; only a supported insight becomes validated", id, cur.Class)
		}
		if controlEvidenceID == 0 {
			return MemoryInsight{}, core.Invalid("validated needs controlled-comparison evidence; an attributed improvement is not causal lift")
		}
		ev, err := l.db.Reader().GetEvidenceByID(ctx, store.GetEvidenceByIDParams{ID: controlEvidenceID, WorkspaceID: actor.WorkspaceID})
		if err != nil {
			return MemoryInsight{}, core.Invalid("evidence %d not found", controlEvidenceID)
		}
		if ev.Class != "observed" || !strings.Contains(ev.Kind, "control") {
			return MemoryInsight{}, core.Invalid("evidence %d is %s %s; validation needs an observed controlled comparison", controlEvidenceID, ev.Class, ev.Kind)
		}
		causal = 1
		control = sql.NullInt64{Int64: controlEvidenceID, Valid: true}
	default:
		return MemoryInsight{}, core.Invalid("to must be supported or validated")
	}
	now := l.clock.Now().UTC()
	n, err := l.db.Writer().PromoteInsight(ctx, store.PromoteInsightParams{Class: to, Causal: causal, ControlEvidenceID: control, PromotedBy: optionalID(actor.UserID), PromotedAt: sql.NullTime{Time: now, Valid: true}, ID: id, Class_2: cur.Class})
	if err != nil {
		return MemoryInsight{}, err
	}
	if n != 1 {
		return MemoryInsight{}, core.Conflict("insight %d changed underneath", id)
	}
	row, err = l.db.Reader().GetInsight(ctx, store.GetInsightParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return MemoryInsight{}, err
	}
	return l.insight(ctx, row)
}

// Revise writes a new version in the same lineage and retires the old one.
// The new version starts as a hypothesis: a changed statement is a new
// claim, however good the old evidence was.
func (l *Learning) Revise(ctx context.Context, actor core.Actor, id int64, statement, scope string) (MemoryInsight, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return MemoryInsight{}, core.ErrForbidden
	}
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return MemoryInsight{}, core.Invalid("a statement is required")
	}
	row, err := l.db.Reader().GetInsight(ctx, store.GetInsightParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return MemoryInsight{}, core.NotFound("insight %d not found", id)
	}
	if err != nil {
		return MemoryInsight{}, err
	}
	if row.RetiredAt.Valid {
		return MemoryInsight{}, core.Conflict("insight %d is retired; revise its current version", id)
	}
	now := l.clock.Now().UTC()
	var out MemoryInsight
	err = l.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		if _, err := q.RetireInsight(ctx, store.RetireInsightParams{RetiredAt: sql.NullTime{Time: now, Valid: true}, ID: id}); err != nil {
			return err
		}
		if scope == "" {
			scope = row.Scope
		}
		next, err := q.CreateInsight(ctx, store.CreateInsightParams{WorkspaceID: actor.WorkspaceID, Lineage: row.Lineage, Version: row.Version + 1, Statement: statement, Scope: scope, Class: "hypothesis", Causal: 0, Support: row.Support, Source: "human", CreatedBy: optionalID(actor.UserID), CreatedAt: now})
		if err != nil {
			return err
		}
		out, err = l.insight(ctx, next)
		return err
	})
	return out, err
}

func (l *Learning) ListInsights(ctx context.Context, actor core.Actor) ([]MemoryInsight, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := l.db.Reader().ListInsights(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]MemoryInsight, 0, len(rows))
	for _, r := range rows {
		i, err := l.insight(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, nil
}

// Findings is memory for other domains: validated insights first, then
// supported ones, each with its lineage; hypotheses are never fed forward.
func (l *Learning) Findings(ctx context.Context, actor core.Actor) ([]MemoryFinding, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	out := []MemoryFinding{}
	for _, class := range []string{"validated", "supported"} {
		rows, err := l.db.Reader().ListInsightsByClass(ctx, store.ListInsightsByClassParams{WorkspaceID: actor.WorkspaceID, Class: class})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, MemoryFinding{Statement: r.Statement, Class: r.Class, Causal: r.Causal == 1, Lineage: r.Lineage, Version: r.Version, Scope: r.Scope})
		}
	}
	return out, nil
}

func (l *Learning) Report(ctx context.Context, actor core.Actor) (LearningReport, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return LearningReport{}, core.ErrForbidden
	}
	r := LearningReport{Decisions: map[string]int{}, Insights: map[string]int{}}
	counts, err := l.db.Reader().CountLearningDecisions(ctx, actor.WorkspaceID)
	if err != nil {
		return r, err
	}
	for _, c := range counts {
		n := int(c.N)
		r.Decisions[c.Decision] = n
		r.Records += n
	}
	r.Interventions = r.Decisions["adopted"] + r.Decisions["modified"]
	r.Ignored = r.Decisions["ignored"]
	var outcomes, unattached int
	if err := l.db.Read.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(CASE WHEN record_id IS NULL THEN 1 ELSE 0 END), 0) FROM learning_outcomes WHERE workspace_id = ?", actor.WorkspaceID).Scan(&outcomes, &unattached); err != nil {
		return r, err
	}
	r.Outcomes, r.Unattached = outcomes, unattached
	insights, err := l.db.Reader().ListInsights(ctx, actor.WorkspaceID)
	if err != nil {
		return r, err
	}
	for _, i := range insights {
		if !i.RetiredAt.Valid {
			r.Insights[i.Class]++
		}
	}
	r.Note = "Interventions are adopted or modified recommendations with an action; ignored advice is counted apart. An attributed outcome is not causal lift; only a validated insight with a controlled comparison is."
	return r, nil
}

func (l *Learning) record(ctx context.Context, r store.LearningRecord) (LearningRecord, error) {
	rec := LearningRecord{ID: r.ID, RecommendationRef: r.RecommendationRef, Source: r.Source, SubjectKind: r.SubjectKind, SubjectRef: r.SubjectRef, Recommended: json.RawMessage(r.Recommended), Decision: r.Decision, DecisionNote: r.DecisionNote, DecidedBy: idPtr(r.DecidedBy), DecidedAt: timePtr(r.DecidedAt), ActionRef: r.ActionRef, ActionAt: timePtr(r.ActionAt), WindowDays: r.WindowDays, CreatedAt: r.CreatedAt.UTC(), Outcomes: []LearningOutcome{}}
	rows, err := l.db.Reader().ListOutcomesForRecord(ctx, sql.NullInt64{Int64: r.ID, Valid: true})
	if err != nil {
		return LearningRecord{}, err
	}
	for _, o := range rows {
		rec.Outcomes = append(rec.Outcomes, outcomeFromRow(o))
	}
	return rec, nil
}

// insight decodes a row and counts the support that qualifies: records
// whose decision was adopted or modified and that carry at least one
// observed outcome. Model agreement is listed and not counted.
func (l *Learning) insight(ctx context.Context, r store.Insight) (MemoryInsight, error) {
	i := MemoryInsight{ID: r.ID, Lineage: r.Lineage, Version: r.Version, Statement: r.Statement, Scope: r.Scope, Class: r.Class, Causal: r.Causal == 1, Support: []SupportItem{}, Source: r.Source, ControlEvidenceID: idPtr(r.ControlEvidenceID), CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC(), PromotedBy: idPtr(r.PromotedBy), PromotedAt: timePtr(r.PromotedAt), RetiredAt: timePtr(r.RetiredAt)}
	_ = json.Unmarshal([]byte(r.Support), &i.Support)
	i.Attributed = i.Class == "supported" || i.Class == "validated"
	seen := map[int64]bool{}
	for _, s := range i.Support {
		if s.Kind != "record" || seen[s.RecordID] {
			continue
		}
		seen[s.RecordID] = true
		rec, err := l.db.Reader().GetLearningRecord(ctx, store.GetLearningRecordParams{ID: s.RecordID, WorkspaceID: r.WorkspaceID})
		if err != nil {
			continue
		}
		if rec.Decision != "adopted" && rec.Decision != "modified" {
			continue
		}
		outcomes, err := l.db.Reader().ListOutcomesForRecord(ctx, sql.NullInt64{Int64: rec.ID, Valid: true})
		if err != nil {
			return MemoryInsight{}, err
		}
		for _, o := range outcomes {
			if o.Class == "observed" {
				i.SupportingRecords++
				break
			}
		}
	}
	return i, nil
}

func outcomeFromRow(r store.LearningOutcome) LearningOutcome {
	return LearningOutcome{ID: r.ID, RecordID: idPtr(r.RecordID), SubjectKind: r.SubjectKind, SubjectRef: r.SubjectRef, Metric: r.Metric, Value: r.Value, Unit: r.Unit, Class: r.Class, ObservedAt: r.ObservedAt.UTC(), EvidenceID: idPtr(r.EvidenceID), Note: r.Note}
}

func nonNil(s []SupportItem) []SupportItem {
	if s == nil {
		return []SupportItem{}
	}
	return s
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func timePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time.UTC()
	return &t
}

var _ = fmt.Sprintf
