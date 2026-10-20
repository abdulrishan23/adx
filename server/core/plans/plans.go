// Package plans holds media plans as immutable, typed, validated versions.
// A plan version says where the money goes (allocations by channel), what
// has to exist before it can run (assets, pages, measurement), what it will
// test, what evidence it rests on and what it merely assumes. Validation is
// code: arithmetic, required fields, known channels, evidence that exists.
// An admin's acceptance is content acceptance of one version and is
// terminal for that version; it grants nothing in any ad account. Export
// has a generic form that always works and an exact-template form that is
// blocked until the actual Excel template is supplied (decisions.md).
package plans

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assets"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/learning"
	"github.com/WokeWorks/adx/server/store"
)

// Channels ADX can plan for. Fixed in code so a plan can't name a channel
// nothing implements; Supported says whether later phases can deploy it.
type ChannelInfo struct {
	Key       string `json:"key"`
	Platform  string `json:"platform"`
	Label     string `json:"label"`
	Supported bool   `json:"supported"`
}

var Channels = []ChannelInfo{
	{"google_search", "google_ads", "Google Search", true},
	{"google_pmax", "google_ads", "Google Performance Max", true},
	{"google_display", "google_ads", "Google Display", true},
	{"google_youtube", "google_ads", "YouTube", true},
	{"google_demand_gen", "google_ads", "Google Demand Gen", true},
	{"meta_lead_form", "meta", "Meta lead form", true},
	{"meta_traffic", "meta", "Meta traffic to landing page", true},
	{"meta_awareness", "meta", "Meta awareness", true},
	{"meta_whatsapp", "meta", "Meta click to WhatsApp", true},
	{"tiktok", "tiktok", "TikTok", false},
	{"snapchat", "snapchat", "Snapchat", false},
	{"linkedin", "linkedin", "LinkedIn", false},
	{"programmatic", "other", "Programmatic display", false},
	{"portals", "other", "Property portals (Bayut, Property Finder)", false},
	{"print_outdoor", "other", "Print and outdoor", false},
}

var channelByKey = func() map[string]ChannelInfo {
	m := map[string]ChannelInfo{}
	for _, c := range Channels {
		m[c.Key] = c
	}
	return m
}()

// PlanDocument is the content of one version (spec section 9, PlanVersion).
type PlanDocument struct {
	Objective string `json:"objective"`
	// BudgetMinor is the whole budget in minor units of Currency.
	BudgetMinor int64  `json:"budgetMinor"`
	Currency    string `json:"currency"`
	// Period is the flight, as dates.
	PeriodFrom              string                   `json:"periodFrom"`
	PeriodTo                string                   `json:"periodTo"`
	Allocations             []Allocation             `json:"allocations"`
	AssetRequirements       []AssetRequirement       `json:"assetRequirements"`
	PageRequirements        []PageRequirement        `json:"pageRequirements"`
	MeasurementRequirements []MeasurementRequirement `json:"measurementRequirements"`
	TestHypotheses          []TestHypothesis         `json:"testHypotheses"`
	ObservationWindows      []ObservationWindow      `json:"observationWindows"`
	EvidenceRefs            []PlanEvidenceRef        `json:"evidenceRefs"`
	Assumptions             []Assumption             `json:"assumptions"`
	Notes                   string                   `json:"notes"`
}

// Allocation is one line of the budget.
type Allocation struct {
	Channel     string `json:"channel"`
	Objective   string `json:"objective"`
	BudgetMinor int64  `json:"budgetMinor"`
	Geography   string `json:"geography"`
	Audience    string `json:"audience"`
	ProjectID   *int64 `json:"projectId"`
	// ExpectedCPLMinor is the planner's expectation, null when unknown. It
	// is an assumption unless an assumption row cites evidence for it.
	ExpectedCPLMinor *int64 `json:"expectedCplMinor"`
	Note             string `json:"note"`
}

type AssetRequirement struct {
	Format   assets.Format   `json:"format"`
	Language assets.Language `json:"language"`
	Count    int             `json:"count"`
	OfferID  *int64          `json:"offerId"`
	Note     string          `json:"note"`
}

type PageRequirement struct {
	// Kind is landing or thank_you.
	Kind     string `json:"kind"`
	Language string `json:"language"`
	URL      string `json:"url"`
	FormPath string `json:"formPath"`
	Note     string `json:"note"`
}

type MeasurementRequirement struct {
	// Kind is conversion_action, pixel_event, utm_scheme, crm_stage or call_tracking.
	Kind     string `json:"kind"`
	Platform string `json:"platform"`
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Note     string `json:"note"`
}

type TestHypothesis struct {
	Hypothesis string `json:"hypothesis"`
	Metric     string `json:"metric"`
	WindowDays int    `json:"windowDays"`
}

