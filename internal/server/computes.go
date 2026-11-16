package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	apigen "portinaia/internal/api/generated"
)

type computeResource struct {
	ID                 string                     `json:"id"`
	Kind               string                     `json:"kind"`
	DisplayName        string                     `json:"display_name"`
	Ownership          string                     `json:"ownership"`
	ProviderID         *string                    `json:"provider_id,omitempty"`
	ProviderResourceID *string                    `json:"provider_resource_id,omitempty"`
	LocationID         *string                    `json:"location_id,omitempty"`
	ParentComputeID    *string                    `json:"parent_compute_id,omitempty"`
	Description        string                     `json:"description"`
	Notes              string                     `json:"notes"`
	Revision           int64                      `json:"revision"`
	CreatedAt          string                     `json:"created_at"`
	UpdatedAt          string                     `json:"updated_at"`
	AgentID            *string                    `json:"agent_id,omitempty"`
	AgentStatus        string                     `json:"agent_status"`
	TagIDs             []string                   `json:"tag_ids"`
	ArchivedAt         *string                    `json:"archived_at,omitempty"`
	Observation        *apigen.ComputeObservation `json:"observation,omitempty"`
}

type computeCursor struct {
	Name       string `json:"n"`
	ID         string `json:"i"`
	Search     string `json:"q"`
	Archived   string `json:"a"`
	Collection string `json:"c"`
	ProviderID string `json:"p"`
	LocationID string `json:"l"`
	Freshness  string `json:"f"`
}

type computeWriteInput struct {
	Kind, DisplayName, Ownership                                string
	ProviderID, ProviderResourceID, LocationID, ParentComputeID *string
	Description, Notes                                          *string
	TagIDs                                                      []string
}

func decodeComputeWrite(w http.ResponseWriter, r *http.Request) (computeWriteInput, error) {
	var transport apigen.ComputeWrite
	if err := decodeJSON(w, r, 32*1024, &transport); err != nil {
		return computeWriteInput{}, err
	}
	if transport.TagIds == nil {
		return computeWriteInput{}, errors.New("tag_ids is required")
	}
	result := computeWriteInput{
		Kind: string(transport.Kind), DisplayName: transport.DisplayName, Ownership: string(transport.Ownership),
		ProviderResourceID: transport.ProviderResourceId, Description: transport.Description,
		Notes: transport.Notes, TagIDs: make([]string, 0, len(transport.TagIds)),
	}
	for source, destination := range map[*apigen.UuidV7]**string{
		transport.ProviderId: &result.ProviderID, transport.LocationId: &result.LocationID, transport.ParentComputeId: &result.ParentComputeID,
	} {
		if source == nil {
			continue
		}
		value := source.String()
		if !validUUIDString(value) {
			return computeWriteInput{}, errors.New("compute reference must be a canonical UUIDv4 or UUIDv7 identifier")
		}
		*destination = &value
	}
	for _, id := range transport.TagIds {
		value := id.String()
		if !validUUIDString(value) {
			return computeWriteInput{}, errors.New("tag_ids must contain canonical UUIDv4 or UUIDv7 identifiers")
		}
		result.TagIDs = append(result.TagIDs, value)
	}
	return result, nil
}

func listComputesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		q := r.URL.Query()
		search := strings.TrimSpace(q.Get("q"))
		if len(search) > 200 {
			http.Error(w, "invalid search", 400)
			return
		}
		providerID, locationID, freshness := q.Get("provider_id"), q.Get("location_id"), q.Get("freshness")
		for _, id := range []string{providerID, locationID} {
			if id != "" && !validUUIDString(id) {
				http.Error(w, "invalid compute filter", 400)
				return
			}
		}
		if freshness != "" && freshness != "online" && freshness != "offline" && freshness != "unenrolled" {
			http.Error(w, "invalid freshness filter", 400)
			return
		}
		pageSize := 50
		archived := q.Get("archived")
		if archived == "" {
			archived = "exclude"
		}
		if archived != "exclude" && archived != "include" && archived != "only" {
			http.Error(w, "invalid archived filter", 400)
			return
		}
		if raw := q.Get("page[size]"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 200 {
				http.Error(w, "invalid page size", 400)
				return
			}
			pageSize = n
		}
		afterName, afterID := "", ""
		if raw := q.Get("page[after]"); raw != "" {
			if !validCursorLength(raw) {
				http.Error(w, "invalid cursor", 400)
				return
			}
			b, err := base64.RawURLEncoding.DecodeString(raw)
			var c computeCursor
			if err != nil || json.Unmarshal(b, &c) != nil || c.Name == "" || c.ID == "" || c.Search != search || c.Archived != archived || c.Collection != "computes" || c.ProviderID != providerID || c.LocationID != locationID || c.Freshness != freshness {
				http.Error(w, "invalid cursor", 400)
				return
			}
			afterName, afterID = c.Name, c.ID
		}
		freshnessSQL := "1=1"
		cutoff := time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
		switch freshness {
		case "online":
			freshnessSQL = "EXISTS (SELECT 1 FROM agents a WHERE a.compute_id=computes.id AND a.revoked_at IS NULL AND a.last_heartbeat_at>=?)"
		case "offline":
			freshnessSQL = "EXISTS (SELECT 1 FROM agents a WHERE a.compute_id=computes.id AND (a.revoked_at IS NOT NULL OR a.last_heartbeat_at IS NULL OR a.last_heartbeat_at<?))"
		case "unenrolled":
			freshnessSQL = "NOT EXISTS (SELECT 1 FROM agents a WHERE a.compute_id=computes.id)"
		}
		query := `SELECT id,kind,display_name,ownership,provider_id,provider_resource_id,location_id,parent_compute_id,description,notes,revision,created_at,updated_at,archived_at FROM computes WHERE ` + treeArchivedSQL("archived_at", archived) + ` AND (?='' OR display_name LIKE '%'||?||'%' COLLATE NOCASE) AND (?='' OR provider_id=?) AND (?='' OR location_id=?) AND ` + freshnessSQL + ` AND (display_name>? OR (display_name=? AND id>?)) ORDER BY display_name,id LIMIT ?`
		args := []any{search, search, providerID, providerID, locationID, locationID}
		if freshness == "online" || freshness == "offline" {
			args = append(args, cutoff)
		}
		args = append(args, afterName, afterName, afterID, pageSize+1)
		rows, err := db.QueryContext(r.Context(), query, args...)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		items := make([]computeResource, 0, pageSize+1)
		for rows.Next() {
			var c computeResource
			var provider, providerResource, location, parent, archivedAt sql.NullString
			if err := rows.Scan(&c.ID, &c.Kind, &c.DisplayName, &c.Ownership, &provider, &providerResource, &location, &parent, &c.Description, &c.Notes, &c.Revision, &c.CreatedAt, &c.UpdatedAt, &archivedAt); err != nil {
				rows.Close()
				http.Error(w, "internal error", 500)
				return
			}
			c.ProviderID = nullableString(provider)
			c.ProviderResourceID = nullableString(providerResource)
			c.LocationID = nullableString(location)
			c.ParentComputeID = nullableString(parent)
			c.ArchivedAt = nullableString(archivedAt)
			c.TagIDs = []string{}
			items = append(items, c)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			http.Error(w, "internal error", 500)
			return
		}
		rows.Close()
		for i := range items {
			items[i].AgentID, items[i].AgentStatus, err = computeAgentInfo(r.Context(), db, items[i].ID)
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
		}
		more := len(items) > pageSize
		var next *string
		if more {
			items = items[:pageSize]
			last := items[len(items)-1]
			b, _ := json.Marshal(computeCursor{Name: last.DisplayName, ID: last.ID, Search: search, Archived: archived, Collection: "computes", ProviderID: providerID, LocationID: locationID, Freshness: freshness})
			token := base64.RawURLEncoding.EncodeToString(b)
			next = &token
		}
		writeJSON(w, 200, map[string]any{"items": items, "page": map[string]any{"size": pageSize, "has_more": more, "next_cursor": next}})
	}
}

