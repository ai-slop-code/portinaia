# WP-01 — Repository and build foundation

Dependencies: WP-00. Sources: implementation plan §§3–4, 20, 21/WP-01;
product §§13.1, 15.4. Read [shared rules](coding-rules.md).

Primary paths: `Makefile`, `Containerfile`, `cmd/`, `internal/version/`, `web/`
build config and embedding, root package/tooling files. Coordinate shared edits.
Baseline: these are substantially present; preserve them rather than recreating.

## WP-01.1 — Reproducible commands

1. Inspect pinned versions in `go.mod`, package manifests/lockfile, generator
   variables, and Containerfile. Resolve unsupported tooling combinations with
   evidence, not unsolicited broad upgrades.
2. Keep `make generate` as the single generator entry point, producing Go
   OpenAPI, sqlc, TypeScript and embedded web assets. Only generated files carry
   do-not-edit headers; `web/src/api/client.ts` is handwritten.
3. Ensure generation drift checks compare all expected outputs and fail on added,
   removed, or changed files. Document that the current check regenerates in place.
4. When new agent/integration Go packages appear, include them in test/vet/format
   targets. Keep host OS tests portable using Linux build tags and injected fixtures.
5. Document stable commands, tool prerequisites, dependency install, and browser
   install; no hosted CI configuration is needed.

## WP-01.2 — Artifacts and embedded assets

1. Preserve separate server/agent binaries and shared non-server version metadata.
   Version commands must work without config/database/network.
2. Build frontend before embedding into the server. Keep SPA deep-link fallback;
   `/api/v1` errors must never return the SPA. Missing static assets must not be
   cached as successful JavaScript/stylesheet content.
3. Cross-build Linux amd64/arm64 with CGO disabled. Release checksums cover exactly
   the release binaries; fail release on any incomplete build.
4. Verify both images from the Containerfile using the named container tool;
   retain non-root runtime and CA certificates. Later runtime paths/config are
   wired by WP-03/WP-15, not hard-coded into this scaffold.

## Acceptance / tests

- FND-01: Clean dependency install, generation, typecheck, lint, tests and embedding
  build work using the documented root commands.
- FND-02: Version output identifies artifact/build; later protocol constants are
  independently represented. Agent binary does not embed UI or link server DB.
- FND-03: Embedded `/` and a deep link render; unknown API path gives API error.
- FND-04: Both agent artifacts and server images build; execution evidence is
  recorded separately from cross-compilation.

Commands: `make verify`, `make test-e2e`, `make image`, `make release`.
Inspect checksum manifest and run native target startup tests where available.
No new business endpoints or screens in this package.
