# Runtime architecture and extension patterns

## Composition and ownership

The server and Linux agent use the same small shared boundaries:

- `internal/identity`: UUIDv7 generation and canonical UUIDv4/v7 validation.
- `internal/logging`: JSON `slog` output, level configuration, bounded request
  correlation, trace/span correlation, and optional log-handler fan-out.
- `internal/lifecycle`: context-based workers. The first worker exit cancels its
  siblings; all workers are joined and all non-cancellation errors are returned.
- `internal/discovery` and `internal/inventory`: transport-independent domain rules.

`cmd/portinaia` owns configuration, SQLite, the HTTP service, retention, and the
telemetry SDK. `cmd/portinaia-agent` parses commands and supplies a signal context
and `agent.RunConfig`. Agent scheduling and retry policy live in
`internal/agent/run.go`, where heartbeat and snapshot work use one policy for
terminal local/protocol failures, bounded jitter, `Retry-After`, and cancellation.
A scheduled tick cannot bypass server-requested backoff. Snapshot collection and
upload remain one serialized operation; each retry recollects before uploading.

Each worker accepts a `context.Context`. Use it for all HTTP, database, and
collector operations. Signal cancellation stops agent work immediately. On the
server, it initiates HTTP draining without canceling in-flight request contexts;
the HTTP drain has its own deadline and force-closes on timeout. SQLite closes
after worker completion and telemetry cleanup.

## Server instrumentation

`internal/telemetry` is the SDK/exporter composition layer. It creates resource
metadata, OTLP/HTTP exporters, bounded batch processors, metric readers, and the
shared JSON/OTLP logger. It owns startup cleanup and idempotent, concurrent,
deadline-bounded shutdown of every provider. The standard `OTEL_*` variables
configure SDK behavior; supported exporter/protocol selections and endpoint
shapes are checked before constructing exporters.

`server.Observability` accepts a logger, tracer provider, and meter provider.
Providers are instance-scoped: application packages do not change global OTel
providers. Tests inject in-memory exporters and manual metric readers. The
agent depends only on OTel APIs for context propagation and correlated logging.

The HTTP composition order is:

1. Establish the shared request identity, including frontend requests.
2. Extract W3C trace context and start the server span and active-request metric.
3. Apply API problem conversion, path validation, proxy handling, and routing.
4. Record the completed response using chi's route template, status, size, and
   elapsed time; emit a context-aware log and finish the span.

This outer instrumentation observes early validation, authentication, 404/405,
readiness failure, and frontend responses. Panic recovery emits a correlated
generic 500 when possible. If output already started, it aborts the connection
instead of appending a misleading second response. Intentional HTTP aborts remain
aborts. Panic values and raw request data are not recorded.

`ObserveRuntime` registers SQLite pool and Go runtime observations without issuing
SQL. Retention records a span, outcome/duration, committed deletion counts, and a
correlated log for each run, including errors and cancellation.

## Adding functionality

- Add HTTP routes inside the existing router so they inherit correlation,
  instrumentation, and API boundaries. Keep request-derived IDs in trace/log
  fields when needed, never in span names or metric labels.
- For deeper operation spans, use the injected tracer and the caller's context;
  do not create a new SDK provider or use `context.Background()` inside requests.
- Add metrics at instrumentation construction, handle registration errors, and
  use fixed label vocabularies. Keep credentials, DSNs, SQL, resource IDs, request
  bodies, and unconstrained remote messages out of telemetry.
- Add background services as `lifecycle.Worker` functions that observe context
  cancellation and finish bounded cleanup before returning.
- Use `logger.InfoContext`, `WarnContext`, or `ErrorContext` inside operations so
  trace/request identity follows the work. Local command output stays on stdout;
  operational logs go to stderr and, on the server, the configured OTel bridge.
- Replace exporters/readers in `internal/telemetry`, without changing handlers,
  agent collection logic, or domain rules.

## Executable coverage

`internal/telemetry/telemetry_test.go` uses an actual local OTLP/HTTP receiver and
decodes protobuf traces, metrics, and logs. It checks resource overrides, remote
parent propagation, log correlation, configuration failures, disabled mode, and
shutdown flushing. Server instrumentation tests check bounded HTTP labels,
sanitized panic responses, error/frontend routes, runtime observations, and
retention success/cancellation/failure. Shared lifecycle and agent tests cover
worker joining, error preservation, retry cancellation, protocol correlation,
and response-limit enforcement. The process-level SIGTERM test verifies that an
in-flight SQLite mutation completes during graceful server shutdown.

Current execution results are recorded in the
[verification ledger](coding/parallel-progress.md).
