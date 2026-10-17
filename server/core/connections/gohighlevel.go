package connections

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

// Provider names GoHighLevel credentials and the location accounts they reach.
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
	TimeZone  string
	Country   string
}

// GHL is implemented by the outside provider, not imported by core.
type GHL interface {
	AuthorizationURL(state string) string
	Exchange(context.Context, string) (Tokens, error)
	Refresh(context.Context, string) (Tokens, error)
	Location(context.Context, string, string) (Location, error)
	ReadAccess(context.Context, string, string) error
}

// Connection is a GoHighLevel location as the API shows it.
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
	accountID      int64
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

// ghlDetails is what a GoHighLevel location keeps in accounts.details.
type ghlDetails struct {
	Country string `json:"country,omitempty"`
}

func (c *Connections) Start(ctx context.Context, actor core.Actor, browserBinding string) (StartResult, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return StartResult{}, core.ErrForbidden
	}
	if len(browserBinding) < 32 || len(browserBinding) > 128 {
		return StartResult{}, core.Invalid("browser verification is missing")
	}
	state := rand.Text()
	authorizationURL := c.ghl.AuthorizationURL(state)
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
	tokens, err := c.ghl.Exchange(ctx, in.Code)
	if err != nil {
		return Connection{}, err
	}
	if err := validTokens(tokens); err != nil {
		return Connection{}, err
	}
	location, err := c.ghl.Location(ctx, tokens.AccessToken, tokens.LocationID)
	if err != nil {
		return Connection{}, err
	}
	if location.ID != tokens.LocationID {
		return Connection{}, core.Invalid("GoHighLevel returned a different location")
	}
	location.CompanyID = tokens.CompanyID
	expiry := c.now().UTC().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	return c.saveGHL(ctx, actor, kindOAuth, tokenSecrets{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken},
		tokens.Scope, sql.NullTime{Time: expiry, Valid: true}, location)
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
	location, err := c.ghl.Location(ctx, token, locationID)
	if err != nil {
		return Connection{}, err
	}
	if location.ID != locationID {
		return Connection{}, core.Invalid("GoHighLevel returned a different workspace")
	}
	if err := c.ghl.ReadAccess(ctx, token, locationID); err != nil {
		return Connection{}, err
	}
	conn, err := c.saveGHL(ctx, actor, kindPrivateToken, tokenSecrets{AccessToken: token},
		strings.Join(RequiredScopes, " "), sql.NullTime{}, location)
	if err != nil {
		return Connection{}, err
	}
	return conn, c.recordCapability(ctx, conn.accountID, CapabilityReadLeads, "verified", nil)
}

func (c *Connections) saveGHL(ctx context.Context, actor core.Actor, kind string, secrets tokenSecrets,
	scopes string, expiresAt sql.NullTime, location Location,
) (Connection, error) {
	sealed, err := c.seal(Provider, location.ID, secrets)
	if err != nil {
		return Connection{}, err
	}
	details, err := json.Marshal(ghlDetails{Country: location.Country})
	if err != nil {
		return Connection{}, err
	}
	account, cred, err := c.save(ctx, actor, store.UpsertCredentialParams{
		Provider: Provider, Kind: kind, Subject: location.ID, EncryptedSecret: sealed,
		Scopes: scopes, TokenExpiresAt: expiresAt,
	}, store.UpsertAccountParams{
		Type: Provider, ExternalID: location.ID, ParentID: nullString(location.CompanyID),
		Name: nullString(locationName(location)), TimeZone: nullString(location.TimeZone), Details: string(details),
	})
	if err != nil {
		return Connection{}, err
	}
	conn := fromRow(account, cred)
	conn.accountID = account.ID
	return conn, nil
}

func (c *Connections) List(ctx context.Context, actor core.Actor) ([]Connection, error) {
	if !actor.AtLeast(core.RoleViewer) {
		return nil, core.ErrForbidden
	}
	rows, err := c.db.Reader().ListAccounts(ctx, Provider)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromRow(row.Account, row.Credential))
	}
	return out, nil
}

// Check proves the saved credentials can still read their own location, and
// refreshes the location's name, time zone and country.
func (c *Connections) Check(ctx context.Context, actor core.Actor, locationID string) (Connection, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return Connection{}, core.ErrForbidden
	}
	c.credentials.Lock()
	defer c.credentials.Unlock()
	account, cred, secrets, err := c.currentSecrets(ctx, locationID)
	if err != nil {
		return Connection{}, err
	}
	location, err := c.ghl.Location(ctx, secrets.AccessToken, locationID)
	if err != nil {
		return Connection{}, errors.Join(err, c.recordCapability(ctx, account.ID, CapabilityReadLeads, capabilityState(err), err))
	}
	if location.ID != locationID {
		return Connection{}, core.Invalid("GoHighLevel returned a different location")
	}
	if cred.Kind == kindPrivateToken {
		if err := c.ghl.ReadAccess(ctx, secrets.AccessToken, locationID); err != nil {
			return Connection{}, errors.Join(err, c.recordCapability(ctx, account.ID, CapabilityReadLeads, capabilityState(err), err))
		}
	}
	if err := c.recordCapability(ctx, account.ID, CapabilityReadLeads, "verified", nil); err != nil {
		return Connection{}, err
	}
	details, err := json.Marshal(ghlDetails{Country: location.Country})
	if err != nil {
		return Connection{}, err
	}
	account.Name, account.TimeZone, account.Details = nullString(locationName(location)), nullString(location.TimeZone), string(details)
	account.VerifiedAt = c.now().UTC()
	n, err := c.db.Writer().UpdateAccountVerification(ctx, store.UpdateAccountVerificationParams{
		Name: account.Name, TimeZone: account.TimeZone, Details: account.Details, VerifiedAt: account.VerifiedAt,
		ID: account.ID, EncryptedSecret: cred.EncryptedSecret,
	})
	if err != nil {
		return Connection{}, err
	}
	if n != 1 {
		return Connection{}, core.Conflict("connection changed; try again")
	}
	return fromRow(account, cred), nil
}

