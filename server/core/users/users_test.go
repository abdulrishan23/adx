package users_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

var (
	system = core.System(core.ViaCLI)
	web    = core.Actor{Via: core.ViaWeb}
)

type env struct {
	u   *users.Users
	db  *store.DB
	dir *userstest.Directory
}

func setup(t *testing.T) env {
	t.Helper()
	db := storetest.Open(t)
	dir := userstest.NewDirectory()
	return env{u: users.New(db, dir), db: db, dir: dir}
}

func (e env) add(t *testing.T, email string, role core.Role) users.User {
	t.Helper()
	user, err := e.u.Add(t.Context(), system, users.AddInput{Email: email, Role: role})
	require.NoError(t, err)
	return user
}

// signIn signs in as the authentik account ADX made for email.
func (e env) signIn(t *testing.T, email string) (users.User, error) {
	t.Helper()
	acct := e.dir.ByEmail(email)
	require.NotNil(t, acct)
	return e.u.SignIn(t.Context(), web, users.Identity{Subject: acct.UUID, Email: email, InAllowedGroup: true})
}

func (e env) actions(t *testing.T) []string {
	t.Helper()
	rows, err := e.db.Reader().ListAuditEntries(t.Context())
	require.NoError(t, err)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Action)
	}
	return out
}

func actorFor(user users.User) core.Actor {
	return core.Actor{UserID: user.ID, Role: user.Role, Via: core.ViaWeb}
}

func requireKind(t *testing.T, err error, kind core.Kind) {
	t.Helper()
	var ce *core.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, kind, ce.Kind)
}

// Catches: an added person without an authentik account, outside the ADX
// group, never sent the set-password email, or shown as able to sign in
// before they have.
func TestAddCreatesTheAuthentikAccount(t *testing.T) {
	e := setup(t)
	admin := e.add(t, "cmo@reportage.ae", core.RoleAdmin)

	sara, err := e.u.Add(t.Context(), actorFor(admin), users.AddInput{Email: "  Sara@Reportage.ae ", Role: core.RoleMarketer})
	require.NoError(t, err)
	assert.Equal(t, "sara@reportage.ae", sara.Email)
	assert.Equal(t, users.StatusInvited, sara.Status)

	acct := e.dir.ByEmail("sara@reportage.ae")
	require.NotNil(t, acct)
	assert.True(t, acct.InGroup)
	assert.Equal(t, 1, acct.Invites)

	rows, err := e.db.Reader().ListAuditEntries(t.Context())
	require.NoError(t, err)
	require.Len(t, rows, 4)
	assert.Equal(t, "user.added", rows[2].Action)
	assert.Equal(t, admin.ID, rows[2].ActorUserID.Int64)
	assert.Equal(t, sara.ID, rows[2].TargetUserID.Int64)
	assert.JSONEq(t, `{"email":"sara@reportage.ae","role":"marketer"}`, rows[2].Detail)
	assert.Equal(t, "user.password_link_sent", rows[3].Action)
}

// Catches: anyone who got an authentik account with a person's email before
// they were added taking their place, through the admin page.
func TestAddRefusesAccountsADXDidntCreate(t *testing.T) {
	e := setup(t)
	e.dir.Existing("sara@reportage.ae", true)

	_, err := e.u.Add(t.Context(), system, users.AddInput{Email: "sara@reportage.ae", Role: core.RoleMarketer})
	requireKind(t, err, core.KindConflict)
	assert.False(t, e.dir.ByEmail("sara@reportage.ae").InGroup)
	assert.Zero(t, e.dir.Created, "no second account is made either")
}

// Catches: the CLI being unable to add WokeWorks people who already have an
// authentik account, or making them a second one.
func TestAddLinksAnExistingAccountByUUID(t *testing.T) {
	e := setup(t)
	uuid := e.dir.Existing("Twaha@WokeWorks.ai", true)

	found, err := e.u.FindAccounts(t.Context(), system, "twaha@wokeworks.ai")
	require.NoError(t, err)
	require.Len(t, found, 1, "the lookup ignores case")
	assert.Equal(t, uuid, found[0].UUID)

	added, err := e.u.Add(t.Context(), system, users.AddInput{Email: "twaha@wokeworks.ai", Role: core.RoleAdmin, LinkUUID: uuid})
	require.NoError(t, err)
	assert.Equal(t, uuid, added.AuthentikUUID)
	assert.Zero(t, e.dir.Created)
	assert.True(t, e.dir.ByEmail("twaha@wokeworks.ai").InGroup)

	me, err := e.u.SignIn(t.Context(), web, users.Identity{Subject: uuid, InAllowedGroup: true})
	require.NoError(t, err)
	assert.Equal(t, "twaha@wokeworks.ai", me.Email)
}

