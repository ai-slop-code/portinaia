-- +goose Up
PRAGMA foreign_keys = ON;

CREATE TABLE administrators (
    id TEXT PRIMARY KEY
        CHECK (length(id) = 36 AND id = lower(id)
            AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-'
            AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
            AND length(replace(id, '-', '')) = 32
            AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    username TEXT NOT NULL COLLATE NOCASE CHECK (length(trim(username)) BETWEEN 1 AND 128),
    password_hash TEXT NOT NULL CHECK (length(password_hash) > 0),
    password_algorithm TEXT NOT NULL CHECK (password_algorithm = 'argon2id'),
    password_parameters TEXT NOT NULL CHECK (json_valid(password_parameters)),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    disabled_at TEXT CHECK (disabled_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', disabled_at) = disabled_at, 0)),
    UNIQUE (username),
    CHECK (updated_at >= created_at),
    CHECK (disabled_at IS NULL OR disabled_at >= created_at)
);

CREATE UNIQUE INDEX administrators_one_enabled_idx ON administrators ((1)) WHERE disabled_at IS NULL;

CREATE TABLE sessions (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    administrator_id TEXT NOT NULL REFERENCES administrators(id) ON DELETE CASCADE,
    credential_hash TEXT NOT NULL UNIQUE CHECK (length(credential_hash) >= 32),
    csrf_hash TEXT NOT NULL CHECK (length(csrf_hash) >= 32),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    expires_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', expires_at) = expires_at, 0)),
    last_seen_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_seen_at) = last_seen_at, 0)),
    revoked_at TEXT CHECK (revoked_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', revoked_at) = revoked_at, 0)),
    CHECK (expires_at > created_at),
    CHECK (last_seen_at >= created_at),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX sessions_administrator_idx ON sessions (administrator_id, revoked_at, expires_at);
CREATE INDEX sessions_expiry_idx ON sessions (expires_at) WHERE revoked_at IS NULL;

CREATE TABLE personal_access_tokens (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    administrator_id TEXT NOT NULL REFERENCES administrators(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 128),
    token_hash TEXT NOT NULL UNIQUE CHECK (length(token_hash) >= 32),
    scopes TEXT NOT NULL CHECK (json_valid(scopes) AND json_type(scopes) = 'array'),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    expires_at TEXT CHECK (expires_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', expires_at) = expires_at, 0)),
    last_used_at TEXT CHECK (last_used_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_used_at) = last_used_at, 0)),
    revoked_at TEXT CHECK (revoked_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', revoked_at) = revoked_at, 0)),
    UNIQUE (administrator_id, name),
    CHECK (expires_at IS NULL OR expires_at > created_at),
    CHECK (last_used_at IS NULL OR last_used_at >= created_at),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX personal_access_tokens_administrator_idx ON personal_access_tokens (administrator_id, revoked_at, created_at DESC);

CREATE TABLE audit_events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    occurred_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', occurred_at) = occurred_at, 0)),
    actor_type TEXT NOT NULL CHECK (actor_type IN ('administrator', 'personal_access_token', 'agent', 'system')),
    actor_id TEXT NOT NULL CHECK (length(actor_id) > 0),
    action TEXT NOT NULL CHECK (length(action) > 0),
    resource_type TEXT NOT NULL CHECK (length(resource_type) > 0),
    resource_id TEXT CHECK (resource_id IS NULL OR length(resource_id) > 0),
    request_id TEXT CHECK (request_id IS NULL OR length(request_id) > 0),
    before_document TEXT CHECK (before_document IS NULL OR json_valid(before_document)),
    after_document TEXT CHECK (after_document IS NULL OR json_valid(after_document)),
    result TEXT NOT NULL CHECK (result IN ('success', 'rejected', 'failure'))
);

CREATE INDEX audit_events_time_idx ON audit_events (occurred_at DESC, sequence DESC);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_type, actor_id, occurred_at DESC);
CREATE INDEX audit_events_resource_idx ON audit_events (resource_type, resource_id, occurred_at DESC);

CREATE TABLE idempotency_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_type TEXT NOT NULL CHECK (actor_type IN ('administrator', 'personal_access_token')),
    actor_id TEXT NOT NULL CHECK (length(actor_id) > 0),
    scope TEXT NOT NULL CHECK (length(scope) > 0),
    key_hash TEXT NOT NULL CHECK (length(key_hash) >= 32),
    request_fingerprint TEXT NOT NULL CHECK (length(request_fingerprint) >= 32),
    response_status INTEGER CHECK (response_status IS NULL OR response_status BETWEEN 100 AND 599),
    response_headers TEXT CHECK (response_headers IS NULL OR json_valid(response_headers)),
    response_body BLOB,
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    expires_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', expires_at) = expires_at, 0)),
    UNIQUE (actor_type, actor_id, scope, key_hash),
    CHECK (expires_at > created_at),
    CHECK ((response_status IS NULL AND response_headers IS NULL AND response_body IS NULL) OR response_status IS NOT NULL)
);

CREATE INDEX idempotency_keys_expiry_idx ON idempotency_keys (expires_at);