type ObservationWindow struct {
	Name string `json:"name"`
	Days int    `json:"days"`
}

// PlanEvidenceRef points at something stored: a research finding, a fact, an
// offer or an evidence row. Validation checks it exists in the workspace.
type PlanEvidenceRef struct {
	// Kind is research_finding, fact, offer or evidence.
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
	Note string `json:"note"`
}

// Assumption is something the plan relies on without proof. Class says how
// it stands: assumption (a planner's guess), unavailable (nobody has the
// figure), cited (an evidence ref backs it).
type Assumption struct {
	Key         string  `json:"key"`
	Text        string  `json:"text"`
	Value       *string `json:"value"`
	Class       string  `json:"class"`
	EvidenceRef *int    `json:"evidenceRef"`
}

type PlanState string

const (
	StateDraft      PlanState = "draft"
	StateValidating PlanState = "validating"
	StateReady      PlanState = "ready"
	StateNeedsInput PlanState = "needs_input"
	StateInvalid    PlanState = "invalid"
	StateAccepted   PlanState = "accepted"
)

// ValidationIssue is one finding of the validator. Code is stable.
type ValidationIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type PlanValidation struct {
	Blocking []ValidationIssue `json:"blocking"`
	Warnings []ValidationIssue `json:"warnings"`
	// AllocatedMinor is the sum of allocation budgets, for the editor.
	AllocatedMinor int64 `json:"allocatedMinor"`
}

type PlanVersion struct {
	ID         int64          `json:"id"`
	PlanID     int64          `json:"planId"`
	Version    int64          `json:"version"`
	Document   PlanDocument   `json:"document"`
	State      PlanState      `json:"state"`
	Validation PlanValidation `json:"validation"`
	Source     string         `json:"source"`
	Note       string         `json:"note"`
	CreatedBy  *int64         `json:"createdBy"`
	CreatedAt  time.Time      `json:"createdAt"`
	// Acceptance is set once an admin accepted this exact version.
	Acceptance *PlanAcceptance `json:"acceptance"`
	// ExecutionAuthorization is always null here: nothing in this domain
	// can authorize spend. Phase 3's proposals carry that reference.
	ExecutionAuthorization *string `json:"executionAuthorization"`
}

type PlanAcceptance struct {
	VersionID  int64     `json:"versionId"`
	AcceptedBy int64     `json:"acceptedBy"`
	AcceptedAt time.Time `json:"acceptedAt"`
}

// GrantsSpend is always false, the same way assets.ContentAcceptance never
// grants delivery.
func (PlanAcceptance) GrantsSpend() bool { return false }