// GHLToken hands a location's current access token to core code that reads
// through it (core/leads). Only ADX itself or an admin may ask, the token
// never crosses an API boundary, and an expired OAuth token is refreshed
// first. It implements leads.Tokens.
func (c *Connections) GHLToken(ctx context.Context, actor core.Actor, locationID string) (string, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return "", core.ErrForbidden
	}
	c.credentials.Lock()
	defer c.credentials.Unlock()
	_, _, secrets, err := c.currentSecrets(ctx, locationID)
	if err != nil {
		return "", err
	}
	return secrets.AccessToken, nil
}

// currentSecrets loads the location's credential and, for an OAuth
// credential near expiry, rotates it. The caller holds c.credentials.
func (c *Connections) currentSecrets(ctx context.Context, locationID string) (store.Account, store.Credential, tokenSecrets, error) {
	row, err := c.db.Reader().GetAccount(ctx, store.GetAccountParams{Type: Provider, ExternalID: locationID})
	if errors.Is(err, sql.ErrNoRows) {
		return store.Account{}, store.Credential{}, tokenSecrets{}, core.NotFound("GoHighLevel location is not connected")
	}
	if err != nil {
		return store.Account{}, store.Credential{}, tokenSecrets{}, err
	}
	account, cred := row.Account, row.Credential
	var secrets tokenSecrets
	if err := c.open(cred, &secrets); err != nil {
		return store.Account{}, store.Credential{}, tokenSecrets{}, err
	}
	if cred.Kind == kindOAuth && !cred.TokenExpiresAt.Time.After(c.now().Add(time.Minute)) {
		tokens, err := c.ghl.Refresh(ctx, secrets.RefreshToken)
		if err != nil {
			return store.Account{}, store.Credential{}, tokenSecrets{}, err
		}
		if err := validTokens(tokens); err != nil {
			return store.Account{}, store.Credential{}, tokenSecrets{}, err
		}
		if tokens.LocationID != locationID || tokens.CompanyID != account.ParentID.String {
			return store.Account{}, store.Credential{}, tokenSecrets{}, core.Invalid("refreshed credentials belong to a different account; reconnect")
		}
		secrets = tokenSecrets{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken}
		sealed, err := c.seal(cred.Provider, cred.Subject, secrets)
		if err != nil {
			return store.Account{}, store.Credential{}, tokenSecrets{}, err
		}
		expiry := sql.NullTime{Time: c.now().UTC().Add(time.Duration(tokens.ExpiresIn) * time.Second), Valid: true}
		n, err := c.db.Writer().UpdateCredentialSecret(ctx, store.UpdateCredentialSecretParams{
			NewSecret: sealed, KeyVersion: keyVersion, Scopes: tokens.Scope, TokenExpiresAt: expiry,
			ID: cred.ID, OldSecret: cred.EncryptedSecret,
		})
		if err != nil {
			return store.Account{}, store.Credential{}, tokenSecrets{}, err
		}
		if n != 1 {
			return store.Account{}, store.Credential{}, tokenSecrets{}, core.Conflict("connection changed; try again")
		}
		cred.EncryptedSecret, cred.KeyVersion, cred.Scopes, cred.TokenExpiresAt = sealed, keyVersion, tokens.Scope, expiry
	}
	return account, cred, secrets, nil
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

func locationName(l Location) string {
	if name := strings.TrimSpace(l.Name); name != "" {
		return name
	}
	return l.ID
}

func fromRow(a store.Account, cred store.Credential) Connection {
	var expires *time.Time
	if cred.TokenExpiresAt.Valid {
		t := cred.TokenExpiresAt.Time.UTC()
		expires = &t
	}
	return Connection{
		Provider: a.Type, LocationID: a.ExternalID, CompanyID: a.ParentID.String, Name: a.Name.String,
		Scopes: strings.Fields(cred.Scopes), TokenExpiresAt: expires, AuthMethod: cred.Kind,
		ConnectedAt: a.ConnectedAt.UTC(), VerifiedAt: a.VerifiedAt.UTC(),
	}
}
