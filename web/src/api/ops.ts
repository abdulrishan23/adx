import { useQuery } from "@tanstack/react-query";
import { ApiError } from "./client";
import type { Readiness } from "./types.gen";

// /api/ready answers 503 when a check fails, with the same body. The query
// treats both as data so the footer can show which check failed.
async function fetchReadiness(): Promise<Readiness> {
  const res = await fetch("/api/ready", { headers: { Accept: "application/json" } });
  if (res.status !== 200 && res.status !== 503) {
    throw new ApiError(res.status, res.statusText);
  }
  return (await res.json()) as Readiness;
}

export function useReadiness() {
  return useQuery({
    queryKey: ["ready"],
    queryFn: fetchReadiness,
    refetchInterval: 60_000,
    retry: false,
  });
}
