# Portinaia Implementation Plan

Status: Approved for implementation

Package-level coding instructions, verified implementation status, shared patterns,
and acceptance checks are indexed in [Coding-agent specifications](coding/README.md).
User-approved clarifications from 2026-09-26 are recorded in
[the decision register](coding/decisions.md) and incorporated below.

## 1. Purpose

This plan translates the approved product and DNS requirements into
dependency-ordered work packages suitable for implementation by AI agents or
human contributors.

Normative product behavior is defined in:

- [Portinaia Product Requirements](high-level-plan.md)
- [Portinaia DNS Management Specification](dns-spec.md)

If this plan conflicts with a product behavior in those documents, the
product-specific document wins. The conflict must be resolved in the
specifications before code proceeds.

## 2. Delivery Strategy

Portinaia is delivered as vertical increments, but the first supported release
contains the complete approved MVP: inventory, discovery, historical
snapshots, Cloudflare DNS, internal CoreDNS publication, API, and frontend.

Implementation principles:

- Establish contracts and migrations before parallel feature work.
- Prefer explicit SQL and small Go packages over framework abstractions.
- Keep one deployable server process.
- Keep provider calls outside HTTP and database transactions.
- Generate API boundary code; hand-write domain behavior.
- Treat all retries, duplicate reports, and worker restarts as normal.
- Make current state easy to query and historical state bounded.
- Add tests in the same work package as behavior.
- Do not add future extension code without a current requirement.
- Do not introduce PostgreSQL compatibility layers in the first version.

## 3. Technology Baseline

### 3.1 Server

- Go version: current stable release pinned in `go.mod` and developer tooling.
- HTTP: standard `net/http` with chi router.
- API contract: OpenAPI 3.0.3.
- OpenAPI Go generation: `oapi-codegen`.
- Database: SQLite in WAL mode.
- Driver: pure-Go modernc SQLite driver.
- Queries: sqlc-generated Go code.
- Migrations: ordered embedded SQL migrations using `pressly/goose/v3`.
- IDs: UUIDv7 represented as lower-case canonical strings at API boundaries.
- Time: UTC in storage, RFC 3339 in API responses.
- Logs: structured `log/slog` JSON in production.
- Metrics: Prometheus text format on a separate listener.
- DNS parsing and zone generation: `miekg/dns`.

Exact dependency versions must be pinned when scaffolding begins. Dependency
selection must avoid CGO and support Linux amd64 and arm64 cross-compilation.

### 3.2 Frontend

- React.
- TypeScript with strict mode.
- Vite.
- Material UI.
- React Router for route ownership and deep links.
- TanStack Query for server state, invalidation, and polling.
- React Hook Form for complex forms.
- Generated TypeScript API types using `openapi-typescript` and a typed client
  using `openapi-fetch`.
- Vitest and React Testing Library.
- Playwright for the browser smoke suite.

The frontend must not create independent API data models where generated
types are sufficient.

### 3.3 Discovery Client

- Go binary separate from the server binary.
- Linux amd64 and arm64.
- Standard systemd service deployment, documented but not provisioned here.
- Host facts collected through Linux files and stable system APIs.
- Podman collection through explicitly configured REST sockets.
- No shell command channel and no inbound listener.

### 3.4 Build and Release

- One root `Makefile` exposes stable local commands.
- Server frontend assets are built before Go embedding.
- Server image supports Linux amd64 and arm64.
- Agent releases contain both architectures and a checksum manifest.
- No hosted CI configuration is required initially.
- Build logic should be reusable by a future CI system without rewriting
  commands.

## 4. Target Repository Layout

```text
/
  api/
    openapi.yaml
  cmd/
    portinaia/
      main.go
    portinaia-agent/
      main.go
  internal/
    api/
      generated/
      handlers/
      middleware/
    auth/
    audit/
    config/
    database/
      generated/
      migrations/
      queries/
    inventory/
    discovery/
    snapshots/
    dns/
      cloudflare/
      internaldns/
    jobs/
    metrics/
  agent/
    config/
    collector/
      host/
      podman/
    protocol/
    reporter/
    state/
  web/
    src/
      api/
      app/
      components/
      features/
        auth/
        dashboard/
        inventory/
        providers/
        locations/
        domains/
        dns/
        agents/
        audit/
        settings/
      routes/
      theme/
    tests/
  integration/
    fixtures/
    tests/
  deploy/
    examples/
      portinaia.yaml
      portinaia-agent.yaml
      coredns/
  spec/
  Containerfile
  Makefile
  go.mod
  package.json
```

Package boundaries may be adjusted when concrete code reveals a simpler
layout. The server and agent must not share packages that import server-only
database, HTTP handler, or frontend concerns.

## 5. Configuration

### 5.1 Server Configuration

Use one versioned YAML configuration file supplied with a command-line flag.
Environment variables may override non-secret deployment values when clearly
documented. Secrets are referenced by file path, not embedded in YAML.

Configuration areas:

- Main HTTP listen address.
- External/public URL used for cookies and links.
- Trusted reverse-proxy addresses and forwarding-header behavior.
- SQLite database path and database tuning limits.
- Session lifetime, defaulting to 30 days.
- Enrollment-token lifetime.
- Personal-token defaults.
- JSON or development log format and log level.
- Metrics enablement and separate listen address.
- Cloudflare token file.
- Cloudflare request timeout and rate-limit behavior.
- Generated zone directory.
- Internal zone SOA nameserver and responsible mailbox values.
- Worker counts, lease duration, and retry bounds.
- Heartbeat and stale thresholds.
- Snapshot retention, defaulting to 90 days.
- Domain expiry warning, defaulting to 30 days.

Configuration validation occurs before the server opens listeners. Startup
errors identify the invalid field without printing secret values.

### 5.2 Agent Configuration

Agent YAML configuration includes:

- Server base URL.
- Optional custom CA file.
- Credential file path.
- One-time enrollment-token file path used only before enrollment.
- Local state file path.
- Heartbeat interval, defaulting to 60 seconds.
- Snapshot interval, defaulting to five minutes.
- Request timeout and retry bounds.
- Protocol version.
- Podman connection names, socket paths, and expected owner identity.

Credential and state files must be atomically written with root-only
permissions. Configuration parsing must reject unknown fields to catch
deployment mistakes.

## 6. Server Process Model

The `portinaia` binary supports subcommands:

- `server`: run API, frontend, and workers.
- `admin create`: create the single administrator.
- `admin reset-password`: reset the administrator password and optionally
  revoke sessions.
