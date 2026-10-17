// Package signals ingests the behaviours ADX can observe about a person and
// turns them into fit and intent scores with an explanation. The pipeline
// is ported in concept from the voice source's signal processor (reuse-
// voice.md): occurrence-key idempotency, per-kind caps, half-life decay.
// Education rules are not ported. Fit and intent add instead of multiply,
// so a lead with no fit data is not scored zero (D-17).
package signals

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/alerts"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/store"
)

// SignalKind is one behaviour. The catalog is fixed in code so a rule or a
// webhook can't name a kind ADX doesn't understand.
type SignalKind struct {
	Kind string `json:"kind"`
	// Source names the adapter that produces it.
	Source      string `json:"source"`
	Description string `json:"description"`
	// Supported false means ADX knows the behaviour exists but has no way
	// to observe it yet; it shows as unavailable, never as zero.
	Supported bool   `json:"supported"`
	Reason    string `json:"reason"`
}

// Catalog is every kind ADX can name.
var Catalog = []SignalKind{
	{Kind: "form_submit", Source: "pages", Description: "A form on an ADX page, after the first", Supported: true},
	{Kind: "whatsapp_tap", Source: "pages", Description: "A WhatsApp tap on a thank-you screen", Supported: true},
	{Kind: "stage_change", Source: "crm", Description: "A CRM status change, including moving back to follow-up", Supported: true},
	{Kind: "page_view", Source: "web", Description: "A pricing or floor-plan page view tied to a known person", Supported: true},
	{Kind: "message_reply", Source: "messaging", Description: "A reply to an ADX message", Supported: true},
	{Kind: "call_outcome", Source: "voice", Description: "A qualification call's outcome", Supported: true},
	{Kind: "email_open", Source: "email", Description: "An email open", Supported: false, Reason: "No email channel sends from ADX, and opens are not reliably observable."},
	{Kind: "portal_view", Source: "portals", Description: "A view on a property portal", Supported: false, Reason: "Portals expose no per-person events."},
}

func kindInfo(kind string) (SignalKind, bool) {
	for _, k := range Catalog {
		if k.Kind == kind {
			return k, true
		}
	}
	return SignalKind{}, false
}

