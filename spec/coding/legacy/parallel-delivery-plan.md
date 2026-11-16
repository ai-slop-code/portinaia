# Current multi-agent orchestrator workload

Updated 2026-09-30. This is a **resumption workload**, not a fresh implementation
plan. Read [current progress](parallel-progress.md) before dispatching. Many helpers,
components and regressions already exist; finish their residual correctness and
integration rather than repeat old assignments.

## 1. Read order and scope

1. [Product scope](../focused-delivery.md): three goals, deferrals, M1–M5, G1/G2.
2. [Progress](../parallel-progress.md) and [interfaces](parallel-interfaces.md).
3. [Rules](coding-rules.md), [decisions](decisions.md).
4. Relevant [backend/UI](backend-requirements.md), [discovery](discovery-requirements.md),
   [runtime](runtime-requirements.md) or [DNS](dns-implementation.md) requirements.
5. [Detailed DNS contract](../dns-spec.md) before post-G1 DNS work.

The removed omnibus plans, work packages and audits are not fallback instructions.
Current acceptance uses BE/DI/RT/DN IDs. G1 precedes new DNS runtime behavior. Costs,
history UI, PAT management, generic purge, maximum-capacity certification and a
physical-Pi test gate remain outside this workload.

## 2. Main-agent responsibilities and safe parallelism

- Inspect and preserve the current modified/untracked tree. Do not reset/stash/clean,
  re-scaffold, rewrite applied migrations, re-key legacy data, commit or push.
- Default to disjoint leases in this checkout. Clean worktrees from HEAD omit much
  current implementation; do not use them without faithfully transferring the entire
  dirty baseline and defining integration. Investigate unfamiliar concurrent edits.
- Use **at most four active coding agents**. Resume partial packets with a specific
  residual acceptance brief; do not blindly relaunch the old foundation workload.
- Maintain the existing progress/interface ledgers, not competing new status files.
  Distinguish implemented, integrated, locally verified and externally verified.
- Waiting agents release execution slots after publishing interfaces; keep their
  file lease until an explicit transfer. Consumers depend on published seams, not
  complete foundation packets, avoiding a four-slot dependency deadlock.
- Main alone owns OpenAPI, migrations/schema version/shared schema tests, generated
  outputs, shared router/server integration tests, `cmd/portinaia/**`, runtime/config,
  manifests/lockfiles, Makefile/Containerfile, README and status/spec changes.
- P1's explicitly listed request-runtime helper files are its exception to the
  runtime lease; main still owns mounting, command/configuration and shared tests.
- Agents request shared changes with behavior, schema/signature, consumers,
  compatibility and acceptance tests. Main publishes one generated checkpoint; no
  hand-edited generated output, concurrent migration numbers or duplicate helpers.
- Main serializes generation, broad formatting/builds and browser verification on
  a stable tree. Feature agents run focused non-generating tests in leased files.
  Pause affected writers for checkpoints; do not let builds rewrite shared assets
  or run multiple browser suites against one controller/port.

### Main-owned remaining integration

1. Resolve audit DTO/PageInfo/filter/legacy compatibility and exact sole-admin
   capabilities with P1; wire the actual route and caller request identity.
2. Mount operation validation and execution cancellation. Unify logs/problems/audit
   correlation, including discovery's separate problem writer. Test forced shutdown,
   not just successful drain, before database closure.
3. Integrate replay storage/query/service into active mutations without duplicating
   effects/audit, secret replay or expired-key conflicts.
4. Align retention dry-run with the existing replay/skew floor; preserve the fixed
   safety invariant. Reconcile password/session/schedule defaults explicitly against
   preserved policy—do not silently accept drift or weaken a requirement.
5. Coordinate frontend caller changes through U0 only. Run complete operation and
   workflow checks after helper/component handoffs, not merely unit compilation.

## 3. Residual packets before G1

The exact paths below are exclusive write leases; new focused tests may be added
within the owner's area. Tests in shared files require main integration. Agents
must recheck that a listed residual defect still exists before changing code.

### P1 — Boundary, authentication, audit and replay primitives

