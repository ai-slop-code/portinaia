-- +goose Up
CREATE TABLE dns_zone_publication_state (
    zone_id TEXT PRIMARY KEY REFERENCES dns_zones(id) ON DELETE CASCADE,
    desired_generation INTEGER NOT NULL CHECK (desired_generation >= 0),
    published_generation INTEGER CHECK (published_generation IS NULL OR (published_generation >= 0 AND published_generation <= desired_generation)),
    publication_state TEXT NOT NULL CHECK (publication_state IN ('not_applicable', 'pending', 'synchronized', 'failed')),
    next_serial INTEGER NOT NULL CHECK (next_serial BETWEEN 1 AND 4294967295),
    published_serial INTEGER CHECK (published_serial IS NULL OR published_serial BETWEEN 1 AND 4294967295),
    published_checksum TEXT CHECK (published_checksum IS NULL OR (length(published_checksum) = 64 AND published_checksum = lower(published_checksum) AND published_checksum NOT GLOB '*[^0-9a-f]*')),
    published_file_name TEXT CHECK (published_file_name IS NULL OR (length(published_file_name) > 0 AND published_file_name NOT LIKE '%/%' AND published_file_name NOT LIKE '%\\%')),
    published_at TEXT CHECK (published_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', published_at) = published_at, 0)),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_retry_at TEXT CHECK (next_retry_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', next_retry_at) = next_retry_at, 0)),
    last_attempt_at TEXT CHECK (last_attempt_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_attempt_at) = last_attempt_at, 0)),
    last_success_at TEXT CHECK (last_success_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_success_at) = last_success_at, 0)),
    last_error_code TEXT CHECK (last_error_code IS NULL OR (length(last_error_code) BETWEEN 1 AND 100 AND last_error_code NOT GLOB '*[^a-z0-9_]*')),
    last_error_message TEXT CHECK (last_error_message IS NULL OR length(last_error_message) <= 2000),
    lease_owner TEXT CHECK (lease_owner IS NULL OR length(lease_owner) > 0),
    lease_expires_at TEXT CHECK (lease_expires_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', lease_expires_at) = lease_expires_at, 0)),
    leased_generation INTEGER CHECK (leased_generation IS NULL OR leased_generation BETWEEN 1 AND desired_generation),
    fencing_token INTEGER NOT NULL DEFAULT 0 CHECK (fencing_token >= 0),
    lease_fencing_token INTEGER CHECK (lease_fencing_token IS NULL OR (lease_fencing_token BETWEEN 1 AND fencing_token)),
    CHECK (
        (published_serial IS NULL AND published_checksum IS NULL AND published_file_name IS NULL AND published_at IS NULL)
        OR (published_serial IS NOT NULL AND published_checksum IS NOT NULL AND published_file_name IS NOT NULL AND published_at IS NOT NULL)
    ),
    CHECK (published_generation IS NULL OR published_generation <> 0 OR published_serial IS NULL),
    CHECK (publication_state <> 'not_applicable' OR desired_generation = 0),
    CHECK (
        publication_state <> 'synchronized'
        OR (desired_generation > 0 AND published_generation = desired_generation AND published_serial IS NOT NULL)
    ),
    CHECK ((last_error_code IS NULL AND last_error_message IS NULL) OR (last_error_code IS NOT NULL AND last_error_message IS NOT NULL)),
    CHECK (
        (lease_owner IS NULL AND lease_expires_at IS NULL AND leased_generation IS NULL AND lease_fencing_token IS NULL)
        OR (lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL AND leased_generation IS NOT NULL AND lease_fencing_token IS NOT NULL)
    )
) WITHOUT ROWID;