- `db migrate`: apply embedded migrations.
- `db status`: report schema compatibility.
- `db check`: run integrity and foreign-key checks.
- `db backup`: create a consistent online SQLite backup.
- `dns reconcile`: enqueue repair of current desired DNS state.
- `version`: print build and protocol versions.

The `server` command:

1. Loads and validates configuration.
2. Opens SQLite and applies required connection pragmas.
3. Verifies schema compatibility without silently applying unexpected
   migrations.
4. Validates required local paths and secret readability.
5. Starts HTTP and optional metrics listeners.
6. Starts bounded background workers.
7. Marks readiness only after local mandatory dependencies are available.
8. Handles graceful shutdown by stopping intake, completing or releasing
   leased work, checkpointing where appropriate, and closing listeners.

## 7. Database Design

### 7.1 Database Rules

- Enable foreign keys for every connection.
- Enable WAL and a documented synchronous setting appropriate for durable
  single-node use.
- Configure busy timeout.
- Bound open connections and concurrent writers.
- Keep write transactions short.
- Use explicit `NOT NULL`, `CHECK`, unique, and foreign-key constraints.
- Store monetary amounts as integer minor units plus ISO currency code and use
  the ISO currency exponent when converting to and from display values; never
  use floating point.
- Store timestamps in one UTC representation.
- Store booleans as constrained integers if required by SQLite.
- Add indexes from concrete list/filter/worker query requirements.
- Archive through `archived_at`; do not overload synchronization state.
- Use monotonic integer `revision` columns for mutable aggregates.

### 7.2 Authentication and Audit Tables

`administrators`:

- ID, username, password hash, password algorithm/parameters, created time,
  updated time, and disabled time.
- Enforce one enabled administrator in application behavior.

`sessions`:

- ID, administrator ID, credential hash, CSRF material, created time, expiry,
  last-seen time, and revoked time.

`personal_access_tokens`:

- ID, administrator ID, name, token hash, scopes, created time, optional
  expiry, last-used time, and revoked time.

`audit_events`:

- Monotonic ordering key, UUID, time, actor type, actor ID, action, resource
  type, resource ID, request ID, redacted before/after documents, and result.
- Audit rows are append-only through application permissions and conventions.

`idempotency_keys`:

- Actor, scope, key hash, request fingerprint, stored response metadata,
  created time, and expiry.
- Unique actor/scope/key constraint.

### 7.3 Inventory Tables

`providers`:

- Core provider fields, revision, timestamps, and archive time.

`provider_roles`:

- Provider ID and constrained role.

`locations`:

- Type, name, provider ID, parent ID, description, revision, timestamps, and
  archive time.

`computes`:

- Kind, display name, ownership, provider ID, provider resource identifier,
  location ID, parent compute ID, description, notes, revision, timestamps,
  and archive time.

`compute_observations`:

- Compute ID, latest machine identity, hostname, OS, kernel, architecture,
  CPU, memory, boot/uptime data, current agent/report references, and observed
  timestamps.
- This table contains only latest observed state.

`containers`:

- Compute ID, Podman connection/owner, name, current runtime ID, image, state,
  health, created/started times, first/last observed, stale time, manual
  description/notes, revision, and archive time.
- Unique active logical identity on compute, Podman owner, and name.

Current container child tables:

- Use foreign-key-backed `container_ports`, `container_networks`,
  `container_addresses`, `container_mounts`, and `container_labels` tables.
- Keep restart policy as constrained columns on `containers`.
- Replace a container's current child facts transactionally after a successful
  authoritative observation.

`ip_assignments`:

- Address, prefix, one compute or container owner, interface/network, scope,
  role, source, primary flag, first/last observed, stale time, description,
  revision, and archive time.
- Constraint ensures exactly one owner.

`domains`:

- Normalized name, registrar provider, registration date, expiry date,
  auto-renew state, nameserver data, description, notes, revision, timestamps,
  and archive time.

`costs`:

- Exactly one compute or domain owner, type, amount, currency, billing
  interval, effective dates, description, provider reference, revision,
  timestamps, and archive time.
- Recurring intervals are monthly, quarterly, semiannual, and annual (API P1M,
  P3M, P6M, P1Y). One-time costs use charge date instead of recurring effective
  dates. Monthly-equivalent dashboard totals use exact rational arithmetic and
  half-up rounding once per currency after summing active entries.

`tags` and resource-specific tag joins:

- Unique normalized tag name and display name.
- Use foreign-key-backed joins for supported resource types rather than an
  unconstrained polymorphic table.

### 7.4 Agent and Discovery Tables

`agents`:

- ID, unique compute ID, credential hash, credential generation, protocol
  version, client version, enrolled time, last heartbeat, last successful
  snapshot, last error summary, revoked time, and revision.

`agent_enrollment_tokens`:

- ID, compute ID, token hash, created time, expiry, consumed time, revoked
  time, and creator.

`discovery_reports`:

- ID supplied or correlated from the client, agent ID, protocol version,
  received time, client collection time, snapshot-content ID, result, and
  collection error summary.
- Unique agent/report ID ensures idempotency.
- Persist report-specific client metadata, collection start/end, acceptance result,
  content-reuse indication, request fingerprint and volatile observations separately
  from the shared structural content. Retained reports preserve uptime and filesystem
  available-byte values even when structural content is reused.

`snapshot_contents`:

- ID, canonical format version, cryptographic content hash, compressed
  normalized document, uncompressed size, created time, and reference count or
  safely derivable references.
- Unique format version/content hash enables deduplication.

`discovery_collector_results`:

- Report ID, collector name or Podman connection, success state, and sanitized
  error.
- This distinguishes an authoritative empty result from a failed collector.

Retention deletes expired `discovery_reports` in bounded batches, then removes
unreferenced `snapshot_contents`. Current inventory state is unaffected.

### 7.5 DNS Tables

`dns_zones`:

- Portinaia ID, Cloudflare zone ID/name/status, selection state, linked domain
  ID, discovery/refresh times, revision, and status metadata.

`dns_names`:

- Zone ID, normalized FQDN, wildcard state, description, desired revision,
  lifecycle state, timestamps, archive time, and deletion tombstone data.
- Unique active zone/name constraint.

`dns_record_sets`:

- DNS name ID, view, type, TTL, proxy state, and deterministic set ordering.
- Unique name/view/type constraint.

`dns_record_values`:

- Record set ID, normalized value, deterministic order, and value checksum.
- Unique set/value constraint.

`dns_inventory_links`:

- DNS name ID and exactly one compute, container, or IP assignment target.

`dns_drafts`:

- ID, operation, optional target name ID, base revision, proposed canonical
  aggregate, content hash, validation state, latest preview, actor, timestamps,
  and applied/discarded time.