type Plan struct {
	ID        int64  `json:"id"`
	BrandID   int64  `json:"brandId"`
	ProjectID *int64 `json:"projectId"`
	// CampaignID is the campaign the plan was made for, or nil.
	CampaignID *int64    `json:"campaignId"`
	Title      string    `json:"title"`
	CreatedBy  *int64    `json:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type PlanSummary struct {
	Plan
	LatestVersion int64     `json:"latestVersion"`
	LatestState   PlanState `json:"latestState"`
	Accepted      bool      `json:"accepted"`
}

type PlanDetail struct {
	Plan     Plan          `json:"plan"`
	Versions []PlanVersion `json:"versions"`
}

type PlanInput struct {
	BrandID    int64        `json:"brandId"`
	ProjectID  int64        `json:"projectId"`
	CampaignID int64        `json:"campaignId"`
	Title      string       `json:"title"`
	Document   PlanDocument `json:"document"`
	Note       string       `json:"note"`
}

type VersionInput struct {
	Document PlanDocument `json:"document"`
	Note     string       `json:"note"`
}

// TemplateGate says what export formats exist. The exact Excel template is
// blocked until Reportage supplies the file.
type TemplateGate struct {
	Generic []string           `json:"generic"`
	Exact   ExactTemplateState `json:"exact"`
}

// ExactTemplateState says whether the exact-template adapter exists.
type ExactTemplateState struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	Unblocks  string `json:"unblocks"`
}

// TemplateAdapter writes a plan version in some outside format and reads
// one back. The exact Excel adapter implements this once the template
// exists; until then only the generic adapters are registered.
type TemplateAdapter interface {
	Name() string
	ContentType() string
	Export(v PlanVersion) ([]byte, error)
	Import(data []byte) (PlanDocument, error)
}

// Memory is what plan generation asks of organizational memory.
type Memory interface {
	Findings(ctx context.Context, actor core.Actor) ([]learning.MemoryFinding, error)
}

type Config struct {
	Clock core.Clock
	// Exact is the exact-template adapter, nil while no template exists.
	Exact TemplateAdapter
	// Model drafts plan documents (Generate). Nil means generation is not
	// configured and says so.
	Model assistant.Model
	// Evidence records each generation and enforces the daily cap.
	Evidence *evidence.Store
	// Memory supplies validated and supported findings (core/learning) for
	// the generation context. Nil means none.
	Memory Memory
	// MaxGenerationsPerDay caps model calls per workspace. Default 40.
	MaxGenerationsPerDay int
	// GenerationMaxTokens caps the model's output per call. Default 8000.
	GenerationMaxTokens int
}

type Plans struct {
	db           *store.DB
	clock        core.Clock
	brands       *brands.Brands
	adapters     map[string]TemplateAdapter
	exact        TemplateAdapter
	model        assistant.Model
	evidence     *evidence.Store
	maxGenPerDay int
	genMaxTokens int
	memory       Memory
}

func New(db *store.DB, br *brands.Brands, cfg Config) *Plans {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	if cfg.MaxGenerationsPerDay <= 0 {
		cfg.MaxGenerationsPerDay = defaultMaxGenPerDay
	}
	if cfg.GenerationMaxTokens <= 0 {
		cfg.GenerationMaxTokens = defaultGenMaxTokens
	}
	p := &Plans{db: db, clock: cfg.Clock, brands: br, adapters: map[string]TemplateAdapter{}, exact: cfg.Exact, model: cfg.Model, evidence: cfg.Evidence, maxGenPerDay: cfg.MaxGenerationsPerDay, genMaxTokens: cfg.GenerationMaxTokens, memory: cfg.Memory}
	for _, a := range []TemplateAdapter{JSONAdapter{}, CSVAdapter{}} {
		p.adapters[a.Name()] = a
	}
	if cfg.Exact != nil {
		p.adapters[cfg.Exact.Name()] = cfg.Exact
	}
	return p
}

// Templates reports the gate.
func (p *Plans) Templates() TemplateGate {
	g := TemplateGate{Generic: []string{"json", "csv"}}
	if p.exact != nil {
		g.Exact.Available = true
		g.Exact.Reason = "exact template adapter " + p.exact.Name()
		return g
	}
	g.Exact.Reason = "Reportage's Excel media-plan template has not been supplied, so there is no exact-template export"
	g.Exact.Unblocks = "the actual .xlsx template and a note on which cells carry what; then an adapter is written and round-trip tested against it"
	return g
}

// Create starts a plan at version 1. Marketers may. The version is
// validated on the way in and stored with its result.
func (p *Plans) Create(ctx context.Context, actor core.Actor, in PlanInput) (PlanDetail, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return PlanDetail{}, core.ErrForbidden
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 160 {
		return PlanDetail{}, core.Invalid("a title of up to 160 characters is required")
	}
	if in.BrandID == 0 {
		return PlanDetail{}, core.Invalid("a brand is required")
	}
	if _, err := p.brands.Get(ctx, actor, in.BrandID); err != nil {
		return PlanDetail{}, err
	}
	if in.ProjectID != 0 {
		owner, err := p.brands.BrandOfProject(ctx, actor, in.ProjectID)
		if err != nil {
			return PlanDetail{}, err
		}
		if owner != in.BrandID {
			return PlanDetail{}, core.Invalid("project %d does not belong to brand %d", in.ProjectID, in.BrandID)
		}
	}
	doc := normalize(in.Document)
	validation := p.Validate(ctx, actor, in.BrandID, doc)
	raw, _ := json.Marshal(doc)
	vraw, _ := json.Marshal(validation)
	now := p.clock.Now().UTC()
	var prow store.Plan
	err := p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		prow, err = q.CreatePlan(ctx, store.CreatePlanParams{WorkspaceID: actor.WorkspaceID, BrandID: in.BrandID, ProjectID: optionalID(in.ProjectID), CampaignID: optionalID(in.CampaignID), Title: in.Title, CreatedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		_, err = q.CreatePlanVersion(ctx, store.CreatePlanVersionParams{PlanID: prow.ID, Version: 1, Document: string(raw), State: string(StateFor(validation)), Validation: string(vraw), Source: "manual", Note: strings.TrimSpace(in.Note), CreatedBy: optionalID(actor.UserID), CreatedAt: now})
		return err
	})
	if err != nil {
		return PlanDetail{}, err
	}
	return p.Get(ctx, actor, prow.ID)
}

// SaveVersion adds the next version with a changed document. Marketers may.
// Earlier versions, accepted or not, are untouched.
func (p *Plans) SaveVersion(ctx context.Context, actor core.Actor, planID int64, in VersionInput) (PlanVersion, error) {
	return p.saveVersion(ctx, actor, planID, in, "manual")
}

func (p *Plans) saveVersion(ctx context.Context, actor core.Actor, planID int64, in VersionInput, source string) (PlanVersion, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return PlanVersion{}, core.ErrForbidden
	}
	prow, err := p.plan(ctx, actor.WorkspaceID, planID)
	if err != nil {
		return PlanVersion{}, err
	}
	doc := normalize(in.Document)
	validation := p.Validate(ctx, actor, prow.BrandID, doc)
	raw, _ := json.Marshal(doc)
	vraw, _ := json.Marshal(validation)
	now := p.clock.Now().UTC()
	var vrow store.PlanVersion
	err = p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		latest, err := q.GetLatestPlanVersion(ctx, planID)
		if err != nil {
			return err
		}
		vrow, err = q.CreatePlanVersion(ctx, store.CreatePlanVersionParams{PlanID: planID, Version: latest.Version + 1, Document: string(raw), State: string(StateFor(validation)), Validation: string(vraw), Source: source, Note: strings.TrimSpace(in.Note), CreatedBy: optionalID(actor.UserID), CreatedAt: now})
		if err != nil {
			return err
		}
		return q.TouchPlan(ctx, store.TouchPlanParams{UpdatedAt: now, ID: planID})
	})
	if store.IsUniqueViolation(err) {
		return PlanVersion{}, core.Conflict("someone saved this plan at the same time; reload and try again")
	}
	if err != nil {
		return PlanVersion{}, err
	}
	return versionFromRow(vrow)
}

// Accept is an admin's content acceptance of one ready version. It is
// terminal for that version and authorizes no spend; later edits are new
// versions without acceptance.
func (p *Plans) Accept(ctx context.Context, actor core.Actor, planID, version int64) (PlanVersion, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return PlanVersion{}, core.ErrForbidden
	}
	if _, err := p.plan(ctx, actor.WorkspaceID, planID); err != nil {
		return PlanVersion{}, err
	}
	now := p.clock.Now().UTC()
	var vrow store.PlanVersion
	err := p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		v, err := q.GetPlanVersion(ctx, store.GetPlanVersionParams{PlanID: planID, Version: version})
		if errors.Is(err, sql.ErrNoRows) {
			return core.NotFound("plan %d has no version %d", planID, version)
		}
		if err != nil {
			return err
		}
		switch PlanState(v.State) {
		case StateAccepted:
			vrow = v
			return nil
		case StateReady:
		default:
			return core.Conflict("version %d is %s; only a ready version can be accepted", version, v.State)
		}
		if err := q.AcceptPlanVersion(ctx, store.AcceptPlanVersionParams{AcceptedBy: optionalID(actor.UserID), AcceptedAt: sql.NullTime{Time: now, Valid: true}, ID: v.ID}); err != nil {
			return err
		}
		vrow, err = q.GetPlanVersion(ctx, store.GetPlanVersionParams{PlanID: planID, Version: version})
		if err != nil {
			return err
		}
		return q.TouchPlan(ctx, store.TouchPlanParams{UpdatedAt: now, ID: planID})
	})
	if err != nil {
		return PlanVersion{}, err
	}
	return versionFromRow(vrow)
}

func (p *Plans) List(ctx context.Context, actor core.Actor) ([]PlanSummary, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := p.db.Reader().ListPlans(ctx, store.ListPlansParams{WorkspaceID: actor.WorkspaceID, Limit: 200})
	if err != nil {
		return nil, err
	}
	out := make([]PlanSummary, 0, len(rows))
	for _, r := range rows {
		s := PlanSummary{Plan: planFromRow(r)}
		versions, err := p.db.Reader().ListPlanVersions(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		for _, v := range versions {
			s.LatestVersion, s.LatestState = v.Version, PlanState(v.State)
			if PlanState(v.State) == StateAccepted {
				s.Accepted = true
			}
		}
		out = append(out, s)
	}
	return out, nil
}

func (p *Plans) Get(ctx context.Context, actor core.Actor, id int64) (PlanDetail, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return PlanDetail{}, core.ErrForbidden
	}
	prow, err := p.plan(ctx, actor.WorkspaceID, id)
	if err != nil {
		return PlanDetail{}, err
	}
	rows, err := p.db.Reader().ListPlanVersions(ctx, id)
	if err != nil {
		return PlanDetail{}, err
	}
	d := PlanDetail{Plan: planFromRow(prow), Versions: make([]PlanVersion, 0, len(rows))}
	for _, r := range rows {
		v, err := versionFromRow(r)
		if err != nil {
			return PlanDetail{}, err
		}
		d.Versions = append(d.Versions, v)
	}
	return d, nil
}

// Version is one version of a plan.
func (p *Plans) Version(ctx context.Context, actor core.Actor, planID, version int64) (PlanVersion, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return PlanVersion{}, core.ErrForbidden
	}
	if _, err := p.plan(ctx, actor.WorkspaceID, planID); err != nil {
		return PlanVersion{}, err
	}
	row, err := p.db.Reader().GetPlanVersion(ctx, store.GetPlanVersionParams{PlanID: planID, Version: version})
	if errors.Is(err, sql.ErrNoRows) {
		return PlanVersion{}, core.NotFound("plan %d has no version %d", planID, version)
	}
	if err != nil {
		return PlanVersion{}, err
	}
	return versionFromRow(row)
}

// Export writes a version in a named format. The exact template format is a
// conflict, not an error, while no template exists.
func (p *Plans) Export(ctx context.Context, actor core.Actor, planID, version int64, format string) ([]byte, string, error) {
	v, err := p.Version(ctx, actor, planID, version)
	if err != nil {
		return nil, "", err
	}
	a, ok := p.adapters[strings.ToLower(format)]
	if !ok {
		if strings.ToLower(format) == "xlsx" || strings.ToLower(format) == "exact" {
			return nil, "", core.Conflict("%s", p.Templates().Exact.Reason)
		}
		return nil, "", core.Invalid("format must be json or csv")
	}
	data, err := a.Export(v)
	if err != nil {
		return nil, "", err
	}
	return data, a.ContentType(), nil
}

// Import reads a document from an adapter's format and saves it as the
// next version. The exact template import is blocked the same way.
func (p *Plans) Import(ctx context.Context, actor core.Actor, planID int64, format string, data []byte, note string) (PlanVersion, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return PlanVersion{}, core.ErrForbidden
	}
	a, ok := p.adapters[strings.ToLower(format)]
	if !ok {
		if strings.ToLower(format) == "xlsx" || strings.ToLower(format) == "exact" {
			return PlanVersion{}, core.Conflict("%s", p.Templates().Exact.Reason)
		}
		return PlanVersion{}, core.Invalid("format must be json or csv")
	}
	doc, err := a.Import(data)
	if err != nil {
		return PlanVersion{}, core.Invalid("the file could not be read as a %s plan: %v", a.Name(), err)
	}
	return p.saveVersion(ctx, actor, planID, VersionInput{Document: doc, Note: note}, "imported")
}

// Validate checks a document in code. Blocking issues keep the version from
// ready; warnings don't. Nothing here calls a model.
func (p *Plans) Validate(ctx context.Context, actor core.Actor, brandID int64, d PlanDocument) PlanValidation {
	v := PlanValidation{Blocking: []ValidationIssue{}, Warnings: []ValidationIssue{}}
	block := func(code, path, msg string) { v.Blocking = append(v.Blocking, ValidationIssue{code, path, msg}) }
	warn := func(code, path, msg string) { v.Warnings = append(v.Warnings, ValidationIssue{code, path, msg}) }

	if strings.TrimSpace(d.Objective) == "" {
		block("objective_missing", "objective", "Say what the plan is for.")
	}
	if d.BudgetMinor <= 0 {
		block("budget_missing", "budgetMinor", "A budget above zero is required.")
	}
	if len(d.Currency) != 3 {
		block("currency_missing", "currency", "A three-letter currency code is required.")
	}
	from, errFrom := time.Parse("2006-01-02", d.PeriodFrom)
	to, errTo := time.Parse("2006-01-02", d.PeriodTo)
	switch {
	case d.PeriodFrom == "" || d.PeriodTo == "":
		block("period_missing", "periodFrom", "The flight needs a start and an end date.")
	case errFrom != nil || errTo != nil:
		block("period_invalid", "periodFrom", "Dates look like 2026-11-01.")
	case to.Before(from):
		block("period_reversed", "periodTo", "The end date is before the start.")
	}
	if len(d.Allocations) == 0 {
		block("no_allocations", "allocations", "At least one allocation is required.")
	}
	var sum int64
	for i, a := range d.Allocations {
		path := fmt.Sprintf("allocations[%d]", i)
		ch, ok := channelByKey[a.Channel]
		switch {
		case a.Channel == "":
			block("channel_missing", path+".channel", "Pick a channel.")
		case !ok:
			block("channel_unknown", path+".channel", fmt.Sprintf("%q is not a channel ADX knows.", a.Channel))
		case !ch.Supported:
			warn("channel_unsupported", path+".channel", ch.Label+" can be planned but ADX cannot deploy or measure it; it stays manual.")
		}
		if a.BudgetMinor <= 0 {
			block("allocation_budget_missing", path+".budgetMinor", "Each allocation needs a budget above zero.")
		}
		sum += a.BudgetMinor
		if a.ExpectedCPLMinor == nil {
			warn("cpl_unavailable", path+".expectedCplMinor", "No expected cost per lead; the plan can't project leads for this line.")
		}
		if a.ProjectID != nil && *a.ProjectID != 0 {
			owner, err := p.brands.BrandOfProject(ctx, actor, *a.ProjectID)
			if err != nil || owner != brandID {
				block("project_foreign", path+".projectId", fmt.Sprintf("Project %d is not this brand's.", *a.ProjectID))
			}
		}
	}
	v.AllocatedMinor = sum
	if len(d.Allocations) > 0 && d.BudgetMinor > 0 && sum != d.BudgetMinor {
		block("budget_mismatch", "allocations", fmt.Sprintf("Allocations total %s but the budget is %s.", money(sum, d.Currency), money(d.BudgetMinor, d.Currency)))
	}
	for i, r := range d.AssetRequirements {
		path := fmt.Sprintf("assetRequirements[%d]", i)
		if _, ok := assets.Dimensions[r.Format]; !ok {
			block("asset_format_unknown", path+".format", "Format must be 9:16, 4:5, 1:1 or 1.91:1.")
		}
		if r.Language != assets.English && r.Language != assets.Arabic {
			block("asset_language_unknown", path+".language", "Language must be en or ar.")
		}
		if r.Count <= 0 {
			block("asset_count_missing", path+".count", "How many of this asset?")
		}
		if r.OfferID != nil && *r.OfferID != 0 {
			if _, _, err := p.brands.OfferVersions(ctx, actor, *r.OfferID); err != nil {
				block("offer_missing", path+".offerId", fmt.Sprintf("Offer %d does not exist.", *r.OfferID))
			}
		}
	}
	for i, r := range d.PageRequirements {
		path := fmt.Sprintf("pageRequirements[%d]", i)
		if r.Kind != "landing" && r.Kind != "thank_you" {
			block("page_kind_unknown", path+".kind", "A page is landing or thank_you.")
		}
		if r.Language != "en" && r.Language != "ar" {
			block("page_language_unknown", path+".language", "Language must be en or ar.")
		}
	}
	if len(d.MeasurementRequirements) == 0 {
		warn("measurement_missing", "measurementRequirements", "No measurement requirement: leads from this plan won't be attributable.")
	}
	for i, r := range d.MeasurementRequirements {
		path := fmt.Sprintf("measurementRequirements[%d]", i)
		switch r.Kind {
		case "conversion_action", "pixel_event", "utm_scheme", "crm_stage", "call_tracking":
		default:
			block("measurement_kind_unknown", path+".kind", "Measurement is conversion_action, pixel_event, utm_scheme, crm_stage or call_tracking.")
		}
		if strings.TrimSpace(r.Name) == "" {
			block("measurement_name_missing", path+".name", "Name the measurement.")
		}
	}
	for i, t := range d.TestHypotheses {
		if strings.TrimSpace(t.Hypothesis) == "" || strings.TrimSpace(t.Metric) == "" {
			block("hypothesis_incomplete", fmt.Sprintf("testHypotheses[%d]", i), "A hypothesis needs a statement and a metric.")
		}
		if t.WindowDays <= 0 {
			warn("hypothesis_window_missing", fmt.Sprintf("testHypotheses[%d].windowDays", i), "No observation window; the test can't be called.")
		}
	}
	for i, ref := range d.EvidenceRefs {
		path := fmt.Sprintf("evidenceRefs[%d]", i)
		if !p.evidenceExists(ctx, actor, ref) {
			block("evidence_missing", path, fmt.Sprintf("%s %d does not exist in this workspace.", ref.Kind, ref.ID))
		}
	}
	for i, a := range d.Assumptions {
		path := fmt.Sprintf("assumptions[%d]", i)
		switch a.Class {
		case "assumption":
			warn("assumption", path, a.Key+": "+a.Text)
		case "unavailable":
			warn("input_unavailable", path, a.Key+": no figure is available; the plan proceeds without it.")
		case "cited":
			if a.EvidenceRef == nil || *a.EvidenceRef < 0 || *a.EvidenceRef >= len(d.EvidenceRefs) {
				block("citation_dangling", path+".evidenceRef", "A cited assumption must point at one of the plan's evidence refs.")
			}
		default:
			block("assumption_class_unknown", path+".class", "An assumption is assumption, unavailable or cited.")
		}
	}
	if len(d.EvidenceRefs) == 0 {
		warn("no_evidence", "evidenceRefs", "The plan cites no research, fact or offer.")
	}
	return v
}

func (p *Plans) evidenceExists(ctx context.Context, actor core.Actor, ref PlanEvidenceRef) bool {
	q := p.db.Reader()
	switch ref.Kind {
	case "research_finding":
		var n int64
		err := p.db.Read.QueryRowContext(ctx, `SELECT COUNT(*) FROM research_findings WHERE id = ? AND workspace_id = ?`, ref.ID, actor.WorkspaceID).Scan(&n)
		return err == nil && n > 0
	case "fact":
		_, err := q.GetFact(ctx, store.GetFactParams{ID: ref.ID, WorkspaceID: actor.WorkspaceID})
		return err == nil
	case "offer":
		_, err := q.GetOffer(ctx, store.GetOfferParams{ID: ref.ID, WorkspaceID: actor.WorkspaceID})
		return err == nil
	case "evidence":
		_, err := q.GetEvidenceByID(ctx, store.GetEvidenceByIDParams{ID: ref.ID, WorkspaceID: actor.WorkspaceID})
		return err == nil
	}
	return false
}

// StateFor maps validation to the state machine: ready, needs_input (a
// required input is missing) or invalid (what's there contradicts itself).
func StateFor(v PlanValidation) PlanState {
	if len(v.Blocking) == 0 {
		return StateReady
	}
	for _, b := range v.Blocking {
		switch b.Code {
		case "budget_mismatch", "period_reversed", "period_invalid", "channel_unknown", "project_foreign", "evidence_missing", "offer_missing", "citation_dangling", "asset_format_unknown", "asset_language_unknown", "page_kind_unknown", "page_language_unknown", "measurement_kind_unknown", "assumption_class_unknown":
			return StateInvalid
		}
	}
	return StateNeedsInput
}

func normalize(d PlanDocument) PlanDocument {
	d.Objective = strings.TrimSpace(d.Objective)
	d.Currency = strings.ToUpper(strings.TrimSpace(d.Currency))
	d.PeriodFrom, d.PeriodTo = strings.TrimSpace(d.PeriodFrom), strings.TrimSpace(d.PeriodTo)
	d.Notes = strings.TrimSpace(d.Notes)
	if d.Allocations == nil {
		d.Allocations = []Allocation{}
	}
	for i := range d.Allocations {
		d.Allocations[i].Channel = strings.ToLower(strings.TrimSpace(d.Allocations[i].Channel))
		d.Allocations[i].Geography = strings.ToUpper(strings.TrimSpace(d.Allocations[i].Geography))
	}
	if d.AssetRequirements == nil {
		d.AssetRequirements = []AssetRequirement{}
	}
	if d.PageRequirements == nil {
		d.PageRequirements = []PageRequirement{}
	}
	if d.MeasurementRequirements == nil {
		d.MeasurementRequirements = []MeasurementRequirement{}
	}
	if d.TestHypotheses == nil {
		d.TestHypotheses = []TestHypothesis{}
	}
	if d.ObservationWindows == nil {
		d.ObservationWindows = []ObservationWindow{}
	}
	if d.EvidenceRefs == nil {
		d.EvidenceRefs = []PlanEvidenceRef{}
	}
	if d.Assumptions == nil {
		d.Assumptions = []Assumption{}
	}
	return d
}

func money(minor int64, currency string) string {
	return fmt.Sprintf("%s %s", currency, strconv.FormatFloat(float64(minor)/100, 'f', 2, 64))
}

func (p *Plans) plan(ctx context.Context, workspaceID, id int64) (store.Plan, error) {
	row, err := p.db.Reader().GetPlan(ctx, store.GetPlanParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Plan{}, core.NotFound("plan %d not found", id)
	}
	return row, err
}

func planFromRow(r store.Plan) Plan {
	return Plan{ID: r.ID, BrandID: r.BrandID, ProjectID: idPtr(r.ProjectID), CampaignID: idPtr(r.CampaignID), Title: r.Title, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

func versionFromRow(r store.PlanVersion) (PlanVersion, error) {
	v := PlanVersion{ID: r.ID, PlanID: r.PlanID, Version: r.Version, State: PlanState(r.State), Source: r.Source, Note: r.Note, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC()}
	if err := json.Unmarshal([]byte(r.Document), &v.Document); err != nil {
		return PlanVersion{}, err
	}
	v.Document = normalize(v.Document)
	if err := json.Unmarshal([]byte(r.Validation), &v.Validation); err != nil {
		return PlanVersion{}, err
	}
	if v.Validation.Blocking == nil {
		v.Validation.Blocking = []ValidationIssue{}
	}
	if v.Validation.Warnings == nil {
		v.Validation.Warnings = []ValidationIssue{}
	}
	if r.AcceptedBy.Valid && r.AcceptedAt.Valid {
		v.Acceptance = &PlanAcceptance{VersionID: r.ID, AcceptedBy: r.AcceptedBy.Int64, AcceptedAt: r.AcceptedAt.Time.UTC()}
	}
	return v, nil
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// Generic adapters. They carry the whole document, so a round trip is
// lossless; they are not Reportage's template.

type JSONAdapter struct{}

func (JSONAdapter) Name() string        { return "json" }
func (JSONAdapter) ContentType() string { return "application/json" }
func (JSONAdapter) Export(v PlanVersion) ([]byte, error) {
	return json.MarshalIndent(map[string]any{"adx_plan_version": 1, "plan": v.PlanID, "version": v.Version, "state": v.State, "document": v.Document}, "", "  ")
}
func (JSONAdapter) Import(data []byte) (PlanDocument, error) {
	var wrapped struct {
		Document *PlanDocument `json:"document"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Document != nil {
		return *wrapped.Document, nil
	}
	var doc PlanDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return PlanDocument{}, err
	}
	return doc, nil
}

