-- +goose Up
CREATE TABLE compute_collector_observations (
    compute_id TEXT NOT NULL REFERENCES computes(id) ON DELETE CASCADE,
    collector_key TEXT NOT NULL CHECK (length(collector_key) BETWEEN 1 AND 64),
    collector_name TEXT NOT NULL CHECK (collector_name IN ('host', 'interfaces', 'disks', 'filesystems', 'podman')),
    owner_uid INTEGER CHECK (owner_uid IS NULL OR owner_uid BETWEEN 0 AND 4294967295),
    connection_name TEXT CHECK (connection_name IS NULL OR length(trim(connection_name)) BETWEEN 1 AND 200),
    last_status TEXT NOT NULL CHECK (last_status IN ('success', 'failed')),
    last_received_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_received_at) = last_received_at, 0)),
    last_report_id TEXT REFERENCES discovery_reports(id) ON DELETE SET NULL,
    last_successful_at TEXT CHECK (last_successful_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_successful_at) = last_successful_at, 0)),
    last_successful_report_id TEXT REFERENCES discovery_reports(id) ON DELETE SET NULL,
    last_failed_at TEXT CHECK (last_failed_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_failed_at) = last_failed_at, 0)),
    last_failed_report_id TEXT REFERENCES discovery_reports(id) ON DELETE SET NULL,
    last_error_code TEXT CHECK (last_error_code IS NULL OR (length(last_error_code) BETWEEN 1 AND 100 AND last_error_code NOT GLOB '*[^a-z0-9_]*')),
    last_error_message TEXT CHECK (last_error_message IS NULL OR length(last_error_message) <= 2000),
    PRIMARY KEY (compute_id, collector_key),
    CHECK (
        (collector_name = 'podman' AND owner_uid IS NOT NULL AND connection_name IS NOT NULL
            AND collector_key = 'podman:' || CAST(owner_uid AS TEXT))
        OR
        (collector_name <> 'podman' AND owner_uid IS NULL AND connection_name IS NULL
            AND collector_key = collector_name)
    ),
    CHECK (
        (last_status = 'success' AND last_successful_at = last_received_at
            AND last_error_code IS NULL AND last_error_message IS NULL)
        OR
        (last_status = 'failed' AND last_failed_at = last_received_at
            AND last_error_code IS NOT NULL AND last_error_message IS NOT NULL)
    ),
    CHECK (last_successful_at IS NULL OR last_successful_at <= last_received_at),
    CHECK (last_failed_at IS NULL OR last_failed_at <= last_received_at)
) WITHOUT ROWID;

CREATE INDEX compute_collector_observations_status_idx
    ON compute_collector_observations (last_status, last_received_at, compute_id);
CREATE INDEX compute_collector_observations_report_idx
    ON compute_collector_observations (last_report_id);

PRAGMA user_version = 6;

-- +goose Down
CREATE TABLE _wp02_down_guard (valid INTEGER NOT NULL CHECK (valid = 1));
INSERT INTO _wp02_down_guard (valid)
SELECT CASE WHEN EXISTS (SELECT 1 FROM compute_collector_observations) THEN 0 ELSE 1 END;
DROP TABLE _wp02_down_guard;
DROP TABLE compute_collector_observations;
PRAGMA user_version = 5;
