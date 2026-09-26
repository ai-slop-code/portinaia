# WP-06 — Enrollment and discovery protocol server

Dependencies: WP-05; WP-02 freezes WP-08 canonical/report contracts first.
Sources: product §§8–10; implementation plan §§7.4, 8.4–8.6, 12, 21/WP-06.
Read [shared rules](coding-rules.md), D-01/D-03/D-04.

Primary paths: `internal/discovery/`, discovery/agent handlers and SQL,
transport-only `agent/protocol/`. Coordinate auth, current inventory and snapshot
content primitives. Baseline: credential/report uniqueness and selected queries;
no real enrollment, heartbeat, ingestion or freshness worker.

## WP-06.1 — Enrollment and administration

1. Enrollment-token creation binds to an existing active compute; agents scope
   required. Issue 32 random bytes, return once/no-store, persist digest. Revoke
   expired outstanding token rows before replacing them, satisfying active index.
   A second still-live token requires explicit replacement/revocation behavior.
2. Enroll authenticates only enrollment bearer, accepts client/protocol metadata
   and cannot supply/change compute binding. Consume token and create/update the
   compute's unique agent in one transaction with audit. Concurrent exchange has
   exactly one winner. Reject expired, consumed, revoked tokens or archived compute.
3. Existing active agent requires explicit rotation before enrollment; do not
   replace a working credential merely because a new token was requested.
4. Revoke increments agent revision and invalidates immediately. Rotate checks
   revision, revokes credential and pending enrollment tokens, issues replacement
   enrollment token once. Actual successful re-enrollment increments credential
   generation and preserves agent ID/report history. Lost enrollment response
   requires explicit rotation/re-enrollment; server cannot recover plaintext.
5. Agent list/detail expose status, compatibility, version, receipt/heartbeat,
   errors and generation, never hashes. Use bounded cursor/filter queries.

## WP-06.2 — Protocol boundary and heartbeats

`/api/v1/agent/v1/enroll`, `/heartbeat`, `/snapshots/{report_id}` use isolated
authentication. First accepted protocol range is [1,1]; unsupported versions fail
without changing observations. Future v2 adds actual v1 adapter fixtures.

Heartbeat records server receipt time, client/protocol version, sanitized collection
error; works despite failed collectors. Throttle timestamp writes if necessary
without permitting more than 60 seconds of freshness error. Return server time,
accepted range, safe schedule values and compatibility warnings, no commands.
Revoked/archived binding cannot heartbeat. Record each persisted business state
change through compact audit according to WP-04.

## WP-06.3 — Snapshot ingestion and reconciliation

Ordered implementation:

1. Authenticate bound agent; enforce content type, protocol, request timeout,
   compressed/decompressed bytes and nested collection limits before expensive work.
2. Validate path/body client report ID equality (409 on mismatch). Decode strictly
   and validate each collector outcome, duplicate identities, port ranges, enums,
   address prefixes, collection start <= finish and diagnostic clock skew bounds.
3. Compute normalized request fingerprint. Same agent/report ID and same payload
   returns persisted prior acceptance without altering freshness, audit or current
   state; different payload returns 409. Authentication is always checked first.
4. Normalize/hash/compress structural state outside writer transaction via WP-08
   primitives. Separate report volatile data. Do not retain raw request bytes.
5. Transaction: recheck credential generation/revocation/compute activity, then
   replay check, insert/reuse content, insert immutable report/collector results,
   reconcile successful current collectors, update agent result, audit, commit.
   A revoke/archive racing normalization must prevent commit with old credentials.
6. Replace successful host/interface/disk/filesystem facts only in their owned
   columns/tables. Failed collector retains previous facts and reports error.
7. For each successful Podman owner scope, upsert by logical identity, preserve
   UUID/manual fields/DNS links, replace current child facts transactionally and
   record new runtime ID. Reactivate the matching archived row with audit.
   Enforce uniqueness across archived identities or reject legacy ambiguity for
   explicit cleanup; never choose an arbitrary archived row.
8. Complete success may establish absence for previously observed children in
   that exact scope. Failed/omitted scope cannot establish absence. Observed
   children clear stale/absence state. Manual IPs are never touched.
9. Keep freshness based on receipt. The agent serializes uploads per host and
   prevents superseded requests being sent after newer reports; current-state
   application follows accepted receipt order, not untrusted client clock order.
   Preserve old reports as history; do not claim cross-client ordering guarantees.

`accepted_with_errors` is accepted history with explicit collector failures, not
a rejected report and not authoritative empty output. Last successful snapshot
and last accepted report semantics must be explicit: the success timestamp advances
only when all configured required collectors succeed; expose last accepted receipt
separately so operators can see partial progress.

## WP-06.4 — Offline, stale and rediscovery

Compute offline: active enrolled agent has no heartbeat within five minutes;
never-heartbeaten agent uses enrollment time as initial grace window. Unenrolled
and revoked are distinct states. Missing child: authoritative success established
absence, and receipt-time age since last observation reaches 24 hours. Collector
failures alone do not mark children missing; display collector/agent freshness
separately. A failure after proven absence preserves that absence and policy age.

Run bounded freshness worker or derive stable read state with audited transitions;
do not rewrite unchanged rows each pass. Never auto-archive. Rediscovery clears
absence/stale; archived container reactivates with same ID/manual metadata.
Export bounded online/offline/revoked/stale and collector-result metrics.

## Acceptance / tests

- DSC-01: Concurrent single-use enrollment, wrong token kind, wrong binding,
  expiry/revocation, lost response recovery, rotation generations and archive race.
- DSC-02: Equal report replay, changed-content conflict and cross-agent report ID
  reuse; transaction rollback leaves no partial report/content/current/audit state.
- DSC-03: Empty success removes authority only within that owner scope; failure
  preserves children. One failed socket does not invalidate successful socket.
- DSC-04: Container recreation keeps ID and manual fields; rediscovery restores;
  ambiguous identity and duplicate collector payloads are rejected.
- DSC-05: Fake-clock 5-minute/24-hour boundary tests; no automatic archive; v1
  accepted and unsupported versions rejected; decompression bomb limits tested.

Commands: `go test ./internal/discovery/... ./internal/inventory/... ./internal/snapshots/... ./internal/api/... ./internal/database/...`,
`make verify`. No remote commands, automatic computes, host actions or DNS writes.
