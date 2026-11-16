//go:build linux

package agent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"portinaia/internal/version"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type collectorError struct {
	Collector string `json:"collector"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

func CollectSnapshot(podmanEndpoints ...PodmanEndpoint) ([]byte, error) {
	return CollectSnapshotContext(context.Background(), podmanEndpoints...)
}

func CollectSnapshotContext(ctx context.Context, podmanEndpoints ...PodmanEndpoint) ([]byte, error) {
	if err := ValidatePodmanEndpoints(podmanEndpoints); err != nil {
		return nil, err
	}
	started := time.Now().UTC().Truncate(time.Second)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	host, hostErr := collectHost()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	disks, diskErr := collectDisks()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	interfaces, interfaceErr := collectInterfaces()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filesystems, filesystemErr := collectFilesystems()
	errorsList := make([]collectorError, 0)
	if hostErr != nil {
		errorsList = append(errorsList, collectorError{"host", "collection_failed", sanitizeError(hostErr.Error())})
	}
	if diskErr != nil {
		errorsList = append(errorsList, collectorError{"disks", "collection_failed", sanitizeError(diskErr.Error())})
	}
	if interfaceErr != nil {
		errorsList = append(errorsList, collectorError{"interfaces", "collection_failed", sanitizeError(interfaceErr.Error())})
	}
	if filesystemErr != nil {
		errorsList = append(errorsList, collectorError{"filesystems", "collection_failed", sanitizeError(filesystemErr.Error())})
	}
	writeCollector := func(value any, err error) any {
		if err != nil {
			return map[string]any{"status": "failed", "error": sanitizeError(err.Error())}
		}
		return value
	}
	var hostResult any
	if hostErr != nil {
		hostResult = writeCollector(nil, hostErr)
	} else {
		hostResult = map[string]any{"status": "success", "host": host}
	}
	podmanCollections := make([]any, 0, len(podmanEndpoints))
	for _, endpoint := range podmanEndpoints {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		collection, err := collectPodmanContext(ctx, endpoint)
		if err != nil {
			podmanCollections = append(podmanCollections, map[string]any{"connection": endpoint.Name, "owner_uid": endpoint.OwnerUID, "status": "failed", "error": sanitizeError(err.Error())})
			errorsList = append(errorsList, collectorError{"podman:" + strconv.FormatUint(uint64(endpoint.OwnerUID), 10), "podman_collection_failed", sanitizeError(err.Error())})
		} else {
			podmanCollections = append(podmanCollections, collection)
		}
	}
	payload := map[string]any{
		"report_id": newReportID(), "protocol_version": protocolVersion, "client_version": version.String(),
		"collection_started_at": started.Format(time.RFC3339), "collection_finished_at": time.Now().UTC().Truncate(time.Second).Format(time.RFC3339),
		"host":        hostResult,
		"disks":       writeCollector(diskCollection("success", disks), diskErr),
		"interfaces":  writeCollector(interfaceCollection("success", interfaces), interfaceErr),
		"filesystems": writeCollector(filesystemCollection("success", filesystems), filesystemErr),
		// An unconfigured Podman connection is not reported as an authoritative empty inventory.
		"podman": podmanCollections, "errors": errorsList,
	}
	return json.Marshal(payload)
}

func collectHost() (map[string]any, error) {
	machineID, err := readTrimmed("/etc/machine-id")
	if err != nil || machineID == "" {
		return nil, fmt.Errorf("machine identity unavailable")
	}
	values, err := readOSRelease("/etc/os-release")
	if err != nil {
		return nil, fmt.Errorf("read operating system release: %w", err)
	}
	osName := values["PRETTY_NAME"]
	if osName == "" {
		osName = values["ID"]
	}
	if osName == "" {
		return nil, errors.New("operating system identity is unavailable")
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return nil, errors.New("hostname unavailable")
	}
	kernel, err := readTrimmed("/proc/sys/kernel/osrelease")
	if err != nil || kernel == "" {
		return nil, errors.New("kernel version unavailable")
	}
	cpuInfo, err := readBoundedFile("/proc/cpuinfo", 4<<20)
	if err != nil {
		return nil, fmt.Errorf("read cpu information: %w", err)
	}
	cpuCount, cpuModel := parseCPUInfo(string(cpuInfo))
	if cpuCount < 1 {
		cpuCount = runtime.NumCPU()
	}
	if cpuCount < 1 {
		return nil, errors.New("logical CPU count unavailable")
	}
	memInfo, err := readBoundedFile("/proc/meminfo", 1<<20)
	if err != nil {
		return nil, fmt.Errorf("read memory information: %w", err)
	}
	memoryBytes, err := parseMemoryBytes(string(memInfo))
	if err != nil {
		return nil, err
	}
	host := map[string]any{"machine_id": machineID, "hostname": hostname, "os_name": osName, "os_version": values["VERSION_ID"], "kernel_version": kernel, "architecture": runtime.GOARCH, "logical_cpu_count": cpuCount, "memory_bytes": memoryBytes, "virtualization_role": "unknown"}
	if cpuModel != "" {
		host["cpu_model"] = cpuModel
	}
	if bootID, err := readTrimmed("/proc/sys/kernel/random/boot_id"); err == nil && bootID != "" {
		host["boot_id"] = bootID
	}
	if uptime, err := readUptimeSeconds("/proc/uptime"); err == nil {
		host["uptime_seconds"] = uptime
		if booted := time.Now().UTC().Add(-time.Duration(uptime) * time.Second); !booted.After(time.Now().UTC()) {
			host["booted_at"] = booted.Truncate(time.Second).Format(time.RFC3339)
		}
	}
	return host, nil
}

func readOSRelease(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			unquoted, err := strconv.Unquote(value)
			if err == nil {
				value = unquoted
			} else {
				value = value[1 : len(value)-1]
			}
		}
		values[key] = value
	}
	return values, nil
}
func readTrimmed(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
func parseCPUInfo(text string) (int, string) {
	count := 0
	model := ""
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "processor":
			count++
		case "model name", "Hardware":
			if model == "" {
				model = strings.TrimSpace(value)
			}
		}
	}
	return count, model
}
func parseMemoryBytes(text string) (int64, error) {
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "MemTotal" {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) < 2 || fields[1] != "kB" {
			return 0, errors.New("malformed MemTotal")
		}
		kilobytes, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil || kilobytes > math.MaxInt64/1024 {
			return 0, errors.New("invalid or overflowing MemTotal")
		}
		return int64(kilobytes * 1024), nil
	}
	return 0, errors.New("MemTotal is missing")
}
func readUptimeSeconds(path string) (int64, error) {
	value, err := readTrimmed(path)
	if err != nil {
		return 0, err
	}
	field, _, _ := strings.Cut(value, " ")
	seconds, err := strconv.ParseFloat(field, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > float64(math.MaxInt64)/float64(time.Second) {
		return 0, errors.New("invalid uptime")
	}
	return int64(seconds), nil
}

func collectInterfaces() ([]map[string]any, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].Name < interfaces[j].Name })
	result := make([]map[string]any, 0, len(interfaces))
	for _, iface := range interfaces {
		addresses, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("enumerate addresses for %s: %w", iface.Name, err)
		}
		facts := make([]map[string]any, 0, len(addresses))
		for _, item := range addresses {
			prefix, err := netip.ParsePrefix(item.String())
			if err != nil {
				if address, addrErr := netip.ParseAddr(item.String()); addrErr == nil {
					prefix = netip.PrefixFrom(address, address.BitLen())
				} else {
					return nil, fmt.Errorf("parse address on %s: %w", iface.Name, err)
				}
			}
			address := prefix.Addr()
			if address.Zone() != "" {
				address = address.WithZone("")
			}
			entry := map[string]any{"address": address.String(), "prefix_length": prefix.Bits()}
			scope := addressScope(address)
			if scope != "" {
				entry["scope"] = scope
			}
			facts = append(facts, entry)
		}
		sort.Slice(facts, func(i, j int) bool { return facts[i]["address"].(string) < facts[j]["address"].(string) })
		up := iface.Flags&net.FlagUp != 0
		entry := map[string]any{"name": iface.Name, "addresses": facts, "up": up}
		if iface.MTU >= 68 {
			entry["mtu"] = iface.MTU
		}
		if iface.HardwareAddr != nil && len(iface.HardwareAddr) > 0 {
			entry["mac_address"] = iface.HardwareAddr.String()
		}
		result = append(result, entry)
	}
	return result, nil
}
func addressScope(address netip.Addr) string {
	if address.IsLoopback() {
		return "loopback"
	}
	if address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() {
		return "link_local"
	}
	if address.IsPrivate() {
		return "private"
	}
	if address.IsGlobalUnicast() {
		return "public"
	}
	return "other"
}

func collectDisks() ([]map[string]any, error) {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "fd") {
			continue
		}
		base := filepath.Join("/sys/block", name)
		raw, err := readTrimmed(filepath.Join(base, "size"))
		if err != nil {
			return nil, fmt.Errorf("read disk size for %s: %w", name, err)
		}
		sectors, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || sectors > math.MaxInt64/512 {
			return nil, fmt.Errorf("invalid or overflowing disk size for %s", name)
		}
		disk := map[string]any{"name": "/dev/" + name, "size_bytes": int64(sectors * 512)}
		if model, err := readTrimmed(filepath.Join(base, "device/model")); err == nil && model != "" {
			disk["model"] = model
		}
		result = append(result, disk)
	}
	sort.Slice(result, func(i, j int) bool { return result[i]["name"].(string) < result[j]["name"].(string) })
	return result, nil
}

func collectFilesystems() ([]map[string]any, error) {
	data, err := readBoundedFile("/proc/self/mounts", 4<<20)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0)
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		device, mount, filesystem := unescapeMount(fields[0]), unescapeMount(fields[1]), fields[2]
		key := device + "\x00" + mount
		if seen[key] {
			continue
		}
		seen[key] = true
		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount, &stat); err != nil {
			return nil, fmt.Errorf("stat filesystem %s: %w", mount, err)
		}
		blockSize := uint64(stat.Bsize)
		if blockSize == 0 || stat.Blocks > math.MaxInt64/blockSize || stat.Bavail > math.MaxInt64/blockSize {
			return nil, fmt.Errorf("filesystem size overflow at %s", mount)
		}
		result = append(result, map[string]any{"device": device, "filesystem": filesystem, "mount_point": mount, "total_bytes": int64(stat.Blocks * blockSize), "available_bytes": int64(stat.Bavail * blockSize)})
	}
	if len(result) > 4096 {
		return nil, errors.New("filesystem mount count exceeds supported limit")
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i]["device"].(string) + "\x00" + result[i]["mount_point"].(string)
		right := result[j]["device"].(string) + "\x00" + result[j]["mount_point"].(string)
		return left < right
	})
	return result, nil
}
func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds supported size", path)
	}
	return data, nil
}
func unescapeMount(value string) string {
	return strings.NewReplacer("\\040", " ", "\\011", "\t", "\\012", "\n", "\\134", "\\").Replace(value)
}

func newReportID() string {
	var bytes [16]byte
	_, _ = rand.Read(bytes[:])
	milliseconds := uint64(time.Now().UTC().UnixMilli())
	bytes[0] = byte(milliseconds >> 40)
	bytes[1] = byte(milliseconds >> 32)
	bytes[2] = byte(milliseconds >> 24)
	bytes[3] = byte(milliseconds >> 16)
	bytes[4] = byte(milliseconds >> 8)
	bytes[5] = byte(milliseconds)
	bytes[6] = (bytes[6] & 0x0f) | 0x70
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	hexValue := fmt.Sprintf("%x", bytes)
	return hexValue[:8] + "-" + hexValue[8:12] + "-" + hexValue[12:16] + "-" + hexValue[16:20] + "-" + hexValue[20:]
}
func sanitizeError(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if len(value) > 2000 {
		return value[:2000]
	}
	if value == "" {
		return "collection failed"
	}
	return value
}

func interfaceCollection(status string, interfaces []map[string]any) any {
	return map[string]any{"status": status, "interfaces": interfaces}
}
func diskCollection(status string, disks []map[string]any) any {
	return map[string]any{"status": status, "disks": disks}
}
func filesystemCollection(status string, filesystems []map[string]any) any {
	return map[string]any{"status": status, "filesystems": filesystems}
}
