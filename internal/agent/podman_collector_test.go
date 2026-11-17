package agent

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPodmanResponseLimitDoesNotHideTrailingData(t *testing.T) {
	const document = `{"ok":true}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(document + "x"))
	}))
	defer server.Close()
	response, err := podmanGET(server.Client(), server.URL, int64(len(document)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var decoded map[string]any
	if err := decodeBoundedJSON(response.Body, &decoded, int64(len(document))); err == nil {
		t.Fatal("accepted a truncated document with hidden trailing bytes")
	}
}

func TestPodmanCollectorUsesReadOnlyUnixAPIAndRedactsLabels(t *testing.T) {
	methods := []string{}
	server, socketPath := fakePodmanServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodGet {
			t.Errorf("mutating Podman API request: %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/libpod/_ping":
			w.Header().Set("Libpod-API-Version", "4.9.0")
			w.WriteHeader(http.StatusOK)
		case "/v4.0.0/libpod/containers/json":
			_, _ = w.Write([]byte(`[{"Id":"runtime-1","Names":["/web"],"Image":"nginx:latest","ImageID":"sha256:image"}]`))
		case "/v4.0.0/libpod/containers/runtime-1/json":
			_, _ = w.Write([]byte(`{"Id":"runtime-1","Created":"2026-09-26T00:00:00Z","Image":"sha256:image","ImageName":"nginx:latest","ImageDigest":"sha256:digest","Config":{"Image":"nginx:latest","Labels":{"app":"demo","secret.token":"do-not-return"},"ExposedPorts":{"80/tcp":{}}},"State":{"Status":"running","StartedAt":"2026-09-26T00:01:00Z","Health":{"Status":"healthy"}},"HostConfig":{"RestartPolicy":{"Name":"on-failure","MaximumRetryCount":3}},"NetworkSettings":{"Networks":{"bridge":{"NetworkID":"network-1","IPAddress":"10.0.0.2","IPPrefixLen":24}},"Ports":{"80/tcp":[{"HostIp":"0.0.0.0","HostPort":"8080"}]}},"Mounts":[{"Type":"bind","Source":"/srv/site","Destination":"/usr/share/nginx/html","RW":false}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	collection, err := collectPodman(PodmanEndpoint{Name: "rootful", OwnerUID: uint32(os.Getuid()), SocketPath: socketPath})
	if err != nil {
		t.Fatal(err)
	}
	if collection["status"] != "success" || collection["owner_uid"] != uint32(os.Getuid()) {
		t.Fatalf("collection metadata = %#v", collection)
	}
	containers := collection["containers"].([]map[string]any)
	if len(containers) != 1 {
		t.Fatalf("containers = %#v", containers)
	}
	container := containers[0]
	if container["name"] != "web" || container["state"] != "running" || container["restart_policy"] != "on_failure" {
		t.Fatalf("container identity/state = %#v", container)
	}
	labels := container["labels"].([]any)
	encoded, _ := json.Marshal(labels)
	if strings.Contains(string(encoded), "do-not-return") || !strings.Contains(string(encoded), "demo") {
		t.Fatalf("labels not safely normalized: %s", encoded)
	}
	ports := container["ports"].([]any)
	if len(ports) != 1 || !strings.Contains(string(mustJSON(t, ports)), "8080") {
		t.Fatalf("ports = %#v", ports)
	}
	networks := container["networks"].([]any)
	if len(networks) != 1 || !strings.Contains(string(mustJSON(t, networks)), "10.0.0.2") {
		t.Fatalf("networks = %#v", networks)
	}
	if len(methods) != 3 {
		t.Fatalf("Podman API calls = %#v", methods)
	}
}

func TestPodmanCollectorDiscardsPartialConnectionOnInspectFailure(t *testing.T) {
	server, socketPath := fakePodmanServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/libpod/_ping":
			w.Header().Set("Libpod-API-Version", "5.0.0")
		case "/v5.0.0/libpod/containers/json":
			_, _ = w.Write([]byte(`[{"Id":"runtime-1","Names":["/web"],"Image":"nginx:latest"},{"Id":"runtime-2","Names":["/db"],"Image":"postgres:latest"}]`))
		case "/v5.0.0/libpod/containers/runtime-1/json":
			_, _ = w.Write([]byte(`{"Id":"runtime-1","Config":{"Image":"nginx:latest"},"State":{"Status":"running"},"HostConfig":{"RestartPolicy":{"Name":"no"}},"NetworkSettings":{}}`))
		case "/v5.0.0/libpod/containers/runtime-2/json":
			http.Error(w, "private socket detail must not leak", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	collection, err := collectPodman(PodmanEndpoint{Name: "rootless", OwnerUID: uint32(os.Getuid()), SocketPath: socketPath})
	if err == nil || collection != nil {
		t.Fatalf("partial Podman result was returned: %#v, %v", collection, err)
	}
	if strings.Contains(err.Error(), "private socket detail") {
		t.Fatalf("raw Podman error leaked: %v", err)
	}
}

func TestPodmanAPIPrefixSupportsVersionsFourThroughSix(t *testing.T) {
	for _, version := range []string{"4.9.0", "5.4.2", "6.1.1"} {
		prefix, err := podmanAPIPrefix(version)
		if err != nil || prefix != "/v"+version[:1]+".0.0" {
			t.Errorf("podmanAPIPrefix(%q) = %q, %v", version, prefix, err)
		}
	}
	if _, err := podmanAPIPrefix("7.0.0"); err == nil {
		t.Fatal("unsupported Podman API major accepted")
	}
}

func fakePodmanServer(t *testing.T, handler http.Handler) (*http.Server, string) {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "pa-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	socketPath := filepath.Join(directory, "s")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close(); _ = listener.Close() })
	return server, socketPath
}
func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	result, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
