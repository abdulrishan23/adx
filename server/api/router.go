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

	"github.com/WokeWorks/adx/server/core/alerts"
	"github.com/WokeWorks/adx/server/core/assistant"
	"github.com/WokeWorks/adx/server/core/brands"
	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/core/creative"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/core/ops"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/projects"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/reports"
	"github.com/WokeWorks/adx/server/core/research"
	"github.com/WokeWorks/adx/server/core/runs"
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
	Runs     *runs.Runs
	Leads    *leads.Leads
	// Reporting serves /api/analytics. Nil leaves the routes out.
	Reporting *reporting.Reporting
	Reports   *reports.Reports
	Alerts    *alerts.Alerts
	Assistant *assistant.Assistant
	Creative  *creative.Service
	Brands    *brands.Brands
	Research  *research.Research
	Plans     *plans.Plans
}

type handlers struct {
	log         *slog.Logger
	dev         bool
	projects    *projects.Projects
	connections *connections.Connections
	ops         *ops.Ops
	identity    *identity.Identity
	policies    *policies.Policies
	runs        *runs.Runs
	leads       *leads.Leads
	reporting   *reporting.Reporting
	reports     *reports.Reports
	alerts      *alerts.Alerts
	assistant   *assistant.Assistant
	creative    *creative.Service
	brands      *brands.Brands
	research    *research.Research
	plans       *plans.Plans
}

