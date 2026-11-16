# Runtime, deployment and recovery requirements

Use the existing Go process, SQLite layer, command names and deployment work.
This is remaining acceptance, not a request to re-scaffold runtime or build a
future configuration/metrics framework. Operational instructions live in
[deploy/recovery.md](../../deploy/recovery.md).

## RT-01 — Startup and SQLite

- Validate configured listener, public URL/proxy CIDRs, DB paths and active feature
  bounds; document CLI/environment/config precedence. Do not require future DNS paths
  while DNS is disabled. Invalid input fails before opening listeners.
- Only explicit `db migrate` changes schema. Server/bootstrap/reset check goose and
  application schema agreement, refusing missing/old/inconsistent/newer versions
  with useful instructions. Preserve immutable migrations and populated user data.
- Per-connection foreign keys, bounded busy timeout, WAL and synchronous FULL for
  file-backed SQLite, bounded connection usage and short write transactions. Network
  and filesystem publication stay outside DB write transactions.
- Keep pure-Go cross-builds for Linux amd64/arm64; exercise actual execution where
  claimed. Local CLI schema operations must not race global migration setup.

**Accept RT-01:** all schema states, explicit fresh install, supported upgrade,
pragmas on acquired connections, bounded contention and cleanup on failed startup.

## RT-02 — Requests, health and shutdown

- Configured public HTTPS controls cookie security even behind HTTP-to-backend TLS
  termination. Only trusted peers supply client identity; forwarded scheme cannot
  weaken cookies. Document loopback development behavior.
- One validated request identity and sanitized structured logs/problems/audit.
  Enforce body sizes, request execution deadlines and cancellation; socket timeouts
  alone do not bound handlers. Do not log raw credentials/bodies/provider headers.
- Liveness reports responsiveness. Readiness checks compatible schema and bounded
  rollback write probe, plus required paths for enabled features. Cloudflare outage
  alone is not a readiness failure.
- On shutdown stop scheduling/intake, drain or cancel within deadline, join workers,
  and close DB only after remaining work terminates. Do not claim unfinished work
  succeeded. Test both normal drain and deadline/forced-cancellation paths.

**Accept RT-02:** trusted/untrusted headers, proxy cookie, read-only readiness,
same correlation throughout failures/audit, bounded expensive work, panic/error
handling, in-flight mutation drain and forced-shutdown resource cleanup.

## RT-03 — Installation and service operation

- Non-root server image with CA trust and dedicated persistent DB/zone mounts;
  configuration and secret files have documented ownership/modes.
- Root-owned native agent service can read explicitly configured rootful/rootless
  owner sockets. Preserve valid sandbox/AF_NETLINK fixes. Do not silently recommend
  an unimplemented alternate service/state model.
- Verify checksum, install, activate required socket, enroll from private token file,
  remove token, then enable service. Do not start before enrollment in the runbook.
- Stable permanent/transient exit codes and unit restart policy agree: revocation
  does not produce an authenticated restart loop. Explicit re-enrollment handles
  credential and pending-state association. Include deploy/config examples in releases.

**Accept RT-03:** executable install/upgrade commands, checksums and stated architecture
execution; native service start/restart/SIGTERM, transient outage, revocation and
re-enrollment. Record build hashes, OS/Podman versions, commands and permissions.

## RT-04 — Backup and isolated restore

- Online backup is consistent, verified, private, cancellation-safe and refuses
  accidental destination overwrite. Never copy a live SQLite file ignoring WAL.
- Restore into a clean isolated destination with matching configuration/secrets,
  permissions and no stale WAL/SHM from another database. Preserve failed original
  DB/sidecars for diagnosis. Verify integrity, FKs and schema; migrate explicitly.
- Review credentials whose later revocation could be undone by restoring a backup.
- Once DNS exists, repair current desired state without adopting unknown Cloudflare
  records, regressing SOA serials or implying DB restore rolled back remote changes.
  Preserve last valid zone files until a validated replacement is published.

**Accept RT-04:** backup/verify and actual isolated restored-server reads, failed
backup/cancellation, no production paths/provider credentials in drills; after M4,
real publication/provider recovery and M1–M4 browser workflows on the restored build.
