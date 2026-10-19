package api_test

import (
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/api"
	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/launch"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/store"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: a missing query answering 500 instead of 400, a viewer recording
// a pick, a pick outside the candidates answering 500, and the resolution
// not reaching the wire with its kinds and budget.
func TestLaunchRoutes(t *testing.T) {
	db := storetest.Open(t)
	clock := coretest.NewClock(time.Date(2026, 10, 12, 5, 0, 0, 0, time.UTC))
	authn := userstest.NewAuthentik()
	people := users.New(db, authn)
	oidc := &apiOIDC{}
	id := identity.New(db, people, oidc, identity.Config{AllowedGroup: "adx-users"})
	br := brands.New(db, brands.Config{Clock: clock})
	pl := plans.New(db, br, plans.Config{Clock: clock})
	rep := reporting.New(db, nil, nil, evidence.New(db, clock), reporting.Config{Clock: clock})
	svc := launch.New(db, pl, rep, launch.Config{Clock: clock})
	e := &authEnv{db: db, users: people, authn: authn, oidc: oidc}
	e.srv = httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Identity: id, Launch: svc}))
	t.Cleanup(e.srv.Close)
	e.addAndPoint(t, "ceo@reportage.ae", core.RoleViewer)
	viewer := e.signIn(t)
	e.addAndPoint(t, "sara@reportage.ae", core.RoleMarketer)
	marketer := e.signIn(t)

	ctx := t.Context()
	admin := core.Actor{UserID: 1, Role: core.RoleAdmin, Via: core.ViaWeb, WorkspaceID: 1}
	b, err := br.Create(ctx, admin, brands.BrandInput{Name: "Reportage", Kind: brands.Own})
	require.NoError(t, err)
	project, err := db.Writer().CreateProject(ctx, store.CreateProjectParams{WorkspaceID: 1, Name: "The Wadi", CreatedAt: clock.Now()})
	require.NoError(t, err)
	require.NoError(t, br.AssignProject(ctx, admin, project.ID, b.ID))
	cred, err := db.Writer().UpsertCredential(ctx, store.UpsertCredentialParams{Provider: "meta", Kind: "oauth", Subject: "u", EncryptedSecret: []byte("x"), KeyVersion: 1, Scopes: "x", ConnectedAt: clock.Now()})
	require.NoError(t, err)
	acct, err := db.Writer().UpsertAccount(ctx, store.UpsertAccountParams{WorkspaceID: 1, Type: "meta", ExternalID: "act_42", CredentialID: cred.ID, Details: "{}", ConnectedAt: clock.Now(), VerifiedAt: clock.Now()})
	require.NoError(t, err)
	var ids []int64
	for _, ext := range []string{"m1", "m2"} {
		c, err := db.Writer().UpsertAdCampaign(ctx, store.UpsertAdCampaignParams{WorkspaceID: 1, AccountID: acct.ID, Platform: "meta", ExternalID: ext, Name: "Wadi " + ext, Status: "ACTIVE",
			ChannelType: sql.NullString{String: "OUTCOME_LEADS", Valid: true}, FirstSeenAt: clock.Now(), LastSeenAt: clock.Now()})
		require.NoError(t, err)
		require.NoError(t, db.Writer().SetAdCampaignProject(ctx, store.SetAdCampaignProjectParams{ProjectID: sql.NullInt64{Int64: project.ID, Valid: true}, ID: c.ID}))
		ids = append(ids, c.ID)
	}
	cpl := int64(25000)
	d, err := pl.Create(ctx, admin, plans.PlanInput{BrandID: b.ID, ProjectID: project.ID, Title: "Wadi Oct", Document: plans.PlanDocument{Objective: "Leads", BudgetMinor: 200_000_00, Currency: "AED", PeriodFrom: "2026-10-01", PeriodTo: "2026-10-31",
		Allocations: []plans.Allocation{{Channel: "meta_lead_form", Objective: "leads", BudgetMinor: 200_000_00, Geography: "AE", ExpectedCPLMinor: &cpl}}}})
	require.NoError(t, err)

	res, body := get(t, e.srv, "/api/launch/resolution?plan_id=1", viewer, nil)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, body)
	res, body = get(t, e.srv, "/api/launch/resolution?plan_id=1&version=1", viewer, nil)
	assert.Equal(t, http.StatusConflict, res.StatusCode, body)
	assert.Contains(t, body, "only an accepted version launches")

	_, err = pl.Accept(ctx, admin, d.Plan.ID, 1)
	require.NoError(t, err)
	res, body = get(t, e.srv, "/api/launch/resolution?plan_id=1&version=1", viewer, nil)
	require.Equal(t, http.StatusOK, res.StatusCode, body)
	assert.Contains(t, body, `"kind":"pick"`)
	assert.Contains(t, body, `"daysLeft":20`)
	assert.Contains(t, body, `"today":"2026-10-12"`)

	post := func(cookie *http.Cookie, json string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/api/launch/picks", strings.NewReader(json))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		r, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		b, _ := io.ReadAll(r.Body)
		return r.StatusCode, string(b)
	}
	code, body := post(viewer, `{"planId":1,"planVersion":1,"line":0,"campaignId":1}`)
	assert.Equal(t, http.StatusForbidden, code, body)
	code, body = post(marketer, `{"planId":1,"planVersion":1,"line":0,"campaignId":999}`)
	assert.Equal(t, http.StatusBadRequest, code, body)
	assert.Contains(t, body, "not one of the 2 campaigns")
	code, body = post(marketer, `{"planId":1,"planVersion":1,"line":0,"campaignId":2}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, `"kind":"update"`)
	assert.Contains(t, body, `"remainingMinor":20000000`)
	assert.Contains(t, body, `"dailyMinor":1000000`)
	assert.Contains(t, body, `"pickedBy":2`)
	_ = ids
}
