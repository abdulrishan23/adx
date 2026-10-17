import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type {
  Deployment,
  PageDetail,
  PageInput,
  PageReadiness,
  PagesResponse,
  PageVersion,
  PageVersionInput,
  SubmissionsResponse,
} from "./types.gen";

export function usePages() {
  return useQuery({ queryKey: ["pages"], queryFn: () => getJSON<PagesResponse>("/api/pages") });
}

export function usePage(id: number | null) {
  return useQuery({
    queryKey: ["pages", id],
    queryFn: () => getJSON<PageDetail>(`/api/pages/${id}`),
    enabled: id !== null,
  });
}

export function usePageReadiness(id: number | null) {
  return useQuery({
    queryKey: ["pages", id, "readiness"],
    queryFn: () => getJSON<PageReadiness>(`/api/pages/${id}/readiness`),
    enabled: id !== null,
  });
}

export function usePageSubmissions(id: number | null) {
  return useQuery({
    queryKey: ["pages", id, "submissions"],
    queryFn: () => getJSON<SubmissionsResponse>(`/api/pages/${id}/submissions?limit=50`),
    enabled: id !== null,
  });
}

export function usePagePreview(id: number | null, version: number | null) {
  return useQuery({
    queryKey: ["pages", id, "preview", version],
    queryFn: async () => {
      const res = await fetch(`/api/pages/${id}/versions/${version}/preview`);
      if (!res.ok) throw new Error(res.statusText);
      return res.text();
    },
    enabled: id !== null && version !== null,
  });
}

export function useCreatePage() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: PageInput) => postJSON<PageDetail>("/api/pages", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pages"] }),
  });
}

export function useSavePageVersion(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: PageVersionInput) =>
      postJSON<PageVersion>(`/api/pages/${id}/versions`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pages"] }),
  });
}

export function usePublishPage(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (version: number) => postJSON<Deployment>(`/api/pages/${id}/publish`, { version }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pages"] }),
  });
}

export function useRollbackPage(id: number | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => postJSON<Deployment>(`/api/pages/${id}/rollback`, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pages"] }),
  });
}

// A test submission goes through the public form endpoint, exactly as a
// visitor's would, carrying the page's test token so it is marked as a test.
export function useTestSubmission() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({
      slug,
      token,
      project,
      fields,
    }: {
      slug: string;
      token: string;
      project: number;
      fields: Record<string, string>;
    }) => {
      const body = new URLSearchParams({ ...fields, _project: String(project), _test: token });
      const res = await fetch(`/p/${slug}/form`, {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/x-www-form-urlencoded",
        },
        body,
      });
      const json = (await res.json().catch(() => null)) as {
        error?: string;
        reason?: string;
      } | null;
      if (!res.ok) throw new Error(json?.error ? `${json.error} (${json.reason})` : res.statusText);
      return json;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["pages"] }),
  });
}
