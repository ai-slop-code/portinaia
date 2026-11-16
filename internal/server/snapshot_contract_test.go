package server

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSnapshotGeneratedUnionTransportAcceptsSuccessAndPartialFailure(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	started := now.Add(-time.Minute)
	base := map[string]any{
		"report_id":              "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4",
		"protocol_version":       1,
		"client_version":         "portinaia-agent test",
		"collection_started_at":  started.Format(time.RFC3339),
		"collection_finished_at": now.Format(time.RFC3339),
		"host": map[string]any{"status": "success", "host": map[string]any{
			"machine_id": "machine-1", "hostname": "host-1", "os_name": "Linux", "os_version": "test",
			"kernel_version": "test-kernel", "architecture": "arm64", "logical_cpu_count": 2, "memory_bytes": 4096,
		}},
		"disks":       map[string]any{"status": "success", "disks": []any{}},
		"interfaces":  map[string]any{"status": "success", "interfaces": []any{}},
		"filesystems": map[string]any{"status": "success", "filesystems": []any{}},
		"podman":      []any{map[string]any{"connection": "rootless", "owner_uid": 1000, "status": "success", "containers": []any{}}},
		"errors":      []any{},
	}
	body, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseSnapshot(body)
	if err != nil {
		t.Fatalf("parse complete generated-v1 payload: %v", err)
	}
	if parsed.Result != "accepted" || parsed.Request.ReportID != base["report_id"] {
		t.Fatalf("complete report parsed as result=%q id=%q", parsed.Result, parsed.Request.ReportID)
	}

	partial := make(map[string]any, len(base))
	for key, value := range base {
		partial[key] = value
	}
	partial["interfaces"] = map[string]any{"status": "failed", "error": "permission denied"}
	partial["podman"] = []any{map[string]any{"connection": "rootless", "owner_uid": 1000, "status": "failed", "error": "socket unavailable"}}
	partialBody, err := json.Marshal(partial)
	if err != nil {
		t.Fatal(err)
	}
	partialParsed, err := parseSnapshot(partialBody)
	if err != nil {
		t.Fatalf("parse partial-failure generated-v1 payload: %v", err)
	}
	if partialParsed.Result != "accepted_with_errors" {
		t.Fatalf("partial collector failures parsed as %q", partialParsed.Result)
	}
}

func TestSnapshotUnionTransportRejectsUnknownNestedFieldsAndDiscriminators(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	valid := map[string]any{
		"report_id": "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4", "protocol_version": 1, "client_version": "agent",
		"collection_started_at": now.Add(-time.Minute).Format(time.RFC3339), "collection_finished_at": now.Format(time.RFC3339),
		"host":        map[string]any{"status": "failed", "error": "unavailable"},
		"disks":       map[string]any{"status": "success", "disks": []any{}},
		"interfaces":  map[string]any{"status": "success", "interfaces": []any{}},
		"filesystems": map[string]any{"status": "success", "filesystems": []any{}},
		"podman":      []any{}, "errors": []any{},
	}
	for name, collector := range map[string]string{
		"unknown nested field":  `{"status":"failed","error":"unavailable","unexpected":true}`,
		"unknown discriminator": `{"status":"partial","error":"unavailable"}`,
	} {
		payload := make(map[string]any, len(valid))
		for key, value := range valid {
			payload[key] = value
		}
		payload["host"] = json.RawMessage(collector)
		if name == "unknown nested field" {
			payload["host"] = json.RawMessage(`{"status":"failed","error":"unavailable","unexpected":true}`)
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseSnapshot(body); err == nil {
			t.Errorf("snapshot parser accepted %s", name)
		}
	}
}
