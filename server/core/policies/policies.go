// Package policies decides whether an actor may make a given change to an
// outside account. It holds the permission checklist, the caps and the kill
// switches (D-13) as a versioned document per workspace, and evaluates a
// request against the document, the actor's role, the account's proven
// capabilities and the switches.
//
// Nothing here is business policy: the document starts absent, and absent
// denies every external write. The owner configures it. SuggestedDefaults
// is D-13's table for an admin to apply deliberately; ADX never applies it
// on its own.
package policies

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

// WriteAction is a kind of change to an outside system. The catalog is fixed in
// code so a policy can't name an action ADX doesn't implement.
type WriteAction string

const (
	AudiencesUpsert    WriteAction = "audiences.upsert"
	ConversionsUpload  WriteAction = "conversions.upload"
	ExclusionsUpdate   WriteAction = "exclusions.update"
	LeadFieldsSet      WriteAction = "ghl.lead_fields.set"
	AdPause            WriteAction = "ad.pause"
	BudgetLower        WriteAction = "budget.lower"
	CampaignCreatePaus WriteAction = "campaign.create_paused"
	AdSetPause         WriteAction = "adset.pause"
	CampaignPause      WriteAction = "campaign.pause"
	CampaignResume     WriteAction = "campaign.resume"
	BudgetRaise        WriteAction = "budget.raise"
	BidsChange         WriteAction = "bids.change"
	Launch             WriteAction = "launch"
	TargetingChange    WriteAction = "targeting.change"
	OrganicPost        WriteAction = "organic.post"
)

// Catalog lists every action with a short description, for the policy editor.
var Catalog = []WriteActionInfo{
	{AudiencesUpsert, "Create or update audiences and Customer Match lists"},
	{ConversionsUpload, "Upload offline conversions and Conversions API events"},
	{ExclusionsUpdate, "Exclusion lists, labels, negative keywords"},
	{LeadFieldsSet, "Set lead temperature and score in GoHighLevel"},
	{AdPause, "Pause an ad"},
	{BudgetLower, "Lower a budget"},
	{CampaignCreatePaus, "Create a campaign or ad (paused)"},
	{AdSetPause, "Pause an ad set or campaign"},
	{CampaignPause, "Pause a whole campaign"},
	{CampaignResume, "Resume a paused campaign"},
	{BudgetRaise, "Raise a budget within the cap"},
	{BidsChange, "Change bids or target CPA or ROAS"},
	{Launch, "Launch anything"},
	{TargetingChange, "Change targeting, geography or GCC-only rules"},
	{OrganicPost, "Organic posting"},
}

type WriteActionInfo struct {
	Action      WriteAction `json:"action"`
	Description string      `json:"description"`
}

var known = func() map[WriteAction]bool {
	m := map[WriteAction]bool{}
	for _, a := range Catalog {
		m[a.Action] = true
	}
	return m
}()

// PolicyMode is what the checklist says for one action and one role.
type PolicyMode string

const (
	// ModeUnconfigured is the zero value: nobody decided, so the action is denied.
	ModeUnconfigured  PolicyMode = ""
	ModeAllowed       PolicyMode = "allowed"
	ModeNeedsApproval PolicyMode = "needs_approval"
	ModeOff           PolicyMode = "off"
)

func (m PolicyMode) valid() bool {
	switch m {
	case ModeUnconfigured, ModeAllowed, ModeNeedsApproval, ModeOff:
		return true
	}
	return false
}

// PolicyRule is the checklist row for one action. Viewers can never write, so the
// row has no viewer column. Admins approve, so an admin's own action is
// allowed where a marketer's needs approval (self-approved, reference.md).
type PolicyRule struct {
	Marketer PolicyMode `json:"marketer"`
	Admin    PolicyMode `json:"admin"`
}

// PolicyCaps are D-13's hard limits. Nil means unconfigured, and an unconfigured
// cap denies the actions that depend on it (budget raises, changes per day).
type PolicyCaps struct {
	DailySpendIncreasePct *int `json:"dailySpendIncreasePct"`
	MaxChangesPerDay      *int `json:"maxChangesPerDay"`
}

