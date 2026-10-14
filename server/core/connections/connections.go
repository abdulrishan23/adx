// Package connections owns account connections and their credentials.
// It currently supports only GoHighLevel. Extend it to other providers, such as
// Google Ads and Search Console, and generalize shared credential handling as
// those integrations are added. Keep provider-specific API calls in providers.
package connections

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

const Provider = "gohighlevel"

var RequiredScopes = []string{
	"contacts.readonly", "locations.readonly",
	"locations/customFields.readonly", "opportunities.readonly",
}

// Tokens never cross an HTTP/MCP response boundary.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	Scope        string
	UserType     string
	LocationID   string
	CompanyID    string
}

type Location struct {
	ID        string
	Name      string
	CompanyID string
}

// GHL is implemented by the outside provider, not imported by core.
type GHL interface {
	AuthorizationURL(state string) string
	Exchange(context.Context, string) (Tokens, error)
	Refresh(context.Context, string) (Tokens, error)
	Location(context.Context, string, string) (Location, error)
	ReadAccess(context.Context, string, string) error
}

type Config struct {
	EncryptionKey []byte
	Now           func() time.Time
}

type Connections struct {
	db       *store.DB
	provider GHL
	aead     cipher.AEAD
	now      func() time.Time
	// Serializes reconnects and rotating refresh tokens in this single-server app.
	credentials sync.Mutex
}

type Connection struct {
	Provider       string     `json:"provider"`
	LocationID     string     `json:"locationId"`
	CompanyID      string     `json:"companyId"`
	Name           string     `json:"name"`
	Scopes         []string   `json:"scopes"`
	TokenExpiresAt *time.Time `json:"tokenExpiresAt"`
	AuthMethod     string     `json:"authMethod"`
	ConnectedAt    time.Time  `json:"connectedAt"`
	VerifiedAt     time.Time  `json:"verifiedAt"`
}

type StartResult struct {
	AuthorizationURL string    `json:"authorizationUrl"`
	ExpiresAt        time.Time `json:"expiresAt"`
}

type FinishInput struct {
	State, BrowserBinding, Code, ProviderError string
}

type tokenSecrets struct {
	AccessToken, RefreshToken string
}

func New(db *store.DB, provider GHL, cfg Config) (*Connections, error) {
	if len(cfg.EncryptionKey) != 32 {
		return nil, errors.New("connection encryption key must contain 32 bytes")
	}
	if provider == nil {
		return nil, errors.New("GHL provider is required")
	}
	block, err := aes.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, err
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Connections{db: db, provider: provider, aead: aead, now: cfg.Now}, nil
}

func (c *Connections) Start(ctx context.Context, actor core.Actor, browserBinding string) (StartResult, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return StartResult{}, core.ErrForbidden
	}
	if len(browserBinding) < 32 || len(browserBinding) > 128 {
		return StartResult{}, core.Invalid("browser verification is missing")
	}
	state := rand.Text()
	authorizationURL := c.provider.AuthorizationURL(state)
	if authorizationURL == "" {
		return StartResult{}, core.Conflict("GoHighLevel OAuth is not configured")
	}
	now := c.now().UTC()
	expires := now.Add(10 * time.Minute)
	if err := c.db.Writer().DeleteExpiredConnectionOAuthAttempts(ctx, now); err != nil {
		return StartResult{}, err
	}
	err := c.db.Writer().CreateConnectionOAuthAttempt(ctx, store.CreateConnectionOAuthAttemptParams{
		StateHash: hash(state), Provider: Provider, UserID: actor.UserID,
		BindingHash: hash(browserBinding), ExpiresAt: expires,
	})
	if err != nil {
		return StartResult{}, err
	}
	return StartResult{AuthorizationURL: authorizationURL, ExpiresAt: expires}, nil
}

