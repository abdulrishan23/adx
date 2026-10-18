package reporting

import (
	"context"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

// ProjectCampaign is one of a project's campaigns as the launch resolution
// and Perform need it: the platform's own type and status, whether it is
// running, its daily budget and what it spent in the asked window.
type ProjectCampaign struct {
	ID         int64  `json:"id"`
	AccountID  int64  `json:"accountId"`
	Platform   string `json:"platform"`
	ExternalID string `json:"externalId"`
	Name       string `json:"name"`
	// Type is the platform's campaign type as read: Google's advertising
	// channel type (SEARCH, PERFORMANCE_MAX) or Meta's objective
	// (OUTCOME_LEADS). Empty when the platform gave none.
	Type   string `json:"type"`
	Status string `json:"status"`
	// Running is the platform's status read as "delivering or able to":
	// Google ENABLED, Meta ACTIVE. Paused, removed and under-review states
	// are not running.
	Running   bool   `json:"running"`
	ProjectID *int64 `json:"projectId"`
	Currency  string `json:"currency"`
	// DailyBudgetMinor is the campaign's own daily budget, nil when the
	// platform gave none or the budget is not daily.
	DailyBudgetMinor *int64 `json:"dailyBudgetMinor"`
	// SpentMinor is the sum of the campaign's daily cost over the asked
	// window. Nil when no day in the window has been read: not available,
	// never zero.
	SpentMinor *int64 `json:"spentMinor"`
	// SpendDays is how many days of the window have figures.
	SpendDays int64 `json:"spendDays"`
}

// Running says whether a platform status means the campaign is live. Only
// the states ADX reads today are known; anything else is not running.
func Running(platform, status string) bool {
	switch platform {
	case "google_ads":
		return status == "ENABLED"
	case "meta":
		return status == "ACTIVE"
	}
	return false
}

// ProjectCampaigns lists a project's campaigns (not removed) with their
// spend over the days from..to inclusive, as the account reported them.
// Viewers may read it.
func (r *Reporting) ProjectCampaigns(ctx context.Context, actor core.Actor, projectID int64, from, to Day) ([]ProjectCampaign, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if projectID == 0 {
		return nil, core.Invalid("a project is required")
	}
	if _, _, err := dayRange(from, to); err != nil {
		return nil, err
	}
	rows, err := r.db.Reader().ListAdCampaignsForProject(ctx, store.ListAdCampaignsForProjectParams{WorkspaceID: actor.WorkspaceID, ProjectID: optionalID(projectID)})
	if err != nil {
		return nil, err
	}
	spend, err := r.db.Reader().SumCampaignSpendForProject(ctx, store.SumCampaignSpendForProjectParams{WorkspaceID: actor.WorkspaceID, ProjectID: optionalID(projectID), FromDay: string(from), ToDay: string(to)})
	if err != nil {
		return nil, err
	}
	spent := map[int64]store.SumCampaignSpendForProjectRow{}
	for _, s := range spend {
		spent[s.CampaignID] = s
	}
	out := make([]ProjectCampaign, 0, len(rows))
	for _, c := range rows {
		pc := ProjectCampaign{ID: c.ID, AccountID: c.AccountID, Platform: c.Platform, ExternalID: c.ExternalID, Name: c.Name, Type: c.ChannelType.String, Status: c.Status,
			Running: Running(c.Platform, c.Status), Currency: c.Currency.String}
		if c.ProjectID.Valid {
			id := c.ProjectID.Int64
			pc.ProjectID = &id
		}
		if c.BudgetMinor.Valid && c.BudgetPeriod.String == "daily" {
			v := c.BudgetMinor.Int64
			pc.DailyBudgetMinor = &v
		}
		if s, ok := spent[c.ID]; ok {
			v := s.SpentMinor
			pc.SpentMinor = &v
			pc.SpendDays = s.Days
		}
		out = append(out, pc)
	}
	return out, nil
}
