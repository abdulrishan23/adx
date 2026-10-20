package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"math"
	"net/http"
	"path"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/studio"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/blobs"
)

// demoProject is one development the demo seeds. The figures are invented
// for trying the studio, not Reportage's real prices; every fact's source
// says so.
type demoProject struct {
	name       string
	location   string
	units      string
	priceAED   int64 // whole dirhams
	priceText  string
	priceAr    string
	handover   string
	plan       string
	discount   float64
	offer      string
	offerAr    string
	concept    string
	slug       string
	imagery    string
	tints      [2]color.RGBA
	renderName [2]string
	// logo is the project's logo on reportagegroup.com, the picture in its
	// page's .r-logo. The Wadi's page has none.
	logo string
}

var demoProjects = []demoProject{
	{
		name: "The Wadi", location: "Dubailand, off Al Ain Road", units: "3 and 4 bedroom villas",
		priceAED: 1_200_000, priceText: "AED 1.2M", priceAr: "1.2 مليون درهم", handover: "2027-12-31",
		plan: "10% on booking, 40% during construction, 50% on handover", discount: 15,
		offer: "Up to 15% off at the launch weekend", offerAr: "خصم يصل إلى 15% في عطلة الإطلاق",
		concept: "Launch weekend", slug: "LAUNCH",
		imagery:    "Golden hour over a gated villa community in Dubailand, warm sandstone facades, a calm pool in the foreground, long shadows, no people in close-up, clear sky above for a headline",
		tints:      [2]color.RGBA{{196, 152, 84, 255}, {72, 110, 140, 255}},
		renderName: [2]string{"Villas at dusk", "Pool at noon"},
	},
	{
		name: "Verdana", location: "Dubai Investments Park", units: "Studios, 1 and 2 bedroom apartments and townhouses",
		priceAED: 550_000, priceText: "AED 550K", priceAr: "550 ألف درهم", handover: "2027-06-30",
		plan: "1% monthly over 100 months", discount: 10,
		offer: "1% a month, 10% off selected units", offerAr: "1% شهرياً وخصم 10% على وحدات مختارة",
		concept: "One percent a month", slug: "MONTHLY",
		imagery:    "Morning light over a green residential community with low-rise apartments and townhouses, landscaped walkways and palm trees, families far in the distance, open sky for a headline",
		tints:      [2]color.RGBA{{104, 150, 98, 255}, {182, 176, 150, 255}},
		renderName: [2]string{"Community park", "Townhouse row"},
		logo:       "https://reportagegroup.com/wp-content/uploads/VERDANA-LOGO.jpg",
	},
	{
		name: "Sila", location: "Masdar City, Abu Dhabi", units: "1, 2 and 3 bedroom apartments",
		priceAED: 780_000, priceText: "AED 780K", priceAr: "780 ألف درهم", handover: "2028-03-31",
		plan: "20% on booking, 80% on handover", discount: 8,
		offer: "8% off for the first 50 bookings", offerAr: "خصم 8% لأول 50 حجزاً",
		concept: "Masdar living", slug: "MASDAR",
		imagery:    "Contemporary low-carbon apartment buildings in Masdar City at blue hour, shaded courtyards, warm interior lights, clean lines, sky above left calm",
		tints:      [2]color.RGBA{{150, 120, 90, 255}, {60, 80, 110, 255}},
		renderName: [2]string{"Courtyard", "Blue hour"},
		logo:       "https://reportagegroup.com/wp-content/uploads/SILA_LOGOS_FINAL-2.svg",
	},
	{
		name: "Oceana", location: "Al Reem Island, Abu Dhabi", units: "Waterfront 1 to 4 bedroom apartments",
		priceAED: 1_450_000, priceText: "AED 1.45M", priceAr: "1.45 مليون درهم", handover: "2028-09-30",
		plan: "10% on booking, 50% during construction, 40% on handover", discount: 12,
		offer: "12% off sea-view units at launch", offerAr: "خصم 12% على الوحدات المطلة على البحر",
		concept: "Sea view", slug: "SEA_VIEW",
		imagery:    "A slender waterfront tower on Al Reem Island at sunset, calm sea in the foreground, a marina boardwalk, warm reflections on glass, open sky for a headline",
		tints:      [2]color.RGBA{{40, 110, 150, 255}, {210, 170, 120, 255}},
		renderName: [2]string{"Tower at sunset", "Marina walk"},
		logo:       "https://reportagegroup.com/wp-content/uploads/OCEANA-LOGO.jpg",
	},
	{
		name: "Selina Bay", location: "Yas Island, Abu Dhabi", units: "Studios, 1 and 2 bedroom apartments",
		priceAED: 690_000, priceText: "AED 690K", priceAr: "690 ألف درهم", handover: "2028-12-31",
		plan: "5% on booking, 55% during construction, 40% on handover", discount: 7,
		offer: "Book with 5% and get 7% off", offerAr: "احجز بـ 5% واحصل على خصم 7%",
		concept: "Five percent to book", slug: "BOOK5",
		imagery:    "Resort-style apartment buildings by the water on Yas Island, a lagoon pool and beach in the foreground, late afternoon sun, sky clear for a headline",
		tints:      [2]color.RGBA{{70, 150, 160, 255}, {225, 200, 160, 255}},
		renderName: [2]string{"Lagoon pool", "Beachfront"},
		logo:       "https://reportagegroup.com/wp-content/uploads/Selina-bay-logo-final-01.jpg",
	},
	{
		name: "Sensi", location: "Arjan, Dubai", units: "Studios and 1 bedroom apartments",
		priceAED: 420_000, priceText: "AED 420K", priceAr: "420 ألف درهم", handover: "2027-09-30",
		plan: "40% during construction, 60% over 3 years after handover", discount: 5,
		offer: "Pay 60% after handover", offerAr: "ادفع 60% بعد التسليم",
		concept: "Pay after handover", slug: "POST_HANDOVER",
		imagery:    "A boutique mid-rise apartment building in Arjan with a rooftop pool and garden terraces at golden hour, young professionals far in the distance, sky above for a headline",
		tints:      [2]color.RGBA{{180, 110, 90, 255}, {120, 120, 140, 255}},
		renderName: [2]string{"Rooftop pool", "Garden terrace"},
		logo:       "https://reportagegroup.com/wp-content/uploads/sensi.png",
	},
}

