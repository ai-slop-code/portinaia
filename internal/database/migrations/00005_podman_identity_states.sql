-- +goose NO TRANSACTION
-- +goose Up
PRAGMA foreign_keys = OFF;
BEGIN IMMEDIATE;

CREATE TABLE _wp02_owner_uid_guard (valid INTEGER NOT NULL CHECK (valid = 1));
INSERT INTO _wp02_owner_uid_guard (valid)
SELECT CASE WHEN EXISTS (
    SELECT 1 FROM containers
    WHERE podman_owner <> 'root'
      AND (length(podman_owner) NOT BETWEEN 1 AND 10
           OR podman_owner GLOB '*[^0-9]*'
           OR CAST(podman_owner AS INTEGER) NOT BETWEEN 0 AND 4294967295
           OR CAST(CAST(podman_owner AS INTEGER) AS TEXT) <> podman_owner)
) THEN 0 ELSE 1 END;
DROP TABLE _wp02_owner_uid_guard;

CREATE TABLE containers_new (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    compute_id TEXT NOT NULL REFERENCES computes(id) ON DELETE RESTRICT,
    podman_owner_uid INTEGER NOT NULL CHECK (podman_owner_uid BETWEEN 0 AND 4294967295),
    podman_connection TEXT NOT NULL CHECK (length(podman_connection) > 0),
    name TEXT NOT NULL CHECK (length(name) > 0),
    runtime_id TEXT NOT NULL CHECK (length(runtime_id) > 0),
    image_name TEXT NOT NULL DEFAULT '',
    image_id TEXT NOT NULL DEFAULT '',
    image_digest TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL CHECK (state IN ('created', 'running', 'paused', 'exited', 'stopped', 'unknown')),
    health TEXT NOT NULL CHECK (health IN ('none', 'starting', 'healthy', 'unhealthy', 'unknown')),
    restart_policy TEXT NOT NULL CHECK (restart_policy IN ('no', 'always', 'on-failure', 'unless-stopped', 'unknown')),
    restart_max_attempts INTEGER CHECK (restart_max_attempts IS NULL OR restart_max_attempts >= 0),
    runtime_created_at TEXT CHECK (runtime_created_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', runtime_created_at) = runtime_created_at, 0)),
    runtime_started_at TEXT CHECK (runtime_started_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', runtime_started_at) = runtime_started_at, 0)),
    first_observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', first_observed_at) = first_observed_at, 0)),
    last_observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_observed_at) = last_observed_at, 0)),
    stale_at TEXT CHECK (stale_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', stale_at) = stale_at, 0)),
    description TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    archived_at TEXT CHECK (archived_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', archived_at) = archived_at, 0)),
    CHECK (restart_policy = 'on-failure' OR restart_max_attempts IS NULL),
    CHECK (last_observed_at >= first_observed_at),
    CHECK (updated_at >= created_at),
    CHECK (archived_at IS NULL OR archived_at >= created_at)
);

INSERT INTO containers_new (
    id, compute_id, podman_owner_uid, podman_connection, name, runtime_id,
    image_name, image_id, image_digest, state, health, restart_policy,
    restart_max_attempts, runtime_created_at, runtime_started_at,
    first_observed_at, last_observed_at, stale_at, description, notes,
    revision, created_at, updated_at, archived_at
)
SELECT id, compute_id,
    CASE WHEN podman_owner = 'root' THEN 0 ELSE CAST(podman_owner AS INTEGER) END,
    podman_connection, name, runtime_id, image_name, image_id, image_digest,
    state, health, restart_policy, restart_max_attempts, runtime_created_at,
    runtime_started_at, first_observed_at, last_observed_at, stale_at,
    description, notes, revision, created_at, updated_at, archived_at
FROM containers;

DROP TABLE containers;
ALTER TABLE containers_new RENAME TO containers;
CREATE UNIQUE INDEX containers_active_identity_idx ON containers (compute_id, podman_owner_uid, name) WHERE archived_at IS NULL;
CREATE INDEX containers_list_idx ON containers (compute_id, archived_at, name, id);
CREATE INDEX containers_stale_idx ON containers (stale_at, last_observed_at) WHERE archived_at IS NULL;

CREATE TABLE container_addresses_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    network_id INTEGER NOT NULL,
    address TEXT NOT NULL CHECK (length(address) > 0),
    family INTEGER NOT NULL CHECK (family IN (4, 6)),
    prefix_length INTEGER CHECK (prefix_length IS NULL OR (family = 4 AND prefix_length BETWEEN 0 AND 32) OR (family = 6 AND prefix_length BETWEEN 0 AND 128)),
    UNIQUE (container_id, network_id, address),
    FOREIGN KEY (network_id, container_id) REFERENCES container_networks(id, container_id) ON DELETE CASCADE
);
INSERT INTO container_addresses_new (id, container_id, network_id, address, family, prefix_length)
SELECT id, container_id, network_id, address, family, prefix_length FROM container_addresses;
DROP TABLE container_addresses;
ALTER TABLE container_addresses_new RENAME TO container_addresses;
CREATE INDEX container_addresses_container_idx ON container_addresses (container_id, network_id, address);

