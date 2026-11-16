package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	apigen "portinaia/internal/api/generated"
	dbgen "portinaia/internal/database/generated"
)

type providerResource struct {
	ID                       string   `json:"id"`
	Name                     string   `json:"name"`
	Website                  *string  `json:"website,omitempty"`
	ExternalAccountReference *string  `json:"external_account_reference,omitempty"`
	Description              string   `json:"description"`
	Notes                    string   `json:"notes"`
	Roles                    []string `json:"roles"`
	TagIDs                   []string `json:"tag_ids"`
	Revision                 int64    `json:"revision"`
	CreatedAt                string   `json:"created_at"`
	UpdatedAt                string   `json:"updated_at"`
	ArchivedAt               *string  `json:"archived_at,omitempty"`
}

type providerWrite struct {
	Name                                   string
	Website, Reference, Description, Notes *string
	Roles, TagIDs                          []string
}

func decodeProviderWrite(w http.ResponseWriter, r *http.Request) (providerWrite, error) {
	var transport apigen.ProviderWrite
	if err := decodeJSON(w, r, 32*1024, &transport); err != nil {
		return providerWrite{}, err
	}
	if transport.Roles == nil || transport.TagIds == nil {
		return providerWrite{}, errors.New("roles and tag_ids are required")
	}
	input := providerWrite{
		Name: transport.Name, Website: transport.Website,
		Reference: transport.ExternalAccountReference, Description: transport.Description,
		Notes: transport.Notes, Roles: make([]string, 0, len(transport.Roles)),
		TagIDs: make([]string, 0, len(transport.TagIds)),
	}
	for _, role := range transport.Roles {
		input.Roles = append(input.Roles, string(role))
	}
	for _, id := range transport.TagIds {
		value := id.String()
		if !validUUIDString(value) {
			return providerWrite{}, errors.New("tag_ids must contain canonical UUIDv4 or UUIDv7 identifiers")
		}
		input.TagIDs = append(input.TagIDs, value)
	}
	return input, nil
}

type providerCursor struct {
	Name       string `json:"n"`
	ID         string `json:"i"`
	Search     string `json:"q"`
	Archived   string `json:"a"`
	Collection string `json:"c"`
}

func listProvidersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		query := r.URL.Query()
		search := strings.TrimSpace(query.Get("q"))
		if len(search) > 200 {
			http.Error(w, "invalid search", 400)
			return
		}
		pageSize := 50
		archived := query.Get("archived")
		if archived == "" {
			archived = "exclude"
		}
		if archived != "exclude" && archived != "include" && archived != "only" {
			http.Error(w, "invalid archived filter", 400)
			return
		}
		if raw := query.Get("page[size]"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 || parsed > 200 {
				http.Error(w, "invalid page size", 400)
				return
			}
			pageSize = parsed
		}
		afterName, afterID := "", ""
		if raw := query.Get("page[after]"); raw != "" {
			if !validCursorLength(raw) {
				http.Error(w, "invalid cursor", 400)
				return
			}
			decoded, err := base64.RawURLEncoding.DecodeString(raw)
			var cursor providerCursor
			if err != nil || json.Unmarshal(decoded, &cursor) != nil || cursor.Name == "" || cursor.ID == "" || cursor.Search != search || cursor.Archived != archived || cursor.Collection != "providers" {
				http.Error(w, "invalid cursor", 400)
				return
			}
			afterName, afterID = cursor.Name, cursor.ID
		}
		rows, err := dbgen.New(db).ListProviders(r.Context(), dbgen.ListProvidersParams{ArchivedMode: archived, Search: search, AfterName: afterName, AfterID: afterID, PageSize: int64(pageSize + 1)})
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		items := make([]providerResource, 0, len(rows))
		for _, row := range rows {
			p := providerResource{ID: row.ID, Name: row.Name, Website: nullableString(row.Website), ExternalAccountReference: nullableString(row.ExternalAccountReference), Description: row.Description, Notes: row.Notes, Revision: row.Revision, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, ArchivedAt: nullableString(row.ArchivedAt)}
			p.TagIDs = []string{}
			items = append(items, p)
		}
		for i := range items {
			items[i].Roles, err = providerRoles(r.Context(), db, items[i].ID)
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
		}
		hasMore := len(items) > pageSize
		var next *string
		if hasMore {
			items = items[:pageSize]
			last := items[len(items)-1]
			raw, _ := json.Marshal(providerCursor{Name: last.Name, ID: last.ID, Search: search, Archived: archived, Collection: "providers"})
			cursor := base64.RawURLEncoding.EncodeToString(raw)
			next = &cursor
		}
		writeJSON(w, 200, map[string]any{"items": items, "page": map[string]any{"size": pageSize, "has_more": hasMore, "next_cursor": next}})
	}
}

