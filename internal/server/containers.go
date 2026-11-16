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

	"github.com/go-chi/chi/v5"
)

type containerResponse struct {
	ID               string                        `json:"id"`
	ComputeID        string                        `json:"compute_id"`
	Name             string                        `json:"name"`
	CreatedAt        string                        `json:"created_at"`
	Description      *string                       `json:"description,omitempty"`
	Notes            *string                       `json:"notes,omitempty"`
	PodmanOwnerUID   int64                         `json:"podman_owner_uid"`
	PodmanConnection *string                       `json:"podman_connection,omitempty"`
	Revision         int64                         `json:"revision"`
	UpdatedAt        string                        `json:"updated_at"`
	ArchivedAt       *string                       `json:"archived_at,omitempty"`
	TagIDs           []string                      `json:"tag_ids"`
	Observation      *containerObservationResponse `json:"observation,omitempty"`
}

type containerObservationResponse struct {
	RuntimeID                string                     `json:"runtime_id"`
	ImageName                string                     `json:"image_name"`
	ImageID                  *string                    `json:"image_id,omitempty"`
	ImageDigest              *string                    `json:"image_digest,omitempty"`
	State                    string                     `json:"state"`
	Health                   string                     `json:"health"`
	RestartPolicy            string                     `json:"restart_policy"`
	RestartMaximumRetryCount *int                       `json:"restart_maximum_retry_count,omitempty"`
	CreatedAt                *string                    `json:"created_at,omitempty"`
	StartedAt                *string                    `json:"started_at,omitempty"`
	FirstObservedAt          string                     `json:"first_observed_at"`
	LastObservedAt           string                     `json:"last_observed_at"`
	StaleAt                  *string                    `json:"stale_at,omitempty"`
	Ports                    []containerPortResponse    `json:"ports"`
	Networks                 []containerNetworkResponse `json:"networks"`
	Mounts                   []containerMountResponse   `json:"mounts"`
	Labels                   []containerLabelResponse   `json:"labels"`
}

type containerPortResponse struct {
	ContainerPort int     `json:"container_port"`
	Protocol      string  `json:"protocol"`
	HostIP        *string `json:"host_ip,omitempty"`
	HostPort      *int    `json:"host_port,omitempty"`
}
type containerNetworkResponse struct {
	Name      string                     `json:"name"`
	NetworkID *string                    `json:"network_id,omitempty"`
	Addresses []containerAddressResponse `json:"addresses"`
}
type containerAddressResponse struct {
	Address      string `json:"address"`
	PrefixLength *int   `json:"prefix_length,omitempty"`
}
type containerMountResponse struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"read_only"`
}
type containerLabelResponse struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
type containerCursor struct {
	Name       string `json:"n"`
	ID         string `json:"i"`
	Archived   string `json:"a"`
	Search     string `json:"q"`
	Compute    string `json:"c"`
	Stale      string `json:"s"`
	Collection string `json:"l"`
}

func listContainersHandler(db *sql.DB, computeFilter string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		archived := q.Get("archived")
		if archived == "" {
			archived = "exclude"
		}
		if archived != "exclude" && archived != "include" && archived != "only" {
			http.Error(w, "invalid archived filter", http.StatusBadRequest)
			return
		}
		search := strings.TrimSpace(q.Get("q"))
		if len(search) > 255 {
			http.Error(w, "invalid search", http.StatusBadRequest)
			return
		}
		computeID := q.Get("compute_id")
		if computeFilter != "" {
			computeID = computeFilter
		}
		pageSize := 50
		if raw := q.Get("page[size]"); raw != "" {
			size, err := strconv.Atoi(raw)
			if err != nil || size < 1 || size > 200 {
				http.Error(w, "invalid page size", http.StatusBadRequest)
				return
			}
			pageSize = size
		}
		afterName, afterID := "", ""
		var decodedCursor *containerCursor
		if raw := q.Get("page[after]"); raw != "" {
			if !validCursorLength(raw) {
				http.Error(w, "invalid cursor", http.StatusBadRequest)
				return
			}
			cursor, err := decodeContainerCursor(raw)
			if err != nil {
				http.Error(w, "invalid cursor", http.StatusBadRequest)
				return
			}
			decodedCursor = &cursor
			afterName, afterID = cursor.Name, cursor.ID
		}
		stale := q.Get("stale")
		if stale != "" && stale != "true" && stale != "false" {
			http.Error(w, "invalid stale filter", http.StatusBadRequest)
			return
		}
		collection := "containers"
		if computeFilter != "" {
			collection = "compute_containers"
		}
		if decodedCursor != nil && (decodedCursor.Name == "" || decodedCursor.ID == "" || decodedCursor.Archived != archived || decodedCursor.Search != search || decodedCursor.Compute != computeID || decodedCursor.Stale != stale || decodedCursor.Collection != collection) {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
		staleClause := "1=1"
		if stale == "true" {
			staleClause = "stale_at IS NOT NULL"
		} else if stale == "false" {
			staleClause = "stale_at IS NULL"
		}
		query := `SELECT id,compute_id,podman_owner_uid,podman_connection,name,description,notes,revision,created_at,updated_at,archived_at
