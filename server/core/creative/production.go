package creative

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assets"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/store"
)

// Production (B3.1): renders as durable runs, outputs labelled by how they
// were made, copy checked against the brand's approved claims, and a
// readiness verdict that says whether a version may be put forward for
// launch. Nothing here reaches an ad account.

// RenderRunKind is the run kind the render worker handles.
const RenderRunKind = "creative.render"

type RenderParams struct {
	CreativeID int64 `json:"creativeId"`
	VersionID  int64 `json:"versionId"`
}

// Capabilities says what this server can produce. AI generation is an
// interface with no provider wired, and the UI says so rather than calling
// a template layout "generated".
type Capabilities struct {
	Renderer     string            `json:"renderer"`
	AIGeneration AIGenerationState `json:"aiGeneration"`
}

// AIGenerationState says whether an image generation provider is wired.
type AIGenerationState struct {
	Available bool   `json:"available"`
	Provider  string `json:"provider"`
	Reason    string `json:"reason"`
}

func (s *Service) Capabilities() Capabilities {
	c := Capabilities{Renderer: "none"}
	if s.renderer != nil {
		c.Renderer = s.renderer.Name()
	}
	if s.generator != nil {
		c.AIGeneration.Available = true
		c.AIGeneration.Provider = s.generator.Name()
		return c
	}
	c.AIGeneration.Reason = "no image generation provider is configured; every output is a template layout over an uploaded render and is labelled as such"
	return c
}

// StartRender queues a render of the creative's latest version. One run per
// version per document hash, so clicking twice doesn't render twice; a
// failed render is requeued by the same call.
func (s *Service) StartRender(ctx context.Context, actor core.Actor, rs *runs.Runs, creativeID int64) (runs.Run, bool, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return runs.Run{}, false, core.ErrForbidden
	}
	if s.store == nil || s.renderer == nil {
		return runs.Run{}, false, core.Conflict("rendering is not configured on this server")
	}
	_, latest, err := s.latest(ctx, actor, creativeID)
	if err != nil {
		return runs.Run{}, false, err
	}
	state := assets.ReviewState(latest.ReviewState)
	if state != assets.Draft && state != assets.RenderedSt {
		return runs.Run{}, false, core.Conflict("version %d is %s; edit it to make a new draft before rendering", latest.Version, state)
	}
	var doc assets.Document
	if err := json.Unmarshal([]byte(latest.Document), &doc); err != nil {
		return runs.Run{}, false, err
	}
	if checks := assets.CheckDocument(doc, false); assets.HasErrors(checks) {
		return runs.Run{}, false, core.Invalid("fix the house-rule errors before rendering: %s", firstError(checks))
	}
	key := fmt.Sprintf("creative:%d/v%d/%s", creativeID, latest.Version, shortHash(latest.Document))
	return rs.Start(ctx, actor, runs.StartInput{Kind: RenderRunKind, OccurrenceKey: key, Params: RenderParams{CreativeID: creativeID, VersionID: latest.ID}})
}

// Execute works one render run. It is the jobs.Executor for RenderRunKind.
// The file is written first (content-addressed, so a crash leaves at most
// an orphan file and a retry lands on the same key); the row is updated
// after, in one statement guarded by the version's state.
func (s *Service) Execute(ctx context.Context, h *runs.Handle) (runs.Completion, error) {
	var params RenderParams
	if err := json.Unmarshal(h.Run.Params, &params); err != nil || params.VersionID == 0 {
		return runs.Completion{}, core.Invalid("render run has invalid parameters")
	}
	row, err := s.db.Reader().GetCreativeVersion(ctx, params.VersionID)
	if errors.Is(err, sql.ErrNoRows) {
		return runs.Completion{}, core.NotFound("creative version %d not found", params.VersionID)
	}
	if err != nil {
		return runs.Completion{}, err
	}
	crow, err := s.db.Reader().GetCreative(ctx, store.GetCreativeParams{ID: row.CreativeID, WorkspaceID: h.Actor.WorkspaceID})
	if err != nil {
		return runs.Completion{}, core.NotFound("creative %d not found in this workspace", row.CreativeID)
	}
	if _, err := s.renderVersion(ctx, h.Actor, crow, row, h.Run.RowID()); err != nil {
		_ = s.db.Writer().SetCreativeVersionRenderError(ctx, store.SetCreativeVersionRenderErrorParams{RenderError: safe(err), RenderRunID: sql.NullInt64{Int64: h.Run.RowID(), Valid: true}, ID: row.ID})
		return runs.Completion{}, err
	}
	return runs.Completion{State: runs.StateSucceeded, Completeness: runs.CompletenessComplete}, nil
}

