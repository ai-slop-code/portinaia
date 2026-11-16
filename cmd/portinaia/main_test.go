package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portinaia/internal/database"
)

func TestDiscoveryRetentionRuntimeConfiguration(t *testing.T) {
	for _, name := range []string{"PORTINAIA_DISCOVERY_RETENTION_DAYS", "PORTINAIA_DISCOVERY_RETENTION_BATCH", "PORTINAIA_DISCOVERY_RETENTION_INTERVAL"} {
		t.Setenv(name, "")
	}
	defaults := loadConfig()
	if defaults.discoveryRetentionDays != 90 || defaults.discoveryRetentionBatch != 1000 || defaults.discoveryRetentionInterval != time.Hour {
		t.Fatalf("retention defaults = %d days / %d batch / %s interval", defaults.discoveryRetentionDays, defaults.discoveryRetentionBatch, defaults.discoveryRetentionInterval)
	}
	t.Setenv("PORTINAIA_DISCOVERY_RETENTION_DAYS", "120")
	t.Setenv("PORTINAIA_DISCOVERY_RETENTION_BATCH", "250")
	t.Setenv("PORTINAIA_DISCOVERY_RETENTION_INTERVAL", "2h")
	configured := loadConfig()
	if configured.discoveryRetentionDays != 120 || configured.discoveryRetentionBatch != 250 || configured.discoveryRetentionInterval != 2*time.Hour {
		t.Fatalf("configured retention = %d days / %d batch / %s interval", configured.discoveryRetentionDays, configured.discoveryRetentionBatch, configured.discoveryRetentionInterval)
	}
	if err := serve(config{listenAddress: "127.0.0.1:0", databasePath: filepath.Join(t.TempDir(), "unused.db"), publicURL: "http://127.0.0.1", discoveryRetentionDays: 0, discoveryRetentionBatch: 1000, discoveryRetentionInterval: time.Hour}); err == nil || !strings.Contains(err.Error(), "retention days") {
		t.Fatalf("invalid retention configuration error = %v", err)
	}
}

func TestAdminBootstrapAndPasswordResetCommands(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "portinaia.db")
	t.Setenv("PORTINAIA_DB", dbPath)
	t.Setenv("PORTINAIA_BOOTSTRAP_PASSWORD", "initial-password-is-long")
	if err := run([]string{"db", "migrate", "-path", dbPath}); err != nil {
		t.Fatalf("database migrate command: %v", err)
	}
	if err := run([]string{"admin", "bootstrap", "-username", "admin"}); err != nil {
		t.Fatalf("bootstrap command: %v", err)
	}
	t.Setenv("PORTINAIA_ADMIN_PASSWORD", "recovered-password-is-long")
	if err := run([]string{"admin", "password-reset", "-username", "admin"}); err != nil {
		t.Fatalf("password-reset command: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := run([]string{"db", "backup", "-path", dbPath, "-output", backupPath}); err != nil {
		t.Fatalf("database backup command: %v", err)
	}
	if err := run([]string{"db", "verify", "-path", backupPath}); err != nil {
		t.Fatalf("database verification command: %v", err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file: %v", err)
	}
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM audit_events WHERE action='administrator.password_reset'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("password-reset audit records = %d, want 1", count)
	}
}

func TestServerAndBootstrapRequireExplicitMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "unmigrated.db")
	t.Setenv("PORTINAIA_DB", dbPath)
	t.Setenv("PORTINAIA_BOOTSTRAP_PASSWORD", "initial-password-is-long")
	if err := run([]string{"admin", "bootstrap", "-username", "admin"}); err == nil || !strings.Contains(err.Error(), "db migrate") {
		t.Fatalf("bootstrap on missing schema error = %v", err)
	}
	if err := run([]string{"server", "-listen", "127.0.0.1:0", "-db", dbPath}); err == nil || !strings.Contains(err.Error(), "db migrate") {
		t.Fatalf("server on missing schema error = %v", err)
	}
}
