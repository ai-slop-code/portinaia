# Portinaia DNS Management Specification

Status: Approved for implementation

Detailed implementation instructions and baseline gaps are in
[WP-09](coding/wp-09-dns-domain.md), [WP-10](coding/wp-10-cloudflare.md),
[WP-11](coding/wp-11-internal-dns.md), and [WP-14](coding/wp-14-web-dns.md).

## 1. Purpose

Portinaia provides one desired-state control plane for selected Cloudflare
public DNS zones and internal DNS overrides served by CoreDNS. The browser,
automation clients, and discovery agents never receive Cloudflare credentials
or direct write access to CoreDNS files.

The public and internal projections are independently reconciled. A failure in
one provider must not roll back, remove, or block a valid state in the other.

This document is normative for DNS behavior. General product, authentication,
deployment, and audit requirements are defined in
[high-level-plan.md](high-level-plan.md).

## 2. Goals

- Manage multiple selected Cloudflare zones in one account.
- Manage internal overrides under those public domains.
- Support split-horizon answers for the same DNS name.
- Support A, AAAA, and CNAME record sets.
- Support more than one A or AAAA value per name and view.
- Support explicit wildcard names.
- Require preview and explicit apply for every DNS-name change.
- Reconcile Cloudflare and CoreDNS asynchronously and idempotently.
- Selectively import existing Cloudflare records.
- Preserve unmanaged provider records.
- Detect and repair drift in Portinaia-owned Cloudflare records.
- Publish complete, validated internal zone files atomically.
- Expose status, retries, errors, and audit history without exposing secrets.
- Keep provider-specific details behind adapter interfaces.

## 3. Non-Goals

- Registrar management.
- DNSSEC key or delegation management.
- TLS certificate management.
- Reverse-proxy configuration.
- Replacing Cloudflare as public authoritative DNS.
- Replacing CoreDNS as the LAN and WireGuard DNS frontend.
- Replacing Unbound as the validating recursive resolver.
- Internal-only private suffixes such as `home.arpa`.
- Record types other than A, AAAA, and CNAME in the first version.
- Automatic DNS writes from inventory discovery.
- Transactional atomicity between Cloudflare and CoreDNS.
- Automatically adopting all provider records.
- Multiple Cloudflare accounts or credentials.
- Dynamic etcd, PowerDNS, or Consul backends.
- Using Ansible for runtime record creation, modification, or deletion.

## 4. Current Environment

- Cloudflare hosts public DNS, initially including `m8.sk`.
- Portinaia may manage multiple zones visible to one Cloudflare API token.
- CoreDNS is the LAN and WireGuard DNS frontend at `192.168.1.35:53`.
- CoreDNS forwards unresolved queries to Unbound at `127.0.0.1:5335`.
- WireGuard uses `10.8.0.0/24`.
- CoreDNS and the Portinaia server run on `krabicka.m8.sk`.
- Portinaia runs in a Podman container.
- A host directory containing generated zones is mounted into the container
  for writing and read by CoreDNS on the host.
- An existing reverse proxy terminates TLS for Portinaia.

Addresses and file paths are deployment configuration, not hard-coded product
defaults.

## 5. Architecture

```text
Browser or API client
        |
        v
Portinaia REST API
        |
        v
DNS desired state in SQLite
        |
        +------------------------+
        |                        |
        v                        v
Cloudflare reconciler      Internal publisher
        |                        |
        v                        v
Cloudflare API          Mounted authoritative zone files
                                 |
                                 v
                              CoreDNS
                                 |
                                 v
                              Unbound
```

The API commits desired state. Background workers materialize provider
projections. HTTP mutation handlers must not call Cloudflare or publish files
inside their database transaction.

## 6. Ownership Boundaries

### 6.1 Portinaia

Portinaia owns:

