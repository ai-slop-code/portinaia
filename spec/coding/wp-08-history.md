# WP-08 — Snapshot history, differences and retention

Dependencies: WP-06; canonical/report schema is frozen in WP-02 and the minimal
store is shared with WP-06 ingestion. Sources: product §10.6; implementation plan
§§7.4, 8.6, 14, 21/WP-08; decision D-01. Read [shared rules](coding-rules.md).

Primary paths: `internal/snapshots/`, history handlers, snapshot SQL and fixtures.
Baseline: unique format/hash table, report references and selected insert/list
queries; no canonicalizer, diff, gzip download or retention worker.

## WP-08.1 — Versioned structural content and report envelope

Define two explicit models, neither being the raw HTTP body:

| Shared immutable structural content                                               | Immutable per-report envelope                                                       |
| --------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| Canonical format version                                                          | Client report ID and unique server history ID                                       |
| Machine/OS/kernel/CPU/memory/boot identity and boot time                          | Agent/compute binding, protocol/client version                                      |
| Interface configuration and IPs                                                   | Collection start/end, server receipt, original acceptance result                    |
| Disk devices/capacity; filesystem identity/type/mount/total capacity              | Host uptime_seconds, filesystem available_bytes keyed by stable filesystem identity |
| Container runtime ID, state/health, images, restart, ports/networks/mounts/labels | Content ID, content-reused flag, request fingerprint                                |
| Collector outcome and sanitized stable error code                                 | Sanitized diagnostic error text and collection timing                               |

1. Explicit allowlist determines volatile fields; do not exclude arbitrary changed
   fields to improve a deduplication metric. Current relational observations still
   store latest volatile measurements. Retained reports preserve their measurements.
2. Canonical structural bytes: UTF-8 JSON, fixed schema/field ordering, sorted
   set-like collections, canonical names/IPs/enums, whole-second UTC timestamps,
   integer numerics, stable omitted/NULL policy. Never sort ordered semantic values
   as if they were sets. Reject duplicate logical keys before canonicalization.
3. Stable keys: interface name; disk device name; filesystem device + mount point;
   container owner UID + name within compute; nested network name, mount destination,
   label key and normalized port tuple. Boot and runtime changes remain state changes.
4. SHA-256 of uncompressed structural bytes; gzip afterward with deterministic
   headers. Store immutable bytes keyed by format/hash, not report ID. Do not put
   the hash itself in its input. Confirm size/hash when retrieving stored content.
5. Report envelope references content in the same ingestion transaction; reuse
   across equivalent reports is allowed. Each accepted report retains its own
   receipt/time/envelope row even if content is reused. A duplicate PUT does not
   produce another report or refresh its received_at.
6. For gzip download, reconstruct the requested report's envelope + structural
   state + volatile values and expose structural SHA-256 with an explicit meaning.
   Do not stream a shared blob with some other report's ID. Stream a bounded gzip
   export; response schema/documentation distinguishes file from JSON API.

## WP-08.2 — Timeline, download and structured differences

Implement agent/compute timelines, discovery report detail, snapshot download,
snapshot comparisons and container-scoped history required by the frontend.
History authorization uses `agents`; check compute/agent consistency for pair
comparisons. Initial comparison supports two reports of the same compute only;
reject cross-compute pairs with 422 rather than inferring unrelated identities.

Timeline is receipt-descending with ID tie-breaker and explicit collector failures/
content reuse. Missing/expired report is a stable not-found response. Download
has safe Content-Disposition and gzip streaming without loading unbounded history.

Diff reconstructs both report states and produces deterministic host scalar,
interface/IP, disk, filesystem, container and collector changes. Runtime ID
recreation is changed on the same logical container. Include volatile uptime/
available-byte differences in a clearly labeled measurements section. Same
structural hash can still have volatile differences. Failed collector -> success
is coverage change, not evidence that unobserved children were removed; mark
unavailable comparison sections rather than inventing deletions.

Bound differences by explicit limit/continuation agreed in WP-02; never silently
truncate. All entries can be retrieved through paging/export. Decode errors or
unsupported canonical formats produce sanitized errors, not partial false diffs.

## WP-08.3 — Retention and observability

Default 90 days by server receipt, configurable. At each bounded pass:

1. Capture cutoff once using server clock and select oldest report IDs with index.
2. Transactionally delete at most configured batch (initially 1000), including
   report-owned collector/volatile rows. Current references may SET NULL; current
   facts themselves persist independently.
3. Delete only bounded content rows with no retained references, verified in the
   same writer transaction; concurrent ingestion must never lose referenced content.
4. Yield between batches, honor shutdown, report partial progress and retry safely.
   Do not automatically VACUUM or delete audit. DB allocated bytes need not shrink
   when live data is removed; metrics distinguish logical reclaim from file size.
5. Expose diagnostic dry-run counts/cutoff via documented admin command, without
   adding a purge-all-history operation. Export ingestion latency/bytes/reuse,
   retained reports/content, cleanup duration/deleted rows/errors with bounded labels.

## Acceptance / tests

- HIS-01: Reordered sets yield same bytes/hash; changed runtime/boot/state changes
  hash; changed uptime/free space shares content and retains per-report values.
- HIS-02: Duplicate reports do not add timestamps; different accepted reports do.
  Download ID/times/measurements match requested report even for shared content.
- HIS-03: Golden structured diffs cover every fact group, collector failure,
  unknown/missing values, recreation, same hash/different measurements and paging.
- HIS-04: Concurrent ingest/retention cannot delete referenced content; current
  observations/manual metadata/audit survive; dry-run makes no changes.
- HIS-05: Representative structural and volatile storage measured at target report
  volume; no assumption that structural dedup makes all history nearly free.

Commands: `go test ./internal/snapshots/... ./internal/discovery/... ./internal/database/... ./internal/api/...`,
`make verify`; WP-16 capacity fixture runs. No raw body archive or arbitrary text-only diff.
