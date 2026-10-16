// Package users keeps the people who may use ADX and their roles. authentik
// checks who someone is. ADX decides whether they get in and what they may do.
//
// There's no self sign-up. An admin adds an email and a role. ADX finds or
// creates that person's authentik account, puts it in the ADX group, and has
// authentik email them a link to set a password. ADX keeps the account's
// UUID, which authentik sends as the subject at sign-in, so sign-in never
// relies on an email address.
//
// ADX only ever changes membership of the ADX group. It never switches
// authentik accounts on or off, so an ADX admin can't lock anyone out of
// authentik itself, such as a WokeWorks admin who also uses ADX.
package users

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/audit"
	"github.com/WokeWorks/adx/server/store"
)

type Status string

const (
	// StatusInvited is someone added but not signed in yet.
	StatusInvited  Status = "invited"
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

type User struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	// Name comes from authentik and is null until the first sign-in.
	Name         *string    `json:"name"`
	Role         core.Role  `json:"role"`
	Status       Status     `json:"status"`
	CreatedAt    time.Time  `json:"createdAt"`
	LastSignInAt *time.Time `json:"lastSignInAt"`
	// AuthentikUUID shows which authentik account the person signs in with.
	AuthentikUUID string `json:"authentikUuid"`
}

type AddInput struct {
	Email string    `json:"email"`
	Role  core.Role `json:"role"`
}

// Account is a person's authentik account.
type Account struct {
	PK       int64
	UUID     string
	Username string
	Email    string
	Active   bool
	// CreatedByADX marks accounts ADX made itself, so a retried add can
	// link them.
	CreatedByADX bool
}

// Accounts is what ADX needs from authentik, where people's accounts live. Every call is safe to repeat,
// so a failed add or disable can simply be retried.
type Accounts interface {
	FindByEmail(ctx context.Context, email string) ([]Account, error)
	Create(ctx context.Context, email string) (Account, error)
	AddToGroup(ctx context.Context, pk int64) error
	RemoveFromGroup(ctx context.Context, pk int64) error
	SendSetPasswordEmail(ctx context.Context, pk int64) error
}

// Identity is who authentik says signed in.
type Identity struct {
	Subject string
	Email   string
	Name    string
	// InAllowedGroup is whether authentik puts them in the group that may use
	// ADX at all.
	InAllowedGroup bool
}

// ErrNoAccess is what someone sees when they signed in to authentik but may
// not use ADX.
var ErrNoAccess = &core.Error{
	Kind:    core.KindForbidden,
	Message: "you don't have access to ADX, ask an admin to add you",
}

var errLastAdmin = core.Conflict("ADX needs at least one admin who isn't disabled")

type Users struct {
	db       *store.DB
	accounts Accounts
}

func New(db *store.DB, accounts Accounts) *Users {
	return &Users{db: db, accounts: accounts}
}

func (u *Users) List(ctx context.Context, actor core.Actor) ([]User, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return nil, core.ErrForbidden
	}
	rows, err := u.db.Reader().ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

// Me returns the person acting.
func (u *Users) Me(ctx context.Context, actor core.Actor) (User, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.UserID == 0 {
		return User{}, core.ErrForbidden
	}
	row, err := u.db.Reader().GetUser(ctx, actor.UserID)
	if err != nil {
		return User{}, err
	}
	return fromRow(row), nil
}

// Add invites someone. It writes to authentik first and ADX second, then
// sends the set-password email. If the email fails, the person is still
// added as invited, and the error says to resend it.
func (u *Users) Add(ctx context.Context, actor core.Actor, in AddInput) (User, error) {
	return u.add(ctx, actor, in, "")
}

