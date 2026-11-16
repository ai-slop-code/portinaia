package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"portinaia/internal/database/generated"
)

const testTime = "2026-08-03T12:34:56Z"
const latestSchemaVersion = 10

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

func openSchemaOneTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "portinaia-schema-one.db"))
	if err != nil {
		t.Fatalf("open schema-one sqlite database: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable schema-one foreign keys: %v", err)
	}
	goose.SetBaseFS(Migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set schema-one goose dialect: %v", err)
	}
	if err := goose.UpToContext(context.Background(), db, "migrations", 1); err != nil {
		t.Fatalf("migrate schema-one database: %v", err)
	}
	return db
}

func TestCheckSchemaRequiresCurrentConsistentMigrationState(t *testing.T) {
	db := openTestDatabase(t)
	if err := CheckSchema(context.Background(), db); err != nil {
		t.Fatalf("current schema rejected: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 8"); err != nil {
		t.Fatal(err)
	}
	if err := CheckSchema(context.Background(), db); err == nil || !strings.Contains(err.Error(), "inconsistent schema versions") {
		t.Fatalf("inconsistent schema error = %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 11"); err != nil {
		t.Fatal(err)
	}
	if err := CheckSchema(context.Background(), db); err == nil || !strings.Contains(err.Error(), "inconsistent schema versions") {
		t.Fatalf("newer mismatched schema error = %v", err)
	}
}

func TestCheckSchemaRejectsMissingOldAndNewerSchemas(t *testing.T) {
	ctx := context.Background()
	missing, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "missing.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Close()
	if err := CheckSchema(ctx, missing); err == nil {
		t.Fatal("missing schema was accepted")
	}

	old := openSchemaOneTestDatabase(t)
	if err := CheckSchema(ctx, old); err == nil || !strings.Contains(err.Error(), "unsupported schema version") {
		t.Fatalf("old schema error = %v", err)
	}

	newer := openTestDatabase(t)
	if _, err := newer.Exec(`INSERT INTO goose_db_version(version_id,is_applied) VALUES(11,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := newer.Exec(`PRAGMA user_version=11`); err != nil {
		t.Fatal(err)
	}
	if err := CheckSchema(ctx, newer); err == nil || !strings.Contains(err.Error(), "unsupported schema version") {
		t.Fatalf("newer schema error = %v", err)
	}
}

func assertNoForeignKeyViolations(t *testing.T, db *sql.DB) {
	t.Helper()
	var violation string
	err := db.QueryRow("PRAGMA foreign_key_check").Scan(&violation)
	if err != sql.ErrNoRows {
		t.Fatalf("foreign key check returned a violation: value=%q err=%v", violation, err)
	}
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
	if tableCount != 50 {
		t.Fatalf("application table count = %d, want 50", tableCount)
	}

	version, err := goose.GetDBVersion(db)
	if err != nil {
		t.Fatalf("get schema version: %v", err)
	}
	if version != latestSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, latestSchemaVersion)
	}
	applicationVersion, err := SchemaVersion(context.Background(), db)
	if err != nil {
		t.Fatalf("get application schema version: %v", err)
	}
	if applicationVersion != latestSchemaVersion {
		t.Fatalf("application schema version = %d, want %d", applicationVersion, latestSchemaVersion)
	}
	var userVersion int
	if err := db.QueryRow("PRAGMA user_version").Scan(&userVersion); err != nil {
		t.Fatalf("get SQLite user version: %v", err)
	}
	if userVersion != latestSchemaVersion {
		t.Fatalf("SQLite user version = %d, want %d", userVersion, latestSchemaVersion)
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

func TestTagAndDraftForwardMigrationPreservesSchemaOneData(t *testing.T) {
	db := openSchemaOneTestDatabase(t)
	providerID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	tagID := "0190d7c0-28f3-71a2-a0fd-d17dd0834631"
	draftID := "0190d7c0-28f3-71a2-a0fd-d17dd0834632"
	recurringCostID := "0190d7c0-28f3-71a2-a0fd-d17dd0834644"
	oneTimeCostID := "0190d7c0-28f3-71a2-a0fd-d17dd0834645"
	locationID := "0190d7c0-28f3-71a2-a0fd-d17dd0834638"
	computeID := "0190d7c0-28f3-71a2-a0fd-d17dd0834639"
	containerID := "0190d7c0-28f3-71a2-a0fd-d17dd0834640"
	domainID := "0190d7c0-28f3-71a2-a0fd-d17dd0834641"
	rootContainerID := "0190d7c0-28f3-71a2-a0fd-d17dd0834642"
	agentID := "0190d7c0-28f3-71a2-a0fd-d17dd0834652"
	snapshotID := "0190d7c0-28f3-71a2-a0fd-d17dd0834653"
	historyID := "0190d7c0-28f3-71a2-a0fd-d17dd0834654"
	zoneID := "0190d7c0-28f3-71a2-a0fd-d17dd0834676"
	dnsNameID := "0190d7c0-28f3-71a2-a0fd-d17dd0834677"
	publicationSuccessID := "0190d7c0-28f3-71a2-a0fd-d17dd0834678"
	publicationFailureID := "0190d7c0-28f3-71a2-a0fd-d17dd0834679"
	projectionID := "0190d7c0-28f3-71a2-a0fd-d17dd0834684"
	legacyTagName := strings.Repeat("x", 64)
	if _, err := db.Exec(`
		INSERT INTO providers (id, name, revision, created_at, updated_at)
		VALUES (?, 'migration provider', 1, ?, ?)
	`, providerID, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one provider: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO tags (id, normalized_name, display_name, created_at)
		VALUES (?, ?, ?, ?)
	`, tagID, legacyTagName, legacyTagName, testTime); err != nil {
		t.Fatalf("insert schema-one tag: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO locations (id, type, name, provider_id, created_at, updated_at)
		VALUES (?, 'site', 'migration site', ?, ?, ?)
	`, locationID, providerID, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one location: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO computes (id, kind, display_name, ownership, created_at, updated_at)
		VALUES (?, 'physical', 'migration compute', 'owned', ?, ?)
	`, computeID, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one compute: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO compute_observations (
			compute_id, machine_id, hostname, architecture, logical_cpu_count,
			memory_bytes, observed_at, received_at
		) VALUES (?, 'machine-1', 'host-1', 'arm64', 4, 4096, ?, ?)
	`, computeID, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one compute observation: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO agents (id, compute_id, credential_hash, credential_generation, protocol_version, client_version, enrolled_at)
		VALUES (?, ?, ?, 1, 1, 'legacy-agent', ?)
	`, agentID, computeID, "legacyagenthashlegacyagenthash12", testTime); err != nil {
		t.Fatalf("insert schema-one agent: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO snapshot_contents (id, format_version, content_hash, compressed_document, uncompressed_size, created_at)
		VALUES (?, 1, ?, x'01', 1, ?)
	`, snapshotID, strings.Repeat("c", 64), testTime); err != nil {
		t.Fatalf("insert schema-one snapshot content: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO discovery_reports (
			id, report_id, agent_id, protocol_version, received_at, client_collected_at,
			snapshot_content_id, result, collection_error_summary
		) VALUES (?, ?, ?, 1, ?, ?, ?, 'accepted', 'legacy diagnostic')
	`, historyID, "0190d7c0-28f3-71a2-a0fd-d17dd0834655", agentID, testTime, testTime, snapshotID); err != nil {
		t.Fatalf("insert schema-one discovery report: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO discovery_collector_results (report_id, collector_name, success, sanitized_error)
		VALUES (?, 'host', 1, NULL), (?, 'podman:1000', 0, 'legacy podman failure')
	`, historyID, historyID); err != nil {
		t.Fatalf("insert schema-one collector results: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO containers (
			id, compute_id, podman_owner, podman_connection, name, runtime_id,
			state, health, restart_policy, restart_max_attempts, first_observed_at, last_observed_at,
			created_at, updated_at
		) VALUES (?, ?, '1000', 'rootless', 'migration-container', 'runtime-1',
			'running', 'none', 'on-failure', 3, ?, ?, ?, ?)
	`, containerID, computeID, testTime, testTime, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one container: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO containers (
			id, compute_id, podman_owner, podman_connection, name, runtime_id,
			state, health, restart_policy, first_observed_at, last_observed_at,
			created_at, updated_at
		) VALUES (?, ?, 'root', 'rootful', 'root-container', 'runtime-root',
			'running', 'none', 'no', ?, ?, ?, ?)
	`, rootContainerID, computeID, testTime, testTime, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one root container: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO container_networks (container_id, name, network_id, driver)
		VALUES (?, 'bridge', 'network-1', 'bridge')
	`, containerID); err != nil {
		t.Fatalf("insert schema-one container network: %v", err)
	}
	var networkID int64
	if err := db.QueryRow(`SELECT id FROM container_networks WHERE container_id = ? AND name = 'bridge'`, containerID).Scan(&networkID); err != nil {
		t.Fatalf("read schema-one container network: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO container_ports (container_id, protocol, container_port, is_published)
		VALUES (?, 'tcp', 80, 0)
	`, containerID); err != nil {
		t.Fatalf("insert schema-one container port: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO container_addresses (container_id, network_id, address, family, prefix_length)
		VALUES (?, ?, '10.0.0.2', 4, 24)
	`, containerID, networkID); err != nil {
		t.Fatalf("insert schema-one container address: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO container_mounts (container_id, type, source, destination, read_only)
		VALUES (?, 'bind', '/srv/data', '/data', 0)
	`, containerID); err != nil {
		t.Fatalf("insert schema-one container mount: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO container_labels (container_id, key, value) VALUES (?, 'app', 'demo')`, containerID); err != nil {
		t.Fatalf("insert schema-one container label: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO ip_assignments (
			id, address, family, container_id, scope, role, source, is_primary, created_at, updated_at
		) VALUES ('0190d7c0-28f3-71a2-a0fd-d17dd0834643', '10.0.0.2', 4, ?, 'private', 'service', 'manual', 0, ?, ?)
	`, containerID, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one container IP assignment: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO domains (id, normalized_name, created_at, updated_at)
		VALUES (?, 'migration.example', ?, ?)
	`, domainID, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one domain: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dns_zones (
			id, cloudflare_zone_id, normalized_name, cloudflare_status, selection_state,
			domain_id, discovered_at, refreshed_at, created_at, updated_at
		) VALUES (?, 'cf-migration-zone', 'migration.example', 'active', 'managed', ?, ?, ?, ?, ?)
	`, zoneID, domainID, testTime, testTime, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one DNS zone: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dns_names (id, zone_id, fqdn, is_wildcard, lifecycle_state, created_at, updated_at)
		VALUES (?, ?, 'host.migration.example', 0, 'active', ?, ?)
	`, dnsNameID, zoneID, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one DNS name: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dns_record_sets (id, dns_name_id, view, type, ttl, proxied, sort_order)
		VALUES ('0190d7c0-28f3-71a2-a0fd-d17dd0834680', ?, 'internal', 'A', 60, 0, 0)
	`, dnsNameID); err != nil {
		t.Fatalf("insert schema-one internal record set: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dns_record_values (id, record_set_id, normalized_value, sort_order, value_checksum)
		VALUES ('0190d7c0-28f3-71a2-a0fd-d17dd0834681', '0190d7c0-28f3-71a2-a0fd-d17dd0834680', '10.0.0.1', 0, ?)
	`, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("insert schema-one internal record value: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dns_publications (id, zone_id, desired_revision, serial, checksum, file_name, created_at, result)
		VALUES (?, ?, 7, 100, ?, 'db.migration.example', ?, 'success')
	`, publicationSuccessID, zoneID, strings.Repeat("d", 64), testTime); err != nil {
		t.Fatalf("insert schema-one successful DNS publication: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dns_publications (id, zone_id, desired_revision, serial, checksum, file_name, created_at, result, sanitized_error)
		VALUES (?, ?, 8, 101, ?, 'db.migration.example', '2026-08-03T12:35:00Z', 'failure', 'render failed')
	`, publicationFailureID, zoneID, strings.Repeat("e", 64)); err != nil {
		t.Fatalf("insert schema-one failed DNS publication: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dns_projections (
			id, dns_name_id, provider, desired_revision, state, attempt_count,
			last_error_code, last_error_message, updated_at
		) VALUES (?, ?, 'cloudflare', 1, 'failed', 1, 'provider_conflict', 'permanent conflict', ?)
	`, projectionID, dnsNameID, testTime); err != nil {
		t.Fatalf("insert schema-one failed projection without retry time: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO costs (
			id, compute_id, type, amount_minor, currency_code, currency_exponent,
			billing_interval, effective_start_date, created_at, updated_at
		) VALUES (?, ?, 'recurring', 1250, 'EUR', 2, 'monthly', '2026-01-01', ?, ?)
	`, recurringCostID, computeID, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one recurring cost: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO costs (
			id, domain_id, type, amount_minor, currency_code, currency_exponent,
			effective_start_date, effective_end_date, charge_date, created_at, updated_at
		) VALUES (?, ?, 'one_time', 3500, 'JPY', 0, '2026-01-01', '2026-01-31', '2026-02-01', ?, ?)
	`, oneTimeCostID, domainID, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one one-time cost: %v", err)
	}
	for _, attachment := range []struct {
		table   string
		query   string
		ownerID string
	}{
		{"provider_tags", `INSERT INTO provider_tags (provider_id, tag_id) VALUES (?, ?)`, providerID},
		{"location_tags", `INSERT INTO location_tags (location_id, tag_id) VALUES (?, ?)`, locationID},
		{"compute_tags", `INSERT INTO compute_tags (compute_id, tag_id) VALUES (?, ?)`, computeID},
		{"container_tags", `INSERT INTO container_tags (container_id, tag_id) VALUES (?, ?)`, containerID},
		{"domain_tags", `INSERT INTO domain_tags (domain_id, tag_id) VALUES (?, ?)`, domainID},
	} {
		if _, err := db.Exec(attachment.query, attachment.ownerID, tagID); err != nil {
			t.Fatalf("attach schema-one tag through %s: %v", attachment.table, err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO dns_drafts (
			id, operation, proposed_aggregate, content_hash, validation_state,
			actor_type, actor_id, created_at, updated_at
		) VALUES (?, 'create', '{}', ?, 'pending', 'administrator', ?, ?, ?)
	`, draftID, strings.Repeat("a", 64), providerID, testTime, testTime); err != nil {
		t.Fatalf("insert schema-one draft: %v", err)
	}

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate populated schema-one database: %v", err)
	}
	var desiredGeneration, fencingToken int64
	var publishedGeneration, publishedSerial sql.NullInt64
	var publishedChecksum, publishedFileName, lastPublicationError sql.NullString
	var publicationState string
	var nextSerial, publicationAttempts int64
	if err := db.QueryRow(`
		SELECT desired_generation, published_generation, publication_state, next_serial,
			published_serial, published_checksum, published_file_name, attempt_count,
			last_error_message, fencing_token
		FROM dns_zone_publication_state WHERE zone_id = ?
	`, zoneID).Scan(&desiredGeneration, &publishedGeneration, &publicationState, &nextSerial,
		&publishedSerial, &publishedChecksum, &publishedFileName, &publicationAttempts,
		&lastPublicationError, &fencingToken); err != nil {
		t.Fatalf("read migrated zone publication state: %v", err)
	}
	if desiredGeneration != 1 || publishedGeneration.Valid || publicationState != "pending" || nextSerial != 102 ||
		!publishedSerial.Valid || publishedSerial.Int64 != 100 || !publishedChecksum.Valid ||
		publishedChecksum.String != strings.Repeat("d", 64) || !publishedFileName.Valid ||
		publishedFileName.String != "db.migration.example" || publicationAttempts != 2 ||
		!lastPublicationError.Valid || lastPublicationError.String != "render failed" || fencingToken != 0 {
		t.Fatalf("legacy zone publication state has incorrect generation/serial/lease mapping")
	}
	var legacyRevision sql.NullInt64
	var legacyGeneration sql.NullInt64
	if err := db.QueryRow(`SELECT legacy_desired_revision, zone_generation FROM dns_publications WHERE id = ?`, publicationSuccessID).
		Scan(&legacyRevision, &legacyGeneration); err != nil {
		t.Fatalf("read migrated publication history: %v", err)
	}
	if !legacyRevision.Valid || legacyRevision.Int64 != 7 || legacyGeneration.Valid {
		t.Fatalf("publication name revision was incorrectly converted to a zone generation: legacy=%v zone=%v", legacyRevision, legacyGeneration)
	}
	var retryable int64
	if err := db.QueryRow(`SELECT retryable FROM dns_projections WHERE id = ?`, projectionID).Scan(&retryable); err != nil {
		t.Fatalf("read migrated projection retryability: %v", err)
	}
	if retryable != 0 {
		t.Fatalf("legacy failed projection without retry time is retryable=%d, want permanent failure", retryable)
	}
	if _, err := db.Exec(`
		UPDATE dns_zone_publication_state SET desired_generation = 2, publication_state = 'pending',
			next_serial = 102, attempt_count = 3, fencing_token = 1, lease_owner = 'publisher-1',
			lease_expires_at = '2026-08-03T12:40:00Z', leased_generation = 2,
			lease_fencing_token = 1
		WHERE zone_id = ?
	`, zoneID); err != nil {
		t.Fatalf("claim zone publication generation: %v", err)
	}
	failedPublicationID := "0190d7c0-28f3-71a2-a0fd-d17dd0834682"
	if _, err := db.Exec(`
		INSERT INTO dns_publications (
			id, zone_id, zone_generation, fencing_token, serial, checksum, file_name,
			created_at, result, sanitized_error
		) VALUES (?, ?, 2, 1, NULL, NULL, NULL, '2026-08-03T12:36:00Z', 'failure', 'render failed before output')
	`, failedPublicationID, zoneID); err != nil {
		t.Fatalf("record render failure without output metadata: %v", err)
	}
	expectConstraintFailure(t, db, `
		INSERT INTO dns_publications (
			id, zone_id, zone_generation, fencing_token, created_at, result
		) VALUES ('0190d7c0-28f3-71a2-a0fd-d17dd0834683', ?, 2, 1, '2026-08-03T12:37:00Z', 'failure')
	`, zoneID)
	expectConstraintFailure(t, db, `
		UPDATE dns_zone_publication_state SET lease_owner = NULL WHERE zone_id = ?
	`, zoneID)
	var ownerUID int64
	var restartPolicy string
	var retryAttempts sql.NullInt64
	if err := db.QueryRow(`SELECT podman_owner_uid, restart_policy, restart_max_attempts FROM containers WHERE id = ?`, containerID).
		Scan(&ownerUID, &restartPolicy, &retryAttempts); err != nil {
		t.Fatalf("read migrated container owner/restart fields: %v", err)
	}
	if ownerUID != 1000 || restartPolicy != "on-failure" || !retryAttempts.Valid || retryAttempts.Int64 != 3 {
		t.Fatalf("migrated container owner/restart = %d/%s/%v", ownerUID, restartPolicy, retryAttempts)
	}
	if err := db.QueryRow(`SELECT podman_owner_uid FROM containers WHERE id = ?`, rootContainerID).Scan(&ownerUID); err != nil || ownerUID != 0 {
		t.Fatalf("root owner UID mapping = %d err=%v, want 0", ownerUID, err)
	}
	for _, childTable := range []string{"container_ports", "container_networks", "container_addresses", "container_mounts", "container_labels", "container_tags", "ip_assignments"} {
		var count int
		var err error
		if childTable == "ip_assignments" {
			err = db.QueryRow(`SELECT count(*) FROM ip_assignments WHERE container_id = ?`, containerID).Scan(&count)
		} else {
			err = db.QueryRow("SELECT count(*) FROM "+childTable+" WHERE container_id = ?", containerID).Scan(&count)
		}
		if err != nil || count != 1 {
			t.Errorf("container child %s count = %d err=%v, want 1", childTable, count, err)
		}
	}
	var revision int64
	var createdAt, updatedAt string
	var archivedAt sql.NullString
	if err := db.QueryRow(`SELECT revision, created_at, updated_at, archived_at FROM tags WHERE id = ?`, tagID).
		Scan(&revision, &createdAt, &updatedAt, &archivedAt); err != nil {
		t.Fatalf("read migrated tag: %v", err)
	}
	if revision != 1 || createdAt != testTime || updatedAt != testTime || archivedAt.Valid {
		t.Fatalf("migrated tag metadata = revision %d created %q updated %q archived %v", revision, createdAt, updatedAt, archivedAt)
	}
	if err := db.QueryRow(`SELECT revision FROM dns_drafts WHERE id = ?`, draftID).Scan(&revision); err != nil {
		t.Fatalf("read migrated draft revision: %v", err)
	}
	if revision != 1 {
		t.Fatalf("migrated draft revision = %d, want 1", revision)
	}
	var envelopeVersion int64
	var migratedClientVersion, startedAt, finishedAt, fingerprint sql.NullString
	var contentReused sql.NullInt64
	var legacyCollectedAt, retainedContentID, legacyResult, legacyError sql.NullString
	if err := db.QueryRow(`
		SELECT envelope_version, client_version, collection_started_at, collection_finished_at,
			request_fingerprint, content_reused, client_collected_at, snapshot_content_id,
			result, collection_error_summary
		FROM discovery_reports WHERE id = ?
	`, historyID).Scan(&envelopeVersion, &migratedClientVersion, &startedAt, &finishedAt,
		&fingerprint, &contentReused, &legacyCollectedAt, &retainedContentID, &legacyResult, &legacyError); err != nil {
		t.Fatalf("read migrated legacy report envelope: %v", err)
	}
	if envelopeVersion != 0 || migratedClientVersion.Valid || startedAt.Valid || finishedAt.Valid || fingerprint.Valid || contentReused.Valid {
		t.Fatalf("legacy report was given fabricated envelope data: version=%d client=%v start=%v end=%v fingerprint=%v reused=%v",
			envelopeVersion, migratedClientVersion, startedAt, finishedAt, fingerprint, contentReused)
	}
	if !legacyCollectedAt.Valid || legacyCollectedAt.String != testTime || !retainedContentID.Valid || retainedContentID.String != snapshotID ||
		!legacyResult.Valid || legacyResult.String != "accepted" || !legacyError.Valid || legacyError.String != "legacy diagnostic" {
		t.Fatalf("legacy report metadata was not preserved: collected=%v content=%v result=%v error=%v", legacyCollectedAt, retainedContentID, legacyResult, legacyError)
	}
	var collectorResults int
	if err := db.QueryRow(`SELECT count(*) FROM discovery_collector_results WHERE report_id = ?`, historyID).Scan(&collectorResults); err != nil || collectorResults != 2 {
		t.Fatalf("legacy collector result count = %d err=%v, want 2", collectorResults, err)
	}
	expectConstraintFailure(t, db, `UPDATE discovery_reports SET collection_error_summary = 'mutated' WHERE id = ?`, historyID)
	var interval string
	var effectiveStart, chargeDate sql.NullString
	if err := db.QueryRow(`
		SELECT billing_interval, effective_start_date, charge_date FROM costs WHERE id = ?
	`, recurringCostID).Scan(&interval, &effectiveStart, &chargeDate); err != nil {
		t.Fatalf("read migrated recurring cost: %v", err)
	}
	if interval != "monthly" || !effectiveStart.Valid || effectiveStart.String != "2026-01-01" || chargeDate.Valid {
		t.Fatalf("recurring cost mapping = interval %q start %v charge %v", interval, effectiveStart, chargeDate)
	}
	var effectiveEnd sql.NullString
	if err := db.QueryRow(`
		SELECT effective_start_date, effective_end_date, charge_date FROM costs WHERE id = ?
	`, oneTimeCostID).Scan(&effectiveStart, &effectiveEnd, &chargeDate); err != nil {
		t.Fatalf("read migrated one-time cost: %v", err)
	}
	if effectiveStart.Valid || effectiveEnd.Valid || !chargeDate.Valid || chargeDate.String != "2026-02-01" {
		t.Fatalf("one-time date mapping = start %v end %v charge %v", effectiveStart, effectiveEnd, chargeDate)
	}
	var virtualizationRole sql.NullString
	if err := db.QueryRow(`SELECT virtualization_role FROM compute_observations WHERE compute_id = ?`, computeID).Scan(&virtualizationRole); err != nil {
		t.Fatalf("read migrated virtualization role: %v", err)
	}
	if virtualizationRole.Valid {
		t.Fatalf("legacy virtualization role fabricated as %q", virtualizationRole.String)
	}
	for _, table := range []string{"provider_tags", "location_tags", "compute_tags", "container_tags", "domain_tags"} {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM "+table+" WHERE tag_id = ?", tagID).Scan(&count); err != nil {
			t.Fatalf("read preserved %s attachments: %v", table, err)
		}
		if count != 1 {
			t.Errorf("preserved %s attachment count = %d, want 1", table, count)
		}
	}
	assertNoForeignKeyViolations(t, db)
	var migratedProviderID, migratedLocationProviderID string
	if err := db.QueryRow(`SELECT id FROM providers WHERE name='migration provider'`).Scan(&migratedProviderID); err != nil {
		t.Fatalf("read migrated legacy provider UUID: %v", err)
	}
	if err := db.QueryRow(`SELECT provider_id FROM locations WHERE id=?`, locationID).Scan(&migratedLocationProviderID); err != nil {
		t.Fatalf("read migrated legacy provider relationship: %v", err)
	}
	if migratedProviderID != providerID || migratedLocationProviderID != providerID {
		t.Fatalf("migration changed legacy UUIDv4 identity/reference: provider=%s location.provider=%s", migratedProviderID, migratedLocationProviderID)
	}

	observedAt := testTime
	if _, err := db.Exec(`
		INSERT INTO compute_interfaces (
			compute_id, interface_name, mac_address, mtu, is_up, first_observed_at,
			last_observed_at, presence_state
		) VALUES (?, 'eth0', '02:00:00:00:00:01', 1500, 1, ?, ?, 'present')
	`, computeID, observedAt, observedAt); err != nil {
		t.Fatalf("insert current interface: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO interface_addresses (
			compute_id, interface_name, address, prefix_length, scope,
			first_observed_at, last_observed_at, presence_state
		) VALUES (?, 'eth0', '10.0.0.2', NULL, 'private', ?, ?, 'present')
	`, computeID, observedAt, observedAt); err != nil {
		t.Fatalf("insert interface address with unknown prefix: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO compute_disks (
			compute_id, device_name, model, size_bytes, first_observed_at,
			last_observed_at, presence_state
		) VALUES (?, 'nvme0n1', 'SSD', 1000000, ?, ?, 'present')
	`, computeID, observedAt, observedAt); err != nil {
		t.Fatalf("insert current disk: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO compute_filesystems (
			compute_id, device_name, mount_point, filesystem_type, total_bytes,
			available_bytes, first_observed_at, last_observed_at, presence_state
		) VALUES (?, '/dev/nvme0n1p1', '/', 'ext4', 900000, 450000, ?, ?, 'present')
	`, computeID, observedAt, observedAt); err != nil {
		t.Fatalf("insert current filesystem: %v", err)
	}
	if _, err := db.Exec(`UPDATE compute_observations SET virtualization_role = 'guest' WHERE compute_id = ?`, computeID); err != nil {
		t.Fatalf("update virtualization role: %v", err)
	}
	if err := db.QueryRow(`SELECT virtualization_role FROM compute_observations WHERE compute_id = ?`, computeID).Scan(&virtualizationRole); err != nil || !virtualizationRole.Valid || virtualizationRole.String != "guest" {
		t.Fatalf("virtualization role round trip = %v err=%v", virtualizationRole, err)
	}
	var prefixLength sql.NullInt64
	if err := db.QueryRow(`SELECT prefix_length FROM interface_addresses WHERE compute_id = ?`, computeID).Scan(&prefixLength); err != nil {
		t.Fatalf("unknown interface prefix did not round-trip as NULL: %v", err)
	}
	if prefixLength.Valid {
		t.Fatalf("unknown interface prefix round-tripped as %d", prefixLength.Int64)
	}
	var availableBytes int64
	if err := db.QueryRow(`SELECT available_bytes FROM compute_filesystems WHERE compute_id = ?`, computeID).Scan(&availableBytes); err != nil || availableBytes != 450000 {
		t.Fatalf("filesystem available bytes = %d err=%v", availableBytes, err)
	}
	if _, err := db.Exec(`UPDATE containers SET restart_policy = 'unknown', restart_max_attempts = NULL WHERE id = ?`, containerID); err != nil {
		t.Fatalf("preserve unknown restart policy: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO container_mounts (container_id, type, source, destination, read_only)
		VALUES (?, 'other', '', '/opaque', 0)
	`, containerID); err != nil {
		t.Fatalf("preserve unknown mount type: %v", err)
	}
	if _, err := db.Exec(`UPDATE container_addresses SET prefix_length = NULL WHERE container_id = ?`, containerID); err != nil {
		t.Fatalf("store unknown container address prefix: %v", err)
	}
	expectConstraintFailure(t, db, `UPDATE containers SET podman_owner_uid = -1 WHERE id = ?`, containerID)
	expectConstraintFailure(t, db, `
		INSERT INTO containers (
			id, compute_id, podman_owner_uid, podman_connection, name, runtime_id,
			state, health, restart_policy, first_observed_at, last_observed_at,
			created_at, updated_at
		) VALUES ('0190d7c0-28f3-71a2-a0fd-d17dd0834646', ?, 1000, 'other-connection', 'migration-container', 'runtime-duplicate',
			'running', 'none', 'no', ?, ?, ?, ?)
	`, computeID, testTime, testTime, testTime, testTime)
	expectConstraintFailure(t, db, `
		INSERT INTO compute_interfaces (compute_id, interface_name, first_observed_at, last_observed_at)
		VALUES (?, 'eth1', ?, ?)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834642", observedAt, observedAt)
	expectConstraintFailure(t, db, `
		INSERT INTO compute_interfaces (
			compute_id, interface_name, first_observed_at, last_observed_at,
			presence_state
		) VALUES (?, 'eth1', ?, ?, 'absent')
	`, computeID, observedAt, observedAt)
	expectConstraintFailure(t, db, `
		INSERT INTO interface_addresses (
			compute_id, interface_name, address, first_observed_at,
			last_observed_at, presence_state
		) VALUES (?, 'missing', '10.0.0.3', ?, ?, 'present')
	`, computeID, observedAt, observedAt)
	expectConstraintFailure(t, db, `UPDATE compute_observations SET virtualization_role = 'hypervisor' WHERE compute_id = ?`, computeID)
	assertNoForeignKeyViolations(t, db)

	longTagID := "0190d7c0-28f3-71a2-a0fd-d17dd0834633"
	longName := strings.Repeat("y", 100)
	if _, err := db.Exec(`
		INSERT INTO tags (id, normalized_name, display_name, revision, created_at, updated_at)
		VALUES (?, ?, ?, 1, ?, ?)
	`, longTagID, longName, longName, testTime, testTime); err != nil {
		t.Fatalf("insert API-limit tag: %v", err)
	}
	if _, err := db.Exec(`
		UPDATE tags SET revision = 2, updated_at = '2026-08-03T12:35:00Z', archived_at = '2026-08-03T12:35:00Z'
		WHERE id = ?
	`, longTagID); err != nil {
		t.Fatalf("update tag revision/archive metadata: %v", err)
	}
	expectConstraintFailure(t, db, `
		INSERT INTO tags (id, normalized_name, display_name, revision, created_at, updated_at)
		VALUES (?, ?, 'too long', 1, ?, ?)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834634", strings.Repeat("z", 101), testTime, testTime)
	expectConstraintFailure(t, db, `
		INSERT INTO tags (id, normalized_name, display_name, revision, created_at, updated_at)
		VALUES (?, 'bad_revision', 'bad revision', 0, ?, ?)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834635", testTime, testTime)
	expectConstraintFailure(t, db, `
		INSERT INTO tags (id, normalized_name, display_name, revision, created_at, updated_at)
		VALUES (?, 'bad_time', 'bad time', 1, ?, '2026-08-03T12:34:56.123Z')
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834636", testTime)
	expectConstraintFailure(t, db, `
		INSERT INTO dns_drafts (
			id, operation, proposed_aggregate, content_hash, validation_state,
			actor_type, actor_id, created_at, updated_at, revision
		) VALUES (?, 'create', '{}', ?, 'pending', 'administrator', ?, ?, ?, 0)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834637", strings.Repeat("b", 64), providerID, testTime, testTime)
	if err := goose.DownToContext(context.Background(), db, "migrations", 1); err == nil {
		t.Fatal("downgrade unexpectedly discarded tag or current observation metadata")
	}
	version, err := goose.GetDBVersion(db)
	if err != nil {
		t.Fatalf("read version after rejected downgrade: %v", err)
	}
	if version != latestSchemaVersion-1 {
		t.Fatalf("schema version after rejected downgrade = %d, want %d", version, latestSchemaVersion-1)
	}
	var archivedTag string
	if err := db.QueryRow(`SELECT id FROM tags WHERE id = ? AND revision = 2 AND archived_at IS NOT NULL`, longTagID).Scan(&archivedTag); err != nil {
		t.Fatalf("rejected downgrade did not preserve tag lifecycle metadata: %v", err)
	}
}

func TestCostCurrencyAndDateTypeConstraints(t *testing.T) {
	db := openTestDatabase(t)
	computeID := "0190d7c0-28f3-71a2-a0fd-d17dd0834648"
	domainID := "0190d7c0-28f3-71a2-a0fd-d17dd0834649"
	if _, err := db.Exec(`
		INSERT INTO computes (id, kind, display_name, ownership, created_at, updated_at)
		VALUES (?, 'physical', 'cost host', 'owned', ?, ?)
	`, computeID, testTime, testTime); err != nil {
		t.Fatalf("insert compute: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO domains (id, normalized_name, created_at, updated_at)
		VALUES (?, 'cost.example', ?, ?)
	`, domainID, testTime, testTime); err != nil {
		t.Fatalf("insert domain: %v", err)
	}

	currencyExponents := map[string]int64{"EUR": 2, "JPY": 0, "BHD": 3, "CLF": 4, "UYW": 4}
	for code, expectedExponent := range currencyExponents {
		var actual int64
		if err := db.QueryRow(`SELECT minor_unit_exponent FROM currency_units WHERE currency_code = ?`, code).Scan(&actual); err != nil {
			t.Fatalf("read supported currency %s: %v", code, err)
		}
		if actual != expectedExponent {
			t.Errorf("currency %s exponent = %d, want %d", code, actual, expectedExponent)
		}
	}

	intervalMappings := []struct {
		api string
		sql string
	}{
		{"P1M", "monthly"},
		{"P3M", "quarterly"},
		{"P6M", "semiannual"},
		{"P1Y", "annual"},
	}
	for index, mapping := range intervalMappings {
		id := fmt.Sprintf("0190d7c0-28f3-71a2-a0fd-d17dd08346%02d", 50+index)
		if _, err := db.Exec(`
			INSERT INTO costs (
				id, compute_id, type, amount_minor, currency_code, currency_exponent,
				billing_interval, effective_start_date, created_at, updated_at
			) VALUES (?, ?, 'recurring', 100, 'EUR', 2, ?, '2026-01-01', ?, ?)
		`, id, computeID, mapping.sql, testTime, testTime); err != nil {
			t.Fatalf("insert SQL interval %s for API interval %s: %v", mapping.sql, mapping.api, err)
		}
		var stored string
		if err := db.QueryRow(`SELECT billing_interval FROM costs WHERE id = ?`, id).Scan(&stored); err != nil || stored != mapping.sql {
			t.Fatalf("API interval %s mapped to SQL %q, want %q (err=%v)", mapping.api, stored, mapping.sql, err)
		}
	}

	oneTimeID := "0190d7c0-28f3-71a2-a0fd-d17dd0834660"
	if _, err := db.Exec(`
		INSERT INTO costs (
			id, domain_id, type, amount_minor, currency_code, currency_exponent,
			charge_date, created_at, updated_at
		) VALUES (?, ?, 'one_time', 2500, 'JPY', 0, '2026-08-01', ?, ?)
	`, oneTimeID, domainID, testTime, testTime); err != nil {
		t.Fatalf("insert one-time cost without recurring dates: %v", err)
	}
	expectConstraintFailure(t, db, `
		INSERT INTO costs (
			id, compute_id, type, amount_minor, currency_code, currency_exponent,
			billing_interval, effective_start_date, created_at, updated_at
		) VALUES ('0190d7c0-28f3-71a2-a0fd-d17dd0834661', ?, 'recurring', 100, 'EUR', 3, 'monthly', '2026-01-01', ?, ?)
	`, computeID, testTime, testTime)
	expectConstraintFailure(t, db, `
		INSERT INTO costs (
			id, compute_id, type, amount_minor, currency_code, currency_exponent,
			billing_interval, effective_start_date, created_at, updated_at
		) VALUES ('0190d7c0-28f3-71a2-a0fd-d17dd0834662', ?, 'recurring', 100, 'ZZZ', 2, 'monthly', '2026-01-01', ?, ?)
	`, computeID, testTime, testTime)
	expectConstraintFailure(t, db, `
		INSERT INTO costs (
			id, compute_id, type, amount_minor, currency_code, currency_exponent,
			billing_interval, effective_start_date, created_at, updated_at
		) VALUES ('0190d7c0-28f3-71a2-a0fd-d17dd0834663', ?, 'recurring', 100, 'CLF', 5, 'annual', '2026-01-01', ?, ?)
	`, computeID, testTime, testTime)
	expectConstraintFailure(t, db, `
		INSERT INTO costs (
			id, compute_id, type, amount_minor, currency_code, currency_exponent,
			billing_interval, effective_start_date, created_at, updated_at
		) VALUES ('0190d7c0-28f3-71a2-a0fd-d17dd0834664', ?, 'recurring', 100, 'EUR', 2, 'P1M', '2026-01-01', ?, ?)
	`, computeID, testTime, testTime)
	expectConstraintFailure(t, db, `
		INSERT INTO costs (
			id, domain_id, type, amount_minor, currency_code, currency_exponent,
			charge_date, effective_start_date, created_at, updated_at
		) VALUES ('0190d7c0-28f3-71a2-a0fd-d17dd0834665', ?, 'one_time', 100, 'EUR', 2, '2026-01-01', '2026-01-01', ?, ?)
	`, domainID, testTime, testTime)
}

func TestPodmanOwnerMigrationRejectsUnmappableLegacyIdentity(t *testing.T) {
	db := openSchemaOneTestDatabase(t)
	computeID := "0190d7c0-28f3-71a2-a0fd-d17dd0834650"
	containerID := "0190d7c0-28f3-71a2-a0fd-d17dd0834651"
	if _, err := db.Exec(`
		INSERT INTO computes (id, kind, display_name, ownership, created_at, updated_at)
		VALUES (?, 'physical', 'legacy owner host', 'owned', ?, ?)
	`, computeID, testTime, testTime); err != nil {
		t.Fatalf("insert compute fixture: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO containers (
			id, compute_id, podman_owner, podman_connection, name, runtime_id,
			state, health, restart_policy, first_observed_at, last_observed_at,
			created_at, updated_at
		) VALUES (?, ?, 'alice', 'rootless', 'legacy-container', 'runtime-alice',
			'running', 'none', 'no', ?, ?, ?, ?)
	`, containerID, computeID, testTime, testTime, testTime, testTime); err != nil {
		t.Fatalf("insert unmappable schema-one owner fixture: %v", err)
	}
	if err := Migrate(context.Background(), db); err == nil {
		t.Fatal("migration silently converted a non-numeric legacy owner to a UID")
	}
	version, err := goose.GetDBVersion(db)
	if err != nil {
		t.Fatalf("read schema version after rejected owner mapping: %v", err)
	}
	if version != 4 {
		t.Fatalf("schema version after rejected owner mapping = %d, want 4", version)
	}
	var owner string
	if err := db.QueryRow(`SELECT podman_owner FROM containers WHERE id = ?`, containerID).Scan(&owner); err != nil || owner != "alice" {
		t.Fatalf("unmappable owner was not preserved for operator resolution: owner=%q err=%v", owner, err)
	}
}

func TestComputeCollectorAuthorityAndHistoryRetention(t *testing.T) {
	db := openTestDatabase(t)
	computeID := "0190d7c0-28f3-71a2-a0fd-d17dd0834670"
	agentID := "0190d7c0-28f3-71a2-a0fd-d17dd0834671"
	snapshotID := "0190d7c0-28f3-71a2-a0fd-d17dd0834672"
	hash := strings.Repeat("c", 64)
	if _, err := db.Exec(`
		INSERT INTO computes (id, kind, display_name, ownership, created_at, updated_at)
		VALUES (?, 'vps', 'collector host', 'rented', ?, ?)
	`, computeID, testTime, testTime); err != nil {
		t.Fatalf("insert compute: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO agents (id, compute_id, credential_hash, credential_generation, protocol_version, client_version, enrolled_at)
		VALUES (?, ?, ?, 1, 1, 'test', ?)
	`, agentID, computeID, "collectoragenthashcollectoragenthash", testTime); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO snapshot_contents (id, format_version, content_hash, compressed_document, uncompressed_size, created_at)
		VALUES (?, 1, ?, x'01', 1, ?)
	`, snapshotID, hash, testTime); err != nil {
		t.Fatalf("insert snapshot content: %v", err)
	}
	reportIDs := []string{
		"0190d7c0-28f3-71a2-a0fd-d17dd0834673",
		"0190d7c0-28f3-71a2-a0fd-d17dd0834674",
		"0190d7c0-28f3-71a2-a0fd-d17dd0834675",
	}
	receivedAt := []string{"2026-08-03T12:34:00Z", "2026-08-03T12:35:00Z", "2026-08-03T12:36:00Z"}
	for i, reportID := range reportIDs {
		if _, err := db.Exec(`
			INSERT INTO discovery_reports (
				id, report_id, agent_id, protocol_version, client_version, received_at,
				client_collected_at, collection_started_at, collection_finished_at,
				snapshot_content_id, result, content_reused, request_fingerprint, envelope_version
			) VALUES (?, ?, ?, 1, '1.0.0', ?, ?, ?, ?, ?, 'accepted', 0, ?, 1)
		`, reportID, fmt.Sprintf("0190d7c0-28f3-71a2-a0fd-d17dd083468%d", i), agentID,
			receivedAt[i], receivedAt[i], receivedAt[i], receivedAt[i], snapshotID,
			strings.Repeat([]string{"1", "2", "3"}[i], 64)); err != nil {
			t.Fatalf("insert discovery report %d: %v", i, err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO compute_disks (
			compute_id, device_name, size_bytes, first_observed_at, last_observed_at,
			last_report_id, presence_state
		) VALUES (?, 'sda', 1000, ?, ?, ?, 'present')
	`, computeID, receivedAt[0], receivedAt[0], reportIDs[0]); err != nil {
		t.Fatalf("insert present disk observation: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO compute_collector_observations (
			compute_id, collector_key, collector_name, last_status, last_received_at,
			last_report_id, last_successful_at, last_successful_report_id
		) VALUES (?, 'disks', 'disks', 'success', ?, ?, ?, ?)
	`, computeID, receivedAt[0], reportIDs[0], receivedAt[0], reportIDs[0]); err != nil {
		t.Fatalf("insert initial successful collector state: %v", err)
	}
	if _, err := db.Exec(`
		UPDATE compute_collector_observations SET
			last_status = 'failed', last_received_at = ?, last_report_id = ?,
			last_failed_at = ?, last_failed_report_id = ?,
			last_error_code = 'collector_unavailable', last_error_message = 'disk scan unavailable'
		WHERE compute_id = ? AND collector_key = 'disks'
	`, receivedAt[1], reportIDs[1], receivedAt[1], reportIDs[1], computeID); err != nil {
		t.Fatalf("record failed collector attempt: %v", err)
	}
	var presence string
	var absentAt, absenceReportID sql.NullString
	if err := db.QueryRow(`SELECT presence_state, absent_at, absence_report_id FROM compute_disks WHERE compute_id = ? AND device_name = 'sda'`, computeID).
		Scan(&presence, &absentAt, &absenceReportID); err != nil {
		t.Fatalf("read disk after failed collector: %v", err)
	}
	if presence != "present" || absentAt.Valid || absenceReportID.Valid {
		t.Fatalf("failed collector incorrectly established disk absence: state=%s at=%v report=%v", presence, absentAt, absenceReportID)
	}
	var lastSuccessID, lastFailureID string
	if err := db.QueryRow(`
		SELECT last_successful_report_id, last_failed_report_id FROM compute_collector_observations
		WHERE compute_id = ? AND collector_key = 'disks'
	`, computeID).Scan(&lastSuccessID, &lastFailureID); err != nil {
		t.Fatalf("read success/failure authority references: %v", err)
	}
	if lastSuccessID != reportIDs[0] || lastFailureID != reportIDs[1] {
		t.Fatalf("collector authority refs = success %s failure %s", lastSuccessID, lastFailureID)
	}

	if _, err := db.Exec(`
		UPDATE compute_collector_observations SET
			last_status = 'success', last_received_at = ?, last_report_id = ?,
			last_successful_at = ?, last_successful_report_id = ?,
			last_error_code = NULL, last_error_message = NULL
		WHERE compute_id = ? AND collector_key = 'disks'
	`, receivedAt[2], reportIDs[2], receivedAt[2], reportIDs[2], computeID); err != nil {
		t.Fatalf("record next successful collector attempt: %v", err)
	}
	if _, err := db.Exec(`
		UPDATE compute_disks SET presence_state = 'absent', absent_at = ?, absence_report_id = ?
		WHERE compute_id = ? AND device_name = 'sda'
	`, receivedAt[2], reportIDs[2], computeID); err != nil {
		t.Fatalf("mark disk absent from authoritative successful report: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO compute_collector_observations (
			compute_id, collector_key, collector_name, owner_uid, connection_name,
			last_status, last_received_at, last_report_id, last_failed_at,
			last_failed_report_id, last_error_code, last_error_message
		) VALUES (?, 'podman:1000', 'podman', 1000, 'rootless', 'failed', ?, ?, ?, ?, 'socket_unavailable', 'socket unavailable')
	`, computeID, receivedAt[2], reportIDs[2], receivedAt[2], reportIDs[2]); err != nil {
		t.Fatalf("insert owner-scoped Podman collector status: %v", err)
	}
	if _, err := db.Exec(`
		UPDATE compute_collector_observations SET connection_name = 'renamed-label'
		WHERE compute_id = ? AND collector_key = 'podman:1000'
	`, computeID); err != nil {
		t.Fatalf("rename Podman display label without changing identity: %v", err)
	}
	expectConstraintFailure(t, db, `
		INSERT INTO compute_collector_observations (
			compute_id, collector_key, collector_name, owner_uid, connection_name,
			last_status, last_received_at, last_report_id, last_failed_at,
			last_failed_report_id, last_error_code, last_error_message
		) VALUES (?, 'podman:root', 'podman', NULL, 'root', 'failed', ?, ?, ?, ?, 'socket_unavailable', 'socket unavailable')
	`, computeID, receivedAt[2], reportIDs[2], receivedAt[2], reportIDs[2])
	assertNoForeignKeyViolations(t, db)

	if _, err := db.Exec(`DELETE FROM discovery_reports WHERE id = ?`, reportIDs[2]); err != nil {
		t.Fatalf("retain current facts while deleting report history: %v", err)
	}
	if err := db.QueryRow(`SELECT presence_state, absent_at, absence_report_id FROM compute_disks WHERE compute_id = ? AND device_name = 'sda'`, computeID).
		Scan(&presence, &absentAt, &absenceReportID); err != nil {
		t.Fatalf("read retained current disk after report retention: %v", err)
	}
	if presence != "absent" || !absentAt.Valid || absenceReportID.Valid {
		t.Fatalf("report retention changed current absence state: state=%s at=%v report=%v", presence, absentAt, absenceReportID)
	}
	if err := goose.DownToContext(context.Background(), db, "migrations", 5); err == nil {
		t.Fatal("downgrade unexpectedly deleted populated collector authority state")
	}
	var collectorStateCount int
	if err := db.QueryRow(`SELECT count(*) FROM compute_collector_observations WHERE compute_id = ?`, computeID).Scan(&collectorStateCount); err != nil || collectorStateCount != 2 {
		t.Fatalf("collector authority state was not preserved after rejected downgrade: count=%d err=%v", collectorStateCount, err)
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
	providerID := "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73ba"
	if _, err := db.Exec(`
		INSERT INTO providers (id, name, revision, created_at, updated_at)
		VALUES (?, 'mapped role provider', 1, ?, ?)
	`, providerID, testTime, testTime); err != nil {
		t.Fatalf("insert provider for role mapping: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO provider_roles (provider_id, role) VALUES (?, 'hosting')`, providerID); err != nil {
		t.Fatalf("insert SQL-side hosting role: %v", err)
	}
	expectConstraintFailure(t, db, `INSERT INTO provider_roles (provider_id, role) VALUES (?, 'hosting_provider')`, providerID)

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
		INSERT INTO discovery_reports (
			id, report_id, agent_id, protocol_version, client_version, received_at,
			client_collected_at, collection_started_at, collection_finished_at,
			snapshot_content_id, result, content_reused, request_fingerprint, envelope_version
		) VALUES (?, ?, ?, 1, '1.0.0', ?, ?, ?, ?, ?, 'accepted', 0, ?, 1)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834635", reportID, agentID, testTime, testTime,
		testTime, testTime, snapshotID, hash); err != nil {
		t.Fatalf("insert report fixture: %v", err)
	}
	expectConstraintFailure(t, db, `
		INSERT INTO discovery_reports (
			id, report_id, agent_id, protocol_version, client_version, received_at,
			client_collected_at, collection_started_at, collection_finished_at,
			snapshot_content_id, result, content_reused, request_fingerprint, envelope_version
		) VALUES (?, ?, ?, 1, '1.0.0', ?, ?, ?, ?, ?, 'accepted', 0, ?, 1)
	`, "0190d7c0-28f3-71a2-a0fd-d17dd0834636", reportID, agentID, testTime, testTime,
		testTime, testTime, snapshotID, hash)

	acceptedWithErrorsID := "0190d7c0-28f3-71a2-a0fd-d17dd0834637"
	acceptedWithErrorsClientID := "0190d7c0-28f3-71a2-a0fd-d17dd0834638"
	fingerprint := "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	collectionStarted := "2026-08-03T12:30:00Z"
	collectionFinished := "2026-08-03T12:34:00Z"
	if _, err := db.Exec(`
		INSERT INTO discovery_reports (
			id, report_id, agent_id, protocol_version, client_version, received_at,
			client_collected_at, collection_started_at, collection_finished_at,
			snapshot_content_id, result, content_reused, request_fingerprint,
			envelope_version
		) VALUES (?, ?, ?, 1, '1.0.0', ?, ?, ?, ?, ?, 'accepted_with_errors', 0, ?, 1)
	`, acceptedWithErrorsID, acceptedWithErrorsClientID, agentID, testTime, collectionFinished,
		collectionStarted, collectionFinished, snapshotID, fingerprint); err != nil {
		t.Fatalf("insert accepted-with-errors report envelope: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO discovery_collector_results (report_id, collector_name, success, sanitized_error)
		VALUES (?, 'podman:1000', 0, 'socket unavailable')
	`, acceptedWithErrorsID); err != nil {
		t.Fatalf("insert immutable collector result: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO discovery_report_measurements (report_id, host_uptime_seconds)
		VALUES (?, 1234)
	`, acceptedWithErrorsID); err != nil {
		t.Fatalf("insert report uptime measurement: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO discovery_report_filesystem_measurements (report_id, device, mount_point, available_bytes)
		VALUES (?, '/dev/sda1', '/', 4096)
	`, acceptedWithErrorsID); err != nil {
		t.Fatalf("insert report filesystem measurement: %v", err)
	}
	expectConstraintFailure(t, db, `UPDATE discovery_reports SET content_reused = 1 WHERE id = ?`, acceptedWithErrorsID)
	expectConstraintFailure(t, db, `UPDATE discovery_collector_results SET success = 1 WHERE report_id = ?`, acceptedWithErrorsID)
	expectConstraintFailure(t, db, `UPDATE discovery_report_measurements SET host_uptime_seconds = 0 WHERE report_id = ?`, acceptedWithErrorsID)

	rejectedID := "0190d7c0-28f3-71a2-a0fd-d17dd0834639"
	if _, err := db.Exec(`
		INSERT INTO discovery_reports (
			id, report_id, agent_id, protocol_version, client_version, received_at,
			client_collected_at, collection_started_at, collection_finished_at,
			result, request_fingerprint, envelope_version
		) VALUES (?, ?, ?, 1, '1.0.0', ?, ?, ?, ?, 'rejected', ?, 1)
	`, rejectedID, "0190d7c0-28f3-71a2-a0fd-d17dd0834646", agentID, testTime,
		collectionFinished, collectionStarted, collectionFinished, fingerprint); err != nil {
		t.Fatalf("insert rejected report without content: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM discovery_reports WHERE id = ?`, acceptedWithErrorsID); err != nil {
		t.Fatalf("delete retained report for history cleanup: %v", err)
	}
	var measurementCount int
	if err := db.QueryRow(`SELECT count(*) FROM discovery_report_filesystem_measurements WHERE report_id = ?`, acceptedWithErrorsID).Scan(&measurementCount); err != nil || measurementCount != 0 {
		t.Fatalf("report measurements did not follow report retention: count=%d err=%v", measurementCount, err)
	}
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
		INSERT INTO dns_projections (
			id, dns_name_id, provider, desired_revision, state, retryable,
			last_error_code, last_error_message, updated_at
		) VALUES ('0190d7c0-28f3-71a2-a0fd-d17dd0834649', ?, 'internal', 2, 'failed', 1,
			'transient', 'retry time missing', ?)
	`, nameID, testTime)
	expectConstraintFailure(t, db, `
		INSERT INTO dns_projections (
			id, dns_name_id, provider, desired_revision, applied_revision, state, updated_at
		) VALUES ('0190d7c0-28f3-71a2-a0fd-d17dd0834650', ?, 'internal', 2, 0, 'not_applicable', ?)
	`, nameID, testTime)

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
	internalProjectionID := "0190d7c0-28f3-71a2-a0fd-d17dd0834654"

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
		AppliedRevision: 0,
		State:           "pending",
		UpdatedAt:       "2026-08-03T12:35:00Z",
	}
	if _, err := queries.ScheduleDNSProjection(ctx, schedule); err != nil {
		t.Fatalf("schedule projection: %v", err)
	}
	internalSchedule := schedule
	internalSchedule.ID = internalProjectionID
	internalSchedule.Provider = "internal"
	if _, err := queries.ScheduleDNSProjection(ctx, internalSchedule); err != nil {
		t.Fatalf("schedule internal projection: %v", err)
	}

	claimed, err := queries.ClaimDNSProjections(ctx, generated.ClaimDNSProjectionsParams{
		LeaseOwner:     "worker-1",
		LeaseExpiresAt: "2026-08-03T12:40:00Z",
		ClaimedAt:      "2026-08-03T12:36:00Z",
		Provider:       "cloudflare",
		BatchSize:      1,
	})
	if err != nil {
		t.Fatalf("claim projection: %v", err)
	}
	if len(claimed) != 1 || claimed[0].AttemptCount != 1 || claimed[0].LastAttemptAt.String != "2026-08-03T12:36:00Z" {
		t.Fatalf("claimed projection = %#v, want one attempt recorded", claimed)
	}
	otherProviderClaim, err := queries.ClaimDNSProjections(ctx, generated.ClaimDNSProjectionsParams{
		LeaseOwner: "wrong-provider-worker", LeaseExpiresAt: "2026-08-03T12:40:00Z",
		ClaimedAt: "2026-08-03T12:36:00Z", Provider: "cloudflare", BatchSize: 1,
	})
	if err != nil || len(otherProviderClaim) != 0 {
		t.Fatalf("provider-specific claim mixed additional Cloudflare work: %#v err=%v", otherProviderClaim, err)
	}
	internalClaim, err := queries.ClaimDNSProjections(ctx, generated.ClaimDNSProjectionsParams{
		LeaseOwner: "internal-worker", LeaseExpiresAt: "2026-08-03T12:40:00Z",
		ClaimedAt: "2026-08-03T12:36:00Z", Provider: "internal", BatchSize: 1,
	})
	if err != nil || len(internalClaim) != 1 || internalClaim[0].Provider != "internal" {
		t.Fatalf("internal provider claim = %#v err=%v", internalClaim, err)
	}
	rows, err := queries.CompleteDNSProjection(ctx, generated.CompleteDNSProjectionParams{
		AppliedRevision: 2, CompletionState: "not_applicable", CompletedAt: "2026-08-03T12:39:00Z",
		ID: internalProjectionID, Provider: "internal", LeaseOwner: "internal-worker",
	})
	if err != nil || rows != 1 {
		t.Fatalf("complete internal not-applicable cleanup rows=%d err=%v", rows, err)
	}

	for _, revision := range []int64{2, 1} {
		rejected := schedule
		rejected.DesiredRevision = revision
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

	rows, err = queries.CompleteDNSProjection(ctx, generated.CompleteDNSProjectionParams{
		AppliedRevision: 2, CompletionState: "synchronized", CompletedAt: "2026-08-03T12:40:00Z",
		ID: projectionID, Provider: "cloudflare", LeaseOwner: "worker-1",
	})
	if err != nil || rows != 0 {
		t.Fatalf("expired completion rows = %d, err = %v, want 0, nil", rows, err)
	}
	rows, err = queries.CompleteDNSProjection(ctx, generated.CompleteDNSProjectionParams{
		AppliedRevision: 2, CompletionState: "synchronized", CompletedAt: "2026-08-03T12:39:00Z",
		ID: projectionID, Provider: "cloudflare", LeaseOwner: "worker-1",
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
	preservedApplied, err := queries.GetDNSProjection(ctx, projectionID)
	if err != nil || preservedApplied.AppliedRevision != 2 {
		t.Fatalf("scheduling a newer revision erased last applied revision: projection=%#v err=%v", preservedApplied, err)
	}
	claimed, err = queries.ClaimDNSProjections(ctx, generated.ClaimDNSProjectionsParams{
		LeaseOwner:     "worker-2",
		LeaseExpiresAt: "2026-08-03T12:50:00Z",
		ClaimedAt:      "2026-08-03T12:42:00Z",
		Provider:       "cloudflare",
		BatchSize:      1,
	})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim newer projection = %#v, err = %v", claimed, err)
	}
	if _, err := db.Exec(`UPDATE dns_names SET desired_revision = 4, updated_at = '2026-08-03T12:43:00Z' WHERE id = ?`, nameID); err != nil {
		t.Fatalf("advance DNS name beyond lease: %v", err)
	}
	rows, err = queries.CompleteDNSProjection(ctx, generated.CompleteDNSProjectionParams{
		AppliedRevision: 3, CompletionState: "synchronized", CompletedAt: "2026-08-03T12:44:00Z",
		ID: projectionID, Provider: "cloudflare", LeaseOwner: "worker-2",
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
		Provider:       "cloudflare",
		BatchSize:      1,
	})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim failure projection = %#v, err = %v", claimed, err)
	}
	rows, err = queries.FailDNSProjection(ctx, generated.FailDNSProjectionParams{
		Retryable:      0,
		NextRetryAt:    sql.NullString{},
		ErrorCode:      "unmanaged_conflict",
		ErrorMessage:   "unmanaged record blocks repair",
		FailedAt:       "2026-08-03T12:47:00Z",
		ID:             projectionID,
		Provider:       "cloudflare",
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
	if failed.State != "failed" || failed.AttemptCount != 1 || failed.LeaseOwner.Valid || failed.Retryable != 0 ||
		failed.LastErrorCode.String != "unmanaged_conflict" || failed.NextRetryAt.Valid {
		t.Fatalf("failed projection = %#v", failed)
	}
	permanentClaim, err := queries.ClaimDNSProjections(ctx, generated.ClaimDNSProjectionsParams{
		LeaseOwner: "worker-4", LeaseExpiresAt: "2026-08-03T13:10:00Z",
		ClaimedAt: "2026-08-03T13:06:00Z", Provider: "cloudflare", BatchSize: 10,
	})
	if err != nil || len(permanentClaim) != 0 {
		t.Fatalf("permanent failed projection was automatically claimed: %#v err=%v", permanentClaim, err)
	}
	if _, err := queries.ScheduleDNSProjection(ctx, schedule); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("same-revision schedule error = %v, want explicit repair", err)
	}
	if _, err := queries.RepairDNSProjection(ctx, generated.RepairDNSProjectionParams{
		DnsNameID: nameID, Provider: "cloudflare", DesiredRevision: 4, RepairedAt: "2026-08-03T13:07:00Z",
	}); err != nil {
		t.Fatalf("explicit same-revision repair: %v", err)
	}
	repairedClaim, err := queries.ClaimDNSProjections(ctx, generated.ClaimDNSProjectionsParams{
		LeaseOwner: "worker-5", LeaseExpiresAt: "2026-08-03T13:20:00Z",
		ClaimedAt: "2026-08-03T13:08:00Z", Provider: "cloudflare", BatchSize: 10,
	})
	if err != nil || len(repairedClaim) != 1 || repairedClaim[0].DesiredRevision != 4 {
		t.Fatalf("same-revision repaired claim = %#v err=%v", repairedClaim, err)
	}
	rows, err = queries.CompleteDNSProjection(ctx, generated.CompleteDNSProjectionParams{
		AppliedRevision: 4, CompletionState: "not_applicable", CompletedAt: "2026-08-03T13:09:00Z",
		ID: projectionID, Provider: "cloudflare", LeaseOwner: "worker-5",
	})
	if err != nil || rows != 1 {
		t.Fatalf("cleanup completion rows = %d err=%v, want not-applicable success", rows, err)
	}
	cleaned, err := queries.GetDNSProjection(ctx, projectionID)
	if err != nil || cleaned.State != "not_applicable" || cleaned.AppliedRevision != 4 {
		t.Fatalf("cleanup transition = %#v err=%v", cleaned, err)
	}
	if _, err := db.Exec(`UPDATE dns_names SET desired_revision = 5, updated_at = '2026-08-03T13:10:00Z' WHERE id = ?`, nameID); err != nil {
		t.Fatalf("advance DNS name for retryable failure: %v", err)
	}
	schedule.DesiredRevision = 5
	schedule.UpdatedAt = "2026-08-03T13:10:00Z"
	if _, err := queries.ScheduleDNSProjection(ctx, schedule); err != nil {
		t.Fatalf("schedule retryable projection: %v", err)
	}
	claimAt := "2026-08-03T13:11:00Z"
	claimed, err = queries.ClaimDNSProjections(ctx, generated.ClaimDNSProjectionsParams{
		LeaseOwner: "worker-6", LeaseExpiresAt: "2026-08-03T13:15:00Z", ClaimedAt: claimAt,
		Provider: "cloudflare", BatchSize: 1,
	})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim retryable projection = %#v err=%v", claimed, err)
	}
	rows, err = queries.FailDNSProjection(ctx, generated.FailDNSProjectionParams{
		Retryable: 1, NextRetryAt: sql.NullString{String: "2026-08-03T13:20:00Z", Valid: true},
		ErrorCode: "provider_unavailable", ErrorMessage: "temporary provider failure",
		FailedAt: "2026-08-03T13:12:00Z", ID: projectionID, Provider: "cloudflare",
		LeaseOwner: "worker-6", FailedRevision: 5,
	})
	if err != nil || rows != 1 {
		t.Fatalf("record retryable failure rows=%d err=%v", rows, err)
	}
	tooEarly, err := queries.ClaimDNSProjections(ctx, generated.ClaimDNSProjectionsParams{
		LeaseOwner: "worker-7", LeaseExpiresAt: "2026-08-03T13:25:00Z", ClaimedAt: "2026-08-03T13:19:00Z",
		Provider: "cloudflare", BatchSize: 1,
	})
	if err != nil || len(tooEarly) != 0 {
		t.Fatalf("retryable projection claimed before next_retry_at: %#v err=%v", tooEarly, err)
	}
	due, err := queries.ClaimDNSProjections(ctx, generated.ClaimDNSProjectionsParams{
		LeaseOwner: "worker-7", LeaseExpiresAt: "2026-08-03T13:30:00Z", ClaimedAt: "2026-08-03T13:20:00Z",
		Provider: "cloudflare", BatchSize: 1,
	})
	if err != nil || len(due) != 1 {
		t.Fatalf("retryable projection not claimed when due: %#v err=%v", due, err)
	}
	rows, err = queries.ReleaseExpiredDNSProjectionLeases(ctx, generated.ReleaseExpiredDNSProjectionLeasesParams{
		UpdatedAt: "2026-08-03T13:31:00Z", Provider: "internal", LeaseExpiresAt: sql.NullString{String: "2026-08-03T13:31:00Z", Valid: true},
	})
	if err != nil || rows != 0 {
		t.Fatalf("other-provider lease cleanup rows=%d err=%v", rows, err)
	}
	rows, err = queries.ReleaseExpiredDNSProjectionLeases(ctx, generated.ReleaseExpiredDNSProjectionLeasesParams{
		UpdatedAt: "2026-08-03T13:31:00Z", Provider: "cloudflare", LeaseExpiresAt: sql.NullString{String: "2026-08-03T13:31:00Z", Valid: true},
	})
	if err != nil || rows != 1 {
		t.Fatalf("provider-scoped expired lease cleanup rows=%d err=%v", rows, err)
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
