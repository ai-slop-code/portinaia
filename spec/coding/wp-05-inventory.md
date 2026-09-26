# WP-05 — Inventory backend

Dependencies: WP-04. Sources: product §§7–9, 12; implementation plan §§7.3,
8.3, 11, 21/WP-05. Read [shared rules](coding-rules.md) and D-02/D-03/D-06.

Primary paths: `internal/inventory/`, inventory handlers, resource-specific SQL
files and tests. Coordinate OpenAPI/migrations through WP-02 owner.
Baseline: entity tables and minimal provider queries, no services or wired routes.

## Common slice procedure

For each resource below: implement domain validation/models -> sqlc queries ->
transactional service -> explicit transport mapper -> generated handler wiring ->
real-DB/HTTP tests. Create starts revision 1; manual updates increment once. Read,
list, search, archive/restore/purge follow actual reconciled OpenAPI operations.
Never expose generic arbitrary-field updates. Exclude archived by default and
support exclude/include/only. Validate referenced resources in the mutation
transaction; retain existing archived links for display but reject new links to
archived targets. Metadata edits never create DNS jobs.

## WP-05.1 — Providers and roles

- Name nonblank within API limits; optional HTTP(S) website/account reference,
  description/notes/tags. No credentials/billing secrets.
- Require at least one distinct approved role; atomically replace role/tag joins.
  Map `hosting_provider` to SQL `hosting`. Registrar references require registrar
  role; block removing that role while referenced by active registered domains.
- Lists search name and filter archive/role as documented; stable name/ID ordering.
- Tests: duplicate case-insensitive name, empty/duplicate roles, missing tag,
  role removal with active dependency and transactional join/audit rollback.

## WP-05.2 — Locations

- Site/provider_region, name, optional provider/parent, description/notes/tags.
  Do not require a provider where the product says optional.
- Parent must be a location; prevent self/indirect cycles using recursive query
  in the same writer transaction. Do not infer location from IP or agent facts.
- Parent changes increment only edited aggregate; hierarchy reads reflect it.
- Tests: deep hierarchy, self/cross cycle including concurrent moves, archived
  parent display, invalid reference, provider filtering and cursor ties.

## WP-05.3 — Computes and VM hierarchy

- Physical/vps/vm; owned/rented/other; manually controlled display name, provider
  identifiers, location, parent, notes/tags. Provider resource ID requires provider.
- Only VM children can have compute parents; a VM may parent another VM if no
  cycle. A location and parent are distinct metadata; tree follows parent first
  to avoid duplicating a VM under both. Do not infer VM parents through discovery.
- Response separates observation and agent status from manual fields. Concurrent
  discovery does not overwrite display name or force unrelated metadata conflicts.
- Tests: type change with existing parent, cyclic move, provider identifier
  uniqueness, unobserved compute, and observation/manual field separation.

## WP-05.4 — Containers

- List/detail and manual description/notes/tags/lifecycle only. No human create
  or runtime-field editing; unknown/discovery properties in writes are rejected.
- Identify via compute + verified Podman owner UID + exact name. Runtime ID is
  an observation, never primary identity. WP-06 owns creation and replacement of
  current ports/networks/addresses/mounts/labels/restart fields.
- Detail joins child facts deterministically; lists use bounded payloads. History
  and DNS links are separate paginated reads, not unbounded nested collections.
- Tests: forged runtime edits, read unknown runtime state, archive metadata
  preservation and resource identity across recreation (integrated with WP-06).

## WP-05.5 — IP assignments

- Exactly one compute/container owner; parse via netip, canonicalize, validate
  address-family prefix. No routability restriction for inventory: DNS has its
  own stricter rules. Unknown prefix stays NULL.
- Human creates source manual; human updates to discovery-owned assignments are
  rejected as declared. Never match-and-overwrite a manual row during ingestion.
- Map combined interface/network input to compute interface or container network.
  Use approved role enum and separate primary designation. Enforce at most one
  active manually selected primary per owner and family with transactional checks;
  auto-discovery does not demote a manual primary.
- Preserve first/last/stale times only for discovered values. Document discovery
  identity as owner + collector scope + interface/network + canonical address +
  prefix so separate networks and manual/discovered duplicates can coexist.
- Tests: zero/two owners, IPv4/IPv6 boundaries, duplicate/manual coexistence,
  primary conflict, source forgery, archived owner and cursor/filter cases.

## WP-05.6 — Domains and expiry

- Normalize IDNA ASCII lowercase without trailing dot; validate DNS label/length
  limits, no wildcard. Registration/expiry are date-only; auto-renew is true,
  false or unknown. Normalize/deduplicate nameservers.