`dns_projections`:

- DNS name ID, provider, desired revision, applied revision, state, attempt
  count, next retry, last attempt/success, sanitized error, observed checksum,
  and lease fields.
- Unique name/provider constraint.

`cloudflare_record_mappings`:

- Projection/name/value association, Cloudflare record ID, expected provider
  fields/checksum, and desired revision.

`dns_publications`:

- Zone ID, desired revision, serial, checksum, file name, created time, result,
  and sanitized error.

`dns_drift_events`:

- Zone/name, observed time, expected checksum, observed state, repair status,
  and audit correlation.

Projection rows act as the durable work queue. Lease owner and expiry fields
permit crash recovery without a second queueing system.

## 8. OpenAPI Contract

### 8.1 Conventions

- Base path `/api/v1`.
- JSON request and response bodies.
- UUIDv7 canonical string IDs.
- RFC 3339 UTC timestamps.
- Cursor pagination using opaque `page[after]` and bounded `page[size]` query
  parameters on every paginated collection.
- Stable sort keys and deterministic default ordering.
- RFC 7807 errors using `application/problem+json`.
- Problem extensions include stable `code`, field violations, and request ID.
- Mutable resource responses expose revision and ETag.
- Mutations use `If-Match` where updating an existing aggregate.
- Retryable creates and applies accept `Idempotency-Key`.
- `Cache-Control: no-store` on authentication and secret-returning responses.

### 8.2 Authentication Endpoints

- `POST /auth/login`
- `POST /auth/logout`
- `GET /auth/session`
- `GET /personal-access-tokens`
- `POST /personal-access-tokens`
- `DELETE /personal-access-tokens/{token_id}`

Plaintext personal tokens appear only in the successful create response.

### 8.3 Inventory Endpoints

CRUD, archive, restore, list, search, and detail operations as appropriate for:

- `/providers`
- `/locations`
- `/computes`
- `/containers`
- `/ip-assignments`
- `/domains`
- `/costs`
- `/tags`

Observed container and compute fields are read-only in human API schemas.
Updates accept only manually owned fields.

Additional aggregate reads:

- `GET /inventory/tree`
- `GET /computes/{compute_id}/summary`
- `GET /computes/{compute_id}/containers`
- `GET /computes/{compute_id}/snapshots`
- `GET /domains/expiring`
- `GET /dashboard/summary`

### 8.4 Agent Administration Endpoints

- `GET /agents`
- `GET /agents/{agent_id}`
- `POST /computes/{compute_id}/enrollment-tokens`
- `DELETE /enrollment-tokens/{token_id}`
- `POST /agents/{agent_id}/revoke`
- `POST /agents/{agent_id}/rotate`

### 8.5 Agent Protocol Endpoints

Use a separately visible protocol prefix while retaining the product API
version, for example:

- `POST /api/v1/agent/v1/enroll`
- `POST /api/v1/agent/v1/heartbeat`
- `PUT /api/v1/agent/v1/snapshots/{report_id}`

Requirements:

- Enrollment token authentication is valid only for enroll.
- Client bearer authentication is valid only for its bound agent endpoints.
- Snapshot PUT is idempotent by agent/report ID.
- Protocol responses include server time, accepted protocol range, and current
  scheduling configuration where useful, but no remote action instructions.
- The first release supports v1 only, with accepted range [1,1]. Starting with v2,
  support the current and previous real versions using explicit boundary adapters.

### 8.6 History Endpoints

- `GET /agents/{agent_id}/discovery-reports`
- `GET /discovery-reports/{report_id}`
- `GET /discovery-reports/{report_id}/snapshot`
- `GET /snapshot-comparisons?from={report_id}&to={report_id}`

Historical endpoints are paginated and never return unbounded container lists
without explicit limits or streaming behavior.

### 8.7 DNS Endpoints

Implement the paths and behavior from `dns-spec.md`, including:

- Zone discovery and selection.
- Import candidates, preview, and apply.
- DNS-name aggregate listing and detail.
- Draft create, update, preview, apply, and discard.
- Projection status and manual reconciliation.

### 8.8 Audit and Operations Endpoints

- `GET /audit-events`
- `GET /health/live`
- `GET /health/ready`

Health endpoints are deliberate authentication exceptions and return no
sensitive configuration. Metrics are not served on the main API listener.

## 9. Authentication Implementation

### 9.1 Passwords

- Use Argon2id with a versioned encoded hash containing salt and parameters.
- Pin reviewed initial parameters and test verification performance on arm64.
- Permit future parameter upgrades after successful login.
- Never log usernames with password material or request bodies from login.

### 9.2 Sessions

- Generate at least 256 bits of cryptographic randomness.
- Store only a keyed or cryptographic token hash.
- Use HTTP-only cookies.
- Set Secure when the configured public URL uses HTTPS.
- Use a restrictive SameSite policy compatible with the same-origin UI.
- Enforce expiry and revocation server-side.
- Update last-seen in a write-throttled manner.
- Protect cookie-authenticated mutations with CSRF tokens.

### 9.3 Personal Tokens

- Prefix tokens so operators can identify token type without exposing secret
  material.
- Return plaintext exactly once.
- Hash before persistence.
- Implement initial scopes for inventory read/write, DNS read/write, agents,
  audit read, and administration.
- A browser session for the administrator has all scopes.

### 9.4 Rate Limiting

- Bound login attempts by source and account with a small in-memory limiter.
- Do not persist attacker-controlled high-cardinality limiter data in SQLite.
- Apply conservative request and body size limits to enrollment and snapshot
  endpoints.
- Rely on authentication and bounded workers rather than introducing a
  distributed rate-limiting dependency.

## 10. Audit Implementation

- Domain services create audit data in the same transaction as mutations.
- HTTP handlers pass actor and request context; repositories do not infer
  actors from globals.
- Redaction occurs before serialization to the audit event.
- Password hashes, token hashes, secret file paths where sensitive, session
  values, and provider headers never enter before/after documents.
- Failed authorization is logged operationally; rejected validated mutations
  may create audit events when an authenticated actor attempted them.
- Audit list API supports time, actor, action, resource type, and resource ID
  filters.

## 11. Inventory Implementation

### 11.1 Domain Services

Implement explicit services for providers, locations, computes, containers,
IPs, domains, costs, tags, and lifecycle operations.

Every service:

- Validates domain rules before repository calls.
- Uses a transaction for mutation plus audit.
- Checks expected revision.
- Distinguishes not-found, archived, validation, and conflict failures.
- Returns domain models independent from generated transport types.

