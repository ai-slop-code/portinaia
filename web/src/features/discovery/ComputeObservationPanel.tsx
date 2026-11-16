import {
  Alert,
  Card,
  CardContent,
  Chip,
  Stack,
  Typography,
} from "@mui/material";
import type { components } from "../../api/generated";

type ComputeObservation = components["schemas"]["ComputeObservation"];
type CollectorObservation = components["schemas"]["CollectorObservation"];
type Container = components["schemas"]["Container"];
type IpAssignment = components["schemas"]["IpAssignment"];

export type ComputeObservationPanelProps = {
  observation?: ComputeObservation;
  collectors?: CollectorObservation[];
  containers?: Container[];
  ipAssignments?: IpAssignment[];
  agentStatus?: components["schemas"]["ComputeAgentStatus"];
  readError?: string;
  unavailable?: { containers?: boolean; ipAssignments?: boolean };
};

function localTime(value?: string) {
  if (!value) return "Never";
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? value
    : `${date.toLocaleString()} (${Intl.DateTimeFormat().resolvedOptions().timeZone})`;
}

function collectorState(item: CollectorObservation) {
  if (item.last_status === "failed") return "Failed — previous facts retained";
  return item.last_successful_at
    ? "Successful; an empty result confirms absence"
    : "No successful result";
}

