// Package brands keeps what ADX is allowed to say about a brand: which
// projects belong to it, the offers it is running and the facts behind its
// claims. Facts are classified by how they got here (an admin approved
// them, a person typed them, ADX inferred them) and two sources that
// disagree are a conflict a person resolves. Nothing here chooses between
// sources on its own: source precedence is an unresolved business rule
// (decisions.md), so a conflict blocks approval until an admin picks.
//
// Scope is the brand. A draft for one brand's project is only ever given
// that brand's offers and claims; a competitor brand's facts stay on the
// competitor side for research to compare against.
package brands

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

type BrandKind string

const (
	// Own is a brand ADX markets: Reportage and its developments.
	Own BrandKind = "own"
	// Competitor is a brand ADX only observes.
	Competitor BrandKind = "competitor"
)

type Brand struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Kind      BrandKind `json:"kind"`
	Notes     string    `json:"notes"`
	CreatedBy *int64    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

type BrandInput struct {
	Name  string    `json:"name"`
	Kind  BrandKind `json:"kind"`
	Notes string    `json:"notes"`
}

// BrandProject is a project as the brand workspace shows it.
type BrandProject struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// BrandSummary is one row of the brand list.
type BrandSummary struct {
	Brand
	ProjectCount  int `json:"projectCount"`
	OpenConflicts int `json:"openConflicts"`
}

// BrandDetail is the brand workspace: projects, current offers, current facts
// and open conflicts.
type BrandDetail struct {
	Brand     Brand          `json:"brand"`
	Projects  []BrandProject `json:"projects"`
	Offers    []Offer        `json:"offers"`
	Facts     []Fact         `json:"facts"`
	Conflicts []Conflict     `json:"conflicts"`
}

type Config struct{ Clock core.Clock }

type Brands struct {
	db    *store.DB
	clock core.Clock
}

func New(db *store.DB, cfg Config) *Brands {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Brands{db: db, clock: cfg.Clock}
}

// Create adds a brand. Admins only.
func (b *Brands) Create(ctx context.Context, actor core.Actor, in BrandInput) (Brand, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return Brand{}, core.ErrForbidden
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 100 {
		return Brand{}, core.Invalid("a name of up to 100 characters is required")
	}
	if in.Kind != Own && in.Kind != Competitor {
		return Brand{}, core.Invalid("kind must be own or competitor")
	}
	row, err := b.db.Writer().CreateBrand(ctx, store.CreateBrandParams{
		WorkspaceID: actor.WorkspaceID, Name: in.Name, Kind: string(in.Kind), Notes: strings.TrimSpace(in.Notes),
		CreatedBy: optionalID(actor.UserID), CreatedAt: b.clock.Now().UTC(),
	})
	if store.IsUniqueViolation(err) {
		return Brand{}, core.Conflict("a brand named %q already exists", in.Name)
	}
	if err != nil {
		return Brand{}, err
	}
	return brandFromRow(row), nil
}

// List is every brand in the workspace with its project count and open
// conflicts.
func (b *Brands) List(ctx context.Context, actor core.Actor) ([]BrandSummary, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := b.db.Reader().ListBrands(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]BrandSummary, 0, len(rows))
	for _, r := range rows {
		projects, err := b.db.Reader().ListProjectsByBrand(ctx, sql.NullInt64{Int64: r.ID, Valid: true})
		if err != nil {
			return nil, err
		}
		conflicts, err := b.db.Reader().ListOpenConflicts(ctx, store.ListOpenConflictsParams{WorkspaceID: actor.WorkspaceID, BrandID: r.ID})
		if err != nil {
			return nil, err
		}
		out = append(out, BrandSummary{Brand: brandFromRow(r), ProjectCount: len(projects), OpenConflicts: len(conflicts)})
	}
	return out, nil
}

