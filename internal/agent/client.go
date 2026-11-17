package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/propagation"

	"portinaia/internal/identity"
	"portinaia/internal/logging"
	"portinaia/internal/version"
)

const protocolVersion = 1

type Client struct {
	baseURL string
	http    *http.Client
}

type HTTPError struct {
	Operation  string
	StatusCode int
	Code       string
	Message    string
	RequestID  string
	RetryAfter time.Duration
	Permanent  bool
}

func (e *HTTPError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s failed (%d, %s): %s", e.Operation, e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("%s failed (%d): %s", e.Operation, e.StatusCode, e.Message)
}

type enrollmentResponse struct {
	AgentID              string `json:"agent_id"`
	ComputeID            string `json:"compute_id"`
	Credential           string `json:"credential"`
	CredentialGeneration int64  `json:"credential_generation"`
	ServerTime           string `json:"server_time"`
	AcceptedProtocol     struct {
		Minimum int `json:"minimum"`
		Maximum int `json:"maximum"`
	} `json:"accepted_protocol"`
	Schedule struct {
		HeartbeatIntervalSeconds int `json:"heartbeat_interval_seconds"`
		SnapshotIntervalSeconds  int `json:"snapshot_interval_seconds"`
	} `json:"schedule"`
}

func NewClient(serverURL string) (*Client, error) {
	return newClient(serverURL, false)
}

// NewClientAllowInsecureHTTP enables HTTP for explicitly trusted private
// networks such as a VPN. HTTPS remains the default for every other client.
func NewClientAllowInsecureHTTP(serverURL string) (*Client, error) {
	return newClient(serverURL, true)
}

func newClient(serverURL string, allowInsecureHTTP bool) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(serverURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, errors.New("server URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("server URL cannot contain credentials, query, or fragment")
	}
	if parsed.Scheme != "https" && !isLoopbackHost(parsed.Hostname()) && !allowInsecureHTTP {
		return nil, errors.New("HTTPS is required except for loopback development servers")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return &Client{baseURL: parsed.String(), http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func Enroll(serverURL, tokenFile, stateFile, clientVersion string) error {
	return EnrollContext(context.Background(), serverURL, tokenFile, stateFile, clientVersion)
}

func EnrollContext(ctx context.Context, serverURL, tokenFile, stateFile, clientVersion string) error {
	return enrollContext(ctx, serverURL, tokenFile, stateFile, clientVersion, false)
}

func EnrollAllowInsecureHTTP(serverURL, tokenFile, stateFile, clientVersion string) error {
	return enrollContext(context.Background(), serverURL, tokenFile, stateFile, clientVersion, true)
}

func enrollContext(ctx context.Context, serverURL, tokenFile, stateFile, clientVersion string, allowInsecureHTTP bool) error {
	if _, err := os.Lstat(stateFile); err == nil {
		return errors.New("credential state already exists; use it or explicitly remove it before re-enrollment")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect credential state: %w", err)
	}
	tokenBytes, err := readPrivateSecret(tokenFile)
	if err != nil {
		return err
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return errors.New("enrollment token file is empty")
	}
	client, err := newClient(serverURL, allowInsecureHTTP)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"protocol_version": protocolVersion, "client_version": clientVersion})
	if err != nil {
		return err
	}
	response, err := client.request(ctx, http.MethodPost, "/api/v1/agent/v1/enroll", token, payload)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return responseError("enroll", response)
	}
	var enrollment enrollmentResponse
	responseData, err := readResponseDocument(response.Body)
	if err != nil {
		return fmt.Errorf("decode enrollment response: %w", err)
	}
	var enrollmentMembers map[string]json.RawMessage
	if err := json.Unmarshal(responseData, &enrollmentMembers); err != nil {
		return fmt.Errorf("decode enrollment response: %w", err)
	}
	if err := requireMembers(enrollmentMembers, "agent_id", "compute_id", "credential", "credential_generation", "server_time", "accepted_protocol", "schedule"); err != nil {
		return fmt.Errorf("decode enrollment response: %w", err)
	}
	if err := decodeFull(bytes.NewReader(responseData), &enrollment); err != nil {
		return fmt.Errorf("decode enrollment response: %w", err)
	}
	if !validUUID(enrollment.AgentID) || !validUUID(enrollment.ComputeID) || enrollment.Credential == "" || enrollment.CredentialGeneration < 1 || enrollment.AcceptedProtocol.Minimum != protocolVersion || enrollment.AcceptedProtocol.Maximum != protocolVersion || !validServerTime(enrollment.ServerTime) || !validSchedule(enrollment.Schedule.HeartbeatIntervalSeconds, enrollment.Schedule.SnapshotIntervalSeconds) {
		return errors.New("server returned an incomplete or incompatible enrollment response")
	}
	state := CredentialState{Version: stateVersion, ServerURL: strings.TrimRight(serverURL, "/"), AllowInsecureHTTP: allowInsecureHTTP, AgentID: enrollment.AgentID, ComputeID: enrollment.ComputeID, Credential: enrollment.Credential, CredentialGeneration: enrollment.CredentialGeneration, ProtocolVersion: protocolVersion, ClientVersion: clientVersion}
	if err := SaveState(stateFile, state); err != nil {
		return fmt.Errorf("enrollment succeeded but credential state could not be saved; replace the token and enroll again: %w", err)
	}
	return nil
}

