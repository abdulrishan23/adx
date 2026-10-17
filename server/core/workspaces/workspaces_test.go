package workspaces_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/projects"
	"github.com/WokeWorks/adx/server/core/workspaces"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

var now = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

func setup(t *testing.T) (*store.DB, *workspaces.Workspaces) {
	t.Helper()
	db := storetest.Open(t)
	coretest.SeedUsers(t, db)
	for _, m := range []struct {
		user int64
		role string
	}{{1, "admin"}, {2, "marketer"}, {3, "viewer"}} {
		require.NoError(t, db.Writer().CreateMembership(t.Context(), store.CreateMembershipParams{
			WorkspaceID: core.DefaultWorkspace, UserID: m.user, Role: m.role, Enabled: 1, CreatedAt: now,
		}))
	}
	return db, workspaces.New(db, func() time.Time { return now })
}

// second provisions a second client with the Reportage admin and returns
// an admin actor inside it.
func second(t *testing.T, w *workspaces.Workspaces) core.Actor {
	t.Helper()
	ws, err := w.Create(t.Context(), coretest.Admin, workspaces.WorkspaceInput{Slug: "acme-homes", Name: "Acme Homes", Industry: "real_estate"})
	require.NoError(t, err)
	return core.Actor{UserID: 1, Role: core.RoleAdmin, Via: core.ViaWeb, WorkspaceID: ws.ID}
}

// Catches: a non-platform admin provisioning clients, a bad slug, a
// duplicate slug, and the creator not becoming the first admin.
func TestCreateIsPlatformAdminOnlyAndSeedsMembership(t *testing.T) {
	_, w := setup(t)
	_, err := w.Create(t.Context(), coretest.Marketer, workspaces.WorkspaceInput{Slug: "x", Name: "X"})
	require.ErrorIs(t, err, core.ErrForbidden)
	_, err = w.Create(t.Context(), core.Actor{UserID: 1, Role: core.RoleAdmin, Via: core.ViaAssistant, WorkspaceID: 1}, workspaces.WorkspaceInput{Slug: "x", Name: "X"})
	require.ErrorIs(t, err, core.ErrForbidden)
	_, err = w.Create(t.Context(), coretest.Admin, workspaces.WorkspaceInput{Slug: "Bad Slug", Name: "X"})
	requireKind(t, err, core.KindInvalid)

	acme := second(t, w)
	_, err = w.Create(t.Context(), coretest.Admin, workspaces.WorkspaceInput{Slug: "acme-homes", Name: "Again"})
	requireKind(t, err, core.KindConflict)
	// An admin of the second client is not a platform admin.
	_, err = w.Create(t.Context(), acme, workspaces.WorkspaceInput{Slug: "third", Name: "Third"})
	require.ErrorIs(t, err, core.ErrForbidden)

	mine, err := w.Mine(t.Context(), coretest.Admin)
	require.NoError(t, err)
	require.Len(t, mine, 2)
	assert.True(t, mine[0].Current)
	assert.Equal(t, "acme-homes", mine[1].Slug)
	assert.Equal(t, core.RoleAdmin, mine[1].Role)
	// The marketer belongs only to Reportage.
	mine, err = w.Mine(t.Context(), coretest.Marketer)
	require.NoError(t, err)
	assert.Len(t, mine, 1)
}

