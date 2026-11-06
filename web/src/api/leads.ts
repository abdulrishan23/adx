// Leads as GoHighLevel last gave them, from the engine (core/leads). Always
// the Go server: a mocked lead list would look like Reportage's real one.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, getJSON, postJSON } from "./client";
import type {
  Coverage,
  ImportLeadsInput,
  Lead,
  LeadDetail,
  LeadsResponse,
  StartRunResponse,
} from "./types.gen";

const PAGE = 500;
// Enough for every lead Reportage has today; a workspace past it shows the
// first pages and the coverage figures still count every lead.
const MAX_PAGES = 40;

function query(params: Record<string, string | number | undefined>) {
  const search = new URLSearchParams();
  for (const [k, v] of Object.entries(params))
    if (v !== undefined && v !== "") search.set(k, String(v));
  const qs = search.toString();
  return qs ? `?${qs}` : "";
}

// The whole list, read page by page the way the server pages (after_id), so
// the screen can filter, search and count the way the design does.
export async function readAllLeads(accountId?: number): Promise<Lead[]> {
  const out: Lead[] = [];
  let after: number | null = null;
  for (let page = 0; page < MAX_PAGES; page++) {
    const res: LeadsResponse = await getJSON<LeadsResponse>(
      `/api/leads${query({ limit: PAGE, account_id: accountId, after_id: after ?? undefined })}`,
    );
    out.push(...res.leads);
    after = res.nextAfterId;
    if (after === null) break;
  }
  return out;
}

export function useLeads(accountId?: number) {
  return useQuery({
    queryKey: ["leads", "all", accountId ?? 0],
    queryFn: () => readAllLeads(accountId),
  });
}

export function useLeadCoverage(accountId?: number) {
  return useQuery({
    queryKey: ["leads", "coverage", accountId ?? 0],
    queryFn: () => getJSON<Coverage>(`/api/leads/coverage${query({ account_id: accountId })}`),
  });
}

// Null is a lead the server does not have: removed from GoHighLevel, or an
// id that never existed.
export function useLead(id: number) {
  return useQuery({
    queryKey: ["leads", "one", id],
    queryFn: async (): Promise<LeadDetail | null> => {
      try {
        return await getJSON<LeadDetail>(`/api/leads/${id}`);
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null;
        throw e;
      }
    },
  });
}

export function useImportLeads() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ImportLeadsInput) => postJSON<StartRunResponse>("/api/leads/import", input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["runs"] });
      queryClient.invalidateQueries({ queryKey: ["leads"] });
    },
  });
}
