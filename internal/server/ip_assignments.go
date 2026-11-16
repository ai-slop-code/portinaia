package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	apigen "portinaia/internal/api/generated"
	"portinaia/internal/inventory"
)

type ipAssignmentResource struct {
	ID                 string         `json:"id"`
	Address            string         `json:"address"`
	PrefixLength       *int           `json:"prefix_length,omitempty"`
	ComputeID          *string        `json:"compute_id,omitempty"`
	ContainerID        *string        `json:"container_id,omitempty"`
	InterfaceOrNetwork *string        `json:"interface_or_network,omitempty"`
	Scope              string         `json:"scope"`
	Role               string         `json:"role"`
	Primary            bool           `json:"primary"`
	Description        string         `json:"description"`
	Revision           int64          `json:"revision"`
	CreatedAt          string         `json:"created_at"`
	UpdatedAt          string         `json:"updated_at"`
	ArchivedAt         *string        `json:"archived_at,omitempty"`
	Source             string         `json:"source"`
	Observation        *ipObservation `json:"observation,omitempty"`
}
type ipObservation struct {
	FirstObservedAt string  `json:"first_observed_at"`
	LastObservedAt  string  `json:"last_observed_at"`
	StaleAt         *string `json:"stale_at,omitempty"`
}
type ipAssignmentCursor struct {
	Address   string `json:"a"`
	ID        string `json:"i"`
	Archived  string `json:"r"`
	ComputeID string `json:"c"`
	Container string `json:"n"`
	Source    string `json:"s"`
	Search    string `json:"q"`
	Stale     string `json:"t"`
}
type ipAssignmentWrite struct {
	Address            string  `json:"address"`
	PrefixLength       *int    `json:"prefix_length" nullable:"true"`
	ComputeID          *string `json:"compute_id"`
	ContainerID        *string `json:"container_id"`
	InterfaceOrNetwork *string `json:"interface_or_network"`
	Scope              string  `json:"scope"`
	Role               *string `json:"role"`
	Primary            bool    `json:"primary"`
	Description        *string `json:"description"`
}

func listIPAssignmentsHandler(db *sql.DB) http.HandlerFunc {
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
		pageSize := 50
		if raw := q.Get("page[size]"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 200 {
				http.Error(w, "invalid page size", 400)
				return
			}
			pageSize = n
		}
		afterAddress, afterID := "", ""
		var decodedCursor *ipAssignmentCursor
		if raw := q.Get("page[after]"); raw != "" {
			if !validCursorLength(raw) {
				http.Error(w, "invalid cursor", 400)
				return
			}
			b, err := base64.RawURLEncoding.DecodeString(raw)
			var c ipAssignmentCursor
			if err != nil || json.Unmarshal(b, &c) != nil {
				http.Error(w, "invalid cursor", 400)
				return
			}
			decodedCursor = &c
			afterAddress, afterID = c.Address, c.ID
		}
		computeID, containerID, source, search := q.Get("compute_id"), q.Get("container_id"), q.Get("source"), strings.TrimSpace(q.Get("q"))
		if source != "" && source != "manual" && source != "discovered" {
			http.Error(w, "invalid source filter", 400)
			return
		}
		if len(search) > 200 {
			http.Error(w, "invalid search", 400)
			return
		}
		stale := q.Get("stale")
		if stale != "" && stale != "true" && stale != "false" {
			http.Error(w, "invalid stale filter", 400)
			return
		}
		if decodedCursor != nil && (decodedCursor.Address == "" || decodedCursor.ID == "" || decodedCursor.Archived != archived || decodedCursor.ComputeID != computeID || decodedCursor.Container != containerID || decodedCursor.Source != source || decodedCursor.Search != search || decodedCursor.Stale != stale) {
			http.Error(w, "invalid cursor", 400)
			return
		}
		staleClause := "1=1"
		if stale == "true" {
			staleClause = "stale_at IS NOT NULL"
		} else if stale == "false" {
			staleClause = "stale_at IS NULL"
		}
		query := `SELECT id,address,prefix_length,compute_id,container_id,interface_name,network_name,scope,role,is_primary,description,revision,created_at,updated_at,archived_at,source,first_observed_at,last_observed_at,stale_at FROM ip_assignments WHERE ` + treeArchivedSQL("archived_at", archived) + ` AND (?='' OR compute_id=?) AND (?='' OR container_id=?) AND (?='' OR source=?) AND (?='' OR address LIKE '%'||?||'%') AND ` + staleClause + ` AND (address>? OR (address=? AND id>?)) ORDER BY address,id LIMIT ?`
		rows, err := db.QueryContext(r.Context(), query, computeID, computeID, containerID, containerID, source, source, search, search, afterAddress, afterAddress, afterID, pageSize+1)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		items := make([]ipAssignmentResource, 0, pageSize+1)
		for rows.Next() {
			item, err := scanIPAssignment(rows)
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
		var next *string
		if more {
			items = items[:pageSize]
			last := items[len(items)-1]
			b, _ := json.Marshal(ipAssignmentCursor{Address: last.Address, ID: last.ID, Archived: archived, ComputeID: computeID, Container: containerID, Source: source, Search: search, Stale: stale})
			c := base64.RawURLEncoding.EncodeToString(b)
			next = &c
		}
		writeJSON(w, 200, map[string]any{"items": items, "page": map[string]any{"size": pageSize, "has_more": more, "next_cursor": next}})
	}
}

