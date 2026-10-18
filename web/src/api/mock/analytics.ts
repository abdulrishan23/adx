import { type AnalyticsCampaign, type AnalyticsSearch, buildAnalytics } from "@/lib/analytics";
import { CAMPAIGNS, PROJECTS } from "./data";

// Temporary sales fixtures for the analytics design. Spread each campaign's
// sample monthly deals over dated events so every filter uses the same data.
// These are illustrative property sale values, not collected cash or profit.
export const ANALYTICS_CAMPAIGNS: AnalyticsCampaign[] = CAMPAIGNS.map((c, index) => {
  const unitValue =
    c.deals > 0 && c.revenueMinor !== null ? Math.round(c.revenueMinor / c.deals) : 0;
  return {
    ...c,
    daily: c.daily.map((day) => {
      const n = Math.floor(Date.parse(day.date) / 86_400_000) + index * 7;
      const deals = Math.floor(((n + 1) * c.deals) / 30) - Math.floor((n * c.deals) / 30);
      return { ...day, deals, revenueMinor: deals * unitValue };
    }),
  };
});

export const analytics = (search: AnalyticsSearch) =>
  buildAnalytics(ANALYTICS_CAMPAIGNS, PROJECTS, search);
