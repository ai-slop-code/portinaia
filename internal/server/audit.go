package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AppendAuditTx adds a redacted audit record in the caller's transaction. An
// append error must be returned by the caller so its transaction rolls back.
func AppendAuditTx(ctx context.Context, tx *sql.Tx, entry AuditEntry) error {
	return appendAuditContext(ctx, tx, entry)
}

type AuditEntry struct {
	OccurredAt, ActorType, ActorID, Action, ResourceType string
	ResourceID, RequestID                                string
	Before, After                                        any
	Result                                               string
}
type auditEntry = AuditEntry

func appendAudit(tx *sql.Tx, entry AuditEntry) error {
	return appendAuditContext(context.Background(), tx, entry)
}
func appendAuditContext(ctx context.Context, tx *sql.Tx, e AuditEntry) error {
	if e.Result == "" {
		e.Result = "success"
	}
	if e.ActorType == "system" {
		e.ActorID = systemActorID
	}
	if e.OccurredAt == "" {
		e.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	}
	before, err := redactedJSON(e.Before)
	if err != nil {
		return err
	}
	after, err := redactedJSON(e.After)
	if err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_events(id,occurred_at,actor_type,actor_id,action,resource_type,resource_id,request_id,before_document,after_document,result) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id, e.OccurredAt, e.ActorType, e.ActorID, e.Action, e.ResourceType, nullableAuditString(e.ResourceID), nullableAuditString(e.RequestID), nullableAuditString(string(before)), nullableAuditString(string(after)), e.Result)
	return err
}

func redactedJSON(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var v any
	if err = json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	redactValue(v)
	return json.Marshal(v)
}
func redactValue(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if !auditFieldAllowed(k) {
				delete(x, k)
				continue
			}
			redactValue(value)
		}
	case []any:
		for _, value := range x {
			redactValue(value)
		}
	}
}

// auditFieldAllowed is deliberately positive: new request fields cannot silently
// become durable audit data. Unknown fields are omitted rather than keyword-scanned.
func auditFieldAllowed(key string) bool {
	switch strings.ToLower(key) {
	case "username", "sessions_revoked", "name", "description", "notes", "roles",
		"website", "external_account_reference", "provider_id", "location_id",
		"parent_compute_id", "kind", "display_name", "address", "prefix_length",
		"compute_id", "scope", "role", "primary", "domain", "registrar_provider_id",
		"registered_on", "expires_on", "auto_renew", "nameservers", "tag_ids",
		"archived", "revision", "state", "result", "count":
		return true
	default:
		return false
	}
}
func nullableAuditString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type AuditFilter struct {
	ActorType, ActorID, Action, ResourceType, ResourceID string
	FromTime, ToTime                                     string
	Limit                                                int
	Cursor                                               string
}
type AuditRecord struct {
	Sequence     int64         `json:"sequence"`
	ID           string        `json:"id"`
	OccurredAt   string        `json:"occurred_at"`
	ActorType    string        `json:"actor_type"`
	ActorID      string        `json:"actor_id"`
	Action       string        `json:"action"`
	ResourceType string        `json:"resource_type"`
	ResourceID   *string       `json:"resource_id,omitempty"`
	RequestID    *string       `json:"request_id,omitempty"`
	Changes      []AuditChange `json:"changes"`
	Result       string        `json:"result"`
}
type AuditChange struct {
	Path     string  `json:"path"`
	Before   *string `json:"before,omitempty"`
	After    *string `json:"after,omitempty"`
	Redacted bool    `json:"redacted"`
}
type AuditPage struct {
	Items []AuditRecord `json:"items"`
	Page  AuditPageInfo `json:"page"`
}

type AuditPageInfo struct {
	Size       int     `json:"size"`
	HasMore    bool    `json:"has_more"`
	NextCursor *string `json:"next_cursor"`
}