-- Legacy per-name desired_revision is not a zone generation. Initialize a new
-- generation independently and retain old revision history in its own column.
CREATE TABLE dns_publications_new (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    zone_id TEXT NOT NULL REFERENCES dns_zones(id) ON DELETE RESTRICT,
    zone_generation INTEGER CHECK (zone_generation IS NULL OR zone_generation >= 1),
    legacy_desired_revision INTEGER CHECK (legacy_desired_revision IS NULL OR legacy_desired_revision >= 1),
    serial INTEGER CHECK (serial IS NULL OR serial BETWEEN 1 AND 4294967295),
    checksum TEXT CHECK (checksum IS NULL OR (length(checksum) = 64 AND checksum = lower(checksum) AND checksum NOT GLOB '*[^0-9a-f]*')),
    file_name TEXT CHECK (file_name IS NULL OR (length(file_name) > 0 AND file_name NOT LIKE '%/%' AND file_name NOT LIKE '%\\%')),
    fencing_token INTEGER CHECK (fencing_token IS NULL OR fencing_token >= 1),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    result TEXT NOT NULL CHECK (result IN ('success', 'failure')),
    sanitized_error TEXT,
    CHECK ((result = 'success' AND serial IS NOT NULL AND checksum IS NOT NULL AND file_name IS NOT NULL AND sanitized_error IS NULL)
        OR (result = 'failure' AND sanitized_error IS NOT NULL)),
    CHECK ((zone_generation IS NULL AND fencing_token IS NULL) OR (zone_generation IS NOT NULL AND fencing_token IS NOT NULL)),
    CHECK (zone_generation IS NULL OR legacy_desired_revision IS NULL)
);

INSERT INTO dns_publications_new (
    id, zone_id, zone_generation, legacy_desired_revision, serial, checksum,
    file_name, fencing_token, created_at, result, sanitized_error
)
SELECT id, zone_id, NULL, desired_revision, serial, checksum, file_name, NULL,
    created_at, result, sanitized_error
FROM dns_publications;

DROP TABLE dns_publications;
ALTER TABLE dns_publications_new RENAME TO dns_publications;
CREATE INDEX dns_publications_zone_idx ON dns_publications (zone_id, created_at DESC, id DESC);
CREATE INDEX dns_publications_generation_idx ON dns_publications (zone_id, zone_generation, result);
CREATE INDEX dns_publications_legacy_revision_idx ON dns_publications (zone_id, legacy_desired_revision, result);

INSERT INTO dns_zone_publication_state (
    zone_id, desired_generation, published_generation, publication_state,
    next_serial, published_serial, published_checksum, published_file_name,
    published_at, attempt_count, last_attempt_at, last_success_at,
    last_error_code, last_error_message, fencing_token
)
SELECT
    zone.id,
    CASE WHEN EXISTS (SELECT 1 FROM dns_publications publication WHERE publication.zone_id = zone.id)
              OR EXISTS (
                  SELECT 1 FROM dns_record_sets record_set
                  JOIN dns_names name ON name.id = record_set.dns_name_id
                  WHERE name.zone_id = zone.id AND name.archived_at IS NULL
                    AND name.lifecycle_state = 'active' AND record_set.view = 'internal'
              )
         THEN 1 ELSE 0 END,
    CASE WHEN EXISTS (SELECT 1 FROM dns_publications publication WHERE publication.zone_id = zone.id AND publication.result = 'success')
         THEN NULL ELSE 0 END,
    CASE WHEN EXISTS (SELECT 1 FROM dns_publications publication WHERE publication.zone_id = zone.id)
              OR EXISTS (
                  SELECT 1 FROM dns_record_sets record_set
                  JOIN dns_names name ON name.id = record_set.dns_name_id
                  WHERE name.zone_id = zone.id AND name.archived_at IS NULL
                    AND name.lifecycle_state = 'active' AND record_set.view = 'internal'
              )
         THEN 'pending' ELSE 'not_applicable' END,
    COALESCE((
        SELECT CASE WHEN publication.serial = 4294967295 THEN 1 ELSE publication.serial + 1 END
        FROM dns_publications publication WHERE publication.zone_id = zone.id
        ORDER BY publication.created_at DESC, publication.id DESC LIMIT 1
    ), 1),
    (SELECT publication.serial FROM dns_publications publication WHERE publication.zone_id = zone.id AND publication.result = 'success'
        ORDER BY publication.created_at DESC, publication.id DESC LIMIT 1),
    (SELECT publication.checksum FROM dns_publications publication WHERE publication.zone_id = zone.id AND publication.result = 'success'
        ORDER BY publication.created_at DESC, publication.id DESC LIMIT 1),
    (SELECT publication.file_name FROM dns_publications publication WHERE publication.zone_id = zone.id AND publication.result = 'success'
        ORDER BY publication.created_at DESC, publication.id DESC LIMIT 1),
    (SELECT publication.created_at FROM dns_publications publication WHERE publication.zone_id = zone.id AND publication.result = 'success'
        ORDER BY publication.created_at DESC, publication.id DESC LIMIT 1),
    (SELECT count(*) FROM dns_publications publication WHERE publication.zone_id = zone.id),
    (SELECT publication.created_at FROM dns_publications publication WHERE publication.zone_id = zone.id
        ORDER BY publication.created_at DESC, publication.id DESC LIMIT 1),
    (SELECT publication.created_at FROM dns_publications publication WHERE publication.zone_id = zone.id AND publication.result = 'success'
        ORDER BY publication.created_at DESC, publication.id DESC LIMIT 1),
    CASE WHEN (SELECT publication.result FROM dns_publications publication WHERE publication.zone_id = zone.id
                   ORDER BY publication.created_at DESC, publication.id DESC LIMIT 1) = 'failure'
         THEN 'publication_failed' ELSE NULL END,
    (SELECT publication.sanitized_error FROM dns_publications publication WHERE publication.zone_id = zone.id AND publication.result = 'failure'
        ORDER BY publication.created_at DESC, publication.id DESC LIMIT 1),
    0
