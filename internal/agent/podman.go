package agent

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type PodmanEndpoint struct {
	Name       string
	OwnerUID   uint32
	SocketPath string
}

// ParsePodmanEndpoint parses the explicit NAME=UID:/absolute/socket syntax.
func ParsePodmanEndpoint(value string) (PodmanEndpoint, error) {
	var endpoint PodmanEndpoint
	name, identity, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(name) == "" {
		return endpoint, errors.New("Podman socket must use NAME=UID:/absolute/socket syntax")
	}
	uidText, path, ok := strings.Cut(identity, ":")
	if !ok || path == "" || !filepath.IsAbs(path) {
		return endpoint, errors.New("Podman socket path must be absolute")
	}
	uid, err := strconv.ParseUint(uidText, 10, 32)
	if err != nil {
		return endpoint, errors.New("Podman owner UID must be a numeric uint32")
	}
	if len(name) > 200 || strings.TrimSpace(name) != name {
		return endpoint, errors.New("Podman connection name must be 1-200 characters without surrounding whitespace")
	}
	for _, character := range name {
		if character < 0x20 || character == 0x7f {
			return endpoint, errors.New("Podman connection name contains control characters")
		}
	}
	return PodmanEndpoint{Name: name, OwnerUID: uint32(uid), SocketPath: path}, nil
}

func ValidatePodmanEndpoints(endpoints []PodmanEndpoint) error {
	names := map[string]bool{}
	uids := map[uint32]bool{}
	for _, endpoint := range endpoints {
		if endpoint.Name == "" || endpoint.SocketPath == "" {
			return errors.New("Podman endpoint requires a name and socket path")
		}
		if names[endpoint.Name] {
			return fmt.Errorf("duplicate Podman connection name %q", endpoint.Name)
		}
		if uids[endpoint.OwnerUID] {
			return fmt.Errorf("only one Podman connection per owner UID is currently supported (UID %d)", endpoint.OwnerUID)
		}
		names[endpoint.Name] = true
		uids[endpoint.OwnerUID] = true
	}
	return nil
}
