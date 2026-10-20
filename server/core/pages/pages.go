// Package pages owns landing pages: structured documents rendered by the
// server, published as immutable versions, and the forms on them. A page is
// never raw HTML: what an editor types is data, escaped on the way out and
// served under a content security policy that allows no script, so nothing
// in a page can act with a visitor's or an editor's privileges. A form
// submission is scoped to the page version and project that received it;
// the team's test sends are marked and never become customer events.
package pages

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/store"
)

// Section is one block of the page. Kind decides which fields matter:
// heading and text use Text; bullets uses Items; image uses ImageKey (an
// engine asset) and Alt; hero and gallery use Pictures (studio files by
// SHA-256, the renders and the pictures Adx made) with Text as the caption
// or the line over the hero; stats uses Stats; plan uses Table, one row per
// instalment; location uses Text for the address and Link for the map;
// offer pulls the bound offer's approved terms at render time; form places
// the form.
type Section struct {
	Kind     string     `json:"kind"`
	Text     string     `json:"text"`
	Items    []string   `json:"items"`
	ImageKey string     `json:"imageKey"`
	Alt      string     `json:"alt"`
	Pictures []string   `json:"pictures"`
	Stats    []Stat     `json:"stats"`
	Table    [][]string `json:"table"`
	Link     string     `json:"link"`
}

// Stat is one figure on the page, such as "From AED 1.2M" under "Starting
// price". Its value is checked against the brand's approved facts.
type Stat struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type FormField struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"` // text, email, phone, select
	Required bool     `json:"required"`
	Options  []string `json:"options"`
}

type FormConfig struct {
	Fields         []FormField `json:"fields"`
	ConsentText    string      `json:"consentText"`
	SubmitLabel    string      `json:"submitLabel"`
	SuccessMessage string      `json:"successMessage"`
}

type PageDocument struct {
	Title        string      `json:"title"`
	Headline     string      `json:"headline"`
	Subheadline  string      `json:"subheadline"`
	Sections     []Section   `json:"sections"`
	OfferID      *int64      `json:"offerId"`
	OfferVersion *int64      `json:"offerVersion"`
	Form         *FormConfig `json:"form"`
	Footer       string      `json:"footer"`
	// TwinSlug is the same page in the other language, linked from the top.
	TwinSlug string `json:"twinSlug"`
}

