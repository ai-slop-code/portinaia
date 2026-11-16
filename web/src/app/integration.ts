import type { FormEvent } from "react";
import type { components } from "../api/generated";
import type { DomainEditSnapshot } from "../features/domains/domainAdapter";

type Provider = components["schemas"]["Provider"];
type Location = components["schemas"]["Location"];
type Compute = components["schemas"]["Compute"];
type IPAssignment = components["schemas"]["IpAssignment"];

/** Immutable edit lease. Metadata not represented by editable fields is retained verbatim. */
export type InventoryEditSnapshot<TValues> = Readonly<{
  id: string;
  etag: string;
  initialValues: Readonly<TValues>;
  preservedTagIDs: readonly string[];
}>;

export type ProviderEditValues = Pick<
  Provider,
  | "name"
  | "website"
  | "external_account_reference"
  | "description"
  | "notes"
  | "roles"
>;
export type LocationEditValues = Pick<
  Location,
  "type" | "name" | "provider_id" | "parent_id" | "description" | "notes"
>;
export type ComputeEditValues = Pick<
  Compute,
  | "kind"
  | "display_name"
  | "ownership"
  | "provider_id"
  | "provider_resource_id"
  | "location_id"
  | "parent_compute_id"
  | "description"
  | "notes"
>;
export type IPEditValues = Pick<
  IPAssignment,
  | "address"
  | "prefix_length"
  | "compute_id"
  | "interface_or_network"
  | "scope"
  | "role"
  | "primary"
  | "description"
>;

export type ProviderEditSnapshot = InventoryEditSnapshot<ProviderEditValues>;
export type LocationEditSnapshot = InventoryEditSnapshot<LocationEditValues>;
export type ComputeEditSnapshot = InventoryEditSnapshot<ComputeEditValues>;
export type IPEditSnapshot = InventoryEditSnapshot<IPEditValues>;
export type ProviderEditSubmission = EditSubmission<ProviderEditSnapshot>;
export type LocationEditSubmission = EditSubmission<LocationEditSnapshot>;
export type ComputeEditSubmission = EditSubmission<ComputeEditSnapshot>;
export type IPEditSubmission = EditSubmission<IPEditSnapshot>;

/** Reference selectors query independently (q + page[size] + optional page[after]);
 * hydrate a selected ID absent from the current page by detail GET, retaining it as
 * an option even when it is not in the active result page. */
export type ReferenceSelectorQuery = IntegrationQuery<
  readonly [query: string, cursor: string | null]
>;
export type ReferenceSelectorPage<T> = PageSnapshot<T>;
export type ReferenceSelectorHydration<T> = Readonly<{
  selectedID: string | null;
  selected: T | null;
  unavailable: boolean;
}>;
export type ReferenceSelectorContract<T> = Readonly<{
  query: ReferenceSelectorQuery;
  page: ReferenceSelectorPage<T>;
  hydration: ReferenceSelectorHydration<T>;
}>;

/** Stable query identity: resource + user-visible parameters + session generation. */
export type IntegrationQuery<
  T extends readonly unknown[] = readonly unknown[],
> = Readonly<{
  key: readonly ["session", number, string, ...T];
  signal?: AbortSignal;
}>;

export type PageSnapshot<T> = Readonly<{
  items: readonly T[];
  nextCursor: string | null;
  hasMore: boolean;
  queryKey: readonly unknown[];
}>;

export type EditSubmission<TSnapshot> = (
  event: FormEvent<HTMLFormElement>,
  snapshot: TSnapshot,
) => void;

export type DomainEditSubmission = EditSubmission<DomainEditSnapshot>;
export type DomainListQuery = IntegrationQuery<
  readonly [query: string, archived: boolean, cursor: string | null]
>;
export type DomainPageSnapshot = PageSnapshot<components["schemas"]["Domain"]>;

/** Query identities affected by a successful mutation; callers invalidate all keys. */
export type MutationInvalidation = Readonly<{
  resource: string;
  queryKeys: readonly (readonly unknown[])[];
}>;
