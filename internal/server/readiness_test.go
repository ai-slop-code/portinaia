package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"portinaia/internal/database"
)

func TestReadinessFailsItsDatabaseWriteProbeOnReadOnlySQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "readonly.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(context.Background(), db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	readOnlyURL := (&url.URL{Scheme: "file", Path: path}).String() + "?mode=ro&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	readOnly, err := sql.Open("sqlite", readOnlyURL)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	handler := NewHandler(readOnly)

	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))
	var problem problemResponse
	if err := json.NewDecoder(ready.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	if ready.Code != http.StatusServiceUnavailable || problem.Readiness == nil || problem.Readiness.Checks[0].Status != "ready" || problem.Readiness.Checks[1].Status != "not_ready" {
		t.Fatalf("readiness did not report a failed write check: %d %+v", ready.Code, problem)
	}
	live := httptest.NewRecorder()
	handler.ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))
	if live.Code != http.StatusOK {
		t.Fatalf("liveness status = %d, want 200", live.Code)
	}
}
