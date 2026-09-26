package database

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"portinaia/internal/database/generated"
)

const testTime = "2026-08-03T12:34:56Z"

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "portinaia.db"))
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func expectConstraintFailure(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err == nil {
		t.Fatal("expected database constraint failure")
	}
}

func TestCleanMigrationAndDependencySafeDown(t *testing.T) {
	db := openTestDatabase(t)

	var tableCount int
	if err := db.QueryRow(`
		SELECT count(*) FROM sqlite_schema
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		  AND name <> 'goose_db_version'
	`).Scan(&tableCount); err != nil {
		t.Fatalf("count migrated tables: %v", err)
	}
	if tableCount != 40 {
		t.Fatalf("application table count = %d, want 40", tableCount)
	}

	version, err := goose.GetDBVersion(db)
	if err != nil {
		t.Fatalf("get schema version: %v", err)
	}
	if version != 1 {
		t.Fatalf("schema version = %d, want 1", version)
	}
	applicationVersion, err := SchemaVersion(context.Background(), db)
	if err != nil {
		t.Fatalf("get application schema version: %v", err)
	}
	if applicationVersion != 1 {
		t.Fatalf("application schema version = %d, want 1", applicationVersion)
	}
	var userVersion int
	if err := db.QueryRow("PRAGMA user_version").Scan(&userVersion); err != nil {
		t.Fatalf("get SQLite user version: %v", err)
	}
	if userVersion != 1 {
		t.Fatalf("SQLite user version = %d, want 1", userVersion)
	}

	var foreignKeyViolation string
	err = db.QueryRow("PRAGMA foreign_key_check").Scan(&foreignKeyViolation)
	if err != sql.ErrNoRows {
		t.Fatalf("foreign key check returned a violation: value=%q err=%v", foreignKeyViolation, err)
	}

	if err := MigrateDown(context.Background(), db); err != nil {
		t.Fatalf("roll back database: %v", err)
	}
	if err := db.QueryRow(`
		SELECT count(*) FROM sqlite_schema
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		  AND name <> 'goose_db_version'
	`).Scan(&tableCount); err != nil {
		t.Fatalf("count tables after rollback: %v", err)
	}
	if tableCount != 0 {
		t.Fatalf("application table count after rollback = %d, want 0", tableCount)
	}
	if err := db.QueryRow("PRAGMA user_version").Scan(&userVersion); err != nil {
		t.Fatalf("get SQLite user version after rollback: %v", err)
	}
	if userVersion != 0 {
		t.Fatalf("SQLite user version after rollback = %d, want 0", userVersion)
	}
}

