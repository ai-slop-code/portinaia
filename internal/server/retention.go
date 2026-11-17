package server

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"portinaia/internal/database"
)

const (
	discoveryRetentionAge   = 90 * 24 * time.Hour
	discoveryRetentionBatch = 1000
	discoveryRetentionEvery = time.Hour
)

func RunDiscoveryRetention(ctx context.Context, db *sql.DB, retention time.Duration, batchSize int, interval time.Duration, instrumentation ...*Observability) {
	var observability *Observability
	if len(instrumentation) > 0 {
		observability = instrumentation[0]
	}
	if retention <= 0 {
		retention = discoveryRetentionAge
	}
	if retention < database.MinimumDiscoveryRetention {
		retention = database.MinimumDiscoveryRetention
	}
	if batchSize < 1 {
		batchSize = discoveryRetentionBatch
	}
	if interval <= 0 {
		interval = discoveryRetentionEvery
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			cutoff := now.UTC().Add(-retention).Format(time.RFC3339)
			runRetention(ctx, db, cutoff, batchSize, observability)
		}
	}
}

func runRetention(ctx context.Context, db *sql.DB, cutoff string, batchSize int, o *Observability) {
	logger := slog.Default()
	started := time.Now()
	if o != nil {
		spanCtx, span := o.tracer.Start(ctx, "discovery.retention")
		ctx = spanCtx
		defer span.End()
		logger = o.logger
	}
	stats, err := database.CleanupExpiredDiscoveryReports(ctx, db, cutoff, batchSize)
	result := "success"
	level := slog.LevelInfo
	if err != nil {
		result, level = "error", slog.LevelError
		if ctx.Err() != nil {
			result, level = "canceled", slog.LevelDebug
		}
	}
	if o != nil {
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(attribute.String("result", result), attribute.Int64("reports_deleted", stats.ReportsDeleted), attribute.Int64("contents_deleted", stats.ContentsDeleted))
		if result == "error" {
			span.SetStatus(codes.Error, "discovery retention failed")
		}
		o.retentionRuns.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
		o.retentionTime.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(attribute.String("result", result)))
		o.retentionDeleted.Add(ctx, stats.ReportsDeleted, metric.WithAttributes(attribute.String("entity", "report")))
		o.retentionDeleted.Add(ctx, stats.ContentsDeleted, metric.WithAttributes(attribute.String("entity", "content")))
	}
	logger.Log(ctx, level, "discovery retention completed", "result", result, "reports_deleted", stats.ReportsDeleted, "contents_deleted", stats.ContentsDeleted, "batches", stats.Batches)
}