func Heartbeat(stateFile string) error {
	return HeartbeatContext(context.Background(), stateFile)
}

func HeartbeatContext(ctx context.Context, stateFile string) error {
	state, err := LoadState(stateFile)
	if err != nil {
		return err
	}
	client, err := newClient(state.ServerURL, state.AllowInsecureHTTP)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"protocol_version": state.ProtocolVersion, "client_version": version.String(), "sent_at": time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		return err
	}
	response, err := client.request(ctx, http.MethodPost, "/api/v1/agent/v1/heartbeat", state.Credential, payload)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return responseError("heartbeat", response)
	}
	var protocol struct {
		ServerTime       string `json:"server_time"`
		AcceptedProtocol *struct {
			Minimum int `json:"minimum"`
			Maximum int `json:"maximum"`
		} `json:"accepted_protocol"`
		Schedule *struct {
			HeartbeatIntervalSeconds int `json:"heartbeat_interval_seconds"`
			SnapshotIntervalSeconds  int `json:"snapshot_interval_seconds"`
		} `json:"schedule"`
		Warnings []string `json:"warnings"`
	}
	if err := decodeFull(response.Body, &protocol); err != nil {
		return fmt.Errorf("decode heartbeat response: %w", err)
	}
	if !validServerTime(protocol.ServerTime) || protocol.AcceptedProtocol == nil || protocol.AcceptedProtocol.Minimum < 1 || protocol.AcceptedProtocol.Maximum < protocol.AcceptedProtocol.Minimum || protocol.AcceptedProtocol.Minimum > protocolVersion || protocol.AcceptedProtocol.Maximum < protocolVersion || protocol.Schedule == nil || !validSchedule(protocol.Schedule.HeartbeatIntervalSeconds, protocol.Schedule.SnapshotIntervalSeconds) || protocol.Warnings == nil {
		return errors.New("server returned an incomplete or incompatible heartbeat response")
	}
	return nil
}

func UploadSnapshot(stateFile string, snapshot []byte) error {
	return UploadSnapshotContext(context.Background(), stateFile, snapshot)
}

func UploadSnapshotContext(ctx context.Context, stateFile string, snapshot []byte) error {
	state, err := LoadState(stateFile)
	if err != nil {
		return err
	}
	var report struct {
		ReportID string `json:"report_id"`
	}
	if err := json.Unmarshal(snapshot, &report); err != nil || report.ReportID == "" {
		return errors.New("snapshot is missing report_id")
	}
	client, err := newClient(state.ServerURL, state.AllowInsecureHTTP)
	if err != nil {
		return err
	}
	response, err := client.request(ctx, http.MethodPut, "/api/v1/agent/v1/snapshots/"+report.ReportID, state.Credential, snapshot)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return responseError("snapshot upload", response)
	}
	var acceptance struct {
		ID                   string `json:"id"`
		ReportID             string `json:"report_id"`
		ProtocolVersion      int    `json:"protocol_version"`
		ClientVersion        string `json:"client_version"`
		CollectionStartedAt  string `json:"collection_started_at"`
		CollectionFinishedAt string `json:"collection_finished_at"`
		Result               string `json:"result"`
		ReceivedAt           string `json:"received_at"`
		ServerTime           string `json:"server_time"`
		AcceptedProtocol     *struct {
			Minimum int `json:"minimum"`
			Maximum int `json:"maximum"`
		} `json:"accepted_protocol"`
		Schedule *struct {
			HeartbeatIntervalSeconds int `json:"heartbeat_interval_seconds"`
			SnapshotIntervalSeconds  int `json:"snapshot_interval_seconds"`
		} `json:"schedule"`
		ContentReusedPresent *bool `json:"content_reused"`
	}
	if err := decodeFull(response.Body, &acceptance); err != nil {
		return fmt.Errorf("decode snapshot acceptance: %w", err)
	}
	if !validUUID(acceptance.ID) || acceptance.ReportID != report.ReportID || acceptance.ProtocolVersion != protocolVersion || acceptance.ClientVersion == "" || !validServerTime(acceptance.CollectionStartedAt) || !validServerTime(acceptance.CollectionFinishedAt) || (acceptance.Result != "accepted" && acceptance.Result != "accepted_with_errors") || acceptance.ContentReusedPresent == nil || !validServerTime(acceptance.ReceivedAt) || !validServerTime(acceptance.ServerTime) || acceptance.AcceptedProtocol == nil || acceptance.AcceptedProtocol.Minimum < 1 || acceptance.AcceptedProtocol.Maximum < acceptance.AcceptedProtocol.Minimum || acceptance.AcceptedProtocol.Minimum > protocolVersion || acceptance.AcceptedProtocol.Maximum < protocolVersion || acceptance.Schedule == nil || !validSchedule(acceptance.Schedule.HeartbeatIntervalSeconds, acceptance.Schedule.SnapshotIntervalSeconds) {
		return errors.New("server acknowledged a different snapshot report ID")
	}
	return nil
}

