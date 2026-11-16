package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"portinaia/internal/database/generated"
)

// Retain deduplication evidence for the 90-day retry window plus the maximum
// accepted five-minute future collection-clock skew.
const MinimumDiscoveryRetention = 90*24*time.Hour + 5*time.Minute

type RetentionStats struct {
	Cutoff          string `json:"cutoff"`
	ReportsDeleted  int64  `json:"reports_deleted"`
	ContentsDeleted int64  `json:"contents_deleted"`
	Batches         int64  `json:"batches"`
}

// RetentionDryRun counts expired reports and content that would be unreferenced
// after their removal. It does not modify the database.
func RetentionDryRun(ctx context.Context, db *sql.DB, cutoff string) (RetentionStats, error) {
	stats := RetentionStats{Cutoff: cutoff}
	queries := generated.New(db)
	reports, err := queries.CountExpiredDiscoveryReports(ctx, cutoff)
	if err != nil {
		return RetentionStats{}, fmt.Errorf("count expired discovery reports: %w", err)
	}
	stats.ReportsDeleted = reports
	stats.ContentsDeleted, err = queries.CountReclaimableSnapshotContents(ctx, cutoff)
	if err != nil {
		return RetentionStats{}, fmt.Errorf("count unreferenced snapshot content: %w", err)
	}
	return stats, nil
}

// CleanupExpiredDiscoveryReports removes report-owned data in bounded SQLite
// transactions. Foreign keys detach current-state report references and content
// is removed only when no retained report refers to it.
func CleanupExpiredDiscoveryReports(ctx context.Context, db *sql.DB, cutoff string, batchSize int) (RetentionStats, error) {
	return cleanupExpiredDiscoveryReports(ctx, db, cutoff, batchSize, nil)
}

func cleanupExpiredDiscoveryReports(ctx context.Context, db *sql.DB, cutoff string, batchSize int, afterBatch func(RetentionStats)) (RetentionStats, error) {
	if batchSize < 1 || batchSize > 10000 {
		return RetentionStats{}, fmt.Errorf("retention batch size must be between 1 and 10000")
	}
	cutoffTime, err := time.Parse(time.RFC3339, cutoff)
	if err != nil {
		return RetentionStats{}, fmt.Errorf("retention cutoff must be RFC 3339 UTC: %w", err)
	}
	minimumCutoff := time.Now().UTC().Truncate(time.Second).Add(-MinimumDiscoveryRetention)
	if cutoffTime.After(minimumCutoff) {
		return RetentionStats{}, fmt.Errorf("retention cutoff is too recent; retain reports for at least 90 days plus the five-minute accepted collection-clock skew")
	}
	stats := RetentionStats{Cutoff: cutoff}
	for {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return stats, fmt.Errorf("begin discovery retention batch: %w", err)
		}
		queries := generated.New(tx)
		reports, err := queries.DeleteExpiredDiscoveryReportsBatch(ctx, generated.DeleteExpiredDiscoveryReportsBatchParams{Cutoff: cutoff, BatchSize: int64(batchSize)})
		if err != nil {
			_ = tx.Rollback()
			return stats, fmt.Errorf("delete expired discovery reports: %w", err)
		}
		contents, err := queries.DeleteUnreferencedSnapshotContentsBatch(ctx, int64(batchSize))
		if err != nil {
			_ = tx.Rollback()
			return stats, fmt.Errorf("delete unreferenced snapshot content: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return stats, fmt.Errorf("commit discovery retention batch: %w", err)
		}
		if reports == 0 && contents == 0 {
			return stats, nil
		}
		stats.ReportsDeleted += reports
		stats.ContentsDeleted += contents
		stats.Batches++
		if afterBatch != nil {
			afterBatch(stats)
		}
		// Give shutdown and foreground work a chance to run between committed batches.
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return stats, ctx.Err()
		case <-timer.C:
		}
	}
}
