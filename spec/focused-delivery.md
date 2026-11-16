# Product scope and acceptance

Status: authoritative pivot scope, reviewed 2026-11-16.
Implementation status lives only in [the progress ledger](coding/parallel-progress.md).
Historical assignments are retained under [legacy coding records](coding/legacy/README.md);
current implementation status lives in the progress ledger.

DNS runtime implementation is intentionally **on hold by product decision**. The
DNS requirements remain approved and authoritative; the resume sequence and
deployment-specific architecture checkpoint are in the [DNS implementation handoff](coding/dns-handoff.md).

This document defines product scope, exclusions, milestones, and delivery gates;
it is not a duplicate implementation-status report. Current deployment evidence,
including the Germany server, Debian discovery VM, root-owned Podman validation,
backup/restore results, and remaining rootless/systemd evidence, is recorded in
the [progress ledger](coding/parallel-progress.md).

## Product goals

1. One overview of how many servers exist, what runs on them, and relationships
   down to containers.
2. A list of domains the owner owns.
3. Management of public/internal answers for split-horizon DNS.

Keep one single-administrator Go server, SQLite, embedded React/MUI UI, and a
read-only Linux/Podman discovery agent. Cloudflare serves public DNS; external
CoreDNS serves generated internal overrides and forwards unresolved queries through
the existing resolver chain. No microservices, external broker, or database rewrite.

## First-release behavior

### Inventory and discovery

- Providers, locations, physical/VPS/VM computes, manual IPs, descriptions/notes,
  archive/restore, search, accurate counts and bounded hierarchy navigation.
- Parent-compute placement wins over location metadata. Unknown placement remains
  navigable. Prevent cycles and duplicate placement; show archived ancestors only
  where needed for active descendants. Context nodes do not inflate active counts.
- Current host facts and Podman workloads with image, state/health, ports, networks,
  addresses, mounts/restart facts, observation times and collector diagnostics.
- One compute-bound agent with enrollment, heartbeat, revocation/re-enrollment,
  rootful/rootless owner scopes, private durable state and safe retry/restart.
- Preserve manual metadata. Failed/omitted collection never means authoritative
  absence; successful empty collection can. Distinguish unknown, stale, offline,
  collection failure and confirmed absence. Keep logical container identity across
  recreation. Compute/VM parentage is manual; no workload control or shell channel.
- Preserve existing tags when editing unrelated fields even though tag management
  is deferred. Keep report storage/deduplication and bounded retention needed by
  ingestion; historical browsing is not a prerequisite.

### Owned domains

- Add/list/detail/edit/archive/restore name, optional registrar, dates, renewal state,
  nameservers and notes. Unknown dates/renewal remain unknown; show known expiry.
- A domain is useful without Cloudflare credentials or a managed zone. Link a
  matching selected zone when DNS is implemented. Domain creation/archive does
  not itself create/delete provider records. No registrar/renewal automation.

### Split-horizon DNS

- Select multiple zones from one Cloudflare account; selectively import supported
  records using a complete explicit preview; preserve unmanaged provider records.
- A/AAAA/CNAME, multiple address values and explicit wildcards, with independent
  public/internal views, including internal-only overrides under selected domains.
- One name editor with revisioned draft/validate/preview/apply, explicit inventory
  links, durable independent reconciliation, truthful status, repair and drift handling.
- Ownership-safe public writes and complete validated, atomically published internal
  zones. Final override removal removes stale answers. Verify actual CoreDNS fallback,
  reload and DNS answers. No automatic DNS changes from inventory/discovery.
- Follow [DNS correctness](dns-spec.md) and [implementation requirements](coding/dns-implementation.md).

### Supporting operations

Secure local administrator/session/CSRF, password bootstrap/reset, exact capability
checks, separate machine credentials, transactional redacted audit and a basic paged
audit view; bounded inputs/queries/workers; explicit migrations; truthful health;
structured diagnostics; graceful shutdown; tested install/upgrade/backup/restore.

## Deferred scope

Financial tracking/currency UI; history timelines/diffs/exports; full tag management;
generic hard purge/dependency browser; human PAT issuance/UI; comprehensive settings
and metrics dashboards; maximum-capacity certification. Dormant schema/contracts may
remain, but do not expose fabricated successful operations or implement deferred
features to satisfy old package gates.

Also excluded: multiple users/RBAC/SSO, PostgreSQL/HA, extra DNS providers/accounts,
private suffixes, other record types, IP allocation workflows, hypervisor discovery,
registrar operations, automatic discovery-to-DNS binding, and agent self-update.

## Acceptance milestones

| Milestone     | Required user-visible exit                                                                                                                                                                                                                                                |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| M1 inventory  | Log in; create provider/location/host/VM/IP; navigate true relationships and correct counts; edit safely; archive/restore; preserve data across actual server restart. Cycles, stale revisions, unauthorized/CSRF, rollback/audit and mixed archived ancestry are tested. |
| M2 discovery  | Real supported Linux/Podman host appears under its compute with containers; rootful/rootless/failure/empty/recreation cases, pending retry/supersession/restart, revocation and manual-data preservation are verified. Current state survives safe retention.             |
| M3 domains    | Maintain owned domains without DNS credentials; unknown fields, normalization, dates, registrar lookup beyond one page, edit conflicts and lifecycle work; list/detail survive restart.                                                                                   |
| M4 DNS        | A controlled name returns intended different public/internal answers. Complete preview/apply/status, unmanaged protection, independent failure/retry, stale-worker protection, final removal and real pinned-CoreDNS fallback/reload are verified.                        |
| M5 operations | The owner can install, upgrade, back up and recover the three workflows, including DNS reconciliation after isolated restore, using the documented process.                                                                                                               |

### Delivery gates

**G1:** finish current inventory/discovery/domain correctness and integration, basic
audit, runtime/agent acceptance and relevant real Linux evidence before new DNS
behavior. Do not require deferred capacity or every historical package. Independent
subtasks may run in parallel; missing external evidence is blocked, not passed.

**G2:** after G1, agree shared DNS contracts, implement M4 in parallel, and verify
M4/M5 including controlled-provider, CoreDNS and recovery evidence. Restoring SQLite
does not roll back external provider changes.

Detailed acceptance is identified by `BE-*`, `DI-*`, `RT-*`, `DN-*` in the focused
technical documents. Those IDs replace the deleted work-package dependency chains.
