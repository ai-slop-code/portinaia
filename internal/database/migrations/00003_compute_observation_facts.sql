-- +goose Up
ALTER TABLE compute_observations
    ADD COLUMN virtualization_role TEXT
    CHECK (virtualization_role IS NULL OR virtualization_role IN ('none', 'host', 'guest', 'unknown'));

-- first/last/absence times are server receipt times, not client collection clocks.
-- A service may mark rows absent only after the matching collector succeeds completely;
-- collector failure preserves the prior presence state. absence_report_id records that evidence.
-- Stable interface identity is scoped to one compute and interface name.
CREATE TABLE compute_interfaces (
    compute_id TEXT NOT NULL REFERENCES computes(id) ON DELETE CASCADE,
    interface_name TEXT NOT NULL CHECK (length(trim(interface_name)) BETWEEN 1 AND 255),
    mac_address TEXT CHECK (mac_address IS NULL OR length(mac_address) <= 64),
    mtu INTEGER CHECK (mtu IS NULL OR mtu BETWEEN 68 AND 1000000),
    is_up INTEGER CHECK (is_up IS NULL OR is_up IN (0, 1)),
    first_observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', first_observed_at) = first_observed_at, 0)),
    last_observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_observed_at) = last_observed_at, 0)),
    last_report_id TEXT REFERENCES discovery_reports(id) ON DELETE SET NULL,
    presence_state TEXT NOT NULL DEFAULT 'present' CHECK (presence_state IN ('present', 'absent')),
    absent_at TEXT CHECK (absent_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', absent_at) = absent_at, 0)),
    absence_report_id TEXT REFERENCES discovery_reports(id) ON DELETE SET NULL,
    PRIMARY KEY (compute_id, interface_name),
    CHECK (last_observed_at >= first_observed_at),
    CHECK ((presence_state = 'present' AND absent_at IS NULL) OR (presence_state = 'absent' AND absent_at IS NOT NULL)),
    CHECK (absent_at IS NULL OR absent_at >= last_observed_at)
) WITHOUT ROWID;

CREATE INDEX compute_interfaces_presence_idx ON compute_interfaces (compute_id, presence_state, interface_name);
CREATE INDEX compute_interfaces_report_idx ON compute_interfaces (last_report_id);

-- Address identity is compute + interface + normalized address; prefix changes are updates to that observation.
CREATE TABLE interface_addresses (
    compute_id TEXT NOT NULL,
    interface_name TEXT NOT NULL,
    address TEXT NOT NULL CHECK (length(trim(address)) BETWEEN 1 AND 45),
    prefix_length INTEGER CHECK (prefix_length IS NULL OR prefix_length BETWEEN 0 AND 128),
    scope TEXT CHECK (scope IS NULL OR scope IN ('public', 'private', 'loopback', 'link_local', 'other')),
    first_observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', first_observed_at) = first_observed_at, 0)),
    last_observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_observed_at) = last_observed_at, 0)),
    last_report_id TEXT REFERENCES discovery_reports(id) ON DELETE SET NULL,
    presence_state TEXT NOT NULL DEFAULT 'present' CHECK (presence_state IN ('present', 'absent')),
    absent_at TEXT CHECK (absent_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', absent_at) = absent_at, 0)),
    absence_report_id TEXT REFERENCES discovery_reports(id) ON DELETE SET NULL,
    PRIMARY KEY (compute_id, interface_name, address),
    FOREIGN KEY (compute_id, interface_name) REFERENCES compute_interfaces(compute_id, interface_name) ON DELETE CASCADE,
    CHECK (last_observed_at >= first_observed_at),
    CHECK ((presence_state = 'present' AND absent_at IS NULL) OR (presence_state = 'absent' AND absent_at IS NOT NULL)),
    CHECK (absent_at IS NULL OR absent_at >= last_observed_at)
) WITHOUT ROWID;

CREATE INDEX interface_addresses_presence_idx ON interface_addresses (compute_id, presence_state, address);
CREATE INDEX interface_addresses_report_idx ON interface_addresses (last_report_id);