// PolicyDocument is one version of the workspace's policy.
type PolicyDocument struct {
	// Actions maps each catalog action to its rule. A missing action is
	// unconfigured.
	Actions map[WriteAction]PolicyRule `json:"actions"`
	Caps    PolicyCaps                 `json:"caps"`
	// WriteAccounts lists the accounts an admin has enabled writes for.
	// An account not listed gets no external write, whatever the checklist.
	WriteAccounts []int64 `json:"writeAccounts"`
	// Note is the admin's own description of this version.
	Note string `json:"note"`
}

// Policy is a stored version.
type Policy struct {
	Version   int64          `json:"version"`
	Document  PolicyDocument `json:"document"`
	CreatedBy *int64         `json:"createdBy"`
	CreatedAt time.Time      `json:"createdAt"`
}

// PolicyState is what the policy screen shows: the current version or an
// unconfigured marker, the catalog, what's still unconfigured, and switches.
type PolicyState struct {
	Configured   bool              `json:"configured"`
	Current      *Policy           `json:"current"`
	Catalog      []WriteActionInfo `json:"catalog"`
	Unconfigured []string          `json:"unconfigured"`
	KillSwitches []KillSwitch      `json:"killSwitches"`
	Suggested    PolicyDocument    `json:"suggested"`
}

type KillSwitch struct {
	// AccountID 0 is the whole workspace.
	AccountID int64     `json:"accountId"`
	Engaged   bool      `json:"engaged"`
	Reason    string    `json:"reason"`
	SetBy     *int64    `json:"setBy"`
	SetAt     time.Time `json:"setAt"`
}

// PolicyRequest is one proposed external change to evaluate.
type PolicyRequest struct {
	AccountID int64
	Action    WriteAction
	// WriteCapability is the account capability the action needs, such as
	// "write.budget". It must be verified for the account.
	WriteCapability string
	// Unplanned says the target is a campaign with no accepted plan behind
	// it (BD-68 rule 3). Such a campaign allows only the safety actions in
	// AllowedUnplanned, whatever the checklist says, until a plan adopts it.
	Unplanned bool
}

// AllowedUnplanned lists what may change on a campaign that is not under a
// plan: a marketer may pause it or lower its budget (BD-68 rule 4). Nothing
// else, not even a resume, until a plan adopts it.
func AllowedUnplanned(a WriteAction) bool {
	return a == CampaignPause || a == BudgetLower
}

type PolicyOutcome string

const (
	Allowed       PolicyOutcome = "allowed"
	NeedsApproval PolicyOutcome = "needs_approval"
	Denied        PolicyOutcome = "denied"
)

// PolicyDecision is the evaluator's answer. Reason is a stable code the UI can
// explain; PolicyVersion is what the decision was made under, for the log.
type PolicyDecision struct {
	Outcome       PolicyOutcome `json:"outcome"`
	Reason        string        `json:"reason"`
	PolicyVersion int64         `json:"policyVersion"`
}

const (
	ReasonOK                  = "ok"
	ReasonForbiddenRole       = "role_cannot_write"
	ReasonUnknownAction       = "unknown_action"
	ReasonPolicyUnconfigured  = "policy_unconfigured"
	ReasonActionUnconfigured  = "action_unconfigured"
	ReasonActionOff           = "action_off"
	ReasonKillSwitch          = "kill_switch"
	ReasonAccountNotEnabled   = "account_write_not_enabled"
	ReasonCapabilityUnproven  = "write_capability_unverified"
	ReasonCapUnconfigured     = "cap_unconfigured"
	ReasonWorkspaceMismatch   = "workspace_mismatch"
	ReasonAccountNotConnected = "account_not_connected"
	ReasonUnplannedCampaign   = "campaign_not_under_a_plan"
)

type Policies struct {
	db  *store.DB
	now func() time.Time
}

func New(db *store.DB) *Policies {
	return &Policies{db: db, now: time.Now}
}

