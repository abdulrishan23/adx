package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/store"
)

// UseModel gives the pages a language model for drafting. Without one,
// Draft refuses with a conflict rather than a stub.
func (p *Pages) UseModel(m assistant.Model) { p.model = m }

const (
	// DraftTool is the tool the model answers through; tests script it.
	DraftTool      = "propose_page"
	draftMaxTokens = 6000
	draftSystem    = `You write landing pages for a property developer's campaign, one page in one language. The page is built from sections and must sell the project with only the facts you are given.

Rules:
- Every number, price, discount, date, payment plan, size, percentage or count you write must be one of the approved facts or the offer, word for word in value. If no fact supports a figure, leave the figure out and say the idea without it. Every value in "stats" and every cell in "plan" must come from a fact or the offer.
- Arabic is a native Gulf copywriter's own writing, formal enough for a developer, not a translation.
- Sections, in a sensible order: one "hero" (its text is one line over the picture), "stats" (two to six figures with short labels), "text" and "heading" blocks, "bullets" for what the project offers, "plan" for the payment plan when the offer has one (rows of two cells: share and milestone), "location" (the address; leave the link empty unless one was given), "offer" only when an offer is given, and one "form" at the end.
- Pictures: use only the picture ids you are given, the first for the hero and up to six others in a "gallery". Never invent an id.
- The form: name and phone required, email optional, a short consent line, a submit label and a success message in the page's language.
- Keep the whole page short enough to read on a phone in a minute.`
)

type PageDraftInput struct {
	ProjectID    int64  `json:"projectId"`
	Language     string `json:"language"` // en or ar; both when empty
	Slug         string `json:"slug"`     // base slug; the Arabic twin gets -ar
	Name         string `json:"name"`
	OfferID      int64  `json:"offerId"`
	Instructions string `json:"instructions"`
	// PageID, when set, drafts a new version of that page instead of
	// creating one (its language and slug are kept).
	PageID int64 `json:"pageId"`
}

type PageDraft struct {
	Page    Page        `json:"page"`
	Version PageVersion `json:"version"`
	Notes   []string    `json:"notes"`
}

type PageDrafts struct {
	Pages []PageDraft     `json:"pages"`
	Model string          `json:"model"`
	Usage assistant.Usage `json:"usage"`
}

type proposedPage struct {
	Title       string      `json:"title"`
	Headline    string      `json:"headline"`
	Subheadline string      `json:"subheadline"`
	Sections    []Section   `json:"sections"`
	Form        *FormConfig `json:"form"`
	Footer      string      `json:"footer"`
}

var slugTail = regexp.MustCompile(`[^a-z0-9]+`)

