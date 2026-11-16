-- +goose Up
CREATE TABLE idempotency_records (
    actor_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    key_hash TEXT NOT NULL CHECK(length(key_hash) = 64),
    request_hash TEXT NOT NULL CHECK(length(request_hash) = 64),
    response_status INTEGER NOT NULL CHECK(response_status BETWEEN 200 AND 299),
    response_headers_json TEXT NOT NULL,
    response_body_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    PRIMARY KEY (actor_id, operation_id, key_hash)
);
CREATE INDEX idempotency_records_expiry_idx ON idempotency_records(expires_at);
PRAGMA user_version = 10;

-- +goose Down
DROP TABLE idempotency_records;
PRAGMA user_version = 9;