func createComputeHandler(db *sql.DB) http.HandlerFunc {
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
		in, err := decodeComputeWrite(w, r)
		if err != nil {
			writeJSONDecodeError(w, r, err)
			return
		}
		in.DisplayName = strings.TrimSpace(in.DisplayName)
		if in.Kind != "physical" && in.Kind != "vps" && in.Kind != "vm" {
			http.Error(w, "invalid compute kind", 422)
			return
		}
		if in.Ownership != "owned" && in.Ownership != "rented" && in.Ownership != "other" {
			http.Error(w, "invalid ownership", 422)
			return
		}
		if len(in.DisplayName) == 0 || len(in.DisplayName) > 200 {
			http.Error(w, "display name must be 1–200 characters", 422)
			return
		}
		if in.TagIDs == nil {
			in.TagIDs = []string{}
		}
		if len(in.TagIDs) > 0 {
			http.Error(w, "compute tags are not available yet", 422)
			return
		}
		if in.Description == nil {
			in.Description = new(string)
		}
		if in.Notes == nil {
			in.Notes = new(string)
		}
		if in.ProviderResourceID != nil {
			*in.ProviderResourceID = strings.TrimSpace(*in.ProviderResourceID)
			if *in.ProviderResourceID == "" {
				in.ProviderResourceID = nil
			}
			if in.ProviderID == nil {
				http.Error(w, "provider is required for a provider resource reference", 422)
				return
			}
		}
		if in.ParentComputeID != nil && in.Kind != "vm" {
			http.Error(w, "only VMs may have a compute parent", 422)
			return
		}
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if len(idempotencyKey) > 128 {
			writeProblemForRequest(w, r, http.StatusBadRequest, "invalid_request", "Idempotency-Key must be at most 128 characters")
			return
		}
		id, err := randomID()
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		defer tx.Rollback()
		fingerprint := ""
		if idempotencyKey != "" {
			fingerprint, err = idempotencyFingerprint(session.AdministratorID, "compute.create", "", "", in)
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			replay, lookupErr := LookupIdempotencyTx(r.Context(), tx, session.AdministratorID, "compute.create", idempotencyKey, fingerprint, time.Now().UTC())
			if errors.Is(lookupErr, ErrIdempotencyConflict) {
				writeProblemForRequest(w, r, http.StatusConflict, "idempotency_conflict", lookupErr.Error())
				return
			}
			if lookupErr != nil {
				http.Error(w, "internal error", 500)
				return
			}
			if replay.Found {
				_ = tx.Rollback()
				writeStoredIdempotencyResponse(w, replay)
				return
			}
		}
		if in.ProviderID != nil {
			var n int
			if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM providers WHERE id=? AND archived_at IS NULL`, *in.ProviderID).Scan(&n); err != nil || n == 0 {
				http.Error(w, "provider not found", 422)
				return
			}
		}
		if in.LocationID != nil {
			var n int
			if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM locations WHERE id=? AND archived_at IS NULL`, *in.LocationID).Scan(&n); err != nil || n == 0 {
				http.Error(w, "location not found", 422)
				return
			}
		}
		if in.ParentComputeID != nil {
			var n int
			if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM computes WHERE id=? AND archived_at IS NULL`, *in.ParentComputeID).Scan(&n); err != nil || n == 0 {
				http.Error(w, "parent compute not found", 422)
				return
			}
			if err := validateComputeAncestry(r.Context(), tx, *in.ParentComputeID, id); err != nil {
				http.Error(w, "compute parent cycle", 422)
				return
			}
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO computes(id,kind,display_name,ownership,provider_id,provider_resource_id,location_id,parent_compute_id,description,notes,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,1,?,?)`, id, in.Kind, in.DisplayName, in.Ownership, in.ProviderID, in.ProviderResourceID, in.LocationID, in.ParentComputeID, *in.Description, *in.Notes, now, now)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				http.Error(w, "compute provider reference already exists", 409)
			} else {
				http.Error(w, "invalid compute", 422)
			}
			return
		}
		resource := computeResource{ID: id, Kind: in.Kind, DisplayName: in.DisplayName, Ownership: in.Ownership, ProviderID: in.ProviderID, ProviderResourceID: in.ProviderResourceID, LocationID: in.LocationID, ParentComputeID: in.ParentComputeID, Description: *in.Description, Notes: *in.Notes, Revision: 1, CreatedAt: now, UpdatedAt: now, AgentStatus: "unenrolled", TagIDs: in.TagIDs}
		if err = AppendAuditTx(r.Context(), tx, AuditEntry{OccurredAt: now, ActorType: "administrator", ActorID: session.AdministratorID, Action: "compute.create", ResourceType: "compute", ResourceID: id, RequestID: requestIdentity(r.Context()), After: resource}); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if idempotencyKey != "" {
			body, marshalErr := json.Marshal(resource)
			headers, headersErr := json.Marshal(map[string]string{"Location": "/api/v1/computes/" + id, "ETag": revisionETag(1)})
			if marshalErr != nil || headersErr != nil || StoreIdempotencyTx(r.Context(), tx, session.AdministratorID, "compute.create", idempotencyKey, fingerprint, http.StatusCreated, string(headers), string(body), time.Now().UTC()) != nil {
				http.Error(w, "internal error", 500)
				return
			}
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("Location", "/api/v1/computes/"+id)
		w.Header().Set("ETag", revisionETag(1))
		writeJSON(w, 201, resource)
	}
}