- Which Cloudflare zones are managed.
- Explicitly imported and newly created DNS names and record sets.
- Desired public and internal values.
- Drafts and previews.
- Provider object identifiers for owned Cloudflare records.
- Internal zone generation and publication metadata.
- Provider synchronization state and retries.
- Tombstones and deletion completion.
- Drift detection for owned records.
- DNS validation and conflict detection.
- Audit events.

### 6.2 Cloudflare

Cloudflare remains the public serving system. Records not explicitly imported
or created through Portinaia are unmanaged. Portinaia must not modify or
delete unmanaged records.

### 6.3 CoreDNS and Unbound

CoreDNS serves internal overrides from Portinaia-generated authoritative zone
files. Unbound remains responsible for recursive resolution. CoreDNS and
Unbound installation and static configuration remain externally managed.

### 6.4 Ansible

Ansible or equivalent external deployment automation owns:

- Installation and static configuration of CoreDNS and Unbound.
- Podman container deployment.
- Persistent directories, ownership, permissions, and mounts.
- Secret-file delivery.
- CoreDNS configuration that loads generated zones and falls through for
  names without an internal override.
- Monitoring and backup configuration.

Ansible must never render or overwrite Portinaia's runtime zone files.

### 6.5 Discovery Client

Discovery clients do not manage DNS. Inventory data may be selected when
editing a DNS value, but creating, modifying, or deleting DNS always requires
an explicit authenticated user or API operation.

## 7. Domain and Zone Model

### 7.1 Domain Inventory

A domain inventory resource may exist without managed DNS. A Cloudflare zone
can be linked only to a matching normalized domain.

### 7.2 Provider Zone

A discovered Cloudflare zone records:

- Stable Portinaia identifier.
- Cloudflare zone identifier.
- Normalized zone name.
- Cloudflare status needed for display.
- Discovery time and last provider refresh.
- Selection state: available, managed, or disabled.
- Linked domain inventory identifier when managed.
- Current reconciliation and drift-scan metadata.

Discovering a zone does not make it managed. The administrator must explicitly
select it and link or create its domain inventory resource.

### 7.3 Internal Zone Scope

Internal overrides are allowed only under selected managed public domains.
Private suffixes are not supported in the first version.

For each managed public zone, the internal publisher may generate a matching
authoritative zone containing only internal overrides and required zone
metadata. CoreDNS must fall through for absent names so public resolution
continues through Unbound.

## 8. DNS Name Aggregate

The unit edited, previewed, revised, and applied is one DNS-name aggregate.

Example:

```yaml
id: 0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4
zone: m8.sk
name: grafana.box.m8.sk
description: Private Grafana endpoint
links:
  - kind: container
    id: 0190d7c0-28f3-71a2-a0fd-d17dd083462a
public:
  record_sets:
    - type: A
      ttl: 60
      values:
        - 8.8.8.8
      proxied: false
internal:
  record_sets:
    - type: A
      ttl: 60
      values:
        - 10.8.0.1
revision: 7
```

The public address above is illustrative globally routable input, not a deployment
recommendation. Documentation-only ranges such as `203.0.113.0/24` are deliberately
invalid public values under §12 and belong in rejection-test fixtures instead.

Required aggregate properties:

- Stable identifier.
- Managed zone identifier.
- Fully qualified normalized name.
- Whether the left-most label is an explicit wildcard.
- Optional description.
- Optional typed links to computes, containers, or IP assignments.
- Public view containing zero or more record sets.
- Internal view containing zero or more record sets.
- Integer desired revision.
- Creation, update, optional archive, and optional deletion timestamps.

A name may exist in only the public view, only the internal view, or both.

## 9. Record Set Model

A record set belongs to one DNS-name aggregate and one view.

Required properties:

- Type: A, AAAA, or CNAME.
- TTL from 60 through 86,400 seconds, inclusive.
- One or more normalized values.
- Cloudflare `proxied` state where valid in the public view.

Defaults:

- TTL: 60 seconds.
- Cloudflare proxying: disabled unless explicitly selected.

Cardinality:

