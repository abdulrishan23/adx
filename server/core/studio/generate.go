package studio

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/store"
)

// Generation is what the studio can ask a model for. Without it, the
// generate functions refuse with a conflict rather than a stub.
type Generation struct {
	Model  assistant.Model
	Brands *brands.Brands
}

// Generate configures the studio's model-backed functions.
func (c *Studio) Generate(g Generation) { c.gen = g }

const (
	conceptsTool        = "propose_concepts"
	conceptsMaxTokens   = 6000
	maxConceptsPerAsk   = 6
	conceptsSystemBrief = `You write ad concepts for a property developer's campaign. A concept is one idea for a project, with copy in English and in Arabic, and a description of the picture it sits on.

Rules:
- Every number, price, discount, date, payment plan or percentage you write must be one of the approved facts or offers given to you, word for word in value. If no fact supports a figure, do not write the figure; say the idea without it. Figures in the "stats" blocks must match a fact's value.
- The Arabic is a native Gulf copywriter's own writing for the same idea, not a translation of the English. Keep it short and formal enough for a developer.
- "why" says, in one line, which fact or finding the concept is built on and who it is for.
- "imagery" is an English prompt for an image model: the scene, light, mood and composition, leaving a calm area for text, with no words, logos or watermarks in the picture. Never ask for text in the image.
- Blocks: eyebrow (a few words), title (the big line), text (one or two lines), stats (up to three figure+label pairs, only from facts), event (only if an event is in the facts), note, info (small print such as a condition), cta (two or three words on the button).
- Codes: use the next free codes you are given.`
)

type GenerateConceptsInput struct {
	Count        int    `json:"count"`
	Instructions string `json:"instructions"`
}

type GeneratedConcepts struct {
	Concepts []Concept       `json:"concepts"`
	Notes    []string        `json:"notes"`
	Model    string          `json:"model"`
	Usage    assistant.Usage `json:"usage"`
}

// proposedConcept is the model's shape; it is checked before anything is
// saved.
type proposedConcept struct {
	Code    string          `json:"code"`
	Name    string          `json:"name"`
	Why     string          `json:"why"`
	En      json.RawMessage `json:"en"`
	Ar      json.RawMessage `json:"ar"`
	Imagery string          `json:"imagery"`
	Meta    struct {
		PrimaryText string `json:"primaryText"`
		Headline    string `json:"headline"`
		Description string `json:"description"`
		CTA         string `json:"cta"`
	} `json:"meta"`
}

type copyBlocks struct {
	Eyebrow   string `json:"eyebrow"`
	Title     string `json:"title"`
	TitleSize string `json:"titleSize"`
	Text      string `json:"text"`
	Stats     []struct {
		Figure string `json:"figure"`
		Label  string `json:"label"`
	} `json:"stats"`
	Event string `json:"event"`
	Note  string `json:"note"`
	Info  string `json:"info"`
	CTA   string `json:"cta"`
}

