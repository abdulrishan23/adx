import { type AnalyticsSearch, rangeError } from "@/lib/analytics";
import type { ProjectPerformance } from "@/lib/project-performance";
import { projectPerformance } from "./mock/project-performance";
// Data hooks for the Adx screens. Each hook names the endpoint it will call.
// While USE_MOCK is on it answers from the mock server; switching a screen to
// real data means the Go endpoint exists and returns this same shape.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useScope } from "@/app/scope";
import { ApiError, getJSON, postForm, postJSON } from "./client";
import { isLive, mockResponse, USE_MOCK } from "./mock";
import * as actions from "./mock/actions";
import * as ask from "./mock/ask";
import * as briefMock from "./mock/brief";
import * as crm from "./mock/crm";
import { PROJECTS } from "./mock/data";
import * as enrich from "./mock/enrich";
import * as more from "./mock/more";
import * as ops from "./mock/ops";
import * as mock from "./mock/server";
import * as sources from "./mock/sources";
import type {
  AdRow,
  Alert,
  AskAnswer,
  Audience,
  Brief,
  Campaign,
  CandidateFilter,
  ChangeRequest,
  Connection,
  Conversation,
  DataQuality,
  EnrichmentJob,
  EnrichmentQueue,
  Funnel,
  Guardrails,
  LandingPage,
  Lead,
  LeadEnrichment,
  LeadsSummary,
  Lists,
  MarketerRow,
  MarketRow,
  Organic,
  Outcomes,
  PermissionMode,
  Person,
  PortalRow,
  Project,
  ProjectSummary,
  Proposal,
  Report,
  Revenue,
  Scope,
  Scoring,
  SourceDetail,
  WriteAction,
} from "./models";
import type {
  ConnectionsResponse,
  Connection as CrmConnection,
  LeadList,
  LeadSync,
  ProjectsResponse,
} from "./types.legacy";

function qs(scope: Scope) {
  return new URLSearchParams({
    days: String(scope.days),
    platform: scope.platform,
    project: String(scope.projectId),
  }).toString();
}

function get<T>(path: string, answer: () => T): Promise<T> {
  return USE_MOCK ? mockResponse(answer) : getJSON<T>(path);
}

// The scope bar's project list. Uses the real projects endpoint when mocks are
// off; it already exists in Go.
export function useProjectsList() {
  return useQuery({
    queryKey: ["projects-list"],
    queryFn: async (): Promise<Pick<Project, "id" | "name">[]> =>
      USE_MOCK
        ? mockResponse(() => PROJECTS)
        : (await getJSON<ProjectsResponse>("/api/projects")).projects,
    staleTime: 60_000,
  });
}

// Project performance uses the same reporting dates as Analytics.
export function useProjectsOverview(search: AnalyticsSearch) {
  const { metric: _, project: _project, platform: _platform, ...dates } = search;
  return useQuery({
    queryKey: ["projects-overview", dates],
    enabled: !rangeError(search),
    queryFn: () =>
      get<ProjectPerformance[]>(
        `/api/projects/overview?${new URLSearchParams(
          Object.entries(dates)
            .filter(([, v]) => v !== undefined)
            .map(([k, v]) => [k, String(v)]),
        )}`,
        () => projectPerformance(search),
      ),
  });
}

// The Brief remains a fixed daily snapshot.
export function useBrief() {
  return useQuery({
    queryKey: ["brief"],
    queryFn: () => get<Brief>("/api/brief", () => briefMock.brief()),
  });
}

export function useCampaigns() {
  const scope = useScope();
  return useQuery({
    queryKey: ["campaigns", scope],
    queryFn: () => get<Campaign[]>(`/api/campaigns?${qs(scope)}`, () => mock.campaigns(scope)),
  });
}

// Where leads live for the screen's scope, from the phone country code.
export function useMarkets() {
  const scope = useScope();
  return useQuery({
    queryKey: ["markets", scope],
    queryFn: () => get<MarketRow[]>(`/api/markets?${qs(scope)}`, () => mock.markets(scope)),
  });
}

export function useFunnel() {
  const scope = useScope();
  return useQuery({
    queryKey: ["funnel", scope],
    queryFn: () => get<Funnel>(`/api/funnel?${qs(scope)}`, () => mock.funnel(scope)),
  });
}