- An A set has one or more unique IPv4 values.
- An AAAA set has one or more unique IPv6 values.
- A CNAME set has exactly one absolute normalized DNS name.
- A view has at most one set of each supported type.
- Values have deterministic ordering for canonicalization and previews.

Cloudflare may represent individual values as provider objects. Portinaia
exposes them as one logical record set and stores provider object identifiers
per projected value.

## 10. Name Validation

- Names are normalized to lower-case ASCII DNS form.
- A name must belong to its selected managed zone.
- Relative names accepted by UI or API are converted to absolute names before
  storage.
- Trailing-dot input is accepted and normalized consistently.
- Labels and total name length must satisfy DNS limits.
- Empty labels are forbidden except for the final root marker.
- The wildcard form is explicit and limited to a single left-most `*` label.
- Embedded or multiple wildcard labels are forbidden.
- The zone apex is represented explicitly and consistently by the API.
- CNAME at the zone apex is rejected so public and internal semantics remain
  portable and compatible with authoritative zone requirements.

## 11. Record Conflict Rules

Within one view of one name:

- CNAME cannot coexist with A, AAAA, or another CNAME set.
- A and AAAA may coexist.
- Duplicate values are rejected after normalization.
- Wildcard and exact names are distinct aggregates.
- A DNS name must not be duplicated in another active aggregate in the same
  managed zone.

Before creating or importing a public record, Portinaia must inspect provider
state for conflicting unmanaged records. An unmanaged conflict blocks apply
and is shown in the preview; it is never silently adopted or deleted.

## 12. Address Validation

### 12.1 Public View

Public A and AAAA values must be globally routable unicast addresses.

The server hard-rejects addresses in categories including:

- RFC 1918 private IPv4.
- IPv6 unique-local addresses.
- Loopback.
- Link-local.
- Unspecified.
- Multicast.
- Documentation and benchmarking ranges.
- Carrier-grade NAT and other non-public special-use ranges.
- Reserved ranges not intended for global unicast publication.

There is no warning override in the first version. A rejected private address
must be placed in the internal view.

### 12.2 Internal View

Internal A and AAAA values may use:

- Standard private IPv4 ranges.
- IPv6 unique-local addresses.
- Link-local addresses where operationally meaningful.
- Globally routable unicast addresses.

Loopback, unspecified, multicast, documentation-only, and invalid addresses
remain rejected unless a later requirement explicitly permits them.

No installation-specific CIDR allowlist is required in the first version.

## 13. CNAME Validation

- CNAME values are normalized absolute DNS names.
- A CNAME value cannot equal its owner name.
- The proposed managed desired state must not create an immediately detectable
  CNAME cycle among Portinaia-owned names.
- External targets are allowed.
- A CNAME target is not required to exist at change time.
- Cloudflare proxying is allowed only when Cloudflare supports it for the
  record and selected configuration.

## 14. Draft, Preview, and Apply Workflow

### 14.1 Draft

Creating, editing, or deleting a DNS name first creates a draft. A draft
contains:

- Stable draft identifier.
- Operation: create, update, or delete.
- Base aggregate identifier and revision for update/delete.
- Complete proposed DNS-name aggregate for create/update.
- Actor and creation/update timestamps.
- Validation status and errors.
- Latest preview and provider-state observation time.

Only one active draft per DNS-name aggregate and actor is needed because the
first version has one administrator. Drafts are not desired state and are not
reconciled.

### 14.2 Preview

Preview validates the complete proposed aggregate and compares it with:

- Current Portinaia desired state.
- Current known Cloudflare state, refreshed when needed.
- Current generated internal projection.
- Unmanaged Cloudflare records that would conflict.

The preview returns:

- Public creates, updates, unchanged values, and deletions.
- Internal creates, updates, unchanged values, and deletions.
- Validation errors and warnings.
- Unmanaged conflicts.
- Current base revision.
- Whether provider state changed since the prior preview.

Preview never mutates desired or provider state.

### 14.3 Apply

Apply requires:

- An authenticated actor with DNS write scope.
- The draft identifier.
- The expected base revision.
- An idempotency key.
- A successful preview matching the draft contents.

