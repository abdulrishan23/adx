package studio

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/store"
)

// VideoRunKind is the run that makes one video ad version.
const VideoRunKind = "studio_video"

// Video is the two ways a video ad is made. Motion moves the project's
// pictures and ends on the finished static ad; a video model grows a clip
// from a picture and a prompt. The person picks per concept.
const (
	VideoMotion = "motion"
	VideoModel  = "model"
)

const (
	videoWidth, videoHeight = 1080, 1920
	videoSlugSuffix         = "_VIDEO"
	videoMotionStyle        = "Slow, steady camera: a gentle push in or drift across the scene, nothing fast. Photoreal, no text, no logos, no people's faces in close-up."
)

// MotionMaker builds a motion video from pictures (providers/ffmpeg).
type MotionMaker interface {
	Build(ctx context.Context, m MotionSpec) (mp4, poster []byte, err error)
	Poster(ctx context.Context, mp4 []byte) ([]byte, error)
}

type MotionSpec struct {
	Pictures [][]byte
	Card     []byte
	Width    int
	Height   int
}

// VideoMaker asks a video model for a clip (providers/google, Veo).
type VideoMaker interface {
	GenerateVideo(ctx context.Context, in ClipRequest) (Clip, error)
}

type ClipRequest struct {
	Prompt      string
	AspectRatio string
	Seconds     int
	Image       *PictureReference
}

type Clip struct {
	MIME string
	Data []byte
}

type VideoInput struct {
	Lang Lang   `json:"lang"`
	Mode string `json:"mode"` // motion or model
	// RenderCodes are the pictures to move, in order; empty means the
	// concept's render and the pictures Adx made for it.
	RenderCodes []string `json:"renderCodes"`
	Prompt      string   `json:"prompt"` // for the model; the concept's imagery by default
	Slug        string   `json:"slug"`   // the ad's name part; the concept's name by default
}

type videoParams struct {
	AdID        int64    `json:"adId"`
	ConceptID   int64    `json:"conceptId"`
	Mode        string   `json:"mode"`
	RenderCodes []string `json:"renderCodes"`
	Prompt      string   `json:"prompt"`
}

type StartedVideo struct {
	Ad      Ad       `json:"ad"`
	Run     runs.Run `json:"run"`
	Started bool     `json:"started"`
}

// StartVideo makes or finds the concept's video ad in a language and starts
// the run that makes its next version. The same ask twice while it runs
// returns the running run.
func (c *Studio) StartVideo(ctx context.Context, actor core.Actor, rs *runs.Runs, conceptID int64, in VideoInput) (StartedVideo, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return StartedVideo{}, core.ErrForbidden
	}
	if rs == nil {
		return StartedVideo{}, core.Conflict("background work is not configured on this server")
	}
	switch in.Mode {
	case VideoMotion:
		if c.gen.Motion == nil {
			return StartedVideo{}, core.Conflict("motion video is not available on this server; ffmpeg is missing")
		}
	case VideoModel:
		if c.gen.Video == nil {
			return StartedVideo{}, core.Conflict("no video model is configured on this server; ask an admin to configure ADX_GEMINI_API_KEY")
		}
	default:
		return StartedVideo{}, core.Invalid("mode must be motion or model")
	}
	if !in.Lang.valid() {
		return StartedVideo{}, core.Invalid("language must be en or ar")
	}
	k, err := c.concept(ctx, conceptID)
	if err != nil {
		return StartedVideo{}, err
	}
	settings, err := c.settings(ctx, c.db.Reader(), k.ProjectID)
	if err != nil {
		return StartedVideo{}, err
	}
	slug := strings.ToUpper(strings.TrimSpace(in.Slug))
	if slug == "" {
		slug = slugFromName(k.Name)
	}
	if !strings.HasSuffix(slug, videoSlugSuffix) {
		slug += videoSlugSuffix
	}
	spec := adSpec{concept: k.Code, slug: slug, lang: in.Lang, fmt: FmtStory, cta: false}
	if err := spec.check(); err != nil {
		return StartedVideo{}, err
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		prompt = strings.TrimSpace(k.Imagery)
	}
	if in.Mode == VideoModel && prompt == "" {
		return StartedVideo{}, core.Invalid("the concept has no imagery prompt; write one or ask Adx for concepts")
	}
	var ad store.CreativeAd
	err = c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		var err error
		ad, _, err = c.upsertAd(ctx, q, k.ProjectID, k.ID, settings.Prefix, spec, "", sql.NullString{})
		if err != nil {
			return err
		}
		if ad.Kind != "video" || !strings.HasSuffix(ad.FileName, ".mp4") {
			return q.SetAdKind(ctx, store.SetAdKindParams{Kind: "video", FileName: strings.TrimSuffix(ad.FileName, ".jpg") + ".mp4", UpdatedAt: c.now(), ID: ad.ID})
		}
		return nil
	})
	if err != nil {
		return StartedVideo{}, err
	}
	next := int64(1)
	if v, err := c.db.Reader().LatestVersion(ctx, ad.ID); err == nil {
		next = v.Version + 1
	}
	run, started, err := rs.Start(ctx, actor, runs.StartInput{
		Kind:          VideoRunKind,
		OccurrenceKey: fmt.Sprintf("studio-ad:%d/v%d", ad.ID, next),
		Params:        videoParams{AdID: ad.ID, ConceptID: k.ID, Mode: in.Mode, RenderCodes: in.RenderCodes, Prompt: prompt},
	})
	if err != nil {
		return StartedVideo{}, err
	}
	out, err := c.ad(ctx, k.ProjectID, ad.ID)
	if err != nil {
		return StartedVideo{}, err
	}
	return StartedVideo{Ad: out, Run: run, Started: started}, nil
}

