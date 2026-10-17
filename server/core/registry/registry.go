// Package registry holds what agents are allowed to be: skills with typed
// input and output, an evidence scope and permitted tools; agents that
// bundle skills under an account scope and never act above marketer;
// workflow definitions as bounded graphs. Versions are immutable. A new
// version is a draft; activating it is a marketer's call only when it does
// not widen access; wider access, and anything a model proposed, waits for
// an admin. Nothing here grants a privilege the saver does not hold.
package registry

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

// AgentCapability is one thing a skill may call. Kind read never changes
// anything; kind propose creates a proposal that policy and a person decide
// on. There is no kind that writes directly.
type AgentCapability struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"` // read, propose
	Domain      string `json:"domain"`
	Description string `json:"description"`
}

// Capabilities is the fixed catalogue. A document naming anything else is
// invalid; the assistant's read tools are here under their own names.
var Capabilities = []AgentCapability{
	{"get_analytics", "read", "reporting", "Spend, leads, qualified leads, deals, cost per lead, with coverage and freshness"},
	{"get_lead_coverage", "read", "leads", "Lead data quality and attribution match levels"},
	{"list_alerts", "read", "alerts", "Alerts from the anomaly checks"},
	{"get_connections", "read", "connections", "Connected accounts and their health"},
	{"read_brand_facts", "read", "brands", "Approved facts and claims for a brand or project"},
	{"read_research", "read", "research", "Research findings and comparative reports"},
	{"read_plan", "read", "plans", "A plan version and its validation"},
	{"read_people", "read", "crm", "People, enquiries and scores"},
	{"read_pipeline", "read", "pipeline", "Opportunities, assignments and follow-ups"},
	{"propose_change", "propose", "proposals", "Create a proposal for an external change; policy and approval decide"},
	{"propose_plan", "propose", "plans", "Generate a plan draft for a person to accept"},
	{"propose_draft_message", "propose", "outreach", "Draft a message that stays unsent until approved under the channel policy"},
}

func capability(name string) (AgentCapability, bool) {
	for _, c := range Capabilities {
		if c.Name == name {
			return c, true
		}
	}
	return AgentCapability{}, false
}

// EvidenceScopes a skill may read from. "public" is research of public
// pages; "account:*" is any connected account; "account:<id>" one account;
// "project:*" any project.
var evidenceScopePrefixes = []string{"public", "account:", "project:", "brand:", "person:"}

type SkillDocument struct {
	Name           string         `json:"name"`
	Instructions   string         `json:"instructions"`
	InputSchema    map[string]any `json:"inputSchema"`
	OutputSchema   map[string]any `json:"outputSchema"`
	EvidenceScope  []string       `json:"evidenceScope"`
	PermittedTools []string       `json:"permittedTools"`
	MaxTokens      int            `json:"maxTokens"`
}

type AgentDocument struct {
	Name         string   `json:"name"`
	Instructions string   `json:"instructions"`
	Skills       []string `json:"skills"`
	// AccountScope lists the account ids the agent may read or propose
	// against. Empty means none.
	AccountScope []int64 `json:"accountScope"`
	// Specialist agents do one domain; a super agent coordinates other
	// agents and holds no tools of its own.
	Coordinates []string `json:"coordinates"`
	Model       string   `json:"model"`
}

type WorkflowStep struct {
	Key       string            `json:"key"`
	Agent     string            `json:"agent"`
	Skill     string            `json:"skill"`
	DependsOn []string          `json:"dependsOn"`
	Inputs    map[string]string `json:"inputs"`
	// Loop repeats the step up to Max times while its output's "again"
	// field is true. Unbounded loops are refused.
	Loop *LoopSpec `json:"loop"`
	// External marks a step whose skill proposes a change; the orchestrator
	// gives such steps stable operation keys and never repeats one that
	// completed.
	External bool `json:"external"`
}

type LoopSpec struct {
	Max int `json:"max"`
}