func NewRouter(d Deps) http.Handler {
	h := &handlers{log: d.Log, dev: d.Dev, projects: d.Projects, connections: d.Connections, ops: d.Ops, identity: d.Identity,
		policies: d.Policies, runs: d.Runs, leads: d.Leads, reporting: d.Reporting, reports: d.Reports, alerts: d.Alerts, assistant: d.Assistant,
		creative: d.Creative, brands: d.Brands, research: d.Research, plans: d.Plans}

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
		if h.runs != nil {
			r.Get("/api/runs", h.listRuns)
			r.Get("/api/runs/{id}", h.getRun)
		}
		if h.leads != nil {
			r.Get("/api/leads", h.listLeads)
			r.Get("/api/leads/coverage", h.leadCoverage)
			r.Post("/api/leads/import", h.importLeads)
			r.Get("/api/settings/funnel", h.getFunnel)
			r.Put("/api/settings/funnel", h.putFunnel)
		}
		r.Get("/api/projects", h.listProjects)
		r.Post("/api/projects", h.createProject)
		r.Get("/api/connections/crm", h.listConnections)
		r.Get("/api/connections/crm/connect", h.connectGHLPage)
		r.Post("/api/connections/crm/start", h.startGHL)
		r.Post("/api/connections/crm/private", h.connectPrivateGHL)
		r.Get("/api/connections/crm/callback", h.finishGHL)
		r.Post("/api/connections/crm/{locationID}/check", h.checkGHL)
		r.Get("/api/connections/{provider}/start", h.startAds)
		r.Get("/api/connections/{provider}/callback", h.finishAds)
		if h.reporting != nil {
			r.Get("/api/analytics", h.analytics)
			r.Post("/api/analytics/refresh", h.refreshAnalytics)
			r.Get("/api/analytics/freshness", h.analyticsFreshness)
			r.Post("/api/analytics/assign-projects", h.assignProjects)
		}
		if h.reports != nil {
			r.Get("/api/reports", h.listReports)
			r.Post("/api/reports", h.generateAdHoc)
			r.Get("/api/reports/schedules", h.listSchedules)
			r.Post("/api/reports/schedules", h.createSchedule)
			r.Put("/api/reports/schedules/{id}", h.updateSchedule)
			r.Post("/api/reports/schedules/{id}/generate", h.generateScheduled)
			r.Get("/api/reports/{id}", h.getReport)
		}
		if h.alerts != nil {
			r.Get("/api/alerts", h.listAlerts)
			r.Post("/api/alerts/{id}/acknowledge", h.alertAction(func(a *alerts.Alerts) func(*http.Request, int64) (alerts.Alert, error) {
				return func(r *http.Request, id int64) (alerts.Alert, error) {
					return a.Acknowledge(r.Context(), actorFrom(r), id)
				}
			}))
			r.Post("/api/alerts/{id}/resolve", h.alertAction(func(a *alerts.Alerts) func(*http.Request, int64) (alerts.Alert, error) {
				return func(r *http.Request, id int64) (alerts.Alert, error) { return a.Resolve(r.Context(), actorFrom(r), id) }
			}))
			r.Get("/api/alerts/config", h.getAlertConfig)
			r.Put("/api/alerts/config", h.putAlertConfig)
			r.Post("/api/alerts/evaluate", h.evaluateAlerts)
		}
		if h.assistant != nil {
			r.Get("/api/assistant/conversations", h.listConversations)
			r.Post("/api/assistant/conversations", h.createConversation)
			r.Get("/api/assistant/conversations/{id}", h.conversationTurns)
			r.Post("/api/assistant/conversations/{id}/messages", h.sendMessage)
		}
		if h.creative != nil {
			r.Get("/api/assets", h.listAssets)
			r.Post("/api/assets", h.uploadAsset)
			r.Get("/api/assets/files/{key}", h.serveAsset)
			r.Get("/api/projects/{projectID}/brand-kit", h.getBrandKit)
			r.Put("/api/projects/{projectID}/brand-kit", h.putBrandKit)
			r.Get("/api/creatives", h.listCreatives)
			r.Post("/api/creatives", h.createCreative)
			r.Get("/api/creatives/{id}", h.getCreative)
			r.Put("/api/creatives/{id}/document", h.saveCreativeDocument)
			r.Post("/api/creatives/{id}/render", h.creativeAction(func(r *http.Request, id int64) (creative.Version, error) {
				return h.creative.Render(r.Context(), actorFrom(r), id)
			}))
			r.Post("/api/creatives/{id}/submit", h.creativeAction(func(r *http.Request, id int64) (creative.Version, error) {
				return h.creative.Submit(r.Context(), actorFrom(r), id)
			}))
			r.Post("/api/creatives/{id}/review", h.reviewCreative)
			r.Get("/api/creatives/{id}/export/{version}", h.exportCreative)
		}
		if h.brands != nil {
			r.Get("/api/brands", h.listBrands)
			r.Post("/api/brands", h.createBrand)
			r.Put("/api/brands/projects", h.assignProjectBrand)
			r.Get("/api/brands/{id}", h.getBrand)
			r.Post("/api/brands/{id}/offers", h.createOffer)
			r.Get("/api/brands/{id}/facts", h.listFacts)
			r.Post("/api/brands/{id}/facts", h.addFact)
			r.Get("/api/brands/{id}/claims", h.claims)
			r.Get("/api/projects/{projectID}/offers", h.projectOffers)
			r.Get("/api/offers/{id}", h.getOffer)
			r.Post("/api/offers/{id}/versions", h.reviseOffer)
			r.Post("/api/offers/{id}/state", h.setOfferState)
			r.Post("/api/facts/{id}/approve", h.approveFact)
			r.Post("/api/facts/{id}/retire", h.retireFact)
			r.Post("/api/fact-conflicts/{id}/resolve", h.resolveConflict)
		}
		if h.research != nil {
			r.Get("/api/research/briefs", h.listBriefs)
			r.Post("/api/research/briefs", h.createBrief)
			r.Get("/api/research/briefs/{id}", h.getBrief)
			r.Post("/api/research/briefs/{id}/runs", h.startResearchRun)
			r.Post("/api/research/briefs/{id}/competitors", h.resolveCompetitor)
			r.Get("/api/research/briefs/{id}/findings", h.listFindings)
			r.Get("/api/research/evidence/{id}", h.researchEvidence)
		}
		if h.plans != nil {
			r.Get("/api/plans", h.listPlans)
			r.Post("/api/plans", h.createPlan)
			r.Post("/api/plans/validate", h.validatePlan)
			r.Get("/api/plans/{id}", h.getPlan)
			r.Post("/api/plans/{id}/versions", h.savePlanVersion)
			r.Post("/api/plans/{id}/generate", h.generatePlan)
			r.Get("/api/plans/{id}/diff", h.diffPlan)
			r.Post("/api/plans/{id}/import", h.importPlanVersion)
			r.Post("/api/plans/{id}/versions/{version}/accept", h.acceptPlanVersion)
			r.Get("/api/plans/{id}/versions/{version}/export", h.exportPlanVersion)
		}
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
