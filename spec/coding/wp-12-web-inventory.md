# WP-12 — Frontend foundation, inventory, audit and settings

Dependencies: WP-04/WP-05; shell can start after WP-02. Sources: product §13;
implementation plan §§16.1–16.5, 16.8, 21/WP-12. Read [shared rules](coding-rules.md).

Primary paths: `web/src/app/`, `routes/`, `components/`, `theme/`,
`features/auth/`, `dashboard/`, `inventory/`, `providers/`, `locations/`, `domains/`,
`audit/`, `settings/`; shared `api/client.ts` coordinated. Baseline: title page,
theme, generated types/client; routing/query/form libraries already installed.

## WP-12.1 — Shell, session and shared controls

1. Preserve theme/embedding. Compose ThemeProvider/CssBaseline, QueryClientProvider,
   React Router, route error boundary and authenticated shell. Bootstrap session
   before protected content; login route with generic credential/limit errors.
2. Primary routes: `/`, `/inventory`, `/providers`, `/locations`, `/domains`,
   `/dns`, `/agents`, `/audit`, `/settings`. Detail paths use stable resource IDs;
   compute/container tab selection is addressable. Not-found is a real UI state.
3. Responsive drawer/top bar, breadcrumbs/title, loading skeleton, empty/create
   state, durable error/retry panel, snackbars and accessible confirmation dialog.
   MUI controls remain keyboard-operable and labelled; focus returns after dialogs.
4. Shared API adapter handles CSRF, RFC 7807 field violations/request ID, no-store
   auth requests, session expiry and network errors. Logout clears session/cache.
   Distinguish 401 from offline network failure; do not force logout on every 5xx.
5. Common list URL state: q, filters, archived mode, page size, opaque cursor.
   Reset cursor when filters change. Browser back restores filter state; cancel
   obsolete requests. Forms hold original ETag and field edits through conflicts.

## WP-12.2 — Dashboard

Use `/dashboard/summary`; no full-collection download or browser aggregation.
Cards: offline computes, stale containers/IPs, expiring domains, pending/failed
DNS projections, monthly-equivalent recurring costs per currency. Each links to
the matching filtered list. State the cost basis and currency; no grand FX total.
Show unknown/no-data separately from zero when API is unavailable. Refresh after
relevant mutations; bounded lightweight polling only where status warrants it.

## WP-12.3 — Providers, locations, domains and cost forms

| Screen    | List/detail/form requirements                                                                                                                                                         |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Providers | Name/search, roles, website, account reference, notes/tags, archived toggle; roles multiselect; show dependencies before purge                                                        |
| Locations | Type/provider/parent filters and editable site/region metadata; parent selector excludes invalid cycles; server remains authoritative                                                 |
| Domains   | Name/registrar, dates, tri-state auto-renew, nameservers, tags/notes, expiry chips; matching managed-zone link; no registrar operations                                               |
| Costs     | Owner compute/domain, recurring vs one-time discriminated form, amount text/currency, four billing intervals, effective dates or charge date, reference/description; multiple entries |

Use React Hook Form for complex forms; initialize from generated responses with
explicit transport/form mapping. Amount entry parses decimal text with ISO exponent
to integer minor units, rejects excess decimals/unsafe integers. Type switching
removes incompatible fields. Date-only inputs preserve calendar dates. Archive
and restore are available on each applicable screen; tag selectors use paginated
search and display existing archived tags without permitting new assignment.

## WP-12.4 — Inventory table and hierarchy

Switch hierarchy/table through URL state. Tables cover computes, containers and
IPs with server pagination/search and relevant provider/location/kind/owner/source/
freshness/tag/archive filters. Do not load all data to filter in the browser.

Hierarchy expands bounded server branches with loading/error/continuation states;
show site -> compute -> VM -> container containment and explicit unplaced roots.
An archived ancestor needed to explain active descendants is visually contextual.
Avoid duplicate VM placement when it has both location and parent metadata.
Mobile tree and tables remain navigable without hover-only actions.

## WP-12.5 — Compute, container and IP detail

- Compute tabs: overview/manual metadata, host facts, network/IPs, containers,
  costs, DNS links, snapshots, audit. Show display name and observed hostname
  separately; collector errors/freshness do not look like a confirmed empty host.
- Container tabs: runtime/image/state/health/restart, ports/networks, mounts,
  manual metadata, DNS links, history and audit. Runtime ID changes retain page ID.
  No human create/start/stop/delete-runtime controls.
- IP list/details show owner/interface/network/address/prefix, manual/discovered
  source, scope/role/primary and observation times. Edit actions only for manual
  values under the reconciled API; explain discovered read-only fields.
- DNS links navigate to DNS editor/detail; copying an inventory IP is a suggestion,
  not a live value binding. History tabs reuse WP-13; audit uses bounded filtered
  audit query. Unimplemented dependency tabs cannot count as feature completion.

## WP-12.6 — Lifecycle and concurrency UX

Archive confirmation explains compute credential revocation where applicable.
Restore explains re-enrollment requirement. Purge is separate high-impact action
on archived resources, requiring admin, explicit confirmation and server dependency
validation. Show blockers and links; do not cascade-delete or silently retry.
Archive/restore/purge invalidate detail, parent tree, affected lists/dashboard and
agent status as appropriate. No optimistic removal before server acceptance.

412 preserves edits and offers reload; 409 shows the named conflict/blocker. Neither
automatically submits with a newer revision. API field errors attach to inputs;
cross-field/domain errors remain visible at form level with request ID.

## WP-12.7 — Audit and settings ownership

These required navigation areas are explicitly assigned here:

- Audit: paginated time/actor/action/resource filters in URL, redacted before/after
  field view, links where resource still exists, graceful purged-resource display.
- Settings: named PAT list/create/revoke with scopes, optional expiry, last-used;
  creation dialog displays plaintext only in local component state, clears on
  close/navigation/logout, never in query cache/analytics/URL/storage. Offer copy
  with clear once-only recovery instructions. Revocation confirmation and errors.
- Show operational/version information already available through approved contract;
  do not invent mutable runtime-config/password-reset endpoints. Explain admin CLI
  reset where necessary. No user management or provider-token editor.
- Tag maintenance may live under Settings with resource assignment controls; use
  the same list/form/lifecycle patterns as other resources.

## Acceptance / tests

- WEB-01: Session bootstrap/login/logout/expiry, protected deep links, CSRF,
  401-vs-network failure and no stale cache flash.
- WEB-02: Each resource create/edit/list/archive/restore workflow, form validation,
  exact money/date behavior, unknown states and server field errors.
- WEB-03: URL filters/back/pagination and hierarchy continuation use server data;
  manual/discovery fields clearly distinct and only approved writes sent.
- WEB-04: 412 keeps edits, purge shows blockers, compute archive explains revocation,
  dashboard values navigate to matching source filters.
- WEB-05: Audit/PAT/tag maintenance, once-only secret lifecycle and token revocation.
- WEB-06: At 390px viewport, reads/basic edits/archive/restore work with keyboard/
  touch; dense factual tables scroll without hiding actions.

Commands: `make test-web`, `npm run typecheck`, `npm run lint`, `make test-e2e`,
`make verify`. Mock at the shared HTTP boundary with generated schemas; do not
replace tested hooks with local arrays that bypass the actual API adapter.