// Catches: someone who changed their authentik email to B's address being
// linked as B, because the CLI trusted the email instead of the account
// it checked.
func TestAddRefusesAUUIDThatDoesntMatchTheEmail(t *testing.T) {
	e := setup(t)
	checked := e.dir.Existing("omar@reportage.ae", true)
	e.dir.Existing("sara@reportage.ae", true)

	_, err := e.u.Add(t.Context(), system, users.AddInput{Email: "sara@reportage.ae", Role: core.RoleMarketer, LinkUUID: checked})
	requireKind(t, err, core.KindConflict)
	assert.False(t, e.dir.ByEmail("sara@reportage.ae").InGroup)
	assert.False(t, e.dir.ByEmail("omar@reportage.ae").InGroup)
}

// Catches: a disabled person put back in adx-users because their authentik
// email was changed to a new hire's, and a vague error about it.
func TestAddRefusesAnAccountThatBelongsToSomeoneInADX(t *testing.T) {
	e := setup(t)
	omar := e.add(t, "omar@reportage.ae", core.RoleMarketer)
	_, err := e.u.Disable(t.Context(), system, omar.ID)
	require.NoError(t, err)
	acct := e.dir.ByEmail("omar@reportage.ae")
	acct.Email = "newhire@reportage.ae"

	_, err = e.u.Add(t.Context(), system, users.AddInput{Email: "newhire@reportage.ae", Role: core.RoleViewer})
	requireKind(t, err, core.KindConflict)
	assert.ErrorContains(t, err, "omar@reportage.ae", "the error names who the account belongs to")
	assert.False(t, acct.InGroup, "Omar isn't put back in the group")
}

// Catches: the CLI creating a second account when the checked account's
// email changed after find-account.
func TestAddWithALinkUUIDNeverCreates(t *testing.T) {
	e := setup(t)

	_, err := e.u.Add(t.Context(), system, users.AddInput{Email: "sara@reportage.ae", Role: core.RoleViewer, LinkUUID: "uuid-9"})
	requireKind(t, err, core.KindConflict)
	assert.Zero(t, e.dir.Created)
}

// Catches: ADX switching back on an account an authentik admin turned off.
func TestAddRefusesADeactivatedAccount(t *testing.T) {
	e := setup(t)
	uuid := e.dir.Existing("leaver@reportage.ae", false)

	_, err := e.u.Add(t.Context(), system, users.AddInput{Email: "leaver@reportage.ae", Role: core.RoleViewer, LinkUUID: uuid})
	requireKind(t, err, core.KindConflict)
	acct := e.dir.ByEmail("leaver@reportage.ae")
	assert.False(t, acct.Active)
	assert.False(t, acct.InGroup)
}

// Catches: linking the wrong account when authentik has two with the same
// email.
func TestAddRefusesAnAmbiguousEmail(t *testing.T) {
	e := setup(t)
	e.dir.Existing("sara@reportage.ae", true)
	e.dir.Existing("sara@reportage.ae", true)

	_, err := e.u.Add(t.Context(), system, users.AddInput{Email: "sara@reportage.ae", Role: core.RoleViewer, LinkUUID: "uuid-1"})
	requireKind(t, err, core.KindConflict)
	list, err := e.u.List(t.Context(), system)
	require.NoError(t, err)
	assert.Empty(t, list)
}

// Catches: half-added people when authentik fails, and duplicate authentik
// accounts when the admin retries.
func TestAddRetriesSafely(t *testing.T) {
	e := setup(t)
	e.dir.Fail["AddToGroup"] = true

	_, err := e.u.Add(t.Context(), system, users.AddInput{Email: "sara@reportage.ae", Role: core.RoleViewer})
	requireKind(t, err, core.KindUnavailable)
	list, err := e.u.List(t.Context(), system)
	require.NoError(t, err)
	assert.Empty(t, list, "nothing is saved in ADX when authentik fails")

	e.dir.Fail["AddToGroup"] = false
	e.add(t, "sara@reportage.ae", core.RoleViewer)
	assert.Equal(t, 1, e.dir.Created, "the retry links the account ADX made the first time")
}

