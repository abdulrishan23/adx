// Launch, straight from the engine: a plan version's readiness across plan,
// creatives, pages, accounts and measurement; where its money goes, line by
// line, before anything is submitted; the intents sent to each platform and
// what came back; and the milestones from acceptance to the first delivered
// day. Readiness, status and milestones are re-exported from the engine's
// clients so both screens share one cache while both exist. The resolution,
// picks and submission have no engine screen, so their hooks live here.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { LaunchPickInput, LaunchResolution, SubmitInput, SubmitResult } from "@/api/types.gen";
import { getJSON, postJSON } from "@/engine/api/client";

export {
  useExecutionAction,
  useExecutionCapabilities,
  useLaunchStatus,
  useMilestones,
} from "@/engine/api/execution";
export { useLaunchReadiness, useMeasurementChecks } from "@/engine/api/measurement";

const resolutionKey = (planId: number | null, version: number | null) => [
  "launch",
  "resolution",
  planId,
  version,
];

// What launching the version would do today: each line as create, update,
// resume, pick or unsupported, with this month's spend netted off (BD-68).
// The server resolves it; nothing is decided in the browser.
export function useLaunchResolution(planId: number | null, version: number | null) {
  return useQuery({
    queryKey: resolutionKey(planId, version),
    queryFn: () =>
      getJSON<LaunchResolution>(`/api/launch/resolution?plan_id=${planId}&version=${version}`),
    enabled: planId !== null && version !== null,
  });
}

// A person's choice for a line that matched more than one campaign. The
// server answers with the resolution the pick applies to, which replaces
// the cached one at once.
export function usePickCampaign() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: LaunchPickInput) => postJSON<LaunchResolution>("/api/launch/picks", input),
    onSuccess: (res) => {
      qc.setQueryData(resolutionKey(res.planId, res.planVersion), res);
      qc.invalidateQueries({ queryKey: resolutionKey(res.planId, res.planVersion) });
    },
  });
}

// Writes the resolution out as one proposal per line under the version's
// envelopes. What policy allows is executed into the outbox; the rest waits
// in Perform, is denied, or is held with the reason.
export function useSubmitLaunch() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: SubmitInput) => postJSON<SubmitResult>("/api/launch/submit", input),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: resolutionKey(res.planId, res.planVersion) });
      qc.invalidateQueries({ queryKey: ["executions"] });
      qc.invalidateQueries({ queryKey: ["proposals"] });
      qc.invalidateQueries({ queryKey: ["envelopes"] });
    },
  });
}