CREATE TABLE providers (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    name TEXT NOT NULL COLLATE NOCASE CHECK (length(trim(name)) BETWEEN 1 AND 200),
    website TEXT CHECK (website IS NULL OR length(website) > 0),
    external_account_reference TEXT CHECK (external_account_reference IS NULL OR length(external_account_reference) > 0),
    description TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    archived_at TEXT CHECK (archived_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', archived_at) = archived_at, 0)),
    UNIQUE (name),
    CHECK (updated_at >= created_at),
    CHECK (archived_at IS NULL OR archived_at >= created_at)
);

CREATE INDEX providers_list_idx ON providers (archived_at, name, id);

CREATE TABLE provider_roles (
    provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('hosting', 'registrar', 'hardware_vendor', 'other')),
    PRIMARY KEY (provider_id, role)
) WITHOUT ROWID;

CREATE TABLE locations (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    type TEXT NOT NULL CHECK (type IN ('site', 'provider_region')),
    name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 200),
    provider_id TEXT REFERENCES providers(id) ON DELETE RESTRICT,
    parent_id TEXT REFERENCES locations(id) ON DELETE RESTRICT,
    description TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    archived_at TEXT CHECK (archived_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', archived_at) = archived_at, 0)),
    CHECK (parent_id IS NULL OR parent_id <> id),
    CHECK (updated_at >= created_at),
    CHECK (archived_at IS NULL OR archived_at >= created_at)
);

CREATE INDEX locations_list_idx ON locations (archived_at, type, name, id);
CREATE INDEX locations_provider_idx ON locations (provider_id, archived_at, name);
CREATE INDEX locations_parent_idx ON locations (parent_id, archived_at, name);

CREATE TABLE computes (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    kind TEXT NOT NULL CHECK (kind IN ('physical', 'vps', 'vm')),
    display_name TEXT NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 200),
    ownership TEXT NOT NULL CHECK (ownership IN ('owned', 'rented', 'other')),
    provider_id TEXT REFERENCES providers(id) ON DELETE RESTRICT,
    provider_resource_id TEXT CHECK (provider_resource_id IS NULL OR length(provider_resource_id) > 0),
    location_id TEXT REFERENCES locations(id) ON DELETE RESTRICT,
    parent_compute_id TEXT REFERENCES computes(id) ON DELETE RESTRICT,
    description TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    archived_at TEXT CHECK (archived_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', archived_at) = archived_at, 0)),
    UNIQUE (provider_id, provider_resource_id),
    CHECK (parent_compute_id IS NULL OR (kind = 'vm' AND parent_compute_id <> id)),
    CHECK (provider_resource_id IS NULL OR provider_id IS NOT NULL),
    CHECK (updated_at >= created_at),
    CHECK (archived_at IS NULL OR archived_at >= created_at)
);

CREATE INDEX computes_list_idx ON computes (archived_at, kind, display_name, id);
CREATE INDEX computes_provider_idx ON computes (provider_id, archived_at, display_name);
CREATE INDEX computes_location_idx ON computes (location_id, archived_at, display_name);
CREATE INDEX computes_parent_idx ON computes (parent_compute_id, archived_at, display_name);

CREATE TABLE containers (
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

CREATE UNIQUE INDEX containers_active_identity_idx ON containers (compute_id, podman_owner, name) WHERE archived_at IS NULL;
CREATE INDEX containers_list_idx ON containers (compute_id, archived_at, name, id);
CREATE INDEX containers_stale_idx ON containers (stale_at, last_observed_at) WHERE archived_at IS NULL;

CREATE TABLE container_ports (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    protocol TEXT NOT NULL CHECK (protocol IN ('tcp', 'udp', 'sctp')),
    container_port INTEGER NOT NULL CHECK (container_port BETWEEN 1 AND 65535),
    host_ip TEXT NOT NULL DEFAULT '',
    host_port INTEGER CHECK (host_port IS NULL OR host_port BETWEEN 1 AND 65535),
    is_published INTEGER NOT NULL CHECK (is_published IN (0, 1)),
    CHECK ((is_published = 0 AND host_port IS NULL) OR (is_published = 1 AND host_port IS NOT NULL)),
    UNIQUE (container_id, protocol, container_port, host_ip, host_port)
);

CREATE INDEX container_ports_container_idx ON container_ports (container_id, container_port, protocol);

CREATE TABLE container_networks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(name) > 0),
    network_id TEXT NOT NULL DEFAULT '',
    driver TEXT NOT NULL DEFAULT '',
    UNIQUE (container_id, name),
    UNIQUE (id, container_id)
);

CREATE INDEX container_networks_container_idx ON container_networks (container_id, name);

CREATE TABLE container_addresses (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    network_id INTEGER NOT NULL,
    address TEXT NOT NULL CHECK (length(address) > 0),
    family INTEGER NOT NULL CHECK (family IN (4, 6)),
    prefix_length INTEGER NOT NULL CHECK ((family = 4 AND prefix_length BETWEEN 0 AND 32) OR (family = 6 AND prefix_length BETWEEN 0 AND 128)),
    UNIQUE (container_id, network_id, address),
    FOREIGN KEY (network_id, container_id) REFERENCES container_networks(id, container_id) ON DELETE CASCADE
);

CREATE INDEX container_addresses_container_idx ON container_addresses (container_id, network_id, address);

CREATE TABLE container_mounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('bind', 'volume', 'tmpfs')),
    source TEXT NOT NULL DEFAULT '',
    destination TEXT NOT NULL CHECK (length(destination) > 0),
    read_only INTEGER NOT NULL CHECK (read_only IN (0, 1)),
    options TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(options) AND json_type(options) = 'array'),
    UNIQUE (container_id, destination)
);