// Get returns the policy screen's state. Viewers may read it: permissions
// are visible to everyone and enforced in core.
func (p *Policies) Get(ctx context.Context, actor core.Actor) (PolicyState, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return PolicyState{}, core.ErrForbidden
	}
	state := PolicyState{Catalog: Catalog, Suggested: SuggestedDefaults(), Unconfigured: []string{}, KillSwitches: []KillSwitch{}}
	current, err := p.current(ctx, actor.WorkspaceID)
	if err != nil {
		return PolicyState{}, err
	}
	if current != nil {
		state.Configured = true
		state.Current = current
	}
	state.Unconfigured = unconfigured(current)
	rows, err := p.db.Reader().ListKillSwitches(ctx, actor.WorkspaceID)
	if err != nil {
		return PolicyState{}, err
	}
	for _, r := range rows {
		state.KillSwitches = append(state.KillSwitches, killSwitchFromRow(r))
	}
	return state, nil
}

// Put stores a new version. Only admins. The document is validated: every
// action must be in the catalog and every mode valid.
func (p *Policies) Put(ctx context.Context, actor core.Actor, doc PolicyDocument) (Policy, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return Policy{}, core.ErrForbidden
	}
	if err := validate(doc); err != nil {
		return Policy{}, err
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return Policy{}, err
	}
	var row store.Policy
	err = p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		version := int64(1)
		if cur, err := q.GetCurrentPolicy(ctx, actor.WorkspaceID); err == nil {
			version = cur.Version + 1
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		row, err = q.CreatePolicy(ctx, store.CreatePolicyParams{
			WorkspaceID: actor.WorkspaceID, Version: version, Document: string(raw),
			CreatedBy: optionalID(actor.UserID), CreatedAt: p.now().UTC(),
		})
		return err
	})
	if store.IsUniqueViolation(err) {
		return Policy{}, core.Conflict("someone saved a policy at the same time; reload and try again")
	}
	if err != nil {
		return Policy{}, err
	}
	return fromRow(row)
}

// SetKillSwitch engages or clears the switch for an account, or for the whole
// workspace with accountID 0. Admins only. It blocks what hasn't been sent;
// it undoes nothing already done.
func (p *Policies) SetKillSwitch(ctx context.Context, actor core.Actor, accountID int64, engaged bool, reason string) (KillSwitch, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return KillSwitch{}, core.ErrForbidden
	}
	if len(reason) > 500 {
		return KillSwitch{}, core.Invalid("reason must be 500 characters or fewer")
	}
	if accountID != 0 {
		if acct, err := p.db.Reader().GetAccountByID(ctx, accountID); errors.Is(err, sql.ErrNoRows) || (err == nil && acct.Account.WorkspaceID != actor.WorkspaceID) {
			return KillSwitch{}, core.NotFound("account %d is not connected", accountID)
		} else if err != nil {
			return KillSwitch{}, err
		}
	}
	now := p.now().UTC()
	err := p.db.Writer().UpsertKillSwitch(ctx, store.UpsertKillSwitchParams{
		WorkspaceID: actor.WorkspaceID, AccountID: accountID, Engaged: boolInt(engaged), Reason: reason,
		SetBy: optionalID(actor.UserID), SetAt: now,
	})
	if err != nil {
		return KillSwitch{}, err
	}
	var by *int64
	if actor.UserID != 0 {
		id := actor.UserID
		by = &id
	}
	return KillSwitch{AccountID: accountID, Engaged: engaged, Reason: reason, SetBy: by, SetAt: now}, nil
}

// KillSwitchEngaged says whether the workspace switch, or the account's,
// is on. Every entry point that could reach an outside system asks this
// (directly or through Evaluate) before doing anything.
func (p *Policies) KillSwitchEngaged(ctx context.Context, workspaceID, accountID int64) (bool, string, error) {
	switches, err := p.db.Reader().ListKillSwitches(ctx, workspaceID)
	if err != nil {
		return false, "", err
	}
	for _, s := range switches {
		if s.Engaged == 1 && (s.AccountID == 0 || s.AccountID == accountID) {
			return true, s.Reason, nil
		}
	}
	return false, "", nil
}

