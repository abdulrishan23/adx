package studio

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

type NewConcept struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// CreateConcept adds a concept, such as C1 Cash offer, to a project.
func (c *Studio) CreateConcept(ctx context.Context, actor core.Actor, projectID int64, in NewConcept) (Concept, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return Concept{}, core.ErrForbidden
	}
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if !conceptCode.MatchString(code) {
		return Concept{}, core.Invalid("a concept id looks like C1, O2 or T1AR")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = code
	}
	if err := maxRunes("name", name, 80); err != nil {
		return Concept{}, err
	}
	if _, err := c.settings(ctx, c.db.Reader(), projectID); err != nil {
		return Concept{}, err
	}
	var out store.CreativeConcept
	err := c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		pos, err := q.MaxConceptPos(ctx, projectID)
		if err != nil {
			return err
		}
		now := c.now()
		out, err = q.InsertConcept(ctx, store.InsertConceptParams{
			ProjectID: projectID, Code: code, Pos: pos + 1, Name: name, CreatedAt: now, UpdatedAt: now,
		})
		return err
	})
	if store.IsUniqueViolation(err) {
		return Concept{}, core.Conflict("%s already exists in this project", code)
	}
	if err != nil {
		return Concept{}, err
	}
	return conceptFrom(out), nil
}

type ConceptInput struct {
	Name        string          `json:"name"`
	Why         string          `json:"why"`
	PrimaryText string          `json:"primaryText"`
	Headline    string          `json:"headline"`
	Description string          `json:"description"`
	CTA         string          `json:"cta"`
	Copy        json.RawMessage `json:"copy"`
}

// SaveConcept saves a concept's copy: the blocks generated ads are laid out
// from, and the Meta ad text shown in the board's phone frames.
func (c *Studio) SaveConcept(ctx context.Context, actor core.Actor, conceptID int64, in ConceptInput) (Concept, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return Concept{}, core.ErrForbidden
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Concept{}, core.Invalid("the concept needs a name")
	}
	for _, f := range []struct {
		what, value string
		max         int
	}{
		{"name", name, 80}, {"reason", in.Why, 600}, {"primary text", in.PrimaryText, 2000},
		{"headline", in.Headline, 200}, {"description", in.Description, 400}, {"button", in.CTA, 60},
	} {
		if err := maxRunes(f.what, f.value, f.max); err != nil {
			return Concept{}, err
		}
	}
	if len(in.Copy) == 0 {
		in.Copy = json.RawMessage(`{}`)
	}
	if err := checkDoc("copy", in.Copy); err != nil {
		return Concept{}, err
	}
	k, err := c.concept(ctx, conceptID)
	if err != nil {
		return Concept{}, err
	}
	err = c.db.Writer().UpdateConcept(ctx, store.UpdateConceptParams{
		Name: name, Why: in.Why, PrimaryText: in.PrimaryText, Headline: in.Headline,
		Description: in.Description, Cta: in.CTA, Copy: string(in.Copy), UpdatedAt: c.now(), ID: k.ID,
	})
	if err != nil {
		return Concept{}, err
	}
	k, err = c.concept(ctx, conceptID)
	if err != nil {
		return Concept{}, err
	}
	return conceptFrom(k), nil
}

func (c *Studio) concept(ctx context.Context, id int64) (store.CreativeConcept, error) {
	k, err := c.db.Reader().GetConcept(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return k, core.NotFound("concept not found")
	}
	return k, err
}

type NewRender struct {
	File    string `json:"file"`    // SHA-256 of the full picture
	Preview string `json:"preview"` // SHA-256 of the small one
	Label   string `json:"label"`
}

