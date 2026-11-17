# Portinaia

Portinaia is a self-hosted application for three workflows:

1. See servers, what runs on them, and relationships down to containers.
2. Keep a list of owned domains.
3. Manage split-horizon DNS through Cloudflare and CoreDNS.

The architecture is one Go server with an embedded React UI and SQLite, plus a
read-only Linux/Podman discovery agent. The active scope, delivery milestones,
and acceptance criteria are in [focused delivery](spec/focused-delivery.md).
Start at the [specification index](spec/README.md). The
[progress ledger](spec/coding/parallel-progress.md) is the sole current state and
verification record; historical planning records are retained in
[legacy coding records](spec/coding/legacy/README.md).

## Delivery Roadmap

1. **M1 — Inventory:** persistent authenticated server inventory, counts and hierarchy.
2. **M2 — Current discovery:** enrolled hosts, Podman containers and freshness.
3. **M3 — Domains:** owned-domain list, registrar and optional expiry information.
4. **M4 — Split-horizon DNS:** public/internal views, preview/apply and publication status.
5. **M5 — Deployment/recovery:** installation, backup/restore and end-to-end verification.

Financial tracking, historical browsing/comparisons, human API-token management,
and large-capacity certification are deferred. Existing storage/contracts are
reused; features are implemented as complete browser/API/database slices.

The running application now supports authenticated providers, locations, computes,
manual IP assignments, and owned domains. Discovery has an agent, ingestion and
container UI but remains partially verified. Split-horizon DNS runtime workflows
are not implemented yet.

The active assignment is to finish and integrate the remaining acceptance work.
The current build
has been deployed to a German server and a Debian VM; enrollment, root-owned
Podman discovery, retry/restart/revocation behavior, and backup/restore were
reported working. Formal evidence is maintained in the
[progress ledger](spec/coding/parallel-progress.md). DNS work is now the next
major implementation area, while rootless Podman and native systemd evidence
remain unverified for the current deployment.

## Implementation Status

- Inventory/domain workflows, explicit SQLite startup, sessions/CSRF, reset,
  hierarchy, manual IPs, pagination and process-restart tests exist.
- Discovery has a Linux/Podman client, enrollment/heartbeats, ingestion, current
  facts/containers/diagnostics and safe bounded retention. A
  [dated Linux/Podman pilot](spec/coding/legacy/consolidation-evidence-2026-09-27.md) records
  real rootful/rootless, recreation, recovery and service work; current-build edge
  acceptance remains separate from that historical evidence.
- Audit browsing is mounted at `/api/v1/audit-events` with authentication,
  filtering, cursor pagination, and redacted changes. Transactional idempotency
  is integrated across current inventory create/update/lifecycle mutations, and
  the shared browser transport supplies mutation keys.
- The current UI remains a functional foundation rather than final visual design;
  visual redesign is deferred.
- DNS runtime implementation is intentionally on hold; the future resume point is
  the [DNS implementation handoff](spec/coding/dns-handoff.md).
- DNS runtime is not implemented. Detailed current packet status is maintained in
  [the ledger](spec/coding/parallel-progress.md), not in historical audit backlogs.

## Prerequisites

- Go 1.26.5
- Node.js 22.13.0 or newer
- GNU Make
- Podman or Docker when building the container image

Install the pinned frontend dependencies with `npm ci`.

Install the Chromium browser used by the foundation smoke test with
`npm exec playwright install chromium`.

## GitHub Actions Images and Binaries

Every push to `main` runs [the build workflow](.github/workflows/main.yml). It
publishes a multi-architecture image to GitHub Container Registry:

```text
ghcr.io/ai-slop-code/portinaia:latest
```

The same workflow builds the Linux discovery agent for `amd64` (x86_64) and
`arm64`, and uploads both files as a 30-day workflow artifact named
`portinaia-agent-binaries-<commit>`. The workflow uses the ephemeral
`GITHUB_TOKEN`; no registry or deployment credentials are stored in the repository.

## Local Run and First Administrator

Build the embedded UI and server, then create the first administrator. Passwords
are read from the environment so they are not exposed as command-line arguments.

```sh
npm ci
make build
export PORTINAIA_DB="$PWD/portinaia.db"
./bin/portinaia db migrate -path "$PORTINAIA_DB"
export PORTINAIA_BOOTSTRAP_PASSWORD='use-a-long-unique-password'
./bin/portinaia admin bootstrap -username admin
unset PORTINAIA_BOOTSTRAP_PASSWORD
./bin/portinaia server -listen 127.0.0.1:8080 -db "$PORTINAIA_DB"
```

