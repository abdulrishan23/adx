// Create's own systems: brand knowledge, research, media plans, landing pages
// and launch. They arrive in phases 2 and 3 (docs/screens.md); until then the
// screens run on api/mock/create.ts so the whole loop can be walked.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { mockResponse } from "./mock";
import * as mock from "./mock/create";
import type { Platform } from "./models";

export type FactStatus = "approved" | "conflict" | "proposed" | "outdated";
export type FactSource = {
  kind: "website" | "brochure" | "crm" | "sales" | "forms";
  label: string;
  seenAt: string;
};
export type Fact = {
  id: string;
  topic: string;
  text: string;
  source: FactSource;
  status: FactStatus;
  // Facts that disagree share a conflict key; approving one outdates the rest.
  conflict?: string;
};
export type BrandKnowledge = {
  projectId: number;
  name: string;
  capturedFrom: { kind: FactSource["kind"]; label: string; items: number }[];
  facts: Fact[];
  updatedAt: string;
};

export type Competitor = {
  name: string;
  positioning: string;
  priceFrom: string;
  adsSeen: number;
  channels: string[];
  note: string;
};
export type Keyword = {
  term: string;
  volume: number;
  cpcMinor: number;
  intent: "buy" | "rent" | "explore";
  included: boolean;
};
export type Audience = { name: string; size: string; why: string; platforms: string[] };
export type Research = {
  projectId: number;
  refreshedAt: string;
  nextRefreshAt: string;
  competitors: Competitor[];
  keywords: Keyword[];
  audiences: Audience[];
  gaps: string[];
};

export type PlanStatus = "draft" | "waiting" | "approved" | "superseded";
export type PlanChannel = {
  platform: Platform | "snapchat" | "tiktok";
  share: number;
  budgetMinor: number;
  expectedQualified: number;
  expectedCpqMinor: number;
};
export type Assumption = { id: string; text: string; source: string; confirmed: boolean };
export type Plan = {
  id: string;
  projectId: number;
  projectName: string;
  version: number;
  status: PlanStatus;
  budgetMinor: number;
  from: string;
  to: string;
  channels: PlanChannel[];
  weekly: { week: string; budgetMinor: number; note?: string }[];
  assumptions: Assumption[];
  kpis: { label: string; target: string }[];
  expectedQualified: number;
  expectedCpqMinor: number;
  changes: string;
  author: string;
  createdAt: string;
  decidedAt?: string;
  decidedBy?: string;
};

export type PageStatus = "draft" | "live" | "broken";
export type BuiltPage = {
  id: string;
  projectId: number;
  projectName: string;
  title: string;
  url: string;
  status: PageStatus;
  factsUsed: number;
  form: string;
  language: "en" | "ar";
  updatedAt: string;
};

export type LaunchStep = {
  label: string;
  status: "ready" | "caution" | "error" | "pending";
  at?: string;
  note?: string;
};
export type Blocker = {
  id: string;
  platform: Platform;
  target: string;
  reason: string;
  fix: string;
  fixed: boolean;
};
export type Launch = {
  projectId: number;
  projectName: string;
  planId: string | null;
  readiness: { label: string; ok: boolean; note: string }[];
  steps: LaunchStep[];
  blockers: Blocker[];
};

// Where a brand stands on each step of the loop, for the Brands list.
export type LoopState = {
  projectId: number;
  steps: Record<
    "brand" | "research" | "plan" | "creative" | "page" | "launch",
    "ready" | "caution" | "error" | "pending"
  >;
  note: string;
};

const KEY = ["create"];

// Starting a brand is the CEO's first step: say what the brand is, who should
// look at it and with which skills, then Adx sets up the project and works
// through brand analysis, the audit and research to a first media plan. The
// brand itself is created for real (api/projects); the setup run is sample
// until phase 2.
export type Analyst = "performance" | "media" | "brand" | "seo" | "creative" | "crm";

export type SetupInput = {
  projectId: number;
  brief: string;
  results: string;
  website: string;
  elsewhere: string;
  // False for a new brand: the audit uses outside data only.
  hasHistory: boolean;
  analysts: Analyst[];
  skillsFile: string | null;
  templateFile: string | null;
};

export type SetupStepId = "analysis" | "audit" | "market" | "competitors" | "seo" | "plan";
export type SetupStep = {
  id: SetupStepId;
  label: string;
  detail: string;
  state: "done" | "working" | "queued";
};

export type BrandSetup = SetupInput & { startedAt: string; steps: SetupStep[]; done: boolean };

export function useBrandSetup(projectId: number) {
  return useQuery({
    queryKey: [...KEY, "setup", projectId],
    queryFn: () => mockResponse(() => mock.brandSetup(projectId)),
    // While Adx is still working through the steps, look again every few
    // seconds so the page moves on its own.
    refetchInterval: (q) => (q.state.data && !q.state.data.done ? 3000 : false),
  });
}

export function useStartSetup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: SetupInput) => mockResponse(() => mock.startSetup(input), 400),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}

export function useLoopStates() {
  return useQuery({ queryKey: [...KEY, "loop"], queryFn: () => mockResponse(mock.loopStates) });
}

export function useBrandKnowledge(projectId: number | null) {
  return useQuery({
    queryKey: [...KEY, "brand", projectId],
    queryFn: () => mockResponse(() => mock.brand(projectId as number)),
    enabled: projectId !== null,
  });
}

export function useApproveFact() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { projectId: number; factId: string }) =>
      mockResponse(() => mock.approveFact(input.projectId, input.factId), 300),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}

export function useResearch(projectId: number | null) {
  return useQuery({
    queryKey: [...KEY, "research", projectId],
    queryFn: () => mockResponse(() => mock.research(projectId as number)),
    enabled: projectId !== null,
  });
}

export function useToggleKeyword() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { projectId: number; term: string }) =>
      mockResponse(() => mock.toggleKeyword(input.projectId, input.term), 150),
    onSuccess: () => qc.invalidateQueries({ queryKey: [...KEY, "research"] }),
  });
}

export function useRefreshResearch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (projectId: number) => mockResponse(() => mock.refreshResearch(projectId), 1800),
    onSuccess: () => qc.invalidateQueries({ queryKey: [...KEY, "research"] }),
  });
}

export function usePlans() {
  return useQuery({ queryKey: [...KEY, "plans"], queryFn: () => mockResponse(mock.plans) });
}

export function usePlan(id: string) {
  return useQuery({
    queryKey: [...KEY, "plan", id],
    queryFn: () => mockResponse(() => mock.plan(id)),
  });
}

export function useConfirmAssumption() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { planId: string; assumptionId: string; confirmed: boolean }) =>
      mockResponse(
        () => mock.confirmAssumption(input.planId, input.assumptionId, input.confirmed),
        150,
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}

export function useDecidePlan() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { planId: string; decision: "approve" | "changes"; note?: string }) =>
      mockResponse(() => mock.decidePlan(input.planId, input.decision, input.note), 400),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}

export function useBuiltPages() {
  return useQuery({ queryKey: [...KEY, "pages"], queryFn: () => mockResponse(mock.pages) });
}

export function useLaunches() {
  return useQuery({ queryKey: [...KEY, "launches"], queryFn: () => mockResponse(mock.launches) });
}

export function useFixBlocker() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { projectId: number; blockerId: string }) =>
      mockResponse(() => mock.fixBlocker(input.projectId, input.blockerId), 900),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  });
}
