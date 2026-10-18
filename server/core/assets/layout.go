package assets

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Layout builds a Document for a format and language from copy, a render
// and a brand kit, following the source's house rules (docs/build/
// reuse-creative.md): logo top centre, text in the lower third, nothing in
// the 9:16 keep-clear zones (top 14%, bottom 20%), Arabic right to left.
// This is template layout, not generation; the renderer label says so.
func Layout(format Format, lang Language, copy Copy, render *Ref, kit BrandKit) (Document, error) {
	dims, ok := Dimensions[format]
	if !ok {
		return Document{}, fmt.Errorf("unknown format %q", format)
	}
	if lang != English && lang != Arabic {
		return Document{}, fmt.Errorf("unknown language %q", lang)
	}
	w, h := float64(dims[0]), float64(dims[1])
	rtl := lang == Arabic
	fonts := kit.Fonts
	if fonts == nil {
		fonts = DefaultFonts
	}
	fontFor := func(role string) string {
		if rtl {
			if f, ok := fonts["ar"+strings.ToUpper(role[:1])+role[1:]]; ok && f.Family != "" {
				return f.Family
			}
		}
		if f, ok := fonts[role]; ok && f.Family != "" {
			return f.Family
		}
		return DefaultFonts[role].Family
	}
	ink := kit.Colors["inkLight"]
	if ink == "" {
		ink = "#FFFFFF"
	}
	accent := kit.Colors["accent"]
	if accent == "" {
		accent = ink
	}
	align := "left"
	x := w * 0.08
	if rtl {
		align = "right"
	}
	textWidth := w * 0.84

	// Scale type to the frame's short side, as the source does.
	scale := minF(w, h) / 1080
	doc := Document{Version: 1, Format: format, Language: lang, Width: int(w), Height: int(h), Background: "#000000"}
	if render != nil {
		doc.Image = &Image{Ref: *render, FX: 0.5, FY: 0.5, Zoom: 1}
	}
	// Logo top centre, under the 9:16 keep-clear zone.
	logoH := 90 * scale
	logoW := logoH * 3
	logoY := h * 0.06
	if format == Format9x16 {
		logoY = h * 0.16
	}
	if logo := kit.Logos["onDark"]; logo != nil {
		doc.Layers = append(doc.Layers, Layer{ID: "logo", Kind: "logo", X: (w - logoW) / 2, Y: logoY, Width: logoW, Height: logoH, Src: logo, Opacity: 1})
	}
	// Text block anchored above the bottom zone.
	bottomLimit := h * 0.92
	if format == Format9x16 {
		bottomLimit = h * 0.78
	}
	titleSize := 72 * scale
	if format == Format191x1 {
		titleSize = 56 * scale
	}
	textSize := 34 * scale
	noteSize := 24 * scale
	cursor := bottomLimit
	add := func(id, text string, size float64, fill string, uppercase bool) {
		if text == "" {
			return
		}
		height := size * 1.2 * float64(lineCount(text, size, textWidth))
		cursor -= height + 18*scale
		doc.Layers = append([]Layer{{ID: id, Kind: "text", X: x, Y: cursor, Width: textWidth, Height: height, Text: text, FontFamily: fontFor(roleFor(id)),
			FontSize: size, Fill: fill, Align: align, LineHeight: 1.2, Uppercase: uppercase, RTL: rtl}}, doc.Layers...)
	}
	// Added bottom-up so the slice ends in reading order: eyebrow, title, text, note, cta.
	add("cta", copy.CTA, textSize, accent, true)
	add("note", copy.Note, noteSize, ink, false)
	add("text", copy.Text, textSize, ink, false)
	add("title", copy.Title, titleSize, ink, false)
	add("eyebrow", copy.Eyebrow, noteSize, accent, true)
	// Keep the logo first for drawing order.
	for i, l := range doc.Layers {
		if l.Kind == "logo" && i != 0 {
			doc.Layers = append([]Layer{l}, append(doc.Layers[:i], doc.Layers[i+1:]...)...)
			break
		}
	}
	return doc, nil
}