In one SQLite transaction, apply:

1. Revalidates the draft and optimistic-concurrency revision.
2. Creates or updates desired state, or creates a deletion tombstone.
3. Increments the desired revision.
4. Creates or resets public and internal projection jobs as appropriate.
5. Writes the audit event.
6. Marks the draft applied.

Provider work starts only after transaction commit. Apply returns accepted
desired state and projection statuses; it does not wait for synchronization.

If synchronization later fails, desired state remains in place and visible.
Reverting is another previewed and applied revision, not a hidden rollback.

## 15. Optimistic Concurrency and Idempotency

- Every DNS-name aggregate has a monotonic integer revision.
- Update/delete drafts capture the base revision.
- Applying against a different current revision returns a conflict and
  requires a new preview.
- Repeated apply with the same idempotency key and equivalent request returns
  the original result.
- Reusing an idempotency key for different content is rejected.
- Provider workers are independently idempotent and may safely retry after
  uncertain outcomes.

## 16. Projection State

Each desired revision has independent public and internal projection state
when the corresponding view is relevant.

Projection properties:

- Provider: Cloudflare or internal.
- Desired revision.
- Last successfully applied revision.
- State: not-applicable, pending, synchronized, failed, or deleting.
- Attempt count.
- Last attempt and next retry time.
- Last success time.
- Last sanitized error code and message.
- Provider object identifiers where applicable.
- Observed provider checksum or publication checksum.

The aggregate is fully synchronized only when every applicable projection has
applied the current desired revision.

## 17. Reconciliation

### 17.1 General Rules

- Workers claim jobs transactionally without running provider calls inside a
  write transaction.
- Retries use bounded exponential backoff with jitter.
- Permanent validation or conflict errors remain failed until user action or a
  relevant state change.
- Transient network, provider, and rate-limit errors retry automatically.
- Provider rate-limit hints are respected.
- Work is safe after process restart.
- A periodic repair scan enqueues work that is missing or behind desired
  state.
- Logs and metrics identify the aggregate, view, revision, and attempt without
  secrets.

### 17.2 Partial Failure

Cloudflare and internal publication are independent:

- A Cloudflare failure does not revert a successful internal publication.
- An internal publication failure does not modify public records.
- One failed aggregate does not block unrelated aggregates or zones.
- UI and API show each projection separately.

## 18. Cloudflare Integration

### 18.1 Credential

- One token is supplied through a mounted secret file.
- The token has DNS read/edit and zone read permissions only for intended
  zones.
- The token is never persisted in SQLite.
- Token content and provider authorization headers are always redacted.

### 18.2 Zone Discovery

- Portinaia lists zones visible to the token.
- Discovery is read-only.
- The administrator explicitly selects each managed zone.
- A selected zone is linked to matching domain inventory.
- Losing provider visibility marks a managed zone degraded; it does not delete
  local desired state.

### 18.3 Import

Import workflow:

1. Fetch supported existing records for a selected zone.
2. Group compatible provider objects into Portinaia record sets.
3. Mark unsupported, malformed, or conflicting records as non-importable with
   a reason.
4. Let the administrator select individual DNS names or compatible sets.
5. Preview the desired objects to be created.
6. Apply import transactionally without rewriting unchanged provider records.

Imported records become Portinaia-owned desired state with synchronized public
projections. Unselected records remain unmanaged.

### 18.4 Provider Object Mapping

The adapter stores enough mapping to update or delete only owned objects:

- Cloudflare zone ID.
- Cloudflare record ID per projected value.
- Type, normalized name, value, TTL, and proxy state checksum.
- Portinaia desired revision last associated with the provider object.

Missing or stale IDs must be recovered by a constrained exact lookup. The
adapter must reject ambiguous matches rather than choosing one.

### 18.5 Drift Detection

Portinaia periodically reads provider state for owned records.

