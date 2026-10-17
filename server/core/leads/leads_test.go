package leads_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// fakeCRM serves pages of contacts. failAtPage makes that page fail once,
// which is how an interrupted scan is simulated.
type fakeCRM struct {
	pages      [][]leads.Contact
	failAtPage int
	calls      []string // cursors asked for
	lastSince  *time.Time
}

func (f *fakeCRM) ListContacts(_ context.Context, token, _ string, cursor string, since *time.Time) (leads.ContactsPage, error) {
	if token != "pit-token" {
		return leads.ContactsPage{}, core.Conflict("GoHighLevel access was rejected")
	}
	f.calls = append(f.calls, cursor)
	f.lastSince = since
	idx := 0
	if cursor != "" {
		idx = int(cursor[len(cursor)-1] - '0')
	}
	if f.failAtPage == idx+1 {
		f.failAtPage = 0
		return leads.ContactsPage{}, core.Conflict("GoHighLevel request limit reached; try again later")
	}
	total := int64(0)
	for _, p := range f.pages {
		total += int64(len(p))
	}
	page := leads.ContactsPage{Contacts: f.pages[idx], Total: &total}
	if idx+1 < len(f.pages) {
		page.NextCursor = "cursor-" + string(rune('0'+idx+1))
	}
	return page, nil
}

type fakeTokens struct{}

func (fakeTokens) GHLToken(context.Context, core.Actor, string) (string, error) {
	return "pit-token", nil
}

type env struct {
	db      *store.DB
	clock   *coretest.Clock
	crm     *fakeCRM
	leads   *leads.Leads
	runs    *runs.Runs
	account int64
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{db: storetest.Open(t), clock: coretest.NewClock(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)), crm: &fakeCRM{}}
	coretest.SeedUsers(t, e.db)
	e.runs = runs.New(e.db, nil, runs.Config{Clock: e.clock})
	e.leads = leads.New(e.db, e.crm, fakeTokens{}, evidence.New(e.db, e.clock), leads.Config{Clock: e.clock})
	cred, err := e.db.Writer().UpsertCredential(t.Context(), store.UpsertCredentialParams{Provider: "gohighlevel", Kind: "private_token", Subject: "loc-1",
		EncryptedSecret: []byte("sealed"), KeyVersion: 1, Scopes: "", ConnectedAt: e.clock.Now()})
	require.NoError(t, err)
	acct, err := e.db.Writer().UpsertAccount(t.Context(), store.UpsertAccountParams{WorkspaceID: core.DefaultWorkspace, Type: "gohighlevel", ExternalID: "loc-1", CredentialID: cred.ID,
		Details: "{}", ConnectedAt: e.clock.Now(), VerifiedAt: e.clock.Now()})
	require.NoError(t, err)
	e.account = acct.ID
	_, err = e.db.Writer().CreateProject(t.Context(), store.CreateProjectParams{WorkspaceID: core.DefaultWorkspace, Name: "The Wadi", CreatedAt: e.clock.Now()})
	require.NoError(t, err)
	return e
}

func stagePtr(s leads.Stage) *leads.Stage { return &s }

func contact(id, phone, status string) leads.Contact {
	c := leads.Contact{ID: id, FirstName: "Lead", LastName: id, Phone: phone, Source: "Meta", CustomFields: map[string]string{"cf_project": "the wadi"}}
	if status != "" {
		c.CustomFields["cf_status"] = status
	}
	c.Raw, _ = json.Marshal(map[string]any{"id": id, "phone": phone, "status": status})
	return c
}

func mapping() leads.FunnelDocument {
	return leads.FunnelDocument{
		StatusField:  leads.FieldLocation{Kind: "custom_field", Ref: "cf_status"},
		ProjectField: leads.FieldLocation{Kind: "custom_field", Ref: "cf_project"},
		Statuses: map[string]leads.Stage{"No Answer": leads.StageAttempted, "Follow Up": leads.StageEngaged, "Done-Deal": leads.StageWon,
			"Not Interested": leads.StageDisqualified},
		BlankStatusStage: stagePtr(leads.StageOpen),
		Sources:          map[string]string{"Meta": "meta", "whats app Camp.": "handling_channel"},
	}
}

