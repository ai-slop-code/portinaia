# DNS implementation and acceptance refinements

The detailed [DNS contract](../dns-spec.md) remains authoritative for DNS behavior.
This file preserves implementation invariants needed for the pivot without a second
package backlog. Status: DNS runtime is not implemented; see [progress](parallel-progress.md).
Start new DNS behavior only after G1 in [product scope](../focused-delivery.md).

## DN-01 — Shared contract agreement before parallel writers

Resolve OpenAPI/SQL/UI together, through one shared-contract owner:

- Internal-only change with unchanged public desired state must work without a
  Cloudflare observation. Existing required preview timestamp/draft constraints
  need an explicit applicability model, not a fabricated current timestamp.
- Accept normalizable IDNA/case input while returning canonical output. Keep write
  strictness without invalid response `allOf` composition. Define default TTL/proxy
  handling, complete preview pagination, apply/provider failure responses and import
  preview identity (do not invent a draft revision for a non-draft workflow).
- Agree aggregate/draft/projection lifecycle mappings, view-specific content versus
  aggregate revisions, generation advancement, same-revision repair, tombstones and
  name reservation until cleanup. Reuse existing projection SQL and migrations.
- Persist provider operation intent before external writes; required record-ID
  mappings alone cannot represent an uncertain create with no returned ID.
- Define unique lease claims/fencing, renewal and stale completion; zone snapshot
  contributing revisions, serial allocation, checksum/publication intent, lock order
  and restore/post-rename uncertainty. Forward migrations only.

**Accept DN-01:** schemas, generated consumers and populated migration tests agree;
domain/provider/publisher/UI agents share explicit interfaces before mutation work.

## DN-02 — Names, drafts, preview/apply and deletion

- Whole-name A/AAAA/CNAME validation, TTL/proxy/value rules, normalized owner/wildcard,
  bounded CNAME graph including effective internal fallthrough. No unbounded live
  external resolution. Public special-use prefix policy is versioned with registry
  source/date; `IsGlobalUnicast` alone is not sufficient.
- Owner and zone are immutable; rename/move is separate delete/create. Draft revision
  and content hash differ from aggregate revision. Edits invalidate previews.
- Preview binds draft/base revision, zone selection, relevant owned/unmanaged
  observation fingerprints and internal generation. Unsupported provider types can
  still conflict with CNAME. Preview contains every selected change without truncation.
- Public-affecting changes need usable provider observation; internal-only changes
  obey applicability rather than global provider availability.
- One apply/import transaction writes desired state/tombstone, projection jobs,
  internal generation, audited change, applied marker and safe idempotent result.
  External calls stay outside the transaction; recheck relevant state inside it.
- Removed views stay applicable until cleanup succeeds. Separate scheduling new
  revisions, same-revision repair and retries. Finalize deletion atomically only
  after all applicable outputs are cleaned; reserve name uniqueness until then.
  Later recreation gets new aggregate/mappings. No direct-record bypass or purge UI.

**Accept DN-02:** invalid graph/record cases, 409/412 stale preview/edit, idempotent
uncertain apply, rollback including audit/jobs, internal-only provider outage,
view-only/metadata changes, deletion races and inventory edits causing no DNS writes.

## DN-03 — Cloudflare ownership, import and uncertain writes

- Bound and complete zone/record pagination. Incomplete discovery cannot mark unseen
  zones missing. Disable blocks new changes but preserves desired state and repair.
- Candidate IDs/selections/cursors bind an observation fingerprint. Explicit selective
  import commits only selected exact mappings and desired state; no implicit sibling
  adoption or rewriting equivalent imported records.
- Durable intent and constrained ownership proof recover response-lost creates.
  Identical unmanaged content is not ownership. Retain obsolete mappings until
  verified deletion. Delete retries verify mapped-object absence safely.
- Serialize per name; recheck lease/revision before calls. A lease cannot undo a call
  already sent: retain its intent/evidence, reject stale success attribution and
  converge the newest desired state. Unique acquisition ownership fences same-revision
  reacquisition too. Permanent failures are not immediately due through null retry time.
- Compare effective proxy/Auto TTL semantically; avoid infinite drift. Independent
  retries honor rate limits/backoff and never roll back valid internal progress.

**Accept DN-03:** pagination/import refresh, unmanaged preservation, drift, lost
create/delete responses, ambiguous duplicates, stale in-flight requests, lease expiry,
provider normalization, permanent failure/repair and controlled-zone execution.

## DN-04 — Complete-zone publication and CoreDNS

- Zone-wide generation is not maximum name revision. Snapshot a whole zone with
  contributing name revisions; render deterministically without wall-clock noise.
- Durable RFC 1982 SOA serial allocation; identical retries reuse intended serial/
  content. Define checksums without self-reference. Recovery never regresses serials.
- DB leases alone do not fence rename. Apply/generation changes and final publication
  share a zone-specific lock order; prevent a second uncontrolled publisher process.
  Separate zones progress independently.
- Selected-zone-derived safe paths; no traversal/symlink substitution. Same-filesystem
  temp write, parse validation, file flush/close, atomic rename, directory flush, then
  truthful completion. Before rename preserve old valid file; after rename uncertainty
  preserve the new valid file and reconcile metadata rather than restoring stale bytes.
- Final removal publishes a valid metadata-only zone with advanced serial. CoreDNS
  discovers new zone files without runtime Corefile editing/hooks; ignore temp/backup
  files. Serve the last valid file while Portinaia is unavailable.

**Accept DN-04:** concurrent names/zones, stale writer/reacquisition, every filesystem
failure boundary, restart/restore and checksum recovery; real pinned CoreDNS queries
for A/AAAA/CNAME/wildcard, absent-name and absent-type/NODATA fallback, reload/new-zone
discovery/final removal. Record exact tested build/checksum; documentation is not proof.

## DN-05 — Browser workflow and cross-system recovery

Zone selection/import, name list and one side-by-side editor use generated contracts.
Refresh invalidates import selection/preview. Preserve original revisions and dirty
edits after conflict; show complete changes and require explicit apply. Stable keys
survive uncertain response retries. HTTP 202 means accepted, not synchronized.
Show independent public/internal outcomes and tombstones until cleanup; no optimistic
DNS success. If domain creation succeeds but zone selection fails, show partial
progress. Inventory IP suggestions are copied values, not automatic live bindings.

**Accept DN-05:** real browser/API preview/apply/status, independent provider failures,
removal and repair, keyboard/mobile flow, same name with distinct controlled public/
internal answers, and isolated restore that neither adopts unknown provider records
nor claims remote rollback. Coordinate existing inventory/domain navigation with their
owners; DNS UI does not independently edit shared application composition.
