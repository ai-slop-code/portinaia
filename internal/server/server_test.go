package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apigen "portinaia/internal/api/generated"
	"portinaia/internal/database"
)

func TestHealthEndpoints(t *testing.T) {
	db, err := database.Open(t.TempDir() + "/health.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{"/api/v1/health/live", "/api/v1/health/ready"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			NewHandler(db).ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}

			if path == "/api/v1/health/live" {
				var body healthResponse
				if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
					t.Fatalf("decode liveness response: %v", err)
				}
				if body.Status != "live" || body.Time.IsZero() {
					t.Fatalf("liveness body = %+v", body)
				}
			} else {
				var body readinessResponse
				if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
					t.Fatalf("decode readiness response: %v", err)
				}
				if body.Status != "ready" || body.Time.IsZero() || len(body.Checks) != 2 {
					t.Fatalf("readiness body = %+v", body)
				}
			}
		})
	}
}

func TestReadinessReportsUnsupportedSchemaAsProblem(t *testing.T) {
	db, err := database.Open(t.TempDir() + "/unsupported.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version=8`); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	response := httptest.NewRecorder()
	NewHandler(db).ServeHTTP(response, request)
	var problem problemResponse
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusServiceUnavailable || problem.Readiness == nil || problem.Readiness.Status != "not_ready" || problem.Readiness.Checks[0].Status != "not_ready" {
		t.Fatalf("readiness schema failure = %d %+v", response.Code, problem)
	}
}

func TestFrontendAndClientRoute(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/", "/inventory"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		NewHandler().ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d", path, response.Code, http.StatusOK)
		}
		if !strings.Contains(response.Body.String(), `<div id="root"></div>`) {
			t.Fatalf("GET %s did not return the frontend entry point", path)
		}
	}
}

func TestUnknownAPIRouteDoesNotReturnFrontend(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	response := httptest.NewRecorder()
	NewHandler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if strings.Contains(response.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("unknown API route returned HTML")
	}
	if response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("unknown API route content type = %q", response.Header().Get("Content-Type"))
	}
	var problem problemResponse
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil || problem.Status != http.StatusNotFound || problem.RequestID == "" {
		t.Fatalf("unknown API problem = %+v, decode error = %v", problem, err)
	}
}

