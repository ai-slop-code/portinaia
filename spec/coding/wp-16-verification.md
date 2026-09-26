# WP-16 — Release verification and acceptance evidence

Dependencies: WP-01–WP-15. Sources: product §§6, 18–19; DNS §§27–28;
implementation plan §§19, 24–25. Read [shared rules](coding-rules.md).

Primary paths: `integration/fixtures/`, `integration/tests/`, `web/tests/`,
release-evidence documentation under `spec/`; coordinated Makefile test targets.
Baseline: Go contract/selected DB/foundation-handler tests and one frontend title
test pass; no complete release scenario, failure isolation or measured capacity.

## WP-16.1 — Acceptance evidence matrix

For every acceptance bullet in product §19 and DNS §28, record: exact source
section/bullet text, owning WP acceptance IDs, test file/test name or manual drill,
command, result, artifact/log reference and environment. Add release-gate A–E rows.
“Implemented” without a passing test/manual result is not evidence. Manual checks
need justification and reproducible steps. Do not alter acceptance to fit failing code.

Required automated suite owners (the row order also supplies release-evidence
categories; each individual source acceptance bullet still needs its own row):

| Suite        | Required evidence                                                                                                   |
| ------------ | ------------------------------------------------------------------------------------------------------------------- |
| Contract     | OpenAPI validation/examples/unions; generated output drift; running handler status/headers/body match, scope matrix |
| DB           | Empty/populated upgrade, FKs/checks/indexes, mutation/audit rollback, revision races, pagination, busy timeout      |
| Auth         | Password/session/CSRF/PAT/enrollment isolation, rate limiting, once-only credentials and redaction                  |
| Inventory    | All resource workflows, hierarchy cycles, date/money semantics, manual ownership, lifecycle/purge dependencies      |
| Discovery    | v1 compatibility, enrollment races, all states/collectors, duplicate reports, recreation, freshness/revocation      |
| History      | Structural canonical reuse plus retained volatile values, complete diff, bounded retention/GC race                  |
| Cloudflare   | Exact ownership, selective import, stale preview, uncertain writes, retry/rate limits, drift and ambiguous lookup   |
| Internal DNS | Deterministic valid zones, serial order, every publication failure stage, real query fallthrough/reload             |
| Frontend     | Protected routing, filters/URL state, typed forms, conflicts, secrets, history, DNS acceptance-vs-sync, mobile      |
| Operations   | Online backup/check, restore/repair, permissions, artifact checksums and native arm64 deployment                    |

## WP-16.2 — Deterministic browser/API release scenario

Use a fresh temporary DB, fixture administrator created via CLI, fake Cloudflare
HTTP service, fixture agent protocol and real pinned CoreDNS/public test upstream.
No live provider credentials or external Internet DNS dependency.

1. Log in, exercise CSRF, create provider (hosting/registrar), site/region, physical
   compute and VM parent, domain, manual IP, tags and recurring/one-time costs.
2. Confirm inventory tree/table/filter deep links and monthly totals per currency.
3. Issue compute enrollment token, enroll fixture agent once, prove second exchange
   rejected. Submit all-state Podman snapshot from root and rootless owners.
4. Recreate same logical container with new runtime ID; preserve manual notes/tags/
   IPs. Submit failed collector and empty success separately, advance fake clock
   for offline/stale boundaries, archive/rediscover container and verify audit.
5. Submit structurally unchanged report with changed uptime/free bytes; history
   keeps two timestamps and measurements with shared structural content; diff/export.
6. Discover at least two zones, select explicitly, preview/import one candidate;
   prove unknown/unselected provider objects untouched.
7. Draft/preview/apply multi-value split-horizon A/AAAA, CNAME and wildcard names;
   reject private public IP, stale preview, stale ETag and unmanaged conflict.
8. Verify fake Cloudflare values and actual CoreDNS internal/public-fallthrough
   queries. UI shows pending then independently synchronized providers.
9. Inject Cloudflare failure while internal succeeds, then reverse failure direction.
   Restart during leased work, restore success, prove newest desired convergence.
10. Modify/delete owned fake provider records and add conflicting unmanaged record;
    verify visible drift, repair and safe blocking. Delete DNS via tombstone and
    verify cleanup of both providers including final internal override removal.
11. Archive compute -> credential rejected -> restore -> explicit re-enrollment.
    Purge blocked by archived dependents/history/links; no implicit cascade.