- Equivalent state updates observed metadata without writing.
- External modification creates an audit-visible drift event and enqueues the
  current desired revision for restoration.
- External deletion is recreated.
- An unexpected conflicting unmanaged object blocks repair and becomes a
  visible failed projection.
- Unknown provider records are never imported automatically.

### 18.6 Proxy and TTL

- Explicit TTL and proxy state are preserved when supported.
- Proxying is public-view-only.
- Proxy validation follows Cloudflare capabilities for A, AAAA, and CNAME.
- Provider normalization that is semantically equivalent must not cause an
  endless drift loop.

## 19. Internal DNS Publication

### 19.1 File Format

The first internal backend is an authoritative zone file, not the CoreDNS
`hosts` plugin. Zone files preserve TTLs and real CNAME semantics.

One complete file is generated per managed zone with internal records.

Generated content includes:

- Required SOA and NS metadata suitable for CoreDNS file serving.
- A deterministic serial derived from or advanced with publication revision.
- All active internal A, AAAA, and CNAME record sets for the zone.
- Deterministic ordering and formatting.
- A generation marker and checksum in comments or sidecar metadata where
  operationally useful.

Portinaia does not generate public-only records into internal files. CoreDNS
fallthrough permits those names to resolve publicly through Unbound.

### 19.2 Publication Algorithm

For an internal desired-state change:

1. Read a consistent database snapshot of all active internal records in the
   affected zone.
2. Generate the complete zone in memory or a private temporary file.
3. Parse and validate the generated zone using a DNS parser.
4. Verify expected origin, name containment, SOA/NS presence, and supported
   types.
5. Write a temporary file in the destination filesystem.
6. Set required ownership and permissions where the container is permitted to
   do so.
7. Flush file contents and atomically rename over the destination.
8. Flush the containing directory when supported.
9. Record checksum, revision, publication time, and success.

A generation, validation, write, or rename failure leaves the previous valid
file untouched. A directory-flush or database-metadata failure after successful
rename is an uncertain durability/metadata outcome: the newly published valid file
remains in place and recovery verifies its checksum and serial. It must not be
overwritten with an older revision or reported as though rename never occurred.

### 19.3 CoreDNS Configuration Contract

External CoreDNS configuration must:

- Load each generated zone from the configured directory.
- Reload changed files automatically.
- Fall through for names not present in the internal override zone.
- Forward unresolved queries to Unbound.
- Retain read access when the Portinaia container is stopped.

The repository provides a documented example but does not own the host's
complete CoreDNS configuration.

The owner permits upgrading to an explicitly pinned and tested CoreDNS build
supporting the required authoritative-file/auto reload and fallthrough behavior.
The implementation must record exact version and real query-test evidence;
external deployment automation performs the host upgrade.

### 19.4 Publication Scope

The publisher runs inside the main Portinaia server process because CoreDNS
and Portinaia share a host. The container receives write access only to the
dedicated generated-zone directory, not unrestricted host or root access.

## 20. Deletion

Deleting a DNS-name aggregate is previewed and explicitly applied.

- Apply creates a tombstone rather than immediately purging desired metadata.
- Public reconciliation deletes only mapped owned Cloudflare objects.
- Internal reconciliation republishes the zone without the deleted name.
- Each applicable projection must confirm the deletion independently.
- The tombstone remains while a projection is pending or failed.
- After all applicable projections confirm deletion, the aggregate remains as
  archived/auditable metadata until an explicit purge policy is implemented.
- An unmanaged conflict or missing provider access must not cause deletion of
  unrelated objects.

## 21. Inventory Links

A DNS-name aggregate may link to one or more:

- Compute resources.
- Containers.
- IP assignments.

Links are descriptive and aid navigation and value selection. They do not
control record values or trigger reconciliation. Archiving a linked inventory
resource does not silently remove DNS; UI and API surface the archived link.

## 22. API Requirements

All paths below are under `/api/v1`. Exact request/response schemas are
normative in the OpenAPI document created during implementation.

### 22.1 Zones

