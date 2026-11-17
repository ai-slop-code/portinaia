package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"portinaia/internal/database"
	webassets "portinaia/web"
)

type healthResponse struct {
	Status string    `json:"status"`
	Time   time.Time `json:"time"`
}

type readinessCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type readinessResponse struct {
	Status string           `json:"status"`
	Time   time.Time        `json:"time"`
	Checks []readinessCheck `json:"checks"`
}

type HandlerConfig struct {
	PublicURL         string
	TrustedProxyCIDRs []string
	Observability     *Observability
}

func ValidateHandlerConfig(config HandlerConfig) error {
	u, err := url.Parse(config.PublicURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("public URL must be an absolute HTTP(S) URL")
	}
	if u.Scheme == "http" {
		host := strings.ToLower(u.Hostname())
		loopback := host == "localhost"
		if address := net.ParseIP(host); address != nil && address.IsLoopback() {
			loopback = true
		}
		if !loopback {
			return fmt.Errorf("public URL must use HTTPS except for loopback development")
		}
	}
	for _, cidr := range config.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR")
		}
	}
	return nil
}

func NewHandler(databases ...*sql.DB) http.Handler {
	return NewConfiguredHandler(HandlerConfig{PublicURL: "http://localhost"}, databases...)
}

func NewConfiguredHandler(config HandlerConfig, databases ...*sql.DB) http.Handler {
	publicURL, err := url.Parse(config.PublicURL)
	secureCookie := err == nil && strings.EqualFold(publicURL.Scheme, "https")
	router := chi.NewRouter()
	router.Use(requestIdentityMiddleware)
	if config.Observability != nil {
		router.Use(config.Observability.middleware)
	}
	router.Route("/api/v1", func(api chi.Router) {
		api.Use(problemMiddleware)
		api.Use(uuidPathMiddleware)
		api.Use(trustedProxyMiddleware(config.TrustedProxyCIDRs))
		api.Get("/health/live", healthHandler)
		api.Get("/health/ready", readinessHandler(databases...))
		if len(databases) > 0 && databases[0] != nil {
			api.Post("/auth/login", loginHandler(databases[0], secureCookie))
			api.Get("/auth/session", sessionHandler(databases[0]))
			api.Post("/auth/logout", logoutHandler(databases[0], secureCookie))
			api.Get("/audit-events", listAuditEventsHandler(databases[0]))
			api.Get("/providers", listProvidersHandler(databases[0]))
			api.Post("/providers", createProviderHandler(databases[0]))
			api.Get("/providers/{provider_id}", getProviderHandler(databases[0]))
			api.Put("/providers/{provider_id}", updateProviderHandler(databases[0]))
			api.Post("/providers/{provider_id}/archive", lifecycleHandler(databases[0], "provider", "archive"))
			api.Post("/providers/{provider_id}/restore", lifecycleHandler(databases[0], "provider", "restore"))
			api.Get("/locations", listLocationsHandler(databases[0]))
			api.Post("/locations", createLocationHandler(databases[0]))
			api.Get("/locations/{location_id}", getLocationHandler(databases[0]))
			api.Put("/locations/{location_id}", updateLocationHandler(databases[0]))
			api.Post("/locations/{location_id}/archive", lifecycleHandler(databases[0], "location", "archive"))
			api.Post("/locations/{location_id}/restore", lifecycleHandler(databases[0], "location", "restore"))
			api.Get("/computes", listComputesHandler(databases[0]))
			api.Get("/computes/{compute_id}/containers", listComputeContainersHandler(databases[0]))
			api.Post("/computes", createComputeHandler(databases[0]))
			api.Get("/computes/{compute_id}", getComputeHandler(databases[0]))
			api.Put("/computes/{compute_id}", updateComputeHandler(databases[0]))
			api.Post("/computes/{compute_id}/archive", lifecycleHandler(databases[0], "compute", "archive"))
			api.Post("/computes/{compute_id}/restore", lifecycleHandler(databases[0], "compute", "restore"))
			api.Get("/computes/{compute_id}/summary", computeSummaryHandler(databases[0]))
			api.Get("/computes/{compute_id}/collector-status", computeCollectorStatusHandler(databases[0]))
			api.Get("/inventory/tree", inventoryTreeHandler(databases[0]))
			api.Get("/inventory/tree/{parent_id}/children", inventoryChildrenHandler(databases[0]))
			api.Get("/inventory/summary", inventorySummaryHandler(databases[0]))
			api.Get("/ip-assignments", listIPAssignmentsHandler(databases[0]))
			api.Post("/ip-assignments", createIPAssignmentHandler(databases[0]))
			api.Get("/ip-assignments/{ip_assignment_id}", getIPAssignmentHandler(databases[0]))
			api.Put("/ip-assignments/{ip_assignment_id}", updateIPAssignmentHandler(databases[0]))
			api.Post("/ip-assignments/{ip_assignment_id}/archive", lifecycleHandler(databases[0], "ip_assignment", "archive"))
			api.Post("/ip-assignments/{ip_assignment_id}/restore", lifecycleHandler(databases[0], "ip_assignment", "restore"))
			api.Get("/domains", listDomainsHandler(databases[0]))
			api.Post("/domains", createDomainHandler(databases[0]))
			api.Get("/domains/{domain_id}", getDomainHandler(databases[0]))
			api.Put("/domains/{domain_id}", updateDomainHandler(databases[0]))
			api.Post("/domains/{domain_id}/archive", lifecycleHandler(databases[0], "domain", "archive"))
			api.Post("/domains/{domain_id}/restore", lifecycleHandler(databases[0], "domain", "restore"))
			api.Get("/containers", listContainersHandler(databases[0], ""))
			api.Get("/containers/{container_id}", getContainerHandler(databases[0]))
			api.Post("/computes/{compute_id}/enrollment-tokens", createEnrollmentTokenHandler(databases[0]))
			api.Delete("/enrollment-tokens/{token_id}", revokeEnrollmentTokenHandler(databases[0]))
			api.Post("/agent/v1/enroll", enrollAgentHandler(databases[0]))
			api.Post("/agent/v1/heartbeat", heartbeatHandler(databases[0]))
			api.Put("/agent/v1/snapshots/{report_id}", putAgentSnapshotHandler(databases[0]))
			api.Post("/agents/{agent_id}/revoke", revokeAgentHandler(databases[0]))
			api.Get("/agents/{agent_id}", getAgentHandler(databases[0]))
		}
		api.NotFound(apiNotFoundHandler)
	})
	router.Get("/*", frontendHandler())
	return router
}

func uuidPathMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if invalidUUIDPath(r.URL.Path) {
			writeProblemForRequest(w, r, http.StatusBadRequest, "invalid_path_parameter", "resource path identifier must be a canonical UUIDv4 or UUIDv7")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func invalidUUIDPath(path string) bool {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	idAfter := map[string]bool{"providers": true, "locations": true, "computes": true, "ip-assignments": true, "domains": true, "containers": true, "agents": true, "enrollment-tokens": true, "snapshots": true, "tree": true}
	for index := 1; index < len(segments); index++ {
		if idAfter[segments[index-1]] && !validUUIDString(segments[index]) {
			return true
		}
	}
	return false
}

func readinessHandler(databases ...*sql.DB) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		checks := []readinessCheck{{Name: "schema", Status: "not_ready"}, {Name: "database", Status: "not_ready"}}
		if len(databases) == 0 || databases[0] == nil {
			checks[1].Message = "database unavailable"
			writeReadinessProblem(response, request, "database_unavailable", "database unavailable", checks)
			return
		}
		ctx, cancel := context.WithTimeout(request.Context(), 500*time.Millisecond)
		defer cancel()
		if err := database.CheckSchema(ctx, databases[0]); err != nil {
			checks[0].Message = "database schema is unsupported or inconsistent"
			checks[1].Status = "ready"
			writeReadinessProblem(response, request, "schema_unsupported", checks[0].Message, checks)
			return
		}
		checks[0].Status = "ready"
		tx, err := databases[0].BeginTx(ctx, nil)
		if err == nil {
			_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portinaia_ready_probe (value INTEGER)`)
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO portinaia_ready_probe(value) VALUES (1)`)
		}
		if tx != nil {
			_ = tx.Rollback()
		}
		if err != nil {
			checks[1].Message = "database read/write probe failed"
			writeReadinessProblem(response, request, "database_unavailable", checks[1].Message, checks)
			return
		}
		checks[1].Status = "ready"
		writeJSON(response, http.StatusOK, readinessResponse{Status: "ready", Time: time.Now().UTC(), Checks: checks})
	}
}

func healthHandler(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, healthResponse{Status: "live", Time: time.Now().UTC()})
}

func writeReadinessProblem(w http.ResponseWriter, r *http.Request, code, detail string, checks []readinessCheck) {
	requestID := requestIdentity(r.Context())
	if requestID == "" {
		requestID, _ = randomID()
		requestID = "req_" + requestID
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(problemResponse{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Detail: detail, Code: code, RequestID: requestID, Readiness: &readinessResponse{Status: "not_ready", Time: time.Now().UTC(), Checks: checks}})
}

func apiNotFoundHandler(response http.ResponseWriter, request *http.Request) {
	writeProblemForRequest(response, request, http.StatusNotFound, "not_found", "API route not found")
}

func frontendHandler() http.HandlerFunc {
	assets := webassets.Assets()
	files := http.FileServer(http.FS(assets))

	return func(response http.ResponseWriter, request *http.Request) {
		path := request.URL.Path[1:]
		if path != "" {
			if info, err := fs.Stat(assets, path); err == nil && !info.IsDir() {
				files.ServeHTTP(response, request)
				return
			}
		}

		index, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.Error(response, "frontend unavailable", http.StatusInternalServerError)
			return
		}

		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = response.Write(index)
	}
}