// runImport executes a full import through the runs machinery with the
// fake CRM, the way the River worker does.
func (e *env) runImport(t *testing.T, mode string) runs.Run {
	t.Helper()
	run, _, err := e.leads.StartImport(t.Context(), coretest.Marketer, e.runs, e.account, mode, nil)
	require.NoError(t, err)
	h, err := e.runs.Begin(t.Context(), run.RowID(), "test")
	require.NoError(t, err)
	completion, execErr := e.leads.Execute(t.Context(), h)
	if execErr != nil {
		require.NoError(t, h.Finish(t.Context(), runs.Completion{State: runs.StateFailed, Error: execErr.Error()}))
	} else {
		require.NoError(t, h.Finish(t.Context(), completion))
	}
	got, err := e.runs.Get(t.Context(), coretest.Viewer, run.ID)
	require.NoError(t, err)
	return got
}

// Catches: a contact returned without its status field wiping the status
// ADX already knew, and the funnel classifying leads before anyone mapped it.
func TestMissingStatusKeepsTheKnownValueAndUnmappedStaysNull(t *testing.T) {
	e := setup(t)
	c := contact("c-1", "+971501234567", "Follow Up")

	// No mapping yet: status can't even be located, so nothing is classified.
	lead, err := e.leads.Ingest(t.Context(), coretest.Marketer, e.account, c)
	require.NoError(t, err)
	assert.Nil(t, lead.StatusRaw)
	assert.Nil(t, lead.Stage)
	assert.Nil(t, lead.StatusMissingSince, "with no field configured, nothing is 'missing'")
	cov, err := e.leads.Coverage(t.Context(), coretest.Viewer, 0)
	require.NoError(t, err)
	assert.False(t, cov.FunnelConfigured)

	_, err = e.leads.PutFunnel(t.Context(), coretest.Admin, mapping())
	require.NoError(t, err)
	lead, err = e.leads.Ingest(t.Context(), coretest.Marketer, e.account, c)
	require.NoError(t, err)
	assert.Equal(t, "Follow Up", *lead.StatusRaw)
	assert.Equal(t, leads.StageEngaged, *lead.Stage)
	assert.NotNil(t, lead.ProjectID, "the wadi matched The Wadi case-insensitively")
	assert.Equal(t, "meta", *lead.Source)
	assert.Equal(t, "971", *lead.DialCode)

	// Same contact comes back without the status field at all.
	e.clock.Advance(time.Hour)
	without := contact("c-1", "+971501234567", "")
	lead, err = e.leads.Ingest(t.Context(), coretest.Marketer, e.account, without)
	require.NoError(t, err)
	assert.Equal(t, "Follow Up", *lead.StatusRaw, "known status is kept")
	assert.Equal(t, leads.StageEngaged, *lead.Stage)
	require.NotNil(t, lead.StatusMissingSince)
	assert.Equal(t, e.clock.Now(), *lead.StatusMissingSince)

	// An unmapped status is kept raw, has no stage, and shows up to be mapped.
	odd := contact("c-2", "+971509999999", "Ready To Move")
	lead, err = e.leads.Ingest(t.Context(), coretest.Marketer, e.account, odd)
	require.NoError(t, err)
	assert.Equal(t, "Ready To Move", *lead.StatusRaw)
	assert.Nil(t, lead.Stage)
	state, err := e.leads.GetFunnel(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	assert.Equal(t, []string{"Ready To Move"}, state.Unmapped)

	// Mapping it re-stages history.
	doc := mapping()
	doc.Statuses["ready to move"] = leads.StageDisqualified
	m, err := e.leads.PutFunnel(t.Context(), coretest.Admin, doc)
	require.NoError(t, err)
	assert.Equal(t, int64(2), m.Version)
	list, err := e.leads.List(t.Context(), coretest.Viewer, leads.ListFilter{StatusRaw: "Ready To Move"})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, leads.StageDisqualified, *list[0].Stage)
	state, err = e.leads.GetFunnel(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	assert.Empty(t, state.Unmapped)
}

// Catches: two people on one phone being folded into one lead.
func TestSharedPhoneNeverMerges(t *testing.T) {
	e := setup(t)
	_, err := e.leads.PutFunnel(t.Context(), coretest.Admin, mapping())
	require.NoError(t, err)
	a, err := e.leads.Ingest(t.Context(), coretest.Marketer, e.account, contact("father", "+971 50 123 4567", "No Answer"))
	require.NoError(t, err)
	b, err := e.leads.Ingest(t.Context(), coretest.Marketer, e.account, contact("son", "0501234567", "Follow Up"))
	require.NoError(t, err)
	assert.NotEqual(t, a.ID, b.ID)
	list, err := e.leads.List(t.Context(), coretest.Viewer, leads.ListFilter{})
	require.NoError(t, err)
	assert.Len(t, list, 2)
	cov, err := e.leads.Coverage(t.Context(), coretest.Viewer, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), cov.SharedPhones, "the shared number is reported, not merged")
	assert.Equal(t, int64(2), cov.WithPhone)
}

