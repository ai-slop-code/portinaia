import { useState, type FormEvent, type ReactNode } from "react";
import {
  Link as RouterLink,
  useParams,
  useSearchParams,
} from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  MenuItem,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@mui/material";

import { ApiError } from "../api/client";
import {
  revisionTag,
  sendJSON,
  useAgent,
  useCollectorStatus,
  useCompute,
  useComputeContainers,
  useComputes,
  useContainer,
  useIPAssignments,
  useInventoryMutation,
  useLocations,
  useProviders,
  type Agent,
  type Compute,
  type Container,
  type IPAssignment,
} from "../app/data";
import {
  agentStatusLabel,
  computeLight,
  formatBytes,
  formatDateTime,
  kindLabel,
  ownershipLabel,
  relativeTime,
} from "../app/format";
import { ConnectAgent } from "../features/agents/ConnectAgent";
import { ActivityFeed } from "../features/audit/ActivityFeed";
import { fonts, tokens } from "../theme/theme";
import { ContainerIcon } from "../ui/icons";
import { SidePanel } from "../ui/SidePanel";
import {
  EmptyHint,
  FactList,
  Mono,
  PageHeader,
  Pill,
  SectionTitle,
  StatusLight,
  Surface,
} from "../ui/primitives";

const tabs = [
  ["overview", "Overview"],
  ["containers", "Containers"],
  ["network", "Network"],
  ["agent", "Agent"],
  ["activity", "Activity"],
] as const;
type TabID = (typeof tabs)[number][0];

export function ComputeDetailPage() {
  const { id } = useParams();
  const [params, setParams] = useSearchParams();
  const tab: TabID =
    (tabs.find(([value]) => value === params.get("tab"))?.[0] as TabID) ??
    "overview";
  const compute = useCompute(id);
  const containers = useComputeContainers(id);
  const ips = useIPAssignments(id);
  const providers = useProviders(true);
  const locations = useLocations(true);
  const computes = useComputes(true);
  const [editing, setEditing] = useState(false);

  if (compute.isPending)
    return <Typography color="text.secondary">Loading host…</Typography>;
  if (compute.isError || !compute.data)
    return (
      <EmptyHint
        title="Host not found"
        body="It may have been purged, or the link is wrong."
        action={
          <Button component={RouterLink} to="/compute" variant="outlined">
            Back to compute
          </Button>
        }
      />
    );

  const host = compute.data;
  const provider = providers.data?.find((item) => item.id === host.provider_id);
  const place = locations.data?.find((item) => item.id === host.location_id);
  const parent = computes.data?.find(
    (item) => item.id === host.parent_compute_id,
  );
  const children = (computes.data ?? []).filter(
    (item) => item.parent_compute_id === host.id,
  );
  const containerCount = containers.data?.length ?? 0;
  const activeIPs = (ips.data ?? []).filter((ip) => !ip.archived_at);

  const setTab = (value: TabID) => {
    const next = new URLSearchParams(params);
    if (value === "overview") next.delete("tab");
    else next.set("tab", value);
    setParams(next, { replace: true });
  };

  return (
    <>
      <Typography
        component="nav"
        aria-label="Breadcrumb"
        sx={{ fontSize: 14, color: tokens.fog }}
      >
        <Box
          component={RouterLink}
          to="/compute"
          sx={{
            color: tokens.fog,
            textDecoration: "none",
            "&:hover": { color: tokens.foam },
          }}
        >
          Compute
        </Box>{" "}
        / {host.display_name}
      </Typography>

      <PageHeader
        mono
        title={host.display_name}
        subtitle={
          <Stack
            component="span"
            direction="row"
            sx={{ flexWrap: "wrap", gap: 1, alignItems: "center", mt: 0.5 }}
          >
            <Pill>
              <StatusLight
                light={computeLight(host)}
                label={
                  host.agent_status === "online" ||
                  host.agent_status === "offline"
                    ? `${agentStatusLabel[host.agent_status]}`
                    : agentStatusLabel[host.agent_status]
                }
              />
            </Pill>
            <Pill>{kindLabel[host.kind]}</Pill>
            <Pill>{ownershipLabel[host.ownership]}</Pill>
            {host.archived_at && <Pill>Archived</Pill>}
            {provider ? (
              <Pill to={`/providers/${provider.id}`}>{provider.name}</Pill>
            ) : (
              <Pill gap onClick={() => setEditing(true)}>
                + Provider
              </Pill>
            )}
            {place ? (
              <Pill to={`/places/${place.id}`}>{place.name}</Pill>
            ) : (
              <Pill gap onClick={() => setEditing(true)}>
                + Place
              </Pill>
            )}
            {parent && (
              <Pill to={`/compute/${parent.id}`}>
                on <Mono>{parent.display_name}</Mono>
              </Pill>
            )}
          </Stack>
        }
        actions={
          <Button variant="outlined" onClick={() => setEditing(true)}>
            {host.archived_at ? "Restore…" : "Edit"}
          </Button>
        }
      />

      <Box sx={{ borderBottom: `1px solid ${tokens.line}` }}>
        <Tabs
          value={tab}
          onChange={(_, value: TabID) => setTab(value)}
          variant="scrollable"
          allowScrollButtonsMobile
          aria-label="Host sections"
        >
          {tabs.map(([value, label]) => (
            <Tab
              key={value}
              value={value}
              label={
                value === "containers" && containers.data
                  ? `${label} · ${containerCount}`
                  : label
              }
            />
          ))}
        </Tabs>
      </Box>

      {tab === "overview" && (
        <OverviewTab
          host={host}
          containerCount={containerCount}
          ipCount={activeIPs.length}
          children={children}
          onTab={setTab}
        />
      )}
      {tab === "containers" && (
        <ContainersTab
          containers={containers.data}
          loading={containers.isPending}
          error={containers.isError}
          agentStatus={host.agent_status}
          onConnect={() => setTab("agent")}
        />
      )}
      {tab === "network" && <NetworkTab host={host} ips={ips.data ?? []} />}
      {tab === "agent" && <AgentTab host={host} />}
      {tab === "activity" && (
        <ActivityFeed
          query={{ resource_id: host.id }}
          emptyText="No recorded changes to this host yet."
        />
      )}

      {editing && (
        <EditComputeDrawer host={host} open onClose={() => setEditing(false)} />
      )}
    </>
  );
}

