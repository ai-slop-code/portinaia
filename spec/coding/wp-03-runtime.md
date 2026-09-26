# WP-03 — Runtime foundation

Dependencies: reconciled WP-02. Sources: implementation plan §§5–7.1, 17,
18.1, 21/WP-03; product §§14.4, 15, 17. Read [shared rules](coding-rules.md).

Primary paths: `internal/config/`, DB connection layer in `internal/database/`,
`internal/server/`, `internal/api/middleware/`, `internal/metrics/`,
`internal/jobs/`, `cmd/portinaia/`, `deploy/examples/portinaia.yaml`.
Baseline: fixed-address `http.ListenAndServe` and two unconditional health handlers.

## WP-03.1 — Configuration and process lifecycle

1. Add versioned YAML and `--config` handling to server/database/admin commands;
   `version` needs none. Reject unknown fields, invalid URLs/CIDRs/durations,
   inconsistent bounds, overlapping listeners, and missing required paths.
2. Cover every implementation-plan §5.1 area. Defaults already approved: sessions
   30 days, enrollment 1 hour within API bounds, heartbeat 60 seconds, snapshot
   300 seconds, offline 300 seconds, child missing threshold 24 hours, retention
   90 days, expiry warning 30 days. Automatic archive is never enabled.
3. Initial tunables: busy timeout 5 seconds, one writer, up to four read
   connections; general HTTP body 1 MiB, credential bodies 16 KiB, snapshot
   decompressed body 32 MiB; worker lease 60 seconds, provider request 15 seconds,
   retry base 1 second/cap 5 minutes with full jitter; retention batch 1000 hourly.
   These are documented engineering defaults, configurable and capacity-tested.
   Validate lease/timeout relation and enforce both compressed/decompressed limits.
4. Resolve config-relative paths consistently. Secrets are file references;
   diagnostics show field names, not secret values/content or sensitive paths.
   Document any environment overrides; do not add undocumented secret env inputs.
5. Startup order: validate config -> open DB -> check supported schema (no automatic
   migrations) -> validate mandatory files/paths -> bind listeners -> workers -> ready.
   Clean up opened resources if any later step fails.
6. SIGTERM/SIGINT: mark not ready, stop intake, cancel worker scheduling, drain
   requests/work within 30-second configurable deadline, release own leases where
   safe, close listeners/DB. Do not falsely complete interrupted external work.

## WP-03.2 — SQLite and administrative foundation

1. Use modernc driver with per-connection FK, busy timeout, WAL, synchronous FULL
   for durable single-node use. A single write connection serializes writes;
   separate bounded reads must not evade pragma configuration.
2. No network/filesystem I/O during a domain write transaction. Bound writer wait
   with context; distinguish cancellation, busy exhaustion, and constraints.
3. `db migrate` runs embedded goose explicitly; `db status` reports current and
   expected schema and exits nonzero for unsupported/migration-needed state.
   Reconcile goose metadata and `user_version`; do not trust one inconsistent value.
4. Keep migration operations serialized; goose's current global setup must not
   race between parallel DB tests or concurrently invoked in-process migrations.

## WP-03.3 — HTTP, logging, health

Compose chi with bounded request IDs, structured slog, panic recovery, request
timeouts, body limits, trusted-proxy resolution, generated request validation and
shared RFC 7807 writer. Only explicitly trusted proxy CIDRs can supply client IP
or forwarded scheme. Public URL, rather than arbitrary request headers, determines
cookie security and links. Log route templates instead of credentials/raw URLs.

Liveness: 200 generated `HealthStatus` with status live/time while responsive.
Readiness: 200 ready/time/checks only when schema is supported, DB read/write probe
succeeds, required directories and enabled zone publication directory are usable;
otherwise 503 problem details with sanitized dependency information. Use a bounded
real write probe that rolls back, not merely `Ping`. Cloudflare outage is not a
readiness failure. Update existing health and smoke tests to the contract.

## WP-03.4 — Metrics and workers

Optional separate Prometheus listener; main listener must not expose `/metrics`.
Register bounded labels and one shared instrumentation API. Foundation supplies
HTTP request/latency/error and DB latency/busy metrics. Features supply auth,
resource counts/freshness, report/retention, DNS/proxy/publication/drift/expiry
groups. Do not put IDs, names, raw errors or arbitrary client versions in labels;
bucket versions into supported compatibility/release categories.

Worker supervisor owns context, bounded concurrency, panic/error reporting, lease
renewal/cancellation and graceful shutdown. Projection rows remain the DNS queue;
do not add an external broker. Inject time/jitter sources for deterministic tests.

## Acceptance / tests

- RUN-01: Invalid config and unsupported schema fail before listeners open.
- RUN-02: Every acquired DB connection has required pragmas; concurrent writer
  tests produce bounded wait/failure without lost committed writes.
- RUN-03: Readiness fails on unwritable DB/zone directory, not Cloudflare failure;
  liveness stays independent. Unknown API routes and panics return safe problems.
- RUN-04: Trusted/untrusted forwarded headers behave differently; request IDs,
  logs and metrics contain no authorization/header/body secrets.
- RUN-05: Shutdown during leased work permits recovery and closes resources.

Commands: `go test ./internal/config/... ./internal/database/... ./internal/server/... ./internal/api/middleware/... ./internal/jobs/... ./internal/metrics/...`,
`make verify`, `make test-e2e`. No inventory/auth business logic in middleware.