export function ComputeObservationPanel({
  observation,
  collectors = [],
  containers = [],
  ipAssignments = [],
  agentStatus,
  readError,
  unavailable,
}: ComputeObservationPanelProps) {
  return (
    <Card>
      <CardContent>
        <Typography variant="h6">Current host facts</Typography>
        {readError && (
          <Alert severity="warning" sx={{ mt: 1 }}>
            Could not refresh current facts. Showing the last known data.{" "}
            {readError}
          </Alert>
        )}
        {agentStatus && (
          <Typography sx={{ mt: 1 }}>Agent: {agentStatus}</Typography>
        )}
        {!observation ? (
          <Typography color="text.secondary" sx={{ mt: 1 }}>
            No successful host collection is available yet; facts are unknown,
            not confirmed absent.
          </Typography>
        ) : (
          <Stack spacing={0.5} sx={{ mt: 1 }}>
            <Typography>Observed hostname: {observation.hostname}</Typography>
            <Typography>
              OS:{" "}
              {[observation.os_name, observation.os_version]
                .filter(Boolean)
                .join(" ") || "Unknown"}
            </Typography>
            <Typography>
              Kernel: {observation.kernel_version ?? "Unknown"} · architecture:{" "}
              {observation.architecture}
            </Typography>
            <Typography>
              CPU: {observation.cpu_model ?? "Unknown"} ·{" "}
              {observation.logical_cpu_count} logical CPUs ·{" "}
              {observation.memory_bytes} bytes memory
            </Typography>
            <Typography>
              Machine identity: {observation.machine_id ?? "Unknown"} · boot ID:{" "}
              {observation.boot_id ?? "Unknown"}
            </Typography>
            <Typography>
              Boot time: {localTime(observation.booted_at)} · uptime:{" "}
              {observation.uptime_seconds == null
                ? "Unknown"
                : `${observation.uptime_seconds} seconds`}
            </Typography>
            <Typography color="text.secondary" variant="caption">
              Last observed: {localTime(observation.observed_at)}
            </Typography>
            <Typography variant="subtitle2">
              Interfaces and addresses
            </Typography>
            {observation.interfaces.length === 0 ? (
              <Typography variant="body2">
                No interfaces confirmed by the latest successful collector.
              </Typography>
            ) : (
              observation.interfaces.map((item) => (
                <Typography key={item.name} variant="body2">
                  {item.name} (
                  {item.up === undefined
                    ? "link state unknown"
                    : item.up
                      ? "up"
                      : "down"}
                  ){item.mac_address ? ` · ${item.mac_address}` : ""}:{" "}
                  {item.addresses
                    .map(
                      (address) =>
                        `${address.address}${address.prefix_length == null ? "" : `/${address.prefix_length}`}`,
                    )
                    .join(", ") || "No addresses observed"}
                </Typography>
              ))
            )}
            <Typography variant="subtitle2">Disks</Typography>
            {observation.disks.length === 0 ? (
              <Typography variant="body2">
                No disks confirmed by the latest successful collector.
              </Typography>
            ) : (
              observation.disks.map((disk) => (
                <Typography key={disk.name} variant="body2">
                  {disk.name}: {disk.model ?? "Unknown model"} ·{" "}
                  {disk.size_bytes} bytes
                </Typography>
              ))
            )}
          </Stack>
        )}
        <Typography variant="subtitle2" sx={{ mt: 2 }}>
          Collector status
        </Typography>
        {collectors.length === 0 ? (
          <Typography color="text.secondary" variant="body2">
            No collector status recorded.
          </Typography>
        ) : (
          <Stack spacing={0.75} sx={{ mt: 0.5 }}>
            {collectors.map((item) => (
              <Stack
                key={item.collector_key}
                direction="row"
                spacing={1}
                sx={{ alignItems: "center", flexWrap: "wrap" }}
              >
                <Chip size="small" label={item.collector_key} />
                <Typography variant="body2">
                  {collectorState(item)} · latest report received{" "}
                  {localTime(item.last_received_at)}
                </Typography>
                {item.last_successful_at && (
                  <Typography variant="caption">
                    Last success {localTime(item.last_successful_at)}
                  </Typography>
                )}
                {item.last_failed_at && (
                  <Typography color="error" variant="caption">
                    Last failure {localTime(item.last_failed_at)}
                    {item.last_error_message
                      ? `: ${item.last_error_message}`
                      : ""}
                  </Typography>
                )}
              </Stack>
            ))}
          </Stack>
        )}
        <Typography variant="subtitle2" sx={{ mt: 2 }}>
          Current discovered IP assignments
        </Typography>
        {unavailable?.ipAssignments ? (
          <Typography color="text.secondary" variant="body2">
            Discovered IP assignments are unavailable; no absence is inferred.
          </Typography>
        ) : ipAssignments.length === 0 ? (
          <Typography variant="body2">
            No discovered IP assignments are currently available; an empty list
            is not proof of absence until its collector succeeded.
          </Typography>
        ) : (
          ipAssignments
            .filter((item) => item.compute_id)
            .map((item) => (
              <Typography key={item.id} variant="body2">
                {item.address}
                {item.prefix_length == null
                  ? ""
                  : `/${item.prefix_length}`} ·{" "}
                {item.interface_or_network ?? "interface unknown"} · last
                observed {localTime(item.observation?.last_observed_at)}
                {item.observation?.stale_at
                  ? ` · stale since ${localTime(item.observation.stale_at)}`
                  : ""}
              </Typography>
            ))
        )}
        <Typography variant="subtitle2" sx={{ mt: 2 }}>
          Current containers
        </Typography>
        {unavailable?.containers ? (
          <Typography color="text.secondary" variant="body2">
            Containers are unavailable; no absence is inferred.
          </Typography>
        ) : containers.length === 0 ? (
          <Typography variant="body2">
            No current containers returned. Check the collector result above
            before treating this as confirmed absence.
          </Typography>
        ) : (
          containers.map((item) => (
            <Typography key={item.id} variant="body2">
              {item.name} · runtime ID{" "}
              {item.observation?.runtime_id ?? "unknown"} ·{" "}
              {item.observation?.state ?? "not observed"} · owner UID{" "}
              {item.podman_owner_uid}
              {item.podman_connection
                ? ` · connection ${item.podman_connection}`
                : ""}
              {item.observation?.stale_at
                ? ` · stale since ${localTime(item.observation.stale_at)}`
                : ""}
              {item.observation?.last_observed_at
                ? ` · last observed ${localTime(item.observation.last_observed_at)}`
                : ""}
            </Typography>
          ))
        )}
      </CardContent>
    </Card>
  );
}
