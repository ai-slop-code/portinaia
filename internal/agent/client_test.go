package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnrollPersistsPrivateCredentialAndHeartbeatUsesIt(t *testing.T) {
	var enrollmentRequests, heartbeatRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/v1/enroll":
			enrollmentRequests++
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer one-time-token" {
				t.Errorf("enroll request method/auth = %s/%q", r.Method, r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusCreated)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"agent_id":"00000000-0000-4000-8000-000000000011","compute_id":"00000000-0000-4000-8000-000000000012","credential":"long-agent-credential","credential_generation":1,"server_time":"2026-09-27T00:00:00Z","accepted_protocol":{"minimum":1,"maximum":1},"schedule":{"heartbeat_interval_seconds":60,"snapshot_interval_seconds":3600}}`))
		case "/api/v1/agent/v1/heartbeat":
			heartbeatRequests++
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer long-agent-credential" {
				t.Errorf("heartbeat method/auth = %s/%q", r.Method, r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"server_time":"2026-09-27T00:00:00Z","accepted_protocol":{"minimum":1,"maximum":1},"schedule":{"heartbeat_interval_seconds":60,"snapshot_interval_seconds":3600},"warnings":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	directory := t.TempDir()
	tokenPath := filepath.Join(directory, "enrollment-token")
	if err := os.WriteFile(tokenPath, []byte("one-time-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(directory, "private", "credential.json")
	if err := Enroll(server.URL, tokenPath, statePath, "test-agent"); err != nil {
		t.Fatal(err)
	}
	state, err := LoadState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state.AgentID != "00000000-0000-4000-8000-000000000011" || state.ComputeID != "00000000-0000-4000-8000-000000000012" || state.CredentialGeneration != 1 {
		t.Fatalf("credential state = %+v", state)
	}
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credential file mode = %o", info.Mode().Perm())
	}
	if err := Heartbeat(statePath); err != nil {
		t.Fatal(err)
	}
	if enrollmentRequests != 1 || heartbeatRequests != 1 {
		t.Fatalf("request counts enrollment=%d heartbeat=%d", enrollmentRequests, heartbeatRequests)
	}
	if raw, err := os.ReadFile(statePath); err != nil {
		t.Fatal(err)
	} else if strings.Contains(string(raw), "one-time-token") {
		t.Fatal("state persisted the consumed enrollment token")
	}
}

func TestStateRejectsUnsafePermissionsAndUnknownFields(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "state.json")
	state := CredentialState{Version: stateVersion, ServerURL: "https://inventory.example", AgentID: "agent", ComputeID: "compute", Credential: "credential", CredentialGeneration: 1, ProtocolVersion: 1, ClientVersion: "test"}
	if err := SaveState(path, state); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(path); err == nil {
		t.Fatal("loaded world-readable credential file")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-1], []byte(`,"unknown":"value"}`)...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(path); err == nil {
		t.Fatal("loaded state with unknown field")
	}
}

func TestPendingSnapshotIsOnePrivateReplaceableReport(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state", "credential.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	report := []byte(`{"report_id":"00000000-0000-4000-8000-000000000001","protocol_version":1,"client_version":"test","collection_started_at":"2026-09-27T00:00:00Z","collection_finished_at":"2026-09-27T00:00:01Z"}`)
	if err := SavePendingSnapshot(statePath, report); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPendingSnapshot(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded) != string(report) {
		t.Fatalf("loaded pending report = %s", loaded)
	}
	info, err := os.Stat(statePath + ".pending")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("pending report permissions = %o", info.Mode().Perm())
	}
	newer := []byte(`{"report_id":"00000000-0000-4000-8000-000000000002","protocol_version":1,"client_version":"test","collection_started_at":"2026-09-27T00:00:00Z","collection_finished_at":"2026-09-27T00:00:01Z"}`)
	if err := SavePendingSnapshot(statePath, newer); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadPendingSnapshot(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded) != string(newer) {
		t.Fatalf("replacement pending report = %s", loaded)
	}
	if err := ClearPendingSnapshot(statePath); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPendingSnapshot(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("load after clear error = %v", err)
	}
}

func TestStaleAcknowledgementCannotClearSupersedingReport(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state", "credential.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	old := []byte(`{"report_id":"00000000-0000-4000-8000-000000000001","protocol_version":1,"client_version":"test","collection_started_at":"2026-09-27T00:00:00Z","collection_finished_at":"2026-09-27T00:00:01Z"}`)
	newer := []byte(`{"report_id":"00000000-0000-4000-8000-000000000002","protocol_version":1,"client_version":"test","collection_started_at":"2026-09-27T00:00:00Z","collection_finished_at":"2026-09-27T00:00:01Z"}`)
	if err := SavePendingSnapshot(statePath, old); err != nil {
		t.Fatal(err)
	}
	if err := SavePendingSnapshot(statePath, newer); err != nil {
		t.Fatal(err)
	}
	if err := ClearPendingSnapshotIfMatch(statePath, "00000000-0000-4000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadPendingSnapshot(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(newer) {
		t.Fatalf("stale ack removed newer report: %s", got)
	}
}

func TestPodmanEndpointRequiresExplicitUniqueOwnerAndSocket(t *testing.T) {
	endpoint, err := ParsePodmanEndpoint("rootless=1000:/run/user/1000/podman/podman.sock")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Name != "rootless" || endpoint.OwnerUID != 1000 || endpoint.SocketPath != "/run/user/1000/podman/podman.sock" {
		t.Fatalf("endpoint = %+v", endpoint)
	}
	if _, err := ParsePodmanEndpoint("rootful=uid:/run/podman/podman.sock"); err == nil {
		t.Fatal("accepted a nonnumeric Podman owner")
	}
	if err := ValidatePodmanEndpoints([]PodmanEndpoint{endpoint, {Name: "other", OwnerUID: 1000, SocketPath: "/run/other.sock"}}); err == nil {
		t.Fatal("accepted duplicate owner namespaces")
	}
}

func TestRetryAfterIsBoundedBeforeDurationConversion(t *testing.T) {
	for _, value := range []string{"9223372036854775807", "18446744073709551615", "999999999999999999999999999999"} {
		if got := parseRetryAfter(value, time.Now()); got != time.Hour {
			t.Errorf("parseRetryAfter(%q) = %s, want one-hour cap", value, got)
		}
	}
}

func TestLoadStateRejectsMissingNullAndTrailingCredentialData(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "state.json")
	valid := `{"version":1,"server_url":"https://example.test","agent_id":"00000000-0000-4000-8000-000000000011","compute_id":"00000000-0000-4000-8000-000000000012","credential":"credential","credential_generation":1,"protocol_version":1,"client_version":"test"}`
	for name, document := range map[string]string{
		"missing required member": strings.Replace(valid, `,"credential":"credential"`, "", 1),
		"null required member":    strings.Replace(valid, `"credential":"credential"`, `"credential":null`, 1),
		"trailing JSON":           valid + ` {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadState(path); err == nil {
				t.Fatalf("accepted malformed credential document %s", document)
			}
		})
	}
}

