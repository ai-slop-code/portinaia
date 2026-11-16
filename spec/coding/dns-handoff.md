# DNS implementation handoff

Status: **on hold by product decision, 2026-11-16**.

This is the resume point for the future split-horizon DNS implementation. Do not
start DNS runtime work until the owner explicitly removes this hold. The DNS
requirements remain active and normative; this file records sequencing and
deployment context so a future coding agent does not reconstruct it from stale
planning records.

## Authoritative requirements

Read these in order:

1. [Product scope and acceptance](../focused-delivery.md)
2. [DNS correctness specification](../dns-spec.md)
3. [DNS implementation requirements](dns-implementation.md)
4. [Current progress ledger](parallel-progress.md)
5. [Active architecture decisions](decisions.md)

The DNS correctness specification is the behavior contract. This handoff does not
replace it and must not weaken its ownership, validation, failure-isolation,
publication, security, or acceptance requirements.

## Current state

- DNS schema, migration, query, and generated-contract groundwork exists.
- DNS runtime behavior is not implemented.
- Cloudflare reconciliation is not implemented.
- DNS drafts, preview/apply, projection workers, drift repair, and DNS UI are not
  implemented.
- CoreDNS zone generation and publication are not implemented.
- No DNS provider credentials are configured in the repository or deployment.
- No DNS packet should be dispatched while this hold is active.
- Inventory, discovery, domains, audit, operations, and backup/restore are the
  pre-DNS foundation; do not reimplement them as DNS prerequisites.

## Current deployment context

The verified deployment currently has:

```text
German dedicated server:
  Portinaia server in Podman
  SQLite database
  Public HTTPS reverse proxy

Home/Debian VM:
  Portinaia discovery agent
  Root-owned Podman discovery

Raspberry Pi:
  CoreDNS is intended to remain the internal DNS frontend
```

The original DNS specification describes CoreDNS and Portinaia sharing a host with
a mounted generated-zone directory. That is not the current deployment topology.
Before implementation, resolve how the German server publishes generated zone
files to the Raspberry Pi over the VPN. The discovery agent must remain read-only
and must not become a DNS publisher or shell channel.

## Required architecture checkpoint before coding

Agree and document:

- Push or pull publication between the German server and Raspberry Pi.
- VPN endpoint, authentication, and trust model.
- Generated-zone bundle or file format and manifest.
- Zone generation, revision, SOA serial, and checksum semantics.
- Raspberry Pi destination directory and ownership/permissions.
- Atomic installation and validation behavior.
- Publication acknowledgement, status, retry, and uncertain-outcome handling.
- CoreDNS reload/discovery behavior and Unbound fallthrough.
- Recovery behavior when Portinaia, SQLite, VPN, or CoreDNS is unavailable.

The preferred direction is a narrow DNS publisher on the Raspberry Pi pulling
validated generations over the VPN, unless the owner chooses another design. Do
not extend the discovery agent for this purpose.

## First implementation slice after the hold is removed

Implement a vertical internal-only slice before Cloudflare integration:

1. Agree shared DNS schemas and persistence transitions.
2. Implement strict name/record/address/CNAME validation.
3. Implement a deterministic complete-zone compiler.
4. Validate generated zones before publication.
5. Implement atomic local publication and checksum/revision status.
6. Exercise CoreDNS internal answers and public fallthrough.
7. Add the cross-host publisher transport and Raspberry Pi acceptance test.
8. Implement Cloudflare discovery, selective import, ownership mapping, and
   independent reconciliation.
9. Implement drafts, preview/apply, UI, drift repair, deletion tombstones, and
   isolated restore/recovery verification.

Do not call Cloudflare or write zone files inside the desired-state database
transaction. Public and internal projections must remain independently retryable.

## Acceptance required to close DNS

The future implementation must demonstrate, with a controlled zone and real
CoreDNS:

- Different public and internal answers for one name.
- A, AAAA, CNAME, multiple values, and explicit wildcard behavior.
- Selective import and unmanaged-record preservation.
- Preview-before-apply and stale-preview rejection.
- Independent Cloudflare and CoreDNS failure/retry.
- Drift detection and ownership-safe repair.
- Atomic publication and pre/post-rename failure handling.
- Final override removal and public fallthrough.
- CoreDNS reload/new-zone discovery.
- Isolated SQLite restore followed by safe DNS reconciliation.