**Own:** `internal/server/{transport,request_validation,request_runtime,idempotency,auth,authorization,audit}.go`,
their dedicated test files; `internal/database/queries/{administrators,audit,idempotency}.sql`;
`api/{active_operation_contract,implemented_handler_contract}_test.go`.
Main retains router, OpenAPI, migrations and generated outputs.

**Requirements:** BE-01/02/03, RT-02. **Reuse:** fixed Provider schema, pre-handler
UUID guard, same-origin/dummy hash/concurrency/session improvements, audit helpers and
migration 00010. Do not recreate them.

**Finish:** adopt raw required/bounded operation validation, actual route response/
auth/CSRF/ETag/filter matrix; cancellation-aware bounded login admission; mounted
credential/capability seam; audited field allowlist; resolved legacy audit DTO and
paging/filter service; transactional idempotency lookup/conflict/result/expiry API.
Publish these interfaces early so P2/P3/U2 can integrate.

**Accept:** real handlers reject missing required/valid-zero distinctions; failed
revocation/limits/cancellation truthful; old audit remains readable without fabricated
IDs; replay has one effect/audit and safe expiry; full responses—not constructed or
field-projected DTOs—validate. Focused Go/SQLite tests and proposed router wiring.

### P2 — Inventory/domain/IP integration and remaining data rules

**Own:** `internal/server/{inventory,providers,locations,computes,domains,ip_assignments,lifecycle}.go`,
`tree_context_test.go`, new `inventory_*_test.go`; `internal/inventory/**`;
`internal/database/queries/providers.sql`, new `internal/database/queries/inventory.sql`.

**Requirements:** BE-01/03/04/05. **Start:** independent regression tests immediately;
shared audit/replay/capability adoption follows P1 checkpoint.

**Finish:** documented compute provider/location/freshness filters; omitted IP primary
and empty provider-role rejection; container-only active descendant under archived
location/all-archived compute chain. Preserve the already-fixed active-VM variant.
Adopt shared correlated audit/idempotency/capability in owned transactions; preserve
legacy links and dormant tags; extract touched SQL/services incrementally.

**Accept:** actual filter-bound cursor/list/count behavior, active container-only
ancestry visible exactly once, required fields/refs/lifecycle conflicts, rollback,
replay and legacy populated relationships. Confirm source-derived edge cases with
regressions before calling them reproduced.

### P3 — Discovery server and container-read completion

**Own:** `internal/server/{snapshots,snapshot_facts,discovery_auth,collector_status,compute_observation,containers}.go`,
dedicated discovery/snapshot/container test files; `internal/discovery/**`;
`internal/database/queries/discovery.sql`. Main owns retention runtime/floor wiring and shared tests.

**Requirements:** DI-01/02/03/04, BE-01/03. **Reuse:** strict owner UID, explicit empty
container checks, existing deduplication and fixed retention floor.

**Finish:** missing top-level required collection validation before DTO conversion;
one correlated problem path; compact accepted/reactivation audit in the transaction;
actual collector/container response/filter/capability coverage and race/absence/
rollback cases. Agree protocol/schedule/required fields with A0; do not impose an
uncoordinated client/server breaking change.

**Accept:** malformed omitted arrays cannot claim authority, explicit empty and UID 0
remain valid; duplicate replay creates no new facts/audit; revoke/archive races and
manual separation hold; actual read DTOs/paging and correlation meet the API.

### A0 — Agent residual protocol, state and lifecycle

**Own:** `internal/agent/**`, `cmd/portinaia-agent/**` and tests. No backend or deploy edits.
**Requirements:** DI-05/06, RT-03. **Start:** immediately with current interfaces.

**Reuse:** context-aware HTTP/Podman, worker join, fresh pending replacement,
conditional-ID clear, Retry-After/permanent HTTP errors and improved validation.

**Finish:** safe compare/delete under replacement or explicit exclusive state owner;
independent collection during retry/backoff; full required response booleans/range/
pending unions and trailing credential validation; bounded blocking Linux collection;
schedule/warnings/local override; terminal config/state/malformed-success failures,
Retry-After overflow bounds and clear one-shot/run exit contract. Correct diagnostics
that still claim every next iteration retries the same bytes while recollecting.