// Evaluate answers whether actor may make req now. It never returns an error
// for a policy reason: those are decisions. Errors are database failures.
func (p *Policies) Evaluate(ctx context.Context, actor core.Actor, req PolicyRequest) (PolicyDecision, error) {
	if actor.WorkspaceID == 0 {
		return PolicyDecision{Outcome: Denied, Reason: ReasonWorkspaceMismatch}, nil
	}
	if !actor.AtLeast(core.RoleMarketer) {
		return PolicyDecision{Outcome: Denied, Reason: ReasonForbiddenRole}, nil
	}
	if !known[req.Action] {
		return PolicyDecision{Outcome: Denied, Reason: ReasonUnknownAction}, nil
	}
	if req.Unplanned && !AllowedUnplanned(req.Action) {
		return PolicyDecision{Outcome: Denied, Reason: ReasonUnplannedCampaign}, nil
	}
	current, err := p.current(ctx, actor.WorkspaceID)
	if err != nil {
		return PolicyDecision{}, err
	}
	if current == nil {
		return PolicyDecision{Outcome: Denied, Reason: ReasonPolicyUnconfigured}, nil
	}
	d := PolicyDecision{Outcome: Denied, PolicyVersion: current.Version}
	switches, err := p.db.Reader().ListKillSwitches(ctx, actor.WorkspaceID)
	if err != nil {
		return PolicyDecision{}, err
	}
	for _, s := range switches {
		if s.Engaged == 1 && (s.AccountID == 0 || s.AccountID == req.AccountID) {
			d.Reason = ReasonKillSwitch
			return d, nil
		}
	}
	// An account connected by another client is "not connected" here: the
	// decision never admits that it exists.
	if acct, err := p.db.Reader().GetAccountByID(ctx, req.AccountID); errors.Is(err, sql.ErrNoRows) || (err == nil && acct.Account.WorkspaceID != actor.WorkspaceID) {
		d.Reason = ReasonAccountNotConnected
		return d, nil
	} else if err != nil {
		return PolicyDecision{}, err
	}
	enabled := false
	for _, id := range current.Document.WriteAccounts {
		if id == req.AccountID {
			enabled = true
		}
	}
	if !enabled {
		d.Reason = ReasonAccountNotEnabled
		return d, nil
	}
	if req.WriteCapability != "" {
		cap, err := p.db.Reader().GetAccountCapability(ctx, store.GetAccountCapabilityParams{AccountID: req.AccountID, Capability: req.WriteCapability})
		if errors.Is(err, sql.ErrNoRows) || (err == nil && cap.State != "verified") {
			d.Reason = ReasonCapabilityUnproven
			return d, nil
		} else if err != nil {
			return PolicyDecision{}, err
		}
	}
	rule, ok := current.Document.Actions[req.Action]
	if !ok {
		d.Reason = ReasonActionUnconfigured
		return d, nil
	}
	mode := rule.Marketer
	if actor.AtLeast(core.RoleAdmin) {
		mode = rule.Admin
	}
	switch mode {
	case ModeUnconfigured:
		d.Reason = ReasonActionUnconfigured
	case ModeOff:
		d.Reason = ReasonActionOff
	case ModeAllowed, ModeNeedsApproval:
		if (req.Action == BudgetRaise && current.Document.Caps.DailySpendIncreasePct == nil) || current.Document.Caps.MaxChangesPerDay == nil {
			d.Reason = ReasonCapUnconfigured
			return d, nil
		}
		d.Reason = ReasonOK
		if mode == ModeAllowed {
			d.Outcome = Allowed
		} else {
			d.Outcome = NeedsApproval
		}
	}
	return d, nil
}

