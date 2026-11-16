package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
)

const ExpectedSchemaVersion int64 = 10

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

// MigrationStatus writes the embedded migration state without changing the schema.
func MigrationStatus(ctx context.Context, db *sql.DB) error {
	if err := CheckSchema(ctx, db); err != nil {
		return err
	}
	goose.SetBaseFS(Migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose sqlite dialect: %w", err)
	}
	if err := goose.StatusContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("read migration status: %w", err)
	}
	return nil
}

// CheckSchema refuses missing, inconsistent, old, and newer schemas. Schema
// changes are performed only by the explicit db migrate command.
func CheckSchema(ctx context.Context, db *sql.DB) error {
	version, err := SchemaVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("database schema is not ready; run `portinaia db migrate`: %w", err)
	}
	var applied sql.NullInt64
	err = db.QueryRowContext(ctx, `SELECT MAX(version_id) FROM goose_db_version WHERE is_applied=1`).Scan(&applied)
	if err != nil {
		return fmt.Errorf("cannot read goose migration metadata; run `portinaia db migrate`: %w", err)
	}
	if !applied.Valid || applied.Int64 != version {
		return fmt.Errorf("inconsistent schema versions: PRAGMA user_version=%d, goose version=%v; run `portinaia db status` and repair the migration state", version, applied)
	}
	if version != ExpectedSchemaVersion {
		return fmt.Errorf("unsupported schema version %d (expected %d); back up the database and run `portinaia db migrate`", version, ExpectedSchemaVersion)
	}
	return nil
}