CREATE TABLE container_mounts_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('bind', 'volume', 'tmpfs', 'other')),
    source TEXT NOT NULL DEFAULT '',
    destination TEXT NOT NULL CHECK (length(destination) > 0),
    read_only INTEGER NOT NULL CHECK (read_only IN (0, 1)),
    options TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(options) AND json_type(options) = 'array'),
    UNIQUE (container_id, destination)
);
INSERT INTO container_mounts_new (id, container_id, type, source, destination, read_only, options)
SELECT id, container_id, type, source, destination, read_only, options FROM container_mounts;
DROP TABLE container_mounts;
ALTER TABLE container_mounts_new RENAME TO container_mounts;
CREATE INDEX container_mounts_container_idx ON container_mounts (container_id, destination);

COMMIT;
PRAGMA foreign_keys = ON;
PRAGMA user_version = 5;

-- +goose Down
PRAGMA foreign_keys = OFF;
BEGIN IMMEDIATE;

CREATE TABLE _wp02_container_down_guard (valid INTEGER NOT NULL CHECK (valid = 1));
INSERT INTO _wp02_container_down_guard (valid)
SELECT CASE WHEN
    EXISTS (SELECT 1 FROM containers WHERE restart_policy = 'unknown')
    OR EXISTS (SELECT 1 FROM container_mounts WHERE type = 'other')
    OR EXISTS (SELECT 1 FROM container_addresses WHERE prefix_length IS NULL)
THEN 0 ELSE 1 END;
DROP TABLE _wp02_container_down_guard;

