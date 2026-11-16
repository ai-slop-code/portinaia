package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContainerResponseKeepsRequiredCollectionsAndPageContract(t *testing.T) {
	item := containerResponse{
		ID: "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b4", ComputeID: "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73b5",
		Name: "empty", TagIDs: []string{}, Observation: &containerObservationResponse{
			RuntimeID: "runtime", ImageName: "image", State: "stopped", Health: "none", RestartPolicy: "no",
			FirstObservedAt: "2026-09-30T00:00:00Z", LastObservedAt: "2026-09-30T00:00:00Z",
			Ports: []containerPortResponse{}, Networks: []containerNetworkResponse{}, Mounts: []containerMountResponse{}, Labels: []containerLabelResponse{},
		},
	}
	page := map[string]any{"items": []containerResponse{item}, "page": map[string]any{"size": 50, "has_more": false, "next_cursor": nil}}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, required := range []string{`"tag_ids":[]`, `"ports":[]`, `"networks":[]`, `"mounts":[]`, `"labels":[]`, `"next_cursor":null`, `"has_more":false`} {
		if !strings.Contains(text, required) {
			t.Errorf("container response lacks contract member %s: %s", required, text)
		}
	}
}
