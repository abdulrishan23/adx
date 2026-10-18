package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/users"
)

type actorKey struct{}
type sessionKey struct{}

// sessionCookie carries the random session ID. It's HttpOnly, SameSite=Lax
// and Secure outside dev, where Caddy terminates TLS.
const sessionCookie = "adx_session"

// withActor puts the acting user on the request. A session cookie is looked
// up in core/identity on every request, so a disabled person or a changed
// role applies at once. Nothing the client sends besides the cookie is
// consulted: a role header is ignored. Without a cookie, dev mode hands out
// an admin in the default workspace, and only dev mode: the dev person when
// startup made one, so actions that record who did them work, else an actor
// with no user id. Outside dev every request without a live session is 401.
func withActor(dev bool, id *identity.Identity, devUser *users.User) func(http.Handler) http.Handler {
	var devUserID int64
	if devUser != nil {
		devUserID = devUser.ID
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" && id != nil {
				actor, session, err := id.Resolve(r.Context(), core.ViaWeb, cookie.Value)
				switch {
				case errors.Is(err, identity.ErrNoSession):
					clearSessionCookie(w, dev)
					writeError(w, http.StatusUnauthorized, "your session has ended, sign in again")
					return
				case err != nil:
					writeError(w, http.StatusInternalServerError, "something went wrong")
					return
				}
				ctx := context.WithValue(r.Context(), actorKey{}, actor)
				ctx = context.WithValue(ctx, sessionKey{}, session)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			if !dev {
				writeError(w, http.StatusUnauthorized, "sign in to continue")
				return
			}
			actor := core.Actor{UserID: devUserID, Role: core.RoleAdmin, Via: core.ViaWeb, WorkspaceID: devWorkspace(r)}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, actor)))
		})
	}
}

func actorFrom(r *http.Request) core.Actor {
	actor, _ := r.Context().Value(actorKey{}).(core.Actor)
	return actor
}

// sessionFrom returns the session behind the request, if a real one exists.
func sessionFrom(r *http.Request) (identity.Session, bool) {
	s, ok := r.Context().Value(sessionKey{}).(identity.Session)
	return s, ok
}

func setSessionCookie(w http.ResponseWriter, token string, dev bool) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: !dev,
		// Session cookie: no Max-Age, so the browser drops it when closed and
		// the server's own expiry is the real limit.
	})
}

func clearSessionCookie(w http.ResponseWriter, dev bool) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: !dev, MaxAge: -1,
	})
}

// devWorkspace is the workspace the dev admin acts in: the adx_workspace
// cookie the dev switch endpoint sets, else Reportage. Only the dev actor
// reads it; a real session's workspace comes from its row.
func devWorkspace(r *http.Request) int64 {
	if c, err := r.Cookie(devWorkspaceCookie); err == nil {
		if id, err := strconv.ParseInt(c.Value, 10, 64); err == nil && id > 0 {
			return id
		}
	}
	return core.DefaultWorkspace
}

const devWorkspaceCookie = "adx_workspace"
