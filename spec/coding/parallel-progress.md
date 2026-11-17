# Current implementation and verification ledger

Reviewed 2026-11-17 after runtime/observability work, retaining the Germany
deployment and current-build acceptance checkpoint. This is the **sole current status document**. Requirements live in the
[index](README.md); historical dispatch records are in [legacy](legacy/README.md).
The dirty/untracked implementation was preserved. Nothing was committed or pushed.

## Milestone state

| Milestone     | Implemented and integrated                                                                                                                                                                                             | Remaining acceptance                                                                                                                                   |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| M1 inventory  | Login, providers/locations/computes/IPs, hierarchy/counts/lifecycle, restart browser flow; compute provider/location/freshness filters; omitted IP primary and empty provider roles rejected                           | Operation-wide validation/filter/capability/idempotency/audit integration; safe forms/selectors and runtime failure cases                              |
| M2 discovery  | Linux/Podman agent, enrollment/heartbeat/ingestion, current facts/container/collector UI, safe retention; required collection validation, correlated acceptance audit/replay, agent state lock and response validation | Complete shape/protocol/state validation, scheduler/ack/cancellation edge cases, mounted capability/read integration and current-build host acceptance |
| M3 domains    | CRUD/lifecycle/expiry, multi-page browser list and restart; snapshot-safe domain adapter and registrar selector work                                                                                                   | Complete U0 composition and safe edit integration, complete registrar selectors, integrated errors/audit                                               |
| M4 DNS        | API/schema/projection-query groundwork                                                                                                                                                                                 | No mounted DNS workflow, provider reconciler, publisher or DNS UI                                                                                      |
| M5 operations | Explicit migrate/schema checks, private backup/reset, deployment procedure/service and historical pilot                                                                                                                | Remaining runtime/service/restore evidence, then DNS-aware deployment/recovery                                                                         |

**G1 is operationally unblocked for the tested root-owned Podman deployment, but
formal evidence still needs to be recorded from the target hosts. G2 remains
pending DNS implementation.** A file/helper existing is not integrated behavior.
No DNS runtime packet has been dispatched.

## Runtime and observability checkpoint (2026-11-17)

Implemented for the codebase-polish and server OpenTelemetry objective:

- Shared UUID generation/validation, structured context-aware logging, and
  cancel-and-join worker ownership across server and agent. Agent scheduling moved
  out of the command package into `agent.RunConfig` / `agent.Run`; local failures
  are terminal for both workers and ticks cannot bypass `Retry-After`.
- Instance-scoped server tracer/meter/logger providers, OTLP/HTTP protobuf export,
  standard environment/resource/sampling configuration, opt-in/disabled modes,
  startup cleanup, and bounded concurrent shutdown flushing.
- Mounted HTTP spans, request/active/duration/size metrics, request/trace/log
  correlation, bounded route/method labels, and sanitized panic handling. Database
  pool, Go runtime, and retention metrics/spans/logs are wired into server startup.
- Agent W3C/request correlation propagation, shared canonical UUID rules, and a
  Podman overflow-sentinel fix preventing hidden trailing response bytes.
- Configuration is documented in the root README, extension conventions in
  [runtime architecture](../runtime-architecture.md), and a diagnostic collector
  in `deploy/otel-collector.yaml`.

Verification on macOS with Go 1.27.4:

- `go test -race ./...`: passed across all packages, including the new telemetry,
  identity, logging, lifecycle, agent, and server regressions.
- Final logging/telemetry refinements also passed
  `go test -race -v ./internal/logging ./internal/telemetry ./cmd/portinaia`;
  concurrent exporter cleanup preserves all failures and is idempotent.
- A local OTLP/HTTP receiver decoded protobuf traces, metrics, and logs and verified
  resource overrides, remote-parent propagation, log correlation, and final flush.
- The subprocess SIGTERM test verified server startup wiring and final export of
  all three signals, including database/runtime observations and the shutdown log.
  The existing in-flight SQLite mutation drain test also passed.
- `go vet ./...`, `go mod verify`, native server build, and `make build-agent`
  (Linux amd64/arm64) passed. Go formatting and `git diff --check` passed.
- `GOTOOLCHAIN=go1.26.5 go test ./...` could not start: the toolchain download
  from `proxy.golang.org` failed with `unexpected EOF`. The passing checks above
  used Go 1.27.4; execution on the declared minimum toolchain remains unverified.