export function useLeads() {
  const scope = useScope();
  return useQuery({
    queryKey: ["leads", scope],
    queryFn: () => get<Lead[]>(`/api/leads?${qs(scope)}`, () => mock.leads(scope)),
  });
}

export function useLeadsSummary() {
  const scope = useScope();
  return useQuery({
    queryKey: ["leads-summary", scope],
    queryFn: () =>
      get<LeadsSummary>(`/api/leads/summary?${qs(scope)}`, () => mock.leadsSummary(scope)),
  });
}

export function useLead(id: number) {
  return useQuery({
    queryKey: ["lead", id],
    queryFn: () => get<Lead | null>(`/api/leads/${id}`, () => mock.lead(id)),
  });
}

export function useProposals() {
  return useQuery({
    queryKey: ["proposals"],
    queryFn: () => get<Proposal[]>("/api/proposals", () => mock.proposals()),
    // Poll while a change is being applied, so its result shows without a reload.
    refetchInterval: (q) => (q.state.data?.some((p) => p.status === "applying") ? 1500 : false),
  });
}

function useProposalAction(action: "retry" | "undo") {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      USE_MOCK
        ? mockResponse(() => (action === "retry" ? mock.retry(id) : mock.undo(id)), 500)
        : postJSON<Proposal>(`/api/proposals/${id}/${action}`, {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["proposals"] });
      queryClient.invalidateQueries({ queryKey: ["proposal"] });
    },
  });
}

export const useRetryProposal = () => useProposalAction("retry");
export const useUndoProposal = () => useProposalAction("undo");

export function useDecideProposal() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { id: string; decision: "approve" | "reject"; note?: string }) =>
      USE_MOCK
        ? mockResponse(() => mock.decide(input.id, input.decision, input.note), 500)
        : postJSON<Proposal>(`/api/proposals/${input.id}/${input.decision}`, { note: input.note }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["proposals"] });
      queryClient.invalidateQueries({ queryKey: ["brief"] });
      queryClient.invalidateQueries({ queryKey: ["proposal"] });
    },
  });
}

export function useAlerts() {
  return useQuery({
    queryKey: ["alerts"],
    queryFn: () => get<Alert[]>("/api/alerts", () => mock.alerts()),
  });
}

export function useConnections() {
  return useQuery({
    queryKey: ["connections"],
    queryFn: async () => {
      const list = await get<Connection[]>("/api/connections", () => mock.connections());
      return isLive("crm") ? Promise.all(list.map(withLiveCrm)) : list;
    },
  });
}

// The GoHighLevel card reads the real workspace list, so it shows what the
// server actually has rather than the fixture.
async function withLiveCrm(c: Connection): Promise<Connection> {
  if (c.key !== "ghl") return c;
  try {
    const { connections } = await getJSON<ConnectionsResponse>("/api/connections/crm");
    if (connections.length === 0)
      return {
        ...c,
        state: "pending",
        stateLabel: "Not set up",
        detail: "No workspace connected.",
        accounts: [],
        lastSyncAt: null,
      };
    const checked = connections.map((x) => x.verifiedAt).sort();
    return {
      ...c,
      state: "ready",
      stateLabel: "Connected",
      detail: `${connections.length} workspace${connections.length > 1 ? "s" : ""}, read access checked.`,
      accounts: connections.map((x) => x.name),
      lastSyncAt: checked[checked.length - 1] ?? null,
    };
  } catch (e) {
    const status = e instanceof ApiError ? e.status : 0;
    return {
      ...c,
      state: status === 409 ? "pending" : "error",
      stateLabel: status === 409 ? "Needs a key" : "Unreachable",
      detail: e instanceof Error ? e.message : "Couldn't read the server.",
      accounts: [],
      lastSyncAt: null,
    };
  }
}

export function useDataQuality() {
  return useQuery({
    queryKey: ["data-quality"],
    queryFn: () => get<DataQuality>("/api/data-quality", () => more.dataQuality()),
  });
}

export function useMarketers() {
  const scope = useScope();
  return useQuery({
    queryKey: ["marketers", scope],
    queryFn: () => get<MarketerRow[]>(`/api/marketers?${qs(scope)}`, () => more.marketers()),
  });
}