Compute archival additionally revokes its agent and pending enrollment tokens in
the same audited transaction; restore requires explicit re-enrollment. Purge is
administrative, requires an archived target, and blocks on archived as well as
active dependent resources, DNS links and retained reports. It never implicitly
purges an archived subtree or retained discovery history.

### 11.2 Hierarchy

- Validate location and compute cycles in the mutation transaction.
- Validate allowed parent/child kinds.
- Build hierarchy reads through bounded recursive CTEs or a tested equivalent.
- Include archived ancestors only when necessary to explain an active broken
  relationship, otherwise exclude archived resources by default.

### 11.3 Current Observations

- Snapshot reconciliation updates discovery-owned current tables.
- Manual columns are not included in discovery update statements.
- A complete successful collector result is authoritative for absence.
- Failed collector results preserve previous children and record errors.
- Staleness is calculated from last-observed timestamps and policy, not by
  deleting rows.
- Rediscovery clears staleness and reactivates an archived discovered
  container with an audit event.

## 12. Discovery Protocol and Reconciliation

### 12.1 Canonical Snapshot

Define a versioned protocol document containing:

- Report ID.
- Agent and protocol metadata.
- Collection start/end time.
- Host facts.
- Interface and IP facts.
- Disk/filesystem facts.
- Per-Podman-connection result.
- All containers for every successful Podman connection.
- Sanitized collection errors.

Server receipt time, not client time, controls freshness. Client times remain
diagnostic and are validated against reasonable limits.

### 12.2 Server Ingestion

Ingestion sequence:

1. Authenticate the client credential and resolve the bound agent/compute.
2. Enforce protocol, content type, body size, and request timeout.
3. Detect an already accepted report ID and return its prior result.
4. Decode and validate the complete protocol document.
5. Normalize ordering, values, and omitted defaults.
6. Produce the canonical snapshot document and content hash.
7. Insert or reuse deduplicated snapshot content.
8. Insert the report and collector results.
9. Reconcile current observed state for successful collectors.
10. Write audit events for automatic reactivation or security-relevant state
    changes.
11. Commit atomically.

Large reports must not hold a global process lock. SQLite write serialization
must be bounded and observable.

### 12.3 Heartbeats

- Authenticate independently of snapshot ingestion.
- Update last heartbeat with write throttling if needed at capacity target.
- Record client/protocol version changes.
- Accept heartbeats even when the last collector failed.
- Return compatibility warnings but no host actions.

### 12.4 Freshness Worker

- Compute offline state from last heartbeat and configured threshold.
- Compute stale child state from last successful authoritative observation.
- Avoid rewriting unchanged rows on every worker pass.
- Expose counts and oldest state through metrics.
- Never auto-archive.

## 13. Discovery Agent Implementation

### 13.1 Process Lifecycle

1. Load and validate configuration.
2. Load existing credential or enroll with the one-time token.
3. Remove or render unusable the enrollment token after success.
4. Start independent heartbeat and snapshot schedules with startup jitter.
5. Run one collection at a time.
6. Replace the pending local snapshot after successful collection.
7. Upload with stable report ID until accepted or superseded by a newer
   complete snapshot according to documented policy.
8. Handle graceful shutdown without corrupting local state.

### 13.2 Host Collector

Use stable Linux sources such as:

- `/etc/os-release`.
- `/proc` for CPU, memory, uptime, and boot data.
- `/sys` where stable facts are needed.
- Netlink or a maintained Go networking API for interfaces and addresses.
- Filesystem/stat APIs for mounts and capacities.
- `/etc/machine-id` or equivalent stable machine identity.

Collection must not follow arbitrary untrusted symlinks or read secret files.
Each fact has explicit normalization and absence behavior.

### 13.3 Podman Collector

- Connect to each configured Unix socket independently.
- Use a pinned compatible Podman API version strategy.
- List all containers, then inspect only when required fields are absent from
  list responses.
- Bound concurrent inspect calls.
- Normalize ports, networks, mounts, labels, image identity, restart policy,
  state, and health.
- Mark a connection authoritative only after all required requests succeed.
- Report connection failure without converting it to an empty inventory.
- Never call mutation endpoints.

### 13.4 Local State

Local state contains:

- Agent credential.
- Credential generation or metadata.
- Latest pending normalized report and report ID.
- Last accepted report metadata useful for diagnostics.

Use separate credential and retry-state files if that produces safer
permissions and replacement. Write temporary files, flush, chmod, and rename
atomically.

### 13.5 Agent Diagnostics

- Structured journal-compatible logs.
- Version command.
- Config validation command.
- One-shot collection diagnostic that redacts credentials and does not upload
  unless explicitly requested.
- Meaningful exit codes for deployment automation.

## 14. Snapshot History and Diff

### 14.1 Canonicalization

- Assign a canonical format version independent from API protocol version.
- Sort all set-like collections deterministically.
- Normalize IPs, names, timestamps, absent values, and numeric types.
- Exclude report-specific timestamps and IDs from content hashing where they
  do not represent infrastructure state.
- Separate volatile uptime and filesystem available-byte measurements from the
  shared structural document. Preserve them in immutable per-report metadata and
  current observations, and reconstruct them for download and differences.
- Hash only the structural document, not its own checksum or report envelope.
- Include collector success/error state when it changes the meaning of the
  snapshot.
- Hash canonical uncompressed JSON bytes with SHA-256.
- Compress stored content with gzip after hashing.

### 14.2 Diff

The diff service returns:

- Host fields added, removed, or changed.
- Interfaces/IPs added, removed, or changed.
- Disks/filesystems added, removed, or changed.
- Containers added, removed, or changed by logical identity.
- Runtime-ID recreation as a field change on the same logical container.
- Collector-status differences.
- Volatile measurement differences, displayed separately from structural changes.

Diff output is structured JSON suitable for UI rendering, not only a text
patch.

### 14.3 Retention

- Default retention is 90 days.
- Delete report references in bounded batches.
- Remove content only when no retained report references it.
- Run cleanup on a low-frequency configurable schedule.
- Expose duration, deleted rows, reclaimed content, and failures as metrics.
- Provide a dry-run or reporting mode for operational inspection.

## 15. DNS Implementation

### 15.1 Domain Layer

Implement domain types for:

- Provider zone.
- DNS-name aggregate.
- Public/internal views.
- A, AAAA, and CNAME record sets.
- Draft and preview.
- Desired revision.
- Provider projection and tombstone.

Validation is provider-independent where possible. Cloudflare-specific proxy
rules belong in the adapter or an explicit capability validator.