type Page struct {
	ID          int64  `json:"id"`
	BrandID     int64  `json:"brandId"`
	ProjectID   *int64 `json:"projectId"`
	PlanID      *int64 `json:"planId"`
	PlanVersion *int64 `json:"planVersion"`
	// CampaignID is the campaign the page was made for, or nil.
	CampaignID *int64    `json:"campaignId"`
	Slug       string    `json:"slug"`
	Name       string    `json:"name"`
	Language   string    `json:"language"`
	CreatedBy  *int64    `json:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type PageVersion struct {
	ID        int64        `json:"id"`
	PageID    int64        `json:"pageId"`
	Version   int64        `json:"version"`
	Document  PageDocument `json:"document"`
	Note      string       `json:"note"`
	CreatedBy *int64       `json:"createdBy"`
	CreatedAt time.Time    `json:"createdAt"`
}

type Deployment struct {
	ID                    int64     `json:"id"`
	PageID                int64     `json:"pageId"`
	VersionID             int64     `json:"versionId"`
	Version               int64     `json:"version"`
	Adapter               string    `json:"adapter"`
	State                 string    `json:"state"`
	URL                   string    `json:"url"`
	OutputSHA256          string    `json:"outputSha256"`
	RolledBackFromID      *int64    `json:"rolledBackFromId"`
	RolledBackFromVersion *int64    `json:"rolledBackFromVersion"`
	Error                 string    `json:"error"`
	DeployedBy            *int64    `json:"deployedBy"`
	DeployedAt            time.Time `json:"deployedAt"`
}

type Submission struct {
	ID          int64             `json:"id"`
	PageID      int64             `json:"pageId"`
	VersionID   int64             `json:"versionId"`
	ProjectID   *int64            `json:"projectId"`
	State       string            `json:"state"`
	Reason      string            `json:"reason"`
	Test        bool              `json:"test"`
	Fields      map[string]string `json:"fields"`
	Attribution map[string]string `json:"attribution"`
	EvidenceID  *int64            `json:"evidenceId"`
	ReceivedAt  time.Time         `json:"receivedAt"`
}

type PageInput struct {
	BrandID     int64        `json:"brandId"`
	ProjectID   int64        `json:"projectId"`
	PlanID      int64        `json:"planId"`
	PlanVersion int64        `json:"planVersion"`
	CampaignID  int64        `json:"campaignId"`
	Slug        string       `json:"slug"`
	Name        string       `json:"name"`
	Language    string       `json:"language"`
	Document    PageDocument `json:"document"`
}

type PageVersionInput struct {
	Document PageDocument `json:"document"`
	Note     string       `json:"note"`
}

type PageDetail struct {
	Page        Page          `json:"page"`
	Versions    []PageVersion `json:"versions"`
	Deployments []Deployment  `json:"deployments"`
	Current     *Deployment   `json:"current"`
	PublicURL   string        `json:"publicUrl"`
	// TestToken lets the team send a test submission that is marked as
	// such. Only marketers and admins see it.
	TestToken string `json:"testToken"`
}

// Publisher puts a rendered page somewhere visitors reach it. ADX hosts
// pages itself (LocalPublisher). An adapter for an outside host (Wix,
// WordPress) would implement this and is not written: the hosts and their
// access are not known (decisions.md).
type Publisher interface {
	Name() string
	// Publish makes html the live page at slug and returns its URL.
	Publish(ctx context.Context, slug string, version int64, html []byte) (string, error)
}

// LocalPublisher serves pages from ADX at /p/{slug}. Publishing is the
// deployment row; the server renders the published version on request.
type LocalPublisher struct{ Base string }

func (LocalPublisher) Name() string { return "adx" }
func (p LocalPublisher) Publish(_ context.Context, slug string, _ int64, _ []byte) (string, error) {
	return strings.TrimRight(p.Base, "/") + "/p/" + slug, nil
}

// PublishingGate says which publishers exist.
type PublishingGate struct {
	Local    string                  `json:"local"`
	External ExternalPublishingState `json:"external"`
}

// ExternalPublishingState says whether an outside-host adapter exists.
type ExternalPublishingState struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	Unblocks  string `json:"unblocks"`
}

type Config struct {
	Clock     core.Clock
	Publisher Publisher
	// PublicBase is ADX's public origin, for page URLs.
	PublicBase string
	// RatePerMinute caps submissions per client address; DailyPerPage caps
	// a page's submissions per day. Defaults 10 and 500.
	RatePerMinute int
	DailyPerPage  int
	// Salt makes address hashes unlinkable across deployments.
	Salt string
}

type Pages struct {
	db        *store.DB
	clock     core.Clock
	brands    *brands.Brands
	evidence  *evidence.Store
	publisher Publisher
	base      string
	rate      *limiter
	daily     int
	salt      string
	model     assistant.Model
}

func New(db *store.DB, br *brands.Brands, ev *evidence.Store, cfg Config) *Pages {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	if cfg.RatePerMinute <= 0 {
		cfg.RatePerMinute = 10
	}
	if cfg.DailyPerPage <= 0 {
		cfg.DailyPerPage = 500
	}
	if cfg.Publisher == nil {
		cfg.Publisher = LocalPublisher{Base: cfg.PublicBase}
	}
	return &Pages{db: db, clock: cfg.Clock, brands: br, evidence: ev, publisher: cfg.Publisher, base: strings.TrimRight(cfg.PublicBase, "/"), rate: newLimiter(cfg.RatePerMinute, cfg.Clock), daily: cfg.DailyPerPage, salt: cfg.Salt}
}

func (p *Pages) Publishing() PublishingGate {
	g := PublishingGate{Local: p.publisher.Name()}
	if p.publisher.Name() != "adx" {
		g.External.Available = true
		g.External.Reason = "publisher " + p.publisher.Name()
		return g
	}
	g.External.Reason = "pages are served by ADX at /p/{slug}; no adapter for Reportage's Wix or WordPress sites exists"
	g.External.Unblocks = "which host each page family lives on, API or deploy access to it, and who owns the domain's DNS"
	return g
}

var (
	slugPattern  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	fieldPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)
	phonePattern = regexp.MustCompile(`^\+?[0-9][0-9 \-()]{6,19}$`)
)

// Create starts a page at version 1. Marketers may.
func (p *Pages) Create(ctx context.Context, actor core.Actor, in PageInput) (PageDetail, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return PageDetail{}, core.ErrForbidden
	}
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if !slugPattern.MatchString(in.Slug) || len(in.Slug) < 3 || len(in.Slug) > 60 {
		return PageDetail{}, core.Invalid("slug must be 3 to 60 lowercase letters, digits and hyphens")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 120 {
		return PageDetail{}, core.Invalid("a name of up to 120 characters is required")
	}
	if in.Language != "en" && in.Language != "ar" {
		return PageDetail{}, core.Invalid("language must be en or ar")
	}
	if in.BrandID == 0 {
		return PageDetail{}, core.Invalid("a brand is required")
	}
	if _, err := p.brands.Get(ctx, actor, in.BrandID); err != nil {
		return PageDetail{}, err
	}
	if in.ProjectID != 0 {
		owner, err := p.brands.BrandOfProject(ctx, actor, in.ProjectID)
		if err != nil {
			return PageDetail{}, err
		}
		if owner != in.BrandID {
			return PageDetail{}, core.Invalid("project %d does not belong to brand %d", in.ProjectID, in.BrandID)
		}
	}
	if in.PlanID != 0 {
		if _, err := p.db.Reader().GetPlan(ctx, store.GetPlanParams{ID: in.PlanID, WorkspaceID: actor.WorkspaceID}); err != nil {
			return PageDetail{}, core.NotFound("plan %d not found", in.PlanID)
		}
	}
	doc, err := p.validateDocument(ctx, actor, in.BrandID, in.Document)
	if err != nil {
		return PageDetail{}, err
	}
	raw, _ := json.Marshal(doc)
	now := p.clock.Now().UTC()
	token := make([]byte, 16)
	_, _ = rand.Read(token)
	var prow store.Page
	err = p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		prow, err = q.CreatePage(ctx, store.CreatePageParams{WorkspaceID: actor.WorkspaceID, BrandID: in.BrandID, ProjectID: optionalID(in.ProjectID), PlanID: optionalID(in.PlanID), PlanVersion: optionalID(in.PlanVersion), CampaignID: optionalID(in.CampaignID),
			Slug: in.Slug, Name: in.Name, Language: in.Language, TestToken: hex.EncodeToString(token), CreatedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		_, err = q.CreatePageVersion(ctx, store.CreatePageVersionParams{PageID: prow.ID, Version: 1, Document: string(raw), CreatedBy: optionalID(actor.UserID), CreatedAt: now})
		return err
	})
	if store.IsUniqueViolation(err) {
		return PageDetail{}, core.Conflict("a page with slug %q already exists", in.Slug)
	}
	if err != nil {
		return PageDetail{}, err
	}
	return p.Get(ctx, actor, prow.ID)
}

// SaveVersion adds the next version. Marketers may. Published versions are
// untouched: publishing the new one is a separate step.
func (p *Pages) SaveVersion(ctx context.Context, actor core.Actor, pageID int64, in PageVersionInput) (PageVersion, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return PageVersion{}, core.ErrForbidden
	}
	prow, err := p.page(ctx, actor.WorkspaceID, pageID)
	if err != nil {
		return PageVersion{}, err
	}
	doc, err := p.validateDocument(ctx, actor, prow.BrandID, in.Document)
	if err != nil {
		return PageVersion{}, err
	}
	raw, _ := json.Marshal(doc)
	now := p.clock.Now().UTC()
	var vrow store.PageVersion
	err = p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		latest, err := q.GetLatestPageVersion(ctx, pageID)
		if err != nil {
			return err
		}
		vrow, err = q.CreatePageVersion(ctx, store.CreatePageVersionParams{PageID: pageID, Version: latest.Version + 1, Document: string(raw), Note: strings.TrimSpace(in.Note), CreatedBy: optionalID(actor.UserID), CreatedAt: now})
		if err != nil {
			return err
		}
		return q.TouchPage(ctx, store.TouchPageParams{UpdatedAt: now, ID: pageID})
	})
	if store.IsUniqueViolation(err) {
		return PageVersion{}, core.Conflict("someone saved this page at the same time; reload and try again")
	}
	if err != nil {
		return PageVersion{}, err
	}
	return versionFromRow(vrow)
}

func (p *Pages) validateDocument(ctx context.Context, actor core.Actor, brandID int64, d PageDocument) (PageDocument, error) {
	d.Title, d.Headline, d.Subheadline, d.Footer = strings.TrimSpace(d.Title), strings.TrimSpace(d.Headline), strings.TrimSpace(d.Subheadline), strings.TrimSpace(d.Footer)
	if d.Title == "" || utf8.RuneCountInString(d.Title) > 160 {
		return d, core.Invalid("a title of up to 160 characters is required")
	}
	if d.Sections == nil {
		d.Sections = []Section{}
	}
	formSections := 0
	for i := range d.Sections {
		s := &d.Sections[i]
		switch s.Kind {
		case "heading", "text":
			if strings.TrimSpace(s.Text) == "" {
				return d, core.Invalid("section %d has no text", i+1)
			}
		case "bullets":
			items := []string{}
			for _, it := range s.Items {
				if it = strings.TrimSpace(it); it != "" {
					items = append(items, it)
				}
			}
			if len(items) == 0 {
				return d, core.Invalid("section %d has no bullet points", i+1)
			}
			s.Items = items
		case "image":
			if _, err := p.db.Reader().GetAssetByKey(ctx, store.GetAssetByKeyParams{WorkspaceID: actor.WorkspaceID, Key: s.ImageKey}); err != nil {
				return d, core.Invalid("section %d points at a file this workspace doesn't have", i+1)
			}
		case "hero", "gallery":
			if len(s.Pictures) == 0 || (s.Kind == "hero" && len(s.Pictures) > 1) || len(s.Pictures) > 12 {
				return d, core.Invalid("section %d: a hero has one picture, a gallery one to twelve", i+1)
			}
			for _, sum := range s.Pictures {
				f, err := p.db.Reader().GetFileBySHA(ctx, sum)
				if err != nil || !strings.HasPrefix(f.ContentType, "image/") {
					return d, core.Invalid("section %d points at a picture the studio doesn't have", i+1)
				}
			}
		case "stats":
			stats := []Stat{}
			for _, st := range s.Stats {
				st.Label, st.Value = strings.TrimSpace(st.Label), strings.TrimSpace(st.Value)
				if st.Value != "" {
					stats = append(stats, st)
				}
			}
			if len(stats) == 0 || len(stats) > 6 {
				return d, core.Invalid("section %d has between one and six figures", i+1)
			}
			s.Stats = stats
		case "plan":
			rows := [][]string{}
			for _, row := range s.Table {
				cells := []string{}
				for _, c := range row {
					if c = strings.TrimSpace(c); c != "" {
						cells = append(cells, c)
					}
				}
				if len(cells) > 0 {
					rows = append(rows, cells)
				}
			}
			if len(rows) == 0 || len(rows) > 12 {
				return d, core.Invalid("section %d: a payment plan has one to twelve rows", i+1)
			}
			s.Table = rows
		case "location":
			if strings.TrimSpace(s.Text) == "" {
				return d, core.Invalid("section %d has no address", i+1)
			}
			if s.Link != "" && !strings.HasPrefix(s.Link, "https://") {
				return d, core.Invalid("section %d: the map link must start with https://", i+1)
			}
		case "offer":
			if d.OfferID == nil || *d.OfferID == 0 {
				return d, core.Invalid("section %d shows an offer but the page is bound to none", i+1)
			}
		case "form":
			formSections++
			if d.Form == nil {
				return d, core.Invalid("section %d places a form but the page has no form", i+1)
			}
		default:
			return d, core.Invalid("section %d has unknown kind %q", i+1, s.Kind)
		}
		if s.Items == nil {
			s.Items = []string{}
		}
		if s.Pictures == nil {
			s.Pictures = []string{}
		}
		if s.Stats == nil {
			s.Stats = []Stat{}
		}
		if s.Table == nil {
			s.Table = [][]string{}
		}
	}
	if formSections > 1 {
		return d, core.Invalid("a page has at most one form")
	}
	if d.OfferID != nil && *d.OfferID != 0 {
		o, versions, err := p.brands.OfferVersions(ctx, actor, *d.OfferID)
		if err != nil {
			return d, err
		}
		if o.BrandID != brandID {
			return d, core.Invalid("offer %d belongs to another brand", *d.OfferID)
		}
		if d.OfferVersion == nil || *d.OfferVersion == 0 {
			v := versions[len(versions)-1].Version
			d.OfferVersion = &v
		}
	} else {
		d.OfferID, d.OfferVersion = nil, nil
	}
	if d.Form != nil {
		f := d.Form
		if len(f.Fields) == 0 || len(f.Fields) > 12 {
			return d, core.Invalid("a form has between 1 and 12 fields")
		}
		seen := map[string]bool{}
		for i := range f.Fields {
			fl := &f.Fields[i]
			fl.Name = strings.ToLower(strings.TrimSpace(fl.Name))
			fl.Label = strings.TrimSpace(fl.Label)
			if !fieldPattern.MatchString(fl.Name) || fl.Name == "website" || strings.HasPrefix(fl.Name, "_") {
				return d, core.Invalid("field %d needs a name like first_name", i+1)
			}
			if seen[fl.Name] {
				return d, core.Invalid("field %q appears twice", fl.Name)
			}
			seen[fl.Name] = true
			if fl.Label == "" {
				fl.Label = fl.Name
			}
			switch fl.Kind {
			case "text", "email", "phone":
			case "select":
				if len(fl.Options) == 0 {
					return d, core.Invalid("field %q needs options", fl.Name)
				}
			default:
				return d, core.Invalid("field %q has unknown kind %q", fl.Name, fl.Kind)
			}
			if fl.Options == nil {
				fl.Options = []string{}
			}
		}
		f.ConsentText, f.SubmitLabel, f.SuccessMessage = strings.TrimSpace(f.ConsentText), strings.TrimSpace(f.SubmitLabel), strings.TrimSpace(f.SuccessMessage)
		if f.SubmitLabel == "" {
			f.SubmitLabel = "Register"
		}
		if f.SuccessMessage == "" {
			f.SuccessMessage = "Thank you. We will be in touch."
		}
	}
	return d, nil
}

func (p *Pages) List(ctx context.Context, actor core.Actor) ([]Page, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := p.db.Reader().ListPages(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]Page, 0, len(rows))
	for _, r := range rows {
		out = append(out, pageFromRow(r))
	}
	return out, nil
}

// PageSummary is a page as the list shows it: whose it is, where it stands
// and where it's served. PublishedVersion is nil while nothing is live.
type PageSummary struct {
	Page
	BrandName        string `json:"brandName"`
	ProjectName      string `json:"projectName"`
	LatestVersion    int64  `json:"latestVersion"`
	PublishedVersion *int64 `json:"publishedVersion"`
	PublicURL        string `json:"publicUrl"`
}

// ListSummaries lists the workspace's pages with their brand, latest and
// published versions, newest first.
func (p *Pages) ListSummaries(ctx context.Context, actor core.Actor) ([]PageSummary, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := p.db.Reader().ListPageSummaries(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]PageSummary, 0, len(rows))
	for _, r := range rows {
		s := PageSummary{
			Page:      pageFromRow(store.Page{ID: r.ID, WorkspaceID: r.WorkspaceID, BrandID: r.BrandID, ProjectID: r.ProjectID, PlanID: r.PlanID, PlanVersion: r.PlanVersion, Slug: r.Slug, Name: r.Name, Language: r.Language, TestToken: r.TestToken, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}),
			BrandName: r.BrandName, ProjectName: r.ProjectName.String, LatestVersion: r.LatestVersion, PublicURL: p.base + "/p/" + r.Slug,
		}
		if r.PublishedVersion > 0 {
			v := r.PublishedVersion
			s.PublishedVersion = &v
		}
		out = append(out, s)
	}
	return out, nil
}

func (p *Pages) Get(ctx context.Context, actor core.Actor, id int64) (PageDetail, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return PageDetail{}, core.ErrForbidden
	}
	prow, err := p.page(ctx, actor.WorkspaceID, id)
	if err != nil {
		return PageDetail{}, err
	}
	d := PageDetail{Page: pageFromRow(prow), Versions: []PageVersion{}, Deployments: []Deployment{}, PublicURL: p.base + "/p/" + prow.Slug}
	if actor.AtLeast(core.RoleMarketer) {
		d.TestToken = prow.TestToken
	}
	vrows, err := p.db.Reader().ListPageVersions(ctx, id)
	if err != nil {
		return PageDetail{}, err
	}
	byID := map[int64]int64{}
	for _, v := range vrows {
		pv, err := versionFromRow(v)
		if err != nil {
			return PageDetail{}, err
		}
		d.Versions = append(d.Versions, pv)
		byID[v.ID] = v.Version
	}
	drows, err := p.db.Reader().ListPageDeployments(ctx, id)
	if err != nil {
		return PageDetail{}, err
	}
	depByID := map[int64]store.PageDeployment{}
	for _, r := range drows {
		depByID[r.ID] = r
	}
	for _, r := range drows {
		dep := deploymentFromRow(r, byID)
		if r.RolledBackFromID.Valid {
			if from, ok := depByID[r.RolledBackFromID.Int64]; ok {
				v := byID[from.VersionID]
				dep.RolledBackFromVersion = &v
			}
		}
		d.Deployments = append(d.Deployments, dep)
		if r.State == "published" && d.Current == nil {
			cur := dep
			d.Current = &cur
		}
	}
	return d, nil
}

// Render produces the HTML of one version for preview. Everything typed by
// an editor passes through html/template escaping; the offer section pulls
// the bound offer version's terms only when that version is approved.
func (p *Pages) Render(ctx context.Context, actor core.Actor, pageID, version int64) ([]byte, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	prow, err := p.page(ctx, actor.WorkspaceID, pageID)
	if err != nil {
		return nil, err
	}
	vrow, err := p.db.Reader().GetPageVersion(ctx, store.GetPageVersionParams{PageID: pageID, Version: version})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.NotFound("page %d has no version %d", pageID, version)
	}
	if err != nil {
		return nil, err
	}
	return p.render(ctx, actor, prow, vrow)
}

// ServePublished renders the live version of a page for a visitor. No
// actor: the page is public. Unpublished pages are not found.
func (p *Pages) ServePublished(ctx context.Context, slug string) ([]byte, error) {
	prow, err := p.db.Reader().GetPageBySlug(ctx, strings.ToLower(slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.NotFound("no such page")
	}
	if err != nil {
		return nil, err
	}
	dep, err := p.db.Reader().GetCurrentPageDeployment(ctx, prow.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.NotFound("no such page")
	}
	if err != nil {
		return nil, err
	}
	vrow, err := p.db.Reader().GetPageVersionByID(ctx, dep.VersionID)
	if err != nil {
		return nil, err
	}
	return p.render(ctx, core.SystemIn(core.ViaWeb, prow.WorkspaceID), prow, vrow)
}

type offerView struct {
	Headline   string
	Terms      string
	Conditions []string
	Pending    bool
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="{{.Lang}}" dir="{{.Dir}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Doc.Title}}</title>
<style>
body{margin:0;font-family:Montserrat,Almarai,Cairo,system-ui,sans-serif;background:#000;color:#fff;line-height:1.5}
main{max-width:720px;margin:0 auto;padding:32px 20px}
h1{font-size:2rem;margin:0 0 .5rem}h2{font-size:1.25rem;margin:1.5rem 0 .5rem}
p{margin:.5rem 0}ul{padding-inline-start:1.25rem}
img{max-width:100%;display:block;border-radius:8px}
.hero{position:relative;margin:-32px -20px 1.5rem;min-height:60vh;display:flex;align-items:flex-end}
.hero img{width:100%;height:60vh;object-fit:cover;border-radius:0}
.hero p{position:absolute;inset-inline:20px;bottom:24px;margin:0;font-size:1.25rem;font-weight:600;text-shadow:0 1px 8px rgba(0,0,0,.6)}
.gallery{display:grid;grid-template-columns:repeat(auto-fill,minmax(160px,1fr));gap:8px;margin:1rem 0}
.gallery img{aspect-ratio:4/3;object-fit:cover}
.stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:12px;margin:1rem 0}
.stats div{border:1px solid #444;border-radius:8px;padding:12px}.stats strong{display:block;font-size:1.5rem}.stats span{color:#bbb;font-size:.85rem}
table{width:100%;border-collapse:collapse;margin:1rem 0}td{padding:8px 4px;border-bottom:1px solid #333}
.twin{position:absolute;top:12px;inset-inline-end:16px;color:#fff;font-size:.85rem;z-index:1}
.location a{color:#fff}
form{display:grid;gap:12px;margin-top:1rem}label{display:grid;gap:4px;font-size:.9rem}
input,select{padding:10px;border-radius:6px;border:1px solid #444;background:#111;color:#fff;font:inherit}
button{padding:12px;border:0;border-radius:6px;background:#fff;color:#000;font:inherit;font-weight:600}
.offer{border:1px solid #444;border-radius:8px;padding:16px;margin:1rem 0}
.hp{position:absolute;left:-10000px}footer{color:#999;font-size:.8rem;margin-top:2rem}
</style>
</head>
<body>
<main>
{{if .Doc.TwinSlug}}<a class="twin" href="/p/{{.Doc.TwinSlug}}">{{if eq .Lang "ar"}}English{{else}}العربية{{end}}</a>{{end}}
{{range .Doc.Sections}}{{if eq .Kind "hero"}}<div class="hero"><img src="/api/files/{{index .Pictures 0}}" alt="{{.Alt}}">{{if .Text}}<p>{{.Text}}</p>{{end}}</div>{{end}}{{end}}
<h1>{{.Doc.Headline}}</h1>
{{if .Doc.Subheadline}}<p>{{.Doc.Subheadline}}</p>{{end}}
{{range .Doc.Sections}}
{{if eq .Kind "heading"}}<h2>{{.Text}}</h2>
{{else if eq .Kind "text"}}<p>{{.Text}}</p>
{{else if eq .Kind "bullets"}}<ul>{{range .Items}}<li>{{.}}</li>{{end}}</ul>
{{else if eq .Kind "image"}}<img src="/api/assets/public/{{.ImageKey}}" alt="{{.Alt}}">
{{else if eq .Kind "hero"}}
{{else if eq .Kind "gallery"}}<div class="gallery">{{range .Pictures}}<img src="/api/files/{{.}}" alt="" loading="lazy">{{end}}</div>{{if .Text}}<p>{{.Text}}</p>{{end}}
{{else if eq .Kind "stats"}}<div class="stats">{{range .Stats}}<div><strong>{{.Value}}</strong><span>{{.Label}}</span></div>{{end}}</div>
{{else if eq .Kind "plan"}}{{if .Text}}<h2>{{.Text}}</h2>{{end}}<table>{{range .Table}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</table>
{{else if eq .Kind "location"}}<div class="location"><p>{{.Text}}</p>{{if .Link}}<p><a href="{{.Link}}" rel="noopener" target="_blank">{{if eq $.Lang "ar"}}افتح الخريطة{{else}}Open in Maps{{end}}</a></p>{{end}}</div>
{{else if eq .Kind "offer"}}<div class="offer">{{if $.Offer.Pending}}<p>Offer details pending approval.</p>{{else}}<p><strong>{{$.Offer.Headline}}</strong></p>{{if $.Offer.Terms}}<p>{{$.Offer.Terms}}</p>{{end}}{{if $.Offer.Conditions}}<ul>{{range $.Offer.Conditions}}<li>{{.}}</li>{{end}}</ul>{{end}}{{end}}</div>
{{else if eq .Kind "form"}}{{if $.Doc.Form}}
<form method="post" action="/p/{{$.Slug}}/form">
<input type="hidden" name="_project" value="{{$.ProjectID}}">
<input type="hidden" name="_version" value="{{$.VersionID}}">
<label class="hp" aria-hidden="true">Website<input type="text" name="website" tabindex="-1" autocomplete="off"></label>
{{range $.Doc.Form.Fields}}<label>{{.Label}}{{if .Required}} *{{end}}
{{if eq .Kind "select"}}<select name="{{.Name}}"{{if .Required}} required{{end}}>{{range .Options}}<option>{{.}}</option>{{end}}</select>
{{else}}<input type="{{if eq .Kind "email"}}email{{else if eq .Kind "phone"}}tel{{else}}text{{end}}" name="{{.Name}}"{{if .Required}} required{{end}}>{{end}}
</label>{{end}}
{{if $.Doc.Form.ConsentText}}<p>{{$.Doc.Form.ConsentText}}</p>{{end}}
<button type="submit">{{$.Doc.Form.SubmitLabel}}</button>
</form>{{end}}
{{end}}
{{end}}
{{if .Doc.Footer}}<footer>{{.Doc.Footer}}</footer>{{end}}
</main>
</body>
</html>
`))

