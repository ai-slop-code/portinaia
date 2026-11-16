package server

import (
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

type locationResource struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	ProviderID  *string  `json:"provider_id,omitempty"`
	ParentID    *string  `json:"parent_id,omitempty"`
	Description string   `json:"description"`
	Notes       string   `json:"notes"`
	TagIDs      []string `json:"tag_ids"`
	Revision    int64    `json:"revision"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	ArchivedAt  *string  `json:"archived_at,omitempty"`
}

type locationCursor struct {
	Name       string `json:"n"`
	ID         string `json:"i"`
	Search     string `json:"q"`
	Archived   string `json:"a"`
	Collection string `json:"c"`
}

type locationWriteInput struct {
	Type, Name  string
	ProviderID  *string
	ParentID    *string
	Description *string
	Notes       *string
	TagIDs      []string
}

func decodeLocationWrite(w http.ResponseWriter, r *http.Request) (locationWriteInput, error) {
	var transport apigen.LocationWrite
	if err := decodeJSON(w, r, 32*1024, &transport); err != nil {
		return locationWriteInput{}, err
	}
	if transport.TagIds == nil {
		return locationWriteInput{}, errors.New("tag_ids is required")
	}
	result := locationWriteInput{Type: string(transport.Type), Name: transport.Name, Description: transport.Description, Notes: transport.Notes, TagIDs: make([]string, 0, len(transport.TagIds))}
	if transport.ProviderId != nil {
		value := transport.ProviderId.String()
		if !validUUIDString(value) {
			return locationWriteInput{}, errors.New("provider_id must be a canonical UUIDv4 or UUIDv7 identifier")
		}
		result.ProviderID = &value
	}
	if transport.ParentId != nil {
		value := transport.ParentId.String()
		if !validUUIDString(value) {
			return locationWriteInput{}, errors.New("parent_id must be a canonical UUIDv4 or UUIDv7 identifier")
		}
		result.ParentID = &value
	}
	for _, id := range transport.TagIds {
		value := id.String()
		if !validUUIDString(value) {
			return locationWriteInput{}, errors.New("tag_ids must contain canonical UUIDv4 or UUIDv7 identifiers")
		}
		result.TagIDs = append(result.TagIDs, value)
	}
	return result, nil
}

func listLocationsHandler(db *sql.DB) http.HandlerFunc {
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
			var c locationCursor
			if err != nil || json.Unmarshal(b, &c) != nil || c.Name == "" || c.ID == "" || c.Search != search || c.Archived != archived || c.Collection != "locations" {
				http.Error(w, "invalid cursor", 400)
				return
			}
			afterName, afterID = c.Name, c.ID
		}
		rows, err := db.QueryContext(r.Context(), `SELECT id,type,name,provider_id,parent_id,description,notes,revision,created_at,updated_at,archived_at FROM locations WHERE `+treeArchivedSQL("archived_at", archived)+` AND (?='' OR name LIKE '%'||?||'%' COLLATE NOCASE) AND (name>? OR (name=? AND id>?)) ORDER BY name,id LIMIT ?`, search, search, afterName, afterName, afterID, pageSize+1)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		items := make([]locationResource, 0, pageSize+1)
		for rows.Next() {
			var l locationResource
			var provider, parent, archived sql.NullString
			if err := rows.Scan(&l.ID, &l.Type, &l.Name, &provider, &parent, &l.Description, &l.Notes, &l.Revision, &l.CreatedAt, &l.UpdatedAt, &archived); err != nil {
				rows.Close()
				http.Error(w, "internal error", 500)
				return
			}
			l.ProviderID = nullableString(provider)
			l.ParentID = nullableString(parent)
			l.ArchivedAt = nullableString(archived)
			l.TagIDs = []string{}
			items = append(items, l)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			http.Error(w, "internal error", 500)
			return
		}
		rows.Close()
		hasMore := len(items) > pageSize
		var next *string
		if hasMore {
			items = items[:pageSize]
			last := items[len(items)-1]
			b, _ := json.Marshal(locationCursor{Name: last.Name, ID: last.ID, Search: search, Archived: archived, Collection: "locations"})
			c := base64.RawURLEncoding.EncodeToString(b)
			next = &c
		}
		writeJSON(w, 200, map[string]any{"items": items, "page": map[string]any{"size": pageSize, "has_more": hasMore, "next_cursor": next}})
	}
}

func createLocationHandler(db *sql.DB) http.HandlerFunc {
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
		input, err := decodeLocationWrite(w, r)
		if err != nil {
			writeJSONDecodeError(w, r, err)
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Type != "site" && input.Type != "provider_region" {
			http.Error(w, "invalid location type", 422)
			return
		}
		if len(input.Name) == 0 || len(input.Name) > 200 {
			http.Error(w, "name must be 1–200 characters", 422)
			return
		}
		if input.TagIDs == nil {
			input.TagIDs = []string{}
		}
		if len(input.TagIDs) > 0 {
			http.Error(w, "location tags are not available yet", 422)
			return
		}
		if input.Description == nil {
			input.Description = new(string)
		}
		if input.Notes == nil {
			input.Notes = new(string)
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
			fingerprint, err = idempotencyFingerprint(session.AdministratorID, "location.create", "", "", input)
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			replay, lookupErr := LookupIdempotencyTx(r.Context(), tx, session.AdministratorID, "location.create", idempotencyKey, fingerprint, time.Now().UTC())
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
		if input.ProviderID != nil {
			var exists int
			if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM providers WHERE id=? AND archived_at IS NULL`, *input.ProviderID).Scan(&exists); err != nil || exists == 0 {
				http.Error(w, "provider not found", 422)
				return
			}
		}
		if input.ParentID != nil {
			var exists int
			if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM locations WHERE id=? AND archived_at IS NULL`, *input.ParentID).Scan(&exists); err != nil || exists == 0 {
				http.Error(w, "parent location not found", 422)
				return
			}
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO locations(id,type,name,provider_id,parent_id,description,notes,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,1,?,?)`, id, input.Type, input.Name, input.ProviderID, input.ParentID, *input.Description, *input.Notes, now, now)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				http.Error(w, "location already exists", 409)
			} else {
				http.Error(w, "invalid location", 422)
			}
			return
		}
		resource := locationResource{ID: id, Type: input.Type, Name: input.Name, ProviderID: input.ProviderID, ParentID: input.ParentID, Description: *input.Description, Notes: *input.Notes, TagIDs: input.TagIDs, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if err = AppendAuditTx(r.Context(), tx, AuditEntry{OccurredAt: now, ActorType: "administrator", ActorID: session.AdministratorID, Action: "location.create", ResourceType: "location", ResourceID: id, RequestID: requestIdentity(r.Context()), After: resource}); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if idempotencyKey != "" {
			body, marshalErr := json.Marshal(resource)
			headers, headersErr := json.Marshal(map[string]string{"Location": "/api/v1/locations/" + id, "ETag": revisionETag(1)})
			if marshalErr != nil || headersErr != nil || StoreIdempotencyTx(r.Context(), tx, session.AdministratorID, "location.create", idempotencyKey, fingerprint, http.StatusCreated, string(headers), string(body), time.Now().UTC()) != nil {
				http.Error(w, "internal error", 500)
				return
			}
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("Location", "/api/v1/locations/"+id)
		w.Header().Set("ETag", revisionETag(1))
		writeJSON(w, 201, resource)
	}
}

func getLocationHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		id := chi.URLParam(r, "location_id")
		var l locationResource
		var provider, parent, archived sql.NullString
		err := db.QueryRowContext(r.Context(), `SELECT id,type,name,provider_id,parent_id,description,notes,revision,created_at,updated_at,archived_at FROM locations WHERE id=?`, id).Scan(&l.ID, &l.Type, &l.Name, &provider, &parent, &l.Description, &l.Notes, &l.Revision, &l.CreatedAt, &l.UpdatedAt, &archived)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", 404)
			return
		}
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		l.ProviderID = nullableString(provider)
		l.ParentID = nullableString(parent)
		l.ArchivedAt = nullableString(archived)
		l.TagIDs = []string{}
		w.Header().Set("ETag", revisionETag(l.Revision))
		writeJSON(w, 200, l)
	}
}

func updateLocationHandler(db *sql.DB) http.HandlerFunc {
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
		in, err := decodeLocationWrite(w, r)
		if err != nil {
			writeJSONDecodeError(w, r, err)
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Type != "site" && in.Type != "provider_region" {
			http.Error(w, "invalid location type", 422)
			return
		}
		if len(in.Name) == 0 || len(in.Name) > 200 {
			http.Error(w, "name must be 1–200 characters", 422)
			return
		}
		if in.TagIDs == nil {
			in.TagIDs = []string{}
		}
		if len(in.TagIDs) > 0 {
			http.Error(w, "location tags are not available yet", 422)
			return
		}
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if len(idempotencyKey) > 128 {
			writeProblemForRequest(w, r, http.StatusBadRequest, "invalid_request", "Idempotency-Key must be at most 128 characters")
			return
		}
		if in.Description == nil {
			in.Description = new(string)
		}
		if in.Notes == nil {
			in.Notes = new(string)
		}
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		defer tx.Rollback()
		id := chi.URLParam(r, "location_id")
		fingerprint := ""
		if idempotencyKey != "" {
			fingerprint, err = idempotencyFingerprint(session.AdministratorID, "location.update", id, strconv.FormatInt(expected, 10), in)
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			replay, lookupErr := LookupIdempotencyTx(r.Context(), tx, session.AdministratorID, "location.update", idempotencyKey, fingerprint, time.Now().UTC())
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
		var before locationResource
		var provider, parent sql.NullString
		err = tx.QueryRowContext(r.Context(), `SELECT id,type,name,provider_id,parent_id,description,notes,revision,created_at,updated_at FROM locations WHERE id=? AND archived_at IS NULL`, id).Scan(&before.ID, &before.Type, &before.Name, &provider, &parent, &before.Description, &before.Notes, &before.Revision, &before.CreatedAt, &before.UpdatedAt)
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
		before.ParentID = nullableString(parent)
		before.TagIDs = []string{}
		if in.ProviderID != nil {
			var n int
			if err := tx.QueryRowContext(r.Context(), `SELECT count(*) FROM providers WHERE id=? AND archived_at IS NULL`, *in.ProviderID).Scan(&n); err != nil || n == 0 {
				http.Error(w, "provider not found", 422)
				return
			}
		}
		if in.ParentID != nil {
			if *in.ParentID == id {
				http.Error(w, "location parent cycle", 422)
				return
			}
			var cyclic int
			err := tx.QueryRowContext(r.Context(), `WITH RECURSIVE ancestors(id,parent_id) AS (SELECT id,parent_id FROM locations WHERE id=? UNION SELECT l.id,l.parent_id FROM locations l JOIN ancestors a ON l.id=a.parent_id) SELECT count(*) FROM ancestors WHERE id=?`, *in.ParentID, id).Scan(&cyclic)
			if err != nil || cyclic > 0 {
				http.Error(w, "location parent cycle or missing parent", 422)
				return
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		revision := before.Revision + 1
		result, err := tx.ExecContext(r.Context(), `UPDATE locations SET type=?,name=?,provider_id=?,parent_id=?,description=?,notes=?,revision=?,updated_at=? WHERE id=? AND revision=? AND archived_at IS NULL`, in.Type, in.Name, in.ProviderID, in.ParentID, *in.Description, *in.Notes, revision, now, id, expected)
		if err != nil {
			http.Error(w, "invalid location", 422)
			return
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			http.Error(w, "resource revision is stale", 412)
			return
		}
		after := locationResource{ID: id, Type: in.Type, Name: in.Name, ProviderID: in.ProviderID, ParentID: in.ParentID, Description: *in.Description, Notes: *in.Notes, TagIDs: in.TagIDs, Revision: revision, CreatedAt: before.CreatedAt, UpdatedAt: now}
		if err = AppendAuditTx(r.Context(), tx, AuditEntry{OccurredAt: now, ActorType: "administrator", ActorID: session.AdministratorID, Action: "location.update", ResourceType: "location", ResourceID: id, RequestID: requestIdentity(r.Context()), Before: before, After: after}); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if idempotencyKey != "" {
			body, marshalErr := json.Marshal(after)
			headers, headersErr := json.Marshal(map[string]string{"ETag": revisionETag(revision)})
			if marshalErr != nil || headersErr != nil || StoreIdempotencyTx(r.Context(), tx, session.AdministratorID, "location.update", idempotencyKey, fingerprint, http.StatusOK, string(headers), string(body), time.Now().UTC()) != nil {
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