// runSeedDemo fills a dev database with Reportage and its developments to
// try the studio on. Each project gets approved facts (price, handover,
// payment plan, units, location, maximum discount), an approved offer, and a
// creative project with two placeholder renders and a concept with copy in
// both languages and a picture prompt. Projects that already exist are left
// alone, so running it again adds only what's missing.
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
	admin := core.Actor{UserID: 0, Role: core.RoleAdmin, Via: core.ViaStartup, WorkspaceID: core.DefaultWorkspace}
	marketer := core.Actor{UserID: 0, Role: core.RoleMarketer, Via: core.ViaStartup, WorkspaceID: core.DefaultWorkspace}
	br := brands.New(db, brands.Config{})
	st := studio.New(db, files)

	var brandID int64
	existing, err := br.List(ctx, admin)
	if err != nil {
		return err
	}
	for _, b := range existing {
		if b.Name == "Reportage" && b.Kind == brands.Own {
			brandID = b.ID
		}
	}
	if brandID == 0 {
		b, err := br.Create(ctx, admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own, Notes: "Developer. Demo data from adx seed-demo."})
		if err != nil {
			return err
		}
		brandID = b.ID
	}

	added := 0
	for _, d := range demoProjects {
		_, err := db.Reader().GetProjectByName(ctx, store.GetProjectByNameParams{WorkspaceID: core.DefaultWorkspace, Name: d.name})
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := seedProject(ctx, db, br, st, admin, marketer, brandID, d); err != nil {
			return fmt.Errorf("%s: %w", d.name, err)
		}
		added++
		slog.Info("seed-demo: added", "project", d.name)
	}
	logos := 0
	for _, d := range demoProjects {
		ok, err := seedLogo(ctx, db, st, marketer, d)
		if err != nil {
			// The site may be down or moved; the projects are still usable.
			slog.Warn("seed-demo: no logo", "project", d.name, "err", err)
			continue
		}
		if ok {
			logos++
			slog.Info("seed-demo: logo added", "project", d.name)
		}
	}
	if added == 0 && logos == 0 {
		slog.Info("seed-demo: every demo project and logo is already there, nothing done")
		return nil
	}
	slog.Info("seed-demo done", "projects", added, "logos", logos)
	return nil
}

// seedLogo fetches the project's logo from reportagegroup.com into its brand
// kit as the logo on light, unless the kit already has one.
func seedLogo(ctx context.Context, db *store.DB, st *studio.Studio, marketer core.Actor, d demoProject) (bool, error) {
	if d.logo == "" {
		return false, nil
	}
	p, err := db.Reader().GetProjectByName(ctx, store.GetProjectByNameParams{WorkspaceID: core.DefaultWorkspace, Name: d.name})
	if err != nil {
		return false, err
	}
	cp, err := st.Project(ctx, marketer, p.ID)
	if err != nil {
		return false, err
	}
	kit := map[string]any{}
	if len(cp.Settings.BrandKit) > 0 {
		if err := json.Unmarshal(cp.Settings.BrandKit, &kit); err != nil {
			return false, err
		}
	}
	logos, _ := kit["logos"].(map[string]any)
	if logos == nil {
		logos = map[string]any{}
	}
	if logos["onLight"] != nil {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.logo, nil)
	if err != nil {
		return false, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("%s: %s", d.logo, res.Status)
	}
	ref, err := st.Upload(ctx, marketer, res.Body)
	if err != nil {
		return false, err
	}
	asset := map[string]any{"sha256": ref.SHA256, "name": path.Base(d.logo)}
	if ref.Width != nil && ref.Height != nil {
		asset["width"], asset["height"] = *ref.Width, *ref.Height
	}
	logos["onLight"] = asset
	kit["logos"] = logos
	b, err := json.Marshal(kit)
	if err != nil {
		return false, err
	}
	if _, err := st.SaveBrandKit(ctx, marketer, p.ID, b); err != nil {
		return false, err
	}
	return true, nil
}