// GenerateConcepts asks the model for new concepts for a project, built on
// the brand's approved facts and current offers, and saves them as concepts
// with their copy. Figures the facts don't support are dropped with a note;
// a concept left with nothing to say is not saved.
func (c *Studio) GenerateConcepts(ctx context.Context, actor core.Actor, projectID int64, in GenerateConceptsInput) (GeneratedConcepts, error) {
	if !actor.AtLeast(core.RoleMarketer) {
		return GeneratedConcepts{}, core.ErrForbidden
	}
	if c.gen.Model == nil || c.gen.Brands == nil {
		return GeneratedConcepts{}, core.Conflict("no model is configured on this server; write the concepts by hand or ask an admin to configure ADX_MODEL_API_KEY")
	}
	count := in.Count
	if count <= 0 {
		count = 3
	}
	if count > maxConceptsPerAsk {
		return GeneratedConcepts{}, core.Invalid("ask for up to %d concepts at a time", maxConceptsPerAsk)
	}
	settings, err := c.settings(ctx, c.db.Reader(), projectID)
	if err != nil {
		return GeneratedConcepts{}, err
	}
	existing, err := c.db.Reader().ListConcepts(ctx, projectID)
	if err != nil {
		return GeneratedConcepts{}, err
	}
	gc, err := c.conceptContext(ctx, actor, projectID, settings, existing, count)
	if err != nil {
		return GeneratedConcepts{}, err
	}
	req := c.conceptRequest(gc, in.Instructions, count)
	resp, err := c.gen.Model.Complete(ctx, req)
	if err != nil {
		var me *assistant.ModelError
		if errors.As(err, &me) {
			return GeneratedConcepts{}, core.Unavailable(err, "%s", me.Safe)
		}
		return GeneratedConcepts{}, core.Unavailable(err, "the model did not answer")
	}
	var proposed []proposedConcept
	for _, b := range resp.Content {
		if b.Type == "tool_use" && b.ToolName == conceptsTool {
			var out struct {
				Concepts []proposedConcept `json:"concepts"`
			}
			if err := json.Unmarshal(b.Input, &out); err != nil {
				return GeneratedConcepts{}, core.Invalid("the model's proposal was not a list of concepts; nothing was saved")
			}
			proposed = out.Concepts
			break
		}
	}
	if proposed == nil {
		if resp.StopReason == "refusal" {
			return GeneratedConcepts{}, core.Invalid("the model declined to propose concepts; nothing was saved")
		}
		return GeneratedConcepts{}, core.Invalid("the model answered in prose instead of concepts; nothing was saved")
	}
	res := GeneratedConcepts{Concepts: []Concept{}, Notes: []string{}, Model: resp.Model, Usage: resp.Usage}
	codes := gc.freeCodes
	taken := map[string]bool{}
	for _, k := range existing {
		taken[k.Code] = true
	}
	for i, p := range proposed {
		if i >= count {
			break
		}
		code := strings.ToUpper(strings.TrimSpace(p.Code))
		if !conceptCode.MatchString(code) || taken[code] {
			if len(codes) == 0 {
				res.Notes = append(res.Notes, fmt.Sprintf("%q had no free code left and was not saved", p.Name))
				continue
			}
			code = codes[0]
		}
		codes = without(codes, code)
		taken[code] = true
		en, enNotes := c.constrainBlocks("en", p.Name, p.En, gc.values)
		ar, arNotes := c.constrainBlocks("ar", p.Name, p.Ar, gc.values)
		res.Notes = append(res.Notes, enNotes...)
		res.Notes = append(res.Notes, arNotes...)
		if en.Title == "" && ar.Title == "" {
			res.Notes = append(res.Notes, fmt.Sprintf("%q had no title in either language and was not saved", p.Name))
			continue
		}
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = code
		}
		if err := maxRunes("name", name, 80); err != nil {
			name = string([]rune(name)[:80])
		}
		doc, _ := json.Marshal(map[string]any{"v": 1, "render": "", "en": en, "ar": ar, "arReviewed": false})
		var k store.CreativeConcept
		now := c.now()
		err := c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
			pos, err := q.MaxConceptPos(ctx, projectID)
			if err != nil {
				return err
			}
			k, err = q.InsertConcept(ctx, store.InsertConceptParams{ProjectID: projectID, Code: code, Pos: pos + 1, Name: name, CreatedAt: now, UpdatedAt: now})
			if err != nil {
				return err
			}
			if err := q.UpdateConcept(ctx, store.UpdateConceptParams{
				Name: name, Why: clip(p.Why, 600), PrimaryText: clip(p.Meta.PrimaryText, 2000), Headline: clip(p.Meta.Headline, 200),
				Description: clip(p.Meta.Description, 400), Cta: clip(p.Meta.CTA, 60), Copy: string(doc), UpdatedAt: now, ID: k.ID,
			}); err != nil {
				return err
			}
			if err := q.SetConceptImagery(ctx, store.SetConceptImageryParams{Imagery: clip(p.Imagery, 1000), UpdatedAt: now, ID: k.ID}); err != nil {
				return err
			}
			k, err = q.GetConcept(ctx, k.ID)
			return err
		})
		if store.IsUniqueViolation(err) {
			res.Notes = append(res.Notes, fmt.Sprintf("%s already exists; %q was not saved", code, name))
			continue
		}
		if err != nil {
			return GeneratedConcepts{}, err
		}
		res.Concepts = append(res.Concepts, conceptFrom(k))
	}
	if err := c.db.Writer().TouchCreativeProject(ctx, store.TouchCreativeProjectParams{UpdatedAt: c.now(), ProjectID: projectID}); err != nil {
		return GeneratedConcepts{}, err
	}
	return res, nil
}

