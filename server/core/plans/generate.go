package plans

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/learning"
	"github.com/WokeWorks/adx/server/store"
)

// Generation: the model proposes a document, code decides what survives.
//
// The model is given the plan's fixed frame (budget, currency, period,
// brand, project), the research findings of the briefs named, the brand's
// approved claims and current offers, and the current version. It must
// answer by calling one tool whose input is a plan document. Code then
// pins the frame back (the model can't move the budget), drops evidence
// refs that weren't in the context, turns every expected cost per lead the
// model typed without a cited source into "unavailable", rejects channels
// ADX can't deploy, and runs the same validator as a hand-written version.
// The result is saved as a version with source "generated" and the same
// states as any other; it is never accepted by being generated.

const (
	proposeTool          = "propose_plan"
	generationSource     = "adx"
	generationKind       = "plan_generation"
	defaultMaxGenPerDay  = 40
	defaultGenMaxTokens  = 8000
	maxFindingsInContext = 60
)

type GenerateInput struct {
	BriefIDs     []int64 `json:"briefIds"`
	Instructions string  `json:"instructions"`
}

// GenerationNote is what the generator did to the model's output before
// validation, so the person sees the corrections, not just the result.
type GenerationNote struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Generation struct {
	Version PlanVersion      `json:"version"`
	Notes   []GenerationNote `json:"notes"`
	Model   string           `json:"model"`
	Usage   assistant.Usage  `json:"usage"`
}

type generationContext struct {
	brandID   int64
	projectID int64
	findings  []contextFinding
	claims    []contextClaim
	offers    []contextOffer
	memory    []learning.MemoryFinding
	refIDs    map[string]map[int64]bool
}

type contextFinding struct {
	ID        int64           `json:"id"`
	Scope     string          `json:"scope"`
	Kind      string          `json:"kind"`
	Class     string          `json:"class"`
	Label     string          `json:"label"`
	Payload   json.RawMessage `json:"payload"`
	Geography string          `json:"geography"`
}

type contextClaim struct {
	FactID   int64   `json:"factId"`
	Key      string  `json:"key"`
	Value    string  `json:"value"`
	Currency *string `json:"currency"`
}

type contextOffer struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Headline string `json:"headline"`
	State    string `json:"state"`
}

// Generate asks the model for a document for the plan's next version.
// Marketers may. Without a configured model it is a conflict, not a stub.
func (p *Plans) Generate(ctx context.Context, actor core.Actor, planID int64, in GenerateInput) (Generation, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Generation{}, core.ErrForbidden
	}
	if p.model == nil {
		return Generation{}, core.Conflict("no model is configured on this server; write the plan by hand or ask an admin to configure ADX_MODEL_API_KEY")
	}
	prow, err := p.plan(ctx, actor.WorkspaceID, planID)
	if err != nil {
		return Generation{}, err
	}
	latest, err := p.db.Reader().GetLatestPlanVersion(ctx, planID)
	if err != nil {
		return Generation{}, err
	}
	current, err := versionFromRow(latest)
	if err != nil {
		return Generation{}, err
	}
	if current.Document.BudgetMinor <= 0 || len(current.Document.Currency) != 3 || current.Document.PeriodFrom == "" || current.Document.PeriodTo == "" {
		return Generation{}, core.Invalid("set the budget, currency and period on the plan first; the model distributes a budget, it doesn't choose one")
	}
	if p.evidence != nil {
		since := p.clock.Now().UTC().Add(-24 * time.Hour)
		n, err := p.db.Reader().CountEvidenceSince(ctx, store.CountEvidenceSinceParams{WorkspaceID: actor.WorkspaceID, Source: generationSource, Kind: generationKind, ObservedAt: since})
		if err != nil {
			return Generation{}, err
		}
		if int(n) >= p.maxGenPerDay {
			return Generation{}, core.Conflict("the workspace's daily plan-generation budget (%d) is used up; try again tomorrow", p.maxGenPerDay)
		}
	}
	gctx, err := p.buildContext(ctx, actor, prow, in.BriefIDs)
	if err != nil {
		return Generation{}, err
	}
	req := p.buildRequest(prow, current, gctx, in.Instructions)
	resp, err := p.model.Complete(ctx, req)
	if err != nil {
		var me *assistant.ModelError
		if errors.As(err, &me) {
			return Generation{}, core.Unavailable(err, "%s", me.Safe)
		}
		return Generation{}, core.Unavailable(err, "the model did not answer")
	}
	p.recordGeneration(ctx, actor, planID, req, resp)
	var proposed *PlanDocument
	for _, b := range resp.Content {
		if b.Type == "tool_use" && b.ToolName == proposeTool {
			var d PlanDocument
			if err := json.Unmarshal(b.Input, &d); err != nil {
				return Generation{}, core.Invalid("the model's proposal was not a plan document; nothing was saved")
			}
			proposed = &d
			break
		}
	}
	if proposed == nil {
		if resp.StopReason == "refusal" {
			return Generation{}, core.Invalid("the model declined to propose a plan; nothing was saved")
		}
		return Generation{}, core.Invalid("the model answered in prose instead of a plan document; nothing was saved")
	}
	doc, notes, reject := p.constrain(*proposed, current.Document, gctx)
	if reject != "" {
		return Generation{}, core.Invalid("%s", reject)
	}
	v, err := p.saveVersion(ctx, actor, planID, VersionInput{Document: doc, Note: "generated from research" + briefNote(in.BriefIDs)}, "generated")
	if err != nil {
		return Generation{}, err
	}
	return Generation{Version: v, Notes: notes, Model: resp.Model, Usage: resp.Usage}, nil
}

