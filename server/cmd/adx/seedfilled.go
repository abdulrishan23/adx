package main

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"log/slog"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/campaigns"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/pages"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/studio"
	"github.com/WokeWorks/adx/server/store"
)

// seedFilled takes The Wadi's launch campaign through every stage short of
// going live, so each one has something to look at: six ads in two ideas
// with the CMO's decisions on them, an English page published and its
// Arabic twin still a draft, and an accepted plan. It runs once: a campaign
// that already has ads is left alone.
func seedFilled(ctx context.Context, db *store.DB, br *brands.Brands, st *studio.Studio, cs *campaigns.Campaigns, admin, marketer core.Actor) (bool, error) {
	d := demoProjects[0]
	p, err := db.Reader().GetProjectByName(ctx, store.GetProjectByNameParams{WorkspaceID: core.DefaultWorkspace, Name: d.name})
	if err != nil {
		return false, err
	}
	list, err := cs.ListForProject(ctx, marketer, p.ID)
	if err != nil || len(list) == 0 {
		return false, err
	}
	camp, err := cs.Get(ctx, marketer, list[0].ID)
	if err != nil {
		return false, err
	}
	if camp.Ads.Total > 0 {
		return false, nil
	}

	// Ads: the first idea in four sizes, a second in two. The pictures are
	// the project's tints, since the demo has no finished artwork.
	cp, err := st.Project(ctx, marketer, p.ID)
	if err != nil {
		return false, err
	}
	first := cp.Concepts[0].Code
	second, err := st.CreateConcept(ctx, marketer, p.ID, studio.NewConcept{Code: "C2", Name: "First villa", CampaignID: camp.ID})
	if err != nil {
		return false, err
	}
	if _, err := st.SaveConcept(ctx, marketer, second.ID, studio.ConceptInput{
		Name: "First villa", Why: "The payment plan, for first-time villa buyers moving up from an apartment.",
		Copy: []byte(`{"v":1,"render":"R02","arReviewed":false,"en":{"eyebrow":"The Wadi","title":"Your first villa","titleSize":"lg","text":"10% to book, the rest as it's built.","stats":[],"event":"","note":"","info":"Selected units.","cta":"Register"},"ar":{"eyebrow":"ذا وادي","title":"فيلتك الأولى","titleSize":"lg","text":"10% للحجز والباقي مع البناء.","stats":[],"event":"","note":"","info":"وحدات مختارة.","cta":"سجّل"}}`),
	}); err != nil {
		return false, err
	}
	type adSpec struct {
		concept, slug string
		lang          studio.Lang
		fmt           studio.Fmt
		w, h          int
		tint          color.RGBA
	}
	specs := []adSpec{
		{first, d.slug, studio.LangEN, studio.FmtStory, 1080, 1920, d.tints[0]},
		{first, d.slug, studio.LangAR, studio.FmtStory, 1080, 1920, d.tints[0]},
		{first, d.slug, studio.LangEN, studio.FmtFeed, 1080, 1350, d.tints[1]},
		{first, d.slug, studio.LangAR, studio.FmtFeed, 1080, 1350, d.tints[1]},
		{"C2", "FIRST_VILLA", studio.LangEN, studio.FmtStory, 1080, 1920, color.RGBA{122, 106, 88, 255}},
		{"C2", "FIRST_VILLA", studio.LangAR, studio.FmtStory, 1080, 1920, color.RGBA{122, 106, 88, 255}},
	}
	uploads := make([]studio.UploadedAd, 0, len(specs))
	for i, s := range specs {
		// A different shade per ad, since identical bytes are stored once.
		t := s.tint
		t.B = uint8(int(t.B)+i*7) % 255
		f, err := st.Upload(ctx, marketer, bytes.NewReader(picture(s.w, s.h, t)))
		if err != nil {
			return false, err
		}
		pv, err := st.Upload(ctx, marketer, bytes.NewReader(picture(s.w/4, s.h/4, t)))
		if err != nil {
			return false, err
		}
		uploads = append(uploads, studio.UploadedAd{Concept: s.concept, Slug: s.slug, Lang: s.lang, Fmt: s.fmt, File: f.SHA256, Preview: pv.SHA256})
	}
	if _, err := st.AddUploads(ctx, marketer, p.ID, uploads); err != nil {
		return false, err
	}
	cp, err = st.Project(ctx, marketer, p.ID)
	if err != nil {
		return false, err
	}
	// The CMO's decisions: the launch idea approved except the Arabic feed,
	// the second idea still waiting.
	for _, k := range cp.Concepts {
		if k.Code != first {
			continue
		}
		for _, a := range k.Ads {
			status := "approved"
			if a.Lang == studio.LangAR && a.Fmt == studio.FmtFeed {
				status = "revise"
			}
			if err := st.Decide(ctx, core.Anonymous(core.ViaWeb), cp.Settings.ReviewKey, studio.BoardDecision{Ad: a.Code, Name: "Dana", Status: status}); err != nil {
				return false, fmt.Errorf("decide %s: %w", a.Code, err)
			}
			if status == "revise" {
				if _, err := st.Comment(ctx, core.Anonymous(core.ViaWeb), cp.Settings.ReviewKey, studio.CommentInput{Ad: a.Code, Name: "Dana", Body: "The discount line wraps on the feed size. One line, please."}); err != nil {
					return false, err
				}
			}
		}
	}

	// Page: English published, Arabic twin drafted.
	brandID, offers, err := br.OffersForProject(ctx, admin, p.ID)
	if err != nil {
		return false, err
	}
	var offerID int64
	for _, o := range offers {
		if o.ProjectID != nil && *o.ProjectID == p.ID {
			offerID = o.ID
		}
	}
	pg := pages.New(db, br, evidence.New(db, core.SystemClock{}), pages.Config{Salt: "demo"})
	hero := cp.Renders[0].File.SHA256
	gallery := []string{}
	for _, r := range cp.Renders {
		gallery = append(gallery, r.File.SHA256)
	}
	form := &pages.FormConfig{Fields: []pages.FormField{{Name: "name", Label: "Name", Kind: "text", Required: true}, {Name: "phone", Label: "Phone", Kind: "phone", Required: true}}, SubmitLabel: "Register", SuccessMessage: "Thank you, we will call you."}
	en, err := pg.Create(ctx, marketer, pages.PageInput{
		BrandID: brandID, ProjectID: p.ID, CampaignID: camp.ID, Slug: "wadi-launch", Name: "The Wadi launch weekend", Language: "en",
		Document: pages.PageDocument{
			Title: "The Wadi · Launch weekend", Headline: "Up to 15% off at the launch weekend", Subheadline: "3 and 4 bedroom villas in Dubailand, from AED 1,200,000.",
			OfferID: &offerID, Form: form, TwinSlug: "wadi-launch-ar",
			Sections: []pages.Section{
				{Kind: "hero", Pictures: []string{hero}, Text: "Villas at dusk"},
				{Kind: "stats", Stats: []pages.Stat{{Label: "Starting price", Value: "AED 1,200,000"}, {Label: "Handover", Value: "December 2027"}, {Label: "Launch discount", Value: "15%"}}},
				{Kind: "text", Text: "A gated community of villas off Al Ain Road, with a park at its centre and a pool for every cluster."},
				{Kind: "gallery", Pictures: gallery},
				{Kind: "plan", Text: "Payment plan", Table: [][]string{{"10%", "On booking"}, {"40%", "During construction"}, {"50%", "On handover"}}},
				{Kind: "location", Text: "Dubailand, off Al Ain Road", Link: "https://maps.google.com/?q=Dubailand"},
				{Kind: "form"},
			},
		},
	})
	if err != nil {
		return false, fmt.Errorf("page en: %w", err)
	}
	if _, err := pg.Publish(ctx, marketer, en.Page.ID, en.Versions[len(en.Versions)-1].Version); err != nil {
		return false, fmt.Errorf("publish en: %w", err)
	}
	if _, err := pg.Create(ctx, marketer, pages.PageInput{
		BrandID: brandID, ProjectID: p.ID, CampaignID: camp.ID, Slug: "wadi-launch-ar", Name: "ذا وادي · عطلة الإطلاق", Language: "ar",
		Document: pages.PageDocument{
			Title: "ذا وادي · عطلة الإطلاق", Headline: "خصم يصل إلى 15% في عطلة الإطلاق", Subheadline: "فلل من 3 و4 غرف نوم في دبي لاند، تبدأ من 1,200,000 درهم.",
			OfferID: &offerID, Form: form, TwinSlug: "wadi-launch",
			Sections: []pages.Section{
				{Kind: "hero", Pictures: []string{hero}, Text: "الفلل عند الغروب"},
				{Kind: "stats", Stats: []pages.Stat{{Label: "السعر يبدأ من", Value: "1,200,000 درهم"}, {Label: "التسليم", Value: "ديسمبر 2027"}, {Label: "خصم الإطلاق", Value: "15%"}}},
				{Kind: "plan", Text: "خطة الدفع", Table: [][]string{{"10%", "عند الحجز"}, {"40%", "أثناء البناء"}, {"50%", "عند التسليم"}}},
				{Kind: "location", Text: "دبي لاند، قبالة طريق العين", Link: "https://maps.google.com/?q=Dubailand"},
				{Kind: "form"},
			},
		},
	}); err != nil {
		return false, fmt.Errorf("page ar: %w", err)
	}

	// Plan: AED 200,000 over the weekend, Meta lead forms and Google search,
	// accepted by the admin.
	pl := plans.New(db, br, plans.Config{})
	cpl := int64(1_200 * 100)
	assumed := "AED 1,050 to 1,330 per lead, from the last Reportage villa launch"
	plan, err := pl.Create(ctx, marketer, plans.PlanInput{
		BrandID: brandID, ProjectID: p.ID, CampaignID: camp.ID, Title: d.concept,
		Document: plans.PlanDocument{
			Objective:   "150 qualified leads for The Wadi at AED 1,300 or less each over the launch weekend",
			BudgetMinor: 200_000 * 100, Currency: "AED", PeriodFrom: "2026-10-24", PeriodTo: "2026-10-26",
			Allocations: []plans.Allocation{
				{Channel: "meta_lead_form", Objective: "leads", BudgetMinor: 80_000 * 100, Geography: "UAE", Audience: "Families 30 to 50 in Dubai and Abu Dhabi", ExpectedCPLMinor: &cpl},
				{Channel: "meta_traffic", Objective: "leads", BudgetMinor: 40_000 * 100, Geography: "India", Audience: "Investors interested in Dubai property", ExpectedCPLMinor: &cpl},
				{Channel: "google_search", Objective: "leads", BudgetMinor: 50_000 * 100, Geography: "UAE", Audience: "Villa and Dubailand searches", ExpectedCPLMinor: &cpl},
				{Channel: "google_pmax", Objective: "leads", BudgetMinor: 30_000 * 100, Geography: "UAE and India", Audience: "Off-plan buyers", ExpectedCPLMinor: &cpl},
			},
			AssetRequirements:       []plans.AssetRequirement{},
			PageRequirements:        []plans.PageRequirement{{Kind: "landing", Language: "en"}, {Kind: "landing", Language: "ar"}},
			MeasurementRequirements: []plans.MeasurementRequirement{{Kind: "crm_stage", Platform: "ghl", Name: "Qualified lead", Required: true}},
			TestHypotheses:          []plans.TestHypothesis{},
			ObservationWindows:      []plans.ObservationWindow{},
			EvidenceRefs:            []plans.PlanEvidenceRef{{Kind: "offer", ID: offerID}},
			Assumptions:             []plans.Assumption{{Key: "cpl", Text: assumed, Value: &assumed, Class: "assumption"}},
		},
	})
	if err != nil {
		return false, fmt.Errorf("plan: %w", err)
	}
	if _, err := pl.Accept(ctx, admin, plan.Plan.ID, 1); err != nil {
		return false, fmt.Errorf("accept plan: %w", err)
	}
	slog.Info("seed-demo: filled the launch campaign", "project", d.name)
	return true, nil
}