// Render renders synchronously, for tests and small servers without a
// worker. The job path calls the same renderVersion.
func (s *Service) Render(ctx context.Context, actor core.Actor, creativeID int64) (Version, error) {
	if !actor.AtLeast(core.RoleMarketer) || actor.WorkspaceID == 0 {
		return Version{}, core.ErrForbidden
	}
	if s.store == nil || s.renderer == nil {
		return Version{}, core.Conflict("rendering is not configured on this server")
	}
	crow, latest, err := s.latest(ctx, actor, creativeID)
	if err != nil {
		return Version{}, err
	}
	return s.renderVersion(ctx, actor, crow, latest, 0)
}

func (s *Service) renderVersion(ctx context.Context, actor core.Actor, crow store.Creative, latest store.CreativeVersion, runID int64) (Version, error) {
	state := assets.ReviewState(latest.ReviewState)
	if state != assets.Draft && state != assets.RenderedSt {
		return Version{}, core.Conflict("version %d is %s; edit it to make a new draft before rendering", latest.Version, state)
	}
	var doc assets.Document
	if err := json.Unmarshal([]byte(latest.Document), &doc); err != nil {
		return Version{}, err
	}
	checks := assets.CheckDocument(doc, false)
	if assets.HasErrors(checks) {
		return s.versionFromRow(ctx, latest), core.Invalid("fix the house-rule errors before rendering: %s", firstError(checks))
	}
	bindings := s.bindClaims(ctx, actor, crow, doc)
	rendered, err := s.renderer.Render(ctx, s.store, doc)
	if err != nil {
		return Version{}, core.Conflict("rendering failed: %s", safe(err))
	}
	if ok, err := s.store.Exists(ctx, rendered.Ref); err != nil || !ok {
		return Version{}, core.Conflict("rendering produced no file; nothing was recorded")
	}
	braw, _ := json.Marshal(bindings)
	now := s.clock.Now().UTC()
	n, err := s.db.Writer().SetCreativeVersionRendered(ctx, store.SetCreativeVersionRenderedParams{Renderer: nullString(rendered.Renderer), OutputKey: nullString(rendered.Ref.Key),
		OutputSha256: nullString(rendered.Ref.SHA256), OutputSize: sql.NullInt64{Int64: rendered.Ref.Size, Valid: true}, OutputType: nullString(rendered.Ref.ContentType),
		RenderedAt: sql.NullTime{Time: now, Valid: true}, Bindings: string(braw), RenderRunID: optionalID(runID), ID: latest.ID})
	if err != nil {
		return Version{}, err
	}
	if n != 1 {
		return Version{}, core.Conflict("the version changed while rendering; reload and try again")
	}
	row, err := s.db.Reader().GetCreativeVersion(ctx, latest.ID)
	if err != nil {
		return Version{}, err
	}
	return s.versionFromRow(ctx, row), nil
}

// Claims in copy.

// ClaimCheck is one figure found in the copy and how it stands against the
// brand's approved facts and the creative's offer.
type ClaimCheck struct {
	Text   string `json:"text"`
	Kind   string `json:"kind"`   // percent, price
	Status string `json:"status"` // approved, unapproved, no_brand
	FactID *int64 `json:"factId"`
	Detail string `json:"detail"`
}

// ClaimBindings is what a render was checked against.
type ClaimBindings struct {
	BrandID      int64        `json:"brandId"`
	OfferID      *int64       `json:"offerId"`
	OfferVersion *int64       `json:"offerVersion"`
	OfferState   string       `json:"offerState"`
	FactIDs      []int64      `json:"factIds"`
	Claims       []ClaimCheck `json:"claims"`
}

var (
	rePercent = regexp.MustCompile(`(\d{1,3}(?:\.\d+)?)\s*[%٪]`)
	rePrice   = regexp.MustCompile(`(?i)(?:AED|د\.إ|درهم)\s*([\d,]+(?:\.\d+)?)\s*(M|million|K|k|مليون)?|([\d,]+(?:\.\d+)?)\s*(M|million|مليون)\b`)
	arDigits  = strings.NewReplacer("٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4", "٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9", "٫", ".", "٬", ",")
)