CREATE TABLE containers_old (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    compute_id TEXT NOT NULL REFERENCES computes(id) ON DELETE RESTRICT,
    podman_owner TEXT NOT NULL CHECK (length(podman_owner) > 0),
    podman_connection TEXT NOT NULL CHECK (length(podman_connection) > 0),
    name TEXT NOT NULL CHECK (length(name) > 0),
    runtime_id TEXT NOT NULL CHECK (length(runtime_id) > 0),
    image_name TEXT NOT NULL DEFAULT '',
    image_id TEXT NOT NULL DEFAULT '',
    image_digest TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL CHECK (state IN ('created', 'running', 'paused', 'exited', 'stopped', 'unknown')),
    health TEXT NOT NULL CHECK (health IN ('none', 'starting', 'healthy', 'unhealthy', 'unknown')),
    restart_policy TEXT NOT NULL CHECK (restart_policy IN ('no', 'always', 'on-failure', 'unless-stopped')),
    restart_max_attempts INTEGER CHECK (restart_max_attempts IS NULL OR restart_max_attempts >= 0),
    runtime_created_at TEXT CHECK (runtime_created_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', runtime_created_at) = runtime_created_at, 0)),
    runtime_started_at TEXT CHECK (runtime_started_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', runtime_started_at) = runtime_started_at, 0)),
    first_observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', first_observed_at) = first_observed_at, 0)),
    last_observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_observed_at) = last_observed_at, 0)),
    stale_at TEXT CHECK (stale_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', stale_at) = stale_at, 0)),
    description TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    archived_at TEXT CHECK (archived_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', archived_at) = archived_at, 0)),
    CHECK (restart_policy = 'on-failure' OR restart_max_attempts IS NULL),
    CHECK (last_observed_at >= first_observed_at),
    CHECK (updated_at >= created_at),
    CHECK (archived_at IS NULL OR archived_at >= created_at)
);
INSERT INTO containers_old (
    id, compute_id, podman_owner, podman_connection, name, runtime_id,
    image_name, image_id, image_digest, state, health, restart_policy,
    restart_max_attempts, runtime_created_at, runtime_started_at,
    first_observed_at, last_observed_at, stale_at, description, notes,
    revision, created_at, updated_at, archived_at
)
SELECT id, compute_id, CASE WHEN podman_owner_uid = 0 THEN 'root' ELSE CAST(podman_owner_uid AS TEXT) END,
    podman_connection, name, runtime_id, image_name, image_id, image_digest,
    state, health, restart_policy, restart_max_attempts, runtime_created_at,
    runtime_started_at, first_observed_at, last_observed_at, stale_at,
    description, notes, revision, created_at, updated_at, archived_at
FROM containers;

DROP TABLE containers;
ALTER TABLE containers_old RENAME TO containers;
CREATE UNIQUE INDEX containers_active_identity_idx ON containers (compute_id, podman_owner, name) WHERE archived_at IS NULL;
CREATE INDEX containers_list_idx ON containers (compute_id, archived_at, name, id);
CREATE INDEX containers_stale_idx ON containers (stale_at, last_observed_at) WHERE archived_at IS NULL;

CREATE TABLE container_addresses_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    network_id INTEGER NOT NULL,
    address TEXT NOT NULL CHECK (length(address) > 0),
    family INTEGER NOT NULL CHECK (family IN (4, 6)),
    prefix_length INTEGER NOT NULL CHECK ((family = 4 AND prefix_length BETWEEN 0 AND 32) OR (family = 6 AND prefix_length BETWEEN 0 AND 128)),
    UNIQUE (container_id, network_id, address),
    FOREIGN KEY (network_id, container_id) REFERENCES container_networks(id, container_id) ON DELETE CASCADE
);
INSERT INTO container_addresses_old (id, container_id, network_id, address, family, prefix_length)
SELECT id, container_id, network_id, address, family, prefix_length FROM container_addresses;
DROP TABLE container_addresses;
ALTER TABLE container_addresses_old RENAME TO container_addresses;
CREATE INDEX container_addresses_container_idx ON container_addresses (container_id, network_id, address);

CREATE TABLE container_mounts_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('bind', 'volume', 'tmpfs')),
    source TEXT NOT NULL DEFAULT '',
    destination TEXT NOT NULL CHECK (length(destination) > 0),
    read_only INTEGER NOT NULL CHECK (read_only IN (0, 1)),
    options TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(options) AND json_type(options) = 'array'),
    UNIQUE (container_id, destination)
);
INSERT INTO container_mounts_old (id, container_id, type, source, destination, read_only, options)
SELECT id, container_id, type, source, destination, read_only, options FROM container_mounts;
DROP TABLE container_mounts;
ALTER TABLE container_mounts_old RENAME TO container_mounts;
CREATE INDEX container_mounts_container_idx ON container_mounts (container_id, destination);

COMMIT;
PRAGMA foreign_keys = ON;
PRAGMA user_version = 4;