// Catches: a scan that failed halfway marking the contacts it never reached
// as removed, and a retry starting from page one instead of the cursor.
func TestIncompleteScanRemovesNothingAndResumes(t *testing.T) {
	e := setup(t)
	_, err := e.leads.PutFunnel(t.Context(), coretest.Admin, mapping())
	require.NoError(t, err)
	e.crm.pages = [][]leads.Contact{
		{contact("c-1", "+971501000001", "Follow Up"), contact("c-2", "+971501000002", "No Answer")},
		{contact("c-3", "+971501000003", "Done-Deal")},
	}
	first := e.runImport(t, "full")
	assert.Equal(t, runs.StateSucceeded, first.State)
	assert.Equal(t, []string{"", "cursor-1"}, e.crm.calls)

	// Next day: c-2 is gone from the CRM, but page 2 fails.
	e.clock.Advance(24 * time.Hour)
	e.crm.calls = nil
	e.crm.pages = [][]leads.Contact{
		{contact("c-1", "+971501000001", "Follow Up")},
		{contact("c-3", "+971501000003", "Done-Deal")},
	}
	e.crm.failAtPage = 2
	failed := e.runImport(t, "full")
	assert.Equal(t, runs.StateFailed, failed.State)
	assert.Contains(t, *failed.Error, "request limit")
	list, err := e.leads.List(t.Context(), coretest.Viewer, leads.ListFilter{})
	require.NoError(t, err)
	assert.Len(t, list, 3, "nothing is removed by a scan that didn't finish")
	var progress leads.ImportProgress
	require.NoError(t, json.Unmarshal(failed.Progress, &progress))
	assert.Equal(t, "cursor-1", progress.Cursor)
	assert.Equal(t, 1, progress.Pages)

	// Retry resumes at the cursor and, being complete now, removes c-2.
	e.crm.calls = nil
	retried := e.runImport(t, "full")
	assert.Equal(t, runs.StateSucceeded, retried.State)
	assert.Equal(t, int64(1), retried.Attempt)
	assert.Equal(t, []string{"cursor-1"}, e.crm.calls, "resumed from the recorded cursor")
	list, err = e.leads.List(t.Context(), coretest.Viewer, leads.ListFilter{})
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "c-1", list[0].ExternalID)
	assert.Equal(t, "c-3", list[1].ExternalID)
	var removed sql.NullTime
	require.NoError(t, e.db.Read.QueryRowContext(t.Context(), "SELECT removed_at FROM leads WHERE external_id = 'c-2'").Scan(&removed))
	assert.True(t, removed.Valid)

	// c-1 was seen twice with identical content: one evidence row.
	var n int
	require.NoError(t, e.db.Read.QueryRowContext(t.Context(), "SELECT count(*) FROM evidence WHERE source='gohighlevel' AND kind='contact' AND source_ref='c-1'").Scan(&n))
	assert.Equal(t, 1, n)

	// An incremental import never removes and passes since through.
	since := e.clock.Now().Add(-2 * time.Hour)
	e.crm.pages = [][]leads.Contact{{contact("c-1", "+971501000001", "Follow Up")}}
	run, _, err := e.leads.StartImport(t.Context(), coretest.Marketer, e.runs, e.account, "incremental", &since)
	require.NoError(t, err)
	h, err := e.runs.Begin(t.Context(), run.RowID(), "test")
	require.NoError(t, err)
	completion, err := e.leads.Execute(t.Context(), h)
	require.NoError(t, err)
	require.NoError(t, h.Finish(t.Context(), completion))
	assert.Equal(t, since.UTC(), e.crm.lastSince.UTC())
	list, err = e.leads.List(t.Context(), coretest.Viewer, leads.ListFilter{})
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

