# WP-15 — Operations, deployment and recovery

Dependencies: WP-03/WP-08/WP-10/WP-11. Sources: product §§15–17;
DNS §25; implementation plan §§6, 18, 21/WP-15.
Read [shared rules](coding-rules.md).

Primary paths: administrative `cmd/portinaia` wiring and domain command services,
`deploy/examples/`, operational documentation, `Containerfile`, `Makefile` release
targets. Baseline: image/cross-build recipes and embedded migration helper only;
no deployed config, backup/check/repair CLI or restore runbook.

## WP-15.1 — Database and repair commands

1. Complete `db migrate`/`db status` from WP-03. Document pre-start explicit
   migration, backup prerequisite, supported upgrade versions, exit codes and
   refusal of newer unsupported schemas. Do not expose destructive migration-down
   as a routine production command.
2. `db check`: integrity_check plus foreign_key_check, bounded safe output, nonzero
   on corruption/violations; it must not “repair” data by deleting rows.
3. `db backup`: modernc-supported SQLite online backup API or proven equivalent
   consistent SQLite snapshot mechanism, not copying a live .db without WAL.
   New destination only; overwrite requires explicit flag. Bound busy retry and
   cancellation, verify backup integrity/FKs, flush/close output and report success
   only afterward. Protect file permissions; failed output never masquerades as
   verified backup. Source remains usable throughout.
4. `dns reconcile`: audit an explicit global desired-state repair request and
   enqueue current revisions in bounded batches. CLI reports enqueued counts,
   failures and continuation; it does not synchronously rewrite all provider state.
   No automatic import of unknown Cloudflare records.
5. Add snapshot-retention reporting/dry-run command from WP-08. It reports cutoff,
   candidate reports/content and bounded estimates; no implicit audit/history purge.

## WP-15.2 — Deployment examples and hardening

Document runnable versioned server/agent config, externally managed reverse proxy
and trusted CIDRs, dedicated metrics listener, SQLite directory mount (including
WAL/SHM), secrets, and generated-zone directory shared with host CoreDNS. Explain
UID/GID 65532 ownership and read-only config/secret mounts. Scratch runtime needs
CA bundle, writable dedicated temp/state paths as applicable and no shell utilities.
Container runs as non-root with only required mounts/capabilities; do not add
privileged mode to solve permissions. Agent remains root-owned system service.

CoreDNS and Unbound config examples are static external deployment inputs. Runtime
zone files belong to Portinaia and are never rendered by Ansible. Agent example
variables describe pinned binary URL/checksum, socket names/UIDs, CA/server URL and
root-only token delivery; do not create roles in this repository. No TLS certificate
management, reverse-proxy installation or backup scheduler in the application.

Release: pinned image builds for Linux amd64/arm64, agent binaries/checksum manifest,
version metadata, config/service/CoreDNS examples and install/upgrade instructions.
Verify native arm64 startup with real mounted permissions. Existing cross-build
files are not proof of successful target execution.

## WP-15.3 — Backup/restore drill and DNS continuity

Runbook sequence:

1. Record software/schema/config versions; create verified online backup during
   concurrent report ingestion and desired DNS edits. External tooling owns
   encryption, off-host copy, schedule and retention of DB/config/secrets/zones.
2. Stop Portinaia. Preserve failed/current DB and sidecars for investigation.
3. Restore verified DB with correct owner/mode and matching config/secrets; avoid
   mixing stale WAL/SHM from the old database with restored file.
4. Run `db check` and `db status`, explicitly migrate if necessary, then start.
5. Verify local readiness, admin access, current inventory and retained history.
6. Run global DNS repair; verify each provider status independently. Recovery must
   compare existing zone serial/checksum and provider ownership safely; ambiguity
   fails visibly rather than adopting unknown records.
7. Confirm real internal DNS answers/fallthrough and fake/approved public provider
   state. CoreDNS kept serving previous files and Cloudflare kept last state during
   downtime. Revoked credentials restored from an older backup require explicit
   operational review/revocation; document backup point-in-time semantics.

## Acceptance / tests

- OPS-01: CLI argument/config errors and exit codes tested; no command prints
  passwords/tokens or sensitive secret-file contents.
- OPS-02: Online backup under writes yields a consistent integrity-checked DB;
  existing destination protected, cancellation/failure does not corrupt source.
- OPS-03: Populated upgrade and restore drill preserve data; repair does not adopt
  unknown records; last valid internal serving continues during outage.
- OPS-04: Non-root Linux amd64/arm64 image startup with documented mounts, agent
  installation and checksum verification demonstrated.

Commands: `go test ./internal/database/... ./internal/dns/... ./cmd/...`,
`go test ./integration/tests/... -run 'TestBackup|TestRestore|TestRecovery'`,
`make verify`, `make image`, `make release`. Record native hardware evidence separately.
