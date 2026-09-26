# Implementation audit

Reviewed: 2026-09-26. Evidence is source inspection plus the commands below.
This directory has no local `.git` directory; no commit-based diff/status is
claimed. Generated binaries in `bin/`, `dist/`, and `release/` are not proof of
current source correctness or deployment verification.

## Verified baseline

| Area           | Evidence                                                                  | What actually exists                                                                                                   |
| -------------- | ------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Tooling        | `go.mod`, root/web `package.json`, lockfile, `Makefile`                   | Go 1.26.5; pinned Go/frontend generators and dependencies; generate/format/lint/test/build/image/release recipes       |
| Server         | `cmd/portinaia/main.go`, `internal/server/server.go`                      | `server`/`version`; fixed `:8080`; chi router, embedded SPA, two hard-coded health routes                              |
| Agent          | `cmd/portinaia-agent/main.go`                                             | Version command only                                                                                                   |
| API contract   | `api/openapi.yaml`, `api/openapi_test.go`                                 | 103 declared operations; OpenAPI validation and focused schema/scope tests                                             |
| API generation | `internal/api/generated/openapi.gen.go`                                   | Generated models, chi and strict interfaces; not connected to the running router                                       |
| SQLite         | `internal/database/schema.go`, `migrations/00001_initial.sql`             | Embedded goose up/down helpers; `user_version`; initial 40-table schema; no runtime DB open/configuration layer        |
| SQL queries    | `queries/administrators.sql`, `providers.sql`, `discovery.sql`, `dns.sql` | Selected administrator/provider/report/agent/projection primitives; generated wrappers                                 |
| DB tests       | `internal/database/schema_test.go`                                        | Empty migration/down; selected ownership/identity/date constraints; snapshot unique keys; lease and agent-cursor cases |
| UI             | `web/src/app/App.tsx`, `theme/theme.ts`                                   | Material UI title page and theme; no protected routes, forms, or product screens                                       |
| Client         | `web/src/api/client.ts`, `generated.ts`                                   | Typed openapi-fetch client with same-origin credentials; no error/CSRF adapter                                         |
| Packaging      | `Containerfile`, `web/embed.go`                                           | Multi-stage build, scratch runtime, CA bundle, UID/GID 65532; host embedding and cross-build recipes                   |
| Browser smoke  | `web/tests/foundation.spec.ts`                                            | Health response and placeholder title at `/` and `/inventory`; not inventory functionality                             |

## Commands executed during this review

- `make test`: PASS; Go package results included cached test passes.
- `npm test`: PASS; one Vitest test file, one placeholder-App test.

Not executed: generation/build pipelines, browser smoke, container image builds,
native arm64 startup, external-provider integration, or capacity tests. The
README's earlier verification statement is historical evidence only. Those
commands are specified as future package gates, not claimed as review results.

## Confirmed gaps requiring WP-02 reconciliation

