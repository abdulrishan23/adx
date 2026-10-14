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
	ViaCLI Via = "cli"
)

// Actor is who is calling a core function. Every core function takes one, so
// web sessions, MCP tokens and jobs all go through the same checks. Core never
// reads cookies or tokens itself. UserID 0 means ADX itself, such as a job or
// the CLI, rather than a person.
type Actor struct {
	UserID int64
	Role   Role
	Via    Via
}

// System is ADX acting on its own behalf. It may do anything an admin may.
func System(via Via) Actor {
	return Actor{UserID: 0, Role: RoleAdmin, Via: via}
}

// Valid reports whether r is one of the roles above.
func (r Role) Valid() bool {
	_, ok := roleRank[r]
	return ok
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
	KindUnavailable
)

// Error is an error the caller can act on. Its message is safe to show to
// users. Any other error is treated as internal and hidden.
type Error struct {
	Kind    Kind
	Message string
	// Cause is what went wrong underneath, for the logs. It's never shown.
	Cause error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Cause }

func Invalid(format string, args ...any) error {
	return &Error{Kind: KindInvalid, Message: fmt.Sprintf(format, args...)}
}

func Conflict(format string, args ...any) error {
	return &Error{Kind: KindConflict, Message: fmt.Sprintf(format, args...)}
}

func NotFound(format string, args ...any) error {
	return &Error{Kind: KindNotFound, Message: fmt.Sprintf(format, args...)}
}

// Unavailable means an outside service such as authentik failed. message
// tells the user what state things are in. cause goes to the logs.
func Unavailable(cause error, format string, args ...any) error {
	return &Error{Kind: KindUnavailable, Message: fmt.Sprintf(format, args...), Cause: cause}
}

var ErrForbidden = &Error{Kind: KindForbidden, Message: "you don't have permission to do this"}
