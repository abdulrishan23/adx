package studio

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

// The review board is public: anyone with the link reads it and can post,
// without an account. These limits are Creative Studio's, per project.
const (
	maxCommentsPerMinute  = 40
	maxComments           = 5000
	maxDecisionsPerMinute = 240
	maxDecisions          = 20000
	maxPicksPerMinute     = 60
	maxPicks              = 5000
)

var keyPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Board is everything a reviewer sees. It carries no ids beyond codes, so
// nothing on it leads back into the signed-in app.
type Board struct {
	ProjectName string          `json:"projectName"`
	Kicker      string          `json:"kicker"`
	PageName    string          `json:"pageName"`
	BrandKit    json.RawMessage `json:"brandKit"`
	Concepts    []BoardConcept  `json:"concepts"`
	Renders     []BoardRender   `json:"renders"`
	Picks       []Pick          `json:"picks"`
	Comments    []Comment       `json:"comments"`
	Decisions   []Decision      `json:"decisions"`
}

type BoardConcept struct {
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Why         string    `json:"why"`
	PrimaryText string    `json:"primaryText"`
	Headline    string    `json:"headline"`
	Description string    `json:"description"`
	CTA         string    `json:"cta"`
	Ads         []BoardAd `json:"ads"`
}

type BoardAd struct {
	Code       string       `json:"code"`
	FileName   string       `json:"fileName"`
	Fmt        Fmt          `json:"fmt"`
	Lang       Lang         `json:"lang"`
	HasCTA     bool         `json:"hasCta"`
	RenderCode string       `json:"renderCode"`
	Version    int64        `json:"version"`
	File       FileRef      `json:"file"`
	Preview    FileRef      `json:"preview"`
	Status     ReviewStatus `json:"status"`
}

type BoardRender struct {
	Code    string  `json:"code"`
	Label   string  `json:"label"`
	File    FileRef `json:"file"`
	Preview FileRef `json:"preview"`
}

// Board loads the review board behind key. Ads without a published picture
// aren't on it.
func (c *Studio) Board(ctx context.Context, actor core.Actor, key string) (Board, error) {
	projectID, err := c.boardProject(ctx, key)
	if err != nil {
		return Board{}, err
	}
	p, err := c.project(ctx, c.db.Reader(), projectID)
	if err != nil {
		return Board{}, err
	}
	out := Board{
		ProjectName: p.Settings.ProjectName, Kicker: p.Settings.Kicker, PageName: p.Settings.PageName,
		BrandKit: p.Settings.BrandKit, Concepts: []BoardConcept{}, Renders: []BoardRender{},
		Picks: p.Picks, Comments: p.Comments, Decisions: []Decision{},
	}
	for _, k := range p.Concepts {
		bc := BoardConcept{
			Code: k.Code, Name: k.Name, Why: k.Why, PrimaryText: k.PrimaryText,
			Headline: k.Headline, Description: k.Description, CTA: k.CTA, Ads: []BoardAd{},
		}
		for _, a := range k.Ads {
			if a.Current == nil {
				continue
			}
			bc.Ads = append(bc.Ads, BoardAd{
				Code: a.Code, FileName: a.FileName, Fmt: a.Fmt, Lang: a.Lang, HasCTA: a.HasCTA,
				RenderCode: a.RenderCode, Version: a.Current.Version, File: a.Current.File,
				Preview: a.Current.Preview, Status: a.Status,
			})
			out.Decisions = append(out.Decisions, a.Decisions...)
		}
		if len(bc.Ads) > 0 {
			out.Concepts = append(out.Concepts, bc)
		}
	}
	for _, r := range p.Renders {
		if !r.Hidden {
			out.Renders = append(out.Renders, BoardRender{Code: r.Code, Label: r.Label, File: r.File, Preview: r.Preview})
		}
	}
	return out, nil
}

type BoardDecision struct {
	Ad     string `json:"ad"` // the ad's code
	Name   string `json:"name"`
	Status string `json:"status"` // approved, revise or clear
}

// Decide records a reviewer's decision on an ad's current version.
func (c *Studio) Decide(ctx context.Context, actor core.Actor, key string, in BoardDecision) error {
	name, err := reviewerName(in.Name)
	if err != nil {
		return err
	}
	if in.Status != "approved" && in.Status != "revise" && in.Status != "clear" {
		return core.Invalid("that decision isn't recognised")
	}
	projectID, err := c.boardProject(ctx, key)
	if err != nil {
		return err
	}
	return c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		v, err := currentVersion(ctx, q, projectID, in.Ad)
		if err != nil {
			return err
		}
		n, err := q.CountDecisions(ctx, store.CountDecisionsParams{Since: c.now().Add(-time.Minute), ProjectID: projectID})
		if err != nil {
			return err
		}
		if err := underLimits(n.Recent, maxDecisionsPerMinute, n.Total, maxDecisions); err != nil {
			return err
		}
		return q.InsertDecision(ctx, store.InsertDecisionParams{
			VersionID: v.ID, Name: name, Status: in.Status, UserID: userID(actor), CreatedAt: c.now(),
		})
	})
}

type CommentInput struct {
	Ad   string   `json:"ad"`
	Name string   `json:"name"`
	Body string   `json:"body"`
	X    *float64 `json:"x"`
	Y    *float64 `json:"y"`
}

