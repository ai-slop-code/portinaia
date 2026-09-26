# WP-13 — Frontend agents and history

Dependencies: WP-06/WP-08 and WP-12 shell. Sources: product §13.3;
implementation plan §§16.6, 21/WP-13. Read [shared rules](coding-rules.md).

Primary paths: `web/src/features/agents/`, inventory history-tab components,
route registration and browser tests. Baseline: no agent/history screens.

## WP-13.1 — Agent status

1. `/agents` paginated list filters online/offline/revoked, version compatibility
   and compute where supported. Display compute link, protocol/client version,
   enrollment, last heartbeat, last accepted report, last fully successful snapshot,
   and collector errors; do not label partial success as no activity.
2. Detail shows compute binding, credential generation, safe compatibility warning,
   collector results and history link. First release correctly says v1 supported;
   no fictitious previous protocol support.
3. Poll active visible status view initially every 10 seconds; pause hidden tabs,
   cancel on navigation/logout, back off on errors and show last update time.
   Retain last known data with stale/error indicator rather than resetting to empty.

## WP-13.2 — Enrollment, revoke and rotation

Compute-bound enrollment dialog identifies the target compute and configurable
expiry, explains deployment via external Ansible/systemd and shows plaintext token
once. Keep secret in local component state only; no TanStack persistent cache,
URL or logs. Close/navigation/logout clears it. Reopen requires a fresh issuance
workflow, not redisplay. Lost response explains revoke/reissue, no blind create retry.

Revoke and rotate require current ETag and explicit confirmation. Rotation explains
immediate old-credential invalidation and issues an enrollment token, not a usable
agent credential. Archived compute cannot enroll; restored compute needs explicit
re-enrollment. Refetch agent, compute summary and token status after acceptance.
Conflict preserves context and offers reload; never silently rotate twice.

## WP-13.3 — Timeline, export and comparison

1. Compute/agent timeline uses cursor pagination, receipt date in browser timezone,
   collection diagnostics, accepted-with-errors and content reuse badge. A reused
   structural snapshot can still differ in volatile measurements; explain that.
2. Container history filters by stable logical identity via approved backend
   endpoint, not by downloading all host snapshots. Recreation displays runtime-ID
   transition on same inventory identity.
3. Select exactly two retained reports of same compute; URL stores server history
   IDs and comparison options. Disable invalid selection and handle expired report
   with a clear message/reselection action.
4. Render structured side-by-side groups for host, interfaces/IPs, disks,
   filesystems, containers and collectors; distinguish added/removed/changed from
   unknown due to collection failure. Volatile measurements form a labelled section.
   No raw HTML rendering or fake removal for failed scans.
5. Paginate/continue large diff sections; display explicit limits. Export streams
   server gzip download; do not put complete snapshots in long-lived query cache.
   Mobile stacks before/after values while retaining field identity and change type.

## Acceptance / tests

- WHI-01: Polling changes online/offline/error states and stops while hidden or
  unmounted; API failure preserves last known timestamp and retry affordance.
- WHI-02: Token/rotation plaintext cleared on close/navigation/logout; repeated
  issuance never reveals a previous secret; revision conflict needs reload.
- WHI-03: Recreated container diff shows changed runtime ID, no duplicate resource;
  failed collector section is unavailable rather than all children removed.
- WHI-04: Same structural content/different uptime renders measurement differences;
  pagination, expired reports, gzip export and mobile comparison work.

Commands: `make test-web`, `npm run typecheck`, `make test-e2e`, `make verify`.
No remote command buttons, agent self-update controls, or historical editing.