// SignalIdentity is what an adapter knows about who did it. Exact identifiers
// resolve to a person; an anonymous or device id alone never does.
type SignalIdentity struct {
	PersonID    int64  `json:"personId"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	External    string `json:"external"`
	AnonymousID string `json:"anonymousId"`
}

// SignalInput is one observation from an adapter.
type SignalInput struct {
	Source string `json:"source"`
	Kind   string `json:"kind"`
	// OccurrenceKey is the adapter's stable id for the event, so a repeated
	// delivery is the same event.
	OccurrenceKey string          `json:"occurrenceKey"`
	OccurredAt    time.Time       `json:"occurredAt"`
	Identity      SignalIdentity  `json:"identity"`
	ProjectID     int64           `json:"projectId"`
	Payload       json.RawMessage `json:"payload"`
}

type SignalEvent struct {
	ID            int64           `json:"id"`
	PersonID      *int64          `json:"personId"`
	EnquiryID     *int64          `json:"enquiryId"`
	Source        string          `json:"source"`
	Kind          string          `json:"kind"`
	OccurrenceKey string          `json:"occurrenceKey"`
	Confidence    string          `json:"confidence"`
	Identity      json.RawMessage `json:"identity"`
	Payload       json.RawMessage `json:"payload"`
	OccurredAt    time.Time       `json:"occurredAt"`
	ReceivedAt    time.Time       `json:"receivedAt"`
	EvidenceID    *int64          `json:"evidenceId"`
	DiscardedAt   *time.Time      `json:"discardedAt"`
	DiscardReason string          `json:"discardReason"`
}

// SignalIngested is the result of one ingest: the event and whether it was new.
type SignalIngested struct {
	Event     SignalEvent `json:"event"`
	Duplicate bool        `json:"duplicate"`
	// Scored says an assessment ran. A duplicate or an unknown identity
	// never scores.
	Scored bool `json:"scored"`
}

// FitRule scores one attribute of the person. Values maps a stated or
// enriched value to points; Present gives points for having any value.
type FitRule struct {
	Attribute string         `json:"attribute"`
	Values    map[string]int `json:"values"`
	Present   int            `json:"present"`
	// StatedOnly ignores enriched values for this attribute.
	StatedOnly bool `json:"statedOnly"`
}

// IntentRule scores one signal kind: Points per event, decaying with
// HalfLifeDays, repeats damped by RepeatFactor, never above Cap.
type IntentRule struct {
	Kind         string  `json:"kind"`
	Points       int     `json:"points"`
	HalfLifeDays float64 `json:"halfLifeDays"`
	RepeatFactor float64 `json:"repeatFactor"`
	Cap          int     `json:"cap"`
}

// StageCriteria classify the combined score. Nil means unconfigured, and an
// unconfigured criteria set classifies nobody.
type StageCriteria struct {
	Hot  int `json:"hot"`
	Warm int `json:"warm"`
}

type ScoringDocument struct {
	Fit    []FitRule      `json:"fit"`
	Intent []IntentRule   `json:"intent"`
	Stages *StageCriteria `json:"stages"`
	Note   string         `json:"note"`
}

type ScoringVersion struct {
	Version   int64           `json:"version"`
	Document  ScoringDocument `json:"document"`
	CreatedBy *int64          `json:"createdBy"`
	CreatedAt time.Time       `json:"createdAt"`
}

type ScoringState struct {
	Configured bool            `json:"configured"`
	Current    *ScoringVersion `json:"current"`
	Catalog    []SignalKind    `json:"catalog"`
	Suggested  ScoringDocument `json:"suggested"`
}

// FitContribution is one line of the fit explanation.
type FitContribution struct {
	Attribute string `json:"attribute"`
	Value     string `json:"value"`
	Class     string `json:"class"`
	Points    int    `json:"points"`
}

// IntentContribution is one kind's line of the intent explanation.
type IntentContribution struct {
	Kind   string  `json:"kind"`
	Events int     `json:"events"`
	Raw    float64 `json:"raw"`
	Cap    int     `json:"cap"`
	Points int     `json:"points"`
}

type ExcludedEvent struct {
	EventID int64  `json:"eventId"`
	Reason  string `json:"reason"`
}

// ScoreExplanation is why a person has the score they have.
type ScoreExplanation struct {
	RulesVersion *int64               `json:"rulesVersion"`
	FitAvailable bool                 `json:"fitAvailable"`
	Fit          []FitContribution    `json:"fit"`
	Intent       []IntentContribution `json:"intent"`
	Excluded     []ExcludedEvent      `json:"excluded"`
	Stages       *StageCriteria       `json:"stages"`
	Temperature  *string              `json:"temperature"`
	Unconfigured []string             `json:"unconfigured"`
	Trigger      string               `json:"trigger"`
	PreviousTemp *string              `json:"previousTemperature"`
}

type ScoreAssessment struct {
	ID           int64            `json:"id"`
	PersonID     int64            `json:"personId"`
	RulesVersion *int64           `json:"rulesVersion"`
	Trigger      string           `json:"trigger"`
	Fit          *int64           `json:"fit"`
	Intent       *int64           `json:"intent"`
	Temperature  *string          `json:"temperature"`
	Explanation  ScoreExplanation `json:"explanation"`
	AssessedAt   time.Time        `json:"assessedAt"`
}

type SignalCorrection struct {
	ID           int64     `json:"id"`
	EventID      int64     `json:"eventId"`
	Action       string    `json:"action"`
	FromPersonID *int64    `json:"fromPersonId"`
	ToPersonID   *int64    `json:"toPersonId"`
	Reason       string    `json:"reason"`
	CreatedBy    *int64    `json:"createdBy"`
	CreatedAt    time.Time `json:"createdAt"`
}

// ScoreView is a person's scoring page: latest assessment, history, the
// timeline of events and the corrections made.
type ScoreView struct {
	PersonID    int64              `json:"personId"`
	Latest      *ScoreAssessment   `json:"latest"`
	History     []ScoreAssessment  `json:"history"`
	Events      []SignalEvent      `json:"events"`
	Corrections []SignalCorrection `json:"corrections"`
	Rules       ScoringState       `json:"rules"`
}

type Config struct {
	Clock core.Clock
	// Alerts, when set, receives a temperature change to hot as an
	// evidence-backed alert.
	Alerts *alerts.Alerts
	// DeriveLimit caps rows per derive pass.
	DeriveLimit int64
}

type Signals struct {
	db       *store.DB
	evidence *evidence.Store
	crm      *crm.CRM
	clock    core.Clock
	alerts   *alerts.Alerts
	limit    int64
}

func New(db *store.DB, ev *evidence.Store, c *crm.CRM, cfg Config) *Signals {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	if cfg.DeriveLimit <= 0 {
		cfg.DeriveLimit = 2000
	}
	return &Signals{db: db, evidence: ev, crm: c, clock: cfg.Clock, alerts: cfg.Alerts, limit: cfg.DeriveLimit}
}

// SuggestedRules are a starting point an admin edits; nothing uses them
// until saved.
func SuggestedRules() ScoringDocument {
	return ScoringDocument{
		Fit: []FitRule{
			{Attribute: "purchase_timeline", Values: map[string]int{"now": 30, "3_months": 20, "6_months": 10, "later": 0}, StatedOnly: true},
			{Attribute: "payment", Values: map[string]int{"cash": 20, "mortgage": 15, "plan": 15}, StatedOnly: true},
			{Attribute: "purpose", Values: map[string]int{"live": 15, "invest": 15}, StatedOnly: true},
			{Attribute: "country", Present: 5},
		},
		Intent: []IntentRule{
			{Kind: "form_submit", Points: 20, HalfLifeDays: 14, RepeatFactor: 0.6, Cap: 40},
			{Kind: "whatsapp_tap", Points: 15, HalfLifeDays: 7, RepeatFactor: 0.5, Cap: 25},
			{Kind: "stage_change", Points: 10, HalfLifeDays: 14, RepeatFactor: 0.7, Cap: 20},
			{Kind: "page_view", Points: 5, HalfLifeDays: 7, RepeatFactor: 0.8, Cap: 20},
			{Kind: "message_reply", Points: 15, HalfLifeDays: 10, RepeatFactor: 0.6, Cap: 30},
			{Kind: "call_outcome", Points: 25, HalfLifeDays: 21, RepeatFactor: 0.5, Cap: 40},
		},
		Stages: nil,
		Note:   "Suggested starting point. Stage thresholds are left unset on purpose: nobody is hot or warm until Reportage says what hot and warm mean.",
	}
}

func (s *Signals) GetRules(ctx context.Context, actor core.Actor) (ScoringState, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return ScoringState{}, core.ErrForbidden
	}
	cur, err := s.current(ctx, actor.WorkspaceID)
	if err != nil {
		return ScoringState{}, err
	}
	st := ScoringState{Configured: cur != nil, Current: cur, Catalog: Catalog, Suggested: SuggestedRules()}
	return st, nil
}

func (s *Signals) PutRules(ctx context.Context, actor core.Actor, doc ScoringDocument) (ScoringVersion, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return ScoringVersion{}, core.ErrForbidden
	}
	if err := validateRules(doc); err != nil {
		return ScoringVersion{}, err
	}
	raw, _ := json.Marshal(doc)
	var out ScoringVersion
	err := s.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var version int64 = 1
		if cur, err := q.GetCurrentScoringRules(ctx, actor.WorkspaceID); err == nil {
			version = cur.Version + 1
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		row, err := q.CreateScoringRules(ctx, store.CreateScoringRulesParams{WorkspaceID: actor.WorkspaceID, Version: version, Document: string(raw), CreatedBy: optionalID(actor.UserID), CreatedAt: s.clock.Now().UTC()})
		if err != nil {
			return err
		}
		out, err = rulesFromRow(row)
		return err
	})
	return out, err
}

func validateRules(doc ScoringDocument) error {
	seen := map[string]bool{}
	for _, r := range doc.Intent {
		info, ok := kindInfo(r.Kind)
		if !ok {
			return core.Invalid("unknown signal kind %q", r.Kind)
		}
		if !info.Supported {
			return core.Invalid("signal kind %q is not observable: %s", r.Kind, info.Reason)
		}
		if seen[r.Kind] {
			return core.Invalid("signal kind %q appears twice", r.Kind)
		}
		seen[r.Kind] = true
		if r.Points <= 0 || r.Cap <= 0 || r.HalfLifeDays <= 0 || r.RepeatFactor <= 0 || r.RepeatFactor > 1 {
			return core.Invalid("rule for %q needs points > 0, cap > 0, half-life > 0 and a repeat factor in (0, 1]", r.Kind)
		}
	}
	for _, f := range doc.Fit {
		if strings.TrimSpace(f.Attribute) == "" {
			return core.Invalid("a fit rule needs an attribute")
		}
		if len(f.Values) == 0 && f.Present == 0 {
			return core.Invalid("fit rule for %q scores nothing", f.Attribute)
		}
	}
	if doc.Stages != nil && (doc.Stages.Hot <= doc.Stages.Warm || doc.Stages.Warm < 0) {
		return core.Invalid("stage criteria need hot above warm and warm at least 0")
	}
	return nil
}

// Ingest records one event. A repeated occurrence key is the same event and
// returns Duplicate without scoring again. Identity is resolved to a person
// only through exact identifiers; a device or anonymous id alone is stored
// as unknown and excluded from scoring.
func (s *Signals) Ingest(ctx context.Context, actor core.Actor, in SignalInput) (SignalIngested, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return SignalIngested{}, core.ErrForbidden
	}
	info, ok := kindInfo(in.Kind)
	if !ok {
		return SignalIngested{}, core.Invalid("unknown signal kind %q", in.Kind)
	}
	if !info.Supported {
		return SignalIngested{}, core.Invalid("signal kind %q is not observable: %s", in.Kind, info.Reason)
	}
	in.Source = strings.TrimSpace(in.Source)
	in.OccurrenceKey = strings.TrimSpace(in.OccurrenceKey)
	if in.Source == "" || in.OccurrenceKey == "" {
		return SignalIngested{}, core.Invalid("source and occurrence key are required")
	}
	if in.OccurredAt.IsZero() {
		return SignalIngested{}, core.Invalid("occurredAt is required")
	}
	if len(in.Payload) == 0 {
		in.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(in.Payload) {
		return SignalIngested{}, core.Invalid("payload is not valid JSON")
	}
	if existing, err := s.db.Reader().GetInteractionEventByKey(ctx, store.GetInteractionEventByKeyParams{WorkspaceID: actor.WorkspaceID, Source: in.Source, OccurrenceKey: in.OccurrenceKey}); err == nil {
		return SignalIngested{Event: eventFromRow(existing), Duplicate: true}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return SignalIngested{}, err
	}
	personID, enquiryID, confidence, idEv, err := s.resolve(ctx, actor.WorkspaceID, in.Identity, in.ProjectID)
	if err != nil {
		return SignalIngested{}, err
	}
	idJSON, _ := json.Marshal(idEv)
	now := s.clock.Now().UTC()
	var evID int64
	if s.evidence != nil {
		ev, _, err := s.evidence.Record(ctx, actor, evidence.Observation{Source: in.Source, Kind: "signal." + in.Kind, SourceRef: in.OccurrenceKey, Scope: scopeFor(personID), Class: evidence.Observed, EventAt: &in.OccurredAt,
			Payload: map[string]any{"kind": in.Kind, "identity": idEv, "payload": in.Payload}, Completeness: evidence.Complete})
		if err != nil {
			return SignalIngested{}, err
		}
		evID = ev.ID
	}
	row, err := s.db.Writer().CreateInteractionEvent(ctx, store.CreateInteractionEventParams{WorkspaceID: actor.WorkspaceID, PersonID: optionalID(personID), EnquiryID: optionalID(enquiryID), Source: in.Source, Kind: in.Kind, OccurrenceKey: in.OccurrenceKey,
		Confidence: confidence, Identity: string(idJSON), Payload: string(in.Payload), OccurredAt: in.OccurredAt.UTC(), ReceivedAt: now, EvidenceID: optionalID(evID)})
	if err != nil {
		if existing, e2 := s.db.Reader().GetInteractionEventByKey(ctx, store.GetInteractionEventByKeyParams{WorkspaceID: actor.WorkspaceID, Source: in.Source, OccurrenceKey: in.OccurrenceKey}); e2 == nil {
			return SignalIngested{Event: eventFromRow(existing), Duplicate: true}, nil
		}
		return SignalIngested{}, err
	}
	out := SignalIngested{Event: eventFromRow(row)}
	if personID != 0 {
		if _, err := s.Assess(ctx, actor, personID, "event"); err != nil {
			return out, err
		}
		out.Scored = true
	}
	return out, nil
}

func scopeFor(personID int64) string {
	if personID == 0 {
		return "person:unknown"
	}
	return fmt.Sprintf("person:%d", personID)
}

// resolve finds the person an identity names. Each exact identifier must
// point at the same single person; disagreement or an anonymous id alone is
// unknown.
func (s *Signals) resolve(ctx context.Context, workspaceID int64, id SignalIdentity, projectID int64) (personID, enquiryID int64, confidence string, ev map[string]any, err error) {
	ev = map[string]any{}
	found := map[int64]bool{}
	note := func(kind string, people []store.Person) {
		switch len(people) {
		case 0:
			ev[kind] = "no person"
		case 1:
			ev[kind] = "exact"
			found[people[0].ID] = true
		default:
			ev[kind] = fmt.Sprintf("shared by %d people", len(people))
			ev[kind+"Ambiguous"] = true
		}
	}
	if id.PersonID != 0 {
		p, err := s.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: id.PersonID, WorkspaceID: workspaceID})
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, "", nil, core.Invalid("person %d not found", id.PersonID)
		}
		if err != nil {
			return 0, 0, "", nil, err
		}
		if p.MergedIntoID.Valid {
			p, err = s.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: p.MergedIntoID.Int64, WorkspaceID: workspaceID})
			if err != nil {
				return 0, 0, "", nil, err
			}
		}
		found[p.ID] = true
		ev["personId"] = "given"
	}
	lookups := map[string]string{}
	if e := strings.ToLower(strings.TrimSpace(id.Email)); e != "" && strings.Contains(e, "@") {
		lookups["email"] = e
	}
	if s.crm != nil {
		if p := s.crm.NormalizePhone(id.Phone); p != "" {
			lookups["phone"] = p
		}
	}
	if x := strings.TrimSpace(id.External); x != "" {
		lookups["external"] = x
	}
	for _, kind := range []string{"email", "phone", "external"} {
		value, ok := lookups[kind]
		if !ok {
			continue
		}
		people, err := s.db.Reader().FindPeopleByIdentifier(ctx, store.FindPeopleByIdentifierParams{WorkspaceID: workspaceID, Kind: kind, Value: value})
		if err != nil {
			return 0, 0, "", nil, err
		}
		note(kind, people)
	}
	if id.AnonymousID != "" {
		ev["anonymousId"] = "device or browser id only; not an identity"
	}
	switch len(found) {
	case 1:
		for p := range found {
			personID = p
		}
		confidence = "confirmed"
		enq, err := s.db.Reader().FindOpenEnquiry(ctx, store.FindOpenEnquiryParams{PersonID: personID, ProjectID: optionalID(projectID)})
		if err == nil {
			enquiryID = enq.ID
		} else if !errors.Is(err, sql.ErrNoRows) {
			return 0, 0, "", nil, err
		}
	case 0:
		confidence = "unknown"
		ev["resolution"] = "no exact identifier matched a person"
	default:
		confidence = "unknown"
		ids := make([]string, 0, len(found))
		for p := range found {
			ids = append(ids, strconv.FormatInt(p, 10))
		}
		sort.Strings(ids)
		ev["resolution"] = "identifiers point at different people: " + strings.Join(ids, ", ")
	}
	return personID, enquiryID, confidence, ev, nil
}

// Assess computes and records a person's scores under the current rules,
// from every confirmed, undiscarded event. It always records, even
// without rules, so the history says what was known when.
func (s *Signals) Assess(ctx context.Context, actor core.Actor, personID int64, trigger string) (ScoreAssessment, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return ScoreAssessment{}, core.ErrForbidden
	}
	if _, err := s.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: personID, WorkspaceID: actor.WorkspaceID}); errors.Is(err, sql.ErrNoRows) {
		return ScoreAssessment{}, core.NotFound("person %d not found", personID)
	} else if err != nil {
		return ScoreAssessment{}, err
	}
	rules, err := s.current(ctx, actor.WorkspaceID)
	if err != nil {
		return ScoreAssessment{}, err
	}
	events, err := s.db.Reader().ListInteractionEventsForPerson(ctx, store.ListInteractionEventsForPersonParams{PersonID: optionalID(personID), Limit: 2000})
	if err != nil {
		return ScoreAssessment{}, err
	}
	attrs, err := s.db.Reader().ListPersonAttributes(ctx, personID)
	if err != nil {
		return ScoreAssessment{}, err
	}
	now := s.clock.Now().UTC()
	exp := ScoreExplanation{Trigger: trigger, Fit: []FitContribution{}, Intent: []IntentContribution{}, Excluded: []ExcludedEvent{}, Unconfigured: []string{}}
	var fit, intent sql.NullInt64
	var temp sql.NullString
	if prev, err := s.db.Reader().LatestScoreAssessment(ctx, personID); err == nil && prev.Temperature.Valid {
		t := prev.Temperature.String
		exp.PreviousTemp = &t
	}
	if rules == nil {
		exp.Unconfigured = append(exp.Unconfigured, "scoring rules")
	} else {
		exp.RulesVersion = &rules.Version
		exp.Stages = rules.Document.Stages
		fitPts, fitAvail, fitLines := scoreFit(rules.Document.Fit, attrs)
		exp.FitAvailable = fitAvail
		exp.Fit = fitLines
		if fitAvail {
			fit = sql.NullInt64{Int64: int64(fitPts), Valid: true}
		}
		intentPts, lines, excluded := scoreIntent(rules.Document.Intent, events, now)
		exp.Intent = lines
		exp.Excluded = excluded
		intent = sql.NullInt64{Int64: int64(intentPts), Valid: true}
		if rules.Document.Stages == nil {
			exp.Unconfigured = append(exp.Unconfigured, "stage criteria")
		} else {
			combined := intentPts
			if fitAvail {
				combined += fitPts
			}
			t := "cold"
			switch {
			case combined >= rules.Document.Stages.Hot:
				t = "hot"
			case combined >= rules.Document.Stages.Warm:
				t = "warm"
			}
			temp = sql.NullString{String: t, Valid: true}
			exp.Temperature = &t
		}
	}
	raw, _ := json.Marshal(exp)
	var version sql.NullInt64
	if rules != nil {
		version = sql.NullInt64{Int64: rules.Version, Valid: true}
	}
	row, err := s.db.Writer().CreateScoreAssessment(ctx, store.CreateScoreAssessmentParams{WorkspaceID: actor.WorkspaceID, PersonID: personID, RulesVersion: version, Trigger: trigger, Fit: fit, Intent: intent, Temperature: temp, Explanation: string(raw), AssessedAt: now})
	if err != nil {
		return ScoreAssessment{}, err
	}
	a := assessmentFromRow(row)
	if s.alerts != nil && temp.Valid && temp.String == "hot" && (exp.PreviousTemp == nil || *exp.PreviousTemp != "hot") {
		_, _ = s.alerts.Raise(ctx, actor, "lead_hot", fmt.Sprintf("person:%d", personID), fmt.Sprintf("person %d", personID), "info",
			"A person's combined score crossed the hot threshold.", map[string]any{"assessmentId": a.ID, "rulesVersion": rules.Version, "intent": intent.Int64, "fit": fit, "events": len(exp.Intent)})
	}
	return a, nil
}

// scoreFit adds the points of each rule whose attribute has a value. With no
// attribute present for any rule, fit is not available rather than zero.
func scoreFit(rules []FitRule, attrs []store.PersonAttribute) (int, bool, []FitContribution) {
	stated := map[string]string{}
	enriched := map[string]string{}
	for _, a := range attrs {
		if a.SupersededAt.Valid {
			continue
		}
		switch a.Class {
		case "stated":
			stated[a.Key] = a.Value
		case "enriched":
			enriched[a.Key] = a.Value
		}
	}
	total, available := 0, false
	lines := []FitContribution{}
	for _, r := range rules {
		value, class := "", ""
		if v, ok := stated[r.Attribute]; ok {
			value, class = v, "stated"
		} else if v, ok := enriched[r.Attribute]; ok && !r.StatedOnly {
			value, class = v, "enriched"
		}
		if class == "" {
			continue
		}
		available = true
		pts := r.Present
		if p, ok := r.Values[strings.ToLower(strings.TrimSpace(value))]; ok {
			pts += p
		}
		total += pts
		lines = append(lines, FitContribution{Attribute: r.Attribute, Value: value, Class: class, Points: pts})
	}
	return total, available, lines
}

// scoreIntent sums each kind's events, newest first, with decay and repeat
// damping, capped per kind. Unknown-identity and discarded events never get
// here with a person, but a reattributed event's old person is re-assessed
// without it, so the exclusion list names what was left out and why.
func scoreIntent(rules []IntentRule, events []store.InteractionEvent, now time.Time) (int, []IntentContribution, []ExcludedEvent) {
	byKind := map[string][]store.InteractionEvent{}
	excluded := []ExcludedEvent{}
	for _, e := range events {
		switch {
		case e.DiscardedAt.Valid:
			excluded = append(excluded, ExcludedEvent{EventID: e.ID, Reason: "discarded: " + e.DiscardReason})
		case e.Confidence != "confirmed":
			excluded = append(excluded, ExcludedEvent{EventID: e.ID, Reason: "identity not confirmed"})
		default:
			byKind[e.Kind] = append(byKind[e.Kind], e)
		}
	}
	total := 0
	lines := []IntentContribution{}
	for _, r := range rules {
		evs := byKind[r.Kind]
		delete(byKind, r.Kind)
		if len(evs) == 0 {
			continue
		}
		sort.Slice(evs, func(i, j int) bool { return evs[i].OccurredAt.After(evs[j].OccurredAt) })
		raw := 0.0
		for i, e := range evs {
			age := now.Sub(e.OccurredAt.UTC()).Hours() / 24
			if age < 0 {
				age = 0
			}
			decay := math.Pow(0.5, age/r.HalfLifeDays)
			raw += float64(r.Points) * math.Pow(r.RepeatFactor, float64(i)) * decay
		}
		pts := int(math.Round(math.Min(raw, float64(r.Cap))))
		total += pts
		lines = append(lines, IntentContribution{Kind: r.Kind, Events: len(evs), Raw: math.Round(raw*100) / 100, Cap: r.Cap, Points: pts})
	}
	for kind, evs := range byKind {
		for _, e := range evs {
			excluded = append(excluded, ExcludedEvent{EventID: e.ID, Reason: "no rule for " + kind})
		}
	}
	sort.Slice(excluded, func(i, j int) bool { return excluded[i].EventID < excluded[j].EventID })
	return total, lines, excluded
}

// Correct discards an event or moves it to another person, with a reason,
// and re-assesses everyone affected. The event and the earlier assessments
// stay; the correction is a row of its own.
func (s *Signals) Correct(ctx context.Context, actor core.Actor, eventID int64, action string, toPersonID int64, reason string) (SignalCorrection, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return SignalCorrection{}, core.ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return SignalCorrection{}, core.Invalid("a reason is required")
	}
	row, err := s.db.Reader().GetInteractionEvent(ctx, store.GetInteractionEventParams{ID: eventID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return SignalCorrection{}, core.NotFound("event %d not found", eventID)
	}
	if err != nil {
		return SignalCorrection{}, err
	}
	now := s.clock.Now().UTC()
	var out SignalCorrection
	switch action {
	case "discard":
		if row.DiscardedAt.Valid {
			return SignalCorrection{}, core.Conflict("event %d is already discarded", eventID)
		}
		txErr := s.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
			n, err := q.DiscardInteractionEvent(ctx, store.DiscardInteractionEventParams{DiscardedAt: sql.NullTime{Time: now, Valid: true}, DiscardReason: reason, ID: eventID})
			if err != nil {
				return err
			}
			if n != 1 {
				return core.Conflict("event %d is already discarded", eventID)
			}
			c, err := q.CreateSignalCorrection(ctx, store.CreateSignalCorrectionParams{WorkspaceID: actor.WorkspaceID, EventID: eventID, Action: "discard", FromPersonID: row.PersonID, Reason: reason, CreatedBy: optionalID(actor.UserID), CreatedAt: now})
			out = correctionFromRow(c)
			return err
		})
		if txErr != nil {
			return SignalCorrection{}, txErr
		}
	case "reattribute":
		if toPersonID == 0 {
			return SignalCorrection{}, core.Invalid("reattribute needs a person")
		}
		if row.PersonID.Valid && row.PersonID.Int64 == toPersonID {
			return SignalCorrection{}, core.Conflict("event %d already belongs to person %d", eventID, toPersonID)
		}
		p, err := s.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: toPersonID, WorkspaceID: actor.WorkspaceID})
		if errors.Is(err, sql.ErrNoRows) {
			return SignalCorrection{}, core.NotFound("person %d not found", toPersonID)
		}
		if err != nil {
			return SignalCorrection{}, err
		}
		if p.MergedIntoID.Valid {
			return SignalCorrection{}, core.Conflict("person %d was merged into %d; attribute to that person", toPersonID, p.MergedIntoID.Int64)
		}
		var enq sql.NullInt64
		if e, err := s.db.Reader().FindOpenEnquiry(ctx, store.FindOpenEnquiryParams{PersonID: toPersonID}); err == nil {
			enq = sql.NullInt64{Int64: e.ID, Valid: true}
		}
		txErr := s.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
			if err := q.SetInteractionEventPerson(ctx, store.SetInteractionEventPersonParams{PersonID: sql.NullInt64{Int64: toPersonID, Valid: true}, EnquiryID: enq, Confidence: "confirmed", ID: eventID}); err != nil {
				return err
			}
			c, err := q.CreateSignalCorrection(ctx, store.CreateSignalCorrectionParams{WorkspaceID: actor.WorkspaceID, EventID: eventID, Action: "reattribute", FromPersonID: row.PersonID, ToPersonID: sql.NullInt64{Int64: toPersonID, Valid: true}, Reason: reason, CreatedBy: optionalID(actor.UserID), CreatedAt: now})
			out = correctionFromRow(c)
			return err
		})
		if txErr != nil {
			return SignalCorrection{}, txErr
		}
	default:
		return SignalCorrection{}, core.Invalid("action must be discard or reattribute")
	}
	if row.PersonID.Valid {
		if _, err := s.Assess(ctx, actor, row.PersonID.Int64, "correction"); err != nil {
			return out, err
		}
	}
	if action == "reattribute" {
		if _, err := s.Assess(ctx, actor, toPersonID, "correction"); err != nil {
			return out, err
		}
	}
	return out, nil
}

// Score is a person's scoring view.
func (s *Signals) Score(ctx context.Context, actor core.Actor, personID int64) (ScoreView, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return ScoreView{}, core.ErrForbidden
	}
	if _, err := s.db.Reader().GetPerson(ctx, store.GetPersonParams{ID: personID, WorkspaceID: actor.WorkspaceID}); errors.Is(err, sql.ErrNoRows) {
		return ScoreView{}, core.NotFound("person %d not found", personID)
	} else if err != nil {
		return ScoreView{}, err
	}
	v := ScoreView{PersonID: personID, History: []ScoreAssessment{}, Events: []SignalEvent{}, Corrections: []SignalCorrection{}}
	hist, err := s.db.Reader().ListScoreAssessments(ctx, store.ListScoreAssessmentsParams{PersonID: personID, Limit: 50})
	if err != nil {
		return ScoreView{}, err
	}
	for i, r := range hist {
		a := assessmentFromRow(r)
		if i == 0 {
			latest := a
			v.Latest = &latest
		}
		v.History = append(v.History, a)
	}
	events, err := s.db.Reader().ListInteractionEventsForPerson(ctx, store.ListInteractionEventsForPersonParams{PersonID: optionalID(personID), Limit: 200})
	if err != nil {
		return ScoreView{}, err
	}
	for _, e := range events {
		v.Events = append(v.Events, eventFromRow(e))
		cs, err := s.db.Reader().ListSignalCorrectionsForEvent(ctx, e.ID)
		if err != nil {
			return ScoreView{}, err
		}
		for _, c := range cs {
			v.Corrections = append(v.Corrections, correctionFromRow(c))
		}
	}
	v.Rules, err = s.GetRules(ctx, actor)
	if err != nil {
		return ScoreView{}, err
	}
	return v, nil
}

// Unknown lists events whose identity could not be confirmed, so the
// uncertainty is visible and a marketer can attribute them by hand.
func (s *Signals) Unknown(ctx context.Context, actor core.Actor, limit int) ([]SignalEvent, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Reader().ListUnknownInteractionEvents(ctx, store.ListUnknownInteractionEventsParams{WorkspaceID: actor.WorkspaceID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]SignalEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, eventFromRow(r))
	}
	return out, nil
}

// Reassess re-scores everyone with an event in the window, so decay shows
// without a new event. The daily job calls it.
func (s *Signals) Reassess(ctx context.Context, actor core.Actor, window time.Duration) (int, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return 0, core.ErrForbidden
	}
	ids, err := s.db.Reader().ListPeopleWithEvents(ctx, store.ListPeopleWithEventsParams{WorkspaceID: actor.WorkspaceID, OccurredAt: s.clock.Now().UTC().Add(-window)})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if !id.Valid {
			continue
		}
		if _, err := s.Assess(ctx, actor, id.Int64, "decay"); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Derive turns records ADX already holds into events: a form submission
// linked to a person, a CRM status that differs from the last one recorded.
// Keys make a re-run idempotent.
func (s *Signals) Derive(ctx context.Context, actor core.Actor) (int, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return 0, core.ErrForbidden
	}
	n := 0
	subs, err := s.db.Reader().ListLinkedSubmissionsForSignals(ctx, store.ListLinkedSubmissionsForSignalsParams{WorkspaceID: actor.WorkspaceID, Limit: s.limit})
	if err != nil {
		return 0, err
	}
	for _, sub := range subs {
		payload, _ := json.Marshal(map[string]any{"submissionId": sub.ID, "pageId": sub.PageID, "projectId": sub.ProjectID.Int64})
		r, err := s.Ingest(ctx, actor, SignalInput{Source: "pages", Kind: "form_submit", OccurrenceKey: "submission:" + strconv.FormatInt(sub.ID, 10), OccurredAt: sub.ReceivedAt.UTC(), Identity: SignalIdentity{PersonID: sub.PersonID}, ProjectID: sub.ProjectID.Int64, Payload: payload})
		if err != nil {
			return n, err
		}
		if !r.Duplicate {
			n++
		}
	}
	leadRows, err := s.db.Reader().ListLinkedLeadsForSignals(ctx, store.ListLinkedLeadsForSignalsParams{WorkspaceID: actor.WorkspaceID, Limit: s.limit})
	if err != nil {
		return n, err
	}
	for _, l := range leadRows {
		status := strings.ToLower(strings.TrimSpace(l.StatusRaw.String))
		if status == "" {
			continue
		}
		last, err := s.lastStageChange(ctx, l.PersonID, l.ID)
		if err != nil {
			return n, err
		}
		if last == status {
			continue
		}
		payload, _ := json.Marshal(map[string]any{"leadId": l.ID, "status": status, "previous": last, "stage": l.Stage.String, "projectId": l.ProjectID.Int64})
		key := fmt.Sprintf("lead:%d:status:%s@%d", l.ID, status, l.StatusObservedAt.Time.UTC().Unix())
		r, err := s.Ingest(ctx, actor, SignalInput{Source: "crm", Kind: "stage_change", OccurrenceKey: key, OccurredAt: l.StatusObservedAt.Time.UTC(), Identity: SignalIdentity{PersonID: l.PersonID}, ProjectID: l.ProjectID.Int64, Payload: payload})
		if err != nil {
			return n, err
		}
		if !r.Duplicate {
			n++
		}
	}
	return n, nil
}

func (s *Signals) lastStageChange(ctx context.Context, personID, leadID int64) (string, error) {
	events, err := s.db.Reader().ListInteractionEventsForPerson(ctx, store.ListInteractionEventsForPersonParams{PersonID: optionalID(personID), Limit: 500})
	if err != nil {
		return "", err
	}
	for _, e := range events {
		if e.Kind != "stage_change" {
			continue
		}
		var p struct {
			LeadID int64  `json:"leadId"`
			Status string `json:"status"`
		}
		if json.Unmarshal([]byte(e.Payload), &p) == nil && p.LeadID == leadID {
			return p.Status, nil
		}
	}
	return "", nil
}

func (s *Signals) current(ctx context.Context, workspaceID int64) (*ScoringVersion, error) {
	row, err := s.db.Reader().GetCurrentScoringRules(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r, err := rulesFromRow(row)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func rulesFromRow(r store.ScoringRule) (ScoringVersion, error) {
	var doc ScoringDocument
	if err := json.Unmarshal([]byte(r.Document), &doc); err != nil {
		return ScoringVersion{}, fmt.Errorf("scoring rules v%d: %w", r.Version, err)
	}
	return ScoringVersion{Version: r.Version, Document: doc, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC()}, nil
}

func eventFromRow(r store.InteractionEvent) SignalEvent {
	e := SignalEvent{ID: r.ID, PersonID: idPtr(r.PersonID), EnquiryID: idPtr(r.EnquiryID), Source: r.Source, Kind: r.Kind, OccurrenceKey: r.OccurrenceKey, Confidence: r.Confidence, Identity: json.RawMessage(r.Identity), Payload: json.RawMessage(r.Payload),
		OccurredAt: r.OccurredAt.UTC(), ReceivedAt: r.ReceivedAt.UTC(), EvidenceID: idPtr(r.EvidenceID), DiscardReason: r.DiscardReason}
	if r.DiscardedAt.Valid {
		t := r.DiscardedAt.Time.UTC()
		e.DiscardedAt = &t
	}
	return e
}

func assessmentFromRow(r store.ScoreAssessment) ScoreAssessment {
	a := ScoreAssessment{ID: r.ID, PersonID: r.PersonID, RulesVersion: idPtr(r.RulesVersion), Trigger: r.Trigger, Fit: idPtr(r.Fit), Intent: idPtr(r.Intent), AssessedAt: r.AssessedAt.UTC()}
	if r.Temperature.Valid {
		t := r.Temperature.String
		a.Temperature = &t
	}
	_ = json.Unmarshal([]byte(r.Explanation), &a.Explanation)
	return a
}

func correctionFromRow(r store.SignalCorrection) SignalCorrection {
	return SignalCorrection{ID: r.ID, EventID: r.EventID, Action: r.Action, FromPersonID: idPtr(r.FromPersonID), ToPersonID: idPtr(r.ToPersonID), Reason: r.Reason, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC()}
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