Migrations are explicit: run `db migrate` before first bootstrap and after each
upgrade, with a verified backup beforehand. `server`, `admin bootstrap`, and
`admin password-reset` refuse missing, old, inconsistent, or newer unsupported
schemas. `db status` checks both goose metadata and the application schema version.
Liveness and readiness are available at `/api/v1/health/live` and
`/api/v1/health/ready`; readiness verifies the supported schema and performs a
bounded SQLite read/write probe that rolls back. `PORTINAIA_LISTEN`, `PORTINAIA_DB`,
`PORTINAIA_PUBLIC_URL`, and `PORTINAIA_TRUSTED_PROXY_CIDRS` configure runtime
defaults; CLI flags override the corresponding listen/database/public URL values.
Discovery retention defaults to 90 days with a 1000-row batch every hour. The
runtime rejects ages below 90 days and retains an additional five-minute margin
for the accepted future collection-clock skew. Set
`PORTINAIA_DISCOVERY_RETENTION_DAYS`, `PORTINAIA_DISCOVERY_RETENTION_BATCH`, or
`PORTINAIA_DISCOVERY_RETENTION_INTERVAL` (or their matching server flags) to tune it.
Only peers in `PORTINAIA_TRUSTED_PROXY_CIDRS` contribute `X-Forwarded-For` client
addresses; forwarded scheme headers do not control cookie security. Set
`PORTINAIA_PUBLIC_URL` to the externally visible HTTPS URL behind TLS termination.
## Observability

Both binaries write structured JSON logs to stderr. Set `PORTINAIA_LOG_LEVEL` to
`debug`, `info` (default), `warn`, or `error`. Successful health checks log at debug;
other requests include method, route template, status, duration, and response size.
Request logs, problem responses, and audit events share `X-Request-ID`. When tracing
is enabled, logs also include `trace_id` and `span_id`.

The server exports OpenTelemetry traces, logs, and metrics using **OTLP/HTTP
protobuf**. Configure a collector endpoint to enable all three signals:

```sh
export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_SERVICE_NAME=portinaia
export OTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=production
./bin/portinaia server -listen 127.0.0.1:8080 -db "$PORTINAIA_DB"
```

With no endpoint or exporter selection, export is disabled and local JSON logging
remains available. Configuration is read at startup:

- `OTEL_TRACES_EXPORTER`, `OTEL_METRICS_EXPORTER`, `OTEL_LOGS_EXPORTER`: `otlp`
  enables that signal; `none` disables it even when an endpoint is configured.
  Explicit `otlp` without an endpoint uses `http://localhost:4318`.
- `OTEL_SDK_DISABLED=true`: disables all telemetry export while retaining local logs.
- `OTEL_EXPORTER_OTLP_ENDPOINT`: base URL; the SDK appends `/v1/traces`,
  `/v1/metrics`, or `/v1/logs`.
- `OTEL_EXPORTER_OTLP_{TRACES,METRICS,LOGS}_ENDPOINT`: overrides one signal with
  its full endpoint URL, including the signal path.
- `OTEL_EXPORTER_OTLP_HEADERS`: collector authentication headers; per-signal
  overrides are supported by the SDK.
- `OTEL_EXPORTER_OTLP_CERTIFICATE`: custom collector CA certificate; standard
  SDK per-signal TLS options also apply.
- `OTEL_TRACES_SAMPLER`, `OTEL_TRACES_SAMPLER_ARG`: SDK sampling policy, e.g.
  `parentbased_traceidratio` and `0.1`. Default is parent-based always-on.
- `OTEL_METRIC_EXPORT_INTERVAL`: metric export interval in milliseconds
  (SDK default: 60000).

Traces cover HTTP requests and discovery retention runs. W3C `traceparent` and
`tracestate` propagate into requests; a context-aware agent caller can forward
them as well. HTTP names and metric labels use route templates, never raw resource
IDs or query strings. Request bodies, cookies, authorization headers, SQL text,
and panic values are excluded from instrumentation.

Exported metrics include:

- `http.server.request.duration`, `http.server.active_requests`,
  `http.server.response.body.size`, and `portinaia.http.server.requests`.
- `portinaia.discovery.retention.{runs,duration,deleted}` with bounded result/entity labels.
- `portinaia.db.connections`, `portinaia.db.pool.waits`, and
  `portinaia.db.pool.wait.duration`.
- `portinaia.process.goroutines`, `portinaia.process.heap.alloc`, and
  `portinaia.process.gc.cycles`.

Metrics are pushed to the collector. The
[diagnostic collector configuration](deploy/otel-collector.yaml) receives all
three signals and prints them through its debug exporter. Connect the collector
to your monitoring backend for storage and dashboards. Shutdown drains HTTP work,
joins background workers, then flushes telemetry within a ten-second budget.

See [runtime architecture](spec/runtime-architecture.md) for extension points and
instrumentation conventions.

## Linux Agent Client (M2 In Progress)

The agent can exchange a one-time token for a private local credential, send
heartbeats, collect Linux host facts, and report from explicitly configured
Podman 4.x–6.x Unix sockets (including actual 6.1.1 Podman machine verification;
see [pilot evidence](spec/coding/legacy/consolidation-evidence-2026-09-27.md)). Do not
put tokens in command-line arguments. Place
the token in a 0600 file, run enrollment, and remove the token file after success:

```sh
sudo install -d -m 0700 /var/lib/portinaia-agent
sudo install -m 0600 /secure/enrollment-token /run/portinaia-enrollment-token
sudo ./dist/portinaia-agent-linux-amd64 enroll \
  -server https://portinaia.example \
  -token-file /run/portinaia-enrollment-token
sudo rm /run/portinaia-enrollment-token
sudo ./dist/portinaia-agent-linux-amd64 run \
  -podman-socket rootful=0:/run/podman/podman.sock
```

