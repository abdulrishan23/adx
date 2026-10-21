// Package launch is where an accepted plan version's money is pointed at
// campaigns before anything is submitted (BD-68, decided 2026-10-18 and
// 2026-10-19). Create owns the first launch: each plan line is resolved
// against the project's campaigns by project, platform and campaign type.
// One running match is updated; more than one and a person picks, never
// both unasked; none running but a paused one and the launch resumes it
// rather than making a second campaign of the type; none at all and a
// campaign is created. A campaign is the target of one line only: when two
// lines would take the same campaign a person decides which line it is for
// and the other creates, so no budget is counted twice. The plan is the
// period's ceiling: a matched campaign's spend inside the plan's period so
// far nets off its line (nothing before the period starts), and the daily
// budget is what remains spread over the days left in the period, by the
// Dubai day. A pick is recorded here, as the person's choice, before any
// intent is written. Nothing in this package talks to a platform or writes
// a proposal; it answers what would happen.
package launch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
)

const (
	KindCreate = "create"
	KindUpdate = "update"
	// KindResume is an update of a paused campaign of the line's type when no
	// running one exists: accepting the plan authorised its spend, so it is
	// resumed rather than duplicated.
	KindResume      = "resume"
	KindPick        = "pick"
	KindUnsupported = "unsupported"
)

// LaunchLine is one allocation of the plan version as the resolution reads
// it: which platform and campaign type it stands for, and whose project.
type LaunchLine struct {
	Index        int    `json:"index"`
	Channel      string `json:"channel"`
	Platform     string `json:"platform"`
	CampaignType string `json:"campaignType"`
	ProjectID    int64  `json:"projectId"`
	BudgetMinor  int64  `json:"budgetMinor"`
	Currency     string `json:"currency"`
	Objective    string `json:"objective"`
	Geography    string `json:"geography"`
}

// MonthBudget is the line's money for the rest of the plan's period.
type MonthBudget struct {
	// SpentMinor is what the matched campaign spent inside the period so
	// far. Zero for a campaign to be created, and zero before the period
	// starts.
	SpentMinor int64 `json:"spentMinor"`
	// SpendKnown is false when the period has started and the matched
	// campaign has no figures read inside it: nothing nets off, and the
	// screen says the spend is not available rather than zero.
	SpendKnown     bool  `json:"spendKnown"`
	RemainingMinor int64 `json:"remainingMinor"`
	DailyMinor     int64 `json:"dailyMinor"`
	DaysLeft       int   `json:"daysLeft"`
}

// LaunchPick is a person's recorded choice for a line.
type LaunchPick struct {
	CampaignID int64     `json:"campaignId"`
	PickedBy   *int64    `json:"pickedBy"`
	PickedAt   time.Time `json:"pickedAt"`
}

// LineResolution is what the launch would do for one line.
type LineResolution struct {
	Kind string     `json:"kind"`
	Line LaunchLine `json:"line"`
	// Campaign is the campaign an update or a resume changes.
	Campaign *reporting.ProjectCampaign `json:"campaign"`
	// Candidates are the campaigns a person chooses between: more than one
	// match, or one match another line also claims. They stay listed after a
	// pick so the choice can be changed.
	Candidates []reporting.ProjectCampaign `json:"candidates"`
	Budget     *MonthBudget                `json:"budget"`
	Pick       *LaunchPick                 `json:"pick"`
	// Paused lists the project's paused campaigns of the line's type that a
	// running match left aside. They are not touched by this launch.
	Paused []reporting.ProjectCampaign `json:"paused"`
	Note   string                      `json:"note"`
}

// LaunchResolution is the whole plan version resolved on a given day.
type LaunchResolution struct {
	PlanID      int64         `json:"planId"`
	PlanVersion int64         `json:"planVersion"`
	ProjectID   *int64        `json:"projectId"`
	Today       reporting.Day `json:"today"`
	PeriodStart reporting.Day `json:"periodStart"`
	PeriodEnd   reporting.Day `json:"periodEnd"`
	// DaysLeft counts the days of the period still to come, today included
	// once the period has started; before it starts, the whole period.
	DaysLeft int              `json:"daysLeft"`
	TimeZone string           `json:"timeZone"`
	Lines    []LineResolution `json:"lines"`
	// Unmatched are the project's running campaigns no line of this version
	// updates. They stay "Not under a plan" until a line matches them.
	Unmatched []reporting.ProjectCampaign `json:"unmatched"`
	Note      string                      `json:"note"`
}

