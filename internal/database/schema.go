package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

// Migrations contains the immutable SQL migrations shipped with the server.
//
//go:embed migrations/*.sql
var Migrations embed.FS

// SchemaVersion returns the application schema version recorded by migrations.
func SchemaVersion(ctx context.Context, db *sql.DB) (int64, error) {
	var version int64
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read database schema version: %w", err)
	}
	return version, nil
}

// Migrate applies all embedded migrations to db.
func Migrate(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(Migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose sqlite dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	return nil
}

// MigrateDown rolls an embedded schema back to version zero. It is intended
// for migration verification and controlled development workflows.
func MigrateDown(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(Migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose sqlite dialect: %w", err)
	}
	if err := goose.DownToContext(ctx, db, "migrations", 0); err != nil {
		return fmt.Errorf("roll back database migrations: %w", err)
	}
	return nil
}
