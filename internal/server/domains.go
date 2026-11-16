package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	apigen "portinaia/internal/api/generated"
	"portinaia/internal/inventory"
)

type domainResource struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	RegistrarProviderID *string  `json:"registrar_provider_id,omitempty"`
	RegisteredOn        *string  `json:"registered_on,omitempty"`
	ExpiresOn           *string  `json:"expires_on,omitempty"`
	AutoRenew           *bool    `json:"auto_renew,omitempty"`
	Nameservers         []string `json:"nameservers"`
	Description         string   `json:"description"`
	Notes               string   `json:"notes"`
	Revision            int64    `json:"revision"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
	ArchivedAt          *string  `json:"archived_at,omitempty"`
	ExpiryState         string   `json:"expiry_state"`
	TagIDs              []string `json:"tag_ids"`
}
type domainCursor struct {
	Name       string `json:"n"`
	ID         string `json:"i"`
	Search     string `json:"q"`
	Archived   string `json:"a"`
	Registrar  string `json:"r"`
	Expiry     string `json:"e"`
	Collection string `json:"c"`
}
type domainWrite struct {
	Name                string   `json:"name"`
	RegistrarProviderID *string  `json:"registrar_provider_id"`
	RegisteredOn        *string  `json:"registered_on"`
	ExpiresOn           *string  `json:"expires_on"`
	AutoRenew           *bool    `json:"auto_renew" nullable:"true"`
	Nameservers         []string `json:"nameservers"`
	Description         *string  `json:"description"`
	Notes               *string  `json:"notes"`
	TagIDs              []string `json:"tag_ids"`
}

func listDomainsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		q := r.URL.Query()
		archived := q.Get("archived")
		if archived == "" {
			archived = "exclude"
		}
		if archived != "exclude" && archived != "include" && archived != "only" {
			http.Error(w, "invalid archived filter", 400)
			return
		}
		search := strings.TrimSpace(q.Get("q"))
		if len(search) > 253 {
			http.Error(w, "invalid search", 400)
			return
		}
		pageSize := 50
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
			var cursor domainCursor
			if err != nil || json.Unmarshal(b, &cursor) != nil || cursor.Name == "" || cursor.ID == "" || cursor.Search != search || cursor.Archived != archived || cursor.Registrar != q.Get("registrar_provider_id") || cursor.Expiry != q.Get("expiry_state") || cursor.Collection != "domains" {
				http.Error(w, "invalid cursor", 400)
				return
			}
			afterName, afterID = cursor.Name, cursor.ID
		}
		registrar, expiry := q.Get("registrar_provider_id"), q.Get("expiry_state")
		if expiry != "" && expiry != "unknown" && expiry != "current" && expiry != "expiring" && expiry != "expired" {
			http.Error(w, "invalid expiry state", 400)
			return
		}
		today := time.Now().UTC().Format("2006-01-02")
		cutoff := time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")
		expiryClause := "1=1"
		switch expiry {
		case "unknown":
			expiryClause = "expires_on IS NULL"
		case "expired":
			expiryClause = "expires_on < '" + today + "'"
		case "expiring":
			expiryClause = "expires_on >= '" + today + "' AND expires_on <= '" + cutoff + "'"
		case "current":
			expiryClause = "expires_on > '" + cutoff + "'"
		}
		query := `SELECT ` + domainColumns + ` FROM domains WHERE ` + treeArchivedSQL("archived_at", archived) + ` AND (?='' OR normalized_name LIKE '%'||?||'%' COLLATE NOCASE) AND (?='' OR registrar_provider_id=?) AND ` + expiryClause + ` AND (normalized_name>? OR (normalized_name=? AND id>?)) ORDER BY normalized_name,id LIMIT ?`
		rows, err := db.QueryContext(r.Context(), query, search, search, registrar, registrar, afterName, afterName, afterID, pageSize+1)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		items := make([]domainResource, 0, pageSize+1)
		for rows.Next() {
			item, err := scanDomain(rows)
			if err != nil {
				rows.Close()
				http.Error(w, "internal error", 500)
				return
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			http.Error(w, "internal error", 500)
			return
		}
		rows.Close()
		more := len(items) > pageSize
		if more {
			items = items[:pageSize]
		}
		var next *string
		if more && len(items) > 0 {
			last := items[len(items)-1]
			b, _ := json.Marshal(domainCursor{Name: last.Name, ID: last.ID, Search: search, Archived: archived, Registrar: q.Get("registrar_provider_id"), Expiry: expiry, Collection: "domains"})
			value := base64.RawURLEncoding.EncodeToString(b)
			next = &value
		}
		writeJSON(w, 200, map[string]any{"items": items, "page": map[string]any{"size": pageSize, "has_more": more, "next_cursor": next}})
	}
}

func createDomainHandler(db *sql.DB) http.HandlerFunc {
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
		input, err := decodeDomainWrite(w, r)
		if err != nil {
			return
		}
		if err = normalizeDomainWrite(&input); err != nil {
			http.Error(w, err.Error(), 422)
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
			fingerprint, err = idempotencyFingerprint(session.AdministratorID, "domain.create", "", "", input)
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			replay, lookupErr := LookupIdempotencyTx(r.Context(), tx, session.AdministratorID, "domain.create", idempotencyKey, fingerprint, time.Now().UTC())
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
		if err = validateRegistrar(r.Context(), tx, input.RegistrarProviderID); err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		nameservers, _ := json.Marshal(input.Nameservers)
		_, err = tx.ExecContext(r.Context(), `INSERT INTO domains(id,normalized_name,registrar_provider_id,registered_on,expires_on,auto_renew,nameservers,description,notes,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,1,?,?)`, id, input.Name, input.RegistrarProviderID, input.RegisteredOn, input.ExpiresOn, boolNullable(input.AutoRenew), string(nameservers), domainDescription(input), domainNotes(input), now, now)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				http.Error(w, "domain already exists", 409)
			} else {
				http.Error(w, "invalid domain", 422)
			}
			return
		}
		resource := domainFromWrite(id, input, 1, now, now, nil)
		if err = insertDomainAudit(r.Context(), tx, session.AdministratorID, "domain.create", id, nil, resource, now); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if idempotencyKey != "" {
			body, marshalErr := json.Marshal(resource)
			headers, headersErr := json.Marshal(map[string]string{"Location": "/api/v1/domains/" + id, "ETag": revisionETag(1)})
			if marshalErr != nil || headersErr != nil || StoreIdempotencyTx(r.Context(), tx, session.AdministratorID, "domain.create", idempotencyKey, fingerprint, http.StatusCreated, string(headers), string(body), time.Now().UTC()) != nil {
				http.Error(w, "internal error", 500)
				return
			}
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("Location", "/api/v1/domains/"+id)
		w.Header().Set("ETag", revisionETag(1))
		writeJSON(w, 201, resource)
	}
}

func getDomainHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		item, err := loadDomain(r.Context(), db, chi.URLParam(r, "domain_id"))
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", 404)
			return
		}
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("ETag", revisionETag(item.Revision))
		writeJSON(w, 200, item)
	}
}

func updateDomainHandler(db *sql.DB) http.HandlerFunc {
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
		input, err := decodeDomainWrite(w, r)
		if err != nil {
			return
		}
		if err = normalizeDomainWrite(&input); err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		id := chi.URLParam(r, "domain_id")
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
		fingerprint := ""
		if idempotencyKey != "" {
			fingerprint, err = idempotencyFingerprint(session.AdministratorID, "domain.update", id, strconv.FormatInt(expected, 10), input)
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			replay, lookupErr := LookupIdempotencyTx(r.Context(), tx, session.AdministratorID, "domain.update", idempotencyKey, fingerprint, time.Now().UTC())
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
		before, err := loadDomain(r.Context(), tx, id)
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
		if err = validateRegistrar(r.Context(), tx, input.RegistrarProviderID); err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		nameservers, _ := json.Marshal(input.Nameservers)
		now := time.Now().UTC().Format(time.RFC3339)
		result, err := tx.ExecContext(r.Context(), `UPDATE domains SET normalized_name=?,registrar_provider_id=?,registered_on=?,expires_on=?,auto_renew=?,nameservers=?,description=?,notes=?,revision=revision+1,updated_at=? WHERE id=? AND revision=? AND archived_at IS NULL`, input.Name, input.RegistrarProviderID, input.RegisteredOn, input.ExpiresOn, boolNullable(input.AutoRenew), string(nameservers), domainDescription(input), domainNotes(input), now, id, expected)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				http.Error(w, "domain already exists", 409)
			} else {
				http.Error(w, "invalid domain", 422)
			}
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			http.Error(w, "resource revision is stale", 412)
			return
		}
		after := domainFromWrite(id, input, before.Revision+1, before.CreatedAt, now, before.ArchivedAt)
		if err = insertDomainAudit(r.Context(), tx, session.AdministratorID, "domain.update", id, before, after, now); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if idempotencyKey != "" {
			body, marshalErr := json.Marshal(after)
			headers, headersErr := json.Marshal(map[string]string{"ETag": revisionETag(after.Revision)})
			if marshalErr != nil || headersErr != nil || StoreIdempotencyTx(r.Context(), tx, session.AdministratorID, "domain.update", idempotencyKey, fingerprint, http.StatusOK, string(headers), string(body), time.Now().UTC()) != nil {
				http.Error(w, "internal error", 500)
				return
			}
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("ETag", revisionETag(after.Revision))
		writeJSON(w, 200, after)
	}
}

func decodeDomainWrite(w http.ResponseWriter, r *http.Request) (domainWrite, error) {
	var transport apigen.DomainWrite
	if err := decodeJSON(w, r, 64*1024, &transport); err != nil {
		writeJSONDecodeError(w, r, err)
		return domainWrite{}, err
	}
	if transport.Nameservers == nil || transport.TagIds == nil {
		err := errors.New("nameservers and tag_ids are required")
		writeJSONDecodeError(w, r, err)
		return domainWrite{}, err
	}
	input := domainWrite{
		Name: transport.Name, RegistrarProviderID: uuidString(transport.RegistrarProviderId),
		AutoRenew: transport.AutoRenew, Nameservers: transport.Nameservers,
		Description: transport.Description, Notes: transport.Notes,
		TagIDs: make([]string, 0, len(transport.TagIds)),
	}
	if transport.RegisteredOn != nil {
		value := transport.RegisteredOn.String()
		input.RegisteredOn = &value
	}
	if transport.ExpiresOn != nil {
		value := transport.ExpiresOn.String()
		input.ExpiresOn = &value
	}
	if transport.RegistrarProviderId != nil && !validUUIDString(transport.RegistrarProviderId.String()) {
		err := errors.New("registrar_provider_id must be a canonical UUIDv4 or UUIDv7 identifier")
		writeJSONDecodeError(w, r, err)
		return domainWrite{}, err
	}
	for _, tagID := range transport.TagIds {
		value := tagID.String()
		if !validUUIDString(value) {
			err := errors.New("tag_ids must contain canonical UUIDv4 or UUIDv7 identifiers")
			writeJSONDecodeError(w, r, err)
			return domainWrite{}, err
		}
		input.TagIDs = append(input.TagIDs, value)
	}
	return input, nil
}

func uuidString(value *apigen.UuidV7) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}
func normalizeDomainWrite(input *domainWrite) error {
	name, err := inventory.NormalizeDNSName(input.Name)
	if err != nil {
		return errors.New("invalid domain name")
	}
	input.Name = name
	if input.Nameservers == nil {
		return errors.New("nameservers are required; use an empty list when unknown")
	}
	if input.TagIDs == nil {
		return errors.New("tag_ids are required; use an empty list when unused")
	}
	if len(input.Nameservers) > 32 {
		return errors.New("too many nameservers")
	}
	normalized := make([]string, 0, len(input.Nameservers))
	seen := map[string]bool{}
	for _, server := range input.Nameservers {
		name, err := inventory.NormalizeDNSName(server)
		if err != nil {
			return errors.New("invalid nameserver")
		}
		if !seen[name] {
			seen[name] = true
			normalized = append(normalized, name)
		}
	}
	sort.Strings(normalized)
	input.Nameservers = normalized
	if input.RegisteredOn != nil {
		if _, err := time.Parse("2006-01-02", *input.RegisteredOn); err != nil {
			return errors.New("registered_on must be a valid YYYY-MM-DD date")
		}
	}
	if input.ExpiresOn != nil {
		if _, err := time.Parse("2006-01-02", *input.ExpiresOn); err != nil {
			return errors.New("expires_on must be a valid YYYY-MM-DD date")
		}
	}
	if input.RegisteredOn != nil && input.ExpiresOn != nil && *input.ExpiresOn < *input.RegisteredOn {
		return errors.New("expiry date cannot precede registration date")
	}
	if len(input.TagIDs) > 0 {
		return errors.New("domain tags are not available yet")
	}
	if input.Description != nil && len(*input.Description) > 2000 {
		return errors.New("description is too long")
	}
	if input.Notes != nil && len(*input.Notes) > 10000 {
		return errors.New("notes are too long")
	}
	return nil
}
func normalizeDNSName(value string) (string, error) {
	return inventory.NormalizeDNSName(value)
}

func validateRegistrar(ctx context.Context, tx *sql.Tx, id *string) error {
	if id == nil {
		return nil
	}
	var count int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM providers p JOIN provider_roles r ON r.provider_id=p.id WHERE p.id=? AND p.archived_at IS NULL AND r.role='registrar'`, *id).Scan(&count)
	if err != nil || count != 1 {
		return errors.New("registrar must reference an active registrar provider")
	}
	return nil
}
func scanDomain(row interface{ Scan(...any) error }) (domainResource, error) {
	var d domainResource
	var registrar, registered, expires, archived sql.NullString
	var renew sql.NullInt64
	var nameservers string
	err := row.Scan(&d.ID, &d.Name, &registrar, &registered, &expires, &renew, &nameservers, &d.Description, &d.Notes, &d.Revision, &d.CreatedAt, &d.UpdatedAt, &archived)
	if err != nil {
		return d, err
	}
	d.RegistrarProviderID = nullableString(registrar)
	d.RegisteredOn = nullableString(registered)
	d.ExpiresOn = nullableString(expires)
	d.ArchivedAt = nullableString(archived)
	if renew.Valid {
		v := renew.Int64 == 1
		d.AutoRenew = &v
	}
	if err := json.Unmarshal([]byte(nameservers), &d.Nameservers); err != nil {
		return d, err
	}
	if d.Nameservers == nil {
		d.Nameservers = []string{}
	}
	d.TagIDs = []string{}
	d.ExpiryState = domainExpiryState(d.ExpiresOn)
	return d, nil
}

