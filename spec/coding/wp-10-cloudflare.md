# WP-10 — Cloudflare adapter, import and reconciliation

Dependencies: WP-09. Sources: DNS §§6, 15–18, 22, 24–27;
implementation plan §§15.3–15.4, 15.6, 21/WP-10. Read [shared rules](coding-rules.md).

Primary paths: `internal/dns/cloudflare/`; coordinated DNS service/worker wiring,
zone/import handlers, projection/mapping SQL. Baseline: no adapter or worker;
`queries/dns.sql` has useful but incomplete lease/scheduling functions.

## WP-10.1 — Adapter and zone lifecycle

1. Narrow domain-owned interface: list visible zones; list records including
   unsupported types for conflict detection; exact object lookup; create/update/
   delete. Keep provider SDK/HTTP response types private to adapter.
2. Read token from configured mounted file; no DB storage/API exposure. Use pinned
   HTTP API version/endpoint, bounded timeouts/pages/response sizes and shared
   account-level rate limiter. Redact errors/headers before persistence/logging.
3. Discover all pages read-only; upsert local observations with stable IDs and audit
   meaningful state changes. Do not mark unseen zones missing on an incomplete scan.
   Confirmed loss of visibility degrades selected zones, preserving desired state.
4. Select explicitly with zone revision and existing matching normalized domain.
   UI may create that domain first, then select; do not create arbitrary inventory
   through zone discovery. Disable blocks new desired edits but keeps repair serving
   old desired state; reselect enables. Neither operation deletes provider records.

## WP-10.2 — Selective import

1. Fetch bounded provider observations, group same owner/type/TTL/proxy into
   compatible sets, and produce importable or reasoned rejected candidates. Include
   malformed, unsupported, invalid public IP, apex CNAME, inconsistent TTL/proxy,
   ambiguous duplicate, already-owned and conflicting combinations.
2. Candidate IDs and cursors bind to one observation fingerprint; pagination cannot
   accidentally mix independent refreshes. Expired refresh requires preview again.
3. Selected candidate sets form complete proposed name aggregates. Validate CNAME
   exclusivity/uniqueness across selected and existing desired state. No implicit
   adoption of unselected sibling objects. Limit selection/preview size as WP-02
   defines; never let hidden truncated actions be applied.
4. Preview binds exact selected provider object IDs and semantic values. Apply
   requires matching observation/preview, idempotency and current zone revision;
   refresh/reject stale observations before short commit transaction.
5. Commit desired names/sets/values, exact owned mappings, synchronized public
   projections, draft/import audit and replay result atomically. Public-only import
   leaves internal not applicable. Do not rewrite equivalent imported records.

## WP-10.3 — Public reconciliation and ownership-safe retry

Worker sequence: claim bounded Cloudflare-only due rows -> read desired/mappings ->
read provider state -> compute semantic plan -> execute owned operations -> persist
mapping/outcome under lease fencing -> complete if current revision still matches.
Each network call is outside DB write transactions. Independent internal work must
progress during Cloudflare rate limits or failure. Do not hold global process locks.

Important cases:

- Equal semantic normalized state is a no-op; update observed metadata.
- Map each A/AAAA value separately. Update fields on owned object, create missing
  values, delete obsolete mapped values; preserve mapping intent until deletion
  success, even if desired value rows are replaced.
- Preserve ownership across uncertain writes with durable operation intent and a
  server-owned provider correlation marker where supported. Exact lookup alone
  is insufficient to adopt an identical unmanaged record. Recovery must require
  evidence of prior ownership/our write intent, normalized exact fields, and one
  unambiguous result. Otherwise fail visibly for explicit user import/cleanup.
- Timeout after create must not blindly create a duplicate; timeout after delete
  retries by mapped ID and treats verified absence as success. Missing object for
  active desired state is recreated only after unmanaged-conflict check.
- Never modify unknown records, including identical-looking duplicates. CNAME
  transformations may need delete-before-create; show this in preview and converge
  safely after interruption, with no promise of public multi-record atomicity.
- Respect Cloudflare proxy TTL normalization: store desired TTL separately from
  effective provider fields. Model Auto/provider equivalents in adapter comparisons;
  unsupported explicit combinations fail validation, not endless drift repair.
- Before each external operation verify lease/current revision and serialize work
  per name. A request already in flight cannot be undone by DB fencing; retain its
  ownership mapping/intent and schedule newest revision to converge after return.
  Stale completion must never report the newer revision synchronized.

Retry transient network/5xx/429 with bounded exponential full jitter and provider
Retry-After. Permanent permission/validation/unmanaged-conflict errors remain failed
until relevant desired change or explicit retry. Add retryability to query eligibility;
NULL next_retry must not accidentally mean a permanent error is always due.

## WP-10.4 — Drift and repair

Periodic bounded scans compare mapped owned records to normalized expected state.
Equivalent state is no-op; changed/deleted owned state creates correlated audit/drift
event and enqueues same current revision for repair. Unexpected conflicting unknown
record blocks repair. Never import unknown state during drift or restore recovery.
Repair scans also find projections missing/behind current desired state. Manual
name/zone/global repair uses same queue path and preserves unchanged desired revision.

Metrics: projections by state/provider, attempt/duration/retry/age, error classes,
rate-limit wait, import outcomes and drift/repair counts. Logs contain safe resource
IDs/revision/attempt, not provider auth or raw response headers.

## Acceptance / tests

- CF-01: Fake HTTP server tests token handling, pagination, incomplete discovery,
  visibility loss, selection/disable and domain matching.
- CF-02: Selective import preserves all unselected provider bytes; malformed/
  unsupported candidates remain explainable; stale selection cannot apply.
- CF-03: Create/update/delete/no-op/multi-value/CNAME transitions, timeout after
  successful write, ambiguous lookup, duplicate prevention and permanent failure.
- CF-04: Fake clock tests lease expiry, retry hints, restart, same-revision repair,
  stale in-flight completion and newest desired convergence.
- CF-05: Drift modification/deletion repairs owned state; unmanaged conflict is
  visible and untouched; semantic TTL/proxy normalization does not loop.
- CF-06: Cloudflare outage does not stop internal publication/readiness/unrelated
  work; secret marker absent from problems, audit, logs and DB.

Commands: `go test ./internal/dns/... ./internal/database/... ./internal/api/...`,
`make verify`; WP-16 fake-provider integration. No live Cloudflare edits in automated tests.
