# Decision register

## User decisions recorded 2026-09-26

### D-01 — Separate volatile snapshot observations

Approved: deduplicate structural state separately from volatile observations.
Keep uptime and filesystem available-byte measurements in current state and
per-report metadata. Preserve those measurements for each retained report so
history/diff can reconstruct the observation; they are not part of the shared
structural content hash. Report identifiers, collection times, receipt times,
client/protocol versions also belong to the report envelope. Boot identity,
hardware capacity, mounts, interfaces, container state, and runtime IDs remain
structural state. WP-08 defines the exact field split. No raw HTTP bodies persist.

### D-02 — Four recurring intervals and monthly-equivalent totals

Approved intervals: monthly, quarterly, semiannual, annual. API values are `P1M`,
`P3M`, `P6M`, `P1Y`; SQL values remain `monthly`, `quarterly`, `semiannual`, `annual`.
Dashboard totals are monthly equivalents grouped by currency. Use exact rational
arithmetic, add amounts before rounding, and round half-up once per currency to
its minor unit. Include active recurring entries whose inclusive effective dates
contain today's UTC date and whose owner is active. Exclude one-time costs.
Label this as an estimate of monthly recurring cost, not a bill or cash-flow total.

### D-03 — Compute archival revokes discovery credentials

Archive the compute and revoke its agent credential and outstanding enrollment
tokens in the same audited transaction. Keep observations/history and agent ID.
Reject subsequent ingestion. Restore does not revive credentials: a new explicit
enrollment issues a new generation for the existing compute-bound agent.

### D-04 — Real protocol compatibility, starting at v1

First release supports protocol v1 only, accepted range `[1,1]`. Do not fabricate
v0. When v2 is introduced, support v2 and v1 using explicit boundary adapters.
Binary version output must distinguish build version, API version, protocol
version, and canonical format version. The canonical format has its own version.

### D-05 — Capacity reference

Reference: Raspberry Pi 4, 4 GB RAM, SSD, 64-bit Linux. Record CPU frequency,
thermal/throttling state, SSD/filesystem, OS, container limits, and CoreDNS version
in results. Emulator or desktop tests do not establish this hardware gate.
The owner approved the WP-16 load, page/detail latency, snapshot latency, memory
and internal-publication budgets. They are acceptance thresholds, not measured
achievements. Storage budget awaits representative measurement and owner approval;
heartbeat/retention timing suggestions remain engineering targets.

### D-06 — Purge requires explicit dependent cleanup

Do not cascade-purge archived subtrees or retained reports. Require the target
to be archived and block purge while any dependent resource, DNS link, retained
report, or other blocking FK reference remains. Return typed dependency counts
and navigable identifiers through bounded responses. Remove children explicitly;
wait for configured report retention. Purge may remove target-owned join/fact
rows and already expired/revoked credential records once no dependent history
remains. Audit records survive, without resource foreign keys.

## Engineering conventions selected for consistent implementation

These refine the approved behavior; change them centrally, with tests, rather
than choosing a different convention in each feature.

- Preserve SQLite's whole-second UTC timestamp convention. Normalize incoming
  RFC 3339 UTC fractional values to seconds before persistence; compare/order by
  time plus stable ID. API may serialize whole seconds. Do not use timestamps
  instead of integer revisions.
- Container identity uses compute ID, verified numeric Podman owner UID, and
  exact container name. Connection names are configuration/display labels.
  Reject duplicate configured owners on one host to avoid overlapping authority.
  Rename means a different logical container; recreation does not.
- Rotation revokes the current credential immediately and issues a one-use
  enrollment token; no plaintext long-lived credential is exposed to the browser.
  Lost successful enrollment responses require a new enrollment token.
- Zone disable blocks new drafts/applies/imports but preserves existing desired
  state, files, and ongoing repair of that desired state. Re-selection re-enables
  changes. No implicit provider deletion.
- DNS owner/zone are immutable after aggregate creation; rename/move uses an
  explicit delete draft and separate create draft. This remains single-name work.
- An active DNS name must have at least one record set across its views. Removing
  the last set requires a delete draft. Removing only one view still schedules
  cleanup of that view before its projection becomes not applicable.
- DNS names are stored as lower-case ASCII without the terminal dot. Zone files
  use absolute trailing-dot names. Normalize IDNs at the server boundary.
- Internal publication uses a zone-wide generation counter separate from each
  DNS-name revision; publication of the final removal writes a valid metadata-only
  zone, rather than leaving stale answers or deleting a loaded file.
- Human idempotency results never persist plaintext secrets. Secret-issuing
  endpoints must not use generic response replay; WP-02 removes their optional
  idempotency promise and documents one-shot recovery.
- Query scopes are exact capabilities: write does not imply read and `admin`
  does not mean every scope. Purge additionally requires `admin`; update the
  existing inventory-only purge contract before implementation.

### D-07 — Tested CoreDNS version upgrade permitted

The owner permits selecting and pinning a CoreDNS build that passes actual
authoritative-file/auto fallthrough and reload integration tests. Document the
minimum supported version and exact tested image/binary checksum. External
deployment automation owns any host upgrade. No need to preserve an unknown
older deployed version; successful target-version testing remains mandatory.

## Remaining evidence and follow-up gates

| ID   | Question / evidence needed                                                                                                                          | Blocks                                  |
| ---- | --------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------- |
| F-01 | Measure representative history/audit storage and obtain a numeric storage budget. Approved latency/load/memory/publication thresholds are in WP-16. | Capacity release sign-off               |
| F-02 | Select/pin a compatible CoreDNS version under D-07 and record real integration evidence. Upgrade permission is granted.                             | WP-11 deployment compatibility sign-off |

The current upstream `file` documentation explicitly lists `fallthrough`; `auto`
documents support for all `file` directives. This is not proof that an older host
binary supports it. Sources checked 2026-09-26:
[file](https://coredns.io/plugins/file/) and
[auto](https://coredns.io/plugins/auto/). WP-11 must run real DNS queries against
the pinned target build, including absent names and final-override removal.
