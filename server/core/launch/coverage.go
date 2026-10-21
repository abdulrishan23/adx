package launch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
)

// Coverage for Perform (BD-68). A campaign is under a plan when an accepted
// version of one of its project's plans, whose period has not ended,
// resolves a line to updating or resuming it. Where its launch stands is
// read from the proposal that version made for it and the proposal's
// intent: in review while a person has to approve it, submitted while the
// platform is being asked, confirmed once the object reads back, and
// delivering only once the platform's own figures show impressions on or
// after the day it was sent. Nothing here is inferred from a submission.

const (
	LaunchNone       = "none"
	LaunchInReview   = "in_review"
	LaunchSubmitted  = "submitted"
	LaunchConfirmed  = "confirmed"
	LaunchDelivering = "delivering"
	LaunchFailed     = "failed"
)

// LaunchState is where the launch of one campaign stands.
type LaunchState struct {
	// State is none, in_review, submitted, confirmed, delivering or failed.
	// Failed covers everything that did not go through: a policy denial,
	// an admin's rejection, a platform refusal, a cancellation or a
	// compensation, with the note saying which.
	State      string `json:"state"`
	Action     string `json:"action"`
	ProposalID *int64 `json:"proposalId"`
	IntentID   *int64 `json:"intentId"`
	// IntentState is the intent's own word (reserved, submitted,
	// externally_confirmed, ...), for the execution screen.
	IntentState string `json:"intentState"`
	// DeliveryDay is the first day with impressions on or after the day the
	// intent was sent, when delivery has been observed.
	DeliveryDay *reporting.Day `json:"deliveryDay"`
	Note        string         `json:"note"`
}