func seedProject(ctx context.Context, db *store.DB, br *brands.Brands, st *studio.Studio, admin, marketer core.Actor, brandID int64, d demoProject) error {
	p, err := db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: d.name, CreatedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	if err := br.AssignProject(ctx, admin, p.ID, brandID); err != nil {
		return err
	}
	src := "Demo data"
	facts := []brands.FactInput{
		{Key: "price.from", Kind: brands.KindMoney, Value: fmt.Sprintf("%d", d.priceAED*100), Currency: "AED"},
		{Key: "handover", Kind: brands.KindDate, Value: d.handover},
		{Key: "payment_plan", Kind: brands.KindText, Value: d.plan},
		{Key: "unit_types", Kind: brands.KindText, Value: d.units},
		{Key: "location", Kind: brands.KindText, Value: d.location},
		{Key: "discount.max", Kind: brands.KindPercent, Value: fmt.Sprintf("%g", d.discount)},
	}
	for _, f := range facts {
		f.ProjectID, f.EffectiveFrom, f.SourceKind, f.SourceRef = p.ID, "2026-10-01", brands.SourceDocument, src
		a, err := br.AddFact(ctx, admin, brandID, f)
		if err != nil {
			return fmt.Errorf("fact %s: %w", f.Key, err)
		}
		if _, err := br.ApproveFact(ctx, admin, a.Fact.ID); err != nil {
			return fmt.Errorf("approve %s: %w", f.Key, err)
		}
	}
	pct := d.discount
	offer, err := br.CreateOffer(ctx, admin, brandID, brands.OfferInput{
		ProjectID: p.ID, Name: d.concept, EffectiveFrom: "2026-10-01",
		Document: brands.OfferDocument{
			Headline: d.offer, HeadlineAr: d.offerAr, DiscountPct: &pct,
			PriceFrom:   &brands.Price{Minor: d.priceAED * 100, Currency: "AED"},
			PaymentPlan: d.plan, Conditions: []string{"Selected units"},
		},
	})
	if err != nil {
		return err
	}
	if _, err := br.SetOfferState(ctx, admin, offer.ID, brands.OfferApproved); err != nil {
		return err
	}

	if _, err := st.SetUp(ctx, marketer, studio.SetupInput{ProjectID: p.ID}); err != nil {
		return err
	}
	var renders []studio.NewRender
	for i, c := range d.tints {
		fref, err := st.Upload(ctx, marketer, bytes.NewReader(picture(1080, 1920, c)))
		if err != nil {
			return err
		}
		pref, err := st.Upload(ctx, marketer, bytes.NewReader(picture(270, 480, c)))
		if err != nil {
			return err
		}
		renders = append(renders, studio.NewRender{File: fref.SHA256, Preview: pref.SHA256, Label: d.renderName[i]})
	}
	if _, err := st.AddRenders(ctx, marketer, p.ID, renders); err != nil {
		return err
	}
	k, err := st.CreateConcept(ctx, marketer, p.ID, studio.NewConcept{Code: "C1", Name: d.concept})
	if err != nil {
		return err
	}
	year := d.handover[:4]
	pctText := fmt.Sprintf("%g%%", d.discount)
	copyDoc, _ := json.Marshal(map[string]any{
		"v": 1, "render": "R01", "arReviewed": false,
		"en": map[string]any{
			"eyebrow": d.name, "title": "From " + d.priceText, "titleSize": "lg",
			"text":  d.units + ", handover " + year + ".",
			"stats": []map[string]string{{"figure": d.priceText, "label": "from"}, {"figure": pctText, "label": "off"}},
			"event": "", "note": "", "info": "Selected units.", "cta": "Register",
		},
		"ar": map[string]any{
			"eyebrow": d.name, "title": "تبدأ من " + d.priceAr, "titleSize": "lg",
			"text":  "التسليم " + year + ".",
			"stats": []map[string]string{{"figure": d.priceAr, "label": "تبدأ من"}, {"figure": pctText, "label": "خصم"}},
			"event": "", "note": "", "info": "على وحدات مختارة.", "cta": "سجّل الآن",
		},
	})
	if _, err := st.SaveConcept(ctx, marketer, k.ID, studio.ConceptInput{
		Name: d.concept, Why: "price.from and the offer, for buyers comparing " + d.location,
		PrimaryText: d.name + " in " + d.location + ": " + d.units + " from " + d.priceText + ". " + d.offer + ".",
		Headline:    "From " + d.priceText, Description: d.offer, CTA: "Learn more", Copy: copyDoc,
	}); err != nil {
		return err
	}
	return db.Writer().SetConceptImagery(ctx, store.SetConceptImageryParams{Imagery: d.imagery, UpdatedAt: time.Now().UTC(), ID: k.ID})
}

// picture is a placeholder render: a sky to ground gradient with a few
// blocks for buildings, in one tint per picture.
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