**Accept:** concurrent stale ack and restart, malformed responses retain pending state,
delayed upload does not freeze collection, abort/shutdown joins bounded workers,
permanent errors terminate correctly and transient retry works. Publish exit/config/
state-repair contract to Q. Focused Go/race tests; Linux behavior executed by Q/main.

### U0 — Frontend composition, transport and query integration

**Own:** `web/src/app/**`, `web/src/main.tsx`, `web/src/routes/**`, `web/src/components/**`,
`web/src/api/client.ts`, its test, and `web/src/features/auth/**`. Only U0 edits Workspace.
**Requirements:** BE-06. **Reuse:** Query/Router, session/error/form primitives and counters.

**Finish:** retain input Request abort signal; guard complete decoded response/state
commit; consistent list/selection/mutation/session invalidation; reset stale editing/
paging on session replacement. Wire U1–U3 callbacks/hooks rather than discard edit
snapshots or leave parallel inline implementations. Use normalized field/network/
request errors and distinct unavailable/empty/zero. Publish independent selector,
original-ID/ETag and query identity contracts before consumer rewrites.

**Accept:** original Request signal cancels generated calls; delayed body/old session
cannot refill state; A→B edits submit only their snapshot or are explicitly rejected;
same-ID refresh cannot silently upgrade ETag. Feature integrations pass without
lost paging, stale secrets, duplicate forms or hidden error states.

### U1 — Inventory/provider safe editing and selectors

**Own:** `web/src/features/inventory/**`, `web/src/features/providers/**` and colocated tests.
**Requirements:** BE-04/06. **Dependency:** U0's published callbacks/selector/query contract.

**Finish:** ID+original ETag snapshot end-to-end for providers/locations/compute/IP,
not identity-only guards; preserve tags; complete extracted compute/IP forms and
independent paged selectable references; loading/conflict/lifecycle recovery.
Return exports/caller change requests to U0, never competing Workspace edits.

**Accept:** resource switch and same-resource revision refresh; dirty recovery after
412; unloaded/archived selected refs displayed but invalid new refs blocked; references
beyond page one independent of list search; metadata edits preserve hidden fields.

### U2 — Domain integration and usable audit browsing

**Own:** `web/src/features/domains/**`, `web/src/features/audit/**`, colocated adapters/tests.
**Requirements:** BE-03/05/06. **Dependencies:** U0 callback consumption and P1 audit contract.

**Finish:** snapshot-initialized domain fields and submission, use existing tag-preserving
adapter, registrar independent lookup/hydration; return audit exports and wiring
requests to U0 (no route/Workspace ownership), with resolved
filters/PageInfo, UTC timestamp conversion, append/continuation semantics and truthful
legacy/error/loading states. Existing components are not final until wired.

**Accept:** correct original revision under selection/refresh, unknown dates/renewal,
normalization/date errors, >1-page refs and audit filter/cursor/session behavior.

### U3 — Wire current discovery and enrollment without regressions

**Own:** `web/src/features/discovery/**`, `web/src/features/agents/**` and colocated tests.
**Requirements:** BE-06, DI-02. **Dependencies:** U0 session/query boundary, P3 facts.

**Finish:** real continuation in `useComputeDiscovery` rather than returning only
first-200 `.items`; integrate safer enrollment through U0; retain diagnostics on read
failure, explicit freshness/absence/time/null-prefix presentation and visible polling.
Keep secrets outside cache with context-bound close/navigation/expiry behavior.

**Accept:** old compute/session requests cannot surface facts or secrets; 201st
container/IP reachable, unknown/unavailable/empty remain distinct, and revoke/reissue
lost-response instructions match actual server/agent behavior.

### Q — Integrated acceptance and executable deployment

**Own:** `web/tests/**`, `web/playwright.config.ts`, `deploy/**`,
`integration/consolidation/**`. Main owns README/build/release/evidence edits requested by Q.
**Requirements:** RT-03/04, DI-06 and integrated BE workflows.

**Finish:** preserve existing isolated restart/paging tests, add delayed body/session/
selection/safe-edit/selector/audit/authority cases. Correct installation ordering to
enroll before service start; activate sockets explicitly; align actual A0 exit codes
and pending-state reset. Execute current-build Linux native service retry/supersession/
stale-ack/revocation and isolated backup/restore; record hashes/counts/commands.

**Accept:** no revocation restart loop; transient recovery and bounded SIGTERM; manual
data/identity preserved; old evidence not claimed as new execution. Do not add a Pi
capacity or arbitrary distro gate. Main runs shared browser/build commands serially.

## 4. Dispatch order and G1

1. Reconcile the current interface issues with P1 and U0, not another full rewrite.
   Initial ready set: **P1, A0, U0, P2** (P2 can start independent regressions).
2. After P1 publishes interfaces, release its waiting slot for P3 and integrate
   shared API/generation in main. Resume P1 for actual caller/route acceptance.
3. U0 publishes callbacks then releases a slot; U1/U2/U3 implement feature-owned
   changes. Resume U0 to wire them. Audit UI waits on actual P1 read contract.
4. Q designs tests during available slots and verifies the integrated build with main.
   Never keep all four slots waiting on unlaunched downstream consumers.
5. Track each BE/DI/RT acceptance item; run focused checks and then stable-tree
   `make verify`, `make test-e2e`, `git diff --check`. Main applies generated changes.

**G1 passes only when M1/M2/M3 and supporting audit/runtime acceptance are met**, not
when files are renamed, helpers exist, or the general suite is green. Complete
unblocked local work if Linux access is missing; record required external checks
as blocked and do not bypass G1 by starting DNS. A scope/gate change requires an
explicit owner decision, not an agent's convenience.

## 5. Post-G1 DNS and operations packets

### D0 — Main-led shared DNS agreement

Use DN-01 and the detailed DNS contract. Freeze provider-observation applicability,
input/output/preview/import identity, desired-state/idempotency/audit transaction,
ownership intent, unique claim fencing, zone generation/serial/lock ordering and
post-rename recovery. Main allocates forward migrations and owns OpenAPI/generated
outputs/shared projection SQL. Domain/provider/publisher/UI reviewers agree one
contract; no unrelated dormant API completion prerequisite.

### D1 — Domain/drafts and apply

**Own:** root Go files in `internal/dns/`, `internal/dns/domain/**`, new
`internal/server/dns_domain*.go`, `internal/database/queries/dns_domain.sql`, focused tests.
**Deliver DN-02:** whole-name validation, preview, revisioned drafts, atomic audited
idempotent apply/import handoff, independent projections, tombstones, reads/links.
**Accept:** stale/invalid/conflict/rollback, internal-only provider outage, complete
preview, same-revision repair and deletion/finalization races. No adapter/UI edits.

### D2 — Cloudflare adapter, import and reconciler

**Own:** `internal/dns/cloudflare/**`, `internal/server/dns_cloudflare*.go`,
`internal/database/queries/dns_cloudflare.sql`, focused tests.
**Deliver DN-03:** bounded discovery/selective import, durable owned-object intent/
mapping, uncertain writes/retry/drift/repair without unmanaged adoption. Import
uses D1's desired-state transaction, not a competing service.
**Accept:** fake provider failures/paging/uncertain create-delete/lease races/TTL
normalization, then controlled-zone evidence. Real credentials only for that fixture.

### D3 — Internal publisher and pinned CoreDNS

**Own:** `internal/dns/internaldns/**`, `internal/database/queries/dns_publication.sql`,
`deploy/coredns/**`, focused tests. Q explicitly releases that deploy subtree first.
**Deliver DN-04:** deterministic whole zones, durable serial/intent, per-zone
fencing/lock/atomic publication and restart/restore recovery.
**Accept:** filesystem faults before/after rename, concurrent names/zones/stale workers,
real pinned CoreDNS A/AAAA/CNAME/wildcard/fallback/reload/new-zone/final-removal queries.

### D4 — DNS frontend workflow

**Own:** `web/src/features/dns/**`, colocated adapters/hooks/tests.
**Deliver DN-05:** real selection/import/editor/preview/apply/status/repair/deletion
workflow, original revisions and stable uncertain-retry keys. No optimistic sync.
U0 integrates routes; U1/U2 own inventory/domain navigation edits or explicitly
transfer those files. Mocks assist development, not completion claims.

### DNS dispatch and G2

After D0, D1–D4 can run in parallel in four disjoint lanes. Q owns new
`integration/dns/**` and browser cross-system scenarios when a slot opens. Main
serializes queries/generation/runtime wiring. Provider/publisher workers depend
on the agreed D1 apply/projection contract, not improvised parallel models.