### 15.2 Draft and Apply

- Persist canonical complete proposed aggregate documents in drafts.
- Recompute validation on every preview and apply.
- Refresh provider observations required for a trustworthy preview.
- Store a preview fingerprint tied to draft content and relevant observed
  state.
- Reject apply if draft content, base desired revision, or required observed
  provider state has become stale.
- Commit desired state, projection scheduling, idempotency result, and audit in
  one transaction.

### 15.3 Worker Model

- Poll due projection rows with bounded batch size.
- Claim work using a lease owner and expiry in a short transaction.
- Perform network or filesystem work outside the claim transaction.
- Complete only if the claimed desired revision is still current.
- If a newer revision exists, schedule that revision rather than overwriting
  its state.
- Recover expired leases after crashes.
- Apply exponential backoff with jitter and provider retry hints.

### 15.4 Cloudflare Adapter

Define a narrow interface supporting:

- List visible zones.
- List supported records for import or drift scan.
- Read exact record state.
- Create, update, and delete a provider record.

Use a fake implementation for domain and worker tests. Keep Cloudflare SDK or
HTTP response types out of domain packages and API responses.

### 15.5 Internal Publisher

- Read one consistent desired-state snapshot for a zone.
- Render deterministic authoritative zone content.
- Parse generated content before writing.
- Write and flush a same-filesystem temporary file.
- Atomically rename over the destination and flush the directory when
  supported.
- Store publication metadata only after successful rename.
- Ensure an older worker cannot overwrite a newer revision.
- Never remove or replace the prior valid file after generation failure.

### 15.6 Repair and Drift

- Periodically scan owned Cloudflare mappings at a bounded configurable rate.
- Compare semantically normalized state.
- Record drift and enqueue desired-state repair.
- Periodically find desired projections behind their current revisions.
- Expose manual name, zone, and global repair commands.
- Never import unknown records during repair.

## 16. Frontend Architecture

### 16.1 Application Shell

- Responsive Material UI navigation drawer and top bar.
- Route-level error boundary and not-found page.
- Session bootstrap before protected routes render.
- Consistent page title, breadcrumbs, loading, empty, and error states.
- Snackbar/toast for transient operation results, with durable failures also
  visible in page state.
- Generated API client wrapped by one authentication/error adapter.

### 16.2 Data Access

- TanStack Query owns server-state caching.
- Query keys are centralized by feature.
- List filters and pagination are reflected in URL search parameters.
- Mutations invalidate the smallest correct query set.
- Active agent and DNS status pages use short polling.
- Background polling pauses when the page is not visible where appropriate.
- Optimistic UI updates are limited to safely reversible metadata edits; DNS
  and archival operations wait for server acceptance.

### 16.3 Forms and Concurrency

- Forms initialize from generated response models.
- Client validation improves usability but never replaces server validation.
- Submit current revision through `If-Match`.
- A revision conflict shows that state changed and offers reload; it must not
  silently overwrite.
- Archive, purge, credential revoke, and DNS apply use explicit confirmation
  proportional to impact.

### 16.4 Dashboard

Implement cards and compact tables for:

- Offline computes.
- Stale discovered resources.
- Domain expiry warnings.
- DNS pending/failed projections.
- Recurring costs grouped by currency.
- Cost totals are monthly equivalents of active monthly, quarterly, semiannual
  and annual entries, explicitly labelled and rounded once per currency.

Every metric links to a filtered source list. The dashboard must not calculate
large aggregates by downloading complete collections to the browser.

### 16.5 Inventory

- Hierarchy view for location, compute, VM, and container containment.
- Table view with server pagination, search, filters, and archived toggle.
- Separate forms for manually owned fields.
- Compute tabs: overview, host facts, network/IPs, containers, costs, DNS links,
  snapshots, and audit.
- Container tabs: runtime, ports/networks, mounts, metadata, DNS links, history,
  and audit.
- Provider, location, domain, and cost list/detail forms.

### 16.6 Discovery and History

- Agent list with online/offline, protocol, version, last heartbeat, last
  snapshot, and collection errors.
- Compute-bound enrollment-token dialog showing plaintext once.
- Revoke/rotate workflow.
- Report timeline with collector result and content-reuse indication.
- Two-report selection and structured side-by-side diff.

### 16.7 DNS

- Zone discovery and explicit selection.
- Import candidate grouping and selective preview/apply.
- DNS-name list grouped by public/internal presence and status.
- Side-by-side public/internal editor.
- Multi-value A/AAAA controls and exclusive CNAME control.
- Wildcard owner control.
- Inventory-linked IP suggestions.
- Preview dialog separating Cloudflare and CoreDNS changes.
- Explicit apply action displaying expected revision.
- Projection status with last error and manual retry.

### 16.8 Responsive Behavior

- Desktop supports full tables and side-by-side DNS editing.
- Mobile converts dense tables to scrollable or card-based summaries.
- Reads, archive/restore, simple metadata edits, enrollment status, and DNS
  status remain practical on mobile.
- Complex DNS value editing remains functionally available without requiring
  visual parity with desktop.

## 17. Observability Implementation

### 17.1 Logging

Standard fields:

- Timestamp, level, component, message.
- Request/correlation ID.
- Actor type and non-secret ID where relevant.
- Resource ID, agent ID, zone ID, or DNS-name ID where relevant.
- Worker provider, revision, attempt, and duration.
- Sanitized error code.

Do not log full authentication requests, snapshot bodies, DNS secret headers,
or generated credential values.

### 17.2 Metrics

Use bounded labels. Never label metrics by DNS name, container ID, request ID,
or other unbounded resource identifiers.

Required groups:

- HTTP requests and latency.
- SQLite operation latency and busy results.
- Authentication successes/failures and active sessions.
- Agent freshness, versions, and collection failures.
- Snapshot ingestion, bytes, deduplication, and retention.
- Inventory resource counts by bounded type/state.
- DNS projection state, attempts, duration, retries, and drift.
- Cloudflare error classes and rate limiting.
- Internal publication revision age and failures.
- Domain expiry warning counts.

### 17.3 Health

Liveness:

- Process and HTTP event loop are responsive.

Readiness:

- Database is readable and writable.
- Schema version is supported.
- Configured required directories are accessible.
- Internal zone directory is writable when the publisher is enabled.

External Cloudflare health is reported in status and metrics, not readiness.

## 18. Backup, Migration, and Recovery

### 18.1 Migration Policy

- Every schema change is an immutable ordered migration.
- Migrations are tested from an empty database and from every released schema
  version still supported for upgrade.
- Destructive transformations use create/copy/verify/swap patterns appropriate
  for SQLite.
