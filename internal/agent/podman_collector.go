package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const supportedPodmanAPIVersionRange = "4.x-6.x"

type podmanListItem struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	ImageID string   `json:"ImageID"`
}
type podmanInspect struct {
	ID        string `json:"Id"`
	Created   string `json:"Created"`
	Image     string `json:"Image"`
	ImageName string `json:"ImageName"`
	Config    struct {
		Image        string                     `json:"Image"`
		Labels       map[string]string          `json:"Labels"`
		ExposedPorts map[string]json.RawMessage `json:"ExposedPorts"`
	} `json:"Config"`
	State struct {
		Status    string `json:"Status"`
		StartedAt string `json:"StartedAt"`
		Health    *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	HostConfig struct {
		RestartPolicy struct {
			Name              string `json:"Name"`
			MaximumRetryCount int    `json:"MaximumRetryCount"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Networks map[string]struct {
			NetworkID           string `json:"NetworkID"`
			IPAddress           string `json:"IPAddress"`
			IPPrefixLen         int    `json:"IPPrefixLen"`
			GlobalIPv6Address   string `json:"GlobalIPv6Address"`
			GlobalIPv6PrefixLen int    `json:"GlobalIPv6PrefixLen"`
		} `json:"Networks"`
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
	Mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
	ImageDigest string `json:"ImageDigest"`
}

func collectPodman(endpoint PodmanEndpoint) (map[string]any, error) {
	return collectPodmanContext(context.Background(), endpoint)
}

func collectPodmanContext(ctx context.Context, endpoint PodmanEndpoint) (map[string]any, error) {
	info, err := os.Lstat(endpoint.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("Podman socket unavailable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeSocket == 0 {
		return nil, errors.New("configured Podman endpoint is not a Unix socket")
	}
	if info.Mode().Perm()&0o002 != 0 {
		return nil, errors.New("configured Podman socket is world-writable")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != endpoint.OwnerUID {
		return nil, fmt.Errorf("Podman socket owner does not match configured UID %d", endpoint.OwnerUID)
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		dialer := net.Dialer{Timeout: 5 * time.Second}
		return dialer.DialContext(ctx, "unix", endpoint.SocketPath)
	}}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	defer transport.CloseIdleConnections()
	ping, err := podmanGETContext(ctx, client, "http://podman/libpod/_ping", 4*1024)
	if err != nil {
		return nil, fmt.Errorf("Podman API ping failed: %w", err)
	}
	ping.Body.Close()
	apiVersion := ping.Header.Get("Libpod-API-Version")
	apiPrefix, err := podmanAPIPrefix(apiVersion)
	if err != nil {
		return nil, err
	}
	list, err := podmanGETContext(ctx, client, "http://podman"+apiPrefix+"/libpod/containers/json?all=true", 8<<20)
	if err != nil {
		return nil, fmt.Errorf("Podman container listing failed: %w", err)
	}
	var containers []podmanListItem
	if err := decodeBoundedJSON(list.Body, &containers, 8<<20); err != nil {
		list.Body.Close()
		return nil, fmt.Errorf("invalid Podman container list: %w", err)
	}
	list.Body.Close()
	if len(containers) > 10000 {
		return nil, errors.New("Podman returned more than 10000 containers")
	}
	result := make([]map[string]any, 0, len(containers))
	for _, listed := range containers {
		if listed.ID == "" || len(listed.Names) == 0 {
			return nil, errors.New("Podman returned a container without stable ID/name")
		}
		requestPath := "http://podman" + apiPrefix + "/libpod/containers/" + url.PathEscape(listed.ID) + "/json?size=false"
		response, err := podmanGETContext(ctx, client, requestPath, 2<<20)
		if err != nil {
			return nil, fmt.Errorf("Podman inspect failed for a container: %w", err)
		}
		var inspect podmanInspect
		if err := decodeBoundedJSON(response.Body, &inspect, 2<<20); err != nil {
			response.Body.Close()
			return nil, fmt.Errorf("invalid Podman inspect response: %w", err)
		}
		response.Body.Close()
		container, err := normalizePodmanContainer(listed, inspect)
		if err != nil {
			return nil, err
		}
		result = append(result, container)
	}
	return map[string]any{"connection": endpoint.Name, "owner_uid": endpoint.OwnerUID, "status": "success", "containers": result}, nil
}

func podmanAPIPrefix(version string) (string, error) {
	major, _, ok := strings.Cut(version, ".")
	if !ok || (major != "4" && major != "5" && major != "6") {
		return "", fmt.Errorf("unsupported Podman API version %q (supported %s)", version, supportedPodmanAPIVersionRange)
	}
	return "/v" + major + ".0.0", nil
}

func podmanGET(client *http.Client, endpoint string, limit int64) (*http.Response, error) {
	return podmanGETContext(context.Background(), client, endpoint, limit)
}

func podmanGETContext(ctx context.Context, client *http.Client, endpoint string, limit int64) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, fmt.Errorf("Podman API returned HTTP status %d", response.StatusCode)
	}
	// Keep the overflow sentinel visible to decodeBoundedJSON. Truncating at
	// exactly limit could otherwise hide trailing bytes after a valid document.
	response.Body = &boundedReadCloser{ReadCloser: response.Body, reader: io.LimitReader(response.Body, limit+1)}
	return response, nil
}

func decodeBoundedJSON(body io.Reader, target any, limit int64) error {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return errors.New("Podman response exceeds size limit")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing Podman response JSON")
		}
		return err
	}
	return nil
}

type boundedReadCloser struct {
	io.ReadCloser
	reader io.Reader
}

func (r *boundedReadCloser) Read(p []byte) (int, error) { return r.reader.Read(p) }

func normalizePodmanContainer(listed podmanListItem, inspect podmanInspect) (map[string]any, error) {
	name := strings.TrimPrefix(listed.Names[0], "/")
	if name == "" {
		return nil, errors.New("Podman returned an empty container name")
	}
	image := inspect.Config.Image
	if image == "" {
		image = inspect.ImageName
	}
	if image == "" {
		image = listed.Image
	}
	if image == "" {
		return nil, errors.New("Podman omitted a required image name")
	}
	state := normalizeContainerState(inspect.State.Status)
	health := normalizeContainerHealth(inspect.State.Health)
	policy, maximum := normalizeRestartPolicy(inspect.HostConfig.RestartPolicy.Name, inspect.HostConfig.RestartPolicy.MaximumRetryCount)
	container := map[string]any{"name": name, "runtime_id": listed.ID, "image_name": image, "state": state, "health": health, "restart_policy": policy, "ports": []any{}, "networks": []any{}, "mounts": []any{}, "labels": []any{}}
	imageID := inspect.Image
	if imageID == "" {
		imageID = listed.ImageID
	}
	if imageID != "" {
		container["image_id"] = imageID
	}
	digest := inspect.ImageDigest
	if digest != "" {
		container["image_digest"] = digest
	}
	if created := normalizePodmanTime(inspect.Created); created != "" {
		container["created_at"] = created
	}
	if started := normalizePodmanTime(inspect.State.StartedAt); started != "" {
		container["started_at"] = started
	}
	if maximum != nil {
		container["restart_maximum_retry_count"] = *maximum
	}
	ports, err := normalizePodmanPorts(inspect)
	if err != nil {
		return nil, err
	}
	container["ports"] = ports
	networks, err := normalizePodmanNetworks(inspect.NetworkSettings.Networks)
	if err != nil {
		return nil, err
	}
	container["networks"] = networks
	mounts := make([]any, 0, len(inspect.Mounts))
	for _, mount := range inspect.Mounts {
		kind := mount.Type
		if kind != "bind" && kind != "volume" && kind != "tmpfs" {
			kind = "other"
		}
		mounts = append(mounts, map[string]any{"type": kind, "source": mount.Source, "destination": mount.Destination, "read_only": !mount.RW})
	}
	container["mounts"] = mounts
	labels := make([]any, 0, len(inspect.Config.Labels))
	for key, value := range inspect.Config.Labels {
		if sensitiveLabel(key) {
			continue
		}
		if len(key) > 255 || len(value) > 4096 {
			return nil, errors.New("Podman label exceeds supported size")
		}
		labels = append(labels, map[string]any{"key": key, "value": value})
	}
	container["labels"] = labels
	return container, nil
}

func normalizeContainerState(value string) string {
	switch strings.ToLower(value) {
	case "created", "running", "paused", "exited", "stopped":
		return strings.ToLower(value)
	default:
		return "unknown"
	}
}
func normalizeContainerHealth(health *struct {
	Status string `json:"Status"`
}) string {
	if health == nil {
		return "none"
	}
	switch strings.ToLower(health.Status) {
	case "starting", "healthy", "unhealthy":
		return strings.ToLower(health.Status)
	default:
		return "unknown"
	}
}
func normalizeRestartPolicy(policy string, max int) (string, *int) {
	switch strings.ToLower(policy) {
	case "no", "":
		return "no", nil
	case "always":
		return "always", nil
	case "on-failure", "on_failure":
		if max < 0 {
			return "unknown", nil
		}
		return "on_failure", &max
	case "unless-stopped", "unless_stopped":
		return "unless_stopped", nil
	default:
		return "unknown", nil
	}
}
func normalizePodmanTime(value string) string {
	if value == "" || strings.HasPrefix(value, "0001-01-01") {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return ""
	}
	return parsed.UTC().Truncate(time.Second).Format(time.RFC3339)
}
func sensitiveLabel(key string) bool {
	lower := strings.ToLower(key)
	for _, part := range []string{"password", "passwd", "secret", "token", "credential", "private_key", "authorization"} {
		if strings.Contains(lower, part) {
			return true
		}
	}
	return false
}

func normalizePodmanPorts(inspect podmanInspect) ([]any, error) {
	type port struct {
		protocol string
		number   int
	}
	exposed := map[port]bool{}
	for key := range inspect.Config.ExposedPorts {
		numberText, protocol, ok := strings.Cut(key, "/")
		if !ok {
			return nil, errors.New("invalid Podman exposed port")
		}
		number, err := strconv.Atoi(numberText)
		if err != nil || number < 1 || number > 65535 {
			return nil, errors.New("invalid Podman exposed port")
		}
		if protocol != "tcp" && protocol != "udp" && protocol != "sctp" {
			return nil, errors.New("unsupported Podman port protocol")
		}
		exposed[port{protocol, number}] = true
	}
	bindings := map[port][]map[string]any{}
	for key, items := range inspect.NetworkSettings.Ports {
		numberText, protocol, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		number, err := strconv.Atoi(numberText)
		if err != nil || number < 1 || number > 65535 {
			continue
		}
		keyPort := port{protocol, number}
		exposed[keyPort] = true
		for _, item := range items {
			hostPort, err := strconv.Atoi(item.HostPort)
			if err != nil || hostPort < 1 || hostPort > 65535 {
				return nil, errors.New("invalid Podman published host port")
			}
			entry := map[string]any{"host_port": hostPort}
			if item.HostIP != "" {
				ip, err := netip.ParseAddr(item.HostIP)
				if err != nil || ip.Zone() != "" {
					return nil, errors.New("invalid Podman host IP")
				}
				entry["host_ip"] = ip.String()
			}
			bindings[keyPort] = append(bindings[keyPort], entry)
		}
	}
	keys := make([]port, 0, len(exposed))
	for key := range exposed {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].protocol != keys[j].protocol {
			return keys[i].protocol < keys[j].protocol
		}
		return keys[i].number < keys[j].number
	})
	result := make([]any, 0, len(keys))
	for _, key := range keys {
		items := bindings[key]
		if len(items) == 0 {
			result = append(result, map[string]any{"container_port": key.number, "protocol": key.protocol})
			continue
		}
		for _, binding := range items {
			entry := map[string]any{"container_port": key.number, "protocol": key.protocol}
			for name, value := range binding {
				entry[name] = value
			}
			result = append(result, entry)
		}
	}
	return result, nil
}

func normalizePodmanNetworks(networks map[string]struct {
	NetworkID           string `json:"NetworkID"`
	IPAddress           string `json:"IPAddress"`
	IPPrefixLen         int    `json:"IPPrefixLen"`
	GlobalIPv6Address   string `json:"GlobalIPv6Address"`
	GlobalIPv6PrefixLen int    `json:"GlobalIPv6PrefixLen"`
}) ([]any, error) {
	names := make([]string, 0, len(networks))
	for name := range networks {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]any, 0, len(names))
	for _, name := range names {
		network := networks[name]
		addresses := make([]any, 0, 2)
		for _, value := range []struct {
			address string
			prefix  int
		}{{network.IPAddress, network.IPPrefixLen}, {network.GlobalIPv6Address, network.GlobalIPv6PrefixLen}} {
			if value.address == "" {
				continue
			}
			address, prefix, err := parsePodmanAddress(value.address, value.prefix)
			if err != nil {
				return nil, err
			}
			entry := map[string]any{"address": address}
			if prefix != nil {
				entry["prefix_length"] = *prefix
			}
			addresses = append(addresses, entry)
		}
		entry := map[string]any{"name": name, "addresses": addresses}
		if network.NetworkID != "" {
			entry["network_id"] = network.NetworkID
		}
		result = append(result, entry)
	}
	return result, nil
}
func parsePodmanAddress(value string, prefixLength int) (string, *int, error) {
	if prefix, err := netip.ParsePrefix(value); err == nil {
		length := prefix.Bits()
		return prefix.Addr().String(), &length, nil
	}
	address, err := netip.ParseAddr(value)
	if err != nil || address.Zone() != "" {
		return "", nil, errors.New("invalid Podman network address")
	}
	if prefixLength < 0 {
		prefixLength = 0
	}
	if prefixLength > 0 {
		max := 128
		if address.Is4() {
			max = 32
		}
		if prefixLength > max {
			return "", nil, errors.New("invalid Podman network prefix")
		}
		return address.String(), &prefixLength, nil
	}
	return address.String(), nil, nil
}