func roleFor(id string) string {
	switch id {
	case "title":
		return "title"
	case "eyebrow", "note", "cta":
		return "label"
	}
	return "text"
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// LayoutCheck is one house-rule finding on a document.
type LayoutCheck struct {
	Level string `json:"level"` // error, warn, info
	Text  string `json:"text"`
}

var percentPattern = regexp.MustCompile(`(\d{1,3})\s*%`)
var conditionPattern = regexp.MustCompile(`(?i)(t&c|terms|condition|within|on |for |when|pay|plan|cash|select|limited|\*)`)
var arabicPattern = regexp.MustCompile(`[\x{0600}-\x{06FF}]`)

// CheckDocument ports the source's house-rule checks (docs/CREATIVE_RULES.md
// there): discounts never above 15% and always with a condition, 9:16
// keep-clear zones, logo top centre, one-line 1.91:1 titles, Arabic flagged
// for native review. arReviewed says a native speaker has reviewed the copy.
func CheckDocument(doc Document, arReviewed bool) []LayoutCheck {
	var out []LayoutCheck
	var texts []Layer
	var all []string
	for _, l := range doc.Layers {
		if l.Kind == "text" {
			texts = append(texts, l)
			all = append(all, l.Text)
		}
	}
	allText := strings.Join(all, "\n")
	for _, m := range percentPattern.FindAllStringSubmatch(allText, -1) {
		if pct, _ := strconv.Atoi(m[1]); pct > 15 {
			out = append(out, LayoutCheck{"error", fmt.Sprintf("%q is above the 15%% limit. Write \"Up to 15%% off and more\" instead.", m[0])})
		}
	}
	if percentPattern.MatchString(allText) && !conditionPattern.MatchString(allText) {
		out = append(out, LayoutCheck{"warn", "A discount is shown without its condition in the same frame."})
	}
	if doc.Format == Format9x16 {
		top, bottom := float64(doc.Height)*0.14, float64(doc.Height)*0.8
		for _, l := range doc.Layers {
			height := l.Height
			if l.Kind == "text" {
				lineHeight := l.LineHeight
				if lineHeight == 0 {
					lineHeight = 1.2
				}
				height = l.FontSize * lineHeight * float64(lineCount(l.Text, l.FontSize, l.Width))
			}
			name := "An image"
			switch l.Kind {
			case "text":
				name = fmt.Sprintf("Text %q", truncate(l.Text, 24))
			case "logo":
				name = "A logo"
			}
			if l.Y < top-1 {
				out = append(out, LayoutCheck{"error", name + " sits in the top 14% keep-clear zone."})
			}
			if l.Y+height > bottom+1 {
				out = append(out, LayoutCheck{"error", name + " sits in the bottom 20% keep-clear zone."})
			}
		}
	}
	var logos []Layer
	for _, l := range doc.Layers {
		if l.Kind == "logo" {
			logos = append(logos, l)
		}
	}
	if len(logos) == 0 {
		out = append(out, LayoutCheck{"warn", "No logo on this ad."})
	} else {
		first := logos[0]
		centre := first.X + first.Width/2
		if abs(centre-float64(doc.Width)/2) > float64(doc.Width)*0.04 {
			out = append(out, LayoutCheck{"warn", "The logo is not centred. House rule: logo top centre."})
		}
		if first.Y > float64(doc.Height)*0.35 {
			out = append(out, LayoutCheck{"info", "The logo sits low. House rule: logo top centre."})
		}
	}
	if doc.Format == Format191x1 {
		for _, t := range texts {
			if (t.ID == "title" || t.FontSize > 40) && lineCount(t.Text, t.FontSize, t.Width) > 1 {
				out = append(out, LayoutCheck{"warn", "1.91:1 titles should be one line."})
				break
			}
		}
	}
	if doc.Language == Arabic && !arReviewed {
		out = append(out, LayoutCheck{"info", "Arabic copy: needs native review before it goes out."})
	}
	if doc.Language != Arabic && arabicPattern.MatchString(allText) {
		out = append(out, LayoutCheck{"info", "Contains Arabic text: mark the ad as Arabic so it is flagged for native review."})
	}
	if len(out) == 0 {
		out = append(out, LayoutCheck{"info", "All house-rule checks pass."})
	}
	return out
}

// HasErrors reports whether any check is an error, which blocks review.
func HasErrors(checks []LayoutCheck) bool {
	for _, c := range checks {
		if c.Level == "error" {
			return true
		}
	}
	return false
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