// add is Add, but it may also use the existing authentik account with
// linkUUID, which ADX didn't create. Only Bootstrap passes one. An email
// alone isn't enough, because an account's email can be changed to anyone's.
func (u *Users) add(ctx context.Context, actor core.Actor, in AddInput, linkUUID string) (User, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return User{}, core.ErrForbidden
	}
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return User{}, err
	}
	if !in.Role.Valid() {
		return User{}, core.Invalid("role must be viewer, marketer or admin")
	}
	existing, err := u.db.Reader().GetUserByEmail(ctx, email)
	switch {
	case err == nil && Status(existing.Status) == StatusDisabled:
		return User{}, core.Conflict("%s is already in ADX but disabled, enable them instead", email)
	case err == nil:
		return User{}, core.Conflict("%s is already in ADX", email)
	case !errors.Is(err, sql.ErrNoRows):
		return User{}, err
	}

	acct, err := u.authentikAccount(ctx, email, linkUUID)
	if err != nil {
		return User{}, err
	}

	var row store.User
	err = u.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		row, err = q.CreateUser(ctx, store.CreateUserParams{
			Email:         email,
			Role:          string(in.Role),
			Status:        string(StatusInvited),
			AuthentikPk:   acct.PK,
			AuthentikUuid: acct.UUID,
			CreatedAt:     time.Now().UTC(),
		})
		if store.IsUniqueViolation(err) {
			return core.Conflict("someone was just added with %s or the same authentik account, so check the list", email)
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, q, actor, audit.Entry{
			Action:       audit.UserAdded,
			TargetUserID: row.ID,
			Detail:       map[string]string{"email": email, "role": string(in.Role)},
		})
	})
	if err != nil {
		return User{}, err
	}
	return fromRow(row), u.sendInvite(ctx, actor, row)
}

// authentikAccount finds the account with this email or creates one, and
// puts it in the ADX group. It links an existing account only if ADX created
// it, such as on a retry, or if its UUID is linkUUID. Otherwise anyone who
// got an authentik account with a person's email before they were added
// would get their access. An account someone switched off in authentik is
// left off.
func (u *Users) authentikAccount(ctx context.Context, email, linkUUID string) (Account, error) {
	found, err := u.accounts.FindByEmail(ctx, email)
	if err != nil {
		return Account{}, unavailable(err)
	}
	var acct Account
	switch len(found) {
	case 0:
		if linkUUID != "" {
			return Account{}, core.Conflict("no authentik account has the email %s any more, so try again", email)
		}
		if acct, err = u.accounts.Create(ctx, email); err != nil {
			return Account{}, unavailable(err)
		}
	case 1:
		acct = found[0]
		if !acct.Active {
			return Account{}, core.Conflict("the authentik account for %s is deactivated, so ask an authentik admin to turn it on first", email)
		}
		switch {
		case linkUUID != "" && linkUUID != acct.UUID:
			return Account{}, core.Conflict("the authentik account with %s is %s (%s), not %s",
				email, acct.Username, acct.UUID, linkUUID)
		case linkUUID == "" && !acct.CreatedByADX:
			return Account{}, core.Conflict("%s already has an authentik account that ADX didn't create, so ADX can't add them yet", email)
		}
	default:
		return Account{}, core.Conflict("%d authentik accounts use %s, so fix that in authentik first", len(found), email)
	}
	// An account can already belong to someone in ADX if its email changed
	// in authentik. Refuse before touching the group, so a disabled person
	// isn't put back in it.
	owner, err := u.db.Reader().GetUserByAuthentikUUID(ctx, acct.UUID)
	if err == nil {
		return Account{}, core.Conflict("the authentik account with the email %s belongs to %s in ADX, so fix its email in authentik first", email, owner.Email)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Account{}, err
	}
	if err := u.accounts.AddToGroup(ctx, acct.PK); err != nil {
		return Account{}, unavailable(err)
	}
	return acct, nil
}

// SendPasswordLink emails someone a link to set their password: again for
// someone invited who lost the first email, or for someone active who forgot
// their password. Disabled people get nothing.
func (u *Users) SendPasswordLink(ctx context.Context, actor core.Actor, id int64) error {
	if !actor.AtLeast(core.RoleAdmin) {
		return core.ErrForbidden
	}
	row, err := u.get(ctx, id)
	if err != nil {
		return err
	}
	if Status(row.Status) == StatusDisabled {
		return core.Conflict("%s is disabled, so enable them first", row.Email)
	}
	if err := u.accounts.SendSetPasswordEmail(ctx, row.AuthentikPk); err != nil {
		return core.Unavailable(err, "the set-password email to %s failed, so try again", row.Email)
	}
	return u.logPasswordLink(ctx, actor, row)
}

func (u *Users) sendInvite(ctx context.Context, actor core.Actor, row store.User) error {
	if err := u.accounts.SendSetPasswordEmail(ctx, row.AuthentikPk); err != nil {
		return core.Unavailable(err, "%s is added, but the set-password email failed, so send it again", row.Email)
	}
	return u.logPasswordLink(ctx, actor, row)
}

