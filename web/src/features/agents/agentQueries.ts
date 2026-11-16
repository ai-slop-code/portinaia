import { useQuery } from "@tanstack/react-query";
import { apiClient, sessionQueryKey } from "../../api/client";

/** Agent status is live operational state: poll only while its view is visible. */
export function useAgent(agentId?: string) {
  return useQuery({
    queryKey: sessionQueryKey("agent", agentId ?? ""),
    enabled: Boolean(agentId),
    queryFn: async ({ signal }) => {
      const { data, error } = await apiClient.GET("/agents/{agent_id}", {
        params: { path: { agent_id: agentId! } },
        signal,
      });
      if (error || !data)
        throw error ?? new Error("Could not load agent status.");
      return data;
    },
    refetchInterval: () =>
      document.visibilityState === "visible" ? 10_000 : false,
    refetchIntervalInBackground: false,
    retry: 2,
    retryDelay: (attempt) => Math.min(1_000 * 2 ** attempt, 15_000),
    // Query data remains available with isRefetchError after a failed refresh.
  });
}

export function useAgentList(status?: "online" | "offline" | "revoked") {
  return useQuery({
    queryKey: sessionQueryKey("agents", status ?? "all"),
    queryFn: async ({ signal }) => {
      const { data, error } = await apiClient.GET("/agents", {
        params: { query: { status, "page[size]": 50 } },
        signal,
      });
      if (error || !data) throw error ?? new Error("Could not load agents.");
      return data;
    },
  });
}
