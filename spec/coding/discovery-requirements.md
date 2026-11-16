# Current discovery and agent requirements

No historical timeline/diff/export UI is required. The storage, identity and
retry semantics below support current inventory and remain in scope.

## DI-01 — Enrollment and authority

- Single-use, expiring, compute-bound enrollment tokens with exactly one winner
  under concurrent exchange. Store credential digests, return issued secrets once.
  Lost successful responses require explicit revoke/reissue recovery.
- Re-enrollment preserves agent identity/history and increments generation. Recheck
  credential generation/revocation and compute activity in ingestion's transaction.
- Support protocol v1, advertised range `[1,1]`; validate response ranges rather
  than fabricating v0. Version output separates build/API/protocol/canonical format.
- Required collector members must be present. Explicit UID 0 is valid; missing/null
  UID is not root. Empty successful collection is authoritative; omission/failure
  is not. A failed required Podman call invalidates that owner's partial list only.
- Enforce independent compressed and decompressed upload limits, bounded nested
  collection counts and decode/validation work before expensive processing. Accepting
  gzip does not waive the uncompressed limit or permit unbounded decompression.
- Separate host/interface/disk/filesystem and explicit verified numeric Podman-owner
  authority. Duplicate owner configuration is invalid. A missing top-level required
  array cannot silently become a successful empty array during DTO conversion.

**Accept DI-01:** concurrent enrollment/revoke/ingest, lost issuance response,
missing/null/zero/empty union cases, compressed/decompressed/count-limit rejection
and no mutation on malformed authority.

## DI-02 — Current facts, identity and replay

- Container identity is compute + verified owner UID + exact name. Runtime recreation
  preserves logical ID/manual metadata; rename is a different logical container.
  Rediscovery may reactivate the unambiguous matching archived container with audit.
- Manual facts are never overwritten. Collector failure preserves previous values;
  only successful scoped results establish absence. No automatic inventory archival.
- Keep last contact, last accepted report and last fully successful observation
  distinct. Freshness uses server receipt time. Default heartbeat is 60 seconds,
  snapshot 300 seconds, offline threshold 300 seconds, child missing threshold
  24 hours; provide enrollment grace and configurable thresholds. Reconcile any
  server schedule response/local-override differences explicitly.
- Path/body report IDs agree. Same agent/report ID with equivalent fingerprint
  returns original acceptance, not new facts/audit/freshness; changed content conflicts.
  Serialize uploads; superseded work must not be uploaded after newer work has been
  sent, because a lost response may already have committed it. Server current-state
  application follows authenticated accepted receipt order, not client timestamps.
  Old acknowledgements cannot clear a newer pending report.

**Accept DI-02:** recreation, failure versus empty/absence/offline, independent owners,
manual preservation, reactivation, duplicates/conflicts, archive races and rollback.

## DI-03 — Immutable structural content and report envelopes

| Shared structural content                                                 | Per-report envelope / volatile data                                                  |
| ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Format version, machine/OS/kernel/CPU/memory/boot identity/time           | Client report ID, server history ID, agent/compute binding, protocol/client versions |
| Interfaces/IPs, disks/capacity, filesystem identity/type/mount/total      | Collection/receipt times, original acceptance, content ID/reuse/fingerprint          |
| Container runtime/state/health/image/restart/ports/networks/mounts/labels | Host uptime and per-filesystem available bytes                                       |
| Collector outcome and stable sanitized error code                         | Sanitized diagnostic text and timing                                                 |

Canonical UTF-8 JSON has explicit field order, sorted set-like collections, stable
omitted/null policy, integer numerics, canonical names/IPs/enums and whole-second
UTC times. Reject duplicate keys before serialization. Stable keys: interface name;
disk device; filesystem device + mount; owner UID + container name; nested network
name, mount destination, label key and normalized port tuple. Do not sort semantic
ordered arrays or exclude changing fields opportunistically for deduplication.

SHA-256 hashes uncompressed structural bytes; gzip afterward deterministically.
Never hash the hash into itself. Store immutable content by format/hash and report
references atomically. Separate reports preserve separate volatile measurements
even when content is reused. No raw HTTP body archive.

