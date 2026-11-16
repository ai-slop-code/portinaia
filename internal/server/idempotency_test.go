package server

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func idempotencyDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/idempotency.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE idempotency_records (
		actor_id TEXT NOT NULL, operation_id TEXT NOT NULL, key_hash TEXT NOT NULL,
		request_hash TEXT NOT NULL, response_status INTEGER NOT NULL,
		response_headers_json TEXT NOT NULL, response_body_json TEXT NOT NULL,
		created_at TEXT NOT NULL, expires_at TEXT NOT NULL,
		PRIMARY KEY(actor_id, operation_id, key_hash))`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestIdempotencyLookupConflictExpiryAndReplacement(t *testing.T) {
	ctx := context.Background()
	db := idempotencyDB(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	key := "client-key"
	beginStore := func(fingerprint, body string, at time.Time) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := StoreIdempotencyTx(ctx, tx, "actor", "createWidget", key, fingerprint, 201, `{}`, body, at); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(fingerprint string, at time.Time) (IdempotencyResult, error) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		return LookupIdempotencyTx(ctx, tx, "actor", "createWidget", key, fingerprint, at)
	}

	beginStore("fingerprint-1", `{"id":"first"}`, now)
	result, err := lookup("fingerprint-1", now.Add(time.Hour))
	if err != nil || !result.Found || result.Expired || result.Status != 201 || result.BodyJSON != `{"id":"first"}` {
		t.Fatalf("matching lookup = %#v, %v", result, err)
	}
	if _, err := lookup("different", now.Add(time.Hour)); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting fingerprint error = %v", err)
	}
	result, err = lookup("different", now.Add(24*time.Hour))
	if err != nil || !result.Expired || result.Found {
		t.Fatalf("expiry-boundary lookup = %#v, %v", result, err)
	}

	beginStore("fingerprint-2", `{"id":"second"}`, now.Add(24*time.Hour))
	result, err = lookup("fingerprint-2", now.Add(24*time.Hour+time.Minute))
	if err != nil || !result.Found || result.BodyJSON != `{"id":"second"}` {
		t.Fatalf("replacement lookup = %#v, %v", result, err)
	}
	if _, err := db.Exec(`INSERT INTO idempotency_records VALUES(?,?,?,?,?,?,?,?,?)`, "actor", "otherOperation", idempotencyKeyHash(key), "other", 201, `{}`, `{}`, now.Format(time.RFC3339), now.Add(24*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("same key under another operation should be independent: %v", err)
	}
}

func TestIdempotencyStoreRollsBackWithCallerTransaction(t *testing.T) {
	ctx := context.Background()
	db := idempotencyDB(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := StoreIdempotencyTx(ctx, tx, "actor", "createWidget", "key", "fingerprint", 201, `{}`, `{"id":"rolled-back"}`, now); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	result, err := func() (IdempotencyResult, error) {
		readTx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return IdempotencyResult{}, err
		}
		defer readTx.Rollback()
		return LookupIdempotencyTx(ctx, readTx, "actor", "createWidget", "key", "fingerprint", now)
	}()
	if err != nil || result.Found || result.Expired {
		t.Fatalf("lookup after rollback = %#v, %v", result, err)
	}
}
