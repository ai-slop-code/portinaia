# WP-09 — DNS domain, drafts and desired-state apply

Dependencies: WP-05 and DNS WP-02 contracts. Sources: DNS §§7–17, 20–22, 24;
implementation plan §15.1–15.3, 21/WP-09. Read [shared rules](coding-rules.md).

Primary paths: `internal/dns/`, DNS handlers/SQL/domain tests. Adapter interfaces
are defined here; implementations belong to WP-10/WP-11. Baseline: tables/API
and projection query primitives only; validators/drafts/services absent.

## WP-09.1 — Normalize and validate one complete DNS name

Implement pure validators with field-addressable error codes and deterministic
canonical ordering. Name/zone are immutable after creation; full proposed state
includes description, typed inventory links and independent public/internal sets.

| Rule             | Required behavior / test                                                                                                                            |
| ---------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| Zone             | Selected managed public zone only; no home.arpa/private suffix; disabled rejects new desired changes                                                |
| Owner            | IDNA/lowercase ASCII, relative to zone or absolute, normalize terminal dot; exact zone-boundary suffix check; apex represented by zone FQDN         |
| Wildcard         | Only one leading `*.` label; flag/name agree; wildcard apex form is distinct from apex; exact/wildcard aggregates coexist                           |
| DNS limits       | Valid labels and full wire-name length; reject empty interior labels, invalid syntax and wildcard targets                                           |
| RRsets           | At most one A and one AAAA, or one CNAME, per view; at least one value; at least one nonempty view                                                  |
| Values           | Canonical IPv4/IPv6 by family; normalized uniqueness; CNAME exactly one absolute target                                                             |
| TTL/proxy        | TTL inclusive 60–86400, default 60; proxy false by default and public-only; adapter capability validation                                           |
| Public address   | Reject private/ULA, loopback, link-local, unspecified, multicast, documentation, benchmarking, CGNAT and other non-global special-use addresses     |
| Internal address | Allow private/ULA, meaningful link-local and global unicast; reject loopback/unspecified/multicast/documentation; no deployment CIDR allowlist      |
| CNAME            | Reject apex, self-target and detectable owned cycles in each effective view; external/unresolved target allowed                                     |
| Links            | Existing compute/container/IP, duplicate typed link rejected; archived existing links displayed; values are copied suggestions, never live bindings |

Use a versioned explicit special-use prefix policy with authoritative registry
source/date and tests; `IsGlobalUnicast` alone wrongly admits private/documentation
ranges. For internal CNAME traversal, include public fallthrough targets where
effective internal resolution reaches them. Do not use live resolver answers to
decide if an external CNAME target exists. Never resolve an unbounded external chain.

## WP-09.2 — Draft persistence and preview

1. `POST /dns/drafts` supports create/update/delete. Update/delete capture target
   and base name revision; creates have no target/base. One active target/actor
   draft, with its own revision and content hash. Drafts are never reconciled.
2. GET/PUT/discard require proper actor/scope/revision. PUT replaces complete
   proposed state, increments draft revision and invalidates previous preview.
   Applied/discarded drafts cannot be modified/applied again with a new key.
3. Preview validates canonical state, reads current desired state, refreshes
   required provider observations outside write transaction through read-only
   adapter and compares actual public/internal projection. Public conflicts include
   unsupported unmanaged record types that conflict with CNAME, not just A/AAAA.
4. Fingerprint binds draft revision/content, base desired revision, zone selection,
   relevant owned and unmanaged observation checksums and internal generation.
   Persist preview only if draft still matches when the short transaction begins.
5. Return exact create/update/delete/unchanged actions per provider, issues,
   conflicts, base revision and observed change indicator. Preview can update
   draft/observation metadata and audit, never desired or provider state.
6. Provider observation unavailable blocks a public-affecting apply. An internal-only
   change whose public desired state is unchanged need not call Cloudflare; encode
   observation applicability explicitly instead of fabricating provider timestamps.

## WP-09.3 — Explicit apply and durable projections

1. Require DNS write, draft ETag, expected base revision (zero for create), matching
   successful preview fingerprint and mandatory Idempotency-Key.
2. Read/normalize required provider observations outside DB transaction and compare
   semantic fingerprint. Inside transaction recheck actor access, draft revision,
   name base revision, zone enabled state, validation, graph cycle/conflicts, and
   local fingerprint. Stale content/base/observation returns 409/412 as declared.
3. Transaction commits desired aggregate/tombstone, next name revision, projection
   scheduling, zone generation increment for internal changes, audit, applied draft
   marker and non-secret idempotency response. Return 202 accepted, not synchronized.
4. Retry same key/equivalent request returns original acceptance even after draft
   is marked applied; changed payload/key fingerprint returns conflict.
5. Keep a projection applicable when old provider state still needs removal even
   if new view is empty. After confirmed cleanup it becomes not_applicable. Do not
   mark synchronized just because no new values exist.
6. Desired changes reset work to newest revision and invalidate old lease ownership.
   Workers complete by claimed revision/owner/fence only. Define separate commands
   for new-revision schedule, same-revision repair and retry; current
   `ScheduleDNSProjection` intentionally rejects equal revisions and cannot do all.

## WP-09.4 — Deletion, reads and manual repair

Delete is a previewed draft. Apply retains a tombstone with enough owned provider
mapping for deletion and schedules each applicable cleanup independently. Keep
deleting/failed until both confirm; then mark archived/auditable metadata. Name
uniqueness remains reserved until deletion completes; recreating afterward is a
new aggregate, not reassignment of old mappings. Do not implement DNS metadata
purge in MVP. Inventory links never drive deletion and can block inventory purge;
users should detach links explicitly before final DNS deletion if needed.

List/detail support zone/name/view/type/wildcard/proxy/projection filters and typed
inventory target lookup. Projection response exposes separate applied/desired
revision, attempt/retry/error/drift state. Name/zone reconcile enqueues current
desired work, does not import provider state or invent a new desired revision.

## Acceptance / tests

- DNS-01: Full table-driven name/address/RRset/CNAME/TTL/proxy/link matrix above,
  including equivalent IPv6 duplicate values and cross-view CNAME chains.
- DNS-02: Preview leaves desired/provider unchanged; stale draft/base/provider
  and unmanaged conflicts cannot apply; internal-only preview works during outage.
- DNS-03: Inject audit/schedule/idempotency failure and prove total rollback;
  concurrent duplicate apply yields one revision/effect/event.
- DNS-04: Old lease cannot complete new desired work; view removal and tombstone
  require actual provider cleanup; same-revision retry is supported.
- DNS-05: Inventory edits/archive never change desired DNS values or enqueue work.

Commands: `go test ./internal/dns/... ./internal/database/... ./internal/api/...`,
`make verify`. No bulk multi-name edits, automatic DNS or direct-record CRUD bypass.
