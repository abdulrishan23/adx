package connections

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

// Capability names. A provider package records the ones it has probed.
const (
	// CapabilityReadLeads is GoHighLevel contacts, custom fields and
	// pipelines, proved by ReadAccess.
	CapabilityReadLeads = "read.leads"
)

// staleAfter is how long a verification counts as current. An engineering
// default: a connection nobody has checked for a day shows as stale and the
// health job (B1.1) re-checks it.
const staleAfter = 24 * time.Hour

// Health is the one-word state the connections screen shows.
type Health string

const (
	HealthConnected       Health = "connected"
	HealthStale           Health = "stale"
	HealthNeedsPermission Health = "needs_permission"
	HealthFailed          Health = "failed"
)

// Capability is one proven or failed ability of an account.
type Capability struct {
	Name       string     `json:"name"`
	State      string     `json:"state"`
	VerifiedAt *time.Time `json:"verifiedAt"`
	CheckedAt  time.Time  `json:"checkedAt"`
	// LastError is a safe message from the last failed check, or null.
	LastError *string `json:"lastError"`
}

// Account is any connected provider account, whatever the provider, as the
// accounts screen shows it. It never carries the credential.
type Account struct {
	ID           int64        `json:"id"`
	Provider     string       `json:"provider"`
	ExternalID   string       `json:"externalId"`
	ParentID     *string      `json:"parentId"`
	Name         *string      `json:"name"`
	Currency     *string      `json:"currency"`
	TimeZone     *string      `json:"timeZone"`
	AuthMethod   string       `json:"authMethod"`
	Scopes       []string     `json:"scopes"`
	ConnectedAt  time.Time    `json:"connectedAt"`
	VerifiedAt   time.Time    `json:"verifiedAt"`
	Health       Health       `json:"health"`
	Capabilities []Capability `json:"capabilities"`
}

// ListAccounts returns every connected account with its health. Health is
// derived, never stored: failed if any capability failed, needs_permission if
// one needs permission, stale if the last verification is older than a day,
// else connected. No account is ever invented: an empty list is empty.
func (c *Connections) ListAccounts(ctx context.Context, actor core.Actor) ([]Account, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return nil, core.ErrForbidden
	}
	rows, err := c.db.Reader().ListAccountsInWorkspace(ctx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	caps, err := c.db.Reader().ListAccountCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	byAccount := map[int64][]Capability{}
	for _, r := range caps {
		byAccount[r.AccountID] = append(byAccount[r.AccountID], capabilityFromRow(r))
	}
	now := c.now().UTC()
	out := make([]Account, 0, len(rows))
	for _, r := range rows {
		a := accountFromRow(r.Account, r.Credential)
		a.Capabilities = byAccount[r.Account.ID]
		if a.Capabilities == nil {
			a.Capabilities = []Capability{}
		}
		a.Health = health(a, now)
		out = append(out, a)
	}
	return out, nil
}

func health(a Account, now time.Time) Health {
	for _, cp := range a.Capabilities {
		if cp.State == "failed" {
			return HealthFailed
		}
	}
	for _, cp := range a.Capabilities {
		if cp.State == "needs_permission" {
			return HealthNeedsPermission
		}
	}
	if now.Sub(a.VerifiedAt) > staleAfter {
		return HealthStale
	}
	return HealthConnected
}

// recordCapability stores the outcome of a probe. A failure keeps the last
// verified_at so the screen can say "worked until Tuesday".
func (c *Connections) recordCapability(ctx context.Context, accountID int64, name, state string, failure error) error {
	now := c.now().UTC()
	params := store.UpsertAccountCapabilityParams{AccountID: accountID, Capability: name, State: state, CheckedAt: now}
	if state == "verified" {
		params.VerifiedAt = sql.NullTime{Time: now, Valid: true}
	}
	if failure != nil {
		params.LastError = sql.NullString{String: safeMessage(failure), Valid: true}
	}
	return c.db.Writer().UpsertAccountCapability(ctx, params)
}

// safeMessage keeps only what core.Error says is safe to show.
func safeMessage(err error) string {
	var ce *core.Error
	if errors.As(err, &ce) {
		return ce.Message
	}
	return "the provider request failed"
}

// capabilityState maps a probe failure to a capability state: a rejection
// is a permission problem, anything else a failure.
func capabilityState(err error) string {
	var ce *core.Error
	if errors.As(err, &ce) && ce.Kind == core.KindConflict && strings.Contains(ce.Message, "rejected") {
		return "needs_permission"
	}
	return "failed"
}

func accountFromRow(a store.Account, cred store.Credential) Account {
	return Account{
		ID: a.ID, Provider: a.Type, ExternalID: a.ExternalID, ParentID: nullStr(a.ParentID), Name: nullStr(a.Name),
		Currency: nullStr(a.Currency), TimeZone: nullStr(a.TimeZone), AuthMethod: cred.Kind, Scopes: strings.Fields(cred.Scopes),
		ConnectedAt: a.ConnectedAt.UTC(), VerifiedAt: a.VerifiedAt.UTC(),
	}
}

func capabilityFromRow(r store.AccountCapability) Capability {
	out := Capability{Name: r.Capability, State: r.State, CheckedAt: r.CheckedAt.UTC(), LastError: nullStr(r.LastError)}
	if r.VerifiedAt.Valid {
		t := r.VerifiedAt.Time.UTC()
		out.VerifiedAt = &t
	}
	return out
}

func nullStr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}

// AccountForLocation returns the connected GoHighLevel account for a location
// id, for the webhook edge which has no actor yet: the location is what tells
// it whose event arrived. An unknown location is not found.
func (c *Connections) AccountForLocation(ctx context.Context, locationID string) (store.Account, error) {
	row, err := c.db.Reader().GetAccount(ctx, store.GetAccountParams{Type: Provider, ExternalID: locationID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Account{}, core.NotFound("no connected GoHighLevel location %q", locationID)
	}
	if err != nil {
		return store.Account{}, err
	}
	return row.Account, nil
}

// WorkspaceIDs lists every workspace, for edges that must decide whether a
// client is implied.
func (c *Connections) WorkspaceIDs(ctx context.Context) ([]int64, error) {
	rows, err := c.db.Reader().ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, w := range rows {
		out = append(out, w.ID)
	}
	return out, nil
}