// AddRenders adds a project's CGI images. They're numbered R01, R02 and on,
// skipping numbers already used.
func (c *Studio) AddRenders(ctx context.Context, actor core.Actor, projectID int64, in []NewRender) ([]Render, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return nil, core.ErrForbidden
	}
	if len(in) == 0 || len(in) > 100 {
		return nil, core.Invalid("add between 1 and 100 renders at a time")
	}
	if _, err := c.settings(ctx, c.db.Reader(), projectID); err != nil {
		return nil, err
	}
	type pair struct{ file, preview store.File }
	files := make([]pair, len(in))
	for i, r := range in {
		if err := maxRunes("label", r.Label, 80); err != nil {
			return nil, err
		}
		var p pair
		var err error
		if p.file, err = c.picture(ctx, r.File); err != nil {
			return nil, err
		}
		if p.preview, err = c.picture(ctx, r.Preview); err != nil {
			return nil, err
		}
		files[i] = p
	}
	err := c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		existing, err := q.ListRenders(ctx, projectID)
		if err != nil {
			return err
		}
		taken := map[string]bool{}
		pos := int64(0)
		for _, r := range existing {
			taken[r.Code] = true
			pos = max(pos, r.Pos)
		}
		n := 1
		for i, r := range in {
			code := ""
			for code == "" || taken[code] {
				code = fmt.Sprintf("R%02d", n)
				n++
			}
			taken[code] = true
			pos++
			if _, err := q.InsertRender(ctx, store.InsertRenderParams{
				ProjectID: projectID, Code: code, Pos: pos, Label: strings.TrimSpace(r.Label),
				FileID: files[i].file.ID, PreviewFileID: files[i].preview.ID, CreatedAt: c.now(),
			}); err != nil {
				return err
			}
		}
		return q.TouchCreativeProject(ctx, store.TouchCreativeProjectParams{UpdatedAt: c.now(), ProjectID: projectID})
	})
	if err != nil {
		return nil, err
	}
	return c.renders(ctx, c.db.Reader(), projectID)
}

type RenderInput struct {
	Label  string `json:"label"`
	Hidden bool   `json:"hidden"`
}

// UpdateRender relabels a render or hides it from the board and the editor.
// Ads already built on it keep it.
func (c *Studio) UpdateRender(ctx context.Context, actor core.Actor, renderID int64, in RenderInput) (Render, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return Render{}, core.ErrForbidden
	}
	if err := maxRunes("label", in.Label, 80); err != nil {
		return Render{}, err
	}
	r, err := c.db.Reader().GetRender(ctx, renderID)
	if errors.Is(err, sql.ErrNoRows) {
		return Render{}, core.NotFound("render not found")
	}
	if err != nil {
		return Render{}, err
	}
	err = c.db.Writer().UpdateRender(ctx, store.UpdateRenderParams{
		Label: strings.TrimSpace(in.Label), Hidden: boolInt(in.Hidden), ID: r.ID,
	})
	if err != nil {
		return Render{}, err
	}
	all, err := c.renders(ctx, c.db.Reader(), r.ProjectID)
	if err != nil {
		return Render{}, err
	}
	for _, x := range all {
		if x.ID == r.ID {
			return x, nil
		}
	}
	return Render{}, core.NotFound("render not found")
}

// UploadedAd is one finished picture from the upload screen. The browser has
// already resized it to its exact size and made the preview.
type UploadedAd struct {
	Concept string `json:"concept"`
	Slug    string `json:"slug"`
	Lang    Lang   `json:"lang"`
	Fmt     Fmt    `json:"fmt"`
	CTA     bool   `json:"cta"`
	File    string `json:"file"`
	Preview string `json:"preview"`
}

type UploadResult struct {
	Concepts int `json:"concepts"` // new concepts created
	Ads      int `json:"ads"`      // new ads created
	Versions int `json:"versions"` // pictures added, including new versions of existing ads
}