type WorkflowDocument struct {
	Name         string         `json:"name"`
	Steps        []WorkflowStep `json:"steps"`
	MaxSteps     int            `json:"maxSteps"`
	BudgetTokens int64          `json:"budgetTokens"`
}

// RegistryEntry is one stored version of a skill, agent or workflow.
type RegistryEntry struct {
	ID          int64           `json:"id"`
	Kind        string          `json:"kind"`
	Key         string          `json:"key"`
	Version     int64           `json:"version"`
	State       string          `json:"state"`
	Source      string          `json:"source"`
	Document    json.RawMessage `json:"document"`
	Validation  []string        `json:"validation"`
	NeedsAdmin  bool            `json:"needsAdmin"`
	Reason      string          `json:"reason"`
	CreatedBy   *int64          `json:"createdBy"`
	CreatedAt   time.Time       `json:"createdAt"`
	ActivatedBy *int64          `json:"activatedBy"`
	ActivatedAt *time.Time      `json:"activatedAt"`
	RetiredAt   *time.Time      `json:"retiredAt"`
}

// Accounts is what the registry needs from connections: which account ids
// exist, so an agent cannot name one that is not connected.
type Accounts interface {
	ConnectedAccountIDs(ctx context.Context, actor core.Actor) ([]int64, error)
}

type Config struct {
	Clock    core.Clock
	Accounts Accounts
}

type Registry struct {
	db    *store.DB
	clock core.Clock
	cfg   Config
}

func New(db *store.DB, cfg Config) *Registry {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Registry{db: db, clock: cfg.Clock, cfg: cfg}
}

func (r *Registry) Catalog() []AgentCapability { return Capabilities }

// SaveSkill stores a new draft version. Source is "model" when the actor
// is the assistant or the caller says so; a model's draft always needs an
// admin. Validation problems are stored on the draft and block activation.
func (r *Registry) SaveSkill(ctx context.Context, actor core.Actor, key string, doc SkillDocument, fromModel bool) (RegistryEntry, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return RegistryEntry{}, core.ErrForbidden
	}
	key = normKey(key)
	if key == "" {
		return RegistryEntry{}, core.Invalid("a key is required (letters, digits, dots, dashes)")
	}
	problems := validateSkill(doc)
	source := "user"
	if fromModel || actor.Via == core.ViaAssistant {
		source = "model"
	}
	prev, err := r.active(ctx, actor.WorkspaceID, "skill", key)
	if err != nil {
		return RegistryEntry{}, err
	}
	needsAdmin, why := source == "model", ""
	if needsAdmin {
		why = "proposed by a model; an admin must accept it"
	}
	if prev != nil {
		var old SkillDocument
		_ = json.Unmarshal(prev.Document, &old)
		if widened := widerStrings(old.PermittedTools, doc.PermittedTools); len(widened) > 0 {
			needsAdmin = true
			why = join(why, "adds tools "+strings.Join(widened, ", "))
		}
		if widened := widerStrings(old.EvidenceScope, doc.EvidenceScope); len(widened) > 0 {
			needsAdmin = true
			why = join(why, "widens evidence scope by "+strings.Join(widened, ", "))
		}
	} else if len(doc.PermittedTools) > 0 || len(doc.EvidenceScope) > 0 {
		// A first version with any access is itself a grant.
		if !actor.AtLeast(core.RoleAdmin) || source == "model" {
			needsAdmin = true
			why = join(why, "a first version with tools or scope needs an admin")
		}
	}
	return r.create(ctx, actor, "skill", key, source, doc, problems, needsAdmin, why)
}