// Comment adds a comment to an ad, optionally pinned to a spot on its
// current picture.
func (c *Studio) Comment(ctx context.Context, actor core.Actor, key string, in CommentInput) (Comment, error) {
	name, err := reviewerName(in.Name)
	if err != nil {
		return Comment{}, err
	}
	body := strings.TrimSpace(in.Body)
	if body == "" || utf8.RuneCountInString(body) > 1200 {
		return Comment{}, core.Invalid("a comment needs 1 to 1,200 characters")
	}
	if (in.X == nil) != (in.Y == nil) || (in.X != nil && (*in.X < 0 || *in.X > 1 || *in.Y < 0 || *in.Y > 1)) {
		return Comment{}, core.Invalid("that pin is off the picture")
	}
	projectID, err := c.boardProject(ctx, key)
	if err != nil {
		return Comment{}, err
	}
	var out Comment
	err = c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		v, err := currentVersion(ctx, q, projectID, in.Ad)
		if err != nil {
			return err
		}
		n, err := q.CountComments(ctx, store.CountCommentsParams{Since: c.now().Add(-time.Minute), ProjectID: projectID})
		if err != nil {
			return err
		}
		if err := underLimits(n.Recent, maxCommentsPerMinute, n.Total, maxComments); err != nil {
			return err
		}
		p := store.InsertCommentParams{
			AdID: v.AdID, VersionID: v.ID, Name: name, Body: body, UserID: userID(actor), CreatedAt: c.now(),
		}
		if in.X != nil {
			p.PinX = sql.NullFloat64{Float64: *in.X, Valid: true}
			p.PinY = sql.NullFloat64{Float64: *in.Y, Valid: true}
		}
		row, err := q.InsertComment(ctx, p)
		if err != nil {
			return err
		}
		out = Comment{ID: row.ID, Ad: in.Ad, Version: v.Version, Name: name, Body: body, X: in.X, Y: in.Y, CreatedAt: row.CreatedAt}
		return nil
	})
	return out, err
}

type PickInput struct {
	Concept string `json:"concept"`
	Render  string `json:"render"` // empty clears the pick
	Name    string `json:"name"`
}

// PickRender records which render a reviewer wants a concept rebuilt on.
func (c *Studio) PickRender(ctx context.Context, actor core.Actor, key string, in PickInput) error {
	name, err := reviewerName(in.Name)
	if err != nil {
		return err
	}
	projectID, err := c.boardProject(ctx, key)
	if err != nil {
		return err
	}
	return c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		k, err := q.GetConceptByCode(ctx, store.GetConceptByCodeParams{ProjectID: projectID, Code: in.Concept})
		if errors.Is(err, sql.ErrNoRows) {
			return core.NotFound("that creative isn't on this board")
		}
		if err != nil {
			return err
		}
		if in.Render != "" {
			r, err := q.GetRenderByCode(ctx, store.GetRenderByCodeParams{ProjectID: projectID, Code: in.Render})
			if errors.Is(err, sql.ErrNoRows) || (err == nil && r.Hidden == 1) {
				return core.NotFound("that render isn't on this board")
			}
			if err != nil {
				return err
			}
		}
		n, err := q.CountPicks(ctx, store.CountPicksParams{Since: c.now().Add(-time.Minute), ProjectID: projectID})
		if err != nil {
			return err
		}
		if err := underLimits(n.Recent, maxPicksPerMinute, n.Total, maxPicks); err != nil {
			return err
		}
		return q.InsertPick(ctx, store.InsertPickParams{ConceptID: k.ID, RenderCode: in.Render, Name: name, CreatedAt: c.now()})
	})
}

func (c *Studio) boardProject(ctx context.Context, key string) (int64, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	if len(key) < 12 || len(key) > 60 || !keyPattern.MatchString(key) {
		return 0, core.NotFound("this review link isn't complete")
	}
	p, err := c.db.Reader().GetCreativeProjectByKey(ctx, key)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && p.ReviewActive == 0) {
		return 0, core.NotFound("this review link isn't active")
	}
	if err != nil {
		return 0, err
	}
	return p.ProjectID, nil
}

func currentVersion(ctx context.Context, q *store.Queries, projectID int64, code string) (store.CreativeAdVersion, error) {
	ad, err := q.GetAdByCode(ctx, store.GetAdByCodeParams{ProjectID: projectID, Code: code})
	if errors.Is(err, sql.ErrNoRows) {
		return store.CreativeAdVersion{}, core.NotFound("that creative isn't on this board")
	}
	if err != nil {
		return store.CreativeAdVersion{}, err
	}
	v, err := q.LatestVersion(ctx, ad.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return v, core.NotFound("that creative isn't on this board")
	}
	return v, err
}

func reviewerName(raw string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")
	if name == "" || utf8.RuneCountInString(name) > 60 {
		return "", core.Invalid("add your name first, up to 60 characters")
	}
	return name, nil
}

func underLimits(recent, perMinute, total, cap int64) error {
	if recent >= perMinute {
		return core.Conflict("too many posts right now, so try again in a minute")
	}
	if total >= cap {
		return core.Conflict("this board is full")
	}
	return nil
}