// Execute works one video run. It is the jobs.Executor for VideoRunKind.
// The files are stored first; a crash leaves at most orphan files, and the
// retry makes the version again.
func (c *Studio) Execute(ctx context.Context, h *runs.Handle) (runs.Completion, error) {
	var p videoParams
	if err := json.Unmarshal(h.Run.Params, &p); err != nil || p.AdID == 0 {
		return runs.Completion{}, core.Invalid("video run has invalid parameters")
	}
	row, err := c.adRow(ctx, p.AdID)
	if err != nil {
		return runs.Completion{}, err
	}
	k, err := c.concept(ctx, p.ConceptID)
	if err != nil {
		return runs.Completion{}, err
	}
	pictures, err := c.videoPictures(ctx, row.ProjectID, k, p.RenderCodes)
	if err != nil {
		return runs.Completion{}, err
	}
	var mp4, poster []byte
	switch p.Mode {
	case VideoMotion:
		if c.gen.Motion == nil {
			return runs.Completion{}, core.Conflict("motion video is not available on this server")
		}
		card, err := c.staticCard(ctx, row)
		if err != nil {
			return runs.Completion{}, err
		}
		if len(pictures) == 0 && card == nil {
			return runs.Completion{}, core.Invalid("nothing to move: the project has no renders and the concept has no finished %s story ad", row.Lang)
		}
		mp4, poster, err = c.gen.Motion.Build(ctx, MotionSpec{Pictures: pictures, Card: card, Width: videoWidth, Height: videoHeight})
		if err != nil {
			return runs.Completion{}, core.Unavailable(err, "the video could not be built")
		}
	case VideoModel:
		if c.gen.Video == nil {
			return runs.Completion{}, core.Conflict("no video model is configured on this server")
		}
		req := ClipRequest{Prompt: p.Prompt + "\n\n" + videoMotionStyle, AspectRatio: "9:16", Seconds: 8}
		if len(pictures) > 0 {
			req.Image = &PictureReference{MIME: "image/jpeg", Data: pictures[0]}
		}
		clip, err := c.gen.Video.GenerateVideo(ctx, req)
		if err != nil {
			var pe *PictureError
			if errors.As(err, &pe) {
				return runs.Completion{}, core.Unavailable(err, "%s", pe.Safe)
			}
			return runs.Completion{}, err
		}
		mp4 = clip.Data
		if c.gen.Motion != nil {
			if poster, err = c.gen.Motion.Poster(ctx, mp4); err != nil {
				return runs.Completion{}, core.Unavailable(err, "the clip's poster could not be taken")
			}
		} else if len(pictures) > 0 {
			poster = pictures[0]
		} else {
			return runs.Completion{}, core.Conflict("a poster needs ffmpeg or a picture")
		}
	default:
		return runs.Completion{}, core.Invalid("video run has an unknown mode")
	}
	actor := h.Actor
	fileRef, err := c.Upload(ctx, actor, bytes.NewReader(mp4))
	if err != nil {
		return runs.Completion{}, err
	}
	posterRef, err := c.Upload(ctx, actor, bytes.NewReader(poster))
	if err != nil {
		return runs.Completion{}, err
	}
	file, err := c.db.Reader().GetFileBySHA(ctx, fileRef.SHA256)
	if err != nil {
		return runs.Completion{}, err
	}
	prev, err := c.db.Reader().GetFileBySHA(ctx, posterRef.SHA256)
	if err != nil {
		return runs.Completion{}, err
	}
	err = c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		if _, err := c.addVersion(ctx, q, actor, row.ID, file.ID, prev.ID, sql.NullString{}, "generated", p.Mode); err != nil {
			return err
		}
		return q.TouchCreativeProject(ctx, store.TouchCreativeProjectParams{UpdatedAt: c.now(), ProjectID: row.ProjectID})
	})
	if err != nil {
		return runs.Completion{}, err
	}
	return runs.Completion{State: runs.StateSucceeded, Completeness: runs.CompletenessComplete}, nil
}

