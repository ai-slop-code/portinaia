package server

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	apigen "portinaia/internal/api/generated"

	"github.com/go-chi/chi/v5"
)

const (
	maxSnapshotBodyBytes = 32 << 20
	reportReplayWindow   = 90 * 24 * time.Hour
)

type snapshotSubmission struct {
	ReportID             string                   `json:"report_id"`
	ProtocolVersion      int                      `json:"protocol_version"`
	ClientVersion        string                   `json:"client_version"`
	CollectionStartedAt  string                   `json:"collection_started_at"`
	CollectionFinishedAt string                   `json:"collection_finished_at"`
	Host                 json.RawMessage          `json:"host"`
	Disks                json.RawMessage          `json:"disks"`
	Interfaces           json.RawMessage          `json:"interfaces"`
	Filesystems          json.RawMessage          `json:"filesystems"`
	Podman               []json.RawMessage        `json:"podman"`
	Errors               []apigen.CollectionError `json:"errors"`
}

type snapshotCollector struct {
	Key        string
	Name       string
	OwnerUID   *int64
	Connection *string
	Success    bool
	Code       string
	Message    string
}

type parsedSnapshot struct {
	Request      snapshotSubmission
	Host         *apigen.HostSnapshot
	Disks        []apigen.DiskSnapshot
	Interfaces   []apigen.NetworkInterfaceObservation
	Filesystems  []apigen.FilesystemSnapshot
	Podman       []parsedPodman
	Collectors   []snapshotCollector
	Structural   []byte
	Fingerprint  string
	ErrorSummary string
	Result       string
}

type parsedPodman struct {
	Connection string
	OwnerUID   int64
	Success    bool
	Code       string
	Message    string
	Containers []apigen.SnapshotContainer
}

type unionHeader struct {
	Status string `json:"status"`
}
type hostSuccess struct {
	Status string              `json:"status"`
	Host   apigen.HostSnapshot `json:"host"`
}
type collectorFailure struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}
type diskSuccess struct {
	Status string                `json:"status"`
	Disks  []apigen.DiskSnapshot `json:"disks"`
}
type interfaceSuccess struct {
	Status     string                               `json:"status"`
	Interfaces []apigen.NetworkInterfaceObservation `json:"interfaces"`
}
type filesystemSuccess struct {
	Status      string                      `json:"status"`
	Filesystems []apigen.FilesystemSnapshot `json:"filesystems"`
}
type podmanSuccess struct {
	Connection string                     `json:"connection"`
	OwnerUID   int64                      `json:"owner_uid"`
	Status     string                     `json:"status"`
	Containers []apigen.SnapshotContainer `json:"containers"`
}
type podmanFailure struct {
	Connection string `json:"connection"`
	OwnerUID   int64  `json:"owner_uid"`
	Status     string `json:"status"`
	Error      string `json:"error"`
}

func putAgentSnapshotHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		credential, ok := bearerCredential(r)
		if !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		body, err := readLimitedBody(w, r, maxSnapshotBodyBytes)
		if err != nil {
			return
		}
		parsed, err := parseSnapshot(body)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		pathReportID := chi.URLParam(r, "report_id")
		if parsed.Request.ReportID != pathReportID {
			writeProblemForRequest(w, r, 409, "report_id_mismatch", "Path report ID does not match the request report ID.")
			return
		}
		if parsed.Request.ProtocolVersion != acceptedProtocolVersion {
			http.Error(w, "unsupported protocol version", 422)
			return
		}
		now := time.Now().UTC().Truncate(time.Second)
		received := now.Format(time.RFC3339)
		started, err := time.Parse(time.RFC3339, parsed.Request.CollectionStartedAt)
		if err != nil {
			http.Error(w, "invalid collection_started_at", 422)
			return
		}
		finished, err := time.Parse(time.RFC3339, parsed.Request.CollectionFinishedAt)
		if err != nil || finished.Before(started) || finished.After(now.Add(5*time.Minute)) {
			http.Error(w, "invalid collection time range", 422)
			return
		}
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		defer tx.Rollback()
		var agentID, computeID string
		var agentProtocol int
		err = tx.QueryRowContext(r.Context(), `SELECT a.id,a.compute_id,a.protocol_version FROM agents a JOIN computes c ON c.id=a.compute_id WHERE a.credential_hash=? AND a.revoked_at IS NULL AND c.archived_at IS NULL`, hashSecret(credential)).Scan(&agentID, &computeID, &agentProtocol)
		if err != nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		if agentProtocol != parsed.Request.ProtocolVersion {
			http.Error(w, "agent protocol does not match uploaded snapshot", 409)
			return
		}
		var priorID, priorClient, priorStarted, priorFinished, priorResult, priorReceived string
		var priorReused sql.NullInt64
		var priorFingerprint sql.NullString
		var priorEnvelope int
		err = tx.QueryRowContext(r.Context(), `SELECT id,client_version,collection_started_at,collection_finished_at,result,received_at,content_reused,request_fingerprint,envelope_version FROM discovery_reports WHERE agent_id=? AND report_id=?`, agentID, pathReportID).Scan(&priorID, &priorClient, &priorStarted, &priorFinished, &priorResult, &priorReceived, &priorReused, &priorFingerprint, &priorEnvelope)
		if err == nil {
			if priorEnvelope != 1 || !priorFingerprint.Valid || priorFingerprint.String != parsed.Fingerprint {
				writeProblemForRequest(w, r, 409, "report_id_conflict", "This report ID was already accepted with different content.")
				return
			}
			var reused bool
			if priorReused.Valid {
				reused = priorReused.Int64 == 1
			}
			writeJSON(w, 200, snapshotAcceptance(priorID, pathReportID, parsed.Request.ProtocolVersion, priorClient, priorStarted, priorFinished, priorResult, priorReused.Valid && reused, priorReceived))
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "internal error", 500)
			return
		}
		if !reportWithinReplayWindow(started, now) {
			http.Error(w, "report is outside the 90-day retry window; collect a fresh report", 409)
			return
		}
		contentHash := sha256.Sum256(parsed.Structural)
		contentHashHex := hex.EncodeToString(contentHash[:])
		compressed, err := compressSnapshot(parsed.Structural)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		contentID, err := randomID()
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		result, err := tx.ExecContext(r.Context(), `INSERT INTO snapshot_contents(id,format_version,content_hash,compressed_document,uncompressed_size,created_at) VALUES(?,1,?,?,?,?) ON CONFLICT(format_version,content_hash) DO NOTHING`, contentID, contentHashHex, compressed, len(parsed.Structural), received)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		inserted, _ := result.RowsAffected()
		reused := inserted == 0
		if reused {
			if err := tx.QueryRowContext(r.Context(), `SELECT id FROM snapshot_contents WHERE format_version=1 AND content_hash=?`, contentHashHex).Scan(&contentID); err != nil {
				http.Error(w, "internal error", 500)
				return
			}
		}
		reportID, err := randomID()
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		reportResult := parsed.Result
		collectionFinished := finished.UTC().Truncate(time.Second).Format(time.RFC3339)
		collectionStarted := started.UTC().Truncate(time.Second).Format(time.RFC3339)
		_, err = tx.ExecContext(r.Context(), `INSERT INTO discovery_reports(id,report_id,agent_id,protocol_version,client_version,received_at,client_collected_at,collection_started_at,collection_finished_at,snapshot_content_id,result,content_reused,request_fingerprint,envelope_version,collection_error_summary) VALUES(?,?,?,?,?,?,?,?,?,?,?, ?,?,1,?)`, reportID, pathReportID, agentID, parsed.Request.ProtocolVersion, parsed.Request.ClientVersion, received, collectionFinished, collectionStarted, collectionFinished, contentID, reportResult, boolInt(reused), parsed.Fingerprint, nullIfEmpty(parsed.ErrorSummary))
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if err := persistCollectorResults(r.Context(), tx, reportID, parsed.Collectors, received); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if parsed.Host != nil {
			if err := applyHostSnapshot(r.Context(), tx, computeID, agentID, reportID, *parsed.Host, received); err != nil {
				http.Error(w, "invalid host facts", 422)
				return
			}
		}
		if err := applyInterfaces(r.Context(), tx, computeID, reportID, parsed.Interfaces, collectorSuccess(parsed.Collectors, "interfaces"), received); err != nil {
			http.Error(w, "invalid interface facts", 422)
			return
		}
		if err := applyDisks(r.Context(), tx, computeID, reportID, parsed.Disks, collectorSuccess(parsed.Collectors, "disks"), received); err != nil {
			http.Error(w, "invalid disk facts", 422)
			return
		}
		if err := applyFilesystems(r.Context(), tx, computeID, reportID, parsed.Filesystems, collectorSuccess(parsed.Collectors, "filesystems"), received); err != nil {
			http.Error(w, "invalid filesystem facts", 422)
			return
		}
		for _, podman := range parsed.Podman {
			if err := applyPodmanCollection(r.Context(), tx, computeID, reportID, podman, received); err != nil {
				http.Error(w, "invalid Podman facts", 422)
				return
			}
		}
		var uptime any
		for _, collector := range parsed.Collectors {
			if collector.Name == "host" && collector.Success && parsed.Host != nil && parsed.Host.UptimeSeconds != nil {
				uptime = *parsed.Host.UptimeSeconds
				break
			}
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO discovery_report_measurements(report_id,host_uptime_seconds) VALUES(?,?)`, reportID, uptime); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		for _, filesystem := range parsed.Filesystems {
			if collectorSuccess(parsed.Collectors, "filesystems") {
				if _, err = tx.ExecContext(r.Context(), `INSERT INTO discovery_report_filesystem_measurements(report_id,device,mount_point,available_bytes) VALUES(?,?,?,?)`, reportID, filesystem.Device, filesystem.MountPoint, filesystem.AvailableBytes); err != nil {
					http.Error(w, "internal error", 500)
					return
				}
			}
		}
		if _, err = tx.ExecContext(r.Context(), `UPDATE agents SET last_successful_snapshot_at=?,last_error_summary=? WHERE id=? AND revoked_at IS NULL`, received, nullIfEmpty(parsed.ErrorSummary), agentID); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if err := AppendAuditTx(r.Context(), tx, AuditEntry{OccurredAt: received, ActorType: "system", ActorID: systemActorID, Action: "discovery.snapshot.accept", ResourceType: "discovery_report", ResourceID: reportID, RequestID: requestIdentity(r.Context()), After: map[string]any{"result": reportResult}, Result: "success"}); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		writeJSON(w, 201, snapshotAcceptance(reportID, pathReportID, parsed.Request.ProtocolVersion, parsed.Request.ClientVersion, collectionStarted, collectionFinished, reportResult, reused, received))
	}
}

func reportWithinReplayWindow(started, now time.Time) bool {
	return !started.Before(now.Add(-reportReplayWindow))
}

func readLimitedBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, "invalid or oversized request body", status)
		return nil, err
	}
	return body, nil
}

func snapshotAcceptance(historyID, reportID string, protocol int, clientVersion, started, finished, result string, reused bool, serverTime string) map[string]any {
	return map[string]any{"id": historyID, "report_id": reportID, "protocol_version": protocol, "client_version": clientVersion, "collection_started_at": started, "collection_finished_at": finished, "result": result, "received_at": serverTime, "content_reused": reused, "server_time": serverTime, "accepted_protocol": protocolRange{1, 1}, "schedule": agentSchedule{heartbeatInterval, snapshotInterval}}
}

func parseSnapshot(body []byte) (parsedSnapshot, error) {
	if err := validateSnapshotRequiredMembers(body); err != nil {
		return parsedSnapshot{}, err
	}
	var transport apigen.DiscoverySnapshotWrite
	if err := decodeStrictRequest(body, &transport); err != nil {
		return parsedSnapshot{}, fmt.Errorf("invalid snapshot: %w", err)
	}
	if transport.CollectionStartedAt.IsZero() || transport.CollectionFinishedAt.IsZero() {
		return parsedSnapshot{}, errors.New("collection timestamps are required")
	}
	host, err := json.Marshal(transport.Host)
	if err != nil {
		return parsedSnapshot{}, errors.New("invalid host collector result")
	}
	disks, err := json.Marshal(transport.Disks)
	if err != nil {
		return parsedSnapshot{}, errors.New("invalid disk collector result")
	}
	interfaces, err := json.Marshal(transport.Interfaces)
	if err != nil {
		return parsedSnapshot{}, errors.New("invalid interface collector result")
	}
	filesystems, err := json.Marshal(transport.Filesystems)
	if err != nil {
		return parsedSnapshot{}, errors.New("invalid filesystem collector result")
	}
	podman := make([]json.RawMessage, len(transport.Podman))
	for index, collection := range transport.Podman {
		encoded, err := json.Marshal(collection)
		if err != nil {
			return parsedSnapshot{}, errors.New("invalid Podman collector result")
		}
		podman[index] = encoded
	}
	request := snapshotSubmission{
		ReportID: transport.ReportId.String(), ProtocolVersion: transport.ProtocolVersion,
		ClientVersion:        transport.ClientVersion,
		CollectionStartedAt:  transport.CollectionStartedAt.Format(time.RFC3339Nano),
		CollectionFinishedAt: transport.CollectionFinishedAt.Format(time.RFC3339Nano),
		Host:                 host, Disks: disks, Interfaces: interfaces, Filesystems: filesystems,
		Podman: podman, Errors: transport.Errors,
	}
	if err := validateGeneratedSnapshotUnions(request); err != nil {
		return parsedSnapshot{}, err
	}
	request.ClientVersion = strings.TrimSpace(request.ClientVersion)
	if request.ProtocolVersion < 1 || len(request.ClientVersion) == 0 || len(request.ClientVersion) > 100 {
		return parsedSnapshot{}, errors.New("invalid protocol_version or client_version")
	}
	if request.ReportID == "" || !validUUIDString(request.ReportID) {
		return parsedSnapshot{}, errors.New("report_id must be a UUID")
	}
	if request.Podman == nil || request.Errors == nil {
		return parsedSnapshot{}, errors.New("podman and errors arrays are required")
	}
	if len(request.Podman) > 64 || len(request.Errors) > 256 {
		return parsedSnapshot{}, errors.New("snapshot exceeds collector count limits")
	}
	parsed := parsedSnapshot{Request: request}
	errorsByCollector := map[string]apigen.CollectionError{}
	for _, item := range request.Errors {
		if item.Collector == "" || len(item.Collector) > 200 || !validCollectorCode(item.Code) || item.Message == "" || len(item.Message) > 2000 {
			return parsedSnapshot{}, errors.New("invalid collector diagnostic")
		}
		errorsByCollector[item.Collector] = item
	}
	var status unionHeader
	if err := json.Unmarshal(request.Host, &status); err != nil {
		return parsedSnapshot{}, errors.New("invalid host collector result")
	}
	switch status.Status {
	case "success":
		var success hostSuccess
		if err := decodeStrict(request.Host, &success); err != nil {
			return parsedSnapshot{}, err
		}
		if success.Host.MachineId == "" || success.Host.Hostname == "" || success.Host.OsName == "" || success.Host.KernelVersion == "" || success.Host.Architecture == "" || success.Host.LogicalCpuCount < 1 || success.Host.MemoryBytes < 0 {
			return parsedSnapshot{}, errors.New("incomplete host collection")
		}
		parsed.Host = &success.Host
		parsed.Collectors = append(parsed.Collectors, snapshotCollector{Key: "host", Name: "host", Success: true})
	case "failed":
		var failed collectorFailure
		if err := decodeStrict(request.Host, &failed); err != nil {
			return parsedSnapshot{}, err
		}
		parsed.Collectors = append(parsed.Collectors, failureCollector("host", "host", nil, nil, failed.Error, errorsByCollector))
	default:
		return parsedSnapshot{}, errors.New("invalid host collector status")
	}
	var diskStatus unionHeader
	if err := json.Unmarshal(request.Disks, &diskStatus); err != nil {
		return parsedSnapshot{}, errors.New("invalid disk collector result")
	}
	switch diskStatus.Status {
	case "success":
		var success diskSuccess
		if err := decodeStrict(request.Disks, &success); err != nil {
			return parsedSnapshot{}, err
		}
		if success.Disks == nil {
			return parsedSnapshot{}, errors.New("disks must be an array")
		}
		if len(success.Disks) > 1024 {
			return parsedSnapshot{}, errors.New("disk collection exceeds item limit")
		}
		parsed.Disks = success.Disks
		sort.Slice(parsed.Disks, func(i, j int) bool { return parsed.Disks[i].Name < parsed.Disks[j].Name })
		parsed.Collectors = append(parsed.Collectors, snapshotCollector{Key: "disks", Name: "disks", Success: true})
	case "failed":
		var failed collectorFailure
		if err := decodeStrict(request.Disks, &failed); err != nil {
			return parsedSnapshot{}, err
		}
		parsed.Collectors = append(parsed.Collectors, failureCollector("disks", "disks", nil, nil, failed.Error, errorsByCollector))
	default:
		return parsedSnapshot{}, errors.New("invalid disk collector status")
	}
	var interfaceStatus unionHeader
	if err := json.Unmarshal(request.Interfaces, &interfaceStatus); err != nil {
		return parsedSnapshot{}, errors.New("invalid interface collector result")
	}
	switch interfaceStatus.Status {
	case "success":
		var success interfaceSuccess
		if err := decodeStrict(request.Interfaces, &success); err != nil {
			return parsedSnapshot{}, err
		}
		if success.Interfaces == nil {
			return parsedSnapshot{}, errors.New("interfaces must be an array")
		}
		if len(success.Interfaces) > 1024 {
			return parsedSnapshot{}, errors.New("interface collection exceeds item limit")
		}
		for _, item := range success.Interfaces {
			if len(item.Addresses) > 128 {
				return parsedSnapshot{}, errors.New("interface address collection exceeds item limit")
			}
		}
		parsed.Interfaces = success.Interfaces
		sort.Slice(parsed.Interfaces, func(i, j int) bool { return parsed.Interfaces[i].Name < parsed.Interfaces[j].Name })
		for i := range parsed.Interfaces {
			sort.Slice(parsed.Interfaces[i].Addresses, func(a, b int) bool {
				return parsed.Interfaces[i].Addresses[a].Address < parsed.Interfaces[i].Addresses[b].Address
			})
		}
		parsed.Collectors = append(parsed.Collectors, snapshotCollector{Key: "interfaces", Name: "interfaces", Success: true})
	case "failed":
		var failed collectorFailure
		if err := decodeStrict(request.Interfaces, &failed); err != nil {
			return parsedSnapshot{}, err
		}
		parsed.Collectors = append(parsed.Collectors, failureCollector("interfaces", "interfaces", nil, nil, failed.Error, errorsByCollector))
	default:
		return parsedSnapshot{}, errors.New("invalid interface collector status")
	}
	var fsStatus unionHeader
	if err := json.Unmarshal(request.Filesystems, &fsStatus); err != nil {
		return parsedSnapshot{}, errors.New("invalid filesystem collector result")
	}
	switch fsStatus.Status {
	case "success":
		var success filesystemSuccess
		if err := decodeStrict(request.Filesystems, &success); err != nil {
			return parsedSnapshot{}, err
		}
		if success.Filesystems == nil {
			return parsedSnapshot{}, errors.New("filesystems must be an array")
		}
		if len(success.Filesystems) > 4096 {
			return parsedSnapshot{}, errors.New("filesystem collection exceeds item limit")
		}
		parsed.Filesystems = success.Filesystems
		sort.Slice(parsed.Filesystems, func(i, j int) bool {
			if parsed.Filesystems[i].Device != parsed.Filesystems[j].Device {
				return parsed.Filesystems[i].Device < parsed.Filesystems[j].Device
			}
			return parsed.Filesystems[i].MountPoint < parsed.Filesystems[j].MountPoint
		})
		parsed.Collectors = append(parsed.Collectors, snapshotCollector{Key: "filesystems", Name: "filesystems", Success: true})
	case "failed":
		var failed collectorFailure
		if err := decodeStrict(request.Filesystems, &failed); err != nil {
			return parsedSnapshot{}, err
		}
		parsed.Collectors = append(parsed.Collectors, failureCollector("filesystems", "filesystems", nil, nil, failed.Error, errorsByCollector))
	default:
		return parsedSnapshot{}, errors.New("invalid filesystem collector status")
	}
	seenOwners := map[int64]bool{}
	for _, raw := range request.Podman {
		if err := validatePodmanEnvelope(raw); err != nil {
			return parsedSnapshot{}, err
		}
		var head unionHeader
		if err := json.Unmarshal(raw, &head); err != nil {
			return parsedSnapshot{}, errors.New("invalid Podman collector result")
		}
		switch head.Status {
		case "success":
			var success podmanSuccess
			if err := decodeStrict(raw, &success); err != nil {
				return parsedSnapshot{}, err
			}
			if success.Containers == nil {
				return parsedSnapshot{}, errors.New("containers must be an array")
			}
			if success.OwnerUID < 0 || success.OwnerUID > 4294967295 || strings.TrimSpace(success.Connection) == "" || len(success.Containers) > 10000 {
				return parsedSnapshot{}, errors.New("invalid Podman collector identity or size")
			}
			if seenOwners[success.OwnerUID] {
				return parsedSnapshot{}, errors.New("duplicate Podman owner UID")
			}
			seenOwners[success.OwnerUID] = true
			sort.Slice(success.Containers, func(i, j int) bool { return success.Containers[i].Name < success.Containers[j].Name })
			for i := range success.Containers {
				container := success.Containers[i]
				if len(container.Ports) > 2048 || len(container.Networks) > 256 || len(container.Mounts) > 2048 || len(container.Labels) > 512 {
					return parsedSnapshot{}, errors.New("container child collection exceeds item limit")
				}
				if err := validateContainerSnapshot(container); err != nil {
					return parsedSnapshot{}, err
				}
				for _, network := range container.Networks {
					if len(network.Addresses) > 64 {
						return parsedSnapshot{}, errors.New("container network address collection exceeds item limit")
					}
				}
				canonicalizeContainer(&success.Containers[i])
			}
			group := parsedPodman{Connection: success.Connection, OwnerUID: success.OwnerUID, Success: true, Containers: success.Containers}
			parsed.Podman = append(parsed.Podman, group)
			uid, connection := success.OwnerUID, success.Connection
			parsed.Collectors = append(parsed.Collectors, snapshotCollector{Key: fmt.Sprintf("podman:%d", uid), Name: "podman", OwnerUID: &uid, Connection: &connection, Success: true})
		case "failed":
			var failed podmanFailure
			if err := decodeStrict(raw, &failed); err != nil {
				return parsedSnapshot{}, err
			}
			if failed.OwnerUID < 0 || failed.OwnerUID > 4294967295 || strings.TrimSpace(failed.Connection) == "" {
				return parsedSnapshot{}, errors.New("invalid Podman collector identity")
			}
			if seenOwners[failed.OwnerUID] {
				return parsedSnapshot{}, errors.New("duplicate Podman owner UID")
			}
			seenOwners[failed.OwnerUID] = true
			group := parsedPodman{Connection: failed.Connection, OwnerUID: failed.OwnerUID, Code: collectionCode(errorsByCollector, fmt.Sprintf("podman:%d", failed.OwnerUID)), Message: sanitizeCollectionError(&failed.Error)}
			parsed.Podman = append(parsed.Podman, group)
			uid, connection := failed.OwnerUID, failed.Connection
			parsed.Collectors = append(parsed.Collectors, snapshotCollector{Key: fmt.Sprintf("podman:%d", uid), Name: "podman", OwnerUID: &uid, Connection: &connection, Code: group.Code, Message: group.Message})
		default:
			return parsedSnapshot{}, errors.New("invalid Podman collector status")
		}
	}
	sort.Slice(parsed.Podman, func(i, j int) bool {
		if parsed.Podman[i].OwnerUID != parsed.Podman[j].OwnerUID {
			return parsed.Podman[i].OwnerUID < parsed.Podman[j].OwnerUID
		}
		return parsed.Podman[i].Connection < parsed.Podman[j].Connection
	})
	for _, diagnostic := range request.Errors {
		parsed.ErrorSummary = appendError(parsed.ErrorSummary, sanitizeCollectionError(&diagnostic.Message))
	}
	for _, collector := range parsed.Collectors {
		if !collector.Success {
			parsed.ErrorSummary = appendError(parsed.ErrorSummary, collector.Message)
		}
	}
	if parsed.ErrorSummary != "" {
		parsed.Result = "accepted_with_errors"
	} else {
		parsed.Result = "accepted"
	}
	structural, err := buildStructuralSnapshot(parsed, errorsByCollector)
	if err != nil {
		return parsedSnapshot{}, err
	}
	parsed.Structural = structural
	fingerprint, err := canonicalRequestFingerprint(body)
	if err != nil {
		return parsedSnapshot{}, err
	}
	parsed.Fingerprint = fingerprint
	return parsed, nil
}

// Inspect required top-level members before generated DTO decoding, which can
// otherwise collapse an omitted collection into an empty slice.
func validateSnapshotRequiredMembers(body []byte) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return errors.New("snapshot must be a JSON object")
	}
	if err := requireJSONMembers(object, "report_id", "protocol_version", "client_version", "collection_started_at", "collection_finished_at", "host", "disks", "interfaces", "filesystems", "podman", "errors"); err != nil {
		return err
	}
	for _, name := range []string{"podman", "errors"} {
		var items []json.RawMessage
		if err := json.Unmarshal(object[name], &items); err != nil || items == nil {
			return fmt.Errorf("%s must be an array", name)
		}
	}
	return nil
}

func validateContainerSnapshot(container apigen.SnapshotContainer) error {
	if strings.TrimSpace(container.Name) == "" || strings.TrimSpace(container.RuntimeId) == "" || strings.TrimSpace(container.ImageName) == "" {
		return errors.New("container name, runtime_id, and image_name are required")
	}
	if !oneOf(string(container.State), "created", "running", "paused", "exited", "stopped", "unknown") || !oneOf(string(container.Health), "none", "starting", "healthy", "unhealthy", "unknown") {
		return errors.New("invalid container state or health")
	}
	policy := string(container.RestartPolicy)
	if !oneOf(policy, "no", "always", "on_failure", "unless_stopped", "unknown") || (container.RestartMaximumRetryCount != nil && (*container.RestartMaximumRetryCount < 0 || policy != "on_failure")) {
		return errors.New("invalid container restart policy or retry count")
	}
	for _, port := range container.Ports {
		if port.ContainerPort < 1 || port.ContainerPort > 65535 || !oneOf(string(port.Protocol), "tcp", "udp", "sctp") || (port.HostPort != nil && (*port.HostPort < 1 || *port.HostPort > 65535)) {
			return errors.New("invalid container port")
		}
	}
	for _, network := range container.Networks {
		if strings.TrimSpace(network.Name) == "" {
			return errors.New("container network name is required")
		}
		for _, address := range network.Addresses {
			if strings.TrimSpace(address.Address) == "" {
				return errors.New("container network address is required")
			}
		}
	}
	for _, mount := range container.Mounts {
		if !oneOf(string(mount.Type), "bind", "volume", "tmpfs", "other") || strings.TrimSpace(mount.Destination) == "" {
			return errors.New("invalid container mount")
		}
	}
	for _, label := range container.Labels {
		if strings.TrimSpace(label.Key) == "" {
			return errors.New("container label key is required")
		}
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// validatePodmanEnvelope runs before decoding into generated structs. Go's JSON
// decoder maps both an absent and null integer to zero, which is a valid Podman
// UID and therefore must be distinguished explicitly at this trust boundary.
func validatePodmanEnvelope(raw json.RawMessage) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return errors.New("Podman collector result must be an object")
	}
	for _, key := range []string{"connection", "owner_uid", "status"} {
		value, ok := object[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("Podman %s is required and must not be null", key)
		}
	}
	var uid json.Number
	decoder := json.NewDecoder(bytes.NewReader(object["owner_uid"]))
	decoder.UseNumber()
	if err := decoder.Decode(&uid); err != nil || uid == "" {
		return errors.New("Podman owner_uid must be an integer")
	}
	if strings.ContainsAny(uid.String(), ".eE") {
		return errors.New("Podman owner_uid must be an integer")
	}
	var status string
	if err := json.Unmarshal(object["status"], &status); err != nil {
		return errors.New("Podman status must be a string")
	}
	var connection string
	if err := json.Unmarshal(object["connection"], &connection); err != nil {
		return errors.New("Podman connection must be a string")
	}
	if status == "success" {
		value, ok := object["containers"]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) || !json.Valid(value) || len(bytes.TrimSpace(value)) == 0 || bytes.TrimSpace(value)[0] != '[' {
			return errors.New("Podman containers must be a non-null array")
		}
		var containers []json.RawMessage
		if err := json.Unmarshal(value, &containers); err != nil || containers == nil {
			return errors.New("Podman containers must be an array")
		}
		for _, container := range containers {
			if err := validateContainerObject(container); err != nil {
				return fmt.Errorf("invalid Podman container shape: %w", err)
			}
			if err := validateNonNullJSON(container); err != nil {
				return fmt.Errorf("invalid Podman container scalar: %w", err)
			}
		}
	} else if status == "failed" {
		value, ok := object["error"]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("Podman error is required and must not be null")
		}
		var message string
		if err := json.Unmarshal(value, &message); err != nil || message == "" {
			return errors.New("Podman error must be a non-empty string")
		}
	}
	return nil
}

func validateContainerObject(raw json.RawMessage) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return errors.New("container must be an object")
	}
	if err := requireJSONMembers(object, "name", "runtime_id", "image_name", "state", "health", "restart_policy", "ports", "networks", "mounts", "labels"); err != nil {
		return err
	}
	for key, required := range map[string][]string{
		"ports": {"container_port", "protocol"}, "networks": {"name", "addresses"},
		"mounts": {"type", "source", "destination", "read_only"}, "labels": {"key", "value"},
	} {
		var children []json.RawMessage
		if err := json.Unmarshal(object[key], &children); err != nil {
			return fmt.Errorf("%s must be an array", key)
		}
		for _, child := range children {
			var childObject map[string]json.RawMessage
			if err := json.Unmarshal(child, &childObject); err != nil || childObject == nil {
				return fmt.Errorf("%s item must be an object", key)
			}
			if err := requireJSONMembers(childObject, required...); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			if key == "networks" {
				var addresses []json.RawMessage
				if err := json.Unmarshal(childObject["addresses"], &addresses); err != nil {
					return errors.New("network addresses must be an array")
				}
				for _, address := range addresses {
					var addressObject map[string]json.RawMessage
					if err := json.Unmarshal(address, &addressObject); err != nil || addressObject == nil {
						return errors.New("network address must be an object")
					}
					if err := requireJSONMembers(addressObject, "address"); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func requireJSONMembers(object map[string]json.RawMessage, required ...string) error {
	for _, key := range required {
		value, ok := object[key]
		if !ok || len(bytes.TrimSpace(value)) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("required member %q is missing or null", key)
		}
	}
	return nil
}

// JSON decoding into Go scalar fields treats null like an absent zero value.
// Reject null throughout the Podman inventory before it can authorize absence
// or enter a transaction; explicitly nullable schema fields are not currently
// part of the Podman snapshot contract.
func validateNonNullJSON(raw json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var visit func(any) error
	visit = func(current any) error {
		switch typed := current.(type) {
		case nil:
			return errors.New("null is not allowed")
		case []any:
			for _, child := range typed {
				if err := visit(child); err != nil {
					return err
				}
			}
		case map[string]any:
			for key, child := range typed {
				if key == "prefix_length" && child == nil {
					continue
				}
				if err := visit(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(value)
}

func validateGeneratedSnapshotUnions(request snapshotSubmission) error {
	validate := func(raw json.RawMessage, kind string) error {
		var status unionHeader
		if err := json.Unmarshal(raw, &status); err != nil {
			return fmt.Errorf("invalid %s collector union", kind)
		}
		switch kind {
		case "host":
			var union apigen.HostCollectionWrite
			if err := json.Unmarshal(raw, &union); err != nil {
				return err
			}
			switch status.Status {
			case "success":
				_, err := union.AsHostCollectionWriteSuccess()
				return err
			case "failed":
				_, err := union.AsHostCollectionWriteFailed()
				return err
			}
		case "disks":
			var union apigen.DiskCollectionWrite
			if err := json.Unmarshal(raw, &union); err != nil {
				return err
			}
			switch status.Status {
			case "success":
				_, err := union.AsDiskCollectionWriteSuccess()
				return err
			case "failed":
				_, err := union.AsDiskCollectionWriteFailed()
				return err
			}
		case "interfaces":
			var union apigen.InterfaceCollectionWrite
			if err := json.Unmarshal(raw, &union); err != nil {
				return err
			}
			switch status.Status {
			case "success":
				_, err := union.AsInterfaceCollectionWriteSuccess()
				return err
			case "failed":
				_, err := union.AsInterfaceCollectionWriteFailed()
				return err
			}
		case "filesystems":
			var union apigen.FilesystemCollectionWrite
			if err := json.Unmarshal(raw, &union); err != nil {
				return err
			}
			switch status.Status {
			case "success":
				_, err := union.AsFilesystemCollectionWriteSuccess()
				return err
			case "failed":
				_, err := union.AsFilesystemCollectionWriteFailed()
				return err
			}
		case "podman":
			var union apigen.PodmanCollectionWrite
			if err := json.Unmarshal(raw, &union); err != nil {
				return err
			}
			switch status.Status {
			case "success":
				_, err := union.AsPodmanCollectionWriteSuccess()
				return err
			case "failed":
				_, err := union.AsPodmanCollectionWriteFailed()
				return err
			}
		}
		return fmt.Errorf("invalid %s collector status %q", kind, status.Status)
	}
	for _, collector := range []struct {
		kind string
		raw  json.RawMessage
	}{{"host", request.Host}, {"disks", request.Disks}, {"interfaces", request.Interfaces}, {"filesystems", request.Filesystems}} {
		if err := validate(collector.raw, collector.kind); err != nil {
			return err
		}
	}
	for _, raw := range request.Podman {
		if err := validate(raw, "podman"); err != nil {
			return err
		}
	}
	return nil
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid collector section: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("invalid collector section: trailing JSON data")
	}
	return nil
}
func failureCollector(key, name string, uid *int64, connection *string, message string, errorsByCollector map[string]apigen.CollectionError) snapshotCollector {
	return snapshotCollector{Key: key, Name: name, OwnerUID: uid, Connection: connection, Code: collectionCode(errorsByCollector, key), Message: sanitizeCollectionError(&message)}
}
func collectionCode(errorsByCollector map[string]apigen.CollectionError, key string) string {
	if item, ok := errorsByCollector[key]; ok && item.Code != "" {
		return item.Code
	}
	if item, ok := errorsByCollector[strings.Split(key, ":")[0]]; ok && item.Code != "" {
		return item.Code
	}
	return "collection_failed"
}
func appendError(existing, next string) string {
	if next == "" {
		return existing
	}
	if existing != "" {
		existing += "; "
	}
	if len(existing)+len(next) > 2000 {
		return (existing + next)[:2000]
	}
	return existing + next
}
func canonicalizeContainer(container *apigen.SnapshotContainer) {
	sort.Slice(container.Labels, func(i, j int) bool { return container.Labels[i].Key < container.Labels[j].Key })
	sort.Slice(container.Mounts, func(i, j int) bool { return container.Mounts[i].Destination < container.Mounts[j].Destination })
	sort.Slice(container.Ports, func(i, j int) bool {
		if container.Ports[i].Protocol != container.Ports[j].Protocol {
			return container.Ports[i].Protocol < container.Ports[j].Protocol
		}
		if container.Ports[i].ContainerPort != container.Ports[j].ContainerPort {
			return container.Ports[i].ContainerPort < container.Ports[j].ContainerPort
		}
		return stringValue(container.Ports[i].HostIp) < stringValue(container.Ports[j].HostIp)
	})
	sort.Slice(container.Networks, func(i, j int) bool { return container.Networks[i].Name < container.Networks[j].Name })
	for i := range container.Networks {
		sort.Slice(container.Networks[i].Addresses, func(a, b int) bool {
			return container.Networks[i].Addresses[a].Address < container.Networks[i].Addresses[b].Address
		})
	}
}
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func buildStructuralSnapshot(parsed parsedSnapshot, errorsByCollector map[string]apigen.CollectionError) ([]byte, error) {
	var host any = map[string]any{"status": "failed", "error_code": collectionCode(errorsByCollector, "host")}
	if parsed.Host != nil {
		copy := *parsed.Host
		copy.UptimeSeconds = nil
		host = map[string]any{"status": "success", "host": copy}
	}
	var disks any = map[string]any{"status": "failed", "error_code": collectionCode(errorsByCollector, "disks")}
	diskStatus := parsedCollectorStatus(parsed.Collectors, "disks")
	if diskStatus {
		disks = map[string]any{"status": "success", "disks": parsed.Disks}
	}
	var interfaces any = map[string]any{"status": "failed", "error_code": collectionCode(errorsByCollector, "interfaces")}
	if parsedCollectorStatus(parsed.Collectors, "interfaces") {
		interfaces = map[string]any{"status": "success", "interfaces": parsed.Interfaces}
	}
	var filesystems any = map[string]any{"status": "failed", "error_code": collectionCode(errorsByCollector, "filesystems")}
	if parsedCollectorStatus(parsed.Collectors, "filesystems") {
		structuralFS := make([]map[string]any, 0, len(parsed.Filesystems))
		for _, item := range parsed.Filesystems {
			structuralFS = append(structuralFS, map[string]any{"device": item.Device, "filesystem": item.Filesystem, "mount_point": item.MountPoint, "total_bytes": item.TotalBytes})
		}
		filesystems = map[string]any{"status": "success", "filesystems": structuralFS}
	}
	podman := make([]any, 0, len(parsed.Podman))
	for _, group := range parsed.Podman {
		if group.Success {
			podman = append(podman, map[string]any{"connection": group.Connection, "owner_uid": group.OwnerUID, "status": "success", "containers": group.Containers})
		} else {
			podman = append(podman, map[string]any{"connection": group.Connection, "owner_uid": group.OwnerUID, "status": "failed", "error_code": group.Code})
		}
	}
	structural := map[string]any{"format_version": 1, "host": host, "disks": disks, "interfaces": interfaces, "filesystems": filesystems, "podman": podman}
	return json.Marshal(structural)
}
func parsedCollectorStatus(items []snapshotCollector, name string) bool {
	for _, item := range items {
		if item.Name == name && item.Success {
			return true
		}
	}
	return false
}
func canonicalRequestFingerprint(body []byte) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	value = sortJSONArrays(value)
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(canonical)
	return hex.EncodeToString(hash[:]), nil
}

func sortJSONArrays(value any) any {
	switch item := value.(type) {
	case []any:
		for index := range item {
			item[index] = sortJSONArrays(item[index])
		}
		sort.Slice(item, func(i, j int) bool {
			left, _ := json.Marshal(item[i])
			right, _ := json.Marshal(item[j])
			return bytes.Compare(left, right) < 0
		})
		return item
	case map[string]any:
		for key, child := range item {
			item[key] = sortJSONArrays(child)
		}
		return item
	default:
		return value
	}
}

func validUUIDString(value string) bool {
	if len(value) != 36 || (value[14] != '4' && value[14] != '7') || (value[19] != '8' && value[19] != '9' && value[19] != 'a' && value[19] != 'b') {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func validCollectorCode(value string) bool {
	if len(value) == 0 || len(value) > 100 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}
func compressSnapshot(data []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	writer.Header.ModTime = time.Time{}
	writer.Header.OS = 255
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
