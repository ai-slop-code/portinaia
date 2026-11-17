package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"portinaia/internal/logging"
)

func TestClientPropagatesTraceAndRequestCorrelation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Request-ID") != "agent-request" || r.Header.Get("traceparent") != "00-01000000000000000000000000000000-0200000000000000-01" {
			t.Error("client lost request/trace correlation")
		}
		w.Header().Set("X-Request-ID", r.Header.Get("X-Request-ID"))
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := trace.ContextWithSpanContext(logging.WithRequestID(context.Background(), "agent-request"), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled,
	}))
	response, err := client.request(ctx, http.MethodPost, "/heartbeat", "credential", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var protocolError *HTTPError
	if err := responseError("heartbeat", response); !errors.As(err, &protocolError) || protocolError.RequestID != "agent-request" {
		t.Fatalf("missing response correlation: %v", err)
	}
}
