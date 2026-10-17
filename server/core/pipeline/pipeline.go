// Package pipeline is ADX's own CRM layer over people and enquiries: one
// opportunity per enquiry with an owned stage, assignments, interactions and
// follow-up tasks, and a previewed, resumable import from the records ADX
// already holds. Stage ownership decides whether a sync may overwrite what a
// person set; a preview computes everything and changes nothing.
package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/store"
)

// StageDef is one configured stage. Kind says what it means for the
// opportunity's state: open stays open, won and lost close it.
type StageDef struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // open, won, lost
}

// Owner says who wins when ADX and the CRM disagree on a stage.
type Owner string

const (
	OwnerADX Owner = "adx"
	OwnerCRM Owner = "crm"
)

type PipelineDocument struct {
	Stages []StageDef `json:"stages"`
	// StageOwner decides whether an import may move a stage a person set.
	StageOwner Owner `json:"stageOwner"`
	// LeadStageMap maps a lead's funnel stage (core/leads) to a pipeline
	// stage key, for imports. Missing lead stages import as the first stage.
	LeadStageMap map[string]string `json:"leadStageMap"`
	Note         string            `json:"note"`
}

type PipelineConfig struct {
	Version   int64            `json:"version"`
	Document  PipelineDocument `json:"document"`
	CreatedBy *int64           `json:"createdBy"`
	CreatedAt time.Time        `json:"createdAt"`
}

type PipelineState struct {
	Configured bool             `json:"configured"`
	Current    *PipelineConfig  `json:"current"`
	Suggested  PipelineDocument `json:"suggested"`
}

