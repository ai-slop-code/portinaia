package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestLoggerPreservesContextGroupsAndLevelAcrossOutputs(t *testing.T) {
	var local, additional bytes.Buffer
	logger := New(&local, slog.LevelWarn, slog.NewJSONHandler(&additional, &slog.HandlerOptions{Level: slog.LevelDebug}))
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(WithRequestID(context.Background(), "request-test"), spanContext)
	logger = logger.With("component", "worker").WithGroup("operation").With("name", "test")
	logger.InfoContext(ctx, "hidden")
	logger.WarnContext(ctx, "visible", "attempt", 1)
	for _, output := range []*bytes.Buffer{&local, &additional} {
		var record struct {
			Message   string `json:"msg"`
			Component string `json:"component"`
			Operation struct {
				Name      string `json:"name"`
				RequestID string `json:"request_id"`
				TraceID   string `json:"trace_id"`
				SpanID    string `json:"span_id"`
			} `json:"operation"`
		}
		if bytes.Count(output.Bytes(), []byte("\n")) != 1 {
			t.Fatalf("level filter not applied to both outputs: %s", output.String())
		}
		if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
			t.Fatal(err)
		}
		if record.Message != "visible" || record.Component != "worker" || record.Operation.Name != "test" || record.Operation.RequestID != "request-test" || record.Operation.TraceID != spanContext.TraceID().String() || record.Operation.SpanID != spanContext.SpanID().String() {
			t.Fatalf("lost structured fields/correlation: %+v", record)
		}
	}
}
