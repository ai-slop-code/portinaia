package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"portinaia/internal/database"
	"portinaia/internal/server"
)

func TestServeSignalHelper(t *testing.T) {
	if os.Getenv("PORTINAIA_SERVE_SIGNAL_HELPER") != "1" {
		return
	}
	cfg := config{
		listenAddress:              os.Getenv("PORTINAIA_TEST_LISTEN"),
		databasePath:               os.Getenv("PORTINAIA_TEST_DATABASE"),
		publicURL:                  "http://127.0.0.1:8080",
		discoveryRetentionDays:     90,
		discoveryRetentionBatch:    1000,
		discoveryRetentionInterval: time.Hour,
	}
	if err := serve(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestServeSIGTERMDrainsAnInFlightDatabaseMutation(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "shutdown.db")
	db, err := database.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(context.Background(), db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := serverBootstrapForShutdownTest(db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestServeSignalHelper$")
	command.Env = append(os.Environ(), "PORTINAIA_SERVE_SIGNAL_HELPER=1", "PORTINAIA_TEST_LISTEN="+address, "PORTINAIA_TEST_DATABASE="+databasePath)
	var processOutput bytes.Buffer
	command.Stdout, command.Stderr = &processOutput, &processOutput
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	baseURL := "http://" + address
	client, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{Jar: client, Timeout: 5 * time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, requestErr := httpClient.Get(baseURL + "/api/v1/health/ready")
		if requestErr == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			command.Process.Kill()
			t.Fatalf("server did not become ready: %v %s", requestErr, processOutput.String())
		}
		time.Sleep(25 * time.Millisecond)
	}
	loginBody := []byte(`{"username":"shutdown-admin","password":"shutdown-test-password"}`)
	loginRequest, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/auth/login", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	loginRequest.Header.Set("Content-Type", "application/json")
	loginRequest.Header.Set("Origin", baseURL)
	loginResponse, err := httpClient.Do(loginRequest)
	if err != nil {
		t.Fatal(err)
	}
	var login struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.NewDecoder(loginResponse.Body).Decode(&login); err != nil {
		loginResponse.Body.Close()
		t.Fatal(err)
	}
	loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", loginResponse.StatusCode)
	}

	writer, err := database.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	tx, err := writer.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(context.Background(), `UPDATE administrators SET updated_at=updated_at WHERE username='shutdown-admin'`); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	mutationRequest, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/providers", bytes.NewReader([]byte(`{"name":"Drained provider","roles":["hosting_provider"],"tag_ids":[]}`)))
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	mutationRequest.Header.Set("Content-Type", "application/json")
	mutationRequest.Header.Set("X-CSRF-Token", login.CSRFToken)
	mutationDone := make(chan struct {
		status int
		err    error
	}, 1)
	go func() {
		response, err := httpClient.Do(mutationRequest)
		if err != nil {
			mutationDone <- struct {
				status int
				err    error
			}{err: err}
			return
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		mutationDone <- struct {
			status int
			err    error
		}{status: response.StatusCode}
	}()
	select {
	case result := <-mutationDone:
		tx.Rollback()
		t.Fatalf("write completed before the held SQLite writer lock was released: %+v", result)
	case <-time.After(500 * time.Millisecond):
	}
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-mutationDone:
		if result.err != nil || result.status != http.StatusCreated {
			t.Fatalf("in-flight mutation was not drained successfully: status=%d err=%v; server=%s", result.status, result.err, processOutput.String())
		}
	case <-time.After(5 * time.Second):
		command.Process.Kill()
		t.Fatal("server did not finish in-flight request after SIGTERM")
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("server shutdown: %v; output=%s", err, processOutput.String())
		}
	case <-time.After(5 * time.Second):
		command.Process.Kill()
		t.Fatal("server process did not exit after draining the request")
	}
	check, err := database.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var providerCount int
	if err := check.QueryRow(`SELECT count(*) FROM providers WHERE name='Drained provider'`).Scan(&providerCount); err != nil || providerCount != 1 {
		t.Fatalf("drained mutation persistence = %d err=%v", providerCount, err)
	}
}

func serverBootstrapForShutdownTest(db *sql.DB) error {
	return server.BootstrapAdministrator(db, "shutdown-admin", "shutdown-test-password")
}
