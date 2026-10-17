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
	"github.com/WokeWorks/adx/server/core/calls"
	"github.com/WokeWorks/adx/server/core/connections"
	"github.com/WokeWorks/adx/server/core/creative"
	"github.com/WokeWorks/adx/server/core/crm"
	"github.com/WokeWorks/adx/server/core/evaluation"
	"github.com/WokeWorks/adx/server/core/execution"
	"github.com/WokeWorks/adx/server/core/identity"
	"github.com/WokeWorks/adx/server/core/leads"
	"github.com/WokeWorks/adx/server/core/measurement"
	"github.com/WokeWorks/adx/server/core/ops"
	"github.com/WokeWorks/adx/server/core/outreach"
	"github.com/WokeWorks/adx/server/core/pages"
	"github.com/WokeWorks/adx/server/core/pipeline"
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/projects"
	"github.com/WokeWorks/adx/server/core/proposals"
	"github.com/WokeWorks/adx/server/core/qualification"
	"github.com/WokeWorks/adx/server/core/registry"
	"github.com/WokeWorks/adx/server/core/reporting"
	"github.com/WokeWorks/adx/server/core/reports"
	"github.com/WokeWorks/adx/server/core/research"
	"github.com/WokeWorks/adx/server/core/runs"
	"github.com/WokeWorks/adx/server/core/signals"
	"github.com/WokeWorks/adx/server/core/workflows"
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
	Reporting     *reporting.Reporting
	Reports       *reports.Reports
	Alerts        *alerts.Alerts
	Assistant     *assistant.Assistant
	Creative      *creative.Service
	Brands        *brands.Brands
	Research      *research.Research
	Plans         *plans.Plans
	Pages         *pages.Pages
	Measurement   *measurement.Measurement
	Proposals     *proposals.Proposals
	Execution     *execution.Dispatcher
	CRM           *crm.CRM
	Signals       *signals.Signals
	Outreach      *outreach.Outreach
	Calls         *calls.Calls
	Qualification *qualification.Qualification
	Evaluation    *evaluation.Evaluation
	Pipeline      *pipeline.Pipeline
	Registry      *registry.Registry
	Workflows     *workflows.Workflows
	// WebhookSecret is the shared secret public webhook adapters require.
	// Empty disables them.
	WebhookSecret string
}

type handlers struct {
	log           *slog.Logger
	dev           bool
	projects      *projects.Projects
	connections   *connections.Connections
	ops           *ops.Ops
	identity      *identity.Identity
	policies      *policies.Policies
	runs          *runs.Runs
	leads         *leads.Leads
	reporting     *reporting.Reporting
	reports       *reports.Reports
	alerts        *alerts.Alerts
	assistant     *assistant.Assistant
	creative      *creative.Service
	brands        *brands.Brands
	research      *research.Research
	plans         *plans.Plans
	pages         *pages.Pages
	measurement   *measurement.Measurement
	proposals     *proposals.Proposals
	execution     *execution.Dispatcher
	crm           *crm.CRM
	signals       *signals.Signals
	outreach      *outreach.Outreach
	calls         *calls.Calls
	qualification *qualification.Qualification
	evaluation    *evaluation.Evaluation
	pipeline      *pipeline.Pipeline
	registry      *registry.Registry
	workflows     *workflows.Workflows
	webhookSecret string
}