CREATE INDEX container_mounts_container_idx ON container_mounts (container_id, destination);

CREATE TABLE container_labels (
    container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    key TEXT NOT NULL CHECK (length(key) > 0),
    value TEXT NOT NULL,
    PRIMARY KEY (container_id, key)
) WITHOUT ROWID;

CREATE TABLE ip_assignments (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    address TEXT NOT NULL CHECK (length(address) > 0),
    family INTEGER NOT NULL CHECK (family IN (4, 6)),
    prefix_length INTEGER CHECK (prefix_length IS NULL OR (family = 4 AND prefix_length BETWEEN 0 AND 32) OR (family = 6 AND prefix_length BETWEEN 0 AND 128)),
    compute_id TEXT REFERENCES computes(id) ON DELETE RESTRICT,
    container_id TEXT REFERENCES containers(id) ON DELETE RESTRICT,
    interface_name TEXT,
    network_name TEXT,
    scope TEXT NOT NULL CHECK (scope IN ('public', 'private', 'loopback', 'link_local', 'other')),
    role TEXT NOT NULL CHECK (role IN ('primary', 'management', 'service', 'other')),
    source TEXT NOT NULL CHECK (source IN ('manual', 'discovered')),
    is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
    first_observed_at TEXT CHECK (first_observed_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', first_observed_at) = first_observed_at, 0)),
    last_observed_at TEXT CHECK (last_observed_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_observed_at) = last_observed_at, 0)),
    stale_at TEXT CHECK (stale_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', stale_at) = stale_at, 0)),
    description TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    archived_at TEXT CHECK (archived_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', archived_at) = archived_at, 0)),
    CHECK ((compute_id IS NOT NULL) + (container_id IS NOT NULL) = 1),
    CHECK ((source = 'manual' AND first_observed_at IS NULL AND last_observed_at IS NULL AND stale_at IS NULL)
        OR (source = 'discovered' AND first_observed_at IS NOT NULL AND last_observed_at IS NOT NULL)),
    CHECK (last_observed_at IS NULL OR last_observed_at >= first_observed_at),
    CHECK (updated_at >= created_at)
);

CREATE INDEX ip_assignments_compute_idx ON ip_assignments (compute_id, archived_at, address);
CREATE INDEX ip_assignments_container_idx ON ip_assignments (container_id, archived_at, address);
CREATE INDEX ip_assignments_stale_idx ON ip_assignments (stale_at, last_observed_at) WHERE source = 'discovered' AND archived_at IS NULL;

CREATE TABLE domains (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    normalized_name TEXT NOT NULL COLLATE NOCASE CHECK (normalized_name = lower(normalized_name) AND length(normalized_name) BETWEEN 1 AND 253 AND normalized_name NOT LIKE '.%' AND normalized_name NOT LIKE '%.'),
    registrar_provider_id TEXT REFERENCES providers(id) ON DELETE RESTRICT,
    registered_on TEXT CHECK (registered_on IS NULL OR (registered_on GLOB '????-??-??' AND date(registered_on) = registered_on)),
    expires_on TEXT CHECK (expires_on IS NULL OR (expires_on GLOB '????-??-??' AND date(expires_on) = expires_on)),
    auto_renew INTEGER CHECK (auto_renew IS NULL OR auto_renew IN (0, 1)),
    nameservers TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(nameservers) AND json_type(nameservers) = 'array'),
    description TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    archived_at TEXT CHECK (archived_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', archived_at) = archived_at, 0)),
    UNIQUE (normalized_name),
    CHECK (expires_on IS NULL OR registered_on IS NULL OR expires_on >= registered_on),
    CHECK (updated_at >= created_at)
);

CREATE INDEX domains_list_idx ON domains (archived_at, normalized_name, id);
CREATE INDEX domains_expiry_idx ON domains (expires_on, normalized_name) WHERE archived_at IS NULL AND expires_on IS NOT NULL;

CREATE TABLE costs (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    compute_id TEXT REFERENCES computes(id) ON DELETE RESTRICT,
    domain_id TEXT REFERENCES domains(id) ON DELETE RESTRICT,
    type TEXT NOT NULL CHECK (type IN ('recurring', 'one_time')),
    amount_minor INTEGER NOT NULL CHECK (amount_minor >= 0),
    currency_code TEXT NOT NULL CHECK (length(currency_code) = 3 AND currency_code = upper(currency_code) AND currency_code NOT GLOB '*[^A-Z]*'),
    currency_exponent INTEGER NOT NULL CHECK (currency_exponent BETWEEN 0 AND 3),
    billing_interval TEXT CHECK (billing_interval IS NULL OR billing_interval IN ('monthly', 'quarterly', 'semiannual', 'annual')),
    effective_start_date TEXT NOT NULL CHECK (effective_start_date GLOB '????-??-??' AND date(effective_start_date) = effective_start_date),
    effective_end_date TEXT CHECK (effective_end_date IS NULL OR (effective_end_date GLOB '????-??-??' AND date(effective_end_date) = effective_end_date)),
    charge_date TEXT CHECK (charge_date IS NULL OR (charge_date GLOB '????-??-??' AND date(charge_date) = charge_date)),
    description TEXT NOT NULL DEFAULT '',
    provider_id TEXT REFERENCES providers(id) ON DELETE RESTRICT,
    provider_reference TEXT,
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    archived_at TEXT CHECK (archived_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', archived_at) = archived_at, 0)),
    CHECK ((compute_id IS NOT NULL) + (domain_id IS NOT NULL) = 1),
    CHECK ((type = 'recurring' AND billing_interval IS NOT NULL AND charge_date IS NULL)
        OR (type = 'one_time' AND billing_interval IS NULL AND charge_date IS NOT NULL)),
    CHECK (effective_end_date IS NULL OR effective_end_date >= effective_start_date),
    CHECK (updated_at >= created_at)
);

CREATE INDEX costs_compute_idx ON costs (compute_id, archived_at, effective_start_date DESC);
CREATE INDEX costs_domain_idx ON costs (domain_id, archived_at, effective_start_date DESC);
CREATE INDEX costs_recurring_idx ON costs (currency_code, billing_interval, effective_start_date) WHERE type = 'recurring' AND archived_at IS NULL;

CREATE TABLE tags (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    normalized_name TEXT NOT NULL CHECK (normalized_name = lower(normalized_name) AND length(trim(normalized_name)) BETWEEN 1 AND 64),
    display_name TEXT NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 64),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    UNIQUE (normalized_name)
);

CREATE TABLE provider_tags (provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (provider_id, tag_id)) WITHOUT ROWID;
CREATE TABLE location_tags (location_id TEXT NOT NULL REFERENCES locations(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (location_id, tag_id)) WITHOUT ROWID;
CREATE TABLE compute_tags (compute_id TEXT NOT NULL REFERENCES computes(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (compute_id, tag_id)) WITHOUT ROWID;
CREATE TABLE container_tags (container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (container_id, tag_id)) WITHOUT ROWID;
CREATE TABLE domain_tags (domain_id TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE, tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE, PRIMARY KEY (domain_id, tag_id)) WITHOUT ROWID;

CREATE INDEX provider_tags_tag_idx ON provider_tags (tag_id, provider_id);
CREATE INDEX location_tags_tag_idx ON location_tags (tag_id, location_id);
CREATE INDEX compute_tags_tag_idx ON compute_tags (tag_id, compute_id);
CREATE INDEX container_tags_tag_idx ON container_tags (tag_id, container_id);
CREATE INDEX domain_tags_tag_idx ON domain_tags (tag_id, domain_id);

CREATE TABLE agents (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    compute_id TEXT NOT NULL UNIQUE REFERENCES computes(id) ON DELETE RESTRICT,
    credential_hash TEXT NOT NULL UNIQUE CHECK (length(credential_hash) >= 32),
    credential_generation INTEGER NOT NULL CHECK (credential_generation >= 1),
    protocol_version INTEGER NOT NULL CHECK (protocol_version >= 1),
    client_version TEXT NOT NULL CHECK (length(client_version) > 0),
    enrolled_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', enrolled_at) = enrolled_at, 0)),
    last_heartbeat_at TEXT CHECK (last_heartbeat_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_heartbeat_at) = last_heartbeat_at, 0)),
    last_successful_snapshot_at TEXT CHECK (last_successful_snapshot_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_successful_snapshot_at) = last_successful_snapshot_at, 0)),
    last_error_summary TEXT,
    revoked_at TEXT CHECK (revoked_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', revoked_at) = revoked_at, 0)),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    CHECK (last_heartbeat_at IS NULL OR last_heartbeat_at >= enrolled_at),
    CHECK (last_successful_snapshot_at IS NULL OR last_successful_snapshot_at >= enrolled_at),
    CHECK (revoked_at IS NULL OR revoked_at >= enrolled_at)
);