-- Stable disk identity is the device name within one compute.
CREATE TABLE compute_disks (
    compute_id TEXT NOT NULL REFERENCES computes(id) ON DELETE CASCADE,
    device_name TEXT NOT NULL CHECK (length(trim(device_name)) BETWEEN 1 AND 255),
    model TEXT CHECK (model IS NULL OR length(model) <= 500),
    size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
    first_observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', first_observed_at) = first_observed_at, 0)),
    last_observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_observed_at) = last_observed_at, 0)),
    last_report_id TEXT REFERENCES discovery_reports(id) ON DELETE SET NULL,
    presence_state TEXT NOT NULL DEFAULT 'present' CHECK (presence_state IN ('present', 'absent')),
    absent_at TEXT CHECK (absent_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', absent_at) = absent_at, 0)),
    absence_report_id TEXT REFERENCES discovery_reports(id) ON DELETE SET NULL,
    PRIMARY KEY (compute_id, device_name),
    CHECK (last_observed_at >= first_observed_at),
    CHECK ((presence_state = 'present' AND absent_at IS NULL) OR (presence_state = 'absent' AND absent_at IS NOT NULL)),
    CHECK (absent_at IS NULL OR absent_at >= last_observed_at)
) WITHOUT ROWID;

CREATE INDEX compute_disks_presence_idx ON compute_disks (compute_id, presence_state, device_name);
CREATE INDEX compute_disks_report_idx ON compute_disks (last_report_id);

-- Stable filesystem identity is the device and mount point within one compute.
-- available_bytes is the latest volatile measurement; it is stored per report in history separately.
CREATE TABLE compute_filesystems (
    compute_id TEXT NOT NULL REFERENCES computes(id) ON DELETE CASCADE,
    device_name TEXT NOT NULL CHECK (length(trim(device_name)) BETWEEN 1 AND 4096),
    mount_point TEXT NOT NULL CHECK (length(trim(mount_point)) BETWEEN 1 AND 4096),
    filesystem_type TEXT NOT NULL CHECK (length(trim(filesystem_type)) BETWEEN 1 AND 100),
    total_bytes INTEGER NOT NULL CHECK (total_bytes >= 0),
    available_bytes INTEGER CHECK (available_bytes IS NULL OR available_bytes >= 0),
    first_observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', first_observed_at) = first_observed_at, 0)),
    last_observed_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', last_observed_at) = last_observed_at, 0)),
    last_report_id TEXT REFERENCES discovery_reports(id) ON DELETE SET NULL,
    presence_state TEXT NOT NULL DEFAULT 'present' CHECK (presence_state IN ('present', 'absent')),
    absent_at TEXT CHECK (absent_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', absent_at) = absent_at, 0)),
    absence_report_id TEXT REFERENCES discovery_reports(id) ON DELETE SET NULL,
    PRIMARY KEY (compute_id, device_name, mount_point),
    CHECK (last_observed_at >= first_observed_at),
    CHECK ((presence_state = 'present' AND absent_at IS NULL) OR (presence_state = 'absent' AND absent_at IS NOT NULL)),
    CHECK (absent_at IS NULL OR absent_at >= last_observed_at)
) WITHOUT ROWID;

CREATE INDEX compute_filesystems_presence_idx ON compute_filesystems (compute_id, presence_state, mount_point, device_name);
CREATE INDEX compute_filesystems_report_idx ON compute_filesystems (last_report_id);

PRAGMA user_version = 3;

-- +goose Down
CREATE TABLE _wp02_down_guard (valid INTEGER NOT NULL CHECK (valid = 1));
INSERT INTO _wp02_down_guard (valid)
SELECT CASE WHEN
    EXISTS (SELECT 1 FROM compute_interfaces)
    OR EXISTS (SELECT 1 FROM interface_addresses)
    OR EXISTS (SELECT 1 FROM compute_disks)
    OR EXISTS (SELECT 1 FROM compute_filesystems)
    OR EXISTS (SELECT 1 FROM compute_observations WHERE virtualization_role IS NOT NULL)
THEN 0 ELSE 1 END;
DROP TABLE _wp02_down_guard;

DROP TABLE compute_filesystems;
DROP TABLE compute_disks;
DROP TABLE interface_addresses;
DROP TABLE compute_interfaces;
ALTER TABLE compute_observations DROP COLUMN virtualization_role;
PRAGMA user_version = 2;