export function usePortals() {
  return useQuery({
    queryKey: ["portals"],
    queryFn: () => get<PortalRow[]>("/api/portals", () => more.portalRows()),
  });
}

export function useSetPortalSpend() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { key: string; spendMinor: number | null }) =>
      USE_MOCK
        ? mockResponse(() => more.setPortalSpend(input.key, input.spendMinor), 400)
        : postJSON<PortalRow>(`/api/portals/${input.key}/spend`, { spendMinor: input.spendMinor }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["portals"] }),
  });
}

export function useAudiences() {
  return useQuery({
    queryKey: ["audiences"],
    queryFn: () => get<Audience[]>("/api/audiences", () => more.AUDIENCES),
  });
}

export function useWriteActions() {
  return useQuery({
    queryKey: ["write-actions"],
    queryFn: () => get<WriteAction[]>("/api/settings/write-actions", () => more.WRITE_ACTIONS),
    staleTime: Number.POSITIVE_INFINITY,
  });
}

export function usePeople() {
  return useQuery({
    queryKey: ["people"],
    queryFn: () => get<Person[]>("/api/people", () => more.PEOPLE),
  });
}

export function useUpdatePerson() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: {
      id: number;
      role?: Person["role"];
      override?: { action: string; mode: PermissionMode | null };
    }) =>
      USE_MOCK
        ? mockResponse(() => {
            const person = more.PEOPLE.find((p) => p.id === input.id);
            if (!person) throw new Error("That person no longer exists");
            if (input.role) person.role = input.role;
            if (input.override) {
              if (input.override.mode === null) delete person.overrides[input.override.action];
              else person.overrides[input.override.action] = input.override.mode;
            }
            return person;
          }, 300)
        : postJSON<Person>(`/api/people/${input.id}`, input),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["people"] }),
  });
}

export function useLists() {
  return useQuery({
    queryKey: ["lists"],
    queryFn: () => get<Lists>("/api/settings/lists", () => more.lists()),
  });
}

export function useSetStage() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { status: string; stage: string }) =>
      USE_MOCK
        ? mockResponse(() => more.setStage(input.status, input.stage), 300)
        : postJSON(`/api/settings/stages`, input),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["lists"] }),
  });
}

export function useLandingPages() {
  const scope = useScope();
  return useQuery({
    queryKey: ["landing-pages", scope],
    queryFn: () => get<LandingPage[]>(`/api/website/pages?${qs(scope)}`, () => more.LANDING_PAGES),
  });
}

export function useOrganic() {
  const scope = useScope();
  return useQuery({
    queryKey: ["organic", scope],
    queryFn: () => get<Organic>(`/api/organic?${qs(scope)}`, () => more.organic()),
  });
}

export function useReports() {
  return useQuery({
    queryKey: ["reports"],
    queryFn: () => get<Report[]>("/api/reports", () => more.REPORTS),
  });
}

export function useConversations() {
  return useQuery({
    queryKey: ["conversations"],
    queryFn: () => get<Conversation[]>("/api/ask/conversations", () => ask.CONVERSATIONS),
  });
}

// Asking is a mutation: the answer belongs to the conversation it was asked in,
// so the page keeps the thread and the hook only fetches one answer.
export function useAsk() {
  return useMutation({
    mutationFn: (input: { conversationId: string | null; question: string }) =>
      USE_MOCK
        ? mockResponse(() => ask.ask(input.question), 1600)
        : postJSON<AskAnswer>("/api/ask", input),
  });
}

export function useLeadEnrichment(id: number) {
  return useQuery({
    queryKey: ["enrichment", id],
    queryFn: () =>
      get<LeadEnrichment | null>(`/api/leads/${id}/enrichment`, () => enrich.enrichment(id)),
  });
}

export function useRequestEnrichment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      USE_MOCK
        ? mockResponse(() => enrich.requestEnrichment(id), 400)
        : postJSON<LeadEnrichment>(`/api/leads/${id}/enrich`, {}),
    onSuccess: (_, id) => {
      queryClient.invalidateQueries({ queryKey: ["enrichment", id] });
      queryClient.invalidateQueries({ queryKey: ["enrichment-queue"] });
    },
  });
}

export function useEnrichmentQueue() {
  return useQuery({
    queryKey: ["enrichment-queue"],
    queryFn: () => get<EnrichmentQueue>("/api/enrichment", () => enrich.enrichmentQueue()),
  });
}

