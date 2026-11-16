# Shared coding rules

These rules apply to the pivot's active operations. [Product scope](../focused-delivery.md)
controls delivery scope; [progress](parallel-progress.md) controls status; the
The current progress ledger controls implementation status. Future schema surfaces
are not automatic implementation requirements.

## Structure and data

- Reuse one Go server and existing database, server, agent, version and web modules.
  Handlers map HTTP/generated transport; small services own business validation,
  revisions, transactions/audit/idempotency; sqlc queries own data access. Extract
  incrementally, not a replacement implementation or generic CRUD/DI framework.
- Keep domain services independent of generated HTTP DTOs. Agent may share transport
  and version primitives, not database/server/frontend implementation. Use narrow
  consumer-owned interfaces for clock, provider, filesystem and other actual tests.
- Context first, bounded cancellation, errors with operation context, no inner
  process exits. One owner logs sanitized errors; do not duplicate request bodies.
- New IDs are UUIDv7, persisted v4 references remain compatible. Canonical lowercase,
  version/variant validation, cryptographic secrets and injected clocks/IDs for tests.
- UTC storage uses whole seconds `YYYY-MM-DDTHH:MM:SSZ`; normalize incoming fractions.
  Calendar dates remain dates. Unknown is null, not zero. Revisions are integers,
  not timestamps. Normalize IPs with netip and DNS/IDNA through a common boundary.
- Preserve immutable numbered migrations and user data. Correct forward with
  populated migration tests; no hand-edited generated source or constraint weakening.
- SQL belongs in `internal/database/queries/`. Use the transaction's query object
  consistently; never mix outside-transaction reads into a mutation's authority checks.
  Parameterize values, allowlist sort/column choices and bound recursive/paged work.
- Short write transaction: reread referenced state, check expected revision, mutate,
  audit, persist any safe replay result, commit. Failure rolls back all effects.
  Provider/network/filesystem publication remains outside SQLite write transactions.

## Transport

- OpenAPI is the transport source; operation IDs remain stable unless explicitly
  reconciled. Validate raw member presence, nulls, unions, enums and limits as well
  as unknown/trailing input. Check IDs before effects. Test complete operation bodies.
- Indexed keyset paging has a stable unique tie-breaker, bounded versioned cursor
  encoding with collection/filter/sort/parent identity, and page-size-plus-one logic.
  Same defaults/limits must hold in API and implementation. No invisible truncation.
- Strong ETags `"revision-N"`; missing/malformed required header 400, stale revision
  412, state/stale-preview conflict 409, invalid semantic input 422, missing 404,
  unauthenticated 401, forbidden/CSRF 403, too large 413, rate limit 429 and unavailable
  local dependency 503. Declare actual operation failures in the contract.
- Problems are `application/problem+json`, stable snake_case code, safe detail and
  consistent request ID. No raw SQL/provider errors. Auth/issuance responses are
  no-store. No plaintext credentials in URL, audit, logs or generic replay storage.
- Exact capabilities do not inherit implicitly. Explicitly grant the sole admin
  the set needed by implemented operations; keep human/agent/enrollment boundaries.

## Frontend

- Feature components/hooks/adapters under `web/src/features/`; composition in app/
  routes/, shared controls in components/. Use installed Router/Query and form tools
  rather than an independent state/fetch framework. Keep generated response/write types.
- One client pipeline handles credentials, CSRF, problems, field errors, request
  IDs, cancellation and session replacement. Complete state commits remain tied to
  the originating session/resource/filter identity, including delayed body decoding.
- Query keys include ID/filter/cursor/sort/session; reset paging with filters and
  invalidate affected lists/details/tree/counts after confirmed mutations.
- A form owns ID, original ETag and initial values together. Preserve edits on 409/412
  and offer explicit recovery; never silently adopt a refreshed revision under them.
- Memory-only issuance secrets are not query cache data. Clear on close/navigation/
  expiry and discard obsolete completions. Poll visible pages only, with backoff.
- Use accessible MUI controls, confirmations where consequential, and distinct
  loading/empty/error/stale/retry/unknown states. Render provider/notes text as text.
  Browser-timezone timestamps are labeled; calendar dates never shift timezone.

## Parallel ownership and completion

- One main owner controls OpenAPI, migrations/schema version, generation, shared
  router/runtime, dependencies/build configuration and status docs. Follow file
  leases; request shared changes, do not make uncoordinated edits or duplicate helpers.
- Use `make generate` after contract/query changes, coordinated by main. It rebuilds
  shared outputs; main pauses affected writers, inspects changes and verifies the
  integrated snapshot. Only main runs broad formatting/build/Playwright checkpoints.
- Meaningful tests: real SQLite constraints/transactions/paging/concurrency, httptest
  real handlers, clocks not sleeps, socket/filesystem fixtures, RTL/browser user flows.
  Mutations cover rejection/stale/auth/rollback/audit; async work covers retries,
  restart, uncertain outcomes and stale completions. No implementation-mirroring tests.
- Main completion checks: `make verify`, `make test-e2e`, `git diff --check`, plus
  required Linux/Podman/CoreDNS/provider execution. Ensure new packages are included
  in verification. A cross-build/mock/old pilot is not new external execution.
- Track implemented, integrated, locally verified and externally verified separately.
  Do not mark a workflow complete from a helper's existence or a general test pass.
