// Package campaigns is one push for a project: an offer, a window, a budget
// and channels. The ads, the page and the plan made for that push belong to
// it, and its stages say what's done and what's next (BD-64).
package campaigns

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/execution"
	"github.com/WokeWorks/adx/server/core/pages"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/studio"
	"github.com/WokeWorks/adx/server/store"
)

type Campaign struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	Name      string `json:"name"`
	Goal      string `json:"goal"`
	Audience  string `json:"audience"`
	Avoid     string `json:"avoid"`
	OfferID   *int64 `json:"offerId"`
	// StartsOn and EndsOn are dates, YYYY-MM-DD, or empty.
	StartsOn string `json:"startsOn"`
	EndsOn   string `json:"endsOn"`
	// BudgetMinor is the whole budget in minor units of Currency, or nil.
	BudgetMinor *int64    `json:"budgetMinor"`
	Currency    string    `json:"currency"`
	Channels    []string  `json:"channels"` // meta, google
	Markets     []string  `json:"markets"`  // free text: UAE, India
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CampaignInput struct {
	ProjectID   int64    `json:"projectId"`
	Name        string   `json:"name"`
	Goal        string   `json:"goal"`
	Audience    string   `json:"audience"`
	Avoid       string   `json:"avoid"`
	OfferID     int64    `json:"offerId"`
	StartsOn    string   `json:"startsOn"`
	EndsOn      string   `json:"endsOn"`
	BudgetMinor *int64   `json:"budgetMinor"`
	Currency    string   `json:"currency"`
	Channels    []string `json:"channels"`
	Markets     []string `json:"markets"`
}

// CampaignStage is one step of the campaign's work, with the state a person needs
// at a glance. Status is ready, caution or pending, the same words the
// screens use.
type CampaignStage struct {
	ID     string `json:"id"` // brief, ads, page, plan, approvals, live
	Status string `json:"status"`
	Note   string `json:"note"`
}

// CampaignSummary is a campaign with its project and overall state, for
// lists.
type CampaignSummary struct {
	Campaign
	ProjectName string          `json:"projectName"`
	Stages      []CampaignStage `json:"stages"`
	// State is the campaign's overall word: planning, in_production,
	// awaiting_approval, live, done.
	State string `json:"state"`
}

// CampaignDetail is one campaign with everything its screen shows.
type CampaignDetail struct {
	CampaignSummary
	Offer    *brands.Offer           `json:"offer"`
	Concepts []studio.Concept        `json:"concepts"`
	Pages    []pages.PageSummary     `json:"pages"`
	Plan     *plans.PlanSummary      `json:"plan"`
	Launch   *execution.LaunchStatus `json:"launch"`
	Ads      AdTally                 `json:"ads"`
	// Needs is what the campaign's channels ask for and what it has.
	Needs []ChannelNeed `json:"needs"`
}

type AdTally struct {
	Total    int `json:"total"`
	Approved int `json:"approved"`
	Revise   int `json:"revise"`
	Waiting  int `json:"waiting"`
}

// Deps are the domains a campaign is made of. Each is read, never written,
// so the campaign screen is a view over what those domains own.
type Deps struct {
	Brands *brands.Brands
	Studio *studio.Studio
	Pages  *pages.Pages
	Plans  *plans.Plans
	Launch LaunchReader
}

// LaunchReader is the part of execution a campaign needs: where an accepted
// plan version stands on the platforms.
type LaunchReader interface {
	Launch(ctx context.Context, actor core.Actor, planID, version int64) (execution.LaunchStatus, error)
}

type Campaigns struct {
	db   *store.DB
	deps Deps
	now  func() time.Time
	// research and measurement are built after campaigns in cmd/adx, so
	// they arrive through AttachReaders. Progress reads them.
	research    ResearchReader
	measurement ReadinessReader
}

func New(db *store.DB, deps Deps) *Campaigns {
	return &Campaigns{db: db, deps: deps, now: func() time.Time { return time.Now().UTC() }}
}

