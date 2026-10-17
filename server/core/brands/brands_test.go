package brands_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/assets"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/creative"
	"github.com/WokeWorks/adx/server/providers/filestore"
	"github.com/WokeWorks/adx/server/providers/svgrender"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db        *store.DB
	clock     *coretest.Clock
	brands    *brands.Brands
	creative  *creative.Service
	reportage int64
	rival     int64
	wadi      int64
	rivalProj int64
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))}
	coretest.SeedUsers(t, e.db)
	e.brands = brands.New(e.db, brands.Config{Clock: e.clock})
	st, err := filestore.New(t.TempDir())
	require.NoError(t, err)
	e.creative = creative.New(e.db, st, svgrender.New(), creative.Config{Clock: e.clock, Brands: e.brands})
	ctx := t.Context()
	r, err := e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	e.reportage = r.ID
	c, err := e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Rival Developments", Kind: brands.Competitor})
	require.NoError(t, err)
	e.rival = c.ID
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.wadi = p.ID
	require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, e.wadi, e.reportage))
	rp, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{Name: "Rival Towers", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.rivalProj = rp.ID
	require.NoError(t, e.brands.AssignProject(ctx, coretest.Admin, e.rivalProj, e.rival))
	return e
}

func price(project int64, minor, from, ref string) brands.FactInput {
	return brands.FactInput{ProjectID: project, Key: "price.from", Kind: brands.KindMoney, Value: minor, Currency: "AED", EffectiveFrom: from, SourceKind: brands.SourceDocument, SourceRef: ref}
}

