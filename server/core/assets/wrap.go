package assets

import (
	"math"
	"strings"
	"unicode"
)

// Text in a document has no font loaded when it is laid out, and SVG does
// not wrap text on its own. So both the layout (how tall a block is) and
// the renderer (where each line breaks) need the same estimate of how wide
// a run of text is. The advances below are averages for the brand fonts
// (Montserrat, Cairo, Almarai) as fractions of the font size, on the wide
// side so a line errs towards breaking early rather than overflowing.
const (
	advanceLatin  = 0.6
	advanceArabic = 0.5
	advanceSpace  = 0.3
)

// textWidth estimates how wide s is at fontSize.
func textWidth(s string, fontSize float64) float64 {
	var w float64
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			w += advanceSpace
		case unicode.Is(unicode.Mn, r):
			// Combining marks (tashkeel) take no width of their own.
		case r >= 0x0600 && r <= 0x06FF:
			w += advanceArabic
		default:
			w += advanceLatin
		}
	}
	return w * fontSize
}

// WrapLines breaks text into the lines it takes at fontSize inside width.
// Explicit line breaks are kept; the rest wraps on spaces, and a word wider
// than the box stands on a line of its own. Without a usable size or width
// only the explicit lines come back.
func WrapLines(text string, fontSize, width float64) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		if fontSize <= 0 || width <= 0 {
			out = append(out, para)
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, para)
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			if textWidth(line+" "+word, fontSize) <= width {
				line += " " + word
				continue
			}
			out = append(out, line)
			line = word
		}
		out = append(out, line)
	}
	return out
}

// lineCount is how many lines text takes in a box: at least one, and never
// fewer than its explicit breaks.
func lineCount(text string, fontSize, width float64) int {
	return int(math.Max(1, float64(len(WrapLines(text, fontSize, width)))))
}
