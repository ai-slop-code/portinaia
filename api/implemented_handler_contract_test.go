package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"portinaia/internal/database"
	"portinaia/internal/server"
)

func TestImplementedProviderHandlersMatchOpenAPIResponses(t *testing.T) {
	db, err := database.Open(t.TempDir() + "/provider-contract.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := server.BootstrapAdministrator(db, "contract-admin", "contract-test-password"); err != nil {
		t.Fatal(err)
	}
	handler := server.NewHandler(db)
	document := loadContract(t)
	for _, item := range []struct {
		path   string
		schema string
	}{{"/api/v1/health/live", "HealthStatus"}, {"/api/v1/health/ready", "ReadinessStatus"}} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, item.path, nil))
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %s response: %v", item.path, err)
		}
		schema := item.schema
		if response.Code == http.StatusServiceUnavailable {
			schema = "Problem"
		}
		if err := document.Components.Schemas[schema].Value.VisitJSON(body); err != nil {
			t.Fatalf("actual %s response does not match OpenAPI %s: %v", item.path, schema, err)
		}
	}

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"contract-admin","password":"contract-test-password"}`))
	login.Header.Set("Content-Type", "application/json")
	login.Header.Set("Origin", "http://example.com")
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", loginResponse.Code, loginResponse.Body.String())
	}
	var session struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.NewDecoder(loginResponse.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	cookie := loginResponse.Result().Cookies()[0]

	create := httptest.NewRequest(http.MethodPost, "/api/v1/providers", strings.NewReader(`{"name":"Contract provider","website":null,"external_account_reference":null,"roles":["hosting_provider"],"tag_ids":[]}`))
	create.AddCookie(cookie)
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("X-CSRF-Token", session.CSRFToken)
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated || created.Header().Get("ETag") != `"revision-1"` {
		t.Fatalf("provider create status/etag = %d/%q: %s", created.Code, created.Header().Get("ETag"), created.Body.String())
	}
	var provider map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &provider); err != nil {
		t.Fatal(err)
	}
	if err := validateOperationResponse(document, "/providers", http.MethodPost, created.Code, provider); err != nil {
		t.Fatalf("actual provider response does not match complete OpenAPI operation response: %v; response=%s", err, created.Body.String())
	}
	providerID := provider["id"].(string)
	updateBody := `{"name":"Contract provider edited","roles":["hosting_provider"],"tag_ids":[]}`
	update := httptest.NewRequest(http.MethodPut, "/api/v1/providers/"+providerID, strings.NewReader(updateBody))
	update.AddCookie(cookie)
	update.Header.Set("Content-Type", "application/json")
	update.Header.Set("X-CSRF-Token", session.CSRFToken)
	update.Header.Set("If-Match", created.Header().Get("ETag"))
	updated := httptest.NewRecorder()
	handler.ServeHTTP(updated, update)
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") != `"revision-2"` {
		t.Fatalf("provider update status/etag = %d/%q: %s", updated.Code, updated.Header().Get("ETag"), updated.Body.String())
	}
	var updatedProvider map[string]any
	if err := json.Unmarshal(updated.Body.Bytes(), &updatedProvider); err != nil {
		t.Fatal(err)
	}
	if err := validateOperationResponse(document, "/providers/{provider_id}", http.MethodPut, updated.Code, updatedProvider); err != nil {
		t.Fatalf("actual provider update does not match complete OpenAPI operation response: %v", err)
	}
	stale := httptest.NewRequest(http.MethodPut, "/api/v1/providers/"+providerID, strings.NewReader(updateBody))
	stale.AddCookie(cookie)
	stale.Header.Set("Content-Type", "application/json")
	stale.Header.Set("X-CSRF-Token", session.CSRFToken)
	stale.Header.Set("If-Match", created.Header().Get("ETag"))
	staleResponse := httptest.NewRecorder()
	handler.ServeHTTP(staleResponse, stale)
	var staleProblem map[string]any
	if err := json.Unmarshal(staleResponse.Body.Bytes(), &staleProblem); err != nil {
		t.Fatal(err)
	}
	if staleResponse.Code != http.StatusPreconditionFailed || staleResponse.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("stale provider update = %d %q: %s", staleResponse.Code, staleResponse.Header().Get("Content-Type"), staleResponse.Body.String())
	}
	if err := document.Components.Schemas["Problem"].Value.VisitJSON(staleProblem); err != nil {
		t.Fatalf("actual stale-revision response does not match OpenAPI Problem: %v", err)
	}

	for _, body := range []string{
		`{"name":"Unknown field","roles":["hosting_provider"],"tag_ids":[],"surprise":1}`,
		`{"name":"Trailing data","roles":["hosting_provider"],"tag_ids":[]} {}`,
		`{"name":null,"roles":["hosting_provider"],"tag_ids":[]}`,
		`{"name":"Invalid null","description":null,"roles":["hosting_provider"],"tag_ids":[]}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/providers", strings.NewReader(body))
		request.AddCookie(cookie)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", session.CSRFToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || response.Header().Get("Content-Type") != "application/problem+json" {
			t.Fatalf("invalid body result = %d %q: %s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
		}
		var problem map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
			t.Fatal(err)
		}
		if err := document.Components.Schemas["Problem"].Value.VisitJSON(problem); err != nil {
			t.Fatalf("actual problem response does not match OpenAPI Problem: %v; response=%s", err, response.Body.String())
		}
	}
	oversized := httptest.NewRequest(http.MethodPost, "/api/v1/providers", strings.NewReader(strings.Repeat(" ", 32*1024+1)))
	oversized.AddCookie(cookie)
	oversized.Header.Set("Content-Type", "application/json")
	oversized.Header.Set("X-CSRF-Token", session.CSRFToken)
	oversizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(oversizedResponse, oversized)
	if oversizedResponse.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized request status = %d: %s", oversizedResponse.Code, oversizedResponse.Body.String())
	}
	var oversizedProblem map[string]any
	if err := json.Unmarshal(oversizedResponse.Body.Bytes(), &oversizedProblem); err != nil {
		t.Fatal(err)
	}
	if err := document.Components.Schemas["Problem"].Value.VisitJSON(oversizedProblem); err != nil {
		t.Fatalf("oversized request problem does not match OpenAPI: %v", err)
	}

	unknown := httptest.NewRequest(http.MethodGet, "/api/v1/not-implemented", nil)
	unknownResponse := httptest.NewRecorder()
	handler.ServeHTTP(unknownResponse, unknown)
	if unknownResponse.Code != http.StatusNotFound || unknownResponse.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("unknown API path response = %d %q", unknownResponse.Code, unknownResponse.Header().Get("Content-Type"))
	}
	var unknownProblem map[string]any
	if err := json.Unmarshal(unknownResponse.Body.Bytes(), &unknownProblem); err != nil {
		t.Fatal(err)
	}
	if err := document.Components.Schemas["Problem"].Value.VisitJSON(unknownProblem); err != nil {
		t.Fatalf("unknown route problem does not match OpenAPI: %v", err)
	}
	invalidID := httptest.NewRequest(http.MethodGet, "/api/v1/providers/not-a-uuid", nil)
	invalidID.AddCookie(cookie)
	invalidIDResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidIDResponse, invalidID)
	if invalidIDResponse.Code != http.StatusBadRequest || invalidIDResponse.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("malformed UUID path parameter = %d %q: %s", invalidIDResponse.Code, invalidIDResponse.Header().Get("Content-Type"), invalidIDResponse.Body.String())
	}
}

func TestRegisteredAPIRoutesHaveOpenAPIOperations(t *testing.T) {
	db, err := database.Open(t.TempDir() + "/route-contract.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	routes, ok := server.NewHandler(db).(chi.Routes)
	if !ok {
		t.Fatal("NewHandler did not expose a chi route tree")
	}
	document := loadContract(t)
	count := 0
	err = chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/api/v1/") {
			return nil
		}
		path := strings.TrimPrefix(route, "/api/v1")
		item := document.Paths.Find(path)
		if item == nil {
			return fmt.Errorf("registered %s %s has no OpenAPI path", method, route)
		}
		operations := item.Operations()
		operation := operations[strings.ToLower(method)]
		if operation == nil {
			operation = operations[strings.ToUpper(method)]
		}
		if operation == nil {
			return fmt.Errorf("registered %s %s has no OpenAPI operation (operations=%v)", method, route, operations)
		}
		if operation.OperationID == "" {
			return fmt.Errorf("registered %s %s maps to an operation without an operationId", method, route)
		}
		count++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no implemented API routes were inventoried")
	}
}
