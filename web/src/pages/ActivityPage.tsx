import { useState } from "react";
import { Stack } from "@mui/material";

import type { AuditQuery } from "../app/data";
import type { ActivityCategory } from "../app/format";
import { ActivityFeed } from "../features/audit/ActivityFeed";
import { PageHeader, Pill } from "../ui/primitives";

type Filter = "all" | ActivityCategory | "mine";

const filters: [Filter, string][] = [
  ["all", "All"],
  ["discovery", "Discovery"],
  ["agents", "Agents"],
  ["inventory", "Inventory"],
  ["mine", "By you"],
];

export function ActivityPage() {
  const [filter, setFilter] = useState<Filter>("all");
  const query: AuditQuery =
    filter === "mine" ? { actor_type: "administrator" } : {};
  return (
    <Stack spacing={3.5} sx={{ maxWidth: 880, width: "100%", mx: "auto" }}>
      <PageHeader
        title="Activity"
        subtitle="Everything that changed, in plain words. Open an entry for the raw record."
      />
      <Stack
        direction="row"
        role="group"
        aria-label="Filter activity"
        sx={{ flexWrap: "wrap", gap: 1 }}
      >
        {filters.map(([value, label]) => (
          <Pill
            key={value}
            selected={filter === value}
            onClick={() => setFilter(value)}
          >
            {label}
          </Pill>
        ))}
      </Stack>
      <ActivityFeed
        query={query}
        category={filter === "all" || filter === "mine" ? undefined : filter}
        emptyText="Nothing matches this filter yet."
      />
    </Stack>
  );
}