var thanksTemplate = template.Must(template.New("thanks").Parse(`<!doctype html><html lang="{{.Lang}}" dir="{{.Dir}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>{{.Title}}</title><style>body{margin:0;font-family:system-ui,sans-serif;background:#000;color:#fff}main{max-width:720px;margin:0 auto;padding:32px 20px}</style></head><body><main><h1>{{.Message}}</h1>{{if .Test}}<p>This was a test submission and is marked as such.</p>{{end}}</main></body></html>`))

func (p *Pages) render(ctx context.Context, actor core.Actor, prow store.Page, vrow store.PageVersion) ([]byte, error) {
	var doc PageDocument
	if err := json.Unmarshal([]byte(vrow.Document), &doc); err != nil {
		return nil, err
	}
	ov := offerView{}
	if doc.OfferID != nil && *doc.OfferID != 0 {
		ov.Pending = true
		if _, versions, err := p.brands.OfferVersions(ctx, actor, *doc.OfferID); err == nil {
			for _, v := range versions {
				if doc.OfferVersion != nil && v.Version == *doc.OfferVersion && v.State == brands.OfferApproved {
					ov = offerView{Headline: v.Document.Headline, Terms: v.Document.Terms, Conditions: v.Document.Conditions}
					if prow.Language == "ar" && v.Document.HeadlineAr != "" {
						ov.Headline, ov.Terms = v.Document.HeadlineAr, v.Document.TermsAr
					}
				}
			}
		}
	}
	dir := "ltr"
	if prow.Language == "ar" {
		dir = "rtl"
	}
	var buf bytes.Buffer
	err := pageTemplate.Execute(&buf, map[string]any{"Doc": doc, "Lang": prow.Language, "Dir": dir, "Slug": prow.Slug, "ProjectID": prow.ProjectID.Int64, "VersionID": vrow.ID, "Offer": ov})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Publish makes a version the live one. Marketers may. The deployment row
// names the exact version and the rendered output's checksum.
func (p *Pages) Publish(ctx context.Context, actor core.Actor, pageID, version int64) (Deployment, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Deployment{}, core.ErrForbidden
	}
	prow, err := p.page(ctx, actor.WorkspaceID, pageID)
	if err != nil {
		return Deployment{}, err
	}
	vrow, err := p.db.Reader().GetPageVersion(ctx, store.GetPageVersionParams{PageID: pageID, Version: version})
	if errors.Is(err, sql.ErrNoRows) {
		return Deployment{}, core.NotFound("page %d has no version %d", pageID, version)
	}
	if err != nil {
		return Deployment{}, err
	}
	return p.deploy(ctx, actor, prow, vrow, 0)
}

