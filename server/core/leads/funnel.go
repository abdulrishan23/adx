// Package leads imports GoHighLevel contacts into ADX and turns their raw
// status, source and project into ADX's own stages, sources and projects
// through a configured mapping. The raw values are kept verbatim, every
// contact observation goes into evidence, and nothing is merged: a lead is
// one provider contact.
//
// The mapping is business policy (reference.md: the status-to-stage table is
// edited by admins; which CRM column holds the status is an open question),
// so it ships unconfigured. Until an admin saves one, every lead's stage is
// null and the data-quality screen lists the raw statuses waiting for a row.
package leads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

// Stage is ADX's funnel grouping (reference.md). The catalog is fixed in
// code; the mapping says which raw statuses fall into each.
type Stage string

const (
	StageOpen         Stage = "open"         // no status yet
	StageAttempted    Stage = "attempted"    // e.g. No Answer
	StageEngaged      Stage = "engaged"      // e.g. Follow Up, Potential
	StageMeeting      Stage = "meeting"      // e.g. Meeting
	StageOffer        Stage = "offer"        // e.g. Sales offer
	StageWon          Stage = "won"          // Done-Deal
	StageDisqualified Stage = "disqualified" // Not Interested, Low Budget, Ready To Move, No availability
	StageExcluded     Stage = "excluded"     // Duplicated, Broker
	StageDead         Stage = "dead"         // Unreachable / wrong number
)

var stages = map[Stage]bool{StageOpen: true, StageAttempted: true, StageEngaged: true, StageMeeting: true, StageOffer: true,
	StageWon: true, StageDisqualified: true, StageExcluded: true, StageDead: true}

// Stages lists the catalog for the settings screen.
var Stages = []Stage{StageOpen, StageAttempted, StageEngaged, StageMeeting, StageOffer, StageWon, StageDisqualified, StageExcluded, StageDead}

// QualifiedStages are the stages reference.md calls qualified: Follow Up,
// Meeting, Potential, Done-Deal. Whether a hot score also counts is open.
var QualifiedStages = map[Stage]bool{StageEngaged: true, StageMeeting: true, StageOffer: true, StageWon: true}

