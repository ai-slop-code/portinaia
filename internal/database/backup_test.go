package database

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupVerifyAndRestore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	livePath := filepath.Join(dir, "live.db")
	live, err := Open(livePath)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	if err := Migrate(ctx, live); err != nil {
		t.Fatal(err)
	}
	if _, err := live.Exec(`INSERT INTO providers(id,name,description,notes,revision,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000001','Backup fixture','','',1,'2026-09-26T00:00:00Z','2026-09-26T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := Verify(ctx, live); err != nil {
		t.Fatalf("verify live database: %v", err)
	}

	backupPath := filepath.Join(dir, "backup.db")
	if err := Backup(ctx, live, backupPath); err != nil {
		t.Fatalf("backup database: %v", err)
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup permissions = %o, want 600", info.Mode().Perm())
	}
	if err := Backup(ctx, live, backupPath); err == nil {
		t.Fatal("backup unexpectedly overwrote an existing file")
	}
	if err := Backup(ctx, live, livePath); err == nil {
		t.Fatal("backup unexpectedly targeted the live database")
	}

	backup, err := Open(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(ctx, backup); err != nil {
		t.Fatalf("verify snapshot: %v", err)
	}
	var name string
	if err := backup.QueryRow(`SELECT name FROM providers WHERE id='00000000-0000-4000-8000-000000000001'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Backup fixture" {
		t.Fatalf("snapshot provider = %q", name)
	}
	if err := backup.Close(); err != nil {
		t.Fatal(err)
	}

	restoredPath := filepath.Join(dir, "restored.db")
	source, err := os.Open(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	restoredFile, err := os.OpenFile(restoredPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		source.Close()
		t.Fatal(err)
	}
	if _, err := io.Copy(restoredFile, source); err != nil {
		t.Fatal(err)
	}
	if err := restoredFile.Close(); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err := Verify(ctx, restored); err != nil {
		t.Fatalf("verify restored database: %v", err)
	}
	if err := restored.QueryRow(`SELECT name FROM providers WHERE id='00000000-0000-4000-8000-000000000001'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
}