FROM dns_zones zone;

PRAGMA user_version = 8;

-- +goose Down
CREATE TABLE _wp02_down_guard (valid INTEGER NOT NULL CHECK (valid = 1));
INSERT INTO _wp02_down_guard (valid)
SELECT CASE WHEN
    EXISTS (SELECT 1 FROM dns_zone_publication_state)
    OR EXISTS (SELECT 1 FROM dns_publications WHERE zone_generation IS NOT NULL OR serial IS NULL OR checksum IS NULL OR file_name IS NULL OR fencing_token IS NOT NULL)
THEN 0 ELSE 1 END;
DROP TABLE _wp02_down_guard;

DROP TABLE dns_zone_publication_state;
CREATE TABLE dns_publications_old (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    zone_id TEXT NOT NULL REFERENCES dns_zones(id) ON DELETE RESTRICT,
    desired_revision INTEGER NOT NULL CHECK (desired_revision >= 1),
    serial INTEGER NOT NULL CHECK (serial BETWEEN 1 AND 4294967295),
    checksum TEXT NOT NULL CHECK (length(checksum) = 64 AND checksum = lower(checksum) AND checksum NOT GLOB '*[^0-9a-f]*'),
    file_name TEXT NOT NULL CHECK (length(file_name) > 0 AND file_name NOT LIKE '%/%' AND file_name NOT LIKE '%\\%'),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    result TEXT NOT NULL CHECK (result IN ('success', 'failure')),
    sanitized_error TEXT,
    CHECK ((result = 'success' AND sanitized_error IS NULL) OR (result = 'failure' AND sanitized_error IS NOT NULL))
);
INSERT INTO dns_publications_old (id, zone_id, desired_revision, serial, checksum, file_name, created_at, result, sanitized_error)
SELECT id, zone_id, legacy_desired_revision, serial, checksum, file_name, created_at, result, sanitized_error
FROM dns_publications;
DROP TABLE dns_publications;
ALTER TABLE dns_publications_old RENAME TO dns_publications;
CREATE INDEX dns_publications_zone_idx ON dns_publications (zone_id, created_at DESC, id DESC);
CREATE INDEX dns_publications_revision_idx ON dns_publications (zone_id, desired_revision, result);
PRAGMA user_version = 7;