type Opportunity struct {
	ID          int64      `json:"id"`
	EnquiryID   int64      `json:"enquiryId"`
	PersonID    int64      `json:"personId"`
	ProjectID   *int64     `json:"projectId"`
	Stage       string     `json:"stage"`
	StageSetBy  string     `json:"stageSetBy"`
	State       string     `json:"state"`
	ValueMinor  *int64     `json:"valueMinor"`
	Currency    *string    `json:"currency"`
	OwnerUserID *int64     `json:"ownerUserId"`
	Version     int64      `json:"version"`
	CreatedBy   *int64     `json:"createdBy"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	ClosedAt    *time.Time `json:"closedAt"`
}

type Assignment struct {
	ID            int64      `json:"id"`
	OpportunityID int64      `json:"opportunityId"`
	UserID        int64      `json:"userId"`
	AssignedBy    *int64     `json:"assignedBy"`
	Reason        string     `json:"reason"`
	AssignedAt    time.Time  `json:"assignedAt"`
	EndedAt       *time.Time `json:"endedAt"`
}

type Interaction struct {
	ID            int64     `json:"id"`
	PersonID      int64     `json:"personId"`
	OpportunityID *int64    `json:"opportunityId"`
	Kind          string    `json:"kind"`
	Summary       string    `json:"summary"`
	OccurredAt    time.Time `json:"occurredAt"`
	RecordedBy    *int64    `json:"recordedBy"`
	EvidenceID    *int64    `json:"evidenceId"`
}

type InteractionInput struct {
	PersonID      int64     `json:"personId"`
	OpportunityID int64     `json:"opportunityId"`
	Kind          string    `json:"kind"`
	Summary       string    `json:"summary"`
	OccurredAt    time.Time `json:"occurredAt"`
}

type FollowUpTask struct {
	ID             int64      `json:"id"`
	OpportunityID  *int64     `json:"opportunityId"`
	PersonID       int64      `json:"personId"`
	Title          string     `json:"title"`
	DueAt          *time.Time `json:"dueAt"`
	State          string     `json:"state"`
	AssigneeUserID *int64     `json:"assigneeUserId"`
	OccurrenceKey  string     `json:"occurrenceKey"`
	CreatedBy      *int64     `json:"createdBy"`
	CreatedAt      time.Time  `json:"createdAt"`
	DoneAt         *time.Time `json:"doneAt"`
	DoneBy         *int64     `json:"doneBy"`
}

type TaskInput struct {
	OpportunityID  int64      `json:"opportunityId"`
	PersonID       int64      `json:"personId"`
	Title          string     `json:"title"`
	DueAt          *time.Time `json:"dueAt"`
	AssigneeUserID int64      `json:"assigneeUserId"`
	// OccurrenceKey makes a repeated request the same task. Empty gets one
	// from the title and opportunity, so "call back Tuesday" is created once.
	OccurrenceKey string `json:"occurrenceKey"`
}

type OpportunityDetail struct {
	Opportunity  Opportunity    `json:"opportunity"`
	Assignments  []Assignment   `json:"assignments"`
	Tasks        []FollowUpTask `json:"tasks"`
	Interactions []Interaction  `json:"interactions"`
}

// ImportRow is one planned change. Action is create, update_stage, skip or
// conflict; Detail says why.
type ImportRow struct {
	ID        int64           `json:"id"`
	Position  int64           `json:"position"`
	EnquiryID int64           `json:"enquiryId"`
	LeadID    *int64          `json:"leadId"`
	Action    string          `json:"action"`
	Detail    json.RawMessage `json:"detail"`
	AppliedAt *time.Time      `json:"appliedAt"`
}

type ImportBatch struct {
	ID            int64       `json:"id"`
	Source        string      `json:"source"`
	State         string      `json:"state"`
	ConfigVersion int64       `json:"configVersion"`
	Cursor        int64       `json:"cursor"`
	Total         int64       `json:"total"`
	Created       int64       `json:"created"`
	Updated       int64       `json:"updated"`
	Skipped       int64       `json:"skipped"`
	Conflicts     int64       `json:"conflicts"`
	Error         string      `json:"error"`
	CreatedBy     *int64      `json:"createdBy"`
	CreatedAt     time.Time   `json:"createdAt"`
	AppliedAt     *time.Time  `json:"appliedAt"`
	Rows          []ImportRow `json:"rows"`
}

type ProviderMapping struct {
	Provider   string `json:"provider"`
	Kind       string `json:"kind"`
	LocalID    int64  `json:"localId"`
	ExternalID string `json:"externalId"`
}

// ExportRow is one opportunity with its provider mappings, for
// reconciliation against the CRM.
type ExportRow struct {
	Opportunity Opportunity       `json:"opportunity"`
	Mappings    []ProviderMapping `json:"mappings"`
}

type Config struct {
	Clock core.Clock
	// FailAfter, in tests, makes an apply fail after n rows, to prove it
	// resumes. Zero is off.
	FailAfter int
}

type Pipeline struct {
	db       *store.DB
	evidence *evidence.Store
	clock    core.Clock
	cfg      Config
}

func New(db *store.DB, ev *evidence.Store, cfg Config) *Pipeline {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Pipeline{db: db, evidence: ev, clock: cfg.Clock, cfg: cfg}
}

// SuggestedConfig mirrors the lead funnel stages, with ADX owning the stage.
func SuggestedConfig() PipelineDocument {
	doc := PipelineDocument{StageOwner: OwnerADX, LeadStageMap: map[string]string{}, Note: "Suggested: one stage per funnel stage; ADX owns the stage, so an import never moves a stage a person set."}
	for _, s := range leads.Stages {
		kind := "open"
		switch s {
		case leads.StageWon:
			kind = "won"
		case leads.StageDisqualified, leads.StageExcluded, leads.StageDead:
			kind = "lost"
		}
		doc.Stages = append(doc.Stages, StageDef{Key: string(s), Label: strings.ToUpper(string(s[:1])) + string(s[1:]), Kind: kind})
		doc.LeadStageMap[string(s)] = string(s)
	}
	return doc
}

func (p *Pipeline) GetConfig(ctx context.Context, actor core.Actor) (PipelineState, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return PipelineState{}, core.ErrForbidden
	}
	cur, err := p.current(ctx, actor.WorkspaceID)
	if err != nil {
		return PipelineState{}, err
	}
	return PipelineState{Configured: cur != nil, Current: cur, Suggested: SuggestedConfig()}, nil
}

func (p *Pipeline) PutConfig(ctx context.Context, actor core.Actor, doc PipelineDocument) (PipelineConfig, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return PipelineConfig{}, core.ErrForbidden
	}
	if len(doc.Stages) == 0 {
		return PipelineConfig{}, core.Invalid("at least one stage is required")
	}
	seen := map[string]bool{}
	for _, s := range doc.Stages {
		if strings.TrimSpace(s.Key) == "" || seen[s.Key] {
			return PipelineConfig{}, core.Invalid("stage keys must be unique and non-empty")
		}
		seen[s.Key] = true
		switch s.Kind {
		case "open", "won", "lost":
		default:
			return PipelineConfig{}, core.Invalid("stage %q needs kind open, won or lost", s.Key)
		}
	}
	if doc.StageOwner != OwnerADX && doc.StageOwner != OwnerCRM {
		return PipelineConfig{}, core.Invalid("stageOwner must be adx or crm")
	}
	for from, to := range doc.LeadStageMap {
		if !seen[to] {
			return PipelineConfig{}, core.Invalid("lead stage %q maps to unknown stage %q", from, to)
		}
	}
	raw, _ := json.Marshal(doc)
	var out PipelineConfig
	err := p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var version int64 = 1
		if cur, err := q.GetCurrentPipelineConfig(ctx, actor.WorkspaceID); err == nil {
			version = cur.Version + 1
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		row, err := q.CreatePipelineConfig(ctx, store.CreatePipelineConfigParams{WorkspaceID: actor.WorkspaceID, Version: version, Document: string(raw), CreatedBy: optionalID(actor.UserID), CreatedAt: p.clock.Now().UTC()})
		if err != nil {
			return err
		}
		out, err = configFromRow(row)
		return err
	})
	return out, err
}

// Open creates the opportunity for an enquiry, or returns the one that
// exists: one enquiry, one opportunity, however many times it is asked.
func (p *Pipeline) Open(ctx context.Context, actor core.Actor, enquiryID int64) (Opportunity, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return Opportunity{}, false, core.ErrForbidden
	}
	cur, err := p.requireConfig(ctx, actor.WorkspaceID)
	if err != nil {
		return Opportunity{}, false, err
	}
	enq, err := p.db.Reader().GetEnquiry(ctx, store.GetEnquiryParams{ID: enquiryID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Opportunity{}, false, core.NotFound("enquiry %d not found", enquiryID)
	}
	if err != nil {
		return Opportunity{}, false, err
	}
	if existing, err := p.db.Reader().GetOpportunityByEnquiry(ctx, enquiryID); err == nil {
		return oppFromRow(existing), false, nil
	}
	now := p.clock.Now().UTC()
	row, err := p.db.Writer().CreateOpportunity(ctx, store.CreateOpportunityParams{WorkspaceID: actor.WorkspaceID, EnquiryID: enquiryID, PersonID: enq.PersonID, ProjectID: enq.ProjectID, Stage: cur.Document.Stages[0].Key, StageSetBy: "system", CreatedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
	if err != nil {
		if existing, e2 := p.db.Reader().GetOpportunityByEnquiry(ctx, enquiryID); e2 == nil {
			return oppFromRow(existing), false, nil
		}
		return Opportunity{}, false, err
	}
	return oppFromRow(row), true, nil
}

// SetStage moves an opportunity. The version must match what the caller
// saw, so two people editing one record do not silently overwrite each
// other. A person's move is recorded as theirs, which an import respects
// when ADX owns the stage.
func (p *Pipeline) SetStage(ctx context.Context, actor core.Actor, id int64, stage string, expectedVersion int64, reason string) (Opportunity, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return Opportunity{}, core.ErrForbidden
	}
	cur, err := p.requireConfig(ctx, actor.WorkspaceID)
	if err != nil {
		return Opportunity{}, err
	}
	def, ok := cur.Document.stage(stage)
	if !ok {
		return Opportunity{}, core.Invalid("unknown stage %q", stage)
	}
	row, err := p.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return Opportunity{}, err
	}
	if row.Version != expectedVersion {
		return Opportunity{}, core.Conflict("opportunity %d is at version %d, not %d; reload and try again", id, row.Version, expectedVersion)
	}
	return p.move(ctx, actor, row, def, "user", reason)
}

func (p *Pipeline) move(ctx context.Context, actor core.Actor, row store.Opportunity, def StageDef, by, reason string) (Opportunity, error) {
	now := p.clock.Now().UTC()
	state := "open"
	var closed sql.NullTime
	switch def.Kind {
	case "won":
		state, closed = "won", sql.NullTime{Time: now, Valid: true}
	case "lost":
		state, closed = "lost", sql.NullTime{Time: now, Valid: true}
	}
	n, err := p.db.Writer().UpdateOpportunityStage(ctx, store.UpdateOpportunityStageParams{Stage: def.Key, StageSetBy: by, State: state, UpdatedAt: now, ClosedAt: closed, ID: row.ID, Version: row.Version})
	if err != nil {
		return Opportunity{}, err
	}
	if n != 1 {
		return Opportunity{}, core.Conflict("opportunity %d changed underneath", row.ID)
	}
	if p.evidence != nil {
		_, _, _ = p.evidence.Record(ctx, actor, evidence.Observation{Source: "adx", Kind: "opportunity_stage", SourceRef: fmt.Sprintf("opportunity:%d/v%d", row.ID, row.Version+1), Scope: fmt.Sprintf("person:%d", row.PersonID), Class: evidence.Observed,
			Payload: map[string]any{"from": row.Stage, "to": def.Key, "by": by, "reason": reason, "user": actor.UserID}, Completeness: evidence.Complete})
	}
	out, err := p.row(ctx, actor.WorkspaceID, row.ID)
	if err != nil {
		return Opportunity{}, err
	}
	return oppFromRow(out), nil
}

func (p *Pipeline) SetValue(ctx context.Context, actor core.Actor, id int64, minor int64, currency string, expectedVersion int64) (Opportunity, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return Opportunity{}, core.ErrForbidden
	}
	if minor < 0 || len(currency) != 3 {
		return Opportunity{}, core.Invalid("value needs minor units >= 0 and a 3-letter currency")
	}
	row, err := p.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return Opportunity{}, err
	}
	n, err := p.db.Writer().UpdateOpportunityValue(ctx, store.UpdateOpportunityValueParams{ValueMinor: sql.NullInt64{Int64: minor, Valid: true}, Currency: sql.NullString{String: strings.ToUpper(currency), Valid: true}, UpdatedAt: p.clock.Now().UTC(), ID: id, Version: expectedVersion})
	if err != nil {
		return Opportunity{}, err
	}
	if n != 1 {
		return Opportunity{}, core.Conflict("opportunity %d is at version %d, not %d", id, row.Version, expectedVersion)
	}
	out, err := p.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return Opportunity{}, err
	}
	return oppFromRow(out), nil
}

// Assign gives the opportunity an owner and ends the previous assignment.
func (p *Pipeline) Assign(ctx context.Context, actor core.Actor, id, userID int64, reason string) (Assignment, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return Assignment{}, core.ErrForbidden
	}
	if _, err := p.row(ctx, actor.WorkspaceID, id); err != nil {
		return Assignment{}, err
	}
	if _, err := p.db.Reader().GetUser(ctx, userID); errors.Is(err, sql.ErrNoRows) {
		return Assignment{}, core.NotFound("user %d not found", userID)
	} else if err != nil {
		return Assignment{}, err
	}
	now := p.clock.Now().UTC()
	var out Assignment
	err := p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		if err := q.EndActiveAssignments(ctx, store.EndActiveAssignmentsParams{EndedAt: sql.NullTime{Time: now, Valid: true}, OpportunityID: id}); err != nil {
			return err
		}
		row, err := q.CreateAssignment(ctx, store.CreateAssignmentParams{OpportunityID: id, UserID: userID, AssignedBy: optionalID(actor.UserID), Reason: strings.TrimSpace(reason), AssignedAt: now})
		if err != nil {
			return err
		}
		out = assignmentFromRow(row)
		return q.SetOpportunityOwner(ctx, store.SetOpportunityOwnerParams{OwnerUserID: sql.NullInt64{Int64: userID, Valid: true}, UpdatedAt: now, ID: id})
	})
	return out, err
}

func (p *Pipeline) RecordInteraction(ctx context.Context, actor core.Actor, in InteractionInput) (Interaction, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return Interaction{}, core.ErrForbidden
	}
	switch in.Kind {
	case "call", "meeting", "message", "note", "visit", "email":
	default:
		return Interaction{}, core.Invalid("kind must be call, meeting, message, note, visit or email")
	}
	in.Summary = strings.TrimSpace(in.Summary)
	if in.Summary == "" {
		return Interaction{}, core.Invalid("a summary is required")
	}
	if in.OccurredAt.IsZero() {
		in.OccurredAt = p.clock.Now()
	}
	if _, err := p.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: in.PersonID, WorkspaceID: actor.WorkspaceID}); errors.Is(err, sql.ErrNoRows) {
		return Interaction{}, core.NotFound("person %d not found", in.PersonID)
	} else if err != nil {
		return Interaction{}, err
	}
	if in.OpportunityID != 0 {
		row, err := p.row(ctx, actor.WorkspaceID, in.OpportunityID)
		if err != nil {
			return Interaction{}, err
		}
		if row.PersonID != in.PersonID {
			return Interaction{}, core.Invalid("opportunity %d belongs to another person", in.OpportunityID)
		}
	}
	now := p.clock.Now().UTC()
	var evID int64
	if p.evidence != nil {
		at := in.OccurredAt.UTC()
		ev, _, err := p.evidence.Record(ctx, actor, evidence.Observation{Source: "manual", Kind: "interaction", SourceRef: fmt.Sprintf("person:%d/%s/%d", in.PersonID, in.Kind, now.UnixNano()), Scope: fmt.Sprintf("person:%d", in.PersonID), Class: evidence.Observed, EventAt: &at,
			Payload: map[string]any{"kind": in.Kind, "summary": in.Summary, "opportunityId": in.OpportunityID}, Completeness: evidence.Complete})
		if err == nil {
			evID = ev.ID
		}
	}
	row, err := p.db.Writer().CreateInteraction(ctx, store.CreateInteractionParams{WorkspaceID: actor.WorkspaceID, PersonID: in.PersonID, OpportunityID: optionalID(in.OpportunityID), Kind: in.Kind, Summary: in.Summary, OccurredAt: in.OccurredAt.UTC(), RecordedBy: optionalID(actor.UserID), EvidenceID: optionalID(evID), CreatedAt: now})
	if err != nil {
		return Interaction{}, err
	}
	return interactionFromRow(row), nil
}

// CreateTask makes a follow-up once per occurrence key.
func (p *Pipeline) CreateTask(ctx context.Context, actor core.Actor, in TaskInput) (FollowUpTask, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return FollowUpTask{}, false, core.ErrForbidden
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return FollowUpTask{}, false, core.Invalid("a title is required")
	}
	if in.OpportunityID != 0 {
		row, err := p.row(ctx, actor.WorkspaceID, in.OpportunityID)
		if err != nil {
			return FollowUpTask{}, false, err
		}
		if in.PersonID == 0 {
			in.PersonID = row.PersonID
		}
	}
	if in.PersonID == 0 {
		return FollowUpTask{}, false, core.Invalid("a person or opportunity is required")
	}
	key := strings.TrimSpace(in.OccurrenceKey)
	if key == "" {
		key = fmt.Sprintf("task:%d:%d:%s", in.OpportunityID, in.PersonID, strings.ToLower(in.Title))
	}
	if existing, err := p.db.Reader().GetFollowUpTaskByKey(ctx, store.GetFollowUpTaskByKeyParams{WorkspaceID: actor.WorkspaceID, OccurrenceKey: key}); err == nil {
		return taskFromRow(existing), false, nil
	}
	var due sql.NullTime
	if in.DueAt != nil {
		due = sql.NullTime{Time: in.DueAt.UTC(), Valid: true}
	}
	row, err := p.db.Writer().CreateFollowUpTask(ctx, store.CreateFollowUpTaskParams{WorkspaceID: actor.WorkspaceID, OpportunityID: optionalID(in.OpportunityID), PersonID: in.PersonID, Title: in.Title, DueAt: due, AssigneeUserID: optionalID(in.AssigneeUserID), OccurrenceKey: key, CreatedBy: optionalID(actor.UserID), CreatedAt: p.clock.Now().UTC()})
	if err != nil {
		if existing, e2 := p.db.Reader().GetFollowUpTaskByKey(ctx, store.GetFollowUpTaskByKeyParams{WorkspaceID: actor.WorkspaceID, OccurrenceKey: key}); e2 == nil {
			return taskFromRow(existing), false, nil
		}
		return FollowUpTask{}, false, err
	}
	return taskFromRow(row), true, nil
}

func (p *Pipeline) CompleteTask(ctx context.Context, actor core.Actor, id int64, canceled bool) (FollowUpTask, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return FollowUpTask{}, core.ErrForbidden
	}
	if _, err := p.db.Reader().GetFollowUpTask(ctx, store.GetFollowUpTaskParams{ID: id, WorkspaceID: actor.WorkspaceID}); errors.Is(err, sql.ErrNoRows) {
		return FollowUpTask{}, core.NotFound("task %d not found", id)
	} else if err != nil {
		return FollowUpTask{}, err
	}
	state := "done"
	if canceled {
		state = "canceled"
	}
	now := p.clock.Now().UTC()
	n, err := p.db.Writer().SetFollowUpTaskState(ctx, store.SetFollowUpTaskStateParams{State: state, DoneAt: sql.NullTime{Time: now, Valid: true}, DoneBy: optionalID(actor.UserID), ID: id})
	if err != nil {
		return FollowUpTask{}, err
	}
	if n != 1 {
		return FollowUpTask{}, core.Conflict("task %d is no longer open", id)
	}
	row, err := p.db.Reader().GetFollowUpTask(ctx, store.GetFollowUpTaskParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return FollowUpTask{}, err
	}
	return taskFromRow(row), nil
}

func (p *Pipeline) ListTasks(ctx context.Context, actor core.Actor, state string, assignee int64, limit int) ([]FollowUpTask, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := p.db.Reader().ListFollowUpTasks(ctx, store.ListFollowUpTasksParams{WorkspaceID: actor.WorkspaceID, State: nullStr(state), AssigneeUserID: optionalID(assignee), Max: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]FollowUpTask, 0, len(rows))
	for _, r := range rows {
		out = append(out, taskFromRow(r))
	}
	return out, nil
}

func (p *Pipeline) List(ctx context.Context, actor core.Actor, stage, state string, owner int64, limit int) ([]Opportunity, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := p.db.Reader().ListOpportunities(ctx, store.ListOpportunitiesParams{WorkspaceID: actor.WorkspaceID, Stage: nullStr(stage), State: nullStr(state), OwnerUserID: optionalID(owner), Max: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Opportunity, 0, len(rows))
	for _, r := range rows {
		out = append(out, oppFromRow(r))
	}
	return out, nil
}

func (p *Pipeline) Get(ctx context.Context, actor core.Actor, id int64) (OpportunityDetail, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return OpportunityDetail{}, core.ErrForbidden
	}
	row, err := p.row(ctx, actor.WorkspaceID, id)
	if err != nil {
		return OpportunityDetail{}, err
	}
	d := OpportunityDetail{Opportunity: oppFromRow(row), Assignments: []Assignment{}, Tasks: []FollowUpTask{}, Interactions: []Interaction{}}
	as, err := p.db.Reader().ListAssignments(ctx, id)
	if err != nil {
		return OpportunityDetail{}, err
	}
	for _, a := range as {
		d.Assignments = append(d.Assignments, assignmentFromRow(a))
	}
	ts, err := p.db.Reader().ListFollowUpTasksForOpportunity(ctx, sql.NullInt64{Int64: id, Valid: true})
	if err != nil {
		return OpportunityDetail{}, err
	}
	for _, t := range ts {
		d.Tasks = append(d.Tasks, taskFromRow(t))
	}
	is, err := p.db.Reader().ListInteractionsForPerson(ctx, store.ListInteractionsForPersonParams{PersonID: row.PersonID, Limit: 200})
	if err != nil {
		return OpportunityDetail{}, err
	}
	for _, i := range is {
		if !i.OpportunityID.Valid || i.OpportunityID.Int64 == id {
			d.Interactions = append(d.Interactions, interactionFromRow(i))
		}
	}
	return d, nil
}

// PreviewImport plans what an import from ADX's enquiries and their linked
// leads would do, and writes only the plan. Nothing in opportunities,
// assignments or tasks changes.
func (p *Pipeline) PreviewImport(ctx context.Context, actor core.Actor, source string) (ImportBatch, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return ImportBatch{}, core.ErrForbidden
	}
	if source == "" {
		source = "gohighlevel"
	}
	cur, err := p.requireConfig(ctx, actor.WorkspaceID)
	if err != nil {
		return ImportBatch{}, err
	}
	rows, err := p.db.Reader().ListEnquiriesWithLeads(ctx, actor.WorkspaceID)
	if err != nil {
		return ImportBatch{}, err
	}
	type planned struct {
		row    store.ListEnquiriesWithLeadsRow
		action string
		detail map[string]any
	}
	var plan []planned
	var created, updated, skipped, conflicts int64
	for _, r := range rows {
		target := cur.Document.Stages[0].Key
		if r.LeadStage.Valid {
			if mapped, ok := cur.Document.LeadStageMap[r.LeadStage.String]; ok {
				target = mapped
			}
		}
		existing, err := p.db.Reader().GetOpportunityByEnquiry(ctx, r.ID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			plan = append(plan, planned{r, "create", map[string]any{"stage": target, "leadStage": r.LeadStage.String, "leadStatus": r.LeadStatus.String}})
			created++
		case err != nil:
			return ImportBatch{}, err
		case existing.Stage == target:
			plan = append(plan, planned{r, "skip", map[string]any{"reason": "already at " + target}})
			skipped++
		case existing.StageSetBy == "user" && cur.Document.StageOwner == OwnerADX:
			plan = append(plan, planned{r, "skip", map[string]any{"reason": "ADX owns the stage and a person set it to " + existing.Stage + "; the CRM shows " + target, "ours": existing.Stage, "theirs": target}})
			skipped++
		case existing.State != "open" && cur.Document.StageOwner == OwnerADX:
			plan = append(plan, planned{r, "conflict", map[string]any{"reason": "the opportunity is " + existing.State + " here but the CRM shows " + target, "ours": existing.Stage, "theirs": target}})
			conflicts++
		default:
			plan = append(plan, planned{r, "update_stage", map[string]any{"from": existing.Stage, "to": target, "owner": cur.Document.StageOwner}})
			updated++
		}
	}
	now := p.clock.Now().UTC()
	var batch store.CrmImportBatch
	err = p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		batch, err = q.CreateImportBatch(ctx, store.CreateImportBatchParams{WorkspaceID: actor.WorkspaceID, Source: source, ConfigVersion: cur.Version, Total: int64(len(plan)), CreatedN: created, UpdatedN: updated, SkippedN: skipped, ConflictN: conflicts, CreatedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		for i, pl := range plan {
			detail, _ := json.Marshal(pl.detail)
			if err := q.CreateImportRow(ctx, store.CreateImportRowParams{BatchID: batch.ID, Position: int64(i), EnquiryID: pl.row.ID, LeadID: pl.row.LeadID, Action: pl.action, Detail: string(detail)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ImportBatch{}, err
	}
	return p.batch(ctx, actor.WorkspaceID, batch.ID)
}

// ApplyImport performs a previewed batch from its cursor, one row per
// transaction, so a failure leaves a resumable position and a repeat skips
// what was applied. Creates are idempotent on the enquiry; stage updates
// re-check ownership against the live record.
func (p *Pipeline) ApplyImport(ctx context.Context, actor core.Actor, batchID int64) (ImportBatch, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return ImportBatch{}, core.ErrForbidden
	}
	b, err := p.db.Reader().GetImportBatch(ctx, store.GetImportBatchParams{ID: batchID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return ImportBatch{}, core.NotFound("import %d not found", batchID)
	}
	if err != nil {
		return ImportBatch{}, err
	}
	if b.State == "applied" {
		return ImportBatch{}, core.Conflict("import %d is already applied", batchID)
	}
	cur, err := p.requireConfig(ctx, actor.WorkspaceID)
	if err != nil {
		return ImportBatch{}, err
	}
	if cur.Version != b.ConfigVersion {
		return ImportBatch{}, core.Conflict("the pipeline configuration changed since the preview (v%d, now v%d); preview again", b.ConfigVersion, cur.Version)
	}
	now := p.clock.Now().UTC()
	if err := p.db.Writer().SetImportBatchState(ctx, store.SetImportBatchStateParams{State: "applying", Cursor: b.Cursor, Error: "", UpdatedAt: now, ID: b.ID}); err != nil {
		return ImportBatch{}, err
	}
	rows, err := p.db.Reader().ListImportRowsFrom(ctx, store.ListImportRowsFromParams{BatchID: b.ID, Position: b.Cursor, Limit: 100000})
	if err != nil {
		return ImportBatch{}, err
	}
	done := 0
	for _, r := range rows {
		if p.cfg.FailAfter > 0 && done >= p.cfg.FailAfter {
			_ = p.db.Writer().SetImportBatchState(ctx, store.SetImportBatchStateParams{State: "failed", Cursor: r.Position, Error: "stopped after " + fmt.Sprint(done) + " rows (test failure)", UpdatedAt: p.clock.Now().UTC(), ID: b.ID})
			return p.batch(ctx, actor.WorkspaceID, b.ID)
		}
		if r.AppliedAt.Valid {
			continue
		}
		if err := p.applyRow(ctx, actor, cur, r); err != nil {
			_ = p.db.Writer().SetImportBatchState(ctx, store.SetImportBatchStateParams{State: "failed", Cursor: r.Position, Error: safe(err), UpdatedAt: p.clock.Now().UTC(), ID: b.ID})
			return p.batch(ctx, actor.WorkspaceID, b.ID)
		}
		done++
		if err := p.db.Writer().SetImportBatchState(ctx, store.SetImportBatchStateParams{State: "applying", Cursor: r.Position + 1, Error: "", UpdatedAt: p.clock.Now().UTC(), ID: b.ID}); err != nil {
			return ImportBatch{}, err
		}
	}
	end := p.clock.Now().UTC()
	if err := p.db.Writer().SetImportBatchState(ctx, store.SetImportBatchStateParams{State: "applied", Cursor: b.Total, Error: "", UpdatedAt: end, AppliedAt: sql.NullTime{Time: end, Valid: true}, ID: b.ID}); err != nil {
		return ImportBatch{}, err
	}
	return p.batch(ctx, actor.WorkspaceID, b.ID)
}

func (p *Pipeline) applyRow(ctx context.Context, actor core.Actor, cur *PipelineConfig, r store.CrmImportRow) error {
	now := p.clock.Now().UTC()
	return p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var detail map[string]any
		_ = json.Unmarshal([]byte(r.Detail), &detail)
		switch r.Action {
		case "create":
			enq, err := q.GetEnquiry(ctx, store.GetEnquiryParams{ID: r.EnquiryID, WorkspaceID: actor.WorkspaceID})
			if err != nil {
				return err
			}
			stage, _ := detail["stage"].(string)
			if _, ok := cur.Document.stage(stage); !ok {
				stage = cur.Document.Stages[0].Key
			}
			if _, err := q.GetOpportunityByEnquiry(ctx, r.EnquiryID); err == nil {
				break // already there: a repeat creates nothing
			}
			def, _ := cur.Document.stage(stage)
			state := "open"
			var closed sql.NullTime
			if def.Kind != "open" {
				state, closed = def.Kind, sql.NullTime{Time: now, Valid: true}
			}
			opp, err := q.CreateOpportunity(ctx, store.CreateOpportunityParams{WorkspaceID: actor.WorkspaceID, EnquiryID: r.EnquiryID, PersonID: enq.PersonID, ProjectID: enq.ProjectID, Stage: stage, StageSetBy: "import", CreatedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
			if err != nil {
				return err
			}
			if state != "open" {
				if _, err := q.UpdateOpportunityStage(ctx, store.UpdateOpportunityStageParams{Stage: stage, StageSetBy: "import", State: state, UpdatedAt: now, ClosedAt: closed, ID: opp.ID, Version: opp.Version}); err != nil {
					return err
				}
			}
			if r.LeadID.Valid {
				lead, err := q.GetLead(ctx, store.GetLeadParams{ID: r.LeadID.Int64, WorkspaceID: actor.WorkspaceID})
				if err == nil {
					if _, err := q.UpsertProviderMapping(ctx, store.UpsertProviderMappingParams{WorkspaceID: actor.WorkspaceID, Provider: "gohighlevel", Kind: "contact", LocalID: opp.ID, ExternalID: lead.ExternalID, CreatedAt: now}); err != nil {
						return err
					}
				}
			}
		case "update_stage":
			existing, err := q.GetOpportunityByEnquiry(ctx, r.EnquiryID)
			if err != nil {
				return err
			}
			to, _ := detail["to"].(string)
			def, ok := cur.Document.stage(to)
			if !ok {
				return nil
			}
			// Ownership is re-checked on the live record: a person who moved
			// the stage since the preview keeps their move when ADX owns it.
			if existing.StageSetBy == "user" && cur.Document.StageOwner == OwnerADX {
				return nil
			}
			if existing.Stage == def.Key {
				return nil
			}
			state := "open"
			var closed sql.NullTime
			if def.Kind != "open" {
				state, closed = def.Kind, sql.NullTime{Time: now, Valid: true}
			}
			if _, err := q.UpdateOpportunityStage(ctx, store.UpdateOpportunityStageParams{Stage: def.Key, StageSetBy: "import", State: state, UpdatedAt: now, ClosedAt: closed, ID: existing.ID, Version: existing.Version}); err != nil {
				return err
			}
		case "skip", "conflict":
		}
		return q.MarkImportRowApplied(ctx, store.MarkImportRowAppliedParams{AppliedAt: sql.NullTime{Time: now, Valid: true}, ID: r.ID})
	})
}

func (p *Pipeline) GetImport(ctx context.Context, actor core.Actor, batchID int64) (ImportBatch, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return ImportBatch{}, core.ErrForbidden
	}
	return p.batch(ctx, actor.WorkspaceID, batchID)
}

func (p *Pipeline) ListImports(ctx context.Context, actor core.Actor, limit int) ([]ImportBatch, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := p.db.Reader().ListImportBatches(ctx, store.ListImportBatchesParams{WorkspaceID: actor.WorkspaceID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]ImportBatch, 0, len(rows))
	for _, r := range rows {
		b := batchFromRow(r)
		b.Rows = []ImportRow{}
		out = append(out, b)
	}
	return out, nil
}

// Export lists every opportunity with its provider mappings, so the CRM can
// be reconciled against ADX's records without guessing which is which.
func (p *Pipeline) Export(ctx context.Context, actor core.Actor) ([]ExportRow, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := p.db.Reader().ListOpportunities(ctx, store.ListOpportunitiesParams{WorkspaceID: actor.WorkspaceID, Max: 100000})
	if err != nil {
		return nil, err
	}
	maps, err := p.db.Reader().ListProviderMappings(ctx, store.ListProviderMappingsParams{WorkspaceID: actor.WorkspaceID, Provider: "gohighlevel", Kind: "contact"})
	if err != nil {
		return nil, err
	}
	byLocal := map[int64][]ProviderMapping{}
	for _, m := range maps {
		byLocal[m.LocalID] = append(byLocal[m.LocalID], ProviderMapping{Provider: m.Provider, Kind: m.Kind, LocalID: m.LocalID, ExternalID: m.ExternalID})
	}
	out := make([]ExportRow, 0, len(rows))
	for _, r := range rows {
		ms := byLocal[r.ID]
		if ms == nil {
			ms = []ProviderMapping{}
		}
		out = append(out, ExportRow{Opportunity: oppFromRow(r), Mappings: ms})
	}
	return out, nil
}

func (d PipelineDocument) stage(key string) (StageDef, bool) {
	for _, s := range d.Stages {
		if s.Key == key {
			return s, true
		}
	}
	return StageDef{}, false
}

func (p *Pipeline) requireConfig(ctx context.Context, workspaceID int64) (*PipelineConfig, error) {
	cur, err := p.current(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, core.Conflict("the pipeline is not configured; an admin saves the stages first")
	}
	return cur, nil
}

func (p *Pipeline) current(ctx context.Context, workspaceID int64) (*PipelineConfig, error) {
	row, err := p.db.Reader().GetCurrentPipelineConfig(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := configFromRow(row)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (p *Pipeline) row(ctx context.Context, workspaceID, id int64) (store.Opportunity, error) {
	row, err := p.db.Reader().GetOpportunity(ctx, store.GetOpportunityParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Opportunity{}, core.NotFound("opportunity %d not found", id)
	}
	return row, err
}

func (p *Pipeline) batch(ctx context.Context, workspaceID, id int64) (ImportBatch, error) {
	row, err := p.db.Reader().GetImportBatch(ctx, store.GetImportBatchParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return ImportBatch{}, core.NotFound("import %d not found", id)
	}
	if err != nil {
		return ImportBatch{}, err
	}
	b := batchFromRow(row)
	rows, err := p.db.Reader().ListImportRows(ctx, id)
	if err != nil {
		return ImportBatch{}, err
	}
	b.Rows = make([]ImportRow, 0, len(rows))
	for _, r := range rows {
		b.Rows = append(b.Rows, ImportRow{ID: r.ID, Position: r.Position, EnquiryID: r.EnquiryID, LeadID: idPtr(r.LeadID), Action: r.Action, Detail: json.RawMessage(r.Detail), AppliedAt: timePtr(r.AppliedAt)})
	}
	return b, nil
}

func configFromRow(r store.PipelineConfig) (PipelineConfig, error) {
	var doc PipelineDocument
	if err := json.Unmarshal([]byte(r.Document), &doc); err != nil {
		return PipelineConfig{}, fmt.Errorf("pipeline config v%d: %w", r.Version, err)
	}
	return PipelineConfig{Version: r.Version, Document: doc, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC()}, nil
}

func oppFromRow(r store.Opportunity) Opportunity {
	return Opportunity{ID: r.ID, EnquiryID: r.EnquiryID, PersonID: r.PersonID, ProjectID: idPtr(r.ProjectID), Stage: r.Stage, StageSetBy: r.StageSetBy, State: r.State, ValueMinor: idPtr(r.ValueMinor), Currency: strPtr(r.Currency), OwnerUserID: idPtr(r.OwnerUserID), Version: r.Version,
		CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(), ClosedAt: timePtr(r.ClosedAt)}
}

func assignmentFromRow(r store.OpportunityAssignment) Assignment {
	return Assignment{ID: r.ID, OpportunityID: r.OpportunityID, UserID: r.UserID, AssignedBy: idPtr(r.AssignedBy), Reason: r.Reason, AssignedAt: r.AssignedAt.UTC(), EndedAt: timePtr(r.EndedAt)}
}

func interactionFromRow(r store.Interaction) Interaction {
	return Interaction{ID: r.ID, PersonID: r.PersonID, OpportunityID: idPtr(r.OpportunityID), Kind: r.Kind, Summary: r.Summary, OccurredAt: r.OccurredAt.UTC(), RecordedBy: idPtr(r.RecordedBy), EvidenceID: idPtr(r.EvidenceID)}
}

func taskFromRow(r store.FollowUpTask) FollowUpTask {
	return FollowUpTask{ID: r.ID, OpportunityID: idPtr(r.OpportunityID), PersonID: r.PersonID, Title: r.Title, DueAt: timePtr(r.DueAt), State: r.State, AssigneeUserID: idPtr(r.AssigneeUserID), OccurrenceKey: r.OccurrenceKey, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC(), DoneAt: timePtr(r.DoneAt), DoneBy: idPtr(r.DoneBy)}
}

func batchFromRow(r store.CrmImportBatch) ImportBatch {
	return ImportBatch{ID: r.ID, Source: r.Source, State: r.State, ConfigVersion: r.ConfigVersion, Cursor: r.Cursor, Total: r.Total, Created: r.CreatedN, Updated: r.UpdatedN, Skipped: r.SkippedN, Conflicts: r.ConflictN, Error: r.Error, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC(), AppliedAt: timePtr(r.AppliedAt)}
}

func safe(err error) string {
	s := err.Error()
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func nullStr(s string) sql.NullString   { return sql.NullString{String: s, Valid: s != ""} }
func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func strPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	v := n.String
	return &v
}

func timePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time.UTC()
	return &t
}