// Catches: a click id graded as low confidence, a UTM graded as high, and
// a repeated attribution record stored twice.
func TestAttributionMatchLevels(t *testing.T) {
	e := setup(t)
	_, err := e.leads.PutFunnel(t.Context(), coretest.Admin, mapping())
	require.NoError(t, err)
	c := contact("c-1", "+971501234567", "Follow Up")
	c.Attributions = []leads.Attribution{
		{IsFirst: true, UTMSource: "Meta", UTMCampaign: "wadi-launch", FBCLID: "fb.1.abc"},
		{UTMSource: "google", UTMMedium: "cpc", UTMCampaign: "wadi-search"},
		{Referrer: "https://instagram.com"},
	}
	_, err = e.leads.Ingest(t.Context(), coretest.Marketer, e.account, c)
	require.NoError(t, err)
	_, err = e.leads.Ingest(t.Context(), coretest.Marketer, e.account, c)
	require.NoError(t, err)

	rows, err := e.db.Reader().ListLeadAttributions(t.Context(), 1)
	require.NoError(t, err)
	require.Len(t, rows, 3, "repeated records are not duplicated")
	assert.Equal(t, "click_id", rows[0].MatchLevel)
	assert.Equal(t, "high", rows[0].MatchConfidence)
	assert.Equal(t, "meta", rows[0].MatchedPlatform.String)
	assert.Equal(t, "utm", rows[1].MatchLevel)
	assert.Equal(t, "low", rows[1].MatchConfidence)
	assert.Equal(t, "wadi-search", rows[1].MatchedCampaignRef.String)
	assert.Equal(t, "none", rows[2].MatchLevel)
	cov, err := e.leads.Coverage(t.Context(), coretest.Viewer, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), cov.MatchLevels["click_id"])
}

// Catches: a viewer saving the funnel, an unknown stage being stored, and a
// non-GHL account being imported as leads.
func TestFunnelValidationAndPermissions(t *testing.T) {
	e := setup(t)
	_, err := e.leads.PutFunnel(t.Context(), coretest.Marketer, mapping())
	assert.ErrorIs(t, err, core.ErrForbidden)
	bad := mapping()
	bad.Statuses["x"] = leads.Stage("hotish")
	_, err = e.leads.PutFunnel(t.Context(), coretest.Admin, bad)
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindInvalid, ce.Kind)
	bad = mapping()
	bad.StatusField = leads.FieldLocation{Kind: "opportunity_stage", Ref: "x"}
	_, err = e.leads.PutFunnel(t.Context(), coretest.Admin, bad)
	require.ErrorAs(t, err, &ce)
	_, _, err = e.leads.StartImport(t.Context(), coretest.Viewer, e.runs, e.account, "full", nil)
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, _, err = e.leads.StartImport(t.Context(), coretest.Marketer, e.runs, 999, "full", nil)
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindNotFound, ce.Kind)
	_, _, err = e.leads.StartImport(t.Context(), coretest.Marketer, e.runs, e.account, "sometimes", nil)
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindInvalid, ce.Kind)

	unconfigured := leads.New(e.db, nil, nil, evidence.New(e.db, e.clock), leads.Config{Clock: e.clock})
	run, _, err := unconfigured.StartImport(t.Context(), coretest.Marketer, e.runs, e.account, "full", nil)
	require.NoError(t, err)
	h, err := e.runs.Begin(t.Context(), run.RowID(), "test")
	require.NoError(t, err)
	_, err = unconfigured.Execute(t.Context(), h)
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, core.KindConflict, ce.Kind)
	assert.False(t, errors.Is(err, core.ErrForbidden))
}
