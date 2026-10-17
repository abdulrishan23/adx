// Package creative is the pilot creative workflow: upload renders and brand
// files, keep a brand kit per project, lay out an ad from copy in one of the
// four formats and two languages, render it to an immutable output, put it
// in review, approve or send back, export. Review decisions bind to a
// version's output checksum (core/assets); editing a rendered or reviewed
// version makes a new draft version with no approval of its own. Nothing
// here delivers an ad anywhere.
package creative

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assets"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/store"
)

type Asset struct {
	ID        int64      `json:"id"`
	ProjectID *int64     `json:"projectId"`
	Kind      string     `json:"kind"`
	Name      string     `json:"name"`
	Ref       assets.Ref `json:"ref"`
	// Provenance says how the file came to be: uploaded, template_render or
	// ai_generated. Shown next to the file.
	Provenance string    `json:"provenance"`
	UploadedBy *int64    `json:"uploadedBy"`
	CreatedAt  time.Time `json:"createdAt"`
}

type BrandKitVersion struct {
	Version   int64           `json:"version"`
	ProjectID int64           `json:"projectId"`
	Kit       assets.BrandKit `json:"kit"`
	CreatedAt time.Time       `json:"createdAt"`
}

type Creative struct {
	ID        int64           `json:"id"`
	ProjectID *int64          `json:"projectId"`
	Name      string          `json:"name"`
	Format    assets.Format   `json:"format"`
	Language  assets.Language `json:"language"`
	// OfferID and OfferVersion name the offer version this creative was built
	// on, if any.
	OfferID      *int64    `json:"offerId"`
	OfferVersion *int64    `json:"offerVersion"`
	PlanID       *int64    `json:"planId"`
	PlanVersion  *int64    `json:"planVersion"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Review struct {
	Decision     string    `json:"decision"`
	OutputSHA256 string    `json:"outputSha256"`
	Note         string    `json:"note"`
	DecidedBy    int64     `json:"decidedBy"`
	DecidedAt    time.Time `json:"decidedAt"`
}

type Version struct {
	ID          int64                `json:"id"`
	CreativeID  int64                `json:"creativeId"`
	Version     int64                `json:"version"`
	Document    assets.Document      `json:"document"`
	ReviewState assets.ReviewState   `json:"reviewState"`
	Renderer    *string              `json:"renderer"`
	Output      *assets.Ref          `json:"output"`
	RenderedAt  *time.Time           `json:"renderedAt"`
	Checks      []assets.LayoutCheck `json:"checks"`
	Reviews     []Review             `json:"reviews"`
	// Acceptance is the content acceptance an approval produced, binding this
	// version's exact output. It grants no delivery.
	Acceptance *assets.ContentAcceptance `json:"acceptance"`
	// Bindings is what the copy was checked against when rendered.
	Bindings ClaimBindings `json:"bindings"`
	// RenderError is the last render failure's safe message, if any.
	RenderError string    `json:"renderError"`
	RenderRunID *int64    `json:"renderRunId"`
	CreatedAt   time.Time `json:"createdAt"`
}

type CreativeInput struct {
	ProjectID     int64           `json:"projectId"`
	Name          string          `json:"name"`
	Format        assets.Format   `json:"format"`
	Language      assets.Language `json:"language"`
	Copy          assets.Copy     `json:"copy"`
	RenderAssetID int64           `json:"renderAssetId"`
	// OfferID builds the creative on an offer of the project's brand. The
	// offer's current version is recorded so a later change lists this
	// creative as affected.
	OfferID int64 `json:"offerId"`
	// PlanID and PlanVersion tie the creative to the plan version whose
	// asset requirements it serves.
	PlanID      int64 `json:"planId"`
	PlanVersion int64 `json:"planVersion"`
}

type Config struct {
	Clock core.Clock
	// Brands checks that an offer belongs to the project's brand. Nil
	// refuses offers.
	Brands *brands.Brands
	// Plans checks plan bindings. Nil refuses them.
	Plans *plans.Plans
	// Generator is an AI image generation provider. None exists; the
	// interface is here so a future one is labelled, not mistaken for a
	// template layout.
	Generator assets.ImageGenerator
}

type Service struct {
	db        *store.DB
	store     assets.Store
	renderer  assets.Renderer
	clock     core.Clock
	brands    *brands.Brands
	plans     *plans.Plans
	generator assets.ImageGenerator
}

// New wires the workflow. store and renderer may be nil when assets aren't
// configured; uploads and renders then fail with a clear message.
func New(db *store.DB, st assets.Store, renderer assets.Renderer, cfg Config) *Service {
	if cfg.Clock == nil {
		cfg.Clock = core.SystemClock{}
	}
	return &Service{db: db, store: st, renderer: renderer, clock: cfg.Clock, brands: cfg.Brands, plans: cfg.Plans, generator: cfg.Generator}
}

var allowedUploadTypes = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true, "image/svg+xml": true, "font/ttf": true, "font/otf": true, "font/woff": true, "font/woff2": true}

// UploadAsset stores a render, logo or font. Marketers may. The bytes are
// content-addressed, so uploading the same file twice is one blob.
func (s *Service) UploadAsset(ctx context.Context, actor core.Actor, projectID int64, kind, name, contentType string, r io.Reader) (Asset, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Asset{}, core.ErrForbidden
	}
	if s.store == nil {
		return Asset{}, core.Conflict("asset storage is not configured on this server")
	}
	switch kind {
	case "render", "logo", "font", "other":
	default:
		return Asset{}, core.Invalid("kind must be render, logo, font or other")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return Asset{}, core.Invalid("a file name of up to 200 characters is required")
	}
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if !allowedUploadTypes[contentType] {
		return Asset{}, core.Invalid("content type %q is not an image or font ADX accepts", contentType)
	}
	if projectID != 0 {
		if _, err := s.db.Reader().GetProject(ctx, projectID); errors.Is(err, sql.ErrNoRows) {
			return Asset{}, core.NotFound("project %d not found", projectID)
		} else if err != nil {
			return Asset{}, err
		}
	}
	ref, err := s.store.Put(ctx, contentType, r)
	if err != nil {
		return Asset{}, core.Invalid("the file could not be stored: %s", err.Error())
	}
	row, err := s.db.Writer().CreateAsset(ctx, store.CreateAssetParams{WorkspaceID: actor.WorkspaceID, ProjectID: optionalID(projectID), Kind: kind, Name: name,
		Key: ref.Key, Sha256: ref.SHA256, Size: ref.Size, ContentType: ref.ContentType, UploadedBy: optionalID(actor.UserID), CreatedAt: s.clock.Now().UTC(), Provenance: "uploaded"})
	if err != nil {
		return Asset{}, err
	}
	return assetFromRow(row), nil
}

func (s *Service) ListAssets(ctx context.Context, actor core.Actor, projectID int64, kind string) ([]Asset, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := s.db.Reader().ListAssets(ctx, store.ListAssetsParams{WorkspaceID: actor.WorkspaceID, ProjectID: optionalID(projectID), Kind: nullString(kind), Max: 500})
	if err != nil {
		return nil, err
	}
	out := make([]Asset, 0, len(rows))
	for _, r := range rows {
		out = append(out, assetFromRow(r))
	}
	return out, nil
}

// Open streams an asset's bytes by key, verifying the checksum. Viewers may
// see what the screens show. Outputs of creatives are opened the same way.
func (s *Service) Open(ctx context.Context, actor core.Actor, key string) (io.ReadCloser, assets.Ref, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, assets.Ref{}, core.ErrForbidden
	}
	if s.store == nil {
		return nil, assets.Ref{}, core.Conflict("asset storage is not configured on this server")
	}
	ref, err := s.refByKey(ctx, actor.WorkspaceID, key)
	if err != nil {
		return nil, assets.Ref{}, err
	}
	rc, err := s.store.Open(ctx, ref)
	if err != nil {
		return nil, assets.Ref{}, core.Conflict("the stored file does not match its checksum; it was not served")
	}
	return rc, ref, nil
}

// refByKey finds a ref the workspace knows: an uploaded asset or a rendered
// output. Unknown keys are not found, even if the blob exists.
func (s *Service) refByKey(ctx context.Context, workspaceID int64, key string) (assets.Ref, error) {
	if row, err := s.db.Reader().GetAssetByKey(ctx, store.GetAssetByKeyParams{WorkspaceID: workspaceID, Key: key}); err == nil {
		return assets.Ref{Key: row.Key, SHA256: row.Sha256, Size: row.Size, ContentType: row.ContentType}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return assets.Ref{}, err
	}
	var ref assets.Ref
	err := s.db.Read.QueryRowContext(ctx, `SELECT v.output_key, v.output_sha256, v.output_size, v.output_type FROM creative_versions v
		JOIN creatives c ON c.id = v.creative_id WHERE c.workspace_id = ? AND v.output_key = ? LIMIT 1`, workspaceID, key).Scan(&ref.Key, &ref.SHA256, &ref.Size, &ref.ContentType)
	if errors.Is(err, sql.ErrNoRows) {
		return assets.Ref{}, core.NotFound("file not found")
	}
	return ref, err
}

func (s *Service) GetBrandKit(ctx context.Context, actor core.Actor, projectID int64) (BrandKitVersion, bool, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return BrandKitVersion{}, false, core.ErrForbidden
	}
	row, err := s.db.Reader().GetCurrentBrandKit(ctx, store.GetCurrentBrandKitParams{ProjectID: projectID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return BrandKitVersion{ProjectID: projectID, Kit: assets.BrandKit{Version: 1, Logos: map[string]*assets.Ref{}, Fonts: assets.DefaultFonts, Colors: map[string]string{}}}, false, nil
	}
	if err != nil {
		return BrandKitVersion{}, false, err
	}
	kit, err := kitFromRow(row)
	return kit, true, err
}

// PutBrandKit saves a new kit version. Marketers may. Logo and font refs
// must be assets this workspace uploaded.
func (s *Service) PutBrandKit(ctx context.Context, actor core.Actor, projectID int64, kit assets.BrandKit) (BrandKitVersion, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return BrandKitVersion{}, core.ErrForbidden
	}
	if _, err := s.db.Reader().GetProject(ctx, projectID); errors.Is(err, sql.ErrNoRows) {
		return BrandKitVersion{}, core.NotFound("project %d not found", projectID)
	} else if err != nil {
		return BrandKitVersion{}, err
	}
	for role, ref := range kit.Logos {
		if ref == nil {
			continue
		}
		if _, err := s.refByKey(ctx, actor.WorkspaceID, ref.Key); err != nil {
			return BrandKitVersion{}, core.Invalid("logo %q points at a file this workspace doesn't have", role)
		}
	}
	for role, f := range kit.Fonts {
		if f.File != nil {
			if _, err := s.refByKey(ctx, actor.WorkspaceID, f.File.Key); err != nil {
				return BrandKitVersion{}, core.Invalid("font %q points at a file this workspace doesn't have", role)
			}
		}
	}
	for name, c := range kit.Colors {
		if c != "" && !validColor(c) {
			return BrandKitVersion{}, core.Invalid("colour %q must be #RGB or #RRGGBB", name)
		}
	}
	kit.Version = 1
	raw, _ := json.Marshal(kit)
	var row store.BrandKit
	err := s.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		version := int64(1)
		if cur, err := q.GetCurrentBrandKit(ctx, store.GetCurrentBrandKitParams{ProjectID: projectID, WorkspaceID: actor.WorkspaceID}); err == nil {
			version = cur.Version + 1
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var err error
		row, err = q.CreateBrandKit(ctx, store.CreateBrandKitParams{WorkspaceID: actor.WorkspaceID, ProjectID: projectID, Version: version, Document: string(raw),
			CreatedBy: optionalID(actor.UserID), CreatedAt: s.clock.Now().UTC()})
		return err
	})
	if store.IsUniqueViolation(err) {
		return BrandKitVersion{}, core.Conflict("someone saved the brand kit at the same time; reload and try again")
	}
	if err != nil {
		return BrandKitVersion{}, err
	}
	return kitFromRow(row)
}

// Create lays out a new creative from copy, a render and the project's brand
// kit, as version 1 in draft. Marketers may.
func (s *Service) Create(ctx context.Context, actor core.Actor, in CreativeInput) (Creative, Version, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Creative{}, Version{}, core.ErrForbidden
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 120 {
		return Creative{}, Version{}, core.Invalid("a name of up to 120 characters is required")
	}
	if _, ok := assets.Dimensions[in.Format]; !ok {
		return Creative{}, Version{}, core.Invalid("format must be 9:16, 4:5, 1:1 or 1.91:1")
	}
	if in.Language != assets.English && in.Language != assets.Arabic {
		return Creative{}, Version{}, core.Invalid("language must be en or ar")
	}
	kit := assets.BrandKit{Fonts: assets.DefaultFonts}
	if in.ProjectID != 0 {
		kv, _, err := s.GetBrandKit(ctx, actor, in.ProjectID)
		if err != nil {
			return Creative{}, Version{}, err
		}
		kit = kv.Kit
	}
	var render *assets.Ref
	if in.RenderAssetID != 0 {
		row, err := s.db.Reader().GetAsset(ctx, store.GetAssetParams{ID: in.RenderAssetID, WorkspaceID: actor.WorkspaceID})
		if errors.Is(err, sql.ErrNoRows) {
			return Creative{}, Version{}, core.NotFound("render asset %d not found", in.RenderAssetID)
		}
		if err != nil {
			return Creative{}, Version{}, err
		}
		// A render uploaded for one project stays with it; a render with no
		// project is shared across the workspace.
		if row.ProjectID.Valid && row.ProjectID.Int64 != in.ProjectID {
			return Creative{}, Version{}, core.Invalid("render asset %d belongs to another project", in.RenderAssetID)
		}
		render = &assets.Ref{Key: row.Key, SHA256: row.Sha256, Size: row.Size, ContentType: row.ContentType}
	}
	if in.PlanID != 0 {
		if s.plans == nil {
			return Creative{}, Version{}, core.Invalid("plans are not configured")
		}
		pd, err := s.plans.Get(ctx, actor, in.PlanID)
		if err != nil {
			return Creative{}, Version{}, err
		}
		if in.PlanVersion == 0 {
			in.PlanVersion = pd.Versions[len(pd.Versions)-1].Version
		}
		found := false
		for _, v := range pd.Versions {
			if v.Version == in.PlanVersion {
				found = true
			}
		}
		if !found {
			return Creative{}, Version{}, core.NotFound("plan %d has no version %d", in.PlanID, in.PlanVersion)
		}
		if in.ProjectID != 0 && s.brands != nil {
			owner, err := s.brands.BrandOfProject(ctx, actor, in.ProjectID)
			if err != nil {
				return Creative{}, Version{}, err
			}
			if owner != pd.Plan.BrandID {
				return Creative{}, Version{}, core.Invalid("plan %d is for another brand than project %d", in.PlanID, in.ProjectID)
			}
		}
	}
	var offerVersion int64
	if in.OfferID != 0 {
		if s.brands == nil {
			return Creative{}, Version{}, core.Invalid("offers are not configured")
		}
		v, err := s.brands.OfferForProject(ctx, actor, in.OfferID, in.ProjectID)
		if err != nil {
			return Creative{}, Version{}, err
		}
		offerVersion = v
	}
	doc, err := assets.Layout(in.Format, in.Language, in.Copy, render, kit)
	if err != nil {
		return Creative{}, Version{}, core.Invalid("%s", err.Error())
	}
	raw, _ := json.Marshal(doc)
	now := s.clock.Now().UTC()
	var crow store.Creative
	var vrow store.CreativeVersion
	err = s.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		crow, err = q.CreateCreative(ctx, store.CreateCreativeParams{WorkspaceID: actor.WorkspaceID, ProjectID: optionalID(in.ProjectID), Name: in.Name, Format: string(in.Format),
			Language: string(in.Language), OfferID: optionalID(in.OfferID), OfferVersion: optionalID(offerVersion), PlanID: optionalID(in.PlanID), PlanVersion: optionalID(in.PlanVersion), CreatedBy: optionalID(actor.UserID), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		vrow, err = q.CreateCreativeVersion(ctx, store.CreateCreativeVersionParams{CreativeID: crow.ID, Version: 1, Document: string(raw), CreatedBy: optionalID(actor.UserID), CreatedAt: now})
		return err
	})
	if err != nil {
		return Creative{}, Version{}, err
	}
	return creativeFromRow(crow), s.versionFromRow(ctx, vrow), nil
}

func (s *Service) List(ctx context.Context, actor core.Actor, projectID int64) ([]Creative, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := s.db.Reader().ListCreatives(ctx, store.ListCreativesParams{WorkspaceID: actor.WorkspaceID, ProjectID: optionalID(projectID), Max: 200})
	if err != nil {
		return nil, err
	}
	out := make([]Creative, 0, len(rows))
	for _, r := range rows {
		out = append(out, creativeFromRow(r))
	}
	return out, nil
}

// Get returns a creative with every version, newest last.
func (s *Service) Get(ctx context.Context, actor core.Actor, id int64) (Creative, []Version, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return Creative{}, nil, core.ErrForbidden
	}
	crow, err := s.db.Reader().GetCreative(ctx, store.GetCreativeParams{ID: id, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Creative{}, nil, core.NotFound("creative %d not found", id)
	}
	if err != nil {
		return Creative{}, nil, err
	}
	rows, err := s.db.Reader().ListCreativeVersions(ctx, id)
	if err != nil {
		return Creative{}, nil, err
	}
	versions := make([]Version, 0, len(rows))
	for _, r := range rows {
		versions = append(versions, s.versionFromRow(ctx, r))
	}
	return creativeFromRow(crow), versions, nil
}

// SaveDocument edits the layout. A draft is edited in place. A version that
// has been rendered, reviewed or approved is left as it is and the edit
// becomes a new draft version, so an approval can never follow changed
// artwork.
func (s *Service) SaveDocument(ctx context.Context, actor core.Actor, creativeID int64, doc assets.Document) (Version, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Version{}, core.ErrForbidden
	}
	crow, latest, err := s.latest(ctx, actor, creativeID)
	if err != nil {
		return Version{}, err
	}
	if doc.Format != assets.Format(crow.Format) || doc.Language != assets.Language(crow.Language) {
		return Version{}, core.Invalid("a creative keeps its format and language; make a new creative for another")
	}
	if dims := assets.Dimensions[doc.Format]; doc.Width != dims[0] || doc.Height != dims[1] {
		return Version{}, core.Invalid("the document size must be %dx%d for %s", dims[0], dims[1], doc.Format)
	}
	for _, l := range doc.Layers {
		if l.Src != nil {
			if _, err := s.refByKey(ctx, actor.WorkspaceID, l.Src.Key); err != nil {
				return Version{}, core.Invalid("layer %q points at a file this workspace doesn't have", l.ID)
			}
		}
	}
	if doc.Image != nil {
		if _, err := s.refByKey(ctx, actor.WorkspaceID, doc.Image.Ref.Key); err != nil {
			return Version{}, core.Invalid("the render points at a file this workspace doesn't have")
		}
	}
	doc.Version = 1
	raw, _ := json.Marshal(doc)
	now := s.clock.Now().UTC()
	var out store.CreativeVersion
	err = s.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		if assets.ReviewState(latest.ReviewState) == assets.Draft {
			n, err := q.UpdateCreativeDraft(ctx, store.UpdateCreativeDraftParams{Document: string(raw), ID: latest.ID})
			if err != nil {
				return err
			}
			if n != 1 {
				return core.Conflict("the draft changed; reload and try again")
			}
			out, err = q.GetCreativeVersion(ctx, latest.ID)
			return err
		}
		var err error
		out, err = q.CreateCreativeVersion(ctx, store.CreateCreativeVersionParams{CreativeID: creativeID, Version: latest.Version + 1, Document: string(raw), CreatedBy: optionalID(actor.UserID), CreatedAt: now})
		if err != nil {
			return err
		}
		return q.TouchCreative(ctx, store.TouchCreativeParams{UpdatedAt: now, ID: creativeID})
	})
	if store.IsUniqueViolation(err) {
		return Version{}, core.Conflict("someone saved this creative at the same time; reload and try again")
	}
	if err != nil {
		return Version{}, err
	}
	return s.versionFromRow(ctx, out), nil
}

// Submit puts the rendered latest version in review: publishing to the
// board. It is not delivery.
func (s *Service) Submit(ctx context.Context, actor core.Actor, creativeID int64) (Version, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Version{}, core.ErrForbidden
	}
	_, latest, err := s.latest(ctx, actor, creativeID)
	if err != nil {
		return Version{}, err
	}
	return s.transition(ctx, latest, assets.RenderedSt, assets.InReview)
}

// ReviewVersion records an admin's decision on the latest version while it
// is in review. Approval binds the exact output checksum; revise sends it
// back, and the next edit makes a new version.
func (s *Service) ReviewVersion(ctx context.Context, actor core.Actor, creativeID int64, decision, note string) (Version, error) {
	if !actor.AtLeast(core.RoleAdmin) || actor.WorkspaceID == 0 || actor.UserID == 0 {
		return Version{}, core.ErrForbidden
	}
	if decision != "approved" && decision != "revise" {
		return Version{}, core.Invalid("decision must be approved or revise")
	}
	if len(note) > 1000 {
		return Version{}, core.Invalid("note must be 1000 characters or fewer")
	}
	_, latest, err := s.latest(ctx, actor, creativeID)
	if err != nil {
		return Version{}, err
	}
	if assets.ReviewState(latest.ReviewState) != assets.InReview {
		return Version{}, core.Conflict("version %d is %s, not in review", latest.Version, latest.ReviewState)
	}
	if !latest.OutputSha256.Valid {
		return Version{}, core.Conflict("version %d has no rendered output to approve", latest.Version)
	}
	to := assets.Approved
	if decision == "revise" {
		to = assets.Revise
	}
	now := s.clock.Now().UTC()
	err = s.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		n, err := q.SetCreativeVersionState(ctx, store.SetCreativeVersionStateParams{ToState: string(to), ID: latest.ID, FromState: string(assets.InReview)})
		if err != nil {
			return err
		}
		if n != 1 {
			return core.Conflict("the version changed; reload and try again")
		}
		_, err = q.CreateCreativeReview(ctx, store.CreateCreativeReviewParams{VersionID: latest.ID, Decision: decision, OutputSha256: latest.OutputSha256.String, Note: strings.TrimSpace(note),
			DecidedBy: actor.UserID, DecidedAt: now})
		return err
	})
	if err != nil {
		return Version{}, err
	}
	row, err := s.db.Reader().GetCreativeVersion(ctx, latest.ID)
	if err != nil {
		return Version{}, err
	}
	return s.versionFromRow(ctx, row), nil
}

// Export opens a version's rendered output, verifying the checksum, for
// download. Viewers may export what they can see.
func (s *Service) Export(ctx context.Context, actor core.Actor, creativeID int64, version int64) (io.ReadCloser, Version, error) {
	_, versions, err := s.Get(ctx, actor, creativeID)
	if err != nil {
		return nil, Version{}, err
	}
	for _, v := range versions {
		if v.Version != version {
			continue
		}
		if v.Output == nil {
			return nil, Version{}, core.Conflict("version %d has not been rendered", version)
		}
		rc, _, err := s.Open(ctx, actor, v.Output.Key)
		return rc, v, err
	}
	return nil, Version{}, core.NotFound("version %d not found", version)
}

func (s *Service) transition(ctx context.Context, latest store.CreativeVersion, from, to assets.ReviewState) (Version, error) {
	if assets.ReviewState(latest.ReviewState) != from {
		return Version{}, core.Conflict("version %d is %s, not %s", latest.Version, latest.ReviewState, from)
	}
	if !assets.CanTransition(from, to) {
		return Version{}, core.Conflict("cannot move from %s to %s", from, to)
	}
	n, err := s.db.Writer().SetCreativeVersionState(ctx, store.SetCreativeVersionStateParams{ToState: string(to), ID: latest.ID, FromState: string(from)})
	if err != nil {
		return Version{}, err
	}
	if n != 1 {
		return Version{}, core.Conflict("the version changed; reload and try again")
	}
	row, err := s.db.Reader().GetCreativeVersion(ctx, latest.ID)
	if err != nil {
		return Version{}, err
	}
	return s.versionFromRow(ctx, row), nil
}

func (s *Service) latest(ctx context.Context, actor core.Actor, creativeID int64) (store.Creative, store.CreativeVersion, error) {
	crow, err := s.db.Reader().GetCreative(ctx, store.GetCreativeParams{ID: creativeID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Creative{}, store.CreativeVersion{}, core.NotFound("creative %d not found", creativeID)
	}
	if err != nil {
		return store.Creative{}, store.CreativeVersion{}, err
	}
	latest, err := s.db.Reader().GetLatestCreativeVersion(ctx, creativeID)
	if err != nil {
		return store.Creative{}, store.CreativeVersion{}, err
	}
	return crow, latest, nil
}

func (s *Service) versionFromRow(ctx context.Context, r store.CreativeVersion) Version {
	v := Version{ID: r.ID, CreativeID: r.CreativeID, Version: r.Version, ReviewState: assets.ReviewState(r.ReviewState), CreatedAt: r.CreatedAt.UTC(), Reviews: []Review{}, RenderError: r.RenderError, RenderRunID: idPtrOf(r.RenderRunID), Bindings: ClaimBindings{FactIDs: []int64{}, Claims: []ClaimCheck{}}}
	_ = json.Unmarshal([]byte(r.Document), &v.Document)
	_ = json.Unmarshal([]byte(r.Bindings), &v.Bindings)
	if v.Bindings.FactIDs == nil {
		v.Bindings.FactIDs = []int64{}
	}
	if v.Bindings.Claims == nil {
		v.Bindings.Claims = []ClaimCheck{}
	}
	v.Checks = assets.CheckDocument(v.Document, false)
	if r.Renderer.Valid {
		v.Renderer = &r.Renderer.String
	}
	if r.OutputKey.Valid {
		v.Output = &assets.Ref{Key: r.OutputKey.String, SHA256: r.OutputSha256.String, Size: r.OutputSize.Int64, ContentType: r.OutputType.String}
	}
	if r.RenderedAt.Valid {
		t := r.RenderedAt.Time.UTC()
		v.RenderedAt = &t
	}
	reviews, err := s.db.Reader().ListCreativeReviews(ctx, r.ID)
	if err == nil {
		for _, rv := range reviews {
			v.Reviews = append(v.Reviews, Review{Decision: rv.Decision, OutputSHA256: rv.OutputSha256, Note: rv.Note, DecidedBy: rv.DecidedBy, DecidedAt: rv.DecidedAt.UTC()})
			if rv.Decision == "approved" && v.Output != nil && rv.OutputSha256 == v.Output.SHA256 && v.ReviewState == assets.Approved {
				v.Acceptance = &assets.ContentAcceptance{VersionID: r.ID, Ref: *v.Output, AcceptedBy: rv.DecidedBy, AcceptedAt: rv.DecidedAt.UTC()}
			}
		}
	}
	return v
}

func idPtrOf(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func assetFromRow(r store.Asset) Asset {
	a := Asset{ID: r.ID, Kind: r.Kind, Name: r.Name, Provenance: r.Provenance, Ref: assets.Ref{Key: r.Key, SHA256: r.Sha256, Size: r.Size, ContentType: r.ContentType}, CreatedAt: r.CreatedAt.UTC()}
	if r.ProjectID.Valid {
		a.ProjectID = &r.ProjectID.Int64
	}
	if r.UploadedBy.Valid {
		a.UploadedBy = &r.UploadedBy.Int64
	}
	return a
}

func creativeFromRow(r store.Creative) Creative {
	c := Creative{ID: r.ID, Name: r.Name, Format: assets.Format(r.Format), Language: assets.Language(r.Language), CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC()}
	if r.ProjectID.Valid {
		c.ProjectID = &r.ProjectID.Int64
	}
	if r.OfferID.Valid {
		c.OfferID = &r.OfferID.Int64
	}
	if r.OfferVersion.Valid {
		c.OfferVersion = &r.OfferVersion.Int64
	}
	if r.PlanID.Valid {
		c.PlanID = &r.PlanID.Int64
	}
	if r.PlanVersion.Valid {
		c.PlanVersion = &r.PlanVersion.Int64
	}
	return c
}

func kitFromRow(r store.BrandKit) (BrandKitVersion, error) {
	var kit assets.BrandKit
	if err := json.Unmarshal([]byte(r.Document), &kit); err != nil {
		return BrandKitVersion{}, fmt.Errorf("brand kit %d is not valid JSON: %w", r.ID, err)
	}
	if kit.Logos == nil {
		kit.Logos = map[string]*assets.Ref{}
	}
	if kit.Fonts == nil {
		kit.Fonts = assets.DefaultFonts
	}
	if kit.Colors == nil {
		kit.Colors = map[string]string{}
	}
	return BrandKitVersion{Version: r.Version, ProjectID: r.ProjectID, Kit: kit, CreatedAt: r.CreatedAt.UTC()}, nil
}

func firstError(checks []assets.LayoutCheck) string {
	for _, c := range checks {
		if c.Level == "error" {
			return c.Text
		}
	}
	return ""
}

func validColor(c string) bool {
	if (len(c) != 4 && len(c) != 7) || c[0] != '#' {
		return false
	}
	for _, r := range c[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

func safe(err error) string {
	var ce *core.Error
	if errors.As(err, &ce) {
		return ce.Message
	}
	return "see the server log"
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
func optionalID(id int64) sql.NullInt64  { return sql.NullInt64{Int64: id, Valid: id != 0} }
