import { useMemo } from "react";

import { useComputes, useDomains, useLocations, useProviders } from "./data";

/** Resolves any inventory ID to the name a person knows it by. */
export function useNameLookup() {
  const computes = useComputes(true);
  const domains = useDomains(true);
  const providers = useProviders(true);
  const locations = useLocations(true);
  return useMemo(() => {
    const names = new Map<string, string>();
    for (const item of computes.data ?? [])
      names.set(item.id, item.display_name);
    for (const item of domains.data ?? []) names.set(item.id, item.name);
    for (const item of providers.data ?? []) names.set(item.id, item.name);
    for (const item of locations.data ?? []) names.set(item.id, item.name);
    return (id?: string) => (id ? names.get(id) : undefined);
  }, [computes.data, domains.data, providers.data, locations.data]);
}
