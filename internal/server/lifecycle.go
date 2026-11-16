package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type lifecycleSpec struct {
	table, idParam, resourceType, action string
	selectSQL                            string
}

func lifecycleHandler(db *sql.DB, kind, action string) http.HandlerFunc {
	spec := lifecycleFor(kind, action)
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
		id := chi.URLParam(r, spec.idParam)
		if id == "" {
			http.Error(w, "not found", 404)
			return
		}
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if len(idempotencyKey) > 128 {
			writeProblemForRequest(w, r, http.StatusBadRequest, "invalid_request", "Idempotency-Key must be at most 128 characters")
			return
		}
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		defer tx.Rollback()
		var fingerprint string
		if idempotencyKey != "" {
			fingerprint, err = idempotencyFingerprint(session.AdministratorID, spec.action, id, strconv.FormatInt(expected, 10), nil)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			replay, lookupErr := LookupIdempotencyTx(r.Context(), tx, session.AdministratorID, spec.action, idempotencyKey, fingerprint, time.Now().UTC())
			if errors.Is(lookupErr, ErrIdempotencyConflict) {
				writeProblemForRequest(w, r, http.StatusConflict, "idempotency_conflict", lookupErr.Error())
				return
			}
			if lookupErr != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if replay.Found {
				_ = tx.Rollback()
				writeStoredIdempotencyResponse(w, replay)
				return
			}
		}
		before, err := loadLifecycleResource(r.Context(), tx, kind, id)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", 404)
			return
		}
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		revision := before["revision"].(int64)
		if revision != expected {
			http.Error(w, "resource revision is stale", 412)
			return
		}
		wasArchived := before["archived_at"] != nil
		if kind == "ip_assignment" && before["source"] != "manual" {
			http.Error(w, "discovery-owned IP assignments cannot be archived or restored", 409)
			return
		}
		if (action == "archive" && wasArchived) || (action == "restore" && !wasArchived) {
			http.Error(w, "resource lifecycle state conflicts with the requested action", 409)
			return
		}
		if kind == "ip_assignment" && action == "restore" {
			var ownerCount int
			if computeID, ok := before["compute_id"].(string); ok {
				err = tx.QueryRowContext(r.Context(), `SELECT count(*) FROM computes WHERE id=? AND archived_at IS NULL`, computeID).Scan(&ownerCount)
			} else if containerID, ok := before["container_id"].(string); ok {
				err = tx.QueryRowContext(r.Context(), `SELECT count(*) FROM containers c JOIN computes m ON m.id=c.compute_id WHERE c.id=? AND c.archived_at IS NULL AND m.archived_at IS NULL`, containerID).Scan(&ownerCount)
			} else {
				http.Error(w, "invalid IP assignment owner", 422)
				return
			}
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			if ownerCount != 1 {
				http.Error(w, "IP assignment owner must be active before restore", 409)
				return
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		var archivedValue any = now
		if action == "restore" {
			archivedValue = nil
		}
		result, err := tx.ExecContext(r.Context(), `UPDATE `+spec.table+` SET archived_at=?,revision=revision+1,updated_at=? WHERE id=? AND revision=?`, archivedValue, now, id, expected)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				http.Error(w, "restore conflicts with an existing resource", 409)
			} else {
				http.Error(w, "internal error", 500)
			}
			return
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			http.Error(w, "resource revision is stale", 412)
			return
		}
		updated := make(map[string]any, len(before)+2)
		for k, v := range before {
			updated[k] = v
		}
		updated["revision"] = revision + 1
		updated["updated_at"] = now
		if archivedValue == nil {
			delete(updated, "archived_at")
		} else {
			updated["archived_at"] = now
		}
		if kind == "compute" {
			status, statusErr := computeAgentStatus(r.Context(), tx, id)
			if statusErr != nil {
				http.Error(w, "internal error", 500)
				return
			}
			if action == "archive" {
				var revoked bool
				revoked, statusErr = revokeComputeCredentials(r.Context(), tx, id, session.AdministratorID, now)
				if statusErr != nil {
					http.Error(w, "internal error", 500)
					return
				}
				if revoked {
					status = "revoked"
				}
			}
			updated["agent_status"] = status
		}
		if err = AppendAuditTx(r.Context(), tx, AuditEntry{OccurredAt: now, ActorType: "administrator", ActorID: session.AdministratorID, Action: spec.action, ResourceType: spec.resourceType, ResourceID: id, RequestID: requestIdentity(r.Context()), Before: before, After: updated}); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if idempotencyKey != "" {
			body, marshalErr := json.Marshal(updated)
			if marshalErr != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			headers, marshalErr := json.Marshal(map[string]string{"ETag": revisionETag(revision + 1)})
			if marshalErr != nil || StoreIdempotencyTx(r.Context(), tx, session.AdministratorID, spec.action, idempotencyKey, fingerprint, http.StatusOK, string(headers), string(body), time.Now().UTC()) != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("ETag", revisionETag(revision+1))
		writeJSON(w, 200, updated)
	}
}

