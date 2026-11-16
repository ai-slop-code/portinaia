import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import {
  apiClient,
  apiFetch,
  readApiError,
  sessionQueryKey,
} from "../api/client";
import type { components } from "../api/generated";

export type Provider = components["schemas"]["Provider"];
export type Location = components["schemas"]["Location"];
export type Compute = components["schemas"]["Compute"];
export type Domain = components["schemas"]["Domain"];
export type Container = components["schemas"]["Container"];
export type Agent = components["schemas"]["Agent"];
export type IPAssignment = components["schemas"]["IpAssignment"];
export type AuditEvent = components["schemas"]["AuditEvent"];
export type InventorySummary = components["schemas"]["InventorySummary"];
export type CollectorObservation =
  components["schemas"]["CollectorObservation"];
export type EnrollmentSecret = components["schemas"]["EnrollmentTokenSecret"];

type Page<T> = {
  items: T[];
  page: { has_more: boolean; next_cursor?: string | null };
};

/** Personal inventories are small: read every page up to a generous bound. */
const PAGE_SIZE = 200;
const MAX_PAGES = 25;

async function collectPages<T>(
  load: (after?: string) => Promise<{ data?: Page<T>; error?: unknown }>,
  what: string,
): Promise<T[]> {
  const items: T[] = [];
  let after: string | undefined;
  for (let index = 0; index < MAX_PAGES; index++) {
    const { data, error } = await load(after);
    if (error || !data) throw new Error(`Could not load ${what}.`);
    items.push(...data.items);
    if (!data.page.has_more || !data.page.next_cursor) break;
    after = data.page.next_cursor;
  }
  return items;
}

const archivedParam = (archived: boolean) =>
  archived ? ("include" as const) : ("exclude" as const);
const cursorParam = (after?: string) => (after ? { "page[after]": after } : {});

export function useProviders(archived = false) {
  return useQuery({
    queryKey: sessionQueryKey("providers", archived),
    queryFn: () =>
      collectPages<Provider>(
        (after) =>
          apiClient.GET("/providers", {
            params: {
              query: {
                archived: archivedParam(archived),
                "page[size]": PAGE_SIZE,
                ...cursorParam(after),
              },
            },
          }),
        "providers",
      ),
  });
}

export function useLocations(archived = false) {
  return useQuery({
    queryKey: sessionQueryKey("locations", archived),
    queryFn: () =>
      collectPages<Location>(
        (after) =>
          apiClient.GET("/locations", {
            params: {
              query: {
                archived: archivedParam(archived),
                "page[size]": PAGE_SIZE,
                ...cursorParam(after),
              },
            },
          }),
        "places",
      ),
  });
}

export function useComputes(archived = false) {
  return useQuery({
    queryKey: sessionQueryKey("computes", archived),
    queryFn: () =>
      collectPages<Compute>(
        (after) =>
          apiClient.GET("/computes", {
            params: {
              query: {
                archived: archivedParam(archived),
                "page[size]": PAGE_SIZE,
                ...cursorParam(after),
              },
            },
          }),
        "compute",
      ),
  });
}

export function useDomains(archived = false) {
  return useQuery({
    queryKey: sessionQueryKey("domains", archived),
    queryFn: () =>
      collectPages<Domain>(
        (after) =>
          apiClient.GET("/domains", {
            params: {
              query: {
                archived: archivedParam(archived),
                "page[size]": PAGE_SIZE,
                ...cursorParam(after),
              },
            },
          }),
        "domains",
      ),
  });
}

export function useInventorySummary() {
  return useQuery({
    queryKey: sessionQueryKey("inventory-summary"),
    queryFn: async () => {
      const { data, error } = await apiClient.GET("/inventory/summary");
      if (error || !data) throw new Error("Could not load inventory counts.");
      return data;
    },
  });
}

export function useCompute(
  id: string | undefined,
  options: { pollUntilOnline?: boolean } = {},
) {
  return useQuery({
    queryKey: sessionQueryKey("compute", id),
    enabled: Boolean(id),
    refetchInterval: (query) =>
      options.pollUntilOnline &&
      query.state.data?.agent_status !== "online" &&
      document.visibilityState === "visible"
        ? 5000
        : false,
    queryFn: async () => {
      const { data, error } = await apiClient.GET("/computes/{compute_id}", {
        params: { path: { compute_id: id! } },
      });
      if (error || !data) throw new Error("Could not load this host.");
      return data;
    },
  });
}

