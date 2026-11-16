package server

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type treeNode struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Archived    bool   `json:"archived"`
	HasChildren bool   `json:"has_children"`
}

type treeCursor struct {
	Kind     string `json:"k"`
	Name     string `json:"n"`
	ID       string `json:"i"`
	Archived string `json:"a"`
	ParentID string `json:"p"`
	Root     bool   `json:"r"`
}

type treePage struct {
	Items []treeNode `json:"items"`
	Page  any        `json:"page"`
}

func inventoryTreeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		items, more, next, err := loadTreePage(r, db, "", "", true)
		if err != nil {
			writeTreeError(w, err)
			return
		}
		writeTreePage(w, items, more, next, treePageSize(r))
	}
}

func inventoryChildrenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		parentID := chi.URLParam(r, "parent_id")
		var kind string
		err := db.QueryRowContext(r.Context(), `SELECT 'location' FROM locations WHERE id=? UNION ALL SELECT 'compute' FROM computes WHERE id=? LIMIT 1`, parentID, parentID).Scan(&kind)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		items, more, next, err := loadTreePage(r, db, parentID, kind, false)
		if err != nil {
			writeTreeError(w, err)
			return
		}
		writeTreePage(w, items, more, next, treePageSize(r))
	}
}

func writeTreePage(w http.ResponseWriter, items []treeNode, more bool, next *string, size int) {
	writeJSON(w, http.StatusOK, treePage{Items: items, Page: map[string]any{"size": size, "has_more": more, "next_cursor": next}})
}

func treePageSize(r *http.Request) int {
	value := r.URL.Query().Get("page[size]")
	if value == "" {
		return 50
	}
	size, err := strconv.Atoi(value)
	if err != nil || size < 1 || size > 200 {
		return 0
	}
	return size
}