func validateComputeAncestry(ctx context.Context, tx *sql.Tx, parentID, selfID string) error {
	seen := map[string]bool{}
	current := parentID
	for current != "" {
		if current == selfID || seen[current] {
			return errors.New("cycle")
		}
		seen[current] = true
		var parent sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT parent_compute_id FROM computes WHERE id=? AND archived_at IS NULL`, current).Scan(&parent)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if !parent.Valid {
			return nil
		}
		current = parent.String
	}
	return nil
}

func getComputeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		id := chi.URLParam(r, "compute_id")
		var c computeResource
		var provider, providerResource, location, parent sql.NullString
		var archived sql.NullString
		err := db.QueryRowContext(r.Context(), `SELECT id,kind,display_name,ownership,provider_id,provider_resource_id,location_id,parent_compute_id,description,notes,revision,created_at,updated_at,archived_at FROM computes WHERE id=?`, id).Scan(&c.ID, &c.Kind, &c.DisplayName, &c.Ownership, &provider, &providerResource, &location, &parent, &c.Description, &c.Notes, &c.Revision, &c.CreatedAt, &c.UpdatedAt, &archived)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", 404)
			return
		}
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		c.ProviderID = nullableString(provider)
		c.ProviderResourceID = nullableString(providerResource)
		c.LocationID = nullableString(location)
		c.ParentComputeID = nullableString(parent)
		c.ArchivedAt = nullableString(archived)
		c.AgentID, c.AgentStatus, err = computeAgentInfo(r.Context(), db, id)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		c.TagIDs = []string{}
		c.Observation, err = loadComputeObservation(r.Context(), db, id)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("ETag", revisionETag(c.Revision))
		writeJSON(w, 200, c)
	}
}

func updateComputeHandler(db *sql.DB) http.HandlerFunc {
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
		in, err := decodeComputeWrite(w, r)
		if err != nil {
			writeJSONDecodeError(w, r, err)
			return
		}
		in.DisplayName = strings.TrimSpace(in.DisplayName)
		if in.Kind != "physical" && in.Kind != "vps" && in.Kind != "vm" {
			http.Error(w, "invalid compute kind", 422)
			return
		}
		if in.Ownership != "owned" && in.Ownership != "rented" && in.Ownership != "other" {
			http.Error(w, "invalid ownership", 422)
			return
		}
		if len(in.DisplayName) == 0 || len(in.DisplayName) > 200 {
			http.Error(w, "display name must be 1–200 characters", 422)
			return
		}
		if in.TagIDs == nil {
			in.TagIDs = []string{}
		}
		if len(in.TagIDs) > 0 {
			http.Error(w, "compute tags are not available yet", 422)
			return
		}
		if in.Description == nil {
			in.Description = new(string)
		}
		if in.Notes == nil {
			in.Notes = new(string)
		}
		if in.ProviderResourceID != nil {
			*in.ProviderResourceID = strings.TrimSpace(*in.ProviderResourceID)
			if *in.ProviderResourceID == "" {
				in.ProviderResourceID = nil
			}
			if in.ProviderID == nil {
				http.Error(w, "provider is required for a provider resource reference", 422)
				return
			}
		}
		if in.ParentComputeID != nil && in.Kind != "vm" {
			http.Error(w, "only VMs may have a compute parent", 422)
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
		id := chi.URLParam(r, "compute_id")
		fingerprint := ""
		if idempotencyKey != "" {
			fingerprint, err = idempotencyFingerprint(session.AdministratorID, "compute.update", id, strconv.FormatInt(expected, 10), in)
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			replay, lookupErr := LookupIdempotencyTx(r.Context(), tx, session.AdministratorID, "compute.update", idempotencyKey, fingerprint, time.Now().UTC())
			if errors.Is(lookupErr, ErrIdempotencyConflict) {
				writeProblemForRequest(w, r, http.StatusConflict, "idempotency_conflict", lookupErr.Error())
				return
			}
			if lookupErr != nil {
				http.Error(w, "internal error", 500)
				return
			}
			if replay.Found {
				_ = tx.Rollback()
				writeStoredIdempotencyResponse(w, replay)
				return
			}
		}
		var before computeResource
		var provider, providerResource, location, parent sql.NullString
		err = tx.QueryRowContext(r.Context(), `SELECT id,kind,display_name,ownership,provider_id,provider_resource_id,location_id,parent_compute_id,description,notes,revision,created_at,updated_at FROM computes WHERE id=? AND archived_at IS NULL`, id).Scan(&before.ID, &before.Kind, &before.DisplayName, &before.Ownership, &provider, &providerResource, &location, &parent, &before.Description, &before.Notes, &before.Revision, &before.CreatedAt, &before.UpdatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", 404)
			return
		}
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if before.Revision != expected {
			http.Error(w, "resource revision is stale", 412)
			return
		}
		before.ProviderID = nullableString(provider)
		before.ProviderResourceID = nullableString(providerResource)
		before.LocationID = nullableString(location)
		before.ParentComputeID = nullableString(parent)
		before.AgentID, before.AgentStatus, err = computeAgentInfo(r.Context(), tx, id)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		before.TagIDs = []string{}
		if in.ProviderID != nil {
			var n int
			if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM providers WHERE id=? AND archived_at IS NULL`, *in.ProviderID).Scan(&n); err != nil || n == 0 {
				http.Error(w, "provider not found", 422)
				return
			}
		}
		if in.LocationID != nil {
			var n int
			if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM locations WHERE id=? AND archived_at IS NULL`, *in.LocationID).Scan(&n); err != nil || n == 0 {
				http.Error(w, "location not found", 422)
				return
			}
		}
		if in.ParentComputeID != nil {
			var n int
			if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM computes WHERE id=? AND archived_at IS NULL`, *in.ParentComputeID).Scan(&n); err != nil || n == 0 {
				http.Error(w, "parent compute not found", 422)
				return
			}
			if err := validateComputeAncestry(r.Context(), tx, *in.ParentComputeID, id); err != nil {
				http.Error(w, "compute parent cycle", 422)
				return
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		revision := before.Revision + 1
		result, err := tx.ExecContext(r.Context(), `UPDATE computes SET kind=?,display_name=?,ownership=?,provider_id=?,provider_resource_id=?,location_id=?,parent_compute_id=?,description=?,notes=?,revision=?,updated_at=? WHERE id=? AND revision=? AND archived_at IS NULL`, in.Kind, in.DisplayName, in.Ownership, in.ProviderID, in.ProviderResourceID, in.LocationID, in.ParentComputeID, *in.Description, *in.Notes, revision, now, id, expected)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				http.Error(w, "provider resource reference already exists", 409)
			} else {
				http.Error(w, "invalid compute", 422)
			}
			return
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			http.Error(w, "resource revision is stale", 412)
			return
		}
		after := computeResource{ID: id, Kind: in.Kind, DisplayName: in.DisplayName, Ownership: in.Ownership, ProviderID: in.ProviderID, ProviderResourceID: in.ProviderResourceID, LocationID: in.LocationID, ParentComputeID: in.ParentComputeID, Description: *in.Description, Notes: *in.Notes, Revision: revision, CreatedAt: before.CreatedAt, UpdatedAt: now, AgentID: before.AgentID, AgentStatus: before.AgentStatus, TagIDs: in.TagIDs}
		if err = AppendAuditTx(r.Context(), tx, AuditEntry{OccurredAt: now, ActorType: "administrator", ActorID: session.AdministratorID, Action: "compute.update", ResourceType: "compute", ResourceID: id, RequestID: requestIdentity(r.Context()), Before: before, After: after}); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if idempotencyKey != "" {
			body, marshalErr := json.Marshal(after)
			headers, headersErr := json.Marshal(map[string]string{"ETag": revisionETag(revision)})
			if marshalErr != nil || headersErr != nil || StoreIdempotencyTx(r.Context(), tx, session.AdministratorID, "compute.update", idempotencyKey, fingerprint, http.StatusOK, string(headers), string(body), time.Now().UTC()) != nil {
				http.Error(w, "internal error", 500)
				return
			}
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("ETag", revisionETag(revision))
		writeJSON(w, 200, after)
	}
}
