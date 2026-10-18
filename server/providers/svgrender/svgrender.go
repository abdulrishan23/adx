// Package svgrender renders a core/assets.Document to SVG: a deterministic
// template layout with the render as an embedded image, text layers with
// their fonts, sizes and direction (Arabic right to left), and logos. It is
// labelled "template": nothing here generates imagery. The browser (or any
// SVG rasterizer) turns it into pixels; the SVG is the immutable output the
// review binds to.
package svgrender

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/WokeWorks/adx/server/core/assets"
)

type Renderer struct{}

func New() *Renderer { return &Renderer{} }

func (Renderer) Name() string { return "template" }

// Render writes the SVG into the store and returns its ref. Images are
// embedded as data URIs so the output stands alone and its checksum covers
// everything that is visible.
func (r Renderer) Render(ctx context.Context, store assets.Store, doc assets.Document) (assets.Rendered, error) {
	svg, err := r.SVG(ctx, store, doc)
	if err != nil {
		return assets.Rendered{}, err
	}
	ref, err := store.Put(ctx, "image/svg+xml", bytes.NewReader(svg))
	if err != nil {
		return assets.Rendered{}, err
	}
	return assets.Rendered{Ref: ref, Format: doc.Format, Renderer: r.Name()}, nil
}

// SVG produces the document's SVG bytes.
func (r Renderer) SVG(ctx context.Context, store assets.Store, doc assets.Document) ([]byte, error) {
	if doc.Width <= 0 || doc.Height <= 0 {
		return nil, fmt.Errorf("document has no size")
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="%d" height="%d" viewBox="0 0 %d %d" data-renderer="template" data-format="%s" data-lang="%s">`,
		doc.Width, doc.Height, doc.Width, doc.Height, html.EscapeString(string(doc.Format)), html.EscapeString(string(doc.Language)))
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="%s"/>`, color(doc.Background, "#000000"))
	if doc.Image != nil {
		uri, err := dataURI(ctx, store, doc.Image.Ref)
		if err != nil {
			return nil, fmt.Errorf("render image: %w", err)
		}
		// preserveAspectRatio slice crops to the frame; fx/fy pick the focus.
		fmt.Fprintf(&b, `<image href="%s" x="0" y="0" width="%d" height="%d" preserveAspectRatio="%s slice"/>`, uri, doc.Width, doc.Height, alignFor(doc.Image.FX, doc.Image.FY))
		// A gradient so text in the lower part stays legible, as the source does.
		b.WriteString(`<defs><linearGradient id="shade" x1="0" y1="0" x2="0" y2="1"><stop offset="0.45" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity="0.75"/></linearGradient></defs>`)
		fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="url(#shade)"/>`)
	}
	for _, l := range doc.Layers {
		switch l.Kind {
		case "logo", "image":
			if l.Src == nil {
				continue
			}
			uri, err := dataURI(ctx, store, *l.Src)
			if err != nil {
				return nil, fmt.Errorf("layer %s: %w", l.ID, err)
			}
			opacity := l.Opacity
			if opacity == 0 {
				opacity = 1
			}
			fmt.Fprintf(&b, `<image href="%s" x="%.1f" y="%.1f" width="%.1f" height="%.1f" opacity="%.2f" preserveAspectRatio="xMidYMid meet" data-layer="%s"/>`, uri, l.X, l.Y, l.Width, l.Height, opacity, html.EscapeString(l.ID))
		case "text":
			writeText(&b, l)
		}
	}
	b.WriteString(`</svg>`)
	return []byte(b.String()), nil
}

func writeText(b *strings.Builder, l assets.Layer) {
	text := l.Text
	if l.Uppercase {
		text = strings.ToUpper(text)
	}
	anchor, x := "start", l.X
	direction := "ltr"
	switch l.Align {
	case "center":
		anchor, x = "middle", l.X+l.Width/2
	case "right":
		anchor, x = "end", l.X+l.Width
	}
	if l.RTL {
		direction = "rtl"
		if l.Align != "center" && l.Align != "left" {
			anchor, x = "end", l.X+l.Width
		}
	}
	lineHeight := l.LineHeight
	if lineHeight == 0 {
		lineHeight = 1.2
	}
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" font-family="%s" font-size="%.1f" fill="%s" text-anchor="%s" direction="%s" unicode-bidi="plaintext" data-layer="%s"`,
		x, l.Y+l.FontSize, html.EscapeString(fontFamily(l.FontFamily)), l.FontSize, color(l.Fill, "#FFFFFF"), anchor, direction, html.EscapeString(l.ID))
	if l.FontStyle != "" {
		if strings.Contains(l.FontStyle, "bold") {
			b.WriteString(` font-weight="700"`)
		}
		if strings.Contains(l.FontStyle, "italic") {
			b.WriteString(` font-style="italic"`)
		}
	}
	if l.LetterSpacing != 0 {
		fmt.Fprintf(b, ` letter-spacing="%.1f"`, l.LetterSpacing)
	}
	b.WriteString(">")
	// SVG has no wrapping of its own; the lines are decided here with the
	// same estimate the layout used for the block's height.
	for i, line := range assets.WrapLines(text, l.FontSize, l.Width) {
		dy := "0"
		if i > 0 {
			dy = fmt.Sprintf("%.1f", l.FontSize*lineHeight)
		}
		fmt.Fprintf(b, `<tspan x="%.1f" dy="%s">%s</tspan>`, x, dy, html.EscapeString(line))
	}
	b.WriteString("</text>")
}

func fontFamily(f string) string {
	if f == "" {
		return "sans-serif"
	}
	return f + ", sans-serif"
}

// color admits only #RGB/#RRGGBB values; anything else falls back, so a
// stray string can't inject attributes.
func color(c, fallback string) string {
	c = strings.TrimSpace(c)
	if len(c) == 4 || len(c) == 7 {
		if c[0] == '#' && isHex(c[1:]) {
			return c
		}
	}
	return fallback
}

func isHex(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

func alignFor(fx, fy float64) string {
	h := "xMid"
	switch {
	case fx < 0.34:
		h = "xMin"
	case fx > 0.66:
		h = "xMax"
	}
	v := "YMid"
	switch {
	case fy < 0.34:
		v = "YMin"
	case fy > 0.66:
		v = "YMax"
	}
	return h + v
}

func dataURI(ctx context.Context, store assets.Store, ref assets.Ref) (string, error) {
	rc, err := store.Open(ctx, ref)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 25<<20))
	if err != nil {
		return "", err
	}
	ct := ref.ContentType
	if ct == "" || strings.ContainsAny(ct, `"' <>`) {
		ct = "application/octet-stream"
	}
	return "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

var _ assets.Renderer = Renderer{}