// Catches: a second price source silently winning, a conflicting fact
// becoming a claim, a resolution that also approves, and history being
// edited instead of retired.
func TestContradictoryPricesNeedReviewBeforeTheyBecomeClaims(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	a, err := e.brands.AddFact(ctx, coretest.Marketer, e.reportage, price(e.wadi, "120000000", "2026-10-01", "price list v3"))
	require.NoError(t, err)
	assert.Equal(t, brands.NeedsReview, a.Fact.Class)
	assert.Empty(t, a.Conflicts)

	// Same value from another source: agreement, not a conflict.
	same, err := e.brands.AddFact(ctx, coretest.Marketer, e.reportage, price(e.wadi, "120,000,000", "2026-10-05", "brochure"))
	require.NoError(t, err)
	assert.Empty(t, same.Conflicts)

	// A different price from the CRM for the same period: conflict.
	b, err := e.brands.AddFact(ctx, coretest.Marketer, e.reportage, price(e.wadi, "115000000", "2026-10-10", "crm export"))
	require.NoError(t, err)
	require.Len(t, b.Conflicts, 2, "the new price disagrees with both earlier rows")
	assert.True(t, b.Fact.InConflict)

	_, err = e.brands.ApproveFact(ctx, coretest.Admin, a.Fact.ID)
	require.Error(t, err, "approving a conflicting fact")
	assert.Equal(t, core.KindConflict, kindOf(err))
	_, err = e.brands.ApproveFact(ctx, coretest.Admin, b.Fact.ID)
	require.Error(t, err)

	claims, err := e.brands.Claims(ctx, coretest.Viewer, e.reportage, e.wadi, e.clock.Now())
	require.NoError(t, err)
	assert.Empty(t, claims.Claims)
	assert.Equal(t, []string{"price.from"}, claims.Pending)

	// Marketers can't resolve; an admin picks the price list.
	conflicts, err := e.brands.ListConflicts(ctx, coretest.Viewer, e.reportage)
	require.NoError(t, err)
	require.Len(t, conflicts, 2)
	_, err = e.brands.ResolveConflict(ctx, coretest.Marketer, conflicts[0].ID, a.Fact.ID, "price list is authoritative")
	assert.ErrorIs(t, err, core.ErrForbidden)
	resolved, err := e.brands.ResolveConflict(ctx, coretest.Admin, conflicts[0].ID, a.Fact.ID, "price list is authoritative")
	require.NoError(t, err)
	assert.Equal(t, "resolved", resolved.State)
	assert.Equal(t, brands.Retired, resolved.FactB.Class)
	assert.Contains(t, resolved.FactB.RetiredReason, "price list is authoritative")

	// Resolving closed the second conflict too (same loser) but approved nothing.
	conflicts, err = e.brands.ListConflicts(ctx, coretest.Viewer, e.reportage)
	require.NoError(t, err)
	assert.Empty(t, conflicts)
	claims, err = e.brands.Claims(ctx, coretest.Viewer, e.reportage, e.wadi, e.clock.Now())
	require.NoError(t, err)
	assert.Empty(t, claims.Claims, "resolving a conflict is not approving")

	approved, err := e.brands.ApproveFact(ctx, coretest.Admin, a.Fact.ID)
	require.NoError(t, err)
	assert.Equal(t, brands.Approved, approved.Class)
	assert.Equal(t, int64(1), *approved.ApprovedBy)
	claims, err = e.brands.Claims(ctx, coretest.Viewer, e.reportage, e.wadi, e.clock.Now())
	require.NoError(t, err)
	require.Len(t, claims.Claims, 1)
	assert.Equal(t, "120000000", claims.Claims[0].Value)
	assert.Equal(t, "AED", *claims.Claims[0].Currency)
	assert.Empty(t, claims.Pending, "the agreeing brochure row is pending but the key is already claimed")

	// Before the effective date there is no claim.
	claims, err = e.brands.Claims(ctx, coretest.Viewer, e.reportage, e.wadi, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Empty(t, claims.Claims)

	// History keeps every row.
	history, err := e.brands.FactHistory(ctx, coretest.Viewer, e.reportage, "price.from")
	require.NoError(t, err)
	assert.Len(t, history, 3)

	// A correction supersedes: the old row is retired with the reason.
	corr, err := e.brands.AddFact(ctx, coretest.Marketer, e.reportage, brands.FactInput{ProjectID: e.wadi, Key: "price.from", Kind: brands.KindMoney, Value: "125000000", Currency: "AED", EffectiveFrom: "2026-11-01", SourceKind: brands.SourceDocument, SourceRef: "price list v4", Note: "November price list", SupersedesID: a.Fact.ID})
	require.NoError(t, err)
	old, err := e.brands.FactHistory(ctx, coretest.Viewer, e.reportage, "price.from")
	require.NoError(t, err)
	for _, f := range old {
		if f.ID == a.Fact.ID {
			assert.Equal(t, brands.Retired, f.Class)
			assert.Equal(t, "superseded: November price list", f.RetiredReason)
		}
	}
	assert.Equal(t, a.Fact.ID, *corr.Fact.SupersedesID)
	assert.Equal(t, brands.NeedsReview, corr.Fact.Class, "a correction is not approved by being a correction")
}

// Catches: a system-inferred fact being approved on entry, a person entering
// an "inferred" fact, and an inferred fact leaking into claims.
func TestInferredFactsAreLabelledAndNeverClaimsOnTheirOwn(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	_, err := e.brands.AddFact(ctx, coretest.Marketer, e.reportage, brands.FactInput{ProjectID: e.wadi, Key: "handover", Kind: brands.KindDate, Value: "2027-06-30", EffectiveFrom: "2026-10-01", SourceKind: brands.SourceEvidence, EvidenceID: 1})
	require.Error(t, err, "people can't mark their own facts inferred")

	sys := core.System(core.ViaJob)
	ev, err := e.db.Writer().CreateEvidence(ctx, store.CreateEvidenceParams{WorkspaceID: 1, Source: "web", Kind: "page", SourceRef: "https://example.com/wadi", Scope: "brand:1", Class: "observed", ObservedAt: e.clock.Now(), PayloadVersion: 1, Payload: `{"handover":"Q2 2027"}`, PayloadHash: "h", Completeness: "complete"})
	require.NoError(t, err)
	inf, err := e.brands.AddFact(ctx, sys, e.reportage, brands.FactInput{ProjectID: e.wadi, Key: "handover", Kind: brands.KindDate, Value: "2027-06-30", EffectiveFrom: "2026-10-01", SourceKind: brands.SourceEvidence, EvidenceID: ev.ID, SourceRef: "https://example.com/wadi"})
	require.NoError(t, err)
	assert.Equal(t, brands.Inferred, inf.Fact.Class)
	assert.Equal(t, ev.ID, *inf.Fact.EvidenceID)

	claims, err := e.brands.Claims(ctx, coretest.Viewer, e.reportage, e.wadi, e.clock.Now())
	require.NoError(t, err)
	assert.Empty(t, claims.Claims)
	assert.Equal(t, []string{"handover"}, claims.Pending)

	// A person can stand behind it.
	ap, err := e.brands.ApproveFact(ctx, coretest.Admin, inf.Fact.ID)
	require.NoError(t, err)
	assert.Equal(t, brands.Approved, ap.Class)
	assert.Equal(t, ev.ID, *ap.EvidenceID, "the evidence link survives approval")
}

// Catches: a competitor's fact appearing among Reportage's claims, a brand-
// wide fact being hidden from a project, a project fact not winning over the
// brand-wide one, and an offer of another brand being attached to a draft.
func TestOtherBrandEvidenceStaysOut(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	add := func(brand, project int64, key, value string) brands.Fact {
		f, err := e.brands.AddFact(ctx, coretest.Marketer, brand, brands.FactInput{ProjectID: project, Key: key, Kind: brands.KindText, Value: value, EffectiveFrom: "2026-01-01", SourceKind: brands.SourceManual})
		require.NoError(t, err)
		_, err = e.brands.ApproveFact(ctx, coretest.Admin, f.Fact.ID)
		require.NoError(t, err)
		return f.Fact
	}
	add(e.rival, e.rivalProj, "payment_plan", "80/20")
	add(e.rival, 0, "developer", "Rival Developments LLC")
	add(e.reportage, 0, "developer", "Reportage Properties LLC")
	add(e.reportage, 0, "payment_plan", "60/40")
	add(e.reportage, e.wadi, "payment_plan", "1% monthly")

	claims, err := e.brands.Claims(ctx, coretest.Viewer, e.reportage, e.wadi, e.clock.Now())
	require.NoError(t, err)
	got := map[string]string{}
	for _, c := range claims.Claims {
		got[c.Key] = c.Value
	}
	assert.Equal(t, map[string]string{"developer": "Reportage Properties LLC", "payment_plan": "1% monthly"}, got)

	// Mixing brand and project is refused outright.
	_, err = e.brands.Claims(ctx, coretest.Viewer, e.rival, e.wadi, e.clock.Now())
	require.Error(t, err)
	_, err = e.brands.AddFact(ctx, coretest.Marketer, e.rival, brands.FactInput{ProjectID: e.wadi, Key: "x", Kind: brands.KindText, Value: "y", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceManual})
	require.Error(t, err)

	// A rival offer can't be built into a Wadi draft; a Reportage one can.
	rivalOffer, err := e.brands.CreateOffer(ctx, coretest.Marketer, e.rival, brands.OfferInput{Name: "Rival summer", Document: brands.OfferDocument{Headline: "Pay 1% monthly"}, EffectiveFrom: "2026-06-01"})
	require.NoError(t, err)
	_, _, err = e.creative.Create(ctx, coretest.Marketer, creative.CreativeInput{ProjectID: e.wadi, Name: "Wadi", Format: assets.Format1x1, Language: assets.English, Copy: assets.Copy{Title: "Villas"}, OfferID: rivalOffer.ID})
	require.Error(t, err)
	assert.Equal(t, core.KindInvalid, kindOf(err))
	ours, err := e.brands.CreateOffer(ctx, coretest.Marketer, e.reportage, brands.OfferInput{ProjectID: e.wadi, Name: "Wadi launch", Document: brands.OfferDocument{Headline: "Up to 15% off and more", Conditions: []string{"on selected units"}}, EffectiveFrom: "2026-10-01", EffectiveTo: strPtr("2026-12-31")})
	require.NoError(t, err)
	c, _, err := e.creative.Create(ctx, coretest.Marketer, creative.CreativeInput{ProjectID: e.wadi, Name: "Wadi", Format: assets.Format1x1, Language: assets.English, Copy: assets.Copy{Title: "Villas", Eyebrow: "Up to 15% off and more", Note: "on selected units"}, OfferID: ours.ID})
	require.NoError(t, err)
	assert.Equal(t, ours.ID, *c.OfferID)
	assert.Equal(t, int64(1), *c.OfferVersion)

	// The draft's offer list for The Wadi has only Reportage's offer.
	brandID, offers, err := e.brands.OffersForProject(ctx, coretest.Viewer, e.wadi)
	require.NoError(t, err)
	assert.Equal(t, e.reportage, brandID)
	require.Len(t, offers, 1)
	assert.Equal(t, ours.ID, offers[0].ID)
}

// Catches: an offer change silently rewriting creatives, an affected list
// that misses rendered or approved work, and approval of the old version
// carrying over to the new one.
func TestChangedOfferNamesTheAffectedCreatives(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	offer, err := e.brands.CreateOffer(ctx, coretest.Marketer, e.reportage, brands.OfferInput{ProjectID: e.wadi, Name: "Wadi launch", Document: brands.OfferDocument{Headline: "10% off", DiscountPct: f64(10)}, EffectiveFrom: "2026-10-01"})
	require.NoError(t, err)
	approved, err := e.brands.SetOfferState(ctx, coretest.Admin, offer.ID, brands.OfferApproved)
	require.NoError(t, err)
	assert.Equal(t, brands.OfferApproved, approved.Current.State)

	c1, _, err := e.creative.Create(ctx, coretest.Marketer, creative.CreativeInput{ProjectID: e.wadi, Name: "Story", Format: assets.Format9x16, Language: assets.English, Copy: assets.Copy{Title: "Villas", Eyebrow: "10% off", Note: "T&C apply"}, OfferID: offer.ID})
	require.NoError(t, err)
	_, err = e.creative.Render(ctx, coretest.Marketer, c1.ID)
	require.NoError(t, err)
	c2, _, err := e.creative.Create(ctx, coretest.Marketer, creative.CreativeInput{ProjectID: e.wadi, Name: "Square", Format: assets.Format1x1, Language: assets.Arabic, Copy: assets.Copy{Title: "فلل", Eyebrow: "خصم ١٠٪", Note: "تطبق الشروط"}, OfferID: offer.ID})
	require.NoError(t, err)
	none, err := e.brands.OfferImpact(ctx, coretest.Viewer, offer.ID)
	require.NoError(t, err)
	assert.Empty(t, none, "nothing is affected while the offer is unchanged")

	change, err := e.brands.ReviseOffer(ctx, coretest.Marketer, offer.ID, brands.OfferInput{Document: brands.OfferDocument{Headline: "12% off", DiscountPct: f64(12)}, EffectiveFrom: "2026-11-01", Note: "November push"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), change.Offer.Current.Version)
	assert.Equal(t, brands.OfferDraft, change.Offer.Current.State, "approval doesn't carry to the new version")
	ids := map[int64]string{}
	for _, a := range change.Affected {
		ids[a.ID] = a.ReviewState
	}
	assert.Equal(t, map[int64]string{c1.ID: "rendered", c2.ID: "draft"}, ids)

	// The creatives still point at version 1; nothing was rewritten.
	got, _, err := e.creative.Get(ctx, coretest.Viewer, c1.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), *got.OfferVersion)
	_, versions, err := e.brands.OfferVersions(ctx, coretest.Viewer, offer.ID)
	require.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, brands.OfferApproved, versions[0].State)

	// A new creative built now is on version 2 and not affected.
	c3, _, err := e.creative.Create(ctx, coretest.Marketer, creative.CreativeInput{ProjectID: e.wadi, Name: "Feed", Format: assets.Format4x5, Language: assets.English, Copy: assets.Copy{Title: "Villas", Eyebrow: "12% off", Note: "T&C apply"}, OfferID: offer.ID})
	require.NoError(t, err)
	affected, err := e.brands.OfferImpact(ctx, coretest.Viewer, offer.ID)
	require.NoError(t, err)
	assert.Len(t, affected, 2)
	for _, a := range affected {
		assert.NotEqual(t, c3.ID, a.ID)
	}
}

