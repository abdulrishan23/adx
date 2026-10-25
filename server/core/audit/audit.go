// Package audit writes the shared log of sign-ins, role changes and other
// actions people need to look back on. Other core packages record entries in
// the same transaction as the change, so a change is never left unlogged.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/store"
)

type Action string

const (
	UserAdded Action = "user.added"
	// UserPasswordLinkSent is a set-password email, for an invite or a
	// forgotten password.
	UserPasswordLinkSent Action = "user.password_link_sent"
	UserRoleChanged      Action = "user.role_changed"
	UserDisabled         Action = "user.disabled"
	UserEnabled          Action = "user.enabled"
	SignIn               Action = "sign_in"
	SignInRefused        Action = "sign_in.refused"
	// AccountEnabled and AccountDisabled are an admin choosing which of the
	// accounts a credential reaches ADX reads (D-26).
	AccountEnabled  Action = "account.enabled"
	AccountDisabled Action = "account.disabled"
)

type Entry struct {
	Action Action
	// TargetUserID is the person the action was about, or 0 for none.
	TargetUserID int64
	// Detail is stored as JSON. Nil stores {}.
	Detail map[string]string
}

// Record adds e to the log as done by actor. Call it with the queries of the
// transaction that makes the change.
func Record(ctx context.Context, q *store.Queries, actor core.Actor, e Entry) error {
	detail := []byte("{}")
	if len(e.Detail) > 0 {
		var err error
		if detail, err = json.Marshal(e.Detail); err != nil {
			return err
		}
	}
	return q.CreateAuditEntry(ctx, store.CreateAuditEntryParams{
		At:           time.Now().UTC(),
		ActorUserID:  optionalID(actor.UserID),
		Via:          string(actor.Via),
		Action:       string(e.Action),
		TargetUserID: optionalID(e.TargetUserID),
		Detail:       string(detail),
	})
}

func optionalID(id int64) sql.NullInt64 {
	return sql.NullInt64{Int64: id, Valid: id != 0}
}
