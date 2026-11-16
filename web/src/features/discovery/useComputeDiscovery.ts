import { useQuery } from "@tanstack/react-query";
import { apiClient, sessionQueryKey } from "../../api/client";
import type { components } from "../../api/generated";

type Container = components["schemas"]["Container"];
type IpAssignment = components["schemas"]["IpAssignment"];
type Page<T> = { items: T[]; page: components["schemas"]["PageInfo"] };

async function readAll<T>(
  load: (cursor?: string) => Promise<{ data?: Page<T>; error?: unknown }>,
  signal: AbortSignal,
) {
  const items: T[] = [];
  let cursor: string | undefined;
  const seen = new Set<string>();
  let hasPage = true;
  while (hasPage) {
    if (signal.aborted) throw new DOMException("Aborted", "AbortError");
    const { data, error } = await load(cursor);
    if (error) throw error;
    if (!data) throw new Error("Incomplete paged discovery response.");
    items.push(...data.items);
    const next = data.page.next_cursor ?? undefined;
    if (!data.page.has_more || !next) {
      hasPage = false;
      continue;
    }
    if (seen.has(next))
      throw new Error("Discovery pagination cursor repeated.");
    seen.add(next);
    cursor = next;
  }
  return items;
}

/** Atomic view-model query for a selected compute; signal cancellation and cache identity are compute-scoped. */
export function useComputeDiscovery(computeId?: string) {
  return useQuery({
    queryKey: sessionQueryKey("compute-discovery", computeId ?? ""),
    enabled: Boolean(computeId),
    queryFn: async ({ signal }) => {
      const path = { compute_id: computeId! };
      const [compute, collector, containers, ips] = await Promise.allSettled([
        apiClient.GET("/computes/{compute_id}", { params: { path }, signal }),
        apiClient.GET("/computes/{compute_id}/collector-status", {
          params: { path },
          signal,
        }),
        readAll<Container>(
          (cursor) =>
            apiClient.GET("/computes/{compute_id}/containers", {
              params: {
                path,
                query: {
                  "page[size]": 200,
                  ...(cursor ? { "page[after]": cursor } : {}),
                },
              },
              signal,
            }),
          signal,
        ),
        readAll<IpAssignment>(
          (cursor) =>
            apiClient.GET("/ip-assignments", {
              params: {
                query: {
                  compute_id: computeId!,
                  source: "discovered",
                  "page[size]": 200,
                  ...(cursor ? { "page[after]": cursor } : {}),
                },
              },
              signal,
            }),
          signal,
        ),
      ]);
      const value = <T>(result: PromiseSettledResult<T>) =>
        result.status === "fulfilled" ? result.value : undefined;
      const computeResult = value(compute);
      const collectorResult = value(collector);
      const containerResult = value(containers);
      const ipResult = value(ips);
      const errors = [compute, collector, containers, ips].flatMap((r) =>
        r.status === "rejected" ? [r.reason] : [],
      );
      for (const result of [computeResult, collectorResult]) {
        if (result?.error) errors.push(result.error);
      }
      const computeData =
        computeResult && !computeResult.error ? computeResult.data : undefined;
      const collectorData =
        collectorResult && !collectorResult.error
          ? collectorResult.data
          : undefined;
      if (!computeData || !collectorData)
        errors.push(new Error("Compute discovery metadata is unavailable."));
      return {
        compute: computeData,
        collectors: collectorData?.items,
        containers: containerResult,
        ipAssignments: ipResult,
        errors,
      };
    },
    refetchInterval: () =>
      typeof document !== "undefined" && document.visibilityState === "visible"
        ? 10_000
        : false,
    refetchIntervalInBackground: false,
    retry: 2,
    retryDelay: (attempt) => Math.min(1_000 * 2 ** attempt, 15_000),
  });
}
