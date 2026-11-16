package server

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPodmanUIDZeroIsValidButMissingOrNullUIDIsNot(t *testing.T) {
	base := map[string]any{
		"report_id": "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4", "protocol_version": 1, "client_version": "agent",
		"collection_started_at": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), "collection_finished_at": time.Now().UTC().Format(time.RFC3339),
		"host": map[string]any{"status": "failed", "error": "unavailable"}, "disks": map[string]any{"status": "success", "disks": []any{}},
		"interfaces": map[string]any{"status": "success", "interfaces": []any{}}, "filesystems": map[string]any{"status": "success", "filesystems": []any{}},
		"podman": []any{map[string]any{"connection": "rootful", "owner_uid": 0, "status": "success", "containers": []any{}}}, "errors": []any{},
	}
	encode := func(value map[string]any) []byte {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	if _, err := parseSnapshot(encode(base)); err != nil {
		t.Fatalf("UID zero should remain valid: %v", err)
	}
	for name, uid := range map[string]any{"missing": nil, "null": nil} {
		payload := cloneSnapshotMap(t, base)
		podman := payload["podman"].([]any)[0].(map[string]any)
		if name == "missing" {
			delete(podman, "owner_uid")
		} else {
			podman["owner_uid"] = uid
		}
		if _, err := parseSnapshot(encode(payload)); err == nil {
			t.Errorf("accepted Podman owner_uid %s", name)
		}
	}
}

func TestSnapshotRequiresTopLevelCollectionMembersBeforeDTOConversion(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	base := map[string]any{
		"report_id": "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4", "protocol_version": 1, "client_version": "agent",
		"collection_started_at": now.Add(-time.Minute).Format(time.RFC3339), "collection_finished_at": now.Format(time.RFC3339),
		"host": map[string]any{"status": "failed", "error": "unavailable"}, "disks": map[string]any{"status": "success", "disks": []any{}},
		"interfaces": map[string]any{"status": "success", "interfaces": []any{}}, "filesystems": map[string]any{"status": "success", "filesystems": []any{}},
		"podman": []any{}, "errors": []any{},
	}
	for _, field := range []string{"host", "disks", "interfaces", "filesystems", "podman", "errors"} {
		payload := cloneSnapshotMap(t, base)
		delete(payload, field)
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseSnapshot(body); err == nil {
			t.Errorf("accepted omitted %s", field)
		}
	}
	for _, field := range []string{"podman", "errors"} {
		payload := cloneSnapshotMap(t, base)
		payload[field] = []any{}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseSnapshot(body); err != nil {
			t.Errorf("explicit empty %s rejected: %v", field, err)
		}
	}
}

func TestPodmanSuccessRequiresNonNullArrayBeforeAuthority(t *testing.T) {
	base := map[string]any{
		"report_id": "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4", "protocol_version": 1, "client_version": "agent",
		"collection_started_at": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), "collection_finished_at": time.Now().UTC().Format(time.RFC3339),
		"host": map[string]any{"status": "failed", "error": "unavailable"}, "disks": map[string]any{"status": "success", "disks": []any{}},
		"interfaces": map[string]any{"status": "success", "interfaces": []any{}}, "filesystems": map[string]any{"status": "success", "filesystems": []any{}},
		"podman": []any{}, "errors": []any{},
	}
	for name, containers := range map[string]any{"missing": nil, "null": nil, "scalar": "not-an-array"} {
		payload := cloneSnapshotMap(t, base)
		collection := map[string]any{"connection": "rootless", "owner_uid": 1000, "status": "success", "containers": containers}
		if name == "missing" {
			delete(collection, "containers")
		}
		payload["podman"] = []any{collection}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseSnapshot(body); err == nil {
			t.Errorf("accepted %s containers", name)
		}
	}
}

func TestPodmanRejectsNullRequiredContainerScalarsBeforeAcceptance(t *testing.T) {
	container := map[string]any{"name": "web", "runtime_id": "runtime-1", "image_name": "image", "state": "running", "health": "none", "restart_policy": "no", "ports": []any{}, "networks": []any{}, "mounts": []any{}, "labels": []any{}}
	for field, value := range map[string]any{"name": nil, "runtime_id": nil, "state": nil} {
		candidate := make(map[string]any, len(container))
		for key, item := range container {
			candidate[key] = item
		}
		candidate[field] = value
		raw, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if err := validateContainerObject(raw); err == nil {
			t.Errorf("accepted null %s", field)
		}
	}
	missing := map[string]any{"name": "web", "runtime_id": "runtime-1", "image_name": "image", "state": "running", "health": "none", "restart_policy": "no", "ports": []any{map[string]any{"container_port": 8080}}, "networks": []any{}, "mounts": []any{}, "labels": []any{}}
	raw, err := json.Marshal(missing)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateContainerObject(raw); err == nil {
		t.Fatal("accepted a port missing its required protocol")
	}
}

func cloneSnapshotMap(t *testing.T, source map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(encoded, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}
