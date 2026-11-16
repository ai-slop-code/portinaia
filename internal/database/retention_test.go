package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDiscoveryRetentionDeletesExpiredRowsInBatchesAndKeepsSharedContent(t *testing.T) {
	db := openTestDatabase(t)
	ctx := context.Background()
	const (
		computeID = "11111111-1111-4111-8111-111111111111"
		agentID   = "22222222-2222-4222-8222-222222222222"
		sharedID  = "33333333-3333-4333-8333-333333333333"
		orphanID  = "44444444-4444-4444-8444-444444444444"
	)
	if _, err := db.Exec(`INSERT INTO computes(id,kind,display_name,ownership,created_at,updated_at) VALUES(?, 'physical','retention host','owned',?,?)`, computeID, testTime, testTime); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agents(id,compute_id,credential_hash,credential_generation,protocol_version,client_version,enrolled_at) VALUES(?,?,?,1,1,'test',?)`, agentID, computeID, strings.Repeat("a", 64), testTime); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ id, hash string }{{sharedID, strings.Repeat("b", 64)}, {orphanID, strings.Repeat("c", 64)}} {
		if _, err := db.Exec(`INSERT INTO snapshot_contents(id,format_version,content_hash,compressed_document,uncompressed_size,created_at) VALUES(?,1,?,x'01',1,?)`, item.id, item.hash, testTime); err != nil {
			t.Fatal(err)
		}
	}
	insertReport := func(id, reportID, receivedAt, contentID string) {
		t.Helper()
		_, err := db.Exec(`INSERT INTO discovery_reports(id,report_id,agent_id,protocol_version,client_version,received_at,client_collected_at,collection_started_at,collection_finished_at,snapshot_content_id,result,content_reused,request_fingerprint,envelope_version) VALUES(?,?,?,1,'test',?,?,?, ?,?,'accepted',0,?,1)`, id, reportID, agentID, receivedAt, receivedAt, receivedAt, receivedAt, contentID, strings.Repeat("d", 64))
		if err != nil {
			t.Fatal(err)
		}
	}
	insertReport("55555555-5555-4555-8555-555555555551", "66666666-6666-4666-8666-666666666661", "2026-01-01T00:00:00Z", sharedID)
	insertReport("55555555-5555-4555-8555-555555555552", "66666666-6666-4666-8666-666666666662", "2026-04-01T00:00:00Z", sharedID)
	insertReport("55555555-5555-4555-8555-555555555553", "66666666-6666-4666-8666-666666666663", "2026-03-31T23:59:59Z", orphanID)
	if _, err := db.Exec(`INSERT INTO compute_observations(compute_id,hostname,architecture,observed_at,received_at,current_agent_id,current_report_id) VALUES(?,'retained-host','arm64','2026-03-31T23:59:59Z','2026-03-31T23:59:59Z',?,?)`, computeID, agentID, "55555555-5555-4555-8555-555555555553"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO audit_events(id,occurred_at,actor_type,actor_id,action,resource_type,resource_id,result) VALUES('77777777-7777-4777-8777-777777777777',?,'system','test','inventory.update','compute',?,'success')`, testTime, computeID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ip_assignments(id,address,family,prefix_length,compute_id,scope,role,source,description,created_at,updated_at) VALUES('88888888-8888-4888-8888-888888888888','192.0.2.42',4,24,?,'private','management','manual','manual metadata',?,?)`, computeID, testTime, testTime); err != nil {
		t.Fatal(err)
	}
	cutoff := "2026-04-01T00:00:00Z"
	dryRun, err := RetentionDryRun(ctx, db, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if dryRun.ReportsDeleted != 2 || dryRun.ContentsDeleted != 1 {
		t.Fatalf("dry-run stats = %+v", dryRun)
	}
	var before int
	if err := db.QueryRow(`SELECT count(*) FROM discovery_reports`).Scan(&before); err != nil || before != 3 {
		t.Fatalf("dry run changed reports: count=%d err=%v", before, err)
	}
	if _, err := db.Exec(`CREATE TRIGGER retention_test_abort BEFORE DELETE ON discovery_reports WHEN OLD.id='55555555-5555-4555-8555-555555555551' BEGIN SELECT RAISE(ABORT,'retention rollback test'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := CleanupExpiredDiscoveryReports(ctx, db, cutoff, 1); err == nil {
		t.Fatal("cleanup succeeded despite injected delete failure")
	}
	if _, err := db.Exec(`DROP TRIGGER retention_test_abort`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM discovery_reports`).Scan(&before); err != nil || before != 3 {
		t.Fatalf("failed cleanup did not roll back: reports=%d err=%v", before, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := CleanupExpiredDiscoveryReports(cancelled, db, cutoff, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled retention error = %v", err)
	}
	var reportsBeforeResume int
	if err := db.QueryRow(`SELECT count(*) FROM discovery_reports`).Scan(&reportsBeforeResume); err != nil || reportsBeforeResume != 3 {
		t.Fatalf("cancelled cleanup changed report rows: %d err=%v", reportsBeforeResume, err)
	}
	stats, err := CleanupExpiredDiscoveryReports(ctx, db, cutoff, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stats.ReportsDeleted != 2 || stats.ContentsDeleted != 1 || stats.Batches < 2 {
		t.Fatalf("cleanup stats = %+v", stats)
	}
	repeated, err := CleanupExpiredDiscoveryReports(ctx, db, cutoff, 1)
	if err != nil || repeated.ReportsDeleted != 0 || repeated.ContentsDeleted != 0 {
		t.Fatalf("repeated cleanup = %+v err=%v", repeated, err)
	}
	if _, err := db.Exec(`INSERT INTO discovery_reports(id,report_id,agent_id,protocol_version,client_version,received_at,client_collected_at,collection_started_at,collection_finished_at,snapshot_content_id,result,content_reused,request_fingerprint,envelope_version) VALUES('55555555-5555-4555-8555-555555555554','66666666-6666-4666-8666-666666666664',?,1,'test','2026-01-02T00:00:00Z','2026-01-02T00:00:00Z','2026-01-02T00:00:00Z','2026-01-02T00:00:00Z',?,'accepted',0,?,1)`, agentID, sharedID, strings.Repeat("e", 64)); err != nil {
		t.Fatal(err)
	}
	type cleanupResult struct {
		stats RetentionStats
		err   error
	}
	cleanupDone := make(chan cleanupResult, 1)
	ingestDone := make(chan error, 1)
	go func() {
		stats, err := CleanupExpiredDiscoveryReports(ctx, db, cutoff, 1)
		cleanupDone <- cleanupResult{stats: stats, err: err}
	}()
	go func() {
		_, err := db.Exec(`INSERT INTO discovery_reports(id,report_id,agent_id,protocol_version,client_version,received_at,client_collected_at,collection_started_at,collection_finished_at,snapshot_content_id,result,content_reused,request_fingerprint,envelope_version) VALUES('55555555-5555-4555-8555-555555555555','66666666-6666-4666-8666-666666666665',?,1,'test','2026-04-02T00:00:00Z','2026-04-02T00:00:00Z','2026-04-02T00:00:00Z','2026-04-02T00:00:00Z',?,'accepted',1,?,1)`, agentID, sharedID, strings.Repeat("f", 64))
		ingestDone <- err
	}()
	concurrentCleanup := <-cleanupDone
	if err := <-ingestDone; err != nil {
		t.Fatalf("concurrent retained report ingestion failed: %v", err)
	}
	if concurrentCleanup.err != nil || concurrentCleanup.stats.ReportsDeleted != 1 || concurrentCleanup.stats.ContentsDeleted != 0 {
		t.Fatalf("concurrent retention result = %+v err=%v", concurrentCleanup.stats, concurrentCleanup.err)
	}
	for index := 0; index < 4; index++ {
		id := fmt.Sprintf("99999999-9999-4999-8999-%012x", index+1)
		reportID := fmt.Sprintf("aaaaaaaa-aaaa-4aaa-8aaa-%012x", index+1)
		_, err := db.Exec(`INSERT INTO discovery_reports(id,report_id,agent_id,protocol_version,client_version,received_at,client_collected_at,collection_started_at,collection_finished_at,snapshot_content_id,result,content_reused,request_fingerprint,envelope_version,collection_error_summary) VALUES(?,?,?,1,'test','2026-01-03T00:00:00Z','2026-01-03T00:00:00Z','2026-01-03T00:00:00Z','2026-01-03T00:00:00Z',NULL,'rejected',NULL,?,1,'rejected for fixture')`, id, reportID, agentID, strings.Repeat("a", 64))
		if err != nil {
			t.Fatal(err)
		}
	}
	partialCtx, stopPartial := context.WithCancel(ctx)
	partial, partialErr := cleanupExpiredDiscoveryReports(partialCtx, db, cutoff, 1, func(stats RetentionStats) {
		if stats.ReportsDeleted == 1 {
			stopPartial()
		}
	})
	stopPartial()
	if !errors.Is(partialErr, context.Canceled) || partial.ReportsDeleted != 1 {
		t.Fatalf("mid-run interruption stats/error = %+v / %v", partial, partialErr)
	}
	resumed, err := CleanupExpiredDiscoveryReports(ctx, db, cutoff, 1)
	if err != nil || resumed.ReportsDeleted != 3 {
		t.Fatalf("resumed cleanup stats/error = %+v / %v", resumed, err)
	}
	var reports, contents int
	if err := db.QueryRow(`SELECT count(*) FROM discovery_reports`).Scan(&reports); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM snapshot_contents`).Scan(&contents); err != nil {
		t.Fatal(err)
	}
	if reports != 2 || contents != 1 {
		t.Fatalf("retained reports/content = %d/%d, want 2/1", reports, contents)
	}
	var hostname string
	var currentReport sql.NullString
	if err := db.QueryRow(`SELECT hostname,current_report_id FROM compute_observations WHERE compute_id=?`, computeID).Scan(&hostname, &currentReport); err != nil {
		t.Fatal(err)
	}
	if hostname != "retained-host" || currentReport.Valid {
		t.Fatalf("retention changed current facts or left dangling report reference: hostname=%q report=%v", hostname, currentReport)
	}
	var auditCount int
	if err := db.QueryRow(`SELECT count(*) FROM audit_events`).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("retention changed audit records: count=%d err=%v", auditCount, err)
	}
	var manualDescription string
	if err := db.QueryRow(`SELECT description FROM ip_assignments WHERE address='192.0.2.42'`).Scan(&manualDescription); err != nil || manualDescription != "manual metadata" {
		t.Fatalf("retention changed manual IP metadata: %q err=%v", manualDescription, err)
	}
	var retainedContentCount int
	if err := db.QueryRow(`SELECT count(*) FROM discovery_reports WHERE snapshot_content_id=?`, sharedID).Scan(&retainedContentCount); err != nil || retainedContentCount != 2 {
		t.Fatalf("retained shared content references = %d err=%v", retainedContentCount, err)
	}
}