// AddUploads saves finished ads. A concept that doesn't exist yet is
// created. Uploading an ad that exists adds a new version, so its review
// starts again.
func (c *Studio) AddUploads(ctx context.Context, actor core.Actor, projectID int64, in []UploadedAd) (UploadResult, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return UploadResult{}, core.ErrForbidden
	}
	if len(in) == 0 || len(in) > 500 {
		return UploadResult{}, core.Invalid("upload between 1 and 500 ads at a time")
	}
	settings, err := c.settings(ctx, c.db.Reader(), projectID)
	if err != nil {
		return UploadResult{}, err
	}
	type item struct {
		spec          adSpec
		file, preview store.File
	}
	items := make([]item, len(in))
	seen := map[string]bool{}
	for i, u := range in {
		spec := adSpec{concept: strings.ToUpper(u.Concept), slug: strings.ToUpper(u.Slug), lang: u.Lang, fmt: u.Fmt, cta: u.CTA}
		if err := spec.check(); err != nil {
			return UploadResult{}, err
		}
		if seen[spec.code()] {
			return UploadResult{}, core.Invalid("%s is in this upload twice", spec.code())
		}
		seen[spec.code()] = true
		file, err := c.adPicture(ctx, u.File, spec.fmt)
		if err != nil {
			return UploadResult{}, err
		}
		preview, err := c.picture(ctx, u.Preview)
		if err != nil {
			return UploadResult{}, err
		}
		items[i] = item{spec, file, preview}
	}
	// Board order: concept, then language, size and CTA.
	slices.SortStableFunc(items, func(a, b item) int {
		if a.spec.concept != b.spec.concept {
			return strings.Compare(a.spec.concept, b.spec.concept)
		}
		return a.spec.sortKey() - b.spec.sortKey()
	})

	var res UploadResult
	err = c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		conceptIDs := map[string]int64{}
		for _, it := range items {
			id, created, err := c.ensureConcept(ctx, q, projectID, it.spec)
			if err != nil {
				return err
			}
			conceptIDs[it.spec.concept] = id
			if created {
				res.Concepts++
			}
		}
		for _, it := range items {
			ad, created, err := c.upsertAd(ctx, q, projectID, conceptIDs[it.spec.concept], settings.Prefix, it.spec, "", sql.NullString{})
			if err != nil {
				return err
			}
			if created {
				res.Ads++
			}
			if _, err := c.addVersion(ctx, q, actor, ad.ID, it.file.ID, it.preview.ID, sql.NullString{}, "upload"); err != nil {
				return err
			}
			res.Versions++
		}
		return q.TouchCreativeProject(ctx, store.TouchCreativeProjectParams{UpdatedAt: c.now(), ProjectID: projectID})
	})
	if err != nil {
		return UploadResult{}, err
	}
	return res, nil
}

type GeneratedAd struct {
	Fmt        Fmt             `json:"fmt"`
	Lang       Lang            `json:"lang"`
	CTA        bool            `json:"cta"`
	Slug       string          `json:"slug"`
	RenderCode string          `json:"renderCode"`
	Design     json.RawMessage `json:"design"`
}

type GenerateResult struct {
	Ad Ad `json:"ad"`
	// HadPicture means the ad already had a published version, which the
	// caller only replaces when asked to.
	HadPicture bool `json:"hadPicture"`
}

// SaveGenerated creates or updates one ad laid out from a concept's copy,
// saving its layout as the draft. Publishing the picture is a separate step.
func (c *Studio) SaveGenerated(ctx context.Context, actor core.Actor, conceptID int64, in GeneratedAd) (GenerateResult, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return GenerateResult{}, core.ErrForbidden
	}
	k, err := c.concept(ctx, conceptID)
	if err != nil {
		return GenerateResult{}, err
	}
	spec := adSpec{concept: k.Code, slug: strings.ToUpper(in.Slug), lang: in.Lang, fmt: in.Fmt, cta: in.CTA}
	if err := spec.check(); err != nil {
		return GenerateResult{}, err
	}
	if err := checkRender(in.RenderCode); err != nil {
		return GenerateResult{}, err
	}
	if err := checkDoc("design", in.Design); err != nil {
		return GenerateResult{}, err
	}
	settings, err := c.settings(ctx, c.db.Reader(), k.ProjectID)
	if err != nil {
		return GenerateResult{}, err
	}
	var adID int64
	var had bool
	err = c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		ad, _, err := c.upsertAd(ctx, q, k.ProjectID, k.ID, settings.Prefix, spec, in.RenderCode, sql.NullString{String: string(in.Design), Valid: true})
		if err != nil {
			return err
		}
		adID = ad.ID
		_, err = q.LatestVersion(ctx, ad.ID)
		had = err == nil
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		return err
	})
	if err != nil {
		return GenerateResult{}, err
	}
	ad, err := c.ad(ctx, k.ProjectID, adID)
	if err != nil {
		return GenerateResult{}, err
	}
	return GenerateResult{Ad: ad, HadPicture: had}, nil
}