func briefNote(ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return " (briefs " + strings.Join(parts, ", ") + ")"
}

// buildContext gathers only this brand's material: the named briefs must
// belong to the brand, claims are the brand's approved facts, offers the
// brand's current versions.
func (p *Plans) buildContext(ctx context.Context, actor core.Actor, prow store.Plan, briefIDs []int64) (generationContext, error) {
	g := generationContext{brandID: prow.BrandID, projectID: prow.ProjectID.Int64, memory: []learning.MemoryFinding{}, refIDs: map[string]map[int64]bool{"research_finding": {}, "fact": {}, "offer": {}}}
	if p.memory != nil {
		if f, err := p.memory.Findings(ctx, actor); err == nil && f != nil {
			g.memory = f
		}
	}
	for _, id := range briefIDs {
		brief, err := p.db.Reader().GetResearchBrief(ctx, store.GetResearchBriefParams{ID: id, WorkspaceID: actor.WorkspaceID})
		if errors.Is(err, sql.ErrNoRows) {
			return g, core.NotFound("research brief %d not found", id)
		}
		if err != nil {
			return g, err
		}
		if brief.BrandID != prow.BrandID {
			return g, core.Invalid("research brief %d is about another brand", id)
		}
		rows, err := p.db.Reader().ListResearchFindings(ctx, store.ListResearchFindingsParams{BriefID: id})
		if err != nil {
			return g, err
		}
		for _, f := range rows {
			if len(g.findings) >= maxFindingsInContext {
				break
			}
			g.findings = append(g.findings, contextFinding{ID: f.ID, Scope: f.Scope, Kind: f.Kind, Class: f.Class, Label: f.Label, Payload: json.RawMessage(f.Payload), Geography: f.Geography})
			g.refIDs["research_finding"][f.ID] = true
		}
	}
	claims, err := p.brands.Claims(ctx, actor, prow.BrandID, prow.ProjectID.Int64, p.clock.Now())
	if err != nil {
		return g, err
	}
	for _, c := range claims.Claims {
		g.claims = append(g.claims, contextClaim{FactID: c.ID, Key: c.Key, Value: c.Value, Currency: c.Currency})
		g.refIDs["fact"][c.ID] = true
	}
	offers, err := p.brands.ListOffers(ctx, actor, prow.BrandID)
	if err != nil {
		return g, err
	}
	for _, o := range offers {
		if o.ProjectID != nil && prow.ProjectID.Valid && *o.ProjectID != prow.ProjectID.Int64 {
			continue
		}
		if o.Current.State == "withdrawn" {
			continue
		}
		g.offers = append(g.offers, contextOffer{ID: o.ID, Name: o.Name, Headline: o.Current.Document.Headline, State: string(o.Current.State)})
		g.refIDs["offer"][o.ID] = true
	}
	return g, nil
}