// Rollback republishes the version that was live before the current one.
// The new deployment names the deployment it rolled back from, so the exact
// old version is on record.
func (p *Pages) Rollback(ctx context.Context, actor core.Actor, pageID int64) (Deployment, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Deployment{}, core.ErrForbidden
	}
	prow, err := p.page(ctx, actor.WorkspaceID, pageID)
	if err != nil {
		return Deployment{}, err
	}
	deps, err := p.db.Reader().ListPageDeployments(ctx, pageID)
	if err != nil {
		return Deployment{}, err
	}
	var current *store.PageDeployment
	for i := range deps {
		if deps[i].State == "published" {
			current = &deps[i]
			break
		}
	}
	if current == nil {
		return Deployment{}, core.Conflict("page %d is not published; nothing to roll back", pageID)
	}
	var previous *store.PageDeployment
	for i := range deps {
		d := &deps[i]
		if d.ID < current.ID && d.VersionID != current.VersionID && d.State != "failed" {
			previous = d
			break
		}
	}
	if previous == nil {
		return Deployment{}, core.Conflict("no earlier published version to roll back to")
	}
	vrow, err := p.db.Reader().GetPageVersionByID(ctx, previous.VersionID)
	if err != nil {
		return Deployment{}, err
	}
	return p.deploy(ctx, actor, prow, vrow, current.ID)
}

