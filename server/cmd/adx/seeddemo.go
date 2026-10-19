package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"math"

	"github.com/caarlos0/env/v11"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/studio"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/blobs"
)

// runSeedDemo fills a dev database with one brand to try the studio on:
// Reportage with The Wadi, approved facts (price, handover, payment plan,
// units, location), an approved launch offer, and a creative project with
// two placeholder renders and a concept with copy in both languages. Run it
// once; a database that already has the brand is left alone.
//
//	adx seed-demo
func runSeedDemo(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: adx seed-demo")
	}
	cfg, err := env.ParseAs[config]()
	if err != nil {
		return err
	}
	if cfg.Env != "dev" {
		return errors.New("seed-demo runs with ADX_ENV=dev only")
	}
	ctx := context.Background()
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}
	files, err := blobs.New(cfg.FilesDir)
	if err != nil {
		return err
	}
	actor := core.Actor{UserID: 0, Role: core.RoleAdmin, Via: core.ViaStartup, WorkspaceID: core.DefaultWorkspace}
	br := brands.New(db, brands.Config{})
	st := studio.New(db, files)

	existing, err := br.List(ctx, actor)
	if err != nil {
		return err
	}
	for _, b := range existing {
		if b.Name == "Reportage" {
			slog.Info("seed-demo: Reportage already exists, nothing done")
			return nil
		}
	}
	brand, err := br.Create(ctx, actor, brands.BrandInput{Name: "Reportage", Kind: brands.Own, Notes: "Abu Dhabi developer. Demo data from adx seed-demo."})
	if err != nil {
		return err
	}
	p, err := db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi"})
	if err != nil {
		return err
	}
	if err := br.AssignProject(ctx, actor, p.ID, brand.ID); err != nil {
		return err
	}
	facts := []brands.FactInput{
		{ProjectID: p.ID, Key: "price.from", Kind: brands.KindMoney, Value: "120000000", Currency: "AED", EffectiveFrom: "2026-10-01", SourceKind: brands.SourceDocument, SourceRef: "Price list Oct 2026"},
		{ProjectID: p.ID, Key: "handover", Kind: brands.KindDate, Value: "2027-12-31", EffectiveFrom: "2026-10-01", SourceKind: brands.SourceDocument, SourceRef: "Sales brochure"},
		{ProjectID: p.ID, Key: "payment_plan", Kind: brands.KindText, Value: "10% on booking, 40% during construction, 50% on handover", EffectiveFrom: "2026-10-01", SourceKind: brands.SourceDocument, SourceRef: "Sales brochure"},
		{ProjectID: p.ID, Key: "unit_types", Kind: brands.KindText, Value: "3 and 4 bedroom villas", EffectiveFrom: "2026-10-01", SourceKind: brands.SourceDocument, SourceRef: "Sales brochure"},
		{ProjectID: p.ID, Key: "location", Kind: brands.KindText, Value: "Dubailand, off Al Ain Road", EffectiveFrom: "2026-10-01", SourceKind: brands.SourceDocument, SourceRef: "Sales brochure"},
		{ProjectID: p.ID, Key: "discount.max", Kind: brands.KindPercent, Value: "15", EffectiveFrom: "2026-10-01", SourceKind: brands.SourceDocument, SourceRef: "Launch offer memo"},
	}
	for _, f := range facts {
		added, err := br.AddFact(ctx, actor, brand.ID, f)
		if err != nil {
			return fmt.Errorf("fact %s: %w", f.Key, err)
		}
		if _, err := br.ApproveFact(ctx, actor, added.Fact.ID); err != nil {
			return fmt.Errorf("approve %s: %w", f.Key, err)
		}
	}
	pct := 15.0
	offer, err := br.CreateOffer(ctx, actor, brand.ID, brands.OfferInput{
		ProjectID: p.ID, Name: "Launch weekend", EffectiveFrom: "2026-10-01",
		Document: brands.OfferDocument{
			Headline: "Up to 15% off at the launch weekend", HeadlineAr: "خصم يصل إلى 15% في عطلة الإطلاق",
			Terms: "Selected villas, booked at the launch event.", TermsAr: "على فلل مختارة عند الحجز خلال فعالية الإطلاق.",
			DiscountPct: &pct, PriceFrom: &brands.Price{Minor: 120000000, Currency: "AED"},
			PaymentPlan: "10% on booking, 40% during construction, 50% on handover",
			Conditions:  []string{"Selected units", "Booking during the launch weekend"},
		},
	})
	if err != nil {
		return err
	}
	if _, err := br.SetOfferState(ctx, actor, offer.ID, brands.OfferApproved); err != nil {
		return err
	}

	marketer := core.Actor{UserID: 0, Role: core.RoleMarketer, Via: core.ViaStartup, WorkspaceID: core.DefaultWorkspace}
	if _, err := st.SetUp(ctx, marketer, studio.SetupInput{ProjectID: p.ID}); err != nil {
		return err
	}
	var renders []studio.NewRender
	for i, c := range []color.RGBA{{196, 152, 84, 255}, {72, 110, 140, 255}} {
		full := picture(1080, 1920, c)
		prev := picture(270, 480, c)
		fref, err := st.Upload(ctx, marketer, bytes.NewReader(full))
		if err != nil {
			return err
		}
		pref, err := st.Upload(ctx, marketer, bytes.NewReader(prev))
		if err != nil {
			return err
		}
		renders = append(renders, studio.NewRender{File: fref.SHA256, Preview: pref.SHA256, Label: []string{"Villas at dusk", "Pool at noon"}[i]})
	}
	if _, err := st.AddRenders(ctx, marketer, p.ID, renders); err != nil {
		return err
	}
	k, err := st.CreateConcept(ctx, marketer, p.ID, studio.NewConcept{Code: "C1", Name: "Launch weekend"})
	if err != nil {
		return err
	}
	copyDoc, _ := json.Marshal(map[string]any{
		"v": 1, "render": "R01", "arReviewed": false,
		"en": map[string]any{"eyebrow": "The Wadi · Dubailand", "title": "Villas from AED 1.2M", "titleSize": "lg", "text": "3 and 4 bedroom villas, handover 2027.", "stats": []map[string]string{{"figure": "AED 1.2M", "label": "from"}, {"figure": "15%", "label": "off at launch"}}, "event": "", "note": "", "info": "Selected units, launch weekend only.", "cta": "Register"},
		"ar": map[string]any{"eyebrow": "الوادي · دبي لاند", "title": "فلل من 1.2 مليون درهم", "titleSize": "lg", "text": "فلل من 3 و4 غرف نوم، التسليم 2027.", "stats": []map[string]string{{"figure": "1.2 مليون", "label": "تبدأ من"}, {"figure": "15%", "label": "خصم الإطلاق"}}, "event": "", "note": "", "info": "على وحدات مختارة خلال عطلة الإطلاق.", "cta": "سجّل الآن"},
	})
	if _, err := st.SaveConcept(ctx, marketer, k.ID, studio.ConceptInput{
		Name: "Launch weekend", Why: "price.from and the launch discount, for first-time villa buyers",
		PrimaryText: "The Wadi launches this weekend: 3 and 4 bedroom villas in Dubailand from AED 1.2M, up to 15% off on selected units.",
		Headline:    "Villas from AED 1.2M", Description: "Launch weekend offer", CTA: "Learn more", Copy: copyDoc,
	}); err != nil {
		return err
	}
	if err := db.Writer().SetConceptImagery(ctx, store.SetConceptImageryParams{Imagery: "Golden hour over a gated villa community in Dubailand, warm sandstone facades, a calm pool in the foreground, long shadows, no people in close-up, clear sky above for a headline", UpdatedAt: p.CreatedAt, ID: k.ID}); err != nil {
		return err
	}
	slog.Info("seed-demo done", "brand", brand.Name, "project", p.Name, "facts", len(facts), "offer", offer.Name, "renders", len(renders), "concept", "C1")
	return nil
}

// picture is a placeholder render: a sky to ground gradient with a few
// blocks for villas, in one tint per picture.
func picture(w, h int, tint color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h)
		sky := color.RGBA{uint8(200 - 60*t), uint8(205 - 40*t), uint8(215 - 20*t), 255}
		for x := 0; x < w; x++ {
			img.Set(x, y, sky)
		}
	}
	ground := int(float64(h) * 0.62)
	for y := ground; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{tint.R / 2, tint.G / 2, tint.B / 2, 255})
		}
	}
	for i := 0; i < 3; i++ {
		top := ground - int(float64(h)*0.14*(1+0.35*math.Sin(float64(i))))
		x0 := w/14 + i*(w/3)
		for y := top; y < ground; y++ {
			for x := x0; x < x0+w/4; x++ {
				img.Set(x, y, tint)
			}
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88})
	return buf.Bytes()
}
