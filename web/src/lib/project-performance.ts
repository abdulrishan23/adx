import type { Pacing } from "@/api/models";
import type { AnalyticsTotals } from "./analytics";

export type ProjectPerformance = {
  projectId: number;
  name: string;
  current: AnalyticsTotals;
  previous: AnalyticsTotals;
  meetings: number | null;
  offers: number | null;
  qualifiedTrend: number[] | null;
  pacing: Pacing | null;
};