func (p *Pages) deploy(ctx context.Context, actor core.Actor, prow store.Page, vrow store.PageVersion, rolledBackFrom int64) (Deployment, error) {
	html, err := p.render(ctx, actor, prow, vrow)
	if err != nil {
		return Deployment{}, err
	}
	sum := sha256.Sum256(html)
	now := p.clock.Now().UTC()
	url, perr := p.publisher.Publish(ctx, prow.Slug, vrow.Version, html)
	state, safeErr := "published", ""
	if perr != nil {
		state, safeErr = "failed", "the publisher did not accept the page"
	}
	var row store.PageDeployment
	err = p.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		row, err = q.CreatePageDeployment(ctx, store.CreatePageDeploymentParams{PageID: prow.ID, VersionID: vrow.ID, Adapter: p.publisher.Name(), State: state, Url: url, OutputSha256: hex.EncodeToString(sum[:]),
			RolledBackFromID: optionalID(rolledBackFrom), ErrorSafe: safeErr, DeployedBy: optionalID(actor.UserID), DeployedAt: now})
		if err != nil {
			return err
		}
		if state == "published" {
			if err := q.SupersedePageDeployments(ctx, store.SupersedePageDeploymentsParams{PageID: prow.ID, ID: row.ID}); err != nil {
				return err
			}
		}
		return q.TouchPage(ctx, store.TouchPageParams{UpdatedAt: now, ID: prow.ID})
	})
	if err != nil {
		return Deployment{}, err
	}
	dep := deploymentFromRow(row, map[int64]int64{vrow.ID: vrow.Version})
	if rolledBackFrom != 0 {
		if from, err := p.db.Reader().GetPageVersionByID(ctx, mustDeployment(ctx, p, rolledBackFrom).VersionID); err == nil {
			dep.RolledBackFromVersion = &from.Version
		}
	}
	if perr != nil {
		return dep, core.Unavailable(perr, "publishing failed; the deployment is recorded as failed")
	}
	return dep, nil
}