func (u *Users) logPasswordLink(ctx context.Context, actor core.Actor, row store.User) error {
	return u.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		return audit.Record(ctx, q, actor, audit.Entry{Action: audit.UserPasswordLinkSent, TargetUserID: row.ID})
	})
}

// SetRole changes someone's role. Setting the role they already have does
// nothing.
func (u *Users) SetRole(ctx context.Context, actor core.Actor, id int64, role core.Role) (User, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return User{}, core.ErrForbidden
	}
	if !role.Valid() {
		return User{}, core.Invalid("role must be viewer, marketer or admin")
	}
	row, err := u.change(ctx, id, func(q *store.Queries, row store.User) error {
		if row.Role == string(role) {
			return nil
		}
		if err := q.SetUserRole(ctx, store.SetUserRoleParams{Role: string(role), ID: id}); err != nil {
			return err
		}
		return audit.Record(ctx, q, actor, audit.Entry{
			Action:       audit.UserRoleChanged,
			TargetUserID: id,
			Detail:       map[string]string{"from": row.Role, "to": string(role)},
		})
	})
	return fromRow(row), err
}

// Disable stops someone from using ADX. It writes to ADX first, so they're
// out at once, then takes them out of the ADX group in authentik so they
// can't sign in again. If authentik fails, run it again: the ADX part is
// already done and is skipped.
func (u *Users) Disable(ctx context.Context, actor core.Actor, id int64) (User, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return User{}, core.ErrForbidden
	}
	row, err := u.change(ctx, id, func(q *store.Queries, row store.User) error {
		if Status(row.Status) == StatusDisabled {
			return nil
		}
		if err := q.SetUserStatus(ctx, store.SetUserStatusParams{Status: string(StatusDisabled), ID: id}); err != nil {
			return err
		}
		return audit.Record(ctx, q, actor, audit.Entry{Action: audit.UserDisabled, TargetUserID: id})
	})
	if err != nil {
		return User{}, err
	}
	if err := u.accounts.RemoveFromGroup(ctx, row.AuthentikPk); err != nil {
		return fromRow(row), core.Unavailable(err,
			"%s is disabled in ADX but still in the ADX group in authentik, so try again", row.Email)
	}
	return fromRow(row), nil
}

// Enable lets a disabled person back in with the role they had. It puts them
// back in the ADX group in authentik first, then ADX.
func (u *Users) Enable(ctx context.Context, actor core.Actor, id int64) (User, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return User{}, core.ErrForbidden
	}
	row, err := u.get(ctx, id)
	if err != nil {
		return User{}, err
	}
	if err := u.accounts.AddToGroup(ctx, row.AuthentikPk); err != nil {
		return User{}, unavailable(err)
	}
	row, err = u.change(ctx, id, func(q *store.Queries, row store.User) error {
		if Status(row.Status) != StatusDisabled {
			return nil
		}
		status := StatusInvited
		if row.LastSignInAt.Valid {
			status = StatusActive
		}
		if err := q.SetUserStatus(ctx, store.SetUserStatusParams{Status: string(status), ID: id}); err != nil {
			return err
		}
		return audit.Record(ctx, q, actor, audit.Entry{Action: audit.UserEnabled, TargetUserID: id})
	})
	return fromRow(row), err
}

func (u *Users) get(ctx context.Context, id int64) (store.User, error) {
	row, err := u.db.Reader().GetUser(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return store.User{}, core.NotFound("user %d not found", id)
	}
	return row, err
}

// change runs fn on one user inside a transaction. If they were an admin who
// isn't disabled, it refuses the result when no such admin is left.
func (u *Users) change(ctx context.Context, id int64, fn func(q *store.Queries, row store.User) error) (store.User, error) {
	var out store.User
	err := u.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		row, err := q.GetUser(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return core.NotFound("user %d not found", id)
		}
		if err != nil {
			return err
		}
		if err := fn(q, row); err != nil {
			return err
		}
		if row.Role == string(core.RoleAdmin) && Status(row.Status) != StatusDisabled {
			admins, err := q.CountEnabledAdmins(ctx)
			if err != nil {
				return err
			}
			if admins == 0 {
				return errLastAdmin
			}
		}
		out, err = q.GetUser(ctx, id)
		return err
	})
	return out, err
}