type conceptContext struct {
	project, brand string
	claims         []brands.Fact
	pending        []string
	offers         []brands.Offer
	existing       []store.CreativeConcept
	freeCodes      []string
	values         map[string]bool // every figure the copy may state
}

var digits = regexp.MustCompile(`[0-9][0-9,.]*`)

func (c *Studio) conceptContext(ctx context.Context, actor core.Actor, projectID int64, s Settings, existing []store.CreativeConcept, count int) (conceptContext, error) {
	gc := conceptContext{project: s.ProjectName, existing: existing, values: map[string]bool{}}
	brandID, offers, err := c.gen.Brands.OffersForProject(ctx, actor, projectID)
	if err != nil {
		return gc, err
	}
	gc.offers = offers
	if brandID != 0 {
		b, err := c.gen.Brands.Get(ctx, actor, brandID)
		if err != nil {
			return gc, err
		}
		gc.brand = b.Brand.Name
		set, err := c.gen.Brands.Claims(ctx, actor, brandID, projectID, c.now())
		if err != nil {
			return gc, err
		}
		gc.claims = set.Claims
		gc.pending = set.Pending
	}
	for _, f := range gc.claims {
		for _, d := range digits.FindAllString(f.Value, -1) {
			gc.values[normalizeFigure(d)] = true
		}
		if f.Kind == brands.KindMoney {
			addMoneyForms(gc.values, f.Value)
		}
	}
	for _, o := range gc.offers {
		for _, text := range []string{o.Current.Document.Headline, o.Current.Document.Terms, o.Current.Document.PaymentPlan} {
			for _, d := range digits.FindAllString(text, -1) {
				gc.values[normalizeFigure(d)] = true
			}
		}
		if o.Current.Document.DiscountPct != nil {
			gc.values[normalizeFigure(fmt.Sprintf("%g", *o.Current.Document.DiscountPct))] = true
		}
		if o.Current.Document.PriceFrom != nil {
			addMoneyForms(gc.values, fmt.Sprintf("%d", o.Current.Document.PriceFrom.Minor))
		}
	}
	taken := map[string]bool{}
	for _, k := range existing {
		taken[k.Code] = true
	}
	for n := 1; len(gc.freeCodes) < count; n++ {
		code := fmt.Sprintf("C%d", n)
		if !taken[code] {
			gc.freeCodes = append(gc.freeCodes, code)
		}
	}
	return gc, nil
}

