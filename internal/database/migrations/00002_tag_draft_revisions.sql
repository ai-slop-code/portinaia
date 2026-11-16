-- +goose Up
ALTER TABLE dns_drafts
    ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1);

CREATE TABLE _wp02_tags_source AS SELECT id, normalized_name, display_name, created_at FROM tags;
CREATE TABLE _wp02_provider_tags_backup AS SELECT provider_id, tag_id FROM provider_tags;
CREATE TABLE _wp02_location_tags_backup AS SELECT location_id, tag_id FROM location_tags;
CREATE TABLE _wp02_compute_tags_backup AS SELECT compute_id, tag_id FROM compute_tags;
CREATE TABLE _wp02_container_tags_backup AS SELECT container_id, tag_id FROM container_tags;
CREATE TABLE _wp02_domain_tags_backup AS SELECT domain_id, tag_id FROM domain_tags;

DROP TABLE provider_tags;
DROP TABLE location_tags;
DROP TABLE compute_tags;
DROP TABLE container_tags;
DROP TABLE domain_tags;
DROP TABLE tags;

CREATE TABLE tags (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    normalized_name TEXT NOT NULL CHECK (normalized_name = lower(normalized_name) AND length(trim(normalized_name)) BETWEEN 1 AND 100),
    display_name TEXT NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 100),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    archived_at TEXT CHECK (archived_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', archived_at) = archived_at, 0)),
    UNIQUE (normalized_name),
    CHECK (updated_at >= created_at)
);

INSERT INTO tags (id, normalized_name, display_name, revision, created_at, updated_at, archived_at)
SELECT id, normalized_name, display_name, 1, created_at, created_at, NULL FROM _wp02_tags_source;

CREATE TABLE provider_tags (provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (provider_id, tag_id)) WITHOUT ROWID;
CREATE TABLE location_tags (location_id TEXT NOT NULL REFERENCES locations(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (location_id, tag_id)) WITHOUT ROWID;
CREATE TABLE compute_tags (compute_id TEXT NOT NULL REFERENCES computes(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (compute_id, tag_id)) WITHOUT ROWID;
CREATE TABLE container_tags (container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (container_id, tag_id)) WITHOUT ROWID;
CREATE TABLE domain_tags (domain_id TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (domain_id, tag_id)) WITHOUT ROWID;

INSERT INTO provider_tags SELECT provider_id, tag_id FROM _wp02_provider_tags_backup;
INSERT INTO location_tags SELECT location_id, tag_id FROM _wp02_location_tags_backup;
INSERT INTO compute_tags SELECT compute_id, tag_id FROM _wp02_compute_tags_backup;
INSERT INTO container_tags SELECT container_id, tag_id FROM _wp02_container_tags_backup;
INSERT INTO domain_tags SELECT domain_id, tag_id FROM _wp02_domain_tags_backup;

DROP TABLE _wp02_provider_tags_backup;
DROP TABLE _wp02_location_tags_backup;
DROP TABLE _wp02_compute_tags_backup;
DROP TABLE _wp02_container_tags_backup;
DROP TABLE _wp02_domain_tags_backup;
DROP TABLE _wp02_tags_source;

CREATE INDEX provider_tags_tag_idx ON provider_tags (tag_id, provider_id);
CREATE INDEX location_tags_tag_idx ON location_tags (tag_id, location_id);
CREATE INDEX compute_tags_tag_idx ON compute_tags (tag_id, compute_id);
CREATE INDEX container_tags_tag_idx ON container_tags (tag_id, container_id);
CREATE INDEX domain_tags_tag_idx ON domain_tags (tag_id, domain_id);

PRAGMA user_version = 2;

-- +goose Down
CREATE TABLE _wp02_down_guard (valid INTEGER NOT NULL CHECK (valid = 1));
INSERT INTO _wp02_down_guard (valid)
SELECT CASE WHEN
    EXISTS (SELECT 1 FROM tags WHERE revision <> 1 OR archived_at IS NOT NULL OR updated_at <> created_at
        OR length(normalized_name) > 64 OR length(display_name) > 64)
    OR EXISTS (SELECT 1 FROM dns_drafts WHERE revision <> 1)
THEN 0 ELSE 1 END;
DROP TABLE _wp02_down_guard;

ALTER TABLE dns_drafts DROP COLUMN revision;

CREATE TABLE _wp02_tags_source AS SELECT id, normalized_name, display_name, created_at FROM tags;
CREATE TABLE _wp02_provider_tags_backup AS SELECT provider_id, tag_id FROM provider_tags;
CREATE TABLE _wp02_location_tags_backup AS SELECT location_id, tag_id FROM location_tags;
CREATE TABLE _wp02_compute_tags_backup AS SELECT compute_id, tag_id FROM compute_tags;
CREATE TABLE _wp02_container_tags_backup AS SELECT container_id, tag_id FROM container_tags;
CREATE TABLE _wp02_domain_tags_backup AS SELECT domain_id, tag_id FROM domain_tags;

DROP TABLE provider_tags;
DROP TABLE location_tags;
DROP TABLE compute_tags;
DROP TABLE container_tags;
DROP TABLE domain_tags;
DROP TABLE tags;

CREATE TABLE tags (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    normalized_name TEXT NOT NULL CHECK (normalized_name = lower(normalized_name) AND length(trim(normalized_name)) BETWEEN 1 AND 64),
    display_name TEXT NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 64),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    UNIQUE (normalized_name)
);

INSERT INTO tags (id, normalized_name, display_name, created_at)
SELECT id, normalized_name, display_name, created_at FROM _wp02_tags_source;

CREATE TABLE provider_tags (provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (provider_id, tag_id)) WITHOUT ROWID;
CREATE TABLE location_tags (location_id TEXT NOT NULL REFERENCES locations(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (location_id, tag_id)) WITHOUT ROWID;
CREATE TABLE compute_tags (compute_id TEXT NOT NULL REFERENCES computes(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (compute_id, tag_id)) WITHOUT ROWID;
CREATE TABLE container_tags (container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (container_id, tag_id)) WITHOUT ROWID;
CREATE TABLE domain_tags (domain_id TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (domain_id, tag_id)) WITHOUT ROWID;

INSERT INTO provider_tags SELECT provider_id, tag_id FROM _wp02_provider_tags_backup;
INSERT INTO location_tags SELECT location_id, tag_id FROM _wp02_location_tags_backup;
INSERT INTO compute_tags SELECT compute_id, tag_id FROM _wp02_compute_tags_backup;
INSERT INTO container_tags SELECT container_id, tag_id FROM _wp02_container_tags_backup;
INSERT INTO domain_tags SELECT domain_id, tag_id FROM _wp02_domain_tags_backup;

DROP TABLE _wp02_provider_tags_backup;
DROP TABLE _wp02_location_tags_backup;
DROP TABLE _wp02_compute_tags_backup;
DROP TABLE _wp02_container_tags_backup;
DROP TABLE _wp02_domain_tags_backup;
DROP TABLE _wp02_tags_source;

CREATE INDEX provider_tags_tag_idx ON provider_tags (tag_id, provider_id);
CREATE INDEX location_tags_tag_idx ON location_tags (tag_id, location_id);
CREATE INDEX compute_tags_tag_idx ON compute_tags (tag_id, compute_id);
CREATE INDEX container_tags_tag_idx ON container_tags (tag_id, container_id);
CREATE INDEX domain_tags_tag_idx ON domain_tags (tag_id, domain_id);

PRAGMA user_version = 1;
