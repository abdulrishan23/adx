package studio

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"strings"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

// Picture is one generated image. Core declares the interface it needs;
// providers/google implements it and cmd/adx passes it in.
type Picture struct {
	MIME string
	Data []byte
}

type PictureRequest struct {
	Prompt      string
	AspectRatio string
	References  []PictureReference
}

type PictureReference struct {
	MIME string
	Data []byte
}

// ImageMaker makes one picture from a prompt. A *PictureError says what
// the person may be told.
type ImageMaker interface {
	GenerateImage(ctx context.Context, in PictureRequest) (Picture, error)
}

// PictureError is a failure the person may see. Retryable marks outages.
type PictureError struct {
	Safe      string
	Retryable bool
	Cause     error
}

func (e *PictureError) Error() string {
	if e.Cause != nil {
		return e.Safe + ": " + e.Cause.Error()
	}
	return e.Safe
}

func (e *PictureError) Unwrap() error { return e.Cause }

const (
	maxPicturesPerAsk = 4
	previewWidth      = 360
	imageryStyle      = "Photoreal property marketing photograph for a UAE real estate developer. No text, no letters, no logos, no watermarks, no people's faces in close-up. Leave a calm, uncluttered area where a headline could sit."
)

type GenerateImageryInput struct {
	Count       int    `json:"count"`
	Prompt      string `json:"prompt"`      // replaces the concept's imagery prompt when set
	AspectRatio string `json:"aspectRatio"` // 9:16 by default; the editor crops the rest
	// UseRenders adds the project's visible renders as references, so the
	// picture takes its cue from the real development.
	UseRenders bool `json:"useRenders"`
}

type GeneratedImagery struct {
	Renders []Render `json:"renders"`
	Notes   []string `json:"notes"`
}

// GenerateImagery asks the image model for pictures for a concept and keeps
// each as a render of the project, labelled with the concept's code, so the
// editor and the generator can lay copy over them like any other render.
func (c *Studio) GenerateImagery(ctx context.Context, actor core.Actor, conceptID int64, in GenerateImageryInput) (GeneratedImagery, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return GeneratedImagery{}, core.ErrForbidden
	}
	if c.gen.Images == nil {
		return GeneratedImagery{}, core.Conflict("no image model is configured on this server; upload renders instead or ask an admin to configure ADX_GEMINI_API_KEY")
	}
	count := in.Count
	if count <= 0 {
		count = 2
	}
	if count > maxPicturesPerAsk {
		return GeneratedImagery{}, core.Invalid("ask for up to %d pictures at a time", maxPicturesPerAsk)
	}
	ratio := in.AspectRatio
	if ratio == "" {
		ratio = "9:16"
	}
	if ratio != "9:16" && ratio != "4:5" && ratio != "1:1" && ratio != "16:9" {
		return GeneratedImagery{}, core.Invalid("the aspect ratio must be 9:16, 4:5, 1:1 or 16:9")
	}
	k, err := c.concept(ctx, conceptID)
	if err != nil {
		return GeneratedImagery{}, err
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		prompt = strings.TrimSpace(k.Imagery)
	}
	if prompt == "" {
		return GeneratedImagery{}, core.Invalid("the concept has no imagery prompt yet; write one, or ask Adx for concepts, which writes it")
	}
	if prompt != k.Imagery {
		if err := c.db.Writer().SetConceptImagery(ctx, store.SetConceptImageryParams{Imagery: clip(prompt, 1000), UpdatedAt: c.now(), ID: k.ID}); err != nil {
			return GeneratedImagery{}, err
		}
	}
	var refs []PictureReference
	if in.UseRenders {
		refs, err = c.referenceRenders(ctx, k.ProjectID)
		if err != nil {
			return GeneratedImagery{}, err
		}
	}
	full := prompt + "\n\n" + imageryStyle
	out := GeneratedImagery{Renders: []Render{}, Notes: []string{}}
	for i := 0; i < count; i++ {
		pic, err := c.gen.Images.GenerateImage(ctx, PictureRequest{Prompt: full, AspectRatio: ratio, References: refs})
		if err != nil {
			var pe *PictureError
			if errors.As(err, &pe) {
				if len(out.Renders) == 0 {
					return GeneratedImagery{}, core.Unavailable(err, "%s", pe.Safe)
				}
				out.Notes = append(out.Notes, fmt.Sprintf("picture %d of %d: %s", i+1, count, pe.Safe))
				break
			}
			return GeneratedImagery{}, err
		}
		r, err := c.keepPicture(ctx, actor, k, pic)
		if err != nil {
			return GeneratedImagery{}, err
		}
		out.Renders = append(out.Renders, r)
	}
	return out, nil
}

