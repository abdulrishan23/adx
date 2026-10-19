// Package studio is the creative production screens' domain, ported from
// Creative Studio: projects' concepts and ads, renders, brand kits, the
// canvas editor's design files, and the public review board. It sits beside
// core/creative, the engine's brief-to-render pipeline; the two will meet
// when one of them proves itself (docs/build/decisions.md, BD-59).
//
// Every published picture of an ad is an immutable version. A review belongs
// to the version it was given on, so publishing a new version puts the ad
// back to waiting. Design files, concept copy and brand kits are JSON
// documents the web app owns; the server checks their size and shape only.
package studio

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg" // registers the decoders image.DecodeConfig uses
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/blobs"
)

const (
	maxUpload   = 25 << 20  // per file, as in Creative Studio
	maxDocBytes = 256 << 10 // a design file, concept copy or brand kit
)

type Studio struct {
	db    *store.DB
	blobs *blobs.Store
	now   func() time.Time
	gen   Generation
}

func New(db *store.DB, files *blobs.Store) *Studio {
	return &Studio{db: db, blobs: files, now: func() time.Time { return time.Now().UTC() }}
}

// FileRef points at an uploaded file. The web app loads it from
// /api/files/{sha256}.
type FileRef struct {
	SHA256      string `json:"sha256"`
	ContentType string `json:"contentType"`
	Width       *int64 `json:"width"`
	Height      *int64 `json:"height"`
}

// Settings is a project's creative setup.
type Settings struct {
	ProjectID    int64           `json:"projectId"`
	ProjectName  string          `json:"projectName"`
	Prefix       string          `json:"prefix"`
	Kicker       string          `json:"kicker"`
	PageName     string          `json:"pageName"`
	ReviewKey    string          `json:"reviewKey"`
	ReviewActive bool            `json:"reviewActive"`
	BrandKit     json.RawMessage `json:"brandKit"`
}

type Render struct {
	ID      int64   `json:"id"`
	Code    string  `json:"code"`
	Label   string  `json:"label"`
	Hidden  bool    `json:"hidden"`
	File    FileRef `json:"file"`
	Preview FileRef `json:"preview"`
}

type AdVersion struct {
	ID        int64     `json:"id"`
	Version   int64     `json:"version"`
	File      FileRef   `json:"file"`
	Preview   FileRef   `json:"preview"`
	Source    string    `json:"source"`
	Method    string    `json:"method"` // "", motion or model
	CreatedAt time.Time `json:"createdAt"`
}

// ReviewStatus is an ad's review state on its current version.
type ReviewStatus string

const (
	StatusUnpublished ReviewStatus = "unpublished" // no picture yet, only a layout
	StatusWaiting     ReviewStatus = "waiting"
	StatusApproved    ReviewStatus = "approved"
	StatusRevise      ReviewStatus = "revise"
)