func (c *Campaigns) Create(ctx context.Context, actor core.Actor, in CampaignInput) (CampaignDetail, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return CampaignDetail{}, core.ErrForbidden
	}
	if in.ProjectID == 0 {
		return CampaignDetail{}, core.Invalid("a project is required")
	}
	if _, err := c.db.Reader().GetProject(ctx, store.GetProjectParams{ID: in.ProjectID, WorkspaceID: actor.WorkspaceID}); errors.Is(err, sql.ErrNoRows) {
		return CampaignDetail{}, core.NotFound("project not found")
	} else if err != nil {
		return CampaignDetail{}, err
	}
	p, err := c.params(ctx, actor, in)
	if err != nil {
		return CampaignDetail{}, err
	}
	now := c.now()
	row, err := c.db.Writer().InsertCampaign(ctx, store.InsertCampaignParams{
		WorkspaceID: actor.WorkspaceID, ProjectID: in.ProjectID, Name: p.name, Goal: p.goal, Audience: p.audience, Avoid: p.avoid,
		OfferID: p.offer, StartsOn: p.starts, EndsOn: p.ends, BudgetMinor: p.budget, Currency: p.currency, Channels: p.channels, Markets: p.markets,
		CreatedBy: sql.NullInt64{Int64: actor.UserID, Valid: actor.UserID != 0}, CreatedAt: now, UpdatedAt: now,
	})
	if store.IsUniqueViolation(err) {
		return CampaignDetail{}, core.Conflict("this project already has a campaign named %q", p.name)
	}
	if err != nil {
		return CampaignDetail{}, err
	}
	// A campaign's ads are made in the studio, so the project gets its file
	// prefix and review link now rather than at a Set up button.
	if c.deps.Studio != nil {
		if _, err := c.deps.Studio.Project(ctx, actor, in.ProjectID); isNotFound(err) {
			if _, err := c.deps.Studio.SetUp(ctx, actor, studio.SetupInput{ProjectID: in.ProjectID}); err != nil && !isConflict(err) {
				return CampaignDetail{}, err
			}
		}
	}
	return c.Get(ctx, actor, row.ID)
}

func (c *Campaigns) Update(ctx context.Context, actor core.Actor, id int64, in CampaignInput) (CampaignDetail, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return CampaignDetail{}, core.ErrForbidden
	}
	cur, err := c.db.Reader().GetCampaign(ctx, store.GetCampaignParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return CampaignDetail{}, core.NotFound("campaign not found")
	}
	if err != nil {
		return CampaignDetail{}, err
	}
	in.ProjectID = cur.ProjectID
	p, err := c.params(ctx, actor, in)
	if err != nil {
		return CampaignDetail{}, err
	}
	_, err = c.db.Writer().UpdateCampaign(ctx, store.UpdateCampaignParams{
		Name: p.name, Goal: p.goal, Audience: p.audience, Avoid: p.avoid, OfferID: p.offer, StartsOn: p.starts, EndsOn: p.ends,
		BudgetMinor: p.budget, Currency: p.currency, Channels: p.channels, Markets: p.markets, UpdatedAt: c.now(), ID: id, WorkspaceID: actor.WorkspaceID,
	})
	if store.IsUniqueViolation(err) {
		return CampaignDetail{}, core.Conflict("this project already has a campaign named %q", p.name)
	}
	if err != nil {
		return CampaignDetail{}, err
	}
	return c.Get(ctx, actor, id)
}

type checked struct {
	name, goal, audience, avoid string
	offer                       sql.NullInt64
	starts, ends                sql.NullTime
	budget                      sql.NullInt64
	currency                    string
	channels, markets           string
}

// legacyChannels are the two words the first campaigns were made with,
// before channels were the plan's catalogue.
var legacyChannels = map[string]string{"meta": "meta_lead_form", "google": "google_search"}