| ID   | Evidence and mismatch                                                                                                                                                                 | Required correction / owner                                                                                                              |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| C-01 | Actual health returns `{status: ok}`; API requires live/ready plus time/checks. API 404 is plain text.                                                                                | WP-03 must use generated health schemas and RFC 7807 middleware; update foundation tests.                                                |
| C-02 | API Tag has revision/update/archive metadata; SQL tags has only ID/names/created time. DNS drafts API also has revision absent in SQL.                                                | Add forward migration and revision-checked queries; WP-02.                                                                               |
| C-03 | `ComputeObservation` includes disks/interfaces/virtualization; current SQL lacks these facts.                                                                                         | Add normalized current disk/filesystem/interface/address tables and virtualization column; WP-02/WP-06.                                  |
| C-04 | API permits arbitrary recurring ISO periods; SQL only four named intervals; one-time API excludes effective dates but SQL requires start date.                                        | D-02 enum mapping; nullable type-specific dates and matching checks; WP-02/WP-05.                                                        |
| C-05 | API currency exponent permits 4, SQL only 0–3; API PAT/tag name limits 200/100, SQL 128/64.                                                                                           | Preserve API limits using migrations; validate real ISO currency/exponent list; WP-02.                                                   |
| C-06 | API provider role `hosting_provider`, restart values `on_failure`/`unless_stopped`; SQL `hosting`, `on-failure`/`unless-stopped`. API unknown restart/mount other unsupported in SQL. | Explicit bidirectional mapping for spelling; forward schema support for valid unknown/other states.                                      |
| C-07 | API IP role free string vs SQL enum; API network address lacks prefix but SQL requires it; combined interface/network transport field vs separate columns.                            | Define bounded role enum, optional unknown prefix, owner-based field mapping, and test round trips.                                      |
| C-08 | API report includes client version/start/end/content reuse and `accepted_with_errors`; SQL report lacks most metadata and permits accepted/rejected only.                             | Persist immutable report envelope and acceptance result, including D-01 observations; make rejected-report content optional.             |
| C-09 | CanonicalSnapshot contains report ID and its own hash; gzip shared content cannot truthfully carry each report's ID. Uptime/free bytes defeat unchanged-state reuse.                  | Separate structural blob from report download envelope; reconstruct requested report download and hash only structural bytes; WP-08.     |
| C-10 | SnapshotComparison omits disk changes; partial host/interface/filesystem success is not modeled like Podman.                                                                          | Typed collector outcomes and disk-diff schema before protocol freeze; WP-02/WP-06/WP-08.                                                 |
| C-11 | Projection scheduling refuses equal revisions; failed rows with NULL retry are eligible; claims mix providers; completion always marks synchronized.                                  | Separate same-revision repair, permanent failure exclusion, provider-specific claiming and cleanup/not-applicable transitions; WP-09/10. |
| C-12 | No durable zone publication generation/lease row; publication revision cannot be derived from unrelated name revisions.                                                               | Add zone-wide generation/publication state; WP-02/WP-11.                                                                                 |
| C-13 | Import candidate schema requires supported type, valid TTL and values even for unsupported/malformed records. Import selections allow more changes than preview can return.           | Discriminated rejected-candidate shapes and bounded preview pagination/selection; never silently truncate a change preview.              |
| C-14 | Hash-only session CSRF storage cannot recover original random CSRF material on `/auth/session`; generic idempotency response blobs could contain credentials.                         | Derivable CSRF token using secret session credential; secret-issuance exceptions; WP-04.                                                 |
| C-15 | Audit SQL permits failure, API only success/rejected; SQL actor arbitrary text, API UUID.                                                                                             | Unified actor representation and enum; redacted document-to-field mapping; WP-02/WP-04.                                                  |
| C-16 | SQL timestamps reject fractions while API allows them; observed time constrained <= receipt time despite tolerated client skew.                                                       | Central whole-second normalization and separate client/server time semantics; remove invalid client-clock constraint.                    |
| C-17 | DNS lifecycle SQL `deleted` vs API `archived`; draft validation SQL `pending` vs API `unvalidated`.                                                                                   | Explicit tested boundary mappings, not blind string casts.                                                                               |
| C-18 | Enrollment active-token unique index includes expired tokens; agent unique compute prevents naive re-enrollment INSERT.                                                               | Expire/revoke old token in issuance transaction; update existing agent/generation on re-enrollment.                                      |
| C-19 | APIs have no comprehensive inventory-linked DNS filters/container-history lookup; tree limits alone cannot browse truncated children.                                                 | Add bounded navigation contracts needed by WP-12/13/14; lazy tree continuation or filtered child lookup.                                 |
| C-20 | Purge endpoints currently require only inventory write, D-06 requires explicit administrative purge.                                                                                  | Add `admin` plus inventory write, dependency problem detail/lookup contract.                                                             |
| C-21 | Initial tables often check UUID formatting, not version/variant; provider roles need at least one member; constraints alone do not implement services.                                | Boundary/domain UUIDv7 and aggregate validators plus appropriate DB constraints/tests.                                                   |

## Patterns worth preserving

- Small Go packages, chi/net/http, table-focused SQL and generated query code.
- SQLite integration tests with real migrations, temporary files, and cleanup.
- `context.Context` on queries and errors wrapped with operation context.
- Generated transport types separated from future domain services.
- Existing `internal/server` composition and `web` embedding; no unnecessary move
  to a different server package just to match the illustrative layout.
- Existing formatter, strict TypeScript, MUI theme, typed API client, Vitest/RTL
  and Playwright. React Router/Query/Form are installed, not yet used.

The inline SQL in migration tests and schema-version helper is not a pattern
for production feature handlers. The placeholder's `log.Printf`, fixed port,
unconditional readiness, and unauthenticated SPA are not production conventions.
