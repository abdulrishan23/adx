// Package launch is where an accepted plan version's money is pointed at
// campaigns before anything is submitted (BD-58, decided 2026-10-18). Create
// owns the first launch: each plan line is resolved against the project's
// running campaigns by project, platform and campaign type. One match is
// updated; more than one and a person picks, never both unasked; none and a
// campaign is created. The plan is the month's ceiling: a matched campaign's
// spend this month nets off its line, and the daily budget is what remains
// spread over the days left in the Dubai month. A pick is recorded here, as
// the person's choice, before any intent is written. Nothing in this package
// talks to a platform or writes a proposal; it answers what would happen.
package launch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
)

const (
	KindCreate      = "create"
	KindUpdate      = "update"
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

// MonthBudget is the line's money for the rest of this month.
type MonthBudget struct {
	// SpentMinor is what the matched campaign spent this month. Zero for a
	// campaign to be created.
	SpentMinor int64 `json:"spentMinor"`
	// SpendKnown is false when the matched campaign has no figures read
	// this month: nothing nets off, and the screen says the spend is not
	// available rather than zero.
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
	// Campaign is the campaign an update changes.
	Campaign *reporting.ProjectCampaign `json:"campaign"`
	// Candidates are the running campaigns the line matched, when more than
	// one did. They stay listed after a pick so the choice can be changed.
	Candidates []reporting.ProjectCampaign `json:"candidates"`
	Budget     *MonthBudget                `json:"budget"`
	Pick       *LaunchPick                 `json:"pick"`
	// Paused lists the project's paused campaigns of the line's type. They
	// are not matched: resuming one is a Perform proposal (BD-58 rule 5).
	Paused []reporting.ProjectCampaign `json:"paused"`
	Note   string                      `json:"note"`
}

// LaunchResolution is the whole plan version resolved on a given day.
type LaunchResolution struct {
	PlanID      int64            `json:"planId"`
	PlanVersion int64            `json:"planVersion"`
	ProjectID   *int64           `json:"projectId"`
	Today       reporting.Day    `json:"today"`
	MonthEnd    reporting.Day    `json:"monthEnd"`
	DaysLeft    int              `json:"daysLeft"`
	TimeZone    string           `json:"timeZone"`
	Lines       []LineResolution `json:"lines"`
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
	if len(line.Candidates) < 2 {
		return LaunchResolution{}, core.Conflict("line %d (%s) matched %s; there is nothing to pick", lineIndex, line.Line.Channel, describe(line))
	}
	found := false
	for _, c := range line.Candidates {
		if c.ID == campaignID {
			found = true
		}
	}
	if !found {
		return LaunchResolution{}, core.Invalid("campaign %d is not one of the %d running campaigns line %d matched", campaignID, len(line.Candidates), lineIndex)
	}
	_, err = s.db.Writer().UpsertLaunchPick(ctx, store.UpsertLaunchPickParams{WorkspaceID: actor.WorkspaceID, PlanID: planID, PlanVersion: version, LineIndex: int64(lineIndex), Channel: line.Line.Channel,
		CampaignID: campaignID, PickedBy: optionalID(actor.UserID), PickedAt: now.UTC()})
	if err != nil {
		return LaunchResolution{}, err
	}
	return s.resolve(ctx, actor, planID, version, now)
}

// Coverage says whether a campaign of an account, by the platform's id, has
// an accepted plan behind it today: a plan of its project whose period
// covers today and whose lines resolve to updating it, by a sole match or a
// recorded pick (BD-58 rule 3). A campaign ADX has not read, or that has no
// project, is not under a plan.
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
		if pv.Document.PeriodFrom == "" || pv.Document.PeriodTo == "" || today < pv.Document.PeriodFrom || today > pv.Document.PeriodTo {
			continue
		}
		res, err := s.resolve(ctx, actor, v.Plan.ID, pv.Version, s.clock.Now())
		if err != nil {
			return PlanCoverage{}, err
		}
		for _, line := range res.Lines {
			if line.Kind == KindUpdate && line.Campaign != nil && line.Campaign.ID == row.ID {
				pid, pver := v.Plan.ID, pv.Version
				return PlanCoverage{UnderPlan: true, PlanID: &pid, PlanVersion: &pver, Reason: fmt.Sprintf("plan %d version %d (%s to %s) line %d updates it", pid, pver, pv.Document.PeriodFrom, pv.Document.PeriodTo, line.Line.Index)}, nil
			}
		}
	}
	return PlanCoverage{Reason: "no accepted plan whose period covers today resolves to this campaign"}, nil
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
	today := now.In(reporting.ReportingZone)
	monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, reporting.ReportingZone)
	monthEnd := monthStart.AddDate(0, 1, -1)
	daysLeft := monthEnd.Day() - today.Day() + 1
	res := LaunchResolution{PlanID: planID, PlanVersion: pv.Version, Today: reporting.DayOf(today), MonthEnd: reporting.DayOf(monthEnd), DaysLeft: daysLeft, TimeZone: reporting.ReportingZone.String(),
		Lines: []LineResolution{}, Unmatched: []reporting.ProjectCampaign{},
		Note: "What the launch would do today. One matching running campaign is updated; more than one needs a person's pick; none and a campaign is created. This month's spend nets off each line and the rest is spread over the days left."}
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
		cs, err := s.reporting.ProjectCampaigns(ctx, actor, project, reporting.DayOf(monthStart), reporting.DayOf(monthEnd))
		if err != nil {
			return nil, err
		}
		campaignsByProject[project] = cs
		return cs, nil
	}
	updated := map[int64]bool{}
	for i, a := range pv.Document.Allocations {
		line := LaunchLine{Index: i, Channel: a.Channel, BudgetMinor: a.BudgetMinor, Currency: pv.Document.Currency, Objective: a.Objective, Geography: a.Geography}
		switch {
		case a.ProjectID != nil && *a.ProjectID != 0:
			line.ProjectID = *a.ProjectID
		case plan.ProjectID.Valid:
			line.ProjectID = plan.ProjectID.Int64
		}
		lr := LineResolution{Line: line, Candidates: []reporting.ProjectCampaign{}, Paused: []reporting.ProjectCampaign{}}
		ch, ok := plans.Channel(a.Channel)
		switch {
		case !ok || !ch.Supported || ch.CampaignType == "":
			lr.Kind, lr.Note = KindUnsupported, fmt.Sprintf("ADX cannot deploy %s; this line is not launched", a.Channel)
			res.Lines = append(res.Lines, lr)
			continue
		case line.ProjectID == 0:
			lr.Kind, lr.Note = KindUnsupported, "the line names no project and the plan has none; lines match campaigns by project"
			res.Lines = append(res.Lines, lr)
			continue
		}
		lr.Line.Platform, lr.Line.CampaignType = ch.Platform, ch.CampaignType
		cs, err := campaigns(line.ProjectID)
		if err != nil {
			return LaunchResolution{}, err
		}
		for _, c := range cs {
			if c.Platform != ch.Platform || c.Type != ch.CampaignType {
				continue
			}
			if c.Running {
				lr.Candidates = append(lr.Candidates, c)
			} else {
				lr.Paused = append(lr.Paused, c)
			}
		}
		switch len(lr.Candidates) {
		case 0:
			lr.Kind = KindCreate
			b := budget(line.BudgetMinor, 0, true, daysLeft)
			lr.Budget = &b
			lr.Candidates = []reporting.ProjectCampaign{}
			if len(lr.Paused) > 0 {
				lr.Note = fmt.Sprintf("no running %s campaign; %d paused one(s) of that type exist and are not resumed here", ch.CampaignType, len(lr.Paused))
			}
		case 1:
			c := lr.Candidates[0]
			lr.Kind, lr.Campaign = KindUpdate, &c
			lr.Candidates = []reporting.ProjectCampaign{}
			b := spendBudget(line.BudgetMinor, c, daysLeft)
			lr.Budget = &b
			updated[c.ID] = true
		default:
			lr.Kind = KindPick
			lr.Note = fmt.Sprintf("%d running %s campaigns match; a person picks which one this line updates", len(lr.Candidates), ch.CampaignType)
			if p, ok := pickByLine[int64(i)]; ok {
				var chosen *reporting.ProjectCampaign
				for _, c := range lr.Candidates {
					if c.ID == p.CampaignID {
						cc := c
						chosen = &cc
					}
				}
				if chosen == nil {
					lr.Note = fmt.Sprintf("the earlier choice, campaign %d, no longer matches this line; pick again", p.CampaignID)
					break
				}
				lr.Kind, lr.Campaign, lr.Note = KindUpdate, chosen, ""
				lr.Pick = &LaunchPick{CampaignID: p.CampaignID, PickedBy: idPtr(p.PickedBy), PickedAt: p.PickedAt.UTC()}
				b := spendBudget(line.BudgetMinor, *chosen, daysLeft)
				lr.Budget = &b
				updated[chosen.ID] = true
			}
		}
		res.Lines = append(res.Lines, lr)
	}
	for _, cs := range campaignsByProject {
		for _, c := range cs {
			if c.Running && !updated[c.ID] {
				res.Unmatched = append(res.Unmatched, c)
			}
		}
	}
	return res, nil
}

func spendBudget(lineMinor int64, c reporting.ProjectCampaign, daysLeft int) MonthBudget {
	if c.SpentMinor == nil {
		return budget(lineMinor, 0, false, daysLeft)
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
		return "no running campaign"
	case KindUpdate:
		return "one running campaign"
	case KindUnsupported:
		return "nothing ADX can deploy"
	}
	return l.Kind
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
