package svgrender_test

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/assets"
	"github.com/WokeWorks/adx/server/providers/filestore"
	"github.com/WokeWorks/adx/server/providers/svgrender"
)

// Catches: a non-deterministic render (two renders of one document must have
// one checksum), Arabic drawn left to right, text that could inject markup,
// and the renderer claiming to be anything but a template.
func TestRenderIsDeterministicAndSafe(t *testing.T) {
	st, err := filestore.New(t.TempDir())
	require.NoError(t, err)
	ctx := t.Context()
	render, err := st.Put(ctx, "image/jpeg", strings.NewReader("fake jpeg"))
	require.NoError(t, err)
	logo, err := st.Put(ctx, "image/png", strings.NewReader("fake png"))
	require.NoError(t, err)
	kit := assets.BrandKit{Logos: map[string]*assets.Ref{"onDark": &logo}, Colors: map[string]string{"inkLight": "#FFFFFF", "accent": "#C9A227"}}
	doc, err := assets.Layout(assets.Format4x5, assets.Arabic, assets.Copy{Title: "فلل <b>فاخرة</b>", CTA: "سجل"}, &render, kit)
	require.NoError(t, err)

	r := svgrender.New()
	assert.Equal(t, "template", r.Name())
	a, err := r.Render(ctx, st, doc)
	require.NoError(t, err)
	b, err := r.Render(ctx, st, doc)
	require.NoError(t, err)
	assert.Equal(t, a.Ref, b.Ref, "same document, same bytes, same key")
	assert.Equal(t, "image/svg+xml", a.Ref.ContentType)
	assert.Equal(t, "template", a.Renderer)

	rc, err := st.Open(ctx, a.Ref)
	require.NoError(t, err)
	svg, _ := io.ReadAll(rc)
	rc.Close()
	s := string(svg)
	assert.Contains(t, s, `width="1080" height="1350"`)
	assert.Contains(t, s, `data-renderer="template"`)
	assert.Contains(t, s, `direction="rtl"`)
	assert.Contains(t, s, `text-anchor="end"`)
	assert.Contains(t, s, "&lt;b&gt;", "text is escaped, never markup")
	assert.NotContains(t, s, "<b>")
	assert.Contains(t, s, "data:image/jpeg;base64,", "the render is embedded so the checksum covers it")
	assert.Contains(t, s, `font-family="Almarai, sans-serif"`)

	// A bad colour can't carry an attribute into the markup.
	doc.Background = `#000" onload="alert(1)`
	c, err := r.Render(ctx, st, doc)
	require.NoError(t, err)
	rc, _ = st.Open(ctx, c.Ref)
	svg, _ = io.ReadAll(rc)
	rc.Close()
	assert.NotContains(t, string(svg), "onload")
	assert.Contains(t, string(svg), `fill="#000000"`)

	_, err = r.Render(ctx, st, assets.Document{})
	require.Error(t, err)
}

// Catches: a long title written as one tspan that runs off the frame, since
// SVG never wraps on its own.
func TestRenderWrapsLongText(t *testing.T) {
	st, err := filestore.New(t.TempDir())
	require.NoError(t, err)
	ctx := t.Context()
	title := "فلل فاخرة من أربع غرف نوم بإطلالات على المساحات الخضراء وبأسعار تبدأ من مليون ومئتي ألف درهم مع خطة سداد مرنة"
	doc, err := assets.Layout(assets.Format9x16, assets.Arabic, assets.Copy{Title: title, CTA: "سجّل الآن"}, nil, assets.BrandKit{})
	require.NoError(t, err)
	svg, err := svgrender.New().SVG(ctx, st, doc)
	require.NoError(t, err)
	s := string(svg)
	titleText := s[strings.Index(s, `data-layer="title"`):]
	titleText = titleText[:strings.Index(titleText, "</text>")]
	lines := assets.WrapLines(title, doc.Layers[0].FontSize, doc.Layers[0].Width)
	assert.Greater(t, len(lines), 1)
	assert.Equal(t, len(lines), strings.Count(titleText, "<tspan"), "one tspan per wrapped line")
	for _, line := range lines {
		assert.Contains(t, titleText, ">"+line+"<")
	}
	assert.Equal(t, 1, strings.Count(titleText, `dy="0"`), "later lines move down by the line height")
}