func validateSkill(doc SkillDocument) []string {
	var problems []string
	if strings.TrimSpace(doc.Name) == "" {
		problems = append(problems, "name is required")
	}
	if strings.TrimSpace(doc.Instructions) == "" {
		problems = append(problems, "instructions are required")
	}
	problems = append(problems, ValidateSchema(doc.InputSchema, "inputSchema")...)
	problems = append(problems, ValidateSchema(doc.OutputSchema, "outputSchema")...)
	for _, t := range doc.PermittedTools {
		if _, ok := capability(t); !ok {
			problems = append(problems, fmt.Sprintf("tool %q is not in the capability catalogue", t))
		}
	}
	for _, s := range doc.EvidenceScope {
		ok := false
		for _, p := range evidenceScopePrefixes {
			if s == p || (strings.HasSuffix(p, ":") && strings.HasPrefix(s, p)) {
				ok = true
			}
		}
		if !ok {
			problems = append(problems, fmt.Sprintf("evidence scope %q is not public, account:*, project:*, brand:* or person:*", s))
		}
	}
	if doc.MaxTokens < 0 || doc.MaxTokens > 32000 {
		problems = append(problems, "maxTokens must be between 0 and 32000")
	}
	return problems
}

// SaveAgent stores a new draft agent. Its tools are the union of its
// skills' tools, never more; its account scope must name connected
// accounts; widening either against the active version needs an admin.
func (r *Registry) SaveAgent(ctx context.Context, actor core.Actor, key string, doc AgentDocument, fromModel bool) (RegistryEntry, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return RegistryEntry{}, core.ErrForbidden
	}
	key = normKey(key)
	if key == "" {
		return RegistryEntry{}, core.Invalid("a key is required")
	}
	var problems []string
	if strings.TrimSpace(doc.Name) == "" {
		problems = append(problems, "name is required")
	}
	if len(doc.Skills) == 0 && len(doc.Coordinates) == 0 {
		problems = append(problems, "an agent needs skills, or agents to coordinate")
	}
	if len(doc.Skills) > 0 && len(doc.Coordinates) > 0 {
		problems = append(problems, "a super agent coordinates agents and holds no skills of its own")
	}
	for _, sk := range doc.Skills {
		if a, err := r.active(ctx, actor.WorkspaceID, "skill", sk); err != nil {
			return RegistryEntry{}, err
		} else if a == nil {
			problems = append(problems, fmt.Sprintf("skill %q has no active version", sk))
		}
	}
	for _, ag := range doc.Coordinates {
		if ag == key {
			problems = append(problems, "an agent cannot coordinate itself")
			continue
		}
		if a, err := r.active(ctx, actor.WorkspaceID, "agent", ag); err != nil {
			return RegistryEntry{}, err
		} else if a == nil {
			problems = append(problems, fmt.Sprintf("agent %q has no active version", ag))
		}
	}
	if r.cfg.Accounts != nil && len(doc.AccountScope) > 0 {
		ids, err := r.cfg.Accounts.ConnectedAccountIDs(ctx, actor)
		if err != nil {
			return RegistryEntry{}, err
		}
		known := map[int64]bool{}
		for _, id := range ids {
			known[id] = true
		}
		for _, id := range doc.AccountScope {
			if !known[id] {
				problems = append(problems, fmt.Sprintf("account %d is not connected", id))
			}
		}
	}
	source := "user"
	if fromModel || actor.Via == core.ViaAssistant {
		source = "model"
	}
	needsAdmin, why := source == "model", ""
	if needsAdmin {
		why = "proposed by a model; an admin must accept it"
	}
	prev, err := r.active(ctx, actor.WorkspaceID, "agent", key)
	if err != nil {
		return RegistryEntry{}, err
	}
	if prev != nil {
		var old AgentDocument
		_ = json.Unmarshal(prev.Document, &old)
		if widened := widerInts(old.AccountScope, doc.AccountScope); len(widened) > 0 {
			needsAdmin = true
			why = join(why, fmt.Sprintf("adds accounts %v", widened))
		}
		if widened := widerStrings(old.Skills, doc.Skills); len(widened) > 0 {
			needsAdmin = true
			why = join(why, "adds skills "+strings.Join(widened, ", "))
		}
		if widened := widerStrings(old.Coordinates, doc.Coordinates); len(widened) > 0 {
			needsAdmin = true
			why = join(why, "coordinates more agents: "+strings.Join(widened, ", "))
		}
	} else if !actor.AtLeast(core.RoleAdmin) || source == "model" {
		needsAdmin = true
		why = join(why, "a first agent version is a grant of access and needs an admin")
	}
	return r.create(ctx, actor, "agent", key, source, doc, problems, needsAdmin, why)
}