func (c *Campaigns) params(ctx context.Context, actor core.Actor, in CampaignInput) (checked, error) {
	var p checked
	p.name = strings.TrimSpace(in.Name)
	if p.name == "" {
		return p, core.Invalid("a name is required")
	}
	if utf8.RuneCountInString(p.name) > 120 {
		return p, core.Invalid("the name can be up to 120 characters")
	}
	for _, f := range []struct {
		what string
		v    string
		max  int
		dst  *string
	}{{"goal", in.Goal, 400, &p.goal}, {"audience", in.Audience, 1000, &p.audience}, {"avoid", in.Avoid, 1000, &p.avoid}} {
		v := strings.TrimSpace(f.v)
		if utf8.RuneCountInString(v) > f.max {
			return p, core.Invalid("the %s can be up to %d characters", f.what, f.max)
		}
		*f.dst = v
	}
	if in.OfferID != 0 {
		_, offers, err := c.deps.Brands.OffersForProject(ctx, actor, in.ProjectID)
		if err != nil {
			return p, err
		}
		found := false
		for _, o := range offers {
			if o.ID == in.OfferID {
				found = true
			}
		}
		if !found {
			return p, core.Invalid("that offer isn't one of this project's")
		}
		p.offer = sql.NullInt64{Int64: in.OfferID, Valid: true}
	}
	var err error
	if p.starts, err = date("start", in.StartsOn); err != nil {
		return p, err
	}
	if p.ends, err = date("end", in.EndsOn); err != nil {
		return p, err
	}
	if p.starts.Valid && p.ends.Valid && p.ends.Time.Before(p.starts.Time) {
		return p, core.Invalid("the end is before the start")
	}
	if in.BudgetMinor != nil {
		if *in.BudgetMinor < 0 {
			return p, core.Invalid("the budget can't be negative")
		}
		p.budget = sql.NullInt64{Int64: *in.BudgetMinor, Valid: true}
	}
	p.currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if p.currency == "" {
		p.currency = "AED"
	}
	if len(p.currency) != 3 {
		return p, core.Invalid("the currency is a three-letter code")
	}
	chans := []string{}
	for _, ch := range in.Channels {
		ch = strings.ToLower(strings.TrimSpace(ch))
		if l, ok := legacyChannels[ch]; ok {
			ch = l
		}
		if _, ok := plans.ChannelByKey(ch); !ok {
			return p, core.Invalid("%q is not a channel Adx knows", ch)
		}
		chans = append(chans, ch)
	}
	markets := []string{}
	for _, m := range in.Markets {
		if m = strings.TrimSpace(m); m != "" {
			markets = append(markets, m)
		}
	}
	b, _ := json.Marshal(chans)
	p.channels = string(b)
	b, _ = json.Marshal(markets)
	p.markets = string(b)
	return p, nil
}

func date(what, v string) (sql.NullTime, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return sql.NullTime{}, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return sql.NullTime{}, core.Invalid("the %s date is YYYY-MM-DD", what)
	}
	return sql.NullTime{Time: t, Valid: true}, nil
}

// List is every campaign in the workspace with its stages, newest first.
func (c *Campaigns) List(ctx context.Context, actor core.Actor) ([]CampaignSummary, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := c.db.Reader().ListCampaigns(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]CampaignSummary, 0, len(rows))
	for _, r := range rows {
		d, err := c.detail(ctx, actor, r, "")
		if err != nil {
			return nil, err
		}
		out = append(out, d.CampaignSummary)
	}
	return out, nil
}