// Catches: bad input accepted, and a viewer or marketer doing admin work.
func TestBrandValidationAndRoles(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	_, err := e.brands.Create(ctx, coretest.Marketer, brands.BrandInput{Name: "X", Kind: brands.Own})
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "reportage", Kind: brands.Own})
	assert.Equal(t, core.KindConflict, kindOf(err), "names are unique ignoring case")
	_, err = e.brands.Create(ctx, coretest.Admin, brands.BrandInput{Name: "Y", Kind: "partner"})
	assert.Equal(t, core.KindInvalid, kindOf(err))
	assert.ErrorIs(t, e.brands.AssignProject(ctx, coretest.Marketer, e.wadi, e.rival), core.ErrForbidden)
	for _, in := range []brands.FactInput{
		{Key: "Price From", Kind: brands.KindText, Value: "x", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceManual},
		{Key: "price.from", Kind: brands.KindMoney, Value: "1.5m", Currency: "AED", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceManual},
		{Key: "price.from", Kind: brands.KindMoney, Value: "100", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceManual},
		{Key: "discount.max", Kind: brands.KindPercent, Value: "150", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceManual},
		{Key: "handover", Kind: brands.KindDate, Value: "June 2027", EffectiveFrom: "2026-01-01", SourceKind: brands.SourceManual},
		{Key: "handover", Kind: brands.KindDate, Value: "2027-06-30", EffectiveFrom: "2026-01-01", EffectiveTo: strPtr("2025-01-01"), SourceKind: brands.SourceManual},
		{Key: "handover", Kind: brands.KindDate, Value: "2027-06-30", EffectiveFrom: "2026-01-01", SourceKind: "rumour"},
	} {
		_, err := e.brands.AddFact(ctx, coretest.Marketer, e.reportage, in)
		assert.Equal(t, core.KindInvalid, kindOf(err), "%+v", in)
	}
	_, err = e.brands.AddFact(ctx, coretest.Viewer, e.reportage, price(e.wadi, "1", "2026-01-01", "x"))
	assert.ErrorIs(t, err, core.ErrForbidden)
	list, err := e.brands.List(ctx, coretest.Viewer)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, 1, list[0].ProjectCount)
	detail, err := e.brands.Get(ctx, coretest.Viewer, e.reportage)
	require.NoError(t, err)
	assert.Equal(t, "The Wadi", detail.Projects[0].Name)
	unassigned, err := e.brands.UnassignedProjects(ctx, coretest.Viewer)
	require.NoError(t, err)
	assert.Empty(t, unassigned)
}

func kindOf(err error) core.Kind {
	var ce *core.Error
	if errors.As(err, &ce) {
		return ce.Kind
	}
	return 0
}

func strPtr(s string) *string { return &s }
func f64(f float64) *float64  { return &f }
