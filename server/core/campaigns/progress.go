package campaigns

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/measurement"
	"github.com/WokeWorks/adx/server/core/research"
	"github.com/WokeWorks/adx/server/core/studio"
	"github.com/WokeWorks/adx/server/store"
)

// ResearchReader is the part of research a project's progress needs: its
// briefs and whether their items have run.
type ResearchReader interface {
	ListBriefs(ctx context.Context, actor core.Actor) ([]research.Brief, error)
	GetBrief(ctx context.Context, actor core.Actor, id int64) (research.BriefDetail, error)
}

// ReadinessReader is the part of measurement a project's progress needs:
// what a plan's launch is missing.
type ReadinessReader interface {
	Readiness(ctx context.Context, actor core.Actor, planID, planVersion int64) (measurement.PlanLaunchReadiness, error)
}

// AttachReaders wires the readers built after campaigns in cmd/adx. Without
// them, Brand reads facts only and Measurement is not available.
func (c *Campaigns) AttachReaders(r ResearchReader, m ReadinessReader) {
	c.research, c.measurement = r, m
}

// SectionState is where one of a project's five sections stands, the way
// the sidebar shows it (docs/screens.md). Status uses the stage vocabulary:
// pending is not started, caution is in progress, ready is done and error
// is blocked.
type SectionState struct {
	ID     string `json:"id"` // brand, media, studio, measurement, campaigns
	Status string `json:"status"`
	Note   string `json:"note"`
}

// ProgressItem is one launch-readiness line about measurement, with the
// plan and campaign it belongs to, so the Measurement section and the
// Campaigns gate show the same list.
type ProgressItem struct {
	measurement.LaunchItem
	PlanID       int64  `json:"planId"`
	PlanVersion  int64  `json:"planVersion"`
	CampaignID   *int64 `json:"campaignId"`
	CampaignName string `json:"campaignName"`
}

type ProjectProgress struct {
	ProjectID int64          `json:"projectId"`
	Sections  []SectionState `json:"sections"`
	// Measurement lists what the project's plans still need before a launch,
	// in the areas measurement checks: accounts, pages and measurement itself.
	Measurement []ProgressItem `json:"measurement"`
	// Blocked is true while a measurement line is blocked. A blocked
	// measurement gates Campaigns.
	Blocked bool `json:"blocked"`
}

// measuredAreas are the readiness areas that belong to Measurement. The
// plan, creative and platform-review areas belong to Campaigns.
var measuredAreas = map[string]bool{"measurement": true, "accounts": true, "pages": true}

// Progress says where each of a project's sections stands, from what the
// domains already hold. It writes nothing. The rules are in docs/screens.md:
// Brand is done with an approved fact and a finished brief, Media with an
// accepted plan, Studio with an approved ad and a page, Measurement with no
// blocker, Campaigns with a push live.
func (c *Campaigns) Progress(ctx context.Context, actor core.Actor, projectID int64) (ProjectProgress, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return ProjectProgress{}, core.ErrForbidden
	}
	if _, err := c.db.Reader().GetProject(ctx, store.GetProjectParams{ID: projectID, WorkspaceID: actor.WorkspaceID}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProjectProgress{}, core.NotFound("project %d not found", projectID)
		}
		return ProjectProgress{}, err
	}
	out := ProjectProgress{ProjectID: projectID, Measurement: []ProgressItem{}}

	brand, err := c.brandProgress(ctx, actor, projectID)
	if err != nil {
		return out, err
	}

	campaigns, err := c.ListForProject(ctx, actor, projectID)
	if err != nil {
		return out, err
	}
	byCampaign := map[int64]CampaignSummary{}
	for _, k := range campaigns {
		byCampaign[k.ID] = k
	}

	media, projectPlans, err := c.mediaProgress(ctx, actor, projectID, byCampaign)
	if err != nil {
		return out, err
	}
	studioState, err := c.studioProgress(ctx, actor, projectID)
	if err != nil {
		return out, err
	}
	measured, items, blocked, err := c.measurementProgress(ctx, actor, projectPlans, byCampaign)
	if err != nil {
		return out, err
	}
	out.Measurement, out.Blocked = items, blocked

	pushes := SectionState{ID: "campaigns", Status: "pending", Note: "No campaigns yet"}
	live := 0
	for _, k := range campaigns {
		if k.State == "live" {
			live++
		}
	}
	switch {
	case blocked:
		pushes = SectionState{ID: "campaigns", Status: "error", Note: "Blocked by measurement"}
	case live > 0:
		pushes = SectionState{ID: "campaigns", Status: "ready", Note: plural(live, "live", "live")}
	case len(campaigns) > 0:
		pushes = SectionState{ID: "campaigns", Status: "caution", Note: plural(len(campaigns), "in progress", "in progress")}
	}

	out.Sections = []SectionState{brand, media, studioState, measured, pushes}
	return out, nil
}

