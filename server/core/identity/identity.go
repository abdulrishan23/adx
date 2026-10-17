// Package identity turns a verified sign-in into a session, and a session
// into the core.Actor every request carries. authentik proves who someone is
// over OpenID Connect. core/users decides whether they may use ADX. This
// package owns the session table and the sign-in attempt that ties a browser
// to a callback.
//
// The cookie holds a random ID and the table only its hash, so a copied
// database holds nothing anyone can sign in with. Every request looks the
// session up, so disabling someone or changing their role applies on their
// next click (D-24). A session ends after IdleTTL without a request or MaxTTL
// after it began, whichever comes first.
package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/users"
	"github.com/WokeWorks/adx/server/store"
)

// Claims is what the identity provider asserts about the person who signed
// in, after the provider adapter verified the token.
type Claims struct {
	Subject string
	Email   string
	Name    string
	// Groups are the provider's group names for the person. Membership of the
	// allowed group decides whether they may use ADX at all.
	Groups []string
}

// OIDC is implemented by providers/authentik, never imported by core. The
// adapter verifies the ID token's signature, issuer, audience and nonce
// before returning claims.
type OIDC interface {
	// AuthorizationURL is where the browser goes to sign in. codeVerifier is
	// the PKCE secret; the adapter derives the S256 challenge from it.
	AuthorizationURL(state, nonce, codeVerifier string) string
	Exchange(ctx context.Context, code, codeVerifier, nonce string) (Claims, error)
}

type Config struct {
	// AllowedGroup is the provider group whose members may use ADX
	// (`adx-users` in D-24). Empty refuses everyone, which is the safe
	// default for a misconfigured server.
	AllowedGroup string
	IdleTTL      time.Duration
	MaxTTL       time.Duration
	Now          func() time.Time
}

// Session is a live session as the API shows it. It never contains the
// cookie value.
type Session struct {
	User        users.User `json:"user"`
	Role        core.Role  `json:"role"`
	WorkspaceID int64      `json:"workspaceId"`
	Workspace   string     `json:"workspace"`
	ExpiresAt   time.Time  `json:"expiresAt"`
}

// ErrNoSession means the token matches nothing live: unknown, ended or
// expired. The edge clears the cookie and asks for a sign-in.
var ErrNoSession = &core.Error{Kind: core.KindForbidden, Message: "sign in to continue"}

// errNotConfigured is what a dev server without authentik settings says.
var errNotConfigured = core.Conflict("sign-in is not configured on this server")

const attemptLifetime = 10 * time.Minute

// touchEvery limits how often a request writes last_seen_at, so reads don't
// each cost a write on SQLite's single writer.
const touchEvery = time.Minute

type Identity struct {
	db    *store.DB
	users *users.Users
	oidc  OIDC
	cfg   Config
}

