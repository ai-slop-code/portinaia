package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	apigen "portinaia/internal/api/generated"
	"portinaia/internal/discovery"
)

func persistCollectorResults(ctx context.Context, tx *sql.Tx, reportID string, collectors []snapshotCollector, receivedAt string) error {
	for _, collector := range collectors {
		name := collector.Name
		if collector.Name == "podman" && collector.OwnerUID != nil {
			name = fmt.Sprintf("podman:%d:%s", *collector.OwnerUID, stringValue(collector.Connection))
		}
		var message any
		if !collector.Success {
			message = collector.Message
			if collector.Message == "" {
				message = collector.Code
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO discovery_collector_results(report_id,collector_name,success,sanitized_error) VALUES(?,?,?,?)`, reportID, name, boolInt(collector.Success), message); err != nil {
			return err
		}
		var ownerUID, connection any
		if collector.OwnerUID != nil {
			ownerUID = *collector.OwnerUID
		}
		if collector.Connection != nil {
			connection = *collector.Connection
		}
		successAt, successID, failedAt, failedID, code, errorMessage := any(nil), any(nil), any(nil), any(nil), any(nil), any(nil)
		if collector.Success {
			successAt, successID = receivedAt, reportID
		} else {
			failedAt, failedID, code, errorMessage = receivedAt, reportID, collector.Code, collector.Message
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO compute_collector_observations(compute_id,collector_key,collector_name,owner_uid,connection_name,last_status,last_received_at,last_report_id,last_successful_at,last_successful_report_id,last_failed_at,last_failed_report_id,last_error_code,last_error_message) VALUES((SELECT compute_id FROM agents WHERE id=(SELECT agent_id FROM discovery_reports WHERE id=?)),?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(compute_id,collector_key) DO UPDATE SET collector_name=excluded.collector_name,owner_uid=excluded.owner_uid,connection_name=excluded.connection_name,last_status=excluded.last_status,last_received_at=excluded.last_received_at,last_report_id=excluded.last_report_id,last_successful_at=CASE WHEN excluded.last_status='success' THEN excluded.last_received_at ELSE compute_collector_observations.last_successful_at END,last_successful_report_id=CASE WHEN excluded.last_status='success' THEN excluded.last_report_id ELSE compute_collector_observations.last_successful_report_id END,last_failed_at=CASE WHEN excluded.last_status='failed' THEN excluded.last_received_at ELSE compute_collector_observations.last_failed_at END,last_failed_report_id=CASE WHEN excluded.last_status='failed' THEN excluded.last_report_id ELSE compute_collector_observations.last_failed_report_id END,last_error_code=excluded.last_error_code,last_error_message=excluded.last_error_message`, reportID, collector.Key, collector.Name, ownerUID, connection, statusString(collector.Success), receivedAt, reportID, successAt, successID, failedAt, failedID, code, errorMessage)
		if err != nil {
			return err
		}
	}
	return nil
}

func statusString(success bool) string {
	status := "failed"
	if success {
		status = "success"
	}
	if (discovery.CollectorOutcome{Status: status}).Authoritative() {
		return "success"
	}
	return "failed"
}
func collectorSuccess(items []snapshotCollector, name string) bool {
	for _, item := range items {
		if item.Name == name && item.Success {
			return true
		}
	}
	return false
}

func applyHostSnapshot(ctx context.Context, tx *sql.Tx, computeID, agentID, reportID string, host apigen.HostSnapshot, received string) error {
	var booted any
	if host.BootedAt != nil {
		booted = host.BootedAt.UTC().Format(time.RFC3339)
	}
	var virtualization any
	if host.VirtualizationRole != nil {
		virtualization = string(*host.VirtualizationRole)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO compute_observations(compute_id,machine_id,hostname,os_name,os_version,kernel_version,architecture,cpu_model,logical_cpu_count,memory_bytes,boot_id,booted_at,uptime_seconds,current_agent_id,current_report_id,observed_at,received_at,virtualization_role) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(compute_id) DO UPDATE SET machine_id=excluded.machine_id,hostname=excluded.hostname,os_name=excluded.os_name,os_version=excluded.os_version,kernel_version=excluded.kernel_version,architecture=excluded.architecture,cpu_model=excluded.cpu_model,logical_cpu_count=excluded.logical_cpu_count,memory_bytes=excluded.memory_bytes,boot_id=excluded.boot_id,booted_at=excluded.booted_at,uptime_seconds=excluded.uptime_seconds,current_agent_id=excluded.current_agent_id,current_report_id=excluded.current_report_id,observed_at=excluded.observed_at,received_at=excluded.received_at,virtualization_role=excluded.virtualization_role`, computeID, host.MachineId, host.Hostname, host.OsName, host.OsVersion, host.KernelVersion, host.Architecture, stringValue(host.CpuModel), host.LogicalCpuCount, host.MemoryBytes, stringValue(host.BootId), booted, host.UptimeSeconds, agentID, reportID, received, received, virtualization)
	return err
}

func applyInterfaces(ctx context.Context, tx *sql.Tx, computeID, reportID string, interfaces []apigen.NetworkInterfaceObservation, success bool, received string) error {
	if !success {
		return nil
	}
	seenInterfaces := map[string]bool{}
	seenAddresses := map[string]map[string]bool{}
	for _, item := range interfaces {
		if strings.TrimSpace(item.Name) == "" || seenInterfaces[item.Name] {
			return errors.New("duplicate or empty interface name")
		}
		seenInterfaces[item.Name] = true
		var mtu, isUp any
		if item.Mtu != nil {
			mtu = *item.Mtu
		}
		if item.Up != nil {
			isUp = boolInt(*item.Up)
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO compute_interfaces(compute_id,interface_name,mac_address,mtu,is_up,first_observed_at,last_observed_at,last_report_id,presence_state,absent_at,absence_report_id) VALUES(?,?,?,?,?,?,?,?,'present',NULL,NULL) ON CONFLICT(compute_id,interface_name) DO UPDATE SET mac_address=excluded.mac_address,mtu=excluded.mtu,is_up=excluded.is_up,last_observed_at=excluded.last_observed_at,last_report_id=excluded.last_report_id,presence_state='present',absent_at=NULL,absence_report_id=NULL`, computeID, item.Name, item.MacAddress, mtu, isUp, received, received, reportID)
		if err != nil {
			return err
		}
		seenAddresses[item.Name] = map[string]bool{}
		for _, address := range item.Addresses {
			parsed, err := netip.ParseAddr(address.Address)
			if err != nil || parsed.Zone() != "" {
				return fmt.Errorf("invalid interface address %q", address.Address)
			}
			canonical := parsed.String()
			if seenAddresses[item.Name][canonical] {
				return errors.New("duplicate interface address")
			}
			seenAddresses[item.Name][canonical] = true
			family := 6
			if parsed.Is4() {
				family = 4
			}
			var prefix any
			if address.PrefixLength != nil {
				max := 128
				if family == 4 {
					max = 32
				}
				if *address.PrefixLength < 0 || *address.PrefixLength > max {
					return errors.New("interface address prefix out of range")
				}
				prefix = *address.PrefixLength
			}
			var scope any
			if address.Scope != nil {
				scope = string(*address.Scope)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO interface_addresses(compute_id,interface_name,address,prefix_length,scope,first_observed_at,last_observed_at,last_report_id,presence_state,absent_at,absence_report_id) VALUES(?,?,?,?,?,?,?,?, 'present',NULL,NULL) ON CONFLICT(compute_id,interface_name,address) DO UPDATE SET prefix_length=excluded.prefix_length,scope=excluded.scope,last_observed_at=excluded.last_observed_at,last_report_id=excluded.last_report_id,presence_state='present',absent_at=NULL,absence_report_id=NULL`, computeID, item.Name, canonical, prefix, scope, received, received, reportID); err != nil {
				return err
			}
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT interface_name FROM compute_interfaces WHERE compute_id=? AND presence_state='present'`, computeID)
	if err != nil {
		return err
	}
	var current []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		current = append(current, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, name := range current {
		if !seenInterfaces[name] {
			if _, err := tx.ExecContext(ctx, `UPDATE compute_interfaces SET presence_state='absent',absent_at=?,absence_report_id=? WHERE compute_id=? AND interface_name=? AND presence_state='present'`, received, reportID, computeID, name); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE interface_addresses SET presence_state='absent',absent_at=?,absence_report_id=? WHERE compute_id=? AND interface_name=? AND presence_state='present'`, received, reportID, computeID, name); err != nil {
				return err
			}
			continue
		}
		rows, err := tx.QueryContext(ctx, `SELECT address FROM interface_addresses WHERE compute_id=? AND interface_name=? AND presence_state='present'`, computeID, name)
		if err != nil {
			return err
		}
		var oldAddresses []string
		for rows.Next() {
			var address string
			if err := rows.Scan(&address); err != nil {
				rows.Close()
				return err
			}
			oldAddresses = append(oldAddresses, address)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, address := range oldAddresses {
			if !seenAddresses[name][address] {
				if _, err := tx.ExecContext(ctx, `UPDATE interface_addresses SET presence_state='absent',absent_at=?,absence_report_id=? WHERE compute_id=? AND interface_name=? AND address=? AND presence_state='present'`, received, reportID, computeID, name, address); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func applyDisks(ctx context.Context, tx *sql.Tx, computeID, reportID string, disks []apigen.DiskSnapshot, success bool, received string) error {
	if !success {
		return nil
	}
	seen := map[string]bool{}
	for _, disk := range disks {
		if strings.TrimSpace(disk.Name) == "" || seen[disk.Name] {
			return errors.New("duplicate or empty disk name")
		}
		seen[disk.Name] = true
		_, err := tx.ExecContext(ctx, `INSERT INTO compute_disks(compute_id,device_name,model,size_bytes,first_observed_at,last_observed_at,last_report_id,presence_state,absent_at,absence_report_id) VALUES(?,?,?,?,?,?,?,'present',NULL,NULL) ON CONFLICT(compute_id,device_name) DO UPDATE SET model=excluded.model,size_bytes=excluded.size_bytes,last_observed_at=excluded.last_observed_at,last_report_id=excluded.last_report_id,presence_state='present',absent_at=NULL,absence_report_id=NULL`, computeID, disk.Name, disk.Model, disk.SizeBytes, received, received, reportID)
		if err != nil {
			return err
		}
	}
	return markMissingNames(ctx, tx, `SELECT device_name FROM compute_disks WHERE compute_id=? AND presence_state='present'`, `UPDATE compute_disks SET presence_state='absent',absent_at=?,absence_report_id=? WHERE compute_id=? AND device_name=? AND presence_state='present'`, computeID, reportID, received, seen)
}

func applyFilesystems(ctx context.Context, tx *sql.Tx, computeID, reportID string, filesystems []apigen.FilesystemSnapshot, success bool, received string) error {
	if !success {
		return nil
	}
	seen := map[string]bool{}
	for _, filesystem := range filesystems {
		key := filesystem.Device + "\x00" + filesystem.MountPoint
		if strings.TrimSpace(filesystem.Device) == "" || strings.TrimSpace(filesystem.MountPoint) == "" || seen[key] {
			return errors.New("duplicate or empty filesystem identity")
		}
		seen[key] = true
		_, err := tx.ExecContext(ctx, `INSERT INTO compute_filesystems(compute_id,device_name,mount_point,filesystem_type,total_bytes,available_bytes,first_observed_at,last_observed_at,last_report_id,presence_state,absent_at,absence_report_id) VALUES(?,?,?,?,?,?,?,?,?,'present',NULL,NULL) ON CONFLICT(compute_id,device_name,mount_point) DO UPDATE SET filesystem_type=excluded.filesystem_type,total_bytes=excluded.total_bytes,available_bytes=excluded.available_bytes,last_observed_at=excluded.last_observed_at,last_report_id=excluded.last_report_id,presence_state='present',absent_at=NULL,absence_report_id=NULL`, computeID, filesystem.Device, filesystem.MountPoint, filesystem.Filesystem, filesystem.TotalBytes, filesystem.AvailableBytes, received, received, reportID)
		if err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT device_name,mount_point FROM compute_filesystems WHERE compute_id=? AND presence_state='present'`, computeID)
	if err != nil {
		return err
	}
	type key struct{ device, mount string }
	var old []key
	for rows.Next() {
		var item key
		if err := rows.Scan(&item.device, &item.mount); err != nil {
			rows.Close()
			return err
		}
		old = append(old, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range old {
		if !seen[item.device+"\x00"+item.mount] {
			if _, err := tx.ExecContext(ctx, `UPDATE compute_filesystems SET presence_state='absent',absent_at=?,absence_report_id=? WHERE compute_id=? AND device_name=? AND mount_point=? AND presence_state='present'`, received, reportID, computeID, item.device, item.mount); err != nil {
				return err
			}
		}
	}
	return nil
}

func markMissingNames(ctx context.Context, tx *sql.Tx, selectSQL, updateSQL, computeID, reportID, received string, seen map[string]bool) error {
	rows, err := tx.QueryContext(ctx, selectSQL, computeID)
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, name := range names {
		if !seen[name] {
			if _, err := tx.ExecContext(ctx, updateSQL, received, reportID, computeID, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyPodmanCollection(ctx context.Context, tx *sql.Tx, computeID, reportID string, collection parsedPodman, received string) error {
	if !collection.Success {
		return nil
	}
	seen := map[string]bool{}
	for _, snapshot := range collection.Containers {
		if snapshot.Name == "" || seen[snapshot.Name] {
			return errors.New("duplicate or empty container name")
		}
		seen[snapshot.Name] = true
		policy := string(snapshot.RestartPolicy)
		switch policy {
		case "on_failure":
			policy = "on-failure"
		case "unless_stopped":
			policy = "unless-stopped"
		}
		if policy != "no" && policy != "always" && policy != "on-failure" && policy != "unless-stopped" && policy != "unknown" {
			return errors.New("invalid container restart policy")
		}
		if policy != "on-failure" && snapshot.RestartMaximumRetryCount != nil {
			return errors.New("restart retry count requires on-failure policy")
		}
		var created, started any
		if snapshot.CreatedAt != nil {
			created = snapshot.CreatedAt.UTC().Format(time.RFC3339)
		}
		if snapshot.StartedAt != nil {
			started = snapshot.StartedAt.UTC().Format(time.RFC3339)
		}
		var containerID string
		err := tx.QueryRowContext(ctx, `SELECT id FROM containers WHERE compute_id=? AND podman_owner_uid=? AND name=? AND archived_at IS NULL`, computeID, collection.OwnerUID, snapshot.Name).Scan(&containerID)
		if errors.Is(err, sql.ErrNoRows) {
			var archivedIDs []string
			rows, queryErr := tx.QueryContext(ctx, `SELECT id FROM containers WHERE compute_id=? AND podman_owner_uid=? AND name=? AND archived_at IS NOT NULL`, computeID, collection.OwnerUID, snapshot.Name)
			if queryErr != nil {
				return queryErr
			}
			for rows.Next() {
				var id string
				if queryErr = rows.Scan(&id); queryErr != nil {
					rows.Close()
					return queryErr
				}
				archivedIDs = append(archivedIDs, id)
			}
			if queryErr = rows.Err(); queryErr != nil {
				rows.Close()
				return queryErr
			}
			if queryErr = rows.Close(); queryErr != nil {
				return queryErr
			}
			if len(archivedIDs) > 1 {
				return errors.New("ambiguous archived container logical identity")
			}
			if len(archivedIDs) == 1 {
				containerID = archivedIDs[0]
				err = nil
			}
		}
		if errors.Is(err, sql.ErrNoRows) {
			containerID, err = randomID()
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO containers(id,compute_id,podman_owner_uid,podman_connection,name,runtime_id,image_name,image_id,image_digest,state,health,restart_policy,restart_max_attempts,runtime_created_at,runtime_started_at,first_observed_at,last_observed_at,stale_at,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULL,1,?,?)`, containerID, computeID, collection.OwnerUID, collection.Connection, snapshot.Name, snapshot.RuntimeId, snapshot.ImageName, stringValue(snapshot.ImageId), stringValue(snapshot.ImageDigest), string(snapshot.State), string(snapshot.Health), policy, snapshot.RestartMaximumRetryCount, created, started, received, received, received, received)
		} else if err == nil {
			var archivedAt sql.NullString
			if err := tx.QueryRowContext(ctx, `SELECT archived_at FROM containers WHERE id=?`, containerID).Scan(&archivedAt); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `UPDATE containers SET podman_connection=?,runtime_id=?,image_name=?,image_id=?,image_digest=?,state=?,health=?,restart_policy=?,restart_max_attempts=?,runtime_created_at=?,runtime_started_at=?,last_observed_at=?,stale_at=NULL,archived_at=NULL,updated_at=? WHERE id=?`, collection.Connection, snapshot.RuntimeId, snapshot.ImageName, stringValue(snapshot.ImageId), stringValue(snapshot.ImageDigest), string(snapshot.State), string(snapshot.Health), policy, snapshot.RestartMaximumRetryCount, created, started, received, received, containerID)
			if err == nil && archivedAt.Valid {
				before, _ := json.Marshal(map[string]any{"state": "archived"})
				after, _ := json.Marshal(map[string]any{"state": "active"})
				err = insertSystemDiscoveryAudit(ctx, tx, "container.rediscover", "container", containerID, before, after, received)
			}
		}
		if err != nil {
			return err
		}
		if err := replaceContainerChildren(ctx, tx, containerID, snapshot); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,name FROM containers WHERE compute_id=? AND podman_owner_uid=? AND archived_at IS NULL`, computeID, collection.OwnerUID)
	if err != nil {
		return err
	}
	type item struct{ id, name string }
	var old []item
	for rows.Next() {
		var c item
		if err := rows.Scan(&c.id, &c.name); err != nil {
			rows.Close()
			return err
		}
		old = append(old, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, container := range old {
		if !seen[container.name] {
			if _, err := tx.ExecContext(ctx, `UPDATE containers SET stale_at=? WHERE id=? AND archived_at IS NULL`, received, container.id); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceContainerChildren(ctx context.Context, tx *sql.Tx, containerID string, snapshot apigen.SnapshotContainer) error {
	for _, table := range []string{"container_ports", "container_networks", "container_mounts", "container_labels"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE container_id=?`, containerID); err != nil {
			return err
		}
	}
	for _, port := range snapshot.Ports {
		hostIP := stringValue(port.HostIp)
		var hostPort any
		published := 0
		if port.HostPort != nil {
			hostPort = *port.HostPort
			published = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO container_ports(container_id,protocol,container_port,host_ip,host_port,is_published) VALUES(?,?,?,?,?,?)`, containerID, string(port.Protocol), port.ContainerPort, hostIP, hostPort, published); err != nil {
			return err
		}
	}
	for _, network := range snapshot.Networks {
		result, err := tx.ExecContext(ctx, `INSERT INTO container_networks(container_id,name,network_id,driver) VALUES(?,?,?, '')`, containerID, network.Name, stringValue(network.NetworkId))
		if err != nil {
			return err
		}
		networkID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		for _, address := range network.Addresses {
			parsed, err := netip.ParseAddr(address.Address)
			if err != nil || parsed.Zone() != "" {
				return fmt.Errorf("invalid container address %q", address.Address)
			}
			family := 6
			if parsed.Is4() {
				family = 4
			}
			var prefix any
			if address.PrefixLength != nil {
				max := 128
				if family == 4 {
					max = 32
				}
				if *address.PrefixLength < 0 || *address.PrefixLength > max {
					return errors.New("container address prefix out of range")
				}
				prefix = *address.PrefixLength
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO container_addresses(container_id,network_id,address,family,prefix_length) VALUES(?,?,?,?,?)`, containerID, networkID, parsed.String(), family, prefix); err != nil {
				return err
			}
		}
	}
	for _, mount := range snapshot.Mounts {
		options, _ := json.Marshal([]string{})
		if _, err := tx.ExecContext(ctx, `INSERT INTO container_mounts(container_id,type,source,destination,read_only,options) VALUES(?,?,?,?,?,?)`, containerID, string(mount.Type), mount.Source, mount.Destination, boolInt(mount.ReadOnly), string(options)); err != nil {
			return err
		}
	}
	for _, label := range snapshot.Labels {
		if _, err := tx.ExecContext(ctx, `INSERT INTO container_labels(container_id,key,value) VALUES(?,?,?)`, containerID, label.Key, label.Value); err != nil {
			return err
		}
	}
	return nil
}
