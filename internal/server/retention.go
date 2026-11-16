package server

import (
	"context"
	"database/sql"
	"log"
	"time"

	"portinaia/internal/database"
)

const (
	discoveryRetentionAge   = 90 * 24 * time.Hour
	discoveryRetentionBatch = 1000
	discoveryRetentionEvery = time.Hour
)

func RunDiscoveryRetention(ctx context.Context, db *sql.DB, retention time.Duration, batchSize int, interval time.Duration) {
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
			stats, err := database.CleanupExpiredDiscoveryReports(ctx, db, cutoff, batchSize)
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("discovery retention failed: reports_deleted=%d content_deleted=%d batches=%d error_type=%T", stats.ReportsDeleted, stats.ContentsDeleted, stats.Batches, err)
				}
				continue
			}
			log.Printf("discovery retention complete: reports_deleted=%d content_deleted=%d batches=%d", stats.ReportsDeleted, stats.ContentsDeleted, stats.Batches)
		}
	}
}