// videoPictures returns the full pictures to move: the named renders in
// order, else the concept's chosen render followed by the pictures Adx made
// for the concept, else every visible render, newest first; at most four.
func (c *Studio) videoPictures(ctx context.Context, projectID int64, k store.CreativeConcept, codes []string) ([][]byte, error) {
	renders, err := c.db.Reader().ListRenders(ctx, projectID)
	if err != nil {
		return nil, err
	}
	byCode := map[string]store.CreativeRender{}
	for _, r := range renders {
		byCode[r.Code] = r
	}
	var picked []store.CreativeRender
	for _, code := range codes {
		if r, ok := byCode[code]; ok {
			picked = append(picked, r)
		}
	}
	if len(picked) == 0 {
		var doc struct {
			Render string `json:"render"`
		}
		_ = json.Unmarshal([]byte(k.Copy), &doc)
		if r, ok := byCode[doc.Render]; ok && doc.Render != "" {
			picked = append(picked, r)
		}
		for i := len(renders) - 1; i >= 0; i-- {
			r := renders[i]
			if r.Hidden == 0 && r.Label == k.Code+" · Adx" && r.Code != doc.Render {
				picked = append(picked, r)
			}
		}
	}
	if len(picked) == 0 {
		for i := len(renders) - 1; i >= 0; i-- {
			if renders[i].Hidden == 0 {
				picked = append(picked, renders[i])
			}
		}
	}
	if len(picked) > 4 {
		picked = picked[:4]
	}
	out := make([][]byte, 0, len(picked))
	for _, r := range picked {
		b, err := c.fileBytes(ctx, r.FileID)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// staticCard is the finished story ad for the same concept and language,
// the picture the motion ends on. Nil when the concept has none yet.
func (c *Studio) staticCard(ctx context.Context, video store.CreativeAd) ([]byte, error) {
	ads, err := c.db.Reader().ListConceptAds(ctx, video.ConceptID)
	if err != nil {
		return nil, err
	}
	for _, a := range ads {
		if a.Kind != "static" || a.Lang != video.Lang || a.Fmt != string(FmtStory) {
			continue
		}
		v, err := c.db.Reader().LatestVersion(ctx, a.ID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return c.fileBytes(ctx, v.FileID)
	}
	return nil, nil
}

func (c *Studio) fileBytes(ctx context.Context, fileID int64) ([]byte, error) {
	f, err := c.db.Reader().GetFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	r, err := c.blobs.Open(f.Sha256)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