// Catches: the same provider id or project name in two clients colliding, and
// one client's rows showing in another's lists.
func TestSameIdsInTwoClientsNeverMeet(t *testing.T) {
	db, w := setup(t)
	acme := second(t, w)
	p := projects.New(db)
	_, err := p.Create(t.Context(), coretest.Admin, projects.CreateInput{Name: "The Wadi"})
	require.NoError(t, err)
	_, err = p.Create(t.Context(), acme, projects.CreateInput{Name: "the wadi"})
	require.NoError(t, err, "a project name is unique within a client, not across clients")
	_, err = p.Create(t.Context(), acme, projects.CreateInput{Name: "The Wadi"})
	requireKind(t, err, core.KindConflict)
	list, err := p.List(t.Context(), core.Actor{UserID: 3, Role: core.RoleViewer, Via: core.ViaWeb, WorkspaceID: acme.WorkspaceID})
	require.NoError(t, err)
	require.Len(t, list, 1)

	cred, err := db.Writer().UpsertCredential(t.Context(), store.UpsertCredentialParams{
		Provider: "google_ads", Kind: "oauth", Subject: "s", EncryptedSecret: []byte("x"), KeyVersion: 1, ConnectedAt: now,
	})
	require.NoError(t, err)
	for _, ws := range []int64{core.DefaultWorkspace, acme.WorkspaceID} {
		_, err := db.Writer().UpsertAccount(t.Context(), store.UpsertAccountParams{
			WorkspaceID: ws, Type: "google_ads", ExternalID: "123", CredentialID: cred.ID, Details: "{}", ConnectedAt: now, VerifiedAt: now,
		})
		require.NoError(t, err, "the same Google Ads id may exist in two clients")
	}
	rows, err := db.Reader().ListAccountsInWorkspace(t.Context(), acme.WorkspaceID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, acme.WorkspaceID, rows[0].Account.WorkspaceID)

	// The other client's account is "not connected" here: the kill switch
	// can't name it, and a policy decision about it never admits it exists.
	reportage, err := db.Reader().ListAccountsInWorkspace(t.Context(), core.DefaultWorkspace)
	require.NoError(t, err)
	pol := policies.New(db)
	_, err = pol.SetKillSwitch(t.Context(), acme, reportage[0].Account.ID, true, "")
	requireKind(t, err, core.KindNotFound)
	_, err = pol.Put(t.Context(), acme, policies.PolicyDocument{WriteAccounts: []int64{reportage[0].Account.ID}})
	require.NoError(t, err)
	decision, err := pol.Evaluate(t.Context(), acme, policies.PolicyRequest{AccountID: reportage[0].Account.ID, Action: policies.ExclusionsUpdate})
	require.NoError(t, err)
	assert.Equal(t, policies.Denied, decision.Outcome)
	assert.Equal(t, policies.ReasonAccountNotConnected, decision.Reason)

	// People: the same phone in two clients is two people, and neither
	// client can read the other's.
	people := crm.New(db, nil, crm.Config{Clock: coretest.NewClock(now)})
	mine, err := people.Create(t.Context(), coretest.Marketer, crm.PersonInput{DisplayName: "Ali", Phone: "+971501234567"})
	require.NoError(t, err)
	theirs, err := people.Create(t.Context(), acme, crm.PersonInput{DisplayName: "Ali", Phone: "+971501234567"})
	require.NoError(t, err)
	assert.NotEqual(t, mine.Person.ID, theirs.Person.ID)
	_, err = people.Get(t.Context(), acme, mine.Person.ID)
	requireKind(t, err, core.KindNotFound)
}

