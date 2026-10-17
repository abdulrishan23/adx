package api

import (
	"context"
	"net/http"

	"github.com/WokeWorks/adx/server/core"
)

type actorKey struct{}

// withActor puts the acting user on the request. Until auth is designed it
// hands every request a dev admin, and only in dev. Outside dev it refuses
// everything, so a deployed server can't run with a fake admin. Real login
// replaces this function and nothing else.
func withActor(dev bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !dev {
				writeError(w, http.StatusUnauthorized, "sign-in is not set up yet")
				return
			}
			actor := core.Actor{UserID: 0, Role: core.RoleAdmin, Via: core.ViaWeb}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, actor)))
		})
	}
}

func actorFrom(r *http.Request) core.Actor {
	actor, _ := r.Context().Value(actorKey{}).(core.Actor)
	return actor
}