type Decision struct {
	Ad        string    `json:"ad"` // the ad's code
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type Ad struct {
	ID         int64        `json:"id"`
	Code       string       `json:"code"`
	Kind       string       `json:"kind"` // static or video
	FileName   string       `json:"fileName"`
	Fmt        Fmt          `json:"fmt"`
	Lang       Lang         `json:"lang"`
	HasCTA     bool         `json:"hasCta"`
	RenderCode string       `json:"renderCode"`
	HasDraft   bool         `json:"hasDraft"`
	Current    *AdVersion   `json:"current"`
	Status     ReviewStatus `json:"status"`
	Decisions  []Decision   `json:"decisions"` // latest per reviewer, on the current version
	Comments   int          `json:"comments"`
}

type Concept struct {
	ID          int64           `json:"id"`
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Why         string          `json:"why"`
	PrimaryText string          `json:"primaryText"`
	Headline    string          `json:"headline"`
	Description string          `json:"description"`
	CTA         string          `json:"cta"`
	Copy        json.RawMessage `json:"copy"`
	Imagery     string          `json:"imagery"` // what to ask an image model for
	Ads         []Ad            `json:"ads"`
}

type Pick struct {
	Concept   string    `json:"concept"` // the concept's code
	Render    string    `json:"render"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type Comment struct {
	ID        int64     `json:"id"`
	Ad        string    `json:"ad"` // the ad's code
	Version   int64     `json:"version"`
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	X         *float64  `json:"x"`
	Y         *float64  `json:"y"`
	Resolved  bool      `json:"resolved"`
	CreatedAt time.Time `json:"createdAt"`
}

// Project is everything the creative screens show for one project.
type CreativeProject struct {
	Settings Settings  `json:"settings"`
	Concepts []Concept `json:"concepts"`
	Renders  []Render  `json:"renders"`
	Picks    []Pick    `json:"picks"`
	Comments []Comment `json:"comments"`
}

type ProjectSummary struct {
	ProjectID    int64     `json:"projectId"`
	Name         string    `json:"name"`
	Prefix       string    `json:"prefix"`
	ReviewActive bool      `json:"reviewActive"`
	Concepts     int64     `json:"concepts"`
	Ads          int64     `json:"ads"`
	Renders      int64     `json:"renders"`
	Approved     int       `json:"approved"`
	Revise       int       `json:"revise"`
	Waiting      int       `json:"waiting"`
	Unpublished  int       `json:"unpublished"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type NotSetUp struct {
	ProjectID int64  `json:"projectId"`
	Name      string `json:"name"`
}

type Overview struct {
	Projects []ProjectSummary `json:"projects"`
	NotSetUp []NotSetUp       `json:"notSetUp"`
}

// Overview lists projects with creatives, and projects that could have them.
func (c *Studio) Overview(ctx context.Context, actor core.Actor) (Overview, error) {
	if !actor.AtLeast(core.RoleViewer) {
		return Overview{}, core.ErrForbidden
	}
	q := c.db.Reader()
	rows, err := q.ListCreativeProjects(ctx, actor.WorkspaceID)
	if err != nil {
		return Overview{}, err
	}
	all, err := q.ListProjects(ctx, actor.WorkspaceID)
	if err != nil {
		return Overview{}, err
	}
	out := Overview{Projects: []ProjectSummary{}, NotSetUp: []NotSetUp{}}
	set := map[int64]bool{}
	for _, r := range rows {
		set[r.ProjectID] = true
		s := ProjectSummary{
			ProjectID: r.ProjectID, Name: r.Name, Prefix: r.Prefix, ReviewActive: r.ReviewActive == 1,
			Concepts: r.Concepts, Ads: r.Ads, Renders: r.Renders, UpdatedAt: r.UpdatedAt,
		}
		ads, err := c.ads(ctx, q, r.ProjectID)
		if err != nil {
			return Overview{}, err
		}
		for _, a := range ads {
			switch a.Status {
			case StatusApproved:
				s.Approved++
			case StatusRevise:
				s.Revise++
			case StatusWaiting:
				s.Waiting++
			case StatusUnpublished:
				s.Unpublished++
			}
		}
		out.Projects = append(out.Projects, s)
	}
	for _, p := range all {
		if !set[p.ID] {
			out.NotSetUp = append(out.NotSetUp, NotSetUp{ProjectID: p.ID, Name: p.Name})
		}
	}
	return out, nil
}

type SetupInput struct {
	ProjectID int64 `json:"projectId"`
}

// SetUp starts creatives for a project. The prefix comes from the project's
// name and can be changed in the settings before any ad exists.
func (c *Studio) SetUp(ctx context.Context, actor core.Actor, in SetupInput) (Settings, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return Settings{}, core.ErrForbidden
	}
	p, err := c.db.Reader().GetProject(ctx, store.GetProjectParams{ID: in.ProjectID, WorkspaceID: actor.WorkspaceID})
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{}, core.NotFound("project not found")
	}
	if err != nil {
		return Settings{}, err
	}
	now := c.now()
	err = c.db.Writer().InsertCreativeProject(ctx, store.InsertCreativeProjectParams{
		ProjectID: p.ID, Prefix: prefixFrom(p.Name), PageName: "Reportage Properties",
		ReviewKey: newReviewKey(p.Name), CreatedAt: now, UpdatedAt: now,
	})
	if store.IsUniqueViolation(err) {
		return Settings{}, core.Conflict("%s already has creatives set up", p.Name)
	}
	if err != nil {
		return Settings{}, err
	}
	return c.settings(ctx, c.db.Reader(), p.ID)
}

type SettingsInput struct {
	Prefix       string `json:"prefix"`
	Kicker       string `json:"kicker"`
	PageName     string `json:"pageName"`
	ReviewActive bool   `json:"reviewActive"`
}