func (c *Campaigns) brandProgress(ctx context.Context, actor core.Actor, projectID int64) (SectionState, error) {
	s := SectionState{ID: "brand", Status: "pending", Note: "No facts yet"}
	approved, review := 0, 0
	if c.deps.Brands != nil {
		brandID, err := c.deps.Brands.BrandOfProject(ctx, actor, projectID)
		if err != nil && !isNotFound(err) {
			return s, err
		}
		if brandID != 0 {
			d, err := c.deps.Brands.Get(ctx, actor, brandID)
			if err != nil {
				return s, err
			}
			for _, f := range d.Facts {
				if f.ProjectID != nil && *f.ProjectID != projectID {
					continue
				}
				switch f.Class {
				case brands.Approved:
					approved++
				case brands.NeedsReview:
					review++
				}
			}
		}
	}
	briefs, finished := 0, 0
	if c.research != nil {
		all, err := c.research.ListBriefs(ctx, actor)
		if err != nil {
			return s, err
		}
		for _, b := range all {
			if b.ProjectID == nil || *b.ProjectID != projectID {
				continue
			}
			briefs++
			d, err := c.research.GetBrief(ctx, actor, b.ID)
			if err != nil {
				return s, err
			}
			if briefFinished(d) {
				finished++
			}
		}
	}
	switch {
	case approved > 0 && finished > 0:
		s = SectionState{ID: "brand", Status: "ready", Note: fmt.Sprintf("%s, research done", plural(approved, "approved fact", "approved facts"))}
	case approved > 0:
		s = SectionState{ID: "brand", Status: "caution", Note: fmt.Sprintf("%s, no research yet", plural(approved, "approved fact", "approved facts"))}
	case review > 0:
		s = SectionState{ID: "brand", Status: "caution", Note: plural(review, "fact needs review", "facts need review")}
	case briefs > 0 && finished == briefs:
		s = SectionState{ID: "brand", Status: "caution", Note: "Research done, no approved facts"}
	case briefs > 0:
		s = SectionState{ID: "brand", Status: "caution", Note: "Research running"}
	}
	return s, nil
}

// briefFinished is a brief whose items have all run, whichever way.
func briefFinished(d research.BriefDetail) bool {
	if len(d.Items) == 0 {
		return false
	}
	for _, it := range d.Items {
		switch it.State {
		case research.ItemFetched, research.ItemFailed, research.ItemRefused:
		default:
			return false
		}
	}
	return true
}

// mediaProgress also returns the project's plans, which measurement reads.
func (c *Campaigns) mediaProgress(ctx context.Context, actor core.Actor, projectID int64, byCampaign map[int64]CampaignSummary) (SectionState, []projectPlan, error) {
	s := SectionState{ID: "media", Status: "pending", Note: "No plan yet"}
	if c.deps.Plans == nil {
		return s, nil, nil
	}
	all, err := c.deps.Plans.List(ctx, actor)
	if err != nil {
		return s, nil, err
	}
	var mine []projectPlan
	accepted := 0
	for _, p := range all {
		ours := p.ProjectID != nil && *p.ProjectID == projectID
		if !ours && p.CampaignID != nil {
			_, ours = byCampaign[*p.CampaignID]
		}
		if !ours {
			continue
		}
		mine = append(mine, projectPlan{id: p.ID, version: p.LatestVersion, campaignID: p.CampaignID})
		if p.Accepted {
			accepted++
		}
	}
	switch {
	case accepted > 0:
		s = SectionState{ID: "media", Status: "ready", Note: plural(accepted, "plan accepted", "plans accepted")}
	case len(mine) > 0:
		s = SectionState{ID: "media", Status: "caution", Note: plural(len(mine), "plan drafted", "plans drafted")}
	}
	return s, mine, nil
}