**Accept DI-03:** deterministic reordering/hash, changed runtime/boot versus volatile
fields, immutable envelopes, duplicate no-op and transactional content/reference safety.

## DI-04 — Bounded retention and retry protection

Unknown reports older than the 90-day collection-time replay window are rejected.
Retained identical duplicates return original acceptance. Receipt-based cleanup must
retain all duplicate evidence still needed within that window, including the accepted
five-minute future clock skew. Current implementation enforces a 90-day minimum
retention and a 90-day-plus-five-minute cleanup floor; preserve that invariant.

Default hourly cleanup uses indexed batches of 1000, yields and honors cancellation.
Delete report-owned metadata/measurements and only unreferenced content; preserve
current facts, manual data, audit, identity and absence state. Dry-run describes the
same enforceable policy. No automatic VACUUM or claim logical deletion shrinks files.

**Accept DI-04:** replay after cleanup cannot overwrite newer facts; short unsafe
configuration refused; default/longer retention, exact cutoff/skew, shared content,
concurrent ingestion, rollback, interruption/resume and dry-run consistency.

## DI-05 — Agent scheduling, private state and protocol handling

- Use HTTPS with certificate/hostname verification outside explicitly documented
  loopback/test use; support configured CA trust, not production insecure-skip-verify.
- Independent heartbeat/collection/upload cadence. Collection proceeds during
  retry/backoff and atomically supersedes one bounded latest pending complete report.
  At most one upload; conditional acknowledgement must be safe against replacement
  and concurrent processes, or enforce one exclusive state owner explicitly.
- Validate full bounded credential/pending/response documents, required values and
  trailing/oversized data before use or deletion. Associate pending work with the
  correct enrollment/generation; record acceptance metadata and explicit reset rules.
- Private owner-checked files/directories, safe paths, atomic temporary write + fsync
  - rename + directory sync; preserve correct state through failed persistence.
- Propagate cancellation through HTTP/Podman/collectors, bound overall collection
  including inaccessible filesystems, and join workers before exit. A abandoned
  goroutine blocked in a syscall is not a sufficient cancellation strategy.
- Capped jittered retry, bounded Retry-After parsing, distinct transient/permanent
  errors for HTTP, config/state and malformed responses; stable CLI exit semantics.
  Revoked/invalid credentials stop until explicit repair, including under systemd.
- Validate accepted range, result, IDs, required booleans (false is not omission),
  schedule and warnings. Document local overrides versus server suggestions. Use
  the running Portinaia version after binary upgrades, not enrollment's old build.
- Strict supported config, CA/timeouts/retry controls, validation and redacted
  collection-only diagnostics. No inbound listener, shell command channel, environment
  collection, secrets or raw inspect persistence.

**Accept DI-05:** fake-clock delayed upload/supersession, stale ack/restart, concurrent
state ownership, partial writes, malformed responses retaining pending work, Retry-After
overflow, TLS trust/rejection, terminal failures, cancellation, worker joining and
secret sentinels.

## DI-06 — Linux/Podman collection and real-host acceptance

Use explicit owner-verified Unix sockets and a read-only endpoint allowlist, not
socket discovery. Parse Linux facts using bounded injectable sources; unknown values
remain unknown. Validate machine/boot identity, numeric overflow, port/protocol and
restart/mount normalization. A missing restart policy is not fabricated as "no".
Bound connection-wide work and inspection concurrency, not merely each HTTP call.

Test supported protocol versions separately from actual daemon-version evidence.
Preserve the real Fedora CoreOS/Podman 6.1.1 rootful/rootless pilot and native service
work already recorded. It does not prove all newer code; new evidence must name the
tested build. A virtualized Linux host is valid functional evidence.

**Accept DI-06:** actual host/container visibility, both owner modes, failed socket
then recovery, successful empty/removal versus failure, recreation preserving ID,
ambiguous upload and restart retry with body/ID evidence, supersession, manual-state
preservation, revocation stopping service attempts, and representative storage counts.
Cross-builds/socket fixtures alone do not establish real-host behavior. No physical
Pi capacity or arbitrary multi-distribution matrix is required for this gate.
