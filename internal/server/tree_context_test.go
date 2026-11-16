package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"portinaia/internal/database"
)

func TestArchivedLocationContextHonorsComputeParentPlacement(t *testing.T) {
	db, err := database.Open(t.TempDir() + "/tree-context.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	const (
		archivedLocationID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		activeLocationID   = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		parentComputeID    = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		activeVMID         = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	)
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	archivedAt := now
	for _, location := range []struct {
		id, name string
		archived any
	}{{archivedLocationID, "Unrelated archived site", archivedAt}, {activeLocationID, "Active site", nil}} {
		if _, err := db.Exec(`INSERT INTO locations(id,type,name,created_at,updated_at,archived_at) VALUES(?,'site',?,?,?,?)`, location.id, location.name, now, now, location.archived); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO computes(id,kind,display_name,ownership,location_id,created_at,updated_at) VALUES(?,'physical','Parent host','owned',?,?,?)`, parentComputeID, activeLocationID, now, now); err != nil {
		t.Fatal(err)
	}
	// This VM has both location metadata and a compute parent. It belongs under
	// the compute parent, so its location must not pull the unrelated archived
	// location into the default tree as contextual ancestry.
	if _, err := db.Exec(`INSERT INTO computes(id,kind,display_name,ownership,location_id,parent_compute_id,created_at,updated_at) VALUES(?,'vm','Child VM','owned',?,?,?,?)`, activeVMID, archivedLocationID, parentComputeID, now, now); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/v1/inventory/tree", nil)
	roots, _, _, err := loadTreePage(request, db, "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0].ID != activeLocationID {
		t.Fatalf("default roots include unrelated archived location: %+v", roots)
	}
	locationRequest := httptest.NewRequest("GET", "/api/v1/inventory/tree/"+activeLocationID+"/children", nil)
	locationChildren, _, _, err := loadTreePage(locationRequest, db, activeLocationID, "location", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(locationChildren) != 1 || locationChildren[0].ID != parentComputeID {
		t.Fatalf("active location placement = %+v", locationChildren)
	}
	computeRequest := httptest.NewRequest("GET", "/api/v1/inventory/tree/"+parentComputeID+"/children", nil)
	computeChildren, _, _, err := loadTreePage(computeRequest, db, parentComputeID, "compute", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(computeChildren) != 1 || computeChildren[0].ID != activeVMID {
		t.Fatalf("compute-parent placement lost VM: %+v", computeChildren)
	}
	var activeComputes int
	if err := db.QueryRow(`SELECT count(*) FROM computes WHERE archived_at IS NULL`).Scan(&activeComputes); err != nil || activeComputes != 2 {
		t.Fatalf("context filtering changed active count = %d err=%v", activeComputes, err)
	}
}

func TestArchivedLocationAndComputeAncestorsKeepActiveVMAndContainerReachable(t *testing.T) {
	db, err := database.Open(t.TempDir() + "/tree-context.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	const (
		locationID  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		hostID      = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		vmID        = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		containerID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	)
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO locations(id,type,name,created_at,updated_at,archived_at) VALUES(?,'site','Archived site',?,?,?)`, []any{locationID, now, now, now}},
		{`INSERT INTO computes(id,kind,display_name,ownership,location_id,created_at,updated_at,archived_at) VALUES(?,'physical','Archived host','owned',?,?,?,?)`, []any{hostID, locationID, now, now, now}},
		{`INSERT INTO computes(id,kind,display_name,ownership,parent_compute_id,created_at,updated_at) VALUES(?,'vm','Active VM','owned',?,?,?)`, []any{vmID, hostID, now, now}},
		{`INSERT INTO containers(id,compute_id,podman_owner_uid,podman_connection,name,runtime_id,state,health,restart_policy,first_observed_at,last_observed_at,created_at,updated_at) VALUES(?,?,0,'root','Active container','runtime-id','running','none','no',?,?,?,?)`, []any{containerID, vmID, now, now, now, now}},
	} {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest("GET", "/api/v1/inventory/tree", nil)
	roots, _, _, err := loadTreePage(request, db, "", "", true)
	if err != nil || len(roots) != 1 || roots[0].ID != locationID || !roots[0].Archived {
		t.Fatalf("archived location context = %+v, err=%v", roots, err)
	}
	children, _, _, err := loadTreePage(httptest.NewRequest("GET", "/", nil), db, locationID, "location", false)
	if err != nil || len(children) != 1 || children[0].ID != hostID || !children[0].Archived {
		t.Fatalf("archived host context = %+v, err=%v", children, err)
	}
	children, _, _, err = loadTreePage(httptest.NewRequest("GET", "/", nil), db, hostID, "compute", false)
	if err != nil || len(children) != 1 || children[0].ID != vmID || children[0].Archived {
		t.Fatalf("active VM under archived host = %+v, err=%v", children, err)
	}
	children, _, _, err = loadTreePage(httptest.NewRequest("GET", "/", nil), db, vmID, "compute", false)
	if err != nil || len(children) != 1 || children[0].ID != containerID {
		t.Fatalf("active container under VM = %+v, err=%v", children, err)
	}
	var activeComputes, activeContainers int
	if err := db.QueryRow(`SELECT count(*) FROM computes WHERE archived_at IS NULL`).Scan(&activeComputes); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM containers WHERE archived_at IS NULL`).Scan(&activeContainers); err != nil {
		t.Fatal(err)
	}
	if activeComputes != 1 || activeContainers != 1 {
		t.Fatalf("context nodes inflated active counts: computes=%d containers=%d", activeComputes, activeContainers)
	}
}

func TestArchivedLocationAndAllArchivedComputeChainKeepActiveContainerReachable(t *testing.T) {
	db, err := database.Open(t.TempDir() + "/tree-context.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	const (
		locationID  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		hostID      = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		vmID        = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		containerID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	)
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO locations(id,type,name,created_at,updated_at,archived_at) VALUES(?,'site','Archived site',?,?,?)`, []any{locationID, now, now, now}},
		{`INSERT INTO computes(id,kind,display_name,ownership,location_id,created_at,updated_at,archived_at) VALUES(?,'physical','Archived host','owned',?,?,?,?)`, []any{hostID, locationID, now, now, now}},
		{`INSERT INTO computes(id,kind,display_name,ownership,parent_compute_id,created_at,updated_at,archived_at) VALUES(?,'vm','Archived VM','owned',?,?,?,?)`, []any{vmID, hostID, now, now, now}},
		{`INSERT INTO containers(id,compute_id,podman_owner_uid,podman_connection,name,runtime_id,state,health,restart_policy,first_observed_at,last_observed_at,created_at,updated_at) VALUES(?,?,0,'root','Active container','runtime-id','running','none','no',?,?,?,?)`, []any{containerID, vmID, now, now, now, now}},
	} {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	roots, _, _, err := loadTreePage(httptest.NewRequest("GET", "/api/v1/inventory/tree", nil), db, "", "", true)
	if err != nil || len(roots) != 1 || roots[0].ID != locationID {
		t.Fatalf("archived location context = %+v, err=%v", roots, err)
	}
	children, _, _, err := loadTreePage(httptest.NewRequest("GET", "/", nil), db, locationID, "location", false)
	if err != nil || len(children) != 1 || children[0].ID != hostID {
		t.Fatalf("archived host context = %+v, err=%v", children, err)
	}
	children, _, _, err = loadTreePage(httptest.NewRequest("GET", "/", nil), db, hostID, "compute", false)
	if err != nil || len(children) != 1 || children[0].ID != vmID {
		t.Fatalf("archived VM context = %+v, err=%v", children, err)
	}
	children, _, _, err = loadTreePage(httptest.NewRequest("GET", "/", nil), db, vmID, "compute", false)
	if err != nil || len(children) != 1 || children[0].ID != containerID {
		t.Fatalf("active container = %+v, err=%v", children, err)
	}
}
