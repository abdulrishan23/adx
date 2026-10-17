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
	"github.com/WokeWorks/adx/server/core/projects"
)

type Deps struct {
	Log         *slog.Logger
	Dev         bool
	Projects    *projects.Projects
	Connections *connections.Connections
}

type handlers struct {
	log         *slog.Logger
	projects    *projects.Projects
	connections *connections.Connections
}

func NewRouter(d Deps) http.Handler {
	h := &handlers{log: d.Log, projects: d.Projects, connections: d.Connections}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer, logRequests(d.Log))

	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Group(func(r chi.Router) {
		r.Use(withActor(d.Dev))
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