function FactCard({
  label,
  value,
  detail,
  dashed,
  action,
}: {
  label: string;
  value: ReactNode;
  detail?: ReactNode;
  dashed?: boolean;
  action?: ReactNode;
}) {
  return (
    <Surface dashed={dashed}>
      <Stack spacing={1.25}>
        <Typography sx={{ color: tokens.fog, fontSize: 14 }}>
          {label}
        </Typography>
        <Typography
          sx={{
            fontFamily: fonts.display,
            fontWeight: 600,
            fontSize: 22,
            color: dashed ? tokens.fog : tokens.foam,
          }}
        >
          {value}
        </Typography>
        {detail && (
          <Typography component="div" sx={{ color: tokens.fog, fontSize: 13 }}>
            {detail}
          </Typography>
        )}
        {action}
      </Stack>
    </Surface>
  );
}

function OverviewTab({
  host,
  containerCount,
  ipCount,
  children,
  onTab,
}: {
  host: Compute;
  containerCount: number;
  ipCount: number;
  children: Compute[];
  onTab: (tab: TabID) => void;
}) {
  const collectors = useCollectorStatus(host.id);
  const failed = (collectors.data ?? []).filter(
    (item) => item.last_status === "failed",
  );
  const facts = host.observation;
  const linkButton = (tab: TabID, label: string) => (
    <Button
      onClick={() => onTab(tab)}
      sx={{ alignSelf: "flex-start", px: 0.5 }}
    >
      {label} →
    </Button>
  );
  return (
    <Stack spacing={4}>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))",
          gap: 2,
        }}
      >
        <FactCard
          label="Agent"
          value={agentStatusLabel[host.agent_status]}
          dashed={host.agent_status === "unenrolled"}
          detail={
            failed.length > 0
              ? `${failed.length} collector${failed.length === 1 ? "" : "s"} failing`
              : undefined
          }
          action={
            host.agent_status === "unenrolled"
              ? linkButton("agent", "Connect an agent")
              : linkButton("agent", "Agent details")
          }
        />
        <FactCard
          label="Containers"
          value={`${containerCount} observed`}
          dashed={containerCount === 0}
          action={linkButton("containers", "See containers")}
        />
        <FactCard
          label="IP addresses"
          value={ipCount === 0 ? "None yet" : `${ipCount} recorded`}
          dashed={ipCount === 0}
          action={linkButton(
            "network",
            ipCount === 0 ? "Add an IP" : "See addresses",
          )}
        />
      </Box>

      <Box component="section" aria-labelledby="facts-h">
        <SectionTitle id="facts-h">Host facts</SectionTitle>
        {facts ? (
          <Surface>
            <FactList
              rows={[
                ["Hostname", <Mono key="h">{facts.hostname}</Mono>],
                [
                  "OS",
                  [facts.os_name, facts.os_version].filter(Boolean).join(" ") ||
                    "Unknown",
                ],
                ["Kernel", facts.kernel_version ?? "Unknown"],
                ["Architecture", facts.architecture],
                [
                  "CPU",
                  `${facts.logical_cpu_count} logical${facts.cpu_model ? ` · ${facts.cpu_model}` : ""}`,
                ],
                ["Memory", formatBytes(facts.memory_bytes)],
                [
                  "Disks",
                  facts.disks.length
                    ? facts.disks
                        .map(
                          (disk) =>
                            `${disk.mount_point ?? disk.name} ${formatBytes(disk.size_bytes)}`,
                        )
                        .join(" · ")
                    : "None reported",
                ],
                ["Booted", formatDateTime(facts.booted_at)],
                ["Observed", relativeTime(facts.observed_at)],
              ]}
            />
          </Surface>
        ) : (
          <Typography color="text.secondary">
            No host facts yet. They appear after the agent’s first successful
            snapshot — unknown, not confirmed absent.
          </Typography>
        )}
      </Box>

      {children.length > 0 && (
        <Box component="section" aria-labelledby="vms-h">
          <SectionTitle id="vms-h">Runs on this host</SectionTitle>
          <Stack direction="row" sx={{ flexWrap: "wrap", gap: 1 }}>
            {children.map((child) => (
              <Pill key={child.id} to={`/compute/${child.id}`}>
                <StatusLight light={computeLight(child)} size={8} />
                <Mono>{child.display_name}</Mono>
              </Pill>
            ))}
          </Stack>
        </Box>
      )}

      {(host.description || host.notes || host.provider_resource_id) && (
        <Box component="section" aria-labelledby="notes-h">
          <SectionTitle id="notes-h">Notes</SectionTitle>
          <Stack spacing={1}>
            {host.provider_resource_id && (
              <Typography color="text.secondary">
                Provider reference: <Mono>{host.provider_resource_id}</Mono>
              </Typography>
            )}
            {host.description && <Typography>{host.description}</Typography>}
            {host.notes && (
              <Typography sx={{ whiteSpace: "pre-wrap", color: tokens.mist }}>
                {host.notes}
              </Typography>
            )}
          </Stack>
        </Box>
      )}
    </Stack>
  );
}