func NewRouter(d Deps) http.Handler {
	h := &handlers{log: d.Log, dev: d.Dev, projects: d.Projects, connections: d.Connections, ops: d.Ops, identity: d.Identity,
		policies: d.Policies, runs: d.Runs, leads: d.Leads, reporting: d.Reporting, reports: d.Reports, alerts: d.Alerts, assistant: d.Assistant,
		creative: d.Creative, brands: d.Brands, research: d.Research, plans: d.Plans, pages: d.Pages, measurement: d.Measurement, proposals: d.Proposals, execution: d.Execution, crm: d.CRM, signals: d.Signals, outreach: d.Outreach, calls: d.Calls, qualification: d.Qualification, evaluation: d.Evaluation, pipeline: d.Pipeline, registry: d.Registry, workflows: d.Workflows, webhookSecret: d.WebhookSecret}

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

	// Public landing pages and their forms: what a visitor's browser calls.
	if h.pages != nil {
		r.Get("/p/{slug}", h.publicPage)
		r.Get("/p/{slug}/thanks", h.publicThanks)
		r.Post("/p/{slug}/form", h.publicForm)
		if h.signals != nil {
			r.Post("/webhooks/ghl", h.ghlWebhook)
		}
		if h.calls != nil {
			r.Post("/webhooks/voice/status", h.voiceStatusWebhook)
			if h.qualification != nil {
				r.Get("/webhooks/voice/stream/{id}", h.voiceStream)
			}
		}
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
			r.Post("/api/assistant/conversations/{id}/voice", h.sendVoice)
			r.Post("/api/assistant/voice/preview", h.previewVoice)
		}
		if h.creative != nil {
			r.Get("/api/assets", h.listAssets)
			r.Post("/api/assets", h.uploadAsset)
			r.Get("/api/assets/files/{key}", h.serveAsset)
			r.Get("/api/projects/{projectID}/brand-kit", h.getBrandKit)
			r.Put("/api/projects/{projectID}/brand-kit", h.putBrandKit)
			r.Get("/api/creatives", h.listCreatives)
			r.Post("/api/creatives", h.createCreative)
			r.Get("/api/creatives/queue", h.creativeQueue)
			r.Get("/api/creatives/{id}", h.getCreative)
			r.Get("/api/creatives/{id}/readiness", h.creativeReadiness)
			r.Put("/api/creatives/{id}/document", h.saveCreativeDocument)
			r.Post("/api/creatives/{id}/render", h.startRender)
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
			r.Get("/api/research/schedules", h.listResearchSchedules)
			r.Post("/api/research/schedules", h.createResearchSchedule)
			r.Put("/api/research/schedules/{id}", h.updateResearchSchedule)
			r.Get("/api/research/briefs/{id}/reports", h.listResearchReports)
			r.Post("/api/research/briefs/{id}/reports", h.generateResearchReport)
			r.Get("/api/research/reports/{id}", h.getResearchReport)
			r.Post("/api/research/reports/{id}/review", h.reviewHypothesis)
			r.Get("/api/research/events", h.listMarketEvents)
			r.Post("/api/research/events", h.recordMarketEvent)
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
		if h.pages != nil {
			r.Get("/api/pages", h.listPages)
			r.Post("/api/pages", h.createPage)
			r.Get("/api/pages/{id}", h.getPage)
			r.Post("/api/pages/{id}/versions", h.savePageVersion)
			r.Get("/api/pages/{id}/versions/{version}/preview", h.previewPage)
			r.Post("/api/pages/{id}/publish", h.publishPage)
			r.Post("/api/pages/{id}/rollback", h.rollbackPage)
			r.Get("/api/pages/{id}/readiness", h.pageReadiness)
			r.Get("/api/pages/{id}/submissions", h.pageSubmissions)
		}
		if h.measurement != nil {
			r.Get("/api/measurement/catalog", h.measurementCatalog)
			r.Post("/api/measurement/checks", h.runMeasurementCheck)
			r.Get("/api/measurement/checks", h.listMeasurementChecks)
			r.Get("/api/measurement/readiness", h.launchReadiness)
			r.Get("/api/measurement/events", h.measurementEvents)
			r.Post("/api/measurement/tracked-url", h.trackedURL)
		}
		if h.proposals != nil {
			r.Get("/api/proposals", h.listProposals)
			r.Post("/api/proposals", h.createProposal)
			r.Get("/api/proposals/{id}", h.getProposal)
			r.Post("/api/proposals/{id}/approve", h.decideProposal("approve"))
			r.Post("/api/proposals/{id}/reject", h.decideProposal("reject"))
			r.Post("/api/proposals/{id}/execute", h.executeProposal)
			r.Get("/api/executions", h.listExecutions)
			r.Get("/api/executions/{id}", h.getExecution)
			r.Post("/api/executions/{id}/cancel", h.cancelExecution)
			r.Get("/api/budgets/envelopes", h.listEnvelopes)
			r.Post("/api/budgets/envelopes", h.openEnvelopes)
		}
		if h.execution != nil {
			r.Get("/api/executions/capabilities", h.executionCapabilities)
			r.Get("/api/executions/launch", h.launchStatus)
			r.Get("/api/executions/{id}/timeline", h.executionTimeline)
			r.Post("/api/executions/{id}/retry", h.retryExecution)
			r.Post("/api/executions/{id}/reconcile", h.reconcileExecution)
			r.Post("/api/executions/{id}/compensate", h.compensateExecution)
			r.Post("/api/executions/{id}/repair", h.repairExecution)
			r.Get("/api/executions/{id}/reversibility", h.executionReversibility)
			r.Get("/api/executions/milestones", h.launchMilestones)
		}
		if h.crm != nil {
			r.Get("/api/crm/people", h.listPeople)
			r.Post("/api/crm/people", h.createPerson)
			r.Post("/api/crm/sync", h.syncCRM)
			r.Get("/api/crm/candidates", h.listCandidates)
			r.Post("/api/crm/candidates/{id}/decide", h.decideCandidate)
			r.Get("/api/crm/enquiries", h.listEnquiries)
			r.Get("/api/crm/people/{id}", h.getPerson)
			r.Post("/api/crm/people/{id}/merge", h.mergePerson)
			r.Post("/api/crm/people/{id}/unmerge", h.unmergePerson)
			r.Post("/api/crm/people/{id}/enrich", h.enrichPerson)
			r.Post("/api/crm/people/{id}/attributes", h.setStatedAttribute)
			r.Post("/api/crm/people/{id}/links", h.linkPerson)
			r.Post("/api/crm/links/{id}/reverse", h.reverseLink)
		}
		if h.signals != nil {
			r.Get("/api/signals/rules", h.getScoringRules)
			r.Put("/api/signals/rules", h.putScoringRules)
			r.Post("/api/signals/events", h.ingestEvent)
			r.Get("/api/signals/events/unknown", h.unknownEvents)
			r.Post("/api/signals/events/{id}/correct", h.correctEvent)
			r.Post("/api/signals/derive", h.deriveSignals)
			r.Get("/api/signals/people/{id}", h.personScore)
		}
		if h.outreach != nil {
			r.Get("/api/outreach/config", h.getOutreachConfig)
			r.Put("/api/outreach/config", h.putOutreachConfig)
			r.Get("/api/outreach/enquiries/{id}/brief", h.enquiryBrief)
			r.Post("/api/outreach/objections", h.recordObjection)
			r.Post("/api/outreach/objections/{id}/resolve", h.resolveObjection)
			r.Post("/api/outreach/suppressions", h.suppress)
			r.Post("/api/outreach/suppressions/{id}/lift", h.liftSuppression)
			r.Get("/api/outreach/drafts", h.listDrafts)
			r.Post("/api/outreach/drafts", h.createDraft)
			r.Post("/api/outreach/drafts/{id}/approve", h.approveDraft)
			r.Post("/api/outreach/drafts/{id}/send", h.sendDraft)
			r.Get("/api/outreach/fields", h.listFieldWrites)
			r.Post("/api/outreach/fields", h.queueFieldWrite)
			r.Post("/api/outreach/fields/{id}/resolve", h.resolveFieldConflict)
			r.Post("/api/outreach/fields/dispatch", h.dispatchFieldWrites)
			r.Post("/api/outreach/fields/sync-scores", h.syncScores)
		}
		if h.calls != nil {
			r.Get("/api/voice/policy", h.getCallPolicy)
			r.Put("/api/voice/policy", h.putCallPolicy)
			r.Get("/api/voice/contactability", h.contactability)
			r.Get("/api/voice/calls", h.listCalls)
			r.Post("/api/voice/calls", h.requestCall)
			r.Get("/api/voice/calls/{id}", h.getCall)
			r.Post("/api/voice/calls/{id}/hangup", h.hangupCall)
			r.Post("/api/voice/calls/{id}/reconcile", h.reconcileCall)
			if h.qualification != nil {
				r.Get("/api/voice/calls/{id}/evidence", h.callEvidence)
				r.Post("/api/voice/calls/{id}/confirm", h.confirmReading)
				r.Post("/api/voice/calls/{id}/outcome", h.correctReading)
				r.Get("/api/voice/calls/{id}/recording", h.callRecording)
			}
		}
		if h.evaluation != nil {
			r.Get("/api/evaluation", h.evaluationReport)
			r.Post("/api/evaluation/labels", h.labelCall)
		}
		if h.pipeline != nil {
			r.Get("/api/crm/pipeline/config", h.getPipelineConfig)
			r.Put("/api/crm/pipeline/config", h.putPipelineConfig)
			r.Get("/api/crm/opportunities", h.listOpportunities)
			r.Post("/api/crm/opportunities", h.openOpportunity)
			r.Get("/api/crm/opportunities/{id}", h.getOpportunity)
			r.Post("/api/crm/opportunities/{id}/stage", h.setOpportunityStage)
			r.Post("/api/crm/opportunities/{id}/value", h.setOpportunityValue)
			r.Post("/api/crm/opportunities/{id}/assign", h.assignOpportunity)
			r.Post("/api/crm/interactions", h.recordInteraction)
			r.Get("/api/crm/tasks", h.listTasks)
			r.Post("/api/crm/tasks", h.createTask)
			r.Post("/api/crm/tasks/{id}/complete", h.completeTask)
			r.Get("/api/crm/imports", h.listImports)
			r.Post("/api/crm/imports/preview", h.previewImport)
			r.Get("/api/crm/imports/{id}", h.getImport)
			r.Post("/api/crm/imports/{id}/apply", h.applyImport)
			r.Get("/api/crm/export", h.exportPipeline)
		}
		if h.registry != nil {
			r.Get("/api/agents/capabilities", h.capabilities)
			r.Get("/api/registry/{kind}", h.listRegistry)
			r.Post("/api/skills", h.saveSkill)
			r.Post("/api/skills/{key}/check-output", h.checkSkillOutput)
			r.Post("/api/agents", h.saveAgent)
			r.Post("/api/workflows/definitions", h.saveWorkflow)
			r.Post("/api/workflows/definitions/validate", h.validateWorkflow)
			r.Post("/api/registry/{id}/activate", h.activateRegistry)
			r.Post("/api/registry/{id}/retire", h.retireRegistry)
		}
		if h.workflows != nil {
			r.Get("/api/workflows/runs", h.listWorkflowRuns)
			r.Post("/api/workflows/runs", h.startWorkflowRun)
			r.Post("/api/workflows/tick", h.tickWorkflows)
			r.Get("/api/workflows/runs/{id}", h.workflowGraph)
			r.Post("/api/workflows/runs/{id}/pause", h.workflowTransition("pause"))
			r.Post("/api/workflows/runs/{id}/resume", h.workflowTransition("resume"))
			r.Post("/api/workflows/runs/{id}/cancel", h.workflowTransition("cancel"))
			r.Post("/api/workflows/tasks/{id}/retry", h.retryWorkflowTask)
			r.Post("/api/workflows/tasks/{id}/correct", h.correctWorkflowTask)
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