Cross-system acceptance covers independent public/internal failure, two names in
one zone and independent zones, same-revision lease reacquisition, stale in-flight
Cloudflare requests, tombstone finalization, last override, post-rename uncertainty,
inventory independence, controlled split answers and isolated restore without unknown
record adoption or serial regression. Q/main finish deployment/recovery documentation.

**G2:** M4/M5 exits, DN-01–DN-05 and DNS-aware RT-04 pass, including real external
evidence. No fabricated CoreDNS/provider passes, remote rollback promises, or capacity
scope expansion. Unavailable environments remain explicit blockers.

## 6. Copyable prompt for the main multi-agent orchestrator

```text
Resume Portinaia implementation using parallel coding subagents.
Read spec/README.md, spec/focused-delivery.md, spec/coding/parallel-progress.md,
spec/coding/parallel-interfaces.md and spec/coding/parallel-delivery-plan.md first.
Use the focused BE/DI/RT/DN requirements and coding rules for acceptance.

This is implementation, not another planning exercise. Preserve the current dirty
and untracked code. Do not reset/stash/clean, recreate the project, rewrite applied
migrations, re-key legacy data, or commit/push. Do not use clean HEAD worktrees that
omit the actual implementation. There are no active old work-package/audit backlogs.

Resume residual packets from the ledger, not their original full assignments.
Keep implemented, integrated, locally verified and externally verified distinct.
You own shared OpenAPI/migrations/generated/router/runtime/build/status files.
Only U0 owns Workspace/client/routes composition. Delegate exact file leases;
require shared change requests with signatures/consumers/tests before integration.

Run no more than four coding agents concurrently. Initial ready work is P1, A0,
U0 and P2's independent regressions. Publish shared interfaces early and release
waiting execution slots so P3, U1/U2/U3 and Q can proceed; resume foundation owners
for caller integration. Do not fill all slots with agents waiting on unlaunched work.

For each agent, provide its complete packet, owned files, relevant requirement IDs,
published interfaces, dependencies and focused acceptance tests. It must not modify
another owner's files, generated artifacts or shared status, and must return exact
changes, tests, integration requests and remaining blockers. Do not duplicate work.

You alone run generation and broad formatting/build/browser checkpoints on a stable
tree. After necessary make generate, inspect output and run make verify,
make test-e2e and git diff --check. Focused agent tests may run in disjoint areas.

Complete G1, including specific current-build Linux/Podman/service evidence, before
new DNS behavior. Existing historical Linux evidence is useful but not a blanket
pass for new changes. No physical-Pi capacity or arbitrary distro gate is required.
If external access is missing, finish independent local work and report exact
blocked checks/reproduction steps. Do not weaken requirements or bypass G1.

After G1, lead D0's shared DNS contract/storage agreement, then dispatch D1–D4 in
parallel. Coordinate Q's real CoreDNS/controlled Cloudflare/race/recovery acceptance
and finish G2/M5. Keep costs/history UI/PAT/purge/capacity work deferred.

Continue through ready packets rather than stopping at the first success. Resolve
implementation seams centrally; request an owner decision for an actual product
policy/protocol change and continue unrelated work while blocked.

Final report: packet and BE/DI/RT/DN acceptance status; changed files and compatibility;
exact command results; real external evidence versus fixtures/cross-builds; G1/G2
and M1–M5 status; blockers and next exact task. Update the existing progress/interface
ledgers and README to match verified facts, not intent.
```

## 7. Subagent handoff format

Main sends: packet ID/objective, exact owned files, relevant requirement IDs,
current implemented baseline to preserve, published interface signatures, dependencies,
and acceptance commands. Agents first recheck source, then implement residual behavior
and focused tests. Source-derived findings need regression reproduction, not blind fixes.

Agents return: implemented acceptance items, changed files, test commands/results,
shared contract/wiring requests, compatibility effects, blocked evidence and
`ready_for_integration` versus `partial`. They do not declare global gates complete,
run shared generation/build/Playwright, or edit outside their lease. Main reviews,
integrates, executes stable checkpoints and updates the sole current ledger.