// PlanCoverage says whether a campaign has an accepted plan behind it.
type PlanCoverage struct {
	UnderPlan   bool   `json:"underPlan"`
	PlanID      *int64 `json:"planId"`
	PlanVersion *int64 `json:"planVersion"`
	Reason      string `json:"reason"`
}

type Config struct{ Clock core.Clock }

type Service struct {
	db        *store.DB
	clock     core.Clock
	plans     *plans.Plans
	reporting *reporting.Reporting
	proposals *proposals.Proposals
}

func New(db *store.DB, pl *plans.Plans, rep *reporting.Reporting, cfg Config) *Service {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Service{db: db, clock: cfg.Clock, plans: pl, reporting: rep}
}

// Resolve answers create, update or pick for every line of an accepted plan
// version, as of today in Dubai. Viewers may read it; it changes nothing.
func (s *Service) Resolve(ctx context.Context, actor core.Actor, planID, version int64) (LaunchResolution, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return LaunchResolution{}, core.ErrForbidden
	}
	return s.resolve(ctx, actor, planID, version, s.clock.Now())
}

// Pick records the person's choice for a line that matched more than one
// running campaign and returns the resolution with it applied. Marketers
// and admins may; the assistant may not. The campaign must be one the line
// matched today.
func (s *Service) Pick(ctx context.Context, actor core.Actor, planID, version int64, lineIndex int, campaignID int64) (LaunchResolution, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 || actor.Via == core.ViaAssistant {
		return LaunchResolution{}, core.ErrForbidden
	}
	now := s.clock.Now()
	res, err := s.resolve(ctx, actor, planID, version, now)
	if err != nil {
		return LaunchResolution{}, err
	}
	if lineIndex < 0 || lineIndex >= len(res.Lines) {
		return LaunchResolution{}, core.Invalid("plan %d version %d has no line %d", planID, version, lineIndex)
	}
	line := res.Lines[lineIndex]
	if len(line.Candidates) == 0 {
		return LaunchResolution{}, core.Conflict("line %d (%s) matched %s; there is nothing to pick", lineIndex, line.Line.Channel, describe(line))
	}
	found := false
	for _, c := range line.Candidates {
		if c.ID == campaignID {
			found = true
		}
	}
	if !found {
		return LaunchResolution{}, core.Invalid("campaign %d is not one of the %d campaigns line %d matched", campaignID, len(line.Candidates), lineIndex)
	}
	_, err = s.db.Writer().UpsertLaunchPick(ctx, store.UpsertLaunchPickParams{WorkspaceID: actor.WorkspaceID, PlanID: planID, PlanVersion: version, LineIndex: int64(lineIndex), Channel: line.Line.Channel,
		CampaignID: campaignID, PickedBy: optionalID(actor.UserID), PickedAt: now.UTC()})
	if err != nil {
		return LaunchResolution{}, err
	}
	return s.resolve(ctx, actor, planID, version, now)
}