12. Create/read with scoped PAT, deny out-of-scope call, revoke it; inspect redacted
    audit and once-only secret UI. Back up under writes, check/restore and repair.

Run desktop and 390px smoke workflows; keyboard-test critical dialogs and forms.

## WP-16.3 — Failure injection and secrets

Inject SQLite busy/rollback, audit failure, lost HTTP success response, stale report,
lease expiry, stale in-flight provider response, rate limiting, malformed provider
payload, filesystem write/flush/rename failure and post-rename DB failure.
Assert preservation of manual/current/history/desired state and independent providers.
Use unique secret canaries and inspect DB (including audit/idempotency), logs,
problem responses, generated zones and browser storage. Issuance responses and
root-only credential files are the deliberate plaintext exceptions.

Review auth route mounting and scope enforcement as executable behavior, not merely
OpenAPI declarations. Check snapshot decompression limits and no shell/provider
mutation path in agent. Repeat only tests affected by fixes after a failure.

## WP-16.4 — Capacity fixture and acceptance budgets

Reference approved by owner: Pi 4 / 4 GB / SSD / 64-bit Linux (D-05).
Load, page/detail latency, snapshot latency, memory and publication thresholds below
were explicitly approved on 2026-09-26. They are not measured performance claims.
Heartbeat and retention timing are engineering targets pending benchmark review;
F-01 requires a measured storage proposal and owner-approved storage budget.

| Metric                                 | Threshold and approval status                                                                                                                              |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Sustained workload                     | 500 agents; 60s heartbeats; 300s snapshots; no growing ingestion/worker backlog over 60 minutes                                                            |
| Inventory/DNS page/detail HTTP latency | p95 <= 500 ms and p99 <= 2 seconds with 50-item pages under ingestion load                                                                                 |
| Snapshot acceptance latency            | p95 <= 2 seconds, p99 <= 5 seconds for representative 20-container reports                                                                                 |
| Heartbeat HTTP latency                 | Engineering target: p95 <= 500 ms; no freshness flapping caused by writer contention                                                                       |
| Server resident memory                 | <= 1 GiB steady state, <= 1.5 GiB peak under specified representative workload                                                                             |
| Internal publish convergence           | p95 <= 10 seconds after apply for ordinary zone update, excluding configured CoreDNS reload interval                                                       |
| SQLite history storage                 | Report structural content, report envelopes/volatile values, indexes and indefinite audit separately; owner-approved storage budget after measured fixture |
| Retention responsiveness               | Engineering target: batch p95 <= 500 ms; approved foreground p99 remains <= 2 seconds while cleanup runs                                                   |

Generate deterministic 500 computes/agents, 10,000 containers, 100 zones and 100,000
individual DNS values (record count means provider records, not aggregate names).
Distribute typical and skewed workloads, including a large single compute/zone.
At 500 * 288 reports/day * 90 days there are 12,960,000 report envelopes. Generate
that history directly for retention/storage tests and separately measure live
ingestion; do not simulate 90 days by sleeping. Include changed uptime/free bytes
every report, stable structural runs, container churn, failures, audit volume and
mostly unchanged reports. Document distribution and random seed.

Measure DB live/allocated bytes, compression/reuse ratios, query plans, writer
wait/busy/latency, worker claim/index behavior, CPU, RSS and thermal throttling.
SQLite indefinite audit growth needs per-day/per-year projection separately from
bounded history. Cloudflare tests use fake latency/429 envelopes; do not imply a
real account's quotas permit syncing all 100,000 values instantly.

## WP-16.5 — Clean release gate

From clean source and pinned dependencies: `make verify`, `make test-e2e`,
`go test ./integration/tests/...`, `make image`, `make release`.
Add deterministic `make test-capacity` with documented fixture sizes/environment;
it is a future target, absent from the audited Makefile. Run race tests on the
supported Go race-test host where appropriate for worker/state concurrency.

Check specification links, source/generated consistency, deployment instructions,
supported schema upgrade, v1-only initial protocol wording, native arm64 startup,
both agent architectures and checksums. Record all skipped/unavailable checks.
No known critical/high-severity defect or failed required acceptance remains.
Release is blocked by unresolved F-01/F-02 where relevant, missing target evidence,
or placeholder behavior in any required module.