- `npm ci` and `make verify` are **blocked by the environment's npm registry HTTP
  403 policy** (`yocto-queue` tarball and `openapi-typescript`). `make format-check`
  passes its Go check but cannot run missing Prettier. Frontend tests, frontend
  build, and complete generated-consistency verification are not claimed for this
  checkpoint. The attempted Go API regeneration produced no diff.

This checkpoint is runtime/observability work; milestone acceptance above retains
its own evidence requirements.

## Current deployment checkpoint (2026-11-16)

The following results were reported after deploying the current build to the
German server and a Debian 13 VM named `krabica`. They are user-reported
external verification and should be retained with command output, versions,
checksums, and sanitized logs when the release evidence is assembled:

- Portinaia is reachable at the configured public HTTPS endpoint; liveness and
  readiness both return ready, including schema and database checks.
- The Linux agent enrolled successfully, sent heartbeats, and accepted snapshots
  through the deployed server.
- Root-owned Podman collection through
  `/run/user/0/podman/podman.sock` works and reports the six containers visible
  in that Podman scope. Additional Podman scopes are not configured.
- Agent restart, temporary server outage/retry, revocation/re-enrollment, and
  container visibility tests passed as reported.
- Backup and isolated restore tests passed as reported.
- Current local verification remains green: `make verify`, `make test-e2e`,
  frontend typecheck/tests, and `git diff --check`.
- Inventory mutation idempotency is integrated for provider, location, compute,
  domain, and manual IP create/update operations plus archive/restore. Browser
  mutations now receive automatic idempotency keys at the shared transport.
- A discovery summary composition bug was fixed locally: collector status,
  current containers, and discovered IP assignments are now passed into the
  summary panel instead of only being rendered in lower sections. The embedded
  server image must be rebuilt/redeployed for this fix to reach the live UI.
- The current UI is functional but visually provisional. A broader visual
  redesign is explicitly deferred until core workflows are accepted.

## DNS hold

DNS runtime implementation is intentionally on hold by product decision. Keep
`spec/dns-spec.md` and `spec/coding/dns-implementation.md` as the normative future
requirements, and use the [DNS handoff](dns-handoff.md) as the resume point. No DNS
runtime packet should start until the owner explicitly removes the hold.

The current-build deployment used manual agent execution rather than the
provided systemd unit. Rootless Podman and systemd service acceptance therefore
remain unverified for this deployment.

## Fresh local verification during this cleanup

Application commands below ran against the resumed implementation after focused
integration changes.

- `make verify`: **passed**: generated consistency, formatting, Go vet, TypeScript,
  ESLint, Go tests (some cached), 4 Vitest files / 13 tests, embedded frontend/server
  and Linux amd64/arm64 agent builds. Vite reported a bundle-size advisory, not failure.
- `make test-e2e`: **passed**, 2 Chromium scenarios including page continuation,
  archived navigation and actual stop/restart using isolated SQLite; schema version 10.
- `git diff --check` passed after implementation/build checkpoints.
- This was local macOS verification. No new Linux/Podman/systemd/CoreDNS/Cloudflare
  execution was performed. Historical execution is retained in the
  [dated pilot](legacy/consolidation-evidence-2026-09-27.md), including native systemd work.
- After documentation edits: `npm run format:check` and `git diff --check` passed;
  all 68 relative Markdown link targets across README and the 14-file spec tree
  resolved. A repository Markdown search found no references to the removed plans,
  work-package files or superseded audit/assignment files.
- Remaining findings below are source-reviewed unless a specific executable
  regression is cited. No new source-derived edge case is represented as reproduced.

## Completed corrections to preserve, not redispatch

- Provider response composition is fixed and whole actual Provider bodies are
  schema-validated (`api/implemented_handler_contract_test.go`).
- UUID path rejection runs before business effects; request identity middleware
  exists (`internal/server/server.go`, `request_runtime.go`). New IDs are v7 and
  populated v4 compatibility is covered.
- Required missing/null Podman owner is rejected and explicit UID 0 accepted;
  collector/container required checks exist (`discovery_protocol_test.go`).
- Archived location → archived host → **active VM** traversal is fixed and tested
  (`tree_context_test.go`). A narrower container-only case below remains.
- Unsafe short report retention is refused; cleanup preserves the full replay
  window plus clock skew and tests old retry not replacing newer facts.
- Domain/IP continuation, shared typed-client transport, many extracted components,
  Query/Router providers, and restart/stale-provider browser scenarios exist.