// FieldLocation says where in the CRM contact a value lives.
type FieldLocation struct {
	// Kind is "custom_field" (Ref is the custom field id), "contact_field"
	// (Ref is a top-level field name such as "source"), "tag_prefix" (Ref is
	// a prefix such as "status:") or "" for not configured.
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

func (f FieldLocation) configured() bool { return f.Kind != "" && f.Ref != "" }

// FunnelDocument is one version of the mapping.
type FunnelDocument struct {
	// StatusField is where the lead status lives (open question #34: the
	// "Status" or "Lead Status" column).
	StatusField FieldLocation `json:"statusField"`
	// ProjectField is where the project name lives.
	ProjectField FieldLocation `json:"projectField"`
	// Statuses maps a raw status, as the CRM spells it, to a stage. Matching
	// is case-insensitive and trims spaces. An unmapped status leaves the
	// stage null and is listed as needing a row.
	Statuses map[string]Stage `json:"statuses"`
	// BlankStatusStage is the stage for a lead whose status is empty, if
	// the owner wants blanks counted (Hisham's table puts them in "Open,
	// never updated"). Empty means leave null.
	BlankStatusStage *Stage `json:"blankStatusStage"`
	// Sources maps a raw source value to ADX's source names (reference.md):
	// meta, google, whatsapp, website, telesales, portal:<name>, walk_in,
	// referral, handling_channel (for buckets like "WhatsApp Camp." that
	// aren't sources).
	Sources map[string]string `json:"sources"`
	Note    string            `json:"note"`
}

type FunnelMapping struct {
	Version   int64          `json:"version"`
	Document  FunnelDocument `json:"document"`
	CreatedBy *int64         `json:"createdBy"`
	CreatedAt time.Time      `json:"createdAt"`
}

// FunnelState is the settings screen: current mapping or unconfigured, the
// stage catalog, and the raw statuses seen in the data with their counts and
// current stage, so unmapped ones are obvious.
type FunnelState struct {
	Configured     bool            `json:"configured"`
	Current        *FunnelMapping  `json:"current"`
	Stages         []Stage         `json:"stages"`
	ObservedStatus []ObservedValue `json:"observedStatuses"`
	Unmapped       []string        `json:"unmapped"`
}

type ObservedValue struct {
	Raw   *string `json:"raw"`
	Stage *Stage  `json:"stage"`
	Count int64   `json:"count"`
}

// GetFunnel returns the mapping state for the settings screen.
func (l *Leads) GetFunnel(ctx context.Context, actor core.Actor) (FunnelState, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return FunnelState{}, core.ErrForbidden
	}
	state := FunnelState{Stages: Stages, ObservedStatus: []ObservedValue{}, Unmapped: []string{}}
	current, err := l.currentFunnel(ctx, actor.WorkspaceID)
	if err != nil {
		return FunnelState{}, err
	}
	if current != nil {
		state.Configured = true
		state.Current = current
	}
	rows, err := l.db.Reader().CountLeadsByStatus(ctx, store.CountLeadsByStatusParams{WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return FunnelState{}, err
	}
	for _, r := range rows {
		v := ObservedValue{Count: r.N}
		if r.StatusRaw.Valid {
			v.Raw = &r.StatusRaw.String
		}
		if r.Stage.Valid {
			s := Stage(r.Stage.String)
			v.Stage = &s
		}
		state.ObservedStatus = append(state.ObservedStatus, v)
		if r.StatusRaw.Valid && !r.Stage.Valid {
			state.Unmapped = append(state.Unmapped, r.StatusRaw.String)
		}
	}
	sort.Strings(state.Unmapped)
	return state, nil
}

// PutFunnel saves a new mapping version and re-derives every lead's stage
// from it, so a corrected table applies to history at once. Admins only.
func (l *Leads) PutFunnel(ctx context.Context, actor core.Actor, doc FunnelDocument) (FunnelMapping, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return FunnelMapping{}, core.ErrForbidden
	}
	if err := validateFunnel(doc); err != nil {
		return FunnelMapping{}, err
	}
	doc = normalizeFunnel(doc)
	raw, err := json.Marshal(doc)
	if err != nil {
		return FunnelMapping{}, err
	}
	var row store.FunnelMapping
	err = l.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		version := int64(1)
		if cur, err := q.GetCurrentFunnelMapping(ctx, actor.WorkspaceID); err == nil {
			version = cur.Version + 1
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		row, err = q.CreateFunnelMapping(ctx, store.CreateFunnelMappingParams{
			WorkspaceID: actor.WorkspaceID, Version: version, Document: string(raw),
			CreatedBy: optionalID(actor.UserID), CreatedAt: l.clock.Now().UTC(),
		})
		if err != nil {
			return err
		}
		// Re-stage: every distinct raw status in the workspace, plus blanks.
		counts, err := q.CountLeadsByStatus(ctx, store.CountLeadsByStatusParams{WorkspaceID: actor.WorkspaceID})
		if err != nil {
			return err
		}
		for _, c := range counts {
			var statusRaw *string
			if c.StatusRaw.Valid {
				statusRaw = &c.StatusRaw.String
			}
			stage := doc.stageFor(statusRaw)
			if _, err := q.RestageLeads(ctx, store.RestageLeadsParams{
				Stage: nullStage(stage), MappingVersion: sql.NullInt64{Int64: version, Valid: true},
				WorkspaceID: actor.WorkspaceID, StatusRaw: c.StatusRaw,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if store.IsUniqueViolation(err) {
		return FunnelMapping{}, core.Conflict("someone saved the funnel mapping at the same time; reload and try again")
	}
	if err != nil {
		return FunnelMapping{}, err
	}
	return funnelFromRow(row)
}

// stageFor derives the stage for a raw status, or nil when unmapped.
func (d FunnelDocument) stageFor(raw *string) *Stage {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		if d.BlankStatusStage != nil && *d.BlankStatusStage != "" {
			s := *d.BlankStatusStage
			return &s
		}
		return nil
	}
	if s, ok := d.Statuses[normalizeKey(*raw)]; ok {
		return &s
	}
	return nil
}

// sourceFor maps a raw source, or returns "" when unmapped.
func (d FunnelDocument) sourceFor(raw string) string {
	if raw == "" {
		return ""
	}
	return d.Sources[normalizeKey(raw)]
}

func normalizeKey(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func normalizeFunnel(doc FunnelDocument) FunnelDocument {
	statuses := make(map[string]Stage, len(doc.Statuses))
	for k, v := range doc.Statuses {
		statuses[normalizeKey(k)] = v
	}
	doc.Statuses = statuses
	sources := make(map[string]string, len(doc.Sources))
	for k, v := range doc.Sources {
		sources[normalizeKey(k)] = strings.TrimSpace(v)
	}
	doc.Sources = sources
	return doc
}

func validateFunnel(doc FunnelDocument) error {
	for raw, stage := range doc.Statuses {
		if strings.TrimSpace(raw) == "" {
			return core.Invalid("a status mapping needs the raw status text")
		}
		if !stages[stage] {
			return core.Invalid("status %q maps to unknown stage %q", raw, stage)
		}
	}
	if doc.BlankStatusStage != nil && *doc.BlankStatusStage != "" && !stages[*doc.BlankStatusStage] {
		return core.Invalid("blank status maps to unknown stage %q", *doc.BlankStatusStage)
	}
	for _, loc := range []FieldLocation{doc.StatusField, doc.ProjectField} {
		switch loc.Kind {
		case "", "custom_field", "contact_field", "tag_prefix":
		default:
			return core.Invalid("field location kind %q is not custom_field, contact_field or tag_prefix", loc.Kind)
		}
		if (loc.Kind == "") != (loc.Ref == "") {
			return core.Invalid("a field location needs both a kind and a reference")
		}
	}
	if len(doc.Note) > 1000 {
		return core.Invalid("note must be 1000 characters or fewer")
	}
	return nil
}

func (l *Leads) currentFunnel(ctx context.Context, workspaceID int64) (*FunnelMapping, error) {
	row, err := l.db.Reader().GetCurrentFunnelMapping(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m, err := funnelFromRow(row)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func funnelFromRow(r store.FunnelMapping) (FunnelMapping, error) {
	var doc FunnelDocument
	if err := json.Unmarshal([]byte(r.Document), &doc); err != nil {
		return FunnelMapping{}, fmt.Errorf("funnel mapping %d is not valid JSON: %w", r.ID, err)
	}
	if doc.Statuses == nil {
		doc.Statuses = map[string]Stage{}
	}
	if doc.Sources == nil {
		doc.Sources = map[string]string{}
	}
	out := FunnelMapping{Version: r.Version, Document: doc, CreatedAt: r.CreatedAt.UTC()}
	if r.CreatedBy.Valid {
		out.CreatedBy = &r.CreatedBy.Int64
	}
	return out, nil
}

func nullStage(s *Stage) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(*s), Valid: true}
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }
