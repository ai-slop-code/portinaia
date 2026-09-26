# Portinaia Product Requirements

Status: Approved for implementation

Implementation instructions and verified source baseline are indexed in
[Coding-agent specifications](coding/README.md). User decisions recorded on
2026-09-26 are detailed in [the decision register](coding/decisions.md).

## 1. Purpose

Portinaia is a self-hosted configuration management database (CMDB) and DNS
control plane for personal infrastructure. It provides one place to understand
which infrastructure exists, where it runs, what it costs, how resources are
related, which containers are present, and how public and internal DNS names
map to those resources.

The product is intentionally optimized for one administrator and one
single-node deployment. It must remain simple to install and operate while
being safe enough to own public DNS records and robust enough to preserve
useful state during client, provider, network, or application failures.

## 2. Goals

- Maintain a structured inventory of physical hosts, VPSs, VMs, containers,
  providers, locations, IP addresses, domains, and costs.
- Combine manually managed business metadata with automatically observed host
  and Podman facts without silently overwriting one source with the other.
- Represent typed infrastructure containment and DNS-to-resource links.
- Discover Linux host and Podman state using a read-only client deployed by
  Ansible.
- Retain current state and bounded, queryable discovery history.
- Manage selected Cloudflare public DNS zones and CoreDNS internal overrides
  through one desired-state API.
- Support split-horizon DNS with independent public and internal answers.
- Provide a versioned REST API and a responsive browser UI.
- Record an immutable audit trail for every mutation.
- Run the API, frontend, background workers, and internal DNS publisher in one
  Podman container on a 64-bit ARM Raspberry Pi.
- Support a medium lab without requiring PostgreSQL or a distributed system.

## 3. Non-Goals

The first version will not provide:

- Multiple human users, role-based access control, or external identity
  provider integration.
- Domain registrar operations.
- Full IP address management with pools, VLANs, or allocation workflows.
- systemd or generic operating-system service discovery.
- Libvirt or KVM host-side discovery.
- Remote command execution or container lifecycle control.
- Automatic DNS creation or deletion from discovered inventory.
- PostgreSQL, active-active operation, or high availability.
- etcd, PowerDNS, or Consul DNS backends.
- TLS certificate or reverse-proxy management.
- Direct email or webhook alert delivery.
- Agent self-update.
- A separate inventory or DNS command-line client.
- Ansible roles in this repository.

## 4. Users and Access

### 4.1 Human User

The first version supports exactly one administrator. The administrator can
read and modify all inventory and DNS data, manage agents, view audit history,
and create scoped personal API tokens.

The administrator authenticates with a local password. Multi-factor
authentication is not required in the first version.

### 4.2 Automation

External scripts may use individually named, scoped, and revocable personal
API tokens. A shared automation API key is not permitted.

### 4.3 Discovery Clients

Every discovery client has a unique credential bound to one pre-created
compute resource. Client credentials cannot access human administration
endpoints.

## 5. Product Modules

Portinaia is one modular product with a shared API, frontend, database,
authentication system, and audit history. Its modules are:

- Inventory
- Discovery
- Domains and costs
- DNS management
- Authentication and audit
- Operations and configuration

The modules must have explicit internal boundaries, but they are not separate
services or deployments.

## 6. Capacity Target

The design and automated performance tests must comfortably support:

- Up to 500 compute resources.
- Up to 10,000 current containers.
- Up to 100 managed DNS zones.
- Up to 100,000 DNS records.
- One concurrent administrator.
- One heartbeat per active agent every 60 seconds.
- One complete discovery snapshot per active agent every five minutes.

The reference capacity environment is a Raspberry Pi 4 with 4 GB RAM, SSD storage,
and 64-bit Linux. Approved numerical load, latency, memory and publication budgets
are defined in [WP-16](coding/wp-16-verification.md); storage budget follows
measurement and explicit approval. These targets are not a claim about the
currently implemented foundation.

These values are design targets, not hard product limits. APIs must use
pagination and bounded request sizes so larger data sets fail predictably
rather than exhausting process memory.

## 7. Inventory Model

### 7.1 Provider

A provider represents a hosting company, registrar, hardware vendor, or other
commercial supplier.

Required capabilities:

- Name and optional website.
- One or more roles, such as hosting provider, registrar, or hardware vendor.
- Optional external account or customer reference.
- Description, notes, tags, creation time, update time, and archive state.
- No provider login credentials or billing secrets.

### 7.2 Location

A location represents either a physical site or a provider region.

Required capabilities:

- Type: site or provider region.
- Name, description, and optional provider.
- Optional parent location to represent structures such as site and room or
  provider region and availability zone.
- Cycle prevention in the parent hierarchy.
- Notes, tags, creation time, update time, and archive state.

### 7.3 Compute

One compute entity represents dedicated physical hardware, a VPS, or a VM.
Using a shared entity keeps inventory queries and client enrollment uniform.

Required properties:

- Stable internal identifier.
- Kind: physical, VPS, or VM.
- Display name.
- Ownership: owned, rented, or other.
- Optional provider and provider-side resource identifier.
- Optional location.
- Optional parent compute for a VM.
- Description, notes, and tags.
- Creation, update, and archive timestamps.
- Current discovery and agent status when enrolled.

Valid containment rules:

- A location may contain computes.
- A compute may contain a VM compute.
- A VM parent relationship is manually managed in the first version.
- A container belongs to one compute.
- Containment cycles are forbidden.

### 7.4 Container

A container is an observed Podman workload attached to one compute.

Logical identity is the combination of:

- Compute identifier.
- Podman owner or connection identifier.
- Container name.

A recreation with the same logical identity and a new runtime ID remains the
same inventory resource. Runtime ID changes are retained in snapshot history.

Current observed properties include:

- Runtime ID and name.
- Image name, image ID, and image digest when available.
- Created time and started time.
- State and health state.
- Published and exposed ports.
- Networks and addresses.
- Volume and bind mounts.
- Labels.
- Restart policy.
- Podman owner or configured connection.
- Last observed time and stale state.

Users may add description, notes, tags, and DNS links without changing
discovery-owned fields.

### 7.5 IP Assignment

Portinaia tracks IP assignments but is not a full IPAM system.

Required properties:

- IPv4 or IPv6 address and prefix where known.
- Owning compute or container.
- Interface or network name where known.
- Scope or role, including public, private, loopback, link-local, or other.
- Source: manual or discovered.
- First observed, last observed, and stale timestamps for discovered values.
- Description and optional primary-address designation.

Manual and discovered assignments may coexist. Discovery updates only
discovery-owned assignments.

### 7.6 Domain

A domain represents a registered DNS domain, whether or not Portinaia manages
its DNS zone.

Required properties:

- Normalized internationalized or ASCII domain representation suitable for
  DNS operations.
- Registrar provider.
- Registration and expiry dates when known.
- Auto-renew state when known.
- Nameservers.
- Description, notes, tags, and archive state.
- Optional link to a managed Cloudflare zone.

The UI and metrics mark a domain as approaching expiry 30 days before its
expiry date. The threshold is configurable.

### 7.7 Cost

Costs may be attached to compute resources or domains.

Required properties:

- Cost type: recurring or one-time.
- Amount as a fixed-precision decimal.
- ISO 4217 currency code.
- Billing interval for recurring costs.
- Supported recurring intervals are monthly, quarterly, semiannual, and annual.
- Effective start and optional end date.
- Purchase or charge date for one-time costs.
- Description and optional provider reference.

One resource may have multiple cost entries. The product must not fetch
exchange rates or combine different currencies into a misleading total.
Totals are grouped by currency.

Dashboard recurring totals are explicitly labelled monthly equivalents. Use exact
rational conversion for the four supported intervals, sum before rounding, then
round half-up once per currency to its minor unit. Include only active recurring
costs with active owners and effective dates containing the current UTC date.
One-time costs are excluded from these totals.

### 7.8 Tags and Notes

Core schemas are fixed. The first version supports tags, descriptions, and
notes but not arbitrary user-defined fields or schemas.

### 7.9 Relationships

The first version supports:

- Typed containment relationships described above.
- DNS links from a managed DNS name to a compute, container, or IP assignment.

Arbitrary dependency graphs such as `uses` or `connects-to` are not part of
the first version.

## 8. Source Ownership

Portinaia uses field-level ownership rather than treating manual or discovered
data as globally authoritative.

Manually owned data includes:

- Compute kind, display name, ownership, provider, provider identifier,
  location, and parent.
- Descriptions, notes, and tags.
- Costs and domain registration data.
- DNS records and DNS links.
- Manually entered IP assignments.

Discovery-owned data includes:

- Machine identity, hostname, OS, kernel, architecture, and uptime.
- CPU, memory, disk, and interface facts.
- Discovered IP assignments.
- Podman container runtime facts.
- Agent version, report times, and collection errors.

Discovery must never silently replace a manually owned field. The UI may show
an observed hostname separately from a manually selected display name.

## 9. Resource Lifecycle

### 9.1 Archival

- Normal removal archives a resource.
- Archive is reversible and preserves relationships and audit history.
- Permanent purge is a separate explicit administrative operation.
- Purge must be blocked while non-archived dependent resources or DNS links
  exist.
- Purge also requires explicit cleanup of archived dependents and expiry of
  retained discovery reports; it must not cascade through a subtree or delete
  retained history. Target-owned fact/join and revoked credential metadata can be
  removed only after those dependency checks. Audit history is preserved.
- No resource is automatically archived.

Archiving a compute revokes its agent credential and outstanding enrollment tokens
in the same audited transaction. Restore does not revive credentials; explicit
re-enrollment is required, preserving the compute-bound agent identity/history.

### 9.2 Discovery Freshness

Defaults are configurable and initially set to:

- Agent heartbeat: 60 seconds.
- Full snapshot: five minutes.
- Compute offline threshold: five minutes without a heartbeat.
- Missing discovered child stale threshold: 24 hours.
- Automatic archival: disabled.

A failed container scan must not be interpreted as an empty successful scan.
The server preserves prior current state, records the collection error, and
waits for a successful authoritative snapshot before marking children absent.

If an archived discovered container is observed again, Portinaia reactivates
it automatically and records an audit event.

## 10. Discovery Client

### 10.1 Deployment and Privilege

The discovery client:

- Is a separate Go binary for Linux amd64 and arm64.
- Runs as a root-owned system service.
- Is installed, configured, credentialed, and upgraded by Ansible outside this
  repository.
- Is read-only and cannot execute server-supplied commands.
- Communicates outbound to the API over HTTPS through WireGuard.
- The initial release supports discovery protocol v1 only. Starting with v2, the
  server supports the current and immediately previous real protocol versions.
  There is no synthetic v0 protocol.

Root privilege is accepted so the client can collect host facts and inspect
explicitly configured root and rootless Podman sockets. The client must not
use that privilege for mutation.

### 10.2 Enrollment

Enrollment follows this workflow:

1. The administrator creates a compute resource.
2. The administrator generates a short-lived, single-use token bound to that
   compute.
3. Ansible installs the token and client configuration on the intended host.
4. The client exchanges the token for a unique long-lived client credential.
5. The returned credential is displayed or transmitted only once and stored
   in a root-only client file.
6. The server stores hashes of enrollment and client credentials.

Enrollment cannot create arbitrary compute resources. Tokens must be
revocable, expire, and become unusable immediately after successful exchange.
Client credentials can be revoked or rotated by re-enrollment.

### 10.3 Collected Host Facts

The standard host snapshot contains:

- Stable operating-system machine identity.
- Hostname.
- OS name and version.
- Kernel version.
- Architecture.
- CPU model and logical CPU count.
- Total memory.
- Disk devices, filesystems, capacity, and mount points.
- Network interfaces and addresses.
- Uptime and boot identifier.
- Virtualization role when reliably detectable.
- Client version and protocol version.

The first version does not collect serial numbers, firmware inventories,
SMART data, PCI/USB inventories, or KVM guest lists.

### 10.4 Podman Collection

- The client uses configured Podman REST sockets.
- Ansible is responsible for enabling and securing root and selected rootless
  sockets.
- All containers are reported, including created, running, paused, exited, and
  stopped containers.
- Socket failures are reported per connection.
- A successful complete response identifies containers absent from that
  connection; a failed response does not.
- The client does not shell out to execute server-provided commands.

### 10.5 Reporting and Offline Behavior

- Heartbeats and snapshots are outbound HTTPS pushes.
- Reports use stable request identifiers and are idempotent.
- Heartbeats continue when an individual collector fails.
- Payloads are bounded and may use HTTP compression.
- Retries use bounded exponential backoff with jitter.
- During API outages the client retains only the latest pending complete
  snapshot in an atomically written root-only local file.
- Missed heartbeat events and superseded snapshots are not replayed.

### 10.6 Snapshot History

The server retains every accepted report timestamp for a default of 90 days.
The retention period is configurable.

To remain viable at the capacity target:

- Current state is stored in normalized relational tables.
- Historical normalized states are canonicalized and content-addressed.
- Unchanged reports reference existing immutable snapshot content.
- Shared content contains structural infrastructure state. Volatile uptime and
  filesystem available-byte measurements are stored separately in current state
  and immutable per-report metadata, preserving historical values without making
  every measurement change duplicate structural content.
- Report identity, collection/receipt times and client/protocol metadata are
  separate from the shared content hash. Downloads/differences reconstruct the
  requested report, including its volatile measurements.
- Snapshot content is deduplicated and may be compressed.
- Raw HTTP request bodies are not retained.
- Retention cleanup removes unreferenced snapshot content safely.
- Audit events are not removed by snapshot retention.

The API and UI provide a timeline and field/resource differences between two
snapshots.

## 11. DNS Management

Portinaia manages desired DNS state for selected Cloudflare zones and internal
CoreDNS overrides. Public and internal provider projections are independently
reconciled and eventually consistent.

Key requirements:

- Multiple Cloudflare zones in one account.
- Explicit zone selection and selective import of existing records.
- A, AAAA, and CNAME record sets.
- Multiple A or AAAA values per name and view.
- Explicit wildcard records.
- Independent public and internal views under public domains.
- A preview and explicit apply step for every DNS-name change.
- No automatic DNS changes from discovery.
- Automatic restoration of Portinaia-owned Cloudflare records after external
  drift.
- Atomically generated authoritative zone files for CoreDNS on the same host.
- Independent retries and status for Cloudflare and internal publication.

The complete DNS behavior is normative in [dns-spec.md](dns-spec.md).

## 12. API Requirements

### 12.1 Contract

- REST JSON under `/api/v1`.
- OpenAPI is the source of truth.
- Go server interfaces and the TypeScript client/types are generated from the
  OpenAPI document.
- All identifiers and timestamps have one documented representation.
- Timestamps are stored in UTC and serialized as RFC 3339.
- The frontend displays times in the browser's local timezone.
- Errors use RFC 7807 problem details with stable machine-readable codes.

### 12.2 Collection Endpoints

- Cursor pagination with documented maximum page size.
- Stable sorting.
- Field-appropriate filtering.
- Text search where required by UI workflows.
- Archived resources excluded by default and available through an explicit
  filter.

### 12.3 Mutation Safety

- Resource revisions and `If-Match` implement optimistic concurrency.
- Retryable create and apply operations accept idempotency keys.
- Validation completes before committing desired state.
- Multi-table mutations and audit events commit in one database transaction.
- Background provider work never runs inside the HTTP request transaction.

### 12.4 Endpoint Areas

The API includes:

- Authentication and session management.
- Personal API token management.
- Providers, locations, computes, containers, IP assignments, domains, costs,
  tags, and DNS links.
- Agent enrollment, heartbeat, snapshot, status, revocation, and rotation.
- Snapshot timeline and comparison.
- DNS zones, names, drafts, preview, apply, imports, and reconciliation.
- Audit history.
- Liveness and readiness.

## 13. Frontend Requirements

### 13.1 Technology and Style

- React and TypeScript built with Vite.
- Material UI component library.
- Balanced administration layout rather than an extremely dense console.
- Embedded static assets served by the Go server.
- Short polling on active agent and reconciliation views.
- English UI.
- Responsive read flows and basic edits on mobile.
- Complex DNS editing remains functional on mobile but is desktop-oriented.

### 13.2 Primary Navigation

- Dashboard
- Inventory
- Providers
- Locations
- Domains
- DNS
- Agents
- Audit
- Settings

### 13.3 Required Workflows

Dashboard:

- Show offline computes and stale resources.
- Show upcoming domain expiry.
- Show pending or failed DNS reconciliation.
- Show recurring costs grouped by currency.
- Link every summary to a filtered detail view.

Inventory:

- Switch between a typed hierarchy and filterable resource tables.
- Create and edit manually owned metadata.
- Archive and restore resources.
- Open compute details with overview, observed facts, IPs, containers, costs,
  DNS links, snapshots, and audit history.
- Open container details with current facts and history.

Domains and DNS:

- Maintain registration and expiry data.
- Discover and select Cloudflare zones.
- Preview and selectively import existing records.
- Edit public and internal record sets together by DNS name.
- Preview a single DNS-name change and explicitly apply it.
- Display independent Cloudflare and CoreDNS synchronization status.

Agents and history:

- Generate enrollment tokens bound to computes.
- Revoke and rotate credentials.
- Show heartbeat, version, collection errors, and protocol compatibility.
- Browse snapshot timelines and compare two snapshots.

## 14. Authentication and Security

### 14.1 Administrator Authentication

- Passwords are hashed with Argon2id using versioned parameters.
- Initial administrator creation and password reset use explicit server binary
  commands against the mounted database.
- There is no unauthenticated first-run setup page.
- Browser sessions use random server-side credentials.
- Session credentials are stored hashed in SQLite.
- Cookies are HTTP-only, secure when served over HTTPS, and use an appropriate
  SameSite policy.
- Persistent session lifetime defaults to 30 days.
- Sessions can be revoked.
- Cookie-authenticated mutations have CSRF protection.
- Login attempts are rate limited without making denial of service trivial.

### 14.2 Personal API Tokens

- Tokens are named, scoped, optionally expiring, and individually revocable.
- Plaintext is returned only at creation.
- Only token hashes are stored.
- Token use updates a last-used timestamp without excessive database writes.

### 14.3 Secrets

- Cloudflare and other server secrets enter the container through mounted
  secret files.
- Provider credentials are not stored in SQLite.
- Secret files are never returned through APIs or included in logs.
- Discovery credentials and personal tokens are stored as one-way hashes.
- Sensitive headers and provider response headers are redacted.

### 14.4 Network Boundary

- The application listen address is configurable.
- Podman port publishing and the existing reverse proxy determine whether the
  UI is reachable from LAN, WireGuard, or public networks.
- The application does not assume that network placement replaces
  authentication.
- Agent traffic is intended to travel through WireGuard over HTTPS.
- Reverse-proxy forwarding headers are trusted only when explicitly enabled
  for configured proxy addresses.

### 14.5 Audit

Every mutation records:

- Actor type and stable actor identifier.
- Timestamp.
- Action and resource.
- Previous and resulting values, with secrets redacted.
- Request or correlation identifier.
- Success or rejection result when relevant.

Audit events are retained indefinitely unless the administrator runs an
explicit future pruning operation.

## 15. Runtime and Deployment

### 15.1 Server Shape

One Go process and image contain:

- REST API.
- Embedded frontend.
- Authentication and audit services.
- Background reconciliation workers.
- Snapshot retention worker.
- Local CoreDNS zone publisher.
- Administrative database commands.

The process uses mounted paths for:

- SQLite database and associated files.
- Configuration.
- Secrets.
- Generated CoreDNS zone files.

### 15.2 Database

- SQLite in WAL mode.
- Pure-Go modernc SQLite driver.
- Foreign key enforcement.
- Busy timeout and deliberately bounded write concurrency.
- Explicit SQL migrations.
- Type-safe sqlc queries.
- Consistent online backup support.
- Integrity-check command.
- The server refuses to start with an unsupported schema version.

### 15.3 Reverse Proxy and TLS

Portinaia listens on configurable internal HTTP. An existing reverse proxy
terminates HTTPS and owns certificates. Portinaia does not deploy or configure
that proxy.

### 15.4 Release Artifacts

The repository produces:

- Multi-architecture server container image for Linux amd64 and arm64.
- Discovery client binaries for Linux amd64 and arm64.
- Checksums for released binaries.
- Example configuration and deployment documentation.
- CoreDNS integration example.
- Local build, test, generation, image, and release commands.

There is no hosted CI requirement initially. Ansible consumes a pinned client
release and verifies its checksum. Ansible implementation remains in another
repository.

## 16. Backup and Recovery

External tooling owns backup schedule, retention, off-host copying, and restore
testing. Portinaia must make safe backup possible.

Required capabilities:

- Online SQLite backup command or equivalent safe mechanism.
- Documented list of persistent data, configuration, and secret paths.
- Documented restore procedure.
- Database integrity check after restore.
- Reconciliation command to rebuild provider projections from desired state.
- No automatic import of unknown provider records during recovery.
- CoreDNS continues serving the last valid zone files while Portinaia is down.
- Cloudflare continues serving its last successfully synchronized state.

## 17. Observability

### 17.1 Logs

- Structured JSON in production.
- Human-readable development mode may be configurable.
- Request and correlation identifiers.
- Secret and authorization-header redaction.
- Clear reconciliation attempt, result, and retry context.

### 17.2 Health

- Liveness reports whether the process can serve requests.
- Readiness checks local mandatory dependencies such as database access and
  required writable paths.
- Cloudflare or other external provider outages do not make the application
  unready.

### 17.3 Metrics

Prometheus-compatible metrics use a separate optional configurable listener
intended for a trusted network.

Metrics include:

- Request count, latency, and error count.
- Database operation and busy metrics.
- Computes by freshness state.
- Agent version and protocol state.
- Collection failures.
- Snapshot acceptance, deduplication, size, and retention cleanup.
- DNS desired, pending, synchronized, failed, and deleting projections.
- Reconciliation latency, retries, Cloudflare errors, and rate limits.
- Internal publication revision and age.
- Domains approaching expiry.

## 18. Testing Requirements

A release requires an automated test pyramid:

- Go unit tests for validation, lifecycle rules, authorization, and
  reconciliation decisions.
- SQLite integration tests using real migrations and queries.
- OpenAPI contract tests.
- HTTP handler tests.
- Agent collector tests using host fixtures and mocked Podman sockets.
- Snapshot canonicalization, deduplication, diff, and retention tests.
- Cloudflare adapter tests against a deterministic fake HTTP server.
- CoreDNS zone generation and atomic-publication tests.
- React component and workflow tests.
- A small browser end-to-end smoke suite.
- Cross-compilation and container startup checks for supported architectures.
- Capacity tests representing the documented medium-lab target.

Tests must include failures and retries, not only successful workflows.

## 19. Acceptance Criteria

The first release is complete when:

- Providers, sites, regions, computes, IPs, domains, costs, tags, and notes can
  be maintained through API and UI.
- Infrastructure can be browsed as a hierarchy and as filtered tables.
- A pre-created compute can issue an enrollment token and accept exactly one
  intended agent enrollment.
- Linux amd64 and arm64 agents report host facts and all Podman container
  states through WireGuard-compatible HTTPS.
- Manual data is not overwritten by discovery.
- Repeated snapshots are idempotent and do not duplicate logical containers.
- Offline, stale, archive, restore, and rediscovery behavior matches this
  specification.
- Every report remains visible during retention without storing duplicate
  unchanged state content.
- Structurally unchanged reports share content even when volatile measurements
  differ; each retained report's measurements remain available in history/diff.
- Snapshot differences can be viewed through API and UI.
- Multiple Cloudflare zones can be discovered and explicitly selected.
- Existing Cloudflare records can be selectively imported.
- Public and internal DNS views can differ for the same name.
- Every DNS edit is previewed and explicitly applied.
- Cloudflare and CoreDNS failures do not roll back or disable one another.
- Reconciliation is idempotent and external drift is corrected for owned
  records.
- Invalid zone content cannot replace the last valid internal publication.
- All mutations are audited.
- Secrets do not appear in logs or ordinary responses. The explicitly authorized
  one-time credential issuance responses are the documented exception and use
  `Cache-Control: no-store`; plaintext is never replayed from persisted responses.
- Safe backup and documented restore succeed.
- The server image runs on arm64 and released agent binaries have checksums.
- The full automated release test suite passes.

## 20. Related Specifications

- [DNS Management Specification](dns-spec.md)
- [Implementation Plan](implementation-plan.md)
- [Coding-agent Specifications and Implementation Audit](coding/README.md)