- Login dummy work/concurrency cap, same-origin check, last-seen throttling, truthful
  logout and bootstrap/reset audit exist; these need completion, not replacement.
- Audit helpers/query/DTO and UI, principal helpers, idempotency helpers/query and
  migration 00010 exist. They are **not yet complete mounted workflows**.
- Agent now propagates contexts into HTTP/Podman, joins workers, recollects pending
  snapshots, conditionally acknowledges IDs, parses Retry-After and validates more
  responses. Unit has permanent-exit restart suppression; preserve these improvements.

## Current residual workload

## Product note

The current UI is functional but is intentionally not considered final visual
design. A broader UI redesign is deferred until the core inventory, discovery,
domain, DNS, and operational workflows are accepted.

Packet IDs below are the current dispatch IDs, not the removed work-package backlog.
No task is actively leased by this documentation cleanup; the next orchestrator
must acquire/update leases before writing. Prior helpers and partial changes remain.

| Packet                   | State                                              | Source-backed remaining work                                                                                                                                                                                                                                                                                                                                                     |
| ------------------------ | -------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| P1 boundary/auth/audit   | Partial implementation; caller integration pending | Required-member/bounds helpers not consistently used; principal/class helpers unmounted; login admission wait needs cancellation; audit read DTO/PageInfo/filter/legacy agreement and route; allowlisted redaction; replay transaction service and expiry reuse. `request_validation.go`, `auth.go`, `authorization.go`, `audit.go`, `idempotency.go`, API tests.                |
| P2 inventory/domain/IP   | Partial                                            | Documented compute provider/location/freshness filters ignored; omitted IP `primary` becomes false; empty provider roles accepted. Container-only active descendant under archived location/all-archived compute chain still appears excluded by predicates (source review, not fresh reproduction). Inventory writer audit/idempotency integration and legacy/tag preservation. |
| P3 discovery server      | Partial                                            | Missing top-level `podman` can become non-nil empty during DTO conversion (`snapshots.go:333–360`); independent problem writer breaks shared correlation; accepted-snapshot audit absent; actual container/collector handler response and race/absence tests incomplete. Preserve fixed required UID behavior.                                                                   |
| A0 agent                 | Partial                                            | Linux blocking calls remain only indirectly cancellable; compare/remove pending ack not atomic with replacement; collect/upload/backoff still coupled; required response booleans/range members and full pending unions not validated; trailing credential JSON; schedule/warning/local override and terminal local-error semantics.                                             |
| U0 platform/integration  | Partial                                            | Propagate input Request signals; decoded-response/session isolation; integrate returned features/hooks; remove competing Workspace orchestration; uniform typed errors and truthful state.                                                                                                                                                                                       |
| U1 inventory/provider UI | Partial                                            | Original ID+ETag snapshots through caller submission, not only identity guards; complete compute/IP forms, independent paged selectors, preserved tags, stale edit recovery.                                                                                                                                                                                                     |
| U2 domain/audit UI       | Partial                                            | Domain snapshot callback is discarded by Workspace; wire adapter preserving tags, registrar hydration/selection, audit endpoint/filter/PageInfo/cursor/datetime and load-more behavior.                                                                                                                                                                                          |
| U3 discovery/agent UI    | Partial; hooks/components exist                    | Wire safer enrollment/discovery hooks without losing paging (new hook currently takes only first 200); selection/expiry/secret races, retained diagnostics and freshness states.                                                                                                                                                                                                 |
| Q acceptance/deploy      | Partial                                            | Correct executable enrollment-before-service sequence, align exits/state reset, current-build Linux retry/ack/revocation/restore evidence and remaining integrated browser cases.                                                                                                                                                                                                |
| Main runtime/shared      | Partial                                            | Mount execution cancellation/capability/audit boundaries, consistent problems/log/audit correlation, forced-shutdown evidence, retention dry-run floor and policy-default reconciliation; serialize generation.                                                                                                                                                                  |
| D0–D4 / G2               | Not started as runtime                             | Shared DNS applicability/intent/fencing agreement, then actual desired-state/provider/publisher/UI/recovery workflow after G1.                                                                                                                                                                                                                                                   |

## Resumption checkpoint (2026-09-30)

The packet table above is the pre-resumption baseline; the following results supersede
its rows where they conflict. The worktree remained dirty/untracked throughout; no
commit or push occurred.