- Optional registrar must have registrar role. Managed zone link must match name.
  Prevent renaming a domain away from its linked zone's normalized name.
- Expiry states: unknown if no date; expired if date < UTC today; expiring if
  today <= date <= today + configured days; otherwise current. Auto-renew does
  not suppress warning. `/domains/expiring` and dashboard/metrics use same function.
- Tests: IDN/trailing-dot normalization, invalid dates, threshold edges, NULL
  expiry/auto-renew, archived exclusion and mismatched managed-zone link.

## WP-05.7 — Costs

- Exactly one compute/domain owner. Amount is nonnegative integer minor units;
  server derives exponent from pinned ISO 4217 data, not client input. Validate
  unsupported currency, overflow, excess input precision, and date order.
- Recurring uses P1M/P3M/P6M/P1Y; map explicitly to SQL enum, requires effective
  start, optional inclusive end. One-time requires charged_on, no recurring dates.
- Multiple entries may overlap; do not replace older costs automatically.
  Active totals follow D-02: rational monthly equivalents summed before half-up
  rounding per currency. Never mix currencies or fetch FX rates.
- Tests: zero/two owners, JPY/EUR/three- and four-decimal supported currency,
  overflow, quarterly rounding across multiple entries, effective-date boundaries,
  archive exclusion and one-time exclusion. Verify API/SQL round trips.

## WP-05.8 — Tags, descriptions and notes

- Tag name is the API's normalized slug; display name is separate, with reconciled
  100-character limits. Enforce uniqueness and revision-checked updates.
- Supported joins: providers, locations, computes, containers, domains. Replace
  tag IDs in the owning resource's transaction and audit that resource revision.
- Tags have archive/restore; existing assignments remain visible as archived.
  No new assignments to archived tags. Purge requires no assignments.
- Store/render plain text notes within bounds; no custom fields/schema or HTML.
- Tests: normalization collision, nonexistent/archived assignment, rename across
  resources without reassigning IDs, stale tag revision and purge dependencies.

## WP-05.9 — Archive, restore and explicit purge

1. Archive preserves relationships and data; never recursively archive children.
   Active children retain navigable archived ancestors. No automatic archival.
2. Compute archive additionally revokes its agent and all outstanding enrollment
   tokens in the same transaction with correlated audit events (D-03).
3. Restore validates identity/reference constraints, increments revision and audits;
   restores no credentials. Archived container rediscovery is WP-06's automatic
   restore exception; do not replace its UUID.
4. Purge requires admin + inventory write, current revision, explicit confirmation,
   archived target and no blocking dependencies including archived resources or
   retained reports. Return bounded typed blockers rather than raw FK errors.
5. Target-owned facts/joins and expired/revoked credential metadata may be cleaned
   only after history/dependent checks. Agent rows can be removed as compute-owned
   metadata once no retained reports reference them. Audit remains indefinitely.
6. DNS links block purge even when related DNS metadata is archived; remove links
   through an explicit approved DNS workflow before purge. Do not cascade DNS.

Tests: stale lifecycle ETag, active/archived child blockers, retained history
blocker, no history loss, compute archive/report race, restore/re-enroll, rollback
of credential revocation on audit failure and audit persistence after purge.

## WP-05.10 — Aggregate reads and dashboard

Implement `/inventory/tree`, compute summary/containers, domain expiry and dashboard.
Use bounded recursive CTEs, depth/node limits and deterministic children. Offer
continuation/filtered child access for truncated branches; never fetch all 10,000
containers merely to build a UI tree. Include archived ancestors as context, not
active counts. Orphan active nodes have an explicit root placement.

Dashboard returns server-side counts and bounded summaries for offline computes,
stale children, expiring domains, pending/failed DNS projections and monthly cost
totals. Counts match source filters and exclude archived resources. Define whether
DNS counts count projections (they do) and stale counts include containers plus
discovered IP assignments. Unenrolled/revoked computes are not silently offline.
DNS counts can be zero before DNS implementation but must query real tables.

## Package acceptance / commands

- INV-01: Every numbered resource slice passes its validation/list/lifecycle tests.
- INV-02: All mutations enforce revisions and audit atomicity; no discovery field
  is writable by a human transport schema/service.
- INV-03: Hierarchies and filters are bounded/indexed; EXPLAIN QUERY PLAN and
  deterministic target-sized fixtures exercise list/search/summary paths.
- INV-04: Dashboard source lists agree with counts and currency totals.

Run `go test ./internal/inventory/... ./internal/api/handlers/... ./internal/database/...`,
`make verify`. Do not introduce provider credentials, VM discovery, generic graphs,
automatic DNS changes, exchange rates or unrequested billing operations.