func TestAuthenticationAndCanonicalValueConstraints(t *testing.T) {
	db := openTestDatabase(t)

	adminID := "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4"
	_, err := db.Exec(`
		INSERT INTO administrators
			(id, username, password_hash, password_algorithm, password_parameters, created_at, updated_at)
		VALUES (?, 'admin', ?, 'argon2id', '{}', ?, ?)
	`, adminID, "hashhashhashhashhashhashhashhash", testTime, testTime)
	if err != nil {
		t.Fatalf("insert administrator fixture: %v", err)
	}

	expectConstraintFailure(t, db, `
		INSERT INTO administrators
			(id, username, password_hash, password_algorithm, password_parameters, created_at, updated_at)
		VALUES (?, 'other', ?, 'argon2id', '{}', ?, ?)
	`, "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b5", "hashhashhashhashhashhashhashhash", testTime, testTime)

	expectConstraintFailure(t, db, `
		INSERT INTO providers (id, name, revision, created_at, updated_at)
		VALUES (?, 'bad uuid', 1, ?, ?)
	`, "0190D7B6-8EF9-7CA1-A8C4-6BCBCA5D73B6", testTime, testTime)

	expectConstraintFailure(t, db, `
		INSERT INTO providers (id, name, revision, created_at, updated_at)
		VALUES (?, 'bad revision', 0, ?, ?)
	`, "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b6", testTime, testTime)

	expectConstraintFailure(t, db, `
		INSERT INTO providers (id, name, revision, created_at, updated_at)
		VALUES (?, 'bad time', 1, '2026-08-03 12:34:56', ?)
	`, "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b6", testTime)

	invalidTimes := []struct {
		id    string
		name  string
		value string
	}{
		{"0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b7", "fractional time", "2026-08-03T12:34:56.123Z"},
		{"0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b8", "offset time", "2026-08-03T14:34:56+02:00"},
		{"0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b9", "invalid day", "2026-02-30T12:34:56Z"},
	}
	for _, invalidTime := range invalidTimes {
		expectConstraintFailure(t, db, `
			INSERT INTO providers (id, name, revision, created_at, updated_at)
			VALUES (?, ?, 1, ?, ?)
		`, invalidTime.id, invalidTime.name, invalidTime.value, testTime)
	}

	if _, err := db.Exec(`
		INSERT INTO idempotency_keys (
			actor_type, actor_id, scope, key_hash, request_fingerprint, created_at, expires_at
		) VALUES ('administrator', ?, 'dns.apply', ?, ?, ?, '2026-08-03T13:34:56Z')
	`, adminID, "keyhashkeyhashkeyhashkeyhashkeyhash", "fingerprintfingerprintfingerprintfingerprint", testTime); err != nil {
		t.Fatalf("insert idempotency fixture: %v", err)
	}
	expectConstraintFailure(t, db, `
		INSERT INTO idempotency_keys (
			actor_type, actor_id, scope, key_hash, request_fingerprint, created_at, expires_at
		) VALUES ('administrator', ?, 'dns.apply', ?, ?, ?, '2026-08-03T13:34:56Z')
	`, adminID, "keyhashkeyhashkeyhashkeyhashkeyhash", "differentfingerprintdifferentfingerprint", testTime)
}