| Packet | Implemented                                                                                                                                                                                                          | Integrated                                                                                                    | Locally verified                                                                                                               | External verification                                             |
| ------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------- |
| P1     | Required-member/bounds decoder; cancellable Argon admission; explicit sole-admin capability set; audit allowlist/filter/page helper; replay lookup/conflict/expiry helper and SQLite tests.                          | Login helper is used; audit DTO and replay primitives are not mounted as complete routed/mutation workflows.  | Focused tests and `go test ./...` passed.                                                                                      | None.                                                             |
| P2     | Compute provider/location/freshness filters; omitted IP primary and empty provider roles rejected; archived ancestry container traversal fixed; audit append adopted for provider/location/compute/lifecycle writes. | Inventory audit in those transactions; replay and UI integration remain.                                      | `go test ./internal/server` passed.                                                                                            | None.                                                             |
| P3     | Required top-level report collection validation; correlated accepted/reactivation audit; replay/rollback assertions.                                                                                                 | Ingestion transaction uses shared audit helper; read capability/response route matrix remains.                | Focused discovery/server tests passed.                                                                                         | None.                                                             |
| A0     | Exclusive snapshot lock; recollection during retry; strict credential/pending/enrollment/response validation; retry bounds and terminal local failures.                                                              | Client behavior implemented; deploy instructions aligned.                                                     | `go test -race ./internal/agent ./cmd/portinaia-agent` and vet passed; Linux amd64/arm64 cross-builds passed in `make verify`. | No current-build Linux/Podman/service execution.                  |
| U0     | Request-signal composition and post-decode guards; session reset; domain snapshot submission; shared edit/selector contract published; audit view mounted.                                                           | Domain/audit composition partial; U1 feature snapshots and U3 discovery hook are not fully wired.             | Focused Vitest, typecheck and lint passed; whole `make verify` passed.                                                         | None.                                                             |
| U1     | No new feature edits; agent remains waiting for caller snapshots to flow through components.                                                                                                                         | Not integrated.                                                                                               | No new packet test.                                                                                                            | None.                                                             |
| U2     | Domain snapshot/registrar and audit filter/cursor UI improved.                                                                                                                                                       | Audit endpoint contract exists but server route and truthful legacy DTO remain unresolved; U0 wiring partial. | Focused Vitest/typecheck passed; whole `make verify` passed.                                                                   | None.                                                             |
| U3     | Discovery pagination through all pages; diagnostic availability and enrollment secret context guards.                                                                                                                | Hook/panel still needs U0 Workspace wiring.                                                                   | Focused Vitest and typecheck passed; whole `make verify` passed.                                                               | None.                                                             |
| Q      | Enrollment-before-service sequence, explicit socket activation, exit/state-repair docs.                                                                                                                              | Runbook updated.                                                                                              | Formatting and agent package tests passed; `make test-e2e` passed locally.                                                     | Linux service/retry/revocation and isolated restore not executed. |

Main regenerated OpenAPI/sqlc outputs after query changes. Final local checkpoints at
this resumption: `make verify` passed (including Go, Vitest, Linux amd64/arm64 agent
cross-builds); `make test-e2e` passed (2 Chromium scenarios); `git diff --check` passed.
These are local macOS results. They do not satisfy G1's required current-build Linux/
Podman/service evidence. G1 therefore remains blocked, and no DNS workload may start.

## Evidence retained from prior checkpoints

- Migration 00010 adds replay storage; generation and schema-10 verification pass.
  It does not establish replay lookup/mutation/audit/result integration.
- Earlier request-path middleware failure exposed chi route-param timing; current
  pre-dispatch ID check and regression are the integrated correction. Do not restore
  the response-buffering post-handler design from deleted instructions.
- The original missing-owner and active-VM ancestry reproductions led to current
  focused regressions. The remaining absent top-level collection/container-only
  ancestry findings are different cases and need their own tests.
- The original retention reproduction accepted an old report with HTTP 201 after
  cleanup and replaced a newer hostname. The minimum age/skew floor and integrated
  HTTP/cleanup regression now prevent that path. Keep the fix and coverage.
- Green earlier runs and the dated Linux pilot are useful provenance, not automatic
  acceptance for the newly integrated code or any DNS behavior.

## Updating this ledger

For each packet record owner/file lease, dependency, implemented items, integrated
callers, commands/results and external evidence separately. Use pending/running/
waiting-for-interface/ready-for-integration/verified/blocked. A waiting packet releases
its execution slot but retains its file lease until explicitly transferred.
Do not copy old completion labels over untested edge cases; reassess G1/G2 against
the focused requirement IDs and provide the next exact task.