func createIPAssignmentHandler(db *sql.DB) http.HandlerFunc {
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
		var input ipAssignmentWrite
		if err := decodeIPWrite(w, r, &input); err != nil {
			return
		}
		family, err := validateIPWrite(&input)
		if err != nil {
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
			fingerprint, err = idempotencyFingerprint(session.AdministratorID, "ip_assignment.create", "", "", input)
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			replay, lookupErr := LookupIdempotencyTx(r.Context(), tx, session.AdministratorID, "ip_assignment.create", idempotencyKey, fingerprint, time.Now().UTC())
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
		if err := validateIPOwner(r.Context(), tx, &input); err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		if input.Primary {
			if conflict, err := ipPrimaryConflict(r.Context(), tx, &input, family, ""); err != nil {
				http.Error(w, "internal error", 500)
				return
			} else if conflict {
				http.Error(w, "an active manual primary IP already exists for this owner and family", 409)
				return
			}
		}
		interfaceName, networkName := ipInterfaceColumns(&input)
		role := ipRole(&input)
		description := ipDescription(&input)
		prefix := nullableInt(input.PrefixLength)
		_, err = tx.ExecContext(r.Context(), `INSERT INTO ip_assignments(id,address,family,prefix_length,compute_id,container_id,interface_name,network_name,scope,role,source,is_primary,description,revision,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,'manual',?,?,1,?,?)`, id, input.Address, family, prefix, input.ComputeID, input.ContainerID, interfaceName, networkName, input.Scope, role, boolInt(input.Primary), description, now, now)
		if err != nil {
			http.Error(w, "invalid IP assignment", 422)
			return
		}
		resource := ipAssignmentResource{ID: id, Address: input.Address, PrefixLength: input.PrefixLength, ComputeID: input.ComputeID, ContainerID: input.ContainerID, InterfaceOrNetwork: input.InterfaceOrNetwork, Scope: input.Scope, Role: role, Primary: input.Primary, Description: description, Revision: 1, CreatedAt: now, UpdatedAt: now, Source: "manual"}
		if err := insertIPAudit(r.Context(), tx, session.AdministratorID, "ip_assignment.create", id, nil, resource, now); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if idempotencyKey != "" {
			body, marshalErr := json.Marshal(resource)
			headers, headersErr := json.Marshal(map[string]string{"Location": "/api/v1/ip-assignments/" + id, "ETag": revisionETag(1)})
			if marshalErr != nil || headersErr != nil || StoreIdempotencyTx(r.Context(), tx, session.AdministratorID, "ip_assignment.create", idempotencyKey, fingerprint, http.StatusCreated, string(headers), string(body), time.Now().UTC()) != nil {
				http.Error(w, "internal error", 500)
				return
			}
		}
		if err = tx.Commit(); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		w.Header().Set("Location", "/api/v1/ip-assignments/"+id)
		w.Header().Set("ETag", revisionETag(1))
		writeJSON(w, 201, resource)
	}
}

func getIPAssignmentHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		item, err := loadIPAssignment(r.Context(), db, chi.URLParam(r, "ip_assignment_id"))
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

