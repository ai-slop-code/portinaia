# Backend and frontend requirements

Scope: active inventory, owned domains, authentication and audit only. These are
acceptance requirements, not a claim of completion. Status: [progress](parallel-progress.md).

## BE-01 — Operation contracts and compatibility

- OpenAPI describes implemented operations truthfully. Validate required-member
  presence separately from zero values; nullability, unknown fields, unions, bounds,
  trailing JSON, body limits, path/query/header inputs and references before effects.
  Generated decoding alone is not sufficient. Preserve authentication precedence.
- Validate complete actual responses, including errors, pages and lifecycle DTOs.
  Closed write schemas must not make composed response schemas invalid.
- Use new canonical UUIDv7 IDs; retain existing UUIDv4 IDs/references unchanged.
  No re-keying or database reset to satisfy a schema. Test populated upgrades.
- Lists default to 50, maximum 200 unless the operation explicitly differs. Bound
  cursors and bind them to collection/filter/sort/parent context. Every documented
  filter must work or be explicitly reconciled, never silently ignored.
- Strong `"revision-N"` ETags protect writes. Check affected rows and distinguish
  missing, stale, validation and dependency conflicts. Problems have stable codes,
  field errors where applicable and one consistent request/correlation ID.
- Implement idempotency where promised: actor + operation + key hash + canonical
  request fingerprint, including target and expected revision. Replay a prior
  success before checking the now-stale revision; changed body conflicts. Mutation,
  audit and non-secret replay result commit together. Default retention is 24 hours;
  handle expired-key replacement and bounded cleanup. No generic replay of secrets.

**Accept BE-01:** real handler request/response matrix, invalid/missing/zero/null
fields, pre-effect rejection, complete response validation, filters/cursors/ETags,
concurrent replay/expiry/conflict and legacy populated-data tests.

## BE-02 — One administrator and explicit credential boundaries

- Keep working bootstrap/reset commands, no setup web page. Password input is not
  argv/log output; documented environment input exists, and no-echo/stdin may be
  added without exposing values. Exactly one enabled administrator.
- Store versioned Argon2id hashes. Preserved target policy is 64 MiB, 3 iterations,
  parallelism 1, 16-byte random salt, 32-byte result; benchmark and centrally record
  changes before release. Current implementation parameters differ: do not confuse
  cleanup of docs with evidence approving those defaults. Upgrade after successful
  login when policy changes; bound encoded parameters.
- Equivalent unknown-user password work, bounded account/source buckets with eviction,
  bounded and cancellation-aware Argon2 admission, generic failures and Retry-After.
- Sessions use at least 32 random credential bytes, digest-only storage, HttpOnly,
  Path `/`, SameSite Strict and Secure based on configured public HTTPS. Preserved
  target lifetime is 30 days; current shorter behavior requires an explicit policy
  reconciliation rather than accidental drift. Expiry/revocation checked each request.
- Derivable purpose-versioned HMAC CSRF bound to session identity, stored as digest;
  same-origin login and CSRF on cookie mutations. Auth success/failure is no-store.
  Throttle last-seen writes. Logout must report failure if revocation fails. Password
  reset invalidates sessions and is audited.
- Separate human, agent and enrollment credentials; no ambiguous fallback. Enforce
  exact declared capabilities. A sole admin can have an explicit capability set;
  `admin` is not an implicit wildcard and PAT issuance remains deferred.

**Accept BE-02:** actual routes enforce class/capability/CSRF; concurrent admission,
unknown account, cancellation, expiry, revocation failure, reset, cookie/proxy and
secret-redaction cases. Do not accept helper-only tests as mounted behavior.

## BE-03 — Transactional audit and basic read view

- Required mutations, bootstrap/reset, credential changes and compact accepted
  discovery/reactivation events append in the mutation transaction. Audit failure
  rolls it back; replay does not append a second event. No per-heartbeat audit flood.
- Allowlist audited fields and redact secret canaries. Keyword filtering alone is
  insufficient. Canonical system actor is
  `0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73c0`; workers/CLI also receive correlation IDs.