func (c *Connections) Finish(ctx context.Context, actor core.Actor, in FinishInput) (Connection, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return Connection{}, core.ErrForbidden
	}
	if len(in.State) < 20 || len(in.State) > 128 || len(in.BrowserBinding) < 32 || len(in.BrowserBinding) > 128 {
		return Connection{}, core.Invalid("connection request is invalid or expired; start again")
	}
	// DELETE RETURNING consumes the state atomically before any outside call.
	_, err := c.db.Writer().ConsumeConnectionOAuthAttempt(ctx, store.ConsumeConnectionOAuthAttemptParams{
		StateHash: hash(in.State), Provider: Provider, UserID: actor.UserID,
		BindingHash: hash(in.BrowserBinding), ExpiresAt: c.now().UTC(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Connection{}, core.Invalid("connection request is invalid or expired; start again")
	}
	if err != nil {
		return Connection{}, err
	}
	if in.ProviderError != "" {
		return Connection{}, core.Invalid("GoHighLevel access was not granted; start again")
	}
	if in.Code == "" || len(in.Code) > 4096 {
		return Connection{}, core.Invalid("GoHighLevel authorization code is missing or invalid; start again")
	}
	c.credentials.Lock()
	defer c.credentials.Unlock()
	tokens, err := c.provider.Exchange(ctx, in.Code)
	if err != nil {
		return Connection{}, err
	}
	if err := validTokens(tokens); err != nil {
		return Connection{}, err
	}
	location, err := c.provider.Location(ctx, tokens.AccessToken, tokens.LocationID)
	if err != nil {
		return Connection{}, err
	}
	if location.ID != tokens.LocationID {
		return Connection{}, core.Invalid("GoHighLevel returned a different location")
	}
	sealed, err := c.seal(tokens)
	if err != nil {
		return Connection{}, err
	}
	now := c.now().UTC()
	row, err := c.db.Writer().UpsertConnection(ctx, store.UpsertConnectionParams{
		Provider: Provider, LocationID: tokens.LocationID, CompanyID: tokens.CompanyID,
		Name: locationName(location), EncryptedTokens: sealed, Scope: tokens.Scope,
		TokenExpiresAt: sql.NullTime{Time: now.Add(time.Duration(tokens.ExpiresIn) * time.Second), Valid: true},
		AuthMethod:     "oauth",
		ConnectedAt:    now, VerifiedAt: now,
	})
	return fromRow(row), err
}

// ConnectPrivate verifies the supplied workspace and read permissions before
// replacing only that workspace's credentials.
func (c *Connections) ConnectPrivate(ctx context.Context, actor core.Actor, locationID, token string) (Connection, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return Connection{}, core.ErrForbidden
	}
	token = strings.TrimSpace(token)
	if !validID(locationID) || token == "" || len(token) > 4096 || strings.ContainsAny(token, "\r\n\t ") {
		return Connection{}, core.Invalid("a workspace ID and valid private integration token are required")
	}
	c.credentials.Lock()
	defer c.credentials.Unlock()
	location, err := c.provider.Location(ctx, token, locationID)
	if err != nil {
		return Connection{}, err
	}
	if location.ID != locationID {
		return Connection{}, core.Invalid("GoHighLevel returned a different workspace")
	}
	if err := c.provider.ReadAccess(ctx, token, locationID); err != nil {
		return Connection{}, err
	}
	sealed, err := c.seal(Tokens{AccessToken: token, LocationID: locationID})
	if err != nil {
		return Connection{}, err
	}
	now := c.now().UTC()
	row, err := c.db.Writer().UpsertConnection(ctx, store.UpsertConnectionParams{
		Provider: Provider, LocationID: locationID, CompanyID: location.CompanyID,
		Name: locationName(location), EncryptedTokens: sealed,
		Scope: strings.Join(RequiredScopes, " "), AuthMethod: "private_token",
		ConnectedAt: now, VerifiedAt: now,
	})
	return fromRow(row), err
}

func (c *Connections) List(ctx context.Context, actor core.Actor) ([]Connection, error) {
	if !actor.AtLeast(core.RoleViewer) {
		return nil, core.ErrForbidden
	}
	rows, err := c.db.Reader().ListConnections(ctx, Provider)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromRow(row))
	}
	return out, nil
}