// UpdateSettings changes the file prefix, the board's wording, and whether
// its review link works. Pausing the link keeps every file and comment.
func (c *Studio) UpdateSettings(ctx context.Context, actor core.Actor, projectID int64, in SettingsInput) (Settings, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return Settings{}, core.ErrForbidden
	}
	prefix := strings.ToUpper(strings.TrimSpace(in.Prefix))
	if prefix == "" || prefixChars.MatchString(prefix) || len(prefix) > 16 {
		return Settings{}, core.Invalid("the file prefix is 1 to 16 capital letters or digits, such as MARLIN")
	}
	if err := maxRunes("kicker", in.Kicker, 120); err != nil {
		return Settings{}, err
	}
	page := strings.TrimSpace(in.PageName)
	if page == "" {
		return Settings{}, core.Invalid("the page name is required")
	}
	if err := maxRunes("page name", page, 80); err != nil {
		return Settings{}, err
	}
	cur, err := c.settings(ctx, c.db.Reader(), projectID)
	if err != nil {
		return Settings{}, err
	}
	if prefix != cur.Prefix {
		// Ads keep the file name they were made with, so a new prefix would
		// mix two names in one download.
		ads, err := c.db.Reader().ListAds(ctx, projectID)
		if err != nil {
			return Settings{}, err
		}
		if len(ads) > 0 {
			return Settings{}, core.Conflict("the prefix can't change once the project has ads")
		}
	}
	err = c.db.Writer().UpdateCreativeSettings(ctx, store.UpdateCreativeSettingsParams{
		Prefix: prefix, Kicker: strings.TrimSpace(in.Kicker), PageName: page,
		ReviewActive: boolInt(in.ReviewActive), UpdatedAt: c.now(), ProjectID: projectID,
	})
	if err != nil {
		return Settings{}, err
	}
	return c.settings(ctx, c.db.Reader(), projectID)
}

// RotateReviewKey gives the board a new link. The old one stops working.
func (c *Studio) RotateReviewKey(ctx context.Context, actor core.Actor, projectID int64) (Settings, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return Settings{}, core.ErrForbidden
	}
	cur, err := c.settings(ctx, c.db.Reader(), projectID)
	if err != nil {
		return Settings{}, err
	}
	err = c.db.Writer().SetReviewKey(ctx, store.SetReviewKeyParams{
		ReviewKey: newReviewKey(cur.ProjectName), UpdatedAt: c.now(), ProjectID: projectID,
	})
	if err != nil {
		return Settings{}, err
	}
	return c.settings(ctx, c.db.Reader(), projectID)
}

// SaveBrandKit stores the project's logos, fonts and colours.
func (c *Studio) SaveBrandKit(ctx context.Context, actor core.Actor, projectID int64, kit json.RawMessage) (Settings, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return Settings{}, core.ErrForbidden
	}
	if err := checkDoc("brand kit", kit); err != nil {
		return Settings{}, err
	}
	if _, err := c.settings(ctx, c.db.Reader(), projectID); err != nil {
		return Settings{}, err
	}
	err := c.db.Writer().SaveBrandKit(ctx, store.SaveBrandKitParams{
		BrandKit: string(kit), UpdatedAt: c.now(), ProjectID: projectID,
	})
	if err != nil {
		return Settings{}, err
	}
	return c.settings(ctx, c.db.Reader(), projectID)
}

// Project returns a project's concepts, ads, renders and review state.
func (c *Studio) Project(ctx context.Context, actor core.Actor, projectID int64) (CreativeProject, error) {
	if !actor.AtLeast(core.RoleViewer) {
		return CreativeProject{}, core.ErrForbidden
	}
	return c.project(ctx, c.db.Reader(), projectID)
}

