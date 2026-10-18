import type { AnalyticsSearch } from "@/lib/analytics";
import type { ProjectPerformance } from "@/lib/project-performance";
import { analytics } from "./analytics";
import { brief } from "./brief";
import { PROJECTS } from "./data";

// Results use the same dated fixtures as Analytics. Meetings and offers keep
// the prototype's illustrative stage ratios until dated CRM events exist.
// Budget pacing is the current month's snapshot, independent of report dates.
export function projectPerformance(search: AnalyticsSearch): ProjectPerformance[] {
  const budgets = new Map(brief().projects.map((p) => [p.projectId, p.pacing]));
  return PROJECTS.map((p) => {
    const data = analytics({ ...search, platform: "all", project: p.id });
    const qualified = data.current.qualified;
    const meetings =
      qualified === null ? null : Math.max(data.current.deals ?? 0, Math.round(qualified * 0.4));
    const offers =
      meetings === null ? null : Math.max(data.current.deals ?? 0, Math.round(meetings * 0.4));
    return {
      projectId: p.id,
      name: p.name,
      current: data.current,
      previous: data.previous,
      meetings,
      offers,
      qualifiedTrend:
        data.availableDays === data.days ? data.daily.map((d) => d.current?.qualified ?? 0) : null,
      pacing: budgets.get(p.id) ?? null,
    };
  });
}
