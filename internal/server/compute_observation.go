package server

import (
	"context"
	"database/sql"
	"errors"
	"time"

	apigen "portinaia/internal/api/generated"
)

func loadComputeObservation(ctx context.Context, db *sql.DB, computeID string) (*apigen.ComputeObservation, error) {
	var result apigen.ComputeObservation
	var machine, osName, osVersion, kernel, cpuModel, bootID, bootedAt, role sql.NullString
	var cpuCount, memory, uptime sql.NullInt64
	var observedAt string
	err := db.QueryRowContext(ctx, `SELECT machine_id,hostname,os_name,os_version,kernel_version,architecture,cpu_model,logical_cpu_count,memory_bytes,boot_id,booted_at,uptime_seconds,observed_at,virtualization_role FROM compute_observations WHERE compute_id=?`, computeID).Scan(&machine, &result.Hostname, &osName, &osVersion, &kernel, &result.Architecture, &cpuModel, &cpuCount, &memory, &bootID, &bootedAt, &uptime, &observedAt, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	parsed, err := time.Parse(time.RFC3339, observedAt)
	if err != nil {
		return nil, err
	}
	result.ObservedAt = apigen.Timestamp(parsed)
	result.MachineId, result.OsName = nullableString(machine), nullableString(osName)
	result.OsVersion, result.KernelVersion = nullableString(osVersion), nullableString(kernel)
	result.CpuModel, result.BootId = nullableString(cpuModel), nullableString(bootID)
	if cpuCount.Valid {
		result.LogicalCpuCount = int(cpuCount.Int64)
	}
	if memory.Valid {
		result.MemoryBytes = memory.Int64
	}
	if uptime.Valid {
		value := uptime.Int64
		result.UptimeSeconds = &value
	}
	if bootedAt.Valid {
		parsed, err := time.Parse(time.RFC3339, bootedAt.String)
		if err != nil {
			return nil, err
		}
		value := apigen.Timestamp(parsed)
		result.BootedAt = &value
	}
	if role.Valid {
		value := apigen.ComputeObservationVirtualizationRole(role.String)
		result.VirtualizationRole = &value
	}
	result.Interfaces = make([]apigen.NetworkInterfaceObservation, 0)
	rows, err := db.QueryContext(ctx, `SELECT interface_name,mac_address,mtu,is_up FROM compute_interfaces WHERE compute_id=? AND presence_state='present' ORDER BY interface_name`, computeID)
	if err != nil {
		return nil, err
	}
	type interfaceRow struct {
		name, mac sql.NullString
		mtu       sql.NullInt64
		up        sql.NullInt64
	}
	interfaces := make([]interfaceRow, 0)
	for rows.Next() {
		var item interfaceRow
		if err := rows.Scan(&item.name, &item.mac, &item.mtu, &item.up); err != nil {
			rows.Close()
			return nil, err
		}
		interfaces = append(interfaces, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, item := range interfaces {
		iface := apigen.NetworkInterfaceObservation{Name: item.name.String, Addresses: make([]apigen.InterfaceAddress, 0)}
		iface.MacAddress = nullableString(item.mac)
		if item.mtu.Valid {
			value := int(item.mtu.Int64)
			iface.Mtu = &value
		}
		if item.up.Valid {
			value := item.up.Int64 != 0
			iface.Up = &value
		}
		addresses, err := db.QueryContext(ctx, `SELECT address,prefix_length,scope FROM interface_addresses WHERE compute_id=? AND interface_name=? AND presence_state='present' ORDER BY address`, computeID, item.name.String)
		if err != nil {
			return nil, err
		}
		for addresses.Next() {
			var address string
			var prefix sql.NullInt64
			var scope sql.NullString
			if err := addresses.Scan(&address, &prefix, &scope); err != nil {
				addresses.Close()
				return nil, err
			}
			entry := apigen.InterfaceAddress{Address: address}
			if prefix.Valid {
				value := int(prefix.Int64)
				entry.PrefixLength = &value
			}
			if scope.Valid {
				value := apigen.IpScope(scope.String)
				entry.Scope = &value
			}
			iface.Addresses = append(iface.Addresses, entry)
		}
		if err := addresses.Err(); err != nil {
			addresses.Close()
			return nil, err
		}
		addresses.Close()
		result.Interfaces = append(result.Interfaces, iface)
	}
	result.Disks = make([]apigen.DiskObservation, 0)
	disks, err := db.QueryContext(ctx, `SELECT device_name,model,size_bytes FROM compute_disks WHERE compute_id=? AND presence_state='present' ORDER BY device_name`, computeID)
	if err != nil {
		return nil, err
	}
	defer disks.Close()
	for disks.Next() {
		var name string
		var model sql.NullString
		var size int64
		if err := disks.Scan(&name, &model, &size); err != nil {
			return nil, err
		}
		result.Disks = append(result.Disks, apigen.DiskObservation{Name: name, Model: nullableString(model), SizeBytes: size})
	}
	if err := disks.Err(); err != nil {
		return nil, err
	}
	return &result, nil
}
