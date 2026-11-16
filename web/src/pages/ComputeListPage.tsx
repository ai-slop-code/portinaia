import { useMemo, useState } from "react";
import { Link as RouterLink } from "react-router-dom";
import { Alert, Button, FormControlLabel, Stack, Switch } from "@mui/material";

import {
  useComputes,
  useLocations,
  useProviders,
  type Compute,
} from "../app/data";
import { agentStatusLabel, computeLight, kindLabel } from "../app/format";
import { tokens } from "../theme/theme";
import { ComputeIcon, PlusIcon } from "../ui/icons";
import {
  EmptyHint,
  Mono,
  PageHeader,
  Pill,
  RowLink,
  StatusLight,
} from "../ui/primitives";

/** Orders hosts so each VM follows the machine it runs on. */
function nestByParent(hosts: Compute[]): { host: Compute; depth: number }[] {
  const byParent = new Map<string, Compute[]>();
  const ids = new Set(hosts.map((host) => host.id));
  const roots: Compute[] = [];
  for (const host of hosts) {
    const parent = host.parent_compute_id;
    if (parent && ids.has(parent)) {
      byParent.set(parent, [...(byParent.get(parent) ?? []), host]);
    } else roots.push(host);
  }
  const ordered: { host: Compute; depth: number }[] = [];
  const visit = (host: Compute, depth: number) => {
    ordered.push({ host, depth });
    for (const child of byParent.get(host.id) ?? []) visit(child, depth + 1);
  };
  const byName = (a: Compute, b: Compute) =>
    a.display_name.localeCompare(b.display_name);
  roots.sort(byName).forEach((root) => visit(root, 0));
  return ordered;
}

export function ComputeListPage() {
  const [archived, setArchived] = useState(false);
  const computes = useComputes(archived);
  const providers = useProviders(true);
  const locations = useLocations(true);
  const [filter, setFilter] = useState<"all" | Compute["kind"]>("all");

  const providerName = useMemo(
    () => new Map((providers.data ?? []).map((item) => [item.id, item.name])),
    [providers.data],
  );
  const placeName = useMemo(
    () => new Map((locations.data ?? []).map((item) => [item.id, item.name])),
    [locations.data],
  );
  const hosts = computes.data ?? [];
  const rows = nestByParent(
    hosts.filter((host) => filter === "all" || host.kind === filter),
  );

  return (
    <>
      <PageHeader
        title="Compute"
        subtitle={
          computes.data
            ? `${hosts.length} host${hosts.length === 1 ? "" : "s"} · ${hosts.filter((host) => host.agent_status === "online").length} reporting`
            : "Loading…"
        }
        actions={
          <Button
            component={RouterLink}
            to="/add?step=compute"
            variant="contained"
            startIcon={<PlusIcon size={16} />}
          >
            Add host
          </Button>
        }
      />
      <Stack
        direction="row"
        sx={{ flexWrap: "wrap", gap: 1, alignItems: "center" }}
        role="group"
        aria-label="Filter hosts"
      >
        {(
          [
            ["all", "All"],
            ["physical", "Physical"],
            ["vps", "VPS"],
            ["vm", "VM"],
          ] as const
        ).map(([value, label]) => (
          <Pill
            key={value}
            selected={filter === value}
            onClick={() => setFilter(value)}
          >
            {label}
          </Pill>
        ))}
        <FormControlLabel
          sx={{ ml: "auto" }}
          control={
            <Switch
              checked={archived}
              onChange={(event) => setArchived(event.target.checked)}
            />
          }
          label="Show archived"
        />
      </Stack>
      {computes.isError && (
        <Alert severity="error">Could not load your hosts.</Alert>
      )}
      {computes.isSuccess && hosts.length === 0 ? (
        <EmptyHint
          title="No hosts yet"
          body="Add a physical box, a VPS or a VM. The agent can fill in the rest."
          action={
            <Button
              component={RouterLink}
              to="/add?step=compute"
              variant="contained"
            >
              Add your first host
            </Button>
          }
        />
      ) : (
        <Stack component="ul" sx={{ listStyle: "none", m: 0, p: 0, gap: 0.5 }}>
          {rows.map(({ host, depth }) => {
            const where = [
              host.provider_id && providerName.get(host.provider_id),
              host.location_id && placeName.get(host.location_id),
            ].filter(Boolean);
            return (
              <li key={host.id} style={{ paddingLeft: depth * 32 }}>
                <RowLink
                  to={`/compute/${host.id}`}
                  ariaLabel={`Open ${host.display_name}`}
                  icon={<ComputeIcon size={20} />}
                  title={
                    <Stack
                      direction="row"
                      spacing={1.5}
                      sx={{ alignItems: "center", flexWrap: "wrap" }}
                    >
                      <Mono>{host.display_name}</Mono>
                      {host.archived_at && <Pill>Archived</Pill>}
                    </Stack>
                  }
                  subtitle={[
                    kindLabel[host.kind],
                    ...(where.length ? where : ["No provider or place"]),
                  ].join(" · ")}
                  trailing={
                    <Stack
                      component="span"
                      direction="row"
                      sx={{
                        color: tokens.fog,
                        fontSize: 13,
                        display: { xs: "none", sm: "flex" },
                      }}
                    >
                      <StatusLight
                        light={computeLight(host)}
                        label={agentStatusLabel[host.agent_status]}
                      />
                    </Stack>
                  }
                />
              </li>
            );
          })}
        </Stack>
      )}
    </>
  );
}