export function useCandidateCount(filter: CandidateFilter) {
  return useQuery({
    queryKey: ["enrichment-candidates", filter],
    queryFn: () =>
      get<number>(
        `/api/enrichment/candidates?${new URLSearchParams(Object.entries(filter).map(([k, v]) => [k, String(v)]))}`,
        () => enrich.countCandidates(filter),
      ),
    placeholderData: (prev) => prev,
  });
}

export function useStartEnrichment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { filter: CandidateFilter; limit: number; label: string }) =>
      USE_MOCK
        ? mockResponse(() => enrich.startJob(input.filter, input.limit, input.label), 500)
        : postJSON<EnrichmentJob>("/api/enrichment/jobs", input),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["enrichment-queue"] }),
  });
}

export function usePauseEnrichment() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (paused: boolean) =>
      USE_MOCK
        ? mockResponse(() => enrich.setPaused(paused), 300)
        : postJSON<EnrichmentQueue>("/api/enrichment/pause", { paused }),
    onSuccess: (data) => queryClient.setQueryData(["enrichment-queue"], data),
  });
}

export function useOutcomes() {
  return useQuery({
    queryKey: ["outcomes"],
    queryFn: () => get<Outcomes>("/api/outcomes", () => ops.outcomes()),
  });
}

export function useGuardrails() {
  return useQuery({
    queryKey: ["guardrails"],
    queryFn: () => get<Guardrails>("/api/settings/guardrails", () => ops.guardrails()),
  });
}

export function useUpdateGuardrails() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: Parameters<typeof ops.updateGuardrails>[0]) =>
      USE_MOCK
        ? mockResponse(() => ops.updateGuardrails(input), 400)
        : postJSON<Guardrails>("/api/settings/guardrails", input),
    onSuccess: (data) => queryClient.setQueryData(["guardrails"], data),
  });
}

export function useScoring() {
  return useQuery({
    queryKey: ["scoring"],
    queryFn: () => get<Scoring>("/api/settings/scoring", () => ops.scoringSettings()),
  });
}

export function useUpdateScoring() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: Parameters<typeof ops.updateScoring>[0]) =>
      USE_MOCK
        ? mockResponse(() => ops.updateScoring(input), 300)
        : postJSON<Scoring>("/api/settings/scoring", input),
    onSuccess: (data) => queryClient.setQueryData(["scoring"], data),
  });
}

export function useRevenue() {
  return useQuery({
    queryKey: ["revenue"],
    queryFn: () => get<Revenue>("/api/revenue", () => ops.revenue()),
  });
}

export function useAds(campaignId: string) {
  return useQuery({
    queryKey: ["ads", campaignId],
    queryFn: () => get<AdRow[]>(`/api/campaigns/${campaignId}/ads`, () => ops.ads(campaignId)),
  });
}

export function useProposal(id: string) {
  return useQuery({
    queryKey: ["proposal", id],
    queryFn: () =>
      get<Proposal | null>(
        `/api/proposals/${id}`,
        () => mock.proposals().find((p) => p.id === id) ?? null,
      ),
  });
}

// GoHighLevel workspaces, served by the Go server (core/connections).
export function useCrmConnections() {
  return useQuery({
    queryKey: ["crm-connections"],
    queryFn: async () =>
      isLive("crm")
        ? (await getJSON<ConnectionsResponse>("/api/connections/crm")).connections
        : mockResponse(() => crm.listCrm()),
  });
}

export function useConnectCrm() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { locationId: string; token: string }) =>
      isLive("crm")
        ? postForm<CrmConnection>("/api/connections/crm/private", input)
        : mockResponse(() => crm.connectPrivate(input.locationId, input.token), 900),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["crm-connections"] });
      queryClient.invalidateQueries({ queryKey: ["connections"] });
    },
  });
}

export function useCheckCrm() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (locationId: string) =>
      isLive("crm")
        ? postJSON<CrmConnection>(
            `/api/connections/crm/${encodeURIComponent(locationId)}/check`,
            {},
          )
        : mockResponse(() => crm.checkCrm(locationId), 700),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["crm-connections"] });
      queryClient.invalidateQueries({ queryKey: ["connections"] });
    },
  });
}

