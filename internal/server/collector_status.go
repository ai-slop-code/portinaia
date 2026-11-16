package server

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type collectorObservation struct {
	Key                    string  `json:"collector_key"`
	Collector              string  `json:"collector"`
	OwnerUID               *int64  `json:"owner_uid"`
	ConnectionName         *string `json:"connection_name"`
	LastStatus             string  `json:"last_status"`
	LastReceivedAt         string  `json:"last_received_at"`
	LastReportID           *string `json:"last_report_id,omitempty"`
	LastSuccessfulAt       *string `json:"last_successful_at,omitempty"`
	LastSuccessfulReportID *string `json:"last_successful_report_id,omitempty"`
	LastFailedAt           *string `json:"last_failed_at,omitempty"`
	LastFailedReportID     *string `json:"last_failed_report_id,omitempty"`
	LastErrorCode          *string `json:"last_error_code,omitempty"`
	LastErrorMessage       *string `json:"last_error_message,omitempty"`
}

func computeCollectorStatusHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		computeID := chi.URLParam(r, "compute_id")
		var exists int
		if err := db.QueryRowContext(r.Context(), `SELECT count(*) FROM computes WHERE id=?`, computeID).Scan(&exists); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if exists == 0 {
			http.Error(w, "not found", 404)
			return
		}
		rows, err := db.QueryContext(r.Context(), `SELECT collector_key,collector_name,owner_uid,connection_name,last_status,last_received_at,last_report_id,last_successful_at,last_successful_report_id,last_failed_at,last_failed_report_id,last_error_code,last_error_message FROM compute_collector_observations WHERE compute_id=? ORDER BY collector_name,owner_uid,collector_key`, computeID)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		items := make([]collectorObservation, 0, 8)
		for rows.Next() {
			var item collectorObservation
			var uid sql.NullInt64
			var connection, lastReport, lastSuccess, lastSuccessReport, lastFailed, lastFailedReport, errorCode, errorMessage sql.NullString
			if err := rows.Scan(&item.Key, &item.Collector, &uid, &connection, &item.LastStatus, &item.LastReceivedAt, &lastReport, &lastSuccess, &lastSuccessReport, &lastFailed, &lastFailedReport, &errorCode, &errorMessage); err != nil {
				rows.Close()
				http.Error(w, "internal error", 500)
				return
			}
			if uid.Valid {
				value := uid.Int64
				item.OwnerUID = &value
			}
			item.ConnectionName = nullableString(connection)
			item.LastReportID = nullableString(lastReport)
			item.LastSuccessfulAt = nullableString(lastSuccess)
			item.LastSuccessfulReportID = nullableString(lastSuccessReport)
			item.LastFailedAt = nullableString(lastFailed)
			item.LastFailedReportID = nullableString(lastFailedReport)
			item.LastErrorCode = nullableString(errorCode)
			item.LastErrorMessage = nullableString(errorMessage)
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			http.Error(w, "internal error", 500)
			return
		}
		rows.Close()
		writeJSON(w, 200, map[string]any{"items": items})
	}
}