// New wires sign-in. oidc may be nil in dev, which makes StartSignIn fail
// with a clear message while sessions that already exist keep working.
func New(db *store.DB, people *users.Users, oidc OIDC, cfg Config) *Identity {
	if cfg.IdleTTL == 0 {
		cfg.IdleTTL = 7 * 24 * time.Hour
	}
	if cfg.MaxTTL == 0 {
		cfg.MaxTTL = 30 * 24 * time.Hour
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Identity{db: db, users: people, oidc: oidc, cfg: cfg}
}

// StartSignIn records an attempt and returns where to send the browser.
// returnTo is the in-app path to land on afterwards and must be a local path.
func (i *Identity) StartSignIn(ctx context.Context, returnTo string) (string, error) {
	if i.oidc == nil {
		return "", errNotConfigured
	}
	returnTo = safeReturnTo(returnTo)
	state, nonce, verifier := rand.Text(), rand.Text(), pkceVerifier()
	now := i.cfg.Now().UTC()
	if err := i.db.Writer().DeleteExpiredSignInAttempts(ctx, now); err != nil {
		return "", err
	}
	err := i.db.Writer().CreateSignInAttempt(ctx, store.CreateSignInAttemptParams{
		StateHash: hash(state), NonceHash: hash(nonce), CodeVerifier: verifier,
		ReturnTo: returnTo, ExpiresAt: now.Add(attemptLifetime),
	})
	if err != nil {
		return "", err
	}
	return i.oidc.AuthorizationURL(state, nonce, verifier), nil
}

// FinishInput is what the provider sends back to the callback.
type FinishInput struct {
	State, Code, ProviderError string
}

// Finished is a new session plus the cookie value for it, which is handed to
// the browser once and never stored or logged.
type Finished struct {
	Token    string
	Session  Session
	ReturnTo string
}

// FinishSignIn consumes the attempt, exchanges the code, asks core/users
// whether the person may use ADX, and begins a session. The attempt is
// consumed before any outside call, so a replayed callback does nothing.
func (i *Identity) FinishSignIn(ctx context.Context, via core.Via, in FinishInput) (Finished, error) {
	if i.oidc == nil {
		return Finished{}, errNotConfigured
	}
	if len(in.State) < 20 || len(in.State) > 128 {
		return Finished{}, core.Invalid("sign-in request is invalid or expired; start again")
	}
	attempt, err := i.db.Writer().ConsumeSignInAttempt(ctx, store.ConsumeSignInAttemptParams{
		StateHash: hash(in.State), ExpiresAt: i.cfg.Now().UTC(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Finished{}, core.Invalid("sign-in request is invalid or expired; start again")
	}
	if err != nil {
		return Finished{}, err
	}
	if in.ProviderError != "" {
		return Finished{}, core.Invalid("sign-in was cancelled or refused; start again")
	}
	if in.Code == "" || len(in.Code) > 4096 {
		return Finished{}, core.Invalid("sign-in code is missing; start again")
	}
	// The nonce is only known as a hash, so the adapter gets the hash to
	// compare against the hash of the token's nonce claim.
	claims, err := i.oidc.Exchange(ctx, in.Code, attempt.CodeVerifier, attempt.NonceHash)
	if err != nil {
		return Finished{}, err
	}
	user, err := i.users.SignIn(ctx, core.Actor{Via: via}, users.Identity{
		Subject: claims.Subject, Email: claims.Email, Name: claims.Name,
		InAllowedGroup: i.cfg.AllowedGroup != "" && slices.Contains(claims.Groups, i.cfg.AllowedGroup),
	})
	if err != nil {
		return Finished{}, err
	}
	token, session, err := i.begin(ctx, user, via)
	if err != nil {
		return Finished{}, err
	}
	return Finished{Token: token, Session: session, ReturnTo: attempt.ReturnTo}, nil
}

func (i *Identity) begin(ctx context.Context, user users.User, via core.Via) (string, Session, error) {
	member, err := i.db.Reader().GetMembership(ctx, store.GetMembershipParams{WorkspaceID: core.DefaultWorkspace, UserID: user.ID})
	if errors.Is(err, sql.ErrNoRows) || (err == nil && member.Enabled == 0) {
		return "", Session{}, users.ErrNoAccess
	}
	if err != nil {
		return "", Session{}, err
	}
	workspace, err := i.db.Reader().GetWorkspace(ctx, core.DefaultWorkspace)
	if err != nil {
		return "", Session{}, err
	}
	token := rand.Text() + rand.Text()
	now := i.cfg.Now().UTC()
	expires := now.Add(i.cfg.MaxTTL)
	err = i.db.Writer().CreateSession(ctx, store.CreateSessionParams{
		IDHash: hash(token), UserID: user.ID, WorkspaceID: workspace.ID, Via: string(via),
		CreatedAt: now, LastSeenAt: now, ExpiresAt: expires,
	})
	if err != nil {
		return "", Session{}, err
	}
	return token, Session{
		User: user, Role: core.Role(member.Role), WorkspaceID: workspace.ID, Workspace: workspace.Name,
		ExpiresAt: i.sessionEnd(now, expires),
	}, nil
}

// Resolve turns a cookie value into the actor for this request. It refuses
// ended, expired and idle sessions, disabled people and disabled memberships,
// and takes the role from the membership, never from the client.
func (i *Identity) Resolve(ctx context.Context, via core.Via, token string) (core.Actor, Session, error) {
	if len(token) < 32 || len(token) > 128 {
		return core.Actor{}, Session{}, ErrNoSession
	}
	row, err := i.db.Reader().GetLiveSession(ctx, hash(token))
	if errors.Is(err, sql.ErrNoRows) {
		return core.Actor{}, Session{}, ErrNoSession
	}
	if err != nil {
		return core.Actor{}, Session{}, err
	}
	now := i.cfg.Now().UTC()
	if !now.Before(row.ExpiresAt) || !now.Before(row.LastSeenAt.Add(i.cfg.IdleTTL)) {
		_, _ = i.db.Writer().EndSession(ctx, store.EndSessionParams{EndedAt: sql.NullTime{Time: now, Valid: true}, IDHash: row.IDHash})
		return core.Actor{}, Session{}, ErrNoSession
	}
	userRow, err := i.db.Reader().GetUser(ctx, row.UserID)
	if err != nil {
		return core.Actor{}, Session{}, err
	}
	member, err := i.db.Reader().GetMembership(ctx, store.GetMembershipParams{WorkspaceID: row.WorkspaceID, UserID: row.UserID})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return core.Actor{}, Session{}, err
	}
	if users.Status(userRow.Status) == users.StatusDisabled || errors.Is(err, sql.ErrNoRows) || member.Enabled == 0 {
		_, _ = i.db.Writer().EndSession(ctx, store.EndSessionParams{EndedAt: sql.NullTime{Time: now, Valid: true}, IDHash: row.IDHash})
		return core.Actor{}, Session{}, ErrNoSession
	}
	if now.Sub(row.LastSeenAt) >= touchEvery {
		if err := i.db.Writer().TouchSession(ctx, store.TouchSessionParams{LastSeenAt: now, IDHash: row.IDHash}); err != nil {
			return core.Actor{}, Session{}, err
		}
		row.LastSeenAt = now
	}
	workspace, err := i.db.Reader().GetWorkspace(ctx, row.WorkspaceID)
	if err != nil {
		return core.Actor{}, Session{}, err
	}
	actor := core.Actor{UserID: userRow.ID, Role: core.Role(member.Role), Via: via, WorkspaceID: workspace.ID}
	user, err := i.users.Me(ctx, actor)
	if err != nil {
		return core.Actor{}, Session{}, err
	}
	return actor, Session{
		User: user, Role: actor.Role, WorkspaceID: workspace.ID, Workspace: workspace.Name,
		ExpiresAt: i.sessionEnd(row.LastSeenAt, row.ExpiresAt),
	}, nil
}

// SignOut ends the session behind token. An unknown token is not an error:
// the browser's cookie goes either way.
func (i *Identity) SignOut(ctx context.Context, token string) error {
	if len(token) < 32 || len(token) > 128 {
		return nil
	}
	_, err := i.db.Writer().EndSession(ctx, store.EndSessionParams{
		EndedAt: sql.NullTime{Time: i.cfg.Now().UTC(), Valid: true}, IDHash: hash(token),
	})
	return err
}

// sessionEnd is when the session dies if nothing else happens: the idle
// limit from the last request, capped by the absolute end.
func (i *Identity) sessionEnd(lastSeen, expires time.Time) time.Time {
	idle := lastSeen.Add(i.cfg.IdleTTL)
	if idle.Before(expires) {
		return idle
	}
	return expires
}

// safeReturnTo keeps the browser on this origin after sign-in.
func safeReturnTo(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.ContainsAny(p, "\\\r\n") || len(p) > 1024 {
		return "/"
	}
	return p
}

func pkceVerifier() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Hash is how attempts and sessions are stored. The adapter uses it to
// compare the token's nonce with the stored nonce hash.
func Hash(s string) string { return hash(s) }

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
