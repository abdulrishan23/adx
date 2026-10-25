import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type {
  Account,
  AccountsResponse,
  CapabilityMatrixResponse,
  KillSwitch,
  KillSwitchInput,
  Policy,
  PolicyDocument,
  PolicyState,
} from "@/api/types.gen";
import { getJSON } from "./client";

// Enabled accounts for everyone; with all, the admin's full list including
// the ones that are off. Only an admin may ask for all.
export function useAccounts(all = false) {
  return useQuery({
    queryKey: ["accounts", all ? "all" : "enabled"],
    queryFn: () => getJSON<AccountsResponse>(all ? "/api/accounts?all=1" : "/api/accounts"),
  });
}

export function useSetAccountEnabled() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ accountId, enabled }: { accountId: number; enabled: boolean }) =>
      putJSON<Account>(`/api/accounts/${accountId}/enabled`, { enabled }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["accounts"] }),
  });
}

export function usePolicies() {
  return useQuery({ queryKey: ["policies"], queryFn: () => getJSON<PolicyState>("/api/policies") });
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

export function usePutPolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (document: PolicyDocument) => putJSON<Policy>("/api/policies", { document }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["policies"] }),
  });
}

export function useKillSwitch() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: KillSwitchInput) => putJSON<KillSwitch>("/api/policies/kill-switch", input),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["policies"] }),
  });
}

export function useCapabilityMatrix() {
  return useQuery({
    queryKey: ["accounts", "capabilities"],
    queryFn: () => getJSON<CapabilityMatrixResponse>("/api/connections/capabilities"),
  });
}