// CampaignCoverage is one campaign ADX has read, as Perform shows it: the
// plan behind it, which line of which version, and where its launch stands.
type CampaignCoverage struct {
	ID         int64  `json:"id"`
	AccountID  int64  `json:"accountId"`
	Platform   string `json:"platform"`
	ExternalID string `json:"externalId"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Running    bool   `json:"running"`
	// ChannelType is the platform's own word for what the campaign is
	// (SEARCH, PERFORMANCE_MAX, OUTCOME_LEADS, ...), as read.
	ChannelType string `json:"channelType"`
	// DailyBudgetMinor is the daily budget as last read, in the campaign's
	// currency. Null when none was read or the budget is lifetime.
	DailyBudgetMinor *int64 `json:"dailyBudgetMinor"`
	Currency         string `json:"currency"`
	ProjectID        *int64 `json:"projectId"`
	PlanCoverage
	Launch LaunchState `json:"launch"`
}

// Campaigns lists every campaign ADX has read in the workspace, or a
// project's when projectID is set, each with its coverage and launch state.
// Viewers may read it. Each plan is resolved once, not once per campaign.
func (s *Service) Campaigns(ctx context.Context, actor core.Actor, projectID int64) ([]CampaignCoverage, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	var rows []store.AdCampaign
	var err error
	if projectID != 0 {
		rows, err = s.db.Reader().ListAdCampaignsForProject(ctx, store.ListAdCampaignsForProjectParams{WorkspaceID: actor.WorkspaceID, ProjectID: optionalID(projectID)})
	} else {
		rows, err = s.db.Reader().ListAllAdCampaigns(ctx, actor.WorkspaceID)
	}
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	hitsByProject := map[int64]map[int64]coverageHit{}
	out := make([]CampaignCoverage, 0, len(rows))
	for _, c := range rows {
		cc := CampaignCoverage{ID: c.ID, AccountID: c.AccountID, Platform: c.Platform, ExternalID: c.ExternalID, Name: c.Name, Status: c.Status, Running: reporting.Running(c.Platform, c.Status),
			ChannelType: c.ChannelType.String, Currency: c.Currency.String, Launch: LaunchState{State: LaunchNone}}
		if c.BudgetMinor.Valid && c.BudgetPeriod.String == "daily" {
			daily := c.BudgetMinor.Int64
			cc.DailyBudgetMinor = &daily
		}
		if !c.ProjectID.Valid {
			cc.Reason = reasonNoProject
			out = append(out, cc)
			continue
		}
		pid := c.ProjectID.Int64
		cc.ProjectID = &pid
		hits, ok := hitsByProject[pid]
		if !ok {
			hits, err = s.coverageHits(ctx, actor.WorkspaceID, pid, now)
			if err != nil {
				return nil, err
			}
			hitsByProject[pid] = hits
		}
		h, ok := hits[c.ID]
		if !ok {
			cc.Reason = reasonNoPlan
			out = append(out, cc)
			continue
		}
		cc.PlanCoverage = h.coverage()
		cc.Launch, err = s.launchState(ctx, actor.WorkspaceID, h.planID, h.version, c)
		if err != nil {
			return nil, err
		}
		out = append(out, cc)
	}
	return out, nil
}

const (
	reasonNoProject = "the campaign is assigned to no project, so no plan can match it"
	reasonNoPlan    = "no accepted plan whose period has not ended resolves to this campaign"
)

// coverageHit is one campaign a plan version's line updates or resumes.
type coverageHit struct {
	planID, version int64
	from, to        string
	line            int
	kind            string
}

func (h coverageHit) coverage() PlanCoverage {
	pid, pver, line := h.planID, h.version, h.line
	return PlanCoverage{UnderPlan: true, PlanID: &pid, PlanVersion: &pver, Line: &line, Kind: h.kind,
		Reason: fmt.Sprintf("plan %d version %d (%s to %s) line %d %ss it", pid, pver, h.from, h.to, line, h.kind)}
}

// coverageHits resolves the latest accepted version of each of a project's
// plans whose period has not ended and maps every campaign a line updates
// or resumes to that line. Coverage runs from acceptance, not from the
// period's start (BD-68 rules 3 and 11). A version that cannot be resolved
// covers nothing.
func (s *Service) coverageHits(ctx context.Context, workspaceID, projectID int64, now time.Time) (map[int64]coverageHit, error) {
	versions, err := s.db.Reader().ListAcceptedPlanVersionsForProject(ctx, store.ListAcceptedPlanVersionsForProjectParams{WorkspaceID: workspaceID, ProjectID: optionalID(projectID)})
	if err != nil {
		return nil, err
	}
	today := string(reporting.DayOf(now.In(reporting.ReportingZone)))
	actor := core.SystemIn(core.ViaJob, workspaceID)
	hits := map[int64]coverageHit{}
	seen := map[int64]bool{}
	for _, v := range versions {
		if seen[v.Plan.ID] {
			continue // the latest accepted version of each plan speaks for it
		}
		seen[v.Plan.ID] = true
		pv, err := plans.VersionFromRow(v.PlanVersion)
		if err != nil {
			return nil, err
		}
		if pv.Document.PeriodFrom == "" || pv.Document.PeriodTo == "" || today > pv.Document.PeriodTo {
			continue
		}
		res, err := s.resolve(ctx, actor, v.Plan.ID, pv.Version, now)
		var ce *core.Error
		if errors.As(err, &ce) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, line := range res.Lines {
			if (line.Kind == KindUpdate || line.Kind == KindResume) && line.Campaign != nil {
				if _, taken := hits[line.Campaign.ID]; taken {
					continue // the earlier plan keeps it; one campaign is one line's
				}
				hits[line.Campaign.ID] = coverageHit{planID: v.Plan.ID, version: pv.Version, from: pv.Document.PeriodFrom, to: pv.Document.PeriodTo, line: line.Line.Index, kind: line.Kind}
			}
		}
	}
	return hits, nil
}

// launchState reads the proposal the plan version made for the campaign,
// by target or by the campaign reference its intent recorded, and the
// intent behind it. The latest proposal speaks for the campaign.
func (s *Service) launchState(ctx context.Context, workspaceID, planID, version int64, c store.AdCampaign) (LaunchState, error) {
	props, err := s.db.Reader().ListProposalsForPlanVersion(ctx, store.ListProposalsForPlanVersionParams{WorkspaceID: workspaceID, PlanID: planID, PlanVersion: version})
	if err != nil {
		return LaunchState{}, err
	}
	intents, err := s.db.Reader().ListIntentsForPlanVersion(ctx, store.ListIntentsForPlanVersionParams{WorkspaceID: workspaceID, PlanID: planID, PlanVersion: version})
	if err != nil {
		return LaunchState{}, err
	}
	byProposal := map[int64]store.ExecutionIntent{}
	for _, in := range intents {
		byProposal[in.ProposalID] = in // ordered by id: the latest intent of a proposal wins
	}
	var match *store.Proposal
	for i := range props {
		p := props[i]
		if p.AccountID != c.AccountID || p.TargetKind != "campaign" {
			continue
		}
		in, hasIntent := byProposal[p.ID]
		if p.TargetRef != c.ExternalID && (!hasIntent || !refsName(in.ProviderRefs, c.ExternalID)) {
			continue
		}
		match = &p
	}
	if match == nil {
		return LaunchState{State: LaunchNone, Note: "the plan version has not been submitted for this campaign"}, nil
	}
	ls := LaunchState{Action: match.Action, ProposalID: &match.ID}
	switch match.State {
	case "proposed":
		ls.State, ls.Note = LaunchInReview, "waits for an admin's approval in Perform"
	case "approved":
		ls.State, ls.Note = LaunchInReview, "approved, not yet executed"
	case "denied":
		ls.State, ls.Note = LaunchFailed, "policy denied it ("+match.PolicyReason+")"
	case "rejected":
		ls.State, ls.Note = LaunchFailed, "rejected by an admin"
	case "stale":
		ls.State, ls.Note = LaunchFailed, "superseded; the plan version needs submitting again"
	case "cancelled":
		ls.State, ls.Note = LaunchFailed, "cancelled before it was sent"
	default: // executing, done, failed: the intent says where it stands
		in, ok := byProposal[match.ID]
		if !ok {
			ls.State, ls.Note = LaunchSubmitted, "executing, with no intent recorded yet"
			return ls, nil
		}
		ls.IntentID, ls.IntentState = &in.ID, in.State
		switch in.State {
		case "failed", "cancelled":
			ls.State, ls.Note = LaunchFailed, in.LastErrorSafe
		case "compensated":
			ls.State, ls.Note = LaunchFailed, "reversed: "+in.LastErrorSafe
		case "externally_confirmed", "confirmed":
			ls.State, ls.Note = LaunchConfirmed, "the platform shows the change; delivery is read from its figures"
		default:
			ls.State, ls.Note = LaunchSubmitted, "sent, or waiting in the outbox to be sent; not yet confirmed, not delivery"
		}
		// Delivery counts only once the request has left: a campaign that
		// was running before the launch delivers on its old settings while
		// the intent waits in the outbox.
		if (ls.State == LaunchSubmitted || ls.State == LaunchConfirmed) && in.DispatchedAt.Valid {
			day := string(reporting.DayOf(in.DispatchedAt.Time.In(reporting.ReportingZone)))
			first, err := s.db.Reader().FirstDeliveryDayFrom(ctx, store.FirstDeliveryDayFromParams{AccountID: c.AccountID, ExternalID: c.ExternalID, Day: day})
			switch {
			case errors.Is(err, sql.ErrNoRows):
			case err != nil:
				return LaunchState{}, err
			default:
				d := reporting.Day(first.Day)
				ls.State, ls.DeliveryDay, ls.Note = LaunchDelivering, &d, fmt.Sprintf("impressions recorded for %s in the platform's figures", first.Day)
			}
		}
	}
	return ls, nil
}

// refsName says whether a recorded provider reference names the campaign.
func refsName(raw, externalID string) bool {
	var refs []proposals.ProviderRef
	if err := json.Unmarshal([]byte(raw), &refs); err != nil {
		return false
	}
	for _, r := range refs {
		if r.Kind == "campaign" && r.ExternalID == externalID {
			return true
		}
	}
	return false
}
