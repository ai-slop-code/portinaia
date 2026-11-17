// Package logging defines the structured logging boundary shared by both binaries.
package logging

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

type requestIDKey struct{}

func LevelFromEnv() (slog.Level, error) {
	switch strings.ToLower(os.Getenv("PORTINAIA_LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, errors.New("PORTINAIA_LOG_LEVEL must be debug, info, warn, or error")
	}
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// ValidRequestID bounds correlation headers before they reach logs or HTTP.
func ValidRequestID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r)) {
			return false
		}
	}
	return true
}

// New writes JSON locally and optionally forwards records to additional handlers.
// Correlation is added at Handle time so With/WithGroup preserve request context.
func New(output io.Writer, level slog.Leveler, extra ...slog.Handler) *slog.Logger {
	if level == nil {
		level = slog.LevelInfo
	}
	handlers := append([]slog.Handler{slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level})}, extra...)
	return slog.New(&handler{handlers: handlers, level: level})
}

type handler struct {
	handlers []slog.Handler
	level    slog.Leveler
}

func (h *handler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.level != nil && level < h.level.Level() {
		return false
	}
	for _, next := range h.handlers {
		if next.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *handler) Handle(ctx context.Context, record slog.Record) error {
	if id := RequestID(ctx); id != "" {
		record.AddAttrs(slog.String("request_id", id))
	}
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		record.AddAttrs(slog.String("trace_id", span.TraceID().String()), slog.String("span_id", span.SpanID().String()))
	}
	var result error
	for _, next := range h.handlers {
		if next.Enabled(ctx, record.Level) {
			if err := next.Handle(ctx, record.Clone()); err != nil {
				result = errors.Join(result, err)
			}
		}
	}
	return result
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := &handler{level: h.level}
	for _, child := range h.handlers {
		next.handlers = append(next.handlers, child.WithAttrs(attrs))
	}
	return next
}

func (h *handler) WithGroup(name string) slog.Handler {
	next := &handler{level: h.level}
	for _, child := range h.handlers {
		next.handlers = append(next.handlers, child.WithGroup(name))
	}
	return next
}
