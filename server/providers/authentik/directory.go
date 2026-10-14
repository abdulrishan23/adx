// Package authentik talks to WokeWorks' authentik through its API, to create
// and switch people's accounts on and off for ADX.
package authentik

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core/users"
)

// inviteLinkLifetime is how long a set-password link works. The email
// stage's own expiry is for "forgot password" and is much shorter.
const inviteLinkLifetime = "days=3"

// adxPath is the authentik folder for accounts ADX creates. People can't
// change their own path, so it marks ADX's accounts. Anyone allowed to
// create authentik users could also create one here, which is why ADX also
// needs the account's email to match and the account to have no ADX row.
const adxPath = "users/adx"

type DirectoryConfig struct {
	// BaseURL is authentik's address, such as https://authentik.wokeworks.ai.
	BaseURL string
	// Token is an API token for a service account that may view and add
	// users, reset passwords, view the email stage, and add people to and
	// remove them from the group. It needs no right to change users.
	Token string
	// GroupID is the UUID of the group whose members may use ADX.
	GroupID string
	// EmailStageID is the UUID of the email stage that sends the
	// set-password link.
	EmailStageID string
}

// Directory implements users.Directory.
type Directory struct {
	cfg  DirectoryConfig
	http *http.Client
}

func NewDirectory(cfg DirectoryConfig) *Directory {
	return &Directory{cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}}
}

type account struct {
	PK       int64  `json:"pk"`
	UUID     string `json:"uuid"`
	Username string `json:"username"`
	Email    string `json:"email"`
	IsActive bool   `json:"is_active"`
	Path     string `json:"path"`
}

func (a account) toUsers() users.Account {
	return users.Account{
		PK:           a.PK,
		UUID:         a.UUID,
		Username:     a.Username,
		Email:        a.Email,
		Active:       a.IsActive,
		CreatedByADX: a.Path == adxPath,
	}
}

// FindByEmail returns the accounts whose email matches, ignoring case.
// authentik's own email filter is exact, so a stored "Sara@Reportage.ae"
// would be missed and a second account made. Search matches loosely and the
// exact check happens here.
func (d *Directory) FindByEmail(ctx context.Context, email string) ([]users.Account, error) {
	var page struct {
		Results []account `json:"results"`
	}
	q := url.Values{"search": {email}, "page_size": {"100"}, "include_groups": {"false"}, "include_roles": {"false"}}
	if err := d.do(ctx, http.MethodGet, "/core/users/?"+q.Encode(), nil, &page); err != nil {
		return nil, err
	}
	out := []users.Account{}
	for _, a := range page.Results {
		if strings.EqualFold(a.Email, email) {
			out = append(out, a.toUsers())
		}
	}
	return out, nil
}

func (d *Directory) Create(ctx context.Context, email string) (users.Account, error) {
	var a account
	body := map[string]any{"username": email, "email": email, "name": email, "is_active": true, "path": adxPath}
	if err := d.do(ctx, http.MethodPost, "/core/users/", body, &a); err != nil {
		return users.Account{}, err
	}
	return a.toUsers(), nil
}

// AddToGroup puts the account in the ADX group. Adding someone already in
// it succeeds.
func (d *Directory) AddToGroup(ctx context.Context, pk int64) error {
	return d.do(ctx, http.MethodPost, "/core/groups/"+url.PathEscape(d.cfg.GroupID)+"/add_user/",
		map[string]any{"pk": pk}, nil)
}

// RemoveFromGroup takes the account out of the ADX group. Removing someone
// who isn't in it succeeds.
func (d *Directory) RemoveFromGroup(ctx context.Context, pk int64) error {
	return d.do(ctx, http.MethodPost, "/core/groups/"+url.PathEscape(d.cfg.GroupID)+"/remove_user/",
		map[string]any{"pk": pk}, nil)
}

func (d *Directory) SendSetPasswordEmail(ctx context.Context, pk int64) error {
	return d.do(ctx, http.MethodPost, "/core/users/"+strconv.FormatInt(pk, 10)+"/recovery_email/",
		map[string]any{"email_stage": d.cfg.EmailStageID, "token_duration": inviteLinkLifetime}, nil)
}

func (d *Directory) do(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.cfg.BaseURL+"/api/v3"+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "adx")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("authentik %s %s: %w", method, path, err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 500))
		return fmt.Errorf("authentik %s %s: %s: %s", method, path, res.Status, bytes.TrimSpace(msg))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}
