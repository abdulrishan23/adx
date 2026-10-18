import { useQuery } from "@tanstack/react-query";
import { type Analytics, type AnalyticsSearch, rangeError } from "@/lib/analytics";
import { getJSON } from "./client";
import { mockResponse, USE_MOCK } from "./mock";
import { analytics } from "./mock/analytics";

export function useAnalytics(search: AnalyticsSearch) {
  const { metric: _, ...filters } = search;
  return useQuery({
    queryKey: ["analytics", filters],
    enabled: !rangeError(search),
    queryFn: () =>
      USE_MOCK
        ? mockResponse(() => analytics(search))
        : getJSON<Analytics>(
            `/api/analytics?${new URLSearchParams(
              Object.entries(filters)
                .filter(([, v]) => v !== undefined)
                .map(([k, v]) => [k, String(v)]),
            )}`,
          ),
  });
}
