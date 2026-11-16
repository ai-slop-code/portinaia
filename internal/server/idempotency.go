package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrIdempotencyConflict = errors.New("idempotency key was used for a different request")

type IdempotencyResult struct {
	Found                 bool
	Expired               bool
	Status                int
	HeadersJSON, BodyJSON string
}

// LookupIdempotencyTx reads a key using the supplied transaction. It does not
// itself enforce that mutation, audit append, and result storage share a transaction;
// callers must establish that boundary before relying on atomic replay semantics.
func LookupIdempotencyTx(ctx context.Context, tx *sql.Tx, actorID, operationID, key, fingerprint string, now time.Time) (IdempotencyResult, error) {
	var stored string
	var result IdempotencyResult
	var expiry string
	err := tx.QueryRowContext(ctx, `SELECT request_hash,response_status,response_headers_json,response_body_json,expires_at FROM idempotency_records WHERE actor_id=? AND operation_id=? AND key_hash=?`, actorID, operationID, idempotencyKeyHash(key)).Scan(&stored, &result.Status, &result.HeadersJSON, &result.BodyJSON, &expiry)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	expires, err := time.Parse(time.RFC3339, expiry)
	if err != nil {
		return result, fmt.Errorf("parse idempotency expiry: %w", err)
	}
	if !now.Before(expires) {
		result.Expired = true
		return result, nil
	}
	if stored != fingerprint {
		return result, ErrIdempotencyConflict
	}
	result.Found = true
	return result, nil
}

// StoreIdempotencyTx replaces expired state and persists a caller-approved,
// non-secret response using the supplied transaction. Atomicity with a mutation
// and audit append depends on the caller using that same transaction for all three.
func StoreIdempotencyTx(ctx context.Context, tx *sql.Tx, actorID, operationID, key, fingerprint string, status int, headersJSON, bodyJSON string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM idempotency_records WHERE actor_id=? AND operation_id=? AND key_hash=? AND expires_at<=?`, actorID, operationID, idempotencyKeyHash(key), now.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO idempotency_records(actor_id,operation_id,key_hash,request_hash,response_status,response_headers_json,response_body_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, actorID, operationID, idempotencyKeyHash(key), fingerprint, status, headersJSON, bodyJSON, now.UTC().Format(time.RFC3339), now.UTC().Add(24*time.Hour).Format(time.RFC3339))
	return err
}

// idempotencyFingerprint binds a key to the actor, operation, target, revision,
// and normalized request. Callers must execute lookup, mutation, audit, and insert
// on the same SQL transaction. Secret-bearing issuance responses are not replayable.
func idempotencyFingerprint(actorID, operationID, target, expectedRevision string, request any) (string, error) {
	canonical, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode idempotency request: %w", err)
	}
	input, err := json.Marshal([]string{actorID, operationID, target, expectedRevision, string(canonical)})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(input)
	return hex.EncodeToString(sum[:]), nil
}

func idempotencyKeyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