// The local copy of a workspace's GoHighLevel leads (core/leads).
export function useCrmLeads(locationId: string) {
  return useQuery({
    queryKey: ["crm-leads", locationId],
    queryFn: () =>
      isLive("crm")
        ? getJSON<LeadList>(`/api/leads?location=${encodeURIComponent(locationId)}&limit=25`)
        : mockResponse(() => crm.listLeads(locationId)),
    // Poll while a sync runs, so its result shows when it finishes.
    refetchInterval: (q) => (q.state.data?.lastSync?.state === "running" ? 1500 : false),
  });
}

export function useSyncCrmLeads() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (locationId: string) =>
      isLive("crm")
        ? postJSON<LeadSync>(`/api/connections/crm/${encodeURIComponent(locationId)}/sync`, {})
        : mockResponse(() => crm.startLeadSync(locationId), 300),
    onSuccess: (_, locationId) =>
      queryClient.invalidateQueries({ queryKey: ["crm-leads", locationId] }),
  });
}

export function useDismissAlert() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { id: string; reason: string }) =>
      USE_MOCK
        ? mockResponse(() => mock.dismissAlert(input.id, input.reason), 300)
        : postJSON(`/api/alerts/${input.id}/dismiss`, { reason: input.reason }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alerts"] });
      // Alerts arrive in the Inbox as cards too.
      queryClient.invalidateQueries({ queryKey: ["home-desk"] });
    },
  });
}

export function useSnoozeAlert() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { id: string; days: number }) =>
      USE_MOCK
        ? mockResponse(() => mock.snoozeAlert(input.id, input.days), 300)
        : postJSON(`/api/alerts/${input.id}/snooze`, { days: input.days }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alerts"] });
      // Alerts arrive in the Inbox as cards too.
      queryClient.invalidateQueries({ queryKey: ["home-desk"] });
    },
  });
}

export function useDailyBudget(campaignId: string) {
  return useQuery({
    queryKey: ["daily-budget", campaignId],
    queryFn: () =>
      get<number>(`/api/campaigns/${campaignId}/budget`, () => actions.dailyBudget(campaignId)),
  });
}

export function useProposeChange() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ChangeRequest) =>
      USE_MOCK
        ? mockResponse(() => actions.proposeChange(input), 600)
        : postJSON<Proposal>("/api/proposals", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["proposals"] });
      queryClient.invalidateQueries({ queryKey: ["brief"] });
      queryClient.invalidateQueries({ queryKey: ["guardrails"] });
      queryClient.invalidateQueries({ queryKey: ["ads"] });
      queryClient.invalidateQueries({ queryKey: ["daily-budget"] });
    },
  });
}

export function useProjectSummary(id: number) {
  const scope = useScope();
  return useQuery({
    queryKey: ["project-summary", id, scope.days],
    queryFn: () =>
      get<ProjectSummary | null>(`/api/projects/${id}/summary?days=${scope.days}`, () =>
        actions.projectSummary(id, scope.days),
      ),
  });
}

export function useSource(key: string) {
  return useQuery({
    queryKey: ["source", key],
    queryFn: () => get<SourceDetail | null>(`/api/connections/${key}`, () => sources.source(key)),
  });
}

function useSourceAction<I>(path: string, run: (key: string, input: I) => SourceDetail) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (v: { key: string; input: I }) =>
      USE_MOCK
        ? mockResponse(() => run(v.key, v.input), 700)
        : postJSON<SourceDetail>(`/api/connections/${v.key}/${path}`, v.input ?? {}),
    onSuccess: (data, v) => {
      queryClient.setQueryData(["source", v.key], data);
      queryClient.invalidateQueries({ queryKey: ["connections"] });
    },
  });
}

export const useSignInSource = () => useSourceAction<null>("start", (k) => sources.signIn(k));
export const useSetSourceAccounts = () =>
  useSourceAction<string[]>("accounts", (k, ids) => sources.setAccounts(k, ids));
export const useSyncSource = () => useSourceAction<null>("sync", (k) => sources.syncNow(k));
export const useDisconnectSource = () =>
  useSourceAction<null>("disconnect", (k) => sources.disconnect(k));
export const useSaveSourceKey = () =>
  useSourceAction<string>("key", (k, apiKey) => sources.saveKey(k, apiKey));
