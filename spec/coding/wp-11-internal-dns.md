# WP-11 — Authoritative CoreDNS publication

Dependencies: WP-09; independent of WP-10 network success.
Sources: DNS §§7.3, 19–20, 25, 27.4; implementation plan §§15.5, 21/WP-11.
Read [shared rules](coding-rules.md), decision F-02 and zone-generation convention.

Primary paths: `internal/dns/internaldns/`, publication SQL,
`deploy/examples/coredns/`, real CoreDNS integration fixtures/tests.
Baseline: dns_publications table only, no renderer/filesystem publisher.

## WP-11.1 — Zone state and deterministic rendering

1. Increment zone-wide internal desired generation in each DNS apply transaction
   affecting internal state. Do not compute it as max(name revision): different
   names can share revision numbers while changing the zone independently.
2. Claim/coalesce internal name work into one zone publication. Read one consistent
   desired DB snapshot with zone generation and contributing name revisions.
   Include only active internal sets plus mandatory SOA/NS, no public-only records.
3. Render deterministic names/types/values and TTLs with `miekg/dns`, configured
   SOA nameserver/mailbox, absolute names, valid apex and wildcard syntax. Fixed
   output contains no wall-clock comments that change bytes on a no-op.
4. Allocate durable serial per zone publication attempt so a new file version
   advances using RFC 1982 unsigned 32-bit serial arithmetic. Retries of identical
   intended publication reuse allocated serial/content; deliberate rebuild after
   restore reconciles existing file serial to avoid serial regression. Serial
   rollover and no-op behavior require tests.
5. Parse rendered content before publication; verify origin, SOA/NS presence,
   supported records and containment. Hash bytes, with checksum stored separately
   or omit the checksum field itself from any comment hash definition.
6. Final override deletion generates metadata-only zone with advanced serial.
   It removes old internal answers while preserving a valid loaded file/fallthrough.

## WP-11.2 — Atomic publication and stale-writer protection

1. One zone-scoped in-process publication lock covers generation coordination,
   final generation check and rename; DB generation updates in apply participate
   in the same lock ordering for that zone. Keep write transactions short; no
   filesystem work inside them. Locks never serialize unrelated zones.
2. Durable lease/fence protects scheduling across crashes, but DB lease alone
   does not prevent a stale filesystem rename. Single server process plus zone
   lock and check-before-rename ensure no older worker overwrites a newer file.
   Reject a second publishing server using deployment/process lock discipline.
3. Filenames derive only from validated selected zone identity and canonical zone
   name; use a fixed safe pattern such as `db.<zone-uuid>.<ascii-zone>`. Reject
   directory traversal/symlink destinations. Dedicated directory only.
4. Create unique temporary file in destination filesystem, write all bytes, set
   least required permissions (typically 0644 in restricted directory), flush,
   close, recheck generation/fence under zone lock, atomically rename, flush parent.
5. Only after successful rename record checksum/generation/serial/publication time
   and mark matching contributing name revisions applied. Failed DB metadata write
   after rename is uncertain success: detect existing checksum/serial on retry,
   do not pretend no file was published or overwrite with older state.
6. Render/parse/write/close/rename failure preserves previous valid file. Directory
   fsync failure after rename cannot restore the prior file guarantee: keep the new
   valid file, report durability uncertainty, and retry/reconcile metadata. Tests
   must distinguish pre-rename preservation from post-rename recovery.
7. Shutdown cancels intake; incomplete temp files are safely cleaned on next start
   using owned filename patterns. Never remove last valid published zone on failure.

## WP-11.3 — CoreDNS integration and deployment contract

Provide a pinned tested CoreDNS version with `auto` directory discovery or explicit
`file` entries supporting `fallthrough` plus forwarding to configurable Unbound.
Use anchored filename matching that ignores temp/backup files and extracts the
validated origin. New selected zones must become available without Portinaia
editing host Corefile or executing reload hooks. External automation owns static
config, directory permissions and service deployment.

Integration test launches real pinned CoreDNS and deterministic public upstream:
internal A/AAAA/CNAME/wildcard override, public-only absent-name fallthrough,
unknown names, apex metadata, missing QTYPE behavior, file reload with serial
advance, new-zone discovery, final-override removal and Portinaia shutdown.
Document tested NODATA/fallthrough behavior rather than assuming every QTYPE falls
through identically. Internal CNAME must retain actual CNAME semantics.

## Acceptance / tests

- INT-01: Golden zones are deterministic, parseable and complete; invalid desired
  input never reaches destination; public-only names are absent from override data.
- INT-02: Inject every pre-rename failure and prove last valid bytes unchanged;
  inject post-rename DB/fsync failure and prove safe recovery without old overwrite.
- INT-03: Race two changes on different names of one zone and different zones;
  newest generation wins, serial advances, stale workers cannot mark current applied.
- INT-04: Real CoreDNS query matrix proves overrides/fallthrough/reload/last-file
  continuity; Cloudflare fake outage does not block publication.
- INT-05: Pinned build/version and deployed compatibility recorded (F-02).

Commands: `go test ./internal/dns/internaldns/... ./internal/database/...`,
`go test ./integration/tests/... -run TestCoreDNS`, `make verify`.
CoreDNS-required tests fail or explicitly report missing prerequisites; a skipped
test does not satisfy INT-04. No hosts-plugin replacement, hooks or host config edits.
