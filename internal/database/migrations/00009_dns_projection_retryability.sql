-- +goose Up
ALTER TABLE dns_projections
    ADD COLUMN retryable INTEGER NOT NULL DEFAULT 1 CHECK (retryable IN (0, 1));

-- Failed rows without a retry time are legacy permanent failures, not due work.
UPDATE dns_projections
SET retryable = 0
WHERE state = 'failed' AND next_retry_at IS NULL;

-- A not-applicable projection is clean through the current desired revision.
UPDATE dns_projections
SET applied_revision = desired_revision
WHERE state = 'not_applicable' AND applied_revision <> desired_revision;

DROP INDEX dns_projections_due_idx;
CREATE INDEX dns_projections_provider_due_idx
    ON dns_projections (provider, state, retryable, next_retry_at, lease_expires_at, updated_at, id)
    WHERE state IN ('pending', 'failed', 'deleting');

-- A transient failed state must be scheduled; permanent failures are never due.
-- +goose StatementBegin
CREATE TRIGGER dns_projections_failed_retryable_insert_guard
BEFORE INSERT ON dns_projections
WHEN NEW.state = 'failed' AND NEW.retryable = 1 AND NEW.next_retry_at IS NULL
BEGIN
    SELECT RAISE(ABORT, 'retryable DNS projection failure requires next_retry_at');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER dns_projections_failed_retryable_update_guard
BEFORE UPDATE OF state, retryable, next_retry_at ON dns_projections
WHEN NEW.state = 'failed' AND NEW.retryable = 1 AND NEW.next_retry_at IS NULL
BEGIN
    SELECT RAISE(ABORT, 'retryable DNS projection failure requires next_retry_at');
END;
-- +goose StatementEnd

-- Cleanup completion is explicitly recorded as not-applicable at the current revision.
-- +goose StatementBegin
CREATE TRIGGER dns_projections_not_applicable_update_guard
BEFORE UPDATE OF state, applied_revision, desired_revision ON dns_projections
WHEN NEW.state = 'not_applicable' AND NEW.applied_revision <> NEW.desired_revision
BEGIN
    SELECT RAISE(ABORT, 'not-applicable DNS projection must be clean at its desired revision');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER dns_projections_not_applicable_insert_guard
BEFORE INSERT ON dns_projections
WHEN NEW.state = 'not_applicable' AND NEW.applied_revision <> NEW.desired_revision
BEGIN
    SELECT RAISE(ABORT, 'not-applicable DNS projection must be clean at its desired revision');
END;
-- +goose StatementEnd

PRAGMA user_version = 9;

-- +goose Down
CREATE TABLE _wp02_down_guard (valid INTEGER NOT NULL CHECK (valid = 1));
INSERT INTO _wp02_down_guard (valid)
SELECT CASE WHEN
    EXISTS (SELECT 1 FROM dns_projections WHERE retryable = 0)
    OR EXISTS (SELECT 1 FROM dns_projections WHERE state = 'not_applicable' AND applied_revision <> desired_revision)
THEN 0 ELSE 1 END;
DROP TABLE _wp02_down_guard;

DROP TRIGGER dns_projections_failed_retryable_insert_guard;
DROP TRIGGER dns_projections_failed_retryable_update_guard;
DROP TRIGGER dns_projections_not_applicable_update_guard;
DROP TRIGGER dns_projections_not_applicable_insert_guard;
DROP INDEX dns_projections_provider_due_idx;
ALTER TABLE dns_projections DROP COLUMN retryable;
CREATE INDEX dns_projections_due_idx ON dns_projections (state, next_retry_at, lease_expires_at, updated_at, id)
    WHERE state IN ('pending', 'failed', 'deleting');
PRAGMA user_version = 8;
