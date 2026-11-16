package database

import (
	"testing"
)

func TestOpenAppliesDurableSQLitePragmas(t *testing.T) {
	db, err := Open(t.TempDir() + "/configured.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var foreignKeys, busyTimeout, synchronous int
	var journalMode string
	for query, target := range map[string]any{
		"PRAGMA foreign_keys": &foreignKeys,
		"PRAGMA busy_timeout": &busyTimeout,
		"PRAGMA synchronous":  &synchronous,
		"PRAGMA journal_mode": &journalMode,
	} {
		if err := db.QueryRow(query).Scan(target); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	if foreignKeys != 1 || busyTimeout != 5000 || synchronous != 2 || journalMode != "wal" {
		t.Fatalf("pragmas: foreign_keys=%d busy_timeout=%d synchronous=%d journal_mode=%q", foreignKeys, busyTimeout, synchronous, journalMode)
	}
}