function containerLight(container: Container) {
  const observation = container.observation;
  if (!observation) return "unset" as const;
  if (observation.stale_at) return "stale" as const;
  if (observation.health === "unhealthy") return "failed" as const;
  if (observation.state === "running") return "live" as const;
  return "offline" as const;
}

function ContainersTab({
  containers,
  loading,
  error,
  agentStatus,
  onConnect,
}: {
  containers?: Container[];
  loading: boolean;
  error: boolean;
  agentStatus: Compute["agent_status"];
  onConnect: () => void;
}) {
  const [filter, setFilter] = useState<"all" | "running" | "stale">("all");
  const [selected, setSelected] = useState<string | null>(null);
  if (loading) return <Typography color="text.secondary">Loading…</Typography>;
  if (error) return <Alert severity="error">Could not load containers.</Alert>;
  const all = containers ?? [];
  if (all.length === 0)
    return (
      <EmptyHint
        title="No containers seen"
        body={
          agentStatus === "unenrolled"
            ? "Containers are discovered by the agent. Connect one to see what runs here."
            : "The agent hasn’t reported any Podman containers on this host."
        }
        action={
          agentStatus === "unenrolled" && (
            <Button variant="contained" onClick={onConnect}>
              Connect an agent
            </Button>
          )
        }
      />
    );
  const shown = all.filter((item) =>
    filter === "all"
      ? true
      : filter === "stale"
        ? Boolean(item.observation?.stale_at)
        : item.observation?.state === "running" && !item.observation.stale_at,
  );
  return (
    <Stack spacing={2.25}>
      <Stack
        direction="row"
        role="group"
        aria-label="Filter containers"
        sx={{ gap: 1, flexWrap: "wrap" }}
      >
        {(
          [
            ["all", "All"],
            ["running", "Running"],
            ["stale", "Stale"],
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
      </Stack>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fill, minmax(300px, 1fr))",
          gap: 2,
        }}
      >
        {shown.map((item) => (
          <Box
            key={item.id}
            component="button"
            type="button"
            onClick={() => setSelected(item.id)}
            aria-label={`Open container ${item.name}`}
            sx={{
              display: "flex",
              flexDirection: "column",
              gap: 1.75,
              p: 2.5,
              borderRadius: "20px",
              background: tokens.hull,
              border: `1px solid ${tokens.line}`,
              color: "inherit",
              font: "inherit",
              textAlign: "left",
              cursor: "pointer",
              "&:hover": { background: tokens.deck, borderColor: tokens.rope },
            }}
          >
            <Stack
              direction="row"
              spacing={1.5}
              sx={{ alignItems: "center", width: "100%" }}
            >
              <Box
                sx={{
                  width: 40,
                  height: 40,
                  borderRadius: "12px",
                  background: tokens.deck,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  color: tokens.mist,
                  flex: "0 0 auto",
                }}
              >
                <ContainerIcon size={20} />
              </Box>
              <Mono
                sx={{ flex: 1, minWidth: 0, fontSize: 16, fontWeight: 500 }}
              >
                {item.name}
              </Mono>
              <StatusLight light={containerLight(item)} />
            </Stack>
            <Mono
              sx={{
                fontSize: 13,
                color: tokens.fog,
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
                overflowWrap: "normal",
                width: "100%",
              }}
            >
              {item.observation?.image_name ?? "image unknown"}
            </Mono>
            <Typography sx={{ fontSize: 13, color: tokens.fog }}>
              {item.observation
                ? `${item.observation.stale_at ? "stale" : item.observation.state}${item.observation.health !== "none" && item.observation.health !== "unknown" ? ` · ${item.observation.health}` : ""} · seen ${relativeTime(item.observation.last_observed_at)}`
                : "No observation"}
            </Typography>
          </Box>
        ))}
      </Box>
      <ContainerDrawer id={selected} onClose={() => setSelected(null)} />
    </Stack>
  );
}