func updateIPAssignmentHandler(db *sql.DB) http.HandlerFunc {
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
		var input ipAssignmentWrite
		if err := decodeIPWrite(w, r, &input); err != nil {
			return
		}
		family, err := validateIPWrite(&input)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		id := chi.URLParam(r, "ip_assignment_id")
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
			fingerprint, err = idempotencyFingerprint(session.AdministratorID, "ip_assignment.update", id, strconv.FormatInt(expected, 10), input)
			if err != nil {
				http.Error(w, "internal error", 500)
				return
			}
			replay, lookupErr := LookupIdempotencyTx(r.Context(), tx, session.AdministratorID, "ip_assignment.update", idempotencyKey, fingerprint, time.Now().UTC())
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
		before, err := loadIPAssignment(r.Context(), tx, id)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", 404)
			return
		}
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if before.Source != "manual" {
			http.Error(w, "discovery-owned IP assignments cannot be edited", 409)
			return
		}
		if before.Revision != expected {
			http.Error(w, "resource revision is stale", 412)
			return
		}
		if err := validateIPOwner(r.Context(), tx, &input); err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		if input.Primary {
			if conflict, err := ipPrimaryConflict(r.Context(), tx, &input, family, id); err != nil {
				http.Error(w, "internal error", 500)
				return
			} else if conflict {
				http.Error(w, "an active manual primary IP already exists for this owner and family", 409)
				return
			}
		}
		now := time.Now().UTC().Format(time.RFC3339)
		interfaceName, networkName := ipInterfaceColumns(&input)
		role := ipRole(&input)
		description := ipDescription(&input)
		result, err := tx.ExecContext(r.Context(), `UPDATE ip_assignments SET address=?,family=?,prefix_length=?,compute_id=?,container_id=?,interface_name=?,network_name=?,scope=?,role=?,is_primary=?,description=?,revision=revision+1,updated_at=? WHERE id=? AND revision=? AND source='manual' AND archived_at IS NULL`, input.Address, family, nullableInt(input.PrefixLength), input.ComputeID, input.ContainerID, interfaceName, networkName, input.Scope, role, boolInt(input.Primary), description, now, id, expected)
		if err != nil {
			http.Error(w, "invalid IP assignment", 422)
			return
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			http.Error(w, "resource revision is stale", 412)
			return
		}
		after := ipAssignmentResource{ID: id, Address: input.Address, PrefixLength: input.PrefixLength, ComputeID: input.ComputeID, ContainerID: input.ContainerID, InterfaceOrNetwork: input.InterfaceOrNetwork, Scope: input.Scope, Role: role, Primary: input.Primary, Description: description, Revision: before.Revision + 1, CreatedAt: before.CreatedAt, UpdatedAt: now, Source: "manual"}
		if err := insertIPAudit(r.Context(), tx, session.AdministratorID, "ip_assignment.update", id, before, after, now); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if idempotencyKey != "" {
			body, marshalErr := json.Marshal(after)
			headers, headersErr := json.Marshal(map[string]string{"ETag": revisionETag(after.Revision)})
			if marshalErr != nil || headersErr != nil || StoreIdempotencyTx(r.Context(), tx, session.AdministratorID, "ip_assignment.update", idempotencyKey, fingerprint, http.StatusOK, string(headers), string(body), time.Now().UTC()) != nil {
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

type ipScanner interface{ Scan(...any) error }

func scanIPAssignment(row ipScanner) (ipAssignmentResource, error) {
	var item ipAssignmentResource
	var prefix sql.NullInt64
	var compute, container, iface, network, archived, first, last, stale sql.NullString
	var primary int64
	err := row.Scan(&item.ID, &item.Address, &prefix, &compute, &container, &iface, &network, &item.Scope, &item.Role, &primary, &item.Description, &item.Revision, &item.CreatedAt, &item.UpdatedAt, &archived, &item.Source, &first, &last, &stale)
	if err != nil {
		return item, err
	}
	if prefix.Valid {
		n := int(prefix.Int64)
		item.PrefixLength = &n
	}
	item.ComputeID = nullableString(compute)
	item.ContainerID = nullableString(container)
	item.ArchivedAt = nullableString(archived)
	item.Primary = primary == 1
	value := iface
	if item.ComputeID == nil {
		value = network
	}
	item.InterfaceOrNetwork = nullableString(value)
	if item.Source == "discovered" && first.Valid && last.Valid {
		item.Observation = &ipObservation{FirstObservedAt: first.String, LastObservedAt: last.String, StaleAt: nullableString(stale)}
	}
	return item, nil
}

const ipAssignmentColumns = `id,address,prefix_length,compute_id,container_id,interface_name,network_name,scope,role,is_primary,description,revision,created_at,updated_at,archived_at,source,first_observed_at,last_observed_at,stale_at`

func loadIPAssignment(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (ipAssignmentResource, error) {
	return scanIPAssignment(queryer.QueryRowContext(ctx, `SELECT `+ipAssignmentColumns+` FROM ip_assignments WHERE id=?`, id))
}

func decodeIPWrite(w http.ResponseWriter, r *http.Request, input *ipAssignmentWrite) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32*1024))
	if err != nil {
		writeJSONDecodeError(w, r, err)
		return err
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(body, &members); err != nil {
		writeJSONDecodeError(w, r, err)
		return err
	}
	if _, ok := members["primary"]; !ok {
		err := errors.New("primary is required")
		writeJSONDecodeError(w, r, err)
		return err
	}
	var transport apigen.IpAssignmentWrite
	if err := decodeStrictRequest(body, &transport); err != nil {
		writeJSONDecodeError(w, r, err)
		return err
	}
	input.Address = transport.Address
	input.PrefixLength = transport.PrefixLength
	input.InterfaceOrNetwork = transport.InterfaceOrNetwork
	input.Scope = string(transport.Scope)
	input.Role = transport.Role
	input.Primary = transport.Primary
	input.Description = transport.Description
	input.ComputeID = uuidString(transport.ComputeId)
	input.ContainerID = uuidString(transport.ContainerId)
	for _, id := range []*apigen.UuidV7{transport.ComputeId, transport.ContainerId} {
		if id != nil && !validUUIDString(id.String()) {
			err := errors.New("owner ID must be a canonical UUIDv4 or UUIDv7 identifier")
			writeJSONDecodeError(w, r, err)
			return err
		}
	}
	return nil
}
func validateIPWrite(input *ipAssignmentWrite) (int, error) {
	address, family, err := inventory.CanonicalIP(input.Address)
	if err != nil {
		return 0, err
	}
	input.Address = address
	if input.PrefixLength != nil {
		max := 128
		if family == 4 {
			max = 32
		}
		if *input.PrefixLength < 0 || *input.PrefixLength > max {
			return 0, errors.New("prefix length is outside the address family range")
		}
	}
	if (input.ComputeID == nil) == (input.ContainerID == nil) {
		return 0, errors.New("exactly one compute or container owner is required")
	}
	switch input.Scope {
	case "public", "private", "loopback", "link_local", "other":
	default:
		return 0, errors.New("invalid IP scope")
	}
	role := ipRole(input)
	switch role {
	case "primary", "management", "service", "other":
	default:
		return 0, errors.New("invalid IP role")
	}
	if input.Description != nil && len(*input.Description) > 2000 {
		return 0, errors.New("description is too long")
	}
	if input.InterfaceOrNetwork != nil {
		value := strings.TrimSpace(*input.InterfaceOrNetwork)
		if len(value) > 255 {
			return 0, errors.New("interface or network name is too long")
		}
		if value == "" {
			input.InterfaceOrNetwork = nil
		} else {
			input.InterfaceOrNetwork = &value
		}
	}
	return family, nil
}

func validateIPOwner(ctx context.Context, tx *sql.Tx, input *ipAssignmentWrite) error {
	if input.ComputeID != nil {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM computes WHERE id=? AND archived_at IS NULL`, *input.ComputeID).Scan(&n); err != nil || n != 1 {
			return errors.New("active compute owner not found")
		}
		return nil
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM containers c JOIN computes m ON m.id=c.compute_id WHERE c.id=? AND c.archived_at IS NULL AND m.archived_at IS NULL`, *input.ContainerID).Scan(&n); err != nil || n != 1 {
		return errors.New("active container owner not found")
	}
	return nil
}
func ipPrimaryConflict(ctx context.Context, tx *sql.Tx, input *ipAssignmentWrite, family int, excludeID string) (bool, error) {
	ownerColumn, ownerID := "compute_id", input.ComputeID
	if ownerID == nil {
		ownerColumn, ownerID = "container_id", input.ContainerID
	}
	var n int
	query := `SELECT count(*) FROM ip_assignments WHERE ` + ownerColumn + `=? AND family=? AND source='manual' AND is_primary=1 AND archived_at IS NULL AND id<>?`
	err := tx.QueryRowContext(ctx, query, *ownerID, family, excludeID).Scan(&n)
	return n > 0, err
}
func ipInterfaceColumns(input *ipAssignmentWrite) (any, any) {
	if input.InterfaceOrNetwork == nil {
		return nil, nil
	}
	if input.ComputeID != nil {
		return *input.InterfaceOrNetwork, nil
	}
	return nil, *input.InterfaceOrNetwork
}
func ipRole(input *ipAssignmentWrite) string {
	if input.Role == nil || strings.TrimSpace(*input.Role) == "" {
		return "other"
	}
	return strings.TrimSpace(*input.Role)
}
func ipDescription(input *ipAssignmentWrite) string {
	if input.Description == nil {
		return ""
	}
	return *input.Description
}
func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func insertIPAudit(ctx context.Context, tx *sql.Tx, adminID, action, id string, before, after any, now string) error {
	var oldJSON, newJSON []byte
	var err error
	if before != nil {
		oldJSON, err = json.Marshal(before)
		if err != nil {
			return err
		}
	}
	newJSON, err = json.Marshal(after)
	if err != nil {
		return err
	}
	auditID, err := randomID()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,occurred_at,actor_type,actor_id,action,resource_type,resource_id,before_document,after_document,result) VALUES(?,?,?,?,?,?,?,?,?,'success')`, auditID, now, "administrator", adminID, action, "ip_assignment", id, nullableBytes(oldJSON), string(newJSON))
	return err
}
func nullableBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}