func TestAdministratorSessionLifecycle(t *testing.T) {
	db, err := database.Open(t.TempDir() + "/auth.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapAdministrator(db, "admin", "a-long-test-password"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHandlerConfig(HandlerConfig{PublicURL: "https://inventory.example", TrustedProxyCIDRs: []string{"192.0.2.0/24"}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateHandlerConfig(HandlerConfig{PublicURL: "http://0.0.0.0:8080"}); err == nil {
		t.Fatal("non-loopback public HTTP URL accepted")
	}
	if err := ValidateHandlerConfig(HandlerConfig{PublicURL: "http://127.example:8080"}); err == nil {
		t.Fatal("non-loopback hostname with a 127 prefix accepted")
	}
	if err := ValidateHandlerConfig(HandlerConfig{PublicURL: "https://inventory.example", TrustedProxyCIDRs: []string{"192.0.2.999/24"}}); err == nil {
		t.Fatal("invalid trusted proxy CIDR accepted")
	}
	handler := NewHandler(db)

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"a-long-test-password"}`))
	login.Header.Set("Origin", "http://example.com")
	login.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", loginResponse.Code, loginResponse.Body.String())
	}
	cookie := loginResponse.Result().Cookies()[0]
	if cookie.Secure {
		t.Fatal("loopback HTTP session cookie unexpectedly secure")
	}
	spoofedScheme := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"a-long-test-password"}`))
	spoofedScheme.Header.Set("Origin", "http://example.com")
	spoofedScheme.Header.Set("X-Forwarded-Proto", "https")
	spoofedResponse := httptest.NewRecorder()
	handler.ServeHTTP(spoofedResponse, spoofedScheme)
	if spoofedResponse.Result().Cookies()[0].Secure {
		t.Fatal("untrusted forwarded scheme changed cookie security")
	}
	proxyHandler := NewConfiguredHandler(HandlerConfig{PublicURL: "https://inventory.example", TrustedProxyCIDRs: []string{"127.0.0.1/32"}}, db)
	proxyLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"a-long-test-password"}`))
	proxyLogin.Header.Set("Origin", "http://example.com")
	proxyLogin.Header.Set("X-Forwarded-Proto", "http")
	proxyResponse := httptest.NewRecorder()
	proxyHandler.ServeHTTP(proxyResponse, proxyLogin)
	if !proxyResponse.Result().Cookies()[0].Secure || proxyResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("public HTTPS configuration did not force secure/no-store session behavior")
	}
	var session sessionInfo
	if err := json.NewDecoder(loginResponse.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRequest(http.MethodPost, "/api/v1/providers", strings.NewReader(`{"name":"Example Host","website":"https://example.test","roles":["hosting_provider"],"tag_ids":[]}`))
	create.AddCookie(cookie)
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("X-CSRF-Token", session.CSRFToken)
	createResponse := httptest.NewRecorder()
	handler.ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("provider create status = %d: %s", createResponse.Code, createResponse.Body.String())
	}
	var provider providerResource
	providerJSON := append([]byte(nil), createResponse.Body.Bytes()...)
	if err := json.NewDecoder(createResponse.Body).Decode(&provider); err != nil {
		t.Fatal(err)
	}
	var generatedProvider apigen.Provider
	if err := json.Unmarshal(providerJSON, &generatedProvider); err != nil {
		t.Fatalf("decode generated provider transport: %v", err)
	}
	if generatedProvider.Id.String() != provider.ID || len(generatedProvider.Roles) != 1 {
		t.Fatalf("generated provider response = %+v", generatedProvider)
	}
	if provider.Name != "Example Host" || len(provider.Roles) != 1 || provider.Roles[0] != "hosting_provider" {
		t.Fatalf("provider response = %+v", provider)
	}
	if provider.ID[14] != '7' {
		t.Fatalf("new provider ID = %q, want UUIDv7", provider.ID)
	}
	for _, invalidBody := range []string{
		`{"name":"Invalid Extra","roles":["hosting_provider"],"tag_ids":[],"extra":true}`,
		`{"name":"Invalid Trailing","roles":["hosting_provider"],"tag_ids":[]} {}`,
	} {
		invalid := httptest.NewRequest(http.MethodPost, "/api/v1/providers", strings.NewReader(invalidBody))
		invalid.AddCookie(cookie)
		invalid.Header.Set("Content-Type", "application/json")
		invalid.Header.Set("X-CSRF-Token", session.CSRFToken)
		invalidResponse := httptest.NewRecorder()
		handler.ServeHTTP(invalidResponse, invalid)
		var problem problemResponse
		if err := json.NewDecoder(invalidResponse.Body).Decode(&problem); err != nil || invalidResponse.Code != http.StatusBadRequest || problem.Status != http.StatusBadRequest {
			t.Fatalf("invalid JSON request result = %d %+v decode=%v", invalidResponse.Code, problem, err)
		}
	}
	providerETag := createResponse.Header().Get("ETag")
	if providerETag != `"revision-1"` {
		t.Fatalf("provider creation ETag = %q, want strong revision-1 tag", providerETag)
	}
	updateProvider := func(etag, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/providers/"+provider.ID, strings.NewReader(`{"name":"Example Hosting","website":"https://example.test","roles":["hosting_provider","registrar"],"tag_ids":[]}`))
		req.AddCookie(cookie)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", session.CSRFToken)
		req.Header.Set("If-Match", etag)
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	providerUpdateResponse := updateProvider(providerETag, "provider-update-retry")
	if providerUpdateResponse.Code != http.StatusOK {
		t.Fatalf("provider update status = %d: %s", providerUpdateResponse.Code, providerUpdateResponse.Body.String())
	}
	replayedProviderUpdate := updateProvider(providerETag, "provider-update-retry")
	if replayedProviderUpdate.Code != http.StatusOK || replayedProviderUpdate.Header().Get("ETag") != providerUpdateResponse.Header().Get("ETag") {
		t.Fatalf("provider update replay status/etag = %d/%q", replayedProviderUpdate.Code, replayedProviderUpdate.Header().Get("ETag"))
	}
	if staleResponse := updateProvider(providerETag, ""); staleResponse.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale provider update status = %d", staleResponse.Code)
	}
	for _, etag := range []string{"", `W/"revision-1"`, `"revision-01"`} {
		response := updateProvider(etag, "")
		if response.Code != http.StatusBadRequest || response.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("malformed/missing If-Match %q = %d %s", etag, response.Code, response.Body.String())
		}
	}
	providerETag = providerUpdateResponse.Header().Get("ETag")
	provider.Name = "Example Hosting"
	legacyProviderID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	legacyLocationID, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	legacyTime := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO providers(id,name,description,notes,revision,created_at,updated_at) VALUES(?,'Legacy V4 Provider','legacy description','legacy notes',1,?,?)`, legacyProviderID, legacyTime, legacyTime); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO provider_roles(provider_id,role) VALUES(?,'hosting')`, legacyProviderID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO locations(id,type,name,provider_id,created_at,updated_at) VALUES(?,'site','Legacy linked location',?,?,?)`, legacyLocationID, legacyProviderID, legacyTime, legacyTime); err != nil {
		t.Fatal(err)
	}
	legacyProviderGet := httptest.NewRequest(http.MethodGet, "/api/v1/providers/"+legacyProviderID, nil)
	legacyProviderGet.AddCookie(cookie)
	legacyProviderResponse := httptest.NewRecorder()
	handler.ServeHTTP(legacyProviderResponse, legacyProviderGet)
	if legacyProviderResponse.Code != http.StatusOK || !strings.Contains(legacyProviderResponse.Body.String(), legacyProviderID) {
		t.Fatalf("legacy UUIDv4 provider read = %d %s", legacyProviderResponse.Code, legacyProviderResponse.Body.String())
	}
	legacyProviderUpdate := httptest.NewRequest(http.MethodPut, "/api/v1/providers/"+legacyProviderID, strings.NewReader(`{"name":"Legacy V4 Provider Edited","roles":["hosting_provider"],"tag_ids":[]}`))
	legacyProviderUpdate.AddCookie(cookie)
	legacyProviderUpdate.Header.Set("Content-Type", "application/json")
	legacyProviderUpdate.Header.Set("X-CSRF-Token", session.CSRFToken)
	legacyProviderUpdate.Header.Set("If-Match", legacyProviderResponse.Header().Get("ETag"))
	legacyUpdateResponse := httptest.NewRecorder()
	handler.ServeHTTP(legacyUpdateResponse, legacyProviderUpdate)
	if legacyUpdateResponse.Code != http.StatusOK || !strings.Contains(legacyUpdateResponse.Body.String(), legacyProviderID) {
		t.Fatalf("legacy UUIDv4 provider update = %d %s", legacyUpdateResponse.Code, legacyUpdateResponse.Body.String())
	}
	var preservedProviderID string
	if err := db.QueryRow(`SELECT provider_id FROM locations WHERE id=?`, legacyLocationID).Scan(&preservedProviderID); err != nil || preservedProviderID != legacyProviderID {
		t.Fatalf("legacy UUIDv4 relationship changed: %q err=%v", preservedProviderID, err)
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	list.AddCookie(cookie)
	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), provider.ID) {
		t.Fatalf("provider list status = %d: %s", listResponse.Code, listResponse.Body.String())
	}
	providerPageRequest := httptest.NewRequest(http.MethodGet, "/api/v1/providers?page[size]=1", nil)
	providerPageRequest.AddCookie(cookie)
	providerPageResponse := httptest.NewRecorder()
	handler.ServeHTTP(providerPageResponse, providerPageRequest)
	var providerPage struct {
		Page struct {
			NextCursor *string `json:"next_cursor"`
		} `json:"page"`
	}
	if err := json.NewDecoder(providerPageResponse.Body).Decode(&providerPage); err != nil || providerPage.Page.NextCursor == nil {
		t.Fatalf("provider cursor response = %s err=%v", providerPageResponse.Body.String(), err)
	}
	changedProviderFilter := httptest.NewRequest(http.MethodGet, "/api/v1/providers?q=changed&page[after]="+*providerPage.Page.NextCursor, nil)
	changedProviderFilter.AddCookie(cookie)
	changedProviderResponse := httptest.NewRecorder()
	handler.ServeHTTP(changedProviderResponse, changedProviderFilter)
	if changedProviderResponse.Code != http.StatusBadRequest || changedProviderResponse.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("cursor reused under a different filter = %d %s", changedProviderResponse.Code, changedProviderResponse.Body.String())
	}
	crossCollectionCursor := httptest.NewRequest(http.MethodGet, "/api/v1/locations?page[after]="+*providerPage.Page.NextCursor, nil)
	crossCollectionCursor.AddCookie(cookie)
	crossCollectionResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossCollectionResponse, crossCollectionCursor)
	if crossCollectionResponse.Code != http.StatusBadRequest {
		t.Fatalf("provider cursor accepted by locations collection: %d %s", crossCollectionResponse.Code, crossCollectionResponse.Body.String())
	}
	for _, query := range []string{"page[size]=201", "page[after]=" + strings.Repeat("a", maxCursorLength+1)} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/providers?"+query, nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || response.Header().Get("Content-Type") != "application/problem+json" {
			t.Errorf("invalid provider pagination query %q = %d %s", query, response.Code, response.Body.String())
		}
	}

	detail := httptest.NewRequest(http.MethodGet, "/api/v1/providers/"+provider.ID, nil)
	detail.AddCookie(cookie)
	detailResponse := httptest.NewRecorder()
	handler.ServeHTTP(detailResponse, detail)
	if detailResponse.Code != http.StatusOK || !strings.Contains(detailResponse.Body.String(), "Example Host") {
		t.Fatalf("provider detail status = %d: %s", detailResponse.Code, detailResponse.Body.String())
	}

	createDomain := httptest.NewRequest(http.MethodPost, "/api/v1/domains", strings.NewReader(`{"name":"Exämple.org.","registrar_provider_id":"`+provider.ID+`","registered_on":"2024-01-01","expires_on":"2027-01-01","auto_renew":false,"nameservers":["NS1.Example.NET.","ns1.example.net"],"description":"owned domain","notes":"","tag_ids":[]}`))
	createDomain.AddCookie(cookie)
	createDomain.Header.Set("Content-Type", "application/json")
	createDomain.Header.Set("X-CSRF-Token", session.CSRFToken)
	createDomainResponse := httptest.NewRecorder()
	handler.ServeHTTP(createDomainResponse, createDomain)
	if createDomainResponse.Code != http.StatusCreated {
		t.Fatalf("domain create status = %d: %s", createDomainResponse.Code, createDomainResponse.Body.String())
	}
	var domain domainResource
	if err := json.NewDecoder(createDomainResponse.Body).Decode(&domain); err != nil {
		t.Fatal(err)
	}
	if domain.Name != "xn--exmple-cua.org" || len(domain.Nameservers) != 1 || domain.Nameservers[0] != "ns1.example.net" || domain.AutoRenew == nil || *domain.AutoRenew {
		t.Fatalf("normalized domain = %+v", domain)
	}
	duplicateDomain := httptest.NewRequest(http.MethodPost, "/api/v1/domains", strings.NewReader(`{"name":"xn--exmple-cua.org","nameservers":[],"tag_ids":[]}`))
	duplicateDomain.AddCookie(cookie)
	duplicateDomain.Header.Set("Content-Type", "application/json")
	duplicateDomain.Header.Set("X-CSRF-Token", session.CSRFToken)
	duplicateDomainResponse := httptest.NewRecorder()
	handler.ServeHTTP(duplicateDomainResponse, duplicateDomain)
	if duplicateDomainResponse.Code != http.StatusConflict {
		t.Fatalf("duplicate normalized domain status = %d", duplicateDomainResponse.Code)
	}
	domainList := httptest.NewRequest(http.MethodGet, "/api/v1/domains?expiry_state=current", nil)
	domainList.AddCookie(cookie)
	domainListResponse := httptest.NewRecorder()
	handler.ServeHTTP(domainListResponse, domainList)
	if domainListResponse.Code != http.StatusOK || !strings.Contains(domainListResponse.Body.String(), domain.ID) {
		t.Fatalf("domain list status = %d: %s", domainListResponse.Code, domainListResponse.Body.String())
	}
	updateDomain := httptest.NewRequest(http.MethodPut, "/api/v1/domains/"+domain.ID, strings.NewReader(`{"name":"xn--exmple-cua.org","registrar_provider_id":"`+provider.ID+`","nameservers":[],"tag_ids":[]}`))
	updateDomain.AddCookie(cookie)
	updateDomain.Header.Set("Content-Type", "application/json")
	updateDomain.Header.Set("X-CSRF-Token", session.CSRFToken)
	updateDomain.Header.Set("If-Match", createDomainResponse.Header().Get("ETag"))
	updateDomain.Header.Set("Idempotency-Key", "domain-update-retry")
	updateDomainResponse := httptest.NewRecorder()
	handler.ServeHTTP(updateDomainResponse, updateDomain)
	if updateDomainResponse.Code != http.StatusOK {
		t.Fatalf("domain update status = %d: %s", updateDomainResponse.Code, updateDomainResponse.Body.String())
	}
	var updatedDomain domainResource
	if err := json.NewDecoder(updateDomainResponse.Body).Decode(&updatedDomain); err != nil {
		t.Fatal(err)
	}
	if updatedDomain.RegisteredOn != nil || updatedDomain.ExpiresOn != nil || updatedDomain.AutoRenew != nil || len(updatedDomain.Nameservers) != 0 {
		t.Fatalf("unknown domain fields were not preserved: %+v", updatedDomain)
	}
	replayedUpdate := httptest.NewRequest(http.MethodPut, "/api/v1/domains/"+domain.ID, strings.NewReader(`{"name":"xn--exmple-cua.org","registrar_provider_id":"`+provider.ID+`","nameservers":[],"tag_ids":[]}`))
	replayedUpdate.AddCookie(cookie)
	replayedUpdate.Header.Set("Content-Type", "application/json")
	replayedUpdate.Header.Set("X-CSRF-Token", session.CSRFToken)
	replayedUpdate.Header.Set("If-Match", createDomainResponse.Header().Get("ETag"))
	replayedUpdate.Header.Set("Idempotency-Key", "domain-update-retry")
	replayedUpdateResponse := httptest.NewRecorder()
	handler.ServeHTTP(replayedUpdateResponse, replayedUpdate)
	if replayedUpdateResponse.Code != http.StatusOK || replayedUpdateResponse.Header().Get("ETag") != updateDomainResponse.Header().Get("ETag") {
		t.Fatalf("domain update replay status/etag = %d/%q", replayedUpdateResponse.Code, replayedUpdateResponse.Header().Get("ETag"))
	}
	archiveDomain := httptest.NewRequest(http.MethodPost, "/api/v1/domains/"+domain.ID+"/archive", nil)
	archiveDomain.AddCookie(cookie)
	archiveDomain.Header.Set("X-CSRF-Token", session.CSRFToken)
	archiveDomain.Header.Set("If-Match", updateDomainResponse.Header().Get("ETag"))
	archiveDomain.Header.Set("Idempotency-Key", "domain-archive-retry")
	archiveDomainResponse := httptest.NewRecorder()
	handler.ServeHTTP(archiveDomainResponse, archiveDomain)
	if archiveDomainResponse.Code != http.StatusOK {
		t.Fatalf("domain archive status = %d: %s", archiveDomainResponse.Code, archiveDomainResponse.Body.String())
	}
	replayedArchive := httptest.NewRecorder()
	handler.ServeHTTP(replayedArchive, archiveDomain)
	if replayedArchive.Code != http.StatusOK || replayedArchive.Header().Get("ETag") != archiveDomainResponse.Header().Get("ETag") {
		t.Fatalf("domain archive replay status/etag = %d/%q", replayedArchive.Code, replayedArchive.Header().Get("ETag"))
	}
	restoreDomain := httptest.NewRequest(http.MethodPost, "/api/v1/domains/"+domain.ID+"/restore", nil)
	restoreDomain.AddCookie(cookie)
	restoreDomain.Header.Set("X-CSRF-Token", session.CSRFToken)
	restoreDomain.Header.Set("If-Match", archiveDomainResponse.Header().Get("ETag"))
	restoreDomainResponse := httptest.NewRecorder()
	handler.ServeHTTP(restoreDomainResponse, restoreDomain)
	if restoreDomainResponse.Code != http.StatusOK {
		t.Fatalf("domain restore status = %d: %s", restoreDomainResponse.Code, restoreDomainResponse.Body.String())
	}

	createLocation := httptest.NewRequest(http.MethodPost, "/api/v1/locations", strings.NewReader(`{"type":"site","name":"Test Datacenter","provider_id":"`+provider.ID+`","tag_ids":[]}`))
	createLocation.AddCookie(cookie)
	createLocation.Header.Set("Content-Type", "application/json")
	createLocation.Header.Set("X-CSRF-Token", session.CSRFToken)
	createLocationResponse := httptest.NewRecorder()
	handler.ServeHTTP(createLocationResponse, createLocation)
	if createLocationResponse.Code != http.StatusCreated {
		t.Fatalf("location create status = %d: %s", createLocationResponse.Code, createLocationResponse.Body.String())
	}
	locationETag := createLocationResponse.Header().Get("ETag")
	var location locationResource
	if err := json.NewDecoder(createLocationResponse.Body).Decode(&location); err != nil {
		t.Fatal(err)
	}
	locationDetail := httptest.NewRequest(http.MethodGet, "/api/v1/locations/"+location.ID, nil)
	locationDetail.AddCookie(cookie)
	locationDetailResponse := httptest.NewRecorder()
	handler.ServeHTTP(locationDetailResponse, locationDetail)
	if locationDetailResponse.Code != http.StatusOK || !strings.Contains(locationDetailResponse.Body.String(), "Test Datacenter") {
		t.Fatalf("location detail status = %d: %s", locationDetailResponse.Code, locationDetailResponse.Body.String())
	}
	updateLocation := httptest.NewRequest(http.MethodPut, "/api/v1/locations/"+location.ID, strings.NewReader(`{"type":"site","name":"Updated Datacenter","provider_id":"`+provider.ID+`","tag_ids":[]}`))
	updateLocation.AddCookie(cookie)
	updateLocation.Header.Set("Content-Type", "application/json")
	updateLocation.Header.Set("X-CSRF-Token", session.CSRFToken)
	updateLocation.Header.Set("If-Match", locationETag)
	updateLocationResponse := httptest.NewRecorder()
	handler.ServeHTTP(updateLocationResponse, updateLocation)
	if updateLocationResponse.Code != http.StatusOK {
		t.Fatalf("location update status = %d: %s", updateLocationResponse.Code, updateLocationResponse.Body.String())
	}
	locationETag = updateLocationResponse.Header().Get("ETag")
	locationList := httptest.NewRequest(http.MethodGet, "/api/v1/locations", nil)
	locationList.AddCookie(cookie)
	locationListResponse := httptest.NewRecorder()
	handler.ServeHTTP(locationListResponse, locationList)
	if locationListResponse.Code != http.StatusOK || !strings.Contains(locationListResponse.Body.String(), location.ID) {
		t.Fatalf("location list status = %d: %s", locationListResponse.Code, locationListResponse.Body.String())
	}

	createCompute := httptest.NewRequest(http.MethodPost, "/api/v1/computes", strings.NewReader(`{"kind":"physical","display_name":"Test Host","ownership":"owned","provider_id":"`+provider.ID+`","location_id":"`+location.ID+`","tag_ids":[]}`))
	createCompute.AddCookie(cookie)
	createCompute.Header.Set("Content-Type", "application/json")
	createCompute.Header.Set("X-CSRF-Token", session.CSRFToken)
	createComputeResponse := httptest.NewRecorder()
	handler.ServeHTTP(createComputeResponse, createCompute)
	if createComputeResponse.Code != http.StatusCreated {
		t.Fatalf("compute create status = %d: %s", createComputeResponse.Code, createComputeResponse.Body.String())
	}
	computeETag := createComputeResponse.Header().Get("ETag")
	var compute computeResource
	if err := json.NewDecoder(createComputeResponse.Body).Decode(&compute); err != nil {
		t.Fatal(err)
	}
	if compute.AgentStatus != "unenrolled" {
		t.Fatalf("new compute agent status = %q", compute.AgentStatus)
	}
	for _, filter := range []string{"provider_id=" + provider.ID, "location_id=" + location.ID, "freshness=unenrolled"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/computes?"+filter, nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		var page struct {
			Items []computeResource `json:"items"`
		}
		if err := json.NewDecoder(response.Body).Decode(&page); err != nil || response.Code != http.StatusOK || len(page.Items) != 1 || page.Items[0].ID != compute.ID {
			t.Fatalf("compute filter %q = %d %+v err=%v", filter, response.Code, page.Items, err)
		}
	}
	invalidFreshness := httptest.NewRequest(http.MethodGet, "/api/v1/computes?freshness=stale", nil)
	invalidFreshness.AddCookie(cookie)
	invalidFreshnessResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidFreshnessResponse, invalidFreshness)
	if invalidFreshnessResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid compute freshness filter = %d", invalidFreshnessResponse.Code)
	}
	createIP := httptest.NewRequest(http.MethodPost, "/api/v1/ip-assignments", strings.NewReader(`{"address":"192.0.2.10","prefix_length":24,"compute_id":"`+compute.ID+`","interface_or_network":"ens3","scope":"private","role":"management","primary":true}`))
	createIP.AddCookie(cookie)
	createIP.Header.Set("Content-Type", "application/json")
	createIP.Header.Set("X-CSRF-Token", session.CSRFToken)
	createIPResponse := httptest.NewRecorder()
	handler.ServeHTTP(createIPResponse, createIP)
	if createIPResponse.Code != http.StatusCreated {
		t.Fatalf("IP create status = %d: %s", createIPResponse.Code, createIPResponse.Body.String())
	}
	var assignment ipAssignmentResource
	if err := json.NewDecoder(createIPResponse.Body).Decode(&assignment); err != nil {
		t.Fatal(err)
	}
	if assignment.Address != "192.0.2.10" || assignment.Source != "manual" || !assignment.Primary {
		t.Fatalf("IP assignment = %+v", assignment)
	}
	omittedPrimary := httptest.NewRequest(http.MethodPost, "/api/v1/ip-assignments", strings.NewReader(`{"address":"192.0.2.12","compute_id":"`+compute.ID+`","scope":"private"}`))
	omittedPrimary.AddCookie(cookie)
	omittedPrimary.Header.Set("Content-Type", "application/json")
	omittedPrimary.Header.Set("X-CSRF-Token", session.CSRFToken)
	omittedPrimaryResponse := httptest.NewRecorder()
	handler.ServeHTTP(omittedPrimaryResponse, omittedPrimary)
	if omittedPrimaryResponse.Code != http.StatusBadRequest {
		t.Fatalf("omitted required primary status = %d: %s", omittedPrimaryResponse.Code, omittedPrimaryResponse.Body.String())
	}
	duplicatePrimary := httptest.NewRequest(http.MethodPost, "/api/v1/ip-assignments", strings.NewReader(`{"address":"192.0.2.11","compute_id":"`+compute.ID+`","scope":"private","role":"management","primary":true}`))
	duplicatePrimary.AddCookie(cookie)
	duplicatePrimary.Header.Set("Content-Type", "application/json")
	duplicatePrimary.Header.Set("X-CSRF-Token", session.CSRFToken)
	duplicatePrimaryResponse := httptest.NewRecorder()
	handler.ServeHTTP(duplicatePrimaryResponse, duplicatePrimary)
	if duplicatePrimaryResponse.Code != http.StatusConflict {
		t.Fatalf("duplicate primary status = %d: %s", duplicatePrimaryResponse.Code, duplicatePrimaryResponse.Body.String())
	}
	updateIP := httptest.NewRequest(http.MethodPut, "/api/v1/ip-assignments/"+assignment.ID, strings.NewReader(`{"address":"192.0.2.10","prefix_length":25,"compute_id":"`+compute.ID+`","interface_or_network":"ens3","scope":"private","role":"management","primary":true}`))
	updateIP.AddCookie(cookie)
	updateIP.Header.Set("Content-Type", "application/json")
	updateIP.Header.Set("X-CSRF-Token", session.CSRFToken)
	updateIP.Header.Set("If-Match", createIPResponse.Header().Get("ETag"))
	updateIPResponse := httptest.NewRecorder()
	handler.ServeHTTP(updateIPResponse, updateIP)
	if updateIPResponse.Code != http.StatusOK {
		t.Fatalf("IP update status = %d: %s", updateIPResponse.Code, updateIPResponse.Body.String())
	}
	updateCompute := httptest.NewRequest(http.MethodPut, "/api/v1/computes/"+compute.ID, strings.NewReader(`{"kind":"physical","display_name":"Updated Host","ownership":"owned","provider_id":"`+provider.ID+`","location_id":"`+location.ID+`","tag_ids":[]}`))
	updateCompute.AddCookie(cookie)
	updateCompute.Header.Set("Content-Type", "application/json")
	updateCompute.Header.Set("X-CSRF-Token", session.CSRFToken)
	updateCompute.Header.Set("If-Match", computeETag)
	updateComputeResponse := httptest.NewRecorder()
	handler.ServeHTTP(updateComputeResponse, updateCompute)
	if updateComputeResponse.Code != http.StatusOK {
		t.Fatalf("compute update status = %d: %s", updateComputeResponse.Code, updateComputeResponse.Body.String())
	}
	computeETag = updateComputeResponse.Header().Get("ETag")
	createEnrollmentToken := func() enrollmentTokenResponse {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/computes/"+compute.ID+"/enrollment-tokens", strings.NewReader(`{"expires_in_seconds":3600}`))
		req.AddCookie(cookie)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", session.CSRFToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusCreated {
			t.Fatalf("enrollment token status = %d: %s", response.Code, response.Body.String())
		}
		var token enrollmentTokenResponse
		if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
			t.Fatal(err)
		}
		return token
	}
	enrollmentToken := createEnrollmentToken()
	duplicateTokenRequest := httptest.NewRequest(http.MethodPost, "/api/v1/computes/"+compute.ID+"/enrollment-tokens", strings.NewReader(`{}`))
	duplicateTokenRequest.AddCookie(cookie)
	duplicateTokenRequest.Header.Set("Content-Type", "application/json")
	duplicateTokenRequest.Header.Set("X-CSRF-Token", session.CSRFToken)
	duplicateTokenResponse := httptest.NewRecorder()
	handler.ServeHTTP(duplicateTokenResponse, duplicateTokenRequest)
	if duplicateTokenResponse.Code != http.StatusCreated {
		t.Fatalf("outstanding token replacement status = %d", duplicateTokenResponse.Code)
	}
	var replacementToken enrollmentTokenResponse
	if err := json.NewDecoder(duplicateTokenResponse.Body).Decode(&replacementToken); err != nil {
		t.Fatal(err)
	}
	if replacementToken.ID == enrollmentToken.ID {
		t.Fatal("replacement re-used the prior token identity")
	}
	oldToken := enrollmentToken
	enrollmentToken = replacementToken
	badProtocol := httptest.NewRequest(http.MethodPost, "/api/v1/agent/v1/enroll", strings.NewReader(`{"protocol_version":2,"client_version":"e2e-agent"}`))
	badProtocol.Header.Set("Authorization", "Bearer "+enrollmentToken.Token)
	badProtocolResponse := httptest.NewRecorder()
	handler.ServeHTTP(badProtocolResponse, badProtocol)
	if badProtocolResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported enrollment protocol status = %d", badProtocolResponse.Code)
	}
	enrollRequest := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/v1/enroll", strings.NewReader(`{"protocol_version":1,"client_version":"e2e-agent"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	if replayOldToken := enrollRequest(oldToken.Token); replayOldToken.Code != http.StatusUnauthorized {
		t.Fatalf("replaced enrollment token status = %d", replayOldToken.Code)
	}
	enrollResponse := enrollRequest(enrollmentToken.Token)
	if enrollResponse.Code != http.StatusCreated {
		t.Fatalf("enroll status = %d: %s", enrollResponse.Code, enrollResponse.Body.String())
	}
	var enrollment agentEnrollmentResponse
	if err := json.NewDecoder(enrollResponse.Body).Decode(&enrollment); err != nil {
		t.Fatal(err)
	}
	if enrollment.ComputeID != compute.ID || enrollment.Credential == "" || enrollment.Credential == enrollmentToken.Token || enrollment.CredentialGeneration != 1 {
		t.Fatalf("enrollment response = %+v", enrollment)
	}
	blockedTokenRequest := httptest.NewRequest(http.MethodPost, "/api/v1/computes/"+compute.ID+"/enrollment-tokens", strings.NewReader(`{}`))
	blockedTokenRequest.AddCookie(cookie)
	blockedTokenRequest.Header.Set("X-CSRF-Token", session.CSRFToken)
	blockedTokenResponse := httptest.NewRecorder()
	handler.ServeHTTP(blockedTokenResponse, blockedTokenRequest)
	if blockedTokenResponse.Code != http.StatusConflict {
		t.Fatalf("active agent enrollment-token request status = %d", blockedTokenResponse.Code)
	}
	if replay := enrollRequest(enrollmentToken.Token); replay.Code != http.StatusUnauthorized {
		t.Fatalf("enrollment token replay status = %d", replay.Code)
	}
	volatileCounter := 0
	snapshotBody := func(reportID, hostname, hostStatus, podmanStatus string, containers []any) []byte {
		volatileCounter++
		now := time.Now().UTC().Truncate(time.Second)
		host := map[string]any{"status": "failed", "error": "host collector unavailable"}
		if hostStatus == "success" {
			host = map[string]any{"status": "success", "host": map[string]any{"machine_id": "machine-test", "hostname": hostname, "os_name": "linux", "os_version": "test", "kernel_version": "test-kernel", "architecture": "amd64", "logical_cpu_count": 2, "memory_bytes": 1024, "uptime_seconds": int64(volatileCounter)}}
		}
		podman := map[string]any{"connection": "local", "owner_uid": 1000, "status": "failed", "error": "podman collector unavailable"}
		if podmanStatus == "success" {
			podman = map[string]any{"connection": "local", "owner_uid": 1000, "status": "success", "containers": containers}
		}
		body, err := json.Marshal(map[string]any{
			"report_id": reportID, "protocol_version": 1, "client_version": "e2e-agent",
			"collection_started_at": now.Add(-time.Minute).Format(time.RFC3339), "collection_finished_at": now.Format(time.RFC3339),
			"host": host, "disks": map[string]any{"status": "success", "disks": []any{}},
			"interfaces":  map[string]any{"status": "success", "interfaces": []any{}},
			"filesystems": map[string]any{"status": "success", "filesystems": []any{map[string]any{"device": "/dev/root", "filesystem": "ext4", "mount_point": "/", "total_bytes": 1000, "available_bytes": int64(100 - volatileCounter)}}},
			"podman":      []any{podman}, "errors": []any{},
		})
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	reportOne := "00000000-0000-4000-8000-000000000101"
	containerSnapshot := map[string]any{
		"name": "web", "runtime_id": "runtime-1", "image_name": "nginx:latest", "state": "running", "health": "none", "restart_policy": "no",
		"ports":    []any{map[string]any{"container_port": 80, "protocol": "tcp", "host_ip": "0.0.0.0", "host_port": 8080}},
		"networks": []any{map[string]any{"name": "bridge", "network_id": "network-1", "addresses": []any{map[string]any{"address": "10.0.0.2", "prefix_length": 24}}}},
		"mounts":   []any{map[string]any{"type": "bind", "source": "/data", "destination": "/data", "read_only": true}},
		"labels":   []any{map[string]any{"key": "app", "value": "demo"}},
	}
	uploadSnapshot := func(reportID string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/agent/v1/snapshots/"+reportID, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+enrollment.Credential)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", "p3-snapshot-correlation")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	firstSnapshotBody := snapshotBody(reportOne, "node-a", "success", "success", []any{containerSnapshot})
	firstSnapshot := uploadSnapshot(reportOne, firstSnapshotBody)
	if firstSnapshot.Code != http.StatusCreated {
		t.Fatalf("snapshot upload status = %d: %s", firstSnapshot.Code, firstSnapshot.Body.String())
	}
	var firstAcceptance apigen.SnapshotAcceptance
	if err := json.NewDecoder(firstSnapshot.Body).Decode(&firstAcceptance); err != nil {
		t.Fatal(err)
	}
	if firstAcceptance.Id.String() == "" || firstAcceptance.ReportId.String() != reportOne || firstAcceptance.Result != apigen.SnapshotAcceptanceResultAccepted {
		t.Fatalf("snapshot acceptance = %+v", firstAcceptance)
	}
	var acceptedAuditCount int
	var acceptedAuditRequestID sql.NullString
	if err := db.QueryRow(`SELECT count(*),max(request_id) FROM audit_events WHERE action='discovery.snapshot.accept' AND resource_id=?`, firstAcceptance.Id.String()).Scan(&acceptedAuditCount, &acceptedAuditRequestID); err != nil {
		t.Fatal(err)
	}
	if acceptedAuditCount != 1 || !acceptedAuditRequestID.Valid || acceptedAuditRequestID.String != "p3-snapshot-correlation" {
		t.Fatalf("accepted snapshot audit count/request ID = %d/%+v", acceptedAuditCount, acceptedAuditRequestID)
	}
	duplicateSnapshot := uploadSnapshot(reportOne, firstSnapshotBody)
	if duplicateSnapshot.Code != http.StatusOK {
		t.Fatalf("duplicate snapshot status = %d: %s", duplicateSnapshot.Code, duplicateSnapshot.Body.String())
	}
	if err := db.QueryRow(`SELECT count(*) FROM audit_events WHERE action='discovery.snapshot.accept' AND resource_id=?`, firstAcceptance.Id.String()).Scan(&acceptedAuditCount); err != nil {
		t.Fatal(err)
	}
	if acceptedAuditCount != 1 {
		t.Fatalf("duplicate replay emitted %d snapshot acceptance audit events", acceptedAuditCount-1)
	}
	changedSnapshot := uploadSnapshot(reportOne, snapshotBody(reportOne, "node-b", "success", "success", []any{containerSnapshot}))
	if changedSnapshot.Code != http.StatusConflict {
		t.Fatalf("conflicting report ID status = %d", changedSnapshot.Code)
	}
	var currentHostname string
	if err := db.QueryRow(`SELECT hostname FROM compute_observations WHERE compute_id=?`, compute.ID).Scan(&currentHostname); err != nil {
		t.Fatal(err)
	}
	if currentHostname != "node-a" {
		t.Fatalf("host fact after duplicate/conflict = %q", currentHostname)
	}
	rollbackReport := "00000000-0000-4000-8000-000000000199"
	if _, err := db.Exec(`CREATE TRIGGER force_snapshot_audit_failure BEFORE INSERT ON audit_events WHEN NEW.action='discovery.snapshot.accept' BEGIN SELECT RAISE(ABORT,'forced audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	rollbackResponse := uploadSnapshot(rollbackReport, snapshotBody(rollbackReport, "must-rollback", "success", "success", []any{containerSnapshot}))
	if rollbackResponse.Code != http.StatusInternalServerError {
		t.Fatalf("snapshot with failed audit status = %d: %s", rollbackResponse.Code, rollbackResponse.Body.String())
	}
	if _, err := db.Exec(`DROP TRIGGER force_snapshot_audit_failure`); err != nil {
		t.Fatal(err)
	}
	var rolledBackReports int
	if err := db.QueryRow(`SELECT count(*) FROM discovery_reports WHERE agent_id=? AND report_id=?`, enrollment.AgentID, rollbackReport).Scan(&rolledBackReports); err != nil {
		t.Fatal(err)
	}
	if rolledBackReports != 0 {
		t.Fatalf("failed snapshot audit left %d discovery report rows", rolledBackReports)
	}
	if err := db.QueryRow(`SELECT hostname FROM compute_observations WHERE compute_id=?`, compute.ID).Scan(&currentHostname); err != nil || currentHostname != "node-a" {
		t.Fatalf("failed snapshot audit changed current host: hostname=%q err=%v", currentHostname, err)
	}
	oldReportID, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	oldBody := snapshotBody(oldReportID, "obsolete-host", "success", "success", []any{containerSnapshot})
	var oldPayload map[string]any
	if err := json.Unmarshal(oldBody, &oldPayload); err != nil {
		t.Fatal(err)
	}
	oldStarted := time.Now().UTC().Add(-91 * 24 * time.Hour).Truncate(time.Second)
	oldFinished := oldStarted.Add(time.Minute)
	oldPayload["collection_started_at"] = oldStarted.Format(time.RFC3339)
	oldPayload["collection_finished_at"] = oldFinished.Format(time.RFC3339)
	oldBody, err = json.Marshal(oldPayload)
	if err != nil {
		t.Fatal(err)
	}
	oldParsed, err := parseSnapshot(oldBody)
	if err != nil {
		t.Fatalf("parse old report retry fixture: %v", err)
	}
	var sharedContentID string
	if err := db.QueryRow(`SELECT snapshot_content_id FROM discovery_reports WHERE agent_id=? AND report_id=?`, enrollment.AgentID, reportOne).Scan(&sharedContentID); err != nil {
		t.Fatal(err)
	}
	oldHistoryID, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	oldReceived := oldFinished.Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO discovery_reports(id,report_id,agent_id,protocol_version,client_version,received_at,client_collected_at,collection_started_at,collection_finished_at,snapshot_content_id,result,content_reused,request_fingerprint,envelope_version) VALUES(?,?,?,1,?,?,?,?,?,?,'accepted',1,?,1)`, oldHistoryID, oldReportID, enrollment.AgentID, "e2e-agent", oldReceived, oldReceived, oldStarted.Format(time.RFC3339), oldReceived, sharedContentID, oldParsed.Fingerprint); err != nil {
		t.Fatal(err)
	}
	shortReportID, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	shortBody := snapshotBody(shortReportID, "must-not-replace-current", "success", "success", []any{containerSnapshot})
	var shortPayload map[string]any
	if err := json.Unmarshal(shortBody, &shortPayload); err != nil {
		t.Fatal(err)
	}
	shortStarted := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	shortFinished := shortStarted.Add(time.Minute)
	shortPayload["collection_started_at"] = shortStarted.Format(time.RFC3339)
	shortPayload["collection_finished_at"] = shortFinished.Format(time.RFC3339)
	shortBody, err = json.Marshal(shortPayload)
	if err != nil {
		t.Fatal(err)
	}
	shortParsed, err := parseSnapshot(shortBody)
	if err != nil {
		t.Fatal(err)
	}
	shortHistoryID, err := randomID()
	if err != nil {
		t.Fatal(err)
	}
	shortReceived := shortFinished.Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO discovery_reports(id,report_id,agent_id,protocol_version,client_version,received_at,client_collected_at,collection_started_at,collection_finished_at,snapshot_content_id,result,content_reused,request_fingerprint,envelope_version) VALUES(?,?,?,1,?,?,?,?,?,?,'accepted',1,?,1)`, shortHistoryID, shortReportID, enrollment.AgentID, "e2e-agent", shortReceived, shortReceived, shortStarted.Format(time.RFC3339), shortReceived, sharedContentID, shortParsed.Fingerprint); err != nil {
		t.Fatal(err)
	}
	shortCutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	if _, err := database.CleanupExpiredDiscoveryReports(context.Background(), db, shortCutoff, 10); err == nil {
		t.Fatal("cleanup accepted a cutoff shorter than report replay protection")
	}
	shortRetry := uploadSnapshot(shortReportID, shortBody)
	if shortRetry.Code != http.StatusOK {
		t.Fatalf("retained short-window duplicate replay = %d: %s", shortRetry.Code, shortRetry.Body.String())
	}
	if err := db.QueryRow(`SELECT hostname FROM compute_observations WHERE compute_id=?`, compute.ID).Scan(&currentHostname); err != nil || currentHostname != "node-a" {
		t.Fatalf("short-retention replay changed current host: hostname=%q err=%v", currentHostname, err)
	}
	retentionCutoff := time.Now().UTC().Add(-database.MinimumDiscoveryRetention - time.Second).Format(time.RFC3339)
	retention, err := database.CleanupExpiredDiscoveryReports(context.Background(), db, retentionCutoff, 10)
	if err != nil || retention.ReportsDeleted != 1 || retention.ContentsDeleted != 0 {
		t.Fatalf("old report cleanup = %+v err=%v", retention, err)
	}
	oldRetry := uploadSnapshot(oldReportID, oldBody)
	if oldRetry.Code != http.StatusConflict {
		t.Fatalf("retry past retention window status = %d: %s", oldRetry.Code, oldRetry.Body.String())
	}
	if err := db.QueryRow(`SELECT hostname FROM compute_observations WHERE compute_id=?`, compute.ID).Scan(&currentHostname); err != nil || currentHostname != "node-a" {
		t.Fatalf("expired retry overwrote current observation: host=%q err=%v", currentHostname, err)
	}
	reuseReport := "00000000-0000-4000-8000-000000000104"
	reusedResponse := uploadSnapshot(reuseReport, snapshotBody(reuseReport, "node-a", "success", "success", []any{containerSnapshot}))
	if reusedResponse.Code != http.StatusCreated {
		t.Fatalf("content reuse report status = %d: %s", reusedResponse.Code, reusedResponse.Body.String())
	}
	var reusedAcceptance struct {
		ContentReused bool `json:"content_reused"`
	}
	if err := json.NewDecoder(reusedResponse.Body).Decode(&reusedAcceptance); err != nil {
		t.Fatal(err)
	}
	if !reusedAcceptance.ContentReused {
		t.Fatal("equivalent structural snapshot did not reuse immutable content")
	}
	var contentCount int
	if err := db.QueryRow(`SELECT count(*) FROM snapshot_contents`).Scan(&contentCount); err != nil {
		t.Fatal(err)
	}
	if contentCount != 1 {
		t.Fatalf("snapshot content rows = %d, want one shared content record", contentCount)
	}
	var containerID string
	if err := db.QueryRow(`SELECT id FROM containers WHERE compute_id=? AND podman_owner_uid=1000 AND name='web' AND archived_at IS NULL`, compute.ID).Scan(&containerID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE containers SET description='manual description',notes='manual notes',revision=revision+1 WHERE id=?`, containerID); err != nil {
		t.Fatal(err)
	}
	recreatedContainer := make(map[string]any, len(containerSnapshot))
	for key, value := range containerSnapshot {
		recreatedContainer[key] = value
	}
	recreatedContainer["runtime_id"] = "runtime-2"
	recreatedReport := "00000000-0000-4000-8000-000000000105"
	recreatedResponse := uploadSnapshot(recreatedReport, snapshotBody(recreatedReport, "node-a", "success", "success", []any{recreatedContainer}))
	if recreatedResponse.Code != http.StatusCreated {
		t.Fatalf("container recreation report status = %d: %s", recreatedResponse.Code, recreatedResponse.Body.String())
	}
	var preservedID, runtimeID, description, notes string
	if err := db.QueryRow(`SELECT id,runtime_id,description,notes FROM containers WHERE compute_id=? AND podman_owner_uid=1000 AND name='web' AND archived_at IS NULL`, compute.ID).Scan(&preservedID, &runtimeID, &description, &notes); err != nil {
		t.Fatal(err)
	}
	if preservedID != containerID || runtimeID != "runtime-2" || description != "manual description" || notes != "manual notes" {
		t.Fatalf("container recreation lost identity/manual metadata: id=%s runtime=%s description=%q notes=%q", preservedID, runtimeID, description, notes)
	}
	failureReport := "00000000-0000-4000-8000-000000000102"
	failureSnapshot := uploadSnapshot(failureReport, snapshotBody(failureReport, "ignored", "failed", "failed", nil))
	if failureSnapshot.Code != http.StatusCreated {
		t.Fatalf("failed collector snapshot status = %d: %s", failureSnapshot.Code, failureSnapshot.Body.String())
	}
	collectorStatusRequest := httptest.NewRequest(http.MethodGet, "/api/v1/computes/"+compute.ID+"/collector-status", nil)
	collectorStatusRequest.AddCookie(cookie)
	collectorStatusResponse := httptest.NewRecorder()
	handler.ServeHTTP(collectorStatusResponse, collectorStatusRequest)
	var collectorStatus struct {
		Items []collectorObservation `json:"items"`
	}
	if err := json.NewDecoder(collectorStatusResponse.Body).Decode(&collectorStatus); err != nil {
		t.Fatal(err)
	}
	hostFailed, podmanFailed := false, false
	for _, item := range collectorStatus.Items {
		if item.Collector == "host" {
			hostFailed = item.LastStatus == "failed" && item.LastErrorMessage != nil && strings.Contains(*item.LastErrorMessage, "host collector unavailable")
		}
		if item.Collector == "podman" {
			podmanFailed = item.LastStatus == "failed" && item.OwnerUID != nil && *item.OwnerUID == 1000
		}
	}
	if collectorStatusResponse.Code != http.StatusOK || !hostFailed || !podmanFailed {
		t.Fatalf("collector status = %d %+v", collectorStatusResponse.Code, collectorStatus.Items)
	}
	var presence sql.NullString
	if err := db.QueryRow(`SELECT stale_at FROM containers WHERE id=?`, containerID).Scan(&presence); err != nil {
		t.Fatal(err)
	}
	if presence.Valid {
		t.Fatal("failed collector erased or marked the previous container observation absent")
	}
	if err := db.QueryRow(`SELECT hostname FROM compute_observations WHERE compute_id=?`, compute.ID).Scan(&currentHostname); err != nil {
		t.Fatal(err)
	}
	if currentHostname != "node-a" {
		t.Fatalf("failed host collection overwrote prior facts with %q", currentHostname)
	}
	emptyReport := "00000000-0000-4000-8000-000000000103"
	emptySnapshot := uploadSnapshot(emptyReport, snapshotBody(emptyReport, "node-a", "success", "success", []any{}))
	if emptySnapshot.Code != http.StatusCreated {
		t.Fatalf("authoritative empty snapshot status = %d: %s", emptySnapshot.Code, emptySnapshot.Body.String())
	}
	if err := db.QueryRow(`SELECT stale_at FROM containers WHERE id=?`, containerID).Scan(&presence); err != nil {
		t.Fatal(err)
	}
	if !presence.Valid {
		t.Fatal("successful authoritative empty Podman inventory did not mark missing container stale")
	}
	containerListRequest := httptest.NewRequest(http.MethodGet, "/api/v1/computes/"+compute.ID+"/containers?stale=true", nil)
	containerListRequest.AddCookie(cookie)
	containerListResponse := httptest.NewRecorder()
	handler.ServeHTTP(containerListResponse, containerListRequest)
	var containerPage struct {
		Items []containerResponse `json:"items"`
	}
	if err := json.NewDecoder(containerListResponse.Body).Decode(&containerPage); err != nil {
		t.Fatal(err)
	}
	if containerListResponse.Code != http.StatusOK || len(containerPage.Items) != 1 || containerPage.Items[0].Name != "web" || containerPage.Items[0].Observation == nil || containerPage.Items[0].Observation.StaleAt == nil {
		t.Fatalf("stale container list = %d %s", containerListResponse.Code, containerListResponse.Body.String())
	}
	containerDetailRequest := httptest.NewRequest(http.MethodGet, "/api/v1/containers/"+containerID, nil)
	containerDetailRequest.AddCookie(cookie)
	containerDetailResponse := httptest.NewRecorder()
	handler.ServeHTTP(containerDetailResponse, containerDetailRequest)
	if containerDetailResponse.Code != http.StatusOK || !strings.Contains(containerDetailResponse.Body.String(), `"runtime_id":"runtime-2"`) || !strings.Contains(containerDetailResponse.Body.String(), `"network_id":"network-1"`) || !strings.Contains(containerDetailResponse.Body.String(), `"host_port":8080`) {
		t.Fatalf("container detail = %d %s", containerDetailResponse.Code, containerDetailResponse.Body.String())
	}
	heartbeat := func(credential string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"protocol_version":1,"client_version":"e2e-agent","sent_at":%q,"last_collection_error":"Podman unavailable"}`, time.Now().UTC().Format(time.RFC3339))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/v1/heartbeat", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+credential)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	if response := heartbeat(enrollment.Credential); response.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d: %s", response.Code, response.Body.String())
	}
	agentDetailRequest := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+enrollment.AgentID, nil)
	agentDetailRequest.AddCookie(cookie)
	agentDetailResponse := httptest.NewRecorder()
	handler.ServeHTTP(agentDetailResponse, agentDetailRequest)
	var agentDetails agentResource
	if err := json.NewDecoder(agentDetailResponse.Body).Decode(&agentDetails); err != nil {
		t.Fatal(err)
	}
	if agentDetailResponse.Code != http.StatusOK || agentDetails.Status != "online" || agentDetails.LastHeartbeatAt == nil {
		t.Fatalf("agent detail = %d %+v", agentDetailResponse.Code, agentDetails)
	}
	computeDetailRequest := httptest.NewRequest(http.MethodGet, "/api/v1/computes/"+compute.ID, nil)
	computeDetailRequest.AddCookie(cookie)
	computeDetailResponse := httptest.NewRecorder()
	handler.ServeHTTP(computeDetailResponse, computeDetailRequest)
	var computeWithAgent computeResource
	if err := json.NewDecoder(computeDetailResponse.Body).Decode(&computeWithAgent); err != nil {
		t.Fatal(err)
	}
	if computeWithAgent.AgentID == nil || *computeWithAgent.AgentID != enrollment.AgentID || computeWithAgent.AgentStatus != "online" {
		t.Fatalf("compute agent state = %+v", computeWithAgent)
	}
	if computeWithAgent.Observation == nil || computeWithAgent.Observation.Hostname != "node-a" || computeWithAgent.Observation.Architecture != "amd64" {
		t.Fatalf("compute host facts are not exposed by detail: %+v", computeWithAgent.Observation)
	}
	staleHeartbeat := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE agents SET enrolled_at=?,last_heartbeat_at=? WHERE id=?`, staleHeartbeat, staleHeartbeat, enrollment.AgentID); err != nil {
		t.Fatal(err)
	}
	computeDetailRequest = httptest.NewRequest(http.MethodGet, "/api/v1/computes/"+compute.ID, nil)
	computeDetailRequest.AddCookie(cookie)
	computeDetailResponse = httptest.NewRecorder()
	handler.ServeHTTP(computeDetailResponse, computeDetailRequest)
	if err := json.NewDecoder(computeDetailResponse.Body).Decode(&computeWithAgent); err != nil {
		t.Fatal(err)
	}
	if computeWithAgent.AgentStatus != "offline" {
		t.Fatalf("stale agent status = %q, want offline", computeWithAgent.AgentStatus)
	}
	if response := heartbeat(enrollment.Credential); response.Code != http.StatusOK {
		t.Fatalf("heartbeat recovery status = %d: %s", response.Code, response.Body.String())
	}
	computeDetailRequest = httptest.NewRequest(http.MethodGet, "/api/v1/computes/"+compute.ID, nil)
	computeDetailRequest.AddCookie(cookie)
	computeDetailResponse = httptest.NewRecorder()
	handler.ServeHTTP(computeDetailResponse, computeDetailRequest)
	if err := json.NewDecoder(computeDetailResponse.Body).Decode(&computeWithAgent); err != nil {
		t.Fatal(err)
	}
	if computeWithAgent.AgentStatus != "online" {
		t.Fatalf("recovered agent status = %q, want online", computeWithAgent.AgentStatus)
	}
	var agentRevision int64
	if err := db.QueryRow(`SELECT revision FROM agents WHERE id=?`, enrollment.AgentID).Scan(&agentRevision); err != nil {
		t.Fatal(err)
	}
	revokeAgent := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+enrollment.AgentID+"/revoke", nil)
	revokeAgent.AddCookie(cookie)
	revokeAgent.Header.Set("X-CSRF-Token", session.CSRFToken)
	revokeAgent.Header.Set("If-Match", revisionETag(agentRevision))
	revokeAgentResponse := httptest.NewRecorder()
	handler.ServeHTTP(revokeAgentResponse, revokeAgent)
	if revokeAgentResponse.Code != http.StatusOK {
		t.Fatalf("agent revoke status = %d: %s", revokeAgentResponse.Code, revokeAgentResponse.Body.String())
	}
	if response := heartbeat(enrollment.Credential); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked credential heartbeat status = %d", response.Code)
	}
	obsoleteToken := createEnrollmentToken()
	revokeToken := httptest.NewRequest(http.MethodDelete, "/api/v1/enrollment-tokens/"+obsoleteToken.ID, nil)
	revokeToken.AddCookie(cookie)
	revokeToken.Header.Set("X-CSRF-Token", session.CSRFToken)
	revokeTokenResponse := httptest.NewRecorder()
	handler.ServeHTTP(revokeTokenResponse, revokeToken)
	if revokeTokenResponse.Code != http.StatusNoContent {
		t.Fatalf("enrollment token revoke status = %d: %s", revokeTokenResponse.Code, revokeTokenResponse.Body.String())
	}
	reenrollmentToken := createEnrollmentToken()
	reenrollResponse := enrollRequest(reenrollmentToken.Token)
	if reenrollResponse.Code != http.StatusCreated {
		t.Fatalf("re-enroll status = %d: %s", reenrollResponse.Code, reenrollResponse.Body.String())
	}
	var reenrollment agentEnrollmentResponse
	if err := json.NewDecoder(reenrollResponse.Body).Decode(&reenrollment); err != nil {
		t.Fatal(err)
	}
	if reenrollment.AgentID != enrollment.AgentID || reenrollment.CredentialGeneration != 2 || reenrollment.Credential == enrollment.Credential {
		t.Fatalf("re-enrollment identity = %+v", reenrollment)
	}
	agentID := enrollment.AgentID
	tokenID, _ := randomID()
	outstandingSecret, _ := randomSecret(32)
	now := time.Now().UTC().Format(time.RFC3339)
	expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO agent_enrollment_tokens(id,compute_id,token_hash,created_at,expires_at,created_by_administrator_id) VALUES(?,?,?,?,?,?)`, tokenID, compute.ID, hashSecret(outstandingSecret), now, expires, session.AdministratorID); err != nil {
		t.Fatal(err)
	}
	archiveCompute := httptest.NewRequest(http.MethodPost, "/api/v1/computes/"+compute.ID+"/archive", nil)
	archiveCompute.AddCookie(cookie)
	archiveCompute.Header.Set("X-CSRF-Token", session.CSRFToken)
	archiveCompute.Header.Set("If-Match", computeETag)
	archiveComputeResponse := httptest.NewRecorder()
	handler.ServeHTTP(archiveComputeResponse, archiveCompute)
	if archiveComputeResponse.Code != http.StatusOK {
		t.Fatalf("compute archive status = %d: %s", archiveComputeResponse.Code, archiveComputeResponse.Body.String())
	}
	var archivedCompute computeResource
	if err := json.NewDecoder(archiveComputeResponse.Body).Decode(&archivedCompute); err != nil {
		t.Fatal(err)
	}
	if archivedCompute.AgentStatus != "revoked" || archivedCompute.ArchivedAt == nil {
		t.Fatalf("archived compute state = %+v", archivedCompute)
	}
	var agentRevoked, tokenRevoked string
	if err := db.QueryRow(`SELECT revoked_at FROM agents WHERE id=?`, agentID).Scan(&agentRevoked); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT revoked_at FROM agent_enrollment_tokens WHERE id=?`, tokenID).Scan(&tokenRevoked); err != nil {
		t.Fatal(err)
	}
	if agentRevoked == "" || tokenRevoked == "" {
		t.Fatal("compute archive did not revoke discovery credentials")
	}
	if response := heartbeat(reenrollment.Credential); response.Code != http.StatusUnauthorized {
		t.Fatalf("archived compute credential heartbeat status = %d", response.Code)
	}
	computeArchiveETag := archiveComputeResponse.Header().Get("ETag")
	restoreCompute := httptest.NewRequest(http.MethodPost, "/api/v1/computes/"+compute.ID+"/restore", nil)
	restoreCompute.AddCookie(cookie)
	restoreCompute.Header.Set("X-CSRF-Token", session.CSRFToken)
	restoreCompute.Header.Set("If-Match", computeArchiveETag)
	restoreComputeResponse := httptest.NewRecorder()
	handler.ServeHTTP(restoreComputeResponse, restoreCompute)
	if restoreComputeResponse.Code != http.StatusOK {
		t.Fatalf("compute restore status = %d: %s", restoreComputeResponse.Code, restoreComputeResponse.Body.String())
	}
	var restoredCompute computeResource
	if err := json.NewDecoder(restoreComputeResponse.Body).Decode(&restoredCompute); err != nil {
		t.Fatal(err)
	}
	if restoredCompute.AgentStatus != "revoked" || restoredCompute.ArchivedAt != nil {
		t.Fatalf("restored compute state = %+v", restoredCompute)
	}
	if err := db.QueryRow(`SELECT revoked_at FROM agents WHERE id=?`, agentID).Scan(&agentRevoked); err != nil {
		t.Fatal(err)
	}
	if agentRevoked == "" {
		t.Fatal("compute restore revived its old agent credential")
	}
	archiveIP := httptest.NewRequest(http.MethodPost, "/api/v1/ip-assignments/"+assignment.ID+"/archive", nil)
	archiveIP.AddCookie(cookie)
	archiveIP.Header.Set("X-CSRF-Token", session.CSRFToken)
	archiveIP.Header.Set("If-Match", updateIPResponse.Header().Get("ETag"))
	archiveIPResponse := httptest.NewRecorder()
	handler.ServeHTTP(archiveIPResponse, archiveIP)
	if archiveIPResponse.Code != http.StatusOK {
		t.Fatalf("IP archive status = %d: %s", archiveIPResponse.Code, archiveIPResponse.Body.String())
	}
	restoreIP := httptest.NewRequest(http.MethodPost, "/api/v1/ip-assignments/"+assignment.ID+"/restore", nil)
	restoreIP.AddCookie(cookie)
	restoreIP.Header.Set("X-CSRF-Token", session.CSRFToken)
	restoreIP.Header.Set("If-Match", archiveIPResponse.Header().Get("ETag"))
	restoreIPResponse := httptest.NewRecorder()
	handler.ServeHTTP(restoreIPResponse, restoreIP)
	if restoreIPResponse.Code != http.StatusOK {
		t.Fatalf("IP restore status = %d: %s", restoreIPResponse.Code, restoreIPResponse.Body.String())
	}

	archiveLocation := httptest.NewRequest(http.MethodPost, "/api/v1/locations/"+location.ID+"/archive", nil)
	archiveLocation.AddCookie(cookie)
	archiveLocation.Header.Set("X-CSRF-Token", session.CSRFToken)
	archiveLocation.Header.Set("If-Match", locationETag)
	archiveLocationResponse := httptest.NewRecorder()
	handler.ServeHTTP(archiveLocationResponse, archiveLocation)
	if archiveLocationResponse.Code != http.StatusOK {
		t.Fatalf("location archive status = %d: %s", archiveLocationResponse.Code, archiveLocationResponse.Body.String())
	}
	var activeComputeArchived sql.NullString
	if err := db.QueryRow(`SELECT archived_at FROM computes WHERE id=?`, compute.ID).Scan(&activeComputeArchived); err != nil {
		t.Fatal(err)
	}
	if activeComputeArchived.Valid {
		t.Fatal("location archive recursively archived its compute")
	}
	restoreLocation := httptest.NewRequest(http.MethodPost, "/api/v1/locations/"+location.ID+"/restore", nil)
	restoreLocation.AddCookie(cookie)
	restoreLocation.Header.Set("X-CSRF-Token", session.CSRFToken)
	restoreLocation.Header.Set("If-Match", archiveLocationResponse.Header().Get("ETag"))
	restoreLocationResponse := httptest.NewRecorder()
	handler.ServeHTTP(restoreLocationResponse, restoreLocation)
	if restoreLocationResponse.Code != http.StatusOK {
		t.Fatalf("location restore status = %d: %s", restoreLocationResponse.Code, restoreLocationResponse.Body.String())
	}

	archiveProvider := httptest.NewRequest(http.MethodPost, "/api/v1/providers/"+provider.ID+"/archive", nil)
	archiveProvider.AddCookie(cookie)
	archiveProvider.Header.Set("X-CSRF-Token", session.CSRFToken)
	archiveProvider.Header.Set("If-Match", providerETag)
	archiveProviderResponse := httptest.NewRecorder()
	handler.ServeHTTP(archiveProviderResponse, archiveProvider)
	if archiveProviderResponse.Code != http.StatusOK {
		t.Fatalf("provider archive status = %d: %s", archiveProviderResponse.Code, archiveProviderResponse.Body.String())
	}
	staleProviderArchive := httptest.NewRequest(http.MethodPost, "/api/v1/providers/"+provider.ID+"/archive", nil)
	staleProviderArchive.AddCookie(cookie)
	staleProviderArchive.Header.Set("X-CSRF-Token", session.CSRFToken)
	staleProviderArchive.Header.Set("If-Match", createResponse.Header().Get("ETag"))
	staleProviderArchiveResponse := httptest.NewRecorder()
	handler.ServeHTTP(staleProviderArchiveResponse, staleProviderArchive)
	if staleProviderArchiveResponse.Code != http.StatusPreconditionFailed {
		t.Fatalf("stale archive status = %d", staleProviderArchiveResponse.Code)
	}
	restoreProvider := httptest.NewRequest(http.MethodPost, "/api/v1/providers/"+provider.ID+"/restore", nil)
	restoreProvider.AddCookie(cookie)
	restoreProvider.Header.Set("X-CSRF-Token", session.CSRFToken)
	restoreProvider.Header.Set("If-Match", archiveProviderResponse.Header().Get("ETag"))
	restoreProviderResponse := httptest.NewRecorder()
	handler.ServeHTTP(restoreProviderResponse, restoreProvider)
	if restoreProviderResponse.Code != http.StatusOK {
		t.Fatalf("provider restore status = %d: %s", restoreProviderResponse.Code, restoreProviderResponse.Body.String())
	}

	createVM := httptest.NewRequest(http.MethodPost, "/api/v1/computes", strings.NewReader(`{"kind":"vm","display_name":"Test VM","ownership":"owned","parent_compute_id":"`+compute.ID+`","tag_ids":[]}`))
	createVM.AddCookie(cookie)
	createVM.Header.Set("Content-Type", "application/json")
	createVM.Header.Set("X-CSRF-Token", session.CSRFToken)
	createVMResponse := httptest.NewRecorder()
	handler.ServeHTTP(createVMResponse, createVM)
	if createVMResponse.Code != http.StatusCreated {
		t.Fatalf("VM create status = %d: %s", createVMResponse.Code, createVMResponse.Body.String())
	}
	var vm computeResource
	if err := json.NewDecoder(createVMResponse.Body).Decode(&vm); err != nil {
		t.Fatal(err)
	}
	computeCycle := httptest.NewRequest(http.MethodPut, "/api/v1/computes/"+vm.ID, strings.NewReader(`{"kind":"vm","display_name":"Test VM","ownership":"owned","parent_compute_id":"`+vm.ID+`","tag_ids":[]}`))
	computeCycle.AddCookie(cookie)
	computeCycle.Header.Set("Content-Type", "application/json")
	computeCycle.Header.Set("X-CSRF-Token", session.CSRFToken)
	computeCycle.Header.Set("If-Match", createVMResponse.Header().Get("ETag"))
	computeCycleResponse := httptest.NewRecorder()
	handler.ServeHTTP(computeCycleResponse, computeCycle)
	if computeCycleResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("compute cycle update status = %d: %s", computeCycleResponse.Code, computeCycleResponse.Body.String())
	}
	createNestedVM := httptest.NewRequest(http.MethodPost, "/api/v1/computes", strings.NewReader(`{"kind":"vm","display_name":"Nested active VM","ownership":"owned","parent_compute_id":"`+vm.ID+`","tag_ids":[]}`))
	createNestedVM.AddCookie(cookie)
	createNestedVM.Header.Set("Content-Type", "application/json")
	createNestedVM.Header.Set("X-CSRF-Token", session.CSRFToken)
	createNestedVMResponse := httptest.NewRecorder()
	handler.ServeHTTP(createNestedVMResponse, createNestedVM)
	if createNestedVMResponse.Code != http.StatusCreated {
		t.Fatalf("nested VM create status = %d: %s", createNestedVMResponse.Code, createNestedVMResponse.Body.String())
	}
	var nestedVM computeResource
	if err := json.NewDecoder(createNestedVMResponse.Body).Decode(&nestedVM); err != nil {
		t.Fatal(err)
	}
	archiveParentVM := httptest.NewRequest(http.MethodPost, "/api/v1/computes/"+vm.ID+"/archive", nil)
	archiveParentVM.AddCookie(cookie)
	archiveParentVM.Header.Set("X-CSRF-Token", session.CSRFToken)
	archiveParentVM.Header.Set("If-Match", createVMResponse.Header().Get("ETag"))
	archiveParentVMResponse := httptest.NewRecorder()
	handler.ServeHTTP(archiveParentVMResponse, archiveParentVM)
	if archiveParentVMResponse.Code != http.StatusOK {
		t.Fatalf("archive nested compute parent status = %d: %s", archiveParentVMResponse.Code, archiveParentVMResponse.Body.String())
	}
	archiveParentCompute := httptest.NewRequest(http.MethodPost, "/api/v1/computes/"+compute.ID+"/archive", nil)
	archiveParentCompute.AddCookie(cookie)
	archiveParentCompute.Header.Set("X-CSRF-Token", session.CSRFToken)
	archiveParentCompute.Header.Set("If-Match", restoreComputeResponse.Header().Get("ETag"))
	archiveParentResponse := httptest.NewRecorder()
	handler.ServeHTTP(archiveParentResponse, archiveParentCompute)
	if archiveParentResponse.Code != http.StatusOK {
		t.Fatalf("archive parent compute status = %d: %s", archiveParentResponse.Code, archiveParentResponse.Body.String())
	}
	parentContextRequest := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/tree/"+location.ID+"/children", nil)
	parentContextRequest.AddCookie(cookie)
	parentContextResponse := httptest.NewRecorder()
	handler.ServeHTTP(parentContextResponse, parentContextRequest)
	if parentContextResponse.Code != http.StatusOK || !strings.Contains(parentContextResponse.Body.String(), compute.ID) {
		t.Fatalf("archived compute ancestor context status = %d: %s", parentContextResponse.Code, parentContextResponse.Body.String())
	}
	grandchildRequest := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/tree/"+compute.ID+"/children", nil)
	grandchildRequest.AddCookie(cookie)
	grandchildResponse := httptest.NewRecorder()
	handler.ServeHTTP(grandchildResponse, grandchildRequest)
	var archivedComputeChildren treePage
	if err := json.NewDecoder(grandchildResponse.Body).Decode(&archivedComputeChildren); err != nil {
		t.Fatal(err)
	}
	if grandchildResponse.Code != http.StatusOK {
		t.Fatalf("archived compute children status = %d", grandchildResponse.Code)
	}
	archivedVMContextFound, containerContextFound := false, false
	for _, node := range archivedComputeChildren.Items {
		if node.ID == vm.ID && node.Archived && node.HasChildren {
			archivedVMContextFound = true
		}
		if node.ID == containerID && node.Kind == "container" {
			containerContextFound = true
		}
	}
	if !archivedVMContextFound || !containerContextFound {
		t.Fatalf("nested archived compute/container context = %+v", archivedComputeChildren.Items)
	}
	leafRequest := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/tree/"+vm.ID+"/children", nil)
	leafRequest.AddCookie(cookie)
	leafResponse := httptest.NewRecorder()
	handler.ServeHTTP(leafResponse, leafRequest)
	if leafResponse.Code != http.StatusOK || !strings.Contains(leafResponse.Body.String(), nestedVM.ID) {
		t.Fatalf("active VM unreachable under nested archived ancestors: %d %s", leafResponse.Code, leafResponse.Body.String())
	}
	restoreParentCompute := httptest.NewRequest(http.MethodPost, "/api/v1/computes/"+compute.ID+"/restore", nil)
	restoreParentCompute.AddCookie(cookie)
	restoreParentCompute.Header.Set("X-CSRF-Token", session.CSRFToken)
	restoreParentCompute.Header.Set("If-Match", archiveParentResponse.Header().Get("ETag"))
	restoreParentResponse := httptest.NewRecorder()
	handler.ServeHTTP(restoreParentResponse, restoreParentCompute)
	if restoreParentResponse.Code != http.StatusOK {
		t.Fatalf("restore parent compute status = %d: %s", restoreParentResponse.Code, restoreParentResponse.Body.String())
	}
	restoreParentVM := httptest.NewRequest(http.MethodPost, "/api/v1/computes/"+vm.ID+"/restore", nil)
	restoreParentVM.AddCookie(cookie)
	restoreParentVM.Header.Set("X-CSRF-Token", session.CSRFToken)
	restoreParentVM.Header.Set("If-Match", archiveParentVMResponse.Header().Get("ETag"))
	restoreParentVMResponse := httptest.NewRecorder()
	handler.ServeHTTP(restoreParentVMResponse, restoreParentVM)
	if restoreParentVMResponse.Code != http.StatusOK {
		t.Fatalf("restore nested compute parent status = %d: %s", restoreParentVMResponse.Code, restoreParentVMResponse.Body.String())
	}
	createVPS := httptest.NewRequest(http.MethodPost, "/api/v1/computes", strings.NewReader(`{"kind":"vps","display_name":"Standalone VPS","ownership":"rented","tag_ids":[]}`))
	createVPS.AddCookie(cookie)
	createVPS.Header.Set("Content-Type", "application/json")
	createVPS.Header.Set("X-CSRF-Token", session.CSRFToken)
	createVPSResponse := httptest.NewRecorder()
	handler.ServeHTTP(createVPSResponse, createVPS)
	if createVPSResponse.Code != http.StatusCreated {
		t.Fatalf("VPS create status = %d: %s", createVPSResponse.Code, createVPSResponse.Body.String())
	}

	computeList := httptest.NewRequest(http.MethodGet, "/api/v1/computes", nil)
	computeList.AddCookie(cookie)
	computeListResponse := httptest.NewRecorder()
	handler.ServeHTTP(computeListResponse, computeList)
	if computeListResponse.Code != http.StatusOK || !strings.Contains(computeListResponse.Body.String(), "Test VM") {
		t.Fatalf("compute list status = %d: %s", computeListResponse.Code, computeListResponse.Body.String())
	}
	treeRequest := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/tree", nil)
	treeRequest.AddCookie(cookie)
	treeResponse := httptest.NewRecorder()
	handler.ServeHTTP(treeResponse, treeRequest)
	if treeResponse.Code != http.StatusOK || !strings.Contains(treeResponse.Body.String(), "Updated Datacenter") {
		t.Fatalf("inventory roots status = %d: %s", treeResponse.Code, treeResponse.Body.String())
	}
	firstRootPage := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/tree?page[size]=2", nil)
	firstRootPage.AddCookie(cookie)
	firstRootResponse := httptest.NewRecorder()
	handler.ServeHTTP(firstRootResponse, firstRootPage)
	var firstTreePage struct {
		Items []treeNode `json:"items"`
		Page  struct {
			HasMore    bool    `json:"has_more"`
			NextCursor *string `json:"next_cursor"`
		} `json:"page"`
	}
	if err := json.NewDecoder(firstRootResponse.Body).Decode(&firstTreePage); err != nil {
		t.Fatal(err)
	}
	if firstRootResponse.Code != http.StatusOK || len(firstTreePage.Items) != 2 || !firstTreePage.Page.HasMore || firstTreePage.Page.NextCursor == nil {
		t.Fatalf("first root page = %s", firstRootResponse.Body.String())
	}
	changedArchiveFilter := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/tree?archived=include&page[after]="+*firstTreePage.Page.NextCursor, nil)
	changedArchiveFilter.AddCookie(cookie)
	changedArchiveResponse := httptest.NewRecorder()
	handler.ServeHTTP(changedArchiveResponse, changedArchiveFilter)
	if changedArchiveResponse.Code != http.StatusBadRequest {
		t.Fatalf("tree cursor reused under a different archive filter = %d %s", changedArchiveResponse.Code, changedArchiveResponse.Body.String())
	}
	secondRootPage := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/tree?page[size]=2&page[after]="+*firstTreePage.Page.NextCursor, nil)
	secondRootPage.AddCookie(cookie)
	secondRootResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondRootResponse, secondRootPage)
	var secondTreePage struct {
		Items []treeNode `json:"items"`
		Page  struct {
			HasMore bool `json:"has_more"`
		} `json:"page"`
	}
	if err := json.NewDecoder(secondRootResponse.Body).Decode(&secondTreePage); err != nil {
		t.Fatal(err)
	}
	if secondRootResponse.Code != http.StatusOK || len(secondTreePage.Items) != 1 || firstTreePage.Items[0].ID == secondTreePage.Items[0].ID || firstTreePage.Items[1].ID == secondTreePage.Items[0].ID || secondTreePage.Page.HasMore {
		t.Fatalf("second root page = %s", secondRootResponse.Body.String())
	}
	childrenRequest := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/tree/"+location.ID+"/children", nil)
	childrenRequest.AddCookie(cookie)
	childrenResponse := httptest.NewRecorder()
	handler.ServeHTTP(childrenResponse, childrenRequest)
	if childrenResponse.Code != http.StatusOK || !strings.Contains(childrenResponse.Body.String(), "Updated Host") {
		t.Fatalf("location children status = %d: %s", childrenResponse.Code, childrenResponse.Body.String())
	}
	archiveLocationAgain := httptest.NewRequest(http.MethodPost, "/api/v1/locations/"+location.ID+"/archive", nil)
	archiveLocationAgain.AddCookie(cookie)
	archiveLocationAgain.Header.Set("X-CSRF-Token", session.CSRFToken)
	archiveLocationAgain.Header.Set("If-Match", restoreLocationResponse.Header().Get("ETag"))
	archiveLocationAgainResponse := httptest.NewRecorder()
	handler.ServeHTTP(archiveLocationAgainResponse, archiveLocationAgain)
	if archiveLocationAgainResponse.Code != http.StatusOK {
		t.Fatalf("second location archive status = %d: %s", archiveLocationAgainResponse.Code, archiveLocationAgainResponse.Body.String())
	}
	archivedContextRequest := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/tree", nil)
	archivedContextRequest.AddCookie(cookie)
	archivedContextResponse := httptest.NewRecorder()
	handler.ServeHTTP(archivedContextResponse, archivedContextRequest)
	var archivedContextPage treePage
	if err := json.NewDecoder(archivedContextResponse.Body).Decode(&archivedContextPage); err != nil {
		t.Fatal(err)
	}
	contextFound := false
	for _, node := range archivedContextPage.Items {
		if node.ID == location.ID && node.Archived {
			contextFound = true
		}
	}
	if !contextFound {
		t.Fatalf("archived location ancestor missing from active tree context: %+v", archivedContextPage.Items)
	}
	contextChildrenRequest := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/tree/"+location.ID+"/children", nil)
	contextChildrenRequest.AddCookie(cookie)
	contextChildrenResponse := httptest.NewRecorder()
	handler.ServeHTTP(contextChildrenResponse, contextChildrenRequest)
	if contextChildrenResponse.Code != http.StatusOK || !strings.Contains(contextChildrenResponse.Body.String(), compute.ID) {
		t.Fatalf("archived context children status = %d: %s", contextChildrenResponse.Code, contextChildrenResponse.Body.String())
	}
	if summaryResponse := httptest.NewRecorder(); func() int {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/summary", nil)
		request.AddCookie(cookie)
		handler.ServeHTTP(summaryResponse, request)
		return summaryResponse.Code
	}() != http.StatusOK || !strings.Contains(summaryResponse.Body.String(), `"compute_count":4`) {
		t.Fatalf("archived ancestor changed active compute summary: %d %s", summaryResponse.Code, summaryResponse.Body.String())
	}
	summaryRequest := httptest.NewRequest(http.MethodGet, "/api/v1/computes/"+compute.ID+"/summary", nil)
	summaryRequest.AddCookie(cookie)
	summaryResponse := httptest.NewRecorder()
	handler.ServeHTTP(summaryResponse, summaryRequest)
	if summaryResponse.Code != http.StatusOK || !strings.Contains(summaryResponse.Body.String(), `"container_count":1`) || !strings.Contains(summaryResponse.Body.String(), `"stale_container_count":1`) {
		t.Fatalf("compute summary status = %d: %s", summaryResponse.Code, summaryResponse.Body.String())
	}
	inventorySummaryRequest := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/summary", nil)
	inventorySummaryRequest.AddCookie(cookie)
	inventorySummaryResponse := httptest.NewRecorder()
	handler.ServeHTTP(inventorySummaryResponse, inventorySummaryRequest)
	if inventorySummaryResponse.Code != http.StatusOK || !strings.Contains(inventorySummaryResponse.Body.String(), `"compute_count":4`) || !strings.Contains(inventorySummaryResponse.Body.String(), `"vps_count":1`) {
		t.Fatalf("inventory summary status = %d: %s", inventorySummaryResponse.Code, inventorySummaryResponse.Body.String())
	}

	read := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	read.AddCookie(cookie)
	readResponse := httptest.NewRecorder()
	handler.ServeHTTP(readResponse, read)
	if readResponse.Code != http.StatusOK {
		t.Fatalf("session status = %d", readResponse.Code)
	}

	logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logout.AddCookie(cookie)
	logout.Header.Set("X-CSRF-Token", session.CSRFToken)
	logoutResponse := httptest.NewRecorder()
	handler.ServeHTTP(logoutResponse, logout)
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", logoutResponse.Code)
	}

	readAfterLogout := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	readAfterLogout.AddCookie(cookie)
	responseAfterLogout := httptest.NewRecorder()
	handler.ServeHTTP(responseAfterLogout, readAfterLogout)
	if responseAfterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d", responseAfterLogout.Code)
	}
}

