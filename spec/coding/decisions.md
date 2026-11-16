# Active decisions and preserved deferred decisions

Consolidated 2026-09-30. This register preserves substantive policies, not old
task ordering. [Product scope](../focused-delivery.md) controls release scope and
The progress ledger is the current implementation and verification record;
superseded scheduling plans are retained under [legacy records](legacy/README.md).

## Active

### D-08 — Three workflows and one deployment

Inventory down to containers, owned domains and split-horizon DNS. Keep one admin,
one server/SQLite/embedded UI and Linux/Podman agents. Deliver useful end-to-end
flows; dormant schemas do not require financial/history/PAT/purge features.

### D-09 — Preserve existing data and finish integration before DNS

New IDs use UUIDv7; persisted UUIDv4 IDs/references remain valid without re-keying.
Applied migrations and legacy relationships survive forward corrections. Complete
G1 in [scope](../focused-delivery.md) before DNS behavior. Parallel residual tasks
are authorized; do not restart completed work or weaken acceptance for concurrency.

### D-01 — Structural content is separate from volatile observations

Preserve the exact structural/envelope split in
[DI-03](discovery-requirements.md). Uptime/free bytes and per-report metadata do not
affect shared structural hashes; boot/runtime identity does. No raw request archive.
History UI is deferred, but current-state ingestion, replay and safe retention are not.

### D-03 — Compute archive revokes credentials

Archive compute and revoke its agent/enrollment tokens atomically with audit.
Preserve observations and identities. Restore does not revive credentials; explicit
re-enrollment advances the existing agent's generation.

### D-04 — Real versioned protocol

Initial range `[1,1]`, no fabricated v0. Future versions require explicit compatibility
adapters. Build/API/protocol/canonical versions are distinct. Validate required
response values and document schedule/local-override semantics.

### D-07 — Tested CoreDNS upgrade is permitted

Select and pin a build passing actual authoritative file/auto fallback and reload
tests. Record minimum version and tested checksum; external deployment owns host
upgrade. Sources: [file](https://coredns.io/plugins/file/) and
[auto](https://coredns.io/plugins/auto/). Upstream docs or a version string alone do
not establish absent-name/type or final-override behavior on the target build.

### Current engineering policies

- UTC whole-second storage; integer revisions; canonical lower-case IDNA names
  without terminal dot internally, absolute dotted names in zone files.
- Container identity is compute + verified numeric UID + exact name. Connection
  labels are display/configuration; duplicate owner authority is rejected.
- Rotation revokes immediately and issues one-use enrollment; lost issuance has
  explicit revoke/reissue recovery, not plaintext response replay.
- Exact capabilities: `admin` and write do not implicitly mean every/read capability.
  Human sessions get an explicit set; PAT issuance remains deferred.
- Canonical system audit actor is `0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73c0`.
  New CLI/worker events have correlation; legacy missing IDs are not invented.
- DNS zone disable prevents new changes but preserves desired state/files/repair.
  Owner/zone are immutable; rename uses delete/create. Active names have at least
  one RRset; removed views clean up before becoming not-applicable.
- Internal publication has a zone generation separate from name revisions; final
  removal writes a metadata-only zone, not deletion of the loaded file.
- Retention preserves the 90-day retry window plus five-minute accepted clock skew.
  Current minimum retention is 90 days; dry-run and cleanup must use compatible policy.
- Auth hash/session target values remain in [BE-02](backend-requirements.md).
  Current source differs in some defaults; an implemented value is not automatic
  approval. Resolve these centrally with compatibility and target evidence.

## Deferred policy provenance — not current release gates

### D-02 — Future costs

Monthly/quarterly/semiannual/annual map API `P1M/P3M/P6M/P1Y` to storage enums.
Future monthly equivalents use exact rational addition, then half-up rounding per
currency minor unit; active inclusive dates, no one-time entries or FX mixing.
Currency catalog provenance is SIX ISO 4217 List One, 2026-09-17. Preserve dormant
data, but do not implement billing/currency UI for this release.

### D-06 — Future hard purge

If later enabled, require explicit administrative + inventory-write capability,
archived target, current revision and explicit dependent cleanup. No cascading
subtree/history purge; report bounded navigable blockers. Audit survives.

### D-05 — Deferred capacity targets

Historical approval reference: Pi 4, 4 GB RAM, SSD, 64-bit Linux; 500 computes/agents,
10,000 containers, 100 zones, 100,000 individual record values. These are neither
measured achievements nor G1/G2 functional gates.

| Historical target                    | Threshold                                                                    |
| ------------------------------------ | ---------------------------------------------------------------------------- |
| Sustained ingest                     | 500 agents, 60s heartbeat, 300s snapshot, no growing backlog over 60 minutes |
| 50-item page/detail latency          | p95 ≤500 ms, p99 ≤2 s under ingest load                                      |
| Representative 20-container snapshot | p95 ≤2 s, p99 ≤5 s                                                           |
| Server memory                        | ≤1 GiB steady, ≤1.5 GiB peak                                                 |
| Internal convergence                 | p95 ≤10 s excluding CoreDNS reload interval                                  |
| Heartbeat / retention batch          | Engineering targets p95 ≤500 ms, not additional approved current gates       |
| Storage                              | Measurement and a future owner-approved budget; no fixed budget was approved |

Future benchmark evidence must record hardware/thermal/filesystem/fixture details.
Current functional pilot evidence may use virtualized Linux; physical-Pi or arbitrary
multi-distribution testing must not reappear as an unrequested prerequisite.