// AdDetail is what the editor opens with.
type AdDetail struct {
	Ad       Ad              `json:"ad"`
	Concept  Concept         `json:"concept"`
	Settings Settings        `json:"settings"`
	Draft    json.RawMessage `json:"draft"`  // null when no layout was saved
	Design   json.RawMessage `json:"design"` // the current version's layout, null for an uploaded picture
	Siblings []Sibling       `json:"siblings"`
	Renders  []Render        `json:"renders"`
}

// Sibling is the same concept, language and CTA variant in another size.
type Sibling struct {
	ID       int64 `json:"id"`
	Fmt      Fmt   `json:"fmt"`
	HasDraft bool  `json:"hasDraft"`
}

func (c *Studio) Ad(ctx context.Context, actor core.Actor, adID int64) (AdDetail, error) {
	if !actor.AtLeast(core.RoleViewer) {
		return AdDetail{}, core.ErrForbidden
	}
	q := c.db.Reader()
	row, err := c.adRow(ctx, adID)
	if err != nil {
		return AdDetail{}, err
	}
	ad, err := c.ad(ctx, row.ProjectID, adID)
	if err != nil {
		return AdDetail{}, err
	}
	k, err := q.GetConcept(ctx, row.ConceptID)
	if err != nil {
		return AdDetail{}, err
	}
	settings, err := c.settings(ctx, q, row.ProjectID)
	if err != nil {
		return AdDetail{}, err
	}
	out := AdDetail{Ad: ad, Concept: conceptFrom(k), Settings: settings, Siblings: []Sibling{}}
	if row.Draft.Valid {
		out.Draft = json.RawMessage(row.Draft.String)
	}
	if v, err := q.LatestVersion(ctx, adID); err == nil && v.Design.Valid {
		out.Design = json.RawMessage(v.Design.String)
	}
	sibs, err := q.ListSiblingAds(ctx, store.ListSiblingAdsParams{
		ConceptID: row.ConceptID, Lang: row.Lang, HasCta: row.HasCta, ID: row.ID,
	})
	if err != nil {
		return AdDetail{}, err
	}
	for _, s := range sibs {
		out.Siblings = append(out.Siblings, Sibling{ID: s.ID, Fmt: Fmt(s.Fmt), HasDraft: s.Draft.Valid})
	}
	if out.Renders, err = c.renders(ctx, q, row.ProjectID); err != nil {
		return AdDetail{}, err
	}
	return out, nil
}

type AdDraft struct {
	Design     json.RawMessage `json:"design"`
	RenderCode string          `json:"renderCode"`
}

// SaveDraft keeps the editor's layers without changing what reviewers see.
func (c *Studio) SaveDraft(ctx context.Context, actor core.Actor, adID int64, in AdDraft) error {
	if !actor.AtLeast(core.RoleMarketer) {
		return core.ErrForbidden
	}
	if err := checkDoc("design", in.Design); err != nil {
		return err
	}
	if err := checkRender(in.RenderCode); err != nil {
		return err
	}
	row, err := c.adRow(ctx, adID)
	if err != nil {
		return err
	}
	return c.db.Writer().SaveAdDraft(ctx, store.SaveAdDraftParams{
		Draft: sql.NullString{String: string(in.Design), Valid: true}, RenderCode: in.RenderCode, UpdatedAt: c.now(), ID: row.ID,
	})
}

type PublishInput struct {
	File       string          `json:"file"`
	Preview    string          `json:"preview"`
	Design     json.RawMessage `json:"design"`
	RenderCode string          `json:"renderCode"`
	// Source is "editor" or "generated".
	Source string `json:"source"`
}