- Server startup reports required migration but does not apply it implicitly
  unless deployment explicitly selects an approved auto-migrate option in a
  future requirement.
- Deployment runs `portinaia db migrate` before starting the new server.

### 18.2 Backup

- `portinaia db backup` uses SQLite's consistent online backup mechanism.
- Backup writes to a new path and never overwrites an existing backup without
  an explicit flag.
- Command runs an integrity check on the produced backup.
- Generated zone files are reproducible but should remain included in host
  backup for immediate DNS continuity.
- Configuration and mounted secrets are backed up by external tooling with
  suitable protection.

### 18.3 Restore

Documented restore sequence:

1. Stop Portinaia.
2. Preserve the failed/current database for diagnosis.
3. Restore SQLite and associated configuration/secrets.
4. Run database integrity and migration status checks.
5. Start Portinaia and verify readiness.
6. Run DNS desired-state repair.
7. Confirm internal publication and Cloudflare projection status.

Unknown Cloudflare records remain unmanaged.

## 19. Testing Strategy

### 19.1 Unit Tests

- Inventory validation and lifecycle.
- Password/token hashing and scope checks.
- Agent protocol validation and normalization.
- Snapshot canonicalization and diff.
- DNS names, addresses, conflicts, drafts, revisions, and projection decisions.
- Retry and backoff calculations.

### 19.2 Database Integration Tests

- Run real migrations against temporary SQLite databases.
- Exercise sqlc queries and constraints.
- Test concurrent expected workloads and busy handling.
- Verify mutation/audit atomicity.
- Verify report idempotency and snapshot deduplication.
- Verify worker leasing and crash recovery.
- Verify retention and unreferenced-content removal.

### 19.3 Adapter and Client Tests

- Fake Cloudflare HTTP server for create/update/delete/import/drift/rate limit.
- Temporary filesystem and fault injection for atomic zone publication.
- Mock Unix HTTP sockets for multiple Podman versions and states.
- Linux host fact fixtures with missing and malformed source data.
- Agent offline state and atomic pending-report replacement.

### 19.4 Contract Tests

- Validate OpenAPI syntax and examples.
- Ensure generated server and frontend clients are current.
- Run handler tests against documented response schemas.
- Test RFC 7807 errors, revisions, ETags, pagination, and idempotency.
- Test v1 and unsupported-version rejection for the first release; test current
  and previous real protocol adapters beginning with v2.

### 19.5 Frontend Tests

- Authentication and session expiry.
- List filtering and URL state.
- Manual/discovered field separation.
- Revision conflicts.
- Agent enrollment plaintext-once behavior.
- Snapshot timeline and diff rendering.
- DNS edit, preview, apply, pending, success, and failure states.
- Responsive smoke coverage for required mobile workflows.

### 19.6 End-to-End Tests

The minimum browser/API scenario:

1. Create administrator and log in.
2. Create provider, location, compute, domain, and costs.
3. Enroll a fixture agent.
4. Ingest snapshots including container recreation and collector failure.
5. Observe freshness and snapshot diff.
6. Discover/select a fake Cloudflare zone and import a record.
7. Preview/apply split-horizon DNS.
8. Verify Cloudflare fake state and generated zone.
9. Inject one provider failure and verify isolation/retry.
10. Archive and restore inventory.
11. Create and use a scoped personal token.
12. Back up and verify the database.

### 19.7 Capacity Tests

Build deterministic data generation for the documented target:

- 500 computes/agents.
- 10,000 current containers.
- 100 zones.
- 100,000 DNS records.
- Five-minute report history with high unchanged-content reuse.

Measure:

- Snapshot ingestion latency and write contention.
- Current-state list/detail query latency.
- DNS list and worker claim latency.
- Snapshot retention batch behavior.
- Database and snapshot-content size.
- Raspberry Pi arm64 memory expectations or the closest reproducible
  constrained environment.

The approved reference is Raspberry Pi 4, 4 GB RAM, SSD, 64-bit Linux. A constrained
substitute may inform development but does not establish native hardware release
verification. [WP-16](coding/wp-16-verification.md) records approved load, latency,
memory and publication thresholds, plus the remaining storage-budget approval and
measurement evidence needed before asserting capacity success.

Do not claim the capacity target until thresholds are written and tests pass.

## 20. Local Developer Commands

The root build interface should provide at least:

```text
make generate       Regenerate sqlc, Go OpenAPI, and TypeScript API code
make format         Format Go, TypeScript, YAML, and specification files
make lint           Run static analysis without changing files
make test           Run unit and integration tests
make test-web       Run frontend tests
make test-e2e       Run browser smoke tests
make verify         Run generation check, formatting check, lint, and tests
make build          Build server and frontend for the host
make build-agent    Build supported discovery-client targets
make image          Build the local server image
make release        Produce cross-platform artifacts and checksums
```

Generated-code checks must fail if committed output differs from source
contracts.

## 21. Work Packages

Each package includes implementation, tests, and relevant documentation. A
package is not complete when only its happy path works.

### WP-00: Specification Baseline

Scope:

- Approve product requirements, DNS specification, and this plan.
- Resolve contradictions before code scaffolding.
- Record future requirement changes in all affected documents.

Dependencies: none.

Exit criteria:

- All three specifications are internally consistent.
- No unresolved product decision blocks schema or API design.

### WP-01: Repository and Build Foundation

Scope:

- Initialize Go and frontend workspaces.
- Add repository layout, Makefile, formatter, linters, and generated-file
  conventions.
- Add minimal server and frontend embed pipeline.
- Add minimal server and agent version commands.
- Add multi-stage Containerfile and cross-build targets.

Dependencies: WP-00.

Exit criteria:

- Minimal embedded UI and health server run locally.
- Server image builds for amd64 and arm64.
- Agent binaries build for amd64 and arm64.
- `make verify` succeeds.

### WP-02: OpenAPI and Database Contract

Scope:

- Define common OpenAPI conventions, security schemes, pagination, errors,
  IDs, times, revisions, and idempotency.
- Add the first complete API paths and schemas for all approved modules.
- Create initial SQLite migrations and sqlc configuration.
- Generate Go server interfaces and TypeScript client/types.
- Add contract and clean-migration tests.

Dependencies: WP-01.

Exit criteria:

- OpenAPI validates.
- Generated code compiles.
- Clean database migrates successfully.
- Major entities and workflow operations have no placeholder schemas.

### WP-03: Runtime Foundation

Scope:

- Configuration loading and validation.
- SQLite connection management and migration/status commands.
- Request IDs, structured logging, panic recovery, body limits, and problem
  responses.