func supportedChannelKeys() []string {
	var keys []string
	for _, c := range Channels {
		if c.Supported {
			keys = append(keys, c.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

// proposeSchema is the tool input: the document fields the model may fill.
// Budget, currency and period are not in it; code pins them.
func proposeSchema() map[string]any {
	str := map[string]any{"type": "string"}
	intT := map[string]any{"type": "integer"}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"objective": str,
			"allocations": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
				"channel": map[string]any{"type": "string", "enum": supportedChannelKeys()}, "objective": str, "budgetMinor": intT, "geography": str, "audience": str, "note": str,
			}, "required": []string{"channel", "objective", "budgetMinor"}, "additionalProperties": false}},
			"assetRequirements": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
				"format": map[string]any{"type": "string", "enum": []string{"9:16", "4:5", "1:1", "1.91:1"}}, "language": map[string]any{"type": "string", "enum": []string{"en", "ar"}}, "count": intT, "offerId": intT, "note": str,
			}, "required": []string{"format", "language", "count"}, "additionalProperties": false}},
			"pageRequirements": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
				"kind": map[string]any{"type": "string", "enum": []string{"landing", "thank_you"}}, "language": map[string]any{"type": "string", "enum": []string{"en", "ar"}}, "url": str, "formPath": str, "note": str,
			}, "required": []string{"kind", "language"}, "additionalProperties": false}},
			"measurementRequirements": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
				"kind": map[string]any{"type": "string", "enum": []string{"conversion_action", "pixel_event", "utm_scheme", "crm_stage", "call_tracking"}}, "platform": str, "name": str, "required": map[string]any{"type": "boolean"}, "note": str,
			}, "required": []string{"kind", "name"}, "additionalProperties": false}},
			"testHypotheses": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
				"hypothesis": str, "metric": str, "windowDays": intT,
			}, "required": []string{"hypothesis", "metric", "windowDays"}, "additionalProperties": false}},
			"evidenceRefs": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
				"kind": map[string]any{"type": "string", "enum": []string{"research_finding", "fact", "offer"}}, "id": intT, "note": str,
			}, "required": []string{"kind", "id"}, "additionalProperties": false}},
			"assumptions": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{
				"key": str, "text": str, "class": map[string]any{"type": "string", "enum": []string{"assumption", "unavailable", "cited"}}, "evidenceRef": intT,
			}, "required": []string{"key", "text", "class"}, "additionalProperties": false}},
			"notes": str,
		},
		"required":             []string{"objective", "allocations", "assetRequirements", "measurementRequirements", "evidenceRefs", "assumptions"},
		"additionalProperties": false,
	}
}

func (p *Plans) buildRequest(prow store.Plan, current PlanVersion, g generationContext, instructions string) assistant.Request {
	d := current.Document
	system := strings.Join([]string{
		"You draft media plans for a UAE real-estate developer's marketing team. You propose; code validates and people decide.",
		"Rules: distribute exactly the given budget across the allowed channels (the allocations must sum to budgetMinor); use only the channels listed; cite research findings, facts and offers by the ids given, and only those; never type a cost per lead, click or conversion figure unless a finding in the context states it, and then cite it as a 'cited' assumption pointing at that evidence ref; for figures nobody has, add an 'unavailable' assumption instead of a number; do not invent prices, handover dates or offers; write short, concrete requirements.",
		"Answer only by calling the propose_plan tool.",
	}, " ")
	frame := map[string]any{
		"brandId": prow.BrandID, "projectId": prow.ProjectID.Int64, "title": prow.Title,
		"budgetMinor": d.BudgetMinor, "currency": d.Currency, "periodFrom": d.PeriodFrom, "periodTo": d.PeriodTo,
		"currentObjective": d.Objective, "currentVersion": current.Version, "currentAllocations": d.Allocations,
		"allowedChannels": supportedChannelKeys(),
	}
	frameJSON, _ := json.Marshal(frame)
	ctxJSON, _ := json.Marshal(map[string]any{"researchFindings": g.findings, "approvedClaims": g.claims, "currentOffers": g.offers, "validatedFindings": g.memory})
	text := "Plan frame (fixed by the team; do not change budget, currency or period):\n" + string(frameJSON) + "\n\nContext you may cite (ids are the only valid evidence refs):\n" + string(ctxJSON)
	if strings.TrimSpace(instructions) != "" {
		text += "\n\nThe team's instructions:\n" + strings.TrimSpace(instructions)
	}
	return assistant.Request{
		System:     system,
		Messages:   []assistant.Message{{Role: assistant.RoleUser, Content: []assistant.Block{{Type: "text", Text: text}}}},
		Tools:      []assistant.ToolSpec{{Name: proposeTool, Description: "Propose the plan document for the next version.", InputSchema: proposeSchema()}},
		MaxTokens:  p.genMaxTokens,
		ToolChoice: proposeTool,
	}
}