func TestEnrollmentRejectsMissingRequiredResponseMember(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"agent_id":"00000000-0000-4000-8000-000000000011","compute_id":"00000000-0000-4000-8000-000000000012","credential":"credential","credential_generation":1,"server_time":"2026-09-27T00:00:00Z","accepted_protocol":{"minimum":1,"maximum":1}}`))
	}))
	defer server.Close()
	directory := t.TempDir()
	token := filepath.Join(directory, "token")
	if err := os.WriteFile(token, []byte("token"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Enroll(server.URL, token, filepath.Join(directory, "state", "credential.json"), "test")
	if err == nil || !strings.Contains(err.Error(), "schedule") {
		t.Fatalf("Enroll error = %v, want missing schedule", err)
	}
}

func TestPendingSnapshotRejectsMalformedCollectorUnionAndTrailingData(t *testing.T) {
	base := `{"report_id":"00000000-0000-4000-8000-000000000001","protocol_version":1,"client_version":"test","collection_started_at":"2026-09-27T00:00:00Z","collection_finished_at":"2026-09-27T00:00:01Z"}`
	union := strings.TrimSuffix(base, "}") + `,"disks":{"status":"success","error":"also failed"}}`
	if err := validatePendingReport([]byte(union)); err == nil {
		t.Fatal("accepted collector union containing both success and error members")
	}
	if err := validatePendingReport([]byte(base + ` {}`)); err == nil {
		t.Fatal("accepted trailing pending-document JSON")
	}
}
