package studio

import (
	"regexp"
	"strings"
)

// Fmt is one of the four ad sizes.
type Fmt string

const (
	FmtStory     Fmt = "9:16"
	FmtFeed      Fmt = "4:5"
	FmtSquare    Fmt = "1:1"
	FmtLandscape Fmt = "1.91:1"
)

type size struct {
	width, height int64
	idToken       string // in ad codes: C1_CASH_EN_191x1
	fileToken     string // in file names: ..._1.91x1.jpg
}

var sizes = map[Fmt]size{
	FmtStory:     {1080, 1920, "9x16", "9x16"},
	FmtFeed:      {1080, 1350, "4x5", "4x5"},
	FmtSquare:    {1080, 1080, "1x1", "1x1"},
	FmtLandscape: {1200, 628, "191x1", "1.91x1"},
}

// fmtOrder is the order sizes appear in on the board.
var fmtOrder = []Fmt{FmtStory, FmtFeed, FmtSquare, FmtLandscape}

func (f Fmt) valid() bool {
	_, ok := sizes[f]
	return ok
}

// Lang is the language an ad's copy is in.
type Lang string

const (
	LangEN Lang = "en"
	LangAR Lang = "ar"
)

func (l Lang) valid() bool { return l == LangEN || l == LangAR }

var (
	conceptCode = regexp.MustCompile(`^[A-Z]{1,3}[0-9]{1,3}[A-Z]{0,3}$`) // C1, O12, T1AR
	slugPattern = regexp.MustCompile(`^[A-Z0-9]+(_[A-Z0-9]+)*$`)         // CASH, RETAIL_2080
	renderCode  = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,39}$`)       // R01
	prefixChars = regexp.MustCompile(`[^A-Z0-9]`)
	// A variation's slug ends with the render it was built on: CASH_R07.
	renderSuffix = regexp.MustCompile(`_R[0-9]+[A-Z]*$`)
)

// adSpec is everything an ad's code and file name are built from. The naming
// matches Creative Studio's, so files exported from either line up.
type adSpec struct {
	concept string
	slug    string
	lang    Lang
	fmt     Fmt
	cta     bool
}

// code is the ad's id within its project: <Concept>_<SLUG>_<LANG>_<SIZE>[_CTA].
func (s adSpec) code() string {
	parts := []string{s.concept, s.slug, strings.ToUpper(string(s.lang)), sizes[s.fmt].idToken}
	if s.cta {
		parts = append(parts, "CTA")
	}
	return strings.Join(parts, "_")
}

// fileName is <PREFIX>_<code>.jpg, with 1.91x1 spelled as in the old files.
func (s adSpec) fileName(prefix string) string {
	parts := []string{prefix, s.concept, s.slug, strings.ToUpper(string(s.lang)), sizes[s.fmt].fileToken}
	if s.cta {
		parts = append(parts, "CTA")
	}
	return strings.Join(parts, "_") + ".jpg"
}

// prefixFrom turns a project name into a file prefix: "Marlin Icon" -> "MARLINICON".
func prefixFrom(name string) string {
	p := prefixChars.ReplaceAllString(strings.ToUpper(name), "")
	if len(p) > 16 {
		p = p[:16]
	}
	if p == "" {
		return "ADX"
	}
	return p
}

// slugFrom turns a project name into the readable start of a review key.
func slugFrom(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.TrimSuffix(b.String(), "-")
	if len(s) > 30 {
		s = strings.TrimSuffix(s[:30], "-")
	}
	if s == "" {
		return "review"
	}
	return s
}

// slugFromName turns a concept name into an ad's slug: "10% payment plan"
// becomes PAYMENT_PLAN, kept to 30 characters.
func slugFromName(name string) string {
	var parts []string
	for _, w := range strings.FieldsFunc(strings.ToUpper(name), func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) {
		if w != "" {
			parts = append(parts, w)
		}
	}
	s := strings.Join(parts, "_")
	if len(s) > 30 {
		s = strings.TrimSuffix(s[:30], "_")
	}
	if s == "" {
		return "AD"
	}
	return s
}
