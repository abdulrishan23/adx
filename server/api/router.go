// Package api turns HTTP requests into calls to core. Handlers parse and
// check input, call one core function, and write the result. No business
// logic lives here.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/ops"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/projects"
)

type Deps struct {
	Log         *slog.Logger
	Dev         bool
	Projects    *projects.Projects
	Connections *connections.Connections
	// Ops answers /api/ready. Nil leaves the route out, which tests do.
	Ops *ops.Ops
	// Identity resolves session cookies and runs sign-in. Nil means no
	// sessions at all: dev gets the dev admin, production refuses everything.
	Identity *identity.Identity
	Policies *policies.Policies
}

type handlers struct {
	log         *slog.Logger
	dev         bool
	projects    *projects.Projects
	connections *connections.Connections
	ops         *ops.Ops
	identity    *identity.Identity
	policies    *policies.Policies
}

func NewRouter(d Deps) http.Handler {
	h := &handlers{log: d.Log, dev: d.Dev, projects: d.Projects, connections: d.Connections, ops: d.Ops, identity: d.Identity, policies: d.Policies}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer, logRequests(d.Log))

	// Liveness: the process answers. Readiness: it can serve. The proxy and
	// the deploy script use the second one and never route traffic to a
	// server that isn't ready.
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	if h.ops != nil {
		r.Get("/api/ready", h.ready)
	}

	// Sign-in happens before there is a session.
	r.Get("/api/auth/login", h.login)
	r.Get("/api/auth/callback", h.authCallback)
	r.Post("/api/auth/logout", h.logout)

	r.Group(func(r chi.Router) {
		r.Use(withActor(d.Dev, d.Identity))
		r.Get("/api/session", h.session)
		r.Get("/api/accounts", h.listAccounts)
		if h.policies != nil {
			r.Get("/api/policies", h.getPolicies)
			r.Put("/api/policies", h.putPolicies)
			r.Put("/api/policies/kill-switch", h.putKillSwitch)
		}
		r.Get("/api/projects", h.listProjects)
		r.Post("/api/projects", h.createProject)
		r.Get("/api/connections/crm", h.listConnections)
		r.Get("/api/connections/crm/connect", h.connectGHLPage)
		r.Post("/api/connections/crm/start", h.startGHL)
		r.Post("/api/connections/crm/private", h.connectPrivateGHL)
		r.Get("/api/connections/crm/callback", h.finishGHL)
		r.Post("/api/connections/crm/{locationID}/check", h.checkGHL)
	})

	// Rejects cross-site POSTs and other unsafe requests, which is what CSRF
	// tokens are usually for.
	return http.NewCrossOriginProtection().Handler(r)
}

func logRequests(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.InfoContext(r.Context(), "request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}

func (h *handlers) ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	result := h.ops.Readiness(r.Context())
	status := http.StatusOK
	if !result.Ready {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, result)
}
