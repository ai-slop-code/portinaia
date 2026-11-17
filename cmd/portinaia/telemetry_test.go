package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"portinaia/internal/database"
)

func TestServeSIGTERMFlushesAllTelemetrySignals(t *testing.T) {
	var mu sync.Mutex
	payloads := map[string][]byte{}
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		mu.Lock()
		payloads[r.URL.Path] = append(payloads[r.URL.Path], body...)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer collector.Close()
	databasePath := filepath.Join(t.TempDir(), "telemetry.db")
	db, err := database.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(context.Background(), db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestServeSignalHelper$")
	command.Env = append(os.Environ(),
		"PORTINAIA_SERVE_SIGNAL_HELPER=1", "PORTINAIA_TEST_LISTEN="+address, "PORTINAIA_TEST_DATABASE="+databasePath,
		"OTEL_SDK_DISABLED=false", "OTEL_EXPORTER_OTLP_ENDPOINT="+collector.URL, "OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf",
		"OTEL_TRACES_EXPORTER=otlp", "OTEL_METRICS_EXPORTER=otlp", "OTEL_LOGS_EXPORTER=otlp", "OTEL_TRACES_SAMPLER=always_on",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT=", "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=",
		"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=", "OTEL_EXPORTER_OTLP_METRICS_PROTOCOL=", "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL=",
		"OTEL_BSP_SCHEDULE_DELAY=600000", "OTEL_BLRP_SCHEDULE_DELAY=600000", "OTEL_METRIC_EXPORT_INTERVAL=600000", "PORTINAIA_LOG_LEVEL=debug",
	)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	var processErr error
	go func() {
		processErr = command.Wait()
		close(exited)
	}()
	defer func() {
		_ = command.Process.Kill()
		<-exited
	}()
	client := &http.Client{Timeout: time.Second}
	for {
		response, err := client.Get("http://" + address + "/api/v1/health/live")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				break
			}
		}
		select {
		case <-exited:
			t.Fatalf("server exited before ready: %v\n%s", processErr, output.String())
		case <-ctx.Done():
			t.Fatal("server did not start before deadline")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	<-exited
	if processErr != nil {
		t.Fatalf("shutdown failed: %v\n%s", processErr, output.String())
	}
	mu.Lock()
	defer mu.Unlock()
	for _, path := range []string{"/v1/traces", "/v1/metrics", "/v1/logs"} {
		if len(payloads[path]) == 0 {
			t.Errorf("SIGTERM failed to flush %s", path)
		}
	}
	for _, name := range []string{"http.server.request.duration", "portinaia.db.connections", "portinaia.process.goroutines"} {
		if !bytes.Contains(payloads["/v1/metrics"], []byte(name)) {
			t.Errorf("final metric collection missing %s", name)
		}
	}
	if !bytes.Contains(payloads["/v1/logs"], []byte("server stopped")) {
		t.Error("shutdown lifecycle log was not exported")
	}
	t.Log("server subprocess received SIGTERM, exported all three signals including final DB/runtime metrics and shutdown log, and exited successfully")
}