- Preserve immutable old events with missing request IDs/noncanonical historical
  actor labels. Reconcile a truthful legacy read shape; do not fabricate history.
- Implement authenticated/capability-checked `GET /audit-events` and a basic browser
  view. Agree actual filters, bounded PageInfo/cursors and redacted changes in OpenAPI
  before wiring. Stable sequence ties ensure same-second events are not skipped.
  Audit survives resource lifecycle and report retention.

**Accept BE-03:** rollback, secret sentinels, legacy rows, same-second pagination,
filter/cursor mismatch, read authorization and real browser integration.

## BE-04 — Inventory integrity

- Provider names/roles are bounded; at least one approved role. Registrar links
  require registrar role; reject removal while actively referenced.
- Locations and computes form cycle-free hierarchies checked in the write transaction.
  Only VM children have compute parents. Parent-compute placement wins over location;
  counts include each active resource once. Keep unplaced resources accessible.
- All active descendants, including a container under entirely archived compute
  ancestry, remain reachable through contextual archived locations/hosts. Hide
  unrelated archived branches; do not promote children and lose their relationships.
- Manual and discovered fields are separate. Discovered IP facts cannot be edited
  as manual ones. Unknown prefix is null, not zero. Manual primary per owner/family
  is unique and not demoted by discovery. Inventory permits addresses DNS may reject.
- Archive is nonrecursive. Compute archival atomically revokes credentials/tokens;
  restore preserves identity/metadata without reviving credentials. No generic purge.
- Notes/description edits preserve existing tags and other nonedited metadata.
  Do not clear dormant features' data merely because their UI is deferred.

**Accept BE-04:** cycle/concurrent move, filters/paging/counts, both mixed archived
VM and container-only chains, manual/discovered coexistence, lifecycle/revocation,
stale updates, legacy relationships and rollback.

## BE-05 — Owned domains

Normalize IDNA/lowercase without trailing dot; reject wildcards and invalid labels.
Dates are optional calendar dates; renewal is true/false/unknown; normalize/deduplicate
nameservers. Optional registrar must have the role. A selected linked zone matches
the domain name; block rename away from it. Expiry is unknown, expired, expiring or
current by UTC date with the configured warning window (default 30 days); auto-renew
does not suppress warning. No DNS credentials are needed for domain inventory.

**Accept BE-05:** normalized duplicates, invalid/unknown dates, threshold edges,
optional registrar/renewal, archive/restore, matching-zone relationship and restart.

## BE-06 — Safe, integrated browser workflows

- Feature modules own typed adapters/hooks/forms; shell composes routes. Router
  owns URLs and Query owns server state. One generated client pipeline handles
  credentials/CSRF/problems/field errors/network failure and session invalidation.
- Query identity includes resource/filter/cursor/sort/session generation. Preserve
  caller abort signals, including those in a Request. Ignore/cancel obsolete results
  through JSON decode and state commit, not only arrival of response headers.
- Forms capture `{ID, original ETag, initial values}` as one edit snapshot; submit
  that snapshot. Selecting another resource or refreshing its revision cannot apply
  old edits to new identity/state. A 409/412 preserves edits and offers explicit
  discard/reload/re-preview. Do not silently retry with newer revisions.
- Lists and references are independently searchable/paged, with selected-ID hydration,
  existing archived-reference display and active-only new assignments. Pagination
  resets with filters and mutations invalidate affected detail/list/tree/count data.
- Loading, empty, stale retained, unavailable and unknown are distinct. Failed
  collector reads do not mean no reports; failed counts do not become zero.
- Secrets stay compute/session-bound, memory-only and outside query caches; clear on
  close/navigation/expiry and ignore late issuance responses. Explain uncertain
  issuance recovery. Confirm consequential lifecycle/revocation actions.
- Poll only visible diagnostics with cancellation/backoff. Render observation times
  and explicit freshness/absence; null prefixes never become `/null`. No inferred
  absence from transport failure. Keyboard/mobile and browser-back navigation work.

**Accept BE-06:** delayed body/selection/session races, original-ETag recovery,
tag preservation, references beyond page one, truthful failures, secret lifecycle,
audit paging, and actual process-restart workflows in isolated browser tests.