func TestOwnershipMoneyAndBooleanConstraints(t *testing.T) {
	db := openTestDatabase(t)
	computeID := "0190d7c0-28f3-71a2-a0fd-d17dd083462a"
	domainID := "0190d7c0-28f3-71a2-a0fd-d17dd083462b"

	if _, err := db.Exec(`
		INSERT INTO computes (id, kind, display_name, ownership, created_at, updated_at)
		VALUES (?, 'physical', 'host', 'owned', ?, ?)
	`, computeID, testTime, testTime); err != nil {
		t.Fatalf("insert compute fixture: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO domains (id, normalized_name, auto_renew, created_at, updated_at)
		VALUES (?, 'example.com', 1, ?, ?)
	`, domainID, testTime, testTime); err != nil {
		t.Fatalf("insert domain fixture: %v", err)
	}

	expectConstraintFailure(t, db, `
		INSERT INTO costs (
			id, compute_id, domain_id, type, amount_minor, currency_code, currency_exponent,
			billing_interval, effective_start_date, created_at, updated_at
		) VALUES (?, ?, ?, 'recurring', 1000, 'EUR', 2, 'monthly', '2026-08-03', ?, ?)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd083462c", computeID, domainID, testTime, testTime)

	expectConstraintFailure(t, db, `
		INSERT INTO costs (
			id, compute_id, type, amount_minor, currency_code, currency_exponent,
			billing_interval, effective_start_date, created_at, updated_at
		) VALUES (?, ?, 'recurring', 1000, 'eur', 2, 'monthly', '2026-08-03', ?, ?)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd083462d", computeID, testTime, testTime)

	expectConstraintFailure(t, db, `
		INSERT INTO domains (id, normalized_name, auto_renew, created_at, updated_at)
		VALUES (?, 'invalid.example', 2, ?, ?)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd083462e", testTime, testTime)

	expectConstraintFailure(t, db, `
		INSERT INTO ip_assignments (
			id, address, family, compute_id, container_id, scope, role, source,
			created_at, updated_at
		) VALUES (?, '192.0.2.1', 4, ?, NULL, 'private', 'primary', 'manual', ?, ?)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd083462f", nil, testTime, testTime)
}

func TestReportIdempotencyAndSnapshotDeduplication(t *testing.T) {
	db := openTestDatabase(t)
	computeID := "0190d7c0-28f3-71a2-a0fd-d17dd0834630"
	agentID := "0190d7c0-28f3-71a2-a0fd-d17dd0834631"
	snapshotID := "0190d7c0-28f3-71a2-a0fd-d17dd0834632"
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	if _, err := db.Exec(`INSERT INTO computes (id, kind, display_name, ownership, created_at, updated_at) VALUES (?, 'vps', 'agent host', 'rented', ?, ?)`, computeID, testTime, testTime); err != nil {
		t.Fatalf("insert compute fixture: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO agents (id, compute_id, credential_hash, credential_generation, protocol_version, client_version, enrolled_at)
		VALUES (?, ?, ?, 1, 1, 'test', ?)
	`, agentID, computeID, "agentcredentialhashagentcredentialhash", testTime); err != nil {
		t.Fatalf("insert agent fixture: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO snapshot_contents (id, format_version, content_hash, compressed_document, uncompressed_size, created_at)
		VALUES (?, 1, ?, x'01', 1, ?)
	`, snapshotID, hash, testTime); err != nil {
		t.Fatalf("insert snapshot fixture: %v", err)
	}
	expectConstraintFailure(t, db, `
		INSERT INTO snapshot_contents (id, format_version, content_hash, compressed_document, uncompressed_size, created_at)
		VALUES (?, 1, ?, x'02', 1, ?)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834633", hash, testTime)

	reportID := "0190d7c0-28f3-71a2-a0fd-d17dd0834634"
	if _, err := db.Exec(`
		INSERT INTO discovery_reports
			(id, report_id, agent_id, protocol_version, received_at, client_collected_at, snapshot_content_id, result)
		VALUES (?, ?, ?, 1, ?, ?, ?, 'accepted')
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834635", reportID, agentID, testTime, testTime, snapshotID); err != nil {
		t.Fatalf("insert report fixture: %v", err)
	}
	expectConstraintFailure(t, db, `
		INSERT INTO discovery_reports
			(id, report_id, agent_id, protocol_version, received_at, client_collected_at, snapshot_content_id, result)
		VALUES (?, ?, ?, 1, ?, ?, ?, 'accepted')
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834636", reportID, agentID, testTime, testTime, snapshotID)
}

func TestDNSOneOwnerRecordConflictsAndLeaseConstraints(t *testing.T) {
	db := openTestDatabase(t)
	domainID := "0190d7c0-28f3-71a2-a0fd-d17dd0834640"
	zoneID := "0190d7c0-28f3-71a2-a0fd-d17dd0834641"
	nameID := "0190d7c0-28f3-71a2-a0fd-d17dd0834642"
	computeID := "0190d7c0-28f3-71a2-a0fd-d17dd0834643"

	if _, err := db.Exec(`INSERT INTO domains (id, normalized_name, created_at, updated_at) VALUES (?, 'example.net', ?, ?)`, domainID, testTime, testTime); err != nil {
		t.Fatalf("insert domain fixture: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO computes (id, kind, display_name, ownership, created_at, updated_at) VALUES (?, 'physical', 'dns host', 'owned', ?, ?)`, computeID, testTime, testTime); err != nil {
		t.Fatalf("insert compute fixture: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dns_zones (
			id, cloudflare_zone_id, normalized_name, cloudflare_status, selection_state,
			domain_id, discovered_at, refreshed_at, created_at, updated_at
		) VALUES (?, 'cf-zone', 'example.net', 'active', 'managed', ?, ?, ?, ?, ?)
	`, zoneID, domainID, testTime, testTime, testTime, testTime); err != nil {
		t.Fatalf("insert zone fixture: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dns_names (id, zone_id, fqdn, is_wildcard, lifecycle_state, created_at, updated_at)
		VALUES (?, ?, 'www.example.net', 0, 'active', ?, ?)
	`, nameID, zoneID, testTime, testTime); err != nil {
		t.Fatalf("insert dns name fixture: %v", err)
	}

	expectConstraintFailure(t, db, `
		INSERT INTO dns_inventory_links (id, dns_name_id, compute_id, container_id, ip_assignment_id)
		VALUES (?, ?, NULL, NULL, NULL)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834644", nameID)

	if _, err := db.Exec(`
		INSERT INTO dns_record_sets (id, dns_name_id, view, type, ttl, proxied, sort_order)
		VALUES (?, ?, 'public', 'A', 60, 0, 0)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834645", nameID); err != nil {
		t.Fatalf("insert A record set: %v", err)
	}
	expectConstraintFailure(t, db, `
		INSERT INTO dns_record_sets (id, dns_name_id, view, type, ttl, proxied, sort_order)
		VALUES (?, ?, 'public', 'CNAME', 60, 0, 1)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834646", nameID)

	expectConstraintFailure(t, db, `
		INSERT INTO dns_projections (
			id, dns_name_id, provider, desired_revision, applied_revision, state,
			lease_owner, updated_at
		) VALUES (?, ?, 'cloudflare', 2, 0, 'pending', 'worker-1', ?)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834647", nameID, testTime)

	if _, err := db.Exec(`
		INSERT INTO dns_projections (
			id, dns_name_id, provider, desired_revision, applied_revision, state,
			lease_owner, lease_expires_at, leased_revision, updated_at
		) VALUES (?, ?, 'cloudflare', 2, 0, 'pending', 'worker-1', '2026-08-03T12:35:56Z', 2, ?)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834648", nameID, testTime); err != nil {
		t.Fatalf("insert leased projection: %v", err)
	}

	expectConstraintFailure(t, db, `
		UPDATE dns_projections SET applied_revision = 3
		WHERE id = ?
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834648")
}

func TestDNSProjectionGeneratedQueries(t *testing.T) {
	db := openTestDatabase(t)
	ctx := context.Background()
	queries := generated.New(db)
	domainID := "0190d7c0-28f3-71a2-a0fd-d17dd0834650"
	zoneID := "0190d7c0-28f3-71a2-a0fd-d17dd0834651"
	nameID := "0190d7c0-28f3-71a2-a0fd-d17dd0834652"
	projectionID := "0190d7c0-28f3-71a2-a0fd-d17dd0834653"

	if _, err := db.Exec(`INSERT INTO domains (id, normalized_name, created_at, updated_at) VALUES (?, 'workers.example', ?, ?)`, domainID, testTime, testTime); err != nil {
		t.Fatalf("insert domain fixture: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dns_zones (
			id, cloudflare_zone_id, normalized_name, cloudflare_status, selection_state,
			domain_id, discovered_at, refreshed_at, created_at, updated_at
		) VALUES (?, 'cf-workers', 'workers.example', 'active', 'managed', ?, ?, ?, ?, ?)
	`, zoneID, domainID, testTime, testTime, testTime, testTime); err != nil {
		t.Fatalf("insert zone fixture: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dns_names (
			id, zone_id, fqdn, is_wildcard, desired_revision, lifecycle_state, created_at, updated_at
		) VALUES (?, ?, 'www.workers.example', 0, 2, 'active', ?, ?)
	`, nameID, zoneID, testTime, testTime); err != nil {
		t.Fatalf("insert DNS name fixture: %v", err)
	}

	schedule := generated.ScheduleDNSProjectionParams{
		ID:              projectionID,
		DnsNameID:       nameID,
		Provider:        "cloudflare",
		DesiredRevision: 2,
		State:           "pending",
		UpdatedAt:       "2026-08-03T12:35:00Z",
	}
	if _, err := queries.ScheduleDNSProjection(ctx, schedule); err != nil {
		t.Fatalf("schedule projection: %v", err)
	}

	claimed, err := queries.ClaimDNSProjections(ctx, generated.ClaimDNSProjectionsParams{
		LeaseOwner:     "worker-1",
		LeaseExpiresAt: "2026-08-03T12:40:00Z",
		ClaimedAt:      "2026-08-03T12:36:00Z",
		BatchSize:      1,
	})
	if err != nil {
		t.Fatalf("claim projection: %v", err)
	}
	if len(claimed) != 1 || claimed[0].AttemptCount != 1 || claimed[0].LastAttemptAt.String != "2026-08-03T12:36:00Z" {
		t.Fatalf("claimed projection = %#v, want one attempt recorded", claimed)
	}

	for _, revision := range []int64{2, 1} {
		rejected := schedule
		rejected.DesiredRevision = revision
		rejected.State = "failed"
		rejected.UpdatedAt = "2026-08-03T12:37:00Z"
		if _, err := queries.ScheduleDNSProjection(ctx, rejected); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("schedule revision %d error = %v, want sql.ErrNoRows", revision, err)
		}
	}
	preserved, err := queries.GetDNSProjection(ctx, projectionID)
	if err != nil {
		t.Fatalf("get preserved projection: %v", err)
	}
	if preserved.DesiredRevision != 2 || preserved.AttemptCount != 1 || preserved.LeaseOwner.String != "worker-1" {
		t.Fatalf("same/lower schedule changed projection: %#v", preserved)
	}

	rows, err := queries.CompleteDNSProjection(ctx, generated.CompleteDNSProjectionParams{
		AppliedRevision: 2,
		CompletedAt:     "2026-08-03T12:40:00Z",
		ID:              projectionID,
		LeaseOwner:      "worker-1",
	})
	if err != nil || rows != 0 {
		t.Fatalf("expired completion rows = %d, err = %v, want 0, nil", rows, err)
	}
	rows, err = queries.CompleteDNSProjection(ctx, generated.CompleteDNSProjectionParams{
		AppliedRevision: 2,
		CompletedAt:     "2026-08-03T12:39:00Z",
		ID:              projectionID,
		LeaseOwner:      "worker-1",
	})
	if err != nil || rows != 1 {
		t.Fatalf("valid completion rows = %d, err = %v, want 1, nil", rows, err)
	}
	completed, err := queries.GetDNSProjection(ctx, projectionID)
	if err != nil {
		t.Fatalf("get completed projection: %v", err)
	}
	if completed.AttemptCount != 1 || completed.AppliedRevision != 2 || completed.State != "synchronized" {
		t.Fatalf("completed projection = %#v, want one synchronized attempt", completed)
	}

	if _, err := db.Exec(`UPDATE dns_names SET desired_revision = 3, updated_at = '2026-08-03T12:41:00Z' WHERE id = ?`, nameID); err != nil {
		t.Fatalf("advance DNS name revision: %v", err)
	}
	schedule.DesiredRevision = 3
	schedule.UpdatedAt = "2026-08-03T12:41:00Z"
	if _, err := queries.ScheduleDNSProjection(ctx, schedule); err != nil {
		t.Fatalf("schedule newer projection: %v", err)
	}
	claimed, err = queries.ClaimDNSProjections(ctx, generated.ClaimDNSProjectionsParams{
		LeaseOwner:     "worker-2",
		LeaseExpiresAt: "2026-08-03T12:50:00Z",
		ClaimedAt:      "2026-08-03T12:42:00Z",
		BatchSize:      1,
	})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim newer projection = %#v, err = %v", claimed, err)
	}
	if _, err := db.Exec(`UPDATE dns_names SET desired_revision = 4, updated_at = '2026-08-03T12:43:00Z' WHERE id = ?`, nameID); err != nil {
		t.Fatalf("advance DNS name beyond lease: %v", err)
	}
	rows, err = queries.CompleteDNSProjection(ctx, generated.CompleteDNSProjectionParams{
		AppliedRevision: 3,
		CompletedAt:     "2026-08-03T12:44:00Z",
		ID:              projectionID,
		LeaseOwner:      "worker-2",
	})
	if err != nil || rows != 0 {
		t.Fatalf("stale-name completion rows = %d, err = %v, want 0, nil", rows, err)
	}

	schedule.DesiredRevision = 4
	schedule.UpdatedAt = "2026-08-03T12:45:00Z"
	if _, err := queries.ScheduleDNSProjection(ctx, schedule); err != nil {
		t.Fatalf("schedule current projection: %v", err)
	}
	claimed, err = queries.ClaimDNSProjections(ctx, generated.ClaimDNSProjectionsParams{
		LeaseOwner:     "worker-3",
		LeaseExpiresAt: "2026-08-03T13:00:00Z",
		ClaimedAt:      "2026-08-03T12:46:00Z",
		BatchSize:      1,
	})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim failure projection = %#v, err = %v", claimed, err)
	}
	rows, err = queries.FailDNSProjection(ctx, generated.FailDNSProjectionParams{
		NextRetryAt:    "2026-08-03T13:05:00Z",
		ErrorCode:      "provider_unavailable",
		ErrorMessage:   "temporary provider failure",
		FailedAt:       "2026-08-03T12:47:00Z",
		ID:             projectionID,
		LeaseOwner:     "worker-3",
		FailedRevision: 4,
	})
	if err != nil || rows != 1 {
		t.Fatalf("fail projection rows = %d, err = %v, want 1, nil", rows, err)
	}
	failed, err := queries.GetDNSProjection(ctx, projectionID)
	if err != nil {
		t.Fatalf("get failed projection: %v", err)
	}
	if failed.State != "failed" || failed.AttemptCount != 1 || failed.LeaseOwner.Valid ||
		failed.LastErrorCode.String != "provider_unavailable" || failed.NextRetryAt.String != "2026-08-03T13:05:00Z" {
		t.Fatalf("failed projection = %#v", failed)
	}
}

func TestListAgentsGeneratedQueryCursor(t *testing.T) {
	db := openTestDatabase(t)
	ctx := context.Background()
	queries := generated.New(db)
	heartbeats := []sql.NullString{
		{},
		{},
		{String: "2026-08-03T12:35:00Z", Valid: true},
		{String: "2026-08-03T12:35:00Z", Valid: true},
		{String: "2026-08-03T12:36:00Z", Valid: true},
	}

	for i, heartbeat := range heartbeats {
		computeID := "0190d7c0-28f3-71a2-a0fd-d17dd083466" + string(rune('0'+i))
		agentID := "0190d7c0-28f3-71a2-a0fd-d17dd083467" + string(rune('0'+i))
		if _, err := db.Exec(`INSERT INTO computes (id, kind, display_name, ownership, created_at, updated_at) VALUES (?, 'vps', ?, 'rented', ?, ?)`, computeID, "agent host "+string(rune('0'+i)), testTime, testTime); err != nil {
			t.Fatalf("insert compute %d: %v", i, err)
		}
		if _, err := queries.CreateAgent(ctx, generated.CreateAgentParams{
			ID:              agentID,
			ComputeID:       computeID,
			CredentialHash:  "agentcredentialhashagentcredentialhash" + string(rune('0'+i)),
			ProtocolVersion: 1,
			ClientVersion:   "test",
			EnrolledAt:      testTime,
		}); err != nil {
			t.Fatalf("insert agent %d: %v", i, err)
		}
		if heartbeat.Valid {
			if _, err := db.Exec(`UPDATE agents SET last_heartbeat_at = ? WHERE id = ?`, heartbeat.String, agentID); err != nil {
				t.Fatalf("set heartbeat %d: %v", i, err)
			}
		}
	}

	first, err := queries.ListAgents(ctx, generated.ListAgentsParams{AfterID: "", PageSize: 2})
	if err != nil {
		t.Fatalf("list first agent page: %v", err)
	}
	if len(first) != 2 || first[0].LastHeartbeatAt.Valid || first[1].LastHeartbeatAt.Valid {
		t.Fatalf("first page = %#v, want two null-heartbeat agents", first)
	}
	second, err := queries.ListAgents(ctx, generated.ListAgentsParams{
		AfterHeartbeat: first[1].LastHeartbeatAt,
		AfterID:        first[1].ID,
		PageSize:       2,
	})
	if err != nil {
		t.Fatalf("list second agent page: %v", err)
	}
	if len(second) != 2 || second[0].LastHeartbeatAt.String != "2026-08-03T12:35:00Z" ||
		second[1].LastHeartbeatAt.String != "2026-08-03T12:35:00Z" || second[0].ID >= second[1].ID {
		t.Fatalf("second page = %#v, want equal heartbeats ordered by ID", second)
	}
	third, err := queries.ListAgents(ctx, generated.ListAgentsParams{
		AfterHeartbeat: second[1].LastHeartbeatAt,
		AfterID:        second[1].ID,
		PageSize:       2,
	})
	if err != nil {
		t.Fatalf("list third agent page: %v", err)
	}
	if len(third) != 1 || third[0].LastHeartbeatAt.String != "2026-08-03T12:36:00Z" {
		t.Fatalf("third page = %#v, want final later heartbeat", third)
	}
}