CREATE INDEX agents_freshness_idx ON agents (revoked_at, last_heartbeat_at, id);

CREATE TABLE agent_enrollment_tokens (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    compute_id TEXT NOT NULL REFERENCES computes(id) ON DELETE RESTRICT,
    token_hash TEXT NOT NULL UNIQUE CHECK (length(token_hash) >= 32),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    expires_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', expires_at) = expires_at, 0)),
    consumed_at TEXT CHECK (consumed_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', consumed_at) = consumed_at, 0)),
    revoked_at TEXT CHECK (revoked_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', revoked_at) = revoked_at, 0)),
    created_by_administrator_id TEXT NOT NULL REFERENCES administrators(id) ON DELETE RESTRICT,
    CHECK (expires_at > created_at),
    CHECK (consumed_at IS NULL OR consumed_at >= created_at),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at),
    CHECK (consumed_at IS NULL OR revoked_at IS NULL)
);

CREATE UNIQUE INDEX agent_enrollment_tokens_active_compute_idx ON agent_enrollment_tokens (compute_id) WHERE consumed_at IS NULL AND revoked_at IS NULL;
CREATE INDEX agent_enrollment_tokens_expiry_idx ON agent_enrollment_tokens (expires_at) WHERE consumed_at IS NULL AND revoked_at IS NULL;

CREATE TABLE snapshot_contents (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    format_version INTEGER NOT NULL CHECK (format_version >= 1),
    content_hash TEXT NOT NULL CHECK (length(content_hash) = 64 AND content_hash = lower(content_hash) AND content_hash NOT GLOB '*[^0-9a-f]*'),
    compressed_document BLOB NOT NULL CHECK (length(compressed_document) > 0),
    uncompressed_size INTEGER NOT NULL CHECK (uncompressed_size > 0),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    UNIQUE (format_version, content_hash)
);

CREATE INDEX snapshot_contents_created_idx ON snapshot_contents (created_at, id);

CREATE TABLE discovery_reports (
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

CREATE INDEX discovery_reports_agent_timeline_idx ON discovery_reports (agent_id, received_at DESC, id DESC);
CREATE INDEX discovery_reports_retention_idx ON discovery_reports (received_at, id);
CREATE INDEX discovery_reports_snapshot_idx ON discovery_reports (snapshot_content_id);

CREATE TABLE discovery_collector_results (
    report_id TEXT NOT NULL REFERENCES discovery_reports(id) ON DELETE CASCADE,
    collector_name TEXT NOT NULL CHECK (length(collector_name) > 0),
    success INTEGER NOT NULL CHECK (success IN (0, 1)),
    sanitized_error TEXT,
    PRIMARY KEY (report_id, collector_name),
    CHECK ((success = 1 AND sanitized_error IS NULL) OR success = 0)
) WITHOUT ROWID;

CREATE TABLE compute_observations (
    compute_id TEXT PRIMARY KEY REFERENCES computes(id) ON DELETE CASCADE,
    machine_id TEXT NOT NULL DEFAULT '',
    hostname TEXT NOT NULL DEFAULT '',
    os_name TEXT NOT NULL DEFAULT '',
    os_version TEXT NOT NULL DEFAULT '',
    kernel_version TEXT NOT NULL DEFAULT '',
    architecture TEXT NOT NULL DEFAULT '',
    cpu_model TEXT NOT NULL DEFAULT '',
    logical_cpu_count INTEGER CHECK (logical_cpu_count IS NULL OR logical_cpu_count > 0),
    memory_bytes INTEGER CHECK (memory_bytes IS NULL OR memory_bytes >= 0),
    boot_id TEXT NOT NULL DEFAULT '',
    booted_at TEXT CHECK (booted_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', booted_at) = booted_at, 0)),
    uptime_seconds INTEGER CHECK (uptime_seconds IS NULL OR uptime_seconds >= 0),
    current_agent_id TEXT REFERENCES agents(id) ON DELETE SET NULL,
    current_report_id TEXT REFERENCES discovery_reports(id) ON DELETE SET NULL,
    observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', observed_at) = observed_at, 0)),
    received_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', received_at) = received_at, 0)),
    CHECK (received_at >= observed_at)
);

CREATE INDEX compute_observations_agent_idx ON compute_observations (current_agent_id);
CREATE INDEX compute_observations_report_idx ON compute_observations (current_report_id);

CREATE TABLE dns_zones (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    cloudflare_zone_id TEXT NOT NULL UNIQUE CHECK (length(cloudflare_zone_id) > 0),
    normalized_name TEXT NOT NULL COLLATE NOCASE CHECK (normalized_name = lower(normalized_name) AND length(normalized_name) BETWEEN 1 AND 253 AND normalized_name NOT LIKE '.%' AND normalized_name NOT LIKE '%.'),
    cloudflare_status TEXT NOT NULL CHECK (length(cloudflare_status) > 0),
    selection_state TEXT NOT NULL CHECK (selection_state IN ('available', 'managed', 'disabled')),
    domain_id TEXT UNIQUE REFERENCES domains(id) ON DELETE RESTRICT,
    discovered_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', discovered_at) = discovered_at, 0)),
    refreshed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', refreshed_at) = refreshed_at, 0)),
    last_drift_scan_at TEXT CHECK (last_drift_scan_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_drift_scan_at) = last_drift_scan_at, 0)),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    status_metadata TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(status_metadata) AND json_type(status_metadata) = 'object'),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    UNIQUE (normalized_name),
    CHECK ((selection_state = 'available' AND domain_id IS NULL) OR (selection_state IN ('managed', 'disabled') AND domain_id IS NOT NULL)),
    CHECK (refreshed_at >= discovered_at),
    CHECK (updated_at >= created_at)
);