// Get is the brand workspace.
func (b *Brands) Get(ctx context.Context, actor core.Actor, id int64) (BrandDetail, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return BrandDetail{}, core.ErrForbidden
	}
	brand, err := b.brand(ctx, actor.WorkspaceID, id)
	if err != nil {
		return BrandDetail{}, err
	}
	d := BrandDetail{Brand: brand, Projects: []BrandProject{}, Offers: []Offer{}, Facts: []Fact{}, Conflicts: []Conflict{}}
	projects, err := b.db.Reader().ListProjectsByBrand(ctx, sql.NullInt64{Int64: id, Valid: true})
	if err != nil {
		return BrandDetail{}, err
	}
	for _, p := range projects {
		d.Projects = append(d.Projects, BrandProject{ID: p.ID, Name: p.Name})
	}
	if d.Offers, err = b.ListOffers(ctx, actor, id); err != nil {
		return BrandDetail{}, err
	}
	if d.Facts, err = b.ListFacts(ctx, actor, id, 0); err != nil {
		return BrandDetail{}, err
	}
	if d.Conflicts, err = b.ListConflicts(ctx, actor, id); err != nil {
		return BrandDetail{}, err
	}
	return d, nil
}

// AssignProject puts a project under a brand, or under none with brandID 0.
// Admins only. Moving a project doesn't move its creatives' offers: an offer
// of the old brand on a creative is reported by OfferImpact as affected.
func (b *Brands) AssignProject(ctx context.Context, actor core.Actor, projectID, brandID int64) error {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return core.ErrForbidden
	}
	if brandID != 0 {
		if _, err := b.brand(ctx, actor.WorkspaceID, brandID); err != nil {
			return err
		}
	}
	n, err := b.db.Writer().SetProjectBrand(ctx, store.SetProjectBrandParams{BrandID: optionalID(brandID), ID: projectID})
	if err != nil {
		return err
	}
	if n == 0 {
		return core.NotFound("project %d not found", projectID)
	}
	return nil
}

// UnassignedProjects lists projects under no brand, for the admin to place.
func (b *Brands) UnassignedProjects(ctx context.Context, actor core.Actor) ([]BrandProject, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := b.db.Reader().ListUnassignedProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]BrandProject, 0, len(rows))
	for _, p := range rows {
		out = append(out, BrandProject{ID: p.ID, Name: p.Name})
	}
	return out, nil
}

// BrandOfProject is the brand a project belongs to, or 0.
func (b *Brands) BrandOfProject(ctx context.Context, actor core.Actor, projectID int64) (int64, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return 0, core.ErrForbidden
	}
	p, err := b.db.Reader().GetProject(ctx, store.GetProjectParams{ID: projectID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, core.NotFound("project %d not found", projectID)
	}
	if err != nil {
		return 0, err
	}
	return p.BrandID.Int64, nil
}

func (b *Brands) brand(ctx context.Context, workspaceID, id int64) (Brand, error) {
	row, err := b.db.Reader().GetBrand(ctx, store.GetBrandParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Brand{}, core.NotFound("brand %d not found", id)
	}
	if err != nil {
		return Brand{}, err
	}
	return brandFromRow(row), nil
}

func brandFromRow(r store.Brand) Brand {
	return Brand{ID: r.ID, Name: r.Name, Kind: BrandKind(r.Kind), Notes: r.Notes, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC()}
}

// Offers.

type OfferState string

const (
	OfferDraft     OfferState = "draft"
	OfferApproved  OfferState = "approved"
	OfferWithdrawn OfferState = "withdrawn"
)

// OfferDocument is what the offer says. Money is minor units with a
// currency; a missing figure is null. Nothing is derived: a discount is only
// what someone wrote down.
type OfferDocument struct {
	Headline    string            `json:"headline"`
	HeadlineAr  string            `json:"headlineAr"`
	Terms       string            `json:"terms"`
	TermsAr     string            `json:"termsAr"`
	DiscountPct *float64          `json:"discountPct"`
	PriceFrom   *Price            `json:"priceFrom"`
	PaymentPlan string            `json:"paymentPlan"`
	Conditions  []string          `json:"conditions"`
	Extra       map[string]string `json:"extra"`
}

// Price is an amount in minor units with its currency.
type Price struct {
	Minor    int64  `json:"minor"`
	Currency string `json:"currency"`
}