func mustDeployment(ctx context.Context, p *Pages, id int64) store.PageDeployment {
	rows, _ := p.db.Reader().ListPageDeployments(ctx, 0)
	_ = rows
	var out store.PageDeployment
	_ = p.db.Read.QueryRowContext(ctx, `SELECT version_id FROM page_deployments WHERE id = ?`, id).Scan(&out.VersionID)
	return out
}

// Forms.

type SubmissionInput struct {
	Slug      string
	Fields    map[string]string
	ProjectID int64
	VersionID int64
	TestToken string
	Honeypot  string
	RemoteIP  string
	UserAgent string
	// Attribution is what the page's URL carried: utm_*, gclid, fbclid,
	// referrer, landing.
	Attribution map[string]string
}

// SubmitError is a refusal the visitor sees. Reason is the stored code.
type SubmitError struct {
	Reason  string
	Message string
	Status  int
}

func (e *SubmitError) Error() string { return e.Message }

// Submit handles a visitor's form post. It is scoped to the published
// version of the page named by slug and to that page's project; a post
// naming another project is rejected and recorded as such. Rate limits
// apply per client address and per page per day. A valid test token marks
// the submission as the team's test.
func (p *Pages) Submit(ctx context.Context, in SubmissionInput) (Submission, error) {
	prow, err := p.db.Reader().GetPageBySlug(ctx, strings.ToLower(in.Slug))
	if errors.Is(err, sql.ErrNoRows) {
		return Submission{}, &SubmitError{Reason: "no_page", Message: "This page does not exist.", Status: 404}
	}
	if err != nil {
		return Submission{}, err
	}
	dep, err := p.db.Reader().GetCurrentPageDeployment(ctx, prow.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return Submission{}, &SubmitError{Reason: "not_published", Message: "This page is not live.", Status: 404}
	}
	if err != nil {
		return Submission{}, err
	}
	vrow, err := p.db.Reader().GetPageVersionByID(ctx, dep.VersionID)
	if err != nil {
		return Submission{}, err
	}
	var doc PageDocument
	_ = json.Unmarshal([]byte(vrow.Document), &doc)
	actor := core.SystemIn(core.ViaWeb, prow.WorkspaceID)
	now := p.clock.Now().UTC()
	ipHash := p.hash(in.RemoteIP)
	isTest := in.TestToken != "" && in.TestToken == prow.TestToken
	reject := func(reason, message string, status int) (Submission, error) {
		row, rerr := p.db.Writer().CreateFormSubmission(ctx, store.CreateFormSubmissionParams{WorkspaceID: prow.WorkspaceID, PageID: prow.ID, VersionID: vrow.ID, ProjectID: prow.ProjectID, State: "rejected", Reason: reason, Test: boolInt(isTest),
			Fields: mustJSON(in.Fields), Attribution: mustJSON(in.Attribution), IpHash: ipHash, UserAgent: truncate(in.UserAgent, 200), ReceivedAt: now})
		if rerr != nil {
			return Submission{}, rerr
		}
		return submissionFromRow(row), &SubmitError{Reason: reason, Message: message, Status: status}
	}
	if doc.Form == nil {
		return reject("no_form", "This page takes no submissions.", 400)
	}
	if in.Honeypot != "" {
		return reject("honeypot", "Thank you.", 200)
	}
	if !isTest {
		if !p.rate.allow(ipHash) {
			return reject("rate_limited", "Too many submissions; please try again in a minute.", 429)
		}
		n, err := p.db.Reader().CountFormSubmissionsSince(ctx, store.CountFormSubmissionsSinceParams{PageID: prow.ID, ReceivedAt: now.Add(-24 * time.Hour)})
		if err != nil {
			return Submission{}, err
		}
		if int(n) >= p.daily {
			return reject("daily_cap", "This form has reached its daily limit; please try again tomorrow.", 429)
		}
	}
	if in.ProjectID != prow.ProjectID.Int64 {
		return reject("wrong_project", "This form belongs to another project.", 400)
	}
	if in.VersionID != 0 && in.VersionID != vrow.ID {
		return reject("stale_version", "The page has changed; please reload and try again.", 409)
	}
	clean := map[string]string{}
	for _, f := range doc.Form.Fields {
		v := strings.TrimSpace(in.Fields[f.Name])
		if v == "" {
			if f.Required {
				return reject("missing:"+f.Name, f.Label+" is required.", 400)
			}
			continue
		}
		if utf8.RuneCountInString(v) > 500 {
			return reject("too_long:"+f.Name, f.Label+" is too long.", 400)
		}
		switch f.Kind {
		case "email":
			if _, err := mail.ParseAddress(v); err != nil {
				return reject("invalid:"+f.Name, f.Label+" is not a valid email address.", 400)
			}
		case "phone":
			if !phonePattern.MatchString(v) {
				return reject("invalid:"+f.Name, f.Label+" is not a valid phone number.", 400)
			}
		case "select":
			ok := false
			for _, o := range f.Options {
				if o == v {
					ok = true
				}
			}
			if !ok {
				return reject("invalid:"+f.Name, f.Label+" has an unknown choice.", 400)
			}
		}
		clean[f.Name] = v
	}
	attribution := map[string]string{}
	for k, v := range in.Attribution {
		k = strings.ToLower(strings.TrimSpace(k))
		if v = strings.TrimSpace(v); v != "" && (strings.HasPrefix(k, "utm_") || k == "gclid" || k == "gbraid" || k == "wbraid" || k == "fbclid" || k == "referrer" || k == "landing") {
			attribution[k] = truncate(v, 500)
		}
	}
	var evidenceID sql.NullInt64
	if p.evidence != nil {
		nonce := make([]byte, 8)
		_, _ = rand.Read(nonce)
		ev, _, err := p.evidence.Record(ctx, actor, evidence.Observation{Source: "web_form", Kind: "submission", SourceRef: fmt.Sprintf("page:%d/%s", prow.ID, hex.EncodeToString(nonce)), Scope: fmt.Sprintf("page:%d/project:%d", prow.ID, prow.ProjectID.Int64),
			Class: evidence.Observed, Payload: map[string]any{"fields": clean, "attribution": attribution, "test": isTest, "pageVersion": vrow.Version}, Completeness: evidence.Complete})
		if err != nil {
			return Submission{}, err
		}
		evidenceID = sql.NullInt64{Int64: ev.ID, Valid: true}
	}
	row, err := p.db.Writer().CreateFormSubmission(ctx, store.CreateFormSubmissionParams{WorkspaceID: prow.WorkspaceID, PageID: prow.ID, VersionID: vrow.ID, ProjectID: prow.ProjectID, State: "received", Test: boolInt(isTest),
		Fields: mustJSON(clean), Attribution: mustJSON(attribution), IpHash: ipHash, UserAgent: truncate(in.UserAgent, 200), EvidenceID: evidenceID, ReceivedAt: now})
	if err != nil {
		return Submission{}, err
	}
	return submissionFromRow(row), nil
}

