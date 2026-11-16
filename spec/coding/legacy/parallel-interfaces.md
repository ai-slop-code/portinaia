# Current integration seams

Updated 2026-09-30. This records **existing helpers and unresolved wiring**, not new
requirements and not a claim every interface is approved or integrated. Main owns
contract/generation/router/runtime changes. Keep authoritative policy in the focused
requirements and [decisions](decisions.md), not in individual handoff notes.

## Backend transport and runtime

**Existing:** shared bounded strict decode, null checks, problem and request-ID
helpers, pre-handler UUID rejection, required-member/bounds helper tests, complete
Provider response schema, and migration 00010/generated replay queries.

**Next interface checkpoint:** operation-wide raw validation (including valid zero
versus missing), complete response matrix, actual filters/cursors, one problem/
correlation path, mounted execution timeout/cancellation and auth ordering. The old
Provider-schema and post-handler-UUID change requests are resolved, not pending.

Replay storage exists. Publish a transaction API for lookup/conflict/expiry/mutation/
audit/result with active caller integration; fingerprint helpers alone are insufficient.
Return prior success before rechecking stale revisions; never replay issuance secrets.

## Principal and audit

**Existing helpers:** `Principal{Class, ID, Scopes}`, exact capability methods and
credential-class rejection; `AuditEntry`, `AppendAuditTx`, audit query/read structs.
Bootstrap/reset and the system-audit helper already use the append path.

**Unresolved, not normative replacements:**

- A human principal with only `admin` cannot satisfy exact read/write requirements
  implicitly. Agree the explicit sole-admin capability set and mount it.
- Current audit keyword redaction must become an audited field allowlist. Canonical
  system actor is the reserved ID in decisions; new events need correlation IDs.
- Current `AuditFilter{..., Limit, Cursor}` / `AuditPage{Items, NextCursor}` do not
  alone match generated PageInfo/operation filters. Agree actor/resource/action/time
  filtering, stable paging and truthful legacy request-ID/actor representation before
  wiring `GET /audit-events` and the already-existing audit UI.
- Missing historical request IDs stay explicitly unknown; do not rewrite immutable
  history or fabricate a past correlation to satisfy a DTO. New events follow policy.
- P2/P3 adopt the shared helper inside their own transactions. They do not let P1
  edit their files concurrently. Main wires routes/capabilities and generated shapes.

## Agent and server protocol

**Existing:** context-bearing HTTP/Podman methods, paired worker cancellation/join,
private atomic file writes, fresh pending replacement, matching-ID clear helper,
Retry-After/permanent HTTP classification and stricter response checks.

**Next agreement:** independent collection cadence during upload backoff; safe
exclusive state ownership or synchronized compare/delete; full required-member
validation including booleans/range bounds/pending unions/trailing credential data;
schedule suggestions versus local overrides; diagnostics and permanent local errors.
Backend P3 and A0 propose together; main changes OpenAPI only after agreement.
Preserve valid UID 0 and explicit empty authority; omission never silently fabricates it.

## Frontend

**Existing:** Query/Router providers, shared session generation/abort/error/form
snapshot primitives, detail request counters, extracted domain adapter, audit UI,
discovery hook and safer enrollment hook.

**Next integration contract:**

1. Transport combines session cancellation with `init.signal` **and** the incoming
   Request signal; callers guard the entire decoded-result/state-commit lifetime.
2. Queries include resource/filter/cursor/session; no first-200 shortcut replaces
   true continuation. Reference selectors own independent search/page/hydration.
3. Edit callbacks carry immutable `{ID, original ETag, initial values, preserved
metadata}`; U0 must use the snapshot rather than discard it or read live selection.
4. Domain/audit/discovery/enrollment components are mounted and competing inline
   logic removed. Secrets remain outside cache and are discarded after context changes.
5. Field/network/problem errors distinguish unavailable from empty/zero and support
   explicit recovery. Audit filters and date formatting match the resolved backend.

U0 alone edits App/Workspace/client/routes during current integration. U1/U2/U3
return component contracts and tests; request caller changes from U0 instead of
duplicating orchestration. All feature exports should identify invalidated queries.

### Resumption checkpoint

U0 published `web/src/app/integration.ts` types: `IntegrationQuery`, `PageSnapshot`,
`EditSubmission`, `DomainEditSubmission`, `InventoryEditSnapshot<TValues>` with
`id`, `etag`, `initialValues`, and `preservedTagIDs`, plus per-resource inventory
snapshot/submission types and `ReferenceSelectorContract<T>` with separate page and
selected-ID hydration. This publishes the contract only; U1 reports that the current
feature callbacks do not yet receive the caller snapshot end-to-end. U0 owns passing
these immutable snapshots and callbacks through Workspace.

U3's `useComputeDiscovery` now returns all paged containers/discovered IPs, preserves
partial diagnostics and errors, and scopes enrollment secrets to compute/session
context. Workspace mounting remains U0-owned and incomplete. P1's audit page/filter
helper and OpenAPI `/audit-events` contract exist, but the server route and truthful
legacy request-ID/actor DTO agreement are not integrated. Do not claim the audit UI
as an end-to-end workflow until main resolves that seam and mounts route capabilities.

## Coordination

Existing helper names are starting points, not permission to change requirements.
Before a breaking signature change, record proposed types, consumers and tests here;
main publishes one checkpoint and consumers adopt it. Coordinate generated changes
only through main. DNS interfaces remain a separate post-G1 checkpoint described in
[DN-01](dns-implementation.md). No helper handoff grants permission to skip G1.
