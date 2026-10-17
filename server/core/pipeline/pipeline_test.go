package pipeline_test

import (
	"database/sql"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/pipeline"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

type env struct {
	db      *store.DB
	clock   *coretest.Clock
	crm     *crm.CRM
	pipe    *pipeline.Pipeline
	acct    int64
	project int64
}

func setup(t *testing.T, failAfter int) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))}
	coretest.SeedUsers(t, e.db)
	ctx := t.Context()
	ev := evidence.New(e.db, e.clock)
	e.crm = crm.New(e.db, ev, crm.Config{Clock: e.clock})
	e.pipe = pipeline.New(e.db, ev, pipeline.Config{Clock: e.clock, FailAfter: failAfter})
	cred, err := e.db.Writer().UpsertCredential(ctx, store.UpsertCredentialParams{Provider: "gohighlevel", Kind: "private_token", Subject: "loc", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "x", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(ctx, store.UpsertAccountParams{Type: "gohighlevel", ExternalID: "loc-1", CredentialID: cred.ID, Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	e.acct = acct.ID
	p, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e.project = p.ID
	return e
}

// lead writes a CRM lead with a funnel stage and resolves it to a person;
// it returns the enquiry id the resolution created for it.
func (e *env) lead(t *testing.T, ext, first, phone, stage string) int64 {
	t.Helper()
	ctx := t.Context()
	row, err := e.db.Writer().UpsertLead(ctx, store.UpsertLeadParams{WorkspaceID: 1, AccountID: e.acct, ExternalID: ext, FirstName: ns(first), LastName: ns("Haddad"), Phone: ns(phone), PhoneE164: ns(e.crm.NormalizePhone(phone)),
		ProjectID: sql.NullInt64{Int64: e.project, Valid: true}, StatusRaw: ns(stage), StatusObservedAt: sql.NullTime{Time: e.clock.Now(), Valid: true}, Stage: ns(stage), FirstSeenAt: e.clock.Now(), LastSeenAt: e.clock.Now()})
	require.NoError(t, err)
	_, err = e.crm.Sync(ctx, coretest.Marketer)
	require.NoError(t, err)
	var enquiryID int64
	require.NoError(t, e.db.Read.QueryRowContext(ctx, "SELECT id FROM enquiries WHERE source_kind = 'lead' AND source_ref = ?", strconv.FormatInt(row.ID, 10)).Scan(&enquiryID))
	return enquiryID
}

func (e *env) restage(t *testing.T, ext, stage string) {
	t.Helper()
	_, err := e.db.Write.ExecContext(t.Context(), "UPDATE leads SET stage = ?, status_raw = ? WHERE external_id = ?", stage, stage, ext)
	require.NoError(t, err)
}

func ns(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func (e *env) configure(t *testing.T, owner pipeline.Owner) {
	t.Helper()
	doc := pipeline.SuggestedConfig()
	doc.StageOwner = owner
	_, err := e.pipe.PutConfig(t.Context(), coretest.Admin, doc)
	require.NoError(t, err)
}

func (e *env) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	require.NoError(t, e.db.Read.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&n))
	return n
}

// Catches: a second import (or a resumed one) creating a second
// opportunity for an enquiry, and an apply that cannot resume where it
// stopped.
func TestRepeatedImportCreatesNoDuplicateOpportunity(t *testing.T) {
	e := setup(t, 2)
	ctx := t.Context()
	e.configure(t, pipeline.OwnerADX)
	e.lead(t, "c1", "Amal", "0501111111", "engaged")
	e.lead(t, "c2", "Omar", "0502222222", "meeting")
	e.lead(t, "c3", "Layla", "0503333333", "")
	b, err := e.pipe.PreviewImport(ctx, coretest.Marketer, "gohighlevel")
	require.NoError(t, err)
	assert.Equal(t, "previewed", b.State)
	assert.Equal(t, int64(3), b.Total)
	assert.Equal(t, int64(3), b.Created)
	assert.Equal(t, 0, e.count(t, "opportunities"), "preview mutates nothing")

	// FailAfter 2: the first apply stops with a cursor; the second resumes.
	b, err = e.pipe.ApplyImport(ctx, coretest.Marketer, b.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", b.State)
	assert.Equal(t, int64(2), b.Cursor)
	assert.Equal(t, 2, e.count(t, "opportunities"))
	e2 := pipeline.New(e.db, evidence.New(e.db, e.clock), pipeline.Config{Clock: e.clock})
	b, err = e2.ApplyImport(ctx, coretest.Marketer, b.ID)
	require.NoError(t, err)
	assert.Equal(t, "applied", b.State)
	assert.Equal(t, 3, e.count(t, "opportunities"))
	applied := 0
	for _, r := range b.Rows {
		if r.AppliedAt != nil {
			applied++
		}
	}
	assert.Equal(t, 3, applied)
	_, err = e2.ApplyImport(ctx, coretest.Marketer, b.ID)
	require.Error(t, err, "an applied batch does not apply twice")

	// A fresh preview finds nothing to create and an apply adds nothing.
	b2, err := e2.PreviewImport(ctx, coretest.Marketer, "gohighlevel")
	require.NoError(t, err)
	assert.Equal(t, int64(0), b2.Created)
	assert.Equal(t, int64(3), b2.Skipped)
	_, err = e2.ApplyImport(ctx, coretest.Marketer, b2.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, e.count(t, "opportunities"))
	// Mappings are stable: one per opportunity, keyed by the contact id.
	rows, err := e2.Export(ctx, coretest.Viewer)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	for _, r := range rows {
		require.Len(t, r.Mappings, 1)
		assert.Equal(t, "contact", r.Mappings[0].Kind)
	}
	assert.Equal(t, 3, e.count(t, "provider_mappings"))
}

// Catches: an import moving a stage a person set when ADX owns it, and not
// moving it when the CRM owns it.
func TestCustomerCorrectionsSurviveSyncAccordingToOwnership(t *testing.T) {
	e := setup(t, 0)
	ctx := t.Context()
	e.configure(t, pipeline.OwnerADX)
	enq := e.lead(t, "c1", "Amal", "0501111111", "engaged")
	b, _ := e.pipe.PreviewImport(ctx, coretest.Marketer, "")
	_, err := e.pipe.ApplyImport(ctx, coretest.Marketer, b.ID)
	require.NoError(t, err)
	opps, _ := e.pipe.List(ctx, coretest.Viewer, "", "", 0, 10)
	require.Len(t, opps, 1)
	assert.Equal(t, "engaged", opps[0].Stage)
	assert.Equal(t, "import", opps[0].StageSetBy)

	// A salesperson corrects the stage after a meeting.
	moved, err := e.pipe.SetStage(ctx, coretest.Marketer, opps[0].ID, "meeting", opps[0].Version, "met them on site")
	require.NoError(t, err)
	assert.Equal(t, "user", moved.StageSetBy)
	assert.Equal(t, int64(2), moved.Version)
	// The CRM still says engaged, then moves to attempted: ADX owns the stage, so the correction survives.
	e.restage(t, "c1", "attempted")
	b, _ = e.pipe.PreviewImport(ctx, coretest.Marketer, "")
	require.Len(t, b.Rows, 1)
	assert.Equal(t, "skip", b.Rows[0].Action)
	assert.Contains(t, string(b.Rows[0].Detail), "a person set it")
	_, err = e.pipe.ApplyImport(ctx, coretest.Marketer, b.ID)
	require.NoError(t, err)
	d, _ := e.pipe.Get(ctx, coretest.Viewer, opps[0].ID)
	assert.Equal(t, "meeting", d.Opportunity.Stage)

	// The CRM owns the stage: the sync moves it, and says who did.
	e.configure(t, pipeline.OwnerCRM)
	b, _ = e.pipe.PreviewImport(ctx, coretest.Marketer, "")
	assert.Equal(t, "update_stage", b.Rows[0].Action)
	_, err = e.pipe.ApplyImport(ctx, coretest.Marketer, b.ID)
	require.NoError(t, err)
	d, _ = e.pipe.Get(ctx, coretest.Viewer, opps[0].ID)
	assert.Equal(t, "attempted", d.Opportunity.Stage)
	assert.Equal(t, "import", d.Opportunity.StageSetBy)
	// A config change between preview and apply is refused.
	b, _ = e.pipe.PreviewImport(ctx, coretest.Marketer, "")
	e.configure(t, pipeline.OwnerADX)
	_, err = e.pipe.ApplyImport(ctx, coretest.Marketer, b.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configuration changed")
	_ = enq
}

// Catches: two enquiries of one person sharing an opportunity, a stage
// move on one touching the other, and tasks duplicated on repeat.
func TestMultipleEnquiriesPerPersonStayIndependent(t *testing.T) {
	e := setup(t, 0)
	ctx := t.Context()
	e.configure(t, pipeline.OwnerADX)
	d, err := e.crm.Create(ctx, coretest.Marketer, crm.PersonInput{DisplayName: "Amal Haddad", Phone: "0501111111"})
	require.NoError(t, err)
	oceana, err := e.db.Writer().CreateProject(ctx, store.CreateProjectParams{Name: "Oceana", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e1, err := e.db.Writer().CreateEnquiry(ctx, store.CreateEnquiryParams{WorkspaceID: 1, PersonID: d.Person.ID, ProjectID: sql.NullInt64{Int64: e.project, Valid: true}, SourceKind: "manual", SourceRef: "a", FirstContactAt: e.clock.Now(), LastActivityAt: e.clock.Now(), CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	e2, err := e.db.Writer().CreateEnquiry(ctx, store.CreateEnquiryParams{WorkspaceID: 1, PersonID: d.Person.ID, ProjectID: sql.NullInt64{Int64: oceana.ID, Valid: true}, SourceKind: "manual", SourceRef: "b", FirstContactAt: e.clock.Now(), LastActivityAt: e.clock.Now(), CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	o1, created, err := e.pipe.Open(ctx, coretest.Marketer, e1.ID)
	require.NoError(t, err)
	assert.True(t, created)
	o1b, created, err := e.pipe.Open(ctx, coretest.Marketer, e1.ID)
	require.NoError(t, err)
	assert.False(t, created, "one enquiry, one opportunity")
	assert.Equal(t, o1.ID, o1b.ID)
	o2, _, err := e.pipe.Open(ctx, coretest.Marketer, e2.ID)
	require.NoError(t, err)
	assert.NotEqual(t, o1.ID, o2.ID)
	assert.Equal(t, 2, e.count(t, "opportunities"))

	_, err = e.pipe.SetStage(ctx, coretest.Marketer, o1.ID, "won", o1.Version, "signed")
	require.NoError(t, err)
	d1, _ := e.pipe.Get(ctx, coretest.Viewer, o1.ID)
	d2, _ := e.pipe.Get(ctx, coretest.Viewer, o2.ID)
	assert.Equal(t, "won", d1.Opportunity.State)
	assert.NotNil(t, d1.Opportunity.ClosedAt)
	assert.Equal(t, "open", d2.Opportunity.State, "the other enquiry is untouched")
	assert.Equal(t, "open", d2.Opportunity.Stage)

	// Assignments are per opportunity; the owner follows the latest.
	_, err = e.pipe.Assign(ctx, coretest.Marketer, o2.ID, coretest.Marketer.UserID, "territory")
	require.NoError(t, err)
	_, err = e.pipe.Assign(ctx, coretest.Admin, o2.ID, coretest.Admin.UserID, "escalated")
	require.NoError(t, err)
	d2, _ = e.pipe.Get(ctx, coretest.Viewer, o2.ID)
	require.Len(t, d2.Assignments, 2)
	assert.NotNil(t, d2.Assignments[0].EndedAt)
	assert.Nil(t, d2.Assignments[1].EndedAt)
	assert.Equal(t, coretest.Admin.UserID, *d2.Opportunity.OwnerUserID)
	d1, _ = e.pipe.Get(ctx, coretest.Viewer, o1.ID)
	assert.Empty(t, d1.Assignments)
	_, err = e.pipe.Assign(ctx, coretest.Marketer, o2.ID, 999, "nobody")
	require.Error(t, err)

	// Follow-up tasks are created once per occurrence key.
	due := e.clock.Now().Add(48 * time.Hour)
	t1, created, err := e.pipe.CreateTask(ctx, coretest.Marketer, pipeline.TaskInput{OpportunityID: o2.ID, Title: "Call back Tuesday", DueAt: &due, AssigneeUserID: coretest.Admin.UserID})
	require.NoError(t, err)
	assert.True(t, created)
	t1b, created, err := e.pipe.CreateTask(ctx, coretest.Marketer, pipeline.TaskInput{OpportunityID: o2.ID, Title: "call back tuesday"})
	require.NoError(t, err)
	assert.False(t, created, "same task")
	assert.Equal(t, t1.ID, t1b.ID)
	open, _ := e.pipe.ListTasks(ctx, coretest.Viewer, "open", 0, 10)
	assert.Len(t, open, 1)
	done, err := e.pipe.CompleteTask(ctx, coretest.Marketer, t1.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "done", done.State)
	_, err = e.pipe.CompleteTask(ctx, coretest.Marketer, t1.ID, false)
	require.Error(t, err)
	// Interactions attach to the person and, optionally, one opportunity.
	i, err := e.pipe.RecordInteraction(ctx, coretest.Marketer, pipeline.InteractionInput{PersonID: d.Person.ID, OpportunityID: o2.ID, Kind: "meeting", Summary: "site visit"})
	require.NoError(t, err)
	assert.NotNil(t, i.EvidenceID)
	_, err = e.pipe.RecordInteraction(ctx, coretest.Marketer, pipeline.InteractionInput{PersonID: d.Person.ID, OpportunityID: o1.ID, Kind: "sms", Summary: "x"})
	require.Error(t, err, "unknown kind")
	d2, _ = e.pipe.Get(ctx, coretest.Viewer, o2.ID)
	assert.Len(t, d2.Interactions, 1)
	d1, _ = e.pipe.Get(ctx, coretest.Viewer, o1.ID)
	assert.Empty(t, d1.Interactions, "the meeting was about Oceana")
}

// Catches: a preview that writes, a stale edit overwriting a newer one,
// and an unconfigured pipeline accepting opportunities.
func TestMigrationPreviewDoesNotMutateAndVersionsCollide(t *testing.T) {
	e := setup(t, 0)
	ctx := t.Context()
	enq := e.lead(t, "c1", "Amal", "0501111111", "engaged")
	_, _, err := e.pipe.Open(ctx, coretest.Marketer, enq)
	var ce *core.Error
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind, "no stages, no pipeline")
	_, err = e.pipe.PutConfig(ctx, coretest.Marketer, pipeline.SuggestedConfig())
	assert.ErrorIs(t, err, core.ErrForbidden)
	bad := pipeline.SuggestedConfig()
	bad.LeadStageMap["won"] = "nope"
	_, err = e.pipe.PutConfig(ctx, coretest.Admin, bad)
	require.Error(t, err)
	e.configure(t, pipeline.OwnerADX)

	before := map[string]int{}
	for _, tbl := range []string{"opportunities", "opportunity_assignments", "follow_up_tasks", "interactions", "provider_mappings", "enquiries", "leads", "people"} {
		before[tbl] = e.count(t, tbl)
	}
	b, err := e.pipe.PreviewImport(ctx, coretest.Marketer, "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), b.Created)
	for tbl, n := range before {
		assert.Equal(t, n, e.count(t, tbl), tbl+" unchanged by preview")
	}
	assert.Equal(t, 1, e.count(t, "crm_import_batches"))
	_, err = e.pipe.PreviewImport(ctx, coretest.Viewer, "")
	assert.ErrorIs(t, err, core.ErrForbidden)

	o, _, err := e.pipe.Open(ctx, coretest.Marketer, enq)
	require.NoError(t, err)
	a, err := e.pipe.SetStage(ctx, coretest.Marketer, o.ID, "engaged", o.Version, "first")
	require.NoError(t, err)
	_, err = e.pipe.SetStage(ctx, coretest.Admin, o.ID, "meeting", o.Version, "stale tab")
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, core.KindConflict, ce.Kind, "a stale version does not overwrite")
	cur, _ := e.pipe.Get(ctx, coretest.Viewer, o.ID)
	assert.Equal(t, "engaged", cur.Opportunity.Stage)
	v, err := e.pipe.SetValue(ctx, coretest.Marketer, o.ID, 150000000, "aed", a.Version)
	require.NoError(t, err)
	assert.Equal(t, "AED", *v.Currency)
	assert.Equal(t, int64(150000000), *v.ValueMinor)
	_, err = e.pipe.SetStage(ctx, coretest.Marketer, o.ID, "unknown", v.Version, "x")
	require.Error(t, err)
	// The preview planned a create; applying it now finds the opportunity
	// already there and creates nothing.
	_, err = e.pipe.ApplyImport(ctx, coretest.Marketer, b.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, e.count(t, "opportunities"))
}
