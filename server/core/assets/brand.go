package assets

// BrandKit is a project's brand: logos, fonts and colours, ported from the
// creative source's BrandKit v1 (docs/build/reuse-creative.md). Files are
// Refs into the Store. Fields are optional; a kit with no logo makes the
// house-rule check say so instead of failing.
type BrandKit struct {
	Version int                 `json:"version"`
	Logos   map[string]*Ref     `json:"logos"`  // onDark, onLight, developer
	Fonts   map[string]FontSpec `json:"fonts"`  // title, text, label, arTitle, arText
	Colors  map[string]string   `json:"colors"` // inkLight, inkDark, accent
	Notes   string              `json:"notes"`
}

// FontSpec names a font family and, optionally, the uploaded font file.
type FontSpec struct {
	Family string `json:"family"`
	File   *Ref   `json:"file"`
}

// DefaultFonts are the creative source's fallbacks: Google fonts with Arabic
// coverage for the Arabic roles.
var DefaultFonts = map[string]FontSpec{
	"title": {Family: "Montserrat"}, "text": {Family: "Montserrat"}, "label": {Family: "Montserrat"},
	"arTitle": {Family: "Almarai"}, "arText": {Family: "Cairo"},
}

// Copy is the text of one ad in one language, ported from the source's
// CopyBlocks: an eyebrow (project or offer line), a title, body text, a note
// (the offer condition), and a call to action.
type Copy struct {
	Eyebrow string `json:"eyebrow"`
	Title   string `json:"title"`
	Text    string `json:"text"`
	Note    string `json:"note"`
	CTA     string `json:"cta"`
}
