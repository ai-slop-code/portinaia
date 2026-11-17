package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	runtimemetrics "runtime/metrics"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Observability is the server instrumentation boundary. Providers are injected
// by the composition root, allowing exporters/readers to change independently.
type Observability struct {
	logger           *slog.Logger
	tracer           trace.Tracer
	meter            metric.Meter
	propagator       propagation.TextMapPropagator
	requests         metric.Int64Counter
	active           metric.Int64UpDownCounter
	duration         metric.Float64Histogram
	responseSize     metric.Int64Histogram
	retentionRuns    metric.Int64Counter
	retentionDeleted metric.Int64Counter
	retentionTime    metric.Float64Histogram
}

func NewObservability(logger *slog.Logger, traces trace.TracerProvider, metrics metric.MeterProvider) (*Observability, error) {
	o := &Observability{logger: logger, tracer: traces.Tracer("portinaia/server"), meter: metrics.Meter("portinaia/server"), propagator: propagation.TraceContext{}}
	var errs []error
	var err error
	o.requests, err = o.meter.Int64Counter("portinaia.http.server.requests", metric.WithUnit("{request}"))
	errs = append(errs, err)
	o.active, err = o.meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}"))
	errs = append(errs, err)
	o.duration, err = o.meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.005, .01, .025, .05, .075, .1, .25, .5, .75, 1, 2.5, 5, 7.5, 10, 30))
	errs = append(errs, err)
	o.responseSize, err = o.meter.Int64Histogram("http.server.response.body.size", metric.WithUnit("By"))
	errs = append(errs, err)
	o.retentionRuns, err = o.meter.Int64Counter("portinaia.discovery.retention.runs", metric.WithUnit("{run}"))
	errs = append(errs, err)
	o.retentionDeleted, err = o.meter.Int64Counter("portinaia.discovery.retention.deleted", metric.WithUnit("{row}"))
	errs = append(errs, err)
	o.retentionTime, err = o.meter.Float64Histogram("portinaia.discovery.retention.duration", metric.WithUnit("s"))
	errs = append(errs, err)
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *Observability) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		method := telemetryMethod(r.Method)
		ctx := o.propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := o.tracer.Start(ctx, method, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		r = r.WithContext(ctx)
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		activeAttrs := metric.WithAttributes(attribute.String("http.request.method", method))
		o.active.Add(ctx, 1, activeAttrs)
		defer func() {
			panicValue := recover()
			aborted := panicValue == http.ErrAbortHandler
			if panicValue != nil && !aborted {
				// Do not export panic values: they can contain request payloads/secrets.
				if wrapped.Status() == 0 {
					writeProblemForRequest(wrapped, r, http.StatusInternalServerError, "internal_error", "internal server error")
				} else {
					aborted = true // A partial response cannot be replaced with a problem.
				}
			}
			status := wrapped.Status()
			if status == 0 {
				status = http.StatusOK
				if aborted {
					status = http.StatusInternalServerError
				}
			}
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			attrs := []attribute.KeyValue{attribute.String("http.request.method", method), attribute.String("http.route", route), attribute.Int("http.response.status_code", status)}
			span.SetName(method + " " + route)
			span.SetAttributes(attrs...)
			if id := requestIdentity(ctx); id != "" {
				span.SetAttributes(attribute.String("portinaia.request_id", id))
			}
			level := slog.LevelInfo
			if status >= 500 || panicValue != nil {
				level = slog.LevelError
				errorType := strconv.Itoa(status)
				if panicValue != nil {
					errorType = "panic"
				}
				span.SetStatus(codes.Error, "request failed")
				span.SetAttributes(attribute.String("error.type", errorType))
			} else if status >= 400 {
				level = slog.LevelWarn
			} else if route == "/api/v1/health/live" || route == "/api/v1/health/ready" {
				level = slog.LevelDebug
			}
			elapsed := time.Since(started).Seconds()
			options := metric.WithAttributes(attrs...)
			o.active.Add(ctx, -1, activeAttrs)
			o.requests.Add(ctx, 1, options)
			o.duration.Record(ctx, elapsed, options)
			o.responseSize.Record(ctx, int64(wrapped.BytesWritten()), options)
			o.logger.Log(ctx, level, "http request completed", "http.request.method", method, "http.route", route, "http.response.status_code", status, "duration_seconds", elapsed, "response_bytes", wrapped.BytesWritten())
			if aborted {
				panic(http.ErrAbortHandler)
			}
		}()
		next.ServeHTTP(wrapped, r)
	})
}

func telemetryMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace, http.MethodPatch:
		return method
	default:
		return "_OTHER"
	}
}

// ObserveRuntime samples the pool and Go runtime without issuing SQL or exporting its DSN.
// Keep observations registered through the final metric flush, then unregister
// before closing the database. Provider shutdown also stops observations.
func (o *Observability) ObserveRuntime(db *sql.DB) (metric.Registration, error) {
	connections, err := o.meter.Int64ObservableGauge("portinaia.db.connections", metric.WithUnit("{connection}"))
	if err != nil {
		return nil, err
	}
	waits, err := o.meter.Int64ObservableCounter("portinaia.db.pool.waits", metric.WithUnit("{wait}"))
	if err != nil {
		return nil, err
	}
	waitTime, err := o.meter.Float64ObservableCounter("portinaia.db.pool.wait.duration", metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	goroutines, err := o.meter.Int64ObservableGauge("portinaia.process.goroutines", metric.WithUnit("{goroutine}"))
	if err != nil {
		return nil, err
	}
	heap, err := o.meter.Int64ObservableGauge("portinaia.process.heap.alloc", metric.WithUnit("By"))
	if err != nil {
		return nil, err
	}
	gc, err := o.meter.Int64ObservableCounter("portinaia.process.gc.cycles", metric.WithUnit("{cycle}"))
	if err != nil {
		return nil, err
	}
	return o.meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		stats := db.Stats()
		observer.ObserveInt64(connections, int64(stats.InUse), metric.WithAttributes(attribute.String("state", "used")))
		observer.ObserveInt64(connections, int64(stats.Idle), metric.WithAttributes(attribute.String("state", "idle")))
		observer.ObserveInt64(waits, stats.WaitCount)
		observer.ObserveFloat64(waitTime, stats.WaitDuration.Seconds())
		samples := []runtimemetrics.Sample{{Name: "/sched/goroutines:goroutines"}, {Name: "/memory/classes/heap/objects:bytes"}, {Name: "/gc/cycles/total:gc-cycles"}}
		runtimemetrics.Read(samples)
		observer.ObserveInt64(goroutines, int64(samples[0].Value.Uint64()))
		observer.ObserveInt64(heap, int64(samples[1].Value.Uint64()))
		observer.ObserveInt64(gc, int64(samples[2].Value.Uint64()))
		return nil
	}, connections, waits, waitTime, goroutines, heap, gc)
}