CREATE INDEX dns_zones_list_idx ON dns_zones (selection_state, normalized_name, id);
CREATE INDEX dns_zones_refresh_idx ON dns_zones (refreshed_at, id);

CREATE TABLE dns_names (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    zone_id TEXT NOT NULL REFERENCES dns_zones(id) ON DELETE RESTRICT,
    fqdn TEXT NOT NULL COLLATE NOCASE CHECK (fqdn = lower(fqdn) AND length(fqdn) BETWEEN 1 AND 253 AND fqdn NOT LIKE '%.%.' AND fqdn NOT LIKE '%.'),
    is_wildcard INTEGER NOT NULL CHECK (is_wildcard IN (0, 1)),
    description TEXT NOT NULL DEFAULT '',
    desired_revision INTEGER NOT NULL DEFAULT 1 CHECK (desired_revision >= 1),
    lifecycle_state TEXT NOT NULL CHECK (lifecycle_state IN ('active', 'deleting', 'deleted')),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    archived_at TEXT CHECK (archived_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', archived_at) = archived_at, 0)),
    deletion_requested_at TEXT CHECK (deletion_requested_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', deletion_requested_at) = deletion_requested_at, 0)),
    deleted_at TEXT CHECK (deleted_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', deleted_at) = deleted_at, 0)),
    tombstone_document TEXT CHECK (tombstone_document IS NULL OR json_valid(tombstone_document)),
    CHECK ((is_wildcard = 1 AND fqdn LIKE '*.%') OR (is_wildcard = 0 AND fqdn NOT LIKE '*.%')),
    CHECK ((lifecycle_state = 'active' AND deletion_requested_at IS NULL AND deleted_at IS NULL)
        OR (lifecycle_state = 'deleting' AND deletion_requested_at IS NOT NULL AND deleted_at IS NULL AND tombstone_document IS NOT NULL)
        OR (lifecycle_state = 'deleted' AND deletion_requested_at IS NOT NULL AND deleted_at IS NOT NULL AND tombstone_document IS NOT NULL)),
    CHECK (updated_at >= created_at)
);

CREATE UNIQUE INDEX dns_names_active_name_idx ON dns_names (zone_id, fqdn) WHERE archived_at IS NULL;
CREATE INDEX dns_names_list_idx ON dns_names (zone_id, archived_at, fqdn, id);
CREATE INDEX dns_names_lifecycle_idx ON dns_names (lifecycle_state, updated_at, id);

CREATE TABLE dns_record_sets (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    dns_name_id TEXT NOT NULL REFERENCES dns_names(id) ON DELETE CASCADE,
    view TEXT NOT NULL CHECK (view IN ('public', 'internal')),
    type TEXT NOT NULL CHECK (type IN ('A', 'AAAA', 'CNAME')),
    ttl INTEGER NOT NULL DEFAULT 60 CHECK (ttl BETWEEN 60 AND 86400),
    proxied INTEGER NOT NULL DEFAULT 0 CHECK (proxied IN (0, 1)),
    sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
    UNIQUE (dns_name_id, view, type),
    UNIQUE (dns_name_id, view, sort_order),
    CHECK (view = 'public' OR proxied = 0)
);

CREATE INDEX dns_record_sets_name_idx ON dns_record_sets (dns_name_id, view, sort_order);

-- +goose StatementBegin
CREATE TRIGGER dns_record_sets_no_cname_conflict_insert
BEFORE INSERT ON dns_record_sets
WHEN EXISTS (
    SELECT 1 FROM dns_record_sets existing
    WHERE existing.dns_name_id = NEW.dns_name_id AND existing.view = NEW.view
      AND (existing.type = 'CNAME' OR NEW.type = 'CNAME')
)
BEGIN
    SELECT RAISE(ABORT, 'CNAME cannot coexist with another record set in one view');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER dns_record_sets_no_cname_conflict_update
BEFORE UPDATE OF dns_name_id, view, type ON dns_record_sets
WHEN EXISTS (
    SELECT 1 FROM dns_record_sets existing
    WHERE existing.id <> OLD.id AND existing.dns_name_id = NEW.dns_name_id AND existing.view = NEW.view
      AND (existing.type = 'CNAME' OR NEW.type = 'CNAME')
)
BEGIN
    SELECT RAISE(ABORT, 'CNAME cannot coexist with another record set in one view');
END;
-- +goose StatementEnd

CREATE TABLE dns_record_values (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    record_set_id TEXT NOT NULL REFERENCES dns_record_sets(id) ON DELETE CASCADE,
    normalized_value TEXT NOT NULL CHECK (length(normalized_value) > 0),
    sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
    value_checksum TEXT NOT NULL CHECK (length(value_checksum) = 64 AND value_checksum = lower(value_checksum) AND value_checksum NOT GLOB '*[^0-9a-f]*'),
    UNIQUE (record_set_id, normalized_value),
    UNIQUE (record_set_id, sort_order)
);

CREATE INDEX dns_record_values_set_idx ON dns_record_values (record_set_id, sort_order);

-- +goose StatementBegin
CREATE TRIGGER dns_record_values_cname_cardinality_insert
BEFORE INSERT ON dns_record_values
WHEN (SELECT type FROM dns_record_sets WHERE id = NEW.record_set_id) = 'CNAME'
 AND EXISTS (SELECT 1 FROM dns_record_values WHERE record_set_id = NEW.record_set_id)
BEGIN
    SELECT RAISE(ABORT, 'CNAME record sets contain exactly one value');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER dns_record_values_cname_cardinality_update
BEFORE UPDATE OF record_set_id ON dns_record_values
WHEN (SELECT type FROM dns_record_sets WHERE id = NEW.record_set_id) = 'CNAME'
 AND EXISTS (SELECT 1 FROM dns_record_values WHERE record_set_id = NEW.record_set_id AND id <> OLD.id)
BEGIN
    SELECT RAISE(ABORT, 'CNAME record sets contain exactly one value');
END;
-- +goose StatementEnd

CREATE TABLE dns_inventory_links (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    dns_name_id TEXT NOT NULL REFERENCES dns_names(id) ON DELETE CASCADE,
    compute_id TEXT REFERENCES computes(id) ON DELETE RESTRICT,
    container_id TEXT REFERENCES containers(id) ON DELETE RESTRICT,
    ip_assignment_id TEXT REFERENCES ip_assignments(id) ON DELETE RESTRICT,
    CHECK ((compute_id IS NOT NULL) + (container_id IS NOT NULL) + (ip_assignment_id IS NOT NULL) = 1)
);

CREATE UNIQUE INDEX dns_inventory_links_compute_idx ON dns_inventory_links (dns_name_id, compute_id) WHERE compute_id IS NOT NULL;
CREATE UNIQUE INDEX dns_inventory_links_container_idx ON dns_inventory_links (dns_name_id, container_id) WHERE container_id IS NOT NULL;
CREATE UNIQUE INDEX dns_inventory_links_ip_idx ON dns_inventory_links (dns_name_id, ip_assignment_id) WHERE ip_assignment_id IS NOT NULL;

CREATE TABLE dns_drafts (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    operation TEXT NOT NULL CHECK (operation IN ('create', 'update', 'delete')),
    target_dns_name_id TEXT REFERENCES dns_names(id) ON DELETE RESTRICT,
    base_revision INTEGER CHECK (base_revision IS NULL OR base_revision >= 1),
    proposed_aggregate TEXT CHECK (proposed_aggregate IS NULL OR json_valid(proposed_aggregate)),
    content_hash TEXT NOT NULL CHECK (length(content_hash) = 64 AND content_hash = lower(content_hash) AND content_hash NOT GLOB '*[^0-9a-f]*'),
    validation_state TEXT NOT NULL CHECK (validation_state IN ('pending', 'valid', 'invalid')),
    validation_errors TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(validation_errors) AND json_type(validation_errors) = 'array'),
    latest_preview TEXT CHECK (latest_preview IS NULL OR json_valid(latest_preview)),
    preview_fingerprint TEXT CHECK (preview_fingerprint IS NULL OR (length(preview_fingerprint) = 64 AND preview_fingerprint = lower(preview_fingerprint) AND preview_fingerprint NOT GLOB '*[^0-9a-f]*')),
    provider_observed_at TEXT CHECK (provider_observed_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', provider_observed_at) = provider_observed_at, 0)),
    actor_type TEXT NOT NULL CHECK (actor_type IN ('administrator', 'personal_access_token')),
    actor_id TEXT NOT NULL CHECK (length(actor_id) > 0),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    applied_at TEXT CHECK (applied_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', applied_at) = applied_at, 0)),
    discarded_at TEXT CHECK (discarded_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', discarded_at) = discarded_at, 0)),
    CHECK ((operation = 'create' AND target_dns_name_id IS NULL AND base_revision IS NULL AND proposed_aggregate IS NOT NULL)
        OR (operation = 'update' AND target_dns_name_id IS NOT NULL AND base_revision IS NOT NULL AND proposed_aggregate IS NOT NULL)
        OR (operation = 'delete' AND target_dns_name_id IS NOT NULL AND base_revision IS NOT NULL AND proposed_aggregate IS NULL)),
    CHECK ((latest_preview IS NULL AND preview_fingerprint IS NULL AND provider_observed_at IS NULL)
        OR (latest_preview IS NOT NULL AND preview_fingerprint IS NOT NULL AND provider_observed_at IS NOT NULL)),
    CHECK (applied_at IS NULL OR discarded_at IS NULL),
    CHECK (updated_at >= created_at)
);

CREATE UNIQUE INDEX dns_drafts_active_target_actor_idx ON dns_drafts (target_dns_name_id, actor_type, actor_id)
    WHERE target_dns_name_id IS NOT NULL AND applied_at IS NULL AND discarded_at IS NULL;
CREATE INDEX dns_drafts_actor_idx ON dns_drafts (actor_type, actor_id, updated_at DESC);

CREATE TABLE dns_projections (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    dns_name_id TEXT NOT NULL REFERENCES dns_names(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider IN ('cloudflare', 'internal')),
    desired_revision INTEGER NOT NULL CHECK (desired_revision >= 1),
    applied_revision INTEGER NOT NULL DEFAULT 0 CHECK (applied_revision >= 0 AND applied_revision <= desired_revision),
    state TEXT NOT NULL CHECK (state IN ('not_applicable', 'pending', 'synchronized', 'failed', 'deleting')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_retry_at TEXT CHECK (next_retry_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', next_retry_at) = next_retry_at, 0)),
    last_attempt_at TEXT CHECK (last_attempt_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_attempt_at) = last_attempt_at, 0)),
    last_success_at TEXT CHECK (last_success_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_success_at) = last_success_at, 0)),
    last_error_code TEXT,
    last_error_message TEXT,
    observed_checksum TEXT CHECK (observed_checksum IS NULL OR (length(observed_checksum) = 64 AND observed_checksum = lower(observed_checksum) AND observed_checksum NOT GLOB '*[^0-9a-f]*')),
    lease_owner TEXT CHECK (lease_owner IS NULL OR length(lease_owner) > 0),
    lease_expires_at TEXT CHECK (lease_expires_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', lease_expires_at) = lease_expires_at, 0)),
    leased_revision INTEGER CHECK (leased_revision IS NULL OR leased_revision >= 1),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    UNIQUE (dns_name_id, provider),
    UNIQUE (id, dns_name_id),
    CHECK ((lease_owner IS NULL AND lease_expires_at IS NULL AND leased_revision IS NULL)
        OR (lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL AND leased_revision = desired_revision AND state IN ('pending', 'failed', 'deleting'))),
    CHECK ((state = 'synchronized' AND applied_revision = desired_revision) OR state <> 'synchronized'),
    CHECK ((last_error_code IS NULL AND last_error_message IS NULL) OR (last_error_code IS NOT NULL AND last_error_message IS NOT NULL))
);

CREATE INDEX dns_projections_due_idx ON dns_projections (state, next_retry_at, lease_expires_at, updated_at, id)
    WHERE state IN ('pending', 'failed', 'deleting');
CREATE INDEX dns_projections_name_idx ON dns_projections (dns_name_id, provider, state);

CREATE TABLE cloudflare_record_mappings (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    projection_id TEXT NOT NULL,
    dns_name_id TEXT NOT NULL REFERENCES dns_names(id) ON DELETE CASCADE,
    record_value_id TEXT REFERENCES dns_record_values(id) ON DELETE SET NULL,
    cloudflare_record_id TEXT NOT NULL CHECK (length(cloudflare_record_id) > 0),
    expected_type TEXT NOT NULL CHECK (expected_type IN ('A', 'AAAA', 'CNAME')),
    expected_name TEXT NOT NULL CHECK (length(expected_name) > 0),
    expected_value TEXT NOT NULL CHECK (length(expected_value) > 0),
    expected_ttl INTEGER NOT NULL CHECK (expected_ttl BETWEEN 60 AND 86400),
    expected_proxied INTEGER NOT NULL CHECK (expected_proxied IN (0, 1)),
    expected_checksum TEXT NOT NULL CHECK (length(expected_checksum) = 64 AND expected_checksum = lower(expected_checksum) AND expected_checksum NOT GLOB '*[^0-9a-f]*'),
    desired_revision INTEGER NOT NULL CHECK (desired_revision >= 1),
    UNIQUE (projection_id, cloudflare_record_id),
    UNIQUE (projection_id, record_value_id),
    FOREIGN KEY (projection_id, dns_name_id) REFERENCES dns_projections(id, dns_name_id) ON DELETE CASCADE
);

CREATE INDEX cloudflare_record_mappings_name_idx ON cloudflare_record_mappings (dns_name_id, desired_revision);

CREATE TABLE dns_publications (
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

CREATE INDEX dns_publications_zone_idx ON dns_publications (zone_id, created_at DESC, id DESC);
CREATE INDEX dns_publications_revision_idx ON dns_publications (zone_id, desired_revision, result);

CREATE TABLE dns_drift_events (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    zone_id TEXT NOT NULL REFERENCES dns_zones(id) ON DELETE RESTRICT,
    dns_name_id TEXT REFERENCES dns_names(id) ON DELETE SET NULL,
    observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', observed_at) = observed_at, 0)),
    expected_checksum TEXT CHECK (expected_checksum IS NULL OR (length(expected_checksum) = 64 AND expected_checksum = lower(expected_checksum) AND expected_checksum NOT GLOB '*[^0-9a-f]*')),
    observed_state TEXT NOT NULL CHECK (json_valid(observed_state)),
    repair_status TEXT NOT NULL CHECK (repair_status IN ('pending', 'enqueued', 'repaired', 'failed', 'ignored')),
    audit_event_id TEXT REFERENCES audit_events(id) ON DELETE SET NULL
);

CREATE INDEX dns_drift_events_zone_idx ON dns_drift_events (zone_id, observed_at DESC, id DESC);
CREATE INDEX dns_drift_events_name_idx ON dns_drift_events (dns_name_id, observed_at DESC);
CREATE INDEX dns_drift_events_repair_idx ON dns_drift_events (repair_status, observed_at, id) WHERE repair_status IN ('pending', 'enqueued', 'failed');

PRAGMA user_version = 1;

-- +goose Down
DROP TABLE IF EXISTS dns_drift_events;
DROP TABLE IF EXISTS dns_publications;
DROP TABLE IF EXISTS cloudflare_record_mappings;
DROP TABLE IF EXISTS dns_projections;
DROP TABLE IF EXISTS dns_drafts;
DROP TABLE IF EXISTS dns_inventory_links;
DROP TRIGGER IF EXISTS dns_record_values_cname_cardinality_update;
DROP TRIGGER IF EXISTS dns_record_values_cname_cardinality_insert;
DROP TABLE IF EXISTS dns_record_values;
DROP TRIGGER IF EXISTS dns_record_sets_no_cname_conflict_update;
DROP TRIGGER IF EXISTS dns_record_sets_no_cname_conflict_insert;
DROP TABLE IF EXISTS dns_record_sets;
DROP TABLE IF EXISTS dns_names;
DROP TABLE IF EXISTS dns_zones;
DROP TABLE IF EXISTS compute_observations;
DROP TABLE IF EXISTS discovery_collector_results;
DROP TABLE IF EXISTS discovery_reports;
DROP TABLE IF EXISTS snapshot_contents;
DROP TABLE IF EXISTS agent_enrollment_tokens;
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS domain_tags;
DROP TABLE IF EXISTS container_tags;
DROP TABLE IF EXISTS compute_tags;
DROP TABLE IF EXISTS location_tags;
DROP TABLE IF EXISTS provider_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS costs;
DROP TABLE IF EXISTS domains;
DROP TABLE IF EXISTS ip_assignments;
DROP TABLE IF EXISTS container_labels;
DROP TABLE IF EXISTS container_mounts;
DROP TABLE IF EXISTS container_addresses;
DROP TABLE IF EXISTS container_networks;
DROP TABLE IF EXISTS container_ports;
DROP TABLE IF EXISTS containers;
DROP TABLE IF EXISTS computes;
DROP TABLE IF EXISTS locations;
DROP TABLE IF EXISTS provider_roles;
DROP TABLE IF EXISTS providers;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS personal_access_tokens;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS administrators;
PRAGMA user_version = 0;