// Draft asks the model for a landing page from the brand's approved facts,
// the offer and the studio's pictures for the project, and saves it as a
// page version a person edits and publishes. With no language it drafts
// English and Arabic as twins that link to each other. Figures the facts
// don't support are dropped and said in the notes.
func (p *Pages) Draft(ctx context.Context, actor core.Actor, in PageDraftInput) (PageDrafts, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return PageDrafts{}, core.ErrForbidden
	}
	if p.model == nil {
		return PageDrafts{}, core.Conflict("no model is configured on this server; write the page by hand or ask an admin to configure ADX_MODEL_API_KEY")
	}
	langs := []string{"en", "ar"}
	switch in.Language {
	case "en", "ar":
		langs = []string{in.Language}
	case "":
	default:
		return PageDrafts{}, core.Invalid("language must be en, ar or empty for both")
	}
	var existing *store.Page
	if in.PageID != 0 {
		row, err := p.page(ctx, actor.WorkspaceID, in.PageID)
		if err != nil {
			return PageDrafts{}, err
		}
		existing = &row
		langs = []string{row.Language}
		in.ProjectID = row.ProjectID.Int64
	}
	if in.ProjectID == 0 {
		return PageDrafts{}, core.Invalid("a project is required")
	}
	gc, err := p.draftContext(ctx, actor, in.ProjectID, in.OfferID)
	if err != nil {
		return PageDrafts{}, err
	}
	base := strings.ToLower(strings.TrimSpace(in.Slug))
	if base == "" {
		base = strings.Trim(slugTail.ReplaceAllString(strings.ToLower(gc.project), "-"), "-")
	}
	if existing == nil && (len(base) < 3 || len(base) > 56) {
		return PageDrafts{}, core.Invalid("slug must be 3 to 56 lowercase letters, digits and hyphens")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = gc.project + " launch"
	}
	out := PageDrafts{Pages: []PageDraft{}}
	slugFor := func(lang string) string {
		if existing != nil {
			return existing.Slug
		}
		if lang == "ar" {
			return base + "-ar"
		}
		return base
	}
	for _, lang := range langs {
		twin := ""
		if len(langs) == 2 {
			if lang == "en" {
				twin = slugFor("ar")
			} else {
				twin = slugFor("en")
			}
		}
		req := p.draftRequest(gc, lang, in.Instructions)
		resp, err := p.model.Complete(ctx, req)
		if err != nil {
			var me *assistant.ModelError
			if errors.As(err, &me) {
				return out, core.Unavailable(err, "%s", me.Safe)
			}
			return out, core.Unavailable(err, "the model did not answer")
		}
		out.Model = resp.Model
		out.Usage.InputTokens += resp.Usage.InputTokens
		out.Usage.OutputTokens += resp.Usage.OutputTokens
		var proposed *proposedPage
		for _, b := range resp.Content {
			if b.Type == "tool_use" && b.ToolName == DraftTool {
				var pp proposedPage
				if err := json.Unmarshal(b.Input, &pp); err != nil {
					return out, core.Invalid("the model's proposal was not a page; nothing was saved")
				}
				proposed = &pp
				break
			}
		}
		if proposed == nil {
			if resp.StopReason == "refusal" {
				return out, core.Invalid("the model declined to draft the page; nothing was saved")
			}
			return out, core.Invalid("the model answered in prose instead of a page; nothing was saved")
		}
		doc, notes := p.constrainDraft(*proposed, gc, twin)
		if existing != nil {
			v, err := p.SaveVersion(ctx, actor, existing.ID, PageVersionInput{Document: doc, Note: "drafted by Adx from the approved facts"})
			if err != nil {
				return out, err
			}
			out.Pages = append(out.Pages, PageDraft{Page: pageFromRow(*existing), Version: v, Notes: notes})
			continue
		}
		pageName := name
		if lang == "ar" {
			pageName += " (Arabic)"
		}
		d, err := p.Create(ctx, actor, PageInput{BrandID: gc.brandID, ProjectID: in.ProjectID, Slug: slugFor(lang), Name: pageName, Language: lang, Document: doc})
		if err != nil {
			return out, err
		}
		out.Pages = append(out.Pages, PageDraft{Page: d.Page, Version: d.Versions[len(d.Versions)-1], Notes: notes})
	}
	return out, nil
}

type draftContext struct {
	brandID        int64
	brand, project string
	claims         []brands.Fact
	pending        []string
	offer          *brands.Offer
	pictures       []string // studio file SHA-256s
	values         map[string]bool
}

var draftDigits = regexp.MustCompile(`[0-9][0-9,.]*`)

func (p *Pages) draftContext(ctx context.Context, actor core.Actor, projectID, offerID int64) (draftContext, error) {
	gc := draftContext{values: map[string]bool{}}
	brandID, offers, err := p.brands.OffersForProject(ctx, actor, projectID)
	if err != nil {
		return gc, err
	}
	if brandID == 0 {
		return gc, core.Invalid("the project has no brand; attach it to one first")
	}
	gc.brandID = brandID
	b, err := p.brands.Get(ctx, actor, brandID)
	if err != nil {
		return gc, err
	}
	gc.brand = b.Brand.Name
	for _, pr := range b.Projects {
		if pr.ID == projectID {
			gc.project = pr.Name
		}
	}
	if gc.project == "" {
		return gc, core.NotFound("project %d is not one of %s's", projectID, gc.brand)
	}
	set, err := p.brands.Claims(ctx, actor, brandID, projectID, p.clock.Now())
	if err != nil {
		return gc, err
	}
	gc.claims, gc.pending = set.Claims, set.Pending
	for _, o := range offers {
		if offerID != 0 && o.ID == offerID {
			oc := o
			gc.offer = &oc
		}
	}
	if offerID != 0 && gc.offer == nil {
		return gc, core.NotFound("offer %d is not this project's", offerID)
	}
	for _, f := range gc.claims {
		for _, d := range draftDigits.FindAllString(f.Value, -1) {
			gc.values[normalizeDraftFigure(d)] = true
		}
		if f.Kind == brands.KindMoney {
			var minor int64
			if _, err := fmt.Sscanf(f.Value, "%d", &minor); err == nil {
				major := minor / 100
				for _, v := range []string{fmt.Sprintf("%d", major), fmt.Sprintf("%g", float64(major)/1e6), fmt.Sprintf("%g", float64(major)/1e3)} {
					gc.values[normalizeDraftFigure(v)] = true
				}
			}
		}
	}
	if gc.offer != nil {
		d := gc.offer.Current.Document
		for _, text := range []string{d.Headline, d.HeadlineAr, d.Terms, d.TermsAr, d.PaymentPlan} {
			for _, x := range draftDigits.FindAllString(text, -1) {
				gc.values[normalizeDraftFigure(x)] = true
			}
		}
		for _, c := range d.Conditions {
			for _, x := range draftDigits.FindAllString(c, -1) {
				gc.values[normalizeDraftFigure(x)] = true
			}
		}
		if d.DiscountPct != nil {
			gc.values[normalizeDraftFigure(fmt.Sprintf("%g", *d.DiscountPct))] = true
		}
		if d.PriceFrom != nil {
			major := d.PriceFrom.Minor / 100
			for _, v := range []string{fmt.Sprintf("%d", major), fmt.Sprintf("%g", float64(major)/1e6), fmt.Sprintf("%g", float64(major)/1e3)} {
				gc.values[normalizeDraftFigure(v)] = true
			}
		}
	}
	// The studio's pictures for the project: its renders, newest first.
	renders, err := p.db.Reader().ListRenders(ctx, projectID)
	if err == nil {
		for i := len(renders) - 1; i >= 0 && len(gc.pictures) < 8; i-- {
			if renders[i].Hidden == 1 {
				continue
			}
			f, err := p.db.Reader().GetFile(ctx, renders[i].FileID)
			if err == nil {
				gc.pictures = append(gc.pictures, f.Sha256)
			}
		}
	}
	return gc, nil
}