// ReadAuditPage returns newest-first records using a filter-bound sequence cursor.
func ReadAuditPage(ctx context.Context, q auditQueryer, f AuditFilter) (AuditPage, error) {
	if f.Limit == 0 {
		f.Limit = 50
	}
	if f.Limit < 1 || f.Limit > 200 {
		return AuditPage{}, fmt.Errorf("audit page limit must be 1..200")
	}
	after := int64(^uint64(0) >> 1)
	if f.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(f.Cursor)
		if err != nil {
			return AuditPage{}, fmt.Errorf("invalid audit cursor")
		}
		parts := strings.SplitN(string(raw), ":", 2)
		if len(parts) != 2 || parts[1] != auditFilterKey(f) {
			return AuditPage{}, fmt.Errorf("audit cursor filter mismatch")
		}
		after, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return AuditPage{}, fmt.Errorf("invalid audit cursor")
		}
	}
	rows, err := q.QueryContext(ctx, `SELECT sequence,id,occurred_at,actor_type,actor_id,action,resource_type,resource_id,request_id,before_document,after_document,result FROM audit_events WHERE sequence<? AND (?='' OR actor_type=?) AND (?='' OR actor_id=?) AND (?='' OR action=?) AND (?='' OR resource_type=?) AND (?='' OR resource_id=?) AND (?='' OR occurred_at>=?) AND (?='' OR occurred_at<=?) ORDER BY sequence DESC LIMIT ?`, after, f.ActorType, f.ActorType, f.ActorID, f.ActorID, f.Action, f.Action, f.ResourceType, f.ResourceType, f.ResourceID, f.ResourceID, f.FromTime, f.FromTime, f.ToTime, f.ToTime, f.Limit+1)
	if err != nil {
		return AuditPage{}, err
	}
	defer rows.Close()
	page := AuditPage{Items: make([]AuditRecord, 0, f.Limit)}
	for rows.Next() {
		var a AuditRecord
		var resource, request, before, afterDoc sql.NullString
		if err := rows.Scan(&a.Sequence, &a.ID, &a.OccurredAt, &a.ActorType, &a.ActorID, &a.Action, &a.ResourceType, &resource, &request, &before, &afterDoc, &a.Result); err != nil {
			return AuditPage{}, err
		}
		if resource.Valid {
			a.ResourceID = &resource.String
		}
		if request.Valid {
			a.RequestID = &request.String
		}
		a.Changes = auditChanges(before, afterDoc)
		page.Items = append(page.Items, a)
		if len(page.Items) > f.Limit {
			page.Items = page.Items[:f.Limit]
			cursor := base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(page.Items[len(page.Items)-1].Sequence, 10) + ":" + auditFilterKey(f)))
			page.Page.HasMore = true
			page.Page.NextCursor = &cursor
			break
		}
	}
	page.Page.Size = len(page.Items)
	return page, rows.Err()
}

type auditQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func auditFilterKey(f AuditFilter) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s", f.ActorType, f.ActorID, f.Action, f.ResourceType, f.ResourceID, f.FromTime+"|"+f.ToTime)
}
func auditChanges(before, after sql.NullString) []AuditChange {
	var b, a map[string]any
	if before.Valid {
		_ = json.Unmarshal([]byte(before.String), &b)
	}
	if after.Valid {
		_ = json.Unmarshal([]byte(after.String), &a)
	}
	keys := map[string]bool{}
	for k := range b {
		keys[k] = true
	}
	for k := range a {
		keys[k] = true
	}
	out := make([]AuditChange, 0, len(keys))
	for k := range keys {
		bv, bok := b[k]
		av, aok := a[k]
		bs, as := auditValue(bv, bok), auditValue(av, aok)
		if bs == as {
			continue
		}
		red := auditText(bs) == `"[REDACTED]"` || auditText(as) == `"[REDACTED]"`
		out = append(out, AuditChange{Path: "/" + k, Before: bs, After: as, Redacted: red})
	}
	return out
}
func auditText(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func auditValue(v any, ok bool) *string {
	if !ok {
		return nil
	}
	b, _ := json.Marshal(v)
	s := string(b)
	return &s
}
