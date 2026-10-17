package connections

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/store"
)

// Ad platforms ADX connects. The names are the accounts.type values.
const (
	ProviderGoogleAds = "google_ads"
	ProviderMeta      = "meta"
)

// CapabilityReadReporting is campaigns and daily metrics, proved by a
// successful account discovery after connecting.
const CapabilityReadReporting = "read.reporting"

// AdsTokens is what an ad platform's OAuth exchange gives back. Subject is
// the signed-in person's stable id at the provider: one credential per
// person per provider (D-26), reaching however many accounts they can see.
type AdsTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scopes       []string
	Subject      string
}

// AdsIdentity is the OAuth side of one ad platform, implemented in
// providers/googleads and providers/meta. Refresh takes the stored refresh
// token (Google) or the current access token (Meta, which has no refresh
// token and re-exchanges long-lived tokens).
type AdsIdentity interface {
	AuthorizationURL(state, codeVerifier string) string
	Exchange(ctx context.Context, code, codeVerifier string) (AdsTokens, error)
	Refresh(ctx context.Context, refreshToken string) (AdsTokens, error)
}

// AdsPlatform is everything core needs from one ad platform.
type AdsPlatform struct {
	Identity AdsIdentity
	Reader   reporting.Provider
}

// WithAds registers the ad platforms. Called from cmd/adx after New.
func (c *Connections) WithAds(platforms map[string]AdsPlatform) *Connections {
	c.ads = platforms
	return c
}

type adsSecrets struct {
	AccessToken, RefreshToken string
}

// StartAds begins connecting an ad platform. Admins only.
func (c *Connections) StartAds(ctx context.Context, actor core.Actor, provider, browserBinding string) (StartResult, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return StartResult{}, core.ErrForbidden
	}
	p, ok := c.ads[provider]
	if !ok || p.Identity == nil {
		return StartResult{}, core.Conflict("%s is not configured on this server", providerLabel(provider))
	}
	if len(browserBinding) < 32 || len(browserBinding) > 128 {
		return StartResult{}, core.Invalid("browser verification is missing")
	}
	state := rand.Text()
	verifier := pkceVerifier()
	now := c.now().UTC()
	expires := now.Add(10 * time.Minute)
	if err := c.db.Writer().DeleteExpiredConnectionOAuthAttempts(ctx, now); err != nil {
		return StartResult{}, err
	}
	err := c.db.Writer().CreateConnectionOAuthAttempt(ctx, store.CreateConnectionOAuthAttemptParams{
		StateHash: hash(state), Provider: provider, UserID: actor.UserID, BindingHash: hash(browserBinding), ExpiresAt: expires, CodeVerifier: verifier,
	})
	if err != nil {
		return StartResult{}, err
	}
	return StartResult{AuthorizationURL: p.Identity.AuthorizationURL(state, verifier), ExpiresAt: expires}, nil
}

// FinishAds consumes the attempt, exchanges the code, seals the tokens as one
// credential for the signed-in person, then discovers the accounts that
// person reaches and saves each one with its currency and time zone. Every
// discovered account gets read.reporting verified; nothing is written to
// any account. It returns the accounts saved.
func (c *Connections) FinishAds(ctx context.Context, actor core.Actor, provider string, in FinishInput) ([]Account, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return nil, core.ErrForbidden
	}
	p, ok := c.ads[provider]
	if !ok || p.Identity == nil || p.Reader == nil {
		return nil, core.Conflict("%s is not configured on this server", providerLabel(provider))
	}
	if len(in.State) < 20 || len(in.State) > 128 || len(in.BrowserBinding) < 32 || len(in.BrowserBinding) > 128 {
		return nil, core.Invalid("connection request is invalid or expired; start again")
	}
	attempt, err := c.db.Writer().ConsumeConnectionOAuthAttempt(ctx, store.ConsumeConnectionOAuthAttemptParams{
		StateHash: hash(in.State), Provider: provider, UserID: actor.UserID, BindingHash: hash(in.BrowserBinding), ExpiresAt: c.now().UTC(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.Invalid("connection request is invalid or expired; start again")
	}
	if err != nil {
		return nil, err
	}
	if in.ProviderError != "" {
		return nil, core.Invalid("%s access was not granted; start again", providerLabel(provider))
	}
	if in.Code == "" || len(in.Code) > 4096 {
		return nil, core.Invalid("%s authorization code is missing; start again", providerLabel(provider))
	}
	c.credentials.Lock()
	defer c.credentials.Unlock()
	tokens, err := p.Identity.Exchange(ctx, in.Code, attempt.CodeVerifier)
	if err != nil {
		return nil, err
	}
	if tokens.AccessToken == "" || tokens.Subject == "" {
		return nil, core.Invalid("%s returned incomplete credentials; try again", providerLabel(provider))
	}
	// Discover before saving, so a token that reaches nothing readable is
	// never stored as a working connection.
	found, err := p.Reader.ListAccounts(ctx, tokens.AccessToken)
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, core.Conflict("the %s sign-in reaches no ad accounts; sign in with a person who has access", providerLabel(provider))
	}
	sealed, err := c.seal(provider, tokens.Subject, adsSecrets{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken})
	if err != nil {
		return nil, err
	}
	now := c.now().UTC()
	var expires sql.NullTime
	if !tokens.ExpiresAt.IsZero() {
		expires = sql.NullTime{Time: tokens.ExpiresAt.UTC(), Valid: true}
	}
	var saved []store.Account
	err = c.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		cred, err := q.UpsertCredential(ctx, store.UpsertCredentialParams{
			Provider: provider, Kind: kindOAuth, Subject: tokens.Subject, EncryptedSecret: sealed, KeyVersion: keyVersion,
			Scopes: strings.Join(tokens.Scopes, " "), TokenExpiresAt: expires,
			ConnectedBy: sql.NullInt64{Int64: actor.UserID, Valid: actor.UserID != 0}, ConnectedAt: now,
		})
		if err != nil {
			return err
		}
		for _, a := range found {
			if a.Manager {
				// Managers and businesses hold no spend; they are the parent
				// of the accounts that do.
				continue
			}
			acct, err := q.UpsertAccount(ctx, store.UpsertAccountParams{
				Type: provider, ExternalID: a.ExternalID, CredentialID: cred.ID, ParentID: nullString(a.ParentID), Name: nullString(a.Name),
				Currency: nullString(a.Currency), TimeZone: nullString(a.TimeZone), Details: fmt.Sprintf(`{"status":%q}`, a.Status),
				ConnectedAt: now, VerifiedAt: now,
			})
			if err != nil {
				return err
			}
			if err := q.UpsertAccountCapability(ctx, store.UpsertAccountCapabilityParams{
				AccountID: acct.ID, Capability: CapabilityReadReporting, State: "verified", VerifiedAt: sql.NullTime{Time: now, Valid: true}, CheckedAt: now,
			}); err != nil {
				return err
			}
			saved = append(saved, acct)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(saved))
	for _, a := range saved {
		acc := accountFromRow(a, store.Credential{Kind: kindOAuth, Scopes: strings.Join(tokens.Scopes, " ")})
		acc.Health = HealthConnected
		acc.Capabilities = []Capability{{Name: CapabilityReadReporting, State: "verified", CheckedAt: now, VerifiedAt: &now}}
		out = append(out, acc)
	}
	return out, nil
}