// Check proves the saved credentials can still read their own location.
func (c *Connections) Check(ctx context.Context, actor core.Actor, locationID string) (Connection, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return Connection{}, core.ErrForbidden
	}
	c.credentials.Lock()
	defer c.credentials.Unlock()
	row, err := c.db.Reader().GetConnection(ctx, store.GetConnectionParams{Provider: Provider, LocationID: locationID})
	if errors.Is(err, sql.ErrNoRows) {
		return Connection{}, core.NotFound("GoHighLevel location is not connected")
	}
	if err != nil {
		return Connection{}, err
	}
	secrets, err := c.open(row)
	if err != nil {
		return Connection{}, err
	}
	if row.AuthMethod == "oauth" && !row.TokenExpiresAt.Time.After(c.now().Add(time.Minute)) {
		tokens, err := c.provider.Refresh(ctx, secrets.RefreshToken)
		if err != nil {
			return Connection{}, err
		}
		if err := validTokens(tokens); err != nil {
			return Connection{}, err
		}
		if tokens.LocationID != locationID || tokens.CompanyID != row.CompanyID {
			return Connection{}, core.Invalid("refreshed credentials belong to a different account; reconnect")
		}
		sealed, err := c.seal(tokens)
		if err != nil {
			return Connection{}, err
		}
		expiry := c.now().UTC().Add(time.Duration(tokens.ExpiresIn) * time.Second)
		n, err := c.db.Writer().UpdateConnectionCredentials(ctx, store.UpdateConnectionCredentialsParams{
			EncryptedTokens: sealed, Scope: tokens.Scope, TokenExpiresAt: sql.NullTime{Time: expiry, Valid: true},
			Provider: Provider, LocationID: locationID, EncryptedTokens_2: row.EncryptedTokens,
		})
		if err != nil {
			return Connection{}, err
		}
		if n != 1 {
			return Connection{}, core.Conflict("connection changed; try again")
		}
		row.EncryptedTokens, row.Scope, row.TokenExpiresAt = sealed, tokens.Scope, sql.NullTime{Time: expiry, Valid: true}
		secrets.AccessToken = tokens.AccessToken
	}
	location, err := c.provider.Location(ctx, secrets.AccessToken, locationID)
	if err != nil {
		return Connection{}, err
	}
	if location.ID != locationID {
		return Connection{}, core.Invalid("GoHighLevel returned a different location")
	}
	if row.AuthMethod == "private_token" {
		if err := c.provider.ReadAccess(ctx, secrets.AccessToken, locationID); err != nil {
			return Connection{}, err
		}
	}
	now := c.now().UTC()
	n, err := c.db.Writer().UpdateConnectionVerification(ctx, store.UpdateConnectionVerificationParams{
		Name: locationName(location), VerifiedAt: now, Provider: Provider,
		LocationID: locationID, EncryptedTokens: row.EncryptedTokens,
	})
	if err != nil {
		return Connection{}, err
	}
	if n != 1 {
		return Connection{}, core.Conflict("connection changed; try again")
	}
	row.Name, row.VerifiedAt = locationName(location), now
	return fromRow(row), nil
}

func validTokens(t Tokens) error {
	if t.AccessToken == "" || t.RefreshToken == "" || t.ExpiresIn <= 0 || t.ExpiresIn > 365*24*60*60 {
		return core.Invalid("GoHighLevel returned incomplete credentials; reconnect")
	}
	if t.UserType != "Location" || !validID(t.LocationID) {
		return core.Invalid("select one GoHighLevel location to connect")
	}
	granted := make(map[string]bool)
	for _, scope := range strings.Fields(t.Scope) {
		granted[scope] = true
	}
	for _, scope := range RequiredScopes {
		if !granted[scope] {
			return core.Invalid("required GoHighLevel read access is missing; reconnect")
		}
	}
	return nil
}

func validID(id string) bool {
	if id == "" || len(id) > 100 {
		return false
	}
	for _, ch := range id {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' {
			continue
		}
		return false
	}
	return true
}

func hash(s string) string {
	digest := sha256.Sum256([]byte(s))
	return hex.EncodeToString(digest[:])
}

func (c *Connections) seal(t Tokens) ([]byte, error) {
	raw, err := json.Marshal(tokenSecrets{AccessToken: t.AccessToken, RefreshToken: t.RefreshToken})
	if err != nil {
		return nil, err
	}
	return c.aead.Seal(nil, nil, raw, []byte(Provider+":"+t.LocationID)), nil
}

func (c *Connections) open(row store.Connection) (tokenSecrets, error) {
	raw, err := c.aead.Open(nil, nil, row.EncryptedTokens, []byte(Provider+":"+row.LocationID))
	if err != nil {
		return tokenSecrets{}, errors.New("cannot decrypt connection credentials")
	}
	var out tokenSecrets
	if err := json.Unmarshal(raw, &out); err != nil {
		return tokenSecrets{}, fmt.Errorf("decode connection credentials: %w", err)
	}
	return out, nil
}

func locationName(l Location) string {
	if name := strings.TrimSpace(l.Name); name != "" {
		return name
	}
	return l.ID
}

func fromRow(r store.Connection) Connection {
	var expires *time.Time
	if r.TokenExpiresAt.Valid {
		t := r.TokenExpiresAt.Time.UTC()
		expires = &t
	}
	return Connection{
		Provider: r.Provider, LocationID: r.LocationID, CompanyID: r.CompanyID, Name: r.Name,
		Scopes: strings.Fields(r.Scope), TokenExpiresAt: expires, AuthMethod: r.AuthMethod,
		ConnectedAt: r.ConnectedAt.UTC(), VerifiedAt: r.VerifiedAt.UTC(),
	}
}