func TestPasswordResetRevokesSessionsAndIsAudited(t *testing.T) {
	db, err := database.Open(t.TempDir() + "/password-reset.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapAdministrator(db, "admin", "old-password-is-long"); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(db)
	login := func(password string) *http.Cookie {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"`+password+`"}`))
		req.Header.Set("Origin", "http://example.com")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("login with supplied password returned %d: %s", resp.Code, resp.Body.String())
		}
		return resp.Result().Cookies()[0]
	}
	first, second := login("old-password-is-long"), login("old-password-is-long")
	if err := ResetAdministratorPassword(db, "admin", "new-password-is-long"); err != nil {
		t.Fatal(err)
	}
	for _, cookie := range []*http.Cookie{first, second} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
		req.AddCookie(cookie)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("pre-reset session status = %d", resp.Code)
		}
	}
	login("new-password-is-long")
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM audit_events WHERE action='administrator.password_reset' AND result='success'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("password reset audit count = %d", count)
	}
}

func TestInventoryMutationAuthenticationAndAuditRollback(t *testing.T) {
	db, err := database.Open(t.TempDir() + "/mutation-security.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(db)
	unauthorized := httptest.NewRequest(http.MethodPost, "/api/v1/providers", strings.NewReader(`{"name":"Blocked","roles":["hosting_provider"],"tag_ids":[]}`))
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated write status = %d", unauthorizedResponse.Code)
	}
	if err := BootstrapAdministrator(db, "admin", "a-long-security-password"); err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"a-long-security-password"}`))
	login.Header.Set("Origin", "http://example.com")
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	var session sessionInfo
	if err := json.NewDecoder(loginResponse.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	cookie := loginResponse.Result().Cookies()[0]
	missingCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/providers", strings.NewReader(`{"name":"Blocked","roles":["hosting_provider"],"tag_ids":[]}`))
	missingCSRF.AddCookie(cookie)
	missingCSRFResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRFResponse, missingCSRF)
	if missingCSRFResponse.Code != http.StatusForbidden {
		t.Fatalf("missing-CSRF write status = %d", missingCSRFResponse.Code)
	}
	if _, err := db.Exec(`CREATE TRIGGER force_provider_audit_failure BEFORE INSERT ON audit_events WHEN NEW.action='provider.create' BEGIN SELECT RAISE(ABORT,'forced audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	write := httptest.NewRequest(http.MethodPost, "/api/v1/providers", strings.NewReader(`{"name":"Rolled Back","roles":["hosting_provider"],"tag_ids":[]}`))
	write.AddCookie(cookie)
	write.Header.Set("Content-Type", "application/json")
	write.Header.Set("X-CSRF-Token", session.CSRFToken)
	writeResponse := httptest.NewRecorder()
	handler.ServeHTTP(writeResponse, write)
	if writeResponse.Code != http.StatusInternalServerError {
		t.Fatalf("audit failure response = %d", writeResponse.Code)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM providers WHERE name='Rolled Back'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("provider persisted despite transactional audit failure")
	}
}