// ListForProject is a project's campaigns with their stages.
func (c *Campaigns) ListForProject(ctx context.Context, actor core.Actor, projectID int64) ([]CampaignSummary, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := c.db.Reader().ListCampaignsForProject(ctx, store.ListCampaignsForProjectParams{WorkspaceID: actor.WorkspaceID, ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	out := make([]CampaignSummary, 0, len(rows))
	for _, r := range rows {
		d, err := c.detail(ctx, actor, r, "")
		if err != nil {
			return nil, err
		}
		out = append(out, d.CampaignSummary)
	}
	return out, nil
}

func (c *Campaigns) Get(ctx context.Context, actor core.Actor, id int64) (CampaignDetail, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return CampaignDetail{}, core.ErrForbidden
	}
	row, err := c.db.Reader().GetCampaign(ctx, store.GetCampaignParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return CampaignDetail{}, core.NotFound("campaign not found")
	}
	if err != nil {
		return CampaignDetail{}, err
	}
	return c.detail(ctx, actor, row, "")
}

// detail reads the campaign's ads, pages, plan and launch, and works out
// each stage from them.
func (c *Campaigns) detail(ctx context.Context, actor core.Actor, row store.Campaign, projectName string) (CampaignDetail, error) {
	d := CampaignDetail{CampaignSummary: CampaignSummary{Campaign: fromRow(row), ProjectName: projectName}, Concepts: []studio.Concept{}, Pages: []pages.PageSummary{}}
	if projectName == "" {
		p, err := c.db.Reader().GetProject(ctx, store.GetProjectParams{ID: row.ProjectID, WorkspaceID: actor.WorkspaceID})
		if err != nil {
			return d, err
		}
		d.ProjectName = p.Name
	}

	// Brief: the offer, when one is named.
	if row.OfferID.Valid {
		_, offers, err := c.deps.Brands.OffersForProject(ctx, actor, row.ProjectID)
		if err != nil {
			return d, err
		}
		for i := range offers {
			if offers[i].ID == row.OfferID.Int64 {
				d.Offer = &offers[i]
			}
		}
	}
	brief := CampaignStage{ID: "brief", Status: "pending", Note: "No goal yet"}
	switch {
	case row.Goal != "" && d.Offer != nil && d.Offer.Current.State == brands.OfferApproved:
		brief = CampaignStage{ID: "brief", Status: "ready", Note: "Set"}
	case row.Goal != "" && d.Offer != nil:
		brief = CampaignStage{ID: "brief", Status: "caution", Note: "Offer not approved"}
	case row.Goal != "":
		brief = CampaignStage{ID: "brief", Status: "ready", Note: "Set, no offer"}
	}

	// Ads: the campaign's concepts and their review state.
	if c.deps.Studio != nil {
		cp, err := c.deps.Studio.Project(ctx, actor, row.ProjectID)
		if err != nil && !isNotFound(err) {
			return d, err
		}
		for _, k := range cp.Concepts {
			if k.CampaignID != nil && *k.CampaignID == row.ID {
				d.Concepts = append(d.Concepts, k)
				for _, a := range k.Ads {
					d.Ads.Total++
					switch a.Status {
					case studio.StatusApproved:
						d.Ads.Approved++
					case studio.StatusRevise:
						d.Ads.Revise++
					case studio.StatusWaiting:
						d.Ads.Waiting++
					}
				}
			}
		}
	}
	d.Needs = needs(d.Channels, d.Concepts)
	ads := CampaignStage{ID: "ads", Status: "pending", Note: "No ads yet"}
	switch {
	case d.Ads.Revise > 0:
		ads = CampaignStage{ID: "ads", Status: "caution", Note: plural(d.Ads.Revise, "needs changes", "need changes")}
	case d.Ads.Total > 0 && d.Ads.Approved == d.Ads.Total:
		ads = CampaignStage{ID: "ads", Status: "ready", Note: "All approved"}
	case d.Ads.Waiting > 0:
		ads = CampaignStage{ID: "ads", Status: "caution", Note: plural(d.Ads.Waiting, "awaiting review", "awaiting review")}
	case d.Ads.Total > 0:
		ads = CampaignStage{ID: "ads", Status: "caution", Note: "Not reviewed"}
	case len(d.Concepts) > 0:
		ads = CampaignStage{ID: "ads", Status: "pending", Note: "Ideas, no ads yet"}
	}

	// Page: the campaign's pages, published or not.
	if c.deps.Pages != nil {
		all, err := c.deps.Pages.ListSummaries(ctx, actor)
		if err != nil {
			return d, err
		}
		for _, p := range all {
			if p.CampaignID != nil && *p.CampaignID == row.ID {
				d.Pages = append(d.Pages, p)
			}
		}
	}
	page := CampaignStage{ID: "page", Status: "pending", Note: "No page yet"}
	if !needsPage(d.Needs) && len(d.Pages) == 0 {
		page = CampaignStage{ID: "page", Status: "ready", Note: "Not needed"}
	}
	if n := len(d.Pages); n > 0 {
		live := 0
		for _, p := range d.Pages {
			if p.PublishedVersion != nil {
				live++
			}
		}
		switch {
		case live == n:
			page = CampaignStage{ID: "page", Status: "ready", Note: "Live"}
		case live > 0:
			page = CampaignStage{ID: "page", Status: "caution", Note: "Partly live"}
		default:
			page = CampaignStage{ID: "page", Status: "caution", Note: "Drafted, not published"}
		}
	}

	// Plan: the campaign's plan, and whether its latest version is accepted.
	if c.deps.Plans != nil {
		all, err := c.deps.Plans.List(ctx, actor)
		if err != nil {
			return d, err
		}
		for i := range all {
			if all[i].CampaignID != nil && *all[i].CampaignID == row.ID {
				d.Plan = &all[i]
				break
			}
		}
	}
	plan := CampaignStage{ID: "plan", Status: "pending", Note: "No plan yet"}
	if d.Plan != nil {
		switch {
		case d.Plan.Accepted:
			plan = CampaignStage{ID: "plan", Status: "ready", Note: "Accepted"}
		case d.Plan.LatestState == plans.StateReady:
			plan = CampaignStage{ID: "plan", Status: "caution", Note: "Ready to accept"}
		default:
			plan = CampaignStage{ID: "plan", Status: "caution", Note: "Draft"}
		}
	}

	// Live: where the accepted plan stands on the platforms.
	if d.Plan != nil && d.Plan.Accepted && c.deps.Launch != nil {
		ls, err := c.deps.Launch.Launch(ctx, actor, d.Plan.ID, d.Plan.LatestVersion)
		if err != nil {
			return d, err
		}
		d.Launch = &ls
	}
	approvals := CampaignStage{ID: "approvals", Status: "pending", Note: "Spend not authorized"}
	live := CampaignStage{ID: "live", Status: "pending", Note: "Not yet"}
	if d.Launch != nil {
		switch d.Launch.State {
		case "none":
		case "confirmed":
			approvals = CampaignStage{ID: "approvals", Status: "ready", Note: "Authorized"}
			live = CampaignStage{ID: "live", Status: "ready", Note: "Live on every platform"}
		case "failed":
			approvals = CampaignStage{ID: "approvals", Status: "ready", Note: "Authorized"}
			live = CampaignStage{ID: "live", Status: "error", Note: "Failed"}
		default:
			approvals = CampaignStage{ID: "approvals", Status: "ready", Note: "Authorized"}
			live = CampaignStage{ID: "live", Status: "caution", Note: "Submitting"}
		}
	}
	if approvals.Status == "pending" && d.Ads.Total > 0 && d.Ads.Approved < d.Ads.Total {
		approvals.Note = "Creative and spend"
	}

	d.Stages = []CampaignStage{brief, ads, page, plan, approvals, live}
	d.State = overall(d.Stages)
	return d, nil
}

// overall names the campaign by its furthest stage.
func overall(stages []CampaignStage) string {
	by := map[string]CampaignStage{}
	for _, s := range stages {
		by[s.ID] = s
	}
	switch {
	case by["live"].Status == "ready":
		return "live"
	case by["live"].Status == "caution" || by["live"].Status == "error":
		return "launching"
	case by["ads"].Status == "ready" && by["page"].Status == "ready" && by["plan"].Status == "ready":
		return "awaiting_approval"
	case by["ads"].Status != "pending" || by["page"].Status != "pending" || by["plan"].Status != "pending":
		return "in_production"
	default:
		return "planning"
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return itoa(n) + " " + many
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func isConflict(err error) bool {
	var ce *core.Error
	return errors.As(err, &ce) && ce.Kind == core.KindConflict
}

func isNotFound(err error) bool {
	var ce *core.Error
	return errors.As(err, &ce) && ce.Kind == core.KindNotFound
}

func fromRow(r store.Campaign) Campaign {
	out := Campaign{
		ID: r.ID, ProjectID: r.ProjectID, Name: r.Name, Goal: r.Goal, Audience: r.Audience, Avoid: r.Avoid,
		Currency: r.Currency, Channels: []string{}, Markets: []string{}, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.OfferID.Valid {
		id := r.OfferID.Int64
		out.OfferID = &id
	}
	if r.StartsOn.Valid {
		out.StartsOn = r.StartsOn.Time.Format("2006-01-02")
	}
	if r.EndsOn.Valid {
		out.EndsOn = r.EndsOn.Time.Format("2006-01-02")
	}
	if r.BudgetMinor.Valid {
		b := r.BudgetMinor.Int64
		out.BudgetMinor = &b
	}
	_ = json.Unmarshal([]byte(r.Channels), &out.Channels)
	for i, ch := range out.Channels {
		if l, ok := legacyChannels[ch]; ok {
			out.Channels[i] = l
		}
	}
	_ = json.Unmarshal([]byte(r.Markets), &out.Markets)
	return out
}
