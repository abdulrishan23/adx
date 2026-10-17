import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "./client";
import type { SessionResponse } from "./types.gen";

// null means "not signed in" (401). Any other failure is an error.
async function fetchSession(): Promise<SessionResponse | null> {
  const res = await fetch("/api/session", { headers: { Accept: "application/json" } });
  if (res.status === 401) return null;
  if (!res.ok) throw new ApiError(res.status, res.statusText);
  return (await res.json()) as SessionResponse;
}

export function useSession() {
  return useQuery({ queryKey: ["session"], queryFn: fetchSession, retry: false });
}

export function signInUrl(returnTo: string): string {
  return `/api/auth/login?return_to=${encodeURIComponent(returnTo)}`;
}

export function useSignOut() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const res = await fetch("/api/auth/logout", { method: "POST" });
      if (!res.ok && res.status !== 204) throw new ApiError(res.status, res.statusText);
    },
    onSuccess: () => queryClient.clear(),
  });
}
