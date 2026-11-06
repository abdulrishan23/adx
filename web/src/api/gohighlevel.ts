// The GoHighLevel connection (core/connections) and the import of its
// contacts (core/leads). Always the Go server: it answers 409 while it has
// no connection key, which the screen turns into the setup note.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getJSON, postForm, postJSON } from "./client";
import type { Connection, ConnectionsResponse } from "./types.gen";

// The scopes a private integration token must carry for ADX to read a
// workspace (docs/gohighlevel-development.md).
export const REQUIRED_SCOPES = [
  "contacts.readonly",
  "locations.readonly",
  "locations/customFields.readonly",
  "opportunities.readonly",
];

const KEY = ["crm-connections"] as const;

export function useCrmConnections() {
  return useQuery({
    queryKey: KEY,
    queryFn: async () => (await getJSON<ConnectionsResponse>("/api/connections/crm")).connections,
    // A 409 (no connection key) is a state, not something to retry.
    retry: false,
  });
}

function useConnectionMutation<I>(fn: (input: I) => Promise<Connection>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: KEY });
      qc.invalidateQueries({ queryKey: ["connections"] });
      qc.invalidateQueries({ queryKey: ["accounts"] });
    },
  });
}

// The server takes the token as a form, never as JSON, so it is not logged
// by accident with a request body.
export const useConnectCrm = () =>
  useConnectionMutation((input: { locationId: string; token: string }) =>
    postForm<Connection>("/api/connections/crm/private", input),
  );

export const useCheckCrm = () =>
  useConnectionMutation((locationId: string) =>
    postJSON<Connection>(`/api/connections/crm/${encodeURIComponent(locationId)}/check`, {}),
  );