- Liveness, readiness, and metrics listener.
- Graceful shutdown and worker lifecycle primitives.

Dependencies: WP-02.

Exit criteria:

- Server starts from a documented example config.
- Invalid config fails before listeners open.
- Health and metrics behavior is tested.
- SQLite connection settings are integration tested.

### WP-04: Authentication and Audit

Scope:

- Admin create/reset commands.
- Argon2id password storage.
- Browser login/logout/session and CSRF.
- Personal API tokens and scopes.
- Agent/enrollment authentication primitives.
- Audit transaction API and list endpoint.
- Login and body-size rate limits.

Dependencies: WP-03.

Exit criteria:

- Browser and token authentication work end to end.
- Plaintext credentials are returned once only.
- Mutation and audit rollback together.
- Secret-redaction tests pass.

### WP-05: Inventory Backend

Scope:

- Providers and roles.
- Hierarchical sites/provider regions.
- Computes and VM parent hierarchy.
- Manual IP assignments.
- Domains and expiry state.
- Multiple recurring and one-time costs.
- Tags, notes, archive, restore, and purge dependency checks.
- Inventory tree, detail aggregates, filters, and dashboard summary queries.

Dependencies: WP-04.

Exit criteria:

- Inventory CRUD and lifecycle APIs meet OpenAPI.
- Hierarchy cycles and invalid ownership are blocked.
- Revisions and audit work for all mutations.
- Medium-target list queries are indexed and tested.

### WP-06: Agent Enrollment and Protocol Server

Scope:

- Bound single-use enrollment tokens.
- Agent credential issuance, revocation, and rotation.
- v1 boundary adapter initially; current and previous real adapters from v2 onward.
- Heartbeat ingestion and freshness policy.
- Snapshot validation, idempotent receipt, collector-result semantics, and
  current-state reconciliation.

Dependencies: WP-05.

Exit criteria:

- Enrollment cannot create or bind another compute.
- Duplicate report IDs are idempotent.
- Failed collectors preserve prior children.
- Container logical identity and rediscovery behavior pass tests.

### WP-07: Discovery Agent

Scope:

- Agent config, credential enrollment, local state, and schedules.
- Linux host facts.
- Root and configured rootless Podman REST sockets.
- All-container collection and normalization.
- Heartbeat and snapshot upload with retry/jitter.
- Atomic latest-pending-snapshot persistence.
- Diagnostics, service example, release binaries, and checksums.

Dependencies: WP-02 for protocol contract; integration completion depends on
WP-06.

Exit criteria:

- Mocked host and Podman tests cover success, partial failure, and malformed
  data.
- Agent never invokes a mutation operation.
- Offline retry and restart preserve only the latest required snapshot.
- Linux amd64 and arm64 artifacts are produced.

### WP-08: Snapshot History

Scope:

- Canonical normalized format.
- Content hashing, compression, deduplication, and report references.
- Timeline and structured diff APIs.
- Ninety-day configurable retention and bounded garbage collection.
- Snapshot ingestion and retention metrics.

Dependencies: WP-06.

Exit criteria:

- Unchanged reports share content.
- Every retained report timestamp remains queryable.
- Runtime-ID changes appear as container changes, not duplicates.
- Retention cannot remove referenced content or current state.
- Capacity tests establish storage behavior.

### WP-09: DNS Domain and Draft Backend

Scope:

- Zone, DNS-name, record-set, value, link, draft, and projection schemas.
- Name/type/address/TTL/proxy/wildcard validation.
- DNS-name aggregate CRUD through drafts.
- Preview calculation and fingerprints.
- Explicit apply with revision, idempotency, projection scheduling, and audit.
- Tombstone lifecycle.

Dependencies: WP-05 and DNS portions of WP-02.

Exit criteria:

- Full DNS validator matrix passes.
- Preview is non-mutating.
- Stale preview/revision and unmanaged conflicts block apply.
- Apply atomically commits desired state, projections, and audit.

### WP-10: Cloudflare Adapter and Reconciler

Scope:

- Mounted token-file loading.
- Zone discovery and explicit selection.
- Selective import candidates, preview, and apply.
- Provider object mapping.
- Public create/update/delete/no-op reconciliation.
- Rate limits, retries, worker leases, and sanitized errors.
- Periodic drift detection and desired-state restoration.

Dependencies: WP-09.

Exit criteria:

- Fake provider suite covers all state transitions and failures.
- Unmanaged records are never changed.
- Equivalent provider normalization does not loop.
- Drift is visible, audited, and repaired.

### WP-11: CoreDNS Publisher

Scope:

- Complete deterministic authoritative zone generation.
- SOA/NS metadata and serial handling.
- DNS parse validation.
- Same-filesystem atomic publication.
- Publication state, retries, and revision ordering.
- Example CoreDNS configuration with reload and fallthrough.

Dependencies: WP-09.

Exit criteria:

- Invalid content cannot replace the last valid file.
- Concurrent revisions converge to newest desired state.
- Integration test proves internal override and public fallthrough.
- Cloudflare failure does not block publication.

### WP-12: Frontend Foundation and Inventory

Scope:

- Material UI theme, application shell, routing, session handling, API adapter,
  standard loading/error/empty states, and responsive patterns.
- Dashboard.
- Provider, location, compute, container, IP, domain, cost, and tag screens.
- Hierarchy and table inventory modes.
- Archive/restore and revision conflict behavior.

Dependencies: WP-04 and WP-05; shell work may begin after WP-02.

Exit criteria:

- Required inventory workflows pass frontend tests.
- Filters/pagination are URL-addressable.
- Manual and discovery-owned fields are visually distinct.
- Required mobile workflows are usable.

### WP-13: Frontend Agents and History

Scope:

- Agent list/detail, enrollment, revoke, rotate, status, and errors.
- Snapshot timeline and comparison.
- Polling lifecycle and compatibility warnings.

Dependencies: WP-06, WP-08, and WP-12 foundation.

Exit criteria:

- Enrollment secret is shown once.
- Offline/error states update through polling.
- Snapshot differences render for host and container changes.

### WP-14: Frontend DNS

Scope:

- Zone discovery/selection.
- Import candidate and selective import flow.
- DNS-name list/detail.
- Side-by-side public/internal editor.
- Single-name preview and explicit apply.
- Projection status, errors, and manual retry.

Dependencies: WP-09, WP-10, WP-11, and WP-12 foundation.

Exit criteria:

- All DNS acceptance workflows pass component and browser tests.
- UI never presents apply as complete synchronization.
- Provider failures remain understandable and independently visible.