// SuggestedDefaults is D-13's table. It is a suggestion the admin applies
// through Put; it never takes effect by itself. PolicyCaps and write accounts stay
// nil/empty: those are the owner's numbers.
func SuggestedDefaults() PolicyDocument {
	allowed := PolicyRule{Marketer: ModeAllowed, Admin: ModeAllowed}
	approval := PolicyRule{Marketer: ModeNeedsApproval, Admin: ModeAllowed}
	return PolicyDocument{
		Actions: map[WriteAction]PolicyRule{
			AudiencesUpsert: allowed, ConversionsUpload: allowed, ExclusionsUpdate: allowed, LeadFieldsSet: allowed,
			AdPause: allowed, BudgetLower: allowed, CampaignCreatePaus: allowed,
			// Resuming a paused campaign is a proposal a marketer applies
			// alone, logged and reversible, without the CMO (BD-68 rule 5).
			CampaignResume: allowed,
			AdSetPause:     approval, CampaignPause: approval, BudgetRaise: approval, BidsChange: approval,
			Launch: approval, TargetingChange: approval,
			OrganicPost: {Marketer: ModeOff, Admin: ModeOff},
		},
		WriteAccounts: []int64{},
		Note:          "D-13 defaults; caps and write accounts still to be set",
	}
}

func validate(doc PolicyDocument) error {
	for action, rule := range doc.Actions {
		if !known[action] {
			return core.Invalid("unknown action %q", action)
		}
		if !rule.Marketer.valid() || !rule.Admin.valid() {
			return core.Invalid("action %q has an invalid mode", action)
		}
	}
	if doc.Caps.DailySpendIncreasePct != nil && (*doc.Caps.DailySpendIncreasePct < 0 || *doc.Caps.DailySpendIncreasePct > 1000) {
		return core.Invalid("daily spend increase cap must be between 0 and 1000 percent")
	}
	if doc.Caps.MaxChangesPerDay != nil && (*doc.Caps.MaxChangesPerDay < 0 || *doc.Caps.MaxChangesPerDay > 10000) {
		return core.Invalid("changes per day cap must be between 0 and 10000")
	}
	if len(doc.Note) > 1000 {
		return core.Invalid("note must be 1000 characters or fewer")
	}
	return nil
}

// unconfigured lists what still blocks writes, for the policy screen.
func unconfigured(current *Policy) []string {
	if current == nil {
		return []string{"policy document"}
	}
	var out []string
	for _, a := range Catalog {
		rule, ok := current.Document.Actions[a.Action]
		if !ok || rule.Marketer == ModeUnconfigured || rule.Admin == ModeUnconfigured {
			out = append(out, "action: "+string(a.Action))
		}
	}
	if current.Document.Caps.DailySpendIncreasePct == nil {
		out = append(out, "cap: daily spend increase")
	}
	if current.Document.Caps.MaxChangesPerDay == nil {
		out = append(out, "cap: changes per day")
	}
	if len(current.Document.WriteAccounts) == 0 {
		out = append(out, "write accounts: none enabled")
	}
	sort.Strings(out)
	if out == nil {
		out = []string{}
	}
	return out
}

func (p *Policies) current(ctx context.Context, workspaceID int64) (*Policy, error) {
	row, err := p.db.Reader().GetCurrentPolicy(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	policy, err := fromRow(row)
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

func fromRow(r store.Policy) (Policy, error) {
	var doc PolicyDocument
	if err := json.Unmarshal([]byte(r.Document), &doc); err != nil {
		return Policy{}, fmt.Errorf("policy %d is not valid JSON: %w", r.ID, err)
	}
	if doc.Actions == nil {
		doc.Actions = map[WriteAction]PolicyRule{}
	}
	if doc.WriteAccounts == nil {
		doc.WriteAccounts = []int64{}
	}
	out := Policy{Version: r.Version, Document: doc, CreatedAt: r.CreatedAt.UTC()}
	if r.CreatedBy.Valid {
		out.CreatedBy = &r.CreatedBy.Int64
	}
	return out, nil
}

func killSwitchFromRow(r store.KillSwitch) KillSwitch {
	out := KillSwitch{AccountID: r.AccountID, Engaged: r.Engaged == 1, Reason: r.Reason, SetAt: r.SetAt.UTC()}
	if r.SetBy.Valid {
		out.SetBy = &r.SetBy.Int64
	}
	return out
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
