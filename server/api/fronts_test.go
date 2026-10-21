package api_test

import (
	"encoding/json"
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
	"github.com/WokeWorks/adx/server/core/alerts"
	"github.com/WokeWorks/adx/server/core/coretest"
	"github.com/WokeWorks/adx/server/core/evidence"
	"github.com/WokeWorks/adx/server/core/fronts"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/core/users/userstest"
	"github.com/WokeWorks/adx/server/store/storetest"
)

// Catches: the desk and the inbox not reaching the wire in the shape the
// screens read (null figure, counts, order), a bad filter answering 500, a
// viewer snoozing an alert, a snooze body with a typo being accepted, and a
// dismissed alert still on the inbox.
func TestFrontsRoutes(t *testing.T) {
	db := storetest.Open(t)
	clock := coretest.NewClock(time.Date(2026, 10, 17, 5, 0, 0, 0, time.UTC))
	authn := userstest.NewAuthentik()
	people := users.New(db, authn)
	oidc := &apiOIDC{}
	id := identity.New(db, people, oidc, identity.Config{AllowedGroup: "adx-users"})
	rep := reporting.New(db, nil, nil, evidence.New(db, clock), reporting.Config{Clock: clock})
	al := alerts.New(db, rep, alerts.Config{Clock: clock})
	fr := fronts.New(fronts.Config{Clock: clock, Alerts: al, Reporting: rep})
	e := &authEnv{db: db, users: people, authn: authn, oidc: oidc}
	e.srv = httptest.NewServer(api.NewRouter(api.Deps{Log: slog.New(slog.DiscardHandler), Identity: id, Alerts: al, Fronts: fr}))
	t.Cleanup(e.srv.Close)
	e.addAndPoint(t, "ceo@reportage.ae", core.RoleViewer)
	viewer := e.signIn(t)
	e.addAndPoint(t, "sara@reportage.ae", core.RoleMarketer)
	marketer := e.signIn(t)
	do := func(method string, cookie *http.Cookie, path, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { res.Body.Close() })
		return res
	}
	get := func(cookie *http.Cookie, path string) *http.Response { return do(http.MethodGet, cookie, path, "") }
	post := func(cookie *http.Cookie, path, body string) *http.Response {
		return do(http.MethodPost, cookie, path, body)
	}

	ctx := t.Context()
	admin := core.Actor{UserID: 1, Role: core.RoleAdmin, Via: core.ViaWeb, WorkspaceID: 1}
	hours := 24
	_, err := al.PutConfig(ctx, admin, alerts.AlertDocument{StaleIngestion: alerts.StaleIngestionConfig{MaxAgeHours: &hours}})
	require.NoError(t, err)
	fired, err := al.Raise(ctx, admin, "lead_hot", "person:7", "person 7", "info", "A person's combined score crossed the hot threshold.", map[string]any{})
	require.NoError(t, err)
	require.True(t, fired)

	res := get(viewer, "/api/fronts")
	require.Equal(t, http.StatusOK, res.StatusCode)
	var desk struct {
		Summary   string            `json:"summary"`
		Cards     []json.RawMessage `json:"cards"`
		Working   []any             `json:"working"`
		Templates []any             `json:"templates"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&desk))
	assert.Equal(t, "1 thing needs your attention.", desk.Summary)
	require.Len(t, desk.Cards, 2, "the hot lead, then the spend figure")
	assert.Contains(t, string(desk.Cards[0]), `"id":"alert-1"`)
	assert.Contains(t, string(desk.Cards[0]), `"alertId":1`)
	assert.Contains(t, string(desk.Cards[0]), `"door":"convert"`)
	assert.Contains(t, string(desk.Cards[1]), `"id":"spend"`)
	assert.Contains(t, string(desk.Cards[1]), `"figure":{"value":null,"count":null`, "a missing figure travels as null")
	assert.NotNil(t, desk.Working)
	assert.Len(t, desk.Templates, 18)

	res = get(viewer, "/api/inbox?show=updates&door=perform")
	require.Equal(t, http.StatusOK, res.StatusCode)
	var inbox struct {
		Counts fronts.Counts     `json:"counts"`
		Cards  []json.RawMessage `json:"cards"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&inbox))
	assert.Equal(t, fronts.Counts{All: 2, NeedsYou: 1, Updates: 1}, inbox.Counts)
	require.Len(t, inbox.Cards, 1)
	assert.Contains(t, string(inbox.Cards[0]), `"id":"spend"`)
	res = get(viewer, "/api/inbox?show=mine")
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)

	res = post(viewer, "/api/alerts/1/snooze", `{"days":7}`)
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
	res = post(marketer, "/api/alerts/1/snooze", `{"day":7}`)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode, "a typo in the body is refused, not ignored")
	res = post(marketer, "/api/alerts/1/snooze", `{"days":7}`)
	require.Equal(t, http.StatusOK, res.StatusCode)
	var snoozed alerts.Alert
	require.NoError(t, json.NewDecoder(res.Body).Decode(&snoozed))
	assert.NotNil(t, snoozed.SnoozedUntil)
	res = get(viewer, "/api/inbox")
	require.NoError(t, json.NewDecoder(res.Body).Decode(&inbox))
	assert.Equal(t, fronts.Counts{All: 1, NeedsYou: 0, Updates: 1}, inbox.Counts)

	res = post(marketer, "/api/alerts/1/dismiss", `{"reason":""}`)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	res = post(marketer, "/api/alerts/1/dismiss", `{"reason":"Expected"}`)
	require.Equal(t, http.StatusOK, res.StatusCode)
	res = post(marketer, "/api/alerts/1/dismiss", `{"reason":"Expected"}`)
	assert.Equal(t, http.StatusNotFound, res.StatusCode)
	res = post(marketer, "/api/alerts/x/dismiss", `{"reason":"Expected"}`)
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	clock.Advance(30 * 24 * time.Hour)
	res = get(viewer, "/api/inbox?show=needs")
	require.NoError(t, json.NewDecoder(res.Body).Decode(&inbox))
	assert.Empty(t, inbox.Cards, "dismissed: it does not come back when the snooze would have ended")
}
