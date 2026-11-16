package server

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// listAuditEventsHandler exposes the already-redacted audit projection. Audit
// reads deliberately use the browser session boundary rather than accepting an
// agent credential, and the cursor remains bound to all supplied filters by
// ReadAuditPage.
func listAuditEventsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			writeProblemForRequest(w, r, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		values := r.URL.Query()
		for _, name := range []string{"from_time", "to_time"} {
			if value := strings.TrimSpace(values.Get(name)); value != "" {
				if _, err := time.Parse(time.RFC3339, value); err != nil {
					writeProblemForRequest(w, r, http.StatusBadRequest, "invalid_request", name+" must be an RFC3339 timestamp")
					return
				}
			}
		}
		for _, name := range []string{"actor_id", "resource_id"} {
			if value := strings.TrimSpace(values.Get(name)); value != "" && !validUUIDString(value) {
				writeProblemForRequest(w, r, http.StatusBadRequest, "invalid_request", name+" must be a UUID")
				return
			}
		}
		if actorType := strings.TrimSpace(values.Get("actor_type")); actorType != "" {
			switch actorType {
			case "administrator", "personal_access_token", "agent", "system":
			default:
				writeProblemForRequest(w, r, http.StatusBadRequest, "invalid_request", "actor_type is not supported")
				return
			}
		}
		limit := 50
		if raw := strings.TrimSpace(values.Get("page[size]")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				writeProblemForRequest(w, r, http.StatusBadRequest, "invalid_request", "page[size] must be an integer")
				return
			}
			limit = parsed
		}
		filter := AuditFilter{
			Limit:        limit,
			Cursor:       strings.TrimSpace(values.Get("page[after]")),
			FromTime:     strings.TrimSpace(values.Get("from_time")),
			ToTime:       strings.TrimSpace(values.Get("to_time")),
			ActorType:    strings.TrimSpace(values.Get("actor_type")),
			ActorID:      strings.TrimSpace(values.Get("actor_id")),
			Action:       strings.TrimSpace(values.Get("action")),
			ResourceType: strings.TrimSpace(values.Get("resource_type")),
			ResourceID:   strings.TrimSpace(values.Get("resource_id")),
		}
		page, err := ReadAuditPage(r.Context(), db, filter)
		if err != nil {
			writeProblemForRequest(w, r, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, page)
	}
}