// referenceRenders returns the project's visible renders as references, at
// most three, newest first.
func (c *Studio) referenceRenders(ctx context.Context, projectID int64) ([]PictureReference, error) {
	rows, err := c.db.Reader().ListRenders(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var refs []PictureReference
	for i := len(rows) - 1; i >= 0 && len(refs) < 3; i-- {
		if rows[i].Hidden == 1 {
			continue
		}
		f, err := c.db.Reader().GetFile(ctx, rows[i].PreviewFileID)
		if err != nil {
			return nil, err
		}
		b, err := c.blobs.Open(f.Sha256)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		_, err = buf.ReadFrom(b)
		b.Close()
		if err != nil {
			return nil, err
		}
		refs = append(refs, PictureReference{MIME: f.ContentType, Data: buf.Bytes()})
	}
	return refs, nil
}

// keepPicture stores the picture and a preview as files, then adds a render
// to the project with the concept's code as its label.
func (c *Studio) keepPicture(ctx context.Context, actor core.Actor, k store.CreativeConcept, pic Picture) (Render, error) {
	img, _, err := image.Decode(bytes.NewReader(pic.Data))
	if err != nil {
		return Render{}, &PictureError{Safe: "the picture could not be read", Cause: err}
	}
	// Everything the studio lays out on is a JPG, whatever the model sent.
	var fullBuf, prevBuf bytes.Buffer
	if err := jpeg.Encode(&fullBuf, img, &jpeg.Options{Quality: 92}); err != nil {
		return Render{}, err
	}
	if err := jpeg.Encode(&prevBuf, downscale(img, previewWidth), &jpeg.Options{Quality: 85}); err != nil {
		return Render{}, err
	}
	fullRef, err := c.Upload(ctx, actor, &fullBuf)
	if err != nil {
		return Render{}, err
	}
	prevRef, err := c.Upload(ctx, actor, &prevBuf)
	if err != nil {
		return Render{}, err
	}
	renders, err := c.AddRenders(ctx, actor, k.ProjectID, []NewRender{{File: fullRef.SHA256, Preview: prevRef.SHA256, Label: k.Code + " · Adx"}})
	if err != nil {
		return Render{}, err
	}
	for i := len(renders) - 1; i >= 0; i-- {
		if renders[i].File.SHA256 == fullRef.SHA256 {
			return renders[i], nil
		}
	}
	return Render{}, sql.ErrNoRows
}

// downscale shrinks an image to the given width with a box filter, enough
// for a preview.
func downscale(src image.Image, width int) image.Image {
	b := src.Bounds()
	if b.Dx() <= width {
		return src
	}
	scale := float64(b.Dx()) / float64(width)
	height := int(float64(b.Dy()) / scale)
	if height < 1 {
		height = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sy0, sy1 := b.Min.Y+int(float64(y)*scale), b.Min.Y+int(float64(y+1)*scale)
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < width; x++ {
			sx0, sx1 := b.Min.X+int(float64(x)*scale), b.Min.X+int(float64(x+1)*scale)
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, a, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					cr, cg, cb, ca := src.At(sx, sy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n>>8), uint8(g/n>>8), uint8(bl/n>>8), uint8(a/n>>8)
		}
	}
	return dst
}