// constrain applies the code-side rules to the model's output. It returns
// the document to validate, what was changed, and a rejection reason when
// the proposal can't be used at all.
func (p *Plans) constrain(proposed, frame PlanDocument, g generationContext) (PlanDocument, []GenerationNote, string) {
	notes := []GenerationNote{}
	doc := normalize(proposed)
	// The frame is the team's.
	if doc.BudgetMinor != 0 && doc.BudgetMinor != frame.BudgetMinor {
		notes = append(notes, GenerationNote{"budget_pinned", fmt.Sprintf("The model proposed a budget of %d; the plan's budget %d was kept.", doc.BudgetMinor, frame.BudgetMinor)})
	}
	doc.BudgetMinor, doc.Currency, doc.PeriodFrom, doc.PeriodTo = frame.BudgetMinor, frame.Currency, frame.PeriodFrom, frame.PeriodTo
	doc.ObservationWindows = frame.ObservationWindows
	// Channels: only what ADX can deploy.
	for _, a := range doc.Allocations {
		ch, ok := channelByKey[a.Channel]
		if !ok {
			return doc, notes, fmt.Sprintf("the model proposed channel %q, which ADX does not know; the proposal was rejected and nothing was saved", a.Channel)
		}
		if !ch.Supported {
			return doc, notes, fmt.Sprintf("the model proposed %s, which ADX cannot deploy or measure; the proposal was rejected and nothing was saved", ch.Label)
		}
	}
	// Evidence refs: only ids that were in the context.
	kept := []PlanEvidenceRef{}
	dropped := 0
	indexMap := map[int]int{}
	for i, r := range doc.EvidenceRefs {
		if ids, ok := g.refIDs[r.Kind]; ok && ids[r.ID] {
			indexMap[i] = len(kept)
			kept = append(kept, r)
		} else {
			dropped++
		}
	}
	if dropped > 0 {
		notes = append(notes, GenerationNote{"evidence_dropped", fmt.Sprintf("%d evidence reference(s) the model cited were not in the context and were dropped.", dropped)})
	}
	doc.EvidenceRefs = kept
	// Assumptions: re-point citations; a citation to a dropped ref becomes unavailable.
	for i := range doc.Assumptions {
		a := &doc.Assumptions[i]
		if a.Class == "cited" {
			if a.EvidenceRef == nil {
				a.Class = "unavailable"
				notes = append(notes, GenerationNote{"citation_removed", a.Key + ": cited without a reference; recorded as unavailable."})
				continue
			}
			if j, ok := indexMap[*a.EvidenceRef]; ok {
				a.EvidenceRef = &j
			} else {
				a.Class, a.EvidenceRef = "unavailable", nil
				notes = append(notes, GenerationNote{"citation_removed", a.Key + ": its reference was not in the context; recorded as unavailable."})
			}
		}
	}
	// Expected CPL: never a number the model made up. Only the figures that
	// a cited assumption keyed cpl.<channel> backs survive.
	cited := map[string]bool{}
	for _, a := range doc.Assumptions {
		if a.Class == "cited" && strings.HasPrefix(a.Key, "cpl.") {
			cited[strings.TrimPrefix(a.Key, "cpl.")] = true
		}
	}
	for i := range doc.Allocations {
		a := &doc.Allocations[i]
		if a.ExpectedCPLMinor != nil && !cited[a.Channel] {
			a.ExpectedCPLMinor = nil
			notes = append(notes, GenerationNote{"cpl_unavailable", fmt.Sprintf("%s: the model typed an expected cost per lead with no cited source; it is recorded as unavailable.", a.Channel)})
			doc.Assumptions = append(doc.Assumptions, Assumption{Key: "cpl." + a.Channel, Text: "No sourced cost per lead for " + a.Channel + " in the context.", Class: "unavailable"})
		}
	}
	// Project scope: allocations inherit the plan's project.
	for i := range doc.Allocations {
		if g.projectID != 0 {
			pid := g.projectID
			doc.Allocations[i].ProjectID = &pid
		} else {
			doc.Allocations[i].ProjectID = nil
		}
	}
	return doc, notes, ""
}

// recordGeneration keeps the fact that a model was asked, with usage and a
// hash of the request, as evidence. Not the prompt itself: it contains the
// brand's findings, which are already stored.
func (p *Plans) recordGeneration(ctx context.Context, actor core.Actor, planID int64, req assistant.Request, resp assistant.Response) {
	if p.evidence == nil {
		return
	}
	raw, _ := json.Marshal(req)
	sum := sha256.Sum256(raw)
	// Each call is its own observation, so the reference carries a nonce
	// rather than the time: two identical requests in one second are two
	// calls, and the daily cap counts both.
	nonce := make([]byte, 8)
	_, _ = rand.Read(nonce)
	_, _, _ = p.evidence.Record(ctx, actor, evidence.Observation{
		Source: generationSource, Kind: generationKind, SourceRef: fmt.Sprintf("plan:%d/%s", planID, hex.EncodeToString(nonce)), Scope: fmt.Sprintf("plan:%d", planID),
		Class: evidence.Observed, Payload: map[string]any{"model": resp.Model, "inputTokens": resp.Usage.InputTokens, "outputTokens": resp.Usage.OutputTokens, "stopReason": resp.StopReason, "requestSha256": hex.EncodeToString(sum[:]), "requestedBy": actor.UserID},
		Completeness: evidence.Complete,
	})
}