function ContainerDrawer({
  id,
  onClose,
}: {
  id: string | null;
  onClose: () => void;
}) {
  const container = useContainer(id ?? undefined);
  const data = container.data;
  const observation = data?.observation;
  return (
    <SidePanel
      open={Boolean(id)}
      onClose={onClose}
      title={data?.name ?? "Container"}
      mono
    >
      {container.isPending && (
        <Typography color="text.secondary">Loading…</Typography>
      )}
      {container.isError && (
        <Alert severity="error">Could not load this container.</Alert>
      )}
      {data && !observation && (
        <Typography color="text.secondary">No observation recorded.</Typography>
      )}
      {observation && (
        <Stack spacing={3}>
          <Stack direction="row" sx={{ gap: 1, flexWrap: "wrap" }}>
            <Pill>
              <StatusLight
                light={containerLight(data)}
                label={observation.stale_at ? "Stale" : observation.state}
              />
            </Pill>
            {observation.health !== "none" && (
              <Pill>health: {observation.health}</Pill>
            )}
            <Pill>restart: {observation.restart_policy.replace("_", " ")}</Pill>
          </Stack>
          {observation.stale_at && (
            <Alert severity="warning">
              Not seen in the latest snapshot — it may no longer be running.
            </Alert>
          )}
          <FactList
            rows={[
              ["Image", <Mono key="i">{observation.image_name}</Mono>],
              ...(observation.image_digest
                ? ([
                    ["Digest", <Mono key="d">{observation.image_digest}</Mono>],
                  ] as [string, ReactNode][])
                : []),
              ["Owner UID", String(data.podman_owner_uid)],
              ["Started", formatDateTime(observation.started_at)],
              ["Last seen", relativeTime(observation.last_observed_at)],
              [
                "Ports",
                observation.ports.length ? (
                  <Mono key="p">
                    {observation.ports
                      .map(
                        (port) =>
                          `${port.host_port ? `${port.host_port}:` : ""}${port.container_port}/${port.protocol}`,
                      )
                      .join(", ")}
                  </Mono>
                ) : (
                  "None"
                ),
              ],
              [
                "Networks",
                observation.networks.length ? (
                  <Mono key="n">
                    {observation.networks
                      .map(
                        (network) =>
                          `${network.name}${network.addresses.length ? ` (${network.addresses.map((address) => `${address.address}${address.prefix_length == null ? "" : `/${address.prefix_length}`}`).join(", ")})` : ""}`,
                      )
                      .join(", ")}
                  </Mono>
                ) : (
                  "None"
                ),
              ],
              [
                "Mounts",
                observation.mounts.length ? (
                  <Stack key="m" spacing={0.5}>
                    {observation.mounts.map((mount) => (
                      <Mono key={`${mount.source}-${mount.destination}`}>
                        {mount.source} → {mount.destination}
                        {mount.read_only ? " (ro)" : ""}
                      </Mono>
                    ))}
                  </Stack>
                ) : (
                  "None"
                ),
              ],
            ]}
          />
          {observation.labels.length > 0 && (
            <details>
              <Box
                component="summary"
                sx={{
                  cursor: "pointer",
                  color: tokens.signal,
                  fontWeight: 500,
                }}
              >
                {observation.labels.length} labels
              </Box>
              <Stack spacing={0.5} sx={{ mt: 1.5 }}>
                {observation.labels.map((label) => (
                  <Mono
                    key={label.key}
                    sx={{ fontSize: 13, color: tokens.mist }}
                  >
                    {label.key}={label.value}
                  </Mono>
                ))}
              </Stack>
            </details>
          )}
        </Stack>
      )}
    </SidePanel>
  );
}