func (p *Pages) draftRequest(gc draftContext, lang, instructions string) assistant.Request {
	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s by %s. Language: %s.\n\nApproved facts (the only figures you may state):\n", gc.project, gc.brand, map[string]string{"en": "English", "ar": "Arabic"}[lang])
	if len(gc.claims) == 0 {
		b.WriteString("(none approved yet; write the page without figures)\n")
	}
	for _, f := range gc.claims {
		fmt.Fprintf(&b, "- %s = %s", f.Key, f.Value)
		if f.Currency != nil {
			fmt.Fprintf(&b, " %s", *f.Currency)
		}
		b.WriteString("\n")
	}
	if len(gc.pending) > 0 {
		fmt.Fprintf(&b, "Facts still under review, not to be stated: %s\n", strings.Join(gc.pending, ", "))
	}
	if gc.offer != nil {
		d := gc.offer.Current.Document
		fmt.Fprintf(&b, "\nOffer (%s): %s", gc.offer.Current.State, d.Headline)
		if d.HeadlineAr != "" {
			fmt.Fprintf(&b, " / %s", d.HeadlineAr)
		}
		if d.Terms != "" {
			fmt.Fprintf(&b, "\nTerms: %s", d.Terms)
		}
		if d.PaymentPlan != "" {
			fmt.Fprintf(&b, "\nPayment plan: %s", d.PaymentPlan)
		}
		if d.DiscountPct != nil {
			fmt.Fprintf(&b, "\nDiscount: %g%%", *d.DiscountPct)
		}
		if len(d.Conditions) > 0 {
			fmt.Fprintf(&b, "\nConditions: %s", strings.Join(d.Conditions, "; "))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("\nNo offer is bound to this page; do not include an offer section.\n")
	}
	b.WriteString("\nPicture ids you may use:\n")
	if len(gc.pictures) == 0 {
		b.WriteString("(none; leave out the hero and the gallery)\n")
	}
	for i, sum := range gc.pictures {
		fmt.Fprintf(&b, "- %s%s\n", sum, map[bool]string{true: " (newest)", false: ""}[i == 0])
	}
	if strings.TrimSpace(instructions) != "" {
		fmt.Fprintf(&b, "\nFrom the marketer: %s\n", strings.TrimSpace(instructions))
	}
	return assistant.Request{
		System:     draftSystem,
		Messages:   []assistant.Message{{Role: assistant.RoleUser, Content: []assistant.Block{{Type: "text", Text: b.String()}}}},
		Tools:      []assistant.ToolSpec{{Name: DraftTool, Description: "Propose the landing page.", InputSchema: draftSchema()}},
		MaxTokens:  draftMaxTokens,
		ToolChoice: DraftTool,
	}
}

func draftSchema() map[string]any {
	str := map[string]any{"type": "string"}
	strs := map[string]any{"type": "array", "items": str}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": str, "headline": str, "subheadline": str, "footer": str,
			"sections": map[string]any{"type": "array", "items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":     map[string]any{"type": "string", "enum": []string{"hero", "heading", "text", "bullets", "stats", "gallery", "plan", "location", "offer", "form"}},
					"text":     str,
					"items":    strs,
					"pictures": strs,
					"alt":      str,
					"link":     str,
					"stats":    map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"label": str, "value": str}, "required": []string{"label", "value"}}},
					"table":    map[string]any{"type": "array", "items": strs},
				},
				"required": []string{"kind"},
			}},
			"form": map[string]any{"type": "object", "properties": map[string]any{
				"fields": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
					"name": str, "label": str, "kind": map[string]any{"type": "string", "enum": []string{"text", "email", "phone", "select"}}, "required": map[string]any{"type": "boolean"}, "options": strs,
				}, "required": []string{"name", "label", "kind"}}},
				"consentText": str, "submitLabel": str, "successMessage": str,
			}, "required": []string{"fields", "submitLabel", "successMessage"}},
		},
		"required": []string{"title", "headline", "sections", "form"},
	}
}