// bindClaims finds the figures in the copy and checks each against the
// brand's approved claims and the offer the creative was built on. It is
// deterministic text matching: a figure it can't read is not a claim it
// can judge, and that is said in the readiness check.
func (s *Service) bindClaims(ctx context.Context, actor core.Actor, crow store.Creative, doc assets.Document) ClaimBindings {
	b := ClaimBindings{FactIDs: []int64{}, Claims: []ClaimCheck{}}
	if s.brands == nil || !crow.ProjectID.Valid {
		return b
	}
	brandID, err := s.brands.BrandOfProject(ctx, actor, crow.ProjectID.Int64)
	if err != nil || brandID == 0 {
		return b
	}
	b.BrandID = brandID
	claims, err := s.brands.Claims(ctx, actor, brandID, crow.ProjectID.Int64, s.clock.Now())
	if err != nil {
		return b
	}
	approvedPct := map[string]int64{}
	approvedPrice := map[int64]int64{}
	for _, c := range claims.Claims {
		b.FactIDs = append(b.FactIDs, c.ID)
		switch c.Kind {
		case brands.KindPercent:
			approvedPct[normNumber(c.Value)] = c.ID
		case brands.KindMoney:
			if n, err := strconv.ParseInt(c.Value, 10, 64); err == nil {
				approvedPrice[n] = c.ID
			}
		}
	}
	var offerPct *float64
	if crow.OfferID.Valid {
		oid := crow.OfferID.Int64
		b.OfferID = &oid
		if crow.OfferVersion.Valid {
			ov := crow.OfferVersion.Int64
			b.OfferVersion = &ov
		}
		if _, versions, err := s.brands.OfferVersions(ctx, actor, oid); err == nil {
			for _, v := range versions {
				if crow.OfferVersion.Valid && v.Version == crow.OfferVersion.Int64 {
					b.OfferState = string(v.State)
					if v.State == brands.OfferApproved {
						offerPct = v.Document.DiscountPct
						if v.Document.PriceFrom != nil {
							approvedPrice[v.Document.PriceFrom.Minor] = 0
						}
					}
				}
			}
		}
	}
	text := ""
	for _, l := range doc.Layers {
		if l.Kind == "text" {
			text += " " + arDigits.Replace(l.Text)
		}
	}
	for _, m := range rePercent.FindAllStringSubmatch(text, -1) {
		cc := ClaimCheck{Text: strings.TrimSpace(m[0]), Kind: "percent", Status: "unapproved", Detail: "no approved fact or approved offer states this percentage"}
		if id, ok := approvedPct[normNumber(m[1])]; ok {
			cc.Status, cc.FactID, cc.Detail = "approved", &id, "matches an approved fact"
		} else if offerPct != nil && normNumber(m[1]) == normNumber(strconv.FormatFloat(*offerPct, 'f', -1, 64)) {
			cc.Status, cc.Detail = "approved", "matches the approved offer"
		}
		b.Claims = append(b.Claims, cc)
	}
	for _, m := range rePrice.FindAllStringSubmatch(text, -1) {
		num, unit := m[1], m[2]
		if num == "" {
			num, unit = m[3], m[4]
		}
		minor, ok := priceMinor(num, unit)
		cc := ClaimCheck{Text: strings.TrimSpace(m[0]), Kind: "price", Status: "unapproved", Detail: "no approved price fact or approved offer states this amount"}
		if ok {
			if id, found := approvedPrice[minor]; found {
				cc.Status, cc.Detail = "approved", "matches an approved price"
				if id != 0 {
					cc.FactID = &id
				}
			}
		} else {
			cc.Detail = "the amount could not be read"
		}
		b.Claims = append(b.Claims, cc)
	}
	return b
}

