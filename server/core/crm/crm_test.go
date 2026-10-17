package crm_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type fakeEnricher struct {
	result crm.Enrichment
	err    error
	calls  int
}

func (f *fakeEnricher) Name() string          { return "fake-enrich" }
func (f *fakeEnricher) Cost() (int64, string) { return 35, "USD" }
func (f *fakeEnricher) Permitted() []string {
	return []string{"company", "job_title", "country", "home_address"}
}
func (f *fakeEnricher) Enrich(context.Context, crm.Identity) (crm.Enrichment, error) {
	f.calls++
	return f.result, f.err
}

type env struct {
	db       *store.DB
	clock    *coretest.Clock
	crm      *crm.CRM
	enricher *fakeEnricher
	acct     int64
	wadi     int64
	oceana   int64
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)), enricher: &fakeEnricher{}}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	e.crm = crm.New(e.db, evidence.New(e.db, e.clock), crm.Config{Clock: e.clock, Enricher: e.enricher})
	cred, err := e.db.Writer().UpsertCredential(ctx, store.UpsertCredentialParams{Provider: "gohighlevel", Kind: "private_token", Subject: "loc", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "x", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(ctx, store.UpsertAccountParams{Type: "gohighlevel", ExternalID: "loc-1", CredentialID: cred.ID, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	e.acct = acct.ID
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.wadi = p.ID
	o, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{Name: "Oceana", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.oceana = o.ID
	return e
}

func (e *env) lead(t *testing.T, ext, first, last, phone, email string, project int64, stage string) int64 {
	t.Helper()
	row, err := e.db.Writer().UpsertLead(t.Context(), store.UpsertLeadParams{WorkspaceID: 1, AccountID: e.acct, ExternalID: ext, FirstName: ns(first), LastName: ns(last), Email: ns(email), Phone: ns(phone), PhoneE164: ns(e.crm.NormalizePhone(phone)),
		ProjectID: sql.NullInt64{Int64: project, Valid: project != 0}, Stage: ns(stage), FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	return row.ID
}

func ns(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// Catches: two family members on one phone being merged, a same-name
// re-enquiry for another project making a second person, and a partial
// match being hidden.
func TestSharedFamilyPhoneIsNotAutoMerged(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.lead(t, "c1", "Amal", "Haddad", "+971 50 123 4567", "", e.wadi, "engaged")
	e.lead(t, "c2", "Omar", "Haddad", "050 123 4567", "", e.wadi, "open")
	n, err := e.crm.Sync(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	people, err := e.crm.List(ctx, coretest.Viewer, "", 50)
	require.NoError(t, err)
	require.Len(t, people, 2, "a shared phone with different names is two people")
	var omar crm.PersonSummary
	for _, p := range people {
		if p.DisplayName == "Omar Haddad" {
			omar = p
		}
	}
	assert.True(t, omar.Uncertain)
	assert.Equal(t, 1, omar.OpenCandidates)
	detail, err := e.crm.Get(ctx, coretest.Viewer, omar.ID)
	require.NoError(t, err)
	require.Len(t, detail.Candidates, 1)
	assert.Equal(t, "Amal Haddad", detail.Candidates[0].Other.DisplayName)
	assert.Contains(t, string(detail.Candidates[0].Reason), `"phone":true`)
	require.Len(t, detail.Links, 1)
	assert.Equal(t, "new", detail.Links[0].MatchLevel)

	// Amal enquires about another project with the same phone and name:
	// the same person, a second enquiry.
	e.lead(t, "c3", "Amal", "Haddad", "+971501234567", "", e.oceana, "meeting")
	_, err = e.crm.Sync(ctx, coretest.Marketer)
	require.NoError(t, err)
	people, _ = e.crm.List(ctx, coretest.Viewer, "Amal", 50)
	require.Len(t, people, 1)
	assert.Equal(t, 2, people[0].Enquiries)
	amal, _ := e.crm.Get(ctx, coretest.Viewer, people[0].ID)
	projects := map[int64]bool{}
	for _, q := range amal.Enquiries {
		projects[*q.ProjectID] = true
	}
	assert.Equal(t, map[int64]bool{e.wadi: true, e.oceana: true}, projects)
	assert.Equal(t, "exact", amal.Links[1].MatchLevel)
	assert.Contains(t, string(amal.Links[1].MatchEvidence), `"name":"agrees"`)

	// A third person on the family phone: candidates for both, no merge.
	e.lead(t, "c4", "Layla", "Haddad", "0501234567", "", e.wadi, "")
	_, err = e.crm.Sync(ctx, coretest.Marketer)
	require.NoError(t, err)
	people, _ = e.crm.List(ctx, coretest.Viewer, "", 50)
	assert.Len(t, people, 3)
	open, err := e.crm.OpenCandidates(ctx, coretest.Viewer, 50)
	require.NoError(t, err)
	assert.Len(t, open, 3, "Omar→Amal, Layla→Amal, Layla→Omar")

	// Sync again adds nothing.
	n, _ = e.crm.Sync(ctx, coretest.Marketer)
	assert.Equal(t, 0, n)
}

// Catches: a candidate decision that doesn't merge, a merge that loses
// enquiries or identifiers, a merge that can't be undone, and a reversed
// link staying attached.
func TestDecisionsMergesAndReversalsAreRecordedAndReversible(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.lead(t, "c1", "Amal", "Haddad", "+971501234567", "amal@example.com", e.wadi, "engaged")
	e.lead(t, "c2", "A. Haddad", "", "+971501234567", "", e.oceana, "open")
	_, err := e.crm.Sync(ctx, coretest.Marketer)
	require.NoError(t, err)
	people, _ := e.crm.List(ctx, coretest.Viewer, "", 50)
	require.Len(t, people, 2, "'A.' does not agree with 'Amal'")
	var newer, older crm.PersonSummary
	for _, p := range people {
		assert.True(t, p.Uncertain, "an open candidate marks both sides uncertain")
		if p.DisplayName == "A. Haddad" {
			newer = p
		} else {
			older = p
		}
	}
	cands, _ := e.crm.OpenCandidates(ctx, coretest.Viewer, 10)
	require.Len(t, cands, 1)
	_, err = e.crm.Decide(ctx, coretest.Viewer, cands[0].ID, true, "")
	assert.ErrorIs(t, err, core.ErrForbidden)
	merged, err := e.crm.Decide(ctx, coretest.Marketer, cands[0].ID, true, "same person, abbreviated name")
	require.NoError(t, err)
	assert.Equal(t, older.ID, merged.Person.ID)
	assert.Len(t, merged.Enquiries, 2, "both enquiries now on one person")
	assert.Len(t, merged.Links, 2)
	kinds := map[string]int{}
	for _, i := range merged.Identifiers {
		kinds[i.Kind]++
	}
	assert.Equal(t, 1, kinds["phone"], "the shared phone is one identifier")
	assert.Equal(t, 1, kinds["email"])
	assert.Equal(t, 2, kinds["external"])
	gone, err := e.crm.Get(ctx, coretest.Viewer, newer.ID)
	require.NoError(t, err)
	assert.Equal(t, older.ID, *gone.Person.MergedIntoID)
	people, _ = e.crm.List(ctx, coretest.Viewer, "", 50)
	assert.Len(t, people, 1, "merged people leave the list")
	_, err = e.crm.Decide(ctx, coretest.Marketer, cands[0].ID, true, "")
	require.Error(t, err, "already decided")

	// Undo: both people back, with their own enquiries and links.
	restored, err := e.crm.Unmerge(ctx, coretest.Marketer, newer.ID, "they are cousins")
	require.NoError(t, err)
	assert.Nil(t, restored.Person.MergedIntoID)
	assert.Len(t, restored.Enquiries, 1)
	assert.Len(t, restored.Links, 1)
	again, _ := e.crm.Get(ctx, coretest.Viewer, older.ID)
	assert.Len(t, again.Enquiries, 1)
	assert.Len(t, again.Links, 1)
	people, _ = e.crm.List(ctx, coretest.Viewer, "", 50)
	assert.Len(t, people, 2)

	// Reverse a link; the lead is unlinked and the next sync re-resolves it.
	link := again.Links[0]
	_, err = e.crm.ReverseLink(ctx, coretest.Marketer, link.ID, "")
	require.Error(t, err, "reason required")
	rev, err := e.crm.ReverseLink(ctx, coretest.Marketer, link.ID, "wrong person")
	require.NoError(t, err)
	assert.NotNil(t, rev.ReversedAt)
	_, err = e.crm.ReverseLink(ctx, coretest.Marketer, link.ID, "again")
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
	n, err := e.crm.Sync(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the unlinked lead is resolved again")
	// Manual link of an already linked record is refused.
	_, err = e.crm.LinkManually(ctx, coretest.Marketer, older.ID, "lead", link.Ref)
	require.Error(t, err)
}

// Catches: an enriched value overwriting what the customer said, an
// attribute outside the permitted list being stored, and an enrichment
// without evidence or cost.
func TestConflictingEnrichmentDoesNotOverwriteStatedFacts(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	d, err := e.crm.Create(ctx, coretest.Marketer, crm.PersonInput{DisplayName: "Amal Haddad", Phone: "0501234567", Email: "Amal@Example.com"})
	require.NoError(t, err)
	assert.Equal(t, "+971501234567", *d.Person.PrimaryPhone)
	assert.Equal(t, "amal@example.com", *d.Person.PrimaryEmail)
	assert.True(t, d.Enrichment.Configured)
	assert.Equal(t, int64(35), d.Enrichment.CostMinor)

	d, err = e.crm.SetStated(ctx, coretest.Marketer, d.Person.ID, "company", "Haddad Trading LLC", "call")
	require.NoError(t, err)
	e.enricher.result = crm.Enrichment{Attributes: map[string]string{"company": "Haddad Holdings", "job_title": "Director", "home_address": "Villa 12, Al Barsha", "net_worth": "high"}, Freshness: e.clock.Now().Add(-48 * time.Hour)}
	d, err = e.crm.Enrich(ctx, coretest.Marketer, d.Person.ID)
	require.NoError(t, err)
	byKey := map[string]crm.AttributeView{}
	for _, a := range d.Attributes {
		byKey[a.Key] = a
	}
	assert.Equal(t, "Haddad Trading LLC", byKey["company"].Value, "the customer's word stands")
	assert.Equal(t, "stated", byKey["company"].Class)
	assert.True(t, byKey["company"].Conflict)
	require.NotNil(t, byKey["company"].Enriched)
	assert.Equal(t, "Haddad Holdings", byKey["company"].Enriched.Value)
	assert.Equal(t, "Director", byKey["job_title"].Value)
	assert.Equal(t, "enriched", byKey["job_title"].Class)
	assert.False(t, byKey["job_title"].Conflict)
	_, hasAddress := byKey["home_address"]
	assert.False(t, hasAddress, "not in ADX's permitted list")
	_, hasWealth := byKey["net_worth"]
	assert.False(t, hasWealth, "not permitted by either side")
	require.NotNil(t, byKey["job_title"].Enriched.EvidenceID)
	ev, err := evidence.New(e.db, e.clock).Get(ctx, coretest.Viewer, *byKey["job_title"].Enriched.EvidenceID)
	require.NoError(t, err)
	assert.Equal(t, evidence.Inferred, ev.Class)
	assert.Contains(t, string(ev.Payload), `"costMinor":35`)
	assert.Contains(t, string(ev.Payload), `"home_address"`, "dropped keys are on record")

	// The customer corrects the company: stated supersedes stated; enriched untouched.
	d, err = e.crm.SetStated(ctx, coretest.Marketer, d.Person.ID, "company", "Haddad Trading", "call")
	require.NoError(t, err)
	for _, a := range d.Attributes {
		if a.Key == "company" {
			assert.Equal(t, "Haddad Trading", a.Value)
			assert.True(t, a.Conflict)
		}
	}
	// Without a provider, enrichment is a stated conflict, not a silent no-op.
	bare := crm.New(e.db, nil, crm.Config{Clock: e.clock})
	assert.False(t, bare.EnrichmentState().Configured)
	_, err = bare.Enrich(ctx, coretest.Marketer, d.Person.ID)
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind)
	_, err = e.crm.Enrich(ctx, coretest.Viewer, d.Person.ID)
	assert.ErrorIs(t, err, core.ErrForbidden)
}

// Catches: a form submission that matches nobody being swallowed, an email
// exact match being treated as uncertain, and bad input on manual creation.
func TestFormSubmissionsResolveLikeLeads(t *testing.T) {
	e := setup(t)
	ctx := t.Context()
	e.lead(t, "c1", "Amal", "Haddad", "+971501234567", "amal@example.com", e.wadi, "engaged")
	_, err := e.crm.Sync(ctx, coretest.Marketer)
	require.NoError(t, err)
	brand, err := e.db.Writer().CreateBrand(ctx, store.CreateBrandParams{WorkspaceID: 1, Name: "Reportage", Kind: "own", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	pg, err := e.db.Writer().CreatePage(ctx, store.CreatePageParams{WorkspaceID: 1, BrandID: brand.ID, ProjectID: sql.NullInt64{Int64: e.oceana, Valid: true}, Slug: "oceana", Name: "Oceana", Language: "en", TestToken: "t", CreatedAt: e.clock.Now(), UpdatedAt: e.clock.Now()})
	require.NoError(t, err)
	v, err := e.db.Writer().CreatePageVersion(ctx, store.CreatePageVersionParams{PageID: pg.ID, Version: 1, Document: `{}`, CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	_, err = e.db.Writer().CreateFormSubmission(ctx, store.CreateFormSubmissionParams{WorkspaceID: 1, PageID: pg.ID, VersionID: v.ID, ProjectID: sql.NullInt64{Int64: e.oceana, Valid: true}, State: "received", Fields: `{"full_name":"Amal H.","email":"AMAL@example.com","mobile":"050 123 4567"}`, Attribution: "{}", ReceivedAt: e.clock.Now()})
	require.NoError(t, err)
	_, err = e.db.Writer().CreateFormSubmission(ctx, store.CreateFormSubmissionParams{WorkspaceID: 1, PageID: pg.ID, VersionID: v.ID, ProjectID: sql.NullInt64{Int64: e.oceana, Valid: true}, State: "received", Test: 1, Fields: `{"name":"QA","phone":"+971500000000"}`, Attribution: "{}", ReceivedAt: e.clock.Now()})
	require.NoError(t, err)
	_, err = e.db.Writer().CreateFormSubmission(ctx, store.CreateFormSubmissionParams{WorkspaceID: 1, PageID: pg.ID, VersionID: v.ID, ProjectID: sql.NullInt64{Int64: e.oceana, Valid: true}, State: "received", Fields: `{"name":"Someone New","phone":"+971559999999"}`, Attribution: "{}", ReceivedAt: e.clock.Now()})
	require.NoError(t, err)
	n, err := e.crm.Sync(ctx, coretest.Marketer)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "the test submission is never a person")
	people, _ := e.crm.List(ctx, coretest.Viewer, "", 50)
	assert.Len(t, people, 2)
	amal, _ := e.crm.List(ctx, coretest.Viewer, "Amal", 50)
	require.Len(t, amal, 1)
	assert.Equal(t, 2, amal[0].Enquiries, "the email matched exactly: same person, new project")
	assert.False(t, amal[0].Uncertain)

	_, err = e.crm.Create(ctx, coretest.Marketer, crm.PersonInput{Phone: "12"})
	require.Error(t, err)
	_, err = e.crm.Create(ctx, coretest.Marketer, crm.PersonInput{Email: "nope"})
	require.Error(t, err)
	_, err = e.crm.Create(ctx, coretest.Viewer, crm.PersonInput{DisplayName: "x"})
	assert.ErrorIs(t, err, core.ErrForbidden)
}