// Coverage says whether a campaign of an account, by the platform's id, has
// an accepted plan behind it: a plan of its project whose period has not
// ended and whose lines resolve to updating or resuming it, by a sole match
// or a recorded pick (BD-68 rules 3 and 11). Coverage runs from acceptance,
// not from the period's start, so a plan accepted ahead of its month covers
// the campaigns it will spend on. A campaign ADX has not read, or that has
// no project, is not under a plan.
func (s *Service) Coverage(ctx context.Context, workspaceID, accountID int64, externalID string) (PlanCoverage, error) {
	row, err := s.db.Reader().GetAdCampaignByExternalID(ctx, store.GetAdCampaignByExternalIDParams{AccountID: accountID, ExternalID: externalID})
	if errors.Is(err, sql.ErrNoRows) || (err == nil && row.WorkspaceID != workspaceID) {
		return PlanCoverage{Reason: fmt.Sprintf("ADX has not read campaign %s from account %d", externalID, accountID)}, nil
	}
	if err != nil {
		return PlanCoverage{}, err
	}
	if !row.ProjectID.Valid {
		return PlanCoverage{Reason: "the campaign is assigned to no project, so no plan can match it"}, nil
	}
	versions, err := s.db.Reader().ListAcceptedPlanVersionsForProject(ctx, store.ListAcceptedPlanVersionsForProjectParams{WorkspaceID: workspaceID, ProjectID: row.ProjectID})
	if err != nil {
		return PlanCoverage{}, err
	}
	today := string(reporting.DayOf(s.clock.Now().In(reporting.ReportingZone)))
	actor := core.SystemIn(core.ViaJob, workspaceID)
	seen := map[int64]bool{}
	for _, v := range versions {
		if seen[v.Plan.ID] {
			continue // the latest accepted version of each plan speaks for it
		}
		seen[v.Plan.ID] = true
		pv, err := plans.VersionFromRow(v.PlanVersion)
		if err != nil {
			return PlanCoverage{}, err
		}
		if pv.Document.PeriodFrom == "" || pv.Document.PeriodTo == "" || today > pv.Document.PeriodTo {
			continue
		}
		res, err := s.resolve(ctx, actor, v.Plan.ID, pv.Version, s.clock.Now())
		if err != nil {
			return PlanCoverage{}, err
		}
		for _, line := range res.Lines {
			if (line.Kind == KindUpdate || line.Kind == KindResume) && line.Campaign != nil && line.Campaign.ID == row.ID {
				pid, pver := v.Plan.ID, pv.Version
				return PlanCoverage{UnderPlan: true, PlanID: &pid, PlanVersion: &pver, Reason: fmt.Sprintf("plan %d version %d (%s to %s) line %d %ss it", pid, pver, pv.Document.PeriodFrom, pv.Document.PeriodTo, line.Line.Index, line.Kind)}, nil
			}
		}
	}
	return PlanCoverage{Reason: "no accepted plan whose period has not ended resolves to this campaign"}, nil
}

// UnderPlan is Coverage as proposals ask it.
func (s *Service) UnderPlan(ctx context.Context, workspaceID, accountID int64, externalID string) (bool, string, error) {
	c, err := s.Coverage(ctx, workspaceID, accountID, externalID)
	return c.UnderPlan, c.Reason, err
}