// SaveWorkflow stores a draft workflow definition with its validation:
// every step names an active agent or skill, dependencies exist, there is
// no cycle, loops are bounded, and the step count is bounded.
func (r *Registry) SaveWorkflow(ctx context.Context, actor core.Actor, key string, doc WorkflowDocument, fromModel bool) (RegistryEntry, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return RegistryEntry{}, core.ErrForbidden
	}
	key = normKey(key)
	if key == "" {
		return RegistryEntry{}, core.Invalid("a key is required")
	}
	problems, err := r.validateWorkflow(ctx, actor, doc)
	if err != nil {
		return RegistryEntry{}, err
	}
	source := "user"
	if fromModel || actor.Via == core.ViaAssistant {
		source = "model"
	}
	needsAdmin, why := source == "model", ""
	if needsAdmin {
		why = "proposed by a model; an admin must accept it"
	}
	for _, s := range doc.Steps {
		if s.External {
			needsAdmin = true
			why = join(why, "step "+s.Key+" proposes external changes")
			break
		}
	}
	return r.create(ctx, actor, "workflow", key, source, doc, problems, needsAdmin, why)
}

// ValidateWorkflow reports the problems of a draft without storing it.
func (r *Registry) ValidateWorkflow(ctx context.Context, actor core.Actor, doc WorkflowDocument) ([]string, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	return r.validateWorkflow(ctx, actor, doc)
}

func (r *Registry) validateWorkflow(ctx context.Context, actor core.Actor, doc WorkflowDocument) ([]string, error) {
	var problems []string
	if strings.TrimSpace(doc.Name) == "" {
		problems = append(problems, "name is required")
	}
	if len(doc.Steps) == 0 {
		problems = append(problems, "at least one step is required")
	}
	if doc.MaxSteps <= 0 || doc.MaxSteps > 200 {
		problems = append(problems, "maxSteps must be between 1 and 200")
	}
	keys := map[string]bool{}
	for _, s := range doc.Steps {
		if s.Key == "" || keys[s.Key] {
			problems = append(problems, "step keys must be unique and non-empty")
			continue
		}
		keys[s.Key] = true
	}
	total := 0
	for _, s := range doc.Steps {
		runs := 1
		if s.Loop != nil {
			if s.Loop.Max <= 0 || s.Loop.Max > 20 {
				problems = append(problems, fmt.Sprintf("step %s: loop max must be between 1 and 20 (unbounded loops are refused)", s.Key))
			} else {
				runs = s.Loop.Max
			}
		}
		total += runs
		if (s.Agent == "") == (s.Skill == "") {
			problems = append(problems, fmt.Sprintf("step %s: name exactly one of agent or skill", s.Key))
		}
		if s.Agent != "" {
			if a, err := r.active(ctx, actor.WorkspaceID, "agent", s.Agent); err != nil {
				return nil, err
			} else if a == nil {
				problems = append(problems, fmt.Sprintf("step %s: agent %q has no active version", s.Key, s.Agent))
			}
		}
		if s.Skill != "" {
			a, err := r.active(ctx, actor.WorkspaceID, "skill", s.Skill)
			if err != nil {
				return nil, err
			}
			if a == nil {
				problems = append(problems, fmt.Sprintf("step %s: skill %q has no active version", s.Key, s.Skill))
			} else {
				var sd SkillDocument
				_ = json.Unmarshal(a.Document, &sd)
				proposes := false
				for _, t := range sd.PermittedTools {
					if c, ok := capability(t); ok && c.Kind == "propose" {
						proposes = true
					}
				}
				if proposes && !s.External {
					problems = append(problems, fmt.Sprintf("step %s: skill %q proposes changes, so the step must be marked external", s.Key, s.Skill))
				}
			}
		}
		for _, d := range s.DependsOn {
			if !keys[d] {
				problems = append(problems, fmt.Sprintf("step %s: depends on unknown step %q", s.Key, d))
			}
			if d == s.Key {
				problems = append(problems, fmt.Sprintf("step %s: depends on itself", s.Key))
			}
		}
	}
	if doc.MaxSteps > 0 && total > doc.MaxSteps {
		problems = append(problems, fmt.Sprintf("the graph can run %d step executions, more than maxSteps %d", total, doc.MaxSteps))
	}
	if cycle := findCycle(doc.Steps); cycle != "" {
		problems = append(problems, "cycle: "+cycle)
	}
	return problems, nil
}