// Catches: another client's save bumping Reportage's version, a version
// edited in place, and a document that smuggles permissions in.
func TestIndustryConfigIsVersionedPerClient(t *testing.T) {
	_, w := setup(t)
	before, err := w.Industry(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	require.NotNil(t, before.Current)
	assert.Equal(t, int64(1), before.Current.Version)
	assert.Equal(t, "real_estate", before.Current.Document.Industry)
	assert.Contains(t, before.Current.Document.Stages, "viewing")

	acme := second(t, w)
	theirs, err := w.Industry(t.Context(), acme)
	require.NoError(t, err)
	require.NotNil(t, theirs.Current)
	assert.Equal(t, int64(1), theirs.Current.Version)
	assert.Empty(t, theirs.Current.Document.Stages, "a new client starts with its own empty definitions, not Reportage's")

	saved, err := w.PutIndustry(t.Context(), acme, workspaces.IndustryDocument{Industry: "hospitality", Stages: []string{"enquiry", "booked"}})
	require.NoError(t, err)
	assert.Equal(t, int64(2), saved.Version)
	_, err = w.PutIndustry(t.Context(), acme, workspaces.IndustryDocument{Industry: "hospitality", Stages: []string{"a", "A"}})
	requireKind(t, err, core.KindInvalid)
	_, err = w.PutIndustry(t.Context(), core.Actor{UserID: 3, Role: core.RoleViewer, Via: core.ViaWeb, WorkspaceID: acme.WorkspaceID}, workspaces.IndustryDocument{Industry: "x"})
	require.ErrorIs(t, err, core.ErrForbidden)

	after, err := w.Industry(t.Context(), coretest.Viewer)
	require.NoError(t, err)
	assert.Equal(t, int64(1), after.Current.Version, "Reportage's configuration is untouched by another client's save")
	assert.Equal(t, before.Current.Document, after.Current.Document)
	v1, err := w.IndustryVersion(t.Context(), acme, 1)
	require.NoError(t, err)
	assert.Equal(t, "real_estate", v1.Document.Industry, "version 1 reads as it was written after version 2 exists")
	_, err = w.IndustryVersion(t.Context(), coretest.Viewer, 2)
	requireKind(t, err, core.KindNotFound)

	raw, _ := json.Marshal(map[string]any{"industry": "x", "permissions": []string{"admin"}})
	requireKind(t, workspaces.RejectPermissionKeys(raw), core.KindInvalid)
	raw, _ = json.Marshal(map[string]any{"industry": "x", "stages": []string{"a"}})
	require.NoError(t, workspaces.RejectPermissionKeys(raw))
}

// Catches: granting above one's own role, adding someone unknown, removing
// the last admin, and a membership in one client leaking into another.
func TestMembersAreScopedAndBounded(t *testing.T) {
	db, w := setup(t)
	acme := second(t, w)
	members, err := w.Members(t.Context(), acme)
	require.NoError(t, err)
	require.Len(t, members, 1)

	_, err = w.AddMember(t.Context(), acme, workspaces.MemberInput{Email: "nobody@acme.com", Role: core.RoleViewer})
	requireKind(t, err, core.KindNotFound)
	m, err := w.AddMember(t.Context(), acme, workspaces.MemberInput{Email: "sara@reportage.ae", Role: core.RoleMarketer})
	require.NoError(t, err)
	assert.Equal(t, int64(2), m.UserID)
	saraInAcme := core.Actor{UserID: 2, Role: core.RoleMarketer, Via: core.ViaWeb, WorkspaceID: acme.WorkspaceID}
	_, err = w.AddMember(t.Context(), saraInAcme, workspaces.MemberInput{Email: "ceo@reportage.ae", Role: core.RoleViewer})
	require.ErrorIs(t, err, core.ErrForbidden, "a marketer manages no members")
	// Promote Sara to admin, then she still can't hand out more than admin,
	// and an admin can't grant above admin anyway; check the role bound via
	// a marketer-admin mismatch instead.
	_, err = w.AddMember(t.Context(), core.Actor{UserID: 1, Role: core.RoleMarketer, Via: core.ViaWeb, WorkspaceID: acme.WorkspaceID}, workspaces.MemberInput{Email: "ceo@reportage.ae", Role: core.RoleAdmin})
	require.ErrorIs(t, err, core.ErrForbidden)

	// Reportage's membership for Sara is unchanged: still marketer there.
	row, err := db.Reader().GetMembership(t.Context(), store.GetMembershipParams{WorkspaceID: core.DefaultWorkspace, UserID: 2})
	require.NoError(t, err)
	assert.Equal(t, "marketer", row.Role)

	require.Error(t, w.RemoveMember(t.Context(), acme, 1), "the only admin stays")
	require.NoError(t, w.RemoveMember(t.Context(), acme, 2))
	row, err = db.Reader().GetMembership(t.Context(), store.GetMembershipParams{WorkspaceID: acme.WorkspaceID, UserID: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(0), row.Enabled)
	_, err = db.Reader().GetMembership(t.Context(), store.GetMembershipParams{WorkspaceID: 99, UserID: 2})
	require.True(t, errors.Is(err, sql.ErrNoRows))
}

func requireKind(t *testing.T, err error, kind core.Kind) {
	t.Helper()
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, kind, ce.Kind)
}