func (c *Studio) project(ctx context.Context, q *store.Queries, projectID int64) (CreativeProject, error) {
	settings, err := c.settings(ctx, q, projectID)
	if err != nil {
		return CreativeProject{}, err
	}
	concepts, err := q.ListConcepts(ctx, projectID)
	if err != nil {
		return CreativeProject{}, err
	}
	ads, err := c.ads(ctx, q, projectID)
	if err != nil {
		return CreativeProject{}, err
	}
	byConcept := map[int64][]Ad{}
	for _, a := range ads {
		byConcept[a.conceptID] = append(byConcept[a.conceptID], a.Ad)
	}
	out := CreativeProject{Settings: settings, Concepts: []Concept{}}
	for _, k := range concepts {
		concept := conceptFrom(k)
		if list := byConcept[k.ID]; list != nil {
			concept.Ads = list
		}
		out.Concepts = append(out.Concepts, concept)
	}
	if out.Renders, err = c.renders(ctx, q, projectID); err != nil {
		return CreativeProject{}, err
	}
	if out.Picks, err = picks(ctx, q, projectID); err != nil {
		return CreativeProject{}, err
	}
	if out.Comments, err = c.comments(ctx, q, projectID, ads); err != nil {
		return CreativeProject{}, err
	}
	return out, nil
}

func (c *Studio) settings(ctx context.Context, q *store.Queries, projectID int64) (Settings, error) {
	r, err := q.GetCreativeProject(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{}, core.NotFound("this project has no creatives set up")
	}
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		ProjectID: r.ProjectID, ProjectName: r.ProjectName, Prefix: r.Prefix, Kicker: r.Kicker,
		PageName: r.PageName, ReviewKey: r.ReviewKey, ReviewActive: r.ReviewActive == 1,
		BrandKit: json.RawMessage(r.BrandKit),
	}, nil
}

// adRow is an Ad plus what the queries need to group and check it.
type adRow struct {
	Ad
	conceptID int64
	versionID int64
}

// ads lists a project's ads with their current version and review state.
func (c *Studio) ads(ctx context.Context, q *store.Queries, projectID int64) ([]adRow, error) {
	rows, err := q.ListAds(ctx, projectID)
	if err != nil {
		return nil, err
	}
	versions, err := q.ListLatestVersions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	decisions, err := q.ListCurrentDecisions(ctx, projectID)
	if err != nil {
		return nil, err
	}
	comments, err := q.ListComments(ctx, projectID)
	if err != nil {
		return nil, err
	}
	current := map[int64]store.CreativeAdVersion{}
	for _, v := range versions {
		current[v.AdID] = v
	}
	commentCount := map[int64]int{}
	for _, cm := range comments {
		commentCount[cm.AdID]++
	}
	codes := map[int64]string{}
	for _, r := range rows {
		codes[r.ID] = r.Code
	}
	latest := latestDecisions(decisions, codes)

	files := fileCache{q: q}
	out := make([]adRow, 0, len(rows))
	for _, r := range rows {
		a := adRow{conceptID: r.ConceptID, Ad: Ad{
			ID: r.ID, Code: r.Code, Kind: r.Kind, FileName: r.FileName, Fmt: Fmt(r.Fmt), Lang: Lang(r.Lang),
			HasCTA: r.HasCta == 1, RenderCode: r.RenderCode, HasDraft: r.Draft.Valid,
			Decisions: []Decision{}, Comments: commentCount[r.ID], Status: StatusUnpublished,
		}}
		if v, ok := current[r.ID]; ok {
			ver, err := files.version(ctx, v)
			if err != nil {
				return nil, err
			}
			a.Current = &ver
			a.versionID = v.ID
			if list := latest[r.ID]; list != nil {
				a.Decisions = list
			}
			a.Status = statusOf(a.Decisions)
		}
		out = append(out, a)
	}
	return out, nil
}

// latestDecisions keeps each reviewer's newest decision per ad, names
// compared without case, and drops cleared ones.
func latestDecisions(rows []store.ListCurrentDecisionsRow, codes map[int64]string) map[int64][]Decision {
	type key struct {
		ad   int64
		name string
	}
	newest := map[key]store.ListCurrentDecisionsRow{}
	order := []key{}
	for _, d := range rows { // oldest first
		k := key{d.AdID, strings.ToLower(d.Name)}
		if _, seen := newest[k]; !seen {
			order = append(order, k)
		}
		newest[k] = d
	}
	out := map[int64][]Decision{}
	for _, k := range order {
		d := newest[k]
		if d.Status == "clear" {
			continue
		}
		out[k.ad] = append(out[k.ad], Decision{Ad: codes[d.AdID], Name: d.Name, Status: d.Status, CreatedAt: d.CreatedAt})
	}
	return out
}

// statusOf applies the board's rule: anyone asking for changes outranks
// every approval, and no decision means waiting.
func statusOf(ds []Decision) ReviewStatus {
	status := StatusWaiting
	for _, d := range ds {
		switch d.Status {
		case "revise":
			return StatusRevise
		case "approved":
			status = StatusApproved
		}
	}
	return status
}

