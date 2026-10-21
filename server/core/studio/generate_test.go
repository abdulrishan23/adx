package studio

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/assistant/assistanttest"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
)

// Catches: a figure the facts don't support reaching the copy, a concept
// code colliding with one the project has, and the model's prose being
// saved as a concept.
func TestGenerateConceptsKeepsToApprovedFacts(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	coretest.SeedUsers(t, f.db)
	b := brands.New(f.db, brands.Config{})
	brand, err := b.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	require.NoError(t, b.AssignProject(ctx, coretest.Admin, f.project, brand.ID))
	fact, err := b.AddFact(ctx, coretest.Marketer, brand.ID, brands.FactInput{ProjectID: f.project, Key: "price.from", Kind: brands.KindMoney, Value: "120000000", Currency: "AED", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceDocument, SourceRef: "price list"})
	require.NoError(t, err)
	_, err = b.ApproveFact(ctx, coretest.Admin, fact.Fact.ID)
	require.NoError(t, err)
	_, err = f.c.CreateConcept(ctx, marketer, f.project, NewConcept{Code: "C1", Name: "Cash offer"})
	require.NoError(t, err)

	model := &assistanttest.Model{Responses: []assistant.Response{assistanttest.ToolCall("1", conceptsTool, map[string]any{
		"concepts": []map[string]any{
			{
				"code": "C1", "name": "Launch weekend", "why": "price.from for first-time buyers",
				"en": map[string]any{"eyebrow": "The Wadi", "title": "Villas from AED 1.2M", "text": "Launch weekend", "cta": "Register",
					"stats": []map[string]string{{"figure": "AED 1.2M", "label": "from"}, {"figure": "40%", "label": "sold"}}},
				"ar":      map[string]any{"title": "فلل من 1.2 مليون درهم", "cta": "سجّل الآن"},
				"imagery": "Golden hour over a gated villa community, calm sky, empty foreground",
				"meta":    map[string]string{"primaryText": "Launching this weekend.", "headline": "The Wadi", "cta": "Learn more"},
			},
			{"code": "C2", "name": "Empty one", "why": "", "en": map[string]any{}, "ar": map[string]any{}, "imagery": ""},
		},
	})}}
	f.c.Generate(Generation{Model: model, Brands: b})
	actor := core.Actor{UserID: 0, Role: core.RoleMarketer, Via: core.ViaWeb, WorkspaceID: core.DefaultWorkspace}
	out, err := f.c.GenerateConcepts(ctx, actor, f.project, GenerateConceptsInput{Count: 2})
	require.NoError(t, err)
	require.Len(t, out.Concepts, 1, "the empty concept is not saved")
	k := out.Concepts[0]
	assert.Equal(t, "C2", k.Code, "C1 was taken, so the next free code")
	assert.Equal(t, "Launch weekend", k.Name)
	assert.Equal(t, "Golden hour over a gated villa community, calm sky, empty foreground", k.Imagery)
	var doc struct {
		En struct {
			Title string              `json:"title"`
			Stats []map[string]string `json:"stats"`
		} `json:"en"`
		Ar struct {
			Title string `json:"title"`
		} `json:"ar"`
	}
	require.NoError(t, json.Unmarshal(k.Copy, &doc))
	assert.Equal(t, "Villas from AED 1.2M", doc.En.Title)
	require.Len(t, doc.En.Stats, 1, "the 40% stat has no fact behind it")
	assert.Equal(t, "AED 1.2M", doc.En.Stats[0]["figure"])
	assert.Equal(t, "فلل من 1.2 مليون درهم", doc.Ar.Title)
	assert.Contains(t, out.Notes[0], "40%")
	req := model.Requests[0]
	assert.Contains(t, req.Messages[0].Content[0].Text, "price.from = 120000000 AED")
	assert.Contains(t, req.Messages[0].Content[0].Text, "C1 Cash offer")

	// Prose instead of a tool call saves nothing.
	model.Responses = []assistant.Response{assistanttest.Text("Here are some ideas...")}
	_, err = f.c.GenerateConcepts(ctx, actor, f.project, GenerateConceptsInput{})
	assert.Error(t, err)
	p, err := f.c.Project(ctx, viewer, f.project)
	require.NoError(t, err)
	assert.Len(t, p.Concepts, 2)

	// No model configured is a conflict, not a stub.
	f.c.Generate(Generation{})
	_, err = f.c.GenerateConcepts(ctx, actor, f.project, GenerateConceptsInput{})
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
}

type fakeImages struct {
	calls []PictureRequest
	fail  error
}

func (f *fakeImages) GenerateImage(_ context.Context, in PictureRequest) (Picture, error) {
	f.calls = append(f.calls, in)
	if f.fail != nil {
		return Picture{}, f.fail
	}
	img := image.NewRGBA(image.Rect(0, 0, 720, 1280))
	for i := range img.Pix {
		img.Pix[i] = uint8(len(f.calls) * 40)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return Picture{}, err
	}
	return Picture{MIME: "image/png", Data: buf.Bytes()}, nil
}

// Catches: a picture kept without a preview, the concept's prompt not
// reaching the model, the project's renders not sent as references when
// asked, and an outage reported as a bad request.
func TestGenerateImageryKeepsPicturesAsRenders(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	k, err := f.c.CreateConcept(ctx, marketer, f.project, NewConcept{Code: "C1", Name: "Launch"})
	require.NoError(t, err)
	fake := &fakeImages{}
	f.c.Generate(Generation{Images: fake})

	_, err = f.c.GenerateImagery(ctx, marketer, k.ID, GenerateImageryInput{})
	assert.Error(t, err, "no prompt anywhere yet")

	out, err := f.c.GenerateImagery(ctx, marketer, k.ID, GenerateImageryInput{Count: 2, Prompt: "Dusk over the lagoon"})
	require.NoError(t, err)
	require.Len(t, out.Renders, 2)
	assert.Equal(t, "R01", out.Renders[0].Code)
	assert.Equal(t, "C1 · Adx", out.Renders[0].Label)
	assert.Equal(t, "image/jpeg", out.Renders[0].File.ContentType)
	assert.Equal(t, int64(720), *out.Renders[0].File.Width)
	assert.Equal(t, int64(360), *out.Renders[0].Preview.Width, "a preview is made")
	require.Len(t, fake.calls, 2)
	assert.Contains(t, fake.calls[0].Prompt, "Dusk over the lagoon")
	assert.Contains(t, fake.calls[0].Prompt, "No text")
	assert.Equal(t, "9:16", fake.calls[0].AspectRatio)
	assert.Empty(t, fake.calls[0].References)
	p, err := f.c.Project(ctx, viewer, f.project)
	require.NoError(t, err)
	assert.Equal(t, "Dusk over the lagoon", p.Concepts[0].Imagery, "the prompt is kept on the concept")

	// With renders as references, the previews go along.
	_, err = f.c.GenerateImagery(ctx, marketer, k.ID, GenerateImageryInput{Count: 1, UseRenders: true, AspectRatio: "1:1"})
	require.NoError(t, err)
	assert.Len(t, fake.calls[2].References, 2)
	assert.Equal(t, "1:1", fake.calls[2].AspectRatio)

	fake.fail = &PictureError{Safe: "the image service is having trouble", Retryable: true}
	_, err = f.c.GenerateImagery(ctx, marketer, k.ID, GenerateImageryInput{Count: 1})
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindUnavailable, ce.Kind)

	_, err = f.c.GenerateImagery(ctx, viewer, k.ID, GenerateImageryInput{Count: 1})
	assert.ErrorIs(t, err, core.ErrForbidden)
}
