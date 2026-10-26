package campaigns

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/measurement"
	"github.com/WokeWorks/adx/server/core/plans"
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

// ProjectNext is the one action that moves the project, read from the first
// section that isn't done (docs/build/graphs/g3-next-step). Role is admin,
// marketer, or "" when nothing waits on a person.
type ProjectNext struct {
	Section string `json:"section"`
	Label   string `json:"label"`
	Role    string `json:"role"`
}

// ProjectNeed is something in the project waiting on a person: what kind, how
// many, who can act and which section holds it
// (docs/build/graphs/g4-approval-queue).
type ProjectNeed struct {
	Kind    string `json:"kind"` // facts_review, offers_draft, ads_review, ads_revise, plans_accept, pages_publish, measurement_blocked, launch_authorize
	Count   int    `json:"count"`
	Role    string `json:"role"`
	Section string `json:"section"`
	Label   string `json:"label"`
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
	// Blocker is the first blocked line's message, or "" when none.
	Blocker string        `json:"blocker"`
	Next    ProjectNext   `json:"next"`
	Needs   []ProjectNeed `json:"needs"`
}

// measuredAreas are the readiness areas that belong to Measurement. The
// plan, creative and platform-review areas belong to Campaigns.
var measuredAreas = map[string]bool{"measurement": true, "accounts": true, "pages": true}