The client requires HTTPS except for loopback development servers. Credential
state defaults to `/var/lib/portinaia-agent/credential.json`, written atomically
with owner-only permissions. `heartbeat` sends one heartbeat; `snapshot` collects
and uploads once; `run` sends heartbeats every 60 seconds and snapshots every five
minutes by default. Both schedules can be adjusted with command flags. The latest
pending snapshot is stored as one private report and retains its report ID until
accepted or superseded by a newer complete report. Independent collection during
upload backoff and concurrent acknowledgement safety remain tracked acceptance
items. Retries are supported for up to 90 days from collection start;
older, unrecognized reports are rejected so retention cannot reapply stale state.
Report history is retained for 90 days by server receipt time. `portinaia db
retention` prints a dry-run report/content count; the server removes expired rows in
bounded hourly batches and never runs VACUUM automatically. Podman sockets must be explicitly listed as
`NAME=UID:/absolute/socket`; the agent verifies socket type and owner, supports
one configured connection per UID, and does not search for arbitrary sockets.
Rootless endpoints use their numeric user's UID and socket path. If no Podman
socket is configured, no Podman inventory or absence is asserted. The example
root-owned unit is [`deploy/portinaia-agent.service`](deploy/portinaia-agent.service).
For rootless Podman, explicitly configure its numeric owner and socket in that
agent's repeated `-podman-socket NAME=UID:/absolute/socket` arguments, retaining
root-owned private agent state and one connection per UID. Enable the relevant
owner's Podman socket before collection; no separate user-service model is assumed.

For a native systemd installation, verify the release checksum, install the binary
as root, provide a one-use token in a mode-0600 file, enroll once, remove the token,
then enable the service:

```sh
sudo install -o root -g root -m 0755 ./portinaia-agent-linux-amd64 /usr/local/bin/portinaia-agent
sudo install -d -o root -g root -m 0700 /var/lib/portinaia-agent
sudo install -o root -g root -m 0600 /secure/enrollment-token /run/portinaia-enrollment-token
sudo /usr/local/bin/portinaia-agent enroll -server https://portinaia.example -token-file /run/portinaia-enrollment-token
sudo rm /run/portinaia-enrollment-token
sudo install -o root -g root -m 0644 deploy/portinaia-agent.service /etc/systemd/system/portinaia-agent.service
sudo systemctl daemon-reload
sudo systemctl enable --now portinaia-agent.service
sudo systemctl status portinaia-agent.service
sudo journalctl -u portinaia-agent.service
```

The service requires an active rootful Podman API socket at the path shown in the
unit. Enable `podman.socket` and confirm its owner before starting it. Never enable
the unit with a missing socket if you expect container collection; the agent will
report that collector as failed rather than claiming an empty host.

To recover access on the host with database-file access, stop the server and run:

```sh
export PORTINAIA_ADMIN_PASSWORD='another-long-unique-password'
./bin/portinaia admin password-reset -username admin -db "$PORTINAIA_DB"
unset PORTINAIA_ADMIN_PASSWORD
```

Password reset invalidates all existing browser sessions. It does not print or
store the new plaintext password.

## Backup and Restore (SQLite)

Create a consistent online SQLite snapshot and verify it before relying on it:

```sh
./bin/portinaia db backup -path "$PORTINAIA_DB" -output /secure/backups/portinaia-$(date +%F).db
./bin/portinaia db verify -path /secure/backups/portinaia-$(date +%F).db
```

The destination must not already exist; snapshots are created with owner-only
permissions. Follow the [recovery procedure](deploy/recovery.md): stop the server,
preserve its database and WAL/SHM sidecars, and restore the verified snapshot to a
clean destination with the expected owner/mode and matching configuration/secrets.
Never combine the restored DB with stale sidecars. Run `db verify` and `db status`
before starting. Restoring SQLite does not roll back external DNS-provider
changes. Keep the database and backup readable only by the service administrator;
protect bootstrap/reset environment values as secrets.

Run `make verify` for formatting, static analysis, Go and frontend tests, generated
contract checks, and builds. `make test-e2e` exercises the embedded UI in Chromium.

Use `./bin/portinaia version` or a built `portinaia-agent version` binary to
inspect build metadata.

## Generated Files

`make generate` is the only supported regeneration entry point. Generated
source belongs in a `generated/` directory and must carry its generator's
standard do-not-edit header. Generated files are never edited by hand.

The API source of truth is `api/openapi.yaml`. `make generate` runs pinned
versions of oapi-codegen and sqlc, generates the TypeScript API types, and
rebuilds the frontend assets. Generated outputs are committed in
`internal/api/generated/`, `internal/database/generated/`, `web/src/api/`, and
`web/dist/` because Go compiles or embeds them. `make generate-check` fails when
these outputs do not match their contracts.

The initial SQLite schema is `internal/database/migrations/00001_initial.sql`.
Migrations are immutable, embedded into the server, and exercised against a
temporary modernc SQLite database by `go test ./internal/database`.