// Catches: losing someone because the email failed, an active person who
// forgot their password having no way back in, and disabled people getting
// a link.
func TestPasswordLinkCanBeSentAgain(t *testing.T) {
	e := setup(t)
	e.dir.Fail["SendSetPasswordEmail"] = true

	sara, err := e.u.Add(t.Context(), system, users.AddInput{Email: "sara@reportage.ae", Role: core.RoleViewer})
	requireKind(t, err, core.KindUnavailable)
	assert.Equal(t, users.StatusInvited, sara.Status, "they're added even though the email failed")

	e.dir.Fail["SendSetPasswordEmail"] = false
	require.NoError(t, e.u.SendPasswordLink(t.Context(), system, sara.ID))
	assert.Equal(t, 1, e.dir.ByEmail("sara@reportage.ae").Invites)

	_, err = e.signIn(t, "sara@reportage.ae")
	require.NoError(t, err)
	require.NoError(t, e.u.SendPasswordLink(t.Context(), system, sara.ID), "an active person can get one too")
	assert.Equal(t, 2, e.dir.ByEmail("sara@reportage.ae").Invites)

	_, err = e.u.Disable(t.Context(), system, sara.ID)
	require.NoError(t, err)
	requireKind(t, e.u.SendPasswordLink(t.Context(), system, sara.ID), core.KindConflict)
	assert.Equal(t, 2, e.dir.ByEmail("sara@reportage.ae").Invites)
}

func TestAddRejects(t *testing.T) {
	e := setup(t)
	admin := actorFor(e.add(t, "cmo@reportage.ae", core.RoleAdmin))
	marketer := actorFor(e.add(t, "sara@reportage.ae", core.RoleMarketer))
	omar := e.add(t, "omar@reportage.ae", core.RoleMarketer)
	_, err := e.u.Disable(t.Context(), admin, omar.ID)
	require.NoError(t, err)

	tests := []struct {
		name  string
		actor core.Actor
		in    users.AddInput
		kind  core.Kind
	}{
		{"marketer can't add", marketer, users.AddInput{Email: "x@reportage.ae", Role: core.RoleViewer}, core.KindForbidden},
		{"no email", admin, users.AddInput{Email: " ", Role: core.RoleViewer}, core.KindInvalid},
		{"not an email", admin, users.AddInput{Email: "Sara <sara@reportage.ae>", Role: core.RoleViewer}, core.KindInvalid},
		{"longer than authentik allows", admin, users.AddInput{Email: strings.Repeat("a", 140) + "@reportage.ae", Role: core.RoleViewer}, core.KindInvalid},
		{"unknown role", admin, users.AddInput{Email: "x@reportage.ae", Role: "owner"}, core.KindInvalid},
		{"same email in another case", admin, users.AddInput{Email: "SARA@reportage.ae", Role: core.RoleViewer}, core.KindConflict},
		{"disabled person", admin, users.AddInput{Email: "omar@reportage.ae", Role: core.RoleViewer}, core.KindConflict},
	}
	created := e.dir.Created
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.u.Add(t.Context(), tt.actor, tt.in)
			requireKind(t, err, tt.kind)
		})
	}
	assert.Equal(t, created, e.dir.Created, "a rejected add never reaches authentik")
}

// Catches: a stranger, someone with an added person's email but another
// account, someone outside adx-users or a disabled person getting in, and a
// refusal that isn't logged.
func TestSignInRefuses(t *testing.T) {
	e := setup(t)
	e.add(t, "cmo@reportage.ae", core.RoleAdmin)
	sara := e.add(t, "sara@reportage.ae", core.RoleMarketer)
	omar := e.add(t, "omar@reportage.ae", core.RoleMarketer)
	_, err := e.u.Disable(t.Context(), system, omar.ID)
	require.NoError(t, err)

	tests := []struct {
		name string
		id   users.Identity
	}{
		{"unknown account with an added email", users.Identity{Subject: "someone-else", Email: sara.Email, InAllowedGroup: true}},
		{"not in the group", users.Identity{Subject: e.dir.ByEmail(sara.Email).UUID, Email: sara.Email}},
		{"disabled", users.Identity{Subject: e.dir.ByEmail(omar.Email).UUID, Email: omar.Email, InAllowedGroup: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.u.SignIn(t.Context(), web, tt.id)
			assert.ErrorIs(t, err, users.ErrNoAccess)
		})
	}

	rows, err := e.db.Reader().ListAuditEntries(t.Context())
	require.NoError(t, err)
	var reasons []string
	for _, r := range rows {
		if r.Action == "sign_in.refused" {
			reasons = append(reasons, r.Detail)
		}
	}
	assert.Equal(t, []string{
		`{"email":"sara@reportage.ae","reason":"unknown_account"}`,
		`{"email":"sara@reportage.ae","reason":"not_in_group"}`,
		`{"email":"omar@reportage.ae","reason":"disabled"}`,
	}, reasons)
}