// ProgressAll is Progress for every project in the workspace, for the
// Projects page and the switcher, so they make one request.
func (c *Campaigns) ProgressAll(ctx context.Context, actor core.Actor) ([]ProjectProgress, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := c.db.Reader().ListProjects(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectProgress, 0, len(rows))
	for _, r := range rows {
		p, err := c.Progress(ctx, actor, r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// Progress says where each of a project's sections stands, from what the
// domains already hold. It writes nothing. The rules are in docs/screens.md:
// Brand is done with an approved fact and a finished brief, Media with an
// accepted plan, Studio with an approved ad and a page, Measurement with no
// blocker, Campaigns with a push live. The same counts give the next step
// and the list of what waits on a person.
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
	out := ProjectProgress{ProjectID: projectID, Measurement: []ProgressItem{}, Needs: []ProjectNeed{}}

	bc, err := c.brandCounts(ctx, actor, projectID)
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
	mc, projectPlans, err := c.mediaCounts(ctx, actor, projectID, byCampaign)
	if err != nil {
		return out, err
	}
	sc, err := c.studioCounts(ctx, actor, projectID)
	if err != nil {
		return out, err
	}
	measured, items, blocked, err := c.measurementProgress(ctx, actor, projectPlans, byCampaign)
	if err != nil {
		return out, err
	}
	out.Measurement, out.Blocked = items, blocked
	for _, it := range items {
		if it.State == "blocked" {
			out.Blocker = it.Message
			break
		}
	}

	live, furthest := 0, ""
	for _, k := range campaigns {
		if k.State == "live" {
			live++
		}
		if furthest == "" || rank(k.State) > rank(byName(campaigns, furthest).State) {
			furthest = k.Name
		}
	}
	pushes := SectionState{ID: "campaigns", Status: "pending", Note: "No campaigns yet"}
	switch {
	case blocked:
		pushes = SectionState{ID: "campaigns", Status: "error", Note: "Blocked by measurement"}
	case live > 0:
		pushes = SectionState{ID: "campaigns", Status: "ready", Note: plural(live, "live", "live")}
	case len(campaigns) > 0:
		pushes = SectionState{ID: "campaigns", Status: "caution", Note: plural(len(campaigns), "in progress", "in progress")}
	}

	out.Sections = []SectionState{bc.state(), mc.state(), sc.state(), measured, pushes}
	out.Next = nextStep(out.Sections, bc, mc, sc, out.Blocker, len(campaigns), furthest)
	out.Needs = waiting(bc, mc, sc, campaigns, blockedCount(items))
	return out, nil
}

func byName(list []CampaignSummary, name string) CampaignSummary {
	for _, k := range list {
		if k.Name == name {
			return k
		}
	}
	return CampaignSummary{}
}

// rank orders campaign states from least to furthest along.
func rank(state string) int {
	switch state {
	case "live":
		return 5
	case "launching":
		return 4
	case "awaiting_approval":
		return 3
	case "in_production":
		return 2
	case "planning":
		return 1
	}
	return 0
}

func blockedCount(items []ProgressItem) int {
	n := 0
	for _, it := range items {
		if it.State == "blocked" {
			n++
		}
	}
	return n
}

// brandCounts is what Brand's state, next step and needs are read from.
type brandCounts struct {
	approved, review, draftOffers int
	briefs, finished              int
}

func (b brandCounts) state() SectionState {
	s := SectionState{ID: "brand", Status: "pending", Note: "No facts yet"}
	switch {
	case b.approved > 0 && b.finished > 0:
		s = SectionState{ID: "brand", Status: "ready", Note: fmt.Sprintf("%s, research done", plural(b.approved, "approved fact", "approved facts"))}
	case b.approved > 0:
		s = SectionState{ID: "brand", Status: "caution", Note: fmt.Sprintf("%s, no research yet", plural(b.approved, "approved fact", "approved facts"))}
	case b.review > 0:
		s = SectionState{ID: "brand", Status: "caution", Note: plural(b.review, "fact needs review", "facts need review")}
	case b.briefs > 0 && b.finished == b.briefs:
		s = SectionState{ID: "brand", Status: "caution", Note: "Research done, no approved facts"}
	case b.briefs > 0:
		s = SectionState{ID: "brand", Status: "caution", Note: "Research running"}
	}
	return s
}

func (c *Campaigns) brandCounts(ctx context.Context, actor core.Actor, projectID int64) (brandCounts, error) {
	var bc brandCounts
	if c.deps.Brands != nil {
		brandID, err := c.deps.Brands.BrandOfProject(ctx, actor, projectID)
		if err != nil && !isNotFound(err) {
			return bc, err
		}
		if brandID != 0 {
			d, err := c.deps.Brands.Get(ctx, actor, brandID)
			if err != nil {
				return bc, err
			}
			for _, f := range d.Facts {
				if f.ProjectID != nil && *f.ProjectID != projectID {
					continue
				}
				switch f.Class {
				case brands.Approved:
					bc.approved++
				case brands.NeedsReview:
					bc.review++
				}
			}
			for _, o := range d.Offers {
				if o.ProjectID != nil && *o.ProjectID != projectID {
					continue
				}
				if o.Current.State == brands.OfferDraft {
					bc.draftOffers++
				}
			}
		}
	}
	if c.research != nil {
		all, err := c.research.ListBriefs(ctx, actor)
		if err != nil {
			return bc, err
		}
		for _, b := range all {
			if b.ProjectID == nil || *b.ProjectID != projectID {
				continue
			}
			bc.briefs++
			d, err := c.research.GetBrief(ctx, actor, b.ID)
			if err != nil {
				return bc, err
			}
			if briefFinished(d) {
				bc.finished++
			}
		}
	}
	return bc, nil
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

// mediaCounts is what Media's state, next step and needs are read from.
type mediaCounts struct {
	plans, accepted, ready int
}

func (m mediaCounts) state() SectionState {
	s := SectionState{ID: "media", Status: "pending", Note: "No plan yet"}
	switch {
	case m.accepted > 0:
		s = SectionState{ID: "media", Status: "ready", Note: plural(m.accepted, "plan accepted", "plans accepted")}
	case m.plans > 0:
		s = SectionState{ID: "media", Status: "caution", Note: plural(m.plans, "plan drafted", "plans drafted")}
	}
	return s
}

type projectPlan struct {
	id, version int64
	campaignID  *int64
}

// mediaCounts also returns the project's plans, which measurement reads.
func (c *Campaigns) mediaCounts(ctx context.Context, actor core.Actor, projectID int64, byCampaign map[int64]CampaignSummary) (mediaCounts, []projectPlan, error) {
	var mc mediaCounts
	if c.deps.Plans == nil {
		return mc, nil, nil
	}
	all, err := c.deps.Plans.List(ctx, actor)
	if err != nil {
		return mc, nil, err
	}
	var mine []projectPlan
	for _, p := range all {
		ours := p.ProjectID != nil && *p.ProjectID == projectID
		if !ours && p.CampaignID != nil {
			_, ours = byCampaign[*p.CampaignID]
		}
		if !ours {
			continue
		}
		mine = append(mine, projectPlan{id: p.ID, version: p.LatestVersion, campaignID: p.CampaignID})
		mc.plans++
		switch {
		case p.Accepted:
			mc.accepted++
		case p.LatestState == plans.StateReady:
			mc.ready++
		}
	}
	return mc, mine, nil
}

// studioCounts is what Studio's state, next step and needs are read from.
type studioCounts struct {
	concepts, ads, approved, waiting, revise int
	pages, live, unpublished                 int
}

func (s studioCounts) state() SectionState {
	out := SectionState{ID: "studio", Status: "pending", Note: "No creatives yet"}
	switch {
	case s.approved > 0 && s.pages > 0:
		note := fmt.Sprintf("%s, %s", plural(s.approved, "ad approved", "ads approved"), plural(s.pages, "page", "pages"))
		if s.live > 0 {
			note = fmt.Sprintf("%s, %s live", plural(s.approved, "ad approved", "ads approved"), plural(s.live, "page", "pages"))
		}
		out = SectionState{ID: "studio", Status: "ready", Note: note}
	case s.approved > 0:
		out = SectionState{ID: "studio", Status: "caution", Note: fmt.Sprintf("%s, no page yet", plural(s.approved, "ad approved", "ads approved"))}
	case s.ads > 0:
		out = SectionState{ID: "studio", Status: "caution", Note: plural(s.ads, "ad awaiting approval", "ads awaiting approval")}
	case s.concepts > 0 || s.pages > 0:
		out = SectionState{ID: "studio", Status: "caution", Note: "Started, no ads approved"}
	}
	return out
}

func (c *Campaigns) studioCounts(ctx context.Context, actor core.Actor, projectID int64) (studioCounts, error) {
	var sc studioCounts
	if c.deps.Studio != nil {
		cp, err := c.deps.Studio.Project(ctx, actor, projectID)
		if err != nil && !isNotFound(err) {
			return sc, err
		}
		sc.concepts = len(cp.Concepts)
		for _, k := range cp.Concepts {
			for _, a := range k.Ads {
				sc.ads++
				switch a.Status {
				case studio.StatusApproved:
					sc.approved++
				case studio.StatusWaiting:
					sc.waiting++
				case studio.StatusRevise:
					sc.revise++
				}
			}
		}
	}
	if c.deps.Pages != nil {
		all, err := c.deps.Pages.ListSummaries(ctx, actor)
		if err != nil {
			return sc, err
		}
		for _, p := range all {
			if p.ProjectID == nil || *p.ProjectID != projectID {
				continue
			}
			sc.pages++
			if p.PublishedVersion != nil {
				sc.live++
			} else {
				sc.unpublished++
			}
		}
	}
	return sc, nil
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
	case len(items) == 0:
		// A plan exists but readiness produced no measurement line for it
		// yet, for example before it names a page or an account.
		return SectionState{ID: "measurement", Status: "pending", Note: "No measurement lines yet"}, items, false, nil
	case blocked > 0:
		return SectionState{ID: "measurement", Status: "error", Note: plural(blocked, "blocker", "blockers")}, items, true, nil
	case warned > 0:
		return SectionState{ID: "measurement", Status: "caution", Note: plural(warned, "warning", "warnings")}, items, false, nil
	default:
		return SectionState{ID: "measurement", Status: "ready", Note: "Checks pass"}, items, false, nil
	}
}

// nextStep is the first section, in order, that isn't done, read with what
// it holds (the table in docs/build/graphs/g3-next-step/graph.md).
func nextStep(sections []SectionState, bc brandCounts, mc mediaCounts, sc studioCounts, blocker string, campaignCount int, furthest string) ProjectNext {
	for _, s := range sections {
		if s.Status == "ready" {
			continue
		}
		switch s.ID {
		case "brand":
			switch {
			case bc.review > 0:
				return ProjectNext{Section: "brand", Label: label(bc.review, "Approve 1 fact", "Approve %d facts"), Role: "admin"}
			case bc.approved == 0:
				return ProjectNext{Section: "brand", Label: "Record the first fact", Role: "marketer"}
			case bc.briefs == 0:
				return ProjectNext{Section: "brand", Label: "Start research", Role: "marketer"}
			default:
				return ProjectNext{Section: "brand", Label: "Research running", Role: ""}
			}
		case "media":
			switch {
			case mc.plans == 0:
				return ProjectNext{Section: "media", Label: "Draft a plan", Role: "marketer"}
			case mc.ready > 0:
				return ProjectNext{Section: "media", Label: "Accept the plan", Role: "admin"}
			default:
				return ProjectNext{Section: "media", Label: "Finish the plan", Role: "marketer"}
			}
		case "studio":
			switch {
			case sc.revise > 0:
				return ProjectNext{Section: "studio", Label: label(sc.revise, "Fix 1 ad", "Fix %d ads"), Role: "marketer"}
			case sc.waiting > 0:
				return ProjectNext{Section: "studio", Label: label(sc.waiting, "Review 1 ad", "Review %d ads"), Role: "marketer"}
			case sc.approved > 0 && sc.pages == 0:
				return ProjectNext{Section: "studio", Label: "Write the page", Role: "marketer"}
			default:
				return ProjectNext{Section: "studio", Label: "Upload ads or ask Adx for concepts", Role: "marketer"}
			}
		case "measurement":
			if blocker != "" {
				return ProjectNext{Section: "measurement", Label: "Fix: " + blocker, Role: "admin"}
			}
			// Pending without a blocker means there is nothing to check
			// yet; the push is the next thing, so fall through.
			continue
		case "campaigns":
			if campaignCount == 0 {
				return ProjectNext{Section: "campaigns", Label: "Start a campaign", Role: "marketer"}
			}
			return ProjectNext{Section: "campaigns", Label: "Open " + furthest, Role: "marketer"}
		}
	}
	return ProjectNext{Section: "", Label: "Nothing waiting", Role: ""}
}

// waiting lists what waits on a person in the project (the table in
// docs/build/graphs/g4-approval-queue/graph.md). Empty when nothing does.
func waiting(bc brandCounts, mc mediaCounts, sc studioCounts, campaigns []CampaignSummary, blockers int) []ProjectNeed {
	out := []ProjectNeed{}
	add := func(kind string, n int, role, section, one, many string) {
		if n > 0 {
			out = append(out, ProjectNeed{Kind: kind, Count: n, Role: role, Section: section, Label: label(n, one, many)})
		}
	}
	add("facts_review", bc.review, "admin", "brand", "1 fact needs review", "%d facts need review")
	add("offers_draft", bc.draftOffers, "admin", "brand", "1 offer awaits approval", "%d offers await approval")
	add("plans_accept", mc.ready, "admin", "media", "1 plan ready to accept", "%d plans ready to accept")
	add("ads_review", sc.waiting, "marketer", "studio", "1 ad awaits review", "%d ads await review")
	add("ads_revise", sc.revise, "marketer", "studio", "1 ad needs changes", "%d ads need changes")
	add("pages_publish", sc.unpublished, "marketer", "studio", "1 page drafted, not published", "%d pages drafted, not published")
	add("measurement_blocked", blockers, "admin", "measurement", "1 measurement blocker", "%d measurement blockers")
	unauthorized := 0
	for _, k := range campaigns {
		if stage(k, "plan").Status == "ready" && stage(k, "approvals").Status == "pending" {
			unauthorized++
		}
	}
	add("launch_authorize", unauthorized, "admin", "campaigns", "1 accepted plan awaits launch authorization", "%d accepted plans await launch authorization")
	return out
}

// label is one for a count of 1, else many with %d filled in.
func label(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return fmt.Sprintf(many, n)
}

func stage(k CampaignSummary, id string) CampaignStage {
	for _, s := range k.Stages {
		if s.ID == id {
			return s
		}
	}
	return CampaignStage{}
}
