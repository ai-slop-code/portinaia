# Shared coding rules

Applies to every package in [the index](README.md). Normative sources:
implementation plan §§2–4, 7–10, 16–19, 23; product §§8–9, 12, 14.

## Architecture and file ownership

1. Keep one server process. `cmd/portinaia` parses commands and delegates;
   `internal/server` composes dependencies/router/listeners/workers.
2. Request flow: generated transport -> handler mapping -> domain service ->
   sqlc queries -> SQLite. Map back explicitly. Domain models do not import
   `internal/api/generated`; generated SQL models do not escape handlers.
3. Services own validation, transaction, revision checks, audit, and idempotency.
   Handlers own HTTP headers/status, authentication context, transport validation,
   and mapping. SQL contains data-access logic, not HTTP/business error strings.
4. Use concrete structs; add narrow consumer-owned interfaces for clock,
   randomness, provider, filesystem, or transaction seams actually tested. No
   generic CRUD framework, ORM, service locator, global current user, or DI library.
5. Reuse `internal/server`, `internal/database`, `internal/version`, and
   `web/src/api/client.ts`. Introduce shared primitives in WP-03 once, not a
   separate pagination/revision/problem helper per feature.
6. Agent packages may share transport-only `agent/protocol` and version primitives.
   They must not import database, server handlers, or embedded frontend code.
7. Shared files need explicit coordinated ownership: OpenAPI, migration numbering,
   generated outputs, config, main composition, web routes/theme/client, Makefile.

## Go and database conventions

- Pin compatible dependencies; preserve pure-Go/CGO-disabled Linux amd64/arm64.
- Context first; propagate cancellation and bounded timeouts. Return errors;
  do not panic or call `os.Exit` inside services. Log an error once at its owner.
- Use UTC `time.Time` in domain code; one codec stores `YYYY-MM-DDTHH:MM:SSZ`.
  Dates use validated `YYYY-MM-DD`. Unknown is nil/NULL, not a fabricated zero.
- UUIDv7 lower-case canonical strings at boundaries. Validate version and variant
  in addition to parser success. Inject clock/ID source where deterministic tests
  need them; use cryptographic randomness for credentials, never math/rand.
- SQL belongs in `internal/database/queries/<feature>.sql`; regenerate with sqlc.
  Use `generated.New(tx)` or `WithTx`, never mix transaction and outer DB queries
  in one mutation. Parameterize values; allowlist sort columns.
- A service validates shape, begins a short write transaction, rereads referenced
  state, checks expected revision, writes rows/joins, writes redacted audit and
  safe idempotency result, commits, then returns. Any failure rolls everything back.
- Enforce revisions in SQL predicates and inspect affected row counts. Distinguish
  missing, archived, invalid, dependency/uniqueness conflict, and stale revision.
- Keep provider calls and filesystem publication outside SQLite write transactions.
  Refresh observations before a short apply transaction; recheck local fingerprints
  inside it. Remote races cannot be made atomic with SQLite; workers recheck safety.
- Preserve immutable migrations. Correct schema 1 using numbered forward migrations;
  never silently rebuild user data or weaken constraints to pass a test.
- Lists use indexed keyset pagination and a unique ID tie-breaker; fetch page size
  plus one. Test equal keys, NULL keys, both ends, filters, and archived-only mode.
- Cursor encoding is versioned base64url JSON with collection, allowlisted sort,
  normalized-filter fingerprint, and last-key tuple. Validate size/shape/collection;
  bind values as SQL parameters. Cursor opacity does not require secrecy.
- Do not count on SQL text comparisons to normalize IPs/domains. Use `net/netip`
  and one IDNA/DNS normalization package before persistence.

## HTTP contract

- OpenAPI 3.0.3 is the transport source. Keep operation IDs stable unless an
  approved reconciliation requires a change. Never hand-edit generated output.
- Install runtime request validation, including `oneOf`, unknown fields, bounds,
  and UUIDv7 patterns; generated Go decoding alone does not enforce the schema.
  Generated binding/strict-handler errors must use the same problem writer.
- Lists default to 50 and cap at 200 per current contract. Reject malformed
  cursors and out-of-range sizes. Never silently return only an unmarked prefix.