func createProviderHandler(db *sql.DB) http.HandlerFunc {
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
		input, err := decodeProviderWrite(w, r)
		if err != nil {
			writeJSONDecodeError(w, r, err)
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if len(input.Name) == 0 || len(input.Name) > 200 {
			http.Error(w, "name must be 1–200 characters", 422)
			return
		}
		if input.Website != nil {
			*input.Website = strings.TrimSpace(*input.Website)
			if *input.Website == "" {
				input.Website = nil
			} else {
				u, err := url.ParseRequestURI(*input.Website)
				if err != nil || u.Scheme == "" || u.Host == "" {
					http.Error(w, "website must be an absolute URL", 422)
					return
				}
			}
		}
		if input.Reference != nil {
			*input.Reference = strings.TrimSpace(*input.Reference)
			if *input.Reference == "" {
				input.Reference = nil
			}
		}
		roles := map[string]bool{}
		for _, role := range input.Roles {
			if role != "hosting_provider" && role != "registrar" && role != "hardware_vendor" && role != "other" {
				http.Error(w, "invalid provider role", 422)
				return
			}
			if roles[role] {
				http.Error(w, "duplicate provider role", 422)
				return
			}
			roles[role] = true
		}
		if len(input.Roles) == 0 {
			http.Error(w, "at least one provider role is required", 422)
			return
		}
		if input.Roles == nil {
			input.Roles = []string{}
		}
		if input.TagIDs == nil {
			input.TagIDs = []string{}
		}
		if len(input.TagIDs) > 0 {
			http.Error(w, "provider tags are not available yet", 422)
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
			fingerprint, err = idempotencyFingerprint(session.AdministratorID, "provider.create", "", "", input)
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			replay, lookupErr := LookupIdempotencyTx(r.Context(), tx, session.AdministratorID, "provider.create", idempotencyKey, fingerprint, time.Now().UTC())
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
		_, err = dbgen.New(tx).CreateProvider(r.Context(), dbgen.CreateProviderParams{ID: id, Name: input.Name, Website: providerSQLString(input.Website), ExternalAccountReference: providerSQLString(input.Reference), Description: *input.Description, Notes: *input.Notes, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				http.Error(w, "provider name already exists", 409)
			} else {
				http.Error(w, "invalid provider", 422)
			}
			return
		}
		for _, role := range input.Roles {
			dbRole := role
			if role == "hosting_provider" {
				dbRole = "hosting"
			}
			if err = dbgen.New(tx).InsertProviderRole(r.Context(), dbgen.InsertProviderRoleParams{ProviderID: id, Role: dbRole}); err != nil {
				http.Error(w, "invalid provider role", 422)
				return
			}
		}
		resource := providerResource{ID: id, Name: input.Name, Website: input.Website, ExternalAccountReference: input.Reference, Description: *input.Description, Notes: *input.Notes, Roles: input.Roles, TagIDs: input.TagIDs, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if err = AppendAuditTx(r.Context(), tx, AuditEntry{OccurredAt: now, ActorType: "administrator", ActorID: session.AdministratorID, Action: "provider.create", ResourceType: "provider", ResourceID: id, RequestID: requestIdentity(r.Context()), After: resource}); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if idempotencyKey != "" {
			body, marshalErr := json.Marshal(resource)
			headers, headersErr := json.Marshal(map[string]string{"Location": "/api/v1/providers/" + id, "ETag": revisionETag(1)})
			if marshalErr != nil || headersErr != nil || StoreIdempotencyTx(r.Context(), tx, session.AdministratorID, "provider.create", idempotencyKey, fingerprint, http.StatusCreated, string(headers), string(body), time.Now().UTC()) != nil {
				http.Error(w, "internal error", 500)
				return
			}
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("Location", "/api/v1/providers/"+id)
		w.Header().Set("ETag", revisionETag(1))
		writeJSON(w, 201, resource)
	}
}

func getProviderHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		id := chi.URLParam(r, "provider_id")
		var p providerResource
		var website, reference, archived sql.NullString
		err := db.QueryRowContext(r.Context(), `SELECT id,name,website,external_account_reference,description,notes,revision,created_at,updated_at,archived_at FROM providers WHERE id=?`, id).Scan(&p.ID, &p.Name, &website, &reference, &p.Description, &p.Notes, &p.Revision, &p.CreatedAt, &p.UpdatedAt, &archived)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", 404)
			return
		}
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		p.Website = nullableString(website)
		p.ExternalAccountReference = nullableString(reference)
		p.ArchivedAt = nullableString(archived)
		p.Roles, err = providerRoles(r.Context(), db, id)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		p.TagIDs = []string{}
		w.Header().Set("ETag", revisionETag(p.Revision))
		writeJSON(w, 200, p)
	}
}

