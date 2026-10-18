package assets_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/assets"
)

// Catches: a 9:16 layout putting text or logo in the keep-clear zones, an
// Arabic layout laid out left to right, and the house-rule checks letting a
// 16% discount or an unmarked Arabic ad through.
func TestLayoutRespectsHouseRules(t *testing.T) {
	logo := &assets.Ref{Key: "sha256-logo", SHA256: "logo", Size: 1, ContentType: "image/png"}
	kit := assets.BrandKit{Logos: map[string]*assets.Ref{"onDark": logo}, Colors: map[string]string{"inkLight": "#FFFFFF", "accent": "#C9A227"}}
	copy := assets.Copy{Eyebrow: "The Wadi", Title: "Villas from AED 1.2M", Text: "Payment plan 1% monthly.\nHandover 2028.", Note: "Up to 15% off when you pay cash. T&C apply.", CTA: "Register interest"}

	for _, f := range []assets.Format{assets.Format9x16, assets.Format4x5, assets.Format1x1, assets.Format191x1} {
		doc, err := assets.Layout(f, assets.English, copy, nil, kit)
		require.NoError(t, err)
		dims := assets.Dimensions[f]
		assert.Equal(t, dims[0], doc.Width)
		assert.Equal(t, dims[1], doc.Height)
		checks := assets.CheckDocument(doc, false)
		assert.False(t, assets.HasErrors(checks), "%s: %v", f, checks)
		assert.Equal(t, "logo", doc.Layers[0].Kind, "logo drawn first")
		assert.Equal(t, "cta", doc.Layers[len(doc.Layers)-1].ID)
	}

	ar, err := assets.Layout(assets.Format9x16, assets.Arabic, assets.Copy{Title: "فلل من 1.2 مليون درهم", CTA: "سجل اهتمامك"}, nil, kit)
	require.NoError(t, err)
	for _, l := range ar.Layers {
		if l.Kind == "text" {
			assert.True(t, l.RTL)
			assert.Equal(t, "right", l.Align)
			assert.Equal(t, "Almarai", ar.Layers[1].FontFamily, "Arabic title font")
		}
	}
	checks := assets.CheckDocument(ar, false)
	assert.Contains(t, texts(checks), "Arabic copy: needs native review before it goes out.")
	assert.NotContains(t, texts(assets.CheckDocument(ar, true)), "Arabic copy: needs native review before it goes out.")

	// Rule violations.
	bad := assets.Document{Version: 1, Format: assets.Format9x16, Language: assets.English, Width: 1080, Height: 1920, Layers: []assets.Layer{
		{ID: "title", Kind: "text", X: 80, Y: 50, Width: 900, Text: "Up to 40% off", FontSize: 72, LineHeight: 1.2},
		{ID: "ar", Kind: "text", X: 80, Y: 1000, Width: 900, Text: "خصم", FontSize: 30, LineHeight: 1.2},
	}}
	checks = assets.CheckDocument(bad, false)
	assert.True(t, assets.HasErrors(checks))
	got := texts(checks)
	assert.Contains(t, got, `"40%" is above the 15% limit. Write "Up to 15% off and more" instead.`)
	assert.Contains(t, got, `Text "Up to 40% off" sits in the top 14% keep-clear zone.`)
	assert.Contains(t, got, "No logo on this ad.")
	assert.Contains(t, got, "Contains Arabic text: mark the ad as Arabic so it is flagged for native review.")

	noCondition := assets.Document{Format: assets.Format1x1, Language: assets.English, Width: 1080, Height: 1080, Layers: []assets.Layer{{ID: "t", Kind: "text", X: 80, Y: 600, Width: 900, Text: "10% off", FontSize: 60, LineHeight: 1.2}}}
	assert.Contains(t, texts(assets.CheckDocument(noCondition, false)), "A discount is shown without its condition in the same frame.")

	_, err = assets.Layout("16:9", assets.English, copy, nil, kit)
	require.Error(t, err)
}

func texts(checks []assets.LayoutCheck) []string {
	out := make([]string, 0, len(checks))
	for _, c := range checks {
		out = append(out, c.Text)
	}
	return out
}

// Catches: a long title or body laid out as one line, so the blocks below
// it are placed on top of its wrapped lines (the pilot gate's long Arabic
// copy), and a renderer line estimate that disagrees with the layout's.
func TestLayoutWrapsLongCopy(t *testing.T) {
	long := assets.Copy{
		Eyebrow: "إطلاق حصري في دبي لاند",
		Title:   "فلل فاخرة من أربع غرف نوم بإطلالات على المساحات الخضراء وبأسعار تبدأ من مليون ومئتي ألف درهم مع خطة سداد مرنة",
		Text:    "سجّل اهتمامك اليوم واحصل على خصم يصل إلى خمسة عشر بالمئة للمسجلين الأوائل، مع تسليم في الربع الأخير من عام ألفين وسبعة وعشرين وخطة سداد على ستين شهراً بدون فوائد.",
		Note:    "الأسعار والعرض خاضعة للشروط والأحكام وقد تتغير دون إشعار مسبق.",
		CTA:     "سجّل الآن",
	}
	for _, f := range []assets.Format{assets.Format9x16, assets.Format4x5, assets.Format1x1, assets.Format191x1} {
		doc, err := assets.Layout(f, assets.Arabic, long, nil, assets.BrandKit{})
		require.NoError(t, err)
		var prev *assets.Layer
		for i := range doc.Layers {
			l := &doc.Layers[i]
			if l.Kind != "text" {
				continue
			}
			lines := assets.WrapLines(l.Text, l.FontSize, l.Width)
			assert.InDelta(t, l.FontSize*1.2*float64(len(lines)), l.Height, 0.01, "%s %s: height covers every wrapped line", f, l.ID)
			if l.ID == "title" {
				assert.Greater(t, len(lines), 1, "%s: the long title wraps", f)
			}
			if prev != nil {
				assert.GreaterOrEqual(t, l.Y, prev.Y+prev.Height, "%s: %s starts below %s", f, l.ID, prev.ID)
			}
			assert.GreaterOrEqual(t, l.Y, 0.0, "%s: %s stays in the frame", f, l.ID)
			prev = l
		}
	}

	// English wraps on spaces too, and an explicit break is kept.
	lines := assets.WrapLines("Villas from AED 1.2M with a 60-month payment plan\nHandover 2028", 72, 900)
	assert.Equal(t, []string{"Villas from AED 1.2M", "with a 60-month", "payment plan", "Handover 2028"}, lines)
	// Without a size or width only the explicit lines come back.
	assert.Equal(t, []string{"a b c", "d"}, assets.WrapLines("a b c\nd", 0, 0))
	// A word wider than the box stands alone instead of being dropped.
	assert.Equal(t, []string{"Short", "Supercalifragilistic", "end"}, assets.WrapLines("Short Supercalifragilistic end", 72, 400))

	// A 1.91:1 title that needs more than one line is warned about.
	wide, err := assets.Layout(assets.Format191x1, assets.Arabic, long, nil, assets.BrandKit{})
	require.NoError(t, err)
	assert.Contains(t, texts(assets.CheckDocument(wide, true)), "1.91:1 titles should be one line.")
}
