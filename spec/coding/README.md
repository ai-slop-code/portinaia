# Coding specification index

This is the only active coding-spec index. Updated 2026-11-16.

| Document                                                 | Responsibility                                                                    |
| -------------------------------------------------------- | --------------------------------------------------------------------------------- |
| [Product scope](../focused-delivery.md)                  | Three goals, deferrals, M1–M5 and G1/G2 acceptance                                |
| [Progress ledger](parallel-progress.md)                  | Sole current implementation/verification status and remaining work                |
| [Coding rules](coding-rules.md)                          | Engineering conventions and verification discipline                               |
| [Backend and UI requirements](backend-requirements.md)   | BE-01–BE-06: contracts, auth/audit, inventory/domains, safe frontend workflows    |
| [Discovery requirements](discovery-requirements.md)      | DI-01–DI-06: authority, identity, protocol, storage/retention and agent lifecycle |
| [Runtime/recovery requirements](runtime-requirements.md) | RT-01–RT-04: startup, health/shutdown, packaging and recovery                     |
| [DNS correctness](../dns-spec.md)                        | Detailed desired-state and split-horizon behavior                                 |
| [DNS implementation requirements](dns-implementation.md) | DN-01–DN-05: domain, ownership/recovery, publication and integration refinements  |
| [DNS implementation handoff](dns-handoff.md)             | DNS hold, resume sequence, deployment context, and architecture checkpoint        |
| [Decision register](decisions.md)                        | Active compatibility policies and clearly deferred prior decisions                |
| [Legacy records](legacy/README.md)                       | Historical pilot evidence and superseded planning/interface records               |

Read scope, progress, and the relevant normative requirements first. Do not
dispatch from old task numbers embedded in migration names, test comments, or
legacy records; those are provenance.
Specifications describe intended behavior. The progress ledger distinguishes code
that exists from wiring and executable acceptance still missing.

The progress ledger is the current implementation and verification record.
