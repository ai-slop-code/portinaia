# Portinaia

Portinaia is a self-hosted infrastructure inventory and DNS control plane. The
approved behavior and implementation order are defined in `spec/`.

## Implementation Status

- WP-00 specification baseline: complete.
- WP-01 repository and build foundation: complete; local verification and the
  browser smoke test pass, while multi-architecture image verification requires
  a container tool.
- WP-02 OpenAPI and database contract: implementation present; contract
  reconciliation remains.
- Next package: complete WP-02 contract reconciliation.

## Prerequisites

- Go 1.26.5
- Node.js 22.13.0 or newer
- GNU Make
- Podman or Docker when building the container image

Install the pinned frontend dependencies with `npm ci`.

Install the Chromium browser used by the foundation smoke test with
`npm exec playwright install chromium`.

## Local Foundation

Run `make verify` to check formatting, static analysis, Go and frontend tests,
the embedded server build, and both Linux agent architectures.

Run `make test-e2e` to build the server and exercise the embedded UI and health
endpoint in Chromium.

Run the minimal server with:

```sh
make build
./bin/portinaia server
```

The embedded frontend is served on `http://localhost:8080`. Liveness and
readiness are available at `/api/v1/health/live` and
`/api/v1/health/ready`. Runtime configuration is introduced by WP-03; the
foundation server intentionally uses port 8080 until then.

Use `./bin/portinaia version` or a built `portinaia-agent version` binary to
inspect build metadata.

## Generated Files

`make generate` is the only supported regeneration entry point. Generated
source belongs in a `generated/` directory and must carry its generator's
standard do-not-edit header. Generated files are never edited by hand.

The API source of truth is `api/openapi.yaml`. `make generate` runs pinned
versions of oapi-codegen and sqlc, generates the TypeScript API types, and
rebuilds the frontend assets. Generated outputs are committed in
`internal/api/generated/`, `internal/database/generated/`, `web/src/api/`, and
`web/dist/` because Go compiles or embeds them. `make generate-check` fails when
these outputs do not match their contracts.

The initial SQLite schema is `internal/database/migrations/00001_initial.sql`.
Migrations are immutable, embedded into the server, and exercised against a
temporary modernc SQLite database by `go test ./internal/database`.