func (c *Studio) renders(ctx context.Context, q *store.Queries, projectID int64) ([]Render, error) {
	rows, err := q.ListRenders(ctx, projectID)
	if err != nil {
		return nil, err
	}
	files := fileCache{q: q}
	out := make([]Render, 0, len(rows))
	for _, r := range rows {
		file, err := files.get(ctx, r.FileID)
		if err != nil {
			return nil, err
		}
		preview, err := files.get(ctx, r.PreviewFileID)
		if err != nil {
			return nil, err
		}
		out = append(out, Render{ID: r.ID, Code: r.Code, Label: r.Label, Hidden: r.Hidden == 1, File: file, Preview: preview})
	}
	return out, nil
}

func picks(ctx context.Context, q *store.Queries, projectID int64) ([]Pick, error) {
	rows, err := q.ListLatestPicks(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := []Pick{}
	for _, p := range rows {
		if p.RenderCode == "" {
			continue // cleared
		}
		out = append(out, Pick{Concept: p.ConceptCode, Render: p.RenderCode, Name: p.Name, CreatedAt: p.CreatedAt})
	}
	return out, nil
}

func (c *Studio) comments(ctx context.Context, q *store.Queries, projectID int64, ads []adRow) ([]Comment, error) {
	rows, err := q.ListComments(ctx, projectID)
	if err != nil {
		return nil, err
	}
	codes := map[int64]string{}
	for _, a := range ads {
		codes[a.ID] = a.Code
	}
	out := make([]Comment, 0, len(rows))
	for _, r := range rows {
		out = append(out, Comment{
			ID: r.ID, Ad: codes[r.AdID], Version: r.Version, Name: r.Name, Body: r.Body,
			X: nullFloat(r.PinX), Y: nullFloat(r.PinY), Resolved: r.ResolvedAt.Valid, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

func conceptFrom(k store.CreativeConcept) Concept {
	return Concept{
		ID: k.ID, Code: k.Code, Name: k.Name, Why: k.Why, PrimaryText: k.PrimaryText,
		Headline: k.Headline, Description: k.Description, CTA: k.Cta, Copy: json.RawMessage(k.Copy), Imagery: k.Imagery, Ads: []Ad{},
	}
}

// fileCache resolves file ids to refs, reading each file row once.
type fileCache struct {
	q    *store.Queries
	seen map[int64]FileRef
}

func (f *fileCache) get(ctx context.Context, id int64) (FileRef, error) {
	if ref, ok := f.seen[id]; ok {
		return ref, nil
	}
	row, err := f.q.GetFile(ctx, id)
	if err != nil {
		return FileRef{}, err
	}
	ref := fileRef(row)
	if f.seen == nil {
		f.seen = map[int64]FileRef{}
	}
	f.seen[id] = ref
	return ref, nil
}

func (f *fileCache) version(ctx context.Context, v store.CreativeAdVersion) (AdVersion, error) {
	file, err := f.get(ctx, v.FileID)
	if err != nil {
		return AdVersion{}, err
	}
	preview, err := f.get(ctx, v.PreviewFileID)
	if err != nil {
		return AdVersion{}, err
	}
	return AdVersion{ID: v.ID, Version: v.Version, File: file, Preview: preview, Source: v.Source, Method: v.Method, CreatedAt: v.CreatedAt}, nil
}

func fileRef(f store.File) FileRef {
	ref := FileRef{SHA256: f.Sha256, ContentType: f.ContentType}
	if f.Width.Valid {
		ref.Width = &f.Width.Int64
	}
	if f.Height.Valid {
		ref.Height = &f.Height.Int64
	}
	return ref
}

// Upload stores a file and returns its reference. Only pictures and fonts
// are accepted, judged by their bytes rather than the name or the browser's
// claim.
func (c *Studio) Upload(ctx context.Context, actor core.Actor, r io.Reader) (FileRef, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return FileRef{}, core.ErrForbidden
	}
	sum, size, err := c.blobs.Put(r, maxUpload)
	if errors.Is(err, blobs.ErrTooLarge) {
		return FileRef{}, core.Invalid("files can be up to 25 MB")
	}
	if err != nil {
		return FileRef{}, err
	}
	if row, err := c.db.Reader().GetFileBySHA(ctx, sum); err == nil {
		return fileRef(row), nil
	}
	f, err := c.blobs.Open(sum)
	if err != nil {
		return FileRef{}, err
	}
	head := make([]byte, 4096)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	contentType := sniff(head)
	var width, height sql.NullInt64
	if contentType == "image/jpeg" || contentType == "image/png" {
		if _, err := f.Seek(0, io.SeekStart); err == nil {
			if cfg, _, err := image.DecodeConfig(f); err == nil {
				width = sql.NullInt64{Int64: int64(cfg.Width), Valid: true}
				height = sql.NullInt64{Int64: int64(cfg.Height), Valid: true}
			}
		}
	}
	f.Close()
	if contentType == "" {
		return FileRef{}, core.Invalid("upload a JPG, PNG, WebP or SVG picture, an MP4 video, or a TTF, OTF, WOFF or WOFF2 font")
	}
	row, err := c.db.Writer().InsertFile(ctx, store.InsertFileParams{
		Sha256: sum, ContentType: contentType, Bytes: size, Width: width, Height: height,
		CreatedBy: userID(actor), CreatedAt: c.now(),
	})
	if store.IsUniqueViolation(err) {
		// Someone uploaded the same bytes at the same moment.
		row, err = c.db.Reader().GetFileBySHA(ctx, sum)
	}
	if err != nil {
		return FileRef{}, err
	}
	return fileRef(row), nil
}

// OpenFile returns a stored file for download. Files are public to anyone
// with their SHA-256, like the public bucket they replace, because the
// review board shows them to people without accounts.
func (c *Studio) OpenFile(ctx context.Context, actor core.Actor, sum string) (io.ReadSeekCloser, FileRef, error) {
	row, err := c.db.Reader().GetFileBySHA(ctx, sum)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, FileRef{}, core.NotFound("file not found")
	}
	if err != nil {
		return nil, FileRef{}, err
	}
	f, err := c.blobs.Open(sum)
	if err != nil {
		return nil, FileRef{}, err
	}
	return f, fileRef(row), nil
}

// sniff names a picture or font from its first bytes, or returns "".
func sniff(head []byte) string {
	switch {
	case bytes.HasPrefix(head, []byte("wOFF")):
		return "font/woff"
	case bytes.HasPrefix(head, []byte("wOF2")):
		return "font/woff2"
	case bytes.HasPrefix(head, []byte("OTTO")):
		return "font/otf"
	case bytes.HasPrefix(head, []byte{0, 1, 0, 0}), bytes.HasPrefix(head, []byte("true")):
		return "font/ttf"
	}
	if len(head) >= 12 && bytes.Equal(head[4:8], []byte("ftyp")) {
		return "video/mp4"
	}
	switch ct := http.DetectContentType(head); ct {
	case "image/jpeg", "image/png", "image/webp":
		return ct
	}
	text := strings.ToLower(string(head))
	if utf8.Valid(head) && strings.Contains(text, "<svg") {
		return "image/svg+xml"
	}
	return ""
}

func (c *Studio) fileID(ctx context.Context, q *store.Queries, sum string) (store.File, error) {
	f, err := q.GetFileBySHA(ctx, sum)
	if errors.Is(err, sql.ErrNoRows) {
		return store.File{}, core.Invalid("file %s hasn't been uploaded", short(sum))
	}
	return f, err
}

func checkDoc(what string, doc json.RawMessage) error {
	if len(doc) > maxDocBytes {
		return core.Invalid("the %s is too large", what)
	}
	var v map[string]any
	if err := json.Unmarshal(doc, &v); err != nil || v == nil {
		return core.Invalid("the %s must be a JSON object", what)
	}
	return nil
}

func maxRunes(what, s string, n int) error {
	if utf8.RuneCountInString(s) > n {
		return core.Invalid("the %s can be up to %d characters", what, n)
	}
	return nil
}

// newReviewKey is the project's slug and ten characters that are hard to
// guess and easy to read aloud.
func newReviewKey(name string) string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return slugFrom(name) + "-" + string(b)
}

func userID(a core.Actor) sql.NullInt64 {
	return sql.NullInt64{Int64: a.UserID, Valid: a.UserID != 0}
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func nullFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func short(sum string) string {
	if len(sum) > 12 {
		return sum[:12]
	}
	return sum
}