// constrainDraft keeps the proposal within the facts and the pictures: a
// stat or plan cell whose figure nothing states is dropped and noted; a
// picture id not given is dropped; an offer section without an offer goes.
func (p *Pages) constrainDraft(pp proposedPage, gc draftContext, twin string) (PageDocument, []string) {
	notes := []string{}
	allowed := map[string]bool{}
	for _, s := range gc.pictures {
		allowed[s] = true
	}
	supported := func(s string) bool {
		for _, d := range draftDigits.FindAllString(s, -1) {
			if !gc.values[normalizeDraftFigure(d)] {
				return false
			}
		}
		return true
	}
	doc := PageDocument{Title: pp.Title, Headline: pp.Headline, Subheadline: pp.Subheadline, Footer: pp.Footer, Form: pp.Form, Sections: []Section{}, TwinSlug: twin}
	hero := false
	for _, s := range pp.Sections {
		switch s.Kind {
		case "hero", "gallery":
			kept := []string{}
			for _, sum := range s.Pictures {
				if allowed[sum] {
					kept = append(kept, sum)
				}
			}
			if len(kept) == 0 {
				notes = append(notes, fmt.Sprintf("dropped the %s: no picture the studio has", s.Kind))
				continue
			}
			if s.Kind == "hero" {
				if hero {
					continue
				}
				hero = true
				kept = kept[:1]
			}
			s.Pictures = kept
		case "stats":
			kept := []Stat{}
			for _, st := range s.Stats {
				if supported(st.Value) {
					kept = append(kept, st)
				} else {
					notes = append(notes, fmt.Sprintf("dropped the figure %q (%s): no approved fact states it", st.Value, st.Label))
				}
			}
			if len(kept) == 0 {
				continue
			}
			s.Stats = kept
		case "plan":
			kept := [][]string{}
			for _, row := range s.Table {
				ok := true
				for _, c := range row {
					if !supported(c) {
						ok = false
					}
				}
				if ok {
					kept = append(kept, row)
				} else {
					notes = append(notes, fmt.Sprintf("dropped the payment plan row %q: no approved fact or offer states it", strings.Join(row, " · ")))
				}
			}
			if len(kept) == 0 {
				continue
			}
			s.Table = kept
		case "offer":
			if gc.offer == nil {
				continue
			}
		case "text", "heading", "bullets":
			if !supported(s.Text) {
				notes = append(notes, fmt.Sprintf("a figure in %q is not an approved fact; check it before publishing", clipText(s.Text, 60)))
			}
		}
		doc.Sections = append(doc.Sections, s)
	}
	if gc.offer != nil {
		doc.OfferID = &gc.offer.ID
		v := gc.offer.Current.Version
		doc.OfferVersion = &v
	}
	if doc.Form == nil {
		doc.Form = &FormConfig{Fields: []FormField{{Name: "name", Label: "Name", Kind: "text", Required: true}, {Name: "phone", Label: "Phone", Kind: "phone", Required: true}}, SubmitLabel: "Register", SuccessMessage: "Thank you, we will call you."}
	}
	hasForm := false
	for _, s := range doc.Sections {
		if s.Kind == "form" {
			hasForm = true
		}
	}
	if !hasForm {
		doc.Sections = append(doc.Sections, Section{Kind: "form"})
	}
	return doc, notes
}

func normalizeDraftFigure(s string) string {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimRight(s, ".")
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

func clipText(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}