// findCycle returns a description of the first dependency cycle, or "".
func findCycle(steps []WorkflowStep) string {
	deps := map[string][]string{}
	for _, s := range steps {
		deps[s.Key] = s.DependsOn
	}
	state := map[string]int{}
	var path []string
	var visit func(k string) string
	visit = func(k string) string {
		switch state[k] {
		case 1:
			return strings.Join(append(path, k), " -> ")
		case 2:
			return ""
		}
		state[k] = 1
		path = append(path, k)
		for _, d := range deps[k] {
			if _, ok := deps[d]; !ok {
				continue
			}
			if c := visit(d); c != "" {
				return c
			}
		}
		path = path[:len(path)-1]
		state[k] = 2
		return ""
	}
	keys := make([]string, 0, len(deps))
	for k := range deps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if c := visit(k); c != "" {
			return c
		}
	}
	return ""
}

func (r *Registry) create(ctx context.Context, actor core.Actor, kind, key, source string, doc any, problems []string, needsAdmin bool, why string) (RegistryEntry, error) {
	raw, _ := json.Marshal(doc)
	if problems == nil {
		problems = []string{}
	}
	val, _ := json.Marshal(problems)
	var out RegistryEntry
	err := r.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var version int64 = 1
		if latest, err := q.GetLatestRegistryVersion(ctx, store.GetLatestRegistryVersionParams{WorkspaceID: actor.WorkspaceID, Kind: kind, Key: key}); err == nil {
			version = latest.Version + 1
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		row, err := q.CreateRegistryEntry(ctx, store.CreateRegistryEntryParams{WorkspaceID: actor.WorkspaceID, Kind: kind, Key: key, Version: version, Source: source, Document: string(raw), Validation: string(val), NeedsAdmin: boolInt(needsAdmin), Reason: why, CreatedBy: optionalID(actor.UserID), CreatedAt: r.clock.Now().UTC()})
		if err != nil {
			return err
		}
		out = entryFromRow(row)
		return nil
	})
	return out, err
}