// Catches: an invited person staying invited after signing in, and an email
// changed in authentik moving the ADX row admins act on to another address.
func TestSignInActivatesAndKeepsTheAddedEmail(t *testing.T) {
	e := setup(t)
	e.add(t, "cmo@reportage.ae", core.RoleAdmin)
	e.add(t, "sara@reportage.ae", core.RoleMarketer)
	uuid := e.dir.ByEmail("sara@reportage.ae").UUID

	first, err := e.u.SignIn(t.Context(), web, users.Identity{Subject: uuid, Email: "ceo@reportage.ae", Name: "Sara K", InAllowedGroup: true})
	require.NoError(t, err)
	assert.Equal(t, users.StatusActive, first.Status)
	assert.Equal(t, "sara@reportage.ae", first.Email, "the email an admin entered stays")
	require.NotNil(t, first.Name)
	assert.Equal(t, "Sara K", *first.Name)

	again, err := e.u.SignIn(t.Context(), web, users.Identity{Subject: uuid, InAllowedGroup: true})
	require.NoError(t, err)
	assert.Equal(t, "Sara K", *again.Name, "an empty name keeps the old one")

	_, err = e.u.Add(t.Context(), system, users.AddInput{Email: "ceo@reportage.ae", Role: core.RoleViewer})
	require.NoError(t, err, "the CEO can still be added")
}

// Catches: a disable that leaves the person in the ADX group, a failed
// authentik call undoing the ADX disable, ADX switching whole authentik
// accounts off, and an enable that brings back a different role or status.
func TestDisableAndEnable(t *testing.T) {
	e := setup(t)
	admin := actorFor(e.add(t, "cmo@reportage.ae", core.RoleAdmin))
	sara := e.add(t, "sara@reportage.ae", core.RoleMarketer)
	_, err := e.signIn(t, "sara@reportage.ae")
	require.NoError(t, err)
	acct := e.dir.ByEmail("sara@reportage.ae")

	e.dir.Fail["RemoveFromGroup"] = true
	_, err = e.u.Disable(t.Context(), admin, sara.ID)
	requireKind(t, err, core.KindUnavailable)
	_, err = e.signIn(t, "sara@reportage.ae")
	assert.ErrorIs(t, err, users.ErrNoAccess, "ADX blocks her even though authentik failed")
	assert.True(t, acct.InGroup)

	e.dir.Fail["RemoveFromGroup"] = false
	_, err = e.u.Disable(t.Context(), admin, sara.ID)
	require.NoError(t, err)
	assert.False(t, acct.InGroup, "the retry finishes the authentik part")
	assert.True(t, acct.Active, "her authentik account itself is never switched off")

	e.dir.Fail["AddToGroup"] = true
	_, err = e.u.Enable(t.Context(), admin, sara.ID)
	requireKind(t, err, core.KindUnavailable)
	e.dir.Fail["AddToGroup"] = false
	enabled, err := e.u.Enable(t.Context(), admin, sara.ID)
	require.NoError(t, err)
	assert.True(t, acct.InGroup)
	assert.Equal(t, users.StatusActive, enabled.Status, "she had signed in before")
	assert.Equal(t, core.RoleMarketer, enabled.Role)

	var changes []string
	for _, a := range e.actions(t) {
		if a == "user.disabled" || a == "user.enabled" {
			changes = append(changes, a)
		}
	}
	assert.Equal(t, []string{"user.disabled", "user.enabled"}, changes, "the repeated disable logs once")
}

// Catches: the CMO locking everyone out of user management by demoting or
// disabling the only admin.
func TestLastAdminCannotBeRemoved(t *testing.T) {
	e := setup(t)
	cmo := e.add(t, "cmo@reportage.ae", core.RoleAdmin)
	self := actorFor(cmo)

	_, err := e.u.SetRole(t.Context(), self, cmo.ID, core.RoleMarketer)
	requireKind(t, err, core.KindConflict)
	_, err = e.u.Disable(t.Context(), self, cmo.ID)
	requireKind(t, err, core.KindConflict)
	assert.True(t, e.dir.ByEmail("cmo@reportage.ae").InGroup, "a refused disable doesn't reach authentik")

	e.add(t, "tamim@wokeworks.ai", core.RoleAdmin)
	demoted, err := e.u.SetRole(t.Context(), self, cmo.ID, core.RoleMarketer)
	require.NoError(t, err)
	assert.Equal(t, core.RoleMarketer, demoted.Role)
}

