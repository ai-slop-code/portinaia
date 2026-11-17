package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"portinaia/internal/database"
	"portinaia/internal/logging"
)

func TestHTTPObservabilityCorrelatesAndBoundsAttributes(t *testing.T) {
	var logs bytes.Buffer
	exporter := tracetest.NewInMemoryExporter()
	traces := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer traces.Shutdown(context.Background())
	reader := sdkmetric.NewManualReader()
	metrics := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer metrics.Shutdown(context.Background())
	o, err := NewObservability(logging.New(&logs, nil), traces, metrics)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Use(requestIdentityMiddleware, o.middleware, problemMiddleware)
	router.Get("/items/{item_id}", func(w http.ResponseWriter, r *http.Request) {
		if chi.URLParam(r, "item_id") == "panic-secret" {
			panic("password=panic-secret")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	for _, id := range []string{"first-secret", "second-secret", "panic-secret"} {
		r := httptest.NewRequest(http.MethodGet, "/items/"+id+"?token=query-secret", nil)
		r.Header.Set("Authorization", "Bearer credential-secret")
		r.Header.Set("Cookie", "session=cookie-secret")
		r.Header.Set("X-Request-ID", "test-request")
		r.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if id == "panic-secret" {
			var problem problemResponse
			if err := json.Unmarshal(w.Body.Bytes(), &problem); err != nil || w.Code != 500 || problem.RequestID != "test-request" || strings.Contains(problem.Detail, "secret") {
				t.Fatalf("panic response=%d %s: %v", w.Code, w.Body.String(), err)
			}
		} else if w.Code != 204 {
			t.Fatalf("status=%d", w.Code)
		}
	}
	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("got %d spans", len(spans))
	}
	for _, span := range spans {
		if span.Name != "GET /items/{item_id}" || span.SpanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" || span.Parent.SpanID().String() != "00f067aa0ba902b7" {
			t.Fatalf("unexpected span correlation: %+v", span)
		}
	}
	if spans[2].Status.Code != codes.Error {
		t.Fatal("panic span was not marked as error")
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	var count int64
	var durationCount uint64
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch m.Name {
			case "portinaia.http.server.requests":
				points := m.Data.(metricdata.Sum[int64]).DataPoints
				if len(points) != 2 { // route/method, split only by response status
					t.Fatalf("unexpected request cardinality: %+v", points)
				}
				for _, point := range points {
					count += point.Value
				}
			case "http.server.active_requests":
				for _, point := range m.Data.(metricdata.Sum[int64]).DataPoints {
					if point.Value != 0 {
						t.Fatalf("leaked active request: %d", point.Value)
					}
				}
			case "http.server.request.duration":
				for _, point := range m.Data.(metricdata.Histogram[float64]).DataPoints {
					durationCount += point.Count
				}
			}
		}
	}
	if count != 3 || durationCount != 3 {
		t.Fatalf("requests=%d durations=%d", count, durationCount)
	}
	encoded, _ := json.Marshal(struct{ Spans, Metrics any }{spans, data})
	if strings.Contains(string(encoded), "secret") || strings.Contains(logs.String(), "secret") {
		t.Fatalf("telemetry leaked request data: %s %s", encoded, logs.String())
	}
	for _, span := range spans {
		if !strings.Contains(logs.String(), span.SpanContext.SpanID().String()) {
			t.Fatal("log missing span correlation")
		}
	}
	t.Logf("verified %d parent-linked spans, %d requests/durations, zero active requests, correlated redacted logs", len(spans), count)
}

func TestMountedObservabilityCoversErrorsAndFrontend(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	traces := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer traces.Shutdown(context.Background())
	metrics := sdkmetric.NewMeterProvider()
	defer metrics.Shutdown(context.Background())
	var logs bytes.Buffer
	o, err := NewObservability(logging.New(&logs, nil), traces, metrics)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewConfiguredHandler(HandlerConfig{PublicURL: "http://localhost", Observability: o})
	for _, test := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/v1/health/live", 200},
		{"GET", "/api/v1/health/ready", 503},
		{"GET", "/api/v1/missing-secret", 404},
		{"GET", "/api/v1/providers/invalid-secret", 400},
		{"POST", "/api/v1/health/live", 405},
		{"GET", "/inventory", 200},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
		if w.Code != test.status || w.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s: status=%d request-id=%q", test.path, w.Code, w.Header().Get("X-Request-ID"))
		}
	}
	if len(exporter.GetSpans()) != 6 || strings.Contains(logs.String(), "secret") {
		t.Fatalf("missing instrumentation or raw paths: spans=%d logs=%s", len(exporter.GetSpans()), logs.String())
	}
}

func TestRetentionAndRuntimeObservability(t *testing.T) {
	db, err := database.Open(t.TempDir() + "/observability.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	exporter := tracetest.NewInMemoryExporter()
	traces := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer traces.Shutdown(context.Background())
	reader := sdkmetric.NewManualReader()
	metrics := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer metrics.Shutdown(context.Background())
	var logs bytes.Buffer
	o, err := NewObservability(logging.New(&logs, nil), traces, metrics)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := o.ObserveRuntime(db)
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Unregister()
	cutoff := time.Now().UTC().Add(-91 * 24 * time.Hour).Format(time.RFC3339)
	runRetention(context.Background(), db, cutoff, 10, o)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runRetention(ctx, db, cutoff, 10, o)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	runRetention(context.Background(), db, cutoff, 10, o)
	spans := exporter.GetSpans()
	if len(spans) != 3 || spans[0].Name != "discovery.retention" || spans[0].Status.Code == codes.Error || spans[1].Status.Code == codes.Error || spans[2].Status.Code != codes.Error {
		t.Fatalf("retention span outcomes: %+v", spans)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			found[m.Name] = true
			if m.Name == "portinaia.discovery.retention.runs" {
				points := m.Data.(metricdata.Sum[int64]).DataPoints
				if len(points) != 3 {
					t.Fatalf("missing success/canceled/error outcomes: %+v", points)
				}
			}
		}
	}
	for _, name := range []string{"portinaia.discovery.retention.runs", "portinaia.discovery.retention.deleted", "portinaia.db.connections", "portinaia.db.pool.waits", "portinaia.process.goroutines", "portinaia.process.heap.alloc", "portinaia.process.gc.cycles"} {
		if !found[name] {
			t.Errorf("missing %s", name)
		}
	}
}
