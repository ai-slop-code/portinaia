package telemetry_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	logpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"

	"portinaia/internal/server"
	"portinaia/internal/telemetry"
)

func cleanEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OTEL_SDK_DISABLED", "OTEL_SERVICE_NAME", "OTEL_RESOURCE_ATTRIBUTES", "PORTINAIA_LOG_LEVEL",
		"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_HEADERS",
		"OTEL_TRACES_SAMPLER", "OTEL_TRACES_SAMPLER_ARG",
	} {
		t.Setenv(key, "")
	}
	for _, signal := range []string{"TRACES", "METRICS", "LOGS"} {
		for _, suffix := range []string{"ENDPOINT", "PROTOCOL", "HEADERS"} {
			t.Setenv("OTEL_EXPORTER_OTLP_"+signal+"_"+suffix, "")
		}
		t.Setenv("OTEL_"+signal+"_EXPORTER", "")
	}
}

func TestOTLPExportsServerTracesMetricsAndCorrelatedLogs(t *testing.T) {
	cleanEnvironment(t)
	var mu sync.Mutex
	var traces tracepb.ExportTraceServiceRequest
	var metrics metricpb.ExportMetricsServiceRequest
	var logs logpb.ExportLogsServiceRequest
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/x-protobuf" || r.Header.Get("x-collector-key") != "test-key" {
			t.Error("exporter did not use configured protobuf transport/headers")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		mu.Lock()
		defer mu.Unlock()
		var message proto.Message
		switch r.URL.Path {
		case "/v1/traces":
			message = &traces
		case "/v1/metrics":
			message = &metrics
		case "/v1/logs":
			message = &logs
		default:
			t.Errorf("unexpected export path: %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if err := proto.Unmarshal(body, message); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer collector.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "x-collector-key=test-key")
	t.Setenv("OTEL_SERVICE_NAME", "telemetry-integration")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "deployment.environment.name=test")
	t.Setenv("PORTINAIA_LOG_LEVEL", "debug")
	// Long batch periods force this test to exercise shutdown flushing.
	t.Setenv("OTEL_BSP_SCHEDULE_DELAY", "600000")
	t.Setenv("OTEL_BLRP_SCHEDULE_DELAY", "600000")
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "600000")
	var local bytes.Buffer
	rt, err := telemetry.New(context.Background(), "portinaia", "test-version", &local)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = rt.Shutdown(ctx)
	})
	o, err := server.NewObservability(rt.Logger, rt.TracerProvider, rt.MeterProvider)
	if err != nil {
		t.Fatal(err)
	}
	handler := server.NewConfiguredHandler(server.HandlerConfig{PublicURL: "http://localhost", Observability: o})
	r := httptest.NewRequest("GET", "/api/v1/health/live?secret=do-not-export", nil)
	r.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	r.Header.Set("X-Request-ID", "otlp-test")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("request status = %d", w.Code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rt.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rt.Shutdown(ctx); err != nil {
		t.Fatalf("repeated shutdown: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(traces.ResourceSpans) != 1 || len(metrics.ResourceMetrics) != 1 || len(logs.ResourceLogs) != 1 {
		t.Fatalf("missing exported signals: traces=%d metrics=%d logs=%d", len(traces.ResourceSpans), len(metrics.ResourceMetrics), len(logs.ResourceLogs))
	}
	resource := traces.ResourceSpans[0].Resource.Attributes
	if attributeValue(resource, "service.name") != "telemetry-integration" || attributeValue(resource, "service.version") != "test-version" || attributeValue(resource, "deployment.environment.name") != "test" {
		t.Fatalf("resource overrides were lost: %v", resource)
	}
	span := traces.ResourceSpans[0].ScopeSpans[0].Spans[0]
	if span.Name != "GET /api/v1/health/live" || hex.EncodeToString(span.TraceId) != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("wrong span: %v", span)
	}
	log := logs.ResourceLogs[0].ScopeLogs[0].LogRecords[0]
	if !bytes.Equal(log.TraceId, span.TraceId) || !bytes.Equal(log.SpanId, span.SpanId) || attributeValue(log.Attributes, "request_id") != "otlp-test" {
		t.Fatalf("log was not correlated: %v", log)
	}
	foundDuration := false
	for _, scope := range metrics.ResourceMetrics[0].ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == "http.server.request.duration" {
				foundDuration = len(metric.GetHistogram().DataPoints) == 1 && metric.GetHistogram().DataPoints[0].Count == 1
			}
		}
	}
	if !foundDuration {
		t.Fatal("request duration was not exported")
	}
	if strings.Contains(traces.String()+metrics.String()+logs.String()+local.String(), "do-not-export") {
		t.Fatal("raw request URL leaked into telemetry")
	}
	t.Log("OTLP receiver decoded traces, metrics, and logs; resource overrides, remote parent, log correlation, and shutdown flush verified")
}

func attributeValue(attrs []*commonpb.KeyValue, key string) string {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value.GetStringValue()
		}
	}
	return ""
}

func TestTelemetryIsOptInAndValidatesConfiguration(t *testing.T) {
	for _, test := range []struct {
		name string
		env  map[string]string
		fail bool
	}{
		{"default disabled", nil, false},
		{"SDK disabled overrides exporters", map[string]string{"OTEL_SDK_DISABLED": "true", "OTEL_TRACES_EXPORTER": "unsupported"}, false},
		{"unsupported exporter", map[string]string{"OTEL_TRACES_EXPORTER": "console"}, true},
		{"unsupported protocol", map[string]string{"OTEL_TRACES_EXPORTER": "otlp", "OTEL_EXPORTER_OTLP_PROTOCOL": "grpc"}, true},
		{"invalid log level", map[string]string{"PORTINAIA_LOG_LEVEL": "verbose"}, true},
		{"invalid endpoint", map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "localhost:4318"}, true},
		{"credentials in endpoint", map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "https://user:secret@collector.example"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cleanEnvironment(t)
			for key, value := range test.env {
				t.Setenv(key, value)
			}
			rt, err := telemetry.New(context.Background(), "test", "test", io.Discard)
			if (err != nil) != test.fail {
				t.Fatalf("configuration error: %v", err)
			}
			if err == nil {
				_, span := rt.TracerProvider.Tracer("test").Start(context.Background(), "disabled")
				if span.IsRecording() {
					t.Error("disabled telemetry recorded a span")
				}
				span.End()
				if err := rt.Shutdown(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