type OfferVersion struct {
	ID            int64         `json:"id"`
	OfferID       int64         `json:"offerId"`
	Version       int64         `json:"version"`
	Document      OfferDocument `json:"document"`
	EffectiveFrom string        `json:"effectiveFrom"`
	EffectiveTo   *string       `json:"effectiveTo"`
	State         OfferState    `json:"state"`
	Note          string        `json:"note"`
	CreatedBy     *int64        `json:"createdBy"`
	CreatedAt     time.Time     `json:"createdAt"`
	ApprovedBy    *int64        `json:"approvedBy"`
	ApprovedAt    *time.Time    `json:"approvedAt"`
}

type Offer struct {
	ID        int64        `json:"id"`
	BrandID   int64        `json:"brandId"`
	ProjectID *int64       `json:"projectId"`
	Name      string       `json:"name"`
	Current   OfferVersion `json:"current"`
	CreatedAt time.Time    `json:"createdAt"`
}

type OfferInput struct {
	ProjectID     int64         `json:"projectId"`
	Name          string        `json:"name"`
	Document      OfferDocument `json:"document"`
	EffectiveFrom string        `json:"effectiveFrom"`
	EffectiveTo   *string       `json:"effectiveTo"`
	Note          string        `json:"note"`
}

// AffectedCreative is a creative built on an older offer version.
type AffectedCreative struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Format       string `json:"format"`
	Language     string `json:"language"`
	OfferVersion int64  `json:"offerVersion"`
	ReviewState  string `json:"reviewState"`
}

// OfferChange is the answer to saving an offer: the new version and what was
// built on an older one.
type OfferChange struct {
	Offer    Offer              `json:"offer"`
	Affected []AffectedCreative `json:"affected"`
}

// CreateOffer starts an offer at version 1 in draft. Marketers may. The
// project, if given, must belong to the brand.
func (b *Brands) CreateOffer(ctx context.Context, actor core.Actor, brandID int64, in OfferInput) (Offer, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Offer{}, core.ErrForbidden
	}
	if _, err := b.brand(ctx, actor.WorkspaceID, brandID); err != nil {
		return Offer{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 120 {
		return Offer{}, core.Invalid("a name of up to 120 characters is required")
	}
	if in.ProjectID != 0 {
		owner, err := b.BrandOfProject(ctx, actor, in.ProjectID)
		if err != nil {
			return Offer{}, err
		}
		if owner != brandID {
			return Offer{}, core.Invalid("project %d does not belong to this brand", in.ProjectID)
		}
	}
	from, to, err := effectiveWindow(in.EffectiveFrom, in.EffectiveTo)
	if err != nil {
		return Offer{}, err
	}
	if err := validateOffer(in.Document); err != nil {
		return Offer{}, err
	}
	raw, _ := json.Marshal(normalizeOffer(in.Document))
	now := b.clock.Now().UTC()
	var orow store.Offer
	var vrow store.OfferVersion
	err = b.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		orow, err = q.CreateOffer(ctx, store.CreateOfferParams{WorkspaceID: actor.WorkspaceID, BrandID: brandID, ProjectID: optionalID(in.ProjectID), Name: in.Name, CreatedBy: optionalID(actor.UserID), CreatedAt: now})
		if err != nil {
			return err
		}
		vrow, err = q.CreateOfferVersion(ctx, store.CreateOfferVersionParams{OfferID: orow.ID, Version: 1, Document: string(raw), EffectiveFrom: from, EffectiveTo: to, State: string(OfferDraft), Note: strings.TrimSpace(in.Note), CreatedBy: optionalID(actor.UserID), CreatedAt: now})
		return err
	})
	if err != nil {
		return Offer{}, err
	}
	return offerFromRows(orow, vrow)
}

