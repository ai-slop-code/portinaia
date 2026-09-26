# WP-00 — Specification baseline

Dependencies: none. Sources: implementation plan §§1–2, 21/WP-00, 23;
product §§3, 19; DNS §§3, 28. Read [shared rules](coding-rules.md).

Primary files: `spec/`. Coordinated files: project README status when implemented
behavior changes. Baseline: three approved plans exist; review uncovered the
contract gaps in [the audit](implementation-audit.md).

## WP-00.1 — Freeze the agreed behavior

1. Read [decisions](decisions.md) D-01–D-07 and their amendments in the normative
   plans. Do not invent protocol v0 or weaken retained-history behavior.
2. Treat API/schema differences C-01–C-21 as explicit WP-02 tasks, not product
   decisions already resolved by the presence of generated code.
3. Preserve all first-release modules and non-goals. No external auth, remote
   commands, auto DNS from discovery, IPAM, PostgreSQL, or Ansible roles.
4. Identify the contract owner before shared contract edits. Follow the dependency
   table in the index; only parallelize disjoint slices after contracts stabilize.
5. Obtain F-01 storage-budget approval and F-02 tested-version evidence before
   corresponding release sign-off. Other implementation can proceed under the
   already approved performance thresholds and CoreDNS upgrade permission.

## Acceptance and verification

- BASE-01: Every work package has scope, paths, ordered steps, failures, tests,
  and explicit exclusions in this specification set.
- BASE-02: No known contradictory rule is left for a feature agent to choose.
  Open follow-ups name exactly what they block.
- BASE-03: Each product/DNS acceptance requirement has an owner in the index and
  a release verification scenario in WP-16.
- Check all Markdown links and run `npm exec prettier -- --check "spec/**/*.md"`.
  This is a documentation gate, not an application completion claim.

Handoff: decision IDs, affected contracts, outstanding owner questions, and the
first unblocked implementation task. No application code belongs in WP-00.