func Snapshot(stateFile string, podmanEndpoints ...PodmanEndpoint) error {
	return SnapshotContext(context.Background(), stateFile, podmanEndpoints...)
}

func SnapshotContext(ctx context.Context, stateFile string, podmanEndpoints ...PodmanEndpoint) error {
	lock, err := lockSnapshotState(ctx, stateFile)
	if err != nil {
		return err
	}
	defer lock.Close()
	pending, err := LoadPendingSnapshot(stateFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Every cycle collects independently. A complete newer report atomically supersedes
	// the old pending generation even when its upload previously failed.
	collectionCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	newest, collectErr := CollectSnapshotContext(collectionCtx, podmanEndpoints...)
	cancel()
	if collectErr != nil {
		if pending == nil {
			return collectErr
		}
	} else {
		pending = newest
		if err = SavePendingSnapshot(stateFile, pending); err != nil {
			return fmt.Errorf("save pending snapshot: %w", err)
		}
	}
	if pending == nil {
		return errors.New("no pending snapshot available")
	}
	var marker struct {
		ReportID string `json:"report_id"`
	}
	if err := json.Unmarshal(pending, &marker); err != nil {
		return err
	}
	if err := UploadSnapshotContext(ctx, stateFile, pending); err != nil {
		return err
	}
	return ClearPendingSnapshotIfMatch(stateFile, marker.ReportID)
}

func (c *Client) request(ctx context.Context, method, path, bearer string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if id := logging.RequestID(ctx); logging.ValidRequestID(id) {
		req.Header.Set("X-Request-ID", id)
	}
	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(req.Header))
	return c.http.Do(req)
}

func responseError(operation string, response *http.Response) error {
	var problem struct {
		Code   string `json:"code"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 16*1024)).Decode(&problem)
	message := problem.Detail
	if message == "" {
		message = problem.Title
	}
	if message == "" {
		message = http.StatusText(response.StatusCode)
	}
	permanent := response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusConflict || response.StatusCode == http.StatusUnprocessableEntity || response.StatusCode == http.StatusBadRequest
	requestID := response.Header.Get("X-Request-ID")
	if !logging.ValidRequestID(requestID) {
		requestID = ""
	}
	return &HTTPError{Operation: operation, StatusCode: response.StatusCode, Code: problem.Code, Message: message, RequestID: requestID, RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now()), Permanent: permanent}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	trimmed := strings.TrimSpace(value)
	if seconds, err := strconv.ParseUint(trimmed, 10, 64); err == nil {
		if seconds >= 3600 {
			return time.Hour
		}
		return time.Duration(seconds) * time.Second
	} else if len(trimmed) > 0 && strings.Trim(trimmed, "0123456789") == "" {
		return time.Hour
	}
	if at, err := http.ParseTime(value); err == nil {
		d := at.Sub(now)
		if d < 0 {
			return 0
		}
		if d > time.Hour {
			return time.Hour
		}
		return d
	}
	return 0
}

func decodeFull(body io.Reader, target any) error {
	data, err := readResponseDocument(body)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("response contains trailing JSON")
		}
		return err
	}
	return nil
}

func readResponseDocument(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("response exceeds size limit")
	}
	return data, nil
}

func validServerTime(value string) bool { _, err := time.Parse(time.RFC3339, value); return err == nil }
func validSchedule(heartbeat, snapshot int) bool {
	return heartbeat >= 15 && heartbeat <= 600 && snapshot >= 60 && snapshot <= 86400
}
func validUUID(value string) bool {
	return identity.Valid(value)
}

func readPrivateSecret(path string) ([]byte, error) {
	file, err := openPrivateFile(path)
	if err != nil {
		return nil, fmt.Errorf("open enrollment token file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("enrollment token file must be regular and private (0600)")
	}
	if err := validateOwner(info); err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(file, 16*1024))
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