const scopes = [
  ["public", "Public"],
  ["private", "Private"],
  ["loopback", "Loopback"],
  ["link_local", "Link-local"],
  ["other", "Other"],
] as const;
const ipRoles = [
  ["primary", "Primary"],
  ["management", "Management"],
  ["service", "Service"],
  ["other", "Other"],
] as const;

function ipPayload(form: FormData, computeID: string) {
  const prefix = String(form.get("prefix_length") ?? "");
  return {
    address: String(form.get("address") ?? "").trim(),
    prefix_length: prefix === "" ? undefined : Number(prefix),
    compute_id: computeID,
    interface_or_network:
      String(form.get("interface_or_network") ?? "") || undefined,
    scope: form.get("scope"),
    role: form.get("role"),
    primary: form.get("primary") === "on",
    description: String(form.get("description") ?? ""),
  };
}

function ipMessage(error: unknown, fallback: string) {
  if (error instanceof ApiError) {
    if (error.status === 409)
      return "That conflicts with an existing address (a primary may already exist for this family, or it is discovery-owned).";
    if (error.status === 412)
      return "This address changed elsewhere; reload before editing.";
    if (error.status === 422 || error.status === 400) return error.message;
  }
  return fallback;
}

function IPFields({ initial }: { initial?: IPAssignment }) {
  return (
    <>
      <Stack direction="row" sx={{ gap: 2, flexWrap: "wrap" }}>
        <TextField
          name="address"
          label="Address"
          placeholder="192.0.2.10"
          defaultValue={initial?.address}
          required
          sx={{ flex: "3 1 220px", "& input": { fontFamily: fonts.mono } }}
        />
        <TextField
          name="prefix_length"
          label="Prefix"
          type="number"
          defaultValue={initial?.prefix_length ?? ""}
          sx={{ flex: "1 1 100px" }}
        />
      </Stack>
      <Stack direction="row" sx={{ gap: 2, flexWrap: "wrap" }}>
        <TextField
          name="scope"
          label="Scope"
          select
          defaultValue={initial?.scope ?? "private"}
          sx={{ flex: "1 1 160px" }}
        >
          {scopes.map(([value, label]) => (
            <MenuItem key={value} value={value}>
              {label}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          name="role"
          label="Role"
          select
          defaultValue={initial?.role ?? "management"}
          sx={{ flex: "1 1 160px" }}
        >
          {ipRoles.map(([value, label]) => (
            <MenuItem key={value} value={value}>
              {label}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          name="interface_or_network"
          label="Interface"
          defaultValue={initial?.interface_or_network ?? ""}
          sx={{ flex: "1 1 160px" }}
        />
      </Stack>
      <FormControlLabel
        control={<Checkbox name="primary" defaultChecked={initial?.primary} />}
        label="Primary address"
      />
      <TextField
        name="description"
        label="Description"
        defaultValue={initial?.description ?? ""}
      />
    </>
  );
}

function NetworkTab({ host, ips }: { host: Compute; ips: IPAssignment[] }) {
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<IPAssignment | null>(null);
  const create = useInventoryMutation((body: unknown) =>
    sendJSON<IPAssignment>("POST", "/ip-assignments", body),
  );
  const update = useInventoryMutation(
    ({ ip, body }: { ip: IPAssignment; body: unknown }) =>
      sendJSON<IPAssignment>(
        "PUT",
        `/ip-assignments/${ip.id}`,
        body,
        revisionTag(ip),
      ),
  );
  const lifecycle = useInventoryMutation(
    ({ ip, action }: { ip: IPAssignment; action: "archive" | "restore" }) =>
      sendJSON<IPAssignment>(
        "POST",
        `/ip-assignments/${ip.id}/${action}`,
        undefined,
        revisionTag(ip),
      ),
  );
  const [showArchived, setShowArchived] = useState(false);
  const visible = ips.filter((ip) => showArchived || !ip.archived_at);
  const archivedCount = ips.filter((ip) => ip.archived_at).length;

  const submitNew = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const formElement = event.currentTarget;
    create.mutate(ipPayload(new FormData(formElement), host.id), {
      onSuccess: () => {
        formElement.reset();
        setAdding(false);
      },
    });
  };

  return (
    <Stack spacing={3}>
      {visible.length === 0 && !adding ? (
        <EmptyHint
          title="No addresses recorded"
          body="Add the public and private IPs this host answers on. Discovered addresses appear here too."
          action={
            !host.archived_at && (
              <Button variant="contained" onClick={() => setAdding(true)}>
                Add IP address
              </Button>
            )
          }
        />
      ) : (
        <Stack spacing={0.5}>
          {visible.map((ip) => (
            <Stack
              key={ip.id}
              direction="row"
              sx={{
                alignItems: "center",
                gap: 2,
                flexWrap: "wrap",
                px: 2,
                py: 1.5,
                borderRadius: "16px",
                border: `1px solid ${tokens.line}`,
              }}
            >
              <Stack sx={{ flex: "1 1 240px", minWidth: 0 }}>
                <Mono sx={{ fontSize: 16 }}>
                  {ip.address}
                  {ip.prefix_length == null ? "" : `/${ip.prefix_length}`}
                </Mono>
                <Typography sx={{ fontSize: 13, color: tokens.fog }}>
                  {[
                    scopes.find(([value]) => value === ip.scope)?.[1] ??
                      ip.scope,
                    ip.role,
                    ip.primary ? "primary" : null,
                    ip.interface_or_network,
                    ip.source === "discovered" ? "discovered" : "manual",
                    ip.archived_at ? "archived" : null,
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </Typography>
              </Stack>
              {ip.source === "manual" && (
                <Stack direction="row" spacing={1}>
                  {!ip.archived_at && (
                    <Button size="small" onClick={() => setEditing(ip)}>
                      Edit
                    </Button>
                  )}
                  <Button
                    size="small"
                    color={ip.archived_at ? "primary" : "inherit"}
                    onClick={() =>
                      lifecycle.mutate({
                        ip,
                        action: ip.archived_at ? "restore" : "archive",
                      })
                    }
                  >
                    {ip.archived_at ? "Restore" : "Archive"}
                  </Button>
                </Stack>
              )}
            </Stack>
          ))}
          {archivedCount > 0 && (
            <Button
              size="small"
              onClick={() => setShowArchived((value) => !value)}
              sx={{ alignSelf: "flex-start" }}
            >
              {showArchived
                ? "Hide archived"
                : `Show ${archivedCount} archived`}
            </Button>
          )}
        </Stack>
      )}
      {lifecycle.isError && (
        <Alert severity="error">
          {ipMessage(lifecycle.error, "Could not change that address.")}
        </Alert>
      )}
      {!host.archived_at && visible.length > 0 && !adding && (
        <Button
          variant="outlined"
          onClick={() => setAdding(true)}
          sx={{ alignSelf: "flex-start" }}
        >
          Add IP address
        </Button>
      )}
      {adding && (
        <Surface>
          <Stack
            component="form"
            spacing={2.25}
            onSubmit={submitNew}
            aria-label="Add IP address"
          >
            <SectionTitle>Add IP address</SectionTitle>
            <IPFields />
            {create.isError && (
              <Alert severity="error">
                {ipMessage(create.error, "Could not add the address.")}
              </Alert>
            )}
            <Stack direction="row" spacing={1}>
              <Button
                type="submit"
                variant="contained"
                disabled={create.isPending}
              >
                Add IP assignment
              </Button>
              <Button onClick={() => setAdding(false)}>Cancel</Button>
            </Stack>
          </Stack>
        </Surface>
      )}
      <SidePanel
        open={Boolean(editing)}
        onClose={() => setEditing(null)}
        title={editing ? `${editing.address}` : "Address"}
        mono
      >
        {editing && (
          <Stack
            component="form"
            spacing={2.25}
            onSubmit={(event: FormEvent<HTMLFormElement>) => {
              event.preventDefault();
              update.mutate(
                {
                  ip: editing,
                  body: ipPayload(new FormData(event.currentTarget), host.id),
                },
                { onSuccess: () => setEditing(null) },
              );
            }}
          >
            <IPFields initial={editing} />
            {update.isError && (
              <Alert severity="error">
                {ipMessage(update.error, "Could not save the address.")}
              </Alert>
            )}
            <Button
              type="submit"
              variant="contained"
              disabled={update.isPending}
              sx={{ alignSelf: "flex-start" }}
            >
              Save IP
            </Button>
          </Stack>
        )}
      </SidePanel>
    </Stack>
  );
}

function AgentTab({ host }: { host: Compute }) {
  const agent = useAgent(host.agent_id);
  const collectors = useCollectorStatus(host.id);
  const live = useCompute(host.id, {
    pollUntilOnline: host.agent_status !== "online",
  });
  const status = live.data?.agent_status ?? host.agent_status;
  const [confirm, setConfirm] = useState(false);
  const revoke = useInventoryMutation((current: Agent) =>
    sendJSON<Agent>(
      "POST",
      `/agents/${current.id}/revoke`,
      undefined,
      revisionTag(current),
    ),
  );
  return (
    <Stack spacing={4}>
      {agent.data && status !== "revoked" && (
        <Surface>
          <Stack spacing={2.5}>
            <Stack
              direction="row"
              sx={{
                alignItems: "center",
                justifyContent: "space-between",
                gap: 2,
                flexWrap: "wrap",
              }}
            >
              <StatusLight
                light={computeLight({ agent_status: status })}
                label={agentStatusLabel[status]}
              />
              <Button
                variant="outlined"
                color="error"
                onClick={() => setConfirm(true)}
              >
                Revoke agent credential
              </Button>
            </Stack>
            <FactList
              rows={[
                ["Last heartbeat", relativeTime(agent.data.last_heartbeat_at)],
                [
                  "Last snapshot",
                  relativeTime(agent.data.last_successful_snapshot_at),
                ],
                ["Enrolled", formatDateTime(agent.data.enrolled_at)],
                ["Client", <Mono key="c">{agent.data.client_version}</Mono>],
                [
                  "Protocol",
                  `v${agent.data.protocol_version} · ${agent.data.compatibility.replace("_", " ")}`,
                ],
                [
                  "Credential generation",
                  String(agent.data.credential_generation),
                ],
              ]}
            />
            {agent.data.last_error && (
              <Alert severity="warning">{agent.data.last_error}</Alert>
            )}
          </Stack>
        </Surface>
      )}
      {!host.archived_at && (
        <ConnectAgent computeID={host.id} agentStatus={status} />
      )}
      {(collectors.data ?? []).length > 0 && (
        <Box component="section" aria-labelledby="col-h">
          <SectionTitle id="col-h">Collectors</SectionTitle>
          <Stack spacing={0.5}>
            {(collectors.data ?? []).map((collector) => (
              <Stack
                key={collector.collector_key}
                direction="row"
                sx={{
                  gap: 2,
                  alignItems: "center",
                  px: 2,
                  py: 1.25,
                  borderRadius: "14px",
                  border: `1px solid ${tokens.line}`,
                }}
              >
                <StatusLight
                  light={collector.last_status === "failed" ? "failed" : "live"}
                />
                <Stack sx={{ flex: 1, minWidth: 0 }}>
                  <Typography sx={{ fontSize: 15 }}>
                    {collector.collector}
                    {collector.owner_uid == null
                      ? ""
                      : ` · UID ${collector.owner_uid}`}
                    {collector.connection_name
                      ? ` · ${collector.connection_name}`
                      : ""}
                  </Typography>
                  <Typography sx={{ fontSize: 13, color: tokens.fog }}>
                    {collector.last_status === "failed"
                      ? `Failed ${relativeTime(collector.last_received_at)}: ${collector.last_error_code ?? "error"} ${collector.last_error_message ?? ""} — previous facts kept`
                      : `Succeeded ${relativeTime(collector.last_successful_at ?? collector.last_received_at)}`}
                  </Typography>
                </Stack>
              </Stack>
            ))}
          </Stack>
        </Box>
      )}
      <Dialog open={confirm} onClose={() => setConfirm(false)}>
        <DialogTitle>Revoke this agent’s credential?</DialogTitle>
        <DialogContent>
          <Typography color="text.secondary">
            The agent on <Mono>{host.display_name}</Mono> will stop reporting
            immediately. You can enroll a new one later with a fresh token.
          </Typography>
          {revoke.isError && (
            <Alert severity="error" sx={{ mt: 2 }}>
              {revoke.error instanceof ApiError && revoke.error.status === 412
                ? "Agent state changed elsewhere; reload and try again."
                : "Could not revoke the agent."}
            </Alert>
          )}
        </DialogContent>
        <DialogActions sx={{ p: 2.5 }}>
          <Button onClick={() => setConfirm(false)}>Cancel</Button>
          <Button
            variant="contained"
            color="error"
            disabled={!agent.data || revoke.isPending}
            onClick={() =>
              agent.data &&
              revoke.mutate(agent.data, { onSuccess: () => setConfirm(false) })
            }
          >
            Revoke
          </Button>
        </DialogActions>
      </Dialog>
    </Stack>
  );
}

function EditComputeDrawer({
  host,
  open,
  onClose,
}: {
  host: Compute;
  open: boolean;
  onClose: () => void;
}) {
  const providers = useProviders();
  const locations = useLocations();
  const computes = useComputes();
  const [kind, setKind] = useState(host.kind);
  const save = useInventoryMutation((body: unknown) =>
    sendJSON<Compute>("PUT", `/computes/${host.id}`, body, revisionTag(host)),
  );
  const lifecycle = useInventoryMutation((action: "archive" | "restore") =>
    sendJSON<Compute>(
      "POST",
      `/computes/${host.id}/${action}`,
      undefined,
      revisionTag(host),
    ),
  );

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    save.mutate(
      {
        kind,
        display_name: String(form.get("display_name") ?? "").trim(),
        ownership: form.get("ownership"),
        provider_id: form.get("provider_id") || undefined,
        provider_resource_id:
          String(form.get("provider_resource_id") ?? "") || undefined,
        location_id: form.get("location_id") || undefined,
        parent_compute_id:
          kind === "vm"
            ? form.get("parent_compute_id") || undefined
            : undefined,
        description: String(form.get("description") ?? ""),
        notes: String(form.get("notes") ?? ""),
        tag_ids: host.tag_ids,
      },
      { onSuccess: onClose },
    );
  };

  return (
    <SidePanel open={open} onClose={onClose} title="Edit host">
      <Stack
        component="form"
        spacing={2.25}
        onSubmit={submit}
        key={`${host.id}-${host.revision}-${open}`}
      >
        <TextField
          name="display_name"
          label="Name"
          defaultValue={host.display_name}
          required
          sx={{ "& input": { fontFamily: fonts.mono } }}
        />
        <TextField
          label="Kind"
          select
          value={kind}
          onChange={(event) => setKind(event.target.value as Compute["kind"])}
        >
          <MenuItem value="physical">Physical</MenuItem>
          <MenuItem value="vps">VPS</MenuItem>
          <MenuItem value="vm">VM</MenuItem>
        </TextField>
        {kind === "vm" && (
          <TextField
            name="parent_compute_id"
            label="Runs on"
            select
            defaultValue={host.parent_compute_id ?? ""}
          >
            <MenuItem value="">Not tracked</MenuItem>
            {(computes.data ?? [])
              .filter((item) => item.id !== host.id)
              .map((item) => (
                <MenuItem key={item.id} value={item.id}>
                  {item.display_name}
                </MenuItem>
              ))}
          </TextField>
        )}
        <TextField
          name="ownership"
          label="Ownership"
          select
          defaultValue={host.ownership}
        >
          <MenuItem value="owned">Owned</MenuItem>
          <MenuItem value="rented">Rented</MenuItem>
          <MenuItem value="other">Other</MenuItem>
        </TextField>
        <TextField
          name="provider_id"
          label="Provider"
          select
          defaultValue={host.provider_id ?? ""}
        >
          <MenuItem value="">None</MenuItem>
          {(providers.data ?? []).map((item) => (
            <MenuItem key={item.id} value={item.id}>
              {item.name}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          name="location_id"
          label="Place"
          select
          defaultValue={host.location_id ?? ""}
        >
          <MenuItem value="">None</MenuItem>
          {(locations.data ?? []).map((item) => (
            <MenuItem key={item.id} value={item.id}>
              {item.name}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          name="provider_resource_id"
          label="Provider reference"
          defaultValue={host.provider_resource_id ?? ""}
        />
        <TextField
          name="description"
          label="Description"
          defaultValue={host.description ?? ""}
          multiline
        />
        <TextField
          name="notes"
          label="Notes"
          defaultValue={host.notes ?? ""}
          multiline
          minRows={3}
        />
        {save.isError && (
          <Alert severity="error">
            {save.error instanceof ApiError && save.error.status === 412
              ? "This host changed elsewhere. Close and reopen to edit the latest version."
              : save.error instanceof ApiError && save.error.status === 409
                ? "That provider reference is already registered."
                : "Could not save the host."}
          </Alert>
        )}
        <Stack direction="row" sx={{ gap: 1, flexWrap: "wrap", pt: 1 }}>
          <Button type="submit" variant="contained" disabled={save.isPending}>
            Save compute
          </Button>
          <Box sx={{ flex: 1 }} />
          <Button
            color="error"
            disabled={lifecycle.isPending}
            onClick={() =>
              lifecycle.mutate(host.archived_at ? "restore" : "archive", {
                onSuccess: onClose,
              })
            }
          >
            {host.archived_at ? "Restore" : "Archive host"}
          </Button>
        </Stack>
        {lifecycle.isError && (
          <Alert severity="error">
            Could not change this host’s lifecycle.
          </Alert>
        )}
      </Stack>
    </SidePanel>
  );
}
