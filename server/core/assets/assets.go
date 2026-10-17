// Package assets owns creative output: brand kits, layout documents, rendered
// files and their review. This file is the boundary phase 0 fixes; B1.6 and
// B3.1 add the tables and the lifecycle on top of it.
//
// Two things are kept apart on purpose. A file's bytes live in a Store under
// a content-addressed key and never change: a new render is a new key.
// Review is about content: an approved version says "this artwork and copy
// may be used", and nothing more. Putting it on the review board, exporting
// it or approving it never reaches an ad account. Delivery is the execution
// domain's job (B3.4, B3.5) and needs its own authorization, which refers to
// an approved version by id and checksum.
package assets

import (
	"context"
	"io"
	"time"
)

// Ref points at immutable bytes in a Store. Key is derived from the content,
// so two identical renders share one key and a changed render gets a new one.
type Ref struct {
	Key         string `json:"key"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType"`
}

// Store keeps rendered files, uploads and brand assets. Implementations are
// injected from cmd/adx (a directory in ADX_ASSETS_DIR for now).
type Store interface {
	// Put writes the bytes and returns their ref. Writing the same bytes
	// again returns the same ref and changes nothing.
	Put(ctx context.Context, contentType string, r io.Reader) (Ref, error)
	// Open reads the bytes behind a ref. A ref whose bytes don't match its
	// checksum is an error, never silently served.
	Open(ctx context.Context, ref Ref) (io.ReadCloser, error)
	Exists(ctx context.Context, ref Ref) (bool, error)
}

// Format is one of the four ad sizes the creative source supports.
type Format string

const (
	Format9x16  Format = "9:16"
	Format4x5   Format = "4:5"
	Format1x1   Format = "1:1"
	Format191x1 Format = "1.91:1"
)

// Dimensions gives the pixel size of each format, from the creative source's
// FORMATS table.
var Dimensions = map[Format][2]int{
	Format9x16: {1080, 1920}, Format4x5: {1080, 1350}, Format1x1: {1080, 1080}, Format191x1: {1200, 628},
}

type Language string

const (
	English Language = "en"
	Arabic  Language = "ar"
)

// Document is the editable layout, a port of the creative source's DesignDoc
// v1. It is the unit the canvas editor edits and the renderer draws. Text
// layers carry their direction, so Arabic is laid out right to left.
type Document struct {
	Version    int      `json:"version"`
	Format     Format   `json:"format"`
	Language   Language `json:"language"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	Background string   `json:"background"`
	Image      *Image   `json:"image"`
	Layers     []Layer  `json:"layers"`
}

type Image struct {
	// Ref is the render (photo) behind the layout.
	Ref  Ref     `json:"ref"`
	FX   float64 `json:"fx"`
	FY   float64 `json:"fy"`
	Zoom float64 `json:"zoom"`
}

// Layer is a text or image layer. Kind decides which fields matter.
type Layer struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"` // "text", "logo", "image"
	X             float64 `json:"x"`
	Y             float64 `json:"y"`
	Width         float64 `json:"width"`
	Height        float64 `json:"height"`
	Text          string  `json:"text,omitempty"`
	FontFamily    string  `json:"fontFamily,omitempty"`
	FontSize      float64 `json:"fontSize,omitempty"`
	FontStyle     string  `json:"fontStyle,omitempty"`
	Fill          string  `json:"fill,omitempty"`
	Align         string  `json:"align,omitempty"`
	LineHeight    float64 `json:"lineHeight,omitempty"`
	LetterSpacing float64 `json:"letterSpacing,omitempty"`
	Uppercase     bool    `json:"uppercase,omitempty"`
	RTL           bool    `json:"rtl,omitempty"`
	Src           *Ref    `json:"src,omitempty"`
	Opacity       float64 `json:"opacity,omitempty"`
}

// Rendered is the output of one render.
type Rendered struct {
	Ref        Ref
	Format     Format
	RenderedAt time.Time
	// Renderer names what produced it, such as "template" or
	// "fake". It is shown next to the file, so a template layout is never
	// presented as AI image generation.
	Renderer string
}

// Renderer turns a Document into a file in the Store. Template layout is the
// only renderer ADX has; an AI generation provider, if ever chosen, is a
// separate interface (B3.1) and labelled as such.
type Renderer interface {
	Render(ctx context.Context, store Store, doc Document) (Rendered, error)
	Name() string
}

// ReviewState is where a version is in review. Board publishing is the
// transition to in_review: the version becomes visible to reviewers.
// Approved is the end of this machine. It is content acceptance, not
// permission to spend or deliver.
type ReviewState string

const (
	Draft      ReviewState = "draft"
	RenderedSt ReviewState = "rendered"
	InReview   ReviewState = "in_review"
	Approved   ReviewState = "approved"
	Revise     ReviewState = "revise"
)

var transitions = map[ReviewState][]ReviewState{
	Draft:      {RenderedSt},
	RenderedSt: {InReview, Draft},
	InReview:   {Approved, Revise},
	Revise:     {Draft},
	Approved:   {},
}

// CanTransition reports whether a version may move from one state to another.
// Approved is terminal: a changed document is a new version in Draft, with no
// approval of its own, so an old approval can't follow new artwork.
func CanTransition(from, to ReviewState) bool {
	for _, next := range transitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// ContentAcceptance is what approval produces. It names the exact bytes it
// accepted. It carries no authority to deliver: that is a separate record in
// the execution domain, and nothing in this package can create one.
type ContentAcceptance struct {
	VersionID  int64     `json:"versionId"`
	Ref        Ref       `json:"ref"`
	AcceptedBy int64     `json:"acceptedBy"`
	AcceptedAt time.Time `json:"acceptedAt"`
}

// GrantsDelivery is always false. It exists so callers that want to deliver
// are forced to go to the execution domain for an authorization.
func (ContentAcceptance) GrantsDelivery() bool { return false }

// Covers reports whether the acceptance is for exactly these bytes. A
// re-rendered version has a different ref and is not covered.
func (a ContentAcceptance) Covers(ref Ref) bool {
	return a.Ref.Key == ref.Key && a.Ref.SHA256 == ref.SHA256
}