func normNumber(s string) string {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
	if err != nil {
		return strings.TrimSpace(s)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func priceMinor(num, unit string) (int64, bool) {
	f, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", ""), 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(unit) {
	case "m", "million", "مليون":
		f *= 1_000_000
	case "k":
		f *= 1_000
	}
	return int64(f*100 + 0.5), true
}

// Readiness.

type ReadinessIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// LaunchReadiness is whether a creative's latest version may be put forward
// for launch: approved content, an offer still current and true, every
// figure in the copy an approved claim. Ready says nothing about delivery;
// phase 3's measurement and proposals add their own gates.
type LaunchReadiness struct {
	CreativeID int64            `json:"creativeId"`
	Version    int64            `json:"version"`
	Ready      bool             `json:"ready"`
	Blockers   []ReadinessIssue `json:"blockers"`
	Warnings   []ReadinessIssue `json:"warnings"`
	Claims     []ClaimCheck     `json:"claims"`
	Bindings   ClaimBindings    `json:"bindings"`
}

func (s *Service) Readiness(ctx context.Context, actor core.Actor, creativeID int64) (LaunchReadiness, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return LaunchReadiness{}, core.ErrForbidden
	}
	crow, latest, err := s.latest(ctx, actor, creativeID)
	if err != nil {
		return LaunchReadiness{}, err
	}
	r := LaunchReadiness{CreativeID: creativeID, Version: latest.Version, Blockers: []ReadinessIssue{}, Warnings: []ReadinessIssue{}, Claims: []ClaimCheck{}}
	block := func(code, msg string) { r.Blockers = append(r.Blockers, ReadinessIssue{code, msg}) }
	warn := func(code, msg string) { r.Warnings = append(r.Warnings, ReadinessIssue{code, msg}) }
	v := s.versionFromRow(ctx, latest)
	if v.ReviewState != assets.Approved || v.Acceptance == nil {
		block("content_not_approved", fmt.Sprintf("version %d is %s; only an approved version with its output accepted can launch", latest.Version, latest.ReviewState))
	}
	if v.Output == nil {
		block("no_output", "the version has no rendered output")
	}
	// Claims are checked now, against today's approved facts, not only as
	// they stood at render time.
	var doc assets.Document
	_ = json.Unmarshal([]byte(latest.Document), &doc)
	bindings := s.bindClaims(ctx, actor, crow, doc)
	r.Bindings = bindings
	r.Claims = bindings.Claims
	if crow.ProjectID.Valid && bindings.BrandID == 0 {
		warn("no_brand", "the project is under no brand, so no claim can be checked")
	}
	for _, c := range bindings.Claims {
		if c.Status != "approved" {
			block("unapproved_claim", fmt.Sprintf("%q is not an approved claim: %s", c.Text, c.Detail))
		}
	}
	if crow.OfferID.Valid {
		if _, versions, err := s.brands.OfferVersions(ctx, actor, crow.OfferID.Int64); err == nil && len(versions) > 0 {
			current := versions[len(versions)-1]
			if crow.OfferVersion.Valid && current.Version != crow.OfferVersion.Int64 {
				block("offer_changed", fmt.Sprintf("the offer is now version %d; this creative was built on version %d", current.Version, crow.OfferVersion.Int64))
			}
			if bindings.OfferState != string(brands.OfferApproved) {
				block("offer_not_approved", "the offer this creative is built on has not been approved as true")
			}
		}
	}
	if assets.Language(crow.Language) == assets.Arabic {
		warn("arabic_review", "Arabic copy: native review is not recorded in ADX; confirm it before launch")
	}
	if v.Renderer != nil && *v.Renderer != "template" {
		warn("renderer", "output produced by "+*v.Renderer)
	}
	r.Ready = len(r.Blockers) == 0
	return r, nil
}

// Production queue.

type QueueItem struct {
	Creative    Creative           `json:"creative"`
	Version     int64              `json:"version"`
	ReviewState assets.ReviewState `json:"reviewState"`
	RenderError string             `json:"renderError"`
}

// Queue lists every creative with its latest version's state, for the
// production board.
func (s *Service) Queue(ctx context.Context, actor core.Actor) ([]QueueItem, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := s.db.Reader().ListCreativesByState(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]QueueItem, 0, len(rows))
	for _, r := range rows {
		c := creativeFromRow(store.Creative{ID: r.ID, WorkspaceID: r.WorkspaceID, ProjectID: r.ProjectID, Name: r.Name, Format: r.Format, Language: r.Language, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, OfferID: r.OfferID, OfferVersion: r.OfferVersion, PlanID: r.PlanID, PlanVersion: r.PlanVersion})
		out = append(out, QueueItem{Creative: c, Version: r.CurrentVersion, ReviewState: assets.ReviewState(r.ReviewState), RenderError: r.RenderError})
	}
	return out, nil
}

func shortHash(s string) string {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return strconv.FormatUint(uint64(h), 16)
}