// Publish adds a picture the browser drew from a layout as the ad's next
// version. Reviews of the previous version stay with it, so the ad is
// waiting again.
func (c *Studio) Publish(ctx context.Context, actor core.Actor, adID int64, in PublishInput) (Ad, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return Ad{}, core.ErrForbidden
	}
	if in.Source != "editor" && in.Source != "generated" {
		return Ad{}, core.Invalid("source must be editor or generated")
	}
	if err := checkDoc("design", in.Design); err != nil {
		return Ad{}, err
	}
	if err := checkRender(in.RenderCode); err != nil {
		return Ad{}, err
	}
	row, err := c.adRow(ctx, adID)
	if err != nil {
		return Ad{}, err
	}
	file, err := c.adPicture(ctx, in.File, Fmt(row.Fmt))
	if err != nil {
		return Ad{}, err
	}
	preview, err := c.picture(ctx, in.Preview)
	if err != nil {
		return Ad{}, err
	}
	design := sql.NullString{String: string(in.Design), Valid: true}
	err = c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		if err := q.SaveAdDraft(ctx, store.SaveAdDraftParams{Draft: design, RenderCode: in.RenderCode, UpdatedAt: c.now(), ID: row.ID}); err != nil {
			return err
		}
		if _, err := c.addVersion(ctx, q, actor, row.ID, file.ID, preview.ID, design, in.Source); err != nil {
			return err
		}
		return q.TouchCreativeProject(ctx, store.TouchCreativeProjectParams{UpdatedAt: c.now(), ProjectID: row.ProjectID})
	})
	if err != nil {
		return Ad{}, err
	}
	return c.ad(ctx, row.ProjectID, row.ID)
}

type VariationInput struct {
	RenderCode string          `json:"renderCode"`
	Design     json.RawMessage `json:"design"`
}

// CreateVariation makes a new ad with the same layout on another render,
// named with the render's code, such as C1_CASH_R07_EN_9x16. The caller
// publishes its picture.
func (c *Studio) CreateVariation(ctx context.Context, actor core.Actor, adID int64, in VariationInput) (Ad, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return Ad{}, core.ErrForbidden
	}
	if err := checkDoc("design", in.Design); err != nil {
		return Ad{}, err
	}
	row, err := c.adRow(ctx, adID)
	if err != nil {
		return Ad{}, err
	}
	render, err := c.db.Reader().GetRenderByCode(ctx, store.GetRenderByCodeParams{ProjectID: row.ProjectID, Code: in.RenderCode})
	if errors.Is(err, sql.ErrNoRows) {
		return Ad{}, core.NotFound("render %s isn't in this project", in.RenderCode)
	}
	if err != nil {
		return Ad{}, err
	}
	k, err := c.db.Reader().GetConcept(ctx, row.ConceptID)
	if err != nil {
		return Ad{}, err
	}
	slug := slugOf(row.Code, k.Code)
	if slug == "" {
		return Ad{}, core.Invalid("this ad's name can't be extended with a render")
	}
	slug = renderSuffix.ReplaceAllString(slug, "") + "_" + strings.ToUpper(strings.ReplaceAll(render.Code, "-", "_"))
	spec := adSpec{concept: k.Code, slug: slug, lang: Lang(row.Lang), fmt: Fmt(row.Fmt), cta: row.HasCta == 1}
	if err := spec.check(); err != nil {
		return Ad{}, err
	}
	settings, err := c.settings(ctx, c.db.Reader(), row.ProjectID)
	if err != nil {
		return Ad{}, err
	}
	var newID int64
	err = c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		ad, _, err := c.upsertAd(ctx, q, row.ProjectID, row.ConceptID, settings.Prefix, spec, render.Code,
			sql.NullString{String: string(in.Design), Valid: true})
		newID = ad.ID
		return err
	})
	if err != nil {
		return Ad{}, err
	}
	return c.ad(ctx, row.ProjectID, newID)
}

// ResolveComment marks a comment as dealt with, or reopens it.
func (c *Studio) ResolveComment(ctx context.Context, actor core.Actor, commentID int64, resolved bool) error {
	if !actor.AtLeast(core.RoleMarketer) {
		return core.ErrForbidden
	}
	cm, err := c.db.Reader().GetComment(ctx, commentID)
	if errors.Is(err, sql.ErrNoRows) {
		return core.NotFound("comment not found")
	}
	if err != nil {
		return err
	}
	p := store.SetCommentResolvedParams{ID: cm.ID}
	if resolved {
		p.ResolvedAt = sql.NullTime{Time: c.now(), Valid: true}
		p.ResolvedBy = userID(actor)
	}
	return c.db.Writer().SetCommentResolved(ctx, p)
}

