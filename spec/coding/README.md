# Coding-agent specification index

Status: implementation instructions; baseline reviewed 2026-09-26.

## Start here

1. Read [implementation audit](implementation-audit.md). Existing schemas and
   generated interfaces are not implemented business features.
2. Read [decisions](decisions.md) and [shared coding rules](coding-rules.md).
3. Read the selected work-package file below and its normative source sections.
4. Complete its dependencies, including contract corrections, before behavior.
5. Implement one numbered feature slice at a time, with its failure tests.

These files supplement [the implementation plan](../implementation-plan.md),
[product requirements](../high-level-plan.md), and [DNS specification](../dns-spec.md).
Product behavior in those documents wins. User-approved amendments are recorded
in those documents and in the decision register. An unresolved contradiction
blocks the affected slice; it is not permission to invent behavior.

All paths in task instructions are repository-relative. All commands run at the
repository root. New package/file paths describe future implementation, not
files found during the audit. No application implementation is included here.

## Work packages and ordering

| ID    | Task specification                                      | Dependencies                   | Audited baseline                                      |
| ----- | ------------------------------------------------------- | ------------------------------ | ----------------------------------------------------- |
| WP-00 | [Specification baseline](wp-00-baseline.md)             | None                           | Amendments recorded; contract reconciliation required |
| WP-01 | [Repository/build foundation](wp-01-foundation.md)      | WP-00                          | Present; supported image execution unverified         |
| WP-02 | [API/database contract](wp-02-contracts.md)             | WP-01                          | Substantial but inconsistent                          |
| WP-03 | [Runtime foundation](wp-03-runtime.md)                  | WP-02                          | Health/static-server stub only                        |
| WP-04 | [Authentication/audit](wp-04-auth-audit.md)             | WP-03                          | Tables/interfaces only                                |
| WP-05 | [Inventory backend](wp-05-inventory.md)                 | WP-04                          | Tables and provider read/create queries only          |
| WP-06 | [Enrollment/protocol server](wp-06-discovery-server.md) | WP-05                          | Contract and selected queries only                    |
| WP-07 | [Discovery client](wp-07-agent.md)                      | WP-02; WP-06 for integration   | Version command only                                  |
| WP-08 | [Snapshot history](wp-08-history.md)                    | WP-06                          | Tables and selected queries only                      |
| WP-09 | [DNS domain/drafts](wp-09-dns-domain.md)                | WP-05, DNS WP-02               | Tables/interfaces only                                |
| WP-10 | [Cloudflare/reconciliation](wp-10-cloudflare.md)        | WP-09                          | Projection query primitives only                      |
| WP-11 | [CoreDNS publisher](wp-11-internal-dns.md)              | WP-09                          | Publication table only                                |
| WP-12 | [Frontend foundation/inventory](wp-12-web-inventory.md) | WP-04/WP-05; shell after WP-02 | Placeholder UI and typed client                       |
| WP-13 | [Frontend agents/history](wp-13-web-history.md)         | WP-06/WP-08/WP-12 shell        | Absent                                                |
| WP-14 | [Frontend DNS](wp-14-web-dns.md)                        | WP-09/WP-10/WP-11/WP-12 shell  | Absent                                                |
| WP-15 | [Operations/recovery](wp-15-operations.md)              | WP-03/WP-08/WP-10/WP-11        | Build recipes only                                    |
| WP-16 | [Release verification](wp-16-verification.md)           | WP-01–WP-15                    | Foundation tests only                                 |

The first coding task is WP-02 reconciliation, following WP-00 review; do not
re-scaffold the existing repository. WP-06 must use the minimal canonical content
storage from WP-02/WP-08's contract before WP-08 exposes timeline/diff/retention.
WP-10 and WP-11 can progress independently after their shared projection contract
is settled. One owner coordinates OpenAPI, migrations, and generation.

## Feature coverage

| Requirement area                                       | Implementation slices                     | Presentation / release evidence |
| ------------------------------------------------------ | ----------------------------------------- | ------------------------------- |
| Providers, roles, sites, regions, containment          | WP-05.1–05.3                              | WP-12.3–12.4                    |
| Computes, containers, manual/discovered IPs            | WP-05.3–05.5, WP-06.3                     | WP-12.4–12.5                    |
| Domains, expiry, costs, currency                       | WP-05.6–05.7                              | WP-12.2–12.3                    |
| Tags, descriptions, notes, lifecycle, purge            | WP-05.8–05.9                              | WP-12.3–12.6                    |
| Inventory tree, search, dashboard aggregates           | WP-05.10                                  | WP-12.2–12.5                    |
| Admin/password, sessions, CSRF, PATs/scopes            | WP-04.1–04.3                              | WP-12.1, WP-12.7                |
| Immutable audit and redaction                          | WP-04.4; every mutation slice             | WP-12.7, WP-16                  |
| Enrollment, revoke, rotate, protocol compatibility     | WP-06.1–06.2                              | WP-07.1, WP-13.1–13.2           |
| Host/Podman collection, retry, local state             | WP-07.1–07.4                              | WP-16 agent/failure suites      |
| Current observations, absence, freshness, reactivation | WP-06.3–06.4                              | WP-12/WP-13                     |
| Canonical content, history, differences, retention     | WP-08.1–08.3                              | WP-13.3                         |
| Zone discovery/selection, selective import             | WP-10.1–10.2                              | WP-14.1–14.2                    |
| DNS validation, links, drafts, preview/apply, deletion | WP-09.1–09.4                              | WP-14.3–14.5                    |
| Cloudflare mapping, drift, independent retries         | WP-10.3–10.4                              | WP-14.5, WP-16                  |
| Internal generation, publication, fallthrough          | WP-11.1–11.3                              | WP-14.5, WP-16                  |
| Config, proxy trust, HTTP, SQLite, workers, shutdown   | WP-03.1–03.4                              | WP-15/WP-16                     |
| Logs, metrics, health                                  | WP-03.3–03.4 plus feature instrumentation | WP-16                           |
| Backup, migrations, repair, restore, deployments       | WP-15.1–15.3                              | WP-16                           |
| Tooling, generated files, builds, cross-architecture   | WP-01/WP-02/WP-15                         | WP-16                           |

## Copyable task brief

Assign a single work-package slice using this checklist:

- Task: WP-XX.N and the exact heading in its spec.
- Read: shared rules, decisions, package spec, referenced normative sections.
- Baseline: re-inspect the audited files; list already working behavior.
- Dependencies: name completed packages and outstanding contract items.
- Ownership: package's primary directories and explicitly approved shared edits.
- Deliverables: specified service/query/handler/UI behavior and failure tests.
- Verification: run package commands plus shared completion checks.
- Stop conditions: unresolved product decisions or incompatible contracts.
- Final report: changed files, implemented acceptance IDs, commands/results,
  unverified environments, migrations/API changes, remaining blockers.

Do not mark an entire package done after one slice. Package completion requires
all listed slices, the implementation-plan exit criteria, and executable evidence.