// Catches: two admins demoting each other at the same moment and leaving
// ADX with none.
func TestLastAdminGuardHoldsUnderRace(t *testing.T) {
	e := setup(t)
	a := e.add(t, "cmo@reportage.ae", core.RoleAdmin)
	b := e.add(t, "tamim@wokeworks.ai", core.RoleAdmin)

	errs := make(chan error, 2)
	for _, pair := range [][2]users.User{{a, b}, {b, a}} {
		go func() {
			_, err := e.u.SetRole(t.Context(), actorFor(pair[0]), pair[1].ID, core.RoleViewer)
			errs <- err
		}()
	}
	var failed int
	for range 2 {
		if err := <-errs; err != nil {
			requireKind(t, err, core.KindConflict)
			failed++
		}
	}
	assert.Equal(t, 1, failed, "exactly one demotion goes through")
}

// Catches: ADX itself (the CLI or a job) being treated as a person.
func TestMeRefusesTheSystemActor(t *testing.T) {
	e := setup(t)
	_, err := e.u.Me(t.Context(), system)
	assert.ErrorIs(t, err, core.ErrForbidden)
}

// Catches: non-admins managing people and a role change logged without
// from and to.
func TestRoleChanges(t *testing.T) {
	e := setup(t)
	admin := actorFor(e.add(t, "cmo@reportage.ae", core.RoleAdmin))
	sara := e.add(t, "sara@reportage.ae", core.RoleMarketer)

	_, err := e.u.SetRole(t.Context(), actorFor(sara), sara.ID, core.RoleAdmin)
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.u.Disable(t.Context(), actorFor(sara), sara.ID)
	assert.ErrorIs(t, err, core.ErrForbidden)
	_, err = e.u.SetRole(t.Context(), admin, 999, core.RoleViewer)
	requireKind(t, err, core.KindNotFound)
	_, err = e.u.SetRole(t.Context(), admin, sara.ID, "owner")
	requireKind(t, err, core.KindInvalid)

	before := len(e.actions(t))
	_, err = e.u.SetRole(t.Context(), admin, sara.ID, core.RoleViewer)
	require.NoError(t, err)
	_, err = e.u.SetRole(t.Context(), admin, sara.ID, core.RoleViewer)
	require.NoError(t, err)
	assert.Len(t, e.actions(t), before+1, "setting the same role again logs nothing")

	rows, err := e.db.Reader().ListAuditEntries(t.Context())
	require.NoError(t, err)
	assert.JSONEq(t, `{"from":"marketer","to":"viewer"}`, rows[len(rows)-1].Detail)
}

func TestOnlyAdminsListPeople(t *testing.T) {
	e := setup(t)
	e.add(t, "cmo@reportage.ae", core.RoleAdmin)
	sara := e.add(t, "sara@reportage.ae", core.RoleMarketer)

	_, err := e.u.List(t.Context(), actorFor(sara))
	assert.ErrorIs(t, err, core.ErrForbidden)
	me, err := e.u.Me(t.Context(), actorFor(sara))
	require.NoError(t, err)
	assert.Equal(t, "sara@reportage.ae", me.Email)
}

// Catches: a fresh install nobody can sign in to, and the bootstrap email
// taking over again on every restart.
func TestBootstrap(t *testing.T) {
	e := setup(t)
	uuid := e.dir.Existing("twaha@wokeworks.ai", true)

	admin, changed, err := e.u.Bootstrap(t.Context(), system, "Twaha@WokeWorks.ai")
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, uuid, admin.AuthentikUUID, "the caller can log which account was linked")
	_, changed, err = e.u.Bootstrap(t.Context(), system, "someone@else.com")
	require.NoError(t, err)
	assert.False(t, changed)

	list, err := e.u.List(t.Context(), system)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "twaha@wokeworks.ai", list[0].Email)
	assert.Equal(t, core.RoleAdmin, list[0].Role)
	assert.Zero(t, e.dir.Created, "the existing account is linked")
}

// Catches: bootstrap failing when the email was already added with a lower
// role before any admin existed.
func TestBootstrapPromotesAnExistingPerson(t *testing.T) {
	e := setup(t)
	e.add(t, "cmo@reportage.ae", core.RoleMarketer)

	_, changed, err := e.u.Bootstrap(t.Context(), system, "cmo@reportage.ae")
	require.NoError(t, err)
	assert.True(t, changed)
	cmo, err := e.u.FindByEmail(t.Context(), system, "cmo@reportage.ae")
	require.NoError(t, err)
	assert.Equal(t, core.RoleAdmin, cmo.Role)
}