// DownloadEntry is one file in a download, at its path inside the zip.
type DownloadEntry struct {
	Path   string
	SHA256 string
}

// Download lists the current pictures to zip: every ad, or only approved
// ones. Folders are the concept, then the size, as in Creative Studio.
func (c *Studio) Download(ctx context.Context, actor core.Actor, projectID int64, approvedOnly bool) (string, []DownloadEntry, error) {
	if !actor.AtLeast(core.RoleViewer) {
		return "", nil, core.ErrForbidden
	}
	p, err := c.project(ctx, c.db.Reader(), projectID)
	if err != nil {
		return "", nil, err
	}
	var out []DownloadEntry
	for _, k := range p.Concepts {
		folder := safeFolder(k.Code + " " + k.Name)
		for _, a := range k.Ads {
			if a.Current == nil || (approvedOnly && a.Status != StatusApproved) {
				continue
			}
			out = append(out, DownloadEntry{
				Path:   folder + "/" + sizes[a.Fmt].fileToken + "/" + a.FileName,
				SHA256: a.Current.File.SHA256,
			})
		}
	}
	set := "all"
	if approvedOnly {
		set = "approved"
	}
	name := strings.ToLower(p.Settings.Prefix) + "-" + set + "-" + c.now().Format("2006-01-02") + ".zip"
	return name, out, nil
}

func (c *Studio) adRow(ctx context.Context, id int64) (store.CreativeAd, error) {
	row, err := c.db.Reader().GetAd(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, core.NotFound("ad not found")
	}
	return row, err
}

func (c *Studio) ad(ctx context.Context, projectID, adID int64) (Ad, error) {
	ads, err := c.ads(ctx, c.db.Reader(), projectID)
	if err != nil {
		return Ad{}, err
	}
	for _, a := range ads {
		if a.ID == adID {
			return a.Ad, nil
		}
	}
	return Ad{}, core.NotFound("ad not found")
}