// Bootstrap makes email an admin when ADX has no admin who isn't disabled,
// so a fresh install can be signed in to. Otherwise it does nothing and
// returns false. It links the one authentik account with that email, which
// is safe only because it runs on an empty install from the server's own
// settings. The caller should log the returned person's AuthentikUUID.
func (u *Users) Bootstrap(ctx context.Context, actor core.Actor, email string) (User, bool, error) {
	if !actor.AtLeast(core.RoleAdmin) {
		return User{}, false, core.ErrForbidden
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return User{}, false, err
	}
	admins, err := u.db.Reader().CountEnabledAdmins(ctx)
	if err != nil || admins > 0 {
		return User{}, false, err
	}
	row, err := u.db.Reader().GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		var linkUUID string
		if found, err := u.accounts.FindByEmail(ctx, email); err != nil {
			return User{}, false, unavailable(err)
		} else if len(found) == 1 {
			linkUUID = found[0].UUID
		}
		user, err := u.add(ctx, actor, AddInput{Email: email, Role: core.RoleAdmin}, linkUUID)
		return user, true, err
	}
	if err != nil {
		return User{}, false, err
	}
	if Status(row.Status) == StatusDisabled {
		if _, err := u.Enable(ctx, actor, row.ID); err != nil {
			return User{}, true, err
		}
	}
	user, err := u.SetRole(ctx, actor, row.ID, core.RoleAdmin)
	return user, true, err
}

// SignIn decides whether the person authentik signed in may use ADX. It
// matches only on authentik's subject, which is the account UUID ADX saved
// when adding them, never on the email. An invited person becomes active.
// The name is refreshed for display. The email isn't: it stays the one the
// admin entered, because admins look people up by it and authentik emails
// can change. Every attempt is logged. actor only says where the attempt
// came from.
func (u *Users) SignIn(ctx context.Context, actor core.Actor, id Identity) (User, error) {
	if id.Subject == "" {
		return User{}, errors.New("sign-in has no subject")
	}
	email := strings.ToLower(strings.TrimSpace(id.Email))
	var (
		out     store.User
		refused string
	)
	err := u.db.Tx(ctx, func(_ *sql.Tx, q *store.Queries) error {
		row, err := q.GetUserByAuthentikUUID(ctx, id.Subject)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			refused = "unknown_account"
		case err != nil:
			return err
		case !id.InAllowedGroup:
			refused = "not_in_group"
		case Status(row.Status) == StatusDisabled:
			refused = "disabled"
		}
		if refused != "" {
			return audit.Record(ctx, q, actor, audit.Entry{
				Action:       audit.SignInRefused,
				TargetUserID: row.ID,
				Detail:       map[string]string{"email": email, "reason": refused},
			})
		}

		name := row.Name
		if n := strings.TrimSpace(id.Name); n != "" {
			name = sql.NullString{String: n, Valid: true}
		}
		out, err = q.RecordSignIn(ctx, store.RecordSignInParams{
			Name:         name,
			LastSignInAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
			ID:           row.ID,
		})
		if err != nil {
			return err
		}
		person := core.Actor{UserID: row.ID, Role: core.Role(row.Role), Via: actor.Via}
		return audit.Record(ctx, q, person, audit.Entry{Action: audit.SignIn, TargetUserID: row.ID})
	})
	if err != nil {
		return User{}, err
	}
	if refused != "" {
		return User{}, ErrNoAccess
	}
	return fromRow(out), nil
}

func unavailable(err error) error {
	var ce *core.Error
	if errors.As(err, &ce) {
		return err
	}
	return core.Unavailable(err, "authentik didn't respond, so nothing was saved in ADX, try again")
}

func normalizeEmail(s string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(s))
	if email == "" {
		return "", core.Invalid("email is required")
	}
	// authentik usernames are the email and can't be longer than 150.
	if len(email) > 150 {
		return "", core.Invalid("email must be 150 characters or fewer")
	}
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email {
		return "", core.Invalid("%q is not a valid email address", strings.TrimSpace(s))
	}
	return email, nil
}

func fromRow(r store.User) User {
	u := User{
		ID:            r.ID,
		Email:         r.Email,
		Role:          core.Role(r.Role),
		Status:        Status(r.Status),
		CreatedAt:     r.CreatedAt,
		AuthentikUUID: r.AuthentikUuid,
	}
	if r.Name.Valid {
		u.Name = &r.Name.String
	}
	if r.LastSignInAt.Valid {
		u.LastSignInAt = &r.LastSignInAt.Time
	}
	return u
}
