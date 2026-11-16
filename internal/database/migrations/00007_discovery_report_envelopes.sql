-- +goose NO TRANSACTION
-- +goose Up
PRAGMA foreign_keys = OFF;
BEGIN IMMEDIATE;

CREATE TABLE discovery_reports_new (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    report_id TEXT NOT NULL CHECK (length(report_id) = 36 AND report_id = lower(report_id) AND length(replace(report_id, '-', '')) = 32
        AND substr(report_id, 9, 1) = '-' AND substr(report_id, 14, 1) = '-' AND substr(report_id, 19, 1) = '-' AND substr(report_id, 24, 1) = '-'
        AND replace(report_id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
    protocol_version INTEGER NOT NULL CHECK (protocol_version >= 1),
    client_version TEXT CHECK (client_version IS NULL OR length(client_version) BETWEEN 1 AND 100),
    received_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', received_at) = received_at, 0)),
    client_collected_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', client_collected_at) = client_collected_at, 0)),
    collection_started_at TEXT CHECK (collection_started_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', collection_started_at) = collection_started_at, 0)),
    collection_finished_at TEXT CHECK (collection_finished_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', collection_finished_at) = collection_finished_at, 0)),
    snapshot_content_id TEXT REFERENCES snapshot_contents(id) ON DELETE RESTRICT,
    result TEXT NOT NULL CHECK (result IN ('accepted', 'accepted_with_errors', 'rejected')),
    content_reused INTEGER CHECK (content_reused IS NULL OR content_reused IN (0, 1)),
    request_fingerprint TEXT CHECK (request_fingerprint IS NULL OR (length(request_fingerprint) = 64 AND request_fingerprint = lower(request_fingerprint) AND request_fingerprint NOT GLOB '*[^0-9a-f]*')),
    envelope_version INTEGER NOT NULL DEFAULT 1 CHECK (envelope_version IN (0, 1)),
    collection_error_summary TEXT,
    UNIQUE (agent_id, report_id),
    CHECK ((result IN ('accepted', 'accepted_with_errors') AND snapshot_content_id IS NOT NULL) OR (result = 'rejected' AND snapshot_content_id IS NULL)),
    CHECK (collection_started_at IS NULL OR collection_finished_at IS NULL OR collection_finished_at >= collection_started_at),
    CHECK (
        (envelope_version = 0 AND client_version IS NULL AND collection_started_at IS NULL
            AND collection_finished_at IS NULL AND content_reused IS NULL AND request_fingerprint IS NULL)
        OR
        (envelope_version = 1 AND client_version IS NOT NULL AND collection_started_at IS NOT NULL
            AND collection_finished_at IS NOT NULL AND request_fingerprint IS NOT NULL
            AND ((result IN ('accepted', 'accepted_with_errors') AND content_reused IS NOT NULL)
                 OR (result = 'rejected' AND content_reused IS NULL)))
    )
);

INSERT INTO discovery_reports_new (
    id, report_id, agent_id, protocol_version, client_version, received_at,
    client_collected_at, collection_started_at, collection_finished_at,
    snapshot_content_id, result, content_reused, request_fingerprint,
    envelope_version, collection_error_summary
)
SELECT id, report_id, agent_id, protocol_version, NULL, received_at,
    client_collected_at, NULL, NULL, snapshot_content_id, result, NULL, NULL, 0,
    collection_error_summary
FROM discovery_reports;

DROP TABLE discovery_reports;
ALTER TABLE discovery_reports_new RENAME TO discovery_reports;

CREATE INDEX discovery_reports_agent_timeline_idx ON discovery_reports (agent_id, received_at DESC, id DESC);
CREATE INDEX discovery_reports_retention_idx ON discovery_reports (received_at, id);
CREATE INDEX discovery_reports_snapshot_idx ON discovery_reports (snapshot_content_id);

CREATE TABLE discovery_report_measurements (
    report_id TEXT PRIMARY KEY REFERENCES discovery_reports(id) ON DELETE CASCADE,
    host_uptime_seconds INTEGER CHECK (host_uptime_seconds IS NULL OR host_uptime_seconds >= 0)
) WITHOUT ROWID;

CREATE TABLE discovery_report_filesystem_measurements (
    report_id TEXT NOT NULL REFERENCES discovery_reports(id) ON DELETE CASCADE,
    device TEXT NOT NULL CHECK (length(trim(device)) BETWEEN 1 AND 4096),
    mount_point TEXT NOT NULL CHECK (length(trim(mount_point)) BETWEEN 1 AND 4096),
    available_bytes INTEGER NOT NULL CHECK (available_bytes >= 0),
    PRIMARY KEY (report_id, device, mount_point)
) WITHOUT ROWID;

CREATE INDEX discovery_report_filesystem_measurements_key_idx
    ON discovery_report_filesystem_measurements (device, mount_point, report_id);

-- +goose StatementBegin
CREATE TRIGGER discovery_reports_no_update
BEFORE UPDATE ON discovery_reports
BEGIN
    SELECT RAISE(ABORT, 'discovery report envelopes are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER discovery_collector_results_no_update
BEFORE UPDATE ON discovery_collector_results
BEGIN
    SELECT RAISE(ABORT, 'discovery collector results are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER discovery_report_measurements_no_update
BEFORE UPDATE ON discovery_report_measurements
BEGIN
    SELECT RAISE(ABORT, 'discovery report measurements are immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER discovery_report_filesystem_measurements_no_update
BEFORE UPDATE ON discovery_report_filesystem_measurements
BEGIN
    SELECT RAISE(ABORT, 'discovery report filesystem measurements are immutable');
END;
-- +goose StatementEnd

COMMIT;
PRAGMA foreign_keys = ON;
PRAGMA user_version = 7;

-- +goose Down
PRAGMA foreign_keys = OFF;
BEGIN IMMEDIATE;

CREATE TABLE _wp02_down_guard (valid INTEGER NOT NULL CHECK (valid = 1));
INSERT INTO _wp02_down_guard (valid)
SELECT CASE WHEN
    EXISTS (SELECT 1 FROM discovery_reports WHERE envelope_version <> 0 OR result = 'accepted_with_errors'
        OR client_version IS NOT NULL OR collection_started_at IS NOT NULL OR collection_finished_at IS NOT NULL
        OR content_reused IS NOT NULL OR request_fingerprint IS NOT NULL)
    OR EXISTS (SELECT 1 FROM discovery_report_measurements)
    OR EXISTS (SELECT 1 FROM discovery_report_filesystem_measurements)
THEN 0 ELSE 1 END;
DROP TABLE _wp02_down_guard;

DROP TRIGGER discovery_reports_no_update;
DROP TRIGGER discovery_collector_results_no_update;
DROP TRIGGER discovery_report_measurements_no_update;
DROP TRIGGER discovery_report_filesystem_measurements_no_update;
DROP TABLE discovery_report_filesystem_measurements;
DROP TABLE discovery_report_measurements;

CREATE TABLE discovery_reports_old (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    report_id TEXT NOT NULL CHECK (length(report_id) = 36 AND report_id = lower(report_id) AND length(replace(report_id, '-', '')) = 32
        AND substr(report_id, 9, 1) = '-' AND substr(report_id, 14, 1) = '-' AND substr(report_id, 19, 1) = '-' AND substr(report_id, 24, 1) = '-'
        AND replace(report_id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE RESTRICT,
    protocol_version INTEGER NOT NULL CHECK (protocol_version >= 1),
    received_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', received_at) = received_at, 0)),
    client_collected_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', client_collected_at) = client_collected_at, 0)),
    snapshot_content_id TEXT REFERENCES snapshot_contents(id) ON DELETE RESTRICT,
    result TEXT NOT NULL CHECK (result IN ('accepted', 'rejected')),
    collection_error_summary TEXT,
    UNIQUE (agent_id, report_id),
    CHECK ((result = 'accepted' AND snapshot_content_id IS NOT NULL) OR (result = 'rejected' AND snapshot_content_id IS NULL))
);

INSERT INTO discovery_reports_old (
    id, report_id, agent_id, protocol_version, received_at, client_collected_at,
    snapshot_content_id, result, collection_error_summary
)
SELECT id, report_id, agent_id, protocol_version, received_at, client_collected_at,
    snapshot_content_id, result, collection_error_summary
FROM discovery_reports;

DROP TABLE discovery_reports;
ALTER TABLE discovery_reports_old RENAME TO discovery_reports;
CREATE INDEX discovery_reports_agent_timeline_idx ON discovery_reports (agent_id, received_at DESC, id DESC);
CREATE INDEX discovery_reports_retention_idx ON discovery_reports (received_at, id);
CREATE INDEX discovery_reports_snapshot_idx ON discovery_reports (snapshot_content_id);

COMMIT;
PRAGMA foreign_keys = ON;
PRAGMA user_version = 6;