- Use strong ETags `"revision-N"`; require the current ETag on documented mutations.
  Missing/malformed required headers: 400; revision mismatch: 412; domain conflict
  or stale DNS preview: 409; semantic invalidity: 422; missing: 404; unauthenticated:
  401; scope/CSRF failure: 403; oversized body: 413; rate limit: 429; unavailable
  local dependency: 503. Reconcile endpoint responses in OpenAPI before use.
- All failures use `application/problem+json` with stable snake_case code and
  request ID. Do not leak raw SQL/provider errors. Add declared responses where
  an existing endpoint omits a failure it can actually return.
- Idempotency uses actor + operation + key hash and canonical request fingerprint,
  including target and expected revision. Same key/different request is 409.
  Read an existing successful result before rechecking a now-stale revision.
  Persist non-secret response metadata/body in the same transaction as mutation.
  Concurrent duplicates produce one effect and one audit event. Default replay
  retention is 24 hours; document expiry and bounded cleanup.
- Use `Cache-Control: no-store` for auth/secret responses. Plaintext credentials
  are returned only by issuance, not generic cached replay. No token in URLs.
- Authentication middleware explicitly separates human, agent, and enrollment
  routes. Enforce all `x-required-scopes`; tests iterate the operation matrix.
  Cookie mutations require `X-CSRF-Token`. Do not fall back from a bad bearer
  credential to a valid cookie or vice versa.

## Frontend conventions

- Feature files under `web/src/features/<feature>/`; shared shell under `app/`,
  route composition under `routes/`, reusable controls under `components/`.
  Keep component, query hooks, form mapping, and tests separate as they grow.
- Use generated `components["schemas"]` types and openapi-fetch path calls. A
  form-only type is acceptable for incomplete UI inputs (for example amount text);
  convert in one feature adapter. Do not duplicate API response interfaces.
- React Router owns routes/search parameters. TanStack Query owns server state;
  React Hook Form owns complex form state. No separate fetch framework/store.
- Centralize feature query keys; include ID, filters, cursor, and sort. Reset cursor
  when filters change; preserve deep links and browser back behavior.
- A shared API adapter handles CSRF, problem details, request IDs, and expiry.
  Store session/secret state in memory, not localStorage or URL/query persistence.
  Clear authenticated queries on logout/session expiry to avoid stale data flashes.
- Send the ETag captured when the form loaded. A 412/409 keeps edits, explains
  conflict, and offers explicit reload/re-preview. Never auto-overwrite or silently
  retry writes with a newer revision.
- Poll only relevant visible status pages; cancel on unmount/logout. DNS apply,
  archive/restore/purge and credential revocation wait for server acceptance.
- Reuse MUI theme, accessible labels, keyboard focus, dialog confirmation, and
  loading/empty/error/retry conventions. Render notes/provider text as text.
- Display timestamps in browser timezone with a timezone indication; date-only
  values must not shift dates. Money formatting uses ISO exponents and exact input
  parsing, never binary floating point for stored or calculated monetary values.

## Tests and completion

Use unit tests for meaningful rules; real temporary SQLite for transaction,
constraints, pagination and concurrency; httptest for handler/provider boundaries;
temporary Unix sockets/filesystems for agent/publication behavior; RTL and
Playwright for actual user flows. Inject clocks instead of sleeping through expiry.

Every mutation slice needs success, invalid input, unauthorized/scope failure,
stale revision where applicable, and rollback/audit evidence. Every asynchronous
slice needs retry, restart, uncertain outcome, and stale-worker tests.

During implementation, run `make generate` after contract/query edits, relevant
package tests, `make format-check`, `make lint`, and `make verify` before package
completion. `make verify` regenerates/builds artifacts; inspect its changes and
include only intended generated output. Run `make test-e2e` for routed workflows.
Once `agent/` and `integration/` exist, extend the Makefile's Go package list so
`make test`/`make lint`/`make format` actually cover them.

Report exact commands and outcomes. A skipped architecture/provider/hardware test
is unverified, not passed. Keep the acceptance checklist open until demonstrated.