- `GET /dns/zones`: list discovered and managed zones.
- `POST /dns/zones/discover`: refresh visible Cloudflare zones.
- `POST /dns/zones/{zone_id}/select`: select and link a managed zone.
- `POST /dns/zones/{zone_id}/disable`: stop accepting new desired changes
  without deleting existing provider state.
- `GET /dns/zones/{zone_id}/status`: return provider and publication status.

### 22.2 Import

- `GET /dns/zones/{zone_id}/import-candidates`: list grouped supported and
  rejected provider records.
- `POST /dns/zones/{zone_id}/imports/preview`: preview selected imports.
- `POST /dns/zones/{zone_id}/imports/apply`: adopt selected records.

### 22.3 Names and Drafts

- `GET /dns/names`: list and search DNS-name aggregates.
- `GET /dns/names/{name_id}`: read desired state and both projections.
- `POST /dns/drafts`: create a create/update/delete draft.
- `GET /dns/drafts/{draft_id}`: read draft and latest preview.
- `PUT /dns/drafts/{draft_id}`: replace proposed draft content.
- `POST /dns/drafts/{draft_id}/preview`: validate and calculate provider diffs.
- `POST /dns/drafts/{draft_id}/apply`: commit desired state and enqueue work.
- `DELETE /dns/drafts/{draft_id}`: discard an unapplied draft.

### 22.4 Reconciliation and Audit

- `GET /dns/names/{name_id}/projections`: read independent provider states.
- `POST /dns/names/{name_id}/reconcile`: enqueue current desired revision.
- `POST /dns/zones/{zone_id}/reconcile`: repair all managed names in a zone.
- `GET /dns/names/{name_id}/audit-events`: read DNS-specific audit history.

### 22.5 API Safety

- Writes require DNS-specific personal token scope or administrator session.
- Draft apply and import apply require idempotency keys.
- Updates use aggregate revisions and `If-Match` where appropriate.
- Provider synchronization never runs in the request transaction.
- Errors use RFC 7807 and stable codes for validation, conflict, stale preview,
  unmanaged conflict, provider unavailable, and publication failure.

## 23. Frontend Requirements

### 23.1 Zone Workflow

- Show available, selected, degraded, and disabled zones.
- Discover zones on demand.
- Create or link matching domain inventory during selection.
- Show Cloudflare connectivity without exposing token details.
- Show internal publication revision and age.

### 23.2 Record List

- Search by name and zone.
- Filter by public/internal presence, type, wildcard, proxy state, and
  synchronization status.
- Group public and internal state under one DNS name.
- Show pending, synchronized, failed, deleting, unmanaged-conflict, and drift
  indicators.

### 23.3 Editor

- Edit public and internal record sets side by side.
- Allow A and AAAA sets together.
- Enforce CNAME exclusivity interactively and on the server.
- Support multiple A/AAAA values.
- Support explicit wildcard owner names.
- Offer inventory IPs as selectable suggestions without automatic binding.
- Make public/private address validation clear before preview.
- Require preview before enabling apply.

### 23.4 Preview and Status

- Display creates, changes, deletions, and unchanged values independently for
  Cloudflare and CoreDNS.
- Show unmanaged conflicts and validation errors.
- Show the revision being applied.
- After apply, poll projection status until synchronized or failed.
- Permit manual retry of current desired state.
- Do not imply transactional atomicity across providers.

## 24. Security

- All business endpoints require authentication.
- Authorization distinguishes DNS read and DNS write token scopes.
- Cloudflare token content is available only to server-side adapter code.
- Generated filenames derive from selected zone identifiers, not unchecked
  user path input.
- File publication never executes hooks or commands from DNS data.
- DNS values are parsed and normalized before storage.
- Provider errors and headers are sanitized before persistence or display.
- Audit representations redact secrets and irrelevant provider metadata.
- Preview output is treated as untrusted display data by the frontend.

## 25. Availability and Recovery

- CoreDNS continues serving the last valid files if Portinaia or SQLite is
  unavailable.
