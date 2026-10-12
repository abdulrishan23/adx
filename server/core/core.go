// Package core holds what every domain package shares: who is acting, and
// the errors the edges know how to show.
package core

import "fmt"

// Role is what a person may do. The roles are in docs/reference.md.
type Role string

const (
	RoleViewer   Role = "viewer"
	RoleMarketer Role = "marketer"
	RoleAdmin    Role = "admin"
)

var roleRank = map[Role]int{RoleViewer: 1, RoleMarketer: 2, RoleAdmin: 3}

// Via is the entry point a call came through.
type Via string

const (
	ViaWeb Via = "web"
	ViaMCP Via = "mcp"
	ViaJob Via = "job"
)

// Actor is who is calling a core function. Every core function takes one, so
// web sessions, MCP tokens and jobs all go through the same checks. Core never
// reads cookies or tokens itself.
type Actor struct {
	UserID int64
	Role   Role
	Via    Via
}

// AtLeast reports whether the actor's role is r or higher. An actor with no
// role has none.
func (a Actor) AtLeast(r Role) bool {
	rank, ok := roleRank[a.Role]
	return ok && rank >= roleRank[r]
}

type Kind int

const (
	KindInvalid Kind = iota + 1
	KindForbidden
	KindNotFound
	KindConflict
)

// Error is an error the caller can act on. Its message is safe to show to
// users. Any other error is treated as internal and hidden.
type Error struct {
	Kind    Kind
	Message string
}

func (e *Error) Error() string { return e.Message }

func Invalid(format string, args ...any) error {
	return &Error{Kind: KindInvalid, Message: fmt.Sprintf(format, args...)}
}

func Conflict(format string, args ...any) error {
	return &Error{Kind: KindConflict, Message: fmt.Sprintf(format, args...)}
}

func NotFound(format string, args ...any) error {
	return &Error{Kind: KindNotFound, Message: fmt.Sprintf(format, args...)}
}

var ErrForbidden = &Error{Kind: KindForbidden, Message: "you don't have permission to do this"}