export function useAgent(id: string | undefined) {
  return useQuery({
    queryKey: sessionQueryKey("agent", id),
    enabled: Boolean(id),
    queryFn: async () => {
      const { data, error } = await apiClient.GET("/agents/{agent_id}", {
        params: { path: { agent_id: id! } },
      });
      if (error || !data) throw new Error("Could not load the agent.");
      return data;
    },
  });
}

export function useCollectorStatus(computeID: string | undefined) {
  return useQuery({
    queryKey: sessionQueryKey("collector-status", computeID),
    enabled: Boolean(computeID),
    queryFn: async () => {
      const { data } = await apiClient.GET(
        "/computes/{compute_id}/collector-status",
        { params: { path: { compute_id: computeID! } } },
      );
      return data?.items ?? [];
    },
  });
}

export function useComputeContainers(
  computeID: string | undefined,
  archived = false,
) {
  return useQuery({
    queryKey: sessionQueryKey("compute-containers", computeID, archived),
    enabled: Boolean(computeID),
    queryFn: () =>
      collectPages<Container>(
        (after) =>
          apiClient.GET("/computes/{compute_id}/containers", {
            params: {
              path: { compute_id: computeID! },
              query: {
                archived: archivedParam(archived),
                "page[size]": PAGE_SIZE,
                ...cursorParam(after),
              },
            },
          }),
        "containers",
      ),
  });
}

export function useContainer(id: string | undefined) {
  return useQuery({
    queryKey: sessionQueryKey("container", id),
    enabled: Boolean(id),
    queryFn: async () => {
      const { data, error } = await apiClient.GET(
        "/containers/{container_id}",
        { params: { path: { container_id: id! } } },
      );
      if (error || !data) throw new Error("Could not load this container.");
      return data;
    },
  });
}

export function useIPAssignments(computeID: string | undefined) {
  return useQuery({
    queryKey: sessionQueryKey("ip-assignments", computeID),
    enabled: Boolean(computeID),
    queryFn: () =>
      collectPages<IPAssignment>(
        (after) =>
          apiClient.GET("/ip-assignments", {
            params: {
              query: {
                compute_id: computeID!,
                archived: "include",
                "page[size]": PAGE_SIZE,
                ...cursorParam(after),
              },
            },
          }),
        "IP addresses",
      ),
  });
}

export type AuditQuery = {
  actor_type?: components["schemas"]["AuditActorType"];
  resource_type?: string;
  resource_id?: string;
};

export function useAuditEvents(filters: AuditQuery, pageSize = 50) {
  return useInfiniteQuery({
    queryKey: sessionQueryKey("audit", filters, pageSize),
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await apiClient.GET("/audit-events", {
        params: {
          query: {
            "page[size]": pageSize,
            ...cursorParam(pageParam),
            ...Object.fromEntries(
              Object.entries(filters).filter(([, value]) => value),
            ),
          },
        },
      });
      if (error || !data) throw new Error("Activity is unavailable.");
      return data;
    },
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
}

/** Revision precondition used by every update and lifecycle request. */
export const revisionTag = (resource: { revision: number }) =>
  `"revision-${resource.revision}"`;

/**
 * Sends a JSON mutation through the shared transport (CSRF, idempotency key and
 * session guards are added there) and throws an ApiError on failure.
 */
export async function sendJSON<T>(
  method: "POST" | "PUT" | "DELETE",
  path: string,
  body?: unknown,
  ifMatch?: string,
): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (ifMatch) headers["If-Match"] = ifMatch;
  const response = await apiFetch(`/api/v1${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) throw await readApiError(response);
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

/** Mutations invalidate the whole session namespace: inventories are small. */
export function useInventoryMutation<TArgs, TResult>(
  run: (args: TArgs) => Promise<TResult>,
) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: run,
    onSuccess: () => client.invalidateQueries({ queryKey: ["session"] }),
  });
}