// ThankYou renders the confirmation for a submission.
func (p *Pages) ThankYou(ctx context.Context, slug string, test bool) ([]byte, error) {
	prow, err := p.db.Reader().GetPageBySlug(ctx, strings.ToLower(slug))
	if err != nil {
		return nil, core.NotFound("no such page")
	}
	dep, err := p.db.Reader().GetCurrentPageDeployment(ctx, prow.ID)
	if err != nil {
		return nil, core.NotFound("no such page")
	}
	vrow, err := p.db.Reader().GetPageVersionByID(ctx, dep.VersionID)
	if err != nil {
		return nil, err
	}
	var doc PageDocument
	_ = json.Unmarshal([]byte(vrow.Document), &doc)
	msg := "Thank you."
	if doc.Form != nil && doc.Form.SuccessMessage != "" {
		msg = doc.Form.SuccessMessage
	}
	dir := "ltr"
	if prow.Language == "ar" {
		dir = "rtl"
	}
	var buf bytes.Buffer
	if err := thanksTemplate.Execute(&buf, map[string]any{"Lang": prow.Language, "Dir": dir, "Title": doc.Title, "Message": msg, "Test": test}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (p *Pages) Submissions(ctx context.Context, actor core.Actor, pageID int64, limit int) ([]Submission, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if _, err := p.page(ctx, actor.WorkspaceID, pageID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := p.db.Reader().ListFormSubmissions(ctx, store.ListFormSubmissionsParams{PageID: pageID, Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]Submission, 0, len(rows))
	for _, r := range rows {
		out = append(out, submissionFromRow(r))
	}
	return out, nil
}

// Readiness.

type PageReadinessIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PageReadiness struct {
	PageID           int64                `json:"pageId"`
	PublishedVersion *int64               `json:"publishedVersion"`
	LatestVersion    int64                `json:"latestVersion"`
	Ready            bool                 `json:"ready"`
	Blockers         []PageReadinessIssue `json:"blockers"`
	Warnings         []PageReadinessIssue `json:"warnings"`
	Claims           []brands.ClaimCheck  `json:"claims"`
	LastTest         *Submission          `json:"lastTest"`
}

// Readiness says whether the live page may be put forward for launch: it
// is published, its form can take an enquiry and has been exercised with a
// test submission on this very version, its offer is approved and current,
// and every figure in its copy is an approved claim.
func (p *Pages) Readiness(ctx context.Context, actor core.Actor, pageID int64) (PageReadiness, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return PageReadiness{}, core.ErrForbidden
	}
	prow, err := p.page(ctx, actor.WorkspaceID, pageID)
	if err != nil {
		return PageReadiness{}, err
	}
	r := PageReadiness{PageID: pageID, Blockers: []PageReadinessIssue{}, Warnings: []PageReadinessIssue{}, Claims: []brands.ClaimCheck{}}
	block := func(code, msg string) { r.Blockers = append(r.Blockers, PageReadinessIssue{code, msg}) }
	warn := func(code, msg string) { r.Warnings = append(r.Warnings, PageReadinessIssue{code, msg}) }
	latest, err := p.db.Reader().GetLatestPageVersion(ctx, pageID)
	if err != nil {
		return PageReadiness{}, err
	}
	r.LatestVersion = latest.Version
	dep, err := p.db.Reader().GetCurrentPageDeployment(ctx, pageID)
	if errors.Is(err, sql.ErrNoRows) {
		block("not_published", "the page is not published")
		r.Ready = false
		return r, nil
	}
	if err != nil {
		return PageReadiness{}, err
	}
	vrow, err := p.db.Reader().GetPageVersionByID(ctx, dep.VersionID)
	if err != nil {
		return PageReadiness{}, err
	}
	r.PublishedVersion = &vrow.Version
	if vrow.Version != latest.Version {
		warn("newer_draft", fmt.Sprintf("version %d is live; version %d is a newer draft", vrow.Version, latest.Version))
	}
	var doc PageDocument
	_ = json.Unmarshal([]byte(vrow.Document), &doc)
	if doc.Form == nil {
		block("form_missing", "the page has no form, so it cannot take an enquiry")
	} else {
		contact := false
		placed := false
		for _, f := range doc.Form.Fields {
			if f.Required && (f.Kind == "email" || f.Kind == "phone") {
				contact = true
			}
		}
		for _, s := range doc.Sections {
			if s.Kind == "form" {
				placed = true
			}
		}
		if !contact {
			block("form_contact_missing", "the form has no required email or phone field; a submission could not reach anyone")
		}
		if !placed {
			block("form_not_placed", "the form is defined but no section places it on the page")
		}
		if doc.Form.ConsentText == "" {
			warn("consent_missing", "the form has no consent text")
		}
		test, err := p.db.Reader().LatestTestSubmissionForVersion(ctx, vrow.ID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			block("untested_form", fmt.Sprintf("no test submission has been received on version %d; send one from the page", vrow.Version))
		case err != nil:
			return PageReadiness{}, err
		default:
			s := submissionFromRow(test)
			r.LastTest = &s
			if s.State != "received" {
				block("test_failed", fmt.Sprintf("the last test submission on version %d was rejected: %s", vrow.Version, s.Reason))
			}
		}
	}
	text := strings.Join([]string{doc.Title, doc.Headline, doc.Subheadline, doc.Footer}, " ")
	for _, s := range doc.Sections {
		text += " " + s.Text + " " + strings.Join(s.Items, " ")
		for _, st := range s.Stats {
			text += " " + st.Label + " " + st.Value
		}
		for _, row := range s.Table {
			text += " " + strings.Join(row, " ")
		}
	}
	var offerID, offerVersion int64
	if doc.OfferID != nil {
		offerID = *doc.OfferID
	}
	if doc.OfferVersion != nil {
		offerVersion = *doc.OfferVersion
	}
	report, err := p.brands.CheckCopy(ctx, actor, prow.BrandID, prow.ProjectID.Int64, offerID, offerVersion, text)
	if err != nil {
		return PageReadiness{}, err
	}
	r.Claims = report.Claims
	for _, c := range report.Claims {
		if c.Status != "approved" {
			block("unapproved_claim", fmt.Sprintf("%q is not an approved claim: %s", c.Text, c.Detail))
		}
	}
	if offerID != 0 {
		if report.OfferState != string(brands.OfferApproved) {
			block("offer_not_approved", "the offer on this page has not been approved as true")
		}
		if report.OfferCurrent != nil && *report.OfferCurrent != offerVersion {
			block("offer_changed", fmt.Sprintf("the offer is now version %d; this page shows version %d", *report.OfferCurrent, offerVersion))
		}
	}
	if prow.Language == "ar" {
		warn("arabic_review", "Arabic copy: native review is not recorded in ADX; confirm it before launch")
	}
	r.Ready = len(r.Blockers) == 0
	return r, nil
}

// PublicURL is where a page is served.
func (p *Pages) PublicURL(slug string) string { return p.base + "/p/" + slug }

// ProjectID64 is the page's project id or 0.
func (pg Page) ProjectID64() int64 {
	if pg.ProjectID == nil {
		return 0
	}
	return *pg.ProjectID
}

// Helpers.

type limiter struct {
	mu    sync.Mutex
	hits  map[string][]time.Time
	limit int
	clock core.Clock
}

func newLimiter(limit int, clock core.Clock) *limiter {
	return &limiter{hits: map[string][]time.Time{}, limit: limit, clock: clock}
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()
	cut := now.Add(-time.Minute)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	if len(l.hits) > 10000 {
		for k, v := range l.hits {
			if len(v) == 0 || !v[len(v)-1].After(cut) {
				delete(l.hits, k)
			}
		}
	}
	return true
}

func (p *Pages) hash(ip string) string {
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(p.salt + "|" + ip))
	return hex.EncodeToString(sum[:8])
}

func (p *Pages) page(ctx context.Context, workspaceID, id int64) (store.Page, error) {
	row, err := p.db.Reader().GetPage(ctx, store.GetPageParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Page{}, core.NotFound("page %d not found", id)
	}
	return row, err
}

func pageFromRow(r store.Page) Page {
	return Page{ID: r.ID, BrandID: r.BrandID, ProjectID: idPtr(r.ProjectID), PlanID: idPtr(r.PlanID), PlanVersion: idPtr(r.PlanVersion), CampaignID: idPtr(r.CampaignID), Slug: r.Slug, Name: r.Name, Language: r.Language, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
}

func versionFromRow(r store.PageVersion) (PageVersion, error) {
	v := PageVersion{ID: r.ID, PageID: r.PageID, Version: r.Version, Note: r.Note, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC()}
	if err := json.Unmarshal([]byte(r.Document), &v.Document); err != nil {
		return PageVersion{}, err
	}
	if v.Document.Sections == nil {
		v.Document.Sections = []Section{}
	}
	return v, nil
}

func deploymentFromRow(r store.PageDeployment, versions map[int64]int64) Deployment {
	return Deployment{ID: r.ID, PageID: r.PageID, VersionID: r.VersionID, Version: versions[r.VersionID], Adapter: r.Adapter, State: r.State, URL: r.Url, OutputSHA256: r.OutputSha256,
		RolledBackFromID: idPtr(r.RolledBackFromID), Error: r.ErrorSafe, DeployedBy: idPtr(r.DeployedBy), DeployedAt: r.DeployedAt.UTC()}
}

func submissionFromRow(r store.FormSubmission) Submission {
	s := Submission{ID: r.ID, PageID: r.PageID, VersionID: r.VersionID, ProjectID: idPtr(r.ProjectID), State: r.State, Reason: r.Reason, Test: r.Test == 1, Fields: map[string]string{}, Attribution: map[string]string{}, EvidenceID: idPtr(r.EvidenceID), ReceivedAt: r.ReceivedAt.UTC()}
	_ = json.Unmarshal([]byte(r.Fields), &s.Fields)
	_ = json.Unmarshal([]byte(r.Attribution), &s.Attribution)
	return s
}

func mustJSON(v any) string {
	if v == nil {
		return "{}"
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
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

// SortedFieldNames is a stable order for a submission's fields.
func SortedFieldNames(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