func writeTreeError(w http.ResponseWriter, err error) {
	if strings.HasPrefix(err.Error(), "invalid") {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func loadTreePage(r *http.Request, db *sql.DB, parentID, parentKind string, root bool) ([]treeNode, bool, *string, error) {
	archived := r.URL.Query().Get("archived")
	if archived == "" {
		archived = "exclude"
	}
	if archived != "exclude" && archived != "include" && archived != "only" {
		return nil, false, nil, errors.New("invalid archived filter")
	}
	size := treePageSize(r)
	if size == 0 {
		return nil, false, nil, errors.New("invalid page size")
	}
	after := treeCursor{}
	if raw := r.URL.Query().Get("page[after]"); raw != "" {
		if !validCursorLength(raw) {
			return nil, false, nil, errors.New("invalid page cursor")
		}
		payload, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || json.Unmarshal(payload, &after) != nil || after.Kind == "" || after.Archived != archived || after.ParentID != parentID || after.Root != root {
			return nil, false, nil, errors.New("invalid page cursor")
		}
	}
	type source struct {
		kind, nameColumn, from, where, activeDescendantContext string
		args                                                   []any
	}
	sources := []source{
		{kind: "location", nameColumn: "name", from: "locations", where: treeArchivedSQL("archived_at", archived), activeDescendantContext: `EXISTS (WITH RECURSIVE descendants(id) AS (SELECT child.id FROM locations child WHERE child.parent_id=locations.id UNION ALL SELECT child.id FROM locations child JOIN descendants d ON child.parent_id=d.id) SELECT 1 FROM locations child JOIN descendants d ON child.id=d.id WHERE child.archived_at IS NULL) OR EXISTS (WITH RECURSIVE location_descendants(id) AS (SELECT locations.id UNION ALL SELECT child.id FROM locations child JOIN location_descendants d ON child.parent_id=d.id), compute_descendants(id) AS (SELECT c.id FROM computes c JOIN location_descendants l ON c.location_id=l.id WHERE c.parent_compute_id IS NULL UNION ALL SELECT child.id FROM computes child JOIN compute_descendants d ON child.parent_compute_id=d.id) SELECT 1 FROM computes active JOIN compute_descendants d ON active.id=d.id WHERE active.archived_at IS NULL) OR EXISTS (WITH RECURSIVE location_descendants(id) AS (SELECT locations.id UNION ALL SELECT child.id FROM locations child JOIN location_descendants d ON child.parent_id=d.id), compute_descendants(id) AS (SELECT c.id FROM computes c JOIN location_descendants l ON c.location_id=l.id WHERE c.parent_compute_id IS NULL UNION ALL SELECT child.id FROM computes child JOIN compute_descendants d ON child.parent_compute_id=d.id) SELECT 1 FROM containers active JOIN compute_descendants d ON active.compute_id=d.id WHERE active.archived_at IS NULL)`},
		{kind: "compute", nameColumn: "display_name", from: "computes", where: treeArchivedSQL("archived_at", archived), activeDescendantContext: `EXISTS (WITH RECURSIVE descendants(id) AS (SELECT child.id FROM computes child WHERE child.parent_compute_id=computes.id UNION ALL SELECT child.id FROM computes child JOIN descendants d ON child.parent_compute_id=d.id) SELECT 1 FROM computes child JOIN descendants d ON child.id=d.id WHERE child.archived_at IS NULL) OR EXISTS (WITH RECURSIVE descendants(id) AS (SELECT computes.id UNION ALL SELECT child.id FROM computes child JOIN descendants d ON child.parent_compute_id=d.id) SELECT 1 FROM containers c JOIN descendants d ON c.compute_id=d.id WHERE c.archived_at IS NULL)`},
		{kind: "container", nameColumn: "name", from: "containers", where: treeArchivedSQL("archived_at", archived), args: []any{}},
	}
	if archived == "exclude" {
		for i := range sources {
			if sources[i].activeDescendantContext != "" {
				sources[i].where = "(archived_at IS NULL OR (" + sources[i].activeDescendantContext + "))"
			}
		}
	}
	if root {
		sources[0].where += ` AND parent_id IS NULL`
		sources[1].where += ` AND parent_compute_id IS NULL AND location_id IS NULL`
		sources = sources[:2]
	} else {
		switch parentKind {
		case "location":
			sources[0].where += ` AND parent_id=?`
			sources[0].args = []any{parentID}
			sources[1].where += ` AND location_id=? AND parent_compute_id IS NULL`
			sources[1].args = []any{parentID}
			sources = sources[:2]
		case "compute":
			sources[0].where = "0=1"
			sources[1].where += ` AND parent_compute_id=?`
			sources[1].args = []any{parentID}
			sources[2].where += ` AND compute_id=?`
			sources[2].args = []any{parentID}
		default:
			return nil, false, nil, errors.New("invalid tree parent")
		}
	}
	nodes := make([]treeNode, 0)
	for _, item := range sources {
		if after.Kind != "" {
			if item.kind < after.Kind {
				continue
			}
			if item.kind == after.Kind {
				item.where += ` AND (` + item.nameColumn + ` COLLATE NOCASE>? OR (` + item.nameColumn + ` COLLATE NOCASE=? AND id>?))`
				item.args = append(item.args, after.Name, after.Name, after.ID)
			}
		}
		query := `SELECT id,` + item.nameColumn + `,archived_at FROM ` + item.from + ` WHERE ` + item.where + ` ORDER BY ` + item.nameColumn + ` COLLATE NOCASE,id LIMIT ?`
		args := append(item.args, size+1)
		rows, err := db.QueryContext(r.Context(), query, args...)
		if err != nil {
			return nil, false, nil, err
		}
		for rows.Next() {
			var node treeNode
			var archivedAt sql.NullString
			node.Kind = item.kind
			if err := rows.Scan(&node.ID, &node.Name, &archivedAt); err != nil {
				rows.Close()
				return nil, false, nil, err
			}
			node.Archived = archivedAt.Valid
			nodes = append(nodes, node)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, false, nil, err
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Kind != nodes[j].Kind {
			return nodes[i].Kind < nodes[j].Kind
		}
		if sqliteNoCase(nodes[i].Name) != sqliteNoCase(nodes[j].Name) {
			return sqliteNoCase(nodes[i].Name) < sqliteNoCase(nodes[j].Name)
		}
		return nodes[i].ID < nodes[j].ID
	})
	more := len(nodes) > size
	if more {
		nodes = nodes[:size]
	}
	for i := range nodes {
		hasChildren, err := treeHasChildren(r, db, nodes[i], archived)
		if err != nil {
			return nil, false, nil, err
		}
		nodes[i].HasChildren = hasChildren
	}
	var next *string
	if more && len(nodes) > 0 {
		last := nodes[len(nodes)-1]
		payload, _ := json.Marshal(treeCursor{Kind: last.Kind, Name: last.Name, ID: last.ID, Archived: archived, ParentID: parentID, Root: root})
		cursor := base64.RawURLEncoding.EncodeToString(payload)
		next = &cursor
	}
	return nodes, more, next, nil
}

func treeArchivedSQL(field, filter string) string {
	switch filter {
	case "only":
		return field + " IS NOT NULL"
	case "include":
		return "1=1"
	default:
		return field + " IS NULL"
	}
}

func treeHasChildren(r *http.Request, db *sql.DB, node treeNode, archived string) (bool, error) {
	var count int
	var err error
	locationContext := treeNodeContextPredicate("location", "child")
	computeContext := treeNodeContextPredicate("compute", "child")
	switch node.Kind {
	case "location":
		locationFilter := treeArchivedSQL("child.archived_at", archived)
		computeFilter := treeArchivedSQL("child.archived_at", archived)
		if archived == "exclude" {
			locationFilter = `(child.archived_at IS NULL OR (` + locationContext + `))`
			computeFilter = `(child.archived_at IS NULL OR (` + computeContext + `))`
		}
		err = db.QueryRowContext(r.Context(), `SELECT (SELECT count(*) FROM locations child WHERE child.parent_id=? AND `+locationFilter+`) + (SELECT count(*) FROM computes child WHERE child.location_id=? AND child.parent_compute_id IS NULL AND `+computeFilter+`)`, node.ID, node.ID).Scan(&count)
	case "compute":
		computeFilter := treeArchivedSQL("child.archived_at", archived)
		if archived == "exclude" {
			computeFilter = `(child.archived_at IS NULL OR (` + computeContext + `))`
		}
		err = db.QueryRowContext(r.Context(), `SELECT (SELECT count(*) FROM computes child WHERE child.parent_compute_id=? AND `+computeFilter+`) + (SELECT count(*) FROM containers WHERE compute_id=? AND `+treeArchivedSQL("archived_at", archived)+`)`, node.ID, node.ID).Scan(&count)
	}
	return count > 0, err
}

func treeNodeContextPredicate(kind, alias string) string {
	switch kind {
	case "location":
		return `EXISTS (WITH RECURSIVE descendants(id) AS (SELECT ` + alias + `.id UNION ALL SELECT child.id FROM locations child JOIN descendants d ON child.parent_id=d.id) SELECT 1 FROM locations active JOIN descendants d ON active.id=d.id WHERE active.archived_at IS NULL) OR EXISTS (WITH RECURSIVE location_descendants(id) AS (SELECT ` + alias + `.id UNION ALL SELECT child.id FROM locations child JOIN location_descendants d ON child.parent_id=d.id), compute_descendants(id) AS (SELECT c.id FROM computes c JOIN location_descendants l ON c.location_id=l.id WHERE c.parent_compute_id IS NULL UNION ALL SELECT child.id FROM computes child JOIN compute_descendants d ON child.parent_compute_id=d.id) SELECT 1 FROM computes active JOIN compute_descendants d ON active.id=d.id WHERE active.archived_at IS NULL) OR EXISTS (WITH RECURSIVE location_descendants(id) AS (SELECT ` + alias + `.id UNION ALL SELECT child.id FROM locations child JOIN location_descendants d ON child.parent_id=d.id), compute_descendants(id) AS (SELECT c.id FROM computes c JOIN location_descendants l ON c.location_id=l.id WHERE c.parent_compute_id IS NULL UNION ALL SELECT child.id FROM computes child JOIN compute_descendants d ON child.parent_compute_id=d.id) SELECT 1 FROM containers active JOIN compute_descendants d ON active.compute_id=d.id WHERE active.archived_at IS NULL)`
	case "compute":
		return `EXISTS (WITH RECURSIVE descendants(id) AS (SELECT ` + alias + `.id UNION ALL SELECT child.id FROM computes child JOIN descendants d ON child.parent_compute_id=d.id) SELECT 1 FROM computes active JOIN descendants d ON active.id=d.id WHERE active.archived_at IS NULL) OR EXISTS (WITH RECURSIVE descendants(id) AS (SELECT ` + alias + `.id UNION ALL SELECT child.id FROM computes child JOIN descendants d ON child.parent_compute_id=d.id) SELECT 1 FROM containers active JOIN descendants d ON active.compute_id=d.id WHERE active.archived_at IS NULL)`
	default:
		return "0=1"
	}
}

// SQLite's built-in NOCASE collation folds ASCII only. Match it here so merged
// pages from the three entity tables use the same order as the SQL keyset filters.
func sqliteNoCase(value string) string {
	return strings.Map(func(character rune) rune {
		if character >= 'A' && character <= 'Z' {
			return character + ('a' - 'A')
		}
		return character
	}, value)
}

func computeSummaryHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		id := chi.URLParam(r, "compute_id")
		var c computeResource
		var provider, providerResource, location, parent sql.NullString
		err := db.QueryRowContext(r.Context(), `SELECT id,kind,display_name,ownership,provider_id,provider_resource_id,location_id,parent_compute_id,description,notes,revision,created_at,updated_at FROM computes WHERE id=? AND archived_at IS NULL`, id).Scan(&c.ID, &c.Kind, &c.DisplayName, &c.Ownership, &provider, &providerResource, &location, &parent, &c.Description, &c.Notes, &c.Revision, &c.CreatedAt, &c.UpdatedAt)
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
		c.AgentID, c.AgentStatus, err = computeAgentInfo(r.Context(), db, id)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		c.Observation, err = loadComputeObservation(r.Context(), db, id)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		c.TagIDs = []string{}
		var containerCount, staleCount, ipCount, costCount, dnsCount, snapshotCount int
		queries := []struct {
			sql    string
			target *int
		}{{`SELECT count(*) FROM containers WHERE compute_id=? AND archived_at IS NULL`, &containerCount}, {`SELECT count(*) FROM containers WHERE compute_id=? AND archived_at IS NULL AND stale_at IS NOT NULL`, &staleCount}, {`SELECT count(*) FROM ip_assignments WHERE compute_id=? AND archived_at IS NULL`, &ipCount}, {`SELECT count(*) FROM costs WHERE compute_id=? AND archived_at IS NULL`, &costCount}, {`SELECT count(*) FROM dns_inventory_links WHERE compute_id=?`, &dnsCount}, {`SELECT count(*) FROM discovery_reports r JOIN agents a ON a.id=r.agent_id WHERE a.compute_id=?`, &snapshotCount}}
		for _, query := range queries {
			if err := db.QueryRowContext(r.Context(), query.sql, id).Scan(query.target); err != nil {
				http.Error(w, "internal error", 500)
				return
			}
		}
		writeJSON(w, 200, map[string]any{"compute": c, "container_count": containerCount, "stale_container_count": staleCount, "ip_assignment_count": ipCount, "cost_count": costCount, "dns_link_count": dnsCount, "snapshot_count": snapshotCount})
	}
}

func inventorySummaryHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authenticate(db, r); !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		var computeCount, physicalCount, vpsCount, vmCount, containers, stale int
		err := db.QueryRowContext(r.Context(), `SELECT count(*),coalesce(sum(kind='physical'),0),coalesce(sum(kind='vps'),0),coalesce(sum(kind='vm'),0) FROM computes WHERE archived_at IS NULL`).Scan(&computeCount, &physicalCount, &vpsCount, &vmCount)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		if err := db.QueryRowContext(r.Context(), `SELECT count(*),coalesce(sum(ct.stale_at IS NOT NULL),0) FROM containers ct JOIN computes c ON c.id=ct.compute_id WHERE ct.archived_at IS NULL AND c.archived_at IS NULL`).Scan(&containers, &stale); err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		writeJSON(w, 200, map[string]int{"compute_count": computeCount, "physical_compute_count": physicalCount, "vps_count": vpsCount, "vm_count": vmCount, "observed_container_count": containers, "stale_container_count": stale})
	}
}