const domainColumns = `id,normalized_name,registrar_provider_id,registered_on,expires_on,auto_renew,nameservers,description,notes,revision,created_at,updated_at,archived_at`

func loadDomain(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (domainResource, error) {
	return scanDomain(q.QueryRowContext(ctx, `SELECT `+domainColumns+` FROM domains WHERE id=?`, id))
}
func domainExpiryState(expires *string) string {
	if expires == nil {
		return "unknown"
	}
	today := time.Now().UTC().Format("2006-01-02")
	if *expires < today {
		return "expired"
	}
	threshold := time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")
	if *expires <= threshold {
		return "expiring"
	}
	return "current"
}
func domainFromWrite(id string, in domainWrite, revision int64, created, updated string, archived *string) domainResource {
	return domainResource{ID: id, Name: in.Name, RegistrarProviderID: in.RegistrarProviderID, RegisteredOn: in.RegisteredOn, ExpiresOn: in.ExpiresOn, AutoRenew: in.AutoRenew, Nameservers: in.Nameservers, Description: domainDescriptionWrite(&in), Notes: domainNotesWrite(&in), Revision: revision, CreatedAt: created, UpdatedAt: updated, ArchivedAt: archived, ExpiryState: domainExpiryState(in.ExpiresOn), TagIDs: []string{}}
}
func domainDescriptionWrite(in *domainWrite) string {
	if in.Description == nil {
		return ""
	}
	return *in.Description
}
func domainNotesWrite(in *domainWrite) string {
	if in.Notes == nil {
		return ""
	}
	return *in.Notes
}
func domainDescription(in domainWrite) string { return domainDescriptionWrite(&in) }
func domainNotes(in domainWrite) string       { return domainNotesWrite(&in) }
func boolNullable(v *bool) any {
	if v == nil {
		return nil
	}
	return *v
}
func insertDomainAudit(ctx context.Context, tx *sql.Tx, adminID, action, id string, before, after any, now string) error {
	oldDoc, err := json.Marshal(before)
	if before == nil {
		oldDoc = nil
		err = nil
	}
	if err != nil {
		return err
	}
	newDoc, err := json.Marshal(after)
	if err != nil {
		return err
	}
	auditID, err := randomID()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,occurred_at,actor_type,actor_id,action,resource_type,resource_id,before_document,after_document,result) VALUES(?,?,?,?,?,?,?,?,?,'success')`, auditID, now, "administrator", adminID, action, "domain", id, nullableBytes(oldDoc), string(newDoc))
	return err
}
