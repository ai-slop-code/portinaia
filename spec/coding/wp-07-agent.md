# WP-07 — Linux discovery client

Dependencies: frozen WP-02 protocol; WP-06 for integration completion.
Sources: product §10; implementation plan §§5.2, 13, 21/WP-07.
Read [shared rules](coding-rules.md), D-04 and WP-06 collector semantics.

Primary paths: `agent/config/`, `collector/host/`, `collector/podman/`,
`protocol/`, `reporter/`, `state/`, `cmd/portinaia-agent/`, agent deployment examples.
Baseline: version command and cross-build recipes only.

## WP-07.1 — Config, credentials and local state

1. Strict versioned YAML: HTTPS server URL, optional custom CA, credential/token/
   state paths, intervals, request timeout/retry bounds, protocol version, explicitly
   configured Podman connections/socket paths/expected numeric UID.
2. Reject duplicate names/owners and invalid socket path/owner/type. Never scan
   arbitrary user sockets. HTTPS verification is mandatory outside test transports;
   no production insecure-skip-verify flag. No inbound listener.
3. Load existing credential first; otherwise exchange single-use token and atomically
   persist credential/generation before making token unusable. A lost success
   response produces actionable re-enrollment diagnostics, not infinite token replay.
4. Files are root-only 0600 under root-only directory. Temporary file in same
   filesystem -> write -> chmod -> flush -> close -> rename -> directory flush.
   Reject unsafe symlinks/ownership; do not truncate the previous file first.
5. Keep separate credential and retry-state files. State has latest complete
   protocol report, stable UUIDv7 report ID and last acceptance metadata. It never
   stores an unbounded queue or replay backlog. Validate bounded state on restart.

## WP-07.2 — Host collectors

Use injectable stable Linux file/system APIs; Linux-specific implementations use
build tags, with fixture-driven portable tests. No shell execution for collection.

| Fact                     | Source and normalization                                                                                                     |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------------- |
| Identity/OS              | `/etc/machine-id`, `/etc/os-release`; trim/validate, no shell expansion; required missing identity reports collector failure |
| Host/kernel/architecture | Stable Go/Linux APIs; normalized architecture names; preserve observed hostname separately from display name                 |
| CPU/memory               | `/proc/cpuinfo`, `/proc/meminfo`; integer byte/count conversion, overflow/malformed checks                                   |
| Boot/uptime              | boot ID and `/proc/uptime`; whole-second uptime, validated boot time, no identity based on uptime                            |
| Interfaces/IPs           | netlink or stable maintained network API; canonical IP/prefix, MAC, MTU, flags; deterministic sorting                        |
| Disks                    | Stable sysfs device sizes/model and device identity; checked sector-to-byte conversion; no serial/SMART probing              |
| Filesystems              | mount/stat APIs; device, fs type, mount point, total/available bytes; bound/block-timeout handling for inaccessible mounts   |
| Virtualization           | Reliable local evidence only; unknown is valid; no KVM guest enumeration                                                     |

Return separate collector outcomes. A missing optional scalar is unknown; inability
to enumerate a collection is failure, not authoritative empty. Do not follow
untrusted filesystem links or read arbitrary file contents from mount/container
paths. Filesystem measurement reads are metadata operations only.

## WP-07.3 — Read-only Podman collection

1. Use Unix HTTP transport, verified configured socket UID, per-connection timeout.
   Pin/test a supported Libpod REST API version range; document exact minimum and
   fixtures before implementation is marked complete. Unsupported version is a
   failed collector with a safe message, not a best-effort empty list.
2. List all containers (`all=true` equivalent). Inspect only missing required
   facts with bounded concurrency (initially four). If any required request fails,
   the entire connection result is failed; discard its partial authoritative list.
3. Normalize all states, health, images/IDs/digest, times, exposed/published ports,
   networks/addresses, mounts, labels and restart policy. Unknown states map to
   documented unknown/other, never a false stopped/no-restart default.
4. Container identity owner is verified numeric UID, connection is human label.
   Renaming connection does not change inventory identity. Report exact container
   name and new runtime ID after recreation.
5. Issue only read endpoints; an allowlisted HTTP method/path transport test proves
   absence of mutation calls. Never include environment variables, auth data,
   secret file contents or raw inspect payloads. Labels are bounded metadata;
   redact known sensitive keys consistently before current/history/audit storage.

## WP-07.4 — Scheduling, upload, retry and diagnostics

Heartbeat and snapshot schedules are independent, startup-jittered and cancellation
aware. Defaults 60/300 seconds. One collection at a time and one snapshot upload
at a time; heartbeats continue during slow/failing collectors and uploads.

After a complete protocol document is produced (possibly failed collector outcomes),
atomically replace latest pending state. Retry same report ID and unchanged bytes
until accepted or superseded. Before sending, ensure candidate is still latest.
An in-flight older report can finish, but its acknowledgement must not clear a
newer pending report; compare IDs. Do not send old work after sending newer work.
Never replay missed heartbeats. Network/429/5xx use bounded full-jitter exponential
backoff and Retry-After; validation/unsupported protocol stops report retry with
diagnostic, auth revocation stops authenticated reporting pending operator repair.

Commands: preserve `version`; add config validation and one-shot redacted collection
diagnostic with no upload unless an explicit upload flag is given. Exit codes
distinguish configuration, credential/protocol, collection and network failures.
Logs are structured journal-compatible, no tokens or snapshot bodies. Provide
root-owned systemd unit and external Ansible variable examples, not an Ansible role.

## Acceptance / tests

- AGT-01: Linux host fixtures cover missing/malformed/overflow/symlink facts;
  mock Unix sockets cover root and rootless success/failure, inspect pagination,
  all container states and unsupported versions.
- AGT-02: Offline restart retains latest pending report only; injected write/
  rename failure preserves old state; stale acknowledgement cannot drop new report.
- AGT-03: Heartbeat progresses during failed collection; auth revocation stops
  report retries; schedule jitter/backoff tested with fake clocks, not long sleeps.
- AGT-04: No mutation endpoint, shell channel or inbound listener; credential/file
  ownership and secret-redaction tests; v1 end-to-end enrollment/upload succeeds.
- AGT-05: Linux amd64/arm64 binaries and checksums produced and installed by the
  documented systemd procedure on Linux.

Commands: `go test ./agent/...`, `make verify`, `make build-agent`, `make release`;
run Linux-specific integration tests on Linux. No self-update or service discovery.
