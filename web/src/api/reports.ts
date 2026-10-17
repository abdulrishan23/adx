import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "./client";
import type {
  Alert,
  AlertConfig,
  AlertConfigState,
  AlertDocument,
  AlertEvaluation,
  AlertsResponse,
  GenerateReportResponse,
  Report,
  ReportSchedule,
  ReportScheduleInput,
  ReportsResponse,
  SchedulesResponse,
} from "./types.gen";

export function useSchedules() {
  return useQuery({
    queryKey: ["reports", "schedules"],
    queryFn: () => getJSON<SchedulesResponse>("/api/reports/schedules"),
  });
}

export function useReports() {
  return useQuery({
    queryKey: ["reports", "list"],
    queryFn: () => getJSON<ReportsResponse>("/api/reports"),
  });
}

export function useReport(id: number | null) {
  return useQuery({
    queryKey: ["reports", "one", id],
    queryFn: () => getJSON<Report>(`/api/reports/${id}`),
    enabled: id !== null,
  });
}

export function useCreateSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ReportScheduleInput) =>
      postJSON<ReportSchedule>("/api/reports/schedules", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["reports"] }),
  });
}

export function useGenerateScheduled() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) =>
      postJSON<GenerateReportResponse>(`/api/reports/schedules/${id}/generate`, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["reports"] }),
  });
}

export function useAlerts(state: string) {
  return useQuery({
    queryKey: ["alerts", state],
    queryFn: () => getJSON<AlertsResponse>(`/api/alerts${state ? `?state=${state}` : ""}`),
    refetchInterval: 60_000,
  });
}

export function useAlertConfig() {
  return useQuery({
    queryKey: ["alerts", "config"],
    queryFn: () => getJSON<AlertConfigState>("/api/alerts/config"),
  });
}

async function putJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const err = (await res.json().catch(() => null)) as { error?: string } | null;
    throw new Error(err?.error ?? res.statusText);
  }
  return (await res.json()) as T;
}

export function usePutAlertConfig() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (document: AlertDocument) =>
      putJSON<AlertConfig>("/api/alerts/config", { document }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alerts"] }),
  });
}

export function useAlertAction() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, action }: { id: number; action: "acknowledge" | "resolve" }) =>
      postJSON<Alert>(`/api/alerts/${id}/${action}`, {}),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alerts"] }),
  });
}

export function useEvaluateAlerts() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (day: string) => postJSON<AlertEvaluation>("/api/alerts/evaluate", { day }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["alerts"] }),
  });
}