type projectPlan struct {
	id, version int64
	campaignID  *int64
}

func (c *Campaigns) studioProgress(ctx context.Context, actor core.Actor, projectID int64) (SectionState, error) {
	s := SectionState{ID: "studio", Status: "pending", Note: "No creatives yet"}
	concepts, ads, approved := 0, 0, 0
	if c.deps.Studio != nil {
		cp, err := c.deps.Studio.Project(ctx, actor, projectID)
		if err != nil && !isNotFound(err) {
			return s, err
		}
		concepts = len(cp.Concepts)
		for _, k := range cp.Concepts {
			for _, a := range k.Ads {
				ads++
				if a.Status == studio.StatusApproved {
					approved++
				}
			}
		}
	}
	pageCount, live := 0, 0
	if c.deps.Pages != nil {
		all, err := c.deps.Pages.ListSummaries(ctx, actor)
		if err != nil {
			return s, err
		}
		for _, p := range all {
			if p.ProjectID == nil || *p.ProjectID != projectID {
				continue
			}
			pageCount++
			if p.PublishedVersion != nil {
				live++
			}
		}
	}
	switch {
	case approved > 0 && pageCount > 0:
		note := fmt.Sprintf("%s, %s", plural(approved, "ad approved", "ads approved"), plural(pageCount, "page", "pages"))
		if live > 0 {
			note = fmt.Sprintf("%s, %s live", plural(approved, "ad approved", "ads approved"), plural(live, "page", "pages"))
		}
		s = SectionState{ID: "studio", Status: "ready", Note: note}
	case approved > 0:
		s = SectionState{ID: "studio", Status: "caution", Note: fmt.Sprintf("%s, no page yet", plural(approved, "ad approved", "ads approved"))}
	case ads > 0:
		s = SectionState{ID: "studio", Status: "caution", Note: plural(ads, "ad awaiting approval", "ads awaiting approval")}
	case concepts > 0 || pageCount > 0:
		s = SectionState{ID: "studio", Status: "caution", Note: "Started, no ads approved"}
	}
	return s, nil
}

// measurementProgress runs the launch-readiness checks for each of the
// project's plans and keeps the measurement lines. No plan means nothing to
// check yet; no reader means the answer isn't available.
func (c *Campaigns) measurementProgress(ctx context.Context, actor core.Actor, planList []projectPlan, byCampaign map[int64]CampaignSummary) (SectionState, []ProgressItem, bool, error) {
	items := []ProgressItem{}
	if c.measurement == nil {
		return SectionState{ID: "measurement", Status: "pending", Note: "Not available"}, items, false, nil
	}
	if len(planList) == 0 {
		return SectionState{ID: "measurement", Status: "pending", Note: "Nothing to check until a campaign has a plan"}, items, false, nil
	}
	blocked, warned := 0, 0
	for _, p := range planList {
		r, err := c.measurement.Readiness(ctx, actor, p.id, p.version)
		if err != nil {
			return SectionState{}, items, false, err
		}
		name := ""
		if p.campaignID != nil {
			name = byCampaign[*p.campaignID].Name
		}
		for _, it := range r.Items {
			if !measuredAreas[it.Area] {
				continue
			}
			items = append(items, ProgressItem{LaunchItem: it, PlanID: p.id, PlanVersion: p.version, CampaignID: p.campaignID, CampaignName: name})
			switch it.State {
			case "blocked":
				blocked++
			case "warning", "unsupported":
				warned++
			}
		}
	}
	switch {
	case blocked > 0:
		return SectionState{ID: "measurement", Status: "error", Note: plural(blocked, "blocker", "blockers")}, items, true, nil
	case warned > 0:
		return SectionState{ID: "measurement", Status: "caution", Note: plural(warned, "warning", "warnings")}, items, false, nil
	default:
		return SectionState{ID: "measurement", Status: "ready", Note: "Checks pass"}, items, false, nil
	}
}