func (c *Studio) conceptRequest(gc conceptContext, instructions string, count int) assistant.Request {
	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s", gc.project)
	if gc.brand != "" {
		fmt.Fprintf(&b, " by %s", gc.brand)
	}
	fmt.Fprintf(&b, "\nWrite %d concepts. Free codes: %s.\n\nApproved facts (the only figures you may state):\n", count, strings.Join(gc.freeCodes, ", "))
	if len(gc.claims) == 0 {
		b.WriteString("(none approved yet; write concepts without figures)\n")
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
	b.WriteString("\nCurrent offers:\n")
	if len(gc.offers) == 0 {
		b.WriteString("(none)\n")
	}
	for _, o := range gc.offers {
		d := o.Current.Document
		fmt.Fprintf(&b, "- %s (%s): %s", o.Name, o.Current.State, d.Headline)
		if d.HeadlineAr != "" {
			fmt.Fprintf(&b, " / %s", d.HeadlineAr)
		}
		if d.PaymentPlan != "" {
			fmt.Fprintf(&b, "; payment plan %s", d.PaymentPlan)
		}
		if d.DiscountPct != nil {
			fmt.Fprintf(&b, "; discount %g%%", *d.DiscountPct)
		}
		if len(d.Conditions) > 0 {
			fmt.Fprintf(&b, "; conditions: %s", strings.Join(d.Conditions, "; "))
		}
		b.WriteString("\n")
	}
	if len(gc.existing) > 0 {
		b.WriteString("\nConcepts the project already has, not to be repeated:\n")
		for _, k := range gc.existing {
			fmt.Fprintf(&b, "- %s %s\n", k.Code, k.Name)
		}
	}
	if strings.TrimSpace(instructions) != "" {
		fmt.Fprintf(&b, "\nFrom the marketer: %s\n", strings.TrimSpace(instructions))
	}
	return assistant.Request{
		System:     conceptsSystemBrief,
		Messages:   []assistant.Message{{Role: assistant.RoleUser, Content: []assistant.Block{{Type: "text", Text: b.String()}}}},
		Tools:      []assistant.ToolSpec{{Name: conceptsTool, Description: "Propose the concepts, with copy in both languages and an imagery prompt each.", InputSchema: conceptsSchema()}},
		MaxTokens:  conceptsMaxTokens,
		ToolChoice: conceptsTool,
	}
}

func conceptsSchema() map[string]any {
	str := map[string]any{"type": "string"}
	blocks := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"eyebrow": str, "title": str, "titleSize": map[string]any{"type": "string", "enum": []string{"xl", "lg", "md"}},
			"text": str, "event": str, "note": str, "info": str, "cta": str,
			"stats": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"figure": str, "label": str}, "required": []string{"figure", "label"}}},
		},
		"required": []string{"title"},
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"concepts": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"code": str, "name": str, "why": str, "en": blocks, "ar": blocks, "imagery": str,
						"meta": map[string]any{"type": "object", "description": "The ad platform's text fields.", "properties": map[string]any{"primaryText": str, "headline": str, "description": str, "cta": str}},
					},
					"required": []string{"code", "name", "why", "en", "ar", "imagery"},
				},
			},
		},
		"required": []string{"concepts"},
	}
}

// constrainBlocks keeps the model's blocks within the facts: a stat whose
// figure no fact or offer states is dropped and noted.
func (c *Studio) constrainBlocks(lang, name string, raw json.RawMessage, values map[string]bool) (copyBlocks, []string) {
	var b copyBlocks
	_ = json.Unmarshal(raw, &b)
	if b.TitleSize == "" {
		b.TitleSize = "md"
	}
	var notes []string
	kept := b.Stats[:0]
	for _, s := range b.Stats {
		ok := true
		for _, d := range digits.FindAllString(s.Figure, -1) {
			if !values[normalizeFigure(d)] {
				ok = false
			}
		}
		if ok {
			kept = append(kept, s)
		} else {
			notes = append(notes, fmt.Sprintf("%s (%s): dropped the stat %q, no approved fact states it", name, lang, s.Figure))
		}
	}
	b.Stats = kept
	if len(b.Stats) > 3 {
		b.Stats = b.Stats[:3]
	}
	for _, f := range []*string{&b.Eyebrow, &b.Title, &b.Text, &b.Event, &b.Note, &b.Info, &b.CTA} {
		*f = clip(strings.TrimSpace(*f), 300)
	}
	return b, notes
}

// addMoneyForms allows the ways copy writes an amount stored in minor
// units: 120000000 fils is AED 1,200,000, 1.2M or 1200K.
func addMoneyForms(values map[string]bool, minorText string) {
	var minor int64
	if _, err := fmt.Sscanf(minorText, "%d", &minor); err != nil {
		return
	}
	major := minor / 100
	for _, v := range []string{
		fmt.Sprintf("%d", major), fmt.Sprintf("%g", float64(major)/1e6), fmt.Sprintf("%g", float64(major)/1e3),
	} {
		values[normalizeFigure(v)] = true
	}
}

func normalizeFigure(s string) string {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimRight(s, ".")
	if i := strings.Index(s, "."); i >= 0 {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		return string(r[:n])
	}
	return string(r)
}

func without(xs []string, x string) []string {
	out := xs[:0:0]
	for _, v := range xs {
		if v != x {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
