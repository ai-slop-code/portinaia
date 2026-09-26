# WP-02 — OpenAPI and database reconciliation

Dependencies: WP-01. Sources: implementation plan §§7–8, 12.1, 14.1, 21/WP-02;
product §§7–12; DNS §§7–22. Read [shared rules](coding-rules.md) and every
C-01–C-21 entry in [the audit](implementation-audit.md).

Primary paths: `api/openapi.yaml`, `api/openapi_test.go`,
`internal/database/migrations/`, `queries/`, `schema_test.go`, `sqlc.yaml`.
Generated outputs only through `make generate`. This package owns shared contracts.
Baseline: 103 operations and 40 initial tables exist; neither count is a permanent
acceptance requirement. Replace brittle count assertions with required-contract
checks when legitimately extending the model.

## WP-02.1 — Complete transport contracts

1. Inventory every declared operation, request type, response type, scope,
   pagination/filter, revision and idempotency rule. Preserve current path names.
2. Implement D-02 interval enum and monthly-equivalent response semantics; make
   currency totals safe for JavaScript integers or change the contract to exact
   decimal-string totals if aggregate bounds would overflow. Never round silently.
3. Give tags actual revision/update/archive storage and lifecycle semantics;
   document tag deletion as administrative purge after detachment and archival.
   Add missing lifecycle operations rather than interpreting delete differently
   per resource. Give DNS drafts their own revisions, separate from name revisions.
4. Specify rejected report responses without required nonexistent content IDs;
   successful envelopes retain client version, both collection times, receipt,
   stable acceptance result and content reuse. Distinguish server history `id`
   from client `report_id`; snapshot PUT path uses the latter, history paths use
   the former. Explicitly expose both in history metadata.
5. Model host/interfaces/disks/filesystems collector success/failure explicitly,
   as Podman already does. Success carries the bounded complete result; failure
   carries sanitized error and no authoritative children. A report may contain
   independent failed collectors; do not fabricate host facts to satisfy a schema.
6. Separate shared canonical structural content from report-specific download
   envelope using WP-08. Add disk diff and volatile differences. Specify pagination
   or explicit bounded continuation for differences and container history.
7. Fix import candidate unions: rejected records can expose raw type/TTL summaries
   safely without pretending to be a supported set. Bound import selections so
   every change can be previewed, or paginate a persisted immutable preview.
8. Add navigation contracts: DNS link target filters; container report timeline
   lookup; lazy tree/child continuation; expiry-state and dashboard source filters.
   Keep arbitrary graph relationships out.
9. Remove generic idempotency from plaintext PAT/enrollment/rotation issuance.
   For these endpoints document lost-response recovery as revoke/reissue. Enroll
   remains single-use and cannot replay the issued credential from SQLite.
10. Require both inventory write and admin for permanent purge. Define bounded
    dependency conflict details and update scope tests. Align audit failure enum
    and system actor representation; use a fixed persisted UUIDv7 system actor ID.
11. Complete 400/401/403/404/409/412/413/422/429/503 responses where applicable,
    transport unknown-field behavior, date/null semantics, and health responses.
    Preserve ETag format and page size 50/max 200.

## WP-02.2 — Forward schema corrections

Create ordered migrations after `00001_initial.sql`; do not edit schema 1.
For rebuilds: create replacement, copy valid data with explicit mapping, verify
row counts/FKs, swap, restore indexes/triggers, update application schema version.

Required schema work:

- Tag and draft revisions/lifecycle metadata; API-compatible string limits.
- Current `compute_interfaces`, interface addresses, `compute_disks`, and
  `compute_filesystems`, with stable collector-scoped keys, compute FK, observed
  times and absence state; virtualization role on observations.
- Nullable type-specific cost dates, exponent through 4 where valid for supported
  ISO currencies; named interval constraints with explicit transport mapping.
- Podman owner UID representation, allowed unknown restart/mount types, optional
  unknown container address prefix, canonical enum mappings from the audit.
- Per-collector authoritative observation/absence tracking: record which success
  established absence; failures alone cannot set a missing-child state.
- Immutable report metadata, content reuse, semantic request fingerprint, volatile
  observations with bounded typed serialization, accepted-with-errors state.
  Retention references must not delete current facts or audit.
- Zone-wide internal desired generation, published generation/checksum/serial,
  publication lease/fencing state. Failure records need nullable output metadata
  when rendering failed before a file/serial/checksum existed.
- Explicit retryability or equivalent queue state so permanent failures are not
  claimed; same-revision repair and provider-specific worker queries.
- Any missing safe-recovery intent needed for Cloudflare uncertain creates;
  identifier lookup must not silently adopt an unmanaged exact match.
- Fix client-clock comparison constraints; use receipt for freshness and client
  time only for diagnostics. All stored timestamps use the shared UTC codec.

Review all tables against API enums, required/optional fields, maxima, uniqueness,
and ownership. Test mappings such as hosting_provider/hosting and deleted/archived.
Do not add empty generic JSON columns as replacements for normalized current facts.

## WP-02.3 — Query contracts and generation

Extend existing SQL files; use one file per feature where simpler. Establish
transaction-aware audit/idempotency helpers, bounded list queries and explicit
mutation predicates. Keep agent transport in a generation target that imports
neither chi nor server database code. Record generator ownership in the Makefile.
WP-06 can depend on WP-08's canonical contract without depending on its UI/API work.

## Acceptance / tests

- CON-01: Every C-01–C-21 item has a fix, tested mapping, or named later runtime
  owner; no unresolved schema/API contradiction blocks downstream services.
- CON-02: Test valid and invalid schema examples, runtime discriminated-union
  decoding, request/path equality, unknown properties, max lengths and enum maps.
- CON-03: Migrate empty DB and populated schema-1 fixtures; preserve rows, links,
  IDs, retained report timestamps and secrets-as-hashes; FK/integrity checks pass.
- CON-04: Read/write round trips can represent every approved field without
  truncation, fake defaults, or lossy conversions. Generated code compiles.
- CON-05: Generated client/server use the same revised contract; the running
  server is explicitly still incomplete until subsequent packages wire behavior.

Commands: `make generate`, `go test ./api/... ./internal/database/...`,
`make verify`. Add new snapshot/protocol schema fixtures to contract tests.
No provider calls, production services, or placeholder 200 business responses.
