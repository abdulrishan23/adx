package authentik_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/providers/authentik"
)

type call struct {
	Method, Path, Query, Auth string
	Body                      map[string]any
}

// fakeAuthentik records each request and answers with the next reply.
func fakeAuthentik(t *testing.T, replies ...string) (*authentik.Directory, *[]call) {
	t.Helper()
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := call{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Auth: r.Header.Get("Authorization")}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			require.NoError(t, json.Unmarshal(b, &c.Body))
		}
		calls = append(calls, c)
		reply := replies[len(calls)-1]
		if reply == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if reply == "403" {
			http.Error(w, `{"detail":"You do not have permission"}`, http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return authentik.NewDirectory(authentik.DirectoryConfig{
		BaseURL:      srv.URL,
		Token:        "tok",
		GroupID:      "group-uuid",
		EmailStageID: "stage-uuid",
	}), &calls
}

// Catches: requests that don't match authentik's API, so provisioning fails
// against the real server while the core tests with a fake still pass.
func TestDirectoryRequests(t *testing.T) {
	d, calls := fakeAuthentik(t,
		`{"results":[{"pk":13,"uuid":"u-13","username":"sara","email":"Sara@Reportage.ae","is_active":false,"path":"users"},`+
			`{"pk":15,"uuid":"u-15","username":"sarah","email":"sarah@reportage.ae","is_active":true,"path":"users"}]}`,
		`{"pk":14,"uuid":"u-14","username":"omar@reportage.ae","email":"omar@reportage.ae","is_active":true,"path":"users/adx"}`,
		"", "", "",
	)
	ctx := t.Context()

	found, err := d.FindByEmail(ctx, "sara@reportage.ae")
	require.NoError(t, err)
	assert.Equal(t, []users.Account{{PK: 13, UUID: "u-13", Username: "sara", Email: "Sara@Reportage.ae", Active: false}}, found,
		"case is ignored, and search's looser matches are dropped")
	created, err := d.Create(ctx, "omar@reportage.ae")
	require.NoError(t, err)
	assert.Equal(t, users.Account{PK: 14, UUID: "u-14", Username: "omar@reportage.ae", Email: "omar@reportage.ae", Active: true, CreatedByADX: true}, created)
	require.NoError(t, d.AddToGroup(ctx, 14))
	require.NoError(t, d.RemoveFromGroup(ctx, 13))
	require.NoError(t, d.SendSetPasswordEmail(ctx, 14))

	assert.Equal(t, []call{
		{Method: "GET", Path: "/api/v3/core/users/", Query: "include_groups=false&include_roles=false&page_size=100&search=sara%40reportage.ae", Auth: "Bearer tok"},
		{Method: "POST", Path: "/api/v3/core/users/", Auth: "Bearer tok",
			Body: map[string]any{"username": "omar@reportage.ae", "email": "omar@reportage.ae", "name": "omar@reportage.ae", "is_active": true, "path": "users/adx"}},
		{Method: "POST", Path: "/api/v3/core/groups/group-uuid/add_user/", Auth: "Bearer tok", Body: map[string]any{"pk": float64(14)}},
		{Method: "POST", Path: "/api/v3/core/groups/group-uuid/remove_user/", Auth: "Bearer tok", Body: map[string]any{"pk": float64(13)}},
		{Method: "POST", Path: "/api/v3/core/users/14/recovery_email/", Auth: "Bearer tok",
			Body: map[string]any{"email_stage": "stage-uuid", "token_duration": "days=3"}},
	}, *calls)
}

// Catches: an authentik refusal being treated as success, which would save
// a person in ADX whose authentik account was never set up.
func TestDirectoryErrorsIncludeStatus(t *testing.T) {
	d, _ := fakeAuthentik(t, "403")
	err := d.AddToGroup(t.Context(), 14)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	assert.NotContains(t, err.Error(), "tok", "the token must never reach an error message")
}