func writeStoredIdempotencyResponse(w http.ResponseWriter, result IdempotencyResult) {
	var headers map[string]string
	if json.Unmarshal([]byte(result.HeadersJSON), &headers) == nil {
		for name, value := range headers {
			w.Header().Set(name, value)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.Status)
	_, _ = w.Write([]byte(result.BodyJSON))
}

func lifecycleFor(kind, action string) lifecycleSpec {
	switch kind {
	case "provider":
		return lifecycleSpec{"providers", "provider_id", "provider", kind + "." + action, `SELECT id,name,website,external_account_reference,description,notes,revision,created_at,updated_at,archived_at FROM providers WHERE id=?`}
	case "location":
		return lifecycleSpec{"locations", "location_id", "location", kind + "." + action, `SELECT id,type,name,provider_id,parent_id,description,notes,revision,created_at,updated_at,archived_at FROM locations WHERE id=?`}
	case "ip_assignment":
		return lifecycleSpec{"ip_assignments", "ip_assignment_id", "ip_assignment", kind + "." + action, `SELECT ` + ipAssignmentColumns + ` FROM ip_assignments WHERE id=?`}
	case "domain":
		return lifecycleSpec{"domains", "domain_id", "domain", kind + "." + action, `SELECT ` + domainColumns + ` FROM domains WHERE id=?`}
	default:
		return lifecycleSpec{"computes", "compute_id", "compute", kind + "." + action, `SELECT id,kind,display_name,ownership,provider_id,provider_resource_id,location_id,parent_compute_id,description,notes,revision,created_at,updated_at,archived_at FROM computes WHERE id=?`}
	}
}

func loadLifecycleResource(ctx context.Context, tx *sql.Tx, kind, id string) (map[string]any, error) {
	var result map[string]any
	switch kind {
	case "provider":
		var p providerResource
		var website, reference, archived sql.NullString
		err := tx.QueryRowContext(ctx, lifecycleFor(kind, "archive").selectSQL, id).Scan(&p.ID, &p.Name, &website, &reference, &p.Description, &p.Notes, &p.Revision, &p.CreatedAt, &p.UpdatedAt, &archived)
		if err != nil {
			return nil, err
		}
		p.Website = nullableString(website)
		p.ExternalAccountReference = nullableString(reference)
		p.Roles, err = providerRolesTxContext(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		p.TagIDs = []string{}
		if archived.Valid {
			p.ArchivedAt = &archived.String
		}
		result = map[string]any{"id": p.ID, "name": p.Name, "description": p.Description, "notes": p.Notes, "roles": p.Roles, "tag_ids": p.TagIDs, "revision": p.Revision, "created_at": p.CreatedAt, "updated_at": p.UpdatedAt}
		if p.Website != nil {
			result["website"] = *p.Website
		}
		if p.ExternalAccountReference != nil {
			result["external_account_reference"] = *p.ExternalAccountReference
		}
		if p.ArchivedAt != nil {
			result["archived_at"] = *p.ArchivedAt
		}
	case "location":
		var l locationResource
		var provider, parent, archived sql.NullString
		err := tx.QueryRowContext(ctx, lifecycleFor(kind, "archive").selectSQL, id).Scan(&l.ID, &l.Type, &l.Name, &provider, &parent, &l.Description, &l.Notes, &l.Revision, &l.CreatedAt, &l.UpdatedAt, &archived)
		if err != nil {
			return nil, err
		}
		l.ProviderID = nullableString(provider)
		l.ParentID = nullableString(parent)
		l.TagIDs = []string{}
		if archived.Valid {
			l.ArchivedAt = &archived.String
		}
		result = map[string]any{"id": l.ID, "type": l.Type, "name": l.Name, "description": l.Description, "notes": l.Notes, "tag_ids": l.TagIDs, "revision": l.Revision, "created_at": l.CreatedAt, "updated_at": l.UpdatedAt}
		if l.ProviderID != nil {
			result["provider_id"] = *l.ProviderID
		}
		if l.ParentID != nil {
			result["parent_id"] = *l.ParentID
		}
		if l.ArchivedAt != nil {
			result["archived_at"] = *l.ArchivedAt
		}
	case "ip_assignment":
		item, err := scanIPAssignment(tx.QueryRowContext(ctx, lifecycleFor(kind, "archive").selectSQL, id))
		if err != nil {
			return nil, err
		}
		encoded, _ := json.Marshal(item)
		if err := json.Unmarshal(encoded, &result); err != nil {
			return nil, err
		}
		result["revision"] = item.Revision
	case "domain":
		item, err := scanDomain(tx.QueryRowContext(ctx, lifecycleFor(kind, "archive").selectSQL, id))
		if err != nil {
			return nil, err
		}
		encoded, _ := json.Marshal(item)
		if err := json.Unmarshal(encoded, &result); err != nil {
			return nil, err
		}
		result["revision"] = item.Revision
	default:
		var c computeResource
		var provider, providerResource, location, parent, archived sql.NullString
		err := tx.QueryRowContext(ctx, lifecycleFor(kind, "archive").selectSQL, id).Scan(&c.ID, &c.Kind, &c.DisplayName, &c.Ownership, &provider, &providerResource, &location, &parent, &c.Description, &c.Notes, &c.Revision, &c.CreatedAt, &c.UpdatedAt, &archived)
		if err != nil {
			return nil, err
		}
		c.ProviderID = nullableString(provider)
		c.ProviderResourceID = nullableString(providerResource)
		c.LocationID = nullableString(location)
		c.ParentComputeID = nullableString(parent)
		agentID, status, statusErr := computeAgentInfo(ctx, tx, id)
		if statusErr != nil {
			return nil, statusErr
		}
		c.AgentStatus = status
		c.TagIDs = []string{}
		if archived.Valid {
			c.ArchivedAt = &archived.String
		}
		result = map[string]any{"id": c.ID, "kind": c.Kind, "display_name": c.DisplayName, "ownership": c.Ownership, "description": c.Description, "notes": c.Notes, "revision": c.Revision, "created_at": c.CreatedAt, "updated_at": c.UpdatedAt, "agent_status": c.AgentStatus, "tag_ids": c.TagIDs}
		if agentID != nil {
			result["agent_id"] = *agentID
		}
		if agentID != nil {
			result["agent_id"] = *agentID
		}
		if c.ProviderID != nil {
			result["provider_id"] = *c.ProviderID
		}
		if c.ProviderResourceID != nil {
			result["provider_resource_id"] = *c.ProviderResourceID
		}
		if c.LocationID != nil {
			result["location_id"] = *c.LocationID
		}
		if c.ParentComputeID != nil {
			result["parent_compute_id"] = *c.ParentComputeID
		}
		if c.ArchivedAt != nil {
			result["archived_at"] = *c.ArchivedAt
		}
	}
	return result, nil
}

func providerRolesTxContext(ctx context.Context, tx *sql.Tx, id string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT role FROM provider_roles WHERE provider_id=? ORDER BY role`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := []string{}
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		if role == "hosting" {
			role = "hosting_provider"
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func revokeComputeCredentials(ctx context.Context, tx *sql.Tx, computeID, administratorID, now string) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id,revision FROM agents WHERE compute_id=? AND revoked_at IS NULL`, computeID)
	if err != nil {
		return false, err
	}
	type record struct {
		id       string
		revision int64
	}
	var agents []record
	for rows.Next() {
		var item record
		if err := rows.Scan(&item.id, &item.revision); err != nil {
			rows.Close()
			return false, err
		}
		agents = append(agents, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	rows.Close()
	for _, agent := range agents {
		if _, err = tx.ExecContext(ctx, `UPDATE agents SET revoked_at=?,revision=revision+1 WHERE id=? AND revision=? AND revoked_at IS NULL`, now, agent.id, agent.revision); err != nil {
			return false, err
		}
		before, _ := json.Marshal(map[string]any{"id": agent.id, "compute_id": computeID, "revoked_at": nil, "revision": agent.revision})
		after, _ := json.Marshal(map[string]any{"id": agent.id, "compute_id": computeID, "revoked_at": now, "revision": agent.revision + 1})
		if err = insertLifecycleAudit(ctx, tx, administratorID, "agent.revoke", "agent", agent.id, before, after, now); err != nil {
			return false, err
		}
	}
	rows, err = tx.QueryContext(ctx, `SELECT id FROM agent_enrollment_tokens WHERE compute_id=? AND consumed_at IS NULL AND revoked_at IS NULL`, computeID)
	if err != nil {
		return false, err
	}
	var tokens []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		tokens = append(tokens, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	rows.Close()
	for _, tokenID := range tokens {
		if _, err = tx.ExecContext(ctx, `UPDATE agent_enrollment_tokens SET revoked_at=? WHERE id=? AND consumed_at IS NULL AND revoked_at IS NULL`, now, tokenID); err != nil {
			return false, err
		}
		before, _ := json.Marshal(map[string]any{"id": tokenID, "compute_id": computeID, "revoked_at": nil})
		after, _ := json.Marshal(map[string]any{"id": tokenID, "compute_id": computeID, "revoked_at": now})
		if err = insertLifecycleAudit(ctx, tx, administratorID, "agent_enrollment_token.revoke", "agent_enrollment_token", tokenID, before, after, now); err != nil {
			return false, err
		}
	}
	return len(agents) > 0, nil
}

type contextRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func computeAgentStatus(ctx context.Context, queryer contextRowQueryer, computeID string) (string, error) {
	_, status, err := computeAgentInfo(ctx, queryer, computeID)
	return status, err
}

func computeAgentInfo(ctx context.Context, queryer contextRowQueryer, computeID string) (*string, string, error) {
	var agentID string
	var revoked, heartbeat sql.NullString
	err := queryer.QueryRowContext(ctx, `SELECT id,revoked_at,last_heartbeat_at FROM agents WHERE compute_id=?`, computeID).Scan(&agentID, &revoked, &heartbeat)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "unenrolled", nil
	}
	if err != nil {
		return nil, "", err
	}
	id := agentID
	if revoked.Valid {
		return &id, "revoked", nil
	}
	if !heartbeat.Valid {
		return &id, "offline", nil
	}
	last, err := time.Parse(time.RFC3339, heartbeat.String)
	if err != nil {
		return &id, "offline", nil
	}
	if time.Since(last) <= 5*time.Minute {
		return &id, "online", nil
	}
	return &id, "offline", nil
}

func insertLifecycleAudit(ctx context.Context, tx *sql.Tx, administratorID, action, resourceType, resourceID string, before, after []byte, now string) error {
	id, err := randomID()
	if err != nil {
		return err
	}
	var beforeDocument any
	if len(before) > 0 {
		beforeDocument = string(before)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,occurred_at,actor_type,actor_id,action,resource_type,resource_id,before_document,after_document,result) VALUES(?,?,?,?,?,?,?,?,?,'success')`, id, now, "administrator", administratorID, action, resourceType, resourceID, beforeDocument, string(after))
	return err
}