func (c *Studio) ensureConcept(ctx context.Context, q *store.Queries, projectID int64, spec adSpec) (int64, bool, error) {
	k, err := q.GetConceptByCode(ctx, store.GetConceptByCodeParams{ProjectID: projectID, Code: spec.concept})
	if err == nil {
		return k.ID, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	pos, err := q.MaxConceptPos(ctx, projectID)
	if err != nil {
		return 0, false, err
	}
	now := c.now()
	k, err = q.InsertConcept(ctx, store.InsertConceptParams{
		ProjectID: projectID, Code: spec.concept, Pos: pos + 1, Name: nameFromSlug(spec.slug), CreatedAt: now, UpdatedAt: now,
	})
	return k.ID, true, err
}

// upsertAd finds the ad with spec's code or creates it. draft replaces the
// saved layout when it's set.
func (c *Studio) upsertAd(ctx context.Context, q *store.Queries, projectID, conceptID int64, prefix string, spec adSpec, render string, draft sql.NullString) (store.CreativeAd, bool, error) {
	now := c.now()
	existing, err := q.GetAdByCode(ctx, store.GetAdByCodeParams{ProjectID: projectID, Code: spec.code()})
	if err == nil {
		if render == "" {
			render = existing.RenderCode
		}
		if err := q.UpdateAdShape(ctx, store.UpdateAdShapeParams{
			ConceptID: conceptID, FileName: existing.FileName, Fmt: string(spec.fmt), Lang: string(spec.lang),
			HasCta: boolInt(spec.cta), RenderCode: render, UpdatedAt: now, ID: existing.ID,
		}); err != nil {
			return existing, false, err
		}
		if draft.Valid {
			if err := q.SaveAdDraft(ctx, store.SaveAdDraftParams{Draft: draft, RenderCode: render, UpdatedAt: now, ID: existing.ID}); err != nil {
				return existing, false, err
			}
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return store.CreativeAd{}, false, err
	}
	pos, err := q.MaxAdPos(ctx, conceptID)
	if err != nil {
		return store.CreativeAd{}, false, err
	}
	ad, err := q.InsertAd(ctx, store.InsertAdParams{
		ProjectID: projectID, ConceptID: conceptID, Code: spec.code(), FileName: spec.fileName(prefix),
		Pos: pos + 1, Fmt: string(spec.fmt), Lang: string(spec.lang), HasCta: boolInt(spec.cta),
		RenderCode: render, Draft: draft, CreatedAt: now, UpdatedAt: now,
	})
	return ad, true, err
}

func (c *Studio) addVersion(ctx context.Context, q *store.Queries, actor core.Actor, adID, fileID, previewID int64, design sql.NullString, source string) (store.CreativeAdVersion, error) {
	next := int64(1)
	if v, err := q.LatestVersion(ctx, adID); err == nil {
		next = v.Version + 1
	} else if !errors.Is(err, sql.ErrNoRows) {
		return store.CreativeAdVersion{}, err
	}
	return q.InsertVersion(ctx, store.InsertVersionParams{
		AdID: adID, Version: next, FileID: fileID, PreviewFileID: previewID, Design: design,
		Source: source, CreatedBy: userID(actor), CreatedAt: c.now(),
	})
}

// picture checks that sum is an uploaded raster picture.
func (c *Studio) picture(ctx context.Context, sum string) (store.File, error) {
	f, err := c.fileID(ctx, c.db.Reader(), sum)
	if err != nil {
		return f, err
	}
	switch f.ContentType {
	case "image/jpeg", "image/png", "image/webp":
		return f, nil
	}
	return f, core.Invalid("file %s isn't a JPG, PNG or WebP picture", short(sum))
}

// adPicture checks that sum is a picture at exactly fmt's size, so the
// board and the ad platforms get what the name says.
func (c *Studio) adPicture(ctx context.Context, sum string, f Fmt) (store.File, error) {
	file, err := c.picture(ctx, sum)
	if err != nil {
		return file, err
	}
	s := sizes[f]
	if file.ContentType != "image/jpeg" || !file.Width.Valid || file.Width.Int64 != s.width || file.Height.Int64 != s.height {
		return file, core.Invalid("an ad in %s must be a %d×%d JPG", f, s.width, s.height)
	}
	return file, nil
}

func (s adSpec) check() error {
	if !conceptCode.MatchString(s.concept) {
		return core.Invalid("%q isn't a concept id like C1, O2 or T1AR", s.concept)
	}
	if !slugPattern.MatchString(s.slug) || len(s.slug) > 40 {
		return core.Invalid("%q isn't a name like CASH or RETAIL_2080", s.slug)
	}
	if !s.lang.valid() {
		return core.Invalid("language must be en or ar")
	}
	if !s.fmt.valid() {
		return core.Invalid("size must be 9:16, 4:5, 1:1 or 1.91:1")
	}
	return nil
}

func (s adSpec) sortKey() int {
	k := slices.Index(fmtOrder, s.fmt) * 2
	if s.lang == LangAR {
		k += 100
	}
	if s.cta {
		k++
	}
	return k
}

func checkRender(code string) error {
	if code != "" && !renderCode.MatchString(code) {
		return core.Invalid("%q isn't a render code like R01", code)
	}
	return nil
}

// slugOf reads the slug back out of an ad code: C1_CASH_EN_9x16_CTA -> CASH.
func slugOf(code, concept string) string {
	rest, ok := strings.CutPrefix(code, concept+"_")
	if !ok {
		return ""
	}
	rest = strings.TrimSuffix(rest, "_CTA")
	for _, s := range sizes {
		rest = strings.TrimSuffix(rest, "_"+s.idToken)
	}
	rest = strings.TrimSuffix(strings.TrimSuffix(rest, "_EN"), "_AR")
	return rest
}

func nameFromSlug(slug string) string {
	words := strings.Split(slug, "_")
	for i, w := range words {
		if w != "" {
			words[i] = w[:1] + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}

func safeFolder(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ' ' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "Untitled"
	}
	return out
}
