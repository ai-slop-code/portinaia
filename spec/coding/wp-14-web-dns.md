# WP-14 — Frontend DNS workflows

Dependencies: WP-09/WP-10/WP-11 and WP-12 shell. Sources: DNS §23;
product §13.3; implementation plan §§16.7, 21/WP-14.
Read [shared rules](coding-rules.md). Primary path: `web/src/features/dns/`.
Baseline: generated types only, no DNS screens.

## WP-14.1 — Zones

Paginated zone list shows available/managed/disabled plus degraded visibility.
Discover refreshes visibility only; it never auto-selects or imports. Selection
lets user choose existing matching domain or explicitly create it using inventory
API, then select zone with latest ETag. Failure after domain creation explains the
remaining selection step rather than pretending the entire operation was atomic.

Zone detail shows Cloudflare connectivity, internal generation/publication age,
pending/failed counts and drift. Disable confirmation says no new desired edits,
existing desired state continues repair and serving; reselect enables edits.
Never expose Cloudflare token or credential paths.

## WP-14.2 — Selective import

1. List refresh-scoped candidates grouped by DNS name/set with type/value/TTL/proxy
   and explicit non-importable reasons. Unsupported records still render safely.
2. User selects individual compatible sets/names. Persist selection only within
   that observation fingerprint; new refresh invalidates stale selection/preview.
3. Preview exactly all selected effects and ownership changes. Apply disabled
   while loading/invalid/conflicting; mandatory idempotency key remains stable
   across an uncertain retry of the same request, changes with a new selection.
4. On success show adopted names and synchronized public status; internal is
   not applicable unless separately configured. Do not claim import rewrites
   provider values. Stale preview returns to explicit refresh/re-preview.

## WP-14.3 — DNS name list, detail and links

One row per aggregate, grouping public/internal state, zone/name, type, wildcard,
proxy and independent projection chips. URL search/filter/cursor uses generated
API filters; expose pending/synchronized/failed/deleting/drift/unmanaged conflict.
Inventory links show archived resources explicitly and navigate without changing
DNS values. Detail includes desired/applied revision, attempts/retry/error and
audit link. Deleted names retain auditable metadata; no immediate purge button.

## WP-14.4 — Side-by-side editor, preview and apply

1. Create/update/delete produces drafts; no direct desired mutation. Existing
   name/zone cannot be renamed in place; UI explains separate create/delete flow.
2. Side-by-side views support multiple unique A/AAAA values, CNAME exclusivity,
   wildcard owner, TTL, public-only proxy. CNAME at apex/self-target is invalid.
   Defaults TTL 60 and proxy off; separate empty view from deleted whole name.
3. IP suggestions come from paginated inventory queries. Copy selected address into
   form; show source resource separately. Future observation changes do not alter
   form/desired state. Explain public non-routable rejection; no warning override.
4. RHF maps generated typed unions, validates usability and renders server field
   issues. Save/replace draft with ETag. Any edit invalidates local preview/apply
   eligibility until new server preview succeeds.
5. Preview dialog separately lists Cloudflare and CoreDNS creates/updates/deletes/
   unchanged values, warnings, unmanaged conflicts and exact base/draft revisions.
   Display complete bounded preview, not a hidden truncated action list.
6. Explicit apply confirmation sends draft ETag, expected base revision, fingerprint
   and stable Idempotency-Key. 202 means desired state accepted; close edit mode
   with pending status, never show an unconditional “DNS synchronized” toast.
7. 409 stale/conflict and 412 revision errors preserve form, explain reason and
   require reload/re-preview. Uncertain network outcome can retry the same request
   and key; it must not create a new desired revision by generating another key.

## WP-14.5 — Synchronization, deletion and retry

Poll active detail initially every 5 seconds while pending/deleting, stop/slow on
terminal states and pause when hidden. Render independent providers: Cloudflare
failed/internal synchronized is a valid visible partial outcome. Manual retry
enqueues current desired state with confirmation/context; it does not edit values.

Delete uses the same preview/apply path and stays deleting until applicable
projections confirm. Show failure/retry on a tombstone; do not remove the name from
view as if cleanup succeeded. Revert is another explicit draft, not UI rollback.
Mobile stacks view panels and preview groups; all controls remain functional.

## Acceptance / tests

- WDN-01: Discover/select/disable and domain matching; selective import with
  rejected candidates, stale fingerprint and unchanged unselected records.
- WDN-02: Multi-value A/AAAA, CNAME conflicts, wildcard, public/private validation,
  inventory suggestions and per-view proxy control.
- WDN-03: Edit invalidates preview, stale revision blocks apply, same-key uncertain
  retry is stable, accepted state is distinct from provider synchronization.
- WDN-04: Independent failure/retry, tombstone deletion and visible stale polling
  state; mobile editor/preview remains usable and keyboard accessible.

Commands: `make test-web`, `npm run typecheck`, `make test-e2e`, `make verify`.
Browser tests run against deterministic fake Cloudflare and actual generated-zone
fixtures. No live provider credentials or optimistic DNS mutation.