- Cloudflare continues serving its last synchronized records if Portinaia is
  unavailable.
- Reconciliation state survives process restarts.
- Pending work is repairable from desired state.
- A recovery command can enqueue all applicable current desired revisions.
- Restoring SQLite does not automatically import unknown Cloudflare records.
- Internal publication can be rebuilt deterministically from desired state.
- A failed rebuild preserves existing valid files until replacement succeeds.

## 26. Observability

Metrics include:

- Managed zones and DNS names by view and type.
- Draft validation failures.
- Pending, synchronized, failed, and deleting projections by provider.
- Reconciliation duration, attempts, retries, and queue age.
- Cloudflare errors and rate-limit responses.
- Drift detections and repair results.
- Import candidates and import failures.
- Internal publication duration, revision, checksum, and age.
- Zone generation and validation failures.

Structured logs include correlation ID, zone, aggregate ID, provider,
revision, operation, attempt, and sanitized result.

Readiness must not fail solely because Cloudflare is unavailable. A missing or
unwritable configured internal publication directory is a readiness failure
when internal DNS publication is enabled.

## 27. Testing Requirements

### 27.1 Validation

- Name normalization, label limits, apex, and wildcard cases.
- A, AAAA, and CNAME values.
- Public special-use address rejection.
- Internal private address acceptance.
- CNAME conflicts and cycles.
- TTL and proxy rules.

### 27.2 Draft and Persistence

- Create, update, and delete previews.
- Stale revision conflict.
- Stale preview conflict.
- Idempotent apply.
- Transaction rollback including audit and jobs.
- Tombstone lifecycle.

### 27.3 Cloudflare

- Zone discovery and explicit selection.
- Selective import grouping.
- Unmanaged conflict handling.
- Create, update, delete, and no-op reconciliation.
- Multiple value mapping.
- Rate limiting and transient retries.
- Ambiguous provider lookup rejection.
- External modification and deletion repair.
- Provider normalization without drift loops.

### 27.4 Internal Publication

- Deterministic complete zone generation.
- A, AAAA, CNAME, wildcard, and split-horizon fixtures.
- SOA serial advancement.
- Parse validation before publication.
- Temporary write and atomic rename.
- Last-valid-file preservation for every failure stage.
- Concurrent updates converge to the newest desired revision.
- CoreDNS integration test proving internal answers and public fallthrough.

### 27.5 Failure Isolation

- Cloudflare failure with successful internal publication.
- Internal failure with successful Cloudflare reconciliation.
- Process restart during pending work.
- Database restoration and full repair reconciliation.

## 28. Acceptance Criteria

- Multiple Cloudflare zones can be discovered and explicitly selected.
- Existing supported records can be previewed and selectively imported.
- Unselected records remain untouched.
- One name can contain public and internal record sets with different values or
  types.
- A and AAAA values can contain multiple addresses.
- Valid wildcard records work in public and internal views.
- Public non-routable addresses are rejected.
- A change cannot alter desired state without preview and explicit apply.
- Stale drafts cannot overwrite newer desired revisions.
- Cloudflare and CoreDNS reconcile independently and idempotently.
- External drift on owned Cloudflare records is detected, audited, and
  repaired.
- CoreDNS receives complete validated zone files through atomic replacement.
- A publication failure leaves the previous valid zone available.
- Internal clients receive internal answers.
- Names without internal overrides fall through and resolve publicly.
- Public clients receive Cloudflare answers.
- Deletions remain tombstoned until every applicable provider confirms them.
- No credentials or sensitive provider metadata appear in API responses,
  audit events, or logs.

## 29. Future Extensions

The architecture may later support:

- Additional record types with type-specific schemas and validation.
- Multiple Cloudflare accounts.
- Private internal-only zones.
- Multi-record change sets.
- etcd or PowerDNS internal adapters.
- Consul integration for ephemeral service discovery.

These extensions must not weaken explicit ownership, preview/apply safety, or
independent provider reconciliation.
