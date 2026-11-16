package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	apigen "portinaia/internal/api/generated"
)

const (
	acceptedProtocolVersion = 1
	heartbeatInterval       = 60
	snapshotInterval        = 3600
	systemActorID           = "0190d7b6-8ef9-7ca1-a8c4-6bcbca5d73c0"
)

type protocolRange struct {
	Minimum int `json:"minimum"`
	Maximum int `json:"maximum"`
}
type agentSchedule struct {
	HeartbeatIntervalSeconds int `json:"heartbeat_interval_seconds"`
	SnapshotIntervalSeconds  int `json:"snapshot_interval_seconds"`
}
type enrollmentTokenResponse struct {
	ID        string `json:"id"`
	ComputeID string `json:"compute_id"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	Token     string `json:"token"`
}
type agentEnrollmentResponse struct {
	AgentID              string        `json:"agent_id"`
	ComputeID            string        `json:"compute_id"`
	Credential           string        `json:"credential"`
	CredentialGeneration int64         `json:"credential_generation"`
	ServerTime           string        `json:"server_time"`
	AcceptedProtocol     protocolRange `json:"accepted_protocol"`
	Schedule             agentSchedule `json:"schedule"`
}
type agentProtocolResponse struct {
	ServerTime       string        `json:"server_time"`
	AcceptedProtocol protocolRange `json:"accepted_protocol"`
	Schedule         agentSchedule `json:"schedule"`
	Warnings         []any         `json:"warnings"`
}

func createEnrollmentTokenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := authenticate(db, r)
		if !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		if !constantEqual([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRFToken)) {
			http.Error(w, "forbidden", 403)
			return
		}
		var input apigen.EnrollmentTokenCreate
		if err := decodeJSON(w, r, 4096, &input); err != nil {
			writeJSONDecodeError(w, r, err)
			return
		}
		expiresIn := 3600
		if input.ExpiresInSeconds != nil {
			expiresIn = *input.ExpiresInSeconds
		}
		if expiresIn < 60 || expiresIn > 86400 {
			http.Error(w, "expires_in_seconds must be between 60 and 86400", 422)
			return
		}
		computeID := chi.URLParam(r, "compute_id")
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		defer tx.Rollback()
		var computeName string
		if err := tx.QueryRowContext(r.Context(), `SELECT display_name FROM computes WHERE id=? AND archived_at IS NULL`, computeID).Scan(&computeName); errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "compute not found", 404)
			return
		} else if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		var activeAgent int
		if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM agents WHERE compute_id=? AND revoked_at IS NULL`, computeID).Scan(&activeAgent); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if activeAgent > 0 {
			http.Error(w, "active agent must be explicitly rotated before issuing another enrollment token", 409)
			return
		}
		now := time.Now().UTC()
		created := now.Format(time.RFC3339)
		expires := now.Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339)
		var tokenID string
		var oldExpiry string
		err = tx.QueryRowContext(r.Context(), `SELECT id,expires_at FROM agent_enrollment_tokens WHERE compute_id=? AND consumed_at IS NULL AND revoked_at IS NULL`, computeID).Scan(&tokenID, &oldExpiry)
		if err == nil {
			if _, err := tx.ExecContext(r.Context(), `UPDATE agent_enrollment_tokens SET revoked_at=? WHERE id=? AND consumed_at IS NULL AND revoked_at IS NULL`, created, tokenID); err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			before, _ := json.Marshal(map[string]any{"id": tokenID, "compute_id": computeID, "revoked_at": nil})
			reason := "replacement"
			if oldExpiry <= created {
				reason = "expired_replacement"
			}
			after, _ := json.Marshal(map[string]any{"id": tokenID, "compute_id": computeID, "revoked_at": created, "reason": reason})
			if err := insertLifecycleAudit(r.Context(), tx, session.AdministratorID, "agent_enrollment_token.revoke", "agent_enrollment_token", tokenID, before, after, created); err != nil {
				http.Error(w, "internal error", 500)
				return
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "internal error", 500)
			return
		}
		secret, err := randomSecret(32)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		tokenID, err = randomID()
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO agent_enrollment_tokens(id,compute_id,token_hash,created_at,expires_at,created_by_administrator_id) VALUES(?,?,?,?,?,?)`, tokenID, computeID, hashSecret(secret), created, expires, session.AdministratorID)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		after, _ := json.Marshal(map[string]any{"id": tokenID, "compute_id": computeID, "created_at": created, "expires_at": expires})
		if err := insertLifecycleAudit(r.Context(), tx, session.AdministratorID, "agent_enrollment_token.create", "agent_enrollment_token", tokenID, nil, after, created); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		writeJSON(w, 201, enrollmentTokenResponse{ID: tokenID, ComputeID: computeID, CreatedAt: created, ExpiresAt: expires, Token: secret})
	}
}

func enrollAgentHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerCredential(r)
		if !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		var input apigen.AgentEnrollRequest
		if err := decodeJSON(w, r, 8*1024, &input); err != nil {
			writeJSONDecodeError(w, r, err)
			return
		}
		input.ClientVersion = strings.TrimSpace(input.ClientVersion)
		if len(input.ClientVersion) == 0 || len(input.ClientVersion) > 100 {
			http.Error(w, "invalid client_version", 422)
			return
		}
		if input.ProtocolVersion != acceptedProtocolVersion {
			http.Error(w, "unsupported protocol version", 422)
			return
		}
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		defer tx.Rollback()
		now := time.Now().UTC().Format(time.RFC3339)
		var tokenID, computeID string
		err = tx.QueryRowContext(r.Context(), `SELECT t.id,t.compute_id FROM agent_enrollment_tokens t JOIN computes c ON c.id=t.compute_id WHERE t.token_hash=? AND t.consumed_at IS NULL AND t.revoked_at IS NULL AND t.expires_at>? AND c.archived_at IS NULL`, hashSecret(token), now).Scan(&tokenID, &computeID)
		if err != nil {
			http.Error(w, "invalid or expired enrollment token", 401)
			return
		}
		var agentID string
		var generation int64
		var existingRevoked sql.NullString
		err = tx.QueryRowContext(r.Context(), `SELECT id,credential_generation,revoked_at FROM agents WHERE compute_id=?`, computeID).Scan(&agentID, &generation, &existingRevoked)
		if err == nil && !existingRevoked.Valid {
			http.Error(w, "agent already enrolled; rotate its credential explicitly", 409)
			return
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "internal error", 500)
			return
		}
		newAgent := errors.Is(err, sql.ErrNoRows)
		credential, err := randomSecret(32)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		credentialHash := hashSecret(credential)
		// The token is consumed conditionally so only one concurrent exchange wins.
		consumed, err := tx.ExecContext(r.Context(), `UPDATE agent_enrollment_tokens SET consumed_at=? WHERE id=? AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at>?`, now, tokenID, now)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		n, _ := consumed.RowsAffected()
		if n != 1 {
			http.Error(w, "enrollment token already consumed", 409)
			return
		}
		if newAgent {
			agentID, err = randomID()
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			generation = 1
			_, err = tx.ExecContext(r.Context(), `INSERT INTO agents(id,compute_id,credential_hash,credential_generation,protocol_version,client_version,enrolled_at,revision) VALUES(?,?,?,?,?,?,?,1)`, agentID, computeID, credentialHash, generation, input.ProtocolVersion, input.ClientVersion, now)
		} else {
			generation++
			_, err = tx.ExecContext(r.Context(), `UPDATE agents SET credential_hash=?,credential_generation=?,protocol_version=?,client_version=?,enrolled_at=?,last_heartbeat_at=NULL,last_successful_snapshot_at=NULL,last_error_summary=NULL,revoked_at=NULL,revision=revision+1 WHERE id=? AND compute_id=? AND revoked_at IS NOT NULL`, credentialHash, generation, input.ProtocolVersion, input.ClientVersion, now, agentID, computeID)
		}
		if err != nil {
			http.Error(w, "could not activate enrolled agent", 409)
			return
		}
		after, _ := json.Marshal(map[string]any{"id": agentID, "compute_id": computeID, "credential_generation": generation, "protocol_version": input.ProtocolVersion, "client_version": input.ClientVersion, "enrolled_at": now, "credential": "[redacted]"})
		if err := insertSystemDiscoveryAudit(r.Context(), tx, "agent_enrollment_token.consume", "agent_enrollment_token", tokenID, nil, []byte(`{"consumed":true}`), now); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if err := insertSystemDiscoveryAudit(r.Context(), tx, "agent.enroll", "agent", agentID, nil, after, now); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		writeJSON(w, 201, agentEnrollmentResponse{AgentID: agentID, ComputeID: computeID, Credential: credential, CredentialGeneration: generation, ServerTime: now, AcceptedProtocol: protocolRange{1, 1}, Schedule: agentSchedule{heartbeatInterval, snapshotInterval}})
	}
}

func revokeEnrollmentTokenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := authenticate(db, r)
		if !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		if !constantEqual([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRFToken)) {
			http.Error(w, "forbidden", 403)
			return
		}
		tokenID := chi.URLParam(r, "token_id")
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		defer tx.Rollback()
		var computeID string
		var consumed, revoked sql.NullString
		err = tx.QueryRowContext(r.Context(), `SELECT compute_id,consumed_at,revoked_at FROM agent_enrollment_tokens WHERE id=?`, tokenID).Scan(&computeID, &consumed, &revoked)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", 404)
			return
		}
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if consumed.Valid || revoked.Valid {
			http.Error(w, "enrollment token is already consumed or revoked", 409)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		result, err := tx.ExecContext(r.Context(), `UPDATE agent_enrollment_tokens SET revoked_at=? WHERE id=? AND consumed_at IS NULL AND revoked_at IS NULL`, now, tokenID)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		count, _ := result.RowsAffected()
		if count != 1 {
			http.Error(w, "enrollment token state changed", 409)
			return
		}
		before, _ := json.Marshal(map[string]any{"id": tokenID, "compute_id": computeID, "revoked_at": nil})
		after, _ := json.Marshal(map[string]any{"id": tokenID, "compute_id": computeID, "revoked_at": now})
		if err := insertLifecycleAudit(r.Context(), tx, session.AdministratorID, "agent_enrollment_token.revoke", "agent_enrollment_token", tokenID, before, after, now); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	}
}

func heartbeatHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		credential, ok := bearerCredential(r)
		if !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		var input apigen.AgentHeartbeatRequest
		if err := decodeJSON(w, r, 8*1024, &input); err != nil {
			writeJSONDecodeError(w, r, err)
			return
		}
		if input.ProtocolVersion != acceptedProtocolVersion {
			http.Error(w, "unsupported protocol version", 422)
			return
		}
		if len(strings.TrimSpace(input.ClientVersion)) == 0 || len(input.ClientVersion) > 100 {
			http.Error(w, "invalid client_version", 422)
			return
		}
		safeError := sanitizeCollectionError(input.LastCollectionError)
		now := time.Now().UTC().Format(time.RFC3339)
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		defer tx.Rollback()
		var agentID, computeID, clientVersion string
		var currentError sql.NullString
		err = tx.QueryRowContext(r.Context(), `SELECT a.id,a.compute_id,a.client_version,a.last_error_summary FROM agents a JOIN computes c ON c.id=a.compute_id WHERE a.credential_hash=? AND a.revoked_at IS NULL AND c.archived_at IS NULL AND a.protocol_version=?`, hashSecret(credential), input.ProtocolVersion).Scan(&agentID, &computeID, &clientVersion, &currentError)
		if err != nil {
			http.Error(w, "unauthorized", 401)
			return
		}
		var errText any
		if safeError != "" {
			errText = safeError
		}
		if _, err := tx.ExecContext(r.Context(), `UPDATE agents SET last_heartbeat_at=?,client_version=?,last_error_summary=? WHERE id=? AND revoked_at IS NULL`, now, input.ClientVersion, errText, agentID); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if clientVersion != input.ClientVersion || currentError.String != safeError || currentError.Valid != (safeError != "") {
			before, _ := json.Marshal(map[string]any{"client_version": clientVersion, "last_error": currentError})
			after, _ := json.Marshal(map[string]any{"client_version": input.ClientVersion, "last_error": safeError})
			if err := insertSystemDiscoveryAudit(r.Context(), tx, "agent.heartbeat_state", "agent", agentID, before, after, now); err != nil {
				http.Error(w, "internal error", 500)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		writeJSON(w, 200, agentProtocolResponse{ServerTime: now, AcceptedProtocol: protocolRange{1, 1}, Schedule: agentSchedule{heartbeatInterval, snapshotInterval}, Warnings: []any{}})
	}
}

func insertSystemDiscoveryAudit(ctx context.Context, tx *sql.Tx, action, resourceType, resourceID string, before, after []byte, now string) error {
	var beforeValue, afterValue any
	if len(before) > 0 {
		beforeValue = json.RawMessage(before)
	}
	if len(after) > 0 {
		afterValue = json.RawMessage(after)
	}
	return AppendAuditTx(ctx, tx, AuditEntry{OccurredAt: now, ActorType: "system", ActorID: systemActorID, Action: action, ResourceType: resourceType, ResourceID: resourceID, RequestID: requestIdentity(ctx), Before: beforeValue, After: afterValue, Result: "success"})
}

func revokeAgentHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := authenticate(db, r)
		if !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		if !constantEqual([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRFToken)) {
			http.Error(w, "forbidden", 403)
			return
		}
		expected, valid := parseRevisionETag(r.Header.Get("If-Match"))
		if !valid {
			http.Error(w, "If-Match with the current resource ETag is required", 400)
			return
		}
		agentID := chi.URLParam(r, "agent_id")
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		defer tx.Rollback()
		var computeID, clientVersion, enrolled string
		var protocol, generation, revision int64
		var heartbeat, successful, lastError, revoked sql.NullString
		err = tx.QueryRowContext(r.Context(), `SELECT compute_id,credential_generation,protocol_version,client_version,enrolled_at,last_heartbeat_at,last_successful_snapshot_at,last_error_summary,revoked_at,revision FROM agents WHERE id=?`, agentID).Scan(&computeID, &generation, &protocol, &clientVersion, &enrolled, &heartbeat, &successful, &lastError, &revoked, &revision)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", 404)
			return
		}
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if revision != expected {
			http.Error(w, "resource revision is stale", 412)
			return
		}
		if revoked.Valid {
			http.Error(w, "agent already revoked", 409)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err = tx.ExecContext(r.Context(), `UPDATE agents SET revoked_at=?,revision=revision+1 WHERE id=? AND revision=? AND revoked_at IS NULL`, now, agentID, revision); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if err := revokeOutstandingEnrollmentTokens(r.Context(), tx, computeID, session.AdministratorID, now); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		before, _ := json.Marshal(map[string]any{"id": agentID, "compute_id": computeID, "credential_generation": generation, "revoked_at": nil, "revision": revision})
		after, _ := json.Marshal(map[string]any{"id": agentID, "compute_id": computeID, "credential_generation": generation, "revoked_at": now, "revision": revision + 1})
		if err := insertLifecycleAudit(r.Context(), tx, session.AdministratorID, "agent.revoke", "agent", agentID, before, after, now); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		response := agentResource{ID: agentID, ComputeID: computeID, CredentialGeneration: generation, ProtocolVersion: protocol, ClientVersion: clientVersion, Status: "revoked", Compatibility: "compatible", EnrolledAt: enrolled, LastHeartbeatAt: nullableString(heartbeat), LastSuccessfulSnapshotAt: nullableString(successful), LastError: nullableString(lastError), RevokedAt: &now, Revision: revision + 1}
		w.Header().Set("ETag", revisionETag(revision+1))
		writeJSON(w, 200, response)
	}
}

func getAgentHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		id := chi.URLParam(r, "agent_id")
		var item agentResource
		var heartbeat, successful, lastError, revoked sql.NullString
		err := db.QueryRowContext(r.Context(), `SELECT id,compute_id,credential_generation,protocol_version,client_version,enrolled_at,last_heartbeat_at,last_successful_snapshot_at,last_error_summary,revoked_at,revision FROM agents WHERE id=?`, id).Scan(&item.ID, &item.ComputeID, &item.CredentialGeneration, &item.ProtocolVersion, &item.ClientVersion, &item.EnrolledAt, &heartbeat, &successful, &lastError, &revoked, &item.Revision)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", 404)
			return
		}
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		item.LastHeartbeatAt = nullableString(heartbeat)
		item.LastSuccessfulSnapshotAt = nullableString(successful)
		item.LastError = nullableString(lastError)
		item.RevokedAt = nullableString(revoked)
		item.Status = "offline"
		item.Compatibility = "unsupported"
		if item.ProtocolVersion == acceptedProtocolVersion {
			item.Compatibility = "compatible"
		}
		if revoked.Valid {
			item.Status = "revoked"
		} else if heartbeat.Valid {
			if last, parseErr := time.Parse(time.RFC3339, heartbeat.String); parseErr == nil && time.Since(last) <= 5*time.Minute {
				item.Status = "online"
			}
		}
		w.Header().Set("ETag", revisionETag(item.Revision))
		writeJSON(w, 200, item)
	}
}

type agentResource struct {
	ID                       string  `json:"id"`
	ComputeID                string  `json:"compute_id"`
	CredentialGeneration     int64   `json:"credential_generation"`
	ProtocolVersion          int64   `json:"protocol_version"`
	ClientVersion            string  `json:"client_version"`
	Status                   string  `json:"status"`
	Compatibility            string  `json:"compatibility"`
	EnrolledAt               string  `json:"enrolled_at"`
	LastHeartbeatAt          *string `json:"last_heartbeat_at,omitempty"`
	LastSuccessfulSnapshotAt *string `json:"last_successful_snapshot_at,omitempty"`
	LastError                *string `json:"last_error"`
	RevokedAt                *string `json:"revoked_at,omitempty"`
	Revision                 int64   `json:"revision"`
}

func bearerCredential(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	scheme, value, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(value) == "" {
		return "", false
	}
	return strings.TrimSpace(value), true
}
func sanitizeCollectionError(value *string) string {
	if value == nil {
		return ""
	}
	var builder strings.Builder
	for _, r := range *value {
		if unicode.IsControl(r) {
			if (r == '\n' || r == '\t') && builder.Len() < 2000 {
				builder.WriteByte(' ')
			}
			continue
		}
		if builder.Len()+utf8.RuneLen(r) > 2000 {
			break
		}
		builder.WriteRune(r)
	}
	return strings.TrimSpace(builder.String())
}

func revokeOutstandingEnrollmentTokens(ctx context.Context, tx *sql.Tx, computeID, administratorID, now string) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM agent_enrollment_tokens WHERE compute_id=? AND consumed_at IS NULL AND revoked_at IS NULL`, computeID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE agent_enrollment_tokens SET revoked_at=? WHERE id=? AND consumed_at IS NULL AND revoked_at IS NULL`, now, id); err != nil {
			return err
		}
		before, _ := json.Marshal(map[string]any{"id": id, "compute_id": computeID, "revoked_at": nil})
		after, _ := json.Marshal(map[string]any{"id": id, "compute_id": computeID, "revoked_at": now})
		if err := insertLifecycleAudit(ctx, tx, administratorID, "agent_enrollment_token.revoke", "agent_enrollment_token", id, before, after, now); err != nil {
			return err
		}
	}
	return nil
}