// ProviderToken implements reporting.Tokens: the current access token for
// the credential behind an ad account, refreshed when it is about to expire.
// Only ADX itself or an admin may ask, and the token never leaves core.
func (c *Connections) ProviderToken(ctx context.Context, actor core.Actor, accountID int64) (string, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return "", core.ErrForbidden
	}
	c.credentials.Lock()
	defer c.credentials.Unlock()
	row, err := c.db.Reader().GetAccountByID(ctx, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", core.NotFound("account %d is not connected", accountID)
	}
	if err != nil {
		return "", err
	}
	cred := row.Credential
	p, ok := c.ads[cred.Provider]
	if !ok || p.Identity == nil {
		return "", core.Conflict("%s is not configured on this server", providerLabel(cred.Provider))
	}
	var secrets adsSecrets
	if err := c.open(cred, &secrets); err != nil {
		return "", err
	}
	// Refresh when within five minutes of expiry. Google needs the refresh
	// token; Meta re-exchanges the access token itself.
	if cred.TokenExpiresAt.Valid && !cred.TokenExpiresAt.Time.After(c.now().Add(5*time.Minute)) {
		input := secrets.RefreshToken
		if input == "" {
			input = secrets.AccessToken
		}
		tokens, err := p.Identity.Refresh(ctx, input)
		if err != nil {
			_ = c.recordCapabilityForCredential(ctx, cred.ID, capabilityState(err), err)
			return "", err
		}
		if tokens.AccessToken == "" {
			return "", core.Conflict("%s returned no credentials on refresh; reconnect", providerLabel(cred.Provider))
		}
		if tokens.RefreshToken == "" {
			tokens.RefreshToken = secrets.RefreshToken
		}
		secrets = adsSecrets{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken}
		sealed, err := c.seal(cred.Provider, cred.Subject, secrets)
		if err != nil {
			return "", err
		}
		scopes := cred.Scopes
		if len(tokens.Scopes) > 0 {
			scopes = strings.Join(tokens.Scopes, " ")
		}
		var expires sql.NullTime
		if !tokens.ExpiresAt.IsZero() {
			expires = sql.NullTime{Time: tokens.ExpiresAt.UTC(), Valid: true}
		}
		n, err := c.db.Writer().UpdateCredentialSecret(ctx, store.UpdateCredentialSecretParams{
			NewSecret: sealed, KeyVersion: keyVersion, Scopes: scopes, TokenExpiresAt: expires, ID: cred.ID, OldSecret: cred.EncryptedSecret,
		})
		if err != nil {
			return "", err
		}
		if n != 1 {
			return "", core.Conflict("connection changed; try again")
		}
	}
	return secrets.AccessToken, nil
}

// recordCapabilityForCredential marks read.reporting on every account the
// credential reaches, for a failure that is about the credential itself.
func (c *Connections) recordCapabilityForCredential(ctx context.Context, credentialID int64, state string, failure error) error {
	accounts, err := c.db.Reader().ListAccountsByCredential(ctx, credentialID)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if err := c.recordCapability(ctx, a.ID, CapabilityReadReporting, state, failure); err != nil {
			return err
		}
	}
	return nil
}

func providerLabel(p string) string {
	switch p {
	case ProviderGoogleAds:
		return "Google Ads"
	case ProviderMeta:
		return "Meta"
	case Provider:
		return "GoHighLevel"
	}
	return p
}

func pkceVerifier() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