func updateProviderHandler(db *sql.DB) http.HandlerFunc {
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
		input, err := decodeProviderWrite(w, r)
		if err != nil {
			writeJSONDecodeError(w, r, err)
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if len(input.Name) == 0 || len(input.Name) > 200 {
			http.Error(w, "name must be 1–200 characters", 422)
			return
		}
		if input.Website != nil {
			*input.Website = strings.TrimSpace(*input.Website)
			if *input.Website == "" {
				input.Website = nil
			} else {
				u, err := url.ParseRequestURI(*input.Website)
				if err != nil || u.Scheme == "" || u.Host == "" {
					http.Error(w, "website must be an absolute URL", 422)
					return
				}
			}
		}
		if input.Reference != nil {
			*input.Reference = strings.TrimSpace(*input.Reference)
			if *input.Reference == "" {
				input.Reference = nil
			}
		}
		if input.Roles == nil {
			input.Roles = []string{}
		}
		seen := map[string]bool{}
		for _, role := range input.Roles {
			if role != "hosting_provider" && role != "registrar" && role != "hardware_vendor" && role != "other" || seen[role] {
				http.Error(w, "invalid or duplicate provider role", 422)
				return
			}
			seen[role] = true
		}
		if len(input.Roles) == 0 {
			http.Error(w, "at least one provider role is required", 422)
			return
		}
		if input.TagIDs == nil {
			input.TagIDs = []string{}
		}
		if len(input.TagIDs) > 0 {
			http.Error(w, "provider tags are not available yet", 422)
			return
		}
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if len(idempotencyKey) > 128 {
			writeProblemForRequest(w, r, http.StatusBadRequest, "invalid_request", "Idempotency-Key must be at most 128 characters")
			return
		}
		if input.Description == nil {
			input.Description = new(string)
		}
		if input.Notes == nil {
			input.Notes = new(string)
		}
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		defer tx.Rollback()
		id := chi.URLParam(r, "provider_id")
		fingerprint := ""
		if idempotencyKey != "" {
			fingerprint, err = idempotencyFingerprint(session.AdministratorID, "provider.update", id, strconv.FormatInt(expected, 10), input)
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			replay, lookupErr := LookupIdempotencyTx(r.Context(), tx, session.AdministratorID, "provider.update", idempotencyKey, fingerprint, time.Now().UTC())
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
		var before providerResource
		var website, reference sql.NullString
		err = tx.QueryRowContext(r.Context(), `SELECT id,name,website,external_account_reference,description,notes,revision,created_at,updated_at FROM providers WHERE id=? AND archived_at IS NULL`, id).Scan(&before.ID, &before.Name, &website, &reference, &before.Description, &before.Notes, &before.Revision, &before.CreatedAt, &before.UpdatedAt)
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
		before.Website = nullableString(website)
		before.ExternalAccountReference = nullableString(reference)
		before.Roles, err = providerRolesTx(r, tx, id)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		before.TagIDs = []string{}
		now := time.Now().UTC().Format(time.RFC3339)
		revision := before.Revision + 1
		affected, err := dbgen.New(tx).UpdateProvider(r.Context(), dbgen.UpdateProviderParams{Name: input.Name, Website: providerSQLString(input.Website), ExternalAccountReference: providerSQLString(input.Reference), Description: *input.Description, Notes: *input.Notes, Revision: revision, UpdatedAt: now, ID: id, Revision_2: expected})
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				http.Error(w, "provider name already exists", 409)
			} else {
				http.Error(w, "invalid provider", 422)
			}
			return
		}
		if affected != 1 {
			http.Error(w, "resource revision is stale", 412)
			return
		}
		if err = dbgen.New(tx).DeleteProviderRoles(r.Context(), id); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		for _, role := range input.Roles {
			persisted := role
			if role == "hosting_provider" {
				persisted = "hosting"
			}
			if err = dbgen.New(tx).InsertProviderRole(r.Context(), dbgen.InsertProviderRoleParams{ProviderID: id, Role: persisted}); err != nil {
				http.Error(w, "invalid provider role", 422)
				return
			}
		}
		after := providerResource{ID: id, Name: input.Name, Website: input.Website, ExternalAccountReference: input.Reference, Description: *input.Description, Notes: *input.Notes, Roles: input.Roles, TagIDs: input.TagIDs, Revision: revision, CreatedAt: before.CreatedAt, UpdatedAt: now}
		if err = AppendAuditTx(r.Context(), tx, AuditEntry{OccurredAt: now, ActorType: "administrator", ActorID: session.AdministratorID, Action: "provider.update", ResourceType: "provider", ResourceID: id, RequestID: requestIdentity(r.Context()), Before: before, After: after}); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if idempotencyKey != "" {
			body, marshalErr := json.Marshal(after)
			headers, headersErr := json.Marshal(map[string]string{"ETag": revisionETag(revision)})
			if marshalErr != nil || headersErr != nil || StoreIdempotencyTx(r.Context(), tx, session.AdministratorID, "provider.update", idempotencyKey, fingerprint, http.StatusOK, string(headers), string(body), time.Now().UTC()) != nil {
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

func providerRolesTx(r *http.Request, tx *sql.Tx, providerID string) ([]string, error) {
	rows, err := dbgen.New(tx).ListProviderRoles(r.Context(), providerID)
	if err != nil {
		return nil, err
	}
	roles := make([]string, 0, len(rows))
	for _, role := range rows {
		if role == "hosting" {
			role = "hosting_provider"
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func providerRoles(ctx context.Context, db *sql.DB, providerID string) ([]string, error) {
	rows, err := dbgen.New(db).ListProviderRoles(ctx, providerID)
	if err != nil {
		return nil, err
	}
	roles := make([]string, 0, len(rows))
	for _, role := range rows {
		if role == "hosting" {
			role = "hosting_provider"
		}
		roles = append(roles, role)
	}
	return roles, nil
}
func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func providerSQLString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}
