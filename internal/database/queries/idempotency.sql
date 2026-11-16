-- name: GetIdempotencyRecord :one
SELECT actor_id, operation_id, key_hash, request_hash, response_status,
       response_headers_json, response_body_json, created_at, expires_at
FROM idempotency_records
WHERE actor_id = ? AND operation_id = ? AND key_hash = ? AND expires_at > ?;

-- name: InsertIdempotencyRecord :exec
INSERT INTO idempotency_records (
    actor_id, operation_id, key_hash, request_hash, response_status,
    response_headers_json, response_body_json, created_at, expires_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: DeleteExpiredIdempotencyRecords :execrows
DELETE FROM idempotency_records WHERE expires_at <= ?;