// ReviseOffer saves a new version of an offer in draft and lists the
// creatives built on earlier versions. Marketers may. The earlier version
// keeps its state and its approval; nothing built on it changes.
func (b *Brands) ReviseOffer(ctx context.Context, actor core.Actor, offerID int64, in OfferInput) (OfferChange, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return OfferChange{}, core.ErrForbidden
	}
	orow, err := b.offer(ctx, actor.WorkspaceID, offerID)
	if err != nil {
		return OfferChange{}, err
	}
	from, to, err := effectiveWindow(in.EffectiveFrom, in.EffectiveTo)
	if err != nil {
		return OfferChange{}, err
	}
	if err := validateOffer(in.Document); err != nil {
		return OfferChange{}, err
	}
	raw, _ := json.Marshal(normalizeOffer(in.Document))
	now := b.clock.Now().UTC()
	var vrow store.OfferVersion
	err = b.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		cur, err := q.GetCurrentOfferVersion(ctx, offerID)
		if err != nil {
			return err
		}
		vrow, err = q.CreateOfferVersion(ctx, store.CreateOfferVersionParams{OfferID: offerID, Version: cur.Version + 1, Document: string(raw), EffectiveFrom: from, EffectiveTo: to, State: string(OfferDraft), Note: strings.TrimSpace(in.Note), CreatedBy: optionalID(actor.UserID), CreatedAt: now})
		return err
	})
	if store.IsUniqueViolation(err) {
		return OfferChange{}, core.Conflict("someone saved this offer at the same time; reload and try again")
	}
	if err != nil {
		return OfferChange{}, err
	}
	offer, err := offerFromRows(orow, vrow)
	if err != nil {
		return OfferChange{}, err
	}
	affected, err := b.OfferImpact(ctx, actor, offerID)
	if err != nil {
		return OfferChange{}, err
	}
	return OfferChange{Offer: offer, Affected: affected}, nil
}

// OfferImpact lists the creatives built on any version of the offer older
// than the current one. They are affected, not updated: each needs a person
// to re-lay it out, which makes a new creative version without approval.
func (b *Brands) OfferImpact(ctx context.Context, actor core.Actor, offerID int64) ([]AffectedCreative, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	if _, err := b.offer(ctx, actor.WorkspaceID, offerID); err != nil {
		return nil, err
	}
	cur, err := b.db.Reader().GetCurrentOfferVersion(ctx, offerID)
	if err != nil {
		return nil, err
	}
	rows, err := b.db.Reader().ListCreativesByOffer(ctx, store.ListCreativesByOfferParams{WorkspaceID: actor.WorkspaceID, OfferID: sql.NullInt64{Int64: offerID, Valid: true}})
	if err != nil {
		return nil, err
	}
	out := []AffectedCreative{}
	for _, r := range rows {
		if r.OfferVersion.Valid && r.OfferVersion.Int64 >= cur.Version {
			continue
		}
		out = append(out, AffectedCreative{ID: r.ID, Name: r.Name, Format: r.Format, Language: r.Language, OfferVersion: r.OfferVersion.Int64, ReviewState: r.ReviewState})
	}
	return out, nil
}

// SetOfferState approves or withdraws the current version. Admins only.
// Approval says the offer is true as written; it authorizes no spend.
func (b *Brands) SetOfferState(ctx context.Context, actor core.Actor, offerID int64, state OfferState) (Offer, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 {
		return Offer{}, core.ErrForbidden
	}
	if state != OfferApproved && state != OfferWithdrawn {
		return Offer{}, core.Invalid("state must be approved or withdrawn")
	}
	orow, err := b.offer(ctx, actor.WorkspaceID, offerID)
	if err != nil {
		return Offer{}, err
	}
	now := b.clock.Now().UTC()
	var vrow store.OfferVersion
	err = b.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		cur, err := q.GetCurrentOfferVersion(ctx, offerID)
		if err != nil {
			return err
		}
		if OfferState(cur.State) == state {
			vrow = cur
			return nil
		}
		approvedBy, approvedAt := cur.ApprovedBy, cur.ApprovedAt
		if state == OfferApproved {
			approvedBy, approvedAt = optionalID(actor.UserID), sql.NullTime{Time: now, Valid: true}
		}
		if err := q.SetOfferVersionState(ctx, store.SetOfferVersionStateParams{State: string(state), ApprovedBy: approvedBy, ApprovedAt: approvedAt, ID: cur.ID}); err != nil {
			return err
		}
		vrow, err = q.GetCurrentOfferVersion(ctx, offerID)
		return err
	})
	if err != nil {
		return Offer{}, err
	}
	return offerFromRows(orow, vrow)
}