FROM containers
WHERE ` + treeArchivedSQL("archived_at", archived) + `
  AND (?='' OR compute_id=?)
  AND (?='' OR name LIKE '%'||?||'%' COLLATE NOCASE)
  AND ` + staleClause + `
  AND (name>? OR (name=? AND id>?))
ORDER BY name,id LIMIT ?`
		rows, err := db.QueryContext(r.Context(), query, computeID, computeID, search, search, afterName, afterName, afterID, pageSize+1)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		items := make([]containerResponse, 0, pageSize+1)
		for rows.Next() {
			item, err := scanContainerBase(rows)
			if err != nil {
				rows.Close()
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		rows.Close()
		more := len(items) > pageSize
		if more {
			items = items[:pageSize]
		}
		for i := range items {
			items[i].Observation, err = loadContainerObservation(r.Context(), db, items[i].ID)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
		}
		var next *string
		if more && len(items) > 0 {
			last := items[len(items)-1]
			value := encodeContainerCursor(containerCursor{Name: last.Name, ID: last.ID, Archived: archived, Search: search, Compute: computeID, Stale: stale, Collection: collection})
			next = &value
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "page": map[string]any{"size": pageSize, "has_more": more, "next_cursor": next}})
	}
}

func listComputeContainersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		computeID := chi.URLParam(r, "compute_id")
		var count int
		if err := db.QueryRowContext(r.Context(), `SELECT count(*) FROM computes WHERE id=?`, computeID).Scan(&count); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if count == 0 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		listContainersHandler(db, computeID)(w, r)
	}
}

func getContainerHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		item, err := loadContainer(r.Context(), db, chi.URLParam(r, "container_id"))
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("ETag", revisionETag(item.Revision))
		writeJSON(w, http.StatusOK, item)
	}
}

type containerScanner interface{ Scan(...any) error }

func scanContainerBase(row containerScanner) (containerResponse, error) {
	var item containerResponse
	var description, notes, connection, archived sql.NullString
	err := row.Scan(&item.ID, &item.ComputeID, &item.PodmanOwnerUID, &connection, &item.Name, &description, &notes, &item.Revision, &item.CreatedAt, &item.UpdatedAt, &archived)
	if err != nil {
		return item, err
	}
	item.Description = nullableString(description)
	item.Notes = nullableString(notes)
	item.PodmanConnection = nullableString(connection)
	item.ArchivedAt = nullableString(archived)
	item.TagIDs = []string{}
	return item, nil
}

func loadContainer(ctx context.Context, db *sql.DB, id string) (containerResponse, error) {
	query := `SELECT id,compute_id,podman_owner_uid,podman_connection,name,description,notes,revision,created_at,updated_at,archived_at FROM containers WHERE id=?`
	item, err := scanContainerBase(db.QueryRowContext(ctx, query, id))
	if err != nil {
		return item, err
	}
	item.Observation, err = loadContainerObservation(ctx, db, id)
	return item, err
}

func loadContainerObservation(ctx context.Context, db *sql.DB, id string) (*containerObservationResponse, error) {
	var o containerObservationResponse
	var imageID, imageDigest, created, started, stale sql.NullString
	var retries sql.NullInt64
	query := `SELECT runtime_id,image_name,image_id,image_digest,state,health,restart_policy,restart_max_attempts,runtime_created_at,runtime_started_at,first_observed_at,last_observed_at,stale_at FROM containers WHERE id=?`
	err := db.QueryRowContext(ctx, query, id).Scan(&o.RuntimeID, &o.ImageName, &imageID, &imageDigest, &o.State, &o.Health, &o.RestartPolicy, &retries, &created, &started, &o.FirstObservedAt, &o.LastObservedAt, &stale)
	if err != nil {
		return nil, err
	}
	o.ImageID, o.ImageDigest = nullableString(imageID), nullableString(imageDigest)
	o.CreatedAt, o.StartedAt, o.StaleAt = nullableString(created), nullableString(started), nullableString(stale)
	if retries.Valid {
		retry := int(retries.Int64)
		o.RestartMaximumRetryCount = &retry
	}
	if o.RestartPolicy == "on-failure" {
		o.RestartPolicy = "on_failure"
	} else if o.RestartPolicy == "unless-stopped" {
		o.RestartPolicy = "unless_stopped"
	}
	o.Ports, err = loadContainerPorts(ctx, db, id)
	if err != nil {
		return nil, err
	}
	o.Networks, err = loadContainerNetworks(ctx, db, id)
	if err != nil {
		return nil, err
	}
	o.Mounts, err = loadContainerMounts(ctx, db, id)
	if err != nil {
		return nil, err
	}
	o.Labels, err = loadContainerLabels(ctx, db, id)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func loadContainerPorts(ctx context.Context, db *sql.DB, id string) ([]containerPortResponse, error) {
	rows, err := db.QueryContext(ctx, `SELECT protocol,container_port,host_ip,host_port FROM container_ports WHERE container_id=? ORDER BY protocol,container_port,host_ip,host_port`, id)
	if err != nil {
		return nil, err
	}
	items := []containerPortResponse{}
	for rows.Next() {
		var item containerPortResponse
		var ip sql.NullString
		var host sql.NullInt64
		if err := rows.Scan(&item.Protocol, &item.ContainerPort, &ip, &host); err != nil {
			rows.Close()
			return nil, err
		}
		if ip.Valid && ip.String != "" {
			item.HostIP = &ip.String
		}
		if host.Valid {
			value := int(host.Int64)
			item.HostPort = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	return items, rows.Close()
}

func loadContainerNetworks(ctx context.Context, db *sql.DB, id string) ([]containerNetworkResponse, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,name,network_id FROM container_networks WHERE container_id=? ORDER BY name`, id)
	if err != nil {
		return nil, err
	}
	type networkRow struct {
		id        int64
		name      string
		networkID sql.NullString
	}
	var networks []networkRow
	for rows.Next() {
		var item networkRow
		if err := rows.Scan(&item.id, &item.name, &item.networkID); err != nil {
			rows.Close()
			return nil, err
		}
		networks = append(networks, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	items := make([]containerNetworkResponse, 0, len(networks))
	for _, network := range networks {
		item := containerNetworkResponse{Name: network.name, Addresses: []containerAddressResponse{}}
		if network.networkID.Valid && network.networkID.String != "" {
			item.NetworkID = &network.networkID.String
		}
		addresses, err := db.QueryContext(ctx, `SELECT address,prefix_length FROM container_addresses WHERE container_id=? AND network_id=? ORDER BY address`, id, network.id)
		if err != nil {
			return nil, err
		}
		for addresses.Next() {
			var address containerAddressResponse
			var prefix sql.NullInt64
			if err := addresses.Scan(&address.Address, &prefix); err != nil {
				addresses.Close()
				return nil, err
			}
			if prefix.Valid {
				value := int(prefix.Int64)
				address.PrefixLength = &value
			}
			item.Addresses = append(item.Addresses, address)
		}
		if err := addresses.Err(); err != nil {
			addresses.Close()
			return nil, err
		}
		if err := addresses.Close(); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func loadContainerMounts(ctx context.Context, db *sql.DB, id string) ([]containerMountResponse, error) {
	rows, err := db.QueryContext(ctx, `SELECT type,source,destination,read_only FROM container_mounts WHERE container_id=? ORDER BY destination`, id)
	if err != nil {
		return nil, err
	}
	items := []containerMountResponse{}
	for rows.Next() {
		var item containerMountResponse
		var readOnly int64
		if err := rows.Scan(&item.Type, &item.Source, &item.Destination, &readOnly); err != nil {
			rows.Close()
			return nil, err
		}
		item.ReadOnly = readOnly == 1
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	return items, rows.Close()
}
func loadContainerLabels(ctx context.Context, db *sql.DB, id string) ([]containerLabelResponse, error) {
	rows, err := db.QueryContext(ctx, `SELECT key,value FROM container_labels WHERE container_id=? ORDER BY key`, id)
	if err != nil {
		return nil, err
	}
	items := []containerLabelResponse{}
	for rows.Next() {
		var item containerLabelResponse
		if err := rows.Scan(&item.Key, &item.Value); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	return items, rows.Close()
}

func encodeContainerCursor(cursor containerCursor) string {
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}
func decodeContainerCursor(value string) (containerCursor, error) {
	var cursor containerCursor
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return cursor, err
	}
	err = json.Unmarshal(raw, &cursor)
	return cursor, err
}
