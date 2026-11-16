package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Backup writes a consistent SQLite snapshot using VACUUM INTO. The output must not exist.
func Backup(ctx context.Context, db *sql.DB, destination string) (err error) {
	if strings.TrimSpace(destination) == "" {
		return errors.New("backup destination must not be empty")
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve backup destination: %w", err)
	}
	var source string
	if err := db.QueryRowContext(ctx, `PRAGMA database_list`).Scan(new(int), new(string), &source); err != nil {
		return fmt.Errorf("read database path: %w", err)
	}
	if source != "" {
		sourcePath, err := filepath.Abs(source)
		if err != nil {
			return fmt.Errorf("resolve database path: %w", err)
		}
		if filepath.Clean(sourcePath) == filepath.Clean(absolute) {
			return errors.New("backup destination must differ from the live database")
		}
	}
	if _, err := os.Stat(absolute); err == nil {
		return fmt.Errorf("backup destination already exists: %s", absolute)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check backup destination: %w", err)
	}
	created := false
	defer func() {
		if err != nil && created {
			_ = os.Remove(absolute)
		}
	}()
	if _, err = db.ExecContext(ctx, `VACUUM INTO ?`, absolute); err != nil {
		return fmt.Errorf("create sqlite backup: %w", err)
	}
	created = true
	if err = os.Chmod(absolute, 0o600); err != nil {
		return fmt.Errorf("restrict backup permissions: %w", err)
	}
	return nil
}

// Verify checks SQLite page integrity, foreign-key consistency, and application schema version.
func Verify(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("run sqlite integrity check: %w", err)
	}
	checks := 0
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			rows.Close()
			return fmt.Errorf("read sqlite integrity check: %w", err)
		}
		checks++
		if result != "ok" {
			rows.Close()
			return fmt.Errorf("sqlite integrity check failed: %s", result)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("read sqlite integrity results: %w", err)
	}
	rows.Close()
	if checks == 0 {
		return errors.New("sqlite integrity check returned no results")
	}
	foreignKeys, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("run sqlite foreign-key check: %w", err)
	}
	defer foreignKeys.Close()
	if foreignKeys.Next() {
		var table string
		var rowID sql.NullInt64
		var parent string
		var constraint int
		if err := foreignKeys.Scan(&table, &rowID, &parent, &constraint); err != nil {
			return fmt.Errorf("read sqlite foreign-key failure: %w", err)
		}
		return fmt.Errorf("sqlite foreign-key check failed: table %s row %v references %s (constraint %d)", table, rowID, parent, constraint)
	}
	if err := foreignKeys.Err(); err != nil {
		return fmt.Errorf("read sqlite foreign-key results: %w", err)
	}
	if _, err := SchemaVersion(ctx, db); err != nil {
		return err
	}
	return nil
}
