# WP-04 — Authentication and audit

Dependencies: WP-03. Sources: implementation plan §§9–10, 21/WP-04;
product §§4, 14; DNS §24. Read [shared rules](coding-rules.md).

Primary paths: `internal/auth/`, `internal/audit/`, auth/audit handlers and SQL,
security middleware; coordinated `cmd/portinaia/` and server composition.
Baseline: tables, administrator SQL and declared endpoints only.

## WP-04.1 — Administrator and password commands

1. Implement `admin create` and `admin reset-password`; accept password through
   a no-echo interactive prompt or explicitly documented stdin, not argv/logs.
   Exactly one enabled administrator is enforced by service and existing index.
2. Use versioned Argon2id encoded hashes: initial memory 64 MiB, iterations 3,
   parallelism 1, random salt 16 bytes, output 32 bytes. Benchmark on target arm64;
   adjust centrally with evidence before release, never per-handler parameters.
3. Constant-time verification, bounded encoded parameter parsing, and upgrade
   after successful login when configured parameters change. Avoid account-existence
   timing leaks with equivalent verification work for unknown usernames.
4. Password reset has an explicit revoke-sessions option. Audit creation/reset
   without password/hash/parameters in before/after documents. No setup web page.

## WP-04.2 — Sessions and CSRF

`POST /auth/login`, `POST /auth/logout`, `GET /auth/session`:

1. Generate 32 random credential bytes; store SHA-256 digest, not credential.
   Cookie `portinaia_session`: HttpOnly, Path `/`, SameSite Strict, Secure when
   configured public URL is HTTPS, expiry matching server-side 30-day lifetime.
2. Derive CSRF bytes via HMAC-SHA-256 keyed by the random session credential over
   a versioned purpose string plus session ID; return base64url in session response.
   Store its digest in existing `csrf_hash`. A later authenticated session request
   can derive the same token from its presented cookie without plaintext storage.
   Compare digests in constant time; use no CSRF credential as an auth substitute.
3. Enforce session expiry/revocation and enabled admin on every request. Throttle
   last-seen updates to at most once per minute; logout revokes and clears cookie.
4. Rate-limit login using bounded source/account buckets with eviction and global
   Argon2 concurrency cap. Return generic invalid credentials and Retry-After on
   limits; never create unbounded SQLite rows from unauthenticated attempts.
5. Verify same-origin login/logout handling; cookie mutations require CSRF.
   Authentication responses including failures are no-store.

## WP-04.3 — Personal and machine credential primitives

PAT endpoints list/create/revoke with exact declared `admin` scope. Token format
uses distinct versioned prefixes for PAT, enrollment, and agent credentials plus
at least 256 random bits; prefixes confer no authorization. Persist only SHA-256
digests of high-entropy credentials. Return plaintext once on successful issuance.
Names/scopes/optional future expiry are validated; revoked/expired credentials
fail immediately. Last-used writes are throttled. No generic idempotency response
storage for secret creation; user recovers a lost response by revoking/reissuing.

Central principal contains actor type/ID, optional administrator/agent/compute
binding, exact scope set, auth method and request ID. Browser admin satisfies all
scopes. Agent/enrollment credentials cannot enter human routes; PAT/session cannot
impersonate an agent. Reject ambiguous multiple auth credentials rather than
silently picking one. WP-06 owns enrollment consumption and generation changes.

## WP-04.4 — Transactional audit and audit API

1. Provide an explicit transaction-bound append function used by every service.
   Input: actor, action, resource, request ID, result, redacted before/after.
   Store append-only audit with monotonic sequence and UUIDv7; never resource FK
   cascade. Use a fixed persisted UUIDv7 system actor for CLI/worker activity and
   fresh correlation ID where no HTTP request exists.
2. Redaction is allowlist-based per resource before serialization. Exclude secret
   material, hashes, auth headers, raw provider payloads, sensitive file paths and
   credential-response bodies. Preserve descriptive changes useful for diagnosis.
3. `GET /audit-events`: cursor ordered occurred_at/sequence descending; filter
   time, actor type/ID, action, resource type/ID. Return bounded field differences
   with explicit redaction markers; describe any truncation rather than leaking
   arbitrary raw JSON. Coordinate API changes if bounded output needs a marker.
4. Audit human writes, credential lifecycle, reports/heartbeat state writes,
   automatic reactivation, freshness transitions, draft/import/apply, provider
   outcome/drift, repair and purge. Operational no-ops/replays create no duplicate
   audit event. Report audit is a compact summary, not the snapshot. Throttled
   heartbeat timestamp writes use compact audit; this volume counts in WP-16.
5. An audit failure rolls back a business mutation. Auth failures are safe
   operational logs; authenticated rejected changes may append a rejection after
   the attempted mutation transaction has rolled back. Snapshot retention never
   prunes audit. Technical lease bookkeeping is operational telemetry, not a new
   user-visible business mutation.

## Acceptance / tests

- SEC-01: Argon2 correct/wrong/malformed hashes, parameter upgrade and measured
  arm64 behavior; concurrent admin creation cannot enable two administrators.
- SEC-02: Login -> session bootstrap -> CSRF mutation -> logout -> denial, with
  cookie flags, expiry, reset revocation, origin and limiter tests.
- SEC-03: Full credential-class/route and exact-scope matrix; expired/revoked
  credentials, cross-agent requests, ambiguous headers and cache headers tested.
- SEC-04: Secret markers absent from SQLite (including idempotency/audit), logs,
  list responses, problems and snapshots; plaintext exists only at issuance.
- SEC-05: Inject audit insert failure and prove mutation rollback. Pagination
  remains stable for same-second audit events; resource purge preserves audit.

Commands: `go test ./internal/auth/... ./internal/audit/... ./internal/api/... ./internal/database/...`,
`make verify`. No external identity, multi-user roles, MFA or token-in-browser-storage.