// CSVAdapter writes the allocation table with the plan header in the first
// rows. It carries allocations only; requirements don't fit a flat sheet.
type CSVAdapter struct{}

func (CSVAdapter) Name() string        { return "csv" }
func (CSVAdapter) ContentType() string { return "text/csv; charset=utf-8" }

var csvHeader = []string{"channel", "objective", "budget_minor", "currency", "geography", "audience", "project_id", "expected_cpl_minor", "note"}

func (CSVAdapter) Export(v PlanVersion) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	d := v.Document
	_ = w.Write([]string{"#plan", strconv.FormatInt(v.PlanID, 10), "version", strconv.FormatInt(v.Version, 10), "objective", d.Objective, "budget_minor", strconv.FormatInt(d.BudgetMinor, 10), "currency", d.Currency, "from", d.PeriodFrom, "to", d.PeriodTo})
	_ = w.Write(csvHeader)
	for _, a := range d.Allocations {
		pid, cpl := "", ""
		if a.ProjectID != nil {
			pid = strconv.FormatInt(*a.ProjectID, 10)
		}
		if a.ExpectedCPLMinor != nil {
			cpl = strconv.FormatInt(*a.ExpectedCPLMinor, 10)
		}
		_ = w.Write([]string{a.Channel, a.Objective, strconv.FormatInt(a.BudgetMinor, 10), d.Currency, a.Geography, a.Audience, pid, cpl, a.Note})
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func (CSVAdapter) Import(data []byte) (PlanDocument, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return PlanDocument{}, err
	}
	if len(rows) < 2 {
		return PlanDocument{}, errors.New("expected a #plan row and a header row")
	}
	d := PlanDocument{}
	head := rows[0]
	for i := 0; i+1 < len(head); i += 2 {
		switch head[i] {
		case "objective":
			d.Objective = head[i+1]
		case "budget_minor":
			d.BudgetMinor, _ = strconv.ParseInt(head[i+1], 10, 64)
		case "currency":
			d.Currency = head[i+1]
		case "from":
			d.PeriodFrom = head[i+1]
		case "to":
			d.PeriodTo = head[i+1]
		}
	}
	col := map[string]int{}
	for i, h := range rows[1] {
		col[h] = i
	}
	for _, name := range []string{"channel", "budget_minor"} {
		if _, ok := col[name]; !ok {
			return PlanDocument{}, fmt.Errorf("missing column %q", name)
		}
	}
	get := func(row []string, name string) string {
		if i, ok := col[name]; ok && i < len(row) {
			return row[i]
		}
		return ""
	}
	for _, row := range rows[2:] {
		if len(row) == 0 || strings.TrimSpace(strings.Join(row, "")) == "" {
			continue
		}
		a := Allocation{Channel: get(row, "channel"), Objective: get(row, "objective"), Geography: get(row, "geography"), Audience: get(row, "audience"), Note: get(row, "note")}
		a.BudgetMinor, _ = strconv.ParseInt(get(row, "budget_minor"), 10, 64)
		if s := get(row, "project_id"); s != "" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				a.ProjectID = &n
			}
		}
		if s := get(row, "expected_cpl_minor"); s != "" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				a.ExpectedCPLMinor = &n
			}
		}
		if d.Currency == "" {
			d.Currency = get(row, "currency")
		}
		d.Allocations = append(d.Allocations, a)
	}
	return d, nil
}

// SortedChannels is the catalog in a stable order for the editor.
func SortedChannels() []ChannelInfo {
	out := append([]ChannelInfo(nil), Channels...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Supported != out[j].Supported {
			return out[i].Supported
		}
		return out[i].Label < out[j].Label
	})
	return out
}