func (s *Service) resolve(ctx context.Context, actor core.Actor, planID, version int64, now time.Time) (LaunchResolution, error) {
	plan, err := s.db.Reader().GetPlan(ctx, store.GetPlanParams{ID: planID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return LaunchResolution{}, core.NotFound("plan %d not found", planID)
	}
	if err != nil {
		return LaunchResolution{}, err
	}
	pv, err := s.plans.Version(ctx, actor, planID, version)
	if err != nil {
		return LaunchResolution{}, err
	}
	if pv.State != plans.StateAccepted {
		return LaunchResolution{}, core.Conflict("plan %d version %d is %s; only an accepted version launches", planID, version, pv.State)
	}
	today := reporting.DayOf(now.In(reporting.ReportingZone))
	from, to := reporting.Day(pv.Document.PeriodFrom), reporting.Day(pv.Document.PeriodTo)
	_, err1 := from.Time()
	toT, err2 := to.Time()
	if err1 != nil || err2 != nil || to < from {
		return LaunchResolution{}, core.Conflict("plan %d version %d has no valid period; nothing launches into it", planID, version)
	}
	if today > to {
		return LaunchResolution{}, core.Conflict("plan %d version %d's period ended on %s; nothing launches into it", planID, version, to)
	}
	// Days left in the period, today included once it has started.
	start := today
	if today < from {
		start = from
	}
	startT, _ := start.Time()
	daysLeft := int(toT.Sub(startT).Hours()/24) + 1
	res := LaunchResolution{PlanID: planID, PlanVersion: pv.Version, Today: today, PeriodStart: from, PeriodEnd: to, DaysLeft: daysLeft, TimeZone: reporting.ReportingZone.String(),
		Lines: []LineResolution{}, Unmatched: []reporting.ProjectCampaign{},
		Note: "What the launch would do today. One matching running campaign is updated; more than one needs a person's pick; a paused one is resumed; none and a campaign is created. Spend inside the plan's period nets off each line and the rest is spread over the days left in it."}
	beforeStart := today < from
	if plan.ProjectID.Valid {
		id := plan.ProjectID.Int64
		res.ProjectID = &id
	}
	picks, err := s.db.Reader().ListLaunchPicks(ctx, store.ListLaunchPicksParams{WorkspaceID: actor.WorkspaceID, PlanID: planID, PlanVersion: pv.Version})
	if err != nil {
		return LaunchResolution{}, err
	}
	pickByLine := map[int64]store.LaunchPick{}
	for _, p := range picks {
		pickByLine[p.LineIndex] = p
	}
	campaignsByProject := map[int64][]reporting.ProjectCampaign{}
	campaigns := func(project int64) ([]reporting.ProjectCampaign, error) {
		if cs, ok := campaignsByProject[project]; ok {
			return cs, nil
		}
		cs, err := s.reporting.ProjectCampaigns(ctx, actor, project, from, to)
		if err != nil {
			return nil, err
		}
		campaignsByProject[project] = cs
		return cs, nil
	}
	// First pass: what each line matches. Running campaigns of the line's
	// type are its candidates; with none running, its paused ones are, and
	// the line resumes rather than creates.
	type matched struct {
		lr    LineResolution
		kind  string // update or resume, once a campaign is settled
		cands []reporting.ProjectCampaign
		ready bool // settled in the first pass (unsupported lines)
	}
	lines := make([]*matched, 0, len(pv.Document.Allocations))
	for i, a := range pv.Document.Allocations {
		line := LaunchLine{Index: i, Channel: a.Channel, BudgetMinor: a.BudgetMinor, Currency: pv.Document.Currency, Objective: a.Objective, Geography: a.Geography}
		switch {
		case a.ProjectID != nil && *a.ProjectID != 0:
			line.ProjectID = *a.ProjectID
		case plan.ProjectID.Valid:
			line.ProjectID = plan.ProjectID.Int64
		}
		m := &matched{lr: LineResolution{Line: line, Candidates: []reporting.ProjectCampaign{}, Paused: []reporting.ProjectCampaign{}}, kind: KindUpdate}
		lines = append(lines, m)
		ch, ok := plans.Channel(a.Channel)
		switch {
		case !ok || !ch.Supported || ch.CampaignType == "":
			m.lr.Kind, m.lr.Note, m.ready = KindUnsupported, fmt.Sprintf("ADX cannot deploy %s; this line is not launched", a.Channel), true
			continue
		case line.ProjectID == 0:
			m.lr.Kind, m.lr.Note, m.ready = KindUnsupported, "the line names no project and the plan has none; lines match campaigns by project", true
			continue
		}
		m.lr.Line.Platform, m.lr.Line.CampaignType = ch.Platform, ch.CampaignType
		cs, err := campaigns(line.ProjectID)
		if err != nil {
			return LaunchResolution{}, err
		}
		var running, paused []reporting.ProjectCampaign
		for _, c := range cs {
			if c.Platform != ch.Platform || c.Type != ch.CampaignType {
				continue
			}
			if c.Running {
				running = append(running, c)
			} else {
				paused = append(paused, c)
			}
		}
		switch {
		case len(running) > 0:
			m.cands = running
			m.lr.Paused = append(m.lr.Paused, paused...)
		case len(paused) > 0:
			m.cands, m.kind = paused, KindResume
		}
	}

	// Second pass: a campaign is the target of one line only. Recorded picks
	// claim first, lowest line first; then a sole match claims its campaign
	// unless another open line could take it too, in which case a person
	// decides which line the campaign is for.
	claimed := map[int64]int{} // campaign id -> line index
	settle := func(m *matched, c reporting.ProjectCampaign, pick *store.LaunchPick) {
		cc := c
		m.lr.Kind, m.lr.Campaign = m.kind, &cc
		b := spendBudget(m.lr.Line.BudgetMinor, cc, daysLeft, beforeStart)
		m.lr.Budget = &b
		if pick != nil {
			m.lr.Pick = &LaunchPick{CampaignID: pick.CampaignID, PickedBy: idPtr(pick.PickedBy), PickedAt: pick.PickedAt.UTC()}
			m.lr.Candidates = append([]reporting.ProjectCampaign{}, m.cands...)
		}
		if m.kind == KindResume {
			m.lr.Note = "no running campaign of this type; the paused one is resumed rather than a second one created"
		}
		claimed[cc.ID] = m.lr.Line.Index
		m.ready = true
	}
	for _, m := range lines {
		if m.ready {
			continue
		}
		p, ok := pickByLine[int64(m.lr.Line.Index)]
		if !ok {
			continue
		}
		var chosen *reporting.ProjectCampaign
		for _, c := range m.cands {
			if c.ID == p.CampaignID {
				cc := c
				chosen = &cc
			}
		}
		switch {
		case chosen == nil:
			m.lr.Note = fmt.Sprintf("the earlier choice, campaign %d, no longer matches this line; pick again", p.CampaignID)
		case hasKey(claimed, chosen.ID):
			m.lr.Note = fmt.Sprintf("the earlier choice, campaign %d, is line %d's; this line no longer takes it", p.CampaignID, claimed[chosen.ID])
		default:
			settle(m, *chosen, &p)
		}
	}
	for _, m := range lines {
		if m.ready {
			continue
		}
		var available []reporting.ProjectCampaign
		for _, c := range m.cands {
			if !hasKey(claimed, c.ID) {
				available = append(available, c)
			}
		}
		contested := false
		for _, c := range available {
			for _, other := range lines {
				if other == m || other.ready {
					continue
				}
				for _, oc := range other.cands {
					if oc.ID == c.ID && !hasKey(claimed, oc.ID) {
						contested = true
					}
				}
			}
		}
		switch {
		case len(available) == 0:
			m.lr.Kind = KindCreate
			b := budget(m.lr.Line.BudgetMinor, 0, true, daysLeft)
			m.lr.Budget = &b
			if len(m.cands) > 0 && m.lr.Note == "" {
				m.lr.Note = "every campaign this line matched is another line's; a new one is created"
			}
			m.ready = true
		case len(available) == 1 && !contested:
			settle(m, available[0], nil)
		default:
			m.lr.Kind = KindPick
			m.lr.Candidates = append([]reporting.ProjectCampaign{}, available...)
			what := "running"
			if m.kind == KindResume {
				what = "paused"
			}
			if contested && len(available) == 1 {
				m.lr.Note = fmt.Sprintf("another line of this plan also matches this %s %s campaign; a person decides which line it is for, and the other creates", what, m.lr.Line.CampaignType)
			} else if m.lr.Note == "" {
				m.lr.Note = fmt.Sprintf("%d %s %s campaigns match; a person picks which one this line %ss", len(available), what, m.lr.Line.CampaignType, m.kind)
			}
		}
	}
	for _, m := range lines {
		res.Lines = append(res.Lines, m.lr)
	}
	for _, cs := range campaignsByProject {
		for _, c := range cs {
			if c.Running && !hasKey(claimed, c.ID) {
				res.Unmatched = append(res.Unmatched, c)
			}
		}
	}
	return res, nil
}

// spendBudget nets the campaign's spend inside the period off the line.
// Before the period starts nothing has been spent in it, so the spend is a
// known zero rather than not available.
func spendBudget(lineMinor int64, c reporting.ProjectCampaign, daysLeft int, beforeStart bool) MonthBudget {
	if c.SpentMinor == nil {
		return budget(lineMinor, 0, beforeStart, daysLeft)
	}
	return budget(lineMinor, *c.SpentMinor, true, daysLeft)
}

// budget nets spend off the line, never below zero, and spreads the rest
// over the days left, today included, rounding half up.
func budget(lineMinor, spentMinor int64, known bool, daysLeft int) MonthBudget {
	remaining := max(lineMinor-spentMinor, 0)
	if daysLeft < 1 {
		daysLeft = 1
	}
	return MonthBudget{SpentMinor: spentMinor, SpendKnown: known, RemainingMinor: remaining, DailyMinor: (remaining + int64(daysLeft)/2) / int64(daysLeft), DaysLeft: daysLeft}
}

func describe(l LineResolution) string {
	switch l.Kind {
	case KindCreate:
		return "no campaign"
	case KindUpdate:
		return "one running campaign"
	case KindResume:
		return "one paused campaign"
	case KindUnsupported:
		return "nothing ADX can deploy"
	}
	return l.Kind
}

func hasKey(m map[int64]int, id int64) bool {
	_, ok := m[id]
	return ok
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
