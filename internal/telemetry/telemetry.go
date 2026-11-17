// Package telemetry owns server-side OpenTelemetry SDK/exporter lifetimes.
// Application packages receive providers explicitly and never mutate globals.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"portinaia/internal/logging"
)

const ShutdownTimeout = 10 * time.Second

type Runtime struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Logger         *slog.Logger
	shutdown       []func(context.Context) error
	once           sync.Once
	shutdownErr    error
}

// New configures OTLP/HTTP protobuf exporters using standard OTEL_* variables.
// Export is opt-in: setting an endpoint enables all signals unless individually
// disabled; OTEL_{TRACES,METRICS,LOGS}_EXPORTER=otlp also enables a signal.
func New(ctx context.Context, service, version string, output io.Writer) (_ *Runtime, err error) {
	level, err := logging.LevelFromEnv()
	if err != nil {
		return nil, err
	}
	rt := &Runtime{TracerProvider: tracenoop.NewTracerProvider(), MeterProvider: metricnoop.NewMeterProvider(), Logger: logging.New(output, level)}
	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
			defer cancel()
			err = errors.Join(err, rt.Shutdown(cleanup))
		}
	}()
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") {
		return rt, nil
	}
	// Validate every signal before constructing any background exporters.
	enabled := make(map[string]bool)
	for _, signal := range []string{"TRACES", "METRICS", "LOGS"} {
		enabled[signal], err = exporterEnabled(signal)
		if err != nil {
			return nil, err
		}
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", service), attribute.String("service.version", version)),
		resource.WithFromEnv(), resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}
	if enabled["TRACES"] {
		exporter, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("create trace exporter: %w", err)
		}
		provider := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(exporter))
		rt.TracerProvider = provider
		rt.shutdown = append(rt.shutdown, provider.Shutdown)
	}
	if enabled["METRICS"] {
		exporter, err := otlpmetrichttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("create metric exporter: %w", err)
		}
		provider := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)))
		rt.MeterProvider = provider
		rt.shutdown = append(rt.shutdown, provider.Shutdown)
	}
	if enabled["LOGS"] {
		exporter, err := otlploghttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("create log exporter: %w", err)
		}
		provider := sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)))
		rt.Logger = logging.New(output, level, otelslog.NewHandler("portinaia", otelslog.WithLoggerProvider(provider)))
		rt.shutdown = append(rt.shutdown, provider.Shutdown)
	}
	return rt, nil
}

func exporterEnabled(signal string) (bool, error) {
	value := strings.TrimSpace(os.Getenv("OTEL_" + signal + "_EXPORTER"))
	if value == "" {
		if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_"+signal+"_ENDPOINT") == "" {
			return false, nil
		}
		value = "otlp"
	}
	if value == "none" {
		return false, nil
	}
	if value != "otlp" {
		return false, fmt.Errorf("OTEL_%s_EXPORTER must be otlp or none", signal)
	}
	protocol := os.Getenv("OTEL_EXPORTER_OTLP_" + signal + "_PROTOCOL")
	if protocol == "" {
		protocol = os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	}
	if protocol != "" && protocol != "http/protobuf" {
		return false, fmt.Errorf("OTLP %s protocol must be http/protobuf", strings.ToLower(signal))
	}
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_" + signal + "_ENDPOINT")
	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	if endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return false, fmt.Errorf("OTLP %s endpoint must be an absolute HTTP(S) URL without credentials, query, or fragment", strings.ToLower(signal))
		}
	}
	return true, nil
}

// Shutdown flushes all signals once. Each provider receives the caller's bounded
// context even if another provider fails, and all errors are preserved.
func (rt *Runtime) Shutdown(ctx context.Context) error {
	rt.once.Do(func() {
		// Flush concurrently so an unavailable signal cannot consume the entire
		// shutdown budget before other signals have a chance to export.
		errs := make([]error, len(rt.shutdown))
		var group sync.WaitGroup
		for index, shutdown := range rt.shutdown {
			group.Add(1)
			go func() {
				defer group.Done()
				errs[index] = shutdown(ctx)
			}()
		}
		group.Wait()
		rt.shutdownErr = errors.Join(errs...)
	})
	return rt.shutdownErr
}