// Activate makes a draft the active version and retires the previous one.
// A draft with validation problems is refused; one that needs an admin is
// refused for a marketer, whatever its instructions say.
func (r *Registry) Activate(ctx context.Context, actor core.Actor, id int64) (RegistryEntry, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.UserID == 0 || actor.Via == core.ViaAssistant {
		return RegistryEntry{}, core.ErrForbidden
	}
	row, err := r.db.Reader().GetRegistryEntry(ctx, store.GetRegistryEntryParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return RegistryEntry{}, core.NotFound("registry entry %d not found", id)
	}
	if err != nil {
		return RegistryEntry{}, err
	}
	if row.State != "draft" {
		return RegistryEntry{}, core.Conflict("version %d of %s %s is %s, not draft", row.Version, row.Kind, row.Key, row.State)
	}
	e := entryFromRow(row)
	if len(e.Validation) > 0 {
		return RegistryEntry{}, core.Invalid("the draft has %d validation problem(s): %s", len(e.Validation), strings.Join(e.Validation, "; "))
	}
	if e.NeedsAdmin && !actor.AtLeast(core.RoleAdmin) {
		return RegistryEntry{}, &core.Error{Kind: core.KindForbidden, Message: "this version needs an admin: " + e.Reason}
	}
	now := r.clock.Now().UTC()
	err = r.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		reason := e.Reason
		if e.NeedsAdmin {
			reason = fmt.Sprintf("accepted by admin %d: %s", actor.UserID, e.Reason)
		}
		n, err := q.ActivateRegistryEntry(ctx, store.ActivateRegistryEntryParams{ActivatedBy: optionalID(actor.UserID), ActivatedAt: sql.NullTime{Time: now, Valid: true}, Reason: reason, ID: id})
		if err != nil {
			return err
		}
		if n != 1 {
			return core.Conflict("the draft changed underneath")
		}
		return q.RetireActiveRegistryEntries(ctx, store.RetireActiveRegistryEntriesParams{RetiredAt: sql.NullTime{Time: now, Valid: true}, WorkspaceID: actor.WorkspaceID, Kind: row.Kind, Key: row.Key, ID: id})
	})
	if err != nil {
		return RegistryEntry{}, err
	}
	out, err := r.db.Reader().GetRegistryEntry(ctx, store.GetRegistryEntryParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return RegistryEntry{}, err
	}
	return entryFromRow(out), nil
}

func (r *Registry) Retire(ctx context.Context, actor core.Actor, id int64) (RegistryEntry, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return RegistryEntry{}, core.ErrForbidden
	}
	n, err := r.db.Writer().RetireRegistryEntry(ctx, store.RetireRegistryEntryParams{RetiredAt: sql.NullTime{Time: r.clock.Now().UTC(), Valid: true}, ID: id})
	if err != nil {
		return RegistryEntry{}, err
	}
	if n != 1 {
		return RegistryEntry{}, core.Conflict("entry %d is not draft or active", id)
	}
	row, err := r.db.Reader().GetRegistryEntry(ctx, store.GetRegistryEntryParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if err != nil {
		return RegistryEntry{}, err
	}
	return entryFromRow(row), nil
}

func (r *Registry) List(ctx context.Context, actor core.Actor, kind string) ([]RegistryEntry, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	switch kind {
	case "skill", "agent", "workflow":
	default:
		return nil, core.Invalid("kind must be skill, agent or workflow")
	}
	rows, err := r.db.Reader().ListRegistryEntries(ctx, store.ListRegistryEntriesParams{WorkspaceID: actor.WorkspaceID, Kind: kind})
	if err != nil {
		return nil, err
	}
	out := make([]RegistryEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, entryFromRow(row))
	}
	return out, nil
}

// Active returns the active version of a key, or nil.
func (r *Registry) Active(ctx context.Context, actor core.Actor, kind, key string) (*RegistryEntry, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	return r.active(ctx, actor.WorkspaceID, kind, key)
}

// ActiveSkill decodes the active skill.
func (r *Registry) ActiveSkill(ctx context.Context, actor core.Actor, key string) (*RegistryEntry, SkillDocument, error) {
	e, err := r.Active(ctx, actor, "skill", key)
	if err != nil || e == nil {
		return e, SkillDocument{}, err
	}
	var doc SkillDocument
	if err := json.Unmarshal(e.Document, &doc); err != nil {
		return nil, SkillDocument{}, err
	}
	return e, doc, nil
}

// ActiveAgent decodes the active agent.
func (r *Registry) ActiveAgent(ctx context.Context, actor core.Actor, key string) (*RegistryEntry, AgentDocument, error) {
	e, err := r.Active(ctx, actor, "agent", key)
	if err != nil || e == nil {
		return e, AgentDocument{}, err
	}
	var doc AgentDocument
	if err := json.Unmarshal(e.Document, &doc); err != nil {
		return nil, AgentDocument{}, err
	}
	return e, doc, nil
}