// ListOffers is the brand's offers at their current versions.
func (b *Brands) ListOffers(ctx context.Context, actor core.Actor, brandID int64) ([]Offer, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := b.db.Reader().ListOffersByBrand(ctx, store.ListOffersByBrandParams{WorkspaceID: actor.WorkspaceID, BrandID: brandID})
	if err != nil {
		return nil, err
	}
	out := make([]Offer, 0, len(rows))
	for _, r := range rows {
		cur, err := b.db.Reader().GetCurrentOfferVersion(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		o, err := offerFromRows(r, cur)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

// OffersForProject lists the offers a draft for the project may use: its
// brand's offers that are for this project or brand-wide. A project under no
// brand has none.
func (b *Brands) OffersForProject(ctx context.Context, actor core.Actor, projectID int64) (int64, []Offer, error) {
	brandID, err := b.BrandOfProject(ctx, actor, projectID)
	if err != nil {
		return 0, nil, err
	}
	if brandID == 0 {
		return 0, []Offer{}, nil
	}
	all, err := b.ListOffers(ctx, actor, brandID)
	if err != nil {
		return 0, nil, err
	}
	out := []Offer{}
	for _, o := range all {
		if o.ProjectID == nil || *o.ProjectID == projectID {
			out = append(out, o)
		}
	}
	return brandID, out, nil
}

// OfferVersions is the history of one offer.
func (b *Brands) OfferVersions(ctx context.Context, actor core.Actor, offerID int64) (Offer, []OfferVersion, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return Offer{}, nil, core.ErrForbidden
	}
	orow, err := b.offer(ctx, actor.WorkspaceID, offerID)
	if err != nil {
		return Offer{}, nil, err
	}
	rows, err := b.db.Reader().ListOfferVersions(ctx, offerID)
	if err != nil {
		return Offer{}, nil, err
	}
	versions := make([]OfferVersion, 0, len(rows))
	for _, r := range rows {
		v, err := versionFromRow(r)
		if err != nil {
			return Offer{}, nil, err
		}
		versions = append(versions, v)
	}
	offer, err := offerFromRows(orow, rows[len(rows)-1])
	if err != nil {
		return Offer{}, nil, err
	}
	return offer, versions, nil
}

// OfferForProject checks that an offer may be used on a project's draft: same
// workspace and the project's brand. It returns the offer's current version
// number. The check is what keeps another brand's offer out of a draft.
func (b *Brands) OfferForProject(ctx context.Context, actor core.Actor, offerID, projectID int64) (int64, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return 0, core.ErrForbidden
	}
	orow, err := b.offer(ctx, actor.WorkspaceID, offerID)
	if err != nil {
		return 0, err
	}
	if projectID == 0 {
		return 0, core.Invalid("an offer needs a project under its brand")
	}
	owner, err := b.BrandOfProject(ctx, actor, projectID)
	if err != nil {
		return 0, err
	}
	if owner == 0 || owner != orow.BrandID {
		return 0, core.Invalid("offer %d belongs to another brand than project %d", offerID, projectID)
	}
	if orow.ProjectID.Valid && orow.ProjectID.Int64 != projectID {
		return 0, core.Invalid("offer %d is for another project", offerID)
	}
	cur, err := b.db.Reader().GetCurrentOfferVersion(ctx, offerID)
	if err != nil {
		return 0, err
	}
	return cur.Version, nil
}

func (b *Brands) offer(ctx context.Context, workspaceID, id int64) (store.Offer, error) {
	row, err := b.db.Reader().GetOffer(ctx, store.GetOfferParams{ID: id, WorkspaceID: workspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Offer{}, core.NotFound("offer %d not found", id)
	}
	return row, err
}

func validateOffer(d OfferDocument) error {
	if strings.TrimSpace(d.Headline) == "" && strings.TrimSpace(d.HeadlineAr) == "" {
		return core.Invalid("an offer needs a headline in English or Arabic")
	}
	if d.DiscountPct != nil && (*d.DiscountPct <= 0 || *d.DiscountPct > 100) {
		return core.Invalid("discount must be between 0 and 100 percent")
	}
	if d.PriceFrom != nil {
		if d.PriceFrom.Minor < 0 {
			return core.Invalid("price can't be negative")
		}
		if len(d.PriceFrom.Currency) != 3 {
			return core.Invalid("price needs a three-letter currency code")
		}
	}
	return nil
}

func normalizeOffer(d OfferDocument) OfferDocument {
	d.Headline, d.HeadlineAr = strings.TrimSpace(d.Headline), strings.TrimSpace(d.HeadlineAr)
	d.Terms, d.TermsAr = strings.TrimSpace(d.Terms), strings.TrimSpace(d.TermsAr)
	d.PaymentPlan = strings.TrimSpace(d.PaymentPlan)
	if d.PriceFrom != nil {
		d.PriceFrom.Currency = strings.ToUpper(d.PriceFrom.Currency)
	}
	conds := make([]string, 0, len(d.Conditions))
	for _, c := range d.Conditions {
		if c = strings.TrimSpace(c); c != "" {
			conds = append(conds, c)
		}
	}
	d.Conditions = conds
	if d.Extra == nil {
		d.Extra = map[string]string{}
	}
	return d
}

func offerFromRows(o store.Offer, v store.OfferVersion) (Offer, error) {
	cur, err := versionFromRow(v)
	if err != nil {
		return Offer{}, err
	}
	return Offer{ID: o.ID, BrandID: o.BrandID, ProjectID: idPtr(o.ProjectID), Name: o.Name, Current: cur, CreatedAt: o.CreatedAt.UTC()}, nil
}

func versionFromRow(r store.OfferVersion) (OfferVersion, error) {
	var doc OfferDocument
	if err := json.Unmarshal([]byte(r.Document), &doc); err != nil {
		return OfferVersion{}, err
	}
	v := OfferVersion{ID: r.ID, OfferID: r.OfferID, Version: r.Version, Document: doc, EffectiveFrom: dateString(r.EffectiveFrom), EffectiveTo: dateStringPtr(r.EffectiveTo),
		State: OfferState(r.State), Note: r.Note, CreatedBy: idPtr(r.CreatedBy), CreatedAt: r.CreatedAt.UTC(), ApprovedBy: idPtr(r.ApprovedBy)}
	if r.ApprovedAt.Valid {
		t := r.ApprovedAt.Time.UTC()
		v.ApprovedAt = &t
	}
	return v, nil
}

// Dates. Effective dates are days, not instants; they are stored as DATE and
// compared as days in the reporting zone.

const dateLayout = "2006-01-02"

func effectiveWindow(from string, to *string) (time.Time, sql.NullTime, error) {
	f, err := time.Parse(dateLayout, strings.TrimSpace(from))
	if err != nil {
		return time.Time{}, sql.NullTime{}, core.Invalid("effective from must be a date like 2026-10-01")
	}
	if to == nil || strings.TrimSpace(*to) == "" {
		return f, sql.NullTime{}, nil
	}
	t, err := time.Parse(dateLayout, strings.TrimSpace(*to))
	if err != nil {
		return time.Time{}, sql.NullTime{}, core.Invalid("effective to must be a date like 2026-12-31")
	}
	if t.Before(f) {
		return time.Time{}, sql.NullTime{}, core.Invalid("effective to is before effective from")
	}
	return f, sql.NullTime{Time: t, Valid: true}, nil
}

func dateString(t time.Time) string { return t.UTC().Format(dateLayout) }

func dateStringPtr(t sql.NullTime) *string {
	if !t.Valid {
		return nil
	}
	s := dateString(t.Time)
	return &s
}

func optionalID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

func idPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