### WP-15: Operations and Recovery

Scope:

- Database migrate/status/check/backup commands.
- DNS global repair command.
- Restore documentation.
- Production container hardening and persistent mount documentation.
- Reverse-proxy, CoreDNS, metrics, and external Ansible variable examples.
- Release packaging and checksum manifest.

Dependencies: WP-03, WP-08, WP-10, and WP-11.

Exit criteria:

- Backup and restore drill succeeds.
- Repair rebuilds projections without adopting unknown provider state.
- Container runs as a non-root application user where mounted permissions
  permit it.
- arm64 server deployment is verified.

### WP-16: Release Verification

Scope:

- Complete browser/API end-to-end scenario.
- Provider and filesystem failure injection.
- Capacity tests.
- Security and secret-redaction review.
- OpenAPI/generated-code drift check.
- Documentation consistency review.

Dependencies: WP-01 through WP-15.

Exit criteria:

- Every acceptance criterion in both normative specifications has an automated
  test or documented manual verification with justification.
- `make verify`, end-to-end tests, and supported builds pass from a clean
  checkout.
- No known critical or high-severity defects remain.
- Backup, restore, and DNS continuity are demonstrated.

## 22. Parallel Execution Plan

### Wave 1: Sequential Foundation

- WP-00 specification baseline.
- WP-01 repository foundation.
- WP-02 OpenAPI and database contract.

These packages should not be parallelized because every subsequent agent
depends on their files and conventions.

### Wave 2: Runtime and Security

- WP-03 runtime foundation.
- WP-04 authentication and audit.

Frontend shell exploration may begin against generated contracts, but it must
not redefine authentication behavior.

### Wave 3: Core Domain Tracks

After WP-04:

- WP-05 inventory backend proceeds first because discovery and DNS links depend
  on its identifiers.
- WP-07 agent collector implementation may proceed in parallel against the
  frozen protocol portion of WP-02.
- WP-12 frontend shell and authentication foundation may proceed in parallel.

### Wave 4: Discovery and DNS

After WP-05:

- WP-06 agent protocol server.
- WP-09 DNS domain and drafts.
- Remaining WP-12 inventory frontend.

These can run in parallel if migrations and OpenAPI are coordinated by one
contract owner.

### Wave 5: Integrations

- WP-08 snapshot history after WP-06.
- WP-10 Cloudflare after WP-09.
- WP-11 CoreDNS after WP-09.
- WP-13 agent/history frontend when backend contracts stabilize.
- WP-14 DNS frontend when backend contracts stabilize.

WP-10 and WP-11 are intentionally independent and should be parallelized.

### Wave 6: Hardening

- WP-15 operations and recovery.
- WP-16 release verification.

## 23. AI Agent Coordination Rules

### 23.1 Contract Ownership

During parallel waves, designate one contract owner for:

- `api/openapi.yaml`.
- Migration numbering and shared schema conventions.
- Generated API code.
- Shared API error and pagination packages.

Feature agents propose contract changes to that owner rather than creating
conflicting edits independently.

### 23.2 File Ownership

Assign each feature agent a disjoint primary directory. Shared-file edits must
be stated in the task before work begins. Agents must not reformat or rewrite
unrelated generated output or specifications.

Suggested ownership:

- Runtime agent: `internal/config`, `internal/database` connection layer,
  `internal/metrics`, server startup.
- Security agent: `internal/auth`, `internal/audit`, security middleware.
- Inventory agent: `internal/inventory` and inventory handlers/queries.
- Discovery-server agent: `internal/discovery`, ingestion handlers/queries.
- Discovery-client agent: `agent` and `cmd/portinaia-agent`.
- Snapshot agent: `internal/snapshots` and history handlers/queries.
- DNS-domain agent: `internal/dns` domain/draft code.
- Cloudflare agent: `internal/dns/cloudflare`.
- Internal-DNS agent: `internal/dns/internaldns`.
- Frontend agents: one `web/src/features` area each plus coordinated shared
  components.

### 23.3 Task Prompt Requirements

Every implementation task given to an AI agent must contain:

- Work-package ID and exact scope.
- Normative specification sections.
- Allowed primary files/directories.
- Dependencies already completed.
- API and migration constraints.
- Required tests and exact verification commands.
- Explicit non-goals.
- Expected final report including changed files, tests, and unresolved risks.

### 23.4 Completion Discipline

An agent must not mark a package complete until:

- Code is formatted and generated files are current.
- Relevant unit and integration tests pass.
- Full existing test suites affected by shared code pass.
- Error paths and retries are tested.
- Specifications or API examples are updated when contract behavior changed.
- No unrelated user or parallel-agent changes were reverted.

## 24. Release Gates

### Gate A: Foundation

- Clean build on supported architectures.
- Valid OpenAPI and migrations.
- Runtime, health, auth, audit, and configuration tests pass.

### Gate B: Inventory and Discovery

- Complete inventory workflows.
- Secure bound enrollment.
- Host and Podman discovery.
- Correct current-state ownership and freshness.
- Deduplicated history and diff.

### Gate C: DNS

- Explicit zones and selective import.
- Full validation matrix.
- Draft, preview, apply, retries, drift, and tombstones.
- Independent Cloudflare and CoreDNS projections.
- Last-valid internal file preservation.

### Gate D: Product UI

- Dashboard and all primary navigation routes.
- Inventory hierarchy/table and detail workflows.
- Agent enrollment/status/history.
- DNS zone/import/edit/preview/apply/status.
- Required responsive behavior.

### Gate E: Operations

- Metrics and structured logs.
- Migration, integrity, backup, restore, and DNS repair.
- arm64 server image and both agent artifacts.
- Capacity and failure-isolation tests.
- Complete deployment documentation.

## 25. Final Definition of Done

Portinaia MVP is done only when:

- All product acceptance criteria in `high-level-plan.md` pass.
- All DNS acceptance criteria in `dns-spec.md` pass.
- All release gates in this plan pass.
- The API document accurately describes the running server.
- The frontend uses the generated API contract.
- Initial v1 has compatibility and unsupported-version tests; once v2 exists,
  current and previous real discovery protocols have compatibility tests.
- A clean installation can create an administrator, build inventory, enroll an
  agent, ingest Podman state, and manage split-horizon DNS.
- A provider outage and an invalid internal publication are demonstrated not
  to destroy the other provider's valid state.
- A database backup can be restored and desired DNS state repaired.
- The server runs in one Podman container on 64-bit ARM Linux.
- Linux amd64 and arm64 client binaries and checksums are available.
- No critical requirement remains represented only by an untracked work item.