// ActiveWorkflow decodes the active workflow.
func (r *Registry) ActiveWorkflow(ctx context.Context, actor core.Actor, key string) (*RegistryEntry, WorkflowDocument, error) {
	e, err := r.Active(ctx, actor, "workflow", key)
	if err != nil || e == nil {
		return e, WorkflowDocument{}, err
	}
	var doc WorkflowDocument
	if err := json.Unmarshal(e.Document, &doc); err != nil {
		return nil, WorkflowDocument{}, err
	}
	return e, doc, nil
}

// CheckOutput validates a skill's output against the active version's
// schema. A malformed output is a list of problems, never accepted.
func (r *Registry) CheckOutput(ctx context.Context, actor core.Actor, skillKey string, output json.RawMessage) ([]string, error) {
	e, doc, err := r.ActiveSkill(ctx, actor, skillKey)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, core.NotFound("skill %q has no active version", skillKey)
	}
	return ValidateRaw(doc.OutputSchema, output), nil
}

// CheckInput validates a skill's input the same way.
func (r *Registry) CheckInput(ctx context.Context, actor core.Actor, skillKey string, input json.RawMessage) ([]string, error) {
	e, doc, err := r.ActiveSkill(ctx, actor, skillKey)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, core.NotFound("skill %q has no active version", skillKey)
	}
	return ValidateRaw(doc.InputSchema, input), nil
}

// AgentTools is the union of an agent's skills' permitted tools, resolved
// from active versions. An agent has no tools of its own to add.
func (r *Registry) AgentTools(ctx context.Context, actor core.Actor, key string) ([]string, error) {
	e, doc, err := r.ActiveAgent(ctx, actor, key)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, core.NotFound("agent %q has no active version", key)
	}
	set := map[string]bool{}
	for _, sk := range doc.Skills {
		_, sd, err := r.ActiveSkill(ctx, actor, sk)
		if err != nil {
			return nil, err
		}
		for _, t := range sd.PermittedTools {
			set[t] = true
		}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out, nil
}

func (r *Registry) active(ctx context.Context, workspaceID int64, kind, key string) (*RegistryEntry, error) {
	row, err := r.db.Reader().GetActiveRegistryEntry(ctx, store.GetActiveRegistryEntryParams{WorkspaceID: workspaceID, Kind: kind, Key: key})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e := entryFromRow(row)
	return &e, nil
}

func entryFromRow(r store.RegistryEntry) RegistryEntry {
	e := RegistryEntry{ID: r.ID, Kind: r.Kind, Key: r.Key, Version: r.Version, State: r.State, Source: r.Source, Document: json.RawMessage(r.Document), Validation: []string{}, NeedsAdmin: r.NeedsAdmin == 1, Reason: r.Reason, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC(), ActivatedBy: idPtr(r.ActivatedBy)}
	_ = json.Unmarshal([]byte(r.Validation), &e.Validation)
	if r.ActivatedAt.Valid {
		t := r.ActivatedAt.Time.UTC()
		e.ActivatedAt = &t
	}
	if r.RetiredAt.Valid {
		t := r.RetiredAt.Time.UTC()
		e.RetiredAt = &t
	}
	return e
}

func normKey(k string) string {
	k = strings.ToLower(strings.TrimSpace(k))
	for _, c := range k {
		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_'
		if !ok {
			return ""
		}
	}
	return k
}

// widerStrings lists what is in new but not in old.
func widerStrings(old, new []string) []string {
	have := map[string]bool{}
	for _, o := range old {
		have[o] = true
	}
	var out []string
	for _, n := range new {
		if !have[n] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func widerInts(old, new []int64) []int64 {
	have := map[int64]bool{}
	for _, o := range old {
		have[o] = true
	}
	var out []int64
	for _, n := range new {
		if !have[n] {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
