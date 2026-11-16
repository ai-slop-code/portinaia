# Portinaia specification

The specification was consolidated on 2026-09-30 around the owner's three goals:
inventory down to containers, owned domains, and split-horizon DNS.

## Read in this order

1. [Product scope and acceptance](focused-delivery.md) — what to deliver and defer.
2. [Current implementation status](coding/parallel-progress.md) — what exists,
   what is integrated, what has been tested, and what remains.
3. [Legacy coding records](coding/legacy/README.md) (historical only)
   — how to finish the remaining work in parallel.
4. [Coding index](coding/README.md) — focused technical requirements and conventions.

Scope, requirements, status, and scheduling have separate responsibilities. Status
does not weaken a requirement; a component file is not an integrated workflow; a
passing general suite is not evidence for every acceptance case.

The obsolete omnibus plans, numbered work-package backlog, superseded audits, and
duplicated consolidation assignment have been removed. Their active correctness
requirements are preserved in the focused documents below. There is no fallback
authority in deleted documents. Do not recreate their broader release scope.

The [dated Linux/Podman pilot](coding/legacy/consolidation-evidence-2026-09-27.md) is
historical execution evidence, not a current work order or complete release claim.
